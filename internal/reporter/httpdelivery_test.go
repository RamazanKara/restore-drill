package reporter

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeliveryDoesNotLogWebhookCredentials(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	delivery := newHTTPDelivery(server.URL+"/services/path-secret?token=query-secret", nil)
	delivery.MaxAttempts = 1
	if err := delivery.post(context.Background(), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "delivery sent") {
		t.Fatal("missing delivery log")
	}
	assertNoWebhookCredentials(t, logs.String())
}

func TestDeliveryErrorsDoNotExposeWebhookCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	delivery := newHTTPDelivery("http://127.0.0.1/services/path-secret?token=query-secret", nil)
	delivery.MaxAttempts = 1
	err := delivery.post(ctx, []byte(`{}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want wrapped cancellation, got %v", err)
	}
	assertNoWebhookCredentials(t, err.Error())

	delivery.URL = "http://127.0.0.1/services/path-secret?token=query-secret#%invalid"
	err = delivery.post(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected malformed URL to fail")
	}
	assertNoWebhookCredentials(t, err.Error())
}

func assertNoWebhookCredentials(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{"path-secret", "query-secret"} {
		if strings.Contains(text, secret) {
			t.Errorf("webhook credential %q exposed in %q", secret, text)
		}
	}
}
