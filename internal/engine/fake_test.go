package engine

import (
	"context"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// fakeChain is a JSON-RPC backend that answers the feed ABI from in-memory
// state. It returns properly ABI-encoded data, so the real decoding path in
// internal/feeds is exercised.
type fakeChain struct {
	mu      sync.Mutex
	chainID uint64
	head    uint64
	headAt  time.Time
	delay   time.Duration
	feeds   map[common.Address]*fakeFeed
	fail    bool
	abis    []abi.ABI
}

type fakeFeed struct {
	decimals    uint8
	description string
	aggregator  common.Address
	phase       uint16
	round       *big.Int
	answer      *big.Int
	startedAt   int64
	updatedAt   int64
	min, max    *big.Int
	typeVersion string
}

func loadABIs(t testing.TB) []abi.ABI {
	t.Helper()
	var out []abi.ABI
	for _, f := range []string{"AggregatorV3Interface", "AggregatorProxy", "AggregatorBounds"} {
		b, err := os.Open(filepath.Join("..", "feeds", "abi", f+".abi.json"))
		if err != nil {
			t.Fatal(err)
		}
		a, err := abi.JSON(b)
		_ = b.Close()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func newFakeChain(t testing.TB, chainID uint64, headAt time.Time) *fakeChain {
	return &fakeChain{chainID: chainID, head: 1000, headAt: headAt, feeds: map[common.Address]*fakeFeed{}, abis: loadABIs(t)}
}

func (f *fakeChain) wait(ctx context.Context) error {
	f.mu.Lock()
	d, fail := f.delay, f.fail
	f.mu.Unlock()
	if d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	if fail {
		return errors.New("connection refused")
	}
	return nil
}

func (f *fakeChain) ChainID(ctx context.Context) (*big.Int, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	return new(big.Int).SetUint64(f.chainID), nil
}

func (f *fakeChain) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	num := f.head
	if n != nil {
		num = n.Uint64()
	}
	ts := f.headAt.Add(-time.Duration(f.head-num) * 2 * time.Second)
	return &types.Header{Number: new(big.Int).SetUint64(num), Time: uint64(ts.Unix()), Difficulty: big.NewInt(0)}, nil
}

func (f *fakeChain) CallContract(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var feed *fakeFeed
	for addr, fd := range f.feeds {
		if addr == *msg.To || fd.aggregator == *msg.To {
			feed = fd
		}
	}
	if feed == nil {
		return nil, nil // no contract
	}
	for _, a := range f.abis {
		m, err := a.MethodById(msg.Data[:4])
		if err != nil {
			continue
		}
		var vals []any
		switch m.Name {
		case "latestRoundData":
			vals = []any{feed.round, feed.answer, big.NewInt(feed.startedAt), big.NewInt(feed.updatedAt), feed.round}
		case "decimals":
			vals = []any{feed.decimals}
		case "description":
			vals = []any{feed.description}
		case "version":
			vals = []any{big.NewInt(6)}
		case "aggregator":
			vals = []any{feed.aggregator}
		case "phaseId":
			vals = []any{feed.phase}
		case "minAnswer":
			vals = []any{feed.min}
		case "maxAnswer":
			vals = []any{feed.max}
		case "typeAndVersion":
			vals = []any{feed.typeVersion}
		default:
			return nil, errors.New("unsupported method " + m.Name)
		}
		return m.Outputs.Pack(vals...)
	}
	return nil, errors.New("unknown selector")
}

func (f *fakeChain) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}
