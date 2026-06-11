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

// buildNewBinary compiles a new server binary at outDir/tinycld.new. The
// compile runs in goSrcDir (where go.work + the app's main package live) but
// the output lands in outDir so it sits next to the live binary ready for
// swapBinary. In the standalone-workspace image goSrcDir is <appDir>/server
// and outDir is <appDir>.
func buildNewBinary(goSrcDir, outDir string) error {
	outPath := filepath.Join(outDir, "tinycld.new")
	cmd := exec.Command("go", "build", "-o", outPath, ".")
	cmd.Dir = goSrcDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go build failed: %v\n%s", err, out)
	}
	log.Printf("pkg_go_build: built tinycld.new in %s (compiled from %s)", outDir, goSrcDir)
	return nil
}

// validateBinary runs the new binary with --help to verify it starts.
func validateBinary(binaryPath string) error {
	cmd := exec.Command(binaryPath, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("binary validation failed: %v\n%s", err, out)
	}
	return nil
}

// backupDatabase creates a consistent SQLite backup using VACUUM INTO.
// Returns a rollback function that restores the backup.
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

	log.Printf("pkg_go_build: database backed up to %s", backupPath)

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
func swapBinary(appDir string) (rollbackFn func() error, err error) {
	currentPath := filepath.Join(appDir, binaryName)
	prevPath := filepath.Join(appDir, binaryName+".prev")
	newPath := filepath.Join(appDir, binaryName+".new")

	// <binary> → <binary>.prev
	if err := os.Rename(currentPath, prevPath); err != nil {
		return nil, fmt.Errorf("failed to move current binary to .prev: %w", err)
	}

	// <binary>.new → <binary>
	if err := os.Rename(newPath, currentPath); err != nil {
		// Try to restore
		os.Rename(prevPath, currentPath)
		return nil, fmt.Errorf("failed to move new binary into place: %w", err)
	}

	log.Printf("pkg_go_build: binary swap complete (prev saved as %s.prev)", binaryName)

	rollbackFn = func() error {
		log.Printf("pkg_go_build: rolling back binary swap")
		failedPath := filepath.Join(appDir, "tinycld.failed")
		os.Rename(currentPath, failedPath)
		return os.Rename(prevPath, currentPath)
	}

	return rollbackFn, nil
}

// swapToArchivedBinary installs an archived build's binary as the live one,
// keeping the current binary as <binary>.prev so the entrypoint's health-check
// can roll back to it if the reverted binary fails to boot. The archived binary
// is copied (not moved) so the build archive stays intact and re-revertible.
// Returns a rollback function that reverses the swap.
func swapToArchivedBinary(appDir, archivedBinary string) (rollbackFn func() error, err error) {
	currentPath := filepath.Join(appDir, binaryName)
	prevPath := filepath.Join(appDir, binaryName+".prev")

	if _, statErr := os.Stat(archivedBinary); statErr != nil {
		return nil, fmt.Errorf("archived binary not found: %w", statErr)
	}

	// <binary> → <binary>.prev
	if err := os.Rename(currentPath, prevPath); err != nil {
		return nil, fmt.Errorf("failed to move current binary to .prev: %w", err)
	}

	// copy archived → <binary>
	cp := exec.Command("cp", "-a", archivedBinary, currentPath)
	if out, err := cp.CombinedOutput(); err != nil {
		os.Rename(prevPath, currentPath) // restore
		return nil, fmt.Errorf("failed to install archived binary: %v\n%s", err, out)
	}

	log.Printf("pkg_go_build: swapped in archived binary %s", archivedBinary)

	rollbackFn = func() error {
		log.Printf("pkg_go_build: rolling back archived-binary swap")
		failedPath := filepath.Join(appDir, "tinycld.failed")
		os.Rename(currentPath, failedPath)
		return os.Rename(prevPath, currentPath)
	}

	return rollbackFn, nil
}
