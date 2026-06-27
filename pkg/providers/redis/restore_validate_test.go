package redis

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

func TestRestoreAOFStagesBackupAndStartsAppendOnlyServer(t *testing.T) {
	path := writeRedisBackupFixture(t, "appendonly.aof")
	rt := &redisRestoreRuntime{}

	_, err := New().Restore(context.Background(), rt, engine.BackupConfig{
		Tool:   "aof",
		Source: path,
	}, fakeContainer{})
	if err != nil {
		t.Fatalf("restore aof: %v", err)
	}

	assertRedisCommandContains(t, rt.findCommand("cp"), "/tmp/restore-drill-backups/appendonly.aof", "/data/appendonly.aof")
	assertRedisCommandContains(t, rt.findCommand("redis-server"), "--appendonly", "yes")
}

func TestRestoreRDBStagesBackupAndCapturesLastSave(t *testing.T) {
	path := writeRedisBackupFixture(t, "dump.rdb")
	rt := &redisRestoreRuntime{lastSave: "1716215400"}

	result, err := New().Restore(context.Background(), rt, engine.BackupConfig{
		Tool:   "rdb",
		Source: path,
	}, fakeContainer{})
	if err != nil {
		t.Fatalf("restore rdb: %v", err)
	}

	assertRedisCommandContains(t, rt.findCommand("cp"), "/tmp/restore-drill-backups/dump.rdb", "/data/dump.rdb")
	assertRedisCommandContains(t, rt.findCommand("redis-server"), "--dir", "/data")
	if result.BackupTimestamp != "1716215400" {
		t.Fatalf("expected LASTSAVE timestamp, got %q", result.BackupTimestamp)
	}
}

func TestRestoreRejectsUnsupportedTool(t *testing.T) {
	_, err := New().Restore(context.Background(), &redisRestoreRuntime{}, engine.BackupConfig{Tool: "snapshot"}, fakeContainer{})
	if err == nil {
		t.Fatal("expected unsupported tool error")
	}
	if !strings.Contains(err.Error(), `unsupported backup tool "snapshot"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRedisChecks(t *testing.T) {
	rt := &redisValidateRuntime{
		outputs: map[string]string{
			"redis-cli DBSIZE":             "(integer) 2",
			"redis-cli EXISTS session:one": "1",
			"redis-cli KEYS cache:*":       "cache:one\ncache:two",
			"redis-cli GET session:one":    "ok",
		},
	}

	result, err := New().Validate(context.Background(), rt, fakeContainer{}, []engine.Check{
		{Name: "key-count", Type: "key_count", Expect: "== 2"},
		{Name: "specific-key", Type: "key_sample", Keys: []string{"session:one"}, Expect: "true"},
		{Name: "glob-key", Type: "key_sample", Keys: []string{"cache:*"}, Expect: "true"},
		{Name: "query", Type: "query", SQL: "GET session:one", Expect: "ok"},
	})
	if err != nil {
		t.Fatalf("validate redis: %v", err)
	}
	if got := result.Checks[0].Actual; got != "2" {
		t.Fatalf("expected extracted key count 2, got %q", got)
	}
	for _, idx := range []int{1, 2} {
		if got := result.Checks[idx].Actual; got != "true" {
			t.Fatalf("expected check %d to be true, got %q", idx, got)
		}
	}
	if got := result.Checks[3].Actual; got != "ok" {
		t.Fatalf("expected query output ok, got %q", got)
	}
}

func TestValidateRedisKeySampleReportsFalseWhenMissing(t *testing.T) {
	rt := &redisValidateRuntime{
		outputs: map[string]string{
			"redis-cli EXISTS missing": "0",
		},
	}

	result, err := New().Validate(context.Background(), rt, fakeContainer{}, []engine.Check{
		{Name: "missing", Type: "key_sample", Keys: []string{"missing"}, Expect: "true"},
	})
	if err != nil {
		t.Fatalf("validate redis: %v", err)
	}
	if got := result.Checks[0].Actual; got != "false" {
		t.Fatalf("expected false for missing key, got %q", got)
	}
}

func TestValidateRedisQueryRejectsEmptyCommand(t *testing.T) {
	result, err := New().Validate(context.Background(), &redisValidateRuntime{}, fakeContainer{}, []engine.Check{
		{Name: "empty", Type: "query", SQL: "   ", Expect: "ok"},
	})
	if err != nil {
		t.Fatalf("validate redis: %v", err)
	}
	if result.Checks[0].Error == nil {
		t.Fatal("expected empty command check error")
	}
}

func TestValidateRedisRejectsUnsupportedCheckType(t *testing.T) {
	result, err := New().Validate(context.Background(), &redisValidateRuntime{}, fakeContainer{}, []engine.Check{
		{Name: "bad", Type: "schema", Expect: "exists"},
	})
	if err != nil {
		t.Fatalf("validate redis: %v", err)
	}
	if result.Checks[0].Error == nil {
		t.Fatal("expected unsupported check type error")
	}
}

func writeRedisBackupFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("redis-backup"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

type redisRestoreRuntime struct {
	commands [][]string
	lastSave string
}

func (r *redisRestoreRuntime) Create(context.Context, engine.ContainerSpec) (engine.Container, error) {
	return fakeContainer{}, nil
}

func (r *redisRestoreRuntime) Exec(_ context.Context, _ engine.Container, cmd []string) ([]byte, error) {
	r.commands = append(r.commands, append([]string(nil), cmd...))
	switch {
	case slices.Equal(cmd, []string{"redis-cli", "PING"}):
		return []byte("PONG"), nil
	case slices.Equal(cmd, []string{"redis-cli", "LASTSAVE"}):
		if r.lastSave == "" {
			r.lastSave = "1716210000"
		}
		return []byte(r.lastSave), nil
	default:
		return []byte("ok"), nil
	}
}

func (r *redisRestoreRuntime) CopyTo(_ context.Context, _ engine.Container, _ string, src io.Reader) error {
	_, err := io.Copy(io.Discard, src)
	return err
}

func (r *redisRestoreRuntime) Destroy(context.Context, engine.Container) error {
	return nil
}

func (r *redisRestoreRuntime) findCommand(name string) []string {
	for _, cmd := range r.commands {
		if len(cmd) > 0 && cmd[0] == name {
			return cmd
		}
	}
	return nil
}

type redisValidateRuntime struct {
	outputs map[string]string
}

func (r *redisValidateRuntime) Create(context.Context, engine.ContainerSpec) (engine.Container, error) {
	return fakeContainer{}, nil
}

func (r *redisValidateRuntime) Exec(_ context.Context, _ engine.Container, cmd []string) ([]byte, error) {
	key := strings.Join(cmd, " ")
	return []byte(r.outputs[key]), nil
}

func (r *redisValidateRuntime) CopyTo(context.Context, engine.Container, string, io.Reader) error {
	return nil
}

func (r *redisValidateRuntime) Destroy(context.Context, engine.Container) error {
	return nil
}

func assertRedisCommandContains(t *testing.T, cmd []string, parts ...string) {
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
