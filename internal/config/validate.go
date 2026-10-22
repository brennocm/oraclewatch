package config

import (
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MinPollInterval is the smallest accepted poll interval. Shorter intervals
// gain little (heartbeats are minutes to hours) and burn free RPC quotas.
const MinPollInterval = 15 * time.Second

// KnownSources lists the implemented reference price sources.
var KnownSources = map[string]bool{"coinbase": true, "kraken": true, "bitstamp": true, "gemini": true}

var (
	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	// Pairs are BASE-QUOTE in upper case. Only USD quotes are accepted: USDT or
	// USDC quotes are not USD and diverge exactly when it matters
	// (docs/research.md §6).
	pairPattern = regexp.MustCompile(`^[A-Z0-9]{2,10}-USD$`)
)

// fieldErrors collects every validation problem instead of stopping at the first.
type fieldErrors []string

func (e *fieldErrors) add(path, format string, args ...any) {
	*e = append(*e, path+": "+fmt.Sprintf(format, args...))
}

func (e fieldErrors) err() error {
	if len(e) == 0 {
		return nil
	}
	return &ValidationError{Problems: e}
}

// ValidationError lists every problem found in a configuration.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid config (%d problems):\n  %s", len(e.Problems), strings.Join(e.Problems, "\n  "))
}

func validate(cfg *Config, errs *fieldErrors) {
	cfg.networks = make(map[string]*Network, len(cfg.Networks))
	if len(cfg.Networks) == 0 {
		errs.add("networks", "at least one network is required")
	}
	for i := range cfg.Networks {
		validateNetwork(cfg, i, errs)
	}
	validateReference(&cfg.Reference, errs)
	if cfg.Reference.MinSources == 1 {
		cfg.Warnings = append(cfg.Warnings, "reference.min_sources: 1 lets a single exchange decide deviation alerts")
	}
	validateCheckParams("defaults.checks", nil, cfg.Defaults.Checks, errs)

	if len(cfg.Feeds) == 0 {
		errs.add("feeds", "at least one feed is required")
	}
	seen := map[string]int{}
	for i := range cfg.Feeds {
		validateFeed(cfg, i, seen, errs)
	}

	if cfg.CrossChain.Enabled && !nonNegative(cfg.CrossChain.Margin) {
		errs.add("cross_chain.margin", "must be >= 0%%")
	}
	if cfg.RPC.ChainIDRecheck < 1 {
		errs.add("rpc.chain_id_recheck", "must be >= 1")
	}
	if cfg.RPC.MetadataRefresh < 1 {
		errs.add("rpc.metadata_refresh", "must be >= 1")
	}
	validateAlerting(cfg, errs)
	if cfg.SelfHealth.BlindAfterCycles < 1 {
		errs.add("self_health.blind_after_cycles", "must be >= 1")
	}
	if !cfg.SelfHealth.DeadmanURL.IsZero() {
		checkHTTPURL(cfg, "self_health.deadman_url", cfg.SelfHealth.DeadmanURL, errs)
	}
	if _, _, err := net.SplitHostPort(cfg.Server.Listen); err != nil {
		errs.add("server.listen", "must be host:port: %v", err)
	}
	switch cfg.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		errs.add("log.level", "must be one of debug, info, warn, error")
	}
	switch cfg.Log.Format {
	case "json", "text":
	default:
		errs.add("log.format", "must be json or text")
	}
}

