package alert

import (
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/checks"
)

var t0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func res(s checks.Status) []checks.Result {
	return []checks.Result{{CheckID: "FW-STALE-001", Network: "ethereum", Feed: "ETH/USD", Status: s}}
}

type step struct {
	at     time.Duration
	status checks.Status
	want   []Kind
}

func run(t *testing.T, o Options, steps []step) {
	t.Helper()
	m := NewManager(o)
	for i, s := range steps {
		got := m.Process(res(s.status), t0.Add(s.at))
		if len(got) != len(s.want) {
			t.Fatalf("step %d (%s at +%s): got %d notifications %+v, want %v", i, s.status, s.at, len(got), got, s.want)
		}
		for j := range got {
			if got[j].Kind != s.want[j] {
				t.Fatalf("step %d: kind %s, want %s", i, got[j].Kind, s.want[j])
			}
		}
	}
}

var std = Options{For: 2 * time.Minute, Cooldown: 30 * time.Minute, SendResolved: true}

func TestFiresOnlyAfterFor(t *testing.T) {
	run(t, std, []step{
		{0, checks.Warning, nil},
		{time.Minute, checks.Warning, nil},
		{2 * time.Minute, checks.Warning, []Kind{KindFiring}}, // exactly "for"
		{3 * time.Minute, checks.Warning, nil},                // deduplicated
	})
}

func TestShortBlipNeverFires(t *testing.T) {
	run(t, std, []step{
		{0, checks.Warning, nil},
		{time.Minute, checks.OK, nil},
		{2 * time.Minute, checks.Warning, nil}, // restarts the pending timer
		{3 * time.Minute, checks.Warning, nil},
		{4 * time.Minute, checks.Warning, []Kind{KindFiring}},
	})
}

func TestEscalationBypassesCooldown(t *testing.T) {
	run(t, std, []step{
		{0, checks.Warning, nil},
		{2 * time.Minute, checks.Warning, []Kind{KindFiring}},
		{3 * time.Minute, checks.Critical, []Kind{KindFiring}},
		{4 * time.Minute, checks.Critical, nil},
		{5 * time.Minute, checks.Warning, nil}, // de-escalation is silent
		{6 * time.Minute, checks.Critical, []Kind{KindFiring}},
	})
}

func TestReminderAfterCooldown(t *testing.T) {
	run(t, std, []step{
		{0, checks.Critical, nil},
		{2 * time.Minute, checks.Critical, []Kind{KindFiring}},
		{31 * time.Minute, checks.Critical, nil},
		{32 * time.Minute, checks.Critical, []Kind{KindFiring}},
	})
}

func TestResolveWithHysteresis(t *testing.T) {
	run(t, std, []step{
		{0, checks.Warning, nil},
		{2 * time.Minute, checks.Warning, []Kind{KindFiring}},
		{3 * time.Minute, checks.OK, nil},
		{4 * time.Minute, checks.Warning, nil}, // flapped back: same episode, no new message
		{5 * time.Minute, checks.OK, nil},
		{6 * time.Minute, checks.OK, nil},
		{7 * time.Minute, checks.OK, []Kind{KindResolved}},
		{8 * time.Minute, checks.OK, nil},
	})
}

func TestNoResolvedWhenDisabled(t *testing.T) {
	o := std
	o.SendResolved = false
	run(t, o, []step{
		{0, checks.Warning, nil},
		{2 * time.Minute, checks.Warning, []Kind{KindFiring}},
		{3 * time.Minute, checks.OK, nil},
		{5 * time.Minute, checks.OK, nil},
		{6 * time.Minute, checks.Warning, nil}, // new episode starts pending
		{8 * time.Minute, checks.Warning, []Kind{KindFiring}},
	})
}

func TestUnknownNeitherFiresNorResolves(t *testing.T) {
	run(t, std, []step{
		{0, checks.Critical, nil},
		{2 * time.Minute, checks.Critical, []Kind{KindFiring}},
		{3 * time.Minute, checks.Unknown, nil},
		{10 * time.Minute, checks.Unknown, nil},
		{40 * time.Minute, checks.Critical, []Kind{KindFiring}}, // reminder: still firing
	})
}

func TestZeroForFiresAndResolvesImmediately(t *testing.T) {
	run(t, Options{Cooldown: time.Hour, SendResolved: true}, []step{
		{0, checks.Warning, []Kind{KindFiring}},
		{time.Minute, checks.OK, []Kind{KindResolved}},
	})
}

func TestInfoIsAnEvent(t *testing.T) {
	run(t, std, []step{
		{0, checks.Info, []Kind{KindEvent}},
		{time.Minute, checks.OK, nil},
	})
}

func TestActiveAndLink(t *testing.T) {
	m := NewManager(Options{For: 0, Cooldown: time.Hour, Link: func(r checks.Result) string { return "https://etherscan.io/" + r.Network }})
	n := m.Process(res(checks.Critical), t0)
	if len(n) != 1 || n[0].Link != "https://etherscan.io/ethereum" {
		t.Fatalf("notifications = %+v", n)
	}
	a := m.Active()
	if len(a) != 1 || a[0].State != StateFiring || a[0].Status != checks.Critical {
		t.Fatalf("active = %+v", a)
	}
}
