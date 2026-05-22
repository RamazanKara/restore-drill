package main

import (
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"github.com/fluentorbit/restore-drill/internal/logging"
	"github.com/fluentorbit/restore-drill/internal/state"
	"github.com/fluentorbit/restore-drill/internal/version"
	"github.com/fluentorbit/restore-drill/pkg/engine"
	"github.com/fluentorbit/restore-drill/pkg/metrics"
	"github.com/fluentorbit/restore-drill/pkg/providers/mysql"
	"github.com/fluentorbit/restore-drill/pkg/providers/postgres"
	"github.com/fluentorbit/restore-drill/pkg/providers/redis"
	"github.com/fluentorbit/restore-drill/pkg/reporter"
	"github.com/fluentorbit/restore-drill/pkg/runtime/docker"
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

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute backup restore drills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := engine.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			rt, err := docker.New()
			if err != nil {
				return fmt.Errorf("init docker runtime: %w", err)
			}

			var rep engine.Reporter
			switch format {
			case "json":
				rep = reporter.NewJSON(true)
			default:
				rep = reporter.NewStdout()
			}

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

			// Save state for 'status' command
			saveState(results)

			// Push metrics if configured
			if cfg.Metrics.Prometheus.Enabled && cfg.Metrics.Prometheus.Pushgateway != "" {
				if pushErr := metrics.PushResults(results, cfg.Metrics.Prometheus.Pushgateway, cfg.Metrics.Prometheus.Labels); pushErr != nil {
					slog.Error("failed to push metrics", "error", pushErr)
				}
			}

			// Exit 1 if any drill failed
			for _, r := range results {
				if r.Error != nil || !r.ValidationPassed {
					os.Exit(1)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "drill.yaml", "Path to drill configuration file")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")
	cmd.Flags().BoolVar(&parallel, "parallel", false, "Run drills concurrently")
	cmd.Flags().BoolVar(&noCleanup, "no-cleanup", false, "Keep containers running after drill (for debugging)")

	return cmd
}

func saveState(results []engine.DrillResult) {
	run := &state.LastRun{
		Timestamp: time.Now(),
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

	if err := state.Save(state.DefaultPath(), run); err != nil {
		slog.Warn("failed to save state", "error", err)
	}
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

			fmt.Printf("Last run: %s\n\n", run.Timestamp.Format(time.RFC3339))

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "DRILL\tPROVIDER\tSTATUS\tDURATION\tCHECKS")
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
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d/%d\n",
					r.Name, r.Provider, status, r.Duration, passed, len(r.Checks))
			}
			w.Flush()
			return nil
		},
	}
}

func reportCmd() *cobra.Command {
	var format string
	var days int

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate compliance reports from drill history",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("restore-drill report: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "html", "Report format: html, json, pdf")
	cmd.Flags().IntVar(&days, "last", 90, "Include drills from the last N days")

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
