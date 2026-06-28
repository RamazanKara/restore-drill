package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// Slack posts a formatted drill summary to a Slack-compatible incoming webhook.
// Slack and Mattermost both accept the {"text": ...} payload shape.
type Slack struct {
	httpDelivery
}

// NewSlack creates a Slack reporter targeting an incoming webhook URL.
func NewSlack(url string, headers map[string]string) *Slack {
	return &Slack{httpDelivery: newHTTPDelivery(url, headers)}
}

// slackPayload is the minimal incoming-webhook message body.
type slackPayload struct {
	Text string `json:"text"`
}

// Report posts a Slack message summarizing the drill results.
func (s *Slack) Report(ctx context.Context, results []engine.DrillResult) error {
	payload := slackPayload{Text: formatSlackMessage(results)}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	return s.post(ctx, body)
}

// formatSlackMessage renders a Slack mrkdwn summary with one line per drill.
func formatSlackMessage(results []engine.DrillResult) string {
	_, failed := countResults(results)
	total := len(results)

	var b strings.Builder
	if failed == 0 {
		fmt.Fprintf(&b, ":white_check_mark: *restore-drill*: all %d drill(s) passed", total)
	} else {
		fmt.Fprintf(&b, ":x: *restore-drill*: %d of %d drill(s) failed", failed, total)
	}

	for _, r := range results {
		if drillPassed(r) {
			fmt.Fprintf(&b, "\n• `%s` (%s) — passed in %s", r.Name, r.Provider, r.Duration.Round(time.Millisecond))
			continue
		}
		fmt.Fprintf(&b, "\n• `%s` (%s) — FAILED: %s", r.Name, r.Provider, drillFailureReason(r))
	}

	return b.String()
}

// drillFailureReason returns a concise human-readable reason a drill failed.
func drillFailureReason(r engine.DrillResult) string {
	if r.Error != nil {
		return r.Error.Error()
	}
	for _, c := range r.Checks {
		if c.Passed {
			continue
		}
		if c.Error != nil {
			return fmt.Sprintf("check %q: %s", c.Name, c.Error.Error())
		}
		return fmt.Sprintf("check %q: expected %s, got %q", c.Name, c.Expected, c.Actual)
	}
	return "validation failed"
}
