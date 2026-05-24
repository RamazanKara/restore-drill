package redis

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

func TestPreflightRequiresRedisTools(t *testing.T) {
	tests := []struct {
		name      string
		available []string
		wantErr   string
	}{
		{
			name:      "redis image has required tools",
			available: []string{"redis-server", "redis-cli"},
		},
		{
			name:      "missing redis server fails early",
			available: []string{"redis-cli"},
			wantErr:   `required command "redis-server" not found`,
		},
		{
			name:      "missing redis cli fails early",
			available: []string{"redis-server"},
			wantErr:   `required command "redis-cli" not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(tt.available...)
			err := New().Preflight(context.Background(), rt, engine.BackupConfig{Tool: "aof"}, fakeContainer{}, nil)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected preflight error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected preflight error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

type fakeRuntime struct {
	available map[string]struct{}
}

func newFakeRuntime(commands ...string) *fakeRuntime {
	available := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		available[command] = struct{}{}
	}
	return &fakeRuntime{available: available}
}

func (f *fakeRuntime) Create(context.Context, engine.ContainerSpec) (engine.Container, error) {
	return fakeContainer{}, nil
}

func (f *fakeRuntime) Exec(_ context.Context, _ engine.Container, cmd []string) ([]byte, error) {
	if len(cmd) == 3 && cmd[0] == "sh" && cmd[1] == "-c" && strings.HasPrefix(cmd[2], "command -v ") {
		name := strings.Trim(strings.TrimPrefix(cmd[2], "command -v "), "'")
		if _, ok := f.available[name]; ok {
			return []byte("/usr/bin/" + name), nil
		}
		return nil, errors.New("not found")
	}
	return nil, errors.New("unexpected command")
}

func (f *fakeRuntime) CopyTo(context.Context, engine.Container, string, io.Reader) error {
	return nil
}

func (f *fakeRuntime) Destroy(context.Context, engine.Container) error {
	return nil
}

func (f *fakeRuntime) Logs(context.Context, engine.Container) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

type fakeContainer struct{}

func (fakeContainer) ID() string {
	return "fake"
}

func (fakeContainer) Host() string {
	return "127.0.0.1"
}

func (fakeContainer) Port(port int) int {
	return port
}
