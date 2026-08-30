package realtime

import (
	"testing"
	"time"
)

// A peer whose send buffer was full when an update was fanned out must be
// brought back into sync, not silently left behind.
//
// Yjs emits each change as a delta exactly ONCE. Nothing retransmits it, and
// the reconnect handshake is a state vector — it asks for what the CLIENT is
// missing, so it cannot recover a frame the SERVER already considered sent. A
// dropped fan-out frame therefore left that peer permanently short of the edit,
// still editing and rendering a document that disagreed with everyone else's,
// with no error anywhere. Under load — a busy full-suite run, a slow tab — that
// is exactly how a description came back missing the words someone watched
// themselves type.
//
// The repair is the mirror's whole state, which a CRDT merges as a no-op when
// the peer was only briefly behind.

// fillSendBuffer saturates c.send so the next deliver to it must fail.
func fillSendBuffer(t *testing.T, c *Client) {
	t.Helper()
	for i := 0; i < sendBufferSize; i++ {
		select {
		case c.send <- []byte{0}:
		default:
			t.Fatalf("send buffer full after %d of %d frames", i, sendBufferSize)
		}
	}
}

func TestFanOutResyncsAPeerWhoseBufferOverflowed(t *testing.T) {
	kind := "test-kind-fanout-overflow"
	RegisterRoomKindWith(kind, RoomKindOptions{
		Authorize:       allowAllAuth,
		RuntimeProvider: stubDocRuntime{},
	})
	t.Cleanup(func() { unregisterRoomKindForTest(kind) })

	b := NewBroker()
	sender := &Client{joinedAt: time.Now()}
	slow := &Client{joinedAt: time.Now()}
	b.join(kind, "room-overflow", sender)
	b.join(kind, "room-overflow", slow)

	room := b.lookupRoomForTest(kind, "room-overflow")
	if room == nil {
		t.Fatal("room not created")
	}

	// The peer was full when the update was fanned out and has since drained —
	// a client that was briefly wedged and caught up, which is the case worth
	// repairing. One that never drains is already doomed: the transport's
	// liveness checks close it and its reconnect resyncs from scratch.
	frame := make([]byte, frameOverhead+3)
	frame[clientIDLen] = byte(MsgDocUpdate)
	copy(frame[frameOverhead:], []byte{0xAA, 0xBB, 0xCC})

	room.resyncStarved([]*Client{slow}, frame)

	select {
	case got := <-slow.send:
		if MessageType(got[clientIDLen]) != MsgSyncReply {
			t.Errorf("the starved peer got %v, want a %v catch-up",
				MessageType(got[clientIDLen]), MsgSyncReply)
		}
	default:
		t.Fatal("the starved peer was left without the update it missed")
	}
	if room.serverDoc.(*stubHandle).encodeCalls == 0 {
		t.Error("the room never encoded its state to repair the starved peer")
	}
}

func TestFanOutReportsAnOverflowingPeer(t *testing.T) {
	// The plumbing the repair hangs off: a full buffer must be REPORTED, not
	// swallowed. deliver returning nothing is what made a lost update
	// indistinguishable from a delivered one.
	c := &Client{send: make(chan []byte, 1)}
	if !deliver(c, []byte{1}) {
		t.Fatal("deliver reported a failure for a frame it accepted")
	}
	if deliver(c, []byte{2}) {
		t.Error("deliver reported success for a frame it dropped")
	}
}

func TestFanOutDoesNotResyncOnAnAwarenessDrop(t *testing.T) {
	// Awareness is ephemeral: the next publish supersedes whatever was
	// dropped, so paying for a full document encode there would be waste on
	// every cursor move a busy room makes.
	kind := "test-kind-fanout-awareness"
	RegisterRoomKindWith(kind, RoomKindOptions{
		Authorize:       allowAllAuth,
		RuntimeProvider: stubDocRuntime{},
	})
	t.Cleanup(func() { unregisterRoomKindForTest(kind) })

	b := NewBroker()
	sender := &Client{joinedAt: time.Now()}
	slow := &Client{joinedAt: time.Now()}
	b.join(kind, "room-aware", sender)
	b.join(kind, "room-aware", slow)

	room := b.lookupRoomForTest(kind, "room-aware")
	if room == nil {
		t.Fatal("room not created")
	}
	before := room.serverDoc.(*stubHandle).encodeCalls
	fillSendBuffer(t, slow)

	frame := make([]byte, frameOverhead)
	frame[clientIDLen] = byte(MsgAwarenessUpdate)
	room.route(sender, frame)

	if room.serverDoc.(*stubHandle).encodeCalls != before {
		t.Error("a dropped awareness frame triggered a document resync")
	}
}

func TestFanOutSkipsAPeerThatLeftDuringTheRepair(t *testing.T) {
	// The repair runs with the room lock RELEASED, so the starved peer may
	// have gone by the time it runs — and room.remove closes its send channel
	// under that same lock. Writing to it then panics the whole process.
	kind := "test-kind-fanout-departed"
	RegisterRoomKindWith(kind, RoomKindOptions{
		Authorize:       allowAllAuth,
		RuntimeProvider: stubDocRuntime{},
	})
	t.Cleanup(func() { unregisterRoomKindForTest(kind) })

	b := NewBroker()
	resident := &Client{joinedAt: time.Now()}
	leaver := &Client{joinedAt: time.Now()}
	b.join(kind, "room-departed", resident)
	b.join(kind, "room-departed", leaver)

	room := b.lookupRoomForTest(kind, "room-departed")
	if room == nil {
		t.Fatal("room not created")
	}
	fillSendBuffer(t, leaver)
	room.remove(leaver)

	frame := make([]byte, frameOverhead+1)
	frame[clientIDLen] = byte(MsgDocUpdate)
	// Must not panic on the closed channel of a departed member.
	room.route(resident, frame)
}
