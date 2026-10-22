package checks

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/brennocm/oraclewatch/internal/feeds"
	"github.com/brennocm/oraclewatch/internal/reference"
)

func r(s string) *big.Rat {
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return v
}

func bi(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic(s)
	}
	return v
}

// priced returns an input whose oracle price is answer/1e8.
func priced(answer string) *FeedInput {
	in := input(blockTime.Unix()-60, time.Hour)
	in.Reading.Round.Answer = bi(answer)
	in.Reading.Decimals = 8
	in.Reading.Price = feeds.Normalize(in.Reading.Round.Answer, 8)
	in.DeviationThreshold = r("0.005")
	return in
}

func trustedRef(median string) *reference.Reference {
	return &reference.Reference{Pair: "ETH-USD", Median: r(median), Spread: r("0.0001"), Trusted: true,
		Quotes: []reference.Quote{{Source: "coinbase"}, {Source: "kraken"}}}
}

func TestDeviationBoundaries(t *testing.T) {
	// reference 2000; warn above 2% (40), critical above 5% (100).
	tests := []struct {
		answer string // 8 decimals
		want   Status
	}{
		{"200000000000", OK},       // 2000.00000000
		{"204000000000", OK},       // exactly +2%
		{"204000000001", Warning},  // +2% + 1e-8
		{"196000000000", OK},       // exactly -2%
		{"195999999999", Warning},  // -2% - 1e-8
		{"210000000000", Warning},  // exactly +5%
		{"210000000001", Critical}, // just above
		{"10700000", Critical},     // LUNA-like: 0.107 vs 2000
	}
	for _, tt := range tests {
		in := priced(tt.answer)
		in.Reference = trustedRef("2000")
		if got := (Deviation{}).Evaluate(in); got.Status != tt.want {
			t.Errorf("answer %s: %s (%s), want %s", tt.answer, got.Status, got.Summary, tt.want)
		}
	}
}

