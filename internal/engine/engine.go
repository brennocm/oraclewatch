// Package engine runs the monitoring cycles: one worker goroutine per
// network, each reading every feed of its network at the network's pinned
// block, fetching market references, evaluating the checks and handing the
// results to the alert manager.
//
// Concurrency model: Run starts one goroutine per network (plus an optional
// dead man's switch) under an errgroup. A worker runs its cycles one after
// another on a time.Ticker; a cycle that takes longer than poll_interval
// causes the missed ticks to be dropped and counted, never queued. Workers
// share only the alert manager, the reference aggregator and the status
// store, each protected by its own mutex.
package engine

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/brennocm/oraclewatch/internal/alert"
	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/feeds"
	"github.com/brennocm/oraclewatch/internal/reference"
)

// Recorder receives values for metrics. float64 conversions for Prometheus
// happen in the implementation, never in a decision path.
type Recorder interface {
	ObserveFeed(network, feed string, r *feeds.Reading, age time.Duration)
	ObserveDeviation(network, feed string, dev, reference *big.Rat)
	ObserveCheck(r checks.Result)
	ObserveSequencer(network string, up bool)
	ObserveCycle(network string, d time.Duration, ok bool)
	ObserveSkipped(network string, n int)
	ObserveHead(network, endpoint string, number uint64)
}

type nopRecorder struct{}

func (nopRecorder) ObserveFeed(string, string, *feeds.Reading, time.Duration) {}
func (nopRecorder) ObserveDeviation(string, string, *big.Rat, *big.Rat)       {}
func (nopRecorder) ObserveCheck(checks.Result)                                {}
func (nopRecorder) ObserveSequencer(string, bool)                             {}
func (nopRecorder) ObserveCycle(string, time.Duration, bool)                  {}
func (nopRecorder) ObserveSkipped(string, int)                                {}
func (nopRecorder) ObserveHead(string, string, uint64)                        {}

// Options configure an Engine.
type Options struct {
	Config    *config.Config
	Networks  []*chain.Network // one per cfg.Networks entry, same order
	Reference *reference.Aggregator
	// Alerts and Dispatcher are used by Run; RunOnce ignores them.
	Alerts     *alert.Manager
	Dispatcher *alert.Dispatcher
	Clock      clock.Clock
	Logger     *slog.Logger
	Recorder   Recorder
	// Heartbeat, if set, is called after cycles while every network is
	// healthy, to feed an external dead man's switch.
	Heartbeat func(ctx context.Context) error
	// ShutdownGrace bounds how long an in-flight cycle may continue after
	// Run's context is cancelled.
	ShutdownGrace time.Duration
}

// Engine is the monitoring loop.
type Engine struct {
	o        Options
	networks []*networkState
	xchain   *crossChainStore

	once bool // set by RunOnce

	mu     sync.RWMutex
	latest map[string]checks.Result
	health map[string]NetworkHealth
}

// NetworkHealth is the per-network summary served by /readyz and /status.
type NetworkHealth struct {
	Network     string    `json:"network"`
	Healthy     bool      `json:"healthy"`
	LastSuccess time.Time `json:"last_success"`
	Block       uint64    `json:"block"`
	Endpoints   int       `json:"healthy_endpoints"`
	Cycles      int       `json:"cycles"`
}

type feedState struct {
	cfg       *config.Feed
	prev      *feeds.Reading
	metadata  *feeds.Metadata
	bounds    *feeds.Bounds
	boundsFor *feeds.Metadata // metadata the bounds were read for
}

type networkState struct {
	cfg         *config.Network
	net         *chain.Network
	feeds       []*feedState
	cycle       int
	failed      int
	refFailed   int
	wrong       map[string]*chain.WrongChainError
	lastSuccess time.Time
}

// New validates the options and prepares per-network state.
func New(o Options) (*Engine, error) {
	if o.Config == nil || len(o.Networks) != len(o.Config.Networks) {
		return nil, errors.New("engine: one chain.Network per configured network is required")
	}
	if o.Clock == nil {
		o.Clock = clock.Real{}
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Recorder == nil {
		o.Recorder = nopRecorder{}
	}
	if o.ShutdownGrace <= 0 {
		o.ShutdownGrace = 10 * time.Second
	}
	e := &Engine{o: o, latest: map[string]checks.Result{}, health: map[string]NetworkHealth{}, xchain: newCrossChainStore()}
	for i := range o.Config.Networks {
		cn := &o.Config.Networks[i]
		ns := &networkState{cfg: cn, net: o.Networks[i], wrong: map[string]*chain.WrongChainError{}}
		for j := range o.Config.Feeds {
			if o.Config.Feeds[j].Network == cn.Name {
				ns.feeds = append(ns.feeds, &feedState{cfg: &o.Config.Feeds[j]})
			}
		}
		e.networks = append(e.networks, ns)
	}
	return e, nil
}

// Run monitors until ctx is cancelled. New cycles stop being scheduled at
// cancellation; a cycle already running gets ShutdownGrace to finish so that
// its alerts are not lost.
func (e *Engine) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, ns := range e.networks {
		g.Go(func() error {
			e.worker(gctx, ns)
			return nil
		})
	}
	if e.o.Heartbeat != nil {
		g.Go(func() error {
			e.heartbeat(gctx)
			return nil
		})
	}
	return g.Wait()
}

