package mysql

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestPreflightRequiresMySQLTools(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		source    string
		available []string
		wantErr   string
	}{
		{
			name:      "mysqldump requires mysql client",
			tool:      "mysqldump",
			available: []string{"mysql", "mysqladmin"},
		},
		{
			name:      "mysqldump accepts mariadb client aliases",
			tool:      "mysqldump",
			available: []string{"mariadb", "mariadb-admin"},
		},
		{
			name:      "compressed mysqldump requires gzip",
			tool:      "mysqldump",
			source:    "/backups/mysql.sql.gz",
			available: []string{"mysql", "mysqladmin"},
			wantErr:   `required command "gzip" not found`,
		},
		{
			name:      "xtrabackup requires xtrabackup",
			tool:      "xtrabackup",
			available: []string{"mysql", "mysqladmin", "mysqld_safe"},
			wantErr:   `required command "xtrabackup" not found`,
		},
		{
			name:      "mariabackup requires mariabackup",
			tool:      "mariabackup",
			available: []string{"mariadb", "mariadb-admin", "mariadbd-safe"},
			wantErr:   `required command "mariabackup" not found`,
		},
		{
			name:      "mariabackup accepts mariadb client aliases",
			tool:      "mariabackup",
			available: []string{"mariadb", "mariadb-admin", "mariadbd-safe", "mariabackup"},
		},
		{
			name:      "xtrabackup tar archive requires tar",
			tool:      "xtrabackup",
			source:    "/backups/full.tar.gz",
			available: []string{"mysql", "mysqladmin", "mysqld_safe", "xtrabackup"},
			wantErr:   `required command "tar" not found`,
		},
		{
			name:      "xtrabackup xbstream archive requires xbstream",
			tool:      "xtrabackup",
			source:    "/backups/full.xbstream",
			available: []string{"mysql", "mysqladmin", "mysqld_safe", "xtrabackup"},
			wantErr:   `required command "xbstream" not found`,
		},
		{
			name:      "mariabackup compressed xbstream requires gzip",
			tool:      "mariabackup",
			source:    "/backups/full.xbstream.gz",
			available: []string{"mysql", "mysqladmin", "mysqld_safe", "mariabackup"},
			wantErr:   `required command "gzip" not found`,
		},
		{
			name:      "missing mysqladmin fails early",
			tool:      "mysqldump",
			available: []string{"mysql"},
			wantErr:   `required mysql admin client (mysqladmin or mariadb-admin) not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(tt.available...)
			err := New().Preflight(context.Background(), rt, config.BackupConfig{Tool: tt.tool, Source: tt.source}, fakeContainer{}, nil)
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
