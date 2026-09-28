package logging

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type ctxKey struct{}

// captured is one record as the capturing handler saw it, with attrs flattened
// to dotted keys so group nesting is visible.
type captured struct {
	level slog.Level
	msg   string
	ctx   context.Context
	attrs map[string]string
}

type captureSink struct {
	mu      sync.Mutex
	records []captured
}

type captureHandler struct {
	sink   *captureSink
	level  slog.Level
	prefix string
	attrs  map[string]string
}

func newCaptureHandler(level slog.Level) *captureHandler {
	return &captureHandler{sink: &captureSink{}, level: level, attrs: map[string]string{}}
}

func (h *captureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := map[string]string{}
	for k, v := range h.attrs {
		attrs[k] = v
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[h.prefix+a.Key] = a.Value.String()
		return true
	})
	h.sink.mu.Lock()
	defer h.sink.mu.Unlock()
	h.sink.records = append(h.sink.records, captured{level: r.Level, msg: r.Message, ctx: ctx, attrs: attrs})
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = map[string]string{}
	for k, v := range h.attrs {
		next.attrs[k] = v
	}
	for _, a := range attrs {
		next.attrs[h.prefix+a.Key] = a.Value.String()
	}
	return &next
}

func (h *captureHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.prefix = h.prefix + name + "."
	return &next
}

func installCapture(t *testing.T, level slog.Level) *captureHandler {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	h := newCaptureHandler(level)
	slog.SetDefault(slog.New(h))
	return h
}

func TestForPackageCreatedBeforeSetDefaultFollowsTheNewDefault(t *testing.T) {
	log := ForPackage("widgets")
	derived := log.With("widgetID", "w1").WithGroup("spin").With("rpm", 42)

	h := installCapture(t, slog.LevelDebug)
	ctx := context.WithValue(context.Background(), ctxKey{}, "user-7")

	log.WarnContext(ctx, "widget jammed", "attempt", 3)
	derived.ErrorContext(ctx, "spin failed", "reason", "stuck")

	if got := len(h.sink.records); got != 2 {
		t.Fatalf("expected 2 records at the capturing handler, got %d", got)
	}

	first := h.sink.records[0]
	if first.level != slog.LevelWarn || first.msg != "widget jammed" {
		t.Errorf("first record: level=%v msg=%q", first.level, first.msg)
	}
	if first.attrs["pkg"] != "widgets" || first.attrs["attempt"] != "3" {
		t.Errorf("first record attrs: %v", first.attrs)
	}
	if first.ctx.Value(ctxKey{}) != "user-7" {
		t.Errorf("ctx was not passed through")
	}

	second := h.sink.records[1]
	if second.level != slog.LevelError {
		t.Errorf("second record level = %v", second.level)
	}
	want := map[string]string{
		"pkg":         "widgets",
		"widgetID":    "w1",
		"spin.rpm":    "42",
		"spin.reason": "stuck",
	}
	for k, v := range want {
		if second.attrs[k] != v {
			t.Errorf("second record attr %q = %q, want %q (all: %v)", k, second.attrs[k], v, second.attrs)
		}
	}
	if second.ctx.Value(ctxKey{}) != "user-7" {
		t.Errorf("ctx was not passed through the derived logger")
	}
}

func TestForPackageEnabledFollowsTheCurrentDefaultLevel(t *testing.T) {
	log := ForPackage("widgets")

	installCapture(t, slog.LevelWarn)
	if log.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("info should be disabled under a warn-level default")
	}
	if !log.Enabled(context.Background(), slog.LevelWarn) {
		t.Errorf("warn should be enabled under a warn-level default")
	}

	h := newCaptureHandler(slog.LevelDebug)
	slog.SetDefault(slog.New(h))
	if !log.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("debug should be enabled once the default is swapped to a debug-level handler")
	}
	log.Debug("after swap")
	if len(h.sink.records) != 1 || !strings.Contains(h.sink.records[0].msg, "after swap") {
		t.Errorf("record did not reach the swapped-in default: %v", h.sink.records)
	}
}

func TestForPackageSiblingLoggersDoNotShareAttrs(t *testing.T) {
	parent := ForPackage("widgets")
	a := parent.With("side", "a")
	b := parent.With("side", "b")

	h := installCapture(t, slog.LevelDebug)
	a.Info("from a")
	b.Info("from b")

	if got := len(h.sink.records); got != 2 {
		t.Fatalf("expected 2 records at the capturing handler, got %d", got)
	}
	if h.sink.records[0].attrs["side"] != "a" || h.sink.records[1].attrs["side"] != "b" {
		t.Errorf("sibling attrs leaked: %v / %v", h.sink.records[0].attrs, h.sink.records[1].attrs)
	}
}

// Run with -race: a package logger resolves the default on every call, so it
// must be safe while another goroutine swaps the default.
func TestForPackageLogsWhileTheDefaultIsSwapped(t *testing.T) {
	log := ForPackage("widgets")
	final := installCapture(t, slog.LevelDebug)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			log.Info("tick", "i", i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			slog.SetDefault(slog.New(newCaptureHandler(slog.LevelDebug)))
		}
	}()
	wg.Wait()

	slog.SetDefault(slog.New(final))
	log.Info("after the swaps")
	last := final.sink.records[len(final.sink.records)-1]
	if last.msg != "after the swaps" || last.attrs["pkg"] != "widgets" {
		t.Errorf("record after the swaps did not reach the current default: %+v", last)
	}
}
