package logging

import (
	"log"
	"os"

	"github.com/getsentry/sentry-go"
)

// InitSentry (re)initializes the process-wide Sentry client for dsn. Safe to
// call repeatedly: sentry.Init swaps the active client, so a DSN change takes
// effect for every later capture. An empty dsn leaves the SDK a no-op.
//
// Every TinyCld process that reports to Sentry calls this, so they all report
// with the same options; each caller decides where its DSN comes from.
func InitSentry(dsn string) {
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      Environment(),
		TracesSampleRate: 0.2,
		AttachStacktrace: true,
	}); err != nil {
		// Deliberately stdlib log, not the slog default: this reports that
		// Sentry itself failed to initialize, and the slog default's warn+
		// path publishes to Sentry. If Sentry is broken, the report that
		// Sentry is broken could never arrive that way. Mirrors the console.*
		// exemption in the client's sentry.ts for the same reason.
		log.Printf("Sentry initialization failed: %v", err)
	}
}

// Environment is the Sentry environment: ENVIRONMENT when set, else
// "development" for a --dev run, else "production".
func Environment() string {
	if env := os.Getenv("ENVIRONMENT"); env != "" {
		return env
	}
	for _, arg := range os.Args {
		if arg == "--dev" {
			return "development"
		}
	}
	return "production"
}
