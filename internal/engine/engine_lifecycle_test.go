package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RamazanKara/restore-drill/internal/config"
)

func TestEngineCleansUpAfterPreflightFailure(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{preflightErr: errors.New("missing required tool")}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)

	result := engine.executeDrill(context.Background(), lifecycleDrill("preflight-fails"))

	if result.Error == nil || !strings.Contains(result.Error.Error(), "preflight: missing required tool") {
		t.Fatalf("expected preflight error, got %v", result.Error)
	}
	if got := provider.cleanupCount(); got != 1 {
		t.Fatalf("expected provider cleanup to run once, got %d", got)
	}
	if got := rt.destroyCount(); got != 1 {
		t.Fatalf("expected runtime destroy to run once, got %d", got)
	}
}

func TestEngineCleansUpAfterRestoreFailure(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{restoreErr: errors.New("restore failed")}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)

	result := engine.executeDrill(context.Background(), lifecycleDrill("restore-fails"))

	if result.Error == nil || !strings.Contains(result.Error.Error(), "restore: restore failed") {
		t.Fatalf("expected restore error, got %v", result.Error)
	}
	if got := provider.cleanupCount(); got != 1 {
		t.Fatalf("expected provider cleanup to run once, got %d", got)
	}
	if got := rt.destroyCount(); got != 1 {
		t.Fatalf("expected runtime destroy to run once, got %d", got)
	}
}

func TestEngineCleansUpAfterValidationFailure(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{validateErr: errors.New("validation failed")}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)

	result := engine.executeDrill(context.Background(), lifecycleDrill("validate-fails"))

	if result.Error == nil || !strings.Contains(result.Error.Error(), "validate: validation failed") {
		t.Fatalf("expected validation error, got %v", result.Error)
	}
	if got := provider.cleanupCount(); got != 1 {
		t.Fatalf("expected provider cleanup to run once, got %d", got)
	}
	if got := rt.destroyCount(); got != 1 {
		t.Fatalf("expected runtime destroy to run once, got %d", got)
	}
}

func TestEngineReportsProviderCleanupFailure(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{cleanupErr: errors.New("remove temp data")}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)

	result := engine.executeDrill(context.Background(), lifecycleDrill("cleanup-fails"))

	if result.Error == nil || !strings.Contains(result.Error.Error(), "cleanup provider: remove temp data") {
		t.Fatalf("expected cleanup error in result, got %v", result.Error)
	}
	if got := provider.cleanupCount(); got != 1 {
		t.Fatalf("expected provider cleanup to run once, got %d", got)
	}
	if got := rt.destroyCount(); got != 1 {
		t.Fatalf("expected runtime destroy to still run once, got %d", got)
	}
}

func TestEngineReportsRuntimeDestroyFailure(t *testing.T) {
	rt := &lifecycleRuntime{destroyErr: errors.New("api refused delete")}
	provider := &lifecycleProvider{}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)

	result := engine.executeDrill(context.Background(), lifecycleDrill("destroy-fails"))

	if result.Error == nil || !strings.Contains(result.Error.Error(), "destroy target: api refused delete") {
		t.Fatalf("expected destroy error in result, got %v", result.Error)
	}
	if got := provider.cleanupCount(); got != 1 {
		t.Fatalf("expected provider cleanup to run once, got %d", got)
	}
	if got := rt.destroyCount(); got != 1 {
		t.Fatalf("expected runtime destroy to run once, got %d", got)
	}
}

func TestEnginePreservesProviderCheckErrors(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{checkErr: errors.New("query failed")}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)

	result := engine.executeDrill(context.Background(), lifecycleDrill("check-error"))

	if result.Error != nil {
		t.Fatalf("expected check failure to stay in check result, got top-level error %v", result.Error)
	}
	if result.ValidationPassed {
		t.Fatal("expected validation to fail")
	}
	if len(result.Checks) != 1 {
		t.Fatalf("expected one check result, got %d", len(result.Checks))
	}
	if result.Checks[0].Error == nil || !strings.Contains(result.Checks[0].Error.Error(), "query failed") {
		t.Fatalf("expected provider check error to be preserved, got %#v", result.Checks[0])
	}
	if result.Checks[0].Passed {
		t.Fatalf("expected errored check not to pass: %#v", result.Checks[0])
	}
}

func TestEngineNoCleanupRetainsTargetDetails(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)
	engine.SetNoCleanup(true)

	result := engine.executeDrill(context.Background(), lifecycleDrill("retain-target"))

	if result.Error != nil {
		t.Fatalf("expected success, got %v", result.Error)
	}
	if !result.CleanupSkipped {
		t.Fatal("expected cleanup to be marked as skipped")
	}
	if result.TargetID == "" {
		t.Fatal("expected retained target id to be recorded")
	}
	if result.TargetHost == "" {
		t.Fatal("expected retained target host to be recorded")
	}
	if result.TargetPorts == nil {
		t.Fatal("expected retained target ports map to be recorded")
	}
	if got := provider.cleanupCount(); got != 0 {
		t.Fatalf("expected provider cleanup to be skipped, got %d calls", got)
	}
	if got := rt.destroyCount(); got != 0 {
		t.Fatalf("expected runtime destroy to be skipped, got %d calls", got)
	}
}

