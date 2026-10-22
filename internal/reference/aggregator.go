package reference

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/redact"
)

// Observer receives per-source results for metrics.
type Observer interface {
	ObserveQuote(source, pair string, q *Quote, err error)
}

type nopObserver struct{}

func (nopObserver) ObserveQuote(string, string, *Quote, error) {}

// Reference is the combined market price for a pair at one moment.
type Reference struct {
	Pair    string
	Median  *big.Rat // nil when no source answered
	Spread  *big.Rat // (max - min) / median of the accepted quotes
	Quotes  []Quote  // accepted quotes, sorted by price
	Errors  map[string]string
	Trusted bool
	// Reason explains why the reference is not trusted.
	Reason string
	At     time.Time
}

// Options configure an Aggregator.
type Options struct {
	MinSources int
	MaxSpread  *big.Rat      // ratio, e.g. 1/100
	MaxAge     time.Duration // quotes with an exchange time older than this are dropped
	Timeout    time.Duration // per source request
	CacheTTL   time.Duration // reuse a source's quote for this long
	// FutureSkew is how far in the future an exchange timestamp may be
	// before the quote is rejected.
	FutureSkew time.Duration
	Clock      clock.Clock
	Observer   Observer
	Redactor   *redact.Redactor
}

// Aggregator fetches quotes from several sources and combines them. It is
// safe for concurrent use; concurrent requests for the same source and pair
// share one HTTP request through the cache.
type Aggregator struct {
	sources map[string]Source
	o       Options

	mu      sync.Mutex
	entries map[string]*cacheEntry // key: source + "|" + pair
	backoff map[string]time.Time   // source → do not call before
	locks   map[string]*sync.Mutex // per key, serialises fetches
}

type cacheEntry struct {
	quote   Quote
	err     error
	fetched time.Time
}

// defaultRateLimitBackoff applies when a 429 carries no Retry-After.
const defaultRateLimitBackoff = time.Minute

// NewAggregator combines the given sources.
func NewAggregator(sources []Source, o Options) *Aggregator {
	if o.Clock == nil {
		o.Clock = clock.Real{}
	}
	if o.Observer == nil {
		o.Observer = nopObserver{}
	}
	if o.Redactor == nil {
		o.Redactor = &redact.Redactor{}
	}
	if o.FutureSkew == 0 {
		o.FutureSkew = time.Minute
	}
	a := &Aggregator{
		sources: map[string]Source{},
		o:       o,
		entries: map[string]*cacheEntry{},
		backoff: map[string]time.Time{},
		locks:   map[string]*sync.Mutex{},
	}
	for _, s := range sources {
		a.sources[s.Name()] = s
	}
	return a
}

// Get fetches pair from the named sources concurrently and returns the
// combined reference. It never returns an error: an untrusted Reference with
// a Reason is the result when sources fail.
func (a *Aggregator) Get(ctx context.Context, pair string, sourceNames []string) Reference {
	now := a.o.Clock.Now()
	ref := Reference{Pair: pair, Errors: map[string]string{}, At: now}

	type result struct {
		name  string
		quote Quote
		err   error
	}
	results := make([]result, len(sourceNames))
	var wg sync.WaitGroup
	for i, name := range sourceNames {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q, err := a.quote(ctx, name, pair)
			results[i] = result{name, q, err}
		}()
	}
	wg.Wait()

	for _, r := range results {
		if r.err == nil {
			r.err = a.validate(r.quote, now)
		}
		if r.err != nil {
			ref.Errors[r.name] = a.o.Redactor.Error(r.err).Error()
			continue
		}
		ref.Quotes = append(ref.Quotes, r.quote)
	}
	sort.Slice(ref.Quotes, func(i, j int) bool { return ref.Quotes[i].Price.Cmp(ref.Quotes[j].Price) < 0 })

	if len(ref.Quotes) == 0 {
		ref.Reason = "no source answered"
		return ref
	}
	ref.Median = median(ref.Quotes)
	lo, hi := ref.Quotes[0].Price, ref.Quotes[len(ref.Quotes)-1].Price
	ref.Spread = new(big.Rat).Sub(hi, lo)
	ref.Spread.Quo(ref.Spread, ref.Median)

	switch {
	case len(ref.Quotes) < a.o.MinSources:
		ref.Reason = fmt.Sprintf("%d of %d required sources answered", len(ref.Quotes), a.o.MinSources)
	case a.o.MaxSpread != nil && ref.Spread.Cmp(a.o.MaxSpread) > 0:
		ref.Reason = fmt.Sprintf("sources disagree: spread %s%% exceeds %s%%", pct(ref.Spread), pct(a.o.MaxSpread))
	default:
		ref.Trusted = true
	}
	return ref
}

