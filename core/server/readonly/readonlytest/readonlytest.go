// Package readonlytest holds helpers for tests of writers that wait out or
// skip read-only mode.
package readonlytest

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
)

type waitProbe struct {
	context.Context
	once    sync.Once
	waiting chan struct{}
}

func (p *waitProbe) Done() <-chan struct{} {
	p.once.Do(func() { close(p.waiting) })
	return p.Context.Done()
}

// WaitProbe returns a context to pass to a writer's wait, and a channel that
// closes the first time the context's Done is read. readonly.WaitInactive
// reads Done only while the mode is on, so the close means the writer has
// reached the wait and cannot write until the mode ends. The writer must not
// read Done itself before its wait.
func WaitProbe(t testing.TB) (context.Context, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p := &waitProbe{Context: ctx, waiting: make(chan struct{})}
	return p, p.waiting
}

// Record is one captured log record, with its attributes as strings.
type Record struct {
	Level slog.Level
	Msg   string
	Attrs map[string]string
}

type recordingHandler struct {
	slog.Handler
	mu      *sync.Mutex
	records *[]Record
	attrs   []slog.Attr
}

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	rec := Record{Level: r.Level, Msg: r.Message, Attrs: map[string]string{}}
	for _, a := range h.attrs {
		rec.Attrs[a.Key] = a.Value.String()
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.Attrs[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	*h.records = append(*h.records, rec)
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	attrs := append(append([]slog.Attr{}, h.attrs...), as...)
	return &recordingHandler{Handler: h.Handler, mu: h.mu, records: h.records, attrs: attrs}
}

// WithGroup keeps recording; a group's name is not kept, which no caller here
// needs.
func (h *recordingHandler) WithGroup(string) slog.Handler { return h }

// CaptureLogs records every log record at Info or above until the test ends.
// It swaps the process-wide default logger, so the calling test must not run
// in parallel (a test that enters read-only mode already must not).
func CaptureLogs(t testing.TB) func() []Record {
	t.Helper()
	var mu sync.Mutex
	var records []Record
	prev := slog.Default()
	// Not prev's handler: wrapping slog's built-in default handler in a new
	// default deadlocks, because that handler writes through the log package,
	// which SetDefault points back at the new default.
	inner := slog.NewTextHandler(io.Discard, nil)
	slog.SetDefault(slog.New(&recordingHandler{Handler: inner, mu: &mu, records: &records}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() []Record {
		mu.Lock()
		defer mu.Unlock()
		return append([]Record{}, records...)
	}
}

// Find returns the first record with message msg.
func Find(records []Record, msg string) (Record, bool) {
	for _, r := range records {
		if r.Msg == msg {
			return r, true
		}
	}
	return Record{}, false
}
