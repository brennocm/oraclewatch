package checks

import (
	"math/big"
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/config"
	"github.com/brennocm/oraclewatch/internal/feeds"
)

var blockTime = time.Unix(1792638000, 0).UTC()

func input(updatedAt int64, heartbeat time.Duration) *FeedInput {
	return &FeedInput{
		Network: "ethereum", Feed: "ETH/USD", Params: config.DefaultCheckParams(),
		Heartbeat: heartbeat, Block: 100, BlockTime: blockTime, Now: blockTime,
		Reading: &feeds.Reading{Block: 100, Round: feeds.Round{
			RoundID: big.NewInt(10), Answer: big.NewInt(244363658508),
			StartedAt: big.NewInt(updatedAt - 14), UpdatedAt: big.NewInt(updatedAt), AnsweredInRound: big.NewInt(10),
		}},
	}
}

func TestStaleBoundaries(t *testing.T) {
	bt := blockTime.Unix()
	tests := []struct {
		name      string
		heartbeat time.Duration
		age       int64
		want      Status
	}{
		// 1h heartbeat: slack = max(360s, 60s) = 360s; max_age 3960s; critical > 7920s.
		{"fresh", time.Hour, 10, OK},
		{"exactly max_age", time.Hour, 3960, OK},
		{"one second over", time.Hour, 3961, Warning},
		{"exactly critical", time.Hour, 7920, Warning},
		{"over critical", time.Hour, 7921, Critical},
		// 255s heartbeat: 10% is 25.5s < 60s min_slack, so max_age = 315s.
		{"short heartbeat observed +17s", 255 * time.Second, 272, OK},
		{"short heartbeat at max_age", 255 * time.Second, 315, OK},
		{"short heartbeat over", 255 * time.Second, 316, Warning},
		{"updatedAt after block", time.Hour, -30, OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Stale{}.Evaluate(input(bt-tt.age, tt.heartbeat))
			if got.Status != tt.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Summary, tt.want)
			}
		})
	}
}

func TestStaleUnknownCases(t *testing.T) {
	in := input(blockTime.Unix(), time.Hour)
	in.ChainStalled = true
	if got := (Stale{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("stalled chain: %s", got.Status)
	}
	in = input(0, time.Hour)
	if got := (Stale{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("zero updatedAt: %s", got.Status)
	}
	in.Reading = nil
	if got := (Stale{}).Evaluate(in); got.Status != Unknown {
		t.Fatalf("no reading: %s", got.Status)
	}
}

func TestInvalid(t *testing.T) {
	bt := blockTime.Unix()
	tests := []struct {
		name   string
		mutate func(in *FeedInput)
		want   Status
	}{
		{"valid", func(*FeedInput) {}, OK},
		{"zero answer", func(in *FeedInput) { in.Reading.Round.Answer = big.NewInt(0) }, Critical},
		{"negative answer", func(in *FeedInput) { in.Reading.Round.Answer = big.NewInt(-1) }, Critical},
		{"zero updatedAt", func(in *FeedInput) { in.Reading.Round.UpdatedAt = big.NewInt(0) }, Critical},
		{"zero startedAt", func(in *FeedInput) { in.Reading.Round.StartedAt = big.NewInt(0) }, Critical},
		{"updatedAt one second after block", func(in *FeedInput) { in.Reading.Round.UpdatedAt = big.NewInt(bt + 1) }, Critical},
		{"updatedAt equal to block", func(in *FeedInput) { in.Reading.Round.UpdatedAt = big.NewInt(bt) }, OK},
		{"round went backwards", func(in *FeedInput) {
			in.Prev = &feeds.Reading{Block: 99, Round: feeds.Round{RoundID: big.NewInt(11)}}
		}, Critical},
		{"older block may show older round", func(in *FeedInput) {
			in.Prev = &feeds.Reading{Block: 101, Round: feeds.Round{RoundID: big.NewInt(11)}}
		}, OK},
		{"startedAt after updatedAt beyond skew", func(in *FeedInput) {
			in.Reading.Round.StartedAt = new(big.Int).Add(in.Reading.Round.UpdatedAt, big.NewInt(61))
		}, Warning},
		{"startedAt after updatedAt within skew", func(in *FeedInput) {
			in.Reading.Round.StartedAt = new(big.Int).Add(in.Reading.Round.UpdatedAt, big.NewInt(60))
		}, OK},
		{"huge answer is valid", func(in *FeedInput) {
			in.Reading.Round.Answer = new(big.Int).Lsh(big.NewInt(1), 254)
		}, OK},
		{"no reading", func(in *FeedInput) { in.Reading = nil }, Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := input(bt-100, time.Hour)
			tt.mutate(in)
			if got := (Invalid{}).Evaluate(in); got.Status != tt.want {
				t.Fatalf("status = %s (%s), want %s", got.Status, got.Summary, tt.want)
			}
		})
	}
}
