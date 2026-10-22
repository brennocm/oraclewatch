package engine

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/feeds"
)

// cycle performs one monitoring cycle for a network and returns its results.
//
//  1. Every chain_id_recheck cycles: verify eth_chainId of every endpoint.
//  2. Snapshot: pin one block (min head - confirmations) agreed by hash.
//  3. Read the sequencer feed and every feed at that block, concurrently.
//  4. Evaluate feed checks, then network checks.
func (e *Engine) cycle(ctx context.Context, ns *networkState) []checks.Result {
	start := e.o.Clock.Now()
	cfg := e.o.Config
	if ns.cycle%cfg.RPC.ChainIDRecheck == 0 {
		for name, err := range ns.net.VerifyChainIDs(ctx) {
			var wrong *chain.WrongChainError
			if errors.As(err, &wrong) {
				ns.wrong[name] = wrong
				e.o.Logger.Error("endpoint serves the wrong chain; disabled", "network", ns.cfg.Name, "endpoint", name, "got", wrong.Got, "want", wrong.Want)
			} else {
				e.o.Logger.Warn("chain id check failed", "network", ns.cfg.Name, "endpoint", name, "err", err)
			}
		}
	}

	snap, serr := ns.net.Snapshot(ctx)
	now := e.o.Clock.Now()
	if snap != nil {
		for _, h := range snap.Heads {
			if h.Err == nil {
				e.o.Recorder.ObserveHead(ns.cfg.Name, h.Endpoint, h.Number)
			}
		}
	}
	if serr != nil {
		e.o.Logger.Warn("snapshot failed", "network", ns.cfg.Name, "err", serr)
	}

	nin := &checks.NetworkInput{
		Network: ns.cfg.Name, Now: now, MaxHeadLag: ns.cfg.MaxHeadLag.Duration,
		Snapshot: snap, SnapshotErr: serr, WrongChain: ns.wrong,
		HasSequencer: ns.cfg.Sequencer != nil, BlindAfter: cfg.SelfHealth.BlindAfterCycles,
	}
	if ns.cfg.Sequencer != nil {
		nin.GracePeriod = ns.cfg.Sequencer.GracePeriod.Duration
	}

	inputs := make([]*checks.FeedInput, len(ns.feeds))
	if serr == nil {
		var wg sync.WaitGroup
		if ns.cfg.Sequencer != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := firstOK(snap.Readers, func(c *chain.Endpoint) (feeds.Round, error) {
					return feeds.LatestRound(ctx, c, ns.cfg.Sequencer.Proxy.Address, snap.BlockNumber())
				})
				if err == nil {
					nin.Sequencer = &r
					e.o.Recorder.ObserveSequencer(ns.cfg.Name, r.Answer.Sign() == 0)
				}
				nin.SequencerErr = err
			}()
		}
		for i, fs := range ns.feeds {
			wg.Add(1)
			go func() {
				defer wg.Done()
				inputs[i] = e.feedInput(ctx, ns, fs, snap, now)
			}()
		}
		wg.Wait()
	} else {
		stalled := errors.Is(serr, chain.ErrChainStalled)
		for i, fs := range ns.feeds {
			inputs[i] = e.baseInput(ns, fs, now)
			inputs[i].ChainStalled = stalled
			inputs[i].ReadErr = serr
		}
	}

	var results []checks.Result
	anyRead, anyRef, wantRef := false, false, false
	for i, in := range inputs {
		fs := ns.feeds[i]
		rs := checks.EvaluateFeed(in)
		results = append(results, rs...)
		if in.Reading != nil {
			anyRead = true
			fs.prev = in.Reading
			e.o.Recorder.ObserveFeed(ns.cfg.Name, fs.cfg.Name, in.Reading, in.BlockTime.Sub(feeds.TimeOf(in.Reading.Round.UpdatedAt)))
			if in.Reference != nil && in.Reference.Trusted {
				e.o.Recorder.ObserveDeviation(ns.cfg.Name, fs.cfg.Name, checks.DeviationRatio(in.Reading.Price, in.Reference.Median), in.Reference.Median)
			}
		}
		if in.Reference != nil {
			wantRef = true
			if len(in.Reference.Quotes) > 0 {
				anyRef = true
			}
		}
		e.xchain.put(fs.cfg.Name, ns.cfg.Name, in, statusOf(rs, "FW-STALE-001"))
	}

	ns.cycle++
	if anyRead {
		ns.failed = 0
		ns.lastSuccess = now
	} else {
		ns.failed++
	}
	if !wantRef || anyRef {
		ns.refFailed = 0
	} else {
		ns.refFailed++
	}
	nin.FailedCycles, nin.ReferenceFailedCycles = ns.failed, ns.refFailed
	if e.once {
		nin.BlindAfter = 1
	}
	for _, c := range checks.NetworkChecks() {
		results = append(results, c.Evaluate(nin))
	}

	for _, r := range results {
		e.o.Recorder.ObserveCheck(r)
	}
	e.o.Recorder.ObserveCycle(ns.cfg.Name, e.o.Clock.Now().Sub(start), anyRead)
	e.store(ns, results, snap, anyRead && serr == nil)
	e.o.Logger.Debug("cycle done", "network", ns.cfg.Name, "block", blockOf(snap), "results", len(results), "read", anyRead)
	return results
}

