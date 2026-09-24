package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// ApplyPendingRestore runs before the database is opened, so it derives every
// path from the data dir alone. Three states:
//
//   - armed present, swapped absent: first boot after a restore staged its
//     data — move pb_data aside, move pending in, write swapped.
//   - swapped present: the previous boot swapped but never finalized (it
//     failed) — move the restored data to restore/failed/<id>, move previous
//     back, clear swapped.
//   - neither: nothing to do, including on a data dir that does not exist yet.
//
// Every step is a rename, so a crash between two of them leaves a state the
// next boot recognises and completes.
func ApplyPendingRestore(dataDir string) error {
	if raw, err := os.ReadFile(swappedPathOf(dataDir)); err == nil {
		var a armed
		if err := json.Unmarshal(raw, &a); err != nil {
			return fmt.Errorf("backup: swapped marker unreadable: %w", err)
		}
		return rollBack(dataDir, a)
	}
	raw, err := os.ReadFile(armedPathOf(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var a armed
	if err := json.Unmarshal(raw, &a); err != nil {
		return fmt.Errorf("backup: armed marker unreadable: %w", err)
	}
	if _, err := os.Stat(filepath.Join(a.Pending, "data.db")); err != nil {
		// Armed but never fully staged: the restore was killed during phase 5,
		// before it could disarm. The marker alone is not evidence of a restore.
		if rerr := os.RemoveAll(a.Pending); rerr != nil {
			return rerr
		}
		return os.Remove(armedPathOf(dataDir))
	}
	return swapIn(dataDir, a)
}

// swapIn moves pb_data aside as a whole, so auxiliary.db, the -wal and -shm
// files and anything else a deployment left there go with it, and moves in only
// the two members an archive carries. The restored pb_data therefore starts
// without auxiliary.db — PocketBase recreates it, and keeping the old one would
// pair a logs database with a database it knows nothing about.
func swapIn(dataDir string, a armed) error {
	prev := filepath.Join(restoreDirOf(dataDir), "previous", a.ID)
	if err := os.MkdirAll(filepath.Dir(prev), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(dataDir); err == nil {
		if err := os.Rename(dataDir, prev); err != nil {
			return fmt.Errorf("backup: move pb_data aside: %w", err)
		}
	}
	// pb_data is gone, either just now or because a crashed earlier attempt had
	// already moved it. Either way prev holds what this organization had.
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"data.db", "storage"} {
		src := filepath.Join(a.Pending, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, filepath.Join(dataDir, name)); err != nil {
			return fmt.Errorf("backup: move %s in: %w", name, err)
		}
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	// swapped is written before armed is removed, so the two markers overlap
	// rather than leaving a gap. A crash in the gap would leave restored data in
	// place with nothing to say so, and the next boot would serve it silently.
	// Overlapping is harmless: ApplyPendingRestore reads swapped first.
	if err := os.WriteFile(swappedPathOf(dataDir), raw, 0o600); err != nil {
		return err
	}
	if err := os.RemoveAll(a.Pending); err != nil {
		return err
	}
	if err := os.Remove(armedPathOf(dataDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	restoring.Store(true)
	return nil
}

// rollBack undoes a swap whose process never finalized. The restored data is
// kept under restore/failed/<id> rather than deleted: it booted far enough to
// swap in, so whatever stopped it is worth looking at.
func rollBack(dataDir string, a armed) error {
	failed := filepath.Join(restoreDirOf(dataDir), "failed", a.ID)
	if err := os.MkdirAll(filepath.Dir(failed), 0o700); err != nil {
		return err
	}
	if err := os.RemoveAll(failed); err != nil {
		return err
	}
	// pb_data can already be gone if something removed it between the two boots.
	// The point of the rollback is getting the previous copy back, so a missing
	// pb_data is not a reason to refuse it.
	if _, err := os.Stat(dataDir); err == nil {
		if err := os.Rename(dataDir, failed); err != nil {
			return fmt.Errorf("backup: set the failed restore aside: %w", err)
		}
	}
	prev := filepath.Join(restoreDirOf(dataDir), "previous", a.ID)
	if err := os.Rename(prev, dataDir); err != nil {
		return fmt.Errorf("backup: move previous data back: %w", err)
	}
	if err := os.Remove(armedPathOf(dataDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// This process serves the data the organization had before the restore, so
	// it is not in maintenance mode.
	restoring.Store(false)
	return os.Remove(swappedPathOf(dataDir))
}

// FinalizeRestore runs after a successful boot of the restored data: the
// restore row is inserted as succeeded, the safety copies are dropped, and
// everyone who can act is told.
//
// It must run BEFORE MarkInterrupted at boot. The restored database is the
// archive's, so it carries the backing-up instance's own rows — including
// whatever was running when the archive was written. MarkInterrupted closes
// those, and running it first would also close the row this inserts.
//
// The row is new rather than an update. After the swap the live database is the
// archive's, where the initiating instance's "running" row does not exist; that
// row stays running in the discarded database, which is gone. The new row has no
// initiator either — the user who asked for the restore may not exist in the
// restored data.
func FinalizeRestore(app core.App) error {
	raw, err := os.ReadFile(swappedPath(app))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var a armed
	if err := json.Unmarshal(raw, &a); err != nil {
		return fmt.Errorf("backup: swapped marker unreadable: %w", err)
	}
	row := newRow(app, KindRestore, "", "")
	row.Set("status", "succeeded")
	row.Set("finished", types.NowDateTime())
	// The manifest comes off the marker: it is the only record of what this
	// process is now running, and Verify compares its counts with the live data.
	row.Set("manifest", a.Manifest)
	row.Set("metadata", map[string]any{
		"restored_from_job": a.ID,
		"note":              "history before this row is from the restored backup",
	})
	if err := app.Save(row); err != nil {
		return err
	}
	if err := os.RemoveAll(previousDir(app, a.ID)); err != nil {
		log.Warn("could not remove the pre-restore copy of pb_data", "id", a.ID, "err", err)
	}
	if a.Pre != "" {
		if err := os.Remove(a.Pre); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warn("could not remove a pre-restore backup", "id", a.ID, "err", err)
		}
	}
	if err := os.Remove(swappedPath(app)); err != nil {
		return err
	}
	restoring.Store(false)
	announceRestore(app, RestoreRequest{}, row, true, "")
	return nil
}
