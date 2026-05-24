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
	"text/tabwriter"
	"time"

	"github.com/RamazanKara/restore-drill/internal/logging"
	"github.com/RamazanKara/restore-drill/internal/state"
	"github.com/RamazanKara/restore-drill/internal/version"
	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/RamazanKara/restore-drill/pkg/metrics"
	"github.com/RamazanKara/restore-drill/pkg/providers/mysql"
	"github.com/RamazanKara/restore-drill/pkg/providers/postgres"
	"github.com/RamazanKara/restore-drill/pkg/providers/redis"
	"github.com/RamazanKara/restore-drill/pkg/reporter"
	"github.com/RamazanKara/restore-drill/pkg/runtime/docker"
	"github.com/RamazanKara/restore-drill/pkg/runtime/k8s"
	"github.com/spf13/cobra"
)

func main() {
	var verbose bool

	root := &cobra.Command{
		Use:   "restore-drill",
		Short: "Automated backup verification for self-hosted infrastructure",
		Long:  "restore-drill continuously proves your recovery works by restoring real backups into ephemeral environments, running validation queries, and publishing RTO/RPO as Prometheus metrics.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Setup(verbose)
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")

	root.AddCommand(runCmd())
	root.AddCommand(validateCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(reportCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	var configPath string
	var format string
	var parallel bool
	var noCleanup bool
	var target string
	var runtimeMode string
	var kubeNamespace string
	var kubeServiceAccount string
	var kubePodLabels []string
	var kubePodAnnotations []string
	var kubeImagePullSecrets []string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute backup restore drills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := engine.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Override target (PITR timestamp) if provided
			if target != "" {
				for i := range cfg.Drills {
					cfg.Drills[i].Restore.Target = target
				}
				slog.Info("PITR target override", "target", target)
			}

			kubeLabels, err := parseKeyValueFlags(kubePodLabels, "--kube-pod-label")
			if err != nil {
				return err
			}
			kubeAnnotations, err := parseKeyValueFlags(kubePodAnnotations, "--kube-pod-annotation")
			if err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{
				mode:             runtimeMode,
				namespace:        kubeNamespace,
				serviceAccount:   kubeServiceAccount,
				podLabels:        kubeLabels,
				podAnnotations:   kubeAnnotations,
				imagePullSecrets: kubeImagePullSecrets,
			})
			if err != nil {
				return err
			}

			rep := buildReporter(format, cfg)

			eng := engine.New(rt, rep)
			if noCleanup {
				eng.SetNoCleanup(true)
			}

			// Register providers
			eng.RegisterProvider(postgres.New())
			eng.RegisterProvider(mysql.New())
			eng.RegisterProvider(redis.New())

			var results []engine.DrillResult
			if parallel {
				results, err = eng.RunParallel(cmd.Context(), cfg.Drills)
			} else {
				results, err = eng.Run(cmd.Context(), cfg.Drills)
			}
			if err != nil {
				return err
			}

			// Save state for 'status' command and configured file reports.
			currentRun := saveState(results)

			reportErr := writeConfiguredReports(cmd.Context(), cfg.Reporting, results, currentRun)
			if reportErr != nil {
				slog.Error("failed to write configured reports", "error", reportErr)
			}

			// Push metrics if configured
			if cfg.Metrics.Prometheus.Enabled && cfg.Metrics.Prometheus.Pushgateway != "" {
				if pushErr := metrics.PushResults(results, cfg.Metrics.Prometheus.Pushgateway, cfg.Metrics.Prometheus.Labels); pushErr != nil {
					slog.Error("failed to push metrics", "error", pushErr)
				}
			}

			failed := false
			for _, r := range results {
				if r.Error != nil || !r.ValidationPassed {
					failed = true
					break
				}
			}
			if failed {
				return fmt.Errorf("one or more drills failed")
			}
			if reportErr != nil {
				return reportErr
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "drill.yaml", "Path to drill configuration file")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")
	cmd.Flags().BoolVar(&parallel, "parallel", false, "Run drills concurrently")
	cmd.Flags().BoolVar(&noCleanup, "no-cleanup", false, "Keep containers running after drill (for debugging)")
	cmd.Flags().StringVar(&target, "target", "", "PITR target timestamp (e.g. 2024-01-15T10:30:00Z) for incident recovery mode")
	cmd.Flags().StringVar(&runtimeMode, "runtime", "auto", "Runtime: auto, docker, kubernetes")
	cmd.Flags().StringVar(&kubeNamespace, "kube-namespace", "restore-drill", "Kubernetes namespace for ephemeral restore pods")
	cmd.Flags().StringVar(&kubeServiceAccount, "kube-service-account", "", "Kubernetes service account for ephemeral restore pods")
	cmd.Flags().StringArrayVar(&kubePodLabels, "kube-pod-label", nil, "Kubernetes label for ephemeral restore pods (key=value, repeatable)")
	cmd.Flags().StringArrayVar(&kubePodAnnotations, "kube-pod-annotation", nil, "Kubernetes annotation for ephemeral restore pods (key=value, repeatable)")
	cmd.Flags().StringArrayVar(&kubeImagePullSecrets, "kube-image-pull-secret", nil, "Kubernetes image pull secret for ephemeral restore pods (repeatable)")

	return cmd
}

type runtimeOptions struct {
	mode             string
	namespace        string
	serviceAccount   string
	podLabels        map[string]string
	podAnnotations   map[string]string
	imagePullSecrets []string
}

func newRuntime(opts runtimeOptions) (engine.Runtime, error) {
	kubeOptions := []k8s.Option{
		k8s.WithNamespace(opts.namespace),
		k8s.WithServiceAccountName(opts.serviceAccount),
		k8s.WithPodLabels(opts.podLabels),
		k8s.WithPodAnnotations(opts.podAnnotations),
		k8s.WithImagePullSecrets(opts.imagePullSecrets),
	}

	switch opts.mode {
	case "auto":
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			rt, err := k8s.New(kubeOptions...)
			if err != nil {
				return nil, fmt.Errorf("init kubernetes runtime: %w", err)
			}
			return rt, nil
		}
		rt, err := docker.New()
		if err != nil {
			return nil, fmt.Errorf("init docker runtime: %w", err)
		}
		return rt, nil
	case "docker":
		rt, err := docker.New()
		if err != nil {
			return nil, fmt.Errorf("init docker runtime: %w", err)
		}
		return rt, nil
	case "kubernetes":
		rt, err := k8s.New(kubeOptions...)
		if err != nil {
			return nil, fmt.Errorf("init kubernetes runtime: %w", err)
		}
		return rt, nil
	default:
		return nil, fmt.Errorf("unknown runtime %q (use auto, docker, or kubernetes)", opts.mode)
	}
}

func parseKeyValueFlags(values []string, flagName string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	parsed := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("%s must use key=value syntax (got %q)", flagName, value)
		}
		parsed[key] = strings.TrimSpace(val)
	}
	return parsed, nil
}

