package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/redact"
)

func newValidateCommand() *cobra.Command {
	var (
		path   string
		online bool
	)
	cmd := &cobra.Command{
		Use:   "validate-config",
		Short: "Validate a configuration file (offline, or on-chain with --online)",
		Long: "validate-config parses and validates the configuration, reporting every problem\n" +
			"at once. Without --online it makes no network calls. Environment variables\n" +
			"referenced by secret fields must be set.\n\n" +
			"--online also checks eth_chainId of every endpoint and decimals()/description()\n" +
			"of every feed at the pinned block, using read-only eth_call.",
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
			if !online {
				return nil
			}
			r := &redact.Redactor{}
			for _, s := range cfg.Secrets() {
				r.Add(s)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			if err := validateOnline(ctx, out, cfg, r); err != nil {
				return &exitError{code: ExitError, err: err}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&path, "config", "c", "", "path to the YAML configuration (required)")
	cmd.Flags().BoolVar(&online, "online", false, "also verify chain IDs, decimals and descriptions on-chain")
	_ = cmd.MarkFlagRequired("config") // only fails if the flag does not exist
	return cmd
}
