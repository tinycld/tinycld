// Package supervise is the supervisor that holds the public ports and swaps
// server child processes. This file holds the file-only steps of the
// armed-backup rollback protocol that moved here from config/entrypoint.sh.
package supervise

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// rollbackRecordPath is where a rollback leaves its RollbackRecord. It is
// beside pb_data, not in it: a restore swap moves pb_data aside whole, and
// would carry a record inside it away from the boot that reads it.
func (s State) rollbackRecordPath() string {
	return filepath.Join(s.Root, ".rollback-pending")
}

// RollbackRecord is the note a rollback leaves for the next boot's install
// log. JSON in <Root>/.rollback-pending (outside pb_data, so a restore swap
// cannot carry it away).
type RollbackRecord struct {
	Build    string    `json:"build"`     // the build that failed and was rolled back from
	RolledTo string    `json:"rolled_to"` // the build serving after the rollback ("" if unknown)
	At       time.Time `json:"at"`
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

// WriteRollbackRecord writes r to <Root>/.rollback-pending, replacing any
// earlier record. The server reads it at boot, perhaps while this write
// runs, so the bytes go to a temp file that is renamed over the record and
// a reader never sees half of one.
func (s State) WriteRollbackRecord(r RollbackRecord) (err error) {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("supervise: encode the rollback record: %w", err)
	}
	rootInfo, err := os.Stat(s.Root)
	if err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	path := s.rollbackRecordPath()
	tmp, err := os.CreateTemp(s.Root, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	if err = tmp.Chmod(0o644); err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	// The supervisor runs as root; the server that reads and then removes
	// the record does not.
	if err = matchOwner(tmp.Name(), rootInfo); err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("supervise: write the rollback record: %w", err)
	}
	log.Info("recorded the rollback for the next boot", "build", r.Build, "rolledTo", r.RolledTo)
	return nil
}

// RestoreBackup puts the armed VACUUM-INTO snapshot back as data.db and
// clears the arm marker, mirroring entrypoint.sh's restore_db_from_backup:
//   - copy (not rename) the backup, so a failed copy leaves the backup intact
//     for a retry
//   - the copy goes to a temp file that is synced and then renamed over
//     data.db, so a kill mid-restore never leaves data.db half written; it
//     is streamed because a database can be far larger than memory
//   - the temp file takes the owner and mode of the data.db it replaces (or
//     of the backup when there is none): the supervisor runs as root and its
//     children do not, and a root-owned data.db is one they cannot write
//   - remove any stale data.db-wal / data.db-shm before the rename (the
//     snapshot has no WAL; SQLite would otherwise replay the migrated
//     database's frames over the restored file, and a kill between a rename
//     and a later removal would leave exactly that)
//   - after the rename, remove the backup and its arm marker
//
// Returns an error without changing data.db when the backup is missing.
func (s State) RestoreBackup() error {
	backupPath := s.dbBackupPath()
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("no armed DB backup at %s: %w", backupPath, err)
	}
	ref := backupInfo
	if info, err := os.Stat(s.dbPath()); err == nil {
		ref = info
	}
	removeWAL := func() error {
		if err := removeIfExists(s.dbPath() + "-wal"); err != nil {
			return err
		}
		return removeIfExists(s.dbPath() + "-shm")
	}
	if err := copyToTempAndRename(backupPath, s.dbPath(), s.dbPath()+".restore-tmp", ref, removeWAL); err != nil {
		return fmt.Errorf("failed to restore database from %s: %w", backupPath, err)
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

// renameFile is os.Rename behind a seam, so a test can see what is on disk
// at the moment of the rename.
var renameFile = os.Rename

// copyToTempAndRename streams src into tmp, syncs it, gives it ref's mode
// and owner, runs beforeRename, and renames tmp over dst. tmp is removed on
// any failure.
func copyToTempAndRename(src, dst, tmp string, ref os.FileInfo, beforeRename func() error) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
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
	if err = out.Chmod(ref.Mode().Perm()); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = matchOwner(tmp, ref); err != nil {
		return err
	}
	if err = beforeRename(); err != nil {
		return err
	}
	return renameFile(tmp, dst)
}

// RollbackCurrent flips <Root>/current back to the build recorded in
// <Root>/.previous-build, mirroring entrypoint.sh's rollback_current_symlink.
// The whole build tree swaps via the symlink (current.tmp then an atomic
// rename over current), never a binary move. Returns an error without
// changing anything when there is nothing to roll back to — no
// .previous-build file, or its build dir is missing.
func (s State) RollbackCurrent() error {
	prev, err := s.PreviousBuild()
	if err != nil {
		return err
	}
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

// PreviousBuild is the build id <Root>/.previous-build records: the build
// RollbackCurrent would flip current back to.
func (s State) PreviousBuild() (string, error) {
	data, err := os.ReadFile(s.previousBuildPath())
	if err != nil {
		return "", fmt.Errorf("no previous build recorded: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
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
