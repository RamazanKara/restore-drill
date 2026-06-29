package reporter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/engine"
)

func TestWebhookPostsPayloadAndHeaders(t *testing.T) {
	startedAt := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	var gotPayload webhookPayload
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("expected application/json content type, got %q", contentType)
		}
		if userAgent := r.Header.Get("User-Agent"); userAgent != "restore-drill/1.0" {
			t.Fatalf("expected restore-drill user agent, got %q", userAgent)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode webhook payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	webhook := NewWebhook(server.URL, map[string]string{"Authorization": "Bearer test-token"})
	webhook.Timeout = time.Second
	webhook.MaxAttempts = 1

	results := []engine.DrillResult{
		{
			Name:             "postgres-prod",
			Provider:         "postgres",
			StartedAt:        startedAt,
			Duration:         1500 * time.Millisecond,
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

	if err := webhook.Report(context.Background(), results); err != nil {
		t.Fatalf("send webhook: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("expected Authorization header, got %q", gotAuth)
	}
	if gotPayload.Summary.Total != 1 || gotPayload.Summary.Failed != 1 || gotPayload.Summary.Status != "fail" {
		t.Fatalf("unexpected summary: %#v", gotPayload.Summary)
	}
	if len(gotPayload.Results) != 1 {
		t.Fatalf("expected one result, got %#v", gotPayload.Results)
	}
	result := gotPayload.Results[0]
	if result.Name != "postgres-prod" || result.Status != "fail" || result.Error == "" {
		t.Fatalf("unexpected result payload: %#v", result)
	}
	if len(result.Checks) != 1 || result.Checks[0].Error == "" {
		t.Fatalf("expected check evidence in webhook payload: %#v", result.Checks)
	}
}

func TestWebhookRetriesServerErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	webhook := NewWebhook(server.URL, nil)
	webhook.Timeout = time.Second
	webhook.RetryBackoff = time.Millisecond

	if err := webhook.Report(context.Background(), []engine.DrillResult{{Name: "redis", Provider: "redis", ValidationPassed: true}}); err != nil {
		t.Fatalf("send webhook: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected retry after 5xx, got %d attempts", attempts)
	}
}

func TestWebhookDoesNotRetryClientErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	webhook := NewWebhook(server.URL, nil)
	webhook.Timeout = time.Second
	webhook.RetryBackoff = time.Millisecond

	if err := webhook.Report(context.Background(), []engine.DrillResult{{Name: "redis", Provider: "redis", ValidationPassed: true}}); err == nil {
		t.Fatal("expected 4xx webhook response to fail")
	}
	if attempts != 1 {
		t.Fatalf("expected no retry after 4xx, got %d attempts", attempts)
	}
}

func TestWebhookRetryWaitHonorsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	webhook := NewWebhook(server.URL, nil)
	webhook.Timeout = time.Second
	webhook.RetryBackoff = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := webhook.Report(ctx, []engine.DrillResult{{Name: "redis", Provider: "redis", ValidationPassed: true}}); err == nil {
		t.Fatal("expected canceled context to stop webhook retry")
	}
}
