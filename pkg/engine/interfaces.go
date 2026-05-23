package engine

import "context"

// Provider handles backup restoration for a specific database/store type.
type Provider interface {
	// Name returns the provider identifier (e.g., "postgres", "mysql").
	Name() string

	// Restore pulls a backup and restores it into the target container.
	Restore(ctx context.Context, rt Runtime, cfg BackupConfig, target Container) (*RestoreResult, error)

	// Validate runs provider-specific checks against the restored data.
	Validate(ctx context.Context, rt Runtime, target Container, checks []Check) (*ValidationResult, error)

	// Cleanup performs provider-specific teardown.
	Cleanup(ctx context.Context, target Container) error
}

// PreflightProvider can validate target capabilities before restore execution.
type PreflightProvider interface {
	Preflight(ctx context.Context, rt Runtime, cfg BackupConfig, target Container, checks []Check) error
}

// RestoreResult holds metadata from a restore operation.
type RestoreResult struct {
	BackupTimestamp string
	BackupSize      int64
	Duration        int64 // milliseconds
}

// ValidationResult holds results from provider-specific validation.
type ValidationResult struct {
	Checks []CheckResult
}

// Reporter publishes drill results.
type Reporter interface {
	// Report publishes results to the configured output(s).
	Report(ctx context.Context, results []DrillResult) error
}
