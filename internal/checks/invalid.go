package checks

import (
	"math/big"
	"strings"

	"github.com/brennocm/oraclewatch/internal/config"
)

// Invalid is FW-INVALID-001: the round returned by latestRoundData cannot be
// a correct round.
//
// Critical if any of:
//   - answer <= 0 (a USD price feed cannot be zero or negative);
//   - updatedAt == 0 or startedAt == 0 (incomplete round, per the docs);
//   - updatedAt > timestamp of the block the read was made at (updatedAt is
//     the timestamp of an earlier block, so this means a broken or lying RPC);
//   - roundId lower than the last roundId seen for this proxy while reading
//     at the same or a later block (state went backwards).
//
// Warning if startedAt > updatedAt + skew: observations are made off-chain
// before the report is included, so this ordering should not occur.
//
// answeredInRound is intentionally not checked: it is deprecated and equals
// roundId on current aggregators (docs/research.md §1.1).
type Invalid struct{}

// ID implements FeedCheck.
func (Invalid) ID() string { return "FW-INVALID-001" }

// Enabled implements FeedCheck.
func (Invalid) Enabled(p config.CheckParams) bool { return p.Invalid.Enabled }

// Evaluate implements FeedCheck.
func (c Invalid) Evaluate(in *FeedInput) Result {
	if in.Reading == nil {
		return in.result(c.ID(), Unknown, "no reading this cycle", nil)
	}
	r := in.Reading.Round
	block := big.NewInt(in.BlockTime.Unix())
	details := map[string]string{
		"round_id":   r.RoundID.String(),
		"answer":     r.Answer.String(),
		"started_at": r.StartedAt.String(),
		"updated_at": r.UpdatedAt.String(),
		"block_time": block.String(),
	}

	var critical []string
	if r.Answer.Sign() <= 0 {
		critical = append(critical, "answer <= 0")
	}
	if r.UpdatedAt.Sign() == 0 {
		critical = append(critical, "updatedAt is zero")
	}
	if r.StartedAt.Sign() == 0 {
		critical = append(critical, "startedAt is zero")
	}
	if r.UpdatedAt.Cmp(block) > 0 {
		critical = append(critical, "updatedAt is after the block it was read at")
	}
	if p := in.Prev; p != nil && in.Block >= p.Block && r.RoundID.Cmp(p.Round.RoundID) < 0 {
		critical = append(critical, "roundId went backwards from "+p.Round.RoundID.String())
	}
	if len(critical) > 0 {
		return in.result(c.ID(), Critical, "invalid round: "+strings.Join(critical, "; "), details)
	}

	skew := big.NewInt(int64(in.Params.Invalid.Skew.Seconds()))
	if r.StartedAt.Cmp(new(big.Int).Add(r.UpdatedAt, skew)) > 0 {
		return in.result(c.ID(), Warning, "startedAt is later than updatedAt", details)
	}
	return in.result(c.ID(), OK, "round is well-formed", details)
}
