// Package postgres implements the restore-drill provider for PostgreSQL databases.
package postgres

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fluentorbit/restore-drill/pkg/engine"
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

func (p *Provider) Restore(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	start := time.Now()

	switch cfg.Tool {
	case "pgbackrest":
		return p.restorePgBackRest(ctx, rt, cfg, target, start)
	case "pg_dump", "pg_restore":
		return p.restorePgDump(ctx, rt, cfg, target, start)
	case "wal-g", "walg":
		return p.restoreWalG(ctx, rt, cfg, target, start)
	default:
		return nil, fmt.Errorf("postgres: unsupported backup tool %q", cfg.Tool)
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
		case "query", "sql", "row_count":
			actual, err = p.execSQL(ctx, rt, target, check.SQL)
		case "schema":
			actual, err = p.execSQL(ctx, rt, target, check.SQL)
		case "freshness":
			actual, err = p.execSQL(ctx, rt, target, check.SQL)
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

func (p *Provider) restorePgBackRest(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	slog.Info("restoring via pgbackrest", "stanza", cfg.Stanza)

	// Wait for PostgreSQL to be ready (container starts postgres)
	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	// Stop postgres before restore
	_, _ = rt.Exec(ctx, target, []string{"pg_ctl", "stop", "-D", "/var/lib/postgresql/data", "-m", "fast"})

	// Run pgbackrest restore
	cmd := []string{"pgbackrest", "restore", "--stanza", cfg.Stanza, "--delta"}
	if cfg.Repo.Type != "" {
		cmd = append(cmd, "--repo-type", cfg.Repo.Type)
	}
	if cfg.Source != "" {
		cmd = append(cmd, "--repo-path", cfg.Source)
	}

	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("pgbackrest restore: %w\noutput: %s", err, string(out))
	}

	// Start postgres after restore
	_, err = rt.Exec(ctx, target, []string{"pg_ctl", "start", "-D", "/var/lib/postgresql/data", "-w"})
	if err != nil {
		return nil, fmt.Errorf("starting postgres after restore: %w", err)
	}

	// Wait for ready again
	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	// Get backup timestamp
	ts, _ := p.getBackupTimestamp(ctx, rt, target)

	return &engine.RestoreResult{
		BackupTimestamp: ts,
		Duration:        time.Since(start).Milliseconds(),
	}, nil
}

func (p *Provider) restorePgDump(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	slog.Info("restoring via pg_dump", "source", cfg.Source)

	// Wait for PostgreSQL to be ready
	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	// Copy the dump file into the container
	destPath := "/tmp/restore.sql"
	if cfg.Source != "" {
		data, err := os.ReadFile(cfg.Source)
		if err != nil {
			return nil, fmt.Errorf("read source file %s: %w", cfg.Source, err)
		}

		// Create a tar archive containing the file (Docker CopyTo expects tar)
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		hdr := &tar.Header{
			Name: filepath.Base(destPath),
			Mode: 0644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("tar header: %w", err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("tar write: %w", err)
		}
		if err := tw.Close(); err != nil {
			return nil, fmt.Errorf("tar close: %w", err)
		}

		if err := rt.CopyTo(ctx, target, "/tmp/", &buf); err != nil {
			return nil, fmt.Errorf("copy dump to container: %w", err)
		}
	}

	// Use psql for plain SQL dumps
	cmd := []string{"psql", "-U", "postgres", "-f", destPath}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		// psql may exit non-zero on warnings; check if output contains fatal errors
		outStr := string(out)
		if strings.Contains(outStr, "FATAL") || strings.Contains(outStr, "could not") {
			return nil, fmt.Errorf("psql restore: %w\noutput: %s", err, outStr)
		}
		slog.Warn("psql restore completed with warnings", "output", outStr)
	}

	ts, _ := p.getBackupTimestamp(ctx, rt, target)

	return &engine.RestoreResult{
		BackupTimestamp: ts,
		Duration:        time.Since(start).Milliseconds(),
	}, nil
}

func (p *Provider) restoreWalG(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	slog.Info("restoring via wal-g", "source", cfg.Source)

	// Wait for PostgreSQL to be ready
	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	// Stop postgres
	_, _ = rt.Exec(ctx, target, []string{"pg_ctl", "stop", "-D", "/var/lib/postgresql/data", "-m", "fast"})

	// Restore with wal-g
	cmd := []string{"wal-g", "backup-fetch", "/var/lib/postgresql/data", "LATEST"}
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return nil, fmt.Errorf("wal-g backup-fetch: %w\noutput: %s", err, string(out))
	}

	// Create recovery signal for WAL replay
	_, _ = rt.Exec(ctx, target, []string{"touch", "/var/lib/postgresql/data/recovery.signal"})

	// Start postgres
	_, err = rt.Exec(ctx, target, []string{"pg_ctl", "start", "-D", "/var/lib/postgresql/data", "-w"})
	if err != nil {
		return nil, fmt.Errorf("starting postgres after wal-g restore: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	ts, _ := p.getBackupTimestamp(ctx, rt, target)

	return &engine.RestoreResult{
		BackupTimestamp: ts,
		Duration:        time.Since(start).Milliseconds(),
	}, nil
}

// getBackupTimestamp queries pg for the latest checkpoint/restore timestamp.
func (p *Provider) getBackupTimestamp(ctx context.Context, rt engine.Runtime, target engine.Container) (string, error) {
	return p.execSQL(ctx, rt, target, "SELECT pg_postmaster_start_time()::text")
}
