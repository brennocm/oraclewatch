package checks

import (
	"math/big"

	"github.com/brennocm/oraclewatch/internal/config"
)

// Deviation is FW-DEV-001: the oracle answer differs from the market
// reference by more than the configured thresholds.
//
//	dev = |oracle - reference| / reference
//	warning  if dev > warn_above
//	critical if dev > critical_above
//
// It only runs on a trusted reference (FW-REF-001 covers the other case).
// Config validation guarantees warn_above >= 2 × deviation_threshold, because
// a healthy feed may lag the market by up to its threshold until the next
// round. Short spikes are absorbed by the alert manager's "for:" duration.
type Deviation struct{}

// ID implements FeedCheck.
func (Deviation) ID() string { return "FW-DEV-001" }

// Enabled implements FeedCheck.
func (Deviation) Enabled(p config.CheckParams) bool { return p.Deviation.Enabled }

// DeviationRatio returns |a - ref| / ref. ref must be positive.
func DeviationRatio(a, ref *big.Rat) *big.Rat {
	d := new(big.Rat).Sub(a, ref)
	d.Abs(d)
	return d.Quo(d, ref)
}

// Evaluate implements FeedCheck.
func (c Deviation) Evaluate(in *FeedInput) Result {
	switch {
	case in.ChainStalled:
		return in.result(c.ID(), Unknown, "chain head is stalled; comparing an old answer with the market now is meaningless", nil)
	case in.Reading == nil:
		return in.result(c.ID(), Unknown, "no reading this cycle", nil)
	case in.Reference == nil:
		return in.result(c.ID(), Unknown, "no reference pair configured", nil)
	case !in.Reference.Trusted || in.Reference.Median == nil || in.Reference.Median.Sign() <= 0:
		return in.result(c.ID(), Unknown, "reference not trusted (see FW-REF-001)", nil)
	}
	p := in.Params.Deviation
	dev := DeviationRatio(in.Reading.Price, in.Reference.Median)
	details := map[string]string{
		"oracle":         in.Reading.Price.FloatString(8),
		"reference":      in.Reference.Median.FloatString(8),
		"deviation":      pct(dev),
		"warn_above":     p.WarnAbove.String(),
		"critical_above": p.CriticalAbove.String(),
		"sources":        sourceNames(in),
	}
	switch {
	case dev.Cmp(p.CriticalAbove.Rat()) > 0:
		return in.result(c.ID(), Critical, "oracle deviates "+pct(dev)+" from the market", details)
	case dev.Cmp(p.WarnAbove.Rat()) > 0:
		return in.result(c.ID(), Warning, "oracle deviates "+pct(dev)+" from the market", details)
	}
	return in.result(c.ID(), OK, "within "+pct(dev)+" of the market", details)
}

func sourceNames(in *FeedInput) string {
	s := ""
	for i, q := range in.Reference.Quotes {
		if i > 0 {
			s += ","
		}
		s += q.Source
	}
	return s
}

// pct formats a ratio as a percentage with 4 decimals, for messages only.
func pct(r *big.Rat) string {
	return new(big.Rat).Mul(r, big.NewRat(100, 1)).FloatString(4) + "%"
}
