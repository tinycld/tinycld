package core

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// touchNotifyFile signals the other instances watching the notify dir that a
// shared runtime state changed.
//
// The file is LEFT IN PLACE rather than written-then-immediately-removed. The
// remove used to run on the very next line, and on a kqueue/FSEvents backend
// (macOS/BSD) that races the backend's own event delivery: the inode is gone
// before the create is reported, so fsnotify delivers NOTHING and the peer
// never reloads. Measured directly — write+remove yields zero events, while
// write, brief pause, remove yields CREATE then REMOVE. On Linux/inotify the
// create is queued synchronously, which is why this only ever showed up as a
// platform-dependent flake (TestNotifyWatcher_SettingsUpdate /
// _CollectionsUpdate failing on their 3s timeout).
//
// Leaving the file costs nothing and loses no signal: the watcher keys on the
// filename prefix, not the contents, so a later notify's overwrite is itself
// the next event (WRITE rather than CREATE — both pass the filter, and only
// REMOVE is skipped). OnTerminate already unlinks both files, and the whole
// .notify dir is excluded from backups (base_backup.go).
//
// Do NOT "tidy" this back into a write+remove pair.
func touchNotifyFile(path string) error {
	return os.WriteFile(path, nil, 0644)
}

// notifyGuard orders the notify watcher with the app's bootstrap. live says
// whether the app is bootstrapped, for the watcher's goroutines: they must
// not call app.IsBootstrapped(), which reads the DB handles that Bootstrap
// and ClearBootstrap write with no lock. mu orders a delayed reload with
// ClearBootstrap (see enterNotifyReload).
type notifyGuard struct {
	mu   sync.RWMutex
	live atomic.Bool

	// watcher is the current bootstrap's notify watcher and loop counts its
	// event loop, so ClearBootstrap can stop both (see stopNotifyWatcher).
	watchMu sync.Mutex
	watcher *fsnotify.Watcher
	loop    *sync.WaitGroup
}

// trackNotifyWatcher records the watcher a bootstrap just started and
// returns the WaitGroup its event loop runs under.
func trackNotifyWatcher(app App, w *fsnotify.Watcher) *sync.WaitGroup {
	g := notifyGuardOf(app)
	g.watchMu.Lock()
	defer g.watchMu.Unlock()
	g.watcher = w
	g.loop = &sync.WaitGroup{}
	return g.loop
}

// stopNotifyWatcher closes the bootstrap's notify watcher and waits for its
// event loop to return. Upstream closes a watcher only at the next Bootstrap
// or at terminate, so every app that was bootstrapped and then cleared kept
// a goroutine and an fsnotify descriptor for the rest of the process.
// Closing twice is harmless: the next Bootstrap and OnTerminate still close
// the same watcher.
func stopNotifyWatcher(app App) {
	g := notifyGuardOf(app)
	g.watchMu.Lock()
	w, loop := g.watcher, g.loop
	g.watcher, g.loop = nil, nil
	g.watchMu.Unlock()
	if w == nil {
		return
	}
	_ = w.Close()
	loop.Wait()
}

// notifyGuards holds the notifyGuard of each app.
var notifyGuards sync.Map // App -> *notifyGuard

func notifyGuardOf(app App) *notifyGuard {
	g, _ := notifyGuards.LoadOrStore(app, &notifyGuard{})
	return g.(*notifyGuard)
}

// notifyLive reports whether app is bootstrapped. Unlike app.IsBootstrapped,
// it is safe to call from the watcher's goroutines.
func notifyLive(app App) bool {
	return notifyGuardOf(app).live.Load()
}

// enterNotifyReload starts a delayed notify reload. A notify event schedules
// the reload 50 ms after it arrives, and the app's bootstrap can be cleared
// in between (a process that terminates just after another instance changed
// a collection): the reload's queries would then run on nil DB handles and
// panic in the timer goroutine. ok is false when the bootstrap is cleared.
// Otherwise the caller runs the reload and then calls release; until then
// ClearBootstrap waits (bindNotifyClearGuard).
func enterNotifyReload(app App) (release func(), ok bool) {
	g := notifyGuardOf(app)
	g.mu.RLock()
	if !g.live.Load() {
		g.mu.RUnlock()
		return nil, false
	}
	return g.mu.RUnlock, true
}

// markNotifyLive is called by Bootstrap once it has opened the DB handles:
// from then on the watcher may reload. A plain call rather than an
// OnBootstrap handler, so the fork adds nothing to an app's hook chain.
func markNotifyLive(app App) {
	notifyGuardOf(app).live.Store(true)
}

// bindNotifyClearGuard makes ClearBootstrap stop the notify watcher, clear the
// live flag, and wait for a notify reload in progress before it closes the DB
// handles (see enterNotifyReload).
func (app *BaseApp) bindNotifyClearGuard() {
	app.OnBootstrapClear().Bind(&hook.Handler[*BootstrapEvent]{
		Id: systemHookIdNotifyWatcher,
		Func: func(e *BootstrapEvent) error {
			// Before the lock: a reload the loop scheduled may be waiting on
			// it, and the loop does not wait for that reload.
			stopNotifyWatcher(app)
			g := notifyGuardOf(app)
			g.mu.Lock()
			defer g.mu.Unlock()
			g.live.Store(false)
			return e.Next()
		},
		Priority: -998,
	})
}
