package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestPushResultsUsesCurrentRunCollectors(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	var bodies []string
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		mu.Lock()
		defer mu.Unlock()
		methods = append(methods, r.Method)
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
	mu.Lock()
	defer mu.Unlock()
	for _, method := range methods {
		if method != http.MethodPut {
			t.Fatalf("expected push to replace metrics with PUT, got %s", method)
		}
	}
	for _, body := range bodies {
		if !strings.Contains(body, "restore_drill_runs_total") {
			t.Fatalf("push body did not contain run counter:\n%s", body)
		}
	}
}