// validate rejects quotes that are too old or claim to be from the future.
func (a *Aggregator) validate(q Quote, now time.Time) error {
	if !q.HasExchangeTime {
		return nil
	}
	if a.o.MaxAge > 0 && now.Sub(q.Time) > a.o.MaxAge {
		return fmt.Errorf("%w: quote is %s old", ErrInvalidQuote, now.Sub(q.Time).Truncate(time.Second))
	}
	if q.Time.Sub(now) > a.o.FutureSkew {
		return fmt.Errorf("%w: quote timestamp is %s in the future", ErrInvalidQuote, q.Time.Sub(now).Truncate(time.Second))
	}
	return nil
}

// quote returns a cached quote or fetches one. Fetches for the same key are
// serialised so that concurrent callers wait for one request.
func (a *Aggregator) quote(ctx context.Context, name, pair string) (Quote, error) {
	src, ok := a.sources[name]
	if !ok {
		return Quote{}, fmt.Errorf("unknown source %q", name)
	}
	key := name + "|" + pair

	a.mu.Lock()
	lock, ok := a.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		a.locks[key] = lock
	}
	a.mu.Unlock()

	lock.Lock()
	defer lock.Unlock()

	now := a.o.Clock.Now()
	a.mu.Lock()
	if e, ok := a.entries[key]; ok && now.Sub(e.fetched) < a.o.CacheTTL {
		a.mu.Unlock()
		return e.quote, e.err
	}
	if until, ok := a.backoff[name]; ok && now.Before(until) {
		a.mu.Unlock()
		return Quote{}, &RateLimitError{RetryAfter: until.Sub(now)}
	}
	a.mu.Unlock()

	fctx, cancel := context.WithTimeout(ctx, a.o.Timeout)
	q, err := src.Quote(fctx, pair)
	cancel()
	if ctx.Err() != nil {
		// The caller gave up; this result says nothing about the source.
		return Quote{}, ctx.Err()
	}
	a.o.Observer.ObserveQuote(name, pair, quotePtr(q, err), err)

	a.mu.Lock()
	defer a.mu.Unlock()
	var rl *RateLimitError
	if errors.As(err, &rl) {
		wait := rl.RetryAfter
		if wait <= 0 {
			wait = defaultRateLimitBackoff
		}
		a.backoff[name] = now.Add(wait)
	}
	// Errors are cached too, so a failing source is not hammered within the TTL.
	a.entries[key] = &cacheEntry{quote: q, err: err, fetched: now}
	return q, err
}

func quotePtr(q Quote, err error) *Quote {
	if err != nil {
		return nil
	}
	return &q
}

// median of quotes sorted by price; the mean of the two middle values for an
// even count. The median, not the mean, is used so that one wrong source
// cannot move the reference by itself when three or more answer.
func median(sorted []Quote) *big.Rat {
	n := len(sorted)
	if n%2 == 1 {
		return new(big.Rat).Set(sorted[n/2].Price)
	}
	m := new(big.Rat).Add(sorted[n/2-1].Price, sorted[n/2].Price)
	return m.Quo(m, big.NewRat(2, 1))
}

func pct(r *big.Rat) string {
	return new(big.Rat).Mul(r, big.NewRat(100, 1)).FloatString(3)
}
