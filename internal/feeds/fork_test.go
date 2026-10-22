//go:build fork

// Fork tests run against an Anvil fork of Ethereum mainnet:
//
//	MAINNET_RPC_URL=https://... make test-fork
//
// They need the anvil binary and an RPC with recent state, so they are not
// part of the default CI run.
package feeds_test

import (
	"context"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/checks"
	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/feeds"
)

// Verified in configs/feeds/ethereum.yaml.
var (
	ethUSDProxy      = common.HexToAddress("0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419")
	ethUSDAggregator = common.HexToAddress("0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5")
)

func startAnvil(t *testing.T) string {
	t.Helper()
	fork := os.Getenv("MAINNET_RPC_URL")
	if fork == "" {
		t.Skip("MAINNET_RPC_URL not set")
	}
	bin, err := exec.LookPath("anvil")
	if err != nil {
		t.Skip("anvil not installed")
	}
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "--fork-url", fork, "--port", fmt.Sprint(port), "--silent")
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		c, err := rpc.DialContext(ctx, url)
		if err == nil {
			var id string
			if c.CallContext(ctx, &id, "eth_chainId") == nil {
				c.Close()
				return url
			}
			c.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("anvil did not start")
	return ""
}

func TestForkReadsRealFeedAndDetectsStaleness(t *testing.T) {
	ctx := context.Background()
	url := startAnvil(t)
	b, err := chain.DialHTTP(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	ep := chain.NewEndpoint("fork", "anvil", b, chain.EndpointOptions{Timeout: 30 * time.Second})
	head, err := ep.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	md, err := feeds.ReadMetadata(ctx, ep, ethUSDProxy, head.Number)
	if err != nil {
		t.Fatal(err)
	}
	if md.Decimals != 8 || md.Description != "ETH / USD" {
		t.Fatalf("metadata = %+v", md)
	}
	if md.Aggregator != ethUSDAggregator {
		t.Logf("note: aggregator changed since verification: %s", md.Aggregator.Hex())
	}
	bounds, err := feeds.ReadBounds(ctx, ep, md.Aggregator, head.Number)
	if err != nil || bounds.Min.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("bounds = %+v %v", bounds, err)
	}

	// No oracle transmits on the fork: advance two hours, mine, and the real
	// feed must be stale at the new block.
	c, err := rpc.DialContext(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.CallContext(ctx, nil, "evm_increaseTime", 2*3600); err != nil {
		t.Fatal(err)
	}
	if err := c.CallContext(ctx, nil, "evm_mine"); err != nil {
		t.Fatal(err)
	}
	later, err := ep.HeaderByNumber(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	blockTime := time.Unix(int64(later.Time), 0)
	r, err := feeds.ReadLatest(ctx, ep, "anvil", ethUSDProxy, md.Decimals, later.Number.Uint64(), blockTime)
	if err != nil {
		t.Fatal(err)
	}
	in := &checks.FeedInput{
		Network: "ethereum", Feed: "ETH/USD", Params: config.DefaultCheckParams(),
		Heartbeat: time.Hour, Block: later.Number.Uint64(), BlockTime: blockTime, Now: blockTime, Reading: &r,
	}
	if res := (checks.Stale{}).Evaluate(in); res.Status < checks.Warning {
		t.Fatalf("real feed two hours after the fork point must be stale: %+v", res)
	}
	if res := (checks.Invalid{}).Evaluate(in); res.Status != checks.OK {
		t.Fatalf("real round must be well-formed: %+v", res)
	}
}
