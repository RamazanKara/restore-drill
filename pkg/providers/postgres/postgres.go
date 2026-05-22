package main
// Package postgres implements the restore-drill provider for PostgreSQL databases.
package postgres

import (
	"context"
	"fmt"

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

func (p *Provider) Restore(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	switch cfg.Tool {
	case "pgbackrest":
		return p.restorePgBackRest(ctx, cfg, target)
	case "pg_dump", "pg_restore":
		return p.restorePgDump(ctx, cfg, target)
	case "wal-g", "walg":
		return p.restoreWalG(ctx, cfg, target)
	default:
		return nil, fmt.Errorf("postgres: unsupported backup tool %q", cfg.Tool)
	}
}

func (p *Provider) Validate(ctx context.Context, target engine.Container, checks []engine.Check) (*engine.ValidationResult, error) {
	// TODO: implement SQL-based validation
	return &engine.ValidationResult{}, nil
}

func (p *Provider) Cleanup(ctx context.Context, target engine.Container) error {
	return nil
}

func (p *Provider) restorePgBackRest(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	// TODO: implement pgBackRest restore
	return &engine.RestoreResult{}, nil
}

func (p *Provider) restorePgDump(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	// TODO: implement pg_dump/pg_restore
	return &engine.RestoreResult{}, nil
}

func (p *Provider) restoreWalG(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	// TODO: implement WAL-G restore
	return &engine.RestoreResult{}, nil
}
