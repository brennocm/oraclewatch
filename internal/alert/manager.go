// Package alert turns check results into notifications: a state machine per
// (check, network, feed) with a persistence delay ("for"), hysteresis,
// cooldown and severity escalation, plus a dispatcher that groups alerts and
// collapses storms into one message.
//
// State lives in memory (ADR 0007): a restart forgets firing alerts, so a
// condition that persists fires again after "for" elapses.
package alert

import (
	"sort"
	"sync"
	"time"

	"github.com/brennocm/oraclewatch/internal/checks"
)

// State of one alert key.
type State int

// States.
const (
	StateOK State = iota
	StatePending
	StateFiring
	// StateResolving: the condition cleared while firing; it must stay clear
	// for "for" before the alert resolves (hysteresis).
	StateResolving
)

func (s State) String() string {
	return [...]string{"ok", "pending", "firing", "resolving"}[s]
}

// Kind of notification.
type Kind int

// Notification kinds.
const (
	KindFiring   Kind = iota // first time, escalation, or reminder after cooldown
	KindResolved             // the condition cleared
	KindEvent                // one-shot informational result (status Info)
)

func (k Kind) String() string { return [...]string{"firing", "resolved", "event"}[k] }

// Notification is one message to deliver.
type Notification struct {
	Kind     Kind
	Result   checks.Result
	Since    time.Time     // when the condition started (firing) or the alert started (resolved)
	Previous checks.Status // status before an escalation, or the status that resolved
	Reminder bool          // re-sent after cooldown without a change
	Link     string        // explorer link for the subject, if known
}

// Options configure the Manager.
type Options struct {
	For          time.Duration
	Cooldown     time.Duration
	SendResolved bool
	// Link returns an explorer URL for a result, or "".
	Link func(checks.Result) string
}

type entry struct {
	state    State
	since    time.Time     // entered PENDING, or start of the firing episode
	clearAt  time.Time     // entered RESOLVING
	status   checks.Status // status that was last sent
	lastSent time.Time
	last     checks.Result
}

// Manager holds alert state. It is safe for concurrent use: each network's
// worker calls Process with its own results.
type Manager struct {
	o  Options
	mu sync.Mutex
	m  map[string]*entry
}

// NewManager returns an empty Manager.
func NewManager(o Options) *Manager {
	if o.Link == nil {
		o.Link = func(checks.Result) string { return "" }
	}
	return &Manager{o: o, m: map[string]*entry{}}
}

// Process applies one cycle's results at time now and returns the
// notifications to send.
func (m *Manager) Process(results []checks.Result, now time.Time) []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Notification
	for _, r := range results {
		if n, ok := m.step(r, now); ok {
			n.Link = m.o.Link(n.Result)
			out = append(out, n)
		}
	}
	return out
}

func (m *Manager) step(r checks.Result, now time.Time) (Notification, bool) {
	key := r.Key()
	e := m.m[key]
	if e == nil {
		e = &entry{}
		m.m[key] = e
	}
	switch r.Status {
	case checks.Unknown:
		// Missing data neither fires nor resolves; the episode keeps its state.
		return Notification{}, false
	case checks.Info:
		e.last = r
		return Notification{Kind: KindEvent, Result: r, Since: now}, true
	case checks.OK:
		return m.clear(e, r, now)
	}
	return m.bad(e, r, now)
}

// bad handles Warning and Critical.
func (m *Manager) bad(e *entry, r checks.Result, now time.Time) (Notification, bool) {
	e.last = r
	switch e.state {
	case StateOK:
		e.state, e.since = StatePending, now
		if m.o.For > 0 {
			return Notification{}, false
		}
		fallthrough
	case StatePending:
		if now.Sub(e.since) < m.o.For {
			return Notification{}, false
		}
		e.state, e.status, e.lastSent = StateFiring, r.Status, now
		return Notification{Kind: KindFiring, Result: r, Since: e.since}, true
	case StateResolving:
		// Came back before the hysteresis elapsed: still the same episode.
		e.state = StateFiring
		fallthrough
	case StateFiring:
		prev := e.status
		switch {
		case r.Status > prev:
			e.status, e.lastSent = r.Status, now
			return Notification{Kind: KindFiring, Result: r, Since: e.since, Previous: prev}, true
		case now.Sub(e.lastSent) >= m.o.Cooldown:
			e.status, e.lastSent = r.Status, now
			return Notification{Kind: KindFiring, Result: r, Since: e.since, Previous: prev, Reminder: true}, true
		}
		// Severity may drop (critical -> warning) without a message; the
		// lower level is what a reminder or a later escalation compares to.
		e.status = r.Status
	}
	return Notification{}, false
}

// clear handles OK.
func (m *Manager) clear(e *entry, r checks.Result, now time.Time) (Notification, bool) {
	switch e.state {
	case StatePending:
		e.state = StateOK
	case StateFiring:
		e.state, e.clearAt = StateResolving, now
		if m.o.For > 0 {
			return Notification{}, false
		}
		fallthrough
	case StateResolving:
		if now.Sub(e.clearAt) < m.o.For {
			return Notification{}, false
		}
		prev, since := e.status, e.since
		*e = entry{state: StateOK, last: r}
		if !m.o.SendResolved {
			return Notification{}, false
		}
		return Notification{Kind: KindResolved, Result: r, Since: since, Previous: prev}, true
	}
	e.last = r
	return Notification{}, false
}

// Active is one alert that is pending, firing or resolving.
type Active struct {
	Key    string
	State  State
	Since  time.Time
	Status checks.Status
	Result checks.Result
}

// Active returns the alerts that are not OK, sorted by key.
func (m *Manager) Active() []Active {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Active
	for k, e := range m.m {
		if e.state != StateOK {
			out = append(out, Active{Key: k, State: e.state, Since: e.since, Status: e.last.Status, Result: e.last})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
