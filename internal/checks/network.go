package checks

import (
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/feeds"
)

// NetworkInput is what the network-level checks need for one cycle.
type NetworkInput struct {
	Network    string
	Now        time.Time
	MaxHeadLag time.Duration

	// Snapshot is the cycle's pinned-block snapshot; SnapshotErr is
	// chain.ErrChainStalled, chain.ErrNoHealthyEndpoint or another error.
	Snapshot    *chain.Snapshot
	SnapshotErr error

	// WrongChain lists endpoints disabled because eth_chainId differs.
	WrongChain map[string]*chain.WrongChainError

	// Sequencer is the uptime feed round at the pinned block; nil when the
	// network has no sequencer feed (HasSequencer false) or the read failed.
	HasSequencer bool
	Sequencer    *feeds.Round
	SequencerErr error
	GracePeriod  time.Duration

	// FailedCycles counts consecutive cycles in which no feed could be read.
	FailedCycles int
	// ReferenceFailedCycles counts consecutive cycles in which no reference
	// source answered for any pair of this network.
	ReferenceFailedCycles int
	BlindAfter            int
}

func (in *NetworkInput) result(id string, s Status, summary string, details map[string]string) Result {
	return Result{CheckID: id, Network: in.Network, Status: s, Summary: summary, Details: details, Observed: in.Now}
}

// NetworkCheck evaluates one network.
type NetworkCheck interface {
	ID() string
	Evaluate(in *NetworkInput) Result
}

// ChainHead is FW-CHAIN-001: an endpoint's head is older than max_head_lag
// (warning), or every endpoint's is (critical: the chain or every provider
// has stopped, and feed freshness cannot be judged).
type ChainHead struct{}

// ID implements NetworkCheck.
func (ChainHead) ID() string { return "FW-CHAIN-001" }

// Evaluate implements NetworkCheck.
func (c ChainHead) Evaluate(in *NetworkInput) Result {
	if in.Snapshot == nil {
		return in.result(c.ID(), Unknown, "no snapshot", nil)
	}
	details := map[string]string{"max_head_lag": in.MaxHeadLag.String()}
	var lagging []string
	for _, h := range in.Snapshot.Heads {
		if h.Err != nil {
			continue
		}
		lag := in.Now.Sub(h.Time).Truncate(time.Second)
		details[h.Endpoint] = "head " + strconv.FormatUint(h.Number, 10) + ", " + lag.String() + " old"
		if h.Lagging {
			lagging = append(lagging, h.Endpoint)
		}
	}
	sort.Strings(lagging)
	switch {
	case errors.Is(in.SnapshotErr, chain.ErrChainStalled):
		return in.result(c.ID(), Critical, "every endpoint reports a head older than "+in.MaxHeadLag.String()+": chain halted or all providers stuck", details)
	case len(lagging) > 0:
		return in.result(c.ID(), Warning, "lagging endpoints excluded: "+strings.Join(lagging, ", "), details)
	}
	return in.result(c.ID(), OK, "heads are current", details)
}

// RPCFork is FW-RPC-002: an endpoint returned a different block hash for the
// pinned block number. It is excluded from the cycle's reads.
type RPCFork struct{}

// ID implements NetworkCheck.
func (RPCFork) ID() string { return "FW-RPC-002" }

// Evaluate implements NetworkCheck.
func (c RPCFork) Evaluate(in *NetworkInput) Result {
	if in.Snapshot == nil || in.SnapshotErr != nil {
		return in.result(c.ID(), Unknown, "no snapshot", nil)
	}
	if len(in.Snapshot.Forked) == 0 {
		return in.result(c.ID(), OK, "endpoints agree on block "+strconv.FormatUint(in.Snapshot.Block, 10), nil)
	}
	details := map[string]string{"block": strconv.FormatUint(in.Snapshot.Block, 10), "agreed_hash": in.Snapshot.BlockHash.Hex()}
	var names []string
	for _, f := range in.Snapshot.Forked {
		details[f.Endpoint] = f.Hash.Hex()
		names = append(names, f.Endpoint)
	}
	return in.result(c.ID(), Warning, "endpoints see a different block "+strconv.FormatUint(in.Snapshot.Block, 10)+": "+strings.Join(names, ", "), details)
}

// WrongChain is FW-RPC-003: an endpoint serves a different chain ID and was
// disabled. It keeps firing until the configuration is fixed and the process
// restarted.
type WrongChain struct{}

// ID implements NetworkCheck.
func (WrongChain) ID() string { return "FW-RPC-003" }

