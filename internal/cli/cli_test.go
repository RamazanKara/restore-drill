package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/state"
)

func TestValidateCommandAcceptsValidConfig(t *testing.T) {
	configPath := writeCLIConfig(t, t.TempDir(), `drills:
  - name: redis
    provider: redis
    backup:
      tool: aof
      source: /mounted/appendonly.aof
    restore:
      container:
        image: redis:7-alpine
    checks:
      - name: ping
        type: query
        sql: PING
        expect: PONG
`)

	out, err := executeRoot(t, "validate", "--config", configPath)
	if err != nil {
		t.Fatalf("validate command failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "config OK: 1 drill(s)") {
		t.Fatalf("unexpected validate output: %s", out)
	}
}

func TestValidateCommandFailsForMissingConfig(t *testing.T) {
	_, err := executeRoot(t, "validate", "--config", filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected missing config to fail")
	}
}

func TestRunCommandRejectsBadRuntimeBeforeRuntimeInit(t *testing.T) {
	configPath := writeCLIConfig(t, t.TempDir(), `drills:
  - name: redis
    provider: redis
    backup:
      tool: aof
      source: /mounted/appendonly.aof
    restore:
      container:
        image: redis:7-alpine
    checks:
      - name: ping
        type: query
        sql: PING
        expect: PONG
`)

	_, err := executeRoot(t, "run", "--config", configPath, "--runtime", "bad")
	if err == nil {
		t.Fatal("expected bad runtime to fail")
	}
	if !strings.Contains(err.Error(), `unknown runtime "bad"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportCommandJSONOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	run := &state.LastRun{
		Timestamp: time.Now().UTC(),
		Results: []state.RunResult{
			{
				Name:             "redis",
				Provider:         "redis",
				StartedAt:        time.Now().UTC(),
				Duration:         "1s",
				ValidationPassed: true,
				Checks: []state.CheckResult{
					{Name: "ping", Type: "query", Expected: "PONG", Actual: "PONG", Passed: true},
				},
			},
		},
	}
	if err := state.AppendHistory(run); err != nil {
		t.Fatalf("append history: %v", err)
	}

	out, err := executeRoot(t, "report", "--format", "json", "--last", "90")
	if err != nil {
		t.Fatalf("report command failed: %v\n%s", err, out)
	}
	for _, want := range []string{`"TotalRuns"`, `"redis"`, `"SuccessRate"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("report output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorCommandJSONOutput(t *testing.T) {
	configPath := writeCLIConfig(t, t.TempDir(), `drills:
  - name: redis
    provider: redis
    backup:
      tool: aof
      source: /mounted/appendonly.aof
    restore:
      container:
        image: redis:7-alpine
    checks:
      - name: ping
        type: query
        sql: PING
        expect: PONG
reporting:
  format: [json]
  output: ./reports/
`)
	restoreDoctorDeps := stubDoctorDeps(t, nil, nil, true)
	defer restoreDoctorDeps()

	out, err := executeRoot(t, "doctor", "--config", configPath, "--runtime", "docker", "--format", "json")
	if err != nil {
		t.Fatalf("doctor command failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"status": "pass"`) {
		t.Fatalf("expected passing doctor json, got:\n%s", out)
	}
}

func TestDoctorStrictTreatsWarningsAsFailure(t *testing.T) {
	configPath := writeCLIConfig(t, t.TempDir(), `drills:
  - name: redis
    provider: redis
    backup:
      tool: aof
      source: /mounted/appendonly.aof
    restore:
      container:
        image: redis:7-alpine
    checks:
      - name: ping
        type: query
        sql: PING
        expect: PONG
`)
	restoreDoctorDeps := stubDoctorDeps(t, nil, nil, false)
	defer restoreDoctorDeps()

	out, err := executeRoot(t, "doctor", "--config", configPath, "--runtime", "docker", "--strict")
	if err == nil {
		t.Fatalf("expected strict doctor to fail on warnings\n%s", out)
	}
	if !strings.Contains(err.Error(), "doctor warning") {
		t.Fatalf("unexpected strict doctor error: %v", err)
	}
}

func executeRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func writeCLIConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "drill.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func stubDoctorDeps(t *testing.T, dockerErr, kubernetesErr error, allTools bool) func() {
	t.Helper()
	original := doctorDepsFactory
	doctorDepsFactory = func() doctorDeps {
		return doctorDeps{
			loadConfig: config.LoadConfig,
			dockerPing: func(context.Context) error {
				return dockerErr
			},
			kubernetesPing: func(context.Context, string) error {
				return kubernetesErr
			},
			lookPath: func(name string) (string, error) {
				if allTools || name == "go" || name == "git" {
					return "/usr/bin/" + name, nil
				}
				return "", errors.New("not found")
			},
			writableDir: func(string) error {
				return nil
			},
			isKubernetesEnv: func() bool {
				return false
			},
		}
	}
	return func() {
		doctorDepsFactory = original
	}
}
