package reporter

import (
	"context"
	"html"
	"io"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/RamazanKara/restore-drill/internal/engine"
)

// Markdown writes drill results as a Markdown report.
type Markdown struct {
	Writer io.Writer
}

func (r *Markdown) Report(_ context.Context, results []engine.DrillResult) error {
	return markdownRunTmpl.Execute(r.Writer, jsonResultsFromDrillResults(results))
}

// RenderMarkdown writes a Markdown evidence report.
func RenderMarkdown(w io.Writer, report *EvidenceReport) error {
	sorted := *report
	sorted.DrillSummary = append([]DrillSummary(nil), report.DrillSummary...)
	sort.Slice(sorted.DrillSummary, func(i, j int) bool {
		return sorted.DrillSummary[i].Name < sorted.DrillSummary[j].Name
	})
	return markdownEvidenceTmpl.Execute(w, &sorted)
}

var markdownEscapes = strings.NewReplacer(
	"\\", "\\\\", "|", "&#124;", "\r\n", "<br>", "\r", "<br>", "\n", "<br>",
	"`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]",
	"#", "\\#", "!", "\\!", "~", "\\~",
)

func escapeMarkdown(s string) string {
	return markdownEscapes.Replace(html.EscapeString(s))
}

var markdownFuncs = template.FuncMap{
	"md":       escapeMarkdown,
	"duration": FormatDuration,
	"timestamp": func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}
		return t.UTC().Format(time.RFC3339)
	},
}

var markdownRunTmpl = template.Must(template.New("run-markdown").Funcs(markdownFuncs).Parse(`# Restore drill results

| Drill | Provider | Status | Duration | Started |
| --- | --- | --- | --- | --- |
{{range .}}| {{md .Name}} | {{md .Provider}} | {{.Status}} | {{md .Duration}} | {{timestamp .StartedAt}} |
{{else}}No drills recorded.
{{end}}{{range .}}
## {{md .Name}}
{{if .Error}}
Error: {{md .Error}}
{{end}}{{if .BackupTimestamp}}
Backup timestamp: {{md .BackupTimestamp}}; backup age: {{md .BackupAge}}
{{end}}{{if .CleanupSkipped}}
Retained target: {{md .TargetID}}; host: {{md .TargetHost}}
{{range $port, $hostPort := .TargetPorts}}
- Port {{$port}} → {{$hostPort}}
{{end}}{{end}}{{if .Checks}}
| Check | Type | Status | Expected | Actual | Duration | Error |
| --- | --- | --- | --- | --- | --- | --- |
{{range .Checks}}| {{md .Name}} | {{md .Type}} | {{if .Passed}}PASS{{else}}FAIL{{end}} | {{md .Expected}} | {{md .Actual}} | {{md .Duration}} | {{md .Error}} |
{{end}}{{else}}
No checks recorded.
{{end}}{{end}}`))

var markdownEvidenceTmpl = template.Must(template.New("evidence-markdown").Funcs(markdownFuncs).Parse(`# Backup Restore Verification Evidence

Generated: {{timestamp .GeneratedAt}}

Period: {{timestamp .PeriodStart}} – {{timestamp .PeriodEnd}}

| Total drills | Passed | Failed | Success rate | Avg RTO | Max RTO |
| --- | --- | --- | --- | --- | --- |
| {{.TotalRuns}} | {{.PassedRuns}} | {{.FailedRuns}} | {{printf "%.1f%%" .SuccessRate}} | {{duration .AvgRTO}} | {{duration .MaxRTO}} |

## Evidence Checks

| Area | Check | Description | Status |
| --- | --- | --- | --- |
{{range .EvidenceChecks}}| {{md .Area}} | {{md .Check}} | {{md .Description}} | {{md .Status}} |
{{end}}
## Failure Evidence
{{if .FailureEvidence}}
| Time | Drill | Provider | Check | Type | Expected | Actual | Error |
| --- | --- | --- | --- | --- | --- | --- | --- |
{{range .FailureEvidence}}| {{timestamp .Timestamp}} | {{md .Drill}} | {{md .Provider}} | {{md .Check}} | {{md .Type}} | {{md .Expected}} | {{md .Actual}} | {{md .Error}} |
{{end}}{{else}}
No failure details recorded.
{{end}}
## Drill History

| Drill | Provider | Runs | Pass | Fail | Success rate | Last run | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
{{range .DrillSummary}}| {{md .Name}} | {{md .Provider}} | {{.RunCount}} | {{.PassCount}} | {{.FailCount}} | {{printf "%.1f%%" .SuccessRate}} | {{timestamp .LastRun}} | {{md .LastStatus}} |
{{else}}No drills recorded.
{{end}}`))
