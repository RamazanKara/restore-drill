package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RamazanKara/restore-drill/internal/state"
	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/RamazanKara/restore-drill/pkg/reporter"
)

func buildReporter(format string, cfg *engine.Config, out io.Writer) engine.Reporter {
	reporters := make([]engine.Reporter, 0, 2)
	switch format {
	case "json":
		reporters = append(reporters, &reporter.JSON{Writer: out, Pretty: true})
	default:
		reporters = append(reporters, &reporter.Stdout{Writer: out})
	}

	seen := make(map[string]struct{})
	for _, drill := range cfg.Drills {
		for _, alert := range drill.Alerts {
			url := webhookAlertURL(alert)
			if url == "" {
				continue
			}

			var base engine.Reporter
			switch alert.Type {
			case "webhook":
				base = reporter.NewWebhook(url, alert.Headers)
			case "slack":
				base = reporter.NewSlack(url, alert.Headers)
			default:
				continue
			}

			onlyOnFailure := alert.On == "failure"
			key := fmt.Sprintf("%s|%t|%s", alert.Type, onlyOnFailure, webhookAlertKey(url, alert.Headers))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			if onlyOnFailure {
				base = reporter.NewConditional(base, true)
			}
			reporters = append(reporters, base)
		}
	}

	if len(reporters) == 1 {
		return reporters[0]
	}
	return reporter.NewMulti(reporters...)
}

func webhookAlertURL(alert engine.AlertSpec) string {
	if alert.URL != "" {
		return alert.URL
	}
	return alert.Endpoint
}

func webhookAlertKey(url string, headers map[string]string) string {
	if len(headers) == 0 {
		return url
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(url)
	for _, key := range keys {
		b.WriteString("\n")
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(headers[key])
	}
	return b.String()
}

func saveState(results []engine.DrillResult) *state.LastRun {
	engine.SortResults(results)
	run := stateRunFromResults(results, time.Now())

	if err := state.Save(state.DefaultPath(), run); err != nil {
		slog.Warn("failed to save state", "error", err)
	}
	if err := state.AppendHistory(run); err != nil {
		slog.Warn("failed to append history", "error", err)
	}
	return run
}

func stateRunFromResults(results []engine.DrillResult, timestamp time.Time) *state.LastRun {
	run := &state.LastRun{
		Timestamp: timestamp,
		Results:   make([]state.RunResult, 0, len(results)),
	}
	for _, r := range results {
		sr := state.RunResult{
			Name:             r.Name,
			Provider:         r.Provider,
			StartedAt:        r.StartedAt,
			Duration:         r.Duration.String(),
			BackupTimestamp:  r.BackupTimestamp,
			ValidationPassed: r.ValidationPassed,
			CleanupSkipped:   r.CleanupSkipped,
			TargetID:         r.TargetID,
			TargetHost:       r.TargetHost,
			TargetPorts:      r.TargetPorts,
		}
		if r.Error != nil {
			sr.Error = r.Error.Error()
		}
		if !r.BackupTimestamp.IsZero() {
			sr.BackupAge = r.BackupAge.Truncate(time.Second).String()
		}
		for _, c := range r.Checks {
			sc := state.CheckResult{
				Name:     c.Name,
				Type:     c.Type,
				Expected: c.Expected,
				Actual:   c.Actual,
				Passed:   c.Passed,
			}
			if c.Error != nil {
				sc.Error = c.Error.Error()
			}
			sr.Checks = append(sr.Checks, sc)
		}
		run.Results = append(run.Results, sr)
	}
	return run
}

func writeConfiguredReports(ctx context.Context, cfg engine.ReportConfig, results []engine.DrillResult, currentRun *state.LastRun) error {
	if strings.TrimSpace(cfg.Output) == "" {
		return nil
	}
	formats := configuredFileReportFormats(cfg.Format)
	if len(formats) == 0 {
		return nil
	}
	if currentRun == nil {
		currentRun = stateRunFromResults(results, time.Now())
	}

	retention, err := cfg.RetentionDuration()
	if err != nil {
		return fmt.Errorf("parse reporting retention: %w", err)
	}
	since := currentRun.Timestamp.Add(-retention)
	timestamp := currentRun.Timestamp.UTC().Format("20060102T150405Z")

	for _, format := range formats {
		path := configuredReportPath(cfg.Output, format, len(formats) > 1, timestamp)
		if err := writeReportFile(path, func(w io.Writer) error {
			switch format {
			case "json":
				return (&reporter.JSON{Writer: w, Pretty: true}).Report(ctx, results)
			case "html":
				runs, err := state.LoadHistory(since)
				if err != nil {
					return err
				}
				if len(runs) == 0 {
					runs = []*state.LastRun{currentRun}
				}
				return reporter.RenderHTML(w, reporter.BuildEvidenceReport(runs, since))
			default:
				return nil
			}
		}); err != nil {
			return fmt.Errorf("write %s report: %w", format, err)
		}
		slog.Info("report written", "format", format, "path", path)
	}
	return nil
}

func configuredFileReportFormats(formats []string) []string {
	seen := make(map[string]struct{})
	fileFormats := make([]string, 0, len(formats))
	for _, format := range formats {
		switch format {
		case "json", "html":
			if _, ok := seen[format]; ok {
				continue
			}
			seen[format] = struct{}{}
			fileFormats = append(fileFormats, format)
		}
	}
	return fileFormats
}

func configuredReportPath(output, format string, multiple bool, timestamp string) string {
	rawOutput := strings.TrimSpace(output)
	cleanOutput := filepath.Clean(rawOutput)
	if reportOutputIsDir(rawOutput, cleanOutput, multiple) {
		return filepath.Join(cleanOutput, configuredReportFilename(format, timestamp))
	}
	return cleanOutput
}

func reportOutputIsDir(rawOutput, cleanOutput string, multiple bool) bool {
	if multiple {
		return true
	}
	if strings.HasSuffix(rawOutput, "/") || strings.HasSuffix(rawOutput, string(os.PathSeparator)) {
		return true
	}
	if info, err := os.Stat(cleanOutput); err == nil && info.IsDir() {
		return true
	}
	return filepath.Ext(cleanOutput) == ""
}

func configuredReportFilename(format, timestamp string) string {
	switch format {
	case "html":
		return "restore-drill-compliance-" + timestamp + ".html"
	default:
		return "restore-drill-run-" + timestamp + ".json"
	}
}

func writeReportFile(path string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	f, err := os.Create(path) // #nosec G304 -- report output path is an explicit config value.
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	renderErr := write(f)
	if closeErr := f.Close(); closeErr != nil && renderErr == nil {
		renderErr = fmt.Errorf("close report file: %w", closeErr)
	}
	return renderErr
}
