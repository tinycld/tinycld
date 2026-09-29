package hold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

	// No hold so Drain proceeds immediately.
	const numWriters = 16
	const keysPerWriter = 500
	var wg sync.WaitGroup

	var mu sync.Mutex
	deleted := make(map[string]bool)
	del := func(k string) error {
		mu.Lock()
		deleted[k] = true
		mu.Unlock()
		return nil
	}

	// Start ONE drain goroutine in a tight loop.
	stop := make(chan struct{})
	drainErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				drainErr <- nil
				return
			default:
			}
			if _, err := Drain(dir, now, del); err != nil {
				drainErr <- err
				return
			}
		}
	}()

	// Start N writer goroutines, each writes K distinct keys in a tight loop.
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for i := 0; i < keysPerWriter; i++ {
				key := fmt.Sprintf("writer-%d-key-%d", writer, i)
				if err := Journal(dir, key); err != nil {
					t.Errorf("journal error: %v", err)
					return
				}
			}
		}(w)
	}

	// Wait for all writers to finish.
	wg.Wait()

	// Stop the drain goroutine.
	close(stop)
	if err := <-drainErr; err != nil {
		t.Fatalf("drain error: %v", err)
	}

	// Run final drains until no more keys appear.
	for {
		n, _ := Drain(dir, now, del)
		if n == 0 {
			break
		}
	}

	// Verify: exactly 8000 keys were deleted.
	expectedCount := numWriters * keysPerWriter
	if len(deleted) != expectedCount {
		t.Fatalf("deleted %d keys, expected %d", len(deleted), expectedCount)
	}

	// Verify: every key is present in deleted.
	for w := 0; w < numWriters; w++ {
		for i := 0; i < keysPerWriter; i++ {
			key := fmt.Sprintf("writer-%d-key-%d", w, i)
			if !deleted[key] {
				t.Fatalf("key %s was journaled but never deleted", key)
			}
		}
	}
}

func TestDrainCanReEntryViaDelHook(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	// Journal a key, then drain with a del function that re-journals a key
	// (simulating a storage delete hook calling Journal).
	if err := Journal(dir, "key1"); err != nil {
		t.Fatal(err)
	}

	// Drain with del that calls Journal for each key.
	done := make(chan error, 1)
	go func() {
		_, err := Drain(dir, now, func(k string) error {
			// Re-journal each key (simulating a hook re-entry).
			return Journal(dir, k+"-rejournal")
		})
		done <- err
	}()

	// Wait for drain to complete with timeout.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("drain error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("drain deadlocked (timeout)")
	}

	// Verify the re-journaled keys are drained by a second drain.
	var drained []string
	_, _ = Drain(dir, now, func(k string) error {
		drained = append(drained, k)
		return nil
	})
	if len(drained) != 1 || drained[0] != "key1-rejournal" {
		t.Fatalf("re-journaled key not drained: %v", drained)
	}
}
