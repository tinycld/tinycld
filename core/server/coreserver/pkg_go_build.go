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

// backupDatabase snapshots the live DB and returns a restore closure. The DB
// lives under the STATE dir (resolveStateDir()), not the build/binary dir, so
// it persists across the per-build symlink swap. The legacy appDir parameter
// is retained for caller compatibility but no longer used for path resolution.
func backupDatabase(_ string) (rollbackFn func() error, err error) {
	dbPath := filepath.Join(statePbDataDir(), "data.db")
	backupPath := filepath.Join(statePbDataDir(), "data.db.backup")

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

	rollbackFn = func() error {
		log.Printf("pkg_go_build: restoring database from backup")
		// Use cp for streaming copy to avoid loading entire DB into memory
		cmd := exec.Command("cp", backupPath, dbPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to restore backup: %v\n%s", err, out)
		}
		os.Remove(backupPath)
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
