package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/spf13/cobra"
)

// newHealthcheckCommand probes /healthz. The runtime image is distroless (no
// shell, no curl), so the container healthcheck uses the binary itself.
func newHealthcheckCommand() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Probe a running instance's /healthz (for container healthchecks)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			u, err := url.Parse(target)
			if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
				return &exitError{code: ExitError, err: fmt.Errorf("--url must point to localhost")}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return &exitError{code: ExitError, err: fmt.Errorf("healthz returned %d", resp.StatusCode)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "url", "http://127.0.0.1:9090/healthz", "healthz URL (localhost only)")
	return cmd
}
