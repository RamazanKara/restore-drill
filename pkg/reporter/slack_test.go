package reporter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

func TestSlackPostsTextPayload(t *testing.T) {
	var gotPayload slackPayload
	var gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode slack payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	slack := NewSlack(server.URL, nil)
	slack.MaxAttempts = 1

	results := []engine.DrillResult{
		{Name: "pg", Provider: "postgres", ValidationPassed: true, Duration: 2 * time.Second},
		{Name: "etcd-prod", Provider: "etcd", ValidationPassed: false, Error: errors.New("restore: snapshot restore failed")},
	}

	if err := slack.Report(context.Background(), results); err != nil {
		t.Fatalf("slack report: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected application/json, got %q", gotContentType)
	}
	if !strings.Contains(gotPayload.Text, "1 of 2 drill(s) failed") {
		t.Errorf("unexpected slack text: %q", gotPayload.Text)
	}
	if !strings.Contains(gotPayload.Text, "etcd-prod") || !strings.Contains(gotPayload.Text, "snapshot restore failed") {
		t.Errorf("expected failure detail in slack text: %q", gotPayload.Text)
	}
}

func TestFormatSlackMessageAllPass(t *testing.T) {
	text := formatSlackMessage([]engine.DrillResult{
		{Name: "pg", Provider: "postgres", ValidationPassed: true, Duration: 1500 * time.Millisecond},
	})
	if !strings.Contains(text, "all 1 drill(s) passed") {
		t.Errorf("expected pass summary, got %q", text)
	}
	if !strings.Contains(text, ":white_check_mark:") {
		t.Errorf("expected success emoji, got %q", text)
	}
}

func TestDrillFailureReasonPrefersCheckEvidence(t *testing.T) {
	r := engine.DrillResult{
		Name:             "redis",
		ValidationPassed: false,
		Checks: []engine.CheckResult{
			{Name: "key-count", Passed: true},
			{Name: "session", Expected: "true", Actual: "false", Passed: false},
		},
	}
	reason := drillFailureReason(r)
	if !strings.Contains(reason, "session") || !strings.Contains(reason, "false") {
		t.Errorf("expected failing check detail, got %q", reason)
	}
}
