package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/config"
	"github.com/RamazanKara/restore-drill/internal/engine"
	"github.com/RamazanKara/restore-drill/internal/reporter"
)

func TestParseKeyValueFlags(t *testing.T) {
	values, err := parseKeyValueFlags([]string{"team=platform", "env=prod"}, "--kube-pod-label")
	if err != nil {
		t.Fatalf("parse key-value flags: %v", err)
	}
	if values["team"] != "platform" {
		t.Fatalf("expected team label, got %#v", values)
	}
	if values["env"] != "prod" {
		t.Fatalf("expected env label, got %#v", values)
	}
}

func TestParseKeyValueFlagsRejectsMissingValueSeparator(t *testing.T) {
	_, err := parseKeyValueFlags([]string{"team"}, "--kube-pod-label")
	if err == nil {
		t.Fatal("expected invalid key-value flag to fail")
	}
}

func TestBuildReporterPassesWebhookHeaders(t *testing.T) {
	cfg := &config.Config{
		Drills: []config.DrillConfig{
			{
				Name: "postgres-prod",
				Alerts: []config.AlertSpec{
					{
						Type:     "webhook",
						Endpoint: "https://hooks.example.invalid/restore-drill",
						Headers:  map[string]string{"Authorization": "Bearer test-token"},
					},
				},
			},
		},
	}

	rep := buildReporter("table", cfg, io.Discard)
	multi, ok := rep.(*reporter.Multi)
	if !ok {
		t.Fatalf("expected multi reporter, got %T", rep)
	}
	if len(multi.Reporters) != 2 {
		t.Fatalf("expected stdout and webhook reporters, got %d", len(multi.Reporters))
	}
	webhook, ok := multi.Reporters[1].(*reporter.Webhook)
	if !ok {
		t.Fatalf("expected webhook reporter, got %T", multi.Reporters[1])
	}
	if webhook.URL != "https://hooks.example.invalid/restore-drill" {
		t.Fatalf("unexpected webhook url %q", webhook.URL)
	}
	if webhook.Headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("expected Authorization header, got %#v", webhook.Headers)
	}
}

func TestBuildReporterWiresSlackOnFailure(t *testing.T) {
	cfg := &config.Config{
		Drills: []config.DrillConfig{
			{
				Name: "etcd-prod",
				Alerts: []config.AlertSpec{
					{Type: "slack", URL: "https://hooks.slack.com/services/x", On: "failure"},
				},
			},
		},
	}

	rep := buildReporter("table", cfg, io.Discard)
	multi, ok := rep.(*reporter.Multi)
	if !ok {
		t.Fatalf("expected multi reporter, got %T", rep)
	}
	if len(multi.Reporters) != 2 {
		t.Fatalf("expected stdout and slack reporters, got %d", len(multi.Reporters))
	}

	cond, ok := multi.Reporters[1].(*reporter.Conditional)
	if !ok {
		t.Fatalf("expected conditional reporter for on:failure, got %T", multi.Reporters[1])
	}
	if !cond.OnlyOnFailure {
		t.Fatal("expected conditional reporter to be gated on failure")
	}
	if _, ok := cond.Inner.(*reporter.Slack); !ok {
		t.Fatalf("expected slack reporter inside conditional, got %T", cond.Inner)
	}
}

func TestBuildReporterIgnoresDeprecatedPrometheusAlerts(t *testing.T) {
	cfg := &config.Config{
		Drills: []config.DrillConfig{
			{
				Name: "postgres-prod",
				Alerts: []config.AlertSpec{
					{Type: "prometheus"},
				},
			},
		},
	}

	rep := buildReporter("table", cfg, io.Discard)
	if _, ok := rep.(*reporter.Stdout); !ok {
		t.Fatalf("expected only stdout reporter for deprecated prometheus alert, got %T", rep)
	}
}

func TestConfiguredReportPath(t *testing.T) {
	timestamp := "20260524T120000Z"
	dirPath := configuredReportPath("reports/", "json", false, timestamp)
	if !strings.HasSuffix(dirPath, filepath.Join("reports", "restore-drill-run-"+timestamp+".json")) {
		t.Fatalf("expected directory output path, got %s", dirPath)
	}

	filePath := configuredReportPath("report.json", "json", false, timestamp)
	if filePath != "report.json" {
		t.Fatalf("expected explicit file path, got %s", filePath)
	}

	multiPath := configuredReportPath("report.json", "html", true, timestamp)
	if !strings.HasSuffix(multiPath, filepath.Join("report.json", "restore-drill-compliance-"+timestamp+".html")) {
		t.Fatalf("expected multi-format output to be treated as a directory, got %s", multiPath)
	}
}

func TestWriteConfiguredReportsWritesJSONAndHTML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	outputDir := t.TempDir()
	ts := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	results := []engine.DrillResult{
		{
			Name:             "postgres-prod",
			Provider:         "postgres",
			StartedAt:        ts,
			Duration:         2 * time.Second,
			ValidationPassed: false,
			Error:            errors.New("validate: expected > 0, got 0"),
			Checks: []engine.CheckResult{
				{
					Name:     "users-exist",
					Type:     "query",
					Expected: "> 0",
					Actual:   "0",
					Passed:   false,
					Duration: 25 * time.Millisecond,
					Error:    errors.New("expected > 0, got 0"),
				},
			},
		},
	}
	run := stateRunFromResults(results, ts)

	err := writeConfiguredReports(context.Background(), config.ReportConfig{
		Format:    []string{"json", "html"},
		Output:    outputDir,
		Retention: "7d",
	}, results, run)
	if err != nil {
		t.Fatalf("write configured reports: %v", err)
	}

	jsonPath := filepath.Join(outputDir, "restore-drill-run-20260524T120000Z.json")
	htmlPath := filepath.Join(outputDir, "restore-drill-compliance-20260524T120000Z.html")
	jsonBody, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read json report: %v", err)
	}
	if !strings.Contains(string(jsonBody), `"name": "postgres-prod"`) {
		t.Fatalf("json report missing drill name:\n%s", string(jsonBody))
	}

	htmlBody, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read html report: %v", err)
	}
	html := string(htmlBody)
	for _, want := range []string{"Failure Evidence", "postgres-prod", "users-exist", "Avg RTO", "Max RTO"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html report missing %q:\n%s", want, html)
		}
	}
}
