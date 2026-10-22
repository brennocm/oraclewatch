package checks

import (
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/brennocm/oraclewatch/internal/chain"
	"github.com/brennocm/oraclewatch/internal/feeds"
)

func netInput() *NetworkInput {
	return &NetworkInput{
		Network: "arbitrum", Now: blockTime, MaxHeadLag: 2 * time.Minute, GracePeriod: time.Hour, BlindAfter: 3,
		Snapshot: &chain.Snapshot{Block: 100, BlockTime: blockTime, Heads: []chain.Head{
			{Endpoint: "a", Number: 110, Time: blockTime},
			{Endpoint: "b", Number: 110, Time: blockTime},
		}},
	}
}

func TestChainHead(t *testing.T) {
	in := netInput()
	if got := (ChainHead{}).Evaluate(in); got.Status != OK {
		t.Fatal(got.Summary)
	}
	in.Snapshot.Heads[1].Lagging = true
	if got := (ChainHead{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("lagging: %s", got.Status)
	}
	in.SnapshotErr = chain.ErrChainStalled
	if got := (ChainHead{}).Evaluate(in); got.Status != Critical {
		t.Fatalf("stalled: %s", got.Status)
	}
}

func TestRPCForkAndWrongChain(t *testing.T) {
	in := netInput()
	if got := (RPCFork{}).Evaluate(in); got.Status != OK {
		t.Fatal(got.Summary)
	}
	in.Snapshot.Forked = []chain.ForkedEndpoint{{Endpoint: "b", Hash: common.HexToHash("0x01")}}
	if got := (RPCFork{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("fork: %s", got.Status)
	}
	if got := (WrongChain{}).Evaluate(in); got.Status != OK {
		t.Fatal(got.Summary)
	}
	in.WrongChain = map[string]*chain.WrongChainError{"b": {Want: 42161, Got: 56}}
	if got := (WrongChain{}).Evaluate(in); got.Status != Critical {
		t.Fatalf("wrong chain: %s", got.Status)
	}
}

func seq(answer, startedAt int64) *feeds.Round {
	return &feeds.Round{RoundID: big.NewInt(1), Answer: big.NewInt(answer), StartedAt: big.NewInt(startedAt), UpdatedAt: big.NewInt(startedAt), AnsweredInRound: big.NewInt(1)}
}

func TestSequencer(t *testing.T) {
	bt := blockTime.Unix()
	tests := []struct {
		name                string
		round               *feeds.Round
		wantDown, wantGrace Status
	}{
		{"up for days", seq(0, bt-5*86400), OK, OK},
		{"down", seq(1, bt-60), Critical, OK},
		{"recovered 10 min ago", seq(0, bt-600), OK, Warning},
		{"recovered exactly grace ago", seq(0, bt-3600), OK, Warning}, // <= as in the official example
		{"recovered grace+1s ago", seq(0, bt-3601), OK, OK},
		{"not initialised (Arbitrum)", seq(0, 0), Critical, OK},
		{"invalid answer", seq(2, bt-60), Critical, OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := netInput()
			in.HasSequencer, in.Sequencer = true, tt.round
			if got := (SequencerDown{}).Evaluate(in); got.Status != tt.wantDown {
				t.Errorf("FW-SEQ-001 = %s (%s)", got.Status, got.Summary)
			}
			if got := (SequencerGrace{}).Evaluate(in); got.Status != tt.wantGrace {
				t.Errorf("FW-SEQ-002 = %s (%s)", got.Status, got.Summary)
			}
		})
	}
	in := netInput()
	if got := (SequencerDown{}).Evaluate(in); got.Status != OK {
		t.Fatal("L1 without sequencer feed must be OK")
	}
	in.HasSequencer = true
	if got := (SequencerDown{}).Evaluate(in); got.Status != Unknown {
		t.Fatal("unread sequencer feed must be Unknown")
	}
}

func TestSelfHealth(t *testing.T) {
	in := netInput()
	if got := (SelfHealth{}).Evaluate(in); got.Status != OK {
		t.Fatal(got.Summary)
	}
	in.ReferenceFailedCycles = 3
	if got := (SelfHealth{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("reference blind: %s", got.Status)
	}
	in.FailedCycles = 2
	in.SnapshotErr = chain.ErrNoHealthyEndpoint
	if got := (SelfHealth{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("below threshold: %s", got.Status)
	}
	in.FailedCycles = 3
	if got := (SelfHealth{}).Evaluate(in); got.Status != Critical {
		t.Fatalf("blind: %s", got.Status)
	}
}

func TestCrossChain(t *testing.T) {
	e := func(n, p, thr string, ok bool) CrossChainEntry {
		return CrossChainEntry{Network: n, Price: r(p), DeviationThreshold: r(thr), Usable: ok}
	}
	// limit for 0.5% + 0.05% + 1% = 1.55%; low price 2000 → 31.
	in := &CrossChainInput{Pair: "ETH/USD", Margin: r("0.01"), Entries: []CrossChainEntry{e("ethereum", "2000", "0.005", true), e("arbitrum", "2031", "0.0005", true)}}
	if got := CrossChain(in); got.Status != OK {
		t.Fatalf("exactly at limit: %s (%s)", got.Status, got.Summary)
	}
	in.Entries[1].Price = r("2031.00000001")
	if got := CrossChain(in); got.Status != Warning {
		t.Fatalf("above limit: %s", got.Status)
	}
	in.Entries[1].Usable = false
	if got := CrossChain(in); got.Status != Unknown {
		t.Fatalf("one usable network: %s", got.Status)
	}
}

func TestCatalogCoversEveryCheck(t *testing.T) {
	ids := map[string]bool{}
	for _, m := range Catalog {
		if ids[m.ID] {
			t.Fatalf("duplicate %s", m.ID)
		}
		ids[m.ID] = true
	}
	n := 1 // cross-chain
	for _, c := range FeedChecks() {
		if !ids[c.ID()] {
			t.Errorf("%s missing from Catalog", c.ID())
		}
		n++
	}
	for _, c := range NetworkChecks() {
		if !ids[c.ID()] {
			t.Errorf("%s missing from Catalog", c.ID())
		}
		n++
	}
	if n != len(Catalog) {
		t.Errorf("Catalog has %d entries, implementations %d", len(Catalog), n)
	}
}

func TestEveryCheckIsDocumented(t *testing.T) {
	for _, m := range Catalog {
		path := filepath.Join("..", "..", "docs", "checks", m.ID+".md")
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", m.ID, err)
			continue
		}
		if !strings.HasPrefix(string(b), "# "+m.ID+": "+m.Title) {
			t.Errorf("%s: title does not match the catalog (%q)", path, m.Title)
		}
	}
}
