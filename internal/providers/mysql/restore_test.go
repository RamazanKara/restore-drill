package mysql

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestRestoreXtrabackupRunsPrepareCopyBackAndRestart(t *testing.T) {
	rt := &restoreRuntime{}

	_, err := New().Restore(context.Background(), rt, config.BackupConfig{
		Tool:   "xtrabackup",
		Source: "/mounted/xtrabackup",
	}, restoreContainer{})
	if err != nil {
		t.Fatalf("restore xtrabackup: %v", err)
	}

	assertCommandContains(
		t, rt.findCommand("xtrabackup", "--prepare"),
		"xtrabackup",
		"--prepare",
		"--target-dir",
		"/mounted/xtrabackup",
	)
	assertCommandContains(
		t, rt.findCommand("xtrabackup", "--copy-back"),
		"xtrabackup",
		"--copy-back",
		"--target-dir",
		"/mounted/xtrabackup",
		"--datadir",
		"/var/lib/mysql",
	)
	if rt.findShellCommandContaining("mysqld_safe --user=mysql") == "" {
		t.Fatalf("expected mysqld_safe restart command, got %v", rt.commands)
	}
}

func TestRestoreMariabackupRunsPrepareCopyBackAndRestart(t *testing.T) {
	rt := &restoreRuntime{}

	_, err := New().Restore(context.Background(), rt, config.BackupConfig{
		Tool:   "mariabackup",
		Source: "/mounted/mariabackup",
	}, restoreContainer{})
	if err != nil {
		t.Fatalf("restore mariabackup: %v", err)
	}

	assertCommandContains(
		t, rt.findCommand("mariabackup", "--prepare"),
		"mariabackup",
		"--prepare",
		"--target-dir",
		"/mounted/mariabackup",
	)
	assertCommandContains(
		t, rt.findCommand("mariabackup", "--copy-back"),
		"mariabackup",
		"--copy-back",
		"--target-dir",
		"/mounted/mariabackup",
		"--datadir",
		"/var/lib/mysql",
	)
	if rt.findShellCommandContaining("mysqld_safe --user=mysql") == "" {
		t.Fatalf("expected mysqld_safe restart command, got %v", rt.commands)
	}
}

func TestRestoreMysqldumpUsesMariaDBClientAlias(t *testing.T) {
	rt := &restoreRuntime{availableCommands: map[string]bool{
		"mariadb":       true,
		"mariadb-admin": true,
	}}

	_, err := New().Restore(context.Background(), rt, config.BackupConfig{
		Tool:   "mysqldump",
		Source: "/mounted/mysql.sql",
	}, restoreContainer{})
	if err != nil {
		t.Fatalf("restore mysqldump: %v", err)
	}

	restore := rt.findShellCommandContaining("'mariadb' -u root < '/mounted/mysql.sql'")
	if restore == "" {
		t.Fatalf("expected mariadb restore command, got %v", rt.commands)
	}
}

func TestRestoreXtrabackupMaterializesArchiveBeforePrepare(t *testing.T) {
	rt := &restoreRuntime{}

	_, err := New().Restore(context.Background(), rt, config.BackupConfig{
		Tool:   "xtrabackup",
		Source: "/mounted/xtrabackup.tar.gz",
	}, restoreContainer{})
	if err != nil {
		t.Fatalf("restore xtrabackup archive: %v", err)
	}

	if script := rt.findShellCommandContaining("physical backup archive materialization"); !strings.Contains(script, "tar -xzf") {
		t.Fatalf("expected tar archive materialization command, got %q", script)
	}
	assertCommandContains(
		t, rt.findCommand("xtrabackup", "--prepare"),
		"--target-dir",
		"/tmp/restore-drill-backups/mysql-physical",
	)
	assertCommandContains(
		t, rt.findCommand("xtrabackup", "--copy-back"),
		"--target-dir",
		"/tmp/restore-drill-backups/mysql-physical",
	)
}

type restoreRuntime struct {
	commands          [][]string
	stopped           bool
	availableCommands map[string]bool
}

func (r *restoreRuntime) Create(context.Context, engine.ContainerSpec) (engine.Container, error) {
	return restoreContainer{}, nil
}

func (r *restoreRuntime) Exec(_ context.Context, _ engine.Container, cmd []string) ([]byte, error) {
	r.commands = append(r.commands, append([]string(nil), cmd...))
	switch {
	case len(cmd) == 3 && cmd[0] == "sh" && cmd[1] == "-c" && strings.HasPrefix(cmd[2], "command -v "):
		if r.availableCommands == nil {
			return []byte("ok"), nil
		}
		name := strings.Trim(strings.TrimPrefix(cmd[2], "command -v "), "'")
		if r.availableCommands[name] {
			return []byte("ok"), nil
		}
		return nil, errors.New("not found")
	case len(cmd) > 2 && cmd[0] == "sh" && cmd[1] == "-c" && strings.Contains(cmd[2], "physical backup archive materialization"):
		return []byte("/tmp/restore-drill-backups/mysql-physical"), nil
	case len(cmd) > 0 && (cmd[0] == "mysqladmin" || cmd[0] == "mariadb-admin") && slices.Contains(cmd, "ping"):
		if r.stopped {
			return nil, errors.New("mysql is stopped")
		}
		return []byte("mysqld is alive"), nil
	case len(cmd) > 0 && (cmd[0] == "mysqladmin" || cmd[0] == "mariadb-admin") && slices.Contains(cmd, "shutdown"):
		r.stopped = true
		return []byte("ok"), nil
	case len(cmd) > 2 && cmd[0] == "sh" && cmd[1] == "-c" && strings.Contains(cmd[2], "mysqld_safe"):
		r.stopped = false
		return []byte("ok"), nil
	}
	return []byte("ok"), nil
}

func (r *restoreRuntime) CopyTo(context.Context, engine.Container, string, io.Reader) error {
	return nil
}

func (r *restoreRuntime) Destroy(context.Context, engine.Container) error {
	return nil
}

func (r *restoreRuntime) findCommand(name, marker string) []string {
	for _, cmd := range r.commands {
		if len(cmd) > 0 && cmd[0] == name && slices.Contains(cmd, marker) {
			return cmd
		}
	}
	return nil
}

func (r *restoreRuntime) findShellCommandContaining(needle string) string {
	for _, cmd := range r.commands {
		if len(cmd) == 3 && cmd[0] == "sh" && cmd[1] == "-c" && strings.Contains(cmd[2], needle) {
			return cmd[2]
		}
	}
	return ""
}

type restoreContainer struct{}

func (restoreContainer) ID() string {
	return "target"
}

func (restoreContainer) Host() string {
	return "127.0.0.1"
}

func (restoreContainer) Port(port int) int {
	return port
}

func assertCommandContains(t *testing.T, cmd []string, parts ...string) {
	t.Helper()
	if len(cmd) == 0 {
		t.Fatalf("expected command containing %v, got no command", parts)
	}
	for _, part := range parts {
		if !slices.Contains(cmd, part) {
			t.Fatalf("expected command %v to contain %q", cmd, part)
		}
	}
}
