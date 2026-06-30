package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand(version, commit, date string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "rere %s (commit %s, built %s)\n", version, commit, date)
		},
	}
}
