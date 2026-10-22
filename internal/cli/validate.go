package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/brennocm/oraclewatch/internal/config"
)

func newValidateCommand() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "validate-config",
		Short: "Validate a configuration file without network access",
		Long: "validate-config parses and validates the configuration, reporting every problem\n" +
			"at once. It makes no network calls. Environment variables referenced by\n" +
			"secret fields must be set.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(path, os.LookupEnv)
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			out := cmd.OutOrStdout()
			for _, w := range cfg.Warnings {
				fmt.Fprintln(out, "warning:", w)
			}
			fmt.Fprintf(out, "OK: %d networks, %d feeds\n", len(cfg.Networks), len(cfg.Feeds))
			return nil
		},
	}
	cmd.Flags().StringVarP(&path, "config", "c", "", "path to the YAML configuration (required)")
	_ = cmd.MarkFlagRequired("config") // only fails if the flag does not exist
	return cmd
}
