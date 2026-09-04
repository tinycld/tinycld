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

	ForPackage("boards").Warn("refusing to flush")

	out := buf.String()
	if !strings.Contains(out, "pkg=boards") {
		t.Errorf("expected pkg=boards, got %q", out)
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
