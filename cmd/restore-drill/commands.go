package main

import (
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"github.com/RamazanKara/restore-drill/internal/state"
	"github.com/RamazanKara/restore-drill/internal/version"
	"github.com/RamazanKara/restore-drill/pkg/engine"
	"github.com/RamazanKara/restore-drill/pkg/metrics"
	"github.com/RamazanKara/restore-drill/pkg/providers/etcd"
	"github.com/RamazanKara/restore-drill/pkg/providers/mysql"
	"github.com/RamazanKara/restore-drill/pkg/providers/postgres"
	"github.com/RamazanKara/restore-drill/pkg/providers/redis"
	"github.com/RamazanKara/restore-drill/pkg/reporter"
	"github.com/spf13/cobra"
)

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

			rep := buildReporter(format, cfg, cmd.OutOrStdout())

			eng := engine.New(rt, rep)
			if noCleanup {
				eng.SetNoCleanup(true)
			}

			eng.RegisterProvider(postgres.New())
			eng.RegisterProvider(mysql.New())
			eng.RegisterProvider(redis.New())
			eng.RegisterProvider(etcd.New())

			var results []engine.DrillResult
			if parallel {
				results, err = eng.RunParallel(cmd.Context(), cfg.Drills)
			} else {
				results, err = eng.Run(cmd.Context(), cfg.Drills)
			}
			if err != nil {
				return err
			}

			currentRun := saveState(results)

			reportErr := writeConfiguredReports(cmd.Context(), cfg.Reporting, results, currentRun)
			if reportErr != nil {
				slog.Error("failed to write configured reports", "error", reportErr)
			}

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

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show last drill results",
		RunE: func(cmd *cobra.Command, args []string) error {
			run, err := state.Load(state.DefaultPath())
			if err != nil {
				return fmt.Errorf("no previous run found: %w", err)
			}

			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Last run: %s\n\n", run.Timestamp.Format(time.RFC3339)); err != nil {
				return err
			}

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
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
		Short: "Generate restore evidence reports from drill history",
		RunE: func(cmd *cobra.Command, args []string) error {
			since := time.Now().AddDate(0, 0, -days)
			runs, err := state.LoadHistory(since)
			if err != nil {
				return fmt.Errorf("load history: %w", err)
			}
			if len(runs) == 0 {
				return fmt.Errorf("no drill history found in the last %d days", days)
			}

			report := reporter.BuildEvidenceReport(runs, since)

			w := cmd.OutOrStdout()
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
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "restore-drill %s\n", version.String())
			return err
		},
	}
}
