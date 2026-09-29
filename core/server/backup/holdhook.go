package backup

import (
	"errors"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"tinycld.org/core/backup/hold"
)

// DrainJobID names the cron job DrainHeldDeletes runs under, so it can be
// looked up or replaced rather than accidentally duplicated.
const DrainJobID = "backup-hold-drain"

// boundApps tracks which app instances already have the delete-hold handler
// bound, so a caller invoking BindDeleteHold more than once (e.g. RegisterBackupBoot
// running twice against the same app in a test) does not stack a second handler.
var boundApps sync.Map // core.App -> struct{}

// BindDeleteHold makes every storage delete honour a backup's delete hold. A
// delete under a valid hold is journaled instead of run; DrainHeldDeletes runs
// it later once the hold clears. Bound once per app, and must be bound before
// the first app.NewFilesystem() call it should cover — the filesystem attaches
// the underlying hook only when a handler already exists at creation time.
func BindDeleteHold(app core.App) {
	if _, loaded := boundApps.LoadOrStore(app, struct{}{}); loaded {
		return
	}
	core.OnFilesystemDelete(app).BindFunc(func(e *core.FilesystemDeleteEvent) error {
		if !hold.Active(app.DataDir(), time.Now()) {
			return e.Next()
		}
		if err := hold.Journal(app.DataDir(), e.FileKey); err != nil {
			// A delete that cannot be journaled must not be lost: run it now.
			// The backup may then miss this file, which is the lesser failure.
			log.Error("could not journal a held delete; deleting now", "err", err)
			return e.Next()
		}
		return nil
	})
}

// DrainHeldDeletes runs the journal once no valid hold exists. Called at boot
// and by a cron job every minute.
func DrainHeldDeletes(app core.App) {
	now := time.Now()
	if st, removed, err := hold.RemoveStale(app.DataDir(), now); err != nil {
		log.Warn("could not read the backup delete hold", "err", err)
	} else if removed {
		log.Warn("a backup delete hold expired without being released", "holder", st.Holder, "expired", st.Expires)
	}
	fs, err := app.NewFilesystem()
	if err != nil {
		log.Error("could not open storage to drain held deletes", "err", err)
		return
	}
	defer fs.Close()
	n, err := hold.Drain(app.DataDir(), now, func(key string) error {
		if err := fs.Delete(key); err != nil && !errors.Is(err, filesystem.ErrNotFound) {
			return err
		}
		return nil
	})
	if err != nil {
		log.Error("could not drain held storage deletes", "done", n, "err", err)
	}
}
