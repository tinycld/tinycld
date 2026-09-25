package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pocketbase/dbx"
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
	// Two questions, and both have to be asked. "Is pending complete?" is
	// answered by the sentinel alone: staging writes it last and removes
	// nothing, whereas the swap moves members OUT of pending, so a missing
	// data.db means "never staged" before the swap starts and "already moved in"
	// after. "Did the swap already start?" is answered by previous/<id>, which
	// only swapIn creates. Reading a missing data.db as an unfinished stage is
	// what discarded a staged storage tree — every attachment in the archive —
	// when a crash landed between the two member renames.
	_, sentinelErr := os.Stat(filepath.Join(a.Pending, stagedSentinel))
	prev := filepath.Join(restoreDirOf(dataDir), "previous", a.ID)
	_, prevErr := os.Stat(prev)
	if sentinelErr != nil && prevErr != nil {
		// Armed, never fully staged, and never swapped: the restore was killed
		// during phase 5 before it could disarm. The marker alone is not
		// evidence of a restore.
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
	// previous/<id> already existing means an earlier attempt got this far, so
	// whatever is at dataDir now is the half-restored tree that attempt built,
	// not the organization's data. Moving it aside would bury the members already
	// swapped in and destroy the one copy of what came before.
	if _, err := os.Stat(prev); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(dataDir); err == nil {
			if err := os.Rename(dataDir, prev); err != nil {
				return fmt.Errorf("backup: move pb_data aside: %w", err)
			}
		}
	} else if err != nil {
		return err
	}
	// pb_data is gone, or holds what an interrupted attempt already moved in.
	// Either way prev holds what this organization had.
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

// rollbackReason is what an operator is told when the restored data was put
// back. The rollback itself cannot know more: it runs before the database is
// open, so the process that failed took its reason with it.
const rollbackReason = "the restored data did not boot; the previous data was put back"

// rollBack undoes a swap whose process never finalized. The restored data is
// kept under restore/failed/<id> rather than deleted: it booted far enough to
// swap in, so whatever stopped it is worth looking at.
//
// It also leaves a marker, because a silent rollback is the worst outcome of
// all: the organization serves its old data, the restore's ledger row stays
// "running", and nobody is told the restore was undone. The marker is written
// BEFORE the swapped marker goes, so a crash in between brings this function
// back rather than losing the news; the finalizer keys on the job id and writes
// one row per rollback.
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
	if err := writeRollbackMarker(dataDir, a); err != nil {
		return err
	}
	// This process serves the data the organization had before the restore, so
	// it is not in maintenance mode.
	restoring.Store(false)
	return os.Remove(swappedPathOf(dataDir))
}

func writeRollbackMarker(dataDir string, a armed) error {
	dir := rolledBackDirOf(dataDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(rolledBack{Armed: a, Reason: rollbackReason, RolledAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, a.ID+".json"), raw, 0o600)
}

// reportRollbacks turns every marker rollBack left into a failed restore row, a
// notification and an audit entry, then deletes it. Called from FinalizeRestore,
// which is the first point at which a database exists to write to.
//
// A marker whose row is already there is dropped rather than recorded twice: the
// removal is the last step, so a crash after the insert brings the marker back.
func reportRollbacks(app core.App) error {
	entries, err := os.ReadDir(rolledBackDir(app))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(rolledBackDir(app), e.Name())
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			log.Warn("could not read a rollback marker", "path", path, "err", rerr)
			continue
		}
		var rb rolledBack
		if jerr := json.Unmarshal(raw, &rb); jerr != nil {
			// An unreadable marker is removed: it cannot be acted on, and
			// keeping it would re-log the same warning on every boot forever.
			log.Error("a rollback marker was unreadable and was discarded", "path", path, "err", jerr)
			if oerr := os.Remove(path); oerr != nil {
				log.Warn("could not remove an unreadable rollback marker", "path", path, "err", oerr)
			}
			continue
		}
		if rerr := recordRollback(app, rb); rerr != nil {
			log.Error("could not record a rolled-back restore", "id", rb.Armed.ID, "err", rerr)
			continue
		}
		if oerr := os.Remove(path); oerr != nil {
			log.Warn("could not remove a rollback marker", "id", rb.Armed.ID, "err", oerr)
		}
	}
	return nil
}

func recordRollback(app core.App, rb rolledBack) error {
	done, err := app.FindRecordsByFilter(
		collection,
		"kind = 'restore' && status = 'failed' && metadata.restored_from_job = {:id}",
		"", 1, 0, dbx.Params{"id": rb.Armed.ID},
	)
	if err != nil {
		return err
	}
	if len(done) > 0 {
		return nil
	}
	reason := rb.Reason
	if reason == "" {
		reason = rollbackReason
	}
	row := newRow(app, KindRestore, "", "")
	row.Set("status", "failed")
	row.Set("finished", types.NowDateTime())
	row.Set("error", truncate(reason, 2000))
	row.Set("manifest", rb.Armed.Manifest)
	row.Set("metadata", map[string]any{"restored_from_job": rb.Armed.ID})
	if err := app.Save(row); err != nil {
		return err
	}
	announceRestore(app, RestoreRequest{}, row, false, reason)
	return nil
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
	// Reported first: a rollback is the only restore outcome nothing else can
	// tell an operator about, and it is independent of whatever the swapped
	// marker says. A failure to report it must not stop a finalize.
	if err := reportRollbacks(app); err != nil {
		log.Error("could not report rolled-back restores", "err", err)
	}
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
	// The marker is cleared last, so a crash between the insert and its removal
	// brings this function back on the next boot with the same marker. Keying on
	// the job id rather than "any succeeded restore" lets a later restore of a
	// different archive still be recorded.
	done, err := app.FindRecordsByFilter(
		collection,
		"kind = 'restore' && status = 'succeeded' && metadata.restored_from_job = {:id}",
		"", 1, 0, dbx.Params{"id": a.ID},
	)
	if err != nil {
		return err
	}
	if len(done) > 0 {
		return finishFinalize(app, a)
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
	if err := finishFinalize(app, a); err != nil {
		return err
	}
	announceRestore(app, RestoreRequest{}, row, true, "")
	return nil
}

// finishFinalize drops the safety copies and leaves maintenance mode. It runs
// both after a fresh insert and on a repeat pass whose row is already there, so
// a finalize interrupted after its insert still completes.
//
// A marker that will not go is logged rather than returned: the data is correct
// and the row is written, so refusing to serve would be worse than serving with
// a stale marker. The next boot re-runs this and, because the insert is keyed on
// the job id, writes no second row.
func finishFinalize(app core.App, a armed) error {
	if err := os.RemoveAll(previousDir(app, a.ID)); err != nil {
		log.Warn("could not remove the pre-restore copy of pb_data", "id", a.ID, "err", err)
	}
	if a.Pre != "" {
		if err := os.Remove(a.Pre); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warn("could not remove a pre-restore backup", "id", a.ID, "err", err)
		}
	}
	if err := os.Remove(swappedPath(app)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Error("could not clear the restore marker; the next boot will retry the finalize",
			"id", a.ID, "err", err)
	}
	restoring.Store(false)
	return nil
}