// Evaluate implements NetworkCheck.
func (c WrongChain) Evaluate(in *NetworkInput) Result {
	if len(in.WrongChain) == 0 {
		return in.result(c.ID(), OK, "every endpoint serves the configured chain", nil)
	}
	details := map[string]string{}
	var names []string
	for name, e := range in.WrongChain {
		details[name] = e.Error()
		names = append(names, name)
	}
	sort.Strings(names)
	return in.result(c.ID(), Critical, "endpoints serve another chain and were disabled: "+strings.Join(names, ", "), details)
}

// SequencerDown is FW-SEQ-001: the L2 sequencer uptime feed reports the
// sequencer down (answer == 1). An answer other than 0 or 1, or startedAt == 0
// (Arbitrum before initialisation), is an invalid sequencer round and is also
// critical: a consumer following the docs would not know whether to trust prices.
type SequencerDown struct{}

// ID implements NetworkCheck.
func (SequencerDown) ID() string { return "FW-SEQ-001" }

// Evaluate implements NetworkCheck.
func (c SequencerDown) Evaluate(in *NetworkInput) Result {
	if !in.HasSequencer {
		return in.result(c.ID(), OK, "no sequencer uptime feed on this network", nil)
	}
	r := in.Sequencer
	if r == nil {
		return in.result(c.ID(), Unknown, "sequencer feed not read", nil)
	}
	details := map[string]string{"answer": r.Answer.String(), "started_at": r.StartedAt.String(), "updated_at": r.UpdatedAt.String()}
	switch {
	case r.StartedAt.Sign() == 0:
		return in.result(c.ID(), Critical, "invalid sequencer round: startedAt is zero", details)
	case r.Answer.Cmp(big.NewInt(1)) == 0:
		return in.result(c.ID(), Critical, "L2 sequencer is DOWN since "+feeds.TimeOf(r.StartedAt).Format(time.RFC3339), details)
	case r.Answer.Sign() != 0:
		return in.result(c.ID(), Critical, "invalid sequencer answer "+r.Answer.String()+" (expected 0 or 1)", details)
	}
	return in.result(c.ID(), OK, "sequencer up", details)
}

// SequencerGrace is FW-SEQ-002: the sequencer is up but recovered less than
// grace_period ago, measured at the pinned block's timestamp as in the
// official example (block.timestamp - startedAt <= GRACE_PERIOD_TIME).
type SequencerGrace struct{}

// ID implements NetworkCheck.
func (SequencerGrace) ID() string { return "FW-SEQ-002" }

// Evaluate implements NetworkCheck.
func (c SequencerGrace) Evaluate(in *NetworkInput) Result {
	if !in.HasSequencer {
		return in.result(c.ID(), OK, "no sequencer uptime feed on this network", nil)
	}
	r := in.Sequencer
	if r == nil || in.Snapshot == nil {
		return in.result(c.ID(), Unknown, "sequencer feed not read", nil)
	}
	if r.Answer.Sign() != 0 || r.StartedAt.Sign() == 0 {
		return in.result(c.ID(), OK, "not up (see FW-SEQ-001)", nil)
	}
	since := new(big.Int).Sub(big.NewInt(in.Snapshot.BlockTime.Unix()), r.StartedAt)
	grace := big.NewInt(int64(in.GracePeriod / time.Second))
	details := map[string]string{"up_for": since.String() + "s", "grace_period": in.GracePeriod.String()}
	if since.Cmp(grace) <= 0 {
		return in.result(c.ID(), Warning, "sequencer recovered "+since.String()+"s ago, inside the "+in.GracePeriod.String()+" grace period", details)
	}
	return in.result(c.ID(), OK, "up for "+since.String()+"s", details)
}

// SelfHealth is FW-SELF-001: the monitor itself is blind. Critical when no
// feed of the network could be read for blind_after_cycles cycles; warning
// when no reference source answered for that long.
type SelfHealth struct{}

// ID implements NetworkCheck.
func (SelfHealth) ID() string { return "FW-SELF-001" }

// Evaluate implements NetworkCheck.
func (c SelfHealth) Evaluate(in *NetworkInput) Result {
	details := map[string]string{"failed_cycles": itoa(in.FailedCycles), "reference_failed_cycles": itoa(in.ReferenceFailedCycles), "blind_after": itoa(in.BlindAfter)}
	switch {
	case in.FailedCycles >= in.BlindAfter:
		reason := "no data"
		if in.SnapshotErr != nil {
			reason = in.SnapshotErr.Error()
		}
		return in.result(c.ID(), Critical, "monitor is blind: no feed read for "+itoa(in.FailedCycles)+" cycles ("+reason+")", details)
	case in.ReferenceFailedCycles >= in.BlindAfter:
		return in.result(c.ID(), Warning, "no market reference for "+itoa(in.ReferenceFailedCycles)+" cycles: deviation checks are blind", details)
	}
	return in.result(c.ID(), OK, "reading normally", details)
}
