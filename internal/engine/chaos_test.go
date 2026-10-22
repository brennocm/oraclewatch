package engine

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/reference"
	"github.com/brennocm/oraclewatch/internal/testutil/fakerpc"
)

// Chaos tests: the monitor must degrade gracefully, alert what can be alerted
// and never hang or panic.

func TestChaosGarbageRPC(t *testing.T) {
	fx := newFixture(t, nil)
	fx.chains["ethereum"][0].Set(func(c *fakerpc.Chain) { c.Garbage = true })
	rs := byKey(fx.eng.RunOnce(context.Background()))
	// The second endpoint still answers correctly, so the feed is read; the
	// garbage endpoint simply fails its reads.
	if r := rs["FW-STALE-001|ethereum|ETH/USD"]; r.Status != checks.OK {
		t.Fatalf("stale: %+v", r)
	}
	if r := rs["FW-RPC-001|ethereum|ETH/USD"]; r.Status != checks.Unknown {
		t.Fatalf("one endpoint answering garbage cannot be compared: %+v", r)
	}
	fx.chains["ethereum"][1].Set(func(c *fakerpc.Chain) { c.Garbage = true })
	for i := 0; i < 3; i++ {
		rs = byKey(fx.eng.RunOnce(context.Background()))
	}
	if r := rs["FW-SELF-001|ethereum|"]; r.Status != checks.Critical {
		t.Fatalf("all endpoints answering garbage must make the monitor blind: %+v", r)
	}
}

func TestChaosEndpointThousandBlocksBehind(t *testing.T) {
	fx := newFixture(t, nil)
	fx.chains["ethereum"][1].Set(func(c *fakerpc.Chain) {
		c.Head -= 1000
		c.HeadAt = c.HeadAt.Add(-2000 * time.Second)
	})
	rs := byKey(fx.eng.RunOnce(context.Background()))
	if r := rs["FW-CHAIN-001|ethereum|"]; r.Status != checks.Warning {
		t.Fatalf("lagging endpoint: %+v", r)
	}
	if r := rs["FW-STALE-001|ethereum|ETH/USD"]; r.Status != checks.OK {
		t.Fatalf("reads must continue on the current endpoint: %+v", r)
	}
}

// exchange serves a fixed Gemini-style ticker, or an error status.
func exchange(t *testing.T, status int, bid, ask string) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"bid":"` + bid + `","ask":"` + ask + `"}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func withReference(t *testing.T, statuses ...int) func(*Options) {
	return func(o *Options) {
		var srcs []reference.Source
		var names []string
		for i, st := range statuses {
			name := string(rune('a' + i))
			srcs = append(srcs, renamed{reference.NewGemini(nil, exchange(t, st, "2443.60", "2443.70").URL, nil), name})
			names = append(names, name)
		}
		for i := range o.Config.Feeds {
			o.Config.Feeds[i].Reference = &config.FeedReference{Pair: "ETH-USD", Sources: names}
		}
		o.Reference = reference.NewAggregator(srcs, reference.Options{MinSources: 2, MaxSpread: big.NewRat(1, 100), Timeout: time.Second, Clock: o.Clock})
	}
}

type renamed struct {
	reference.Source
	name string
}

func (r renamed) Name() string { return r.name }

func TestChaosExchanges(t *testing.T) {
	fx := newFixture(t, withReference(t, 200, 200, 200))
	rs := byKey(fx.eng.RunOnce(context.Background()))
	if r := rs["FW-DEV-001|ethereum|ETH/USD"]; r.Status != checks.OK {
		t.Fatalf("healthy reference: %+v", r)
	}

	fx = newFixture(t, withReference(t, 200, 500, 500))
	rs = byKey(fx.eng.RunOnce(context.Background()))
	if r := rs["FW-REF-001|ethereum|ETH/USD"]; r.Status != checks.Warning {
		t.Fatalf("two of three exchanges down: %+v", r)
	}
	if r := rs["FW-DEV-001|ethereum|ETH/USD"]; r.Status != checks.Unknown {
		t.Fatalf("deviation must not alert on a degraded reference: %+v", r)
	}

	fx = newFixture(t, withReference(t, 500, 503))
	for i := 0; i < 3; i++ {
		rs = byKey(fx.eng.RunOnce(context.Background()))
	}
	if r := rs["FW-SELF-001|ethereum|"]; r.Status != checks.Warning {
		t.Fatalf("all exchanges down for 3 cycles: %+v", r)
	}
}

type hangingNotifier struct{}

func (hangingNotifier) Name() string { return "hang" }
func (hangingNotifier) Send(ctx context.Context, _ alert.Message) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestChaosNotifierDownDoesNotBlock(t *testing.T) {
	ok := &countingNotifier{}
	d := alert.NewDispatcher([]alert.Notifier{hangingNotifier{}, ok}, alert.DispatcherOptions{GroupWindow: time.Millisecond, StormThreshold: 10, SendTimeout: 100 * time.Millisecond, FlushTimeout: 200 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	go d.Run(ctx)
	note := alert.Notification{Kind: alert.KindFiring, Result: checks.Result{CheckID: "FW-STALE-001", Status: checks.Critical}}
	start := time.Now()
	for i := 0; i < 3; i++ {
		d.Enqueue(ctx, []alert.Notification{note})
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-d.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("dispatcher hung on a notifier that never answers")
	}
	ok.mu.Lock()
	defer ok.mu.Unlock()
	if len(ok.msgs) == 0 {
		t.Fatal("the healthy channel must still receive alerts")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("send timeout not applied")
	}
}

func TestChaosSlowRPCBeyondPollInterval(t *testing.T) {
	fx := newFixture(t, func(o *Options) { o.Clock = clock.Real{} })
	for _, nw := range fx.eng.networks {
		nw.net.Clock = clock.Real{}
		nw.cfg.PollInterval = config.Duration{Duration: 300 * time.Millisecond}
	}
	for name, pair := range fx.chains {
		for _, c := range pair {
			c.Set(func(c *fakerpc.Chain) {
				c.HeadAt = time.Now()
				if name == "arbitrum" {
					c.Delay = 200 * time.Millisecond // a cycle needs several sequential calls
				}
			})
		}
	}
	rec := &skipRecorder{}
	fx.eng.o.Recorder = rec
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := fx.eng.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.skipped["arbitrum"] == 0 {
		t.Fatal("overrunning cycles must drop ticks, not queue them")
	}
	if rec.skipped["ethereum"] != 0 {
		t.Fatal("a slow network must not affect the others")
	}
}

type skipRecorder struct {
	nopRecorder
	skipped map[string]int
}

func (s *skipRecorder) ObserveSkipped(n string, k int) {
	if s.skipped == nil {
		s.skipped = map[string]int{}
	}
	s.skipped[n] += k
}
