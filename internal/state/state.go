package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunResult is a serializable version of engine.DrillResult.
type RunResult struct {
	Name             string        `json:"name"`
	Provider         string        `json:"provider"`
	StartedAt        time.Time     `json:"started_at"`
	Duration         string        `json:"duration"`
	BackupTimestamp  time.Time     `json:"backup_timestamp,omitempty"`
	BackupAge        string        `json:"backup_age,omitempty"`
	ValidationPassed bool          `json:"validation_passed"`
	Error            string        `json:"error,omitempty"`
	Checks           []CheckResult `json:"checks"`
}

// CheckResult is a serializable check outcome.
type CheckResult struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
	Error    string `json:"error,omitempty"`
}

// LastRun holds the results of the most recent drill execution.
type LastRun struct {
	Timestamp time.Time   `json:"timestamp"`
	Results   []RunResult `json:"results"`
}

// DefaultPath returns the default state file path.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/.restore-drill/last-run.json"
	}
	return filepath.Join(home, ".restore-drill", "last-run.json")
}

// Save persists drill results to the state file.
func Save(path string, run *LastRun) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// Load reads the last run results from the state file.
func Load(path string) (*LastRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	var run LastRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &run, nil
}