// baseInput fills the static part of a feed's input.
func (e *Engine) baseInput(ns *networkState, fs *feedState, now time.Time) *checks.FeedInput {
	f := fs.cfg
	return &checks.FeedInput{
		Network: ns.cfg.Name, Feed: f.Name, Params: f.Checks,
		Heartbeat: f.Heartbeat.Duration, DeviationThreshold: f.DeviationThreshold.Rat(),
		ExpectedDecimals: *f.ExpectedDecimals, ExpectedDescription: f.ExpectedDescription,
		Now: now, Prev: fs.prev, Metadata: fs.metadata, PrevMetadata: fs.metadata, Bounds: fs.bounds,
	}
}

// feedInput reads one feed at the snapshot's block.
func (e *Engine) feedInput(ctx context.Context, ns *networkState, fs *feedState, snap *chain.Snapshot, now time.Time) *checks.FeedInput {
	in := e.baseInput(ns, fs, now)
	in.Block, in.BlockTime = snap.Block, snap.BlockTime
	block := snap.BlockNumber()
	proxy := fs.cfg.Proxy.Address

	// Metadata: on the first cycle and every metadata_refresh cycles.
	if fs.metadata == nil || ns.cycle%e.o.Config.RPC.MetadataRefresh == 0 {
		md, err := firstOK(snap.Readers, func(c *chain.Endpoint) (feeds.Metadata, error) {
			return feeds.ReadMetadata(ctx, c, proxy, block)
		})
		if err != nil {
			e.o.Logger.Warn("metadata read failed", "network", ns.cfg.Name, "feed", fs.cfg.Name, "err", err)
		} else {
			in.PrevMetadata = fs.metadata
			fs.metadata = &md
			in.Metadata = &md
		}
	}
	// Bounds: whenever the aggregator differs from the one they were read for.
	if md := fs.metadata; md != nil && (fs.bounds == nil || fs.boundsFor == nil || fs.boundsFor.Aggregator != md.Aggregator) {
		b, err := firstOK(snap.Readers, func(c *chain.Endpoint) (feeds.Bounds, error) {
			return feeds.ReadBounds(ctx, c, md.Aggregator, block)
		})
		if err != nil {
			e.o.Logger.Warn("bounds read failed", "network", ns.cfg.Name, "feed", fs.cfg.Name, "err", err)
		} else {
			fs.bounds, fs.boundsFor = &b, md
			in.Bounds = &b
		}
	}

	// latestRoundData on every reader (or the first, if comparison is off).
	readers := snap.Readers
	if !e.o.Config.RPC.CompareEndpoints {
		readers = readers[:1]
	}
	reads := make([]checks.EndpointRead, len(readers))
	var wg sync.WaitGroup
	for i, ep := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := feeds.LatestRound(ctx, ep, proxy, block)
			reads[i] = checks.EndpointRead{Endpoint: ep.Name(), Err: err}
			if err == nil {
				reads[i].Round = &r
			}
		}()
	}
	wg.Wait()
	if e.o.Config.RPC.CompareEndpoints {
		in.Reads = reads
	}
	decimals := in.ExpectedDecimals
	if fs.metadata != nil {
		decimals = fs.metadata.Decimals // normalise with the on-chain value
	}
	for _, r := range reads {
		if r.Err == nil {
			in.Reading = &feeds.Reading{
				Proxy: proxy, Round: *r.Round, Decimals: decimals, Price: feeds.Normalize(r.Round.Answer, decimals),
				Block: snap.Block, BlockTime: snap.BlockTime, Endpoint: r.Endpoint,
			}
			break
		}
		in.ReadErr = r.Err
	}
	if in.Reading == nil && in.ReadErr != nil {
		e.o.Logger.Warn("feed read failed", "network", ns.cfg.Name, "feed", fs.cfg.Name, "err", in.ReadErr)
	}

	if ref := fs.cfg.Reference; ref != nil && e.o.Reference != nil {
		sources := ref.Sources
		if sources == nil {
			sources = e.o.Config.Reference.Sources
		}
		r := e.o.Reference.Get(ctx, ref.Pair, sources)
		in.Reference = &r
	}
	return in
}

