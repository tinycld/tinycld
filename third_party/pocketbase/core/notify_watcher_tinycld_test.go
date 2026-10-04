package core

import (
	"testing"
	"time"
)

// A notify event schedules a reload 50 ms later. When the app's bootstrap is
// cleared in between (a test's cleanup, or a process that terminates right
// after another instance changed a collection), the reload must do nothing:
// its DB handles are nil, and a query through them panics in the timer
// goroutine, which ends the process.
func TestNotifyReload_SkippedAfterClearBootstrap(t *testing.T) {
	app := NewBaseApp(BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := app.ClearBootstrap(); err != nil {
		t.Fatal(err)
	}
	if release, ok := enterNotifyReload(app); ok {
		release()
		t.Fatal("a reload was allowed on an app whose bootstrap was cleared")
	}
}

// A reload already running when the bootstrap is cleared finishes first:
// ClearBootstrap waits for it before it closes the DB handles.
func TestNotifyReload_ClearBootstrapWaitsForAReloadInProgress(t *testing.T) {
	app := NewBaseApp(BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	release, ok := enterNotifyReload(app)
	if !ok {
		t.Fatal("a reload was refused on a bootstrapped app")
	}

	cleared := make(chan error, 1)
	go func() { cleared <- app.ClearBootstrap() }()
	select {
	case <-cleared:
		release()
		t.Fatal("ClearBootstrap closed the DB handles under a reload in progress")
	case <-time.After(100 * time.Millisecond):
	}
	if !app.IsBootstrapped() {
		release()
		t.Fatal("the DB handles were cleared under a reload in progress")
	}

	release()
	select {
	case err := <-cleared:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ClearBootstrap did not finish after the reload")
	}
	if app.IsBootstrapped() {
		t.Fatal("still bootstrapped after ClearBootstrap")
	}
}
