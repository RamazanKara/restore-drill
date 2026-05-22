package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/fluentorbit/restore-drill/internal/logging"
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

	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show last drill results",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("restore-drill status: not yet implemented")
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