func buildReporter(format string, cfg *engine.Config) engine.Reporter {
	reporters := make([]engine.Reporter, 0, 2)
	switch format {
	case "json":
		reporters = append(reporters, reporter.NewJSON(true))
	default:
		reporters = append(reporters, reporter.NewStdout())
	}

	seenWebhook := make(map[string]struct{})
	for _, drill := range cfg.Drills {
		for _, alert := range drill.Alerts {
			if alert.Type != "webhook" {
				continue
			}
			url := webhookAlertURL(alert)
			if url == "" {
				continue
			}
			key := webhookAlertKey(url, alert.Headers)
			if _, ok := seenWebhook[key]; ok {
				continue
			}
			seenWebhook[key] = struct{}{}
			reporters = append(reporters, reporter.NewWebhook(url, alert.Headers))
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

func validateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a drill configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := engine.LoadConfig(configPath)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "config OK: %d drill(s)\n", len(cfg.Drills)); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "drill.yaml", "Path to drill configuration file")
	return cmd
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
				return reporter.RenderHTML(w, reporter.BuildComplianceReport(runs, since))
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

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show last drill results",
		RunE: func(cmd *cobra.Command, args []string) error {
			run, err := state.Load(state.DefaultPath())
			if err != nil {
				return fmt.Errorf("no previous run found: %w", err)
			}

			if _, err := fmt.Printf("Last run: %s\n\n", run.Timestamp.Format(time.RFC3339)); err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "DRILL\tPROVIDER\tSTATUS\tDURATION\tCHECKS"); err != nil {
				return err
			}
			for _, r := range run.Results {
				status := "PASS"
				if !r.ValidationPassed || r.Error != "" {
					status = "FAIL"
				}
				passed := 0
				for _, c := range r.Checks {
					if c.Passed {
						passed++
					}
				}
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d/%d\n",
					r.Name, r.Provider, status, r.Duration, passed, len(r.Checks)); err != nil {
					return err
				}
			}
			return w.Flush()
		},
	}
}

func reportCmd() *cobra.Command {
	var format string
	var days int
	var output string

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate compliance reports from drill history",
		RunE: func(cmd *cobra.Command, args []string) error {
			since := time.Now().AddDate(0, 0, -days)
			runs, err := state.LoadHistory(since)
			if err != nil {
				return fmt.Errorf("load history: %w", err)
			}
			if len(runs) == 0 {
				return fmt.Errorf("no drill history found in the last %d days", days)
			}

			report := reporter.BuildComplianceReport(runs, since)

			var w io.Writer = os.Stdout
			var closeOutput func() error
			if output != "" {
				f, err := os.Create(output) // #nosec G304 -- output path is an explicit user CLI argument.
				if err != nil {
					return fmt.Errorf("create output file: %w", err)
				}
				w = f
				closeOutput = f.Close
			}

			var renderErr error
			switch format {
			case "json":
				renderErr = reporter.RenderJSON(w, report)
			default:
				renderErr = reporter.RenderHTML(w, report)
			}
			if closeOutput != nil {
				if err := closeOutput(); err != nil && renderErr == nil {
					renderErr = fmt.Errorf("close output file: %w", err)
				}
			}
			return renderErr
		},
	}

	cmd.Flags().StringVar(&format, "format", "html", "Report format: html, json")
	cmd.Flags().IntVar(&days, "last", 90, "Include drills from the last N days")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file (default: stdout)")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("restore-drill %s\n", version.String())
		},
	}
}
