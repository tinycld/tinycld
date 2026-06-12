package coreserver

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pocketbase/pocketbase"
)

// recoverLiveDBAfterExternalWrite re-establishes the live app's SQLite
// connections after a SEPARATE process (the `migrate` subprocess, or the
// `sqlite3 VACUUM INTO` backup) opened pb_data/data.db in WAL mode.
//
// Why this is needed: PocketBase/modernc keep the WAL shared-memory index
// (data.db-shm) mmap'd for the life of the connection pool. When another process
// opens the same WAL database and resets the WAL on close (PocketBase truncate-
// checkpoints, unlinking -wal/-shm), the live server's mmap'd index is left
// pointing at a stale/unlinked inode. The live WRITE connection then validates
// that stale index against the db header on its next write and fails with
// "database disk image is malformed (11)" — even though the on-disk file is
// perfectly intact (reads, served from a separate snapshot, keep working; the
// first post-migrate app.Save is what breaks). This bites only when pb_data is a
// bind-mount/overlay where the unlink-then-recreate isn't transparent to the
// holder of the old mmap, which is exactly the operator's deployment.
//
// app.Bootstrap() tears down and re-opens both DB pools (initDataDB/initAuxDB),
// dropping the stale mmap and re-reading the current WAL index, then reloads the
// cached collections + settings so the rest of the pipeline sees a consistent
// app. It re-fires OnBootstrap hooks, which is acceptable here: the install job
// is mid-flight on a background goroutine, no request is being served against
// these specific writes, and the process restarts moments later anyway.
func recoverLiveDBAfterExternalWrite(app *pocketbase.PocketBase) error {
	log.Printf("pkg_install: reconnecting live DB after external WAL access")
	if err := app.Bootstrap(); err != nil {
		return fmt.Errorf("re-bootstrap app DB after migrate: %w", err)
	}
	return nil
}

// checkpointWAL flushes the write-ahead log into the main data.db file. The
// rebuild restart path os.Exit(75)'s the process immediately after the final
// DB writes (install-log finalize, registry mirror). In WAL mode a committed
// transaction lives in the -wal file until a checkpoint folds it into data.db;
// a hard os.Exit before that checkpoint leaves the new binary reading a data.db
// that's missing those writes (observed: pkg_install_log stuck at "running").
// TRUNCATE forces a full checkpoint and resets the WAL so the next process sees
// every committed write. Best-effort: a checkpoint failure is logged, not fatal.
func checkpointWAL(app *pocketbase.PocketBase) {
	if _, err := app.DB().NewQuery("PRAGMA wal_checkpoint(TRUNCATE)").Execute(); err != nil {
		log.Printf("pkg_install: WAL checkpoint before restart failed: %v", err)
	}
}

// checkGoBuildPrereqs verifies that Go and gcc are available on PATH.
func checkGoBuildPrereqs() error {
	for _, tool := range []string{"go", "gcc"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found on PATH: %w", tool, err)
		}
	}
	return nil
}

// dbBackupPath / dbArmedMarkerPath are the deterministic locations the armed-
// backup rollback protocol shares with the entrypoint (config/entrypoint.sh).
// Both live under statePbDataDir() so they survive the per-build symlink swap.
// The entrypoint hard-codes the same paths as $PB_DATA_DIR/data.db.backup and
// $PB_DATA_DIR/.db-backup-armed — keep the two in sync.
func dbBackupPath() string      { return filepath.Join(statePbDataDir(), "data.db.backup") }
func dbArmedMarkerPath() string { return filepath.Join(statePbDataDir(), ".db-backup-armed") }

// armDatabaseBackup records the build id that owns the surviving data.db.backup
// just before the rebuild success path exits 75. The backup itself is left in
// place (NOT deleted) so it survives the restart as a rollback snapshot; the
// marker tells the entrypoint two things it can't otherwise know: (1) the backup
// is intentionally armed (awaiting a post-boot health verdict), not a stale
// leftover, and (2) which build it predates — so a SIGKILL mid-rebuild leaves an
// unambiguous "restore me if `current` already points past this build" signal.
//
// Why marker-gated rather than just "leave the file": the file alone can't tell
// the entrypoint whether the new binary already booted healthy (commit) or never
// did (rollback). The entrypoint deletes BOTH file and marker on a confirmed-
// healthy boot ("commit"); restores from the file and clears the marker on a
// failed probe ("rollback"). Best-effort: a marker write failure is logged, not
// fatal — the rollback restore still works, only the SIGKILL-recovery heuristic
// degrades.
func armDatabaseBackup(buildID string) {
	if _, err := os.Stat(dbBackupPath()); err != nil {
		// No backup on disk (e.g. an uninstall rebuild that never migrated, or a
		// dev run). Nothing to arm; make sure no stale marker lingers.
		_ = os.Remove(dbArmedMarkerPath())
		return
	}
	if err := os.WriteFile(dbArmedMarkerPath(), []byte(buildID), 0o644); err != nil {
		log.Printf("[pkg_install] WARNING: failed to arm DB backup marker (rollback still works; SIGKILL-recovery degraded): %v", err)
		return
	}
	log.Printf("[pkg_install] DB backup armed for build %s (entrypoint commits on healthy boot, restores on failed probe)", buildID)
}

