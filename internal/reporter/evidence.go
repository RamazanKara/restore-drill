package reporter

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"time"

	"github.com/RamazanKara/restore-drill/internal/state"
)

// EvidenceCheck summarizes a restore-drill-native evidence signal.
type EvidenceCheck struct {
	Area        string
	Check       string
	Description string
	Status      string
}

// EvidenceReport aggregates drill history into a restore evidence view.
type EvidenceReport struct {
	GeneratedAt     time.Time
	PeriodStart     time.Time
	PeriodEnd       time.Time
	TotalRuns       int
	PassedRuns      int
	FailedRuns      int
	SuccessRate     float64
	AvgRTO          time.Duration
	MaxRTO          time.Duration
	DrillSummary    []DrillSummary
	FailureEvidence []FailureEvidence
	EvidenceChecks  []EvidenceCheck
}

// DrillSummary is per-drill aggregated data.
type DrillSummary struct {
	Name        string
	Provider    string
	RunCount    int
	PassCount   int
	FailCount   int
	SuccessRate float64
	LastRun     time.Time
	LastStatus  string
}

// FailureEvidence captures the concrete failed check or run error behind a failed drill.
type FailureEvidence struct {
	Timestamp time.Time
	Drill     string
	Provider  string
	Check     string
	Type      string
	Expected  string
	Actual    string
	Error     string
}

// BuildEvidenceReport builds a report from history.
func BuildEvidenceReport(runs []*state.LastRun, since time.Time) *EvidenceReport {
	report := &EvidenceReport{
		GeneratedAt: time.Now().UTC(),
		PeriodStart: since,
		PeriodEnd:   time.Now().UTC(),
	}

	drillStats := make(map[string]*DrillSummary)
	var totalDuration time.Duration

	for _, run := range runs {
		for _, r := range run.Results {
			report.TotalRuns++

			ds, ok := drillStats[r.Name]
			if !ok {
				ds = &DrillSummary{Name: r.Name, Provider: r.Provider}
				drillStats[r.Name] = ds
			}
			ds.RunCount++

			dur, _ := time.ParseDuration(r.Duration)
			totalDuration += dur
			if dur > report.MaxRTO {
				report.MaxRTO = dur
			}

			if r.ValidationPassed && r.Error == "" {
				report.PassedRuns++
				ds.PassCount++
			} else {
				report.FailedRuns++
				ds.FailCount++
				report.FailureEvidence = append(report.FailureEvidence, failureEvidenceForRun(run.Timestamp, r)...)
			}

			if run.Timestamp.After(ds.LastRun) {
				ds.LastRun = run.Timestamp
				if r.ValidationPassed && r.Error == "" {
					ds.LastStatus = "PASS"
				} else {
					ds.LastStatus = "FAIL"
				}
			}
		}
	}

	if report.TotalRuns > 0 {
		report.SuccessRate = float64(report.PassedRuns) / float64(report.TotalRuns) * 100
		report.AvgRTO = totalDuration / time.Duration(report.TotalRuns)
	}

	for _, ds := range drillStats {
		if ds.RunCount > 0 {
			ds.SuccessRate = float64(ds.PassCount) / float64(ds.RunCount) * 100
		}
		report.DrillSummary = append(report.DrillSummary, *ds)
	}

	report.EvidenceChecks = evaluateEvidenceChecks(report)
	return report
}

func failureEvidenceForRun(ts time.Time, r state.RunResult) []FailureEvidence {
	var evidence []FailureEvidence

	if r.Error != "" {
		evidence = append(evidence, FailureEvidence{
			Timestamp: ts,
			Drill:     r.Name,
			Provider:  r.Provider,
			Check:     "restore-drill",
			Error:     r.Error,
		})
	}

	for _, check := range r.Checks {
		if check.Passed && check.Error == "" {
			continue
		}
		evidence = append(evidence, FailureEvidence{
			Timestamp: ts,
			Drill:     r.Name,
			Provider:  r.Provider,
			Check:     check.Name,
			Type:      check.Type,
			Expected:  check.Expected,
			Actual:    check.Actual,
			Error:     check.Error,
		})
	}

	return evidence
}

func evaluateEvidenceChecks(r *EvidenceReport) []EvidenceCheck {
	historyPresent := "PASS"
	if r.TotalRuns == 0 {
		historyPresent = "FAIL"
	}

	restoreOutcomes := "PASS"
	if r.TotalRuns == 0 {
		restoreOutcomes = "FAIL"
	} else if r.FailedRuns > 0 {
		restoreOutcomes = "WARN"
	}

	failureEvidence := "PASS"
	if r.FailedRuns > 0 && len(r.FailureEvidence) == 0 {
		failureEvidence = "FAIL"
	} else if r.FailedRuns > 0 {
		failureEvidence = "WARN"
	}

	return []EvidenceCheck{
		{
			Area:        "History",
			Check:       "run-history-present",
			Description: "At least one restore-drill run exists in the selected reporting window.",
			Status:      historyPresent,
		},
		{
			Area:        "Restore result",
			Check:       "all-drills-passed",
			Description: "Every recorded restore drill in the reporting window completed without restore or validation failure.",
			Status:      restoreOutcomes,
		},
		{
			Area:        "Failure evidence",
			Check:       "failure-details-captured",
			Description: "Failed restore drills include check-level or run-level evidence for follow-up.",
			Status:      failureEvidence,
		},
	}
}

