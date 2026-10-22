// Package cli wires the oraclewatch subcommands. Each command is a thin layer
// over the internal packages; no business logic lives here.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// BuildInfo carries the values injected by the linker at build time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Exit codes shared by all commands. check documents them for CI and cron use.
const (
	ExitOK    = 0
	ExitAlert = 1
	ExitError = 2
)

// exitError lets a command choose its exit code while still returning an error
// through cobra.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

// NewRootCommand builds the command tree. It is exported for the CLI tests.
func NewRootCommand(info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:           "oraclewatch",
		Short:         "Consumer-side monitor for Chainlink Data Feeds",
		Long:          "oraclewatch reads Chainlink Data Feeds directly from the chain and alerts on stale,\ninvalid or deviating prices, L2 sequencer outages, circuit-breaker proximity,\naggregator changes and RPC disagreement. It is read-only and holds no keys.\n\nNot affiliated with Chainlink Labs.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCommand(info))
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute(info BuildInfo) int {
	root := NewRootCommand(info)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		var ee *exitError
		if errors.As(err, &ee) {
			return ee.code
		}
		return ExitError
	}
	return ExitOK
}
