package chain

import (
	"context"
	"errors"
	"math/big"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/redact"
)

// RetryPolicy bounds retries of transient errors on one endpoint.
type RetryPolicy struct {
	MaxRetries int           // retries after the first attempt
	BaseDelay  time.Duration // delay before the first retry
	MaxDelay   time.Duration // cap of the exponential backoff
}

// DefaultRetryPolicy keeps the worst case of one call well inside a cycle:
// 3 attempts with 200ms and 400ms (plus jitter) between them.
var DefaultRetryPolicy = RetryPolicy{MaxRetries: 2, BaseDelay: 200 * time.Millisecond, MaxDelay: 2 * time.Second}

// backoff returns the delay before retry number attempt (0-based): exponential
// with "full jitter", so that many workers retrying after a provider hiccup
// do not hit it again at the same instant.
func (p RetryPolicy) backoff(attempt int) time.Duration {
	d := p.BaseDelay << attempt
	if d <= 0 || d > p.MaxDelay {
		d = p.MaxDelay
	}
	// Jitter is not security sensitive.
	return d/2 + rand.N(d/2+1) //nolint:gosec // G404: jitter does not need a CSPRNG
}

// BreakerPolicy configures the per-endpoint circuit breaker.
type BreakerPolicy struct {
	FailureThreshold int           // consecutive failed calls that open the circuit
	OpenFor          time.Duration // first rest period
	MaxOpenFor       time.Duration // rest periods double up to this
}

// DefaultBreakerPolicy rests a failing endpoint for 30s, then 60s, ... up to 5m.
var DefaultBreakerPolicy = BreakerPolicy{FailureThreshold: 3, OpenFor: 30 * time.Second, MaxOpenFor: 5 * time.Minute}

// Observer receives per-request measurements. The metrics package implements
// it; labels are endpoint names, never URLs.
type Observer interface {
	ObserveRPC(network, endpoint, method, result string, d time.Duration)
}

type nopObserver struct{}

func (nopObserver) ObserveRPC(string, string, string, string, time.Duration) {}

// Endpoint is one RPC endpoint of a network. It is safe for concurrent use.
type Endpoint struct {
	name     string
	network  string
	backend  Backend
	timeout  time.Duration
	retry    RetryPolicy
	breaker  BreakerPolicy
	clock    clock.Clock
	obs      Observer
	redactor *redact.Redactor
	sleep    func(context.Context, time.Duration) error

	mu        sync.Mutex
	failures  int
	openUntil time.Time
	openFor   time.Duration
	disabled  error
	verified  bool // eth_chainId matched at least once
}

// EndpointOptions configures an Endpoint. Zero values select the defaults.
type EndpointOptions struct {
	Timeout  time.Duration
	Retry    *RetryPolicy
	Breaker  *BreakerPolicy
	Clock    clock.Clock
	Observer Observer
	Redactor *redact.Redactor
	// Sleep replaces the backoff sleep in tests.
	Sleep func(context.Context, time.Duration) error
}

// NewEndpoint wraps a Backend.
func NewEndpoint(network, name string, b Backend, o EndpointOptions) *Endpoint {
	e := &Endpoint{
		name: name, network: network, backend: b,
		timeout: o.Timeout, retry: DefaultRetryPolicy, breaker: DefaultBreakerPolicy,
		clock: o.Clock, obs: o.Observer, redactor: o.Redactor, sleep: o.Sleep,
	}
	if e.timeout <= 0 {
		e.timeout = 10 * time.Second
	}
	if o.Retry != nil {
		e.retry = *o.Retry
	}
	if o.Breaker != nil {
		e.breaker = *o.Breaker
	}
	if e.clock == nil {
		e.clock = clock.Real{}
	}
	if e.obs == nil {
		e.obs = nopObserver{}
	}
	if e.redactor == nil {
		e.redactor = &redact.Redactor{}
	}
	if e.sleep == nil {
		e.sleep = sleepCtx
	}
	return e
}

// Name is the configured endpoint name, safe to log.
func (e *Endpoint) Name() string { return e.name }

// Disabled returns the reason the endpoint was permanently disabled, or nil.
func (e *Endpoint) Disabled() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.disabled
}

// Available reports whether the endpoint may be used now: not disabled and the
// circuit is closed or its rest period has passed (half-open).
func (e *Endpoint) Available() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.disabled == nil && !e.clock.Now().Before(e.openUntil)
}

// Disable excludes the endpoint until restart.
func (e *Endpoint) Disable(reason error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.disabled == nil {
		e.disabled = reason
	}
}