// RenderHTML writes an HTML evidence report.
func RenderHTML(w io.Writer, report *EvidenceReport) error {
	return htmlTmpl.Execute(w, report)
}

// RenderJSON writes a JSON evidence report.
func RenderJSON(w io.Writer, report *EvidenceReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// FormatDuration formats a duration for display.
func FormatDuration(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Truncate(time.Second).String()
}

var funcMap = template.FuncMap{
	"fmtDuration": FormatDuration,
	"fmtTime": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.Format("2006-01-02 15:04 UTC")
	},
	"fmtPercent": func(f float64) string {
		return fmt.Sprintf("%.1f%%", f)
	},
	"statusClass": func(s string) string {
		switch s {
		case "PASS":
			return "status-pass"
		case "FAIL":
			return "status-fail"
		default:
			return "status-warn"
		}
	},
}

var htmlTmpl = template.Must(template.New("evidence").Funcs(funcMap).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>restore-drill Evidence Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 2em auto; max-width: 960px; color: #333; }
  h1 { border-bottom: 2px solid #2563eb; padding-bottom: 0.3em; }
  h2 { margin-top: 2em; color: #1e40af; }
  table { border-collapse: collapse; width: 100%; margin: 1em 0; }
  th, td { border: 1px solid #d1d5db; padding: 0.5em 0.75em; text-align: left; }
  th { background: #f3f4f6; }
  .status-pass { color: #16a34a; font-weight: bold; }
  .status-fail { color: #dc2626; font-weight: bold; }
  .status-warn { color: #d97706; font-weight: bold; }
  .evidence { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 0.9em; white-space: pre-wrap; }
  .summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 1em; margin: 1em 0; }
  .summary-card { border: 1px solid #e5e7eb; border-radius: 8px; padding: 1em; text-align: center; }
  .summary-card .value { font-size: 2em; font-weight: bold; color: #1e40af; }
  .summary-card .label { font-size: 0.85em; color: #6b7280; }
  footer { margin-top: 3em; font-size: 0.8em; color: #9ca3af; border-top: 1px solid #e5e7eb; padding-top: 1em; }
</style>
</head>
<body>
<h1>Backup Restore Verification Evidence</h1>
<p>Generated: {{fmtTime .GeneratedAt}} | Period: {{fmtTime .PeriodStart}} – {{fmtTime .PeriodEnd}}</p>

<h2>Executive Summary</h2>
<div class="summary-grid">
  <div class="summary-card"><div class="value">{{.TotalRuns}}</div><div class="label">Total Drills</div></div>
  <div class="summary-card"><div class="value">{{.PassedRuns}}</div><div class="label">Passed</div></div>
  <div class="summary-card"><div class="value">{{.FailedRuns}}</div><div class="label">Failed</div></div>
  <div class="summary-card"><div class="value">{{fmtPercent .SuccessRate}}</div><div class="label">Success Rate</div></div>
  <div class="summary-card"><div class="value">{{fmtDuration .AvgRTO}}</div><div class="label">Avg RTO</div></div>
  <div class="summary-card"><div class="value">{{fmtDuration .MaxRTO}}</div><div class="label">Max RTO</div></div>
</div>

<h2>Evidence Checks</h2>
<table>
<tr><th>Area</th><th>Check</th><th>Description</th><th>Status</th></tr>
{{range .EvidenceChecks}}
<tr><td>{{.Area}}</td><td>{{.Check}}</td><td>{{.Description}}</td><td class="{{statusClass .Status}}">{{.Status}}</td></tr>
{{end}}
</table>

{{if .FailureEvidence}}
<h2>Failure Evidence</h2>
<table>
<tr><th>Time</th><th>Drill</th><th>Check</th><th>Expected</th><th>Actual</th><th>Error</th></tr>
{{range .FailureEvidence}}
<tr>
  <td>{{fmtTime .Timestamp}}</td>
  <td>{{.Drill}} <span class="evidence">({{.Provider}})</span></td>
  <td>{{.Check}}{{if .Type}} <span class="evidence">[{{.Type}}]</span>{{end}}</td>
  <td class="evidence">{{.Expected}}</td>
  <td class="evidence">{{.Actual}}</td>
  <td class="evidence">{{.Error}}</td>
</tr>
{{end}}
</table>
{{end}}

<h2>Drill History</h2>
<table>
<tr><th>Drill</th><th>Provider</th><th>Runs</th><th>Pass</th><th>Fail</th><th>Success Rate</th><th>Last Run</th><th>Status</th></tr>
{{range .DrillSummary}}
<tr>
  <td>{{.Name}}</td>
  <td>{{.Provider}}</td>
  <td>{{.RunCount}}</td>
  <td>{{.PassCount}}</td>
  <td>{{.FailCount}}</td>
  <td>{{fmtPercent .SuccessRate}}</td>
  <td>{{fmtTime .LastRun}}</td>
  <td class="{{statusClass .LastStatus}}">{{.LastStatus}}</td>
</tr>
{{end}}
</table>

<footer>
  <p>Report generated by <strong>restore-drill</strong> — automated backup verification for self-hosted infrastructure.</p>
  <p>Evidence checks are evaluated automatically from restore drill history. A "PASS" status means the evidence condition was met for the selected reporting period.</p>
</footer>
</body>
</html>
`))
