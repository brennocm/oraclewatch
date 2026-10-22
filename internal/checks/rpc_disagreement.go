package checks

import (
	"sort"
	"strconv"
	"strings"

	"github.com/brennocm/oraclewatch/internal/config"
)

// RPCDisagreement is FW-RPC-001: endpoints that agree on the pinned block's
// hash returned different latestRoundData for the same proxy at that block.
//
// The EVM is deterministic, so two honest nodes with the same block hash must
// return byte-identical results. Any difference means at least one endpoint
// is broken, misconfigured or lying, and no value from this cycle should be
// trusted blindly. Endpoints that failed to answer are not a disagreement;
// they are tracked by their circuit breaker.
type RPCDisagreement struct{}

// ID implements FeedCheck.
func (RPCDisagreement) ID() string { return "FW-RPC-001" }

// Enabled implements FeedCheck; controlled by rpc.compare_endpoints, which
// the engine applies by not filling Reads.
func (RPCDisagreement) Enabled(config.CheckParams) bool { return true }

// Evaluate implements FeedCheck.
func (c RPCDisagreement) Evaluate(in *FeedInput) Result {
	var ok []EndpointRead
	for _, r := range in.Reads {
		if r.Err == nil && r.Round != nil {
			ok = append(ok, r)
		}
	}
	if len(ok) < 2 {
		return in.result(c.ID(), Unknown, "fewer than two endpoints answered; nothing to compare", map[string]string{"answered": itoa(len(ok))})
	}
	ref := ok[0]
	details := map[string]string{"block": strconv.FormatUint(in.Block, 10)}
	var differ []string
	for _, r := range ok {
		details[r.Endpoint] = "answer=" + r.Round.Answer.String() + " updatedAt=" + r.Round.UpdatedAt.String() + " roundId=" + r.Round.RoundID.String()
		if !r.Round.Equal(*ref.Round) {
			differ = append(differ, r.Endpoint)
		}
	}
	if len(differ) > 0 {
		sort.Strings(differ)
		return in.result(c.ID(), Critical, "endpoints disagree at the same block: "+strings.Join(differ, ", ")+" differ from "+ref.Endpoint, details)
	}
	return in.result(c.ID(), OK, itoa(len(ok))+" endpoints agree", details)
}
