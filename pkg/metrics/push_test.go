package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

func TestPushResultsUsesCurrentRunCollectors(t *testing.T) {
	var calls atomic.Int32
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		bodies = append(bodies, string(body))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	result := engine.DrillResult{
		Name:             "pg",
		Provider:         "postgres",
		Duration:         time.Second,
		ValidationPassed: true,
		Checks: []engine.CheckResult{
			{Name: "rows", Passed: true},
		},
	}

	if err := PushResults([]engine.DrillResult{result}, server.URL, map[string]string{"environment": "test"}); err != nil {
		t.Fatalf("first push: %v", err)
	}
	if err := PushResults([]engine.DrillResult{result}, server.URL, map[string]string{"environment": "test"}); err != nil {
		t.Fatalf("second push: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 push calls, got %d", calls.Load())
	}
	for _, body := range bodies {
		if !strings.Contains(body, "restore_drill_runs_total") {
			t.Fatalf("push body did not contain run counter:\n%s", body)
		}
	}
}
