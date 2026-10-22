package checks

import (
	"strconv"
	"strings"

	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/feeds"
)

// AggregatorChanged is FW-AGG-001: the proxy points to a different aggregator
// (or phase) than at the previous metadata read. Usually a legitimate
// upgrade, so it is Info; consumers still want to know, because bounds,
// decimals and behaviour can change with the aggregator.
type AggregatorChanged struct{}

// ID implements FeedCheck.
func (AggregatorChanged) ID() string { return "FW-AGG-001" }

// Enabled implements FeedCheck. It has no parameters and is always on.
func (AggregatorChanged) Enabled(config.CheckParams) bool { return true }

// Evaluate implements FeedCheck.
func (c AggregatorChanged) Evaluate(in *FeedInput) Result {
	if in.Metadata == nil {
		return in.result(c.ID(), Unknown, "metadata not read", nil)
	}
	details := map[string]string{
		"aggregator": in.Metadata.Aggregator.Hex(),
		"phase_id":   strconv.Itoa(int(in.Metadata.PhaseID)),
	}
	if in.PrevMetadata == nil {
		return in.result(c.ID(), OK, "first observation", details)
	}
	var changed []string
	for _, ch := range feeds.Diff(*in.PrevMetadata, *in.Metadata) {
		if ch.Field == "aggregator" || ch.Field == "phase_id" {
			changed = append(changed, ch.Field+" "+ch.Old+" -> "+ch.New)
			details["previous_"+ch.Field] = ch.Old
		}
	}
	if len(changed) > 0 {
		return in.result(c.ID(), Info, "aggregator changed: "+strings.Join(changed, ", "), details)
	}
	return in.result(c.ID(), OK, "aggregator unchanged", details)
}

// MetadataDrift is FW-META-001: decimals() differs from the configured value
// (critical: consumers that hard-code decimals misprice by powers of ten),
// description() differs from the configured value (warning), or version()
// changed since the previous read (info).
type MetadataDrift struct{}

// ID implements FeedCheck.
func (MetadataDrift) ID() string { return "FW-META-001" }

// Enabled implements FeedCheck. It has no parameters and is always on.
func (MetadataDrift) Enabled(config.CheckParams) bool { return true }

// Evaluate implements FeedCheck.
func (c MetadataDrift) Evaluate(in *FeedInput) Result {
	m := in.Metadata
	if m == nil {
		return in.result(c.ID(), Unknown, "metadata not read", nil)
	}
	details := map[string]string{
		"decimals":          strconv.Itoa(int(m.Decimals)),
		"expected_decimals": strconv.Itoa(int(in.ExpectedDecimals)),
		"description":       m.Description,
		"version":           m.Version.String(),
	}
	if m.Decimals != in.ExpectedDecimals {
		return in.result(c.ID(), Critical, "decimals() is "+details["decimals"]+", configured "+details["expected_decimals"], details)
	}
	if in.ExpectedDescription != "" && m.Description != in.ExpectedDescription {
		details["expected_description"] = in.ExpectedDescription
		return in.result(c.ID(), Warning, "description() is "+strconv.Quote(m.Description)+", configured "+strconv.Quote(in.ExpectedDescription), details)
	}
	if p := in.PrevMetadata; p != nil && p.Version != nil && m.Version.Cmp(p.Version) != 0 {
		details["previous_version"] = p.Version.String()
		return in.result(c.ID(), Info, "version() changed from "+p.Version.String()+" to "+m.Version.String(), details)
	}
	return in.result(c.ID(), OK, "metadata as configured", details)
}

func itoa(n int) string { return strconv.Itoa(n) }
