// Package engine orchestrates backup restore drills.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	CleanupSkipped   bool
	TargetID         string
	TargetHost       string
	TargetPorts      map[int]int
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
	results := make([]DrillResult, 0, len(drills))

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
	var wg sync.WaitGroup

	for i, drill := range drills {
		wg.Add(1)
		go func(idx int, d DrillConfig) {
			defer wg.Done()
			results[idx] = e.executeDrill(ctx, d)
		}(i, drill)
	}

	wg.Wait()

	if err := e.reporter.Report(ctx, results); err != nil {
		return results, err
	}

	return results, nil
}

func (e *Engine) executeDrill(ctx context.Context, drill DrillConfig) (result DrillResult) {
	result = DrillResult{
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
	if drill.Provider == "redis" && (drill.Backup.Tool == "rdb" || drill.Backup.Tool == "aof") {
		spec.Cmd = []string{"sh", "-c", "sleep infinity"}
	}
	if drill.Provider == "mysql" && (drill.Backup.Tool == "xtrabackup" || drill.Backup.Tool == "mariabackup") {
		spec.Cmd = []string{"sh", "-c", "sleep infinity"}
	}
	if drill.Provider == "postgres" && (drill.Backup.Tool == "pgbackrest" || drill.Backup.Tool == "wal-g" || drill.Backup.Tool == "walg") {
		spec.Cmd = []string{"sh", "-c", "sleep infinity"}
	}
	if drill.Provider == "etcd" {
		spec.Cmd = []string{"sh", "-c", "sleep infinity"}
	}
	if drill.Restore.Container.Resources.Memory != "" {
		spec.MemoryLimit = parseMemory(drill.Restore.Container.Resources.Memory)
	}
	if drill.Restore.Container.Resources.CPU != "" {
		spec.CPULimit = parseCPU(drill.Restore.Container.Resources.CPU)
	}

	// 1. Provision container
	slog.Info("creating container", "image", spec.Image)
	container, err := e.runtime.Create(ctx, spec)
	if err != nil {
		result.Error = fmt.Errorf("create container: %w", err)
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	result.TargetID = container.ID()
	result.TargetHost = container.Host()
	result.TargetPorts = make(map[int]int, len(spec.Ports))
	for _, port := range spec.Ports {
		result.TargetPorts[port] = container.Port(port)
	}
	result.CleanupSkipped = e.noCleanup
	defer func() {
		if e.noCleanup {
			slog.Info("keeping container (--no-cleanup)", "id", container.ID())
			return
		}
		if cleanupErr := provider.Cleanup(ctx, container); cleanupErr != nil {
			slog.Error("provider cleanup failed", "id", container.ID(), "error", cleanupErr)
			result.Error = joinResultError(result.Error, "cleanup provider", cleanupErr)
		}
		slog.Info("destroying container", "id", container.ID())
		if err := e.runtime.Destroy(ctx, container); err != nil {
			slog.Error("failed to destroy container", "id", container.ID(), "error", err)
			result.Error = joinResultError(result.Error, "destroy target", err)
		}
	}()

	if preflight, ok := provider.(PreflightProvider); ok {
		if err := preflight.Preflight(ctx, e.runtime, drill.Backup, container, drill.Validate); err != nil {
			result.Error = fmt.Errorf("preflight: %w", err)
			result.Duration = time.Since(result.StartedAt)
			return result
		}
	}

	// 2. Restore backup
	slog.Info("restoring backup", "tool", drill.Backup.Tool)
	backup := drill.Backup
	backup.Target = drill.Restore.Target
	restoreResult, err := provider.Restore(ctx, e.runtime, backup, container)
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
			providerCheck := valResult.Checks[i]
			cr.Actual = providerCheck.Actual
			if providerCheck.Duration > 0 {
				cr.Duration = providerCheck.Duration
			}
			if providerCheck.Error != nil {
				cr.Error = providerCheck.Error
				cr.Passed = false
				allPassed = false
				result.Checks = append(result.Checks, cr)
				continue
			}
		}

		passed, evalErr := EvalExpression(check.Expect, cr.Actual)
		cr.Passed = passed
		if cr.Duration == 0 {
			cr.Duration = time.Since(start)
		}
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
	s = strings.TrimSpace(s)
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

	n, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0
	}
	return n * multiplier
}

func joinResultError(current error, prefix string, err error) error {
	wrapped := fmt.Errorf("%s: %w", prefix, err)
	if current == nil {
		return wrapped
	}
	return errors.Join(current, wrapped)
}

// parseCPU converts Kubernetes-style CPU values to Docker NanoCPUs.
func parseCPU(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "m") {
		n, err := strconv.ParseInt(strings.TrimSuffix(s, "m"), 10, 64)
		if err != nil {
			return 0
		}
		return n * 1_000_000
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1_000_000_000)
}

// SortResults orders results by drill name for deterministic persisted state.
func SortResults(results []DrillResult) {
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
}
