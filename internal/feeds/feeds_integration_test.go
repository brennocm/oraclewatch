//go:build integration

package feeds_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/clock"
	"github.com/brennocm/oraclewatch/internal/feeds"
	"github.com/brennocm/oraclewatch/internal/feeds/feedstest"
)

var maxAnswer, _ = new(big.Int).SetString("95780971304118053647396689196894323976171195136475135", 10)

func setup(t *testing.T) (*feedstest.Chain, *feedstest.Feed, *chain.Endpoint) {
	t.Helper()
	c := feedstest.NewChain(t)
	f := c.DeployFeed(feedstest.AggregatorParams{
		Decimals: 8, Description: "ETH / USD", MinAnswer: big.NewInt(1), MaxAnswer: maxAnswer,
		TypeAndVersion: "AccessControlledOCR2Aggregator 1.0.0",
	})
	ep := chain.NewEndpoint("sim", "sim", c.Client, chain.EndpointOptions{Clock: clock.Real{}})
	return c, f, ep
}

func blockOf(t *testing.T, c *feedstest.Chain) *big.Int {
	t.Helper()
	n, _ := c.Head()
	return new(big.Int).SetUint64(n)
}

func TestReadLatestAndMetadata(t *testing.T) {
	ctx := context.Background()
	c, f, ep := setup(t)
	f.SetRound(42, big.NewInt(244363658508), 1792635525, 1792635539)
	n, bt := c.Head()

	r, err := feeds.ReadLatest(ctx, ep, ep.Name(), f.Proxy, 8, n, bt)
	if err != nil {
		t.Fatal(err)
	}
	if r.Price.FloatString(8) != "2443.63658508" {
		t.Errorf("price = %s", r.Price.FloatString(8))
	}
	if r.Round.PhaseID() != 1 || r.Round.AggregatorRoundID() != 42 {
		t.Errorf("phase=%d aggRound=%d", r.Round.PhaseID(), r.Round.AggregatorRoundID())
	}
	if r.Round.AnsweredInRound.Cmp(r.Round.RoundID) != 0 {
		t.Error("answeredInRound should equal roundId, as on OCR2")
	}

	md, err := feeds.ReadMetadata(ctx, ep, f.Proxy, big.NewInt(int64(n)))
	if err != nil {
		t.Fatal(err)
	}
	if md.Decimals != 8 || md.Description != "ETH / USD" || md.Version.Int64() != 6 || md.Aggregator != f.Aggregator || md.PhaseID != 1 {
		t.Errorf("metadata = %+v", md)
	}

	b, err := feeds.ReadBounds(ctx, ep, md.Aggregator, big.NewInt(int64(n)))
	if err != nil {
		t.Fatal(err)
	}
	if b.Min.Int64() != 1 || b.Max.Cmp(maxAnswer) != 0 || b.TypeAndVersion != "AccessControlledOCR2Aggregator 1.0.0" {
		t.Errorf("bounds = %+v", b)
	}
}

// TestReadsArePinnedToTheBlock is the property the whole design relies on: a
// read at block N returns the state at N even after later updates.
func TestReadsArePinnedToTheBlock(t *testing.T) {
	ctx := context.Background()
	c, f, ep := setup(t)
	f.SetRound(1, big.NewInt(100_00000000), 1000, 1000)
	pinned := blockOf(t, c)
	f.SetRound(2, big.NewInt(200_00000000), 2000, 2000)
	f.SetDecimals(18)

	r, err := feeds.LatestRound(ctx, ep, f.Proxy, pinned)
	if err != nil {
		t.Fatal(err)
	}
	if r.Answer.Cmp(big.NewInt(100_00000000)) != 0 || r.UpdatedAt.Int64() != 1000 {
		t.Fatalf("read at pinned block returned later state: %+v", r)
	}
	md, err := feeds.ReadMetadata(ctx, ep, f.Proxy, pinned)
	if err != nil || md.Decimals != 8 {
		t.Fatalf("metadata at pinned block: %+v %v", md, err)
	}
	latest, err := feeds.LatestRound(ctx, ep, f.Proxy, blockOf(t, c))
	if err != nil || latest.Answer.Cmp(big.NewInt(200_00000000)) != 0 {
		t.Fatalf("latest read: %+v %v", latest, err)
	}
}

