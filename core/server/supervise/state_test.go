package supervise

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// newTestState builds a Root matching the real layout:
//
//	pb_data/data.db
//	pb_data/data.db.backup
//	pb_data/.db-backup-armed
//	builds/<id>/tinycld/tinycld
//	current -> builds/<id>/tinycld
func newTestState(t *testing.T) State {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pb_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	return State{Root: root}
}

func writeBuild(t *testing.T, s State, buildID string) string {
	t.Helper()
	dir := filepath.Join(s.buildsDir(), buildID, "tinycld")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tinycld"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func pointCurrentAt(t *testing.T, s State, buildDir string) {
	t.Helper()
	if err := os.Symlink(buildDir, s.currentLinkPath()); err != nil {
		t.Fatal(err)
	}
}

func TestState_Current(t *testing.T) {
	s := newTestState(t)
	buildDir := writeBuild(t, s, "build-1")
	pointCurrentAt(t, s, buildDir)

	got, err := s.Current()
	if err != nil {
		t.Fatal(err)
	}
	if got != buildDir {
		t.Fatalf("Current() = %q, want %q", got, buildDir)
	}
}

func TestState_Current_NoSymlink(t *testing.T) {
	s := newTestState(t)
	if _, err := s.Current(); err == nil {
		t.Fatal("Current() should error when current is unset")
	}
}

func TestState_BackupArmed(t *testing.T) {
	s := newTestState(t)

	if _, armed := s.BackupArmed(); armed {
		t.Fatal("BackupArmed() should be false with no marker")
	}

	if err := os.WriteFile(s.dbArmedMarkerPath(), []byte("build-42"), 0o644); err != nil {
		t.Fatal(err)
	}
	buildID, armed := s.BackupArmed()
	if !armed || buildID != "build-42" {
		t.Fatalf("BackupArmed() = %q, %v; want build-42, true", buildID, armed)
	}
}

