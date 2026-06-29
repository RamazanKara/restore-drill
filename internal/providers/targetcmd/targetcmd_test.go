package targetcmd

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "redis-cli", want: "'redis-cli'"},
		{in: "", want: "''"},
		{in: "a b", want: "'a b'"},
		{in: "it's", want: `'it'"'"'s'`},
		{in: "'", want: `''"'"''`},
	}
	for _, tt := range tests {
		if got := ShellQuote(tt.in); got != tt.want {
			t.Errorf("ShellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShellEscapePostgresLiteral(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "plain", want: "plain"},
		{in: "O'Brien", want: "O''Brien"},
		{in: "''", want: "''''"},
	}
	for _, tt := range tests {
		if got := ShellEscapePostgresLiteral(tt.in); got != tt.want {
			t.Errorf("ShellEscapePostgresLiteral(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestConfiguredBackupPath(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.BackupConfig
		want string
	}{
		{
			name: "source wins",
			cfg:  config.BackupConfig{Source: "/backups/x.sql", Repo: config.RepoConfig{Prefix: "p/"}},
			want: "/backups/x.sql",
		},
		{
			name: "falls back to repo prefix",
			cfg:  config.BackupConfig{Repo: config.RepoConfig{Prefix: "backups/postgres/"}},
			want: "backups/postgres/",
		},
		{
			name: "empty when neither set",
			cfg:  config.BackupConfig{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConfiguredBackupPath(tt.cfg); got != tt.want {
				t.Errorf("ConfiguredBackupPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandExists(t *testing.T) {
	rt := newFakeRuntime("psql")
	if err := CommandExists(context.Background(), rt, fakeContainer{}, "psql"); err != nil {
		t.Errorf("expected psql to be found, got %v", err)
	}
	err := CommandExists(context.Background(), rt, fakeContainer{}, "mysql")
	if err == nil || !strings.Contains(err.Error(), `required command "mysql" not found`) {
		t.Errorf("expected not-found error for mysql, got %v", err)
	}
}

func TestCommandExistsAny(t *testing.T) {
	rt := newFakeRuntime("mariadb")
	if err := CommandExistsAny(context.Background(), rt, fakeContainer{}, "mysql client", "mysql", "mariadb"); err != nil {
		t.Errorf("expected one client to be found, got %v", err)
	}

	empty := newFakeRuntime()
	err := CommandExistsAny(context.Background(), empty, fakeContainer{}, "mysql client", "mysql", "mariadb")
	if err == nil || !strings.Contains(err.Error(), "required mysql client (mysql or mariadb) not found") {
		t.Errorf("expected labelled not-found error, got %v", err)
	}
}

func TestFirstAvailableCommand(t *testing.T) {
	rt := newFakeRuntime("xtrabackup")
	got, err := FirstAvailableCommand(context.Background(), rt, fakeContainer{}, "mariabackup", "xtrabackup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "xtrabackup" {
		t.Errorf("FirstAvailableCommand() = %q, want %q", got, "xtrabackup")
	}

	if _, err := FirstAvailableCommand(context.Background(), rt, fakeContainer{}, "mariabackup"); err == nil {
		t.Error("expected error when no command is available")
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

func (fakeContainer) ID() string { return "fake" }

func (fakeContainer) Host() string { return "127.0.0.1" }

func (fakeContainer) Port(port int) int { return port }
