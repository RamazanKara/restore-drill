package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RamazanKara/restore-drill/internal/config"
)

func TestWebhookAlertURL(t *testing.T) {
	tests := []struct {
		name  string
		alert config.AlertSpec
		want  string
	}{
		{name: "prefers url", alert: config.AlertSpec{URL: "https://hook", Endpoint: "https://old"}, want: "https://hook"},
		{name: "falls back to endpoint", alert: config.AlertSpec{Endpoint: "https://old"}, want: "https://old"},
		{name: "empty when neither set", alert: config.AlertSpec{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webhookAlertURL(tt.alert); got != tt.want {
				t.Errorf("webhookAlertURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWebhookAlertKeyIsStableAndHeaderSensitive(t *testing.T) {
	base := webhookAlertKey("https://hook", nil)
	if base != "https://hook" {
		t.Fatalf("expected bare url for no headers, got %q", base)
	}

	// Header order must not change the key (deterministic dedup key).
	a := webhookAlertKey("https://hook", map[string]string{"A": "1", "B": "2"})
	b := webhookAlertKey("https://hook", map[string]string{"B": "2", "A": "1"})
	if a != b {
		t.Errorf("key must be order-independent: %q != %q", a, b)
	}
	if a == base {
		t.Error("headers must change the dedup key")
	}
}

func TestCheckWritableDir(t *testing.T) {
	dir := t.TempDir()
	if err := checkWritableDir(filepath.Join(dir, "nested", "reports")); err != nil {
		t.Errorf("expected writable nested dir, got %v", err)
	}

	// A path whose parent is a file cannot be created as a directory.
	fileParent := filepath.Join(dir, "afile")
	if err := os.WriteFile(fileParent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := checkWritableDir(filepath.Join(fileParent, "sub")); err == nil {
		t.Error("expected error when parent path is a file")
	}
}

func TestRenderDoctorReport(t *testing.T) {
	report := doctorReport{
		Status:  doctorWarn,
		Strict:  false,
		Summary: doctorSummary{Passed: 1, Warnings: 1, Failed: 0},
		Checks: []doctorCheck{
			{Name: "config", Status: doctorPass, Detail: "loaded"},
			{Name: "docker", Status: doctorWarn, Detail: "daemon unreachable", Remediation: "start docker"},
		},
	}

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		if err := renderDoctorReport(&buf, "json", report); err != nil {
			t.Fatalf("render json: %v", err)
		}
		var round doctorReport
		if err := json.Unmarshal(buf.Bytes(), &round); err != nil {
			t.Fatalf("json output not parseable: %v", err)
		}
		if round.Summary.Warnings != 1 || len(round.Checks) != 2 {
			t.Errorf("unexpected round-tripped report: %+v", round)
		}
	})

	t.Run("table", func(t *testing.T) {
		var buf bytes.Buffer
		if err := renderDoctorReport(&buf, "table", report); err != nil {
			t.Fatalf("render table: %v", err)
		}
		out := buf.String()
		for _, want := range []string{"STATUS", "config", "start docker", "SUMMARY"} {
			if !strings.Contains(out, want) {
				t.Errorf("table output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("unknown format", func(t *testing.T) {
		var buf bytes.Buffer
		if err := renderDoctorReport(&buf, "yaml", report); err == nil {
			t.Error("expected error for unknown format")
		}
	})
}
