package checks

import (
	"math/big"
	"sort"
	"strings"
	"time"
)

// CrossChainEntry is one network's latest answer for a pair.
type CrossChainEntry struct {
	Network            string
	Price              *big.Rat
	DeviationThreshold *big.Rat
	// Usable is false when the feed is stale, its chain is stalled or it was
	// not read; such entries are skipped instead of producing a false alarm.
	Usable bool
}

// CrossChainInput is the same pair across networks in one moment.
type CrossChainInput struct {
	Pair    string // e.g. "ETH/USD"
	Entries []CrossChainEntry
	Margin  *big.Rat
	Now     time.Time
}

// CrossChainID is the ID of the cross-chain divergence check.
const CrossChainID = "FW-XCHAIN-001"

// CrossChain is FW-XCHAIN-001: the same pair on two networks differs by more
// than both feeds' deviation thresholds allow, plus a margin:
//
//	dev = |a - b| / min(a, b) > threshold_a + threshold_b + margin
//
// Each network updates independently, so two healthy feeds can be apart by up
// to the sum of their thresholds; the margin covers the reads being taken at
// slightly different moments.
func CrossChain(in *CrossChainInput) Result {
	res := Result{CheckID: CrossChainID, Network: "*", Feed: in.Pair, Observed: in.Now, Details: map[string]string{}}
	var usable []CrossChainEntry
	for _, e := range in.Entries {
		if e.Usable && e.Price != nil && e.Price.Sign() > 0 {
			usable = append(usable, e)
			res.Details[e.Network] = e.Price.FloatString(8)
		}
	}
	sort.Slice(usable, func(i, j int) bool { return usable[i].Network < usable[j].Network })
	if len(usable) < 2 {
		res.Status, res.Summary = Unknown, "fewer than two usable networks"
		return res
	}
	var divergent []string
	for i := 0; i < len(usable); i++ {
		for j := i + 1; j < len(usable); j++ {
			a, b := usable[i], usable[j]
			low := a.Price
			if b.Price.Cmp(low) < 0 {
				low = b.Price
			}
			dev := new(big.Rat).Sub(a.Price, b.Price)
			dev.Abs(dev)
			dev.Quo(dev, low)
			limit := new(big.Rat).Add(a.DeviationThreshold, b.DeviationThreshold)
			limit.Add(limit, in.Margin)
			if dev.Cmp(limit) > 0 {
				divergent = append(divergent, a.Network+"/"+b.Network+" "+pct(dev)+" > "+pct(limit))
			}
		}
	}
	if len(divergent) > 0 {
		res.Status, res.Summary = Warning, in.Pair+" diverges across networks: "+strings.Join(divergent, "; ")
		return res
	}
	res.Status, res.Summary = OK, in.Pair+" consistent across "+itoa(len(usable))+" networks"
	return res
}
