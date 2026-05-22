// Package engine orchestrates backup restore drills.
package engine

import (
	"context"
	"fmt"
	"log/slog"
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
	runtime   Runtime
	reporter  Reporter
	providers map[string]Provider
	noCleanup bool
}

// New creates a new drill engine.
func New(runtime Runtime, reporter Reporter) *Engine {
	return &Engine{
		runtime:   runtime,
		reporter:  reporter,
		providers: make(map[string]Provider),
	}
}

// SetNoCleanup configures whether containers are kept after drills.
func (e *Engine) SetNoCleanup(v bool) {
	e.noCleanup = v
}

// RegisterProvider adds a provider to the engine's registry.
func (e *Engine) RegisterProvider(p Provider) {
	e.providers[p.Name()] = p
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

// RunParallel executes all configured drills concurrently.
func (e *Engine) RunParallel(ctx context.Context, drills []DrillConfig) ([]DrillResult, error) {
	results := make([]DrillResult, len(drills))
	done := make(chan struct{}, len(drills))

	for i, drill := range drills {
		go func(idx int, d DrillConfig) {
			results[idx] = e.executeDrill(ctx, d)
			done <- struct{}{}
		}(i, drill)
	}

	for range drills {
		<-done
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

	// Apply per-drill timeout
	timeout := drill.DrillTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	slog.Info("starting drill", "name", drill.Name, "provider", drill.Provider, "timeout", timeout)

	// Look up provider
	provider, ok := e.providers[drill.Provider]
	if !ok {
		result.Error = fmt.Errorf("no provider registered for %q", drill.Provider)
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Build container spec
	env := make(map[string]string)
	for k, v := range drill.Restore.Container.Env {
		env[k] = v
	}
	spec := ContainerSpec{
		Image: drill.Restore.Container.Image,
		Env:   env,
		Ports: GetDefaultPorts(drill.Provider),
	}
	if drill.Restore.Container.Resources.Memory != "" {
		spec.MemoryLimit = parseMemory(drill.Restore.Container.Resources.Memory)
	}

	// 1. Provision container
	slog.Info("creating container", "image", spec.Image)
	container, err := e.runtime.Create(ctx, spec)
	if err != nil {
		result.Error = fmt.Errorf("create container: %w", err)
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	defer func() {
		if e.noCleanup {
			slog.Info("keeping container (--no-cleanup)", "id", container.ID())
			return
		}
		slog.Info("destroying container", "id", container.ID())
		if err := e.runtime.Destroy(ctx, container); err != nil {
			slog.Error("failed to destroy container", "id", container.ID(), "error", err)
		}
	}()

	// 2. Restore backup
	slog.Info("restoring backup", "tool", drill.Backup.Tool)
	restoreResult, err := provider.Restore(ctx, e.runtime, drill.Backup, container)
	if err != nil {
		result.Error = fmt.Errorf("restore: %w", err)
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	if restoreResult.BackupTimestamp != "" {
		if ts, err := parseTimestamp(restoreResult.BackupTimestamp); err == nil {
			result.BackupTimestamp = ts
			result.BackupAge = time.Since(ts)
		}
	}

	// 3. Run validation checks
	slog.Info("running validation", "checks", len(drill.Validate))
	valResult, err := provider.Validate(ctx, e.runtime, container, drill.Validate)
	if err != nil {
		result.Error = fmt.Errorf("validate: %w", err)
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// 4. Evaluate check expressions
	allPassed := true
	for i, check := range drill.Validate {
		cr := CheckResult{
			Name:     check.Name,
			Type:     check.Type,
			Expected: check.Expect,
		}

		start := time.Now()

		// Get actual value from validation result
		if valResult != nil && i < len(valResult.Checks) {
			cr.Actual = valResult.Checks[i].Actual
		}

		passed, evalErr := EvalExpression(check.Expect, cr.Actual)
		cr.Passed = passed
		cr.Duration = time.Since(start)
		if evalErr != nil {
			cr.Error = evalErr
			cr.Passed = false
		}
		if !cr.Passed {
			allPassed = false
		}

		result.Checks = append(result.Checks, cr)
	}

	result.ValidationPassed = allPassed
	result.Duration = time.Since(result.StartedAt)

	status := "passed"
	if !allPassed {
		status = "failed"
	}
	slog.Info("drill completed", "name", drill.Name, "status", status, "duration", result.Duration)

	return result
}

// parseMemory converts memory strings like "512Mi" or "1Gi" to bytes.
func parseMemory(s string) int64 {
	if len(s) < 2 {
		return 0
	}
	suffix := s[len(s)-2:]
	numStr := s[:len(s)-2]

	var multiplier int64
	switch suffix {
	case "Ki":
		multiplier = 1024
	case "Mi":
		multiplier = 1024 * 1024
	case "Gi":
		multiplier = 1024 * 1024 * 1024
	default:
		// Try single suffix (M, G)
		suffix = s[len(s)-1:]
		numStr = s[:len(s)-1]
		switch suffix {
		case "M":
			multiplier = 1000 * 1000
		case "G":
			multiplier = 1000 * 1000 * 1000
		default:
			return 0
		}
	}

	var n int64
	fmt.Sscanf(numStr, "%d", &n)
	return n * multiplier
}
