package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/brennocm/oraclewatch/internal/checks"
)

func newCheckCommand() *cobra.Command {
	var path, format, failOn string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run one cycle on every network, print the results and exit",
		Long: "check runs a single monitoring cycle and exits with:\n" +
			"  0  no result at or above --fail-on\n" +
			"  1  at least one result at or above --fail-on\n" +
			"  2  error (invalid config, ...)\n\n" +
			"No alerts are sent and the alerting \"for:\" delay does not apply. Intended for CI\n" +
			"and cron.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			threshold, ok := map[string]checks.Status{"warning": checks.Warning, "critical": checks.Critical}[failOn]
			if !ok {
				return &exitError{code: ExitError, err: fmt.Errorf("--fail-on must be warning or critical")}
			}
			if format != "text" && format != "json" {
				return &exitError{code: ExitError, err: fmt.Errorf("--format must be text or json")}
			}
			a, err := loadApp(cmd.Context(), path, cmd.ErrOrStderr(), false)
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			eng, err := a.engine(nil, nil, nil)
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			results := eng.RunOnce(cmd.Context())
			out := cmd.OutOrStdout()
			if format == "json" {
				err = writeResultsJSON(out, results)
			} else {
				err = writeResultsText(out, results)
			}
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			worst := checks.OK
			for _, r := range results {
				if r.Status > worst {
					worst = r.Status
				}
			}
			if worst >= threshold {
				return &exitError{code: ExitAlert, err: fmt.Errorf("worst result is %s (fail-on %s)", worst, failOn)}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&path, "config", "c", "", "path to the YAML configuration (required)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&failOn, "fail-on", "critical", "exit 1 when a result is at or above: warning or critical")
	_ = cmd.MarkFlagRequired("config") // only fails if the flag does not exist
	return cmd
}

func writeResultsText(w io.Writer, rs []checks.Result) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tCHECK\tNETWORK\tFEED\tSUMMARY")
	for _, r := range rs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Status, r.CheckID, r.Network, r.Feed, r.Summary)
	}
	return tw.Flush()
}

type jsonResult struct {
	Check    string            `json:"check"`
	Network  string            `json:"network"`
	Feed     string            `json:"feed,omitempty"`
	Status   string            `json:"status"`
	Summary  string            `json:"summary"`
	Details  map[string]string `json:"details,omitempty"`
	Observed time.Time         `json:"observed"`
}

func writeResultsJSON(w io.Writer, rs []checks.Result) error {
	out := make([]jsonResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, jsonResult{r.CheckID, r.Network, r.Feed, r.Status.String(), r.Summary, r.Details, r.Observed})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
