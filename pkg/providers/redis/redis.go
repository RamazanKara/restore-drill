// Package redis implements the restore-drill provider for Redis.
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/backup"
	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/RamazanKara/restore-drill/pkg/providers/internal/targetcmd"
)

// Provider implements engine.Provider for Redis.
type Provider struct{}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) Name() string {
	return "redis"
}

func (p *Provider) Preflight(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, checks []engine.Check) error {
	for _, cmd := range []string{"redis-server", "redis-cli"} {
		if err := targetcmd.CommandExists(ctx, rt, target, cmd); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) Restore(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring redis", "tool", cfg.Tool, "source", cfg.Source)

	switch cfg.Tool {
	case "rdb":
		return p.restoreRDB(ctx, rt, cfg, target)
	case "aof":
		return p.restoreAOF(ctx, rt, cfg, target)
	default:
		return nil, fmt.Errorf("redis: unsupported backup tool %q", cfg.Tool)
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
		case "key_count":
			actual, err = p.execRedis(ctx, rt, target, "DBSIZE")
			// DBSIZE returns "keys=N" format, extract just the number
			if err == nil {
				actual = extractDBSize(actual)
			}
		case "key_sample":
			// Check if specific keys exist
			for _, key := range check.Keys {
				if hasGlobMeta(key) {
					out, kerr := p.execRedis(ctx, rt, target, "KEYS", key)
					if kerr != nil {
						err = kerr
						break
					}
					if strings.TrimSpace(out) == "" {
						actual = "false"
						break
					}
				} else {
					out, kerr := p.execRedis(ctx, rt, target, "EXISTS", key)
					if kerr != nil {
						err = kerr
						break
					}
					if strings.TrimSpace(out) == "0" {
						actual = "false"
						break
					}
				}
			}
			if err == nil && actual == "" {
				actual = "true"
			}
		case "query":
			// Generic redis-cli command from SQL field (reused as command)
			parts := strings.Fields(check.SQL)
			if len(parts) > 0 {
				actual, err = p.execRedis(ctx, rt, target, parts...)
			} else {
				err = fmt.Errorf("redis: empty command in check %q", check.Name)
			}
		default:
			err = fmt.Errorf("redis: unsupported check type %q", check.Type)
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

func (p *Provider) execRedis(ctx context.Context, rt engine.Runtime, target engine.Container, args ...string) (string, error) {
	cmd := append([]string{"redis-cli"}, args...)
	out, err := rt.Exec(ctx, target, cmd)
	if err != nil {
		return "", fmt.Errorf("redis-cli exec: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (p *Provider) waitReady(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		out, err := rt.Exec(ctx, target, []string{"redis-cli", "PING"})
		if err == nil && strings.TrimSpace(string(out)) == "PONG" {
			slog.Debug("redis is ready")
			return nil
		}

		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("redis: timeout waiting for server to respond")
}

func (p *Provider) restoreRDB(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := rt.Exec(ctx, target, []string{"mkdir", "-p", "/data"}); err != nil {
		return nil, fmt.Errorf("redis: create data dir: %w", err)
	}
	if _, err := rt.Exec(ctx, target, []string{"cp", staged.Path, "/data/dump.rdb"}); err != nil {
		return nil, fmt.Errorf("redis: copy RDB file: %w", err)
	}

	if _, err := rt.Exec(ctx, target, []string{"redis-server", "--daemonize", "yes", "--dir", "/data"}); err != nil {
		return nil, fmt.Errorf("redis: start server: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	// Get last save time as backup timestamp
	ts, _ := p.execRedis(ctx, rt, target, "LASTSAVE")

	return &engine.RestoreResult{
		BackupTimestamp: ts,
	}, nil
}

func (p *Provider) restoreAOF(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := rt.Exec(ctx, target, []string{"mkdir", "-p", "/data"}); err != nil {
		return nil, fmt.Errorf("redis: create data dir: %w", err)
	}
	if _, err := rt.Exec(ctx, target, []string{"cp", staged.Path, "/data/appendonly.aof"}); err != nil {
		return nil, fmt.Errorf("redis: copy AOF file: %w", err)
	}

	if _, err := rt.Exec(ctx, target, []string{"redis-server", "--daemonize", "yes", "--dir", "/data", "--appendonly", "yes"}); err != nil {
		return nil, fmt.Errorf("redis: start server: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	return &engine.RestoreResult{}, nil
}

// extractDBSize extracts the numeric value from DBSIZE output.
// Redis DBSIZE returns "(integer) N" in redis-cli, or just "N" in raw mode.
func extractDBSize(s string) string {
	s = strings.TrimSpace(s)
	// Handle "(integer) 42" format
	if strings.HasPrefix(s, "(integer) ") {
		return strings.TrimPrefix(s, "(integer) ")
	}
	// Handle "keys=42,expires=0" format
	if strings.Contains(s, "keys=") {
		parts := strings.Split(s, ",")
		for _, p := range parts {
			if strings.HasPrefix(p, "keys=") {
				return strings.TrimPrefix(p, "keys=")
			}
		}
	}
	return s
}

func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
