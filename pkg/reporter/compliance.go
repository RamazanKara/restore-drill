package reporter

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"time"

	"github.com/RamazanKara/restore-drill/internal/state"
)

// ComplianceControl maps a drill outcome to a regulatory control.
type ComplianceControl struct {
	Framework string
	Control   string
	Title     string
	Status    string
}

// ComplianceReport aggregates drill history into a compliance view.
type ComplianceReport struct {
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
	Controls        []ComplianceControl
}

// DrillSummary is per-drill aggregated data.
type DrillSummary struct {
	Name        string
	Provider    string
	RunCount    int
	PassCount   int
	FailCount   int
	SuccessRate float64
	AvgDuration time.Duration
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

// BuildComplianceReport builds a report from history.
func BuildComplianceReport(runs []*state.LastRun, since time.Time) *ComplianceReport {
	report := &ComplianceReport{
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

	report.Controls = evaluateControls(report)
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

func evaluateControls(r *ComplianceReport) []ComplianceControl {
	pass := "COMPLIANT"
	fail := "NON-COMPLIANT"
	partial := "PARTIAL"

	backupTested := pass
	if r.TotalRuns == 0 {
		backupTested = fail
	} else if r.SuccessRate < 100 {
		backupTested = partial
	}

	rtoMet := pass
	if r.MaxRTO > 15*time.Minute {
		rtoMet = partial
	}

	return []ComplianceControl{
		{
			Framework: "ISO 27001:2022",
			Control:   "A.8.13",
			Title:     "Information backup — restore testing",
			Status:    backupTested,
		},
		{
			Framework: "NIS2 Directive",
			Control:   "Art. 21(2)(c)",
			Title:     "Business continuity and crisis management — backup restore verification",
			Status:    backupTested,
		},
		{
			Framework: "BSI C5:2020",
			Control:   "OPS-04",
			Title:     "Data backup concept — regular restore tests",
			Status:    backupTested,
		},
		{
			Framework: "BSI C5:2020",
			Control:   "OPS-05",
			Title:     "Recovery time objectives — restore within RTO",
			Status:    rtoMet,
		},
		{
			Framework: "SOC 2",
			Control:   "A1.2",
			Title:     "Recovery testing — environmental provisions for recovery",
			Status:    backupTested,
		},
	}
}

// RenderHTML writes an HTML compliance report.
func RenderHTML(w io.Writer, report *ComplianceReport) error {
	return htmlTmpl.Execute(w, report)
}

// RenderJSON writes a JSON compliance report.
func RenderJSON(w io.Writer, report *ComplianceReport) error {
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
		case "COMPLIANT", "PASS":
			return "status-pass"
		case "NON-COMPLIANT", "FAIL":
			return "status-fail"
		default:
			return "status-partial"
		}
	},
}

var htmlTmpl = template.Must(template.New("compliance").Funcs(funcMap).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>restore-drill Compliance Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; margin: 2em auto; max-width: 960px; color: #333; }
  h1 { border-bottom: 2px solid #2563eb; padding-bottom: 0.3em; }
  h2 { margin-top: 2em; color: #1e40af; }
  table { border-collapse: collapse; width: 100%; margin: 1em 0; }
  th, td { border: 1px solid #d1d5db; padding: 0.5em 0.75em; text-align: left; }
  th { background: #f3f4f6; }
  .status-pass { color: #16a34a; font-weight: bold; }
  .status-fail { color: #dc2626; font-weight: bold; }
  .status-partial { color: #d97706; font-weight: bold; }
  .evidence { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 0.9em; white-space: pre-wrap; }
  .summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 1em; margin: 1em 0; }
  .summary-card { border: 1px solid #e5e7eb; border-radius: 8px; padding: 1em; text-align: center; }
  .summary-card .value { font-size: 2em; font-weight: bold; color: #1e40af; }
  .summary-card .label { font-size: 0.85em; color: #6b7280; }
  footer { margin-top: 3em; font-size: 0.8em; color: #9ca3af; border-top: 1px solid #e5e7eb; padding-top: 1em; }
</style>
</head>
<body>
<h1>Backup Restore Verification — Compliance Report</h1>
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

<h2>Compliance Controls</h2>
<table>
<tr><th>Framework</th><th>Control</th><th>Title</th><th>Status</th></tr>
{{range .Controls}}
<tr><td>{{.Framework}}</td><td>{{.Control}}</td><td>{{.Title}}</td><td class="{{statusClass .Status}}">{{.Status}}</td></tr>
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
  <p>Controls are evaluated automatically based on drill execution history. A "COMPLIANT" status means all restore drills within the reporting period passed successfully.</p>
</footer>
</body>
</html>
`))