func TestAggregatorSwitchStartsNewPhase(t *testing.T) {
	ctx := context.Background()
	c, f, ep := setup(t)
	f.SetRound(7, big.NewInt(1), 1, 1)
	before, err := feeds.ReadMetadata(ctx, ep, f.Proxy, blockOf(t, c))
	if err != nil {
		t.Fatal(err)
	}
	next := c.DeployAggregator(feedstest.AggregatorParams{Decimals: 8, Description: "ETH / USD", MinAnswer: big.NewInt(1), MaxAnswer: maxAnswer, TypeAndVersion: "DualAggregator 1.0.0"})
	f.SwitchAggregator(next)
	f.SetRound(1, big.NewInt(2), 2, 2)

	after, err := feeds.ReadMetadata(ctx, ep, f.Proxy, blockOf(t, c))
	if err != nil {
		t.Fatal(err)
	}
	diff := feeds.Diff(before, after)
	if len(diff) != 2 || after.PhaseID != 2 || after.Aggregator != next {
		t.Fatalf("diff = %+v after = %+v", diff, after)
	}
	r, err := feeds.LatestRound(ctx, ep, f.Proxy, blockOf(t, c))
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(2), 64), big.NewInt(1))
	if r.RoundID.Cmp(want) != 0 {
		t.Fatalf("roundId = %s, want (2<<64)|1 = %s", r.RoundID, want)
	}
	// Phase 1 history stays readable through the proxy.
	old := new(big.Int).Or(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(7))
	h, err := feeds.RoundData(ctx, ep, f.Proxy, blockOf(t, c), old)
	if err != nil || h.Answer.Int64() != 1 {
		t.Fatalf("history: %+v %v", h, err)
	}
}

func TestUnknownRoundReturnsZerosWithoutRevert(t *testing.T) {
	ctx := context.Background()
	c, f, ep := setup(t)
	f.SetRound(5, big.NewInt(1), 1, 1)
	r, err := feeds.RoundData(ctx, ep, f.Proxy, blockOf(t, c), new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		t.Fatal(err)
	}
	if r.UpdatedAt.Sign() != 0 || r.Answer.Sign() != 0 {
		t.Fatalf("want zero round, got %+v", r)
	}
}

func TestNoContractIsEmptyResult(t *testing.T) {
	ctx := context.Background()
	c, f, ep := setup(t)
	// The aggregator address before deployment had no code: read the proxy at
	// block 0, when it did not exist.
	_ = c
	if _, err := feeds.LatestRound(ctx, ep, f.Proxy, big.NewInt(0)); !errors.Is(err, feeds.ErrEmptyResult) {
		t.Fatalf("err = %v, want ErrEmptyResult", err)
	}
}

func TestSnapshotOnSimulatedChain(t *testing.T) {
	ctx := context.Background()
	c, _, _ := setup(t)
	for i := 0; i < 5; i++ {
		c.Backend.Commit()
	}
	head, _ := c.Head()
	ep := chain.NewEndpoint("sim", "a", c.Client, chain.EndpointOptions{})
	ep2 := chain.NewEndpoint("sim", "b", c.Client, chain.EndpointOptions{})
	n := &chain.Network{Name: "sim", ChainID: 1337, Confirmations: 2, MaxHeadLag: 24 * time.Hour, Endpoints: []*chain.Endpoint{ep, ep2}, Clock: clock.Real{}}
	if res := n.VerifyChainIDs(ctx); len(res) != 0 {
		t.Fatalf("chain id check: %v", res)
	}
	snap, err := n.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Block != head-2 || len(snap.Readers) != 2 {
		t.Fatalf("block=%d head=%d readers=%d", snap.Block, head, len(snap.Readers))
	}
}