func TestDeviationUnknownWithoutTrustedReference(t *testing.T) {
	in := priced("100000000000")
	if got := (Deviation{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("nil reference: %s", got.Status)
	}
	in.Reference = trustedRef("1")
	in.Reference.Trusted = false
	if got := (Deviation{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("untrusted reference must not alert: %s", got.Status)
	}
	in.Reference.Trusted = true
	in.ChainStalled = true
	if got := (Deviation{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("stalled chain: %s", got.Status)
	}
}

func TestReferenceDegraded(t *testing.T) {
	in := priced("1")
	if got := (ReferenceDegraded{}).Evaluate(in); got.Status != OK {
		t.Fatal("no reference configured is not a problem")
	}
	in.Reference = &reference.Reference{Pair: "ETH-USD", Reason: "1 of 2 required sources answered", Errors: map[string]string{"kraken": "HTTP 500"}}
	got := (ReferenceDegraded{}).Evaluate(in)
	if got.Status != Warning || !strings.Contains(got.Details["errors"], "kraken") {
		t.Fatalf("%+v", got)
	}
	in.Reference = trustedRef("1")
	if got := (ReferenceDegraded{}).Evaluate(in); got.Status != OK {
		t.Fatalf("trusted: %s", got.Status)
	}
}

const ocr2 = "AccessControlledOCR2Aggregator 1.0.0"

var ethMax = "95780971304118053647396689196894323976171195136475135"

func TestCircuitBreaker(t *testing.T) {
	// USDC/USD: minAnswer 1 (nominal), maxAnswer 105000000 ($1.05), as on-chain.
	usdc := func(answer string) *FeedInput {
		in := priced(answer)
		in.Bounds = &feeds.Bounds{Min: big.NewInt(1), Max: big.NewInt(105000000), TypeAndVersion: ocr2}
		return in
	}
	tests := []struct {
		name  string
		in    *FeedInput
		want  Status
		prior map[string]Status
	}{
		{"USDC at peg is 5% from max", usdc("100000000"), OK, nil},
		// dist = (105000000 - a) / a; warn below 2%: a > 102941176.47
		{"USDC just outside warn", usdc("102941176"), OK, nil},
		{"USDC inside warn", usdc("102941177"), Warning, nil},
		// critical below 0.5%: a > 104477611.94
		{"USDC just outside critical", usdc("104477611"), Warning, nil},
		{"USDC inside critical", usdc("104477612"), Critical, nil},
		{"USDC at the bound", usdc("105000000"), Critical, nil},
		{"near bound and stale is frozen", usdc("103000000"), Critical, map[string]Status{"FW-STALE-001": Warning}},
		{"near bound and deviating is frozen", usdc("103000000"), Critical, map[string]Status{"FW-DEV-001": Critical}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Prior = tt.prior
			if got := (CircuitBreaker{}).Evaluate(tt.in); got.Status != tt.want {
				t.Fatalf("%s (%s), want %s", got.Status, got.Summary, tt.want)
			}
		})
	}

	in := priced("244363658508")
	in.Bounds = &feeds.Bounds{Min: big.NewInt(1), Max: bi(ethMax), TypeAndVersion: "DualAggregator 1.0.0"}
	if got := (CircuitBreaker{}).Evaluate(in); got.Status != OK || !strings.Contains(got.Summary, "nominal") {
		t.Fatalf("ETH nominal bounds: %+v", got)
	}

	in = priced("10700000") // LUNA-like 0.107 with min 0.10
	in.Bounds = &feeds.Bounds{Min: big.NewInt(10000000), Max: bi(ethMax), TypeAndVersion: ocr2}
	if got := (CircuitBreaker{}).Evaluate(in); got.Status != OK {
		t.Fatalf("7%% from minAnswer is outside warn_within: %s", got.Status)
	}
	in.Reading.Round.Answer = big.NewInt(10100000) // 0.101: 0.99% from min
	if got := (CircuitBreaker{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("near minAnswer: %s", got.Status)
	}

	in.Bounds.TypeAndVersion = "OffchainAggregator 4.0.0"
	if got := (CircuitBreaker{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("unverified type: %s", got.Status)
	}
	in.Bounds = &feeds.Bounds{Min: big.NewInt(20000000), Max: bi(ethMax), TypeAndVersion: ocr2}
	if got := (CircuitBreaker{}).Evaluate(in); got.Status != Critical {
		t.Fatalf("answer below minAnswer: %s", got.Status)
	}
	in.Bounds = nil
	if got := (CircuitBreaker{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("no bounds: %s", got.Status)
	}
}

func meta(dec uint8, desc string, agg string, phase uint16, version int64) *feeds.Metadata {
	return &feeds.Metadata{Decimals: dec, Description: desc, Aggregator: common.HexToAddress(agg), PhaseID: phase, Version: big.NewInt(version)}
}

func TestAggregatorChanged(t *testing.T) {
	in := priced("1")
	in.Metadata = meta(8, "ETH / USD", "0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5", 7, 6)
	if got := (AggregatorChanged{}).Evaluate(in); got.Status != OK {
		t.Fatalf("first observation: %s", got.Status)
	}
	in.PrevMetadata = meta(8, "ETH / USD", "0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5", 7, 6)
	if got := (AggregatorChanged{}).Evaluate(in); got.Status != OK {
		t.Fatalf("unchanged: %s", got.Status)
	}
	in.PrevMetadata = meta(8, "ETH / USD", "0x54bCC589d9743E521c64233706FC8cB36D275b07", 6, 6)
	got := (AggregatorChanged{}).Evaluate(in)
	if got.Status != Info || got.Details["previous_aggregator"] == "" {
		t.Fatalf("changed: %+v", got)
	}
}

func TestMetadataDrift(t *testing.T) {
	in := priced("1")
	in.ExpectedDecimals, in.ExpectedDescription = 8, "ETH / USD"
	in.Metadata = meta(8, "ETH / USD", "0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5", 7, 6)
	if got := (MetadataDrift{}).Evaluate(in); got.Status != OK {
		t.Fatalf("as configured: %s", got.Status)
	}
	in.Metadata.Decimals = 18
	if got := (MetadataDrift{}).Evaluate(in); got.Status != Critical {
		t.Fatalf("decimals: %s", got.Status)
	}
	in.Metadata.Decimals = 8
	in.Metadata.Description = "ETH / USD (deprecated)"
	if got := (MetadataDrift{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("description: %s", got.Status)
	}
	in.Metadata.Description = "ETH / USD"
	in.PrevMetadata = meta(8, "ETH / USD", "0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5", 7, 4)
	if got := (MetadataDrift{}).Evaluate(in); got.Status != Info {
		t.Fatalf("version: %s", got.Status)
	}
	in.Metadata = nil
	if got := (MetadataDrift{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("no metadata: %s", got.Status)
	}
}

func TestJump(t *testing.T) {
	in := priced("120000000")
	in.Params.Jump.Enabled = true
	if got := (Jump{}).Evaluate(in); got.Status != OK {
		t.Fatalf("no previous: %s", got.Status)
	}
	in.Prev = &feeds.Reading{Round: feeds.Round{RoundID: big.NewInt(9), Answer: big.NewInt(100000000)}}
	if got := (Jump{}).Evaluate(in); got.Status != OK {
		t.Fatalf("exactly 20%%: %s", got.Status)
	}
	in.Reading.Round.Answer = big.NewInt(120000001)
	if got := (Jump{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("above 20%%: %s", got.Status)
	}
	// Moonwell wrsETH: 1 wrsETH reported as ~1,649,934 ETH (18 decimals scale irrelevant).
	in.Prev.Round.Answer = bi("1050000000000000000")
	in.Reading.Round.Answer = bi("1649934607320000000000000")
	if got := (Jump{}).Evaluate(in); got.Status != Warning {
		t.Fatalf("wrsETH-like jump: %s", got.Status)
	}
	in.Prev.Round.RoundID = in.Reading.Round.RoundID
	if got := (Jump{}).Evaluate(in); got.Status != OK {
		t.Fatalf("same round: %s", got.Status)
	}
}

func TestRPCDisagreement(t *testing.T) {
	round := func(answer int64) *feeds.Round {
		return &feeds.Round{RoundID: big.NewInt(1), Answer: big.NewInt(answer), StartedAt: big.NewInt(1), UpdatedAt: big.NewInt(2), AnsweredInRound: big.NewInt(1)}
	}
	in := priced("1")
	in.Reads = []EndpointRead{{Endpoint: "a", Round: round(100)}}
	if got := (RPCDisagreement{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("single endpoint: %s", got.Status)
	}
	in.Reads = append(in.Reads, EndpointRead{Endpoint: "b", Round: round(100)}, EndpointRead{Endpoint: "c", Err: errNope{}})
	if got := (RPCDisagreement{}).Evaluate(in); got.Status != OK {
		t.Fatalf("agree (failed endpoint ignored): %s", got.Status)
	}
	in.Reads[1].Round = round(101)
	got := (RPCDisagreement{}).Evaluate(in)
	if got.Status != Critical || !strings.Contains(got.Summary, "b") {
		t.Fatalf("disagree: %+v", got)
	}
	in.Reads[1].Round = round(100)
	in.Reads[1].Round.UpdatedAt = big.NewInt(3) // only the timestamp differs
	if got := (RPCDisagreement{}).Evaluate(in); got.Status != Critical {
		t.Fatalf("any field difference is a disagreement: %s", got.Status)
	}
}

type errNope struct{}

func (errNope) Error() string { return "nope" }

func TestEvaluateFeedOrderAndPrior(t *testing.T) {
	in := priced("103000000")
	in.Reading.Round.UpdatedAt = big.NewInt(blockTime.Unix() - 3*3600) // stale
	in.Reading.Round.StartedAt = new(big.Int).Sub(in.Reading.Round.UpdatedAt, big.NewInt(10))
	in.Bounds = &feeds.Bounds{Min: big.NewInt(1), Max: big.NewInt(105000000), TypeAndVersion: ocr2}
	results := EvaluateFeed(in)
	byID := map[string]Status{}
	for _, res := range results {
		byID[res.CheckID] = res.Status
	}
	if byID["FW-STALE-001"] != Critical || byID["FW-CB-001"] != Critical {
		t.Fatalf("stale near bound must escalate FW-CB-001: %v", byID)
	}
	if _, ran := byID["FW-JUMP-001"]; ran {
		t.Fatal("experimental check must be off by default")
	}
}

func FuzzDeviationRatio(f *testing.F) {
	f.Add(int64(2000), int64(1), int64(2040), int64(1))
	f.Add(int64(-5), int64(3), int64(7), int64(2))
	f.Fuzz(func(t *testing.T, an, ad, bn, bd int64) {
		if ad == 0 || bd == 0 || bn == 0 {
			return
		}
		a, b := big.NewRat(an, ad), big.NewRat(bn, bd)
		if b.Sign() <= 0 {
			return
		}
		d := DeviationRatio(a, b)
		if d.Sign() < 0 {
			t.Fatalf("negative deviation %s", d)
		}
		if (d.Sign() == 0) != (a.Cmp(b) == 0) {
			t.Fatalf("zero iff equal violated: %s vs %s", a, b)
		}
		// Swapping the operand order around the reference does not change
		// the distance: |a-b| = |b-a|.
		if d.Cmp(DeviationRatio(new(big.Rat).Sub(new(big.Rat).Mul(b, big.NewRat(2, 1)), a), b)) != 0 {
			t.Fatalf("mirror symmetry violated for %s around %s", a, b)
		}
	})
}
