// Package checks implements the oraclewatch check catalog
// (docs/research.md §8). Every check is a pure function of its input: no I/O,
// no clock, no shared state. The engine gathers the data and the alert
// manager decides what to send. That split is what makes each threshold
// testable to the exact unit.
//
// All comparisons use math/big. float64 never appears in this package.
package checks

import (
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/feeds"
	"github.com/brennocm/oraclewatch/internal/reference"
)

// Status is the outcome of a check. The order matters: a higher value is
// worse, except that Unknown sits between Info and Warning so that "worst of"
// summaries surface missing data above informational events.
type Status int

// Statuses, from best to worst.
const (
	OK Status = iota
	Info
	Unknown
	Warning
	Critical
)

func (s Status) String() string {
	switch s {
	case OK:
		return "ok"
	case Info:
		return "info"
	case Unknown:
		return "unknown"
	case Warning:
		return "warning"
	case Critical:
		return "critical"
	}
	return fmt.Sprintf("status(%d)", int(s))
}

// Result is the outcome of one check for one subject. Summary and Details
// must never contain secrets; they only ever hold on-chain values, configured
// names and computed numbers.
type Result struct {
	CheckID  string
	Network  string // "*" for cross-network results
	Feed     string // "" for network-level results; the pair for cross-chain
	Status   Status
	Summary  string
	Details  map[string]string
	Observed time.Time
}

// Key identifies the subject of a result for alert state tracking.
func (r Result) Key() string { return r.CheckID + "|" + r.Network + "|" + r.Feed }

// EndpointRead is one endpoint's read of a feed at the pinned block.
type EndpointRead struct {
	Endpoint string
	Round    *feeds.Round
	Err      error
}

// FeedInput is everything the feed-level checks need about one feed in one cycle.
type FeedInput struct {
	Network string
	Feed    string
	Params  config.CheckParams

	Heartbeat           time.Duration
	DeviationThreshold  *big.Rat
	ExpectedDecimals    uint8
	ExpectedDescription string

	Block     uint64
	BlockTime time.Time // timestamp of the pinned block
	Now       time.Time

	// ChainStalled is true when the network's head is older than max_head_lag
	// on every endpoint; freshness cannot be judged then (FW-CHAIN-001).
	ChainStalled bool

	Reading *feeds.Reading // nil if the read failed
	ReadErr error

	// Prev is the reading of the previous successful cycle, if any.
	Prev *feeds.Reading

	Metadata     *feeds.Metadata // nil if not read this cycle and never read
	PrevMetadata *feeds.Metadata // metadata from the previous read, if any
	Bounds       *feeds.Bounds

	Reference *reference.Reference // nil if the feed has no reference pair

	// Reads holds every reader endpoint's latestRoundData at Block, for FW-RPC-001.
	Reads []EndpointRead

	// Prior holds the status of checks already evaluated for this feed in this
	// cycle (FW-CB-001 combines with FW-STALE-001 and FW-DEV-001).
	Prior map[string]Status
}

func (in *FeedInput) result(id string, s Status, summary string, details map[string]string) Result {
	return Result{CheckID: id, Network: in.Network, Feed: in.Feed, Status: s, Summary: summary, Details: details, Observed: in.Now}
}

// FeedCheck evaluates one feed.
type FeedCheck interface {
	ID() string
	Enabled(p config.CheckParams) bool
	Evaluate(in *FeedInput) Result
}

// --- exact arithmetic helpers ---

// durationRat returns d in seconds as an exact rational.
func durationRat(d time.Duration) *big.Rat {
	return new(big.Rat).SetFrac(big.NewInt(int64(d)), big.NewInt(int64(time.Second)))
}

// secs formats a rational number of seconds as a duration ("40m24s"), for
// messages only. Values beyond time.Duration's range fall back to seconds.
func secs(r *big.Rat) string {
	n, _ := new(big.Int).SetString(r.FloatString(0), 10)
	if n == nil || !n.IsInt64() || n.Int64() > math.MaxInt64/int64(time.Second) {
		return r.FloatString(0) + "s"
	}
	return (time.Duration(n.Int64()) * time.Second).String()
}

func maxRat(a, b *big.Rat) *big.Rat {
	if a.Cmp(b) >= 0 {
		return a
	}
	return b
}
