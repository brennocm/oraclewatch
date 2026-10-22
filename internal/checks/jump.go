package checks

import (
	"math/big"

	"github.com/brennocm/oraclewatch/internal/config"
)

// Jump is the experimental FW-JUMP-001: the answer changed by more than
// max_jump between the last observed round and the current one.
//
//	jump = |answer_now - answer_prev| / answer_prev    (answer_prev > 0)
//
// It is meant for feeds with no market reference, such as exchange-rate feeds
// (the Moonwell wrsETH/ETH incident, docs/incidents.md §2). It compares with
// the round seen in the previous cycle, so several rounds published between
// two cycles are compared as one step.
type Jump struct{}

// ID implements FeedCheck.
func (Jump) ID() string { return "FW-JUMP-001" }

// Enabled implements FeedCheck. Off by default (experimental).
func (Jump) Enabled(p config.CheckParams) bool { return p.Jump.Enabled }

// Evaluate implements FeedCheck.
func (c Jump) Evaluate(in *FeedInput) Result {
	switch {
	case in.Reading == nil:
		return in.result(c.ID(), Unknown, "no reading this cycle", nil)
	case in.Prev == nil:
		return in.result(c.ID(), OK, "no previous round to compare", nil)
	case in.Prev.Round.RoundID.Cmp(in.Reading.Round.RoundID) == 0:
		return in.result(c.ID(), OK, "no new round", nil)
	case in.Prev.Round.Answer.Sign() <= 0:
		return in.result(c.ID(), Unknown, "previous answer <= 0", nil)
	}
	prev := new(big.Rat).SetInt(in.Prev.Round.Answer)
	cur := new(big.Rat).SetInt(in.Reading.Round.Answer)
	jump := DeviationRatio(cur, prev)
	details := map[string]string{
		"previous_answer": in.Prev.Round.Answer.String(),
		"answer":          in.Reading.Round.Answer.String(),
		"previous_round":  in.Prev.Round.RoundID.String(),
		"round":           in.Reading.Round.RoundID.String(),
		"change":          pct(jump),
		"max_jump":        in.Params.Jump.MaxJump.String(),
	}
	if jump.Cmp(in.Params.Jump.MaxJump.Rat()) > 0 {
		return in.result(c.ID(), Warning, "answer changed "+pct(jump)+" since the last observed round", details)
	}
	return in.result(c.ID(), OK, "changed "+pct(jump), details)
}
