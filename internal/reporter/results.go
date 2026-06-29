package reporter

import (
	"time"

	"github.com/RamazanKara/restore-drill/internal/engine"
)

func jsonResultsFromDrillResults(results []engine.DrillResult) []jsonResult {
	output := make([]jsonResult, 0, len(results))
	for _, res := range results {
		jr := jsonResult{
			Name:             res.Name,
			Provider:         res.Provider,
			Status:           "pass",
			StartedAt:        res.StartedAt,
			Duration:         res.Duration.String(),
			DurationMs:       res.Duration.Milliseconds(),
			ValidationPassed: res.ValidationPassed,
			CleanupSkipped:   res.CleanupSkipped,
			TargetID:         res.TargetID,
			TargetHost:       res.TargetHost,
			TargetPorts:      res.TargetPorts,
		}

		if res.Error != nil || !res.ValidationPassed {
			jr.Status = "fail"
		}
		if res.Error != nil {
			jr.Error = res.Error.Error()
		}
		if !res.BackupTimestamp.IsZero() {
			jr.BackupTimestamp = res.BackupTimestamp.Format(time.RFC3339)
			jr.BackupAge = res.BackupAge.Truncate(time.Second).String()
		}

		for _, c := range res.Checks {
			jc := jsonCheck{
				Name:     c.Name,
				Type:     c.Type,
				Expected: c.Expected,
				Actual:   c.Actual,
				Passed:   c.Passed,
				Duration: c.Duration.String(),
			}
			if c.Error != nil {
				jc.Error = c.Error.Error()
			}
			jr.Checks = append(jr.Checks, jc)
		}

		output = append(output, jr)
	}
	return output
}
