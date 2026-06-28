package reporter

import (
	"context"
	"errors"
	"testing"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

type countingReporter struct {
	calls int
}

func (c *countingReporter) Report(context.Context, []engine.DrillResult) error {
	c.calls++
	return nil
}

func TestConditionalSkipsWhenAllPass(t *testing.T) {
	inner := &countingReporter{}
	cond := NewConditional(inner, true)

	err := cond.Report(context.Background(), []engine.DrillResult{
		{Name: "a", ValidationPassed: true},
		{Name: "b", ValidationPassed: true},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if inner.calls != 0 {
		t.Errorf("expected inner reporter to be skipped, called %d times", inner.calls)
	}
}

func TestConditionalForwardsOnFailure(t *testing.T) {
	for _, tt := range []struct {
		name    string
		results []engine.DrillResult
	}{
		{name: "validation failed", results: []engine.DrillResult{{Name: "a", ValidationPassed: false}}},
		{name: "errored", results: []engine.DrillResult{{Name: "a", ValidationPassed: true, Error: errors.New("boom")}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inner := &countingReporter{}
			cond := NewConditional(inner, true)
			if err := cond.Report(context.Background(), tt.results); err != nil {
				t.Fatalf("report: %v", err)
			}
			if inner.calls != 1 {
				t.Errorf("expected inner reporter to be called once, got %d", inner.calls)
			}
		})
	}
}

func TestConditionalAlwaysForwardsWhenNotGated(t *testing.T) {
	inner := &countingReporter{}
	cond := NewConditional(inner, false)

	if err := cond.Report(context.Background(), []engine.DrillResult{{Name: "a", ValidationPassed: true}}); err != nil {
		t.Fatalf("report: %v", err)
	}
	if inner.calls != 1 {
		t.Errorf("expected inner reporter to always be called, got %d", inner.calls)
	}
}