// ChainID calls eth_chainId.
func (e *Endpoint) ChainID(ctx context.Context) (uint64, error) {
	var id *big.Int
	err := e.call(ctx, "eth_chainId", func(ctx context.Context) error {
		var err error
		id, err = e.backend.ChainID(ctx)
		return err
	})
	if err != nil {
		return 0, err
	}
	if !id.IsUint64() {
		return 0, &EndpointError{Endpoint: e.name, Method: "eth_chainId", Err: errors.New("chain id does not fit in 64 bits")}
	}
	return id.Uint64(), nil
}

// VerifyChainID disables the endpoint if it serves another chain. It returns a
// *WrongChainError in that case, and the call error if the check could not be
// made (the endpoint is then left enabled and checked again later).
func (e *Endpoint) VerifyChainID(ctx context.Context, want uint64) error {
	got, err := e.ChainID(ctx)
	if err != nil {
		return err
	}
	if got != want {
		wrong := &WrongChainError{Want: want, Got: got}
		e.Disable(wrong)
		return wrong
	}
	e.mu.Lock()
	e.verified = true
	e.mu.Unlock()
	return nil
}

// Verified reports whether eth_chainId has matched the configured chain at
// least once.
func (e *Endpoint) Verified() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.verified
}

// HeaderByNumber calls eth_getBlockByNumber; nil means the latest block.
func (e *Endpoint) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	var h *types.Header
	err := e.call(ctx, "eth_getBlockByNumber", func(ctx context.Context) error {
		var err error
		h, err = e.backend.HeaderByNumber(ctx, number)
		if err == nil && h == nil {
			err = errors.New("empty header")
		}
		return err
	})
	return h, err
}

// CallContract calls eth_call at blockNumber. It implements Caller.
func (e *Endpoint) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	var out []byte
	err := e.call(ctx, "eth_call", func(ctx context.Context) error {
		var err error
		out, err = e.backend.CallContract(ctx, msg, blockNumber)
		return err
	})
	return out, err
}

// call runs fn with a per-attempt timeout, retries transient errors with
// backoff, and updates the circuit breaker. The returned error is an
// *EndpointError whose message has been redacted.
func (e *Endpoint) call(ctx context.Context, method string, fn func(context.Context) error) error {
	if err := e.admit(); err != nil {
		return &EndpointError{Endpoint: e.name, Method: method, Err: err}
	}
	var err error
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, e.timeout)
		start := e.clock.Now()
		err = fn(actx)
		cancel()
		e.obs.ObserveRPC(e.network, e.name, method, resultLabel(err), e.clock.Now().Sub(start))

		if err == nil {
			e.recordSuccess()
			return nil
		}
		if ctx.Err() != nil {
			// The caller gave up (shutdown or cycle deadline); this says
			// nothing about the endpoint's health.
			return &EndpointError{Endpoint: e.name, Method: method, Err: ctx.Err()}
		}
		if !IsTransient(err) {
			// A JSON-RPC error (for example "execution reverted") is an
			// answer: the endpoint works. Anything else that is not worth
			// retrying (HTTP 401/403 from a revoked key, a malformed
			// response) is a failure of the endpoint.
			if isJSONRPCAnswer(err) {
				e.recordSuccess()
			} else {
				e.recordFailure()
			}
			return &EndpointError{Endpoint: e.name, Method: method, Err: e.redactor.Error(err)}
		}
		if attempt >= e.retry.MaxRetries {
			break
		}
		if serr := e.sleep(ctx, e.retry.backoff(attempt)); serr != nil {
			return &EndpointError{Endpoint: e.name, Method: method, Err: serr}
		}
	}
	e.recordFailure()
	return &EndpointError{Endpoint: e.name, Method: method, Err: e.redactor.Error(err)}
}

func (e *Endpoint) admit() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.disabled != nil {
		return e.disabled
	}
	if e.clock.Now().Before(e.openUntil) {
		return ErrCircuitOpen
	}
	return nil
}

func (e *Endpoint) recordSuccess() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failures = 0
	e.openFor = 0
	e.openUntil = time.Time{}
}

func (e *Endpoint) recordFailure() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failures++
	if e.failures < e.breaker.FailureThreshold {
		return
	}
	switch {
	case e.openFor == 0:
		e.openFor = e.breaker.OpenFor
	case e.openFor*2 > e.breaker.MaxOpenFor:
		e.openFor = e.breaker.MaxOpenFor
	default:
		e.openFor *= 2
	}
	e.openUntil = e.clock.Now().Add(e.openFor)
}

func resultLabel(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case IsTransient(err):
		return "transient_error"
	default:
		return "error"
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