// backupDatabase snapshots the live DB and returns a restore closure. The DB
// lives under the STATE dir (resolveStateDir()), not the build/binary dir, so
// it persists across the per-build symlink swap. The legacy appDir parameter
// is retained for caller compatibility but no longer used for path resolution.
func backupDatabase(_ string) (rollbackFn func() error, err error) {
	dbPath := filepath.Join(statePbDataDir(), "data.db")
	backupPath := dbBackupPath()

	// SQLite's VACUUM INTO refuses to overwrite an existing file ("output file
	// already exists"). A prior install/revert leaves data.db.backup behind, so
	// clear any stale copy before snapshotting.
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to clear stale backup %s: %w", backupPath, err)
	}

	// Use sqlite3 VACUUM INTO for a consistent snapshot
	cmd := exec.Command("sqlite3", dbPath, fmt.Sprintf("VACUUM INTO '%s';", backupPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("database backup failed: %v\n%s", err, out)
	}

	var sizeNote string
	if fi, statErr := os.Stat(backupPath); statErr == nil {
		sizeNote = fmt.Sprintf(" (%.1f MB)", float64(fi.Size())/(1024*1024))
	}
	log.Printf("[pkg_install] database backed up to %s%s (restore-on-failure armed)", backupPath, sizeNote)

	// In-process restore, used ONLY for PRE-activation failures (migrate/activate
	// fail before the symlink swaps). It restores the live DB and disarms — both
	// the backup file and any arm marker — because the live `current` never moved,
	// so there is no post-restart rollback to keep the backup for. The success
	// path deliberately does NOT call this: it leaves the backup ARMED (via
	// armDatabaseBackup) so the entrypoint can roll the DB back if the new binary
	// fails its post-restart health probe (the failure happens in a different
	// process, so it can't be handled here). See entrypoint.sh's rollback branch.
	rollbackFn = func() error {
		log.Printf("pkg_go_build: restoring database from backup")
		// Use cp for streaming copy to avoid loading entire DB into memory
		cmd := exec.Command("cp", backupPath, dbPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to restore backup: %v\n%s", err, out)
		}
		// The backup is a VACUUM-INTO snapshot (no WAL). Any data.db-wal/-shm next
		// to the live DB belong to the now-overwritten (forward-migrated) file; if
		// left, SQLite would replay those frames over the restored snapshot and
		// silently undo the restore (or trip "disk image is malformed"). Drop them
		// — the caller re-bootstraps the pools (recoverDB) so no live connection is
		// mid-checkpoint against them. Mirrors entrypoint.sh's restore_db_from_backup.
		dbWAL := dbPath + "-wal"
		dbSHM := dbPath + "-shm"
		if err := os.Remove(dbWAL); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to clear stale WAL %s: %w", dbWAL, err)
		}
		if err := os.Remove(dbSHM); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to clear stale SHM %s: %w", dbSHM, err)
		}
		os.Remove(backupPath)
		os.Remove(dbArmedMarkerPath())
		return nil
	}

	return rollbackFn, nil
}

// swapBinary atomically swaps the current binary with the new one.
// Returns a rollback function that reverses the swap. Uses the configured
// `binaryName` (set via Options.BinaryName at Register time) to locate
// the binary; the new and previous copies have `.new` and `.prev`
// suffixes.

// swapToArchivedBinary installs an archived build's binary as the live one,
// keeping the current binary as <binary>.prev so the entrypoint's health-check
// can roll back to it if the reverted binary fails to boot. The archived binary
// is copied (not moved) so the build archive stays intact and re-revertible.
// Returns a rollback function that reverses the swap.
