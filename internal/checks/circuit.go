package checks

import (
	"math/big"

	"github.com/brennocm/oraclewatch/internal/config"
)

// VerifiedAggregatorTypes are the typeAndVersion() values whose deployed,
// verified source was read to confirm that transmit() rejects a median
// outside [minAnswer, maxAnswer] (docs/research.md §3.2). For other types
// FW-CB-001 reports Unknown.
var VerifiedAggregatorTypes = map[string]bool{
	"AccessControlledOCR2Aggregator 1.0.0": true,
	"DualAggregator 1.0.0":                 true,
}

// nominalFactor: an upper bound at least this many times the answer is
// treated as "no bound" (ETH/USD maxAnswer is ~9.6e52 against an answer of
// ~2.4e11). A lower bound <= 1 (the smallest positive answer) is nominal.
var nominalFactor = new(big.Int).Exp(big.NewInt(10), big.NewInt(30), nil)

// CircuitBreaker is FW-CB-001: the answer is close to the aggregator's
// minAnswer or maxAnswer.
//
// The aggregator does not clamp: a report outside the range reverts, so the
// feed freezes at the last accepted answer near the bound and grows stale
// while the market moves on. Hence:
//
//	dist = (answer - minAnswer) / answer   or   (maxAnswer - answer) / answer
//	warning  if dist < warn_within
//	critical if dist < critical_within, or dist < warn_within while
//	         FW-STALE-001 or FW-DEV-001 fire (frozen at the bound)
type CircuitBreaker struct{}

// ID implements FeedCheck.
func (CircuitBreaker) ID() string { return "FW-CB-001" }

// Enabled implements FeedCheck.
func (CircuitBreaker) Enabled(p config.CheckParams) bool { return p.CircuitBreaker.Enabled }

// Evaluate implements FeedCheck.
func (c CircuitBreaker) Evaluate(in *FeedInput) Result {
	switch {
	case in.Reading == nil:
		return in.result(c.ID(), Unknown, "no reading this cycle", nil)
	case in.Bounds == nil:
		return in.result(c.ID(), Unknown, "aggregator bounds not read", nil)
	case !VerifiedAggregatorTypes[in.Bounds.TypeAndVersion]:
		return in.result(c.ID(), Unknown, "bound behaviour of aggregator type "+quoteOr(in.Bounds.TypeAndVersion, "unknown")+" is not verified", nil)
	case in.Reading.Round.Answer.Sign() <= 0:
		return in.result(c.ID(), Unknown, "answer <= 0 (see FW-INVALID-001)", nil)
	}
	answer := in.Reading.Round.Answer
	lo, hi := in.Bounds.Min, in.Bounds.Max
	details := map[string]string{
		"answer":          answer.String(),
		"min_answer":      lo.String(),
		"max_answer":      hi.String(),
		"aggregator":      in.Bounds.TypeAndVersion,
		"warn_within":     in.Params.CircuitBreaker.WarnWithin.String(),
		"critical_within": in.Params.CircuitBreaker.CriticalWithin.String(),
	}
	if answer.Cmp(lo) < 0 || answer.Cmp(hi) > 0 {
		return in.result(c.ID(), Critical, "answer is outside [minAnswer, maxAnswer], which the aggregator should make impossible", details)
	}

	lowerNominal := lo.Cmp(big.NewInt(1)) <= 0
	upperNominal := hi.Cmp(new(big.Int).Mul(answer, nominalFactor)) >= 0
	if lowerNominal && upperNominal {
		return in.result(c.ID(), OK, "bounds are nominal", details)
	}

	ans := new(big.Rat).SetInt(answer)
	var dist *big.Rat
	side := ""
	if !lowerNominal {
		dist = new(big.Rat).Quo(new(big.Rat).SetInt(new(big.Int).Sub(answer, lo)), ans)
		side = "minAnswer"
	}
	if !upperNominal {
		d := new(big.Rat).Quo(new(big.Rat).SetInt(new(big.Int).Sub(hi, answer)), ans)
		if dist == nil || d.Cmp(dist) < 0 {
			dist, side = d, "maxAnswer"
		}
	}
	details["distance"] = pct(dist)
	details["nearest_bound"] = side

	p := in.Params.CircuitBreaker
	frozen := in.Prior["FW-STALE-001"] >= Warning || in.Prior["FW-DEV-001"] >= Warning
	switch {
	case dist.Cmp(p.CriticalWithin.Rat()) < 0:
		return in.result(c.ID(), Critical, "answer is "+pct(dist)+" from "+side, details)
	case dist.Cmp(p.WarnWithin.Rat()) < 0 && frozen:
		return in.result(c.ID(), Critical, "answer is "+pct(dist)+" from "+side+" and the feed is stale or deviating: likely frozen at the bound", details)
	case dist.Cmp(p.WarnWithin.Rat()) < 0:
		return in.result(c.ID(), Warning, "answer is "+pct(dist)+" from "+side, details)
	}
	return in.result(c.ID(), OK, pct(dist)+" from "+side, details)
}

func quoteOr(s, def string) string {
	if s == "" {
		return def
	}
	return `"` + s + `"`
}
