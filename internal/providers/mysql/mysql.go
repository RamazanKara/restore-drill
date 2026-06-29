// Package mysql implements the restore-drill provider for MySQL/MariaDB databases.
package mysql

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/RamazanKara/restore-drill/internal/backup"
	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
	"github.com/RamazanKara/restore-drill/internal/providers/targetcmd"
)

// Provider implements engine.Provider for MySQL/MariaDB.
type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string {
	return "mysql"
}

func (p *Provider) Preflight(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container, checks []config.Check) error {
	if err := targetcmd.CommandExistsAny(ctx, rt, target, "mysql client", "mysql", "mariadb"); err != nil {
		return err
	}
	if err := targetcmd.CommandExistsAny(ctx, rt, target, "mysql admin client", "mysqladmin", "mariadb-admin"); err != nil {
		return err
	}

	required := make([]string, 0)
	switch cfg.Tool {
	case "mysqldump":
		if strings.HasSuffix(targetcmd.ConfiguredBackupPath(cfg), ".gz") {
			required = append(required, "gzip")
		}
	case "xtrabackup":
		required = append(required, "xtrabackup")
		if err := targetcmd.CommandExistsAny(ctx, rt, target, "mysql safe launcher", "mysqld_safe", "mariadbd-safe"); err != nil {
			return err
		}
		required = append(required, backup.ArchiveRequirements(targetcmd.ConfiguredBackupPath(cfg))...)
	case "mariabackup":
		required = append(required, "mariabackup")
		if err := targetcmd.CommandExistsAny(ctx, rt, target, "mysql safe launcher", "mysqld_safe", "mariadbd-safe"); err != nil {
			return err
		}
		required = append(required, backup.ArchiveRequirements(targetcmd.ConfiguredBackupPath(cfg))...)
	}
	for _, cmd := range required {
		if err := targetcmd.CommandExists(ctx, rt, target, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) Restore(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	switch cfg.Tool {
	case "mysqldump":
		return p.restoreMysqldump(ctx, rt, cfg, target)
	case "xtrabackup":
		return p.restoreXtrabackup(ctx, rt, cfg, target)
	case "mariabackup":
		return p.restoreMariabackup(ctx, rt, cfg, target)
	default:
		return nil, fmt.Errorf("mysql: unsupported backup tool %q", cfg.Tool)
	}
}

func (p *Provider) Validate(ctx context.Context, rt engine.Runtime, target engine.Container, checks []config.Check) (*engine.ValidationResult, error) {
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
	client, err := targetcmd.FirstAvailableCommand(ctx, rt, target, "mysql", "mariadb")
	if err != nil {
		return "", err
	}
	out, err := rt.Exec(ctx, target, []string{
		client, "-u", "root", "-N", "-B", "-e", sql,
	})
	if err != nil {
		return "", fmt.Errorf("mysql exec: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (p *Provider) waitReady(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	deadline := time.Now().Add(60 * time.Second)
	admin, err := targetcmd.FirstAvailableCommand(ctx, rt, target, "mysqladmin", "mariadb-admin")
	if err != nil {
		return err
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, err := rt.Exec(ctx, target, []string{
			admin, "-u", "root", "ping",
		})
		if err == nil {
			slog.Debug("mysql is ready")
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("mysql: timeout waiting for database to accept connections")
}

func (p *Provider) restoreMysqldump(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring via mysqldump", "source", cfg.Source)

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}

	client, err := targetcmd.FirstAvailableCommand(ctx, rt, target, "mysql", "mariadb")
	if err != nil {
		return nil, err
	}

	restoreCmd := fmt.Sprintf("%s -u root < %s", targetcmd.ShellQuote(client), targetcmd.ShellQuote(staged.Path))
	if strings.HasSuffix(staged.Path, ".gz") {
		restoreCmd = fmt.Sprintf("gzip -dc %s | %s -u root", targetcmd.ShellQuote(staged.Path), targetcmd.ShellQuote(client))
	}

	cmd := []string{"sh", "-c", restoreCmd}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("mysql restore: %w\noutput: %s", err, string(out))
	}

	return &engine.RestoreResult{}, nil
}

func (p *Provider) restoreXtrabackup(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring via xtrabackup", "source", cfg.Source)
	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}
	backupDir, err := backup.MaterializeArchive(ctx, rt, target, staged.Path, "/tmp/restore-drill-backups/mysql-physical")
	if err != nil {
		return nil, err
	}

	// Prepare the backup
	cmd := []string{"xtrabackup", "--prepare", "--target-dir", backupDir}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("xtrabackup prepare: %w\noutput: %s", err, string(out))
	}

	if err := p.stopMySQL(ctx, rt, target); err != nil {
		return nil, err
	}
	_, _ = rt.Exec(ctx, target, []string{"sh", "-c", "rm -rf /var/lib/mysql/*"})

	// Copy back
	cmd = []string{"xtrabackup", "--copy-back", "--target-dir", backupDir, "--datadir", "/var/lib/mysql"}
	out, err = rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("xtrabackup copy-back: %w\noutput: %s", err, string(out))
	}

	// Fix ownership
	_, _ = rt.Exec(ctx, target, []string{"chown", "-R", "mysql:mysql", "/var/lib/mysql"})

	// Start MySQL
	if err := p.startMySQL(ctx, rt, target); err != nil {
		return nil, fmt.Errorf("start mysql after xtrabackup restore: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	return &engine.RestoreResult{}, nil
}

func (p *Provider) restoreMariabackup(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring via mariabackup", "source", cfg.Source)
	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}
	backupDir, err := backup.MaterializeArchive(ctx, rt, target, staged.Path, "/tmp/restore-drill-backups/mysql-physical")
	if err != nil {
		return nil, err
	}

	// Prepare
	cmd := []string{"mariabackup", "--prepare", "--target-dir", backupDir}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("mariabackup prepare: %w\noutput: %s", err, string(out))
	}

	// Copy back
	if err := p.stopMySQL(ctx, rt, target); err != nil {
		return nil, err
	}
	_, _ = rt.Exec(ctx, target, []string{"sh", "-c", "rm -rf /var/lib/mysql/*"})

	cmd = []string{"mariabackup", "--copy-back", "--target-dir", backupDir, "--datadir", "/var/lib/mysql"}
	out, err = rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("mariabackup copy-back: %w\noutput: %s", err, string(out))
	}

	_, _ = rt.Exec(ctx, target, []string{"chown", "-R", "mysql:mysql", "/var/lib/mysql"})

	if err := p.startMySQL(ctx, rt, target); err != nil {
		return nil, fmt.Errorf("start mysql after mariabackup restore: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	return &engine.RestoreResult{}, nil
}

func (p *Provider) stopMySQL(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	admin, err := targetcmd.FirstAvailableCommand(ctx, rt, target, "mysqladmin", "mariadb-admin")
	if err != nil {
		return err
	}
	if _, err := rt.Exec(ctx, target, []string{admin, "-u", "root", "ping"}); err != nil {
		return nil
	}

	out, shutdownErr := rt.Exec(ctx, target, []string{admin, "-u", "root", "shutdown"})
	stoppedErr := p.waitStopped(ctx, rt, target, admin)
	if shutdownErr != nil && stoppedErr != nil {
		return fmt.Errorf("mysql shutdown: %w\noutput: %s", shutdownErr, string(out))
	}
	return stoppedErr
}

func (p *Provider) waitStopped(ctx context.Context, rt engine.Runtime, target engine.Container, admin string) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := rt.Exec(ctx, target, []string{admin, "-u", "root", "ping"}); err != nil {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("mysql: timeout waiting for database to stop")
}

func (p *Provider) startMySQL(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	safe, err := targetcmd.FirstAvailableCommand(ctx, rt, target, "mysqld_safe", "mariadbd-safe")
	if err != nil {
		return err
	}
	if _, err = rt.Exec(ctx, target, []string{"sh", "-c", safe + " --user=mysql >/tmp/mysqld_safe.log 2>&1 &"}); err != nil {
		return err
	}
	return nil
}
