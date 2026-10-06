package core

import (
	"os"
	"sync"
	"sync/atomic"

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

// bindNotifyClearGuard keeps the guard's live flag in step with the app's
// bootstrap: set once Bootstrap has opened the DB handles, cleared before
// ClearBootstrap closes them. ClearBootstrap also waits for a notify reload in
// progress before it closes the DB handles (see enterNotifyReload).
func (app *BaseApp) bindNotifyClearGuard() {
	app.OnBootstrap().Bind(&hook.Handler[*BootstrapEvent]{
		Id: "__tinycldNotifyLive__",
		Func: func(e *BootstrapEvent) error {
			if err := e.Next(); err != nil {
				return err
			}
			notifyGuardOf(app).live.Store(true)
			return nil
		},
		Priority: -998,
	})
	app.OnBootstrapClear().Bind(&hook.Handler[*BootstrapEvent]{
		Id: systemHookIdNotifyWatcher,
		Func: func(e *BootstrapEvent) error {
			g := notifyGuardOf(app)
			g.mu.Lock()
			defer g.mu.Unlock()
			g.live.Store(false)
			return e.Next()
		},
		Priority: -998,
	})
}
