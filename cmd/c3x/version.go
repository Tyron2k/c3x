package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// newVersionCmd prints the build metadata. We keep this as a real
// subcommand (in addition to cobra's `--version` flag) so machine
// scripts have a predictable surface to grep against.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print c3x version and build info.",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(),
				"c3x %s (%s/%s, %s)\n",
				version, runtime.GOOS, runtime.GOARCH, runtime.Version())
		},
	}
}
