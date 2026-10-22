// Package config loads, defaults and validates the oraclewatch YAML
// configuration.
//
// Loading is deliberately strict: unknown fields are errors, percentages need
// a "%" sign, addresses need a valid EIP-55 checksum, and every problem in the
// file is reported at once with the path of the offending field.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// maxConfigSize bounds the file read; a real config is a few KiB.
const maxConfigSize = 1 << 20

// Config is the validated configuration.
type Config struct {
	Networks   []Network  `yaml:"networks"`
	Reference  Reference  `yaml:"reference"`
	Defaults   Defaults   `yaml:"defaults"`
	Feeds      []Feed     `yaml:"feeds"`
	CrossChain CrossChain `yaml:"cross_chain"`
	RPC        RPCPolicy  `yaml:"rpc"`
	Alerting   Alerting   `yaml:"alerting"`
	SelfHealth SelfHealth `yaml:"self_health"`
	Server     Server     `yaml:"server"`
	Log        Log        `yaml:"log"`
	Warnings   []string   `yaml:"-"` // non-fatal findings from validation
	networks   map[string]*Network
}

// Network is one EVM chain and the endpoints used to read it.
type Network struct {
	Name          string     `yaml:"name"`
	ChainID       uint64     `yaml:"chain_id"`
	L2            bool       `yaml:"l2"`
	RPCEndpoints  []Endpoint `yaml:"rpc_endpoints"`
	PollInterval  Duration   `yaml:"poll_interval"`
	Confirmations uint64     `yaml:"confirmations"`
	MaxHeadLag    Duration   `yaml:"max_head_lag"`
	CallTimeout   Duration   `yaml:"call_timeout"`
	ExplorerURL   string     `yaml:"explorer_url"`
	Sequencer     *Sequencer `yaml:"sequencer"`
}

// Endpoint is one RPC endpoint. Name is what appears in logs and metrics; the
// URL is a Secret because it normally embeds an API key.
type Endpoint struct {
	Name string `yaml:"name"`
	URL  Secret `yaml:"url"`
}

// Sequencer configures the L2 sequencer uptime feed of a network.
type Sequencer struct {
	Proxy       Address  `yaml:"proxy"`
	GracePeriod Duration `yaml:"grace_period"`
}

// Reference configures the market price sources used for deviation checks.
type Reference struct {
	Sources         []string `yaml:"sources"`
	MinSources      int      `yaml:"min_sources"`
	MaxSourceSpread Percent  `yaml:"max_source_spread"`
	Timeout         Duration `yaml:"timeout"`
	MaxQuoteAge     Duration `yaml:"max_quote_age"`
	CacheTTL        Duration `yaml:"cache_ttl"`
}

// Defaults holds check parameters applied to every feed unless overridden.
type Defaults struct {
	Checks CheckParams `yaml:"checks"`
}

// Feed is one Chainlink Data Feed proxy to monitor.
type Feed struct {
	Name                string         `yaml:"name"`
	Network             string         `yaml:"network"`
	Proxy               Address        `yaml:"proxy"`
	ExpectedDecimals    *uint8         `yaml:"expected_decimals"`
	ExpectedDescription string         `yaml:"expected_description"`
	Heartbeat           Duration       `yaml:"heartbeat"`
	DeviationThreshold  Percent        `yaml:"deviation_threshold"`
	Reference           *FeedReference `yaml:"reference"`
	// ChecksOverride holds the feed's own "checks:" block. Load merges it on
	// top of Defaults.Checks into Checks.
	ChecksOverride yaml.Node   `yaml:"checks"`
	Checks         CheckParams `yaml:"-"`
}

// FeedReference selects the market pair (and optionally the sources) used as
// reference for a feed.
type FeedReference struct {
	Pair    string   `yaml:"pair"`
	Sources []string `yaml:"sources"`
}

// CheckParams are the tunable parameters of every check. See
// docs/research.md §8 for the meaning and rationale of each default.
type CheckParams struct {
	Staleness      StalenessParams      `yaml:"staleness"`
	Invalid        InvalidParams        `yaml:"invalid"`
	Deviation      DeviationParams      `yaml:"deviation"`
	CircuitBreaker CircuitBreakerParams `yaml:"circuit_breaker"`
	Jump           JumpParams           `yaml:"jump"`
}

// StalenessParams configures FW-STALE-001.
type StalenessParams struct {
	Enabled        bool     `yaml:"enabled"`
	Tolerance      Percent  `yaml:"tolerance"`
	MinSlack       Duration `yaml:"min_slack"`
	CriticalFactor int      `yaml:"critical_factor"`
}

// InvalidParams configures FW-INVALID-001.
type InvalidParams struct {
	Enabled bool     `yaml:"enabled"`
	Skew    Duration `yaml:"skew"`
}

