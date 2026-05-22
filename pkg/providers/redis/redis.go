package main
// Package redis implements the restore-drill provider for Redis.
package redis

import (
	"context"

	"github.com/fluentorbit/restore-drill/pkg/engine"
)

// Provider implements engine.Provider for Redis.
type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string {
	return "redis"
}

func (p *Provider) Restore(ctx context.Context, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	// TODO: copy RDB file into container, start redis-server with it
	return &engine.RestoreResult{}, nil
}

func (p *Provider) Validate(ctx context.Context, target engine.Container, checks []engine.Check) (*engine.ValidationResult, error) {
	// TODO: implement key_count, key_sample checks
	return &engine.ValidationResult{}, nil
}

func (p *Provider) Cleanup(ctx context.Context, target engine.Container) error {
	return nil
}
