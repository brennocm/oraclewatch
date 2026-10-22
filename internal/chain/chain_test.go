package chain

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/redact"
)

// fakeBackend scripts responses per call. It is safe for concurrent use.
type fakeBackend struct {
	mu      sync.Mutex
	chainID uint64
	// heads maps block number to header; head is the latest.
	head    uint64
	headers map[uint64]*types.Header
	// errs is consumed one per call (any method) before succeeding.
	errs  []error
	calls int
	call  func(msg ethereum.CallMsg, block *big.Int) ([]byte, error)
}

func newFake(chainID, head uint64, headTime time.Time, extra byte) *fakeBackend {
	f := &fakeBackend{chainID: chainID, head: head, headers: map[uint64]*types.Header{}}
	for n := uint64(0); n <= head; n++ {
		f.headers[n] = &types.Header{
			Number: new(big.Int).SetUint64(n),
			Time:   uint64(headTime.Add(-time.Duration(head-n) * 12 * time.Second).Unix()),
			Extra:  []byte{extra}, // changes the hash, to simulate forks
		}
	}
	return f
}

func (f *fakeBackend) nextErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return err
	}
	return nil
}

func (f *fakeBackend) ChainID(context.Context) (*big.Int, error) {
	if err := f.nextErr(); err != nil {
		return nil, err
	}
	return new(big.Int).SetUint64(f.chainID), nil
}