// firstOK runs fn on each endpoint in order until one succeeds.
func firstOK[T any](eps []*chain.Endpoint, fn func(*chain.Endpoint) (T, error)) (T, error) {
	var zero T
	err := chain.ErrNoHealthyEndpoint
	for _, ep := range eps {
		v, e := fn(ep)
		if e == nil {
			return v, nil
		}
		err = e
	}
	return zero, err
}

func statusOf(rs []checks.Result, id string) checks.Status {
	for _, r := range rs {
		if r.CheckID == id {
			return r.Status
		}
	}
	return checks.Unknown
}

func blockOf(s *chain.Snapshot) uint64 {
	if s == nil {
		return 0
	}
	return s.Block
}

// --- cross-chain ---

type xchainEntry struct {
	price     *big.Rat
	threshold *big.Rat
	usable    bool
	at        time.Time
}

// crossChainStore keeps the latest answer of every feed name on every
// network so that FW-XCHAIN-001 can compare networks read by different
// workers.
type crossChainStore struct {
	mu sync.Mutex
	m  map[string]map[string]xchainEntry // feed name -> network -> entry
}

func newCrossChainStore() *crossChainStore {
	return &crossChainStore{m: map[string]map[string]xchainEntry{}}
}

func (s *crossChainStore) put(feed, network string, in *checks.FeedInput, stale checks.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[feed] == nil {
		s.m[feed] = map[string]xchainEntry{}
	}
	e := xchainEntry{threshold: in.DeviationThreshold, at: in.Now}
	if in.Reading != nil {
		e.price = in.Reading.Price
		e.usable = !in.ChainStalled && stale < checks.Warning && stale != checks.Unknown
	}
	s.m[feed][network] = e
}

// crossChain evaluates FW-XCHAIN-001 for the feed names present on network
// (all names if network is ""). Entries older than three of their network's
// poll intervals are treated as unusable.
func (e *Engine) crossChain(network string) []checks.Result {
	if !e.o.Config.CrossChain.Enabled {
		return nil
	}
	now := e.o.Clock.Now()
	maxAge := map[string]time.Duration{}
	for _, ns := range e.networks {
		maxAge[ns.cfg.Name] = 3 * ns.cfg.PollInterval.Duration
	}
	e.xchain.mu.Lock()
	defer e.xchain.mu.Unlock()
	var out []checks.Result
	for feed, byNet := range e.xchain.m {
		if network != "" {
			if _, ok := byNet[network]; !ok {
				continue
			}
		}
		if len(byNet) < 2 {
			continue
		}
		in := &checks.CrossChainInput{Pair: feed, Margin: e.o.Config.CrossChain.Margin.Rat(), Now: now}
		for n, x := range byNet {
			in.Entries = append(in.Entries, checks.CrossChainEntry{
				Network: n, Price: x.price, DeviationThreshold: x.threshold,
				Usable: x.usable && now.Sub(x.at) <= maxAge[n],
			})
		}
		out = append(out, checks.CrossChain(in))
	}
	return out
}
