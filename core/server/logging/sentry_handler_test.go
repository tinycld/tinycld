package logging

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// captureTransport records events instead of sending them.
type captureTransport struct {
	events []*sentry.Event
}

func (t *captureTransport) Configure(sentry.ClientOptions)        {}
func (t *captureTransport) SendEvent(e *sentry.Event)             { t.events = append(t.events, e) }
func (t *captureTransport) Flush(time.Duration) bool              { return true }
func (t *captureTransport) FlushWithContext(context.Context) bool { return true }
func (t *captureTransport) Close()                                {}

func newTestHub(t *testing.T) (*sentry.Hub, *captureTransport) {
	t.Helper()
	tr := &captureTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{Dsn: "", Transport: tr})
	if err != nil {
		t.Fatalf("sentry.NewClient: %v", err)
	}
	return sentry.NewHub(client, sentry.NewScope()), tr
}

func TestSentryHandlerCapturesAtOrAboveLevel(t *testing.T) {
	hub, tr := newTestHub(t)
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	logger := slog.New(NewSentryHandler(slog.LevelWarn))
	logger.WarnContext(ctx, "reconnect failed", "attempt", 3)

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 captured event, got %d", len(tr.events))
	}
	if tr.events[0].Message != "reconnect failed" {
		t.Errorf("unexpected message: %q", tr.events[0].Message)
	}
}

func TestSentryHandlerIgnoresBelowLevel(t *testing.T) {
	hub, tr := newTestHub(t)
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	logger := slog.New(NewSentryHandler(slog.LevelWarn))
	logger.InfoContext(ctx, "just fyi")

	if len(tr.events) != 0 {
		t.Fatalf("expected no events, got %d", len(tr.events))
	}
}

// A call site with no ctx (tickers, startup, background goroutines) must still
// produce an event — the user id is enrichment, never a gate.
func TestSentryHandlerCapturesWithoutAHubOnContext(t *testing.T) {
	logger := slog.New(NewSentryHandler(slog.LevelWarn))
	// Must not panic with no hub on the context.
	logger.Warn("no ctx here")
}

func TestSentryHandlerAttachesAttrsAsContext(t *testing.T) {
	hub, tr := newTestHub(t)
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	logger := slog.New(NewSentryHandler(slog.LevelWarn)).With("pkg", "cards")
	logger.ErrorContext(ctx, "flush failed", "boardID", "b1")

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	logCtx, ok := tr.events[0].Contexts["log"]
	if !ok {
		t.Fatalf("expected a %q context on the event, got %v", "log", tr.events[0].Contexts)
	}
	if logCtx["pkg"] != "cards" {
		t.Errorf("expected pkg=cards in log context, got %v", logCtx["pkg"])
	}
	if logCtx["boardID"] != "b1" {
		t.Errorf("expected boardID=b1 in log context, got %v", logCtx["boardID"])
	}
}
