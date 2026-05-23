// Package mysql implements the restore-drill provider for MySQL/MariaDB databases.
package mysql

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/backup"
	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// Provider implements engine.Provider for MySQL/MariaDB.
type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string {
	return "mysql"
}

func (p *Provider) Preflight(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, checks []engine.Check) error {
	required := []string{"mysql", "mysqladmin"}
	switch cfg.Tool {
	case "xtrabackup":
		required = append(required, "xtrabackup")
	case "mariabackup":
		required = append(required, "mariabackup")
	}
	for _, cmd := range required {
		if err := commandExists(ctx, rt, target, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) Restore(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	start := time.Now()

	switch cfg.Tool {
	case "mysqldump":
		return p.restoreMysqldump(ctx, rt, cfg, target, start)
	case "xtrabackup":
		return p.restoreXtrabackup(ctx, rt, cfg, target, start)
	case "mariabackup":
		return p.restoreMariabackup(ctx, rt, cfg, target, start)
	default:
		return nil, fmt.Errorf("mysql: unsupported backup tool %q", cfg.Tool)
	}
}

func (p *Provider) Validate(ctx context.Context, rt engine.Runtime, target engine.Container, checks []engine.Check) (*engine.ValidationResult, error) {
	result := &engine.ValidationResult{}

	for _, check := range checks {
		cr := engine.CheckResult{
			Name:     check.Name,
			Type:     check.Type,
			Expected: check.Expect,
		}

		start := time.Now()
		var actual string
		var err error

		switch check.Type {
		case "query", "sql", "row_count", "schema", "freshness":
			actual, err = p.execSQL(ctx, rt, target, check.SQL)
		default:
			err = fmt.Errorf("mysql: unsupported check type %q", check.Type)
		}

		cr.Duration = time.Since(start)
		if err != nil {
			cr.Error = err
		} else {
			cr.Actual = strings.TrimSpace(actual)
		}

		result.Checks = append(result.Checks, cr)
	}

	return result, nil
}

func (p *Provider) Cleanup(ctx context.Context, target engine.Container) error {
	return nil
}

func (p *Provider) execSQL(ctx context.Context, rt engine.Runtime, target engine.Container, sql string) (string, error) {
	out, err := rt.Exec(ctx, target, []string{
		"mysql", "-u", "root", "-N", "-B", "-e", sql,
	})
	if err != nil {
		return "", fmt.Errorf("mysql exec: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (p *Provider) waitReady(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, err := rt.Exec(ctx, target, []string{
			"mysqladmin", "-u", "root", "ping",
		})
		if err == nil {
			slog.Debug("mysql is ready")
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("mysql: timeout waiting for database to accept connections")
}

func (p *Provider) restoreMysqldump(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	slog.Info("restoring via mysqldump", "source", cfg.Source)

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}

	restoreCmd := fmt.Sprintf("mysql -u root < %s", shellQuote(staged.Path))
	if strings.HasSuffix(staged.Path, ".gz") {
		restoreCmd = fmt.Sprintf("gzip -dc %s | mysql -u root", shellQuote(staged.Path))
	}

	cmd := []string{"sh", "-c", restoreCmd}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("mysql restore: %w\noutput: %s", err, string(out))
	}

	return &engine.RestoreResult{
		Duration: time.Since(start).Milliseconds(),
	}, nil
}

func (p *Provider) restoreXtrabackup(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	slog.Info("restoring via xtrabackup", "source", cfg.Source)
	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}

	// Prepare the backup
	cmd := []string{"xtrabackup", "--prepare", "--target-dir", staged.Path}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("xtrabackup prepare: %w\noutput: %s", err, string(out))
	}

	_, _ = rt.Exec(ctx, target, []string{"mysqladmin", "-u", "root", "shutdown"})
	_, _ = rt.Exec(ctx, target, []string{"sh", "-c", "rm -rf /var/lib/mysql/*"})

	// Copy back
	cmd = []string{"xtrabackup", "--copy-back", "--target-dir", staged.Path, "--datadir", "/var/lib/mysql"}
	out, err = rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("xtrabackup copy-back: %w\noutput: %s", err, string(out))
	}

	// Fix ownership
	_, _ = rt.Exec(ctx, target, []string{"chown", "-R", "mysql:mysql", "/var/lib/mysql"})

	// Start MySQL
	if _, err = rt.Exec(ctx, target, []string{"sh", "-c", "mysqld_safe --user=mysql >/tmp/mysqld_safe.log 2>&1 &"}); err != nil {
		return nil, fmt.Errorf("start mysql after xtrabackup restore: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	return &engine.RestoreResult{
		Duration: time.Since(start).Milliseconds(),
	}, nil
}

func (p *Provider) restoreMariabackup(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	slog.Info("restoring via mariabackup", "source", cfg.Source)
	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}

	// Prepare
	cmd := []string{"mariabackup", "--prepare", "--target-dir", staged.Path}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("mariabackup prepare: %w\noutput: %s", err, string(out))
	}

	// Copy back
	_, _ = rt.Exec(ctx, target, []string{"mysqladmin", "-u", "root", "shutdown"})
	_, _ = rt.Exec(ctx, target, []string{"sh", "-c", "rm -rf /var/lib/mysql/*"})

	cmd = []string{"mariabackup", "--copy-back", "--target-dir", staged.Path, "--datadir", "/var/lib/mysql"}
	out, err = rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("mariabackup copy-back: %w\noutput: %s", err, string(out))
	}

	_, _ = rt.Exec(ctx, target, []string{"chown", "-R", "mysql:mysql", "/var/lib/mysql"})

	if _, err = rt.Exec(ctx, target, []string{"sh", "-c", "mysqld_safe --user=mysql >/tmp/mysqld_safe.log 2>&1 &"}); err != nil {
		return nil, fmt.Errorf("start mysql after mariabackup restore: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	return &engine.RestoreResult{
		Duration: time.Since(start).Milliseconds(),
	}, nil
}

func commandExists(ctx context.Context, rt engine.Runtime, target engine.Container, name string) error {
	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", "command -v " + shellQuote(name)}); err != nil {
		return fmt.Errorf("required command %q not found in restore image", name)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
