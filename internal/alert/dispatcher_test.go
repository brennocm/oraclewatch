package alert

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/brennocm/oraclewatch/internal/checks"
)

type capture struct {
	mu   sync.Mutex
	msgs []Message
}

func (c *capture) Name() string { return "capture" }
func (c *capture) Send(_ context.Context, m Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}
func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

func notes(n int) []Notification {
	out := make([]Notification, n)
	for i := range out {
		out[i] = Notification{Kind: KindFiring, Result: checks.Result{CheckID: "FW-RPC-001", Network: "ethereum", Feed: string(rune('A' + i)), Status: checks.Critical}}
	}
	return out
}

func TestDispatcherGroupsAndCollapsesStorms(t *testing.T) {
	c := &capture{}
	d := NewDispatcher([]Notifier{c}, DispatcherOptions{GroupWindow: 50 * time.Millisecond, StormThreshold: 3})
	ctx, cancel := context.WithCancel(context.Background())
	go d.Run(ctx)

	d.Enqueue(ctx, notes(2))
	waitFor(t, func() bool { return c.count() == 2 })

	d.Enqueue(ctx, notes(2))
	d.Enqueue(ctx, notes(3)) // same window: 5 > threshold
	waitFor(t, func() bool { return c.count() == 3 })
	if !c.msgs[2].Storm || len(c.msgs[2].Notifications) != 5 {
		t.Fatalf("expected one storm message with 5 alerts, got %+v", c.msgs[2])
	}
	cancel()
	<-d.Done()
}

func TestDispatcherFlushesOnShutdown(t *testing.T) {
	c := &capture{}
	d := NewDispatcher([]Notifier{c}, DispatcherOptions{GroupWindow: time.Hour, StormThreshold: 5})
	ctx, cancel := context.WithCancel(context.Background())
	go d.Run(ctx)
	d.Enqueue(ctx, notes(1))
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-d.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("dispatcher did not stop")
	}
	if c.count() != 1 {
		t.Fatalf("pending alert lost on shutdown: %d sent", c.count())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
