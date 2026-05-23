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
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

// NewWebhook creates a webhook reporter.
func NewWebhook(url string, headers map[string]string) *Webhook {
	return &Webhook{
		URL:     url,
		Headers: headers,
		Timeout: 30 * time.Second,
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

	jsonResults := make([]jsonResult, 0, len(results))
	for _, r := range results {
		if r.Error == nil && r.ValidationPassed {
			passed++
		} else {
			failed++
		}
		totalDuration += r.Duration

		jr := jsonResult{
			Name:             r.Name,
			Provider:         r.Provider,
			Status:           "pass",
			StartedAt:        r.StartedAt,
			Duration:         r.Duration.String(),
			DurationMs:       r.Duration.Milliseconds(),
			ValidationPassed: r.ValidationPassed,
			CleanupSkipped:   r.CleanupSkipped,
			TargetID:         r.TargetID,
			TargetHost:       r.TargetHost,
			TargetPorts:      r.TargetPorts,
		}
		if r.Error != nil || !r.ValidationPassed {
			jr.Status = "fail"
		}
		if r.Error != nil {
			jr.Error = r.Error.Error()
		}
		if !r.BackupTimestamp.IsZero() {
			jr.BackupTimestamp = r.BackupTimestamp.Format(time.RFC3339)
			jr.BackupAge = r.BackupAge.Truncate(time.Second).String()
		}
		for _, c := range r.Checks {
			jc := jsonCheck{
				Name:     c.Name,
				Type:     c.Type,
				Expected: c.Expected,
				Actual:   c.Actual,
				Passed:   c.Passed,
				Duration: c.Duration.String(),
			}
			if c.Error != nil {
				jc.Error = c.Error.Error()
			}
			jr.Checks = append(jr.Checks, jc)
		}
		jsonResults = append(jsonResults, jr)
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
		Results: jsonResults,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	client := &http.Client{Timeout: w.Timeout}

	// Retry up to 3 times on 5xx errors.
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
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
