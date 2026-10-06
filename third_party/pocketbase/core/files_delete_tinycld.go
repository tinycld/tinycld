package core

import (
	"sync"

	"github.com/pocketbase/pocketbase/tools/hook"
)

// bindFileDeletesDrain makes ClearBootstrap wait for the storage deletes that
// registerBaseHooks runs in the background after a model with files is
// deleted. Without the wait, such a delete outlives the bootstrap: it opens
// the filesystem, which recreates <dataDir>/storage, while the caller is
// already removing the data dir.
//
// A delete must not start while ClearBootstrap runs (sync.WaitGroup forbids
// an Add concurrent with Wait); a model delete at that time would already
// race the DB handles being closed.
//
// The WaitGroup is a pointer so that the shallow clones a transaction makes
// of the app share it.
func (app *BaseApp) bindFileDeletesDrain() {
	app.fileDeletes = &sync.WaitGroup{}
	app.OnBootstrapClear().Bind(&hook.Handler[*BootstrapEvent]{
		Id: "__tinycldFileDeletesDrain__",
		Func: func(e *BootstrapEvent) error {
			app.fileDeletes.Wait()
			return e.Next()
		},
		Priority: -999,
	})
}

// fileDeleteGroup counts the background storage deletes in flight. A name of
// its own so that base.go's fork field needs no new import.
type fileDeleteGroup = sync.WaitGroup
