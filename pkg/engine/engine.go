// Package engine orchestrates backup restore drills.
package engine

import (
	"context"
	"time"
)

// DrillResult holds the outcome of a single drill execution.
type DrillResult struct {
	Name             string
	Provider         string
	StartedAt        time.Time
	Duration         time.Duration
	BackupTimestamp  time.Time
	BackupAge        time.Duration
	ValidationPassed bool
	Checks           []CheckResult
	Error            error
}

// CheckResult holds the outcome of a single validation check.
type CheckResult struct {
	Name     string
	Type     string
	Expected string
	Actual   string
	Passed   bool
	Duration time.Duration
	Error    error
}

// Engine orchestrates the drill lifecycle.
type Engine struct {
	runtime  Runtime
	reporter Reporter
}

// New creates a new drill engine.
func New(runtime Runtime, reporter Reporter) *Engine {
	return &Engine{
		runtime:  runtime,
		reporter: reporter,
	}
}

// Run executes all configured drills sequentially.
func (e *Engine) Run(ctx context.Context, drills []DrillConfig) ([]DrillResult, error) {
	var results []DrillResult

	for _, drill := range drills {
		result := e.executeDrill(ctx, drill)
		results = append(results, result)
	}

	if err := e.reporter.Report(ctx, results); err != nil {
		return results, err
	}

	return results, nil
}

func (e *Engine) executeDrill(ctx context.Context, drill DrillConfig) DrillResult {
	result := DrillResult{
		Name:      drill.Name,
		Provider:  drill.Provider,
		StartedAt: time.Now(),
	}

	// TODO: implement full drill lifecycle
	// 1. Provision container
	// 2. Pull and restore backup
	// 3. Wait for ready
	// 4. Run validation checks
	// 5. Collect results
	// 6. Destroy container

	result.Duration = time.Since(result.StartedAt)
	return result
}
