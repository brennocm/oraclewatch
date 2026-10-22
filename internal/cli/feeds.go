package cli

import (
	"context"
	"fmt"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/feeds"
)

func newFeedsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "feeds", Short: "Inspect configured feeds"}
	var path string
	list := &cobra.Command{
		Use:   "list",
		Short: "List configured feeds with their latest on-chain value",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a, err := loadApp(cmd.Context(), path, cmd.ErrOrStderr(), false)
			if err != nil {
				return &exitError{code: ExitError, err: err}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute)
			defer cancel()

			type row struct {
				network, feed, proxy, value, updated, age, block string
			}
			rows := make([][]row, len(a.networks))
			var wg sync.WaitGroup
			for i, n := range a.networks {
				wg.Add(1)
				go func() {
					defer wg.Done()
					n.VerifyChainIDs(ctx)
					snap, serr := n.Snapshot(ctx)
					for _, f := range a.cfg.Feeds {
						if f.Network != n.Name {
							continue
						}
						r := row{network: n.Name, feed: f.Name, proxy: f.Proxy.Hex(), value: "-", updated: "-", age: "-", block: "-"}
						if serr != nil {
							r.value = "error: " + serr.Error()
							rows[i] = append(rows[i], r)
							continue
						}
						r.block = fmt.Sprint(snap.Block)
						rd, err := readFirst(ctx, snap, f.Proxy.Address, *f.ExpectedDecimals)
						if err != nil {
							r.value = "error: " + err.Error()
						} else {
							upd := feeds.TimeOf(rd.Round.UpdatedAt)
							r.value = rd.Price.FloatString(int(rd.Decimals))
							r.updated = upd.Format(time.RFC3339)
							r.age = snap.BlockTime.Sub(upd).Truncate(time.Second).String()
						}
						rows[i] = append(rows[i], r)
					}
				}()
			}
			wg.Wait()
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NETWORK\tFEED\tPROXY\tANSWER\tUPDATED\tAGE\tBLOCK")
			for _, rs := range rows {
				for _, r := range rs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.network, r.feed, r.proxy, a.redactor.String(r.value), r.updated, r.age, r.block)
				}
			}
			return tw.Flush()
		},
	}
	list.Flags().StringVarP(&path, "config", "c", "", "path to the YAML configuration (required)")
	_ = list.MarkFlagRequired("config") // only fails if the flag does not exist
	cmd.AddCommand(list)
	return cmd
}

// readFirst reads the latest round at the snapshot's block from the first
// reader that answers, normalised with on-chain decimals when available.
func readFirst(ctx context.Context, snap *chain.Snapshot, proxy [20]byte, fallbackDecimals uint8) (feeds.Reading, error) {
	last := chain.ErrNoHealthyEndpoint
	for _, ep := range snap.Readers {
		decimals := fallbackDecimals
		if md, err := feeds.ReadMetadata(ctx, ep, proxy, snap.BlockNumber()); err == nil {
			decimals = md.Decimals
		}
		r, err := feeds.ReadLatest(ctx, ep, ep.Name(), proxy, decimals, snap.Block, snap.BlockTime)
		if err == nil {
			return r, nil
		}
		last = err
	}
	return feeds.Reading{}, last
}