func (e *Engine) worker(ctx context.Context, ns *networkState) {
	interval := ns.cfg.PollInterval.Duration
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		e.runCycle(ctx, ns)

		// Ticks that fired while the cycle ran are dropped, not queued.
		skipped := 0
	drain:
		for {
			select {
			case <-ticker.C:
				skipped++
			default:
				break drain
			}
		}
		if skipped > 0 {
			e.o.Recorder.ObserveSkipped(ns.cfg.Name, skipped)
			e.o.Logger.Warn("cycle overran poll_interval; skipped ticks", "network", ns.cfg.Name, "skipped", skipped)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runCycle runs one cycle with its own deadline. Its context is detached from
// ctx so that shutdown does not abort it immediately, and cancelled
// ShutdownGrace after ctx is.
func (e *Engine) runCycle(ctx context.Context, ns *networkState) {
	if ctx.Err() != nil {
		return
	}
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ns.cfg.PollInterval.Duration)
	defer cancel()
	stop := context.AfterFunc(ctx, func() {
		t := time.AfterFunc(e.o.ShutdownGrace, cancel)
		context.AfterFunc(cctx, func() { t.Stop() })
	})
	defer stop()

	results := e.cycle(cctx, ns)
	xc := e.crossChain(ns.cfg.Name)
	e.storeResults(xc)
	results = append(results, xc...)
	if e.o.Alerts != nil {
		notes := e.o.Alerts.Process(results, e.o.Clock.Now())
		if e.o.Dispatcher != nil {
			e.o.Dispatcher.Enqueue(cctx, notes)
		}
	}
}

// RunOnce runs a single cycle on every network concurrently and returns all
// results, including cross-chain ones. It is the `check` command. With a
// single cycle there is no point waiting blind_after_cycles: a network that
// could not be read at all makes FW-SELF-001 critical immediately, so a blind
// CI run fails instead of passing on Unknown results.
func (e *Engine) RunOnce(ctx context.Context) []checks.Result {
	e.once = true
	var mu sync.Mutex
	var all []checks.Result
	var wg sync.WaitGroup
	for _, ns := range e.networks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, ns.cfg.PollInterval.Duration)
			defer cancel()
			r := e.cycle(cctx, ns)
			mu.Lock()
			all = append(all, r...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	xc := e.crossChain("")
	e.storeResults(xc)
	all = append(all, xc...)
	sortResults(all)
	return all
}

// Results returns the latest result of every check, for /status.
func (e *Engine) Results() []checks.Result {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]checks.Result, 0, len(e.latest))
	for _, r := range e.latest {
		out = append(out, r)
	}
	sortResults(out)
	return out
}

// Health returns per-network health, sorted by network.
func (e *Engine) Health() []NetworkHealth {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]NetworkHealth, 0, len(e.health))
	for _, h := range e.health {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Network < out[j].Network })
	return out
}

// readyCycles is how many poll intervals may pass after a network's last
// successful cycle before the engine stops being ready.
const readyCycles = 3

// Ready reports whether every network's last cycle succeeded and that cycle
// is recent. The age matters as much as the outcome: a worker that stopped
// completing cycles would otherwise keep its last healthy state, and the
// dead man's switch, which pings only while Ready, would never fall silent.
func (e *Engine) Ready() bool {
	now := e.o.Clock.Now()
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, ns := range e.networks {
		h, ok := e.health[ns.cfg.Name]
		if !ok || !h.Healthy || now.Sub(h.LastSuccess) > readyCycles*ns.cfg.PollInterval.Duration {
			return false
		}
	}
	return true
}

// storeResults records results that are not tied to one network's cycle
// (cross-chain) in metrics and the status store.
func (e *Engine) storeResults(rs []checks.Result) {
	for _, r := range rs {
		e.o.Recorder.ObserveCheck(r)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range rs {
		e.latest[r.Key()] = r
	}
}

func (e *Engine) store(ns *networkState, results []checks.Result, snap *chain.Snapshot, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range results {
		e.latest[r.Key()] = r
	}
	h := NetworkHealth{Network: ns.cfg.Name, Healthy: ok, LastSuccess: ns.lastSuccess, Cycles: ns.cycle, Endpoints: len(ns.net.AvailableEndpoints())}
	if snap != nil {
		h.Block = snap.Block
	}
	e.health[ns.cfg.Name] = h
}

// heartbeat pings the external dead man's switch while every network is
// healthy, at the shortest poll interval.
func (e *Engine) heartbeat(ctx context.Context) {
	interval := e.networks[0].cfg.PollInterval.Duration
	for _, ns := range e.networks[1:] {
		interval = min(interval, ns.cfg.PollInterval.Duration)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if !e.Ready() {
			continue // silence is the signal
		}
		hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := e.o.Heartbeat(hctx); err != nil {
			e.o.Logger.Warn("dead man's switch ping failed", "err", err)
		}
		cancel()
	}
}

func sortResults(rs []checks.Result) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].Key() < rs[j].Key() })
}
