package checks

import (
	"sort"
	"strings"

	"github.com/brennocm/oraclewatch/internal/config"
)

// ReferenceDegraded is FW-REF-001: the market reference for the feed's pair
// is not trusted (too few sources answered, or they disagree). While it fires,
// FW-DEV-001 reports Unknown instead of alerting on a reference that may be
// wrong.
type ReferenceDegraded struct{}

// ID implements FeedCheck.
func (ReferenceDegraded) ID() string { return "FW-REF-001" }

// Enabled implements FeedCheck. It follows FW-DEV-001: without the deviation
// check, the reference has no consumer.
func (ReferenceDegraded) Enabled(p config.CheckParams) bool { return p.Deviation.Enabled }

// Evaluate implements FeedCheck.
func (c ReferenceDegraded) Evaluate(in *FeedInput) Result {
	ref := in.Reference
	if ref == nil {
		return in.result(c.ID(), OK, "no reference pair configured", nil)
	}
	details := map[string]string{"pair": ref.Pair, "answered": itoa(len(ref.Quotes))}
	if ref.Median != nil {
		details["median"] = ref.Median.FloatString(8)
		details["spread"] = pct(ref.Spread)
	}
	if len(ref.Errors) > 0 {
		names := make([]string, 0, len(ref.Errors))
		for n, e := range ref.Errors {
			names = append(names, n+": "+e)
		}
		sort.Strings(names)
		details["errors"] = strings.Join(names, "; ")
	}
	if !ref.Trusted {
		return in.result(c.ID(), Warning, "reference "+ref.Pair+" untrusted: "+ref.Reason, details)
	}
	return in.result(c.ID(), OK, "reference trusted", details)
}
