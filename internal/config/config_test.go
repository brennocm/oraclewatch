package config

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// exampleEnv defines every variable referenced by configs/example.yaml.
var exampleEnv = env(map[string]string{
	"ETH_RPC_URL_PRIMARY": "https://eth-a.example.com/v2/key-eth-a-0123456789", "ETH_RPC_URL_SECONDARY": "https://eth-b.example.com/key-eth-b-0123456789",
	"ARB_RPC_URL_PRIMARY": "https://arb-a.example.com/v2/key-arb-a-0123456789", "ARB_RPC_URL_SECONDARY": "https://arb-b.example.com/key-arb-b-0123456789",
	"BASE_RPC_URL_PRIMARY": "https://base-a.example.com/v2/key-base-a-0123456789", "BASE_RPC_URL_SECONDARY": "https://base-b.example.com/key-base-b-0123456789",
	"OP_RPC_URL_PRIMARY": "https://op-a.example.com/v2/key-op-a-0123456789", "OP_RPC_URL_SECONDARY": "https://op-b.example.com/key-op-b-0123456789",
	"DISCORD_WEBHOOK_URL": "", "TELEGRAM_BOT_TOKEN": "", "TELEGRAM_CHAT_ID": "", "DEADMAN_URL": "",
})

// minimal is the smallest valid configuration; invalid cases mutate it.
const minimal = `
networks:
  - name: ethereum
    chain_id: 1
    rpc_endpoints:
      - name: a
        url: ${RPC_A}
      - name: b
        url: https://rpc.example.com
feeds:
  - name: ETH/USD
    network: ethereum
    proxy: "0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419"
    expected_decimals: 8
    heartbeat: 1h
    deviation_threshold: 0.5%
    reference:
      pair: ETH-USD
`

var minimalEnv = env(map[string]string{"RPC_A": "https://a.example.com/v2/abcdefghijklmnop"})

func TestExampleConfigLoads(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "configs", "example.yaml"), exampleEnv)
	if err != nil {
		t.Fatalf("example config: %v", err)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("example config produces warnings: %v", cfg.Warnings)
	}
	if got := len(cfg.Feeds); got != 12 {
		t.Errorf("feeds = %d, want 12", got)
	}
	if cfg.Alerting.Discord == nil || !cfg.Alerting.Discord.WebhookURL.IsZero() {
		t.Error("empty DISCORD_WEBHOOK_URL should leave the notifier configured but empty")
	}
}

// verifiedFile mirrors configs/feeds/<network>.yaml.
type verifiedFile struct {
	Network   string `yaml:"network"`
	ChainID   uint64 `yaml:"chain_id"`
	Sequencer *struct {
		Proxy string `yaml:"proxy"`
	} `yaml:"sequencer_uptime_feed"`
	Feeds []struct {
		Name               string `yaml:"name"`
		Proxy              string `yaml:"proxy"`
		Description        string `yaml:"description"`
		Decimals           uint8  `yaml:"decimals"`
		Heartbeat          string `yaml:"heartbeat"`
		DeviationThreshold string `yaml:"deviation_threshold"`
	} `yaml:"feeds"`
}

// TestExampleMatchesVerifiedFeeds guarantees that every address, decimals,
// description, heartbeat and threshold in the example equals the verified
// lists, so a hand edit of either file cannot go unnoticed.
func TestExampleMatchesVerifiedFeeds(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "configs", "example.yaml"), exampleEnv)
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "configs", "feeds", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no verified feed files: %v", err)
	}
	want := map[string]bool{}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var vf verifiedFile
		if err := yaml.Unmarshal(raw, &vf); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		n := cfg.Network(vf.Network)
		if n == nil {
			t.Fatalf("%s: network %q missing from example", path, vf.Network)
		}
		if n.ChainID != vf.ChainID {
			t.Errorf("%s: chain_id %d != %d", vf.Network, n.ChainID, vf.ChainID)
		}
		if (vf.Sequencer == nil) != (n.Sequencer == nil) {
			t.Errorf("%s: sequencer presence differs", vf.Network)
		} else if vf.Sequencer != nil && n.Sequencer.Proxy.Hex() != vf.Sequencer.Proxy {
			t.Errorf("%s: sequencer proxy %s != %s", vf.Network, n.Sequencer.Proxy.Hex(), vf.Sequencer.Proxy)
		}
		for _, vfeed := range vf.Feeds {
			key := vf.Network + "/" + vfeed.Name
			want[key] = true
			f := findFeed(cfg, vf.Network, vfeed.Name)
			if f == nil {
				t.Errorf("%s missing from example", key)
				continue
			}
			hb, _ := time.ParseDuration(vfeed.Heartbeat)
			dt := MustPercent(vfeed.DeviationThreshold)
			switch {
			case f.Proxy.Hex() != vfeed.Proxy:
				t.Errorf("%s: proxy %s != verified %s", key, f.Proxy.Hex(), vfeed.Proxy)
			case *f.ExpectedDecimals != vfeed.Decimals:
				t.Errorf("%s: decimals %d != %d", key, *f.ExpectedDecimals, vfeed.Decimals)
			case f.ExpectedDescription != vfeed.Description:
				t.Errorf("%s: description %q != %q", key, f.ExpectedDescription, vfeed.Description)
			case f.Heartbeat.Duration != hb:
				t.Errorf("%s: heartbeat %s != %s", key, f.Heartbeat, hb)
			case f.DeviationThreshold.Rat().Cmp(dt.Rat()) != 0:
				t.Errorf("%s: deviation %s != %s", key, f.DeviationThreshold, dt)
			}
		}
	}
	for _, f := range cfg.Feeds {
		if !want[f.Network+"/"+f.Name] {
			t.Errorf("example feed %s/%s is not in the verified lists", f.Network, f.Name)
		}
	}
}

