package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
)

// fakeFreeSpace pins what the precheck sees. The real Statfs answers about the
// machine the test runs on, so an assertion against it would pass or fail with
// the developer's disk.
func fakeFreeSpace(t *testing.T, have int64) {
	t.Helper()
	prev := availableBytes
	availableBytes = func(string) (int64, error) { return have, nil }
	t.Cleanup(func() { availableBytes = prev })
}

// A backup that runs out of space mid-VACUUM leaves a truncated snapshot and
// reports a SQLite error nobody can act on.
func TestRunRefusesWhenThereIsNoRoomForTheSnapshot(t *testing.T) {
	app := newTestApp(t)
	fakeFreeSpace(t, 1) // one byte
	id, _ := age.GenerateX25519Identity()
	sink := &closeBuffer{}
	rowID, err := Run(app, Request{Kind: KindManual, Recipient: id.Recipient(), Sink: sink})
	if err == nil {
		t.Fatal("a backup with no room must be refused")
	}
	if !strings.Contains(err.Error(), "not enough free space") {
		t.Fatalf("the refusal must say why: %v", err)
	}
	row, rerr := app.FindRecordById("backups", rowID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got := row.GetString("status"); got != "failed" {
		t.Fatalf("status %q", got)
	}
}

func TestRunProceedsWithRoom(t *testing.T) {
	app := newTestApp(t)
	fakeFreeSpace(t, 1<<40)
	id, _ := age.GenerateX25519Identity()
	sink := &closeBuffer{}
	if _, err := Run(app, Request{Kind: KindManual, Recipient: id.Recipient(), Sink: sink}); err != nil {
		t.Fatal(err)
	}
}

// A platform that cannot measure free space must not be refused a backup.
func TestFreeSpaceCheckAllowsAnUnmeasurableFilesystem(t *testing.T) {
	dir := t.TempDir()
	prev := availableBytes
	availableBytes = func(string) (int64, error) { return 0, os.ErrInvalid }
	t.Cleanup(func() { availableBytes = prev })
	if err := requireFreeSpace(dir, 1<<40); err != nil {
		t.Fatalf("an unmeasurable filesystem must not block a run: %v", err)
	}
}

func TestFreeSpaceCheckIgnoresAZeroEstimate(t *testing.T) {
	prev := availableBytes
	availableBytes = func(string) (int64, error) { t.Fatal("must not be consulted"); return 0, nil }
	t.Cleanup(func() { availableBytes = prev })
	if err := requireFreeSpace(t.TempDir(), 0); err != nil {
		t.Fatal(err)
	}
}

func TestStatfsAvailableReportsARealFilesystem(t *testing.T) {
	have, err := statfsAvailable(t.TempDir())
	if err != nil {
		t.Skipf("free space is not measurable here: %v", err)
	}
	if have <= 0 {
		t.Fatalf("free space %d", have)
	}
}

// Both safety copies are kept on failure by design, and nothing removed them: a
// deployment with three failed restores carried three whole copies of itself.
func TestSweepSafetyCopiesKeepsOnlyTheCurrentRestores(t *testing.T) {
	app := newTestApp(t)
	pre := filepath.Join(restoreDir(app), "pre")
	failed := filepath.Join(restoreDir(app), "failed")
	for _, d := range []string{pre, failed} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pre, "old.age"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pre, "keep.age"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(failed, "old"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(failed, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}

	sweepSafetyCopies(app, "keep")

	for _, gone := range []string{filepath.Join(pre, "old.age"), filepath.Join(failed, "old")} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s survived the sweep", gone)
		}
	}
	for _, kept := range []string{filepath.Join(pre, "keep.age"), filepath.Join(failed, "keep")} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s was swept: %v", kept, err)
		}
	}
}

func TestHumanBytesReadsAsSizes(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KiB"},
		{3 << 30, "3.0 GiB"},
	} {
		if got := humanBytes(tc.in); got != tc.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A restore holds three copies at its peak: the staged archive, a pre-restore
// copy of the live database, and the previous pb_data the swap keeps beside the
// restored one. A disk that fills in the middle of that leaves a half-staged
// restore whose safety copy did not fit either.
func TestRestoreRefusesWhenThereIsNoRoomForTheStagedArchive(t *testing.T) {
	data, identity := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	fakeFreeSpace(t, 1)

	jobID, err := Restore(app, RestoreRequest{Source: readCloser{bytes.NewReader(data)}, Identity: identity})
	if err == nil {
		t.Fatal("a restore with no room must be refused")
	}
	if !strings.Contains(err.Error(), "not enough free space") {
		t.Fatalf("the refusal must say why: %v", err)
	}
	// Refused before phase 3, so nothing was written: no safety copy and no
	// armed marker to confuse the next boot.
	if _, serr := os.Stat(preBackupPath(app, jobID)); !os.IsNotExist(serr) {
		t.Fatal("the refusal happened after the pre-restore copy")
	}
	if _, serr := os.Stat(armedPath(app)); !os.IsNotExist(serr) {
		t.Fatal("a refused restore must not arm anything")
	}
}

// The sweep runs as part of a real restore, not only on its own.
func TestRestoreSweepsAnEarlierFailedRestoresCopies(t *testing.T) {
	data, identity := archiveFor(t)
	app := newTestApp(t)
	resetRestoreState(t)
	fakeFreeSpace(t, 1<<40)

	stale := preBackupPath(app, "older")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(app, RestoreRequest{
		Source: readCloser{bytes.NewReader(data)}, Identity: identity, Force: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("the copy from the earlier failed restore was not swept")
	}
}
