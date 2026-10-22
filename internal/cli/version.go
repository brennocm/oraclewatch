package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit and build date",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "oraclewatch %s (commit %s, built %s, %s %s/%s)\n",
				info.Version, info.Commit, info.Date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return err
		},
	}
}
