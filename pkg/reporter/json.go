package reporter

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/RamazanKara/restore-drill/pkg/engine"
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
	CleanupSkipped   bool        `json:"cleanup_skipped,omitempty"`
	TargetID         string      `json:"target_id,omitempty"`
	TargetHost       string      `json:"target_host,omitempty"`
	TargetPorts      map[int]int `json:"target_ports,omitempty"`
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

// Report serializes results to JSON.
func (r *JSON) Report(_ context.Context, results []engine.DrillResult) error {
	enc := json.NewEncoder(r.Writer)
	if r.Pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(jsonResultsFromDrillResults(results))
}
