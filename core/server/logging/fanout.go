// Package logging installs the process-wide slog handler for TinyCld servers.
//
// Call sites declare what happened (level + message + attrs); this package
// decides where it goes. That separation is the point: before it, the function
// you called silently picked the destination (slog → stderr, app.Logger() →
// the _logs table, sentry.CaptureException → Sentry).
package logging

import (
	"context"
	"log/slog"
)

// fanout forwards each record to every child handler that accepts it, so a
// single call site can reach stderr, the _logs table, and Sentry at once with
// independent per-destination levels.
type fanout struct {
	handlers []slog.Handler
}

// NewFanout returns a handler that forwards to each of handlers.
func NewFanout(handlers ...slog.Handler) slog.Handler {
	return &fanout{handlers: handlers}
}

// Enabled reports whether ANY child would accept the level. Returning false
// only when every child declines is what lets a quiet stderr handler coexist
// with a chatty Sentry one without either suppressing the other.
func (f *fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle re-checks Enabled per child, because the parent's Enabled is an OR
// across all of them — without this a debug record aimed at stderr would also
// be written to the error-level Sentry handler.
//
// A child returning an error must not stop the others: losing a stderr line
// should never also lose the Sentry event. The last error is returned for the
// caller's benefit and otherwise ignored.
func (f *fanout) Handle(ctx context.Context, r slog.Record) error {
	var lastErr error
	for _, h := range f.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (f *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &fanout{handlers: next}
}

func (f *fanout) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithGroup(name)
	}
	return &fanout{handlers: next}
}