func findFeed(cfg *Config, network, name string) *Feed {
	for i := range cfg.Feeds {
		if cfg.Feeds[i].Network == network && cfg.Feeds[i].Name == name {
			return &cfg.Feeds[i]
		}
	}
	return nil
}

func TestMinimalDefaults(t *testing.T) {
	cfg, err := Parse([]byte(minimal), minimalEnv)
	if err != nil {
		t.Fatal(err)
	}
	n := cfg.Network("ethereum")
	if n.PollInterval.Duration != 60*time.Second || n.MaxHeadLag.Duration != 5*time.Minute || n.CallTimeout.Duration != 10*time.Second {
		t.Errorf("network defaults not applied: %+v", n)
	}
	f := cfg.Feeds[0]
	if !f.Checks.Staleness.Enabled || f.Checks.Jump.Enabled {
		t.Errorf("check defaults: staleness=%v jump=%v", f.Checks.Staleness.Enabled, f.Checks.Jump.Enabled)
	}
	if got := n.RPCEndpoints[0].URL.Value(); got != "https://a.example.com/v2/abcdefghijklmnop" {
		t.Errorf("secret not expanded: %q", got)
	}
	if got := n.RPCEndpoints[0].URL.String(); got != "[REDACTED]" {
		t.Errorf("Secret.String() = %q", got)
	}
	if got := fmt.Sprintf("%v %+v %#v", n.RPCEndpoints[0], n.RPCEndpoints[0], n.RPCEndpoints[0].URL); strings.Contains(got, "abcdefghijklmnop") {
		t.Errorf("secret printed by fmt: %s", got)
	}
	if len(cfg.Secrets()) != 2 {
		t.Errorf("Secrets() = %d values, want 2", len(cfg.Secrets()))
	}
}

