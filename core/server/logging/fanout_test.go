package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestFanoutWritesToEveryEnabledHandler(t *testing.T) {
	var a, b bytes.Buffer
	h := NewFanout(
		slog.NewTextHandler(&a, &slog.HandlerOptions{Level: slog.LevelDebug}),
		slog.NewTextHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
	slog.New(h).Info("hello", "k", "v")

	if !strings.Contains(a.String(), "hello") {
		t.Errorf("handler a missing record: %q", a.String())
	}
	if !strings.Contains(b.String(), "hello") {
		t.Errorf("handler b missing record: %q", b.String())
	}
}

func TestFanoutRespectsPerHandlerLevel(t *testing.T) {
	var chatty, quiet bytes.Buffer
	h := NewFanout(
		slog.NewTextHandler(&chatty, &slog.HandlerOptions{Level: slog.LevelDebug}),
		slog.NewTextHandler(&quiet, &slog.HandlerOptions{Level: slog.LevelError}),
	)
	slog.New(h).Info("only-chatty")

	if !strings.Contains(chatty.String(), "only-chatty") {
		t.Errorf("debug handler should have received the record")
	}
	if quiet.Len() != 0 {
		t.Errorf("error-level handler should have dropped the record, got %q", quiet.String())
	}
}

func TestFanoutEnabledIsTrueIfAnyChildIsEnabled(t *testing.T) {
	var buf bytes.Buffer
	h := NewFanout(
		slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}),
		slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}),
	)
	if !h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("fanout should be enabled when any child is enabled")
	}
}

func TestFanoutPropagatesAttrsToChildren(t *testing.T) {
	var buf bytes.Buffer
	h := NewFanout(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.New(h).With("pkg", "cards").Info("msg")

	if !strings.Contains(buf.String(), "pkg=cards") {
		t.Errorf("WithAttrs did not reach the child handler: %q", buf.String())
	}
}