// DeviationParams configures FW-DEV-001.
type DeviationParams struct {
	Enabled       bool    `yaml:"enabled"`
	WarnAbove     Percent `yaml:"warn_above"`
	CriticalAbove Percent `yaml:"critical_above"`
}

// CircuitBreakerParams configures FW-CB-001.
type CircuitBreakerParams struct {
	Enabled        bool    `yaml:"enabled"`
	WarnWithin     Percent `yaml:"warn_within"`
	CriticalWithin Percent `yaml:"critical_within"`
}

// JumpParams configures the experimental FW-JUMP-001.
type JumpParams struct {
	Enabled bool    `yaml:"enabled"`
	MaxJump Percent `yaml:"max_jump"`
}

// CrossChain configures FW-XCHAIN-001.
type CrossChain struct {
	Enabled bool    `yaml:"enabled"`
	Margin  Percent `yaml:"margin"`
}

// RPCPolicy configures the RPC trust checks.
type RPCPolicy struct {
	CompareEndpoints bool `yaml:"compare_endpoints"` // FW-RPC-001/002
	ChainIDRecheck   int  `yaml:"chain_id_recheck"`  // cycles between eth_chainId re-checks
	MetadataRefresh  int  `yaml:"metadata_refresh"`  // cycles between metadata reads
}

// Alerting configures the alert state machine and notifiers.
type Alerting struct {
	For            Duration  `yaml:"for"`
	Cooldown       Duration  `yaml:"cooldown"`
	SendResolved   bool      `yaml:"send_resolved"`
	GroupWindow    Duration  `yaml:"group_window"`
	StormThreshold int       `yaml:"storm_threshold"`
	Stdout         bool      `yaml:"stdout"`
	Discord        *Discord  `yaml:"discord"`
	Telegram       *Telegram `yaml:"telegram"`
}

// Discord configures the Discord webhook notifier.
type Discord struct {
	WebhookURL Secret `yaml:"webhook_url"`
}

// Telegram configures the Telegram Bot API notifier.
type Telegram struct {
	BotToken Secret `yaml:"bot_token"`
	ChatID   Secret `yaml:"chat_id"`
}

// SelfHealth configures FW-SELF-001 and the external dead man's switch.
type SelfHealth struct {
	BlindAfterCycles int    `yaml:"blind_after_cycles"`
	DeadmanURL       Secret `yaml:"deadman_url"`
}

// Server configures the HTTP server for /metrics, /healthz, /readyz, /status.
type Server struct {
	Listen string `yaml:"listen"`
}

// Log configures structured logging.
type Log struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, text
}

// Network returns the network with the given name, or nil.
func (c *Config) Network(name string) *Network { return c.networks[name] }

// Secrets returns every non-empty secret value, for registration with the
// redactor before anything is logged.
func (c *Config) Secrets() []string {
	var out []string
	add := func(s Secret) {
		if !s.IsZero() {
			out = append(out, s.Value())
		}
	}
	for _, n := range c.Networks {
		for _, e := range n.RPCEndpoints {
			add(e.URL)
		}
	}
	if c.Alerting.Discord != nil {
		add(c.Alerting.Discord.WebhookURL)
	}
	if c.Alerting.Telegram != nil {
		add(c.Alerting.Telegram.BotToken)
		add(c.Alerting.Telegram.ChatID)
	}
	add(c.SelfHealth.DeadmanURL)
	return out
}

// defaultConfig returns the values applied before the file is decoded. The
// YAML decoder only overwrites fields present in the file.
func defaultConfig() Config {
	return Config{
		Reference: Reference{
			Sources:         []string{"coinbase", "kraken", "bitstamp"},
			MinSources:      2,
			MaxSourceSpread: MustPercent("1%"),
			Timeout:         Duration{5 * time.Second},
			MaxQuoteAge:     Duration{2 * time.Minute},
			CacheTTL:        Duration{10 * time.Second},
		},
		Defaults:   Defaults{Checks: DefaultCheckParams()},
		CrossChain: CrossChain{Enabled: true, Margin: MustPercent("1%")},
		RPC:        RPCPolicy{CompareEndpoints: true, ChainIDRecheck: 60, MetadataRefresh: 10},
		Alerting: Alerting{
			For:            Duration{2 * time.Minute},
			Cooldown:       Duration{30 * time.Minute},
			SendResolved:   true,
			GroupWindow:    Duration{10 * time.Second},
			StormThreshold: 5,
			Stdout:         true,
		},
		SelfHealth: SelfHealth{BlindAfterCycles: 3},
		Server:     Server{Listen: "127.0.0.1:9090"},
		Log:        Log{Level: "info", Format: "json"},
	}
}

