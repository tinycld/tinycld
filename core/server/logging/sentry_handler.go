package logging

import (
	"context"
	"log/slog"

	"github.com/getsentry/sentry-go"
)

var sentryLevel = map[slog.Level]sentry.Level{
	slog.LevelDebug: sentry.LevelDebug,
	slog.LevelInfo:  sentry.LevelInfo,
	slog.LevelWarn:  sentry.LevelWarning,
	slog.LevelError: sentry.LevelError,
}

// sentryHandler turns log records at or above minLevel into Sentry events.
//
// User attribution is inherited, not plumbed: the per-request middleware in
// coreserver/sentry.go already puts a hub carrying sentry.User{ID: …} on the
// request context, so a *Context log call picks it up automatically. Calls
// without a ctx (tickers, startup, background goroutines) fall back to the
// current hub and produce an unattributed event — which is correct, because
// there genuinely is no user in those paths.
type sentryHandler struct {
	minLevel slog.Level
	attrs    []slog.Attr
	groups   []string
}

// NewSentryHandler returns a handler capturing records at or above minLevel.
func NewSentryHandler(minLevel slog.Level) slog.Handler {
	return &sentryHandler{minLevel: minLevel}
}

func (h *sentryHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *sentryHandler) Handle(ctx context.Context, r slog.Record) error {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}

	extra := make(map[string]any, len(h.attrs)+r.NumAttrs())
	for _, a := range h.attrs {
		extra[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		extra[a.Key] = a.Value.Any()
		return true
	})

	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentryLevel[r.Level])
		scope.SetExtras(extra)
		hub.CaptureMessage(r.Message)
	})
	return nil
}

func (h *sentryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	next = append(next, h.attrs...)
	next = append(next, attrs...)
	return &sentryHandler{minLevel: h.minLevel, attrs: next, groups: h.groups}
}

func (h *sentryHandler) WithGroup(name string) slog.Handler {
	next := make([]string, 0, len(h.groups)+1)
	next = append(next, h.groups...)
	next = append(next, name)
	return &sentryHandler{minLevel: h.minLevel, attrs: h.attrs, groups: next}
}
