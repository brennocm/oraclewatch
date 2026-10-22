package checks

import (
	"math/big"

	"github.com/brennocm/oraclewatch/internal/config"
)

// Stale is FW-STALE-001: the feed has not updated within its heartbeat plus a
// tolerance.
//
//	age     = blockTime - updatedAt
//	max_age = heartbeat + max(heartbeat × tolerance, min_slack)
//	warning  if age >  max_age
//	critical if age >  critical_factor × max_age
//
// Age is measured against the pinned block's timestamp, not the wall clock:
// if the chain itself stopped, FW-CHAIN-001 reports that once instead of every
// feed of the network turning stale. The absolute min_slack exists because
// updates land a few seconds after the heartbeat (up to +17 s observed on a
// 255 s heartbeat, docs/research.md §4.3), which a pure percentage of a short
// heartbeat does not cover.
type Stale struct{}

// ID implements FeedCheck.
func (Stale) ID() string { return "FW-STALE-001" }

// Enabled implements FeedCheck.
func (Stale) Enabled(p config.CheckParams) bool { return p.Staleness.Enabled }

// MaxAge returns max_age in seconds for a heartbeat and parameters.
func MaxAge(in *FeedInput) *big.Rat {
	p := in.Params.Staleness
	hb := durationRat(in.Heartbeat)
	slack := new(big.Rat).Mul(hb, p.Tolerance.Rat())
	slack = maxRat(slack, durationRat(p.MinSlack.Duration))
	return new(big.Rat).Add(hb, slack)
}

// Evaluate implements FeedCheck.
func (c Stale) Evaluate(in *FeedInput) Result {
	switch {
	case in.ChainStalled:
		return in.result(c.ID(), Unknown, "chain head is stalled; freshness cannot be judged (see FW-CHAIN-001)", nil)
	case in.Reading == nil:
		return in.result(c.ID(), Unknown, "no reading this cycle", nil)
	case in.Reading.Round.UpdatedAt.Sign() == 0:
		return in.result(c.ID(), Unknown, "updatedAt is zero (see FW-INVALID-001)", nil)
	}
	updated := in.Reading.Round.UpdatedAt
	block := big.NewInt(in.BlockTime.Unix())
	age := new(big.Rat).SetInt(new(big.Int).Sub(block, updated))
	if age.Sign() < 0 {
		age.SetInt64(0) // updatedAt after the block time is FW-INVALID-001's finding
	}
	maxAge := MaxAge(in)
	critAge := new(big.Rat).Mul(maxAge, big.NewRat(int64(in.Params.Staleness.CriticalFactor), 1))
	details := map[string]string{
		"age":        secs(age),
		"max_age":    secs(maxAge),
		"heartbeat":  in.Heartbeat.String(),
		"updated_at": updated.String(),
		"block":      big.NewInt(0).SetUint64(in.Block).String(),
	}
	switch {
	case age.Cmp(critAge) > 0:
		return in.result(c.ID(), Critical, "feed not updated for "+secs(age)+", more than "+secs(critAge), details)
	case age.Cmp(maxAge) > 0:
		return in.result(c.ID(), Warning, "feed not updated for "+secs(age)+", more than "+secs(maxAge), details)
	}
	return in.result(c.ID(), OK, "updated "+secs(age)+" ago", details)
}
