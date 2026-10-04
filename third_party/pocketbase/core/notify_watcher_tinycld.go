package core

import (
	"os"
	"sync"

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

// notifyGuards holds, per app, the lock that orders the notify watcher's
// delayed reloads with ClearBootstrap (see enterNotifyReload).
var notifyGuards sync.Map // App -> *sync.RWMutex

func notifyGuardOf(app App) *sync.RWMutex {
	g, _ := notifyGuards.LoadOrStore(app, &sync.RWMutex{})
	return g.(*sync.RWMutex)
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
	g.RLock()
	if !app.IsBootstrapped() {
		g.RUnlock()
		return nil, false
	}
	return g.RUnlock, true
}

// bindNotifyClearGuard makes ClearBootstrap wait for a notify reload in
// progress before it closes the DB handles (see enterNotifyReload).
func (app *BaseApp) bindNotifyClearGuard() {
	app.OnBootstrapClear().Bind(&hook.Handler[*BootstrapEvent]{
		Id: systemHookIdNotifyWatcher,
		Func: func(e *BootstrapEvent) error {
			g := notifyGuardOf(app)
			g.Lock()
			defer g.Unlock()
			return e.Next()
		},
		Priority: -998,
	})
}
