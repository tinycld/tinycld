package realtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tinycld.org/core/readonly"
)

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestFlushDirtyFlushesEveryDirtyRoomAndIgnoresReadOnly(t *testing.T) {
	var mu sync.Mutex
	flushed := map[string]int{}
	fc := newFastCoordWithFlush(t, func(_ context.Context, id string, _ DocHandle) error {
		mu.Lock()
		defer mu.Unlock()
		flushed[id]++
		return nil
	})
	fc.c.debounceEvery = time.Hour
	fc.c.ceilingEvery = time.Hour
	for _, id := range []string{"a", "b", "clean"} {
		fc.c.OnRoomCreate(id, &stubHandle{}, nil)
	}
	fc.c.OnDocUpdate("a")
	fc.c.OnDocUpdate("b")

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	if err := fc.c.FlushDirty(context.Background()); err != nil {
		t.Fatalf("FlushDirty: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if flushed["a"] != 1 || flushed["b"] != 1 || flushed["clean"] != 0 {
		t.Fatalf("flushed = %v; want a and b once, clean never", flushed)
	}
	// Both rooms are clean now: a second FlushDirty flushes nothing.
	mu.Unlock()
	_ = fc.c.FlushDirty(context.Background())
	mu.Lock()
	if flushed["a"] != 1 || flushed["b"] != 1 {
		t.Fatalf("a clean room was flushed again: %v", flushed)
	}
}

func TestFlushDirtyReturnsTheErrorAndKeepsTheRoomDirty(t *testing.T) {
	var calls atomic.Int32
	boom := errors.New("exporter broke")
	fc := newFastCoordWithFlush(t, func(context.Context, string, DocHandle) error {
		calls.Add(1)
		return boom
	})
	fc.c.debounceEvery = time.Hour
	fc.c.ceilingEvery = time.Hour
	fc.c.OnRoomCreate("r", &stubHandle{}, nil)
	fc.c.OnDocUpdate("r")

	if err := fc.c.FlushDirty(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("FlushDirty err = %v; want %v", err, boom)
	}
	if err := fc.c.FlushDirty(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("second FlushDirty err = %v; want the room still dirty and flushed again", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("flush calls = %d; want 2", calls.Load())
	}
}

// After a failed save the room is dirty with no ceiling timer. A constant
// typist then resets the debounce on every edit, and without a re-armed
// ceiling the room would never save until they stopped.
func TestCeilingRearmedAfterFailedSave(t *testing.T) {
	var calls atomic.Int32
	var failNext atomic.Bool
	failNext.Store(true)
	fc := newFastCoordWithFlush(t, func(context.Context, string, DocHandle) error {
		calls.Add(1)
		if failNext.CompareAndSwap(true, false) {
			return errors.New("synthetic")
		}
		return nil
	})
	fc.c.debounceEvery = 40 * time.Millisecond
	fc.c.ceilingEvery = 120 * time.Millisecond
	fc.c.backoff = func(int) time.Duration { return time.Hour } // the retry timer must not be what saves us
	fc.c.OnRoomCreate("r", &stubHandle{}, nil)

	fc.c.OnDocUpdate("r")
	if !waitFor(t, time.Second, func() bool { return calls.Load() == 1 }) {
		t.Fatal("the first save never ran")
	}
	// Keep typing faster than the debounce for longer than the ceiling.
	stop := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(stop) {
		fc.c.OnDocUpdate("r")
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("flush calls = %d; the ceiling did not fire while the typist kept resetting the debounce", calls.Load())
	}
}

func TestSaveDeferredWhileReadOnlyIsNotAFailure(t *testing.T) {
	var calls atomic.Int32
	fc := newFastCoordWithFlush(t, func(context.Context, string, DocHandle) error {
		calls.Add(1)
		return nil
	})
	fc.c.debounceEvery = 20 * time.Millisecond
	fc.c.ceilingEvery = time.Hour
	fc.c.maxAttempts = 2
	fc.c.backoff = func(int) time.Duration { return 20 * time.Millisecond }
	var gaveUp atomic.Int32
	fc.c.captureGiveUp = func(giveUpDetail) { gaveUp.Add(1) }
	fc.c.OnRoomCreate("r", &stubHandle{}, nil)

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	fc.c.OnDocUpdate("r")
	time.Sleep(150 * time.Millisecond) // several deferrals' worth
	if calls.Load() != 0 {
		t.Fatalf("flush ran %d times while read-only", calls.Load())
	}
	if gaveUp.Load() != 0 {
		t.Fatal("deferrals counted toward giving up")
	}
	readonly.Leave()
	if !waitFor(t, time.Second, func() bool { return calls.Load() == 1 }) {
		t.Fatal("the deferred save did not run after Leave")
	}
}

func TestTeardownSkipsTheFlushWhileReadOnly(t *testing.T) {
	var calls atomic.Int32
	fc := newFastCoordWithFlush(t, func(context.Context, string, DocHandle) error {
		calls.Add(1)
		return nil
	})
	fc.c.debounceEvery = time.Hour
	fc.c.ceilingEvery = time.Hour
	fc.c.OnRoomCreate("r", &stubHandle{}, nil)
	fc.c.OnDocUpdate("r")

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	fc.c.OnRoomEmpty("r")
	if calls.Load() != 0 {
		t.Fatalf("teardown flushed %d times while read-only", calls.Load())
	}
}

func TestFlushNowReturnsErrReadOnly(t *testing.T) {
	fc := newFastCoord(t)
	fc.c.OnRoomCreate("r", &stubHandle{}, nil)
	readonly.Enter()
	t.Cleanup(readonly.Leave)
	if err := fc.c.FlushNow("r"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("FlushNow err = %v; want ErrReadOnly", err)
	}
	if fc.callCount() != 0 {
		t.Fatal("FlushNow flushed while read-only")
	}
}

// A room that reopens after a teardown (an unparked document) registers
// again under the same id and must save like a new room.
func TestOnRoomCreateAfterOnRoomEmptyMakesAFreshSaver(t *testing.T) {
	fc := newFastCoord(t)
	fc.c.debounceEvery = 20 * time.Millisecond
	handle := &stubHandle{}
	fc.c.OnRoomCreate("r", handle, nil)
	fc.c.OnRoomEmpty("r")
	fc.c.OnRoomCreate("r", handle, nil)
	fc.c.OnDocUpdate("r")
	if !waitFor(t, time.Second, func() bool { return fc.callCount() == 1 }) {
		t.Fatal("the reopened room never saved")
	}
}

var _ = slog.New(slog.NewTextHandler(io.Discard, nil))
