// Package fakerpc is an in-memory EVM chain that answers the subset of
// JSON-RPC oraclewatch uses (eth_chainId, eth_getBlockByNumber, eth_call for
// the feed ABIs). It can be used directly as a chain.Backend or served over
// HTTP for end-to-end tests. Test-only: the binary must not import it.
package fakerpc

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/brennocm/oraclewatch/internal/feeds"
)

// Feed is the state of one proxy and its aggregator.
type Feed struct {
	Decimals       uint8
	Description    string
	Aggregator     common.Address
	Phase          uint16
	Round          *big.Int
	Answer         *big.Int
	StartedAt      int64
	UpdatedAt      int64
	Min, Max       *big.Int
	TypeAndVersion string
}

// Chain is the fake chain. All fields are guarded by Set.
type Chain struct {
	mu     sync.Mutex
	ID     uint64 // chain ID served by eth_chainId
	Head   uint64
	HeadAt time.Time
	Delay  time.Duration
	Fail   bool
	// Garbage makes eth_call return bytes that are not valid ABI output.
	Garbage bool
	// Revert lists methods (by ABI name) whose eth_call reverts.
	Revert map[string]bool
	Feeds  map[common.Address]*Feed
}

// New returns a chain at block 1000 with head time headAt.
func New(chainID uint64, headAt time.Time) *Chain {
	return &Chain{ID: chainID, Head: 1000, HeadAt: headAt, Feeds: map[common.Address]*Feed{}}
}

// Set mutates the chain under its lock.
func (c *Chain) Set(fn func(c *Chain)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fn(c)
}

func (c *Chain) wait(ctx context.Context) error {
	c.mu.Lock()
	d, fail := c.Delay, c.Fail
	c.mu.Unlock()
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

// ChainID implements chain.Backend.
func (c *Chain) ChainID(ctx context.Context) (*big.Int, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return new(big.Int).SetUint64(c.ID), nil
}

// HeaderByNumber implements chain.Backend. Blocks are 2 s apart.
func (c *Chain) HeaderByNumber(ctx context.Context, n *big.Int) (*types.Header, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	num := c.Head
	if n != nil && n.Sign() >= 0 {
		num = n.Uint64()
	}
	if num > c.Head {
		return nil, errors.New("header not found")
	}
	ts := c.HeadAt.Add(-time.Duration(c.Head-num) * 2 * time.Second)                                                   //nolint:gosec // G115: num <= Head, and test chains are short
	return &types.Header{Number: new(big.Int).SetUint64(num), Time: uint64(ts.Unix()), Difficulty: big.NewInt(0)}, nil //nolint:gosec // test timestamps are positive
}

// CallContract implements chain.Backend.
func (c *Chain) CallContract(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	if msg.To == nil || len(msg.Data) < 4 {
		return nil, errors.New("invalid call")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Garbage {
		return []byte{0xde, 0xad, 0xbe, 0xef}, nil
	}
	var f *Feed
	for addr, fd := range c.Feeds {
		if addr == *msg.To || fd.Aggregator == *msg.To {
			f = fd
		}
	}
	if f == nil {
		return nil, nil // no code at the address
	}
	for _, a := range feeds.ABIs() {
		m, err := a.MethodById(msg.Data[:4])
		if err != nil {
			continue
		}
		if c.Revert[m.Name] {
			return nil, errors.New("execution reverted")
		}
		var vals []any
		switch m.Name {
		case "latestRoundData":
			vals = []any{f.Round, f.Answer, big.NewInt(f.StartedAt), big.NewInt(f.UpdatedAt), f.Round}
		case "decimals":
			vals = []any{f.Decimals}
		case "description":
			vals = []any{f.Description}
		case "version":
			vals = []any{big.NewInt(6)}
		case "aggregator":
			vals = []any{f.Aggregator}
		case "phaseId":
			vals = []any{f.Phase}
		case "minAnswer":
			vals = []any{f.Min}
		case "maxAnswer":
			vals = []any{f.Max}
		case "typeAndVersion":
			vals = []any{f.TypeAndVersion}
		default:
			return nil, errors.New("unsupported method " + m.Name)
		}
		return m.Outputs.Pack(vals...)
	}
	return nil, errors.New("execution reverted")
}

// Handler serves the chain as JSON-RPC over HTTP.
func (c *Chain) Handler() http.Handler {
	srv := rpc.NewServer()
	if err := srv.RegisterName("eth", &ethAPI{c}); err != nil {
		panic(err)
	}
	return srv
}

type ethAPI struct{ c *Chain }

// ChainId serves eth_chainId.
func (a *ethAPI) ChainId(ctx context.Context) (*hexutil.Big, error) {
	id, err := a.c.ChainID(ctx)
	return (*hexutil.Big)(id), err
}

// GetBlockByNumber serves eth_getBlockByNumber.
func (a *ethAPI) GetBlockByNumber(ctx context.Context, n rpc.BlockNumber, _ bool) (*types.Header, error) {
	var num *big.Int
	if n >= 0 {
		num = big.NewInt(n.Int64())
	}
	return a.c.HeaderByNumber(ctx, num)
}

type callArgs struct {
	To   *common.Address `json:"to"`
	Data hexutil.Bytes   `json:"input"`
	// Older clients send "data".
	DataOld hexutil.Bytes `json:"data"`
}

// Call serves eth_call.
func (a *ethAPI) Call(ctx context.Context, args callArgs, _ rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	data := args.Data
	if len(data) == 0 {
		data = args.DataOld
	}
	return a.c.CallContract(ctx, ethereum.CallMsg{To: args.To, Data: data}, nil)
}
