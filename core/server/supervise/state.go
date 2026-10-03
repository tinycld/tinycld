// Package supervise is the supervisor that holds the public ports and swaps
// server child processes. This file holds the file-only steps of the
// armed-backup rollback protocol that moved here from config/entrypoint.sh.
package supervise

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("supervise")

// State resolves every path the entrypoint's rollback/promote steps touch,
// all rooted at Root (the entrypoint's TINYCLD_STATE_DIR, e.g. /workspace).
type State struct {
	Root string
}

func (s State) pbDataDir() string { return filepath.Join(s.Root, "pb_data") }

// currentLinkPath / previousBuildPath mirror coreserver's state_paths.go /
// rebuild_activate.go: the same files, read from the other side of the
// handoff (the supervisor reads what the Go rebuild job wrote).
func (s State) currentLinkPath() string   { return filepath.Join(s.Root, "current") }
func (s State) previousBuildPath() string { return filepath.Join(s.Root, ".previous-build") }
func (s State) buildsDir() string         { return filepath.Join(s.Root, "builds") }

// dbPath / dbBackupPath / dbArmedMarkerPath mirror coreserver's
// pkg_go_build.go dbBackupPath/dbArmedMarkerPath — same paths, kept in sync
// deliberately (see that file's comment).
func (s State) dbPath() string       { return filepath.Join(s.pbDataDir(), "data.db") }
func (s State) dbBackupPath() string { return filepath.Join(s.pbDataDir(), "data.db.backup") }
func (s State) dbArmedMarkerPath() string {
	return filepath.Join(s.pbDataDir(), ".db-backup-armed")
}

// rollbackPendingMarkerPath mirrors coreserver's pkg_rollback_reconcile.go
// rollbackPendingMarkerPath — the breadcrumb the boot reconciler consumes.
func (s State) rollbackPendingMarkerPath() string {
	return filepath.Join(s.pbDataDir(), ".rollback-pending")
}

// Current returns the build's tinycld dir that <Root>/current resolves to,
// made absolute (readlink alone can return a relative target).
func (s State) Current() (string, error) {
	dest, err := os.Readlink(s.currentLinkPath())
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(s.Root, dest)
	}
	return dest, nil
}

// BackupArmed reports whether the armed-backup marker is present and, if so,
// the build id it recorded — entrypoint.sh's recover_interrupted_rebuild
// guard (`[ -f "$DB_BACKUP_MARKER" ]`). A missing or unreadable marker is
// "not armed", matching the shell's `cat ... 2>/dev/null || echo` fallback
// for an empty-but-present marker versus a wholly absent one.
func (s State) BackupArmed() (buildID string, armed bool) {
	data, err := os.ReadFile(s.dbArmedMarkerPath())
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

// CommitBackup drops the armed snapshot + marker because the new build
// proved healthy, so the forward-migrated schema is the keeper. Idempotent,
// like entrypoint.sh's commit_db_backup: removing an absent file is not an
// error.
func (s State) CommitBackup() error {
	if err := removeIfExists(s.dbBackupPath()); err != nil {
		return err
	}
	if err := removeIfExists(s.dbArmedMarkerPath()); err != nil {
		return err
	}
	log.Info("committed migration (disarmed DB backup)")
	return nil
}

// WriteRollbackPending drops the breadcrumb the boot reconciler reads to
// mark a stranded install-log row 'rolled_back'. It captures the armed
// marker's build id BEFORE RestoreBackup clears that marker — entrypoint.sh
// calls write_rollback_pending before restore_db_from_backup for the same
// reason. A no-op (no file written) when no marker is armed, matching the
// shell's `cat ... 2>/dev/null || echo` fallback falling through to an empty
// write; Go instead skips the write entirely since there is nothing to record.
func (s State) WriteRollbackPending() error {
	buildID, armed := s.BackupArmed()
	if !armed {
		return nil
	}
	if err := os.WriteFile(s.rollbackPendingMarkerPath(), []byte(buildID), 0o644); err != nil {
		return err
	}
	log.Info("wrote rollback-pending breadcrumb for the boot reconciler", "buildID", buildID)
	return nil
}

// RestoreBackup puts the armed VACUUM-INTO snapshot back as data.db and
// clears the arm marker, mirroring entrypoint.sh's restore_db_from_backup:
//   - copy (not rename) the backup, so a failed copy leaves the backup intact
//     for a retry
//   - the copy goes to a temp file that is synced and then renamed over
//     data.db, so a kill mid-restore never leaves data.db half written; it
//     is streamed because a database can be far larger than memory
//   - remove any stale data.db-wal / data.db-shm (the snapshot has no WAL;
//     SQLite would otherwise replay stale frames over the restored file)
//   - remove the backup and its arm marker
//
// Returns an error without changing data.db when the backup is missing.
func (s State) RestoreBackup() error {
	backupPath := s.dbBackupPath()
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("no armed DB backup at %s: %w", backupPath, err)
	}
	if err := copyToTempAndRename(backupPath, s.dbPath(), s.dbPath()+".restore-tmp"); err != nil {
		return fmt.Errorf("failed to restore database from %s: %w", backupPath, err)
	}

	if err := removeIfExists(s.dbPath() + "-wal"); err != nil {
		return err
	}
	if err := removeIfExists(s.dbPath() + "-shm"); err != nil {
		return err
	}
	if err := removeIfExists(backupPath); err != nil {
		return err
	}
	if err := removeIfExists(s.dbArmedMarkerPath()); err != nil {
		return err
	}
	log.Info("database restored from armed backup; backup disarmed")
	return nil
}

// copyToTempAndRename streams src into tmp, syncs it, and renames it over
// dst. tmp is removed on any failure.
func copyToTempAndRename(src, dst, tmp string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			out.Close()
			os.Remove(tmp)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// RollbackCurrent flips <Root>/current back to the build recorded in
// <Root>/.previous-build, mirroring entrypoint.sh's rollback_current_symlink.
// The whole build tree swaps via the symlink (current.tmp then an atomic
// rename over current), never a binary move. Returns an error without
// changing anything when there is nothing to roll back to — no
// .previous-build file, or its build dir is missing.
func (s State) RollbackCurrent() error {
	data, err := os.ReadFile(s.previousBuildPath())
	if err != nil {
		return fmt.Errorf("no previous build recorded: %w", err)
	}
	prev := strings.TrimSpace(string(data))
	target := filepath.Join(s.buildsDir(), prev, "tinycld")
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("previous build %s not on disk: %w", prev, err)
	}

	tmp := s.currentLinkPath() + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.currentLinkPath()); err != nil {
		return err
	}
	log.Info("rolled back current symlink", "to", prev)
	return nil
}

// removeIfExists removes path, treating "already gone" as success — every
// entrypoint.sh cleanup step here is `rm -f`, which never fails on a missing
// file.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
