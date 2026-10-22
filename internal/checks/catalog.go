package checks

// Scope tells which input a check evaluates.
type Scope string

// Scopes.
const (
	ScopeFeed       Scope = "feed"
	ScopeNetwork    Scope = "network"
	ScopeCrossChain Scope = "cross-chain"
)

// Meta describes a check for documentation and the README table. A test keeps
// the README and docs/checks in sync with this list.
type Meta struct {
	ID      string
	Title   string
	Scope   Scope
	Max     Status // highest status the check can emit
	Stable  bool   // false: experimental, off by default
	Detects string
}

// Catalog lists every check in evaluation order.
var Catalog = []Meta{
	{"FW-STALE-001", "Feed stale", ScopeFeed, Critical, true, "no update within heartbeat + tolerance (block time)"},
	{"FW-INVALID-001", "Invalid round data", ScopeFeed, Critical, true, "answer <= 0, zero timestamps, updatedAt after the block, roundId going backwards"},
	{"FW-DEV-001", "Deviation from market reference", ScopeFeed, Critical, true, "oracle vs median of USD exchange mids"},
	{"FW-REF-001", "Reference degraded", ScopeFeed, Warning, true, "too few exchanges answered or they disagree; suppresses FW-DEV-001"},
	{"FW-CB-001", "Near circuit-breaker bound", ScopeFeed, Critical, true, "answer close to minAnswer/maxAnswer (verified aggregator types only)"},
	{"FW-AGG-001", "Aggregator changed", ScopeFeed, Info, true, "proxy points to a new aggregator or phase"},
	{"FW-META-001", "Metadata drift", ScopeFeed, Critical, true, "decimals/description differ from config, version changed"},
	{"FW-JUMP-001", "Abrupt round-to-round change", ScopeFeed, Warning, false, "implausible jump between observed rounds"},
	{"FW-RPC-001", "RPC disagreement", ScopeFeed, Critical, true, "endpoints return different data at the same block hash"},
	{"FW-CHAIN-001", "Chain head stalled / RPC lagging", ScopeNetwork, Critical, true, "endpoint head older than max_head_lag"},
	{"FW-RPC-002", "RPC fork view", ScopeNetwork, Warning, true, "endpoint has a different hash for the pinned block"},
	{"FW-RPC-003", "Wrong chain", ScopeNetwork, Critical, true, "eth_chainId differs from config; endpoint disabled"},
	{"FW-SEQ-001", "L2 sequencer down", ScopeNetwork, Critical, true, "sequencer uptime feed answer == 1 or invalid"},
	{"FW-SEQ-002", "L2 sequencer grace period", ScopeNetwork, Warning, true, "sequencer recovered less than grace_period ago"},
	{"FW-SELF-001", "Monitor blind", ScopeNetwork, Critical, true, "no successful reads (or no reference) for N cycles"},
	{CrossChainID, "Cross-chain divergence", ScopeCrossChain, Warning, true, "same pair differs across networks beyond both thresholds + margin"},
}

// FeedChecks returns the feed-level checks in evaluation order. FW-CB-001
// comes after FW-STALE-001 and FW-DEV-001 because it combines with them.
func FeedChecks() []FeedCheck {
	return []FeedCheck{Stale{}, Invalid{}, Deviation{}, ReferenceDegraded{}, CircuitBreaker{}, AggregatorChanged{}, MetadataDrift{}, Jump{}, RPCDisagreement{}}
}

// NetworkChecks returns the network-level checks.
func NetworkChecks() []NetworkCheck {
	return []NetworkCheck{ChainHead{}, RPCFork{}, WrongChain{}, SequencerDown{}, SequencerGrace{}, SelfHealth{}}
}

// EvaluateFeed runs every enabled feed check on in, filling in.Prior as it
// goes, and returns the results in catalog order.
func EvaluateFeed(in *FeedInput) []Result {
	in.Prior = map[string]Status{}
	var out []Result
	for _, c := range FeedChecks() {
		if !c.Enabled(in.Params) {
			continue
		}
		r := c.Evaluate(in)
		in.Prior[c.ID()] = r.Status
		out = append(out, r)
	}
	return out
}
