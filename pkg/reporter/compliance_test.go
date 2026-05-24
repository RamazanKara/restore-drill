package reporter

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/state"
)

func TestBuildComplianceReportIncludesFailureEvidence(t *testing.T) {
	ts := time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)
	runs := []*state.LastRun{
		{
			Timestamp: ts,
			Results: []state.RunResult{
				{
					Name:             "production-postgres",
					Provider:         "postgres",
					Duration:         "2s",
					ValidationPassed: false,
					Checks: []state.CheckResult{
						{
							Name:     "users-exist",
							Type:     "query",
							Expected: "> 0",
							Actual:   "0",
							Passed:   false,
							Error:    "expected > 0, got 0",
						},
					},
				},
				{
					Name:             "redis-cache",
					Provider:         "redis",
					Duration:         "1s",
					ValidationPassed: false,
					Error:            "restore: missing appendonly.aof",
				},
			},
		},
	}

	report := BuildComplianceReport(runs, ts.Add(-time.Hour))
	if len(report.FailureEvidence) != 2 {
		t.Fatalf("expected 2 failure evidence entries, got %d: %#v", len(report.FailureEvidence), report.FailureEvidence)
	}
	if report.FailureEvidence[0].Check != "users-exist" {
		t.Fatalf("expected check failure evidence, got %#v", report.FailureEvidence[0])
	}
	if report.FailureEvidence[1].Check != "restore-drill" {
		t.Fatalf("expected run error evidence, got %#v", report.FailureEvidence[1])
	}
}

func TestRenderHTMLIncludesFailureEvidence(t *testing.T) {
	ts := time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)
	report := &ComplianceReport{
		GeneratedAt: ts,
		PeriodStart: ts.Add(-24 * time.Hour),
		PeriodEnd:   ts,
		TotalRuns:   1,
		FailedRuns:  1,
		FailureEvidence: []FailureEvidence{
			{
				Timestamp: ts,
				Drill:     "production-postgres",
				Provider:  "postgres",
				Check:     "users-exist",
				Type:      "query",
				Expected:  "> 0",
				Actual:    "0",
				Error:     "expected > 0, got 0",
			},
		},
	}

	var buf bytes.Buffer
	if err := RenderHTML(&buf, report); err != nil {
		t.Fatalf("render html: %v", err)
	}

	html := buf.String()
	for _, want := range []string{"Failure Evidence", "production-postgres", "users-exist", "&gt; 0", "expected &gt; 0, got 0"} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected rendered HTML to contain %q:\n%s", want, html)
		}
	}
}
