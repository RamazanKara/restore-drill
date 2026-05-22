package main
// Package mysql implements the restore-drill provider for MySQL/MariaDB databases.
package mysql

import (
	"context"
	"fmt"

	"github.com/fluentorbit/restore-drill/pkg/engine"
)

// Provider implements engine.Provider for MySQL/MariaDB.
type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string {
	return "mysql"
}

func (p *Provider) Restore(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	switch cfg.Tool {
	case "mysqldump":
		return p.restoreMysqldump(ctx, cfg, target)
	case "xtrabackup":
		return p.restoreXtrabackup(ctx, cfg, target)
	case "mariabackup":
		return p.restoreMariabackup(ctx, cfg, target)
	default:
		return nil, fmt.Errorf("mysql: unsupported backup tool %q", cfg.Tool)
	}
}

func (p *Provider) Validate(ctx context.Context, target engine.Container, checks []engine.Check) (*engine.ValidationResult, error) {
	return &engine.ValidationResult{}, nil
}

func (p *Provider) Cleanup(ctx context.Context, target engine.Container) error {
	return nil
}

func (p *Provider) restoreMysqldump(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	return &engine.RestoreResult{}, nil
}

func (p *Provider) restoreXtrabackup(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	return &engine.RestoreResult{}, nil
}

func (p *Provider) restoreMariabackup(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	return &engine.RestoreResult{}, nil
}
