package reporter

import (
	"context"

	"github.com/RamazanKara/restore-drill/internal/engine"
)

// Conditional wraps a reporter and delegates only when the results match its
// gating condition. It is used to implement alert "on: failure" semantics.
type Conditional struct {
	Inner         engine.Reporter
	OnlyOnFailure bool
}

// NewConditional returns a reporter that forwards to inner. When onlyOnFailure
// is true, it forwards only when at least one drill errored or failed validation.
func NewConditional(inner engine.Reporter, onlyOnFailure bool) *Conditional {
	return &Conditional{Inner: inner, OnlyOnFailure: onlyOnFailure}
}

// Report forwards results to the inner reporter when the condition is met.
func (c *Conditional) Report(ctx context.Context, results []engine.DrillResult) error {
	if c.Inner == nil {
		return nil
	}
	if c.OnlyOnFailure && !anyFailed(results) {
		return nil
	}
	return c.Inner.Report(ctx, results)
}
