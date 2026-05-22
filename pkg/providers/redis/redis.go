// Package redis implements the restore-drill provider for Redis.
package redis

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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

func (p *Provider) Restore(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	start := time.Now()
	slog.Info("restoring redis", "tool", cfg.Tool, "source", cfg.Source)

	// Wait for Redis to be ready
	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	switch cfg.Tool {
	case "rdb":
		return p.restoreRDB(ctx, rt, cfg, target, start)
	case "aof":
		return p.restoreAOF(ctx, rt, cfg, target, start)
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

func (p *Provider) restoreRDB(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	// Stop redis, copy RDB file, restart
	_, _ = rt.Exec(ctx, target, []string{"redis-cli", "SHUTDOWN", "NOSAVE"})
	time.Sleep(500 * time.Millisecond)

	// Copy RDB file to data directory
	if cfg.Source != "" {
		_, err := rt.Exec(ctx, target, []string{"cp", cfg.Source, "/data/dump.rdb"})
		if err != nil {
			return nil, fmt.Errorf("redis: copy RDB file: %w", err)
		}
	}

	// Start redis again (it loads RDB on startup)
	_, _ = rt.Exec(ctx, target, []string{"redis-server", "--daemonize", "yes", "--dir", "/data"})

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	// Get last save time as backup timestamp
	ts, _ := p.execRedis(ctx, rt, target, "LASTSAVE")

	return &engine.RestoreResult{
		BackupTimestamp: ts,
		Duration:        time.Since(start).Milliseconds(),
	}, nil
}

func (p *Provider) restoreAOF(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container, start time.Time) (*engine.RestoreResult, error) {
	// Stop redis, copy AOF file, restart with AOF enabled
	_, _ = rt.Exec(ctx, target, []string{"redis-cli", "SHUTDOWN", "NOSAVE"})
	time.Sleep(500 * time.Millisecond)

	if cfg.Source != "" {
		_, err := rt.Exec(ctx, target, []string{"cp", cfg.Source, "/data/appendonly.aof"})
		if err != nil {
			return nil, fmt.Errorf("redis: copy AOF file: %w", err)
		}
	}

	_, _ = rt.Exec(ctx, target, []string{"redis-server", "--daemonize", "yes", "--dir", "/data", "--appendonly", "yes"})

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	return &engine.RestoreResult{
		Duration: time.Since(start).Milliseconds(),
	}, nil
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
