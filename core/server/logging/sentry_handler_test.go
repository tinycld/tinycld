package logging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
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

	logger := slog.New(NewSentryHandler(slog.LevelWarn)).With("pkg", "widgets")
	logger.ErrorContext(ctx, "flush failed", "widgetID", "w1")

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	logCtx, ok := tr.events[0].Contexts["log"]
	if !ok {
		t.Fatalf("expected a %q context on the event, got %v", "log", tr.events[0].Contexts)
	}
	if logCtx["pkg"] != "widgets" {
		t.Errorf("expected pkg=widgets in log context, got %v", logCtx["pkg"])
	}
	if logCtx["widgetID"] != "w1" {
		t.Errorf("expected widgetID=w1 in log context, got %v", logCtx["widgetID"])
	}
}

// An error attr must reach Sentry as its text. Most error values have no
// exported fields, so sent as they are they serialize to {} and the event
// loses the one detail that explains it.
func TestSentryHandlerSendsErrorsAsText(t *testing.T) {
	hub, tr := newTestHub(t)
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	base := errors.New("disk full")
	logger := slog.New(NewSentryHandler(slog.LevelWarn)).With("cause", base)
	logger.ErrorContext(ctx, "save failed", "err", fmt.Errorf("save: %w", base))

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	data, err := json.Marshal(tr.events[0].Contexts["log"])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"err":"save: disk full"`, `"cause":"disk full"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log context = %s, want it to contain %s", data, want)
		}
	}
}

// A typed-nil pointer in an error interface is not a nil error, and its
// Error method dereferences the pointer. The handler runs on every warn+
// record, so a panic here would take down the logging caller.
func TestSentryHandlerSendsATypedNilErrorWithoutPanicking(t *testing.T) {
	hub, tr := newTestHub(t)
	ctx := sentry.SetHubOnContext(context.Background(), hub)

	var typedNil error = (*fs.PathError)(nil)
	logger := slog.New(NewSentryHandler(slog.LevelWarn))
	logger.ErrorContext(ctx, "open failed", "err", typedNil)

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	if got := tr.events[0].Contexts["log"]["err"]; got != "<nil>" {
		t.Errorf("err = %v, want <nil>", got)
	}
}
