package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestForPackageStampsThePkgAttr(t *testing.T) {
	var buf bytes.Buffer
	Install(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) })

	ForPackage("cards").Warn("refusing to flush")

	out := buf.String()
	if !strings.Contains(out, "pkg=cards") {
		t.Errorf("expected pkg=cards, got %q", out)
	}
	if !strings.Contains(out, "refusing to flush") {
		t.Errorf("expected the message, got %q", out)
	}
}

func TestInstallSetsTheDefaultLogger(t *testing.T) {
	var buf bytes.Buffer
	Install(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) })

	slog.Info("via the package-level default")

	if !strings.Contains(buf.String(), "via the package-level default") {
		t.Errorf("slog.Default was not installed: %q", buf.String())
	}
}
