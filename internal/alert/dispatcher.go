package alert

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/brennocm/oraclewatch/internal/checks"
)

// Message is what a notifier sends. A normal message carries one
// notification; a storm summary carries many.
type Message struct {
	Storm         bool
	Notifications []Notification
}

// Worst returns the highest status among the message's notifications;
// resolved notifications count as OK.
func (m Message) Worst() checks.Status {
	worst := checks.OK
	for _, n := range m.Notifications {
		s := n.Result.Status
		if n.Kind == KindResolved {
			s = checks.OK
		}
		if s > worst {
			worst = s
		}
	}
	return worst
}

// Notifier delivers messages to one channel.
type Notifier interface {
	Name() string
	Send(ctx context.Context, m Message) error
}

// SentObserver counts delivered messages for metrics.
type SentObserver interface {
	ObserveSent(channel string, severity checks.Status, err error)
}

type nopSent struct{}

func (nopSent) ObserveSent(string, checks.Status, error) {}

// DispatcherOptions configure a Dispatcher.
type DispatcherOptions struct {
	// GroupWindow: notifications arriving within this window after the
	// first one are delivered together.
	GroupWindow time.Duration
	// StormThreshold: more notifications than this in one window are
	// collapsed into a single summary message per notifier.
	StormThreshold int
	SendTimeout    time.Duration
	// FlushTimeout bounds delivery of pending alerts at shutdown.
	FlushTimeout time.Duration
	Logger       *slog.Logger
	Observer     SentObserver
}

// Dispatcher buffers notifications for GroupWindow and delivers them to every
// notifier. Run it in its own goroutine; it drains pending notifications when
// its context is cancelled.
type Dispatcher struct {
	o         DispatcherOptions
	notifiers []Notifier
	in        chan []Notification
	done      chan struct{}
}

// NewDispatcher returns a Dispatcher for the given notifiers.
func NewDispatcher(notifiers []Notifier, o DispatcherOptions) *Dispatcher {
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Observer == nil {
		o.Observer = nopSent{}
	}
	if o.SendTimeout <= 0 {
		o.SendTimeout = 30 * time.Second
	}
	if o.FlushTimeout <= 0 {
		o.FlushTimeout = 10 * time.Second
	}
	if o.StormThreshold < 2 {
		o.StormThreshold = 2
	}
	return &Dispatcher{o: o, notifiers: notifiers, in: make(chan []Notification, 64), done: make(chan struct{})}
}

// Enqueue hands notifications to the dispatcher. It never blocks the caller
// for long: if the buffer is full the call waits until ctx is done, and the
// notifications are then dropped. A drop is logged as an error: the manager
// already counts them as sent, so they would otherwise vanish without trace.
func (d *Dispatcher) Enqueue(ctx context.Context, ns []Notification) {
	if len(ns) == 0 {
		return
	}
	select {
	case d.in <- ns:
	case <-ctx.Done():
		d.o.Logger.Error("alert queue full; notifications dropped", "count", len(ns), "worst", Message{Notifications: ns}.Worst().String())
	}
}

// Done is closed when Run has returned and every pending message was sent.
func (d *Dispatcher) Done() <-chan struct{} { return d.done }

// Run delivers until ctx is cancelled, then flushes what is pending within
// FlushTimeout, so alerts raised just before shutdown are not lost.
func (d *Dispatcher) Run(ctx context.Context) {
	defer close(d.done)
	var pending []Notification
	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case ns := <-d.in:
			pending = append(pending, ns...)
			if timer == nil {
				timer = time.NewTimer(d.o.GroupWindow)
				timerC = timer.C
			}
		case <-timerC:
			d.deliver(ctx, pending)
			pending, timer, timerC = nil, nil, nil
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			// Collect anything already enqueued, then send it all.
		drain:
			for {
				select {
				case ns := <-d.in:
					pending = append(pending, ns...)
				default:
					break drain
				}
			}
			// ctx is already cancelled; keep its values but not its cancellation.
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.o.FlushTimeout)
			d.deliver(fctx, pending)
			cancel()
			return
		}
	}
}

// deliver sends one batch to every notifier concurrently.
func (d *Dispatcher) deliver(ctx context.Context, batch []Notification) {
	if len(batch) == 0 {
		return
	}
	var msgs []Message
	if len(batch) > d.o.StormThreshold {
		msgs = []Message{{Storm: true, Notifications: batch}}
	} else {
		for _, n := range batch {
			msgs = append(msgs, Message{Notifications: []Notification{n}})
		}
	}
	var wg sync.WaitGroup
	for _, nt := range d.notifiers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, m := range msgs {
				sctx, cancel := context.WithTimeout(ctx, d.o.SendTimeout)
				err := nt.Send(sctx, m)
				cancel()
				d.o.Observer.ObserveSent(nt.Name(), m.Worst(), err)
				if err != nil {
					d.o.Logger.Error("notification failed", "channel", nt.Name(), "err", err)
				}
			}
		}()
	}
	wg.Wait()
}
