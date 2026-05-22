package reporter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fluentorbit/restore-drill/pkg/engine"
)

// jsonResult is the serialization format for drill results.
type jsonResult struct {
	Name             string      `json:"name"`
	Provider         string      `json:"provider"`
	Status           string      `json:"status"`
	StartedAt        time.Time   `json:"started_at"`
	Duration         string      `json:"duration"`
	DurationMs       int64       `json:"duration_ms"`
	BackupTimestamp  string      `json:"backup_timestamp,omitempty"`
	BackupAge        string      `json:"backup_age,omitempty"`
	ValidationPassed bool        `json:"validation_passed"`
	Error            string      `json:"error,omitempty"`
	Checks           []jsonCheck `json:"checks"`
}

type jsonCheck struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
	Duration string `json:"duration"`
	Error    string `json:"error,omitempty"`
}

// JSON writes drill results as JSON.
type JSON struct {
	Writer io.Writer
	Pretty bool
}

// NewJSON creates a JSON reporter that writes to stdout.
func NewJSON(pretty bool) *JSON {
	return &JSON{Writer: os.Stdout, Pretty: pretty}
}

// NewJSONFile creates a JSON reporter that writes to a file.
func NewJSONFile(path string, pretty bool) (*JSON, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create json output file: %w", err)
	}
	return &JSON{Writer: f, Pretty: pretty}, nil
}

// Report serializes results to JSON.
func (r *JSON) Report(_ context.Context, results []engine.DrillResult) error {
	output := make([]jsonResult, 0, len(results))

	for _, res := range results {
		jr := jsonResult{
			Name:             res.Name,
			Provider:         res.Provider,
			Status:           "pass",
			StartedAt:        res.StartedAt,
			Duration:         res.Duration.String(),
			DurationMs:       res.Duration.Milliseconds(),
			ValidationPassed: res.ValidationPassed,
		}

		if res.Error != nil || !res.ValidationPassed {
			jr.Status = "fail"
		}
		if res.Error != nil {
			jr.Error = res.Error.Error()
		}
		if !res.BackupTimestamp.IsZero() {
			jr.BackupTimestamp = res.BackupTimestamp.Format(time.RFC3339)
			jr.BackupAge = res.BackupAge.Truncate(time.Second).String()
		}

		for _, c := range res.Checks {
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

		output = append(output, jr)
	}

	enc := json.NewEncoder(r.Writer)
	if r.Pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(output)
}
