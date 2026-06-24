package main

import (
	"fmt"
	"os"

	"github.com/RamazanKara/restore-drill/internal/logging"
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
		fmt.Fprintln(root.ErrOrStderr(), err)
		os.Exit(1)
	}
}
