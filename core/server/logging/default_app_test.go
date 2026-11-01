package logging_test

import (
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
)

// newBareApp is an app that is never bootstrapped: SetDefaultApp reads only
// its data dir (through appstate.ID) and its store.
func newBareApp(dataDir string) core.App {
	return core.NewBaseApp(core.BaseAppConfig{DataDir: dataDir})
}

// A process that holds many apps must not have one of them take every record
// with no app in its ctx. Only the first app becomes the default; a second
// app is refused with an error, and does not get the process Sentry hub.
func TestSetDefaultAppKeepsTheFirstApp(t *testing.T) {
	stderr := &syncBuffer{}
	t.Cleanup(logging.InstallTo(stderr))
	t.Cleanup(func() { logging.SetDefaultApp(nil) })

	a, b := newBareApp(t.TempDir()), newBareApp(t.TempDir())
	logging.SetDefaultApp(a)
	logging.SetDefaultApp(b)

	if got := logging.DefaultApp(); got != a {
		t.Fatalf("DefaultApp() is the app on %s, want the first app on %s", dataDirOf(got), a.DataDir())
	}
	if logging.Hub(b) == sentry.CurrentHub() {
		t.Error("the refused app got the process Sentry hub")
	}
	if logging.Hub(a) != sentry.CurrentHub() {
		t.Error("the default app lost the process Sentry hub")
	}
	if !strings.Contains(stderr.String(), "a second app cannot be the default log app") {
		t.Errorf("stderr has no error for the refused app:\n%s", stderr.String())
	}
}

// An app reopened on the same data dir (a restore, a second bootstrap) is the
// same app to its owner, so it replaces the default.
func TestSetDefaultAppReplacesTheSameApp(t *testing.T) {
	t.Cleanup(logging.InstallTo(&syncBuffer{}))
	t.Cleanup(func() { logging.SetDefaultApp(nil) })

	dir := t.TempDir()
	first, reopened := newBareApp(dir), newBareApp(dir)
	logging.SetDefaultApp(first)
	logging.SetDefaultApp(reopened)

	if got := logging.DefaultApp(); got != reopened {
		t.Fatalf("DefaultApp() is not the reopened app on %s", dir)
	}
	if logging.Hub(reopened) != sentry.CurrentHub() {
		t.Error("the reopened app did not get the process Sentry hub")
	}
}

// Nil clears the default, after which another app may take it.
func TestSetDefaultAppNilClears(t *testing.T) {
	t.Cleanup(logging.InstallTo(&syncBuffer{}))
	t.Cleanup(func() { logging.SetDefaultApp(nil) })

	a, b := newBareApp(t.TempDir()), newBareApp(t.TempDir())
	logging.SetDefaultApp(a)
	logging.SetDefaultApp(nil)
	if got := logging.DefaultApp(); got != nil {
		t.Fatalf("DefaultApp() after nil is the app on %s, want none", got.DataDir())
	}
	logging.SetDefaultApp(b)
	if got := logging.DefaultApp(); got != b {
		t.Fatalf("DefaultApp() is the app on %s, want the app on %s", dataDirOf(got), b.DataDir())
	}
}

func dataDirOf(app core.App) string {
	if app == nil {
		return "(none)"
	}
	return app.DataDir()
}
