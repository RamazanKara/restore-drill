package reporter

import (
	"context"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// Multi publishes results to a sequence of reporters.
type Multi struct {
	Reporters []engine.Reporter
}

// NewMulti creates a fan-out reporter.
func NewMulti(reporters ...engine.Reporter) *Multi {
	return &Multi{Reporters: reporters}
}

// Report publishes results to each configured reporter.
func (m *Multi) Report(ctx context.Context, results []engine.DrillResult) error {
	for _, reporter := range m.Reporters {
		if reporter == nil {
			continue
		}
		if err := reporter.Report(ctx, results); err != nil {
			return err
		}
	}
	return nil
}
