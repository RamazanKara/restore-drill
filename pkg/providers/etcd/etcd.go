// Package etcd implements the restore-drill provider for etcd snapshots.
package etcd

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

const (
	dataDir      = "/etcd-restore"
	clientURL    = "http://127.0.0.1:2379"
	peerURL      = "http://127.0.0.1:2380"
	initialPeers = "default=http://127.0.0.1:2380"
)

// Provider implements engine.Provider for etcd.
type Provider struct{}

// New returns a new etcd provider.
func New() *Provider {
	return &Provider{}
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "etcd"
}

// Preflight verifies the restore image ships the etcd server and client.
func (p *Provider) Preflight(ctx context.Context, rt engine.Runtime, _ engine.BackupConfig, target engine.Container, _ []engine.Check) error {
	for _, cmd := range []string{"etcd", "etcdctl"} {
		if err := targetcmd.CommandExists(ctx, rt, target, cmd); err != nil {
			return err
		}
	}
	return nil
}

// Restore restores an etcd snapshot into a fresh data directory and starts etcd.
func (p *Provider) Restore(ctx context.Context, rt engine.Runtime, cfg engine.BackupConfig, target engine.Container) (*engine.RestoreResult, error) {
	slog.Info("restoring etcd", "tool", cfg.Tool, "source", cfg.Source)

	if cfg.Tool != "snapshot" {
		return nil, fmt.Errorf("etcd: unsupported backup tool %q", cfg.Tool)
	}

	staged, err := backup.Stage(ctx, rt, target, cfg)
	if err != nil {
		return nil, err
	}

	// etcdctl snapshot restore rebuilds a member data directory offline.
	restoreScript := `ETCDCTL_API=3 etcdctl snapshot restore "$1" ` +
		`--data-dir "$2" --name default ` +
		`--initial-cluster ` + initialPeers + ` ` +
		`--initial-advertise-peer-urls ` + peerURL
	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", restoreScript, "sh", staged.Path, dataDir}); err != nil {
		return nil, fmt.Errorf("etcd: snapshot restore: %w", err)
	}

	// Start etcd against the restored data directory in the background.
	startScript := `etcd --data-dir "$1" --name default ` +
		`--initial-cluster ` + initialPeers + ` ` +
		`--initial-advertise-peer-urls ` + peerURL + ` ` +
		`--listen-peer-urls ` + peerURL + ` ` +
		`--listen-client-urls ` + clientURL + ` ` +
		`--advertise-client-urls ` + clientURL + ` ` +
		`> /tmp/etcd.log 2>&1 &`
	if _, err := rt.Exec(ctx, target, []string{"sh", "-c", startScript, "sh", dataDir}); err != nil {
		return nil, fmt.Errorf("etcd: start server: %w", err)
	}

	if err := p.waitReady(ctx, rt, target); err != nil {
		return nil, err
	}

	return &engine.RestoreResult{}, nil
}

// Validate runs etcd-specific checks against the restored keyspace.
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
			actual, err = p.keyCount(ctx, rt, target, check.Key)
		case "key_get":
			actual, err = p.etcdctl(ctx, rt, target, "get", check.Key, "--print-value-only")
		case "query":
			parts := strings.Fields(check.SQL)
			if len(parts) > 0 {
				actual, err = p.etcdctl(ctx, rt, target, parts...)
			} else {
				err = fmt.Errorf("etcd: empty command in check %q", check.Name)
			}
		default:
			err = fmt.Errorf("etcd: unsupported check type %q", check.Type)
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

// Cleanup performs etcd-specific teardown (none required; the target is disposable).
func (p *Provider) Cleanup(_ context.Context, _ engine.Container) error {
	return nil
}

// keyCount returns the number of keys under the given prefix, or all keys when
// the prefix is empty.
func (p *Provider) keyCount(ctx context.Context, rt engine.Runtime, target engine.Container, prefix string) (string, error) {
	var out string
	var err error
	if prefix == "" {
		out, err = p.etcdctl(ctx, rt, target, "get", "", "--from-key", "--keys-only")
	} else {
		out, err = p.etcdctl(ctx, rt, target, "get", prefix, "--prefix", "--keys-only")
	}
	if err != nil {
		return "", err
	}

	count := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return fmt.Sprintf("%d", count), nil
}

// etcdctl runs an etcdctl command against the restored member. Arguments are
// passed positionally so keys with spaces or shell metacharacters are safe.
func (p *Provider) etcdctl(ctx context.Context, rt engine.Runtime, target engine.Container, args ...string) (string, error) {
	full := append([]string{"sh", "-c", `ETCDCTL_API=3 etcdctl --endpoints=` + clientURL + ` "$@"`, "sh"}, args...)
	out, err := rt.Exec(ctx, target, full)
	if err != nil {
		return "", fmt.Errorf("etcdctl exec: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// waitReady blocks until etcd reports a healthy endpoint or the deadline passes.
func (p *Provider) waitReady(ctx context.Context, rt engine.Runtime, target engine.Container) error {
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		out, err := p.etcdctl(ctx, rt, target, "endpoint", "health")
		if err == nil && strings.Contains(out, "is healthy") {
			slog.Debug("etcd is ready")
			return nil
		}

		time.Sleep(250 * time.Millisecond)
	}

	return fmt.Errorf("etcd: timeout waiting for server to respond")
}