func TestRunParallelPreservesInputOrder(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{
		restoreDelays: map[string]time.Duration{
			"slow": 40 * time.Millisecond,
			"fast": 1 * time.Millisecond,
		},
	}
	reporter := &captureReporter{}
	engine := New(rt, reporter)
	engine.RegisterProvider(provider)

	drills := []config.DrillConfig{
		lifecycleDrillWithSource("first", "slow"),
		lifecycleDrillWithSource("second", "fast"),
	}
	results, err := engine.RunParallel(context.Background(), drills)
	if err != nil {
		t.Fatalf("run parallel: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "first" || results[1].Name != "second" {
		t.Fatalf("expected input order to be preserved, got %q then %q", results[0].Name, results[1].Name)
	}
	if got := len(reporter.results); got != 2 {
		t.Fatalf("expected reporter to receive 2 results, got %d", got)
	}
}

func TestEngineUsesToolContainerForMySQLPhysicalRestore(t *testing.T) {
	rt := &lifecycleRuntime{}
	provider := &lifecycleProvider{name: "mysql"}
	engine := New(rt, &captureReporter{})
	engine.RegisterProvider(provider)

	drill := lifecycleDrill("physical-mysql")
	drill.Provider = "mysql"
	drill.Backup.Tool = "mariabackup"

	result := engine.executeDrill(context.Background(), drill)
	if result.Error != nil {
		t.Fatalf("expected success, got %v", result.Error)
	}

	spec := rt.lastSpec()
	if len(spec.Cmd) != 3 || spec.Cmd[0] != "sh" || spec.Cmd[1] != "-c" || spec.Cmd[2] != "sleep infinity" {
		t.Fatalf("expected physical mysql restore to use tool-container command, got %#v", spec.Cmd)
	}
}

type captureReporter struct {
	results []DrillResult
}

func (r *captureReporter) Report(_ context.Context, results []DrillResult) error {
	r.results = append(r.results[:0], results...)
	return nil
}

type lifecycleRuntime struct {
	mu         sync.Mutex
	nextID     int
	destroyed  []string
	specs      []ContainerSpec
	destroyErr error
}

func (r *lifecycleRuntime) Create(_ context.Context, spec ContainerSpec) (Container, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	r.specs = append(r.specs, spec)

	ports := make(map[int]int, len(spec.Ports))
	for _, port := range spec.Ports {
		ports[port] = port + 10_000
	}

	return &lifecycleContainer{
		id:    fmt.Sprintf("target-%d", r.nextID),
		host:  "127.0.0.1",
		ports: ports,
	}, nil
}

func (r *lifecycleRuntime) Exec(context.Context, Container, []string) ([]byte, error) {
	return nil, nil
}

func (r *lifecycleRuntime) CopyTo(context.Context, Container, string, io.Reader) error {
	return nil
}

func (r *lifecycleRuntime) Destroy(_ context.Context, c Container) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.destroyed = append(r.destroyed, c.ID())
	return r.destroyErr
}

func (r *lifecycleRuntime) destroyCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.destroyed)
}

func (r *lifecycleRuntime) lastSpec() ContainerSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.specs) == 0 {
		return ContainerSpec{}
	}
	return r.specs[len(r.specs)-1]
}

type lifecycleContainer struct {
	id    string
	host  string
	ports map[int]int
}

func (c *lifecycleContainer) ID() string {
	return c.id
}

func (c *lifecycleContainer) Host() string {
	return c.host
}

func (c *lifecycleContainer) Port(containerPort int) int {
	port, ok := c.ports[containerPort]
	if !ok {
		return containerPort
	}
	return port
}

type lifecycleProvider struct {
	name          string
	mu            sync.Mutex
	preflightErr  error
	restoreErr    error
	validateErr   error
	checkErr      error
	cleanupErr    error
	restoreDelays map[string]time.Duration
	cleanupCalls  int
}

func (p *lifecycleProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "lifecycle"
}

func (p *lifecycleProvider) Preflight(context.Context, Runtime, config.BackupConfig, Container, []config.Check) error {
	return p.preflightErr
}

func (p *lifecycleProvider) Restore(ctx context.Context, _ Runtime, cfg config.BackupConfig, _ Container) (*RestoreResult, error) {
	if delay := p.restoreDelays[cfg.Source]; delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if p.restoreErr != nil {
		return nil, p.restoreErr
	}
	return &RestoreResult{BackupTimestamp: "2024-01-02T03:04:05Z"}, nil
}

func (p *lifecycleProvider) Validate(_ context.Context, _ Runtime, _ Container, checks []config.Check) (*ValidationResult, error) {
	if p.validateErr != nil {
		return nil, p.validateErr
	}

	results := make([]CheckResult, 0, len(checks))
	for _, check := range checks {
		results = append(results, CheckResult{
			Name:   check.Name,
			Type:   check.Type,
			Actual: "1",
			Error:  p.checkErr,
		})
	}
	return &ValidationResult{Checks: results}, nil
}

func (p *lifecycleProvider) Cleanup(context.Context, Container) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanupCalls++
	return p.cleanupErr
}

func (p *lifecycleProvider) cleanupCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cleanupCalls
}

func lifecycleDrill(name string) config.DrillConfig {
	return lifecycleDrillWithSource(name, name)
}

func lifecycleDrillWithSource(name, source string) config.DrillConfig {
	return config.DrillConfig{
		Name:     name,
		Provider: "lifecycle",
		Backup: config.BackupConfig{
			Tool:   "fixture",
			Source: source,
		},
		Restore: config.RestoreSpec{
			Target:  "latest",
			Timeout: "5s",
			Container: config.ContainerConf{
				Image: "restore-drill-fixture:latest",
			},
		},
		Validate: []config.Check{
			{
				Name:   "count rows",
				Type:   "query",
				Expect: "== 1",
			},
		},
	}
}
