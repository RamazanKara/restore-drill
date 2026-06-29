package postgres

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestRestorePgBackRestBuildsPITRCommand(t *testing.T) {
	rt := &restoreRuntime{}
	cfg := config.BackupConfig{
		Tool:   "pgbackrest",
		Stanza: "main",
		Repo: config.RepoConfig{
			Type:     "s3",
			Bucket:   "restore-drill-backups",
			Endpoint: "s3.example.invalid",
			Prefix:   "/pgbackrest",
			Region:   "eu-central-1",
		},
		Target: "2026-05-20T14:30:00Z",
	}

	_, err := New().Restore(context.Background(), rt, cfg, restoreContainer{})
	if err != nil {
		t.Fatalf("restore pgbackrest: %v", err)
	}

	cmd := rt.findCommand("pgbackrest")
	assertCommandContains(
		t, cmd,
		"restore",
		"--stanza", "main",
		"--repo1-type", "s3",
		"--repo1-s3-bucket", "restore-drill-backups",
		"--repo1-s3-endpoint", "s3.example.invalid",
		"--repo1-s3-region", "eu-central-1",
		"--repo1-path", "/pgbackrest",
		"--type", "time",
		"--target", "2026-05-20T14:30:00Z",
	)
}

func TestRestoreWalGBuildsPITRRecoveryConfig(t *testing.T) {
	rt := &restoreRuntime{}
	cfg := config.BackupConfig{
		Tool:   "wal-g",
		Target: "2026-05-20T14:30:00Z",
	}

	_, err := New().Restore(context.Background(), rt, cfg, restoreContainer{})
	if err != nil {
		t.Fatalf("restore wal-g: %v", err)
	}

	assertCommandContains(t, rt.findCommand("wal-g"), "backup-fetch", "/var/lib/postgresql/data", "LATEST")

	recovery := rt.findShellCommandContaining("recovery_target_time")
	if !strings.Contains(recovery, "recovery_target_time = '2026-05-20T14:30:00Z'") {
		t.Fatalf("expected recovery target in command, got %q", recovery)
	}
	if !strings.Contains(recovery, "wal-g wal-fetch %f %p") {
		t.Fatalf("expected wal-g restore_command in command, got %q", recovery)
	}
}

func TestRestoreWalGStagesLocalRepository(t *testing.T) {
	rt := &restoreRuntime{}
	cfg := config.BackupConfig{
		Tool:   "wal-g",
		Source: "/mounted/walg",
	}

	_, err := New().Restore(context.Background(), rt, cfg, restoreContainer{})
	if err != nil {
		t.Fatalf("restore wal-g: %v", err)
	}

	fetch := rt.findShellCommandContaining("wal-g backup-fetch /var/lib/postgresql/data LATEST")
	if !strings.Contains(fetch, "WALG_FILE_PREFIX='/mounted/walg'") {
		t.Fatalf("expected WALG_FILE_PREFIX in fetch command, got %q", fetch)
	}
	recovery := rt.findShellCommandContaining("restore_command")
	if !strings.Contains(recovery, "WALG_FILE_PREFIX=''/mounted/walg'' wal-g wal-fetch %f %p") {
		t.Fatalf("expected WALG_FILE_PREFIX in restore_command, got %q", recovery)
	}
}

func TestRestoreWalGUsesWalgBinaryAlias(t *testing.T) {
	rt := &restoreRuntime{availableCommands: map[string]bool{
		"walg": true,
	}}
	cfg := config.BackupConfig{
		Tool:   "walg",
		Source: "/mounted/walg",
	}

	_, err := New().Restore(context.Background(), rt, cfg, restoreContainer{})
	if err != nil {
		t.Fatalf("restore walg: %v", err)
	}

	fetch := rt.findShellCommandContaining("walg backup-fetch /var/lib/postgresql/data LATEST")
	if !strings.Contains(fetch, "WALG_FILE_PREFIX='/mounted/walg'") {
		t.Fatalf("expected walg fetch command, got %q", fetch)
	}
	recovery := rt.findShellCommandContaining("restore_command")
	if !strings.Contains(recovery, "WALG_FILE_PREFIX=''/mounted/walg'' walg wal-fetch %f %p") {
		t.Fatalf("expected walg restore_command, got %q", recovery)
	}
}

func TestRestorePgRestoreUsesCustomArchiveCommand(t *testing.T) {
	rt := &restoreRuntime{}
	cfg := config.BackupConfig{
		Tool:   "pg_restore",
		Source: "/mounted/postgres.dump",
	}

	_, err := New().Restore(context.Background(), rt, cfg, restoreContainer{})
	if err != nil {
		t.Fatalf("restore pg_restore: %v", err)
	}

	assertCommandContains(
		t, rt.findCommand("pg_restore"),
		"-U", "postgres",
		"-d", "postgres",
		"--clean",
		"--if-exists",
		"/mounted/postgres.dump",
	)
}

func TestRestorePgBackRestMaterializesLocalArchive(t *testing.T) {
	rt := &restoreRuntime{}
	cfg := config.BackupConfig{
		Tool:   "pgbackrest",
		Stanza: "main",
		Source: "/mounted/pgbackrest-repo.tar.gz",
	}

	_, err := New().Restore(context.Background(), rt, cfg, restoreContainer{})
	if err != nil {
		t.Fatalf("restore pgbackrest archive: %v", err)
	}

	if script := rt.findShellCommandContaining("physical backup archive materialization"); !strings.Contains(script, "tar -xzf") {
		t.Fatalf("expected tar archive materialization command, got %q", script)
	}
	assertCommandContains(
		t, rt.findCommand("pgbackrest"),
		"--repo1-path",
		"/tmp/restore-drill-backups/pgbackrest-repo",
	)
}

type restoreRuntime struct {
	commands          [][]string
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
		return nil, io.ErrUnexpectedEOF
	case len(cmd) > 2 && cmd[0] == "sh" && cmd[1] == "-c" && strings.Contains(cmd[2], "physical backup archive materialization"):
		return []byte("/tmp/restore-drill-backups/pgbackrest-repo"), nil
	case len(cmd) > 0 && cmd[0] == "pg_isready":
		return []byte("/var/run/postgresql:5432 - accepting connections"), nil
	case len(cmd) > 0 && cmd[0] == "psql":
		return []byte("2026-05-24T00:00:00Z\n"), nil
	default:
		return []byte("ok"), nil
	}
}

func (r *restoreRuntime) CopyTo(context.Context, engine.Container, string, io.Reader) error {
	return nil
}

func (r *restoreRuntime) Destroy(context.Context, engine.Container) error {
	return nil
}

func (r *restoreRuntime) findCommand(name string) []string {
	for _, cmd := range r.commands {
		if len(cmd) > 0 && cmd[0] == name {
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
