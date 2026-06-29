// Package postgres implements the restore-drill provider for PostgreSQL databases.
package postgres

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

// Provider implements engine.Provider for PostgreSQL.
type Provider struct{}

// New creates a new PostgreSQL provider.
func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string {
	return "postgres"
}

func (p *Provider) Preflight(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container, checks []config.Check) error {
	required := []string{"psql", "pg_isready"}
	switch cfg.Tool {
	case "pgbackrest":
		required = append(required, "pg_ctl", "pgbackrest")
		required = append(required, backup.ArchiveRequirements(targetcmd.ConfiguredBackupPath(cfg))...)
	case "pg_restore":
		required = append(required, "pg_restore")
	case "pg_dump":
		if strings.HasSuffix(targetcmd.ConfiguredBackupPath(cfg), ".gz") {
			required = append(required, "gzip")
		}
	case "wal-g", "walg":
		required = append(required, "pg_ctl")
		required = append(required, backup.ArchiveRequirements(targetcmd.ConfiguredBackupPath(cfg))...)
		if err := targetcmd.CommandExistsAny(ctx, rt, target, "WAL-G", "wal-g", "walg"); err != nil {
			return err
		}
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
	case "pgbackrest":
		return p.restorePgBackRest(ctx, rt, cfg, target)
	case "pg_dump", "pg_restore":
		return p.restorePgDump(ctx, rt, cfg, target)
	case "wal-g", "walg":
		return p.restoreWalG(ctx, rt, cfg, target)
	default:
		return nil, fmt.Errorf("postgres: unsupported backup tool %q", cfg.Tool)
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
		case "query", "sql", "row_count":
			actual, err = p.execSQL(ctx, rt, target, check.SQL)
		case "schema":
			actual, err = p.execSQL(ctx, rt, target, check.SQL)
		case "freshness":
			actual, err = p.execSQL(ctx, rt, target, check.SQL)
		case "extensions":
			actual, err = p.execSQL(ctx, rt, target, "SELECT string_agg(extname, ', ' ORDER BY extname) FROM pg_extension")
		default:
			err = fmt.Errorf("postgres: unsupported check type %q", check.Type)
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

// execSQL runs a SQL query via psql and returns the result.
func (p *Provider) execSQL(ctx context.Context, rt engine.Runtime, target engine.Container, sql string) (string, error) {
	// psql -At: unaligned, tuples only (no headers/footers)
	out, err := rt.Exec(ctx, target, []string{
		"psql", "-U", "postgres", "-At", "-c", sql,
	})
	if err != nil {
		return "", fmt.Errorf("psql exec: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// waitReady waits for PostgreSQL to accept connections.
func (p *Provider) waitReady(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	deadline := time.Now().Add(60 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		out, err := rt.Exec(ctx, target, []string{
			"pg_isready", "-U", "postgres",
		})
		if err == nil && strings.Contains(string(out), "accepting connections") {
			slog.Debug("postgres is ready")
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("postgres: timeout waiting for database to accept connections")
}

func (p *Provider) restorePgBackRest(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring via pgbackrest", "stanza", cfg.Stanza)

	if err := p.stopPostgresIfRunning(ctx, rt, target); err != nil {
		return nil, err
	}

	repoPath := cfg.Source
	if cfg.Source != "" {
		staged, err := backup.Stage(ctx, rt, target, cfg)
		if err != nil {
			return nil, err
		}
		repoPath, err = backup.MaterializeArchive(ctx, rt, target, staged.Path, "/tmp/restore-drill-backups/pgbackrest-repo")
		if err != nil {
			return nil, err
		}
	}

	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", "rm -rf /var/lib/postgresql/data/*"}); err != nil {
		return nil, fmt.Errorf("clear postgres data directory: %w", err)
	}

	cmd := []string{"pgbackrest", "restore", "--stanza", cfg.Stanza, "--pg1-path", "/var/lib/postgresql/data", "--delta"}
	if cfg.Repo.Type != "" {
		cmd = append(cmd, "--repo1-type", cfg.Repo.Type)
		if cfg.Repo.Bucket != "" {
			cmd = append(cmd, "--repo1-s3-bucket", cfg.Repo.Bucket)
		}
		if cfg.Repo.Endpoint != "" {
			cmd = append(cmd, "--repo1-s3-endpoint", cfg.Repo.Endpoint)
		}
		if cfg.Repo.Region != "" {
			cmd = append(cmd, "--repo1-s3-region", cfg.Repo.Region)
		}
		if cfg.Repo.Prefix != "" {
			cmd = append(cmd, "--repo1-path", cfg.Repo.Prefix)
		}
	}
	if repoPath != "" {
		cmd = append(cmd, "--repo1-path", repoPath)
	}
	if cfg.Target != "" && cfg.Target != "latest" {
		cmd = append(cmd, "--type", "time", "--target", cfg.Target)
	}

	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("pgbackrest restore: %w\noutput: %s", err, string(out))
	}

	if err := p.startPostgres(ctx, rt, target, "pgbackrest"); err != nil {
		return nil, err
	}

	// Wait for ready again
	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	// Get backup timestamp
	ts, _ := p.getBackupTimestamp(ctx, rt, target)

	return &engine.RestoreResult{
		BackupTimestamp: ts,
	}, nil
}

func (p *Provider) restorePgDump(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring via pg_dump", "source", cfg.Source)

	// Wait for PostgreSQL to be ready
	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}
	destPath := staged.Path

	var cmd []string
	switch {
	case cfg.Tool == "pg_restore":
		cmd = []string{"pg_restore", "-U", "postgres", "-d", "postgres", "--clean", "--if-exists", destPath}
	case strings.HasSuffix(destPath, ".gz"):
		cmd = []string{"sh", "-c", fmt.Sprintf("gzip -dc %s | psql -U postgres -v ON_ERROR_STOP=1", targetcmd.ShellQuote(destPath))}
	default:
		cmd = []string{"psql", "-U", "postgres", "-v", "ON_ERROR_STOP=1", "-f", destPath}
	}
	out, err := p.execWhenStable(ctx, rt, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("postgres dump restore: %w\noutput: %s", err, string(out))
	}

	ts, _ := p.getBackupTimestamp(ctx, rt, target)

	return &engine.RestoreResult{
		BackupTimestamp: ts,
	}, nil
}

func (p *Provider) restoreWalG(ctx context.Context, rt engine.Runtime, cfg config.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring via wal-g", "source", cfg.Source)

	walGBin, err := targetcmd.FirstAvailableCommand(ctx, rt, target, "wal-g", "walg")
	if err != nil {
		return nil, err
	}

	if err := p.stopPostgresIfRunning(ctx, rt, target); err != nil {
		return nil, err
	}

	repoPath := ""
	if cfg.Source != "" {
		staged, err := backup.Stage(ctx, rt, target, cfg)
		if err != nil {
			return nil, err
		}
		repoPath, err = backup.MaterializeArchive(ctx, rt, target, staged.Path, "/tmp/restore-drill-backups/walg-repo")
		if err != nil {
			return nil, err
		}
	}

	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", "rm -rf /var/lib/postgresql/data/*"}); err != nil {
		return nil, fmt.Errorf("clear postgres data directory: %w", err)
	}

	cmd := []string{walGBin, "backup-fetch", "/var/lib/postgresql/data", "LATEST"}
	restoreCommand := walGBin + " wal-fetch %f %p"
	if repoPath != "" {
		env := "WALG_FILE_PREFIX=" + targetcmd.ShellQuote(repoPath) + " "
		cmd = []string{"sh", "-c", env + walGBin + " backup-fetch /var/lib/postgresql/data LATEST"}
		restoreCommand = "WALG_FILE_PREFIX=" + targetcmd.ShellQuote(repoPath) + " " + walGBin + " wal-fetch %f %p"
	}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("wal-g backup-fetch: %w\noutput: %s", err, string(out))
	}

	recoveryLines := fmt.Sprintf("restore_command = '%s'\n", targetcmd.ShellEscapePostgresLiteral(restoreCommand))
	if cfg.Target != "" && cfg.Target != "latest" {
		recoveryLines += fmt.Sprintf("recovery_target_time = '%s'\n", targetcmd.ShellEscapePostgresLiteral(cfg.Target))
	}
	recoveryConfig := "touch /var/lib/postgresql/data/recovery.signal && cat >> /var/lib/postgresql/data/postgresql.auto.conf <<'RESTORE_DRILL_RECOVERY'\n" +
		recoveryLines +
		"RESTORE_DRILL_RECOVERY"
	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", recoveryConfig}); err != nil {
		return nil, fmt.Errorf("configure wal-g recovery: %w", err)
	}

	if err := p.startPostgres(ctx, rt, target, "wal-g"); err != nil {
		return nil, err
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	ts, _ := p.getBackupTimestamp(ctx, rt, target)

	return &engine.RestoreResult{
		BackupTimestamp: ts,
	}, nil
}

// getBackupTimestamp queries pg for the latest checkpoint/restore timestamp.
func (p *Provider) getBackupTimestamp(ctx context.Context, rt engine.Runtime, target engine.Container) (string, error) {
	return p.execSQL(ctx, rt, target, "SELECT pg_postmaster_start_time()::text")
}

func (p *Provider) execWhenStable(ctx context.Context, rt engine.Runtime, target engine.Container, cmd []string) ([]byte, error) {
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err := p.waitReady(ctx, rt, target); err != nil {
			return nil, err
		}

		out, err := rt.Exec(ctx, target, cmd)
		if err == nil {
			return out, nil
		}
		if !isConnectionStartupError(err, out) || time.Now().After(deadline) {
			return out, err
		}

		slog.Debug("postgres command hit startup race, retrying", "error", err)
		if err := sleepContext(ctx, 500*time.Millisecond); err != nil {
			return out, err
		}
	}
}

func isConnectionStartupError(err error, out []byte) bool {
	msg := strings.ToLower(err.Error() + "\n" + string(out))
	return strings.Contains(msg, "connection to server") &&
		(strings.Contains(msg, "no such file or directory") ||
			strings.Contains(msg, "connection refused") ||
			strings.Contains(msg, "could not connect") ||
			strings.Contains(msg, "starting up") ||
			strings.Contains(msg, "no response"))
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *Provider) stopPostgresIfRunning(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	out, err := rt.Exec(ctx, target, []string{"pg_isready", "-U", "postgres"})
	if err != nil || !strings.Contains(string(out), "accepting connections") {
		return nil
	}
	stopOut, err := rt.Exec(ctx, target, []string{"pg_ctl", "stop", "-D", "/var/lib/postgresql/data", "-m", "fast", "-w"})
	if err != nil {
		return fmt.Errorf("stop postgres before physical restore: %w\noutput: %s", err, string(stopOut))
	}
	return nil
}

func (p *Provider) startPostgres(ctx context.Context, rt engine.Runtime, target engine.Container, tool string) error {
	out, err := rt.Exec(ctx, target, []string{
		"sh",
		"-c",
		"pg_ctl start -D /var/lib/postgresql/data -l /tmp/restore-drill-postgres.log -w || { rc=$?; cat /tmp/restore-drill-postgres.log 2>/dev/null || true; exit $rc; }",
	})
	if err != nil {
		return fmt.Errorf("starting postgres after %s restore: %w\noutput: %s", tool, err, string(out))
	}
	return nil
}
