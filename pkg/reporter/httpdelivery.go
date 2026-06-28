package reporter

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// httpDelivery holds the shared HTTP POST configuration used by the webhook and
// Slack reporters: a target URL, optional headers, and retry behavior.
type httpDelivery struct {
	URL          string
	Headers      map[string]string
	Timeout      time.Duration
	MaxAttempts  int
	RetryBackoff time.Duration
}

// newHTTPDelivery returns a delivery with copied headers and sensible defaults.
func newHTTPDelivery(url string, headers map[string]string) httpDelivery {
	copiedHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		copiedHeaders[k] = v
	}
	return httpDelivery{
		URL:          url,
		Headers:      copiedHeaders,
		Timeout:      30 * time.Second,
		MaxAttempts:  3,
		RetryBackoff: 2 * time.Second,
	}
}

// post sends a JSON body to the configured URL, retrying on transport errors and
// 5xx responses while treating 4xx responses as terminal.
func (d httpDelivery) post(ctx context.Context, body []byte) error {
	client := &http.Client{Timeout: d.Timeout}

	maxAttempts := d.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	retryBackoff := d.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = 2 * time.Second
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, time.Duration(attempt)*retryBackoff); err != nil {
				return fmt.Errorf("delivery retry wait: %w", err)
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "restore-drill/1.0")
		for k, v := range d.Headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request: %w", err)
			continue
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("failed to close response body", "error", closeErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			slog.Info("delivery sent", "url", d.URL, "status", resp.StatusCode)
			return nil
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("endpoint returned %d", resp.StatusCode)
			continue
		}
		// 4xx - do not retry.
		return fmt.Errorf("endpoint returned %d", resp.StatusCode)
	}

	return lastErr
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
