package etcd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

func TestPreflightRequiresEtcdTools(t *testing.T) {
	tests := []struct {
		name      string
		available []string
		wantErr   string
	}{
		{name: "image has both tools", available: []string{"etcd", "etcdctl"}},
		{name: "missing etcd server", available: []string{"etcdctl"}, wantErr: `required command "etcd" not found`},
		{name: "missing etcdctl client", available: []string{"etcd"}, wantErr: `required command "etcdctl" not found`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newEtcdRuntime(tt.available...)
			err := New().Preflight(context.Background(), rt, engine.BackupConfig{Tool: "snapshot"}, fakeContainer{}, nil)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected preflight error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRestoreRunsSnapshotRestoreStartAndHealth(t *testing.T) {
	path := writeEtcdSnapshotFixture(t, "snapshot.db")
	rt := newEtcdRuntime("etcd", "etcdctl")

	_, err := New().Restore(context.Background(), rt, engine.BackupConfig{
		Tool:   "snapshot",
		Source: path,
	}, fakeContainer{})
	if err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}

	restore := rt.findScript("snapshot restore")
	if restore == nil {
		t.Fatal("expected an etcdctl snapshot restore command")
	}
	// cmd = [sh, -c, script, sh, <snapshot>, <data-dir>]
	if got := restore[4]; got != "/tmp/restore-drill-backups/snapshot.db" {
		t.Errorf("snapshot restore got staged path %q", got)
	}
	if got := restore[5]; got != dataDir {
		t.Errorf("snapshot restore got data dir %q, want %q", got, dataDir)
	}

	start := rt.findScriptPrefix("etcd --data-dir")
	if start == nil {
		t.Fatal("expected an etcd start command")
	}
	if got := start[4]; got != dataDir {
		t.Errorf("etcd start got data dir %q, want %q", got, dataDir)
	}

	if rt.healthChecks == 0 {
		t.Error("expected at least one endpoint health check during readiness wait")
	}
}

func TestRestoreRejectsUnsupportedTool(t *testing.T) {
	_, err := New().Restore(context.Background(), newEtcdRuntime("etcd", "etcdctl"), engine.BackupConfig{Tool: "rdb"}, fakeContainer{})
	if err == nil || !strings.Contains(err.Error(), `unsupported backup tool "rdb"`) {
		t.Fatalf("expected unsupported tool error, got %v", err)
	}
}

func TestValidateEtcdChecks(t *testing.T) {
	rt := newEtcdRuntime("etcd", "etcdctl")
	rt.values["/registry/namespaces/default"] = "v1.Namespace default"
	rt.prefixKeys["/registry/namespaces/"] = "/registry/namespaces/default\n\n/registry/namespaces/kube-system\n"

	result, err := New().Validate(context.Background(), rt, fakeContainer{}, []engine.Check{
		{Name: "namespace-count", Type: "key_count", Key: "/registry/namespaces/", Expect: ">= 2"},
		{Name: "default-namespace", Type: "key_get", Key: "/registry/namespaces/default", Expect: `contains "default"`},
		{Name: "healthy", Type: "query", SQL: "endpoint health", Expect: `contains "is healthy"`},
	})
	if err != nil {
		t.Fatalf("validate etcd: %v", err)
	}
	if got := result.Checks[0].Actual; got != "2" {
		t.Errorf("expected key count 2, got %q", got)
	}
	if got := result.Checks[1].Actual; !strings.Contains(got, "default") {
		t.Errorf("expected key value containing default, got %q", got)
	}
	if got := result.Checks[2].Actual; !strings.Contains(got, "is healthy") {
		t.Errorf("expected health output, got %q", got)
	}
}

func TestValidateKeyCountAllKeys(t *testing.T) {
	rt := newEtcdRuntime("etcd", "etcdctl")
	rt.allKeys = "/a\n\n/b\n\n/c\n"

	result, err := New().Validate(context.Background(), rt, fakeContainer{}, []engine.Check{
		{Name: "all", Type: "key_count", Expect: "== 3"},
	})
	if err != nil {
		t.Fatalf("validate etcd: %v", err)
	}
	if got := result.Checks[0].Actual; got != "3" {
		t.Errorf("expected 3 total keys, got %q", got)
	}
}

func TestValidateRejectsUnsupportedCheckType(t *testing.T) {
	result, err := New().Validate(context.Background(), newEtcdRuntime("etcd", "etcdctl"), fakeContainer{}, []engine.Check{
		{Name: "bad", Type: "schema", Expect: "exists"},
	})
	if err != nil {
		t.Fatalf("validate etcd: %v", err)
	}
	if result.Checks[0].Error == nil {
		t.Fatal("expected unsupported check type error")
	}
}

// --- fakes ---

type etcdRuntime struct {
	available    map[string]struct{}
	calls        [][]string
	values       map[string]string // key -> value for `get --print-value-only`
	prefixKeys   map[string]string // prefix -> keys-only output
	allKeys      string            // keys-only output for `get "" --from-key`
	healthChecks int
}

func newEtcdRuntime(available ...string) *etcdRuntime {
	set := make(map[string]struct{}, len(available))
	for _, a := range available {
		set[a] = struct{}{}
	}
	return &etcdRuntime{
		available:  set,
		values:     map[string]string{},
		prefixKeys: map[string]string{},
	}
}

func (r *etcdRuntime) Create(context.Context, engine.ContainerSpec) (engine.Container, error) {
	return fakeContainer{}, nil
}

func (r *etcdRuntime) Exec(_ context.Context, _ engine.Container, cmd []string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), cmd...))

	if len(cmd) >= 3 && cmd[0] == "sh" && cmd[1] == "-c" {
		script := cmd[2]
		switch {
		case strings.HasPrefix(script, "command -v '") && strings.HasSuffix(script, "'"):
			// Provider preflight probe form: command -v 'name'
			name := strings.TrimSuffix(strings.TrimPrefix(script, "command -v '"), "'")
			if _, ok := r.available[name]; ok {
				return []byte("/usr/local/bin/" + name), nil
			}
			return nil, errors.New("not found")
		case strings.Contains(script, "snapshot restore"):
			return []byte("restored"), nil
		case strings.HasPrefix(script, "etcd --data-dir"):
			return []byte(""), nil
		case strings.Contains(script, "etcdctl --endpoints"):
			return r.handleEtcdctl(cmd[4:])
		}
	}
	// Default: staging commands (mkdir, tar, cp) succeed.
	return []byte("ok"), nil
}

