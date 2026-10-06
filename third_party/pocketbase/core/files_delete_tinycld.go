package core

import "sync"

// fileDeleteGroup counts the storage deletes that registerBaseHooks runs in
// the background after a model with files is deleted. A name of its own so
// that base.go's fork field needs no new import. It is a pointer field so the
// shallow clones a transaction makes of the app share it.
type fileDeleteGroup = sync.WaitGroup

// waitFileDeletes is called by ClearBootstrap before it closes the DB
// handles. Without the wait, a background delete outlives the bootstrap: it
// opens the filesystem, which recreates <dataDir>/storage, while the caller is
// already removing the data dir.
//
// A plain call rather than an OnBootstrapClear handler, so the fork adds
// nothing to an app's hook chain.
//
// A delete must not start while ClearBootstrap runs (sync.WaitGroup forbids an
// Add concurrent with Wait); a model delete at that time would already race
// the DB handles being closed.
func (app *BaseApp) waitFileDeletes() {
	if app.fileDeletes != nil {
		app.fileDeletes.Wait()
	}
}
