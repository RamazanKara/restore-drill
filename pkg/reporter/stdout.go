// Package reporter implements drill result output formats.
package reporter

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/RamazanKara/restore-drill/pkg/engine"
)

// Stdout writes drill results as a formatted table to stdout.
type Stdout struct {
	Writer io.Writer
}

// NewStdout creates a stdout reporter.
func NewStdout() *Stdout {
	return &Stdout{Writer: os.Stdout}
}

// Report prints drill results as a table.
func (r *Stdout) Report(_ context.Context, results []engine.DrillResult) error {
	w := tabwriter.NewWriter(r.Writer, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintf(w, "\n%s\t%s\t%s\t%s\t%s\n", "DRILL", "PROVIDER", "STATUS", "DURATION", "CHECKS"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", "-----", "--------", "------", "--------", "------"); err != nil {
		return err
	}

	allPassed := true
	for _, res := range results {
		status := "PASS"
		if res.Error != nil || !res.ValidationPassed {
			status = "FAIL"
			allPassed = false
		}

		passed := 0
		total := len(res.Checks)
		for _, c := range res.Checks {
			if c.Passed {
				passed++
			}
		}

		checkStr := fmt.Sprintf("%d/%d", passed, total)
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			res.Name, res.Provider, status, res.Duration.Truncate(1e6).String(), checkStr); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// Print detailed check results
	for _, res := range results {
		if len(res.Checks) == 0 && res.Error == nil && !res.CleanupSkipped {
			continue
		}
		if len(res.Checks) > 0 {
			if _, err := fmt.Fprintf(r.Writer, "  %s checks:\n", res.Name); err != nil {
				return err
			}
			for _, c := range res.Checks {
				icon := "OK"
				if !c.Passed {
					icon = "FAIL"
				}
				detail := ""
				if c.Error != nil {
					detail = fmt.Sprintf(" (%s)", c.Error)
				} else if !c.Passed {
					detail = fmt.Sprintf(" (got: %s, want: %s)", c.Actual, c.Expected)
				}
				if _, err := fmt.Fprintf(r.Writer, "    %s %s [%s]%s\n", icon, c.Name, c.Duration.Truncate(1e6).String(), detail); err != nil {
					return err
				}
			}
		} else if _, err := fmt.Fprintf(r.Writer, "  %s:\n", res.Name); err != nil {
			return err
		}
		if res.Error != nil {
			if _, err := fmt.Fprintf(r.Writer, "    error: %s\n", res.Error); err != nil {
				return err
			}
		}
		if res.CleanupSkipped {
			if _, err := fmt.Fprintf(r.Writer, "    retained target: %s (%s %v)\n", res.TargetID, res.TargetHost, res.TargetPorts); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(r.Writer); err != nil {
			return err
		}
	}

	if !allPassed {
		if _, err := fmt.Fprintf(r.Writer, "%s\n", strings.Repeat("-", 40)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(r.Writer, "Result: FAILED\n"); err != nil {
			return err
		}
	} else if len(results) > 0 {
		if _, err := fmt.Fprintf(r.Writer, "%s\n", strings.Repeat("-", 40)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(r.Writer, "Result: PASSED\n"); err != nil {
			return err
		}
	}

	return nil
}
