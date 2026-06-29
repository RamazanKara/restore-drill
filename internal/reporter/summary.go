package reporter

import "github.com/RamazanKara/restore-drill/internal/engine"

// drillPassed reports whether a single drill fully succeeded: it neither errored
// nor failed any validation check.
func drillPassed(r engine.DrillResult) bool {
	return r.Error == nil && r.ValidationPassed
}

// anyFailed reports whether any drill in the set errored or failed validation.
func anyFailed(results []engine.DrillResult) bool {
	for _, r := range results {
		if !drillPassed(r) {
			return true
		}
	}
	return false
}

// countResults returns the number of passed and failed drills.
func countResults(results []engine.DrillResult) (passed, failed int) {
	for _, r := range results {
		if drillPassed(r) {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}