func (f *fakeBackend) HeaderByNumber(_ context.Context, n *big.Int) (*types.Header, error) {
	if err := f.nextErr(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	num := f.head
	if n != nil {
		num = n.Uint64()
	}
	h, ok := f.headers[num]
	if !ok {
		return nil, errors.New("not found")
	}
	return h, nil
}

func (f *fakeBackend) CallContract(_ context.Context, msg ethereum.CallMsg, b *big.Int) ([]byte, error) {
	if err := f.nextErr(); err != nil {
		return nil, err
	}
	if f.call != nil {
		return f.call(msg, b)
	}
	return []byte{1}, nil
}

func noSleep(context.Context, time.Duration) error { return nil }

func newTestEndpoint(name string, b Backend, clk clock.Clock) *Endpoint {
	return NewEndpoint("testnet", name, b, EndpointOptions{Timeout: time.Second, Clock: clk, Sleep: noSleep})
}

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func TestRetryTransientThenSucceed(t *testing.T) {
	f := newFake(1, 10, t0, 0)
	f.errs = []error{rpc.HTTPError{StatusCode: 429}, rpc.HTTPError{StatusCode: 503}}
	e := newTestEndpoint("a", f, clock.NewFake(t0))
	if _, err := e.CallContract(context.Background(), ethereum.CallMsg{}, nil); err != nil {
		t.Fatalf("want success after 2 retries, got %v", err)
	}
	if f.calls != 3 {
		t.Fatalf("calls = %d, want 3", f.calls)
	}
}

type revertErr struct{}

func (revertErr) Error() string  { return "execution reverted" }
func (revertErr) ErrorCode() int { return 3 }

func TestNoRetryOnRevert(t *testing.T) {
	f := newFake(1, 10, t0, 0)
	f.errs = []error{revertErr{}}
	e := newTestEndpoint("a", f, clock.NewFake(t0))
	_, err := e.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	if err == nil || f.calls != 1 {
		t.Fatalf("err=%v calls=%d, want 1 call and an error", err, f.calls)
	}
	if !e.Available() {
		t.Fatal("a revert is an answer and must not count against the endpoint")
	}
}

func TestBreakerOpensAndRecovers(t *testing.T) {
	clk := clock.NewFake(t0)
	f := newFake(1, 10, t0, 0)
	e := newTestEndpoint("a", f, clk)
	fail := rpc.HTTPError{StatusCode: 500}
	for i := 0; i < DefaultBreakerPolicy.FailureThreshold; i++ {
		f.errs = []error{fail, fail, fail}
		if _, err := e.CallContract(context.Background(), ethereum.CallMsg{}, nil); err == nil {
			t.Fatal("want failure")
		}
	}
	if e.Available() {
		t.Fatal("breaker should be open after consecutive failures")
	}
	before := f.calls
	_, err := e.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	if !errors.Is(err, ErrCircuitOpen) || f.calls != before {
		t.Fatalf("open breaker must reject without calling: err=%v calls=%d->%d", err, before, f.calls)
	}
	clk.Advance(DefaultBreakerPolicy.OpenFor)
	if !e.Available() {
		t.Fatal("breaker should be half-open after the rest period")
	}
	if _, err := e.CallContract(context.Background(), ethereum.CallMsg{}, nil); err != nil {
		t.Fatalf("half-open call should go through: %v", err)
	}
	if !e.Available() {
		t.Fatal("success should close the breaker")
	}
}

func TestBreakerBackoffDoubles(t *testing.T) {
	clk := clock.NewFake(t0)
	f := newFake(1, 10, t0, 0)
	e := newTestEndpoint("a", f, clk)
	fail := rpc.HTTPError{StatusCode: 500}
	failOnce := func() {
		f.errs = []error{fail, fail, fail}
		_, _ = e.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	}
	for i := 0; i < DefaultBreakerPolicy.FailureThreshold; i++ {
		failOnce()
	}
	clk.Advance(DefaultBreakerPolicy.OpenFor)
	failOnce() // half-open probe fails
	clk.Advance(DefaultBreakerPolicy.OpenFor)
	if e.Available() {
		t.Fatal("second rest period should be twice as long")
	}
	clk.Advance(DefaultBreakerPolicy.OpenFor)
	if !e.Available() {
		t.Fatal("endpoint should be available after the doubled rest period")
	}
}

func TestParentCancellationDoesNotPenalise(t *testing.T) {
	f := newFake(1, 10, t0, 0)
	e := newTestEndpoint("a", f, clock.NewFake(t0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 5; i++ {
		f.errs = []error{context.Canceled}
		_, _ = e.CallContract(ctx, ethereum.CallMsg{}, nil)
	}
	if !e.Available() {
		t.Fatal("shutdown must not open the breaker")
	}
}

func TestErrorsAreRedacted(t *testing.T) {
	secretURL := "https://rpc.example.com/v2/very-secret-api-key-123456"
	r := &redact.Redactor{}
	r.Add(secretURL)
	f := newFake(1, 10, t0, 0)
	f.errs = []error{&url.Error{Op: "Post", URL: secretURL, Err: errors.New("connection reset")}}
	f.errs = append(f.errs, f.errs[0], f.errs[0])
	e := NewEndpoint("testnet", "a", f, EndpointOptions{Clock: clock.NewFake(t0), Sleep: noSleep, Redactor: r})
	_, err := e.CallContract(context.Background(), ethereum.CallMsg{}, nil)
	if err == nil || strings.Contains(err.Error(), "very-secret-api-key") {
		t.Fatalf("error leaks the URL: %v", err)
	}
	if !strings.Contains(err.Error(), "endpoint a") {
		t.Fatalf("error should name the endpoint: %v", err)
	}
}

func TestVerifyChainIDDisablesWrongChain(t *testing.T) {
	clk := clock.NewFake(t0)
	good := newTestEndpoint("good", newFake(1, 10, t0, 0), clk)
	bad := newTestEndpoint("bad", newFake(56, 10, t0, 0), clk)
	n := &Network{Name: "ethereum", ChainID: 1, Endpoints: []*Endpoint{good, bad}, Clock: clk}
	res := n.VerifyChainIDs(context.Background())
	var wrong *WrongChainError
	if len(res) != 1 || !errors.As(res["bad"], &wrong) || wrong.Got != 56 {
		t.Fatalf("results = %v", res)
	}
	if bad.Available() || !good.Available() {
		t.Fatal("only the wrong-chain endpoint should be disabled")
	}
	if got := n.AvailableEndpoints(); len(got) != 1 || got[0] != good {
		t.Fatalf("available = %v", got)
	}
}

func TestSnapshotPinsMinHeadMinusConfirmations(t *testing.T) {
	clk := clock.NewFake(t0)
	a := newTestEndpoint("a", newFake(1, 100, t0, 0), clk)
	b := newTestEndpoint("b", newFake(1, 98, t0.Add(-24*time.Second), 0), clk)
	n := &Network{Name: "ethereum", ChainID: 1, Confirmations: 2, MaxHeadLag: 5 * time.Minute, Endpoints: []*Endpoint{a, b}, Clock: clk}
	snap, err := n.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Block != 96 {
		t.Fatalf("pinned %d, want min(100,98)-2 = 96", snap.Block)
	}
	if len(snap.Readers) != 2 || len(snap.Forked) != 0 {
		t.Fatalf("readers=%d forked=%v", len(snap.Readers), snap.Forked)
	}
}

func TestSnapshotExcludesLaggingEndpoint(t *testing.T) {
	clk := clock.NewFake(t0)
	a := newTestEndpoint("a", newFake(1, 2000, t0, 0), clk)
	// b is 1,000 blocks (3h20m) behind.
	b := newTestEndpoint("b", newFake(1, 1000, t0.Add(-200*time.Minute), 0), clk)
	n := &Network{Name: "ethereum", ChainID: 1, Confirmations: 2, MaxHeadLag: 5 * time.Minute, Endpoints: []*Endpoint{a, b}, Clock: clk}
	snap, err := n.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Block != 1998 || len(snap.Readers) != 1 || snap.Readers[0] != a {
		t.Fatalf("block=%d readers=%v", snap.Block, snap.Readers)
	}
	if !snap.Heads[1].Lagging {
		t.Fatal("b should be reported as lagging")
	}
}

func TestSnapshotAllLaggingIsStalled(t *testing.T) {
	clk := clock.NewFake(t0)
	old := t0.Add(-time.Hour)
	n := &Network{Name: "arbitrum", ChainID: 1, MaxHeadLag: 2 * time.Minute, Clock: clk, Endpoints: []*Endpoint{
		newTestEndpoint("a", newFake(1, 50, old, 0), clk),
		newTestEndpoint("b", newFake(1, 50, old, 0), clk),
	}}
	if _, err := n.Snapshot(context.Background()); !errors.Is(err, ErrChainStalled) {
		t.Fatalf("err = %v, want ErrChainStalled", err)
	}
}

func TestSnapshotDetectsForkedEndpoint(t *testing.T) {
	clk := clock.NewFake(t0)
	n := &Network{Name: "ethereum", ChainID: 1, Confirmations: 0, MaxHeadLag: 5 * time.Minute, Clock: clk, Endpoints: []*Endpoint{
		newTestEndpoint("a", newFake(1, 10, t0, 0), clk),
		newTestEndpoint("b", newFake(1, 10, t0, 0), clk),
		newTestEndpoint("c", newFake(1, 10, t0, 9), clk), // different block contents
	}}
	snap, err := n.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Readers) != 2 || len(snap.Forked) != 1 || snap.Forked[0].Endpoint != "c" {
		t.Fatalf("readers=%d forked=%+v", len(snap.Readers), snap.Forked)
	}
	if snap.BlockHash == (common.Hash{}) || snap.BlockTime.IsZero() {
		t.Fatal("snapshot hash and time must be set")
	}
}

func TestSnapshotNoEndpoints(t *testing.T) {
	clk := clock.NewFake(t0)
	e := newTestEndpoint("a", newFake(1, 10, t0, 0), clk)
	e.Disable(errors.New("test"))
	n := &Network{Name: "x", Endpoints: []*Endpoint{e}, Clock: clk, MaxHeadLag: time.Minute}
	if _, err := n.Snapshot(context.Background()); !errors.Is(err, ErrNoHealthyEndpoint) {
		t.Fatalf("err = %v", err)
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{context.DeadlineExceeded, true},
		{rpc.HTTPError{StatusCode: 429}, true},
		{rpc.HTTPError{StatusCode: 502}, true},
		{rpc.HTTPError{StatusCode: 401}, false},
		{revertErr{}, false},
		// Connection failures are transient: retried, and counted by the breaker.
		{&url.Error{Op: "Post", URL: "x", Err: errors.New("dial tcp: connection refused")}, true},
		{errors.New("unknown"), false},
	}
	for _, tt := range tests {
		if got := IsTransient(tt.err); got != tt.want {
			t.Errorf("IsTransient(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestBackoffBounds(t *testing.T) {
	p := DefaultRetryPolicy
	for attempt := 0; attempt < 20; attempt++ {
		d := p.backoff(attempt)
		if d <= 0 || d > p.MaxDelay {
			t.Fatalf("backoff(%d) = %s out of (0, %s]", attempt, d, p.MaxDelay)
		}
	}
}

func TestHugeRPCResponseIsRejected(t *testing.T) {
	huge := strings.Repeat("a", MaxResponseSize+1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"` + huge + `"}`))
	}))
	defer srv.Close()
	b, err := DialHTTP(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ChainID(context.Background()); err == nil {
		t.Fatal("a response larger than MaxResponseSize must fail")
	}
}
