package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// Webhook sends drill results as JSON to an HTTP endpoint.
type Webhook struct {
	httpDelivery
}

// NewWebhook creates a webhook reporter.
func NewWebhook(url string, headers map[string]string) *Webhook {
	return &Webhook{httpDelivery: newHTTPDelivery(url, headers)}
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
		if drillPassed(r) {
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

	return w.post(ctx, body)
}
