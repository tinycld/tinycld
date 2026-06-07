package coreserver

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

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
func backupDatabase(appDir string) (rollbackFn func() error, err error) {
	dbPath := filepath.Join(appDir, "pb_data", "data.db")
	backupPath := filepath.Join(appDir, "pb_data", "data.db.backup")

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
