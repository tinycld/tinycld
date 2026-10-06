package core

import (
	"fmt"
	"os"
	"runtime"
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

// Another instance's change reaches the watcher at any time, also while this
// app clears its bootstrap and starts it again. The watcher must not read the
// app's DB handles while ClearBootstrap and Bootstrap write them. Run with
// -race: the failure is a reported data race.
func TestNotifyWatcher_EventsDuringClearBootstrapAreRaceFree(t *testing.T) {
	dir := t.TempDir()
	app := NewBaseApp(BaseAppConfig{DataDir: dir})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	other := NewBaseApp(BaseAppConfig{DataDir: dir})
	if err := other.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = other.ClearBootstrap()
		_ = app.ClearBootstrap()
	})

	for i := range 20 {
		col := NewBaseCollection(fmt.Sprintf("notify_race_%d", i))
		if err := other.Save(col); err != nil {
			t.Fatal(err)
		}
		if err := app.ClearBootstrap(); err != nil {
			t.Fatal(err)
		}
		if err := app.Bootstrap(); err != nil {
			t.Fatal(err)
		}
	}
}

// openFDs counts this process's open file descriptors, or -1 where the
// platform does not list them.
func openFDs() int {
	if runtime.GOOS == "windows" {
		return -1
	}
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

// Each bootstrap starts a notify watcher: a goroutine and an fsnotify
// descriptor. ClearBootstrap ends the app's bootstrap, so it must end the
// watcher too. Otherwise every app that is bootstrapped and cleared (each
// test's app, each tenant a router tears down) leaks both for the rest of the
// process.
func TestClearBootstrap_StopsTheNotifyWatcher(t *testing.T) {
	const apps = 20
	// Settle the runtime's own goroutines first.
	warm := NewBaseApp(BaseAppConfig{DataDir: t.TempDir()})
	if err := warm.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := warm.ClearBootstrap(); err != nil {
		t.Fatal(err)
	}

	goroutinesBefore, fdsBefore := runtime.NumGoroutine(), openFDs()
	for range apps {
		app := NewBaseApp(BaseAppConfig{DataDir: t.TempDir()})
		if err := app.Bootstrap(); err != nil {
			t.Fatal(err)
		}
		if err := app.ClearBootstrap(); err != nil {
			t.Fatal(err)
		}
	}

	// ClearBootstrap waits for the watcher's loop, so the counts are back on
	// its return. The slack absorbs a goroutine another test left ending.
	if got := runtime.NumGoroutine(); got > goroutinesBefore+apps/2 {
		t.Fatalf("goroutines = %d after %d bootstrap/clear cycles, was %d: the notify watchers outlive ClearBootstrap", got, apps, goroutinesBefore)
	}
	if fdsBefore >= 0 {
		if got := openFDs(); got > fdsBefore+apps/2 {
			t.Fatalf("open fds = %d after %d bootstrap/clear cycles, was %d: the notify watchers outlive ClearBootstrap", got, apps, fdsBefore)
		}
	}
}
