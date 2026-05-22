// Package reporter implements drill result output formats.
package reporter

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/fluentorbit/restore-drill/pkg/engine"
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

	fmt.Fprintf(w, "\n%s\t%s\t%s\t%s\t%s\n", "DRILL", "PROVIDER", "STATUS", "DURATION", "CHECKS")
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", "-----", "--------", "------", "--------", "------")

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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			res.Name, res.Provider, status, res.Duration.Truncate(1e6).String(), checkStr)
	}

	fmt.Fprintln(w)
	w.Flush()

	// Print detailed check results
	for _, res := range results {
		if len(res.Checks) == 0 {
			continue
		}
		fmt.Fprintf(r.Writer, "  %s checks:\n", res.Name)
		for _, c := range res.Checks {
			icon := "✓"
			if !c.Passed {
				icon = "✗"
			}
			detail := ""
			if c.Error != nil {
				detail = fmt.Sprintf(" (%s)", c.Error)
			} else if !c.Passed {
				detail = fmt.Sprintf(" (got: %s, want: %s)", c.Actual, c.Expected)
			}
			fmt.Fprintf(r.Writer, "    %s %s [%s]%s\n", icon, c.Name, c.Duration.Truncate(1e6).String(), detail)
		}
		fmt.Fprintln(r.Writer)
	}

	if !allPassed {
		fmt.Fprintf(r.Writer, "%s\n", strings.Repeat("─", 40))
		fmt.Fprintf(r.Writer, "Result: FAILED\n")
	} else if len(results) > 0 {
		fmt.Fprintf(r.Writer, "%s\n", strings.Repeat("─", 40))
		fmt.Fprintf(r.Writer, "Result: PASSED\n")
	}

	return nil
}
