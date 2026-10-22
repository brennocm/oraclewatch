package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/feeds"
	"github.com/brennocm/oraclewatch/internal/redact"
)

// errOnlineMismatch signals that at least one on-chain value differs from
// the configuration.
var errOnlineMismatch = errors.New("on-chain values differ from the configuration")

// validateOnline checks, for every network, that each endpoint serves the
// configured chain and that each feed's decimals and description on-chain
// match the configuration. Reads use the network's pinned block.
func validateOnline(ctx context.Context, out io.Writer, cfg *config.Config, r *redact.Redactor) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NETWORK\tCHECK\tSUBJECT\tRESULT")
	failed := false
	report := func(network, check, subject string, err error, ok string) {
		res := ok
		if err != nil {
			res = "FAIL: " + r.Error(err).Error()
			failed = true
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", network, check, subject, res)
	}

	for i := range cfg.Networks {
		cn := &cfg.Networks[i]
		n, err := chain.NewNetworkFromConfig(ctx, cn, chain.BuildOptions{Redactor: r})
		if err != nil {
			report(cn.Name, "dial", "-", err, "")
			continue
		}
		ids := n.VerifyChainIDs(ctx)
		for _, e := range n.Endpoints {
			report(cn.Name, "chain_id", e.Name(), ids[e.Name()], fmt.Sprintf("OK (%d)", cn.ChainID))
		}
		snap, err := n.Snapshot(ctx)
		if err != nil {
			report(cn.Name, "snapshot", "-", err, "")
			continue
		}
		report(cn.Name, "pinned_block", fmt.Sprint(snap.Block), nil, fmt.Sprintf("OK (%d readers, %d forked)", len(snap.Readers), len(snap.Forked)))
		reader := snap.Readers[0]
		block := snap.BlockNumber()

		for _, f := range cfg.Feeds {
			if f.Network != cn.Name {
				continue
			}
			md, err := feeds.ReadMetadata(ctx, reader, f.Proxy.Address, block)
			if err == nil && md.Decimals != *f.ExpectedDecimals {
				err = fmt.Errorf("decimals %d, configured %d", md.Decimals, *f.ExpectedDecimals)
			}
			if err == nil && f.ExpectedDescription != "" && md.Description != f.ExpectedDescription {
				err = fmt.Errorf("description %q, configured %q", md.Description, f.ExpectedDescription)
			}
			report(cn.Name, "feed", f.Name, err, fmt.Sprintf("OK (decimals %d, %q, aggregator %s)", md.Decimals, md.Description, md.Aggregator.Hex()))
		}
		if cn.Sequencer != nil {
			rd, err := feeds.LatestRound(ctx, reader, cn.Sequencer.Proxy.Address, block)
			ok := ""
			if err == nil {
				ok = fmt.Sprintf("OK (answer %s, last change %s)", rd.Answer, feeds.TimeOf(rd.StartedAt).Format("2006-01-02T15:04:05Z"))
			}
			report(cn.Name, "sequencer", cn.Sequencer.Proxy.Hex(), err, ok)
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if failed {
		return errOnlineMismatch
	}
	return nil
}
