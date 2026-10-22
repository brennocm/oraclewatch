package redact

import (
	"context"
	"fmt"
	"log/slog"
)

// Handler wraps a slog.Handler and redacts the message and every attribute
// value before they are written. It is the last line of defence: code should
// not log secrets in the first place, but a wrapped error from a dependency
// can carry one without the caller knowing.
type Handler struct {
	next slog.Handler
	r    *Redactor
}

// NewHandler returns a Handler that redacts with r before passing records to next.
func NewHandler(next slog.Handler, r *Redactor) *Handler {
	return &Handler{next: next, r: r}
}

// Enabled implements slog.Handler.
func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

// Handle implements slog.Handler.
func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, h.r.String(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.attr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

// WithAttrs implements slog.Handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = h.attr(a)
	}
	return &Handler{next: h.next.WithAttrs(clean), r: h.r}
}

// WithGroup implements slog.Handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), r: h.r}
}

func (h *Handler) attr(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, h.r.String(v.String()))
	case slog.KindGroup:
		group := v.Group()
		clean := make([]any, len(group))
		for i, g := range group {
			clean[i] = h.attr(g)
		}
		return slog.Group(a.Key, clean...)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, h.r.Error(err).Error())
		}
		// Anything else is formatted and redacted as text, which loses
		// structure but can never leak through a custom String method.
		return slog.String(a.Key, h.r.String(fmt.Sprint(v.Any())))
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}
