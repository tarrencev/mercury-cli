package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tarrence/mercury-cli/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version.Version())
		},
	}
}
