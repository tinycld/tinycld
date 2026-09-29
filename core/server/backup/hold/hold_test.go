package hold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func clock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestAcquireWritesAReadableHold(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	h, err := Acquire(dir, "engine", clock(now))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	st, ok, err := Read(dir)
	if err != nil || !ok {
		t.Fatalf("read: %v %v", ok, err)
	}
	if st.Holder != "engine" || !st.Expires.Equal(now.Add(Lease)) {
		t.Fatalf("state = %+v", st)
	}
	// Another process must be able to read it: a hold written by one user is
	// read by the app running as another.
	fi, _ := os.Stat(filepath.Join(dir, FileName))
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

func TestSecondHolderIsRefusedWhileValid(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	h, err := Acquire(dir, "engine", clock(now))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	if _, err := Acquire(dir, "router", clock(now)); !errors.Is(err, ErrHeld) {
		t.Fatalf("err = %v, want ErrHeld", err)
	}
}

func TestExpiredHoldIsTakenOver(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if _, err := Acquire(dir, "crashed", clock(now.Add(-2*Lease))); err != nil {
		t.Fatal(err)
	}
	h, err := Acquire(dir, "engine", clock(now))
	if err != nil {
		t.Fatalf("expired hold not taken over: %v", err)
	}
	defer h.Release()
	st, _, _ := Read(dir)
	if st.Holder != "engine" {
		t.Fatalf("holder = %q", st.Holder)
	}
}

func TestReleaseRemovesOnlyItsOwnHold(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	h, _ := Acquire(dir, "engine", clock(now))
	if err := h.Release(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Read(dir); ok {
		t.Fatal("hold still present")
	}
	// A release after another holder took over (ours expired) leaves theirs.
	h1, _ := Acquire(dir, "a", clock(now.Add(-2*Lease)))
	h2, _ := Acquire(dir, "b", clock(now))
	_ = h1.Release()
	st, ok, _ := Read(dir)
	if !ok || st.Holder != "b" {
		t.Fatalf("release removed another holder's hold: %+v %v", st, ok)
	}
	_ = h2.Release()
}

func TestDrainWaitsForTheHoldThenDeletesEveryKey(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	h, _ := Acquire(dir, "engine", clock(now))
	for _, k := range []string{"a/1.txt", "b/2.txt"} {
		if err := Journal(dir, k); err != nil {
			t.Fatal(err)
		}
	}
	var deleted []string
	del := func(k string) error { deleted = append(deleted, k); return nil }
	if n, err := Drain(dir, now, del); err != nil || n != 0 {
		t.Fatalf("drain under a valid hold: n=%d err=%v", n, err)
	}
	_ = h.Release()
	n, err := Drain(dir, now, del)
	if err != nil || n != 2 || len(deleted) != 2 {
		t.Fatalf("drain: n=%d err=%v deleted=%v", n, err, deleted)
	}
	if n, _ := Drain(dir, now, del); n != 0 {
		t.Fatalf("second drain deleted %d", n)
	}
}

func TestDrainKeepsTheRestAfterAnError(t *testing.T) {
	dir := t.TempDir()
	_ = Journal(dir, "a")
	_ = Journal(dir, "b")
	boom := errors.New("boom")
	calls := 0
	_, err := Drain(dir, time.Now(), func(k string) error {
		calls++
		if k == "b" {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	var again []string
	if _, err := Drain(dir, time.Now(), func(k string) error { again = append(again, k); return nil }); err != nil {
		t.Fatal(err)
	}
	// Idempotent: the retry may repeat "a"; it must not skip "b".
	if len(again) == 0 || again[len(again)-1] != "b" {
		t.Fatalf("retry = %v", again)
	}
}

func TestRemoveStaleReportsAnAbandonedHold(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	_, _ = Acquire(dir, "crashed", clock(now.Add(-2*Lease)))
	st, removed, err := RemoveStale(dir, now)
	if err != nil || !removed || st.Holder != "crashed" {
		t.Fatalf("st=%+v removed=%v err=%v", st, removed, err)
	}
	if _, ok, _ := Read(dir); ok {
		t.Fatal("stale hold still present")
	}
}

func TestConcurrentJournalAndDrain(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	h, _ := Acquire(dir, "engine", clock(now))
	defer h.Release()

	// Journal some keys while the hold is active (Drain will return 0).
	const numKeys = 100
	for i := 0; i < numKeys; i++ {
		if err := Journal(dir, fmt.Sprintf("key-%d", i)); err != nil {
			t.Fatal(err)
		}
	}

	// Release the hold to allow Drain to proceed, then run Journal and Drain
	// concurrently in goroutines.
	_ = h.Release()

	journaled := make(map[string]bool)
	for i := 0; i < numKeys; i++ {
		journaled[fmt.Sprintf("key-%d", i)] = true
	}

	var deleted map[string]bool
	drainErr := make(chan error, 1)
	go func() {
		var deltmp []string
		_, err := Drain(dir, now, func(k string) error {
			deltmp = append(deltmp, k)
			return nil
		})
		deleted = make(map[string]bool)
		for _, k := range deltmp {
			deleted[k] = true
		}
		drainErr <- err
	}()

	journalErr := make(chan error, 1)
	go func() {
		for i := numKeys; i < 2*numKeys; i++ {
			if err := Journal(dir, fmt.Sprintf("key-%d", i)); err != nil {
				journalErr <- err
				return
			}
		}
		journalErr <- nil
	}()

	if err := <-drainErr; err != nil {
		t.Fatalf("drain error: %v", err)
	}
	if err := <-journalErr; err != nil {
		t.Fatalf("journal error: %v", err)
	}

	// After the concurrent operations, all initially journaled keys must either
	// be deleted or not yet seen by the drain. Run a final Drain to capture
	// any keys journaled during the first drain.
	var finalDeleted []string
	_, _ = Drain(dir, now, func(k string) error {
		finalDeleted = append(finalDeleted, k)
		return nil
	})
	for _, k := range finalDeleted {
		deleted[k] = true
	}

	// Verify: every key initially journaled (0..99) appears in the deleted set.
	// Keys 100+ may or may not be deleted (they were journaled during the drain).
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key-%d", i)
		if !deleted[key] {
			t.Fatalf("key %s was journaled but never deleted", key)
		}
	}
}