func armBackup(t *testing.T, s State, buildID string, dbBytes []byte) {
	t.Helper()
	if err := os.WriteFile(s.dbBackupPath(), dbBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.dbArmedMarkerPath(), []byte(buildID), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestState_CommitBackup(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-7", []byte("snapshot"))
	if err := os.WriteFile(s.dbPath(), []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.CommitBackup(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(s.dbBackupPath()); !os.IsNotExist(err) {
		t.Fatalf("backup should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(s.dbArmedMarkerPath()); !os.IsNotExist(err) {
		t.Fatalf("marker should be gone, stat err = %v", err)
	}
	data, err := os.ReadFile(s.dbPath())
	if err != nil || string(data) != "live" {
		t.Fatalf("data.db should be untouched: data=%q err=%v", data, err)
	}
}

func TestState_CommitBackup_NoOpWithoutMarker(t *testing.T) {
	s := newTestState(t)
	if err := s.CommitBackup(); err != nil {
		t.Fatalf("CommitBackup() on a clean state should not error: %v", err)
	}
}

func TestState_WriteRollbackPending(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-9", []byte("snapshot"))

	if err := s.WriteRollbackPending(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(s.rollbackPendingMarkerPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "build-9" {
		t.Fatalf("rollback-pending marker = %q, want build-9", data)
	}
}

func TestState_WriteRollbackPending_NoOpWithoutArmedMarker(t *testing.T) {
	s := newTestState(t)
	if err := s.WriteRollbackPending(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.rollbackPendingMarkerPath()); !os.IsNotExist(err) {
		t.Fatalf("rollback-pending marker should not be written, stat err = %v", err)
	}
}

func TestState_RestoreBackup(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-3", []byte("snapshot-bytes"))
	if err := os.WriteFile(s.dbPath(), []byte("forward-migrated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.dbPath()+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.dbPath()+"-shm", []byte("shm"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.RestoreBackup(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(s.dbPath())
	if err != nil || string(data) != "snapshot-bytes" {
		t.Fatalf("data.db should hold the backup's bytes: data=%q err=%v", data, err)
	}
	for _, p := range []string{s.dbPath() + "-wal", s.dbPath() + "-shm", s.dbBackupPath(), s.dbArmedMarkerPath()} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone, stat err = %v", p, err)
		}
	}
}

func TestState_RestoreBackup_MissingBackup(t *testing.T) {
	s := newTestState(t)
	if err := os.WriteFile(s.dbPath(), []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.RestoreBackup(); err == nil {
		t.Fatal("RestoreBackup() should error when the backup is missing")
	}

	data, err := os.ReadFile(s.dbPath())
	if err != nil || string(data) != "live" {
		t.Fatalf("data.db should be untouched on a missing backup: data=%q err=%v", data, err)
	}
}

func TestState_RollbackCurrent(t *testing.T) {
	s := newTestState(t)
	prevDir := writeBuild(t, s, "build-old")
	newDir := writeBuild(t, s, "build-new")
	pointCurrentAt(t, s, newDir)
	if err := os.WriteFile(s.previousBuildPath(), []byte("build-old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.RollbackCurrent(); err != nil {
		t.Fatal(err)
	}

	got, err := s.Current()
	if err != nil {
		t.Fatal(err)
	}
	if got != prevDir {
		t.Fatalf("Current() after rollback = %q, want %q", got, prevDir)
	}
}

func TestState_RollbackCurrent_NoPreviousBuildMarker(t *testing.T) {
	s := newTestState(t)
	newDir := writeBuild(t, s, "build-new")
	pointCurrentAt(t, s, newDir)

	if err := s.RollbackCurrent(); err == nil {
		t.Fatal("RollbackCurrent() should error without a .previous-build marker")
	}
}

func TestState_RollbackCurrent_PreviousBuildMissingOnDisk(t *testing.T) {
	s := newTestState(t)
	newDir := writeBuild(t, s, "build-new")
	pointCurrentAt(t, s, newDir)
	if err := os.WriteFile(s.previousBuildPath(), []byte("build-ghost"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.RollbackCurrent(); err == nil {
		t.Fatal("RollbackCurrent() should error when the previous build's binary does not exist")
	}

	got, err := s.Current()
	if err != nil {
		t.Fatal(err)
	}
	if got != newDir {
		t.Fatalf("Current() should be unchanged on a failed rollback, got %q", got)
	}
}

// The rebuild writes these ids with whatever trailing newline its writer
// left, and the shell's $(cat ...) stripped it; a raw read would look for a
// build dir named "build-old\n".
func TestState_RollbackCurrent_TrimsTrailingNewline(t *testing.T) {
	s := newTestState(t)
	prevDir := writeBuild(t, s, "build-old")
	newDir := writeBuild(t, s, "build-new")
	pointCurrentAt(t, s, newDir)
	if err := os.WriteFile(s.previousBuildPath(), []byte("build-old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.RollbackCurrent(); err != nil {
		t.Fatal(err)
	}
	got, err := s.Current()
	if err != nil {
		t.Fatal(err)
	}
	if got != prevDir {
		t.Fatalf("Current() after rollback = %q, want %q", got, prevDir)
	}
}

func TestState_BackupArmed_TrimsTrailingNewline(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-2\n", []byte("snap"))

	id, armed := s.BackupArmed()
	if !armed || id != "build-2" {
		t.Fatalf("BackupArmed() = (%q, %v), want (build-2, true)", id, armed)
	}
}

// A kill during the restore must never leave data.db half written: the
// backup goes to a temp file that renames over data.db in one step. A
// handle on the old data.db therefore still reads the old bytes.
func TestState_RestoreBackup_RenamesOverDataDB(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-3", []byte("snapshot-bytes"))
	if err := os.WriteFile(s.dbPath(), []byte("forward-migrated"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(s.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(s.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()

	if err := s.RestoreBackup(); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(s.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("data.db was rewritten in place; the backup must rename over it")
	}
	oldBytes, err := io.ReadAll(old)
	if err != nil || string(oldBytes) != "forward-migrated" {
		t.Fatalf("a handle on the old data.db read %q (err %v)", oldBytes, err)
	}
	if got := mustRead(t, s.dbPath()); got != "snapshot-bytes" {
		t.Fatalf("data.db = %q, want snapshot-bytes", got)
	}
	if _, err := os.Stat(s.dbPath() + ".restore-tmp"); !os.IsNotExist(err) {
		t.Fatalf("restore temp file should be gone, stat err = %v", err)
	}
}

// The supervisor runs as root and its children do not, so the restored
// data.db must keep the mode (and owner, see state_unix_test.go) of the file
// it replaces; a fresh 0644 root file is one the next child cannot write.
func TestState_RestoreBackup_KeepsDataDBMode(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-3", []byte("snapshot-bytes"))
	if err := os.WriteFile(s.dbPath(), []byte("forward-migrated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.dbPath(), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.RestoreBackup(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("restored data.db mode = %v, want 0600 (the replaced file's)", info.Mode().Perm())
	}
}

// With no data.db to copy from, the backup's mode is the next best guess:
// the same process wrote it beside data.db.
func TestState_RestoreBackup_ModeFromBackupWhenDataDBMissing(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-3", []byte("snapshot-bytes"))
	if err := os.Chmod(s.dbBackupPath(), 0o640); err != nil {
		t.Fatal(err)
	}
	os.Remove(s.dbPath())

	if err := s.RestoreBackup(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("restored data.db mode = %v, want 0640 (the backup's)", info.Mode().Perm())
	}
}

// The stale WAL must be gone before the restored file takes data.db's name:
// a kill between the two would otherwise leave the migrated database's WAL
// beside the restored file, and SQLite would replay it over the snapshot.
func TestState_RestoreBackup_RemovesWALBeforeRename(t *testing.T) {
	s := newTestState(t)
	armBackup(t, s, "build-3", []byte("snapshot-bytes"))
	mustWrite(t, s.dbPath(), "forward-migrated")
	mustWrite(t, s.dbPath()+"-wal", "wal")
	mustWrite(t, s.dbPath()+"-shm", "shm")

	var leftAtRename []string
	prev := renameFile
	renameFile = func(from, to string) error {
		for _, p := range []string{s.dbPath() + "-wal", s.dbPath() + "-shm"} {
			if _, err := os.Stat(p); err == nil {
				leftAtRename = append(leftAtRename, filepath.Base(p))
			}
		}
		return prev(from, to)
	}
	t.Cleanup(func() { renameFile = prev })

	if err := s.RestoreBackup(); err != nil {
		t.Fatal(err)
	}
	if len(leftAtRename) != 0 {
		t.Fatalf("%v still existed when the restored file was renamed over data.db", leftAtRename)
	}
}