func TestFeedOverrideMergesOnDefaults(t *testing.T) {
	data := minimal + `    checks:
      staleness:
        tolerance: 25%
      jump:
        enabled: true
`
	cfg, err := Parse([]byte(data), minimalEnv)
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Feeds[0].Checks
	if c.Staleness.Tolerance.Rat().Cmp(big.NewRat(1, 4)) != 0 {
		t.Errorf("tolerance = %s, want 25%%", c.Staleness.Tolerance)
	}
	if c.Staleness.MinSlack.Duration != 60*time.Second || !c.Staleness.Enabled {
		t.Error("fields not in the override must keep their defaults")
	}
	if !c.Jump.Enabled || c.Jump.MaxJump.Rat().Cmp(big.NewRat(1, 5)) != 0 {
		t.Errorf("jump override: %+v", c.Jump)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		replace [2]string // applied to minimal
		append  string
		env     map[string]string
		want    []string // substrings of the error
	}{
		{name: "unknown top-level key", append: "\nbogus: 1\n", want: []string{"field bogus not found"}},
		{name: "percent without sign", replace: [2]string{"deviation_threshold: 0.5%", "deviation_threshold: 0.5"}, want: []string{"must end with %"}},
		{name: "bad checksum", replace: [2]string{"0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419", "0x5f4ec3df9cbd43714fe2740f5e3616155c5b8419"}, want: []string{"EIP-55"}},
		{name: "undefined env var", env: map[string]string{}, want: []string{"networks[0].rpc_endpoints[0].url", "RPC_A"}},
		{name: "unknown network", replace: [2]string{"network: ethereum", "network: solana"}, want: []string{`feeds[0].network: unknown network "solana"`}},
		{name: "zero heartbeat", replace: [2]string{"heartbeat: 1h", "heartbeat: 0s"}, want: []string{"feeds[0].heartbeat: must be > 0"}},
		{name: "missing decimals", replace: [2]string{"    expected_decimals: 8\n", ""}, want: []string{"feeds[0].expected_decimals: is required"}},
		{name: "usdt quote rejected", replace: [2]string{"pair: ETH-USD", "pair: ETH-USDT"}, want: []string{"feeds[0].reference.pair"}},
		{name: "deviation alert below 2x threshold", replace: [2]string{"deviation_threshold: 0.5%", "deviation_threshold: 1.5%"}, want: []string{"below 2 × deviation_threshold (3%)"}},
		{name: "sequencer on L1", replace: [2]string{"    rpc_endpoints:\n", "    sequencer:\n      proxy: \"0xFdB631F5EE196F0ed6FAa767959853A9F217697D\"\n    rpc_endpoints:\n"}, want: []string{"networks[0].sequencer: only allowed on networks with l2: true"}},
		{name: "poll too fast", replace: [2]string{"chain_id: 1\n", "chain_id: 1\n    poll_interval: 5s\n"}, want: []string{"poll_interval: must be >= 15s"}},
		{name: "unknown check key in override", append: "    checks:\n      staleness:\n        max_age: 5m\n", want: []string{"feeds[0].checks", "max_age"}},
		{name: "unknown source", append: "reference:\n  sources: [binance]\n", want: []string{`reference.sources[0]: unknown source "binance"`}},
		{name: "dollar outside reference", replace: [2]string{"url: https://rpc.example.com", "url: https://rpc.example.com/$KEY"}, want: []string{"networks[0].rpc_endpoints[1].url", "${NAME}"}},
		{name: "empty", replace: [2]string{minimal, ""}, want: []string{"config is empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := minimal
			if tt.replace[0] != "" {
				if !strings.Contains(data, tt.replace[0]) {
					t.Fatalf("test setup: %q not found", tt.replace[0])
				}
				data = strings.Replace(data, tt.replace[0], tt.replace[1], 1)
			}
			data += tt.append
			e := minimalEnv
			if tt.env != nil {
				e = env(tt.env)
			}
			_, err := Parse([]byte(data), e)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestAllProblemsReportedAtOnce(t *testing.T) {
	data := strings.NewReplacer("heartbeat: 1h", "heartbeat: 0s", "network: ethereum", "network: nowhere").Replace(minimal)
	_, err := Parse([]byte(data), minimalEnv)
	problems, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want ValidationError, got %v", err)
	}
	if len(problems) < 2 {
		t.Fatalf("want every problem reported, got %v", problems)
	}
}

func TestSecretValueNotInErrors(t *testing.T) {
	// An invalid URL in a secret must not echo the value back.
	e := env(map[string]string{"RPC_A": "ftp://user:hunter2-very-secret@host/path"})
	_, err := Parse([]byte(minimal), e)
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("secret leaked in error: %v", err)
	}
}

func TestSingleEndpointWarns(t *testing.T) {
	data := strings.Replace(minimal, "      - name: b\n        url: https://rpc.example.com\n", "", 1)
	cfg, err := Parse([]byte(data), minimalEnv)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "FW-RPC-001") {
		t.Fatalf("warnings = %v", cfg.Warnings)
	}
}

func TestParsePercent(t *testing.T) {
	tests := []struct {
		in      string
		num     int64
		den     int64
		wantErr bool
	}{
		{"0.5%", 1, 200, false},
		{"100%", 1, 1, false},
		{" 0.05 % ", 1, 2000, false},
		{"0%", 0, 1, false},
		{"0.5", 0, 0, true},
		{"%", 0, 0, true},
		{"1/3%", 0, 0, true},
		{"1e2%", 0, 0, true},
		{"abc%", 0, 0, true},
	}
	for _, tt := range tests {
		p, err := ParsePercent(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParsePercent(%q) err = %v", tt.in, err)
			continue
		}
		if !tt.wantErr && p.Rat().Cmp(big.NewRat(tt.num, tt.den)) != 0 {
			t.Errorf("ParsePercent(%q) = %s", tt.in, p.Rat())
		}
	}
	if got := MustPercent("0.15%").String(); got != "0.15%" {
		t.Errorf("String() = %q", got)
	}
}

func TestExpand(t *testing.T) {
	lookup := env(map[string]string{"A": "x", "B_2": "y", "EMPTY": ""})
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{"${A}", "x", false},
		{"https://h/${A}/${B_2}", "https://h/x/y", false},
		{"${EMPTY}", "", false},
		{"literal", "literal", false},
		{"${MISSING}", "", true},
		{"$A", "", true},
		{"${A", "", true},
		{"${a}", "", true},
		{"${1A}", "", true},
		{"tail$", "", true},
	}
	for _, tt := range tests {
		got, err := expand(tt.in, lookup)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("expand(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(minimal))
	if b, err := os.ReadFile(filepath.Join("..", "..", "configs", "example.yaml")); err == nil {
		f.Add(b)
	}
	f.Add([]byte("networks: [{name: x}]"))
	f.Add([]byte("feeds:\n  - checks: {staleness: {tolerance: '%'}}"))
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := Parse(data, minimalEnv)
		if err == nil && cfg == nil {
			t.Fatal("nil config without error")
		}
	})
}

func FuzzParsePercent(f *testing.F) {
	for _, s := range []string{"0.5%", "100%", "-1%", "1e9%", "%", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParsePercent(s)
		if err != nil {
			return
		}
		// Round trip through String must give the same value for inputs
		// with at most 4 decimals in percent units.
		if !strings.Contains(s, ".") || len(strings.SplitN(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%")), ".", 2)[1]) <= 4 {
			q, err := ParsePercent(p.String())
			if err != nil || q.Rat().Cmp(p.Rat()) != 0 {
				t.Fatalf("round trip %q -> %q -> %v (%v)", s, p.String(), q.Rat(), err)
			}
		}
	})
}
