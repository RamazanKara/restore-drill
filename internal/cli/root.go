// Package cli wires up and executes the restore-drill command-line interface.
package cli

import (
	"fmt"

	"github.com/RamazanKara/restore-drill/internal/logging"
	"github.com/spf13/cobra"
)

// Execute builds the root command, runs it, and returns a process exit code.
func Execute() int {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(root.ErrOrStderr(), err)
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
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
	root.AddCommand(doctorCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(reportCmd())
	root.AddCommand(versionCmd())

	return root
}
