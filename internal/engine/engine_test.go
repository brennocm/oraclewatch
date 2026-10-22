package engine

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/testutil/fakerpc"
)

const (
	ethProxy = "0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419"
	ethAgg   = "0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5"
	arbProxy = "0x639Fe6ab55C921f74e7fac1ee960C0B6293ba612"
	arbAgg   = "0xD827123D014578C965F6c9d87A641ec05FaA5501"
	seqProxy = "0xFdB631F5EE196F0ed6FAa767959853A9F217697D"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	yaml := fmt.Sprintf(`
networks:
  - name: ethereum
    chain_id: 1
    rpc_endpoints: [{name: a, url: "https://a.invalid"}, {name: b, url: "https://b.invalid"}]
    poll_interval: 15s
    call_timeout: 2s
    confirmations: 2
  - name: arbitrum
    chain_id: 42161
    l2: true
    rpc_endpoints: [{name: a, url: "https://c.invalid"}, {name: b, url: "https://d.invalid"}]
    poll_interval: 15s
    call_timeout: 2s
    sequencer: {proxy: "%s"}
feeds:
  - {name: ETH/USD, network: ethereum, proxy: "%s", expected_decimals: 8, expected_description: "ETH / USD", heartbeat: 1h, deviation_threshold: 0.5%%}
  - {name: ETH/USD, network: arbitrum, proxy: "%s", expected_decimals: 8, expected_description: "ETH / USD", heartbeat: 1755s, deviation_threshold: 0.05%%}
alerting: {for: 0s}
`, seqProxy, ethProxy, arbProxy)
	cfg, err := config.Parse([]byte(yaml), func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

type fixture struct {
	cfg    *config.Config
	clk    *clock.Fake
	chains map[string][2]*fakerpc.Chain
	eng    *Engine
}

var now0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func ethFeed(agg string, answer int64, updatedAt time.Time) *fakerpc.Feed {
	return &fakerpc.Feed{
		Decimals: 8, Description: "ETH / USD", Aggregator: common.HexToAddress(agg), Phase: 7,
		Round: big.NewInt(100), Answer: big.NewInt(answer), StartedAt: updatedAt.Unix() - 10, UpdatedAt: updatedAt.Unix(),
		Min: big.NewInt(1), Max: new(big.Int).Lsh(big.NewInt(1), 180), TypeAndVersion: "AccessControlledOCR2Aggregator 1.0.0",
	}
}

func newFixture(t *testing.T, opts func(*Options)) *fixture {
	t.Helper()
	cfg := testConfig(t)
	clk := clock.NewFake(now0)
	fx := &fixture{cfg: cfg, clk: clk, chains: map[string][2]*fakerpc.Chain{}}
	var nets []*chain.Network
	for _, cn := range cfg.Networks {
		pair := [2]*fakerpc.Chain{fakerpc.New(cn.ChainID, now0), fakerpc.New(cn.ChainID, now0)}
		for _, c := range pair {
			if cn.Name == "ethereum" {
				c.Feeds[common.HexToAddress(ethProxy)] = ethFeed(ethAgg, 244363658508, now0.Add(-time.Minute))
			} else {
				c.Feeds[common.HexToAddress(arbProxy)] = ethFeed(arbAgg, 244300000000, now0.Add(-time.Minute))
				seq := ethFeed(seqProxy, 0, now0.Add(-48*time.Hour))
				seq.Aggregator = common.HexToAddress("0xC1303BBBaf172C55848D3Cb91606d8E27FF38428")
				c.Feeds[common.HexToAddress(seqProxy)] = seq
			}
		}
		fx.chains[cn.Name] = pair
		n := &chain.Network{Name: cn.Name, ChainID: cn.ChainID, Confirmations: cn.Confirmations, MaxHeadLag: cn.MaxHeadLag.Duration, Clock: clk, RequireVerified: true}
		for i, c := range pair {
			n.Endpoints = append(n.Endpoints, chain.NewEndpoint(cn.Name, string(rune('a'+i)), c, chain.EndpointOptions{Timeout: time.Second, Clock: clk}))
		}
		nets = append(nets, n)
	}
	o := Options{Config: cfg, Networks: nets, Clock: clk}
	if opts != nil {
		opts(&o)
	}
	eng, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	fx.eng = eng
	return fx
}

func byKey(rs []checks.Result) map[string]checks.Result {
	m := map[string]checks.Result{}
	for _, r := range rs {
		m[r.CheckID+"|"+r.Network+"|"+r.Feed] = r
	}
	return m
}

func TestRunOnceHealthy(t *testing.T) {
	fx := newFixture(t, nil)
	rs := byKey(fx.eng.RunOnce(context.Background()))
	for key, r := range rs {
		if r.Status >= checks.Warning {
			t.Errorf("%s: %s %s", key, r.Status, r.Summary)
		}
	}
	for _, key := range []string{"FW-STALE-001|ethereum|ETH/USD", "FW-RPC-001|arbitrum|ETH/USD", "FW-SEQ-001|arbitrum|", "FW-XCHAIN-001|*|ETH/USD", "FW-CB-001|ethereum|ETH/USD"} {
		if r, ok := rs[key]; !ok || (r.Status != checks.OK) {
			t.Errorf("%s = %+v", key, r)
		}
	}
	if !fx.eng.Ready() {
		t.Error("engine should be ready after a healthy cycle")
	}
}

func TestRunOnceDetectsLyingRPCAndStaleFeed(t *testing.T) {
	fx := newFixture(t, nil)
	eth := fx.chains["ethereum"]
	eth[1].Set(func(*fakerpc.Chain) { eth[1].Feeds[common.HexToAddress(ethProxy)].Answer = big.NewInt(100) }) // lies
	arb := fx.chains["arbitrum"]
	for _, c := range arb {
		c.Set(func(*fakerpc.Chain) {
			f := c.Feeds[common.HexToAddress(arbProxy)]
			f.UpdatedAt = now0.Add(-3 * time.Hour).Unix()
			f.StartedAt = f.UpdatedAt - 5
		})
	}
	rs := byKey(fx.eng.RunOnce(context.Background()))
	if r := rs["FW-RPC-001|ethereum|ETH/USD"]; r.Status != checks.Critical {
		t.Errorf("RPC disagreement: %+v", r)
	}
	if r := rs["FW-STALE-001|arbitrum|ETH/USD"]; r.Status != checks.Critical {
		t.Errorf("stale: %+v", r)
	}
	if r := rs["FW-XCHAIN-001|*|ETH/USD"]; r.Status != checks.Unknown {
		t.Errorf("cross-chain must skip a stale side: %+v", r)
	}
}

func TestWrongChainEndpointIsDisabled(t *testing.T) {
	fx := newFixture(t, nil)
	fx.chains["ethereum"][1].Set(func(c *fakerpc.Chain) { c.ID = 56 })
	rs := byKey(fx.eng.RunOnce(context.Background()))
	if r := rs["FW-RPC-003|ethereum|"]; r.Status != checks.Critical || !strings.Contains(r.Summary, "b") {
		t.Fatalf("wrong chain: %+v", r)
	}
	if r := rs["FW-RPC-001|ethereum|ETH/USD"]; r.Status != checks.Unknown {
		t.Fatalf("only one endpoint left to read: %+v", r)
	}
	if r := rs["FW-STALE-001|ethereum|ETH/USD"]; r.Status != checks.OK {
		t.Fatalf("reads continue on the healthy endpoint: %+v", r)
	}
}

func TestBlindMonitorAndStalledChain(t *testing.T) {
	fx := newFixture(t, nil)
	for _, c := range fx.chains["ethereum"] {
		c.Set(func(*fakerpc.Chain) { c.Fail = true })
	}
	for _, c := range fx.chains["arbitrum"] {
		c.Set(func(*fakerpc.Chain) { c.HeadAt = now0.Add(-time.Hour) })
	}
	var rs map[string]checks.Result
	for i := 0; i < 3; i++ {
		rs = byKey(fx.eng.RunOnce(context.Background()))
	}
	if r := rs["FW-SELF-001|ethereum|"]; r.Status != checks.Critical {
		t.Errorf("blind: %+v", r)
	}
	if r := rs["FW-CHAIN-001|arbitrum|"]; r.Status != checks.Critical {
		t.Errorf("stalled: %+v", r)
	}
	if r := rs["FW-STALE-001|arbitrum|ETH/USD"]; r.Status != checks.Unknown {
		t.Errorf("stale must be Unknown on a stalled chain: %+v", r)
	}
	if fx.eng.Ready() {
		t.Error("not ready while networks are blind")
	}
}

func TestAggregatorChangeIsReportedAndBoundsReread(t *testing.T) {
	fx := newFixture(t, nil)
	fx.eng.RunOnce(context.Background())
	newAgg := "0x54bCC589d9743E521c64233706FC8cB36D275b07"
	for _, c := range fx.chains["ethereum"] {
		c.Set(func(*fakerpc.Chain) {
			f := c.Feeds[common.HexToAddress(ethProxy)]
			f.Aggregator, f.Phase = common.HexToAddress(newAgg), 8
			f.Max = big.NewInt(244400000000) // tight bound on the new aggregator: 0.0149% away
		})
	}
	// Metadata is refreshed every metadata_refresh (10) cycles.
	var rs map[string]checks.Result
	for i := 0; i < 10; i++ {
		rs = byKey(fx.eng.RunOnce(context.Background()))
	}
	if r := rs["FW-AGG-001|ethereum|ETH/USD"]; r.Status != checks.Info {
		t.Fatalf("aggregator change: %+v", r)
	}
	if r := rs["FW-CB-001|ethereum|ETH/USD"]; r.Status != checks.Critical {
		t.Fatalf("bounds of the new aggregator must be read: %+v", r)
	}
}

type countingNotifier struct {
	mu   sync.Mutex
	msgs []alert.Message
}

func (c *countingNotifier) Name() string { return "test" }
func (c *countingNotifier) Send(_ context.Context, m alert.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}

// TestRunConcurrentSlowRPCAndShutdown runs the daemon loop with several
// networks, a slow endpoint and cancellation in the middle of a cycle. Run it
// with -race. It must return promptly and deliver the alert raised before
// shutdown.
func TestRunConcurrentSlowRPCAndShutdown(t *testing.T) {
	n := &countingNotifier{}
	disp := alert.NewDispatcher([]alert.Notifier{n}, alert.DispatcherOptions{GroupWindow: time.Hour, StormThreshold: 10, FlushTimeout: 2 * time.Second})
	fx := newFixture(t, func(o *Options) {
		o.Clock = clock.Real{}
		o.Alerts = alert.NewManager(alert.Options{For: 0, Cooldown: time.Hour})
		o.Dispatcher = disp
		o.ShutdownGrace = 500 * time.Millisecond
	})
	for _, nw := range fx.eng.networks {
		nw.net.Clock = clock.Real{}
	}
	for _, c := range fx.chains["ethereum"] {
		c.Set(func(*fakerpc.Chain) {
			c.HeadAt = time.Now()
			c.Feeds[common.HexToAddress(ethProxy)].Answer = big.NewInt(0)
		}) // invalid -> critical
	}
	for _, c := range fx.chains["arbitrum"] {
		c.Set(func(*fakerpc.Chain) { c.HeadAt = time.Now(); c.Delay = 300 * time.Millisecond })
	}

	dctx, dcancel := context.WithCancel(context.Background())
	go disp.Run(dctx)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- fx.eng.Run(ctx) }()

	time.Sleep(400 * time.Millisecond) // arbitrum is mid-cycle
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	dcancel()
	<-disp.Done()

	n.mu.Lock()
	defer n.mu.Unlock()
	found := false
	for _, m := range n.msgs {
		for _, nt := range m.Notifications {
			if nt.Result.CheckID == "FW-INVALID-001" && nt.Result.Network == "ethereum" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("alert raised before shutdown was not delivered: %d messages", len(n.msgs))
	}
}

func TestDecimalsChangeIsDetectedInTheNextCycle(t *testing.T) {
	fx := newFixture(t, nil)
	fx.eng.RunOnce(context.Background()) // cycle 0: metadata read
	for _, c := range fx.chains["ethereum"] {
		c.Set(func(c *fakerpc.Chain) { c.Feeds[common.HexToAddress(ethProxy)].Decimals = 18 })
	}
	rs := byKey(fx.eng.RunOnce(context.Background())) // cycle 1: not a refresh cycle
	if r := rs["FW-META-001|ethereum|ETH/USD"]; r.Status != checks.Critical {
		t.Fatalf("decimals change must be reported at once, not after metadata_refresh cycles: %+v", r)
	}
}

func TestCrossChainResultIsInStatus(t *testing.T) {
	fx := newFixture(t, nil)
	fx.eng.RunOnce(context.Background())
	found := false
	for _, r := range fx.eng.Results() {
		if r.CheckID == checks.CrossChainID {
			found = true
		}
	}
	if !found {
		t.Fatal("FW-XCHAIN-001 must appear in Results() (/status)")
	}
}
