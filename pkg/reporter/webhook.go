package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// Webhook sends drill results as JSON to an HTTP endpoint.
type Webhook struct {
	URL          string
	Headers      map[string]string
	Timeout      time.Duration
	MaxAttempts  int
	RetryBackoff time.Duration
}

// NewWebhook creates a webhook reporter.
func NewWebhook(url string, headers map[string]string) *Webhook {
	copiedHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		copiedHeaders[k] = v
	}
	return &Webhook{
		URL:          url,
		Headers:      copiedHeaders,
		Timeout:      30 * time.Second,
		MaxAttempts:  3,
		RetryBackoff: 2 * time.Second,
	}
}

// webhookPayload is the JSON structure sent to the webhook endpoint.
type webhookPayload struct {
	Timestamp time.Time    `json:"timestamp"`
	Summary   summary      `json:"summary"`
	Results   []jsonResult `json:"results"`
}

type summary struct {
	Total    int    `json:"total"`
	Passed   int    `json:"passed"`
	Failed   int    `json:"failed"`
	Status   string `json:"status"`
	Duration string `json:"duration"`
}

// Report sends results to the configured webhook URL.
func (w *Webhook) Report(ctx context.Context, results []engine.DrillResult) error {
	passed := 0
	failed := 0
	var totalDuration time.Duration

	for _, r := range results {
		if r.Error == nil && r.ValidationPassed {
			passed++
		} else {
			failed++
		}
		totalDuration += r.Duration
	}

	status := "pass"
	if failed > 0 {
		status = "fail"
	}

	payload := webhookPayload{
		Timestamp: time.Now().UTC(),
		Summary: summary{
			Total:    len(results),
			Passed:   passed,
			Failed:   failed,
			Status:   status,
			Duration: totalDuration.String(),
		},
		Results: jsonResultsFromDrillResults(results),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	client := &http.Client{Timeout: w.Timeout}

	maxAttempts := w.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	retryBackoff := w.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = 2 * time.Second
	}

	// Retry on transport errors and 5xx responses.
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, time.Duration(attempt)*retryBackoff); err != nil {
				return fmt.Errorf("webhook retry wait: %w", err)
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "restore-drill/1.0")
		for k, v := range w.Headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("webhook request: %w", err)
			continue
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("failed to close webhook response", "error", closeErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			slog.Info("webhook delivered", "url", w.URL, "status", resp.StatusCode)
			return nil
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("webhook returned %d", resp.StatusCode)
			continue
		}
		// 4xx - do not retry
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
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
