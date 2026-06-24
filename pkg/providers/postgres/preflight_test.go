package postgres

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

func TestPreflightRequiresPostgresTools(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		source    string
		available []string
		wantErr   string
	}{
		{
			name:      "pg_dump requires postgres client",
			tool:      "pg_dump",
			available: []string{"psql", "pg_isready"},
		},
		{
			name:      "pgbackrest requires pgbackrest",
			tool:      "pgbackrest",
			available: []string{"psql", "pg_isready", "pg_ctl"},
			wantErr:   `required command "pgbackrest" not found`,
		},
		{
			name:      "pgbackrest archive source requires tar",
			tool:      "pgbackrest",
			source:    "/backups/pgbackrest.tar.gz",
			available: []string{"psql", "pg_isready", "pg_ctl", "pgbackrest"},
			wantErr:   `required command "tar" not found`,
		},
		{
			name:      "wal-g requires wal-g binary",
			tool:      "walg",
			available: []string{"psql", "pg_isready", "pg_ctl"},
			wantErr:   `required WAL-G (wal-g or walg) not found`,
		},
		{
			name:      "wal-g accepts walg binary alias",
			tool:      "walg",
			available: []string{"psql", "pg_isready", "pg_ctl", "walg"},
		},
		{
			name:      "wal-g archive source requires tar",
			tool:      "wal-g",
			source:    "/backups/walg.tar.gz",
			available: []string{"psql", "pg_isready", "pg_ctl", "wal-g"},
			wantErr:   `required command "tar" not found`,
		},
		{
			name:      "compressed pg_dump requires gzip",
			tool:      "pg_dump",
			source:    "/backups/postgres.sql.gz",
			available: []string{"psql", "pg_isready"},
			wantErr:   `required command "gzip" not found`,
		},
		{
			name:      "pg_restore requires pg_restore",
			tool:      "pg_restore",
			available: []string{"psql", "pg_isready", "pg_restore"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(tt.available...)
			err := New().Preflight(context.Background(), rt, engine.BackupConfig{Tool: tt.tool, Source: tt.source}, fakeContainer{}, nil)
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