// DefaultCheckParams returns the default parameters of every check
// (docs/research.md §8). Experimental checks are disabled.
func DefaultCheckParams() CheckParams {
	return CheckParams{
		Staleness: StalenessParams{
			Enabled:        true,
			Tolerance:      MustPercent("10%"),
			MinSlack:       Duration{60 * time.Second},
			CriticalFactor: 2,
		},
		Invalid: InvalidParams{Enabled: true, Skew: Duration{60 * time.Second}},
		Deviation: DeviationParams{
			Enabled:       true,
			WarnAbove:     MustPercent("2%"),
			CriticalAbove: MustPercent("5%"),
		},
		CircuitBreaker: CircuitBreakerParams{
			Enabled:        true,
			WarnWithin:     MustPercent("2%"),
			CriticalWithin: MustPercent("0.5%"),
		},
		Jump: JumpParams{Enabled: false, MaxJump: MustPercent("20%")},
	}
}

// networkDefaults fills fields left empty in a network block.
func networkDefaults(n *Network) {
	if n.PollInterval.Duration == 0 {
		n.PollInterval = Duration{60 * time.Second}
	}
	if n.CallTimeout.Duration == 0 {
		n.CallTimeout = Duration{10 * time.Second}
	}
	if n.MaxHeadLag.Duration == 0 {
		if n.L2 {
			n.MaxHeadLag = Duration{2 * time.Minute}
		} else {
			n.MaxHeadLag = Duration{5 * time.Minute}
		}
	}
	if n.Sequencer != nil && n.Sequencer.GracePeriod.Duration == 0 {
		// GRACE_PERIOD_TIME in the official sequencer feed example.
		n.Sequencer.GracePeriod = Duration{time.Hour}
	}
}

// Load reads, defaults, expands and validates the configuration at path.
// lookupEnv is normally os.LookupEnv; tests pass a map-backed function.
func Load(path string, lookupEnv func(string) (string, bool)) (*Config, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the path is chosen by the operator on the command line
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file; a close error cannot lose data
	data, err := io.ReadAll(io.LimitReader(f, maxConfigSize+1))
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if len(data) > maxConfigSize {
		return nil, fmt.Errorf("config file is larger than %d bytes", maxConfigSize)
	}
	return Parse(data, lookupEnv)
}

// Parse is Load for in-memory data.
func Parse(data []byte, lookupEnv func(string) (string, bool)) (*Config, error) {
	cfg := defaultConfig()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config is empty")
		}
		return nil, fmt.Errorf("parse config: %w", err)
	}

	var errs fieldErrors
	for i := range cfg.Networks {
		networkDefaults(&cfg.Networks[i])
	}
	for i := range cfg.Feeds {
		params, err := mergeChecks(cfg.Defaults.Checks, &cfg.Feeds[i].ChecksOverride)
		if err != nil {
			errs.add(fmt.Sprintf("feeds[%d].checks", i), "%v", err)
		}
		cfg.Feeds[i].Checks = params
	}
	expandSecrets(&cfg, lookupEnv, &errs)
	validate(&cfg, &errs)
	if err := errs.err(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// mergeChecks decodes a feed's checks block on top of a copy of the defaults.
// The node is re-encoded and decoded with KnownFields so that a typo in a
// per-feed override is an error, as it is everywhere else.
func mergeChecks(defaults CheckParams, override *yaml.Node) (CheckParams, error) {
	params := defaults
	if override.Kind == 0 {
		return params, nil
	}
	raw, err := yaml.Marshal(override)
	if err != nil {
		return params, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&params); err != nil {
		return defaults, err
	}
	return params, nil
}

// expandSecrets resolves ${VAR} in every Secret field. This explicit list is
// the only place where environment values enter the configuration.
func expandSecrets(cfg *Config, lookup func(string) (string, bool), errs *fieldErrors) {
	exp := func(path string, s *Secret) {
		v, err := expand(s.raw, lookup)
		if err != nil {
			errs.add(path, "%v", err)
			return
		}
		s.value = v
	}
	for i := range cfg.Networks {
		for j := range cfg.Networks[i].RPCEndpoints {
			exp(fmt.Sprintf("networks[%d].rpc_endpoints[%d].url", i, j), &cfg.Networks[i].RPCEndpoints[j].URL)
		}
	}
	if d := cfg.Alerting.Discord; d != nil {
		exp("alerting.discord.webhook_url", &d.WebhookURL)
	}
	if t := cfg.Alerting.Telegram; t != nil {
		exp("alerting.telegram.bot_token", &t.BotToken)
		exp("alerting.telegram.chat_id", &t.ChatID)
	}
	exp("self_health.deadman_url", &cfg.SelfHealth.DeadmanURL)
}