func validateNetwork(cfg *Config, i int, errs *fieldErrors) {
	n := &cfg.Networks[i]
	p := fmt.Sprintf("networks[%d]", i)
	if !namePattern.MatchString(n.Name) {
		errs.add(p+".name", "must match %s", namePattern)
	} else if _, dup := cfg.networks[n.Name]; dup {
		errs.add(p+".name", "duplicate network %q", n.Name)
	} else {
		cfg.networks[n.Name] = n
	}
	if n.ChainID == 0 {
		errs.add(p+".chain_id", "must be > 0")
	}
	switch len(n.RPCEndpoints) {
	case 0:
		errs.add(p+".rpc_endpoints", "at least one endpoint is required")
	case 1:
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"%s.rpc_endpoints: only one endpoint for %q; RPC disagreement (FW-RPC-001) cannot run and a single RPC is trusted for every alert", p, n.Name))
	}
	names := map[string]bool{}
	hosts := map[string]string{} // host -> first endpoint name
	for j, e := range n.RPCEndpoints {
		ep := fmt.Sprintf("%s.rpc_endpoints[%d]", p, j)
		if !namePattern.MatchString(e.Name) {
			errs.add(ep+".name", "must match %s", namePattern)
		} else if names[e.Name] {
			errs.add(ep+".name", "duplicate endpoint name %q", e.Name)
		}
		names[e.Name] = true
		checkHTTPURL(cfg, ep+".url", e.URL, errs)
		// Two endpoints on one host are one provider: they share failures and,
		// if it lies, the lie. FW-RPC-001 would compare a provider with itself.
		if u, err := url.Parse(e.URL.Value()); err == nil && u.Host != "" {
			host := strings.ToLower(u.Hostname())
			if first, dup := hosts[host]; dup {
				cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
					"%s.url: same host as endpoint %q; use independent providers so that FW-RPC-001 compares two views", ep, first))
			} else {
				hosts[host] = e.Name
			}
		}
	}
	if n.PollInterval.Duration < MinPollInterval {
		errs.add(p+".poll_interval", "must be >= %s", MinPollInterval)
	}
	if n.CallTimeout.Duration <= 0 || n.CallTimeout.Duration >= n.PollInterval.Duration {
		errs.add(p+".call_timeout", "must be > 0 and shorter than poll_interval")
	}
	if n.MaxHeadLag.Duration <= 0 {
		errs.add(p+".max_head_lag", "must be > 0")
	}
	if n.ExplorerURL != "" {
		if u, err := url.Parse(n.ExplorerURL); err != nil || u.Scheme != "https" || u.Host == "" {
			errs.add(p+".explorer_url", "must be an https URL")
		}
	}
	if n.Sequencer != nil {
		if !n.L2 {
			errs.add(p+".sequencer", "only allowed on networks with l2: true")
		}
		if !n.Sequencer.Proxy.IsSet() {
			errs.add(p+".sequencer.proxy", "is required")
		}
		if n.Sequencer.GracePeriod.Duration <= 0 {
			errs.add(p+".sequencer.grace_period", "must be > 0")
		}
	} else if n.L2 {
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"%s: L2 network %q has no sequencer uptime feed; FW-SEQ-001/002 will not run", p, n.Name))
	}
}

func validateReference(r *Reference, errs *fieldErrors) {
	validateSources("reference.sources", r.Sources, errs)
	if r.MinSources < 1 {
		errs.add("reference.min_sources", "must be >= 1")
	}
	if !positive(r.MaxSourceSpread) {
		errs.add("reference.max_source_spread", "must be > 0%%")
	}
	if r.Timeout.Duration <= 0 {
		errs.add("reference.timeout", "must be > 0")
	}
	if r.MaxQuoteAge.Duration <= 0 {
		errs.add("reference.max_quote_age", "must be > 0")
	}
	if r.CacheTTL.Duration < 0 {
		errs.add("reference.cache_ttl", "must be >= 0")
	}
}

func validateSources(path string, sources []string, errs *fieldErrors) {
	if len(sources) == 0 {
		errs.add(path, "at least one source is required")
	}
	seen := map[string]bool{}
	for j, s := range sources {
		if !KnownSources[s] {
			errs.add(fmt.Sprintf("%s[%d]", path, j), "unknown source %q (known: coinbase, kraken, bitstamp, gemini)", s)
		}
		if seen[s] {
			errs.add(fmt.Sprintf("%s[%d]", path, j), "duplicate source %q", s)
		}
		seen[s] = true
	}
}

func validateFeed(cfg *Config, i int, seen map[string]int, errs *fieldErrors) {
	f := &cfg.Feeds[i]
	p := fmt.Sprintf("feeds[%d]", i)
	if f.Name == "" {
		errs.add(p+".name", "is required")
	}
	n := cfg.networks[f.Network]
	if n == nil {
		errs.add(p+".network", "unknown network %q", f.Network)
	}
	key := f.Network + "/" + f.Name
	if j, dup := seen[key]; dup {
		errs.add(p, "duplicate feed %q on %q (also feeds[%d])", f.Name, f.Network, j)
	}
	seen[key] = i
	if !f.Proxy.IsSet() {
		errs.add(p+".proxy", "is required")
	}
	if f.ExpectedDecimals == nil {
		errs.add(p+".expected_decimals", "is required (consumers depend on it; FW-META-001 compares against it)")
	}
	if f.Heartbeat.Duration <= 0 {
		errs.add(p+".heartbeat", "must be > 0")
	}
	if !positive(f.DeviationThreshold) || f.DeviationThreshold.Rat().Cmp(big.NewRat(1, 1)) >= 0 {
		errs.add(p+".deviation_threshold", "must be > 0%% and < 100%%")
	}
	if f.Reference != nil {
		if !pairPattern.MatchString(f.Reference.Pair) {
			errs.add(p+".reference.pair", "must look like ETH-USD (USD quotes only; USDT/USDC are not USD)")
		}
		if f.Reference.Sources != nil {
			validateSources(p+".reference.sources", f.Reference.Sources, errs)
		}
	}
	validateCheckParams(p+".checks", f, f.Checks, errs)
}

