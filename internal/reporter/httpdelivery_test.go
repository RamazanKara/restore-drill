package reporter

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestDeliveryRedirectErrorsDoNotExposeWebhookCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/services/path-secret?token=query-secret#%invalid")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	delivery := newHTTPDelivery(server.URL, nil)
	delivery.MaxAttempts = 1
	err := delivery.post(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected malformed redirect URL to fail")
	}
	assertNoWebhookCredentials(t, err.Error())
}

func TestRedactDeliveryURLPreservesNestedErrors(t *testing.T) {
	const webhookURL = "http://127.0.0.1/services/path-secret?token=query-secret"
	cause := &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
	nested := &url.Error{Op: "Get", URL: webhookURL, Err: cause}
	original := &url.Error{Op: "Post", URL: webhookURL, Err: nested}
	err := redactDeliveryURL(original)
	assertNoWebhookCredentials(t, err.Error())
	if !errors.Is(err, cause) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want wrapped transport error and deadline, got %v", err)
	}
	var transportErr *net.OpError
	if !errors.As(err, &transportErr) || transportErr != cause {
		t.Fatalf("want original transport error, got %v", err)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("want wrapped URL error, got %v", err)
	}
	if urlErr.URL != "[redacted]" {
		t.Errorf("URL was not redacted: %q", urlErr.URL)
	}
	if !urlErr.Timeout() || !urlErr.Temporary() {
		t.Errorf("lost timeout or temporary error behavior: %v", err)
	}
	if original.URL != webhookURL || original.Err != nested {
		t.Fatal("redaction changed the original error")
	}
}

func assertNoWebhookCredentials(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{"path-secret", "query-secret"} {
		if strings.Contains(text, secret) {
			t.Errorf("webhook credential %q exposed in %q", secret, text)
		}
	}
}