func (r *etcdRuntime) handleEtcdctl(args []string) ([]byte, error) {
	switch {
	case len(args) >= 2 && args[0] == "endpoint" && args[1] == "health":
		r.healthChecks++
		return []byte("http://127.0.0.1:2379 is healthy: successfully committed proposal"), nil
	case len(args) >= 1 && args[0] == "get" && slices.Contains(args, "--keys-only"):
		if slices.Contains(args, "--from-key") {
			return []byte(r.allKeys), nil
		}
		return []byte(r.prefixKeys[args[1]]), nil
	case len(args) >= 3 && args[0] == "get" && slices.Contains(args, "--print-value-only"):
		return []byte(r.values[args[1]]), nil
	default:
		return []byte(""), nil
	}
}

func (r *etcdRuntime) CopyTo(_ context.Context, _ engine.Container, _ string, src io.Reader) error {
	_, err := io.Copy(io.Discard, src)
	return err
}

func (r *etcdRuntime) Destroy(context.Context, engine.Container) error { return nil }

func (r *etcdRuntime) findScript(substr string) []string {
	for _, cmd := range r.calls {
		if len(cmd) >= 3 && cmd[0] == "sh" && strings.Contains(cmd[2], substr) {
			return cmd
		}
	}
	return nil
}

func (r *etcdRuntime) findScriptPrefix(prefix string) []string {
	for _, cmd := range r.calls {
		if len(cmd) >= 3 && cmd[0] == "sh" && strings.HasPrefix(cmd[2], prefix) {
			return cmd
		}
	}
	return nil
}

func writeEtcdSnapshotFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("etcd-snapshot"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

type fakeContainer struct{}

func (fakeContainer) ID() string     { return "fake" }
func (fakeContainer) Host() string   { return "127.0.0.1" }
func (fakeContainer) Port(p int) int { return p }