// validateCheckParams checks parameter ranges. When f is not nil it also checks
// the constraints that depend on the feed, such as the deviation alert level
// having to exceed the feed's own deviation threshold.
func validateCheckParams(p string, f *Feed, c CheckParams, errs *fieldErrors) {
	one := big.NewRat(1, 1)
	if s := c.Staleness; s.Enabled {
		if !nonNegative(s.Tolerance) {
			errs.add(p+".staleness.tolerance", "must be >= 0%%")
		}
		if s.MinSlack.Duration < 0 {
			errs.add(p+".staleness.min_slack", "must be >= 0")
		}
		if s.CriticalFactor < 1 {
			errs.add(p+".staleness.critical_factor", "must be >= 1")
		}
	}
	if c.Invalid.Enabled && c.Invalid.Skew.Duration < 0 {
		errs.add(p+".invalid.skew", "must be >= 0")
	}
	if d := c.Deviation; d.Enabled {
		if !positive(d.WarnAbove) {
			errs.add(p+".deviation.warn_above", "must be > 0%%")
		}
		if d.CriticalAbove.Rat().Cmp(d.WarnAbove.Rat()) <= 0 {
			errs.add(p+".deviation.critical_above", "must be greater than warn_above")
		}
		// The oracle may legitimately differ from the market by up to its
		// deviation threshold before the next round, so alerting below twice
		// that value produces alerts on healthy feeds.
		if f != nil && positive(f.DeviationThreshold) {
			minWarn := new(big.Rat).Mul(f.DeviationThreshold.Rat(), big.NewRat(2, 1))
			if d.WarnAbove.Rat().Cmp(minWarn) < 0 {
				errs.add(p+".deviation.warn_above", "%s is below 2 × deviation_threshold (%s)", d.WarnAbove, Percent{rat: minWarn})
			}
		}
	}
	if cb := c.CircuitBreaker; cb.Enabled {
		if !positive(cb.WarnWithin) || cb.WarnWithin.Rat().Cmp(one) >= 0 {
			errs.add(p+".circuit_breaker.warn_within", "must be > 0%% and < 100%%")
		}
		if !positive(cb.CriticalWithin) || cb.CriticalWithin.Rat().Cmp(cb.WarnWithin.Rat()) >= 0 {
			errs.add(p+".circuit_breaker.critical_within", "must be > 0%% and smaller than warn_within")
		}
	}
	if j := c.Jump; j.Enabled && !positive(j.MaxJump) {
		errs.add(p+".jump.max_jump", "must be > 0%%")
	}
}

func validateAlerting(cfg *Config, errs *fieldErrors) {
	a := &cfg.Alerting
	if a.For.Duration < 0 {
		errs.add("alerting.for", "must be >= 0")
	}
	if a.Cooldown.Duration < 0 {
		errs.add("alerting.cooldown", "must be >= 0")
	}
	if a.GroupWindow.Duration < 0 {
		errs.add("alerting.group_window", "must be >= 0")
	}
	if a.StormThreshold < 2 {
		errs.add("alerting.storm_threshold", "must be >= 2")
	}
	if d := a.Discord; d != nil && !d.WebhookURL.IsZero() {
		checkHTTPURL(cfg, "alerting.discord.webhook_url", d.WebhookURL, errs)
	}
	if t := a.Telegram; t != nil && t.BotToken.IsZero() != t.ChatID.IsZero() {
		errs.add("alerting.telegram", "bot_token and chat_id must both be set or both be empty")
	}
}

// checkHTTPURL validates a URL held in a Secret. Error messages never include
// the value itself. Plain http is accepted for loopback (a local node, tests)
// and warned about otherwise: the URL carries a credential, and an RPC
// response over plain http can be rewritten in transit.
func checkHTTPURL(cfg *Config, path string, s Secret, errs *fieldErrors) {
	if s.IsZero() {
		errs.add(path, "is empty")
		return
	}
	u, err := url.Parse(s.Value())
	if err != nil {
		errs.add(path, "is not a valid URL")
		return
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		errs.add(path, "must use http or https")
	}
	if u.Host == "" {
		errs.add(path, "has no host")
		return
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		cfg.Warnings = append(cfg.Warnings, path+": plain http to a non-loopback host sends the credential in clear text and lets the response be altered in transit; use https")
	}
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func positive(p Percent) bool    { return p.IsSet() && p.Rat().Sign() > 0 }
func nonNegative(p Percent) bool { return p.IsSet() && p.Rat().Sign() >= 0 }

// AsValidationError returns the problems of a validation error, if err is one.
func AsValidationError(err error) ([]string, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve.Problems, true
	}
	return nil, false
}
