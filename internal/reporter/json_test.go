package reporter

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestJSONReportStableShape(t *testing.T) {
	startedAt := time.Date(2026, 5, 20, 14, 30, 0, 0, time.UTC)
	backupAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	results := []engine.DrillResult{
		{
			Name:             "production-postgres",
			Provider:         "postgres",
			StartedAt:        startedAt,
			Duration:         1500 * time.Millisecond,
			BackupTimestamp:  backupAt,
			BackupAge:        2*time.Hour + 9*time.Second,
			ValidationPassed: true,
			CleanupSkipped:   true,
			TargetID:         "pod/restore-drill-production-postgres",
			TargetHost:       "127.0.0.1",
			TargetPorts:      map[int]int{5432: 15432},
			Checks: []engine.CheckResult{
				{
					Name:     "users-exist",
					Type:     "query",
					Expected: "> 0",
					Actual:   "42",
					Passed:   true,
					Duration: 25 * time.Millisecond,
				},
			},
		},
		{
			Name:             "redis-cache",
			Provider:         "redis",
			StartedAt:        startedAt.Add(time.Minute),
			Duration:         900 * time.Millisecond,
			ValidationPassed: false,
			Error:            errors.New("validate: key count below threshold"),
			Checks: []engine.CheckResult{
				{
					Name:     "keys-present",
					Type:     "key_count",
					Expected: "> 10",
					Actual:   "7",
					Passed:   false,
					Duration: 10 * time.Millisecond,
					Error:    errors.New("expected > 10, got 7"),
				},
			},
		},
	}

	var buf bytes.Buffer
	reporter := &JSON{Writer: &buf, Pretty: true}
	if err := reporter.Report(context.Background(), results); err != nil {
		t.Fatalf("report json: %v", err)
	}

	const want = `[
  {
    "name": "production-postgres",
    "provider": "postgres",
    "status": "pass",
    "started_at": "2026-05-20T14:30:00Z",
    "duration": "1.5s",
    "duration_ms": 1500,
    "backup_timestamp": "2026-05-20T12:00:00Z",
    "backup_age": "2h0m9s",
    "validation_passed": true,
    "cleanup_skipped": true,
    "target_id": "pod/restore-drill-production-postgres",
    "target_host": "127.0.0.1",
    "target_ports": {
      "5432": 15432
    },
    "checks": [
      {
        "name": "users-exist",
        "type": "query",
        "expected": "\u003e 0",
        "actual": "42",
        "passed": true,
        "duration": "25ms"
      }
    ]
  },
  {
    "name": "redis-cache",
    "provider": "redis",
    "status": "fail",
    "started_at": "2026-05-20T14:31:00Z",
    "duration": "900ms",
    "duration_ms": 900,
    "validation_passed": false,
    "error": "validate: key count below threshold",
    "checks": [
      {
        "name": "keys-present",
        "type": "key_count",
        "expected": "\u003e 10",
        "actual": "7",
        "passed": false,
        "duration": "10ms",
        "error": "expected \u003e 10, got 7"
      }
    ]
  }
]
`
	if got := buf.String(); got != want {
		t.Fatalf("unexpected json report\nwant:\n%s\ngot:\n%s", want, got)
	}
}
