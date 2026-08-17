package coreserver

import (
	"log/slog"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
)

// TestLoggerInstallDoesNotHangDuringBootstrap pins the ordering bug fixed
// alongside this test: logging.Install must run from an app.OnBootstrap()
// hook, AFTER e.Next(), not directly inside Register/registerSharedEarly.
//
// Before bootstrap, app.Logger() falls back to slog.Default() (see
// pocketbase/core/base.go). If Install were called with that fallback
// handler, the fan-out it installs as the new default would contain a
// handler that resolves back into itself — and the first log call
// (registerSharedCore reaches automation.Register's slog.Info) recurses
// forever. That is exactly what happened when the install lived directly
// in Register: go test ./coreserver/ -run
// TestTenantCompositionMatchesHostMinusRecordedExceptions hung until its
// 150s timeout.
//
// Registering a full app (Register/RegisterTenant) is the only way to
// reach registerSharedEarly's hook binding, so this lives in coreserver
// rather than the logging package, which can't see that wiring.
//
// The run happens in a goroutine with a bounded timeout: if the ordering
// regresses, the bug is a deadlock, not a returned error, so nothing short
// of a timeout will turn it into a clean test failure.
func TestLoggerInstallDoesNotHangDuringBootstrap(t *testing.T) {
	preBootstrapHandler := slog.Default().Handler()
	t.Cleanup(func() { slog.SetDefault(slog.New(preBootstrapHandler)) })

	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	Register(app, Options{
		HooksDir:      t.TempDir(),
		MigrationsDir: t.TempDir(),
		TypesDir:      t.TempDir(),
		PublicDir:     t.TempDir(),
		HooksPoolSize: 1,
	})

	done := make(chan error, 1)
	go func() {
		if err := app.Bootstrap(); err != nil {
			done <- err
			return
		}
		// The path that previously recursed: a plain slog.Info on the
		// process-wide default, taken right after bootstrap completes —
		// the same point automation.Register logs from during
		// registerSharedCore.
		slog.Info("post-bootstrap log reaches the installed fan-out")
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Bootstrap failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Bootstrap + log call did not complete within 10s — logging.Install likely wired the default logger to itself before bootstrap")
	}

	// Install replaces slog's default handler with the fan-out; confirming
	// it changed (rather than asserting the unexported fanout type, which
	// this package can't see) is what proves Install actually ran here
	// rather than being skipped or silently no-op'd.
	if slog.Default().Handler() == preBootstrapHandler {
		t.Fatal("slog.Default() handler is unchanged after Bootstrap — logging.Install did not run")
	}
}
