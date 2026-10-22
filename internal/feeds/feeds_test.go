package feeds

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

func TestNormalize(t *testing.T) {
	maxInt256, _ := new(big.Int).SetString("57896044618658097711785492504343953926634992332820282019728792003956564819967", 10)
	tests := []struct {
		answer   *big.Int
		decimals uint8
		want     *big.Rat
	}{
		{big.NewInt(244363658508), 8, big.NewRat(61090914627, 25000000)}, // 2443.63658508
		{big.NewInt(99981853), 8, big.NewRat(99981853, 100000000)},
		{big.NewInt(7), 0, big.NewRat(7, 1)},
		{big.NewInt(1), 18, new(big.Rat).SetFrac(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))},
		{big.NewInt(-5), 1, big.NewRat(-1, 2)},
		{maxInt256, 18, new(big.Rat).SetFrac(maxInt256, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))},
	}
	for _, tt := range tests {
		if got := Normalize(tt.answer, tt.decimals); got.Cmp(tt.want) != 0 {
			t.Errorf("Normalize(%s, %d) = %s, want %s", tt.answer, tt.decimals, got, tt.want)
		}
	}
	if got := Normalize(big.NewInt(244363658508), 8).FloatString(8); got != "2443.63658508" {
		t.Errorf("FloatString = %s", got)
	}
}

func TestRoundIDDecoding(t *testing.T) {
	// Values read on-chain on 2026-10-22 (docs/evidence).
	tests := []struct {
		id       string
		phase    uint16
		aggRound uint64
	}{
		{"129127208515966896195", 7, 34883}, // Ethereum ETH/USD
		{"36893488147419244286", 2, 141054}, // Arbitrum ETH/USD
		{"18446744073709561001", 1, 9385},   // Base ETH/USD
		{"73786976294838206464", 4, 0},      // Ethereum USDC/USD, nonexistent round 0
		{"18446744073709551657", 1, 41},     // Arbitrum sequencer feed
	}
	for _, tt := range tests {
		id, _ := new(big.Int).SetString(tt.id, 10)
		r := Round{RoundID: id}
		if r.PhaseID() != tt.phase || r.AggregatorRoundID() != tt.aggRound {
			t.Errorf("%s: phase %d aggRound %d, want %d %d", tt.id, r.PhaseID(), r.AggregatorRoundID(), tt.phase, tt.aggRound)
		}
	}
}

func TestRoundEqual(t *testing.T) {
	a := Round{big.NewInt(1), big.NewInt(2), big.NewInt(3), big.NewInt(4), big.NewInt(1)}
	b := Round{big.NewInt(1), big.NewInt(2), big.NewInt(3), big.NewInt(4), big.NewInt(1)}
	if !a.Equal(b) {
		t.Fatal("identical rounds must be equal")
	}
	b.Answer = big.NewInt(3)
	if a.Equal(b) {
		t.Fatal("rounds with different answers must differ")
	}
	if a.Equal(Round{}) {
		t.Fatal("nil fields must not equal set fields")
	}
}

func TestDiff(t *testing.T) {
	agg1 := common.HexToAddress("0x7d4E742018fb52E48b08BE73d041C18B21de6Fb5")
	agg2 := common.HexToAddress("0x54bCC589d9743E521c64233706FC8cB36D275b07")
	prev := Metadata{Decimals: 8, Description: "ETH / USD", Version: big.NewInt(6), Aggregator: agg1, PhaseID: 7}
	if d := Diff(prev, prev); len(d) != 0 {
		t.Fatalf("no change expected: %v", d)
	}
	cur := prev
	cur.Decimals = 18
	cur.Aggregator = agg2
	cur.PhaseID = 8
	d := Diff(prev, cur)
	fields := map[string]bool{}
	for _, c := range d {
		fields[c.Field] = true
	}
	if len(d) != 3 || !fields["decimals"] || !fields["aggregator"] || !fields["phase_id"] {
		t.Fatalf("diff = %+v", d)
	}
}

func TestTimeOf(t *testing.T) {
	if got := TimeOf(big.NewInt(1792635539)).Unix(); got != 1792635539 {
		t.Fatalf("TimeOf = %d", got)
	}
	if !TimeOf(nil).Equal(TimeOf(big.NewInt(0))) {
		t.Fatal("nil and 0 must map to the epoch")
	}
	huge := new(big.Int).Lsh(big.NewInt(1), 200)
	if TimeOf(huge).Unix() <= 0 {
		t.Fatal("out-of-range timestamps must clamp to the far future")
	}
}

type emptyCaller struct{}

func (emptyCaller) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return nil, nil
}

type garbageCaller struct{}

func (garbageCaller) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return []byte{0xde, 0xad}, nil
}

func TestEmptyAndGarbageResults(t *testing.T) {
	addr := common.HexToAddress("0x5f4eC3Df9cbd43714FE2740f5E3616155c5b8419")
	if _, err := LatestRound(context.Background(), emptyCaller{}, addr, big.NewInt(1)); !errors.Is(err, ErrEmptyResult) {
		t.Fatalf("empty result: %v", err)
	}
	if _, err := LatestRound(context.Background(), garbageCaller{}, addr, big.NewInt(1)); err == nil {
		t.Fatal("garbage result must be an error")
	}
	if _, err := ReadMetadata(context.Background(), garbageCaller{}, addr, big.NewInt(1)); err == nil {
		t.Fatal("garbage metadata must be an error")
	}
}

func FuzzDecodeLatestRound(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, 160))
	f.Add(append(make([]byte, 159), 0xff))
	f.Fuzz(func(_ *testing.T, data []byte) {
		// A malicious RPC controls these bytes; decoding must never panic.
		_, _ = unpackRound("latestRoundData", data)
		_, _ = unpackOne[string](aggregatorV3ABI, "description", data)
		_, _ = unpackOne[*big.Int](boundsABI, "minAnswer", data)
		_, _ = unpackAddress(proxyABI, "aggregator", data)
	})
}
