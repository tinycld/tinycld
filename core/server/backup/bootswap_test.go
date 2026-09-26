package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// layout builds the on-disk shape a restore leaves behind at phase 4/5: a live
// pb_data, a fully staged pending directory, and the armed marker beside them.
func layout(t *testing.T) (dataDir string) {
	t.Helper()
	root := t.TempDir()
	dataDir = filepath.Join(root, "pb_data")
	mk := func(p, content string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join(dataDir, "data.db"), "old")
	mk(filepath.Join(dataDir, "storage", "a", "b", "old.txt"), "old")
	mk(filepath.Join(dataDir, "auxiliary.db"), "aux")
	pending := filepath.Join(root, "restore", "pending", "r1")
	mk(filepath.Join(pending, "data.db"), "new")
	mk(filepath.Join(pending, "storage", "c", "d", "new.txt"), "new")
	mk(filepath.Join(pending, stagedSentinel), "")
	raw, err := json.Marshal(armed{ID: "r1", Pending: pending, Pre: filepath.Join(root, "restore", "pre", "r1.age")})
	if err != nil {
		t.Fatal(err)
	}
	mk(armedPathOf(dataDir), string(raw))
	return dataDir
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func TestApplyPendingRestoreSwaps(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "new" {
		t.Fatal("db not swapped")
	}
	if read(t, filepath.Join(dataDir, "storage", "c", "d", "new.txt")) != "new" {
		t.Fatal("files not swapped")
	}
	if read(t, filepath.Join(dataDir, "storage", "a", "b", "old.txt")) != "<missing>" {
		t.Fatal("old files still present")
	}
	// auxiliary.db went aside with the rest of pb_data; PocketBase recreates it.
	if _, err := os.Stat(filepath.Join(dataDir, "auxiliary.db")); !os.IsNotExist(err) {
		t.Fatal("the discarded auxiliary database was left behind")
	}
	if _, err := os.Stat(armedPathOf(dataDir)); !os.IsNotExist(err) {
		t.Fatal("armed marker not cleared")
	}
	prev := filepath.Join(filepath.Dir(dataDir), "restore", "previous", "r1")
	if read(t, filepath.Join(prev, "data.db")) != "old" {
		t.Fatal("previous not kept")
	}
	if read(t, filepath.Join(prev, "auxiliary.db")) != "aux" {
		t.Fatal("previous must carry everything pb_data held")
	}
	if read(t, swappedPathOf(dataDir)) == "<missing>" {
		t.Fatal("swapped marker missing")
	}
	if !Restoring() {
		t.Fatal("Restoring() must be true until finalize")
	}
}

func TestApplyPendingRestoreNoop(t *testing.T) {
	resetRestoreState(t)
	dataDir := filepath.Join(t.TempDir(), "pb_data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if Restoring() {
		t.Fatal("a boot with nothing staged must not enter maintenance mode")
	}
}

// A first boot on a data dir that does not exist yet must not fail: the swap
// runs before PocketBase creates it.
func TestApplyPendingRestoreMissingDataDir(t *testing.T) {
	resetRestoreState(t)
	dataDir := filepath.Join(t.TempDir(), "pb_data")
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
}

// The armed marker is written before staging finishes, so it can name a pending
// directory that was never completed. Staging removes nothing, so the sentinel's
// absence is the only sound evidence: a missing data.db can equally mean the
// swap already moved it in.
func TestApplyPendingRestoreDiscardsAnUnstagedPending(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	pending := filepath.Join(filepath.Dir(dataDir), "restore", "pending", "r1")
	if err := os.Remove(filepath.Join(pending, stagedSentinel)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(pending, "data.db")); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "old" {
		t.Fatal("the live database must be left alone")
	}
	if _, err := os.Stat(armedPathOf(dataDir)); !os.IsNotExist(err) {
		t.Fatal("armed marker not cleared")
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatal("the half-staged directory was kept")
	}
	if Restoring() {
		t.Fatal("an aborted stage must not enter maintenance mode")
	}
}

func TestApplyPendingRestoreRecoversFromHalfSwap(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	// Simulate a crash after pb_data was moved aside but before pending moved in.
	prev := filepath.Join(filepath.Dir(dataDir), "restore", "previous", "r1")
	if err := os.MkdirAll(filepath.Dir(prev), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dataDir, prev); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "new" {
		t.Fatal("did not complete the swap")
	}
	if read(t, filepath.Join(prev, "data.db")) != "old" {
		t.Fatal("the previous copy must survive the completed swap")
	}
}

func TestApplyPendingRestoreRollsBackAfterFailedBoot(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	// The process boots, fails before FinalizeRestore, and is relaunched:
	// the swapped marker is still there, so the next boot must swap back.
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "old" {
		t.Fatal("did not roll back")
	}
	if read(t, filepath.Join(dataDir, "auxiliary.db")) != "aux" {
		t.Fatal("the rolled-back pb_data lost a member")
	}
	if _, err := os.Stat(swappedPathOf(dataDir)); !os.IsNotExist(err) {
		t.Fatal("swapped marker not cleared after rollback")
	}
	if read(t, filepath.Join(filepath.Dir(dataDir), "restore", "failed", "r1", "data.db")) != "new" {
		t.Fatal("failed restore data not kept for inspection")
	}
	if Restoring() {
		t.Fatal("a rolled-back boot serves the previous data, so it is not restoring")
	}
}

// A second failed boot must not trip over the failed/<id> directory the first
// one left behind.
func TestApplyPendingRestoreRollsBackTwice(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	// Re-arm with a freshly staged pending directory and fail again with the
	// same id, so the second rollback meets the failed/<id> the first one left.
	pending := filepath.Join(filepath.Dir(dataDir), "restore", "pending", "r1")
	if err := os.MkdirAll(filepath.Join(pending, "storage"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending, "data.db"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	markStaged(t, pending)
	raw, err := json.Marshal(armed{ID: "r1", Pending: pending})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(armedPathOf(dataDir), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "old" {
		t.Fatal("did not roll back the second attempt")
	}
}

// The two markers overlap for one instant. A crash inside that window leaves
// both, and swapped must win: the data is already restored.
func TestApplyPendingRestorePrefersTheSwappedMarker(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	// Put the armed marker back, as a crash between the two writes would. It
	// names the pending directory the swap already consumed.
	raw, err := json.Marshal(armed{ID: "r1", Pending: filepath.Join(filepath.Dir(dataDir), "restore", "pending", "r1")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(armedPathOf(dataDir), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "old" {
		t.Fatal("the swapped marker must decide: the data was already restored")
	}
	if _, err := os.Stat(armedPathOf(dataDir)); !os.IsNotExist(err) {
		t.Fatal("the stale armed marker would re-restore on the next boot")
	}
}

// A rollback exists to get the previous copy back. It must still do that if
// something removed pb_data between the two boots.
func TestApplyPendingRestoreRollsBackWithoutPbData(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "old" {
		t.Fatal("the previous copy was not restored")
	}
}

// stagePending completes a staged directory the way phase 5 does, so a test's
// layout is one the boot swap accepts.
func markStaged(t *testing.T, pending string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(pending, stagedSentinel), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A crash between swapIn's two member renames leaves data.db in the new pb_data
// and storage/ still in pending. The staged database is gone from pending, but
// previous/<id> exists — the swap started, so the next boot must finish it. The
// old code read a missing <pending>/data.db as an aborted stage and deleted the
// staged storage tree, losing every attachment in the archive.
func TestApplyPendingRestoreResumesAfterTheDatabaseMoved(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	root := filepath.Dir(dataDir)
	pending := filepath.Join(root, "restore", "pending", "r1")
	prev := filepath.Join(root, "restore", "previous", "r1")

	// Replay swapIn up to and including the data.db rename, then stop.
	if err := os.MkdirAll(filepath.Dir(prev), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dataDir, prev); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(pending, "data.db"), filepath.Join(dataDir, "data.db")); err != nil {
		t.Fatal(err)
	}

	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "new" {
		t.Fatal("the database that was already moved in was discarded")
	}
	if read(t, filepath.Join(dataDir, "storage", "c", "d", "new.txt")) != "new" {
		t.Fatal("the staged attachments were lost")
	}
	if read(t, swappedPathOf(dataDir)) == "<missing>" {
		t.Fatal("the resumed swap left no marker, so a failed boot could not roll back")
	}
	if _, err := os.Stat(armedPathOf(dataDir)); !os.IsNotExist(err) {
		t.Fatal("armed marker not cleared")
	}
	if !Restoring() {
		t.Fatal("a resumed swap is still a restore awaiting finalize")
	}
}

// A crash after both members moved but before the swapped marker was written.
// Pending is empty, previous/<id> exists: finish by writing the marker.
func TestApplyPendingRestoreResumesAfterBothMembersMoved(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	root := filepath.Dir(dataDir)
	pending := filepath.Join(root, "restore", "pending", "r1")
	prev := filepath.Join(root, "restore", "previous", "r1")

	if err := os.MkdirAll(filepath.Dir(prev), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dataDir, prev); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"data.db", "storage"} {
		if err := os.Rename(filepath.Join(pending, name), filepath.Join(dataDir, name)); err != nil {
			t.Fatal(err)
		}
	}

	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "new" || read(t, filepath.Join(dataDir, "storage", "c", "d", "new.txt")) != "new" {
		t.Fatal("the restored data was disturbed")
	}
	if read(t, swappedPathOf(dataDir)) == "<missing>" {
		t.Fatal("the swap was never marked, so a failed boot could not roll back")
	}
	if !Restoring() {
		t.Fatal("a resumed swap is still a restore awaiting finalize")
	}
}

// An armed marker whose pending directory has no sentinel and no previous/<id>
// beside it: staging never finished and the swap never started. Discard.
func TestApplyPendingRestoreDiscardsAStageWithoutItsSentinel(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	pending := filepath.Join(filepath.Dir(dataDir), "restore", "pending", "r1")
	if err := os.Remove(filepath.Join(pending, stagedSentinel)); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "old" {
		t.Fatal("a stage that never finished must not be swapped in")
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatal("the half-staged directory was kept")
	}
	if _, err := os.Stat(armedPathOf(dataDir)); !os.IsNotExist(err) {
		t.Fatal("armed marker not cleared")
	}
	if Restoring() {
		t.Fatal("an aborted stage must not enter maintenance mode")
	}
}

// A rollback with no marker is a silent one: the organization serves its old
// data, the restore's row stays "running", and nobody is told the restore was
// undone. The rollback runs before the database is open, so the marker is the
// only way the news reaches the ledger.
func TestApplyPendingRestoreLeavesARollbackMarker(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rolledBackDirOf(dataDir), "r1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no rollback marker: %v", err)
	}
	var rb rolledBack
	if err := json.Unmarshal(raw, &rb); err != nil {
		t.Fatal(err)
	}
	if rb.Armed.ID != "r1" {
		t.Fatalf("marker names job %q", rb.Armed.ID)
	}
	if rb.Reason == "" {
		t.Fatal("the marker must carry a reason an operator can read")
	}
	if rb.RolledAt.IsZero() {
		t.Fatal("the marker must say when the rollback happened")
	}
}

// A crash after the two renames but before the swapped marker is removed brings
// the next boot straight back into rollBack. Re-running the renames was
// destructive: RemoveAll wiped the kept failed copy, the RECOVERED pb_data was
// renamed into its place, and the final rename then failed on the missing
// previous/<id> — leaving no pb_data at all.
func TestRollBackTwiceKeepsTheRecoveredData(t *testing.T) {
	resetRestoreState(t)
	dataDir := layout(t)
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	// The failed boot: the data goes back and the marker is cleared.
	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dataDir, "data.db")) != "old" {
		t.Fatal("the first rollback did not restore the previous data")
	}

	// The crash window: swapped is still there, previous/<id> is already gone.
	raw, err := os.ReadFile(filepath.Join(rolledBackDirOf(dataDir), "r1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rb rolledBack
	if err := json.Unmarshal(raw, &rb); err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(rb.Armed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(swappedPathOf(dataDir), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	prev := filepath.Join(restoreDirOf(dataDir), "previous", "r1")
	if _, serr := os.Stat(prev); !os.IsNotExist(serr) {
		t.Fatal("this test is only meaningful with previous/<id> already consumed")
	}

	if err := ApplyPendingRestore(dataDir); err != nil {
		t.Fatalf("re-entering the rollback must not fail: %v", err)
	}
	if got := read(t, filepath.Join(dataDir, "data.db")); got != "old" {
		t.Fatalf("the recovered data.db is %q, want the organization's own data", got)
	}
	if got := read(t, filepath.Join(dataDir, "auxiliary.db")); got != "aux" {
		t.Fatalf("the recovered pb_data lost a member: auxiliary.db is %q", got)
	}
	if got := read(t, filepath.Join(dataDir, "storage", "a", "b", "old.txt")); got != "old" {
		t.Fatalf("the recovered storage tree is %q", got)
	}
	// The failed restore's data is still set aside for inspection, not buried
	// under the recovered copy.
	if got := read(t, filepath.Join(restoreDirOf(dataDir), "failed", "r1", "data.db")); got != "new" {
		t.Fatalf("the failed restore's data is %q; the re-entry overwrote it", got)
	}
	if _, serr := os.Stat(swappedPathOf(dataDir)); !os.IsNotExist(serr) {
		t.Fatal("the swapped marker was not cleared on the second pass")
	}
	if Restoring() {
		t.Fatal("a rolled-back boot serves the previous data, so it is not restoring")
	}
}
