//go:build integration

// Package feedstest deploys the Solidity test doubles from testdata/contracts
// on the go-ethereum simulated backend, for integration tests.
//
// It is compiled only with the "integration" build tag. The key it uses is
// generated in memory for the simulated chain and discarded with it; it never
// exists outside a test process. The oraclewatch binary has no signing code.
package feedstest

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/simulated"

	"github.com/brennocm/oraclewatch/internal/feeds/feedstest/mockaggregator"
	"github.com/brennocm/oraclewatch/internal/feeds/feedstest/mockproxy"
)

// Chain is a simulated chain with a funded throwaway deployer.
type Chain struct {
	t       testing.TB
	Backend *simulated.Backend
	Client  simulated.Client
	key     *ecdsa.PrivateKey
	opts    *bind.TransactOpts
}

// NewChain starts a simulated chain. It is closed when the test ends.
func NewChain(t testing.TB) *Chain {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	balance, _ := new(big.Int).SetString("1000000000000000000000", 10)
	b := simulated.NewBackend(types.GenesisAlloc{addr: {Balance: balance}})
	t.Cleanup(func() { _ = b.Close() })
	c := &Chain{t: t, Backend: b, Client: b.Client(), key: key}
	chainID, err := c.Client.ChainID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.opts = bind.NewKeyedTransactor(key, chainID)
	return c
}

// Feed is a deployed MockProxy in front of a MockAggregator.
type Feed struct {
	c          *Chain
	Proxy      common.Address
	Aggregator common.Address
	agg        *mockaggregator.MockAggregator
	proxy      *mockproxy.MockProxy
}

// AggregatorParams configure a MockAggregator.
type AggregatorParams struct {
	Decimals       uint8
	Description    string
	MinAnswer      *big.Int
	MaxAnswer      *big.Int
	TypeAndVersion string
}

// DeployFeed deploys an aggregator and a proxy pointing at it.
func (c *Chain) DeployFeed(p AggregatorParams) *Feed {
	c.t.Helper()
	agg := c.DeployAggregator(p)
	proxyBin := common.FromHex(mockproxy.MockProxyMetaData.Bin)
	pb := mockproxy.NewMockProxy()
	proxyAddr, _, err := bind.DeployContract(c.opts, proxyBin, c.Client, pb.PackConstructor(agg))
	if err != nil {
		c.t.Fatal(err)
	}
	c.Backend.Commit()
	return &Feed{c: c, Proxy: proxyAddr, Aggregator: agg, agg: mockaggregator.NewMockAggregator(), proxy: pb}
}

// DeployAggregator deploys a standalone MockAggregator.
func (c *Chain) DeployAggregator(p AggregatorParams) common.Address {
	c.t.Helper()
	ab := mockaggregator.NewMockAggregator()
	bin := common.FromHex(mockaggregator.MockAggregatorMetaData.Bin)
	addr, _, err := bind.DeployContract(c.opts, bin, c.Client, ab.PackConstructor(p.Decimals, p.Description, p.MinAnswer, p.MaxAnswer, p.TypeAndVersion))
	if err != nil {
		c.t.Fatal(err)
	}
	c.Backend.Commit()
	return addr
}

func (c *Chain) transact(to common.Address, data []byte) {
	c.t.Helper()
	// data is already ABI-packed, so the bound contract needs no ABI.
	bc := bind.NewBoundContract(to, abi.ABI{}, c.Client, c.Client, c.Client)
	if _, err := bind.Transact(bc, c.opts, data); err != nil {
		c.t.Fatal(err)
	}
	c.Backend.Commit()
}

// SetRound sets the latest round on the feed's current aggregator.
func (f *Feed) SetRound(aggRoundID uint64, answer *big.Int, startedAt, updatedAt uint64) {
	f.c.t.Helper()
	f.c.transact(f.Aggregator, f.agg.PackSetRound(aggRoundID, answer, new(big.Int).SetUint64(startedAt), new(big.Int).SetUint64(updatedAt)))
}

// SetDecimals changes decimals() of the current aggregator.
func (f *Feed) SetDecimals(d uint8) { f.c.transact(f.Aggregator, f.agg.PackSetDecimals(d)) }

// SetDescription changes description() of the current aggregator.
func (f *Feed) SetDescription(s string) { f.c.transact(f.Aggregator, f.agg.PackSetDescription(s)) }

// SwitchAggregator points the proxy at agg, starting a new phase.
func (f *Feed) SwitchAggregator(agg common.Address) {
	f.c.t.Helper()
	f.c.transact(f.Proxy, f.proxy.PackSetAggregator(agg))
	f.Aggregator = agg
}

// Head returns the latest block number and time.
func (c *Chain) Head() (uint64, time.Time) {
	c.t.Helper()
	h, err := c.Client.HeaderByNumber(context.Background(), nil)
	if err != nil {
		c.t.Fatal(err)
	}
	return h.Number.Uint64(), time.Unix(int64(h.Time), 0) //nolint:gosec // G115: test chain timestamps are small
}
