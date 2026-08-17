package logging

import (
	"log/slog"
	"os"
)

// Destination levels. Independent by design: stderr and the DB-backed _logs
// table are for operators reading history, while Sentry is for paging someone.
//
// _logs has no level constant here — pbHandler (app.Logger().Handler(), the
// caller-supplied destination below) already gates itself on PocketBase's own
// admin-configurable Settings.Logs.MinLevel (Info by default). Wrapping it in
// a second, hardcoded filter would either duplicate that gate or silently
// override an admin who lowered MinLevel to Debug to troubleshoot — and
// _logs must keep receiving records unconditionally: the OTA e2e harness
// polls it for an "app-boot: rendered" beacon to confirm a promoted bundle
// booted on a device.
const (
	StderrLevel = slog.LevelInfo
	SentryLevel = slog.LevelWarn
)

// Install sets the process-wide default logger to a fan-out over stderr, the
// PocketBase _logs table, and Sentry.
//
// pbHandler is the caller-resolved handler for the _logs table — normally
// app.Logger().Handler(). It is passed in rather than resolved here because
// app.Logger() falls back to slog.Default() before bootstrap; resolving it
// inside the handler we are installing AS the default would recurse.
//
// Pass nil for pbHandler to skip the _logs destination (useful in tests and in
// binaries with no PocketBase app).
func Install(pbHandler slog.Handler) {
	handlers := []slog.Handler{
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: StderrLevel}),
		NewSentryHandler(SentryLevel),
	}
	if pbHandler != nil {
		handlers = append(handlers, pbHandler)
	}
	slog.SetDefault(slog.New(NewFanout(handlers...)))
}

// ForPackage returns a logger stamped with a pkg attribute, replacing the old
// hand-written "cards: " message prefixes with a queryable structured field.
//
//	log := logging.ForPackage("cards")
//	log.WarnContext(ctx, "refusing to flush a card from another board", "cardID", id)
func ForPackage(name string) *slog.Logger {
	return slog.Default().With("pkg", name)
}
