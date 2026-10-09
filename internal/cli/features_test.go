package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
	"github.com/RamazanKara/restore-drill/internal/state"
)

func TestRunSelectedDrills(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(fmt.Sprintf("parallel=%t", parallel), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var selected, unselected atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/selected":
					selected.Add(1)
				case "/unselected":
					unselected.Add(1)
				default:
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"message":"daemon unavailable"}`))
				}
			}))
			defer server.Close()
			t.Setenv("DOCKER_HOST", server.URL)
			t.Setenv("DOCKER_API_VERSION", "1.47")
			t.Setenv("DOCKER_TLS_VERIFY", "")
			t.Setenv("DOCKER_CERT_PATH", "")
			path := writeCLIConfig(t, t.TempDir(), fmt.Sprintf(`drills:
  - &common
    name: unselected
    provider: redis
    backup: {tool: aof, source: /mounted/appendonly.aof}
    restore: {container: {image: redis:7-alpine}}
    alerts:
      - {type: webhook, url: %q}
  - <<: *common
    name: cache,canary
    alerts:
      - {type: webhook, url: %q}
`, server.URL+"/unselected", server.URL+"/selected"))
			out, err := executeRoot(t, "run", "--config", path, "--runtime", "docker", "--format", "json", "--drill", "cache,canary", "--drill", "cache,canary", fmt.Sprintf("--parallel=%t", parallel))
			if err == nil || !strings.Contains(err.Error(), "one or more drills failed") {
				t.Fatalf("expected failed drill, got %v", err)
			}
			var results []struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(strings.NewReader(out)).Decode(&results); err != nil {
				t.Fatalf("invalid run JSON: %v\n%s", err, out)
			}
			if len(results) != 1 || results[0].Name != "cache,canary" {
				t.Fatalf("unexpected selected results: %+v", results)
			}
			if selected.Load() != 1 || unselected.Load() != 0 {
				t.Fatalf("alert calls: selected=%d, unselected=%d", selected.Load(), unselected.Load())
			}
			run, err := state.Load(state.DefaultPath())
			if err != nil || len(run.Results) != 1 || run.Results[0].Name != "cache,canary" {
				t.Fatalf("unexpected state: %+v, %v", run, err)
			}
			history, err := state.LoadHistory(time.Time{})
			if err != nil || len(history) != 1 || len(history[0].Results) != 1 {
				t.Fatalf("unexpected history: %+v, %v", history, err)
			}
		})
	}
}

func TestUnknownDrillFailsBeforeRuntimeInit(t *testing.T) {
	path := writeCLIConfig(t, t.TempDir(), `drills:
  - name: redis
    provider: redis
    backup: {tool: aof, source: /mounted/appendonly.aof}
    restore: {container: {image: redis:7-alpine}}
`)
	_, err := executeRoot(t, "run", "--config", path, "--drill", "missing", "--runtime", "bad")
	if err == nil || !strings.Contains(err.Error(), `unknown drill "missing" (available: redis)`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportCommandMarkdown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if err := state.AppendHistory(&state.LastRun{Timestamp: time.Now(), Results: []state.RunResult{{Name: "redis", Provider: "redis", Duration: "1s", ValidationPassed: true}}}); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{"", filepath.Join(t.TempDir(), "report.md")} {
		t.Run(output, func(t *testing.T) {
			out, err := executeRoot(t, "report", "--format", "markdown", "--output", output)
			if err != nil {
				t.Fatal(err)
			}
			if output != "" {
				data, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				out = string(data)
			}
			if !strings.Contains(out, "# Backup Restore Verification Evidence") || !strings.Contains(out, "| redis |") {
				t.Fatalf("unexpected Markdown: %s", out)
			}
		})
	}
}

func TestConfiguredMarkdownReports(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	ts := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	results := []engine.DrillResult{{Name: "redis", Provider: "redis", StartedAt: ts, Duration: time.Second, ValidationPassed: true}}
	tests := []struct {
		name, output, file string
		formats            []string
		count              int
	}{
		{"file", "report.md", "report.md", []string{"markdown"}, 1},
		{"directory", "reports/", "reports/restore-drill-evidence-20260524T120000Z.md", []string{"markdown"}, 1},
		{"multiple", "reports", "reports/restore-drill-evidence-20260524T120000Z.md", []string{"json", "html", "markdown", "markdown"}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := config.ReportConfig{Format: tt.formats, Output: filepath.Join(dir, tt.output), Retention: "7d"}
			if err := writeConfiguredReports(context.Background(), cfg, results, stateRunFromResults(results, ts)); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, tt.file)
			data, err := os.ReadFile(file)
			if err != nil || !strings.Contains(string(data), "| redis |") {
				t.Fatalf("unexpected Markdown report: %s, %v", data, err)
			}
			files, err := os.ReadDir(filepath.Dir(file))
			if err != nil || len(files) != tt.count {
				t.Fatalf("expected %d files, got %d, %v", tt.count, len(files), err)
			}
		})
	}
}

func TestRunMarkdownReporter(t *testing.T) {
	var out strings.Builder
	if err := buildReporter("markdown", &config.Config{}, &out).Report(context.Background(), []engine.DrillResult{{Name: "redis", ValidationPassed: true}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "# Restore drill results") || !strings.Contains(out.String(), "| redis |") {
		t.Fatalf("unexpected report: %s", out.String())
	}
}

func FuzzKeyValueFlags(f *testing.F) {
	for _, input := range []string{"team=platform", "a=b=c", " a = b ", "=value", "missing", "key="} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		parsed, err := parseKeyValueFlags([]string{input}, "--kube-pod-label")
		if err != nil {
			if parsed != nil {
				t.Fatal("partial flags returned with an error")
			}
			return
		}
		if len(parsed) != 1 {
			t.Fatalf("expected one flag, got %v", parsed)
		}
		for key, value := range parsed {
			roundtrip, err := parseKeyValueFlags([]string{key + "=" + value}, "--kube-pod-label")
			if err != nil || !reflect.DeepEqual(parsed, roundtrip) {
				t.Fatalf("flag did not round trip: %v, %v", roundtrip, err)
			}
		}
	})
}
