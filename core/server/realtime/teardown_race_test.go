package realtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// A client that reconnects while the room it just left is still tearing down
// must NOT be admitted into the dying room.
//
// The teardown window is real and wide: OnEmpty runs a final persistence
// flush — database writes, slow under load — and the room stays in the
// broker's map the whole time. A page reload reconnects exactly then. A
// client admitted into that room lands in a shell whose save hooks are
// already deregistered and whose server doc is about to close: sync requests
// come back empty, and every subsequent edit is relayed but never journaled,
// never applied, never saved. That is precisely how a card description came
// back blank — gone from the record AND the fragment — after a reload under
// full-suite load.
func TestJoinDuringTeardownWaitsForAFreshRoom(t *testing.T) {
	kind := "test-kind-join-during-teardown"
	var creates atomic.Int32
	teardownStarted := make(chan struct{})
	releaseTeardown := make(chan struct{})
	RegisterRoomKindWith(kind, RoomKindOptions{
		Authorize:       allowAllAuth,
		RuntimeProvider: stubDocRuntime{},
		OnRoomCreate:    func(string, DocHandle, *Room) { creates.Add(1) },
		OnEmpty: func(string) {
			close(teardownStarted)
			<-releaseTeardown
		},
	})
	t.Cleanup(func() { unregisterRoomKindForTest(kind) })

	b := NewBroker()
	first := &Client{joinedAt: time.Now()}
	b.join(kind, "room-x", first)
	original := b.lookupRoomForTest(kind, "room-x")
	if original == nil {
		t.Fatal("room not created")
	}

	go original.remove(first)
	<-teardownStarted

	second := &Client{joinedAt: time.Now()}
	joined := make(chan struct{})
	go func() {
		b.join(kind, "room-x", second)
		close(joined)
	}()

	select {
	case <-joined:
		t.Fatal("a client was admitted into a room whose teardown was already committed")
	case <-time.After(50 * time.Millisecond):
		// Correct: the join is waiting for the teardown to finish.
	}

	close(releaseTeardown)
	select {
	case <-joined:
	case <-time.After(2 * time.Second):
		t.Fatal("the join never completed after teardown finished")
	}

	fresh := b.lookupRoomForTest(kind, "room-x")
	if fresh == original {
		t.Fatal("the rejoin landed in the torn-down room instead of a fresh one")
	}
	if fresh == nil || fresh.serverDoc == nil {
		t.Fatal("the fresh room has no server doc — persistence would be dead for its lifetime")
	}
	if got := creates.Load(); got != 2 {
		t.Fatalf("OnRoomCreate fired %d times, want 2 (once per room incarnation)", got)
	}
}

// OnRoomEmpty must wait out an in-flight timer-driven save before deciding
// whether there is anything left to flush.
//
// triggerSave clears dirty BEFORE running its flush unlocked. A teardown
// racing that window read dirty==false, concluded "already saved", and let
// the broker close the DocHandle under the running flush. If that flush then
// FAILED (a busy SQLite under parallel load), its dirty re-mark went to a
// room the coordinator had already deregistered: the retry it scheduled
// found nothing and silently did nothing. The teardown flush below is that
// edit's last chance to reach durable storage.
func TestOnRoomEmptyWaitsOutAnInFlightSaveAndFlushesItsFailure(t *testing.T) {
	var calls atomic.Int32
	flushEntered := make(chan struct{})
	releaseFlush := make(chan struct{})
	c := NewSaveCoordinator(func(context.Context, string, DocHandle) error {
		if calls.Add(1) == 1 {
			close(flushEntered)
			<-releaseFlush
			return errors.New("synthetic flush failure")
		}
		return nil
	})
	c.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.debounceEvery = 5 * time.Millisecond
	c.ceilingEvery = time.Minute
	c.teardownTimeout = 2 * time.Second

	c.OnRoomCreate("room", &stubHandle{}, nil)
	c.OnDocUpdate("room")
	<-flushEntered // the debounce fired; the save is now in flight

	done := make(chan struct{})
	go func() {
		c.OnRoomEmpty("room")
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("OnRoomEmpty returned while a save was still in flight")
	case <-time.After(50 * time.Millisecond):
		// Correct: teardown is waiting for the in-flight save.
	}

	close(releaseFlush)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRoomEmpty never returned after the in-flight save finished")
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("flush ran %d times, want 2 — the failed in-flight save was never flushed at teardown", got)
	}
}
