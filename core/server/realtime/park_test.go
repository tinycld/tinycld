package realtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	ycrdt "github.com/skyterra/y-crdt"

	"tinycld.org/core/readonly"
)

func TestRoomEmptyParksTheDocument(t *testing.T) {
	f := newOpenFixture(t, "park-empty", NewMemoryCheckpointStore())
	room, c := f.open("r1")
	handle := room.serverDoc.(*ycrdtHandle)
	epoch := room.DocEpoch()
	room.remove(c)

	if f.broker.parkedCount() != 1 {
		t.Fatalf("parked = %d; want 1", f.broker.parkedCount())
	}
	if handle.isClosed() {
		t.Fatal("the parked document was closed")
	}
	if f.evictCount() != 0 {
		t.Fatal("OnEvict ran at park time")
	}
	if f.empties != 1 {
		t.Fatalf("OnEmpty ran %d times; want 1", f.empties)
	}
	p := f.broker.lookupParkedForTest(f.kind, "r1")
	if p.fingerprint != "fp-1" || p.epoch != epoch {
		t.Fatalf("parked = {fp %q, epoch %d}; want fp-1 and epoch %d", p.fingerprint, p.epoch, epoch)
	}
}

func TestReopenAdoptsParkedDocument(t *testing.T) {
	f := newOpenFixture(t, "park-reopen", NewMemoryCheckpointStore())
	room, c := f.open("r1")
	epoch := room.DocEpoch()
	room.serverDoc.(*ycrdtHandle).doc.GetText("t").Insert(0, "edit:", nil)
	room.remove(c)

	again, _ := f.open("r1")
	if again.openedFrom != "parked" {
		t.Fatalf("openedFrom = %q; want parked", again.openedFrom)
	}
	if again.DocEpoch() != epoch {
		t.Fatalf("epoch changed across a park: %d -> %d", epoch, again.DocEpoch())
	}
	if _, seeds := f.rt.counts(); seeds != 1 {
		t.Fatalf("seeds = %d; want 1 (no re-seed)", seeds)
	}
	if got := again.serverDoc.(*ycrdtHandle).text(); got != "edit:seeded" {
		t.Fatalf("text = %q; want the parked content", got)
	}
	if f.createCount() != 2 {
		t.Fatalf("OnRoomCreate ran %d times; want 2", f.createCount())
	}
	if f.broker.parkedCount() != 0 {
		t.Fatal("the adopted document is still parked")
	}
}

func TestReopenWithChangedFingerprintReseeds(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "park-stale", store)
	room, c := f.open("r1")
	epoch := room.DocEpoch()
	old := room.serverDoc.(*ycrdtHandle)
	room.remove(c)
	_ = store.Save(f.kind, "r1", Checkpoint{Epoch: epoch, Fingerprint: "fp-1", State: []byte{1}})

	f.fp.set("fp-2")
	again, _ := f.open("r1")
	if again.openedFrom != "seed" {
		t.Fatalf("openedFrom = %q; want seed", again.openedFrom)
	}
	if again.DocEpoch() <= epoch {
		t.Fatalf("epoch = %d; want > %d", again.DocEpoch(), epoch)
	}
	if !old.isClosed() {
		t.Fatal("the stale parked document was not closed")
	}
	if f.evictCount() != 1 {
		t.Fatalf("OnEvict ran %d times; want 1", f.evictCount())
	}
	if _, found, _ := store.Load(f.kind, "r1"); found {
		t.Fatal("the stale checkpoint was not deleted")
	}
}

func TestJanitorEvictsAfterParkIdle(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "park-evict", store)
	room, c := f.open("r1")
	epoch := room.DocEpoch()
	handle := room.serverDoc.(*ycrdtHandle)
	room.remove(c)

	f.broker.evictIdle(time.Now().Add(ParkIdle / 2))
	if f.broker.parkedCount() != 1 {
		t.Fatal("evicted before ParkIdle")
	}
	f.broker.evictIdle(time.Now().Add(ParkIdle + time.Second))
	if f.broker.parkedCount() != 0 {
		t.Fatal("not evicted after ParkIdle")
	}
	cp, found, _ := store.Load(f.kind, "r1")
	if !found || cp.Epoch != epoch || cp.Fingerprint != "fp-1" {
		t.Fatalf("checkpoint = %+v found=%v; want epoch %d fp-1", cp, found, epoch)
	}
	if !handle.isClosed() || f.evictCount() != 1 {
		t.Fatalf("closed=%v evicts=%d; want closed and one OnEvict", handle.isClosed(), f.evictCount())
	}

	// The next open is the same incarnation, from the store.
	again, _ := f.open("r1")
	if again.openedFrom != "checkpoint" || again.DocEpoch() != epoch {
		t.Fatalf("openedFrom=%q epoch=%d; want checkpoint and %d", again.openedFrom, again.DocEpoch(), epoch)
	}
	if got := again.serverDoc.(*ycrdtHandle).text(); got != "seeded" {
		t.Fatalf("text = %q", got)
	}
}

// An edit a client made against the original incarnation still applies to
// the room after park, eviction and reopen from the checkpoint. This is the
// whole point: the journal replayed onto a re-seeded document never did.
func TestCheckpointKeepsTheIncarnation(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "park-incarnation", store)
	room, c := f.open("r1")
	client := ycrdt.NewDoc("client", false, nil, nil, false)
	state, _ := room.serverDoc.EncodeStateAsUpdate()
	ycrdt.ApplyUpdate(client, state, nil)
	room.remove(c)
	f.broker.evictIdle(time.Now().Add(ParkIdle + time.Second))

	again, c2 := f.open("r1")
	sv := ycrdt.EncodeStateVector(client, nil, ycrdt.NewUpdateEncoderV1())
	client.GetText("t").Insert(len("seeded"), " +offline", nil)
	update := ycrdt.EncodeStateAsUpdate(client, sv)
	frame := make([]byte, frameOverhead+len(update))
	frame[clientIDLen] = byte(MsgDocUpdate)
	copy(frame[frameOverhead:], update)
	again.route(c2, frame)

	if got := again.serverDoc.(*ycrdtHandle).text(); got != "seeded +offline" {
		t.Fatalf("text = %q; want the offline edit applied once", got)
	}
}

func TestJanitorCompactsOversizeState(t *testing.T) {
	store := NewMemoryCheckpointStore()
	rt := newStubRuntime()
	kind := "park-compact"
	fp := &fixedFingerprint{val: "fp"}
	RegisterRoomKindWith(kind, RoomKindOptions{
		Authorize:       allowAllAuth,
		RuntimeProvider: rt,
		Checkpoints:     store,
		Fingerprint:     fp.fn,
	})
	t.Cleanup(func() { unregisterRoomKindForTest(kind) })
	b := NewBroker()
	t.Cleanup(b.Close)
	c := &Client{joinedAt: time.Now()}
	b.join(kind, "r1", c)
	room := b.lookupRoomForTest(kind, "r1")
	_ = store.Save(kind, "r1", Checkpoint{Epoch: 1, Fingerprint: "fp", State: []byte{1}})
	rt.doc("r1").cannedEncoding = make([]byte, CompactCheckpointBytes+1)
	room.remove(c)

	b.evictIdle(time.Now().Add(ParkIdle + time.Second))
	if _, found, _ := store.Load(kind, "r1"); found {
		t.Fatal("an oversize state was stored instead of compacted")
	}
	if !rt.doc("r1").closed {
		t.Fatal("the evicted document was not closed")
	}
}

func TestJanitorSkipsWhileReadOnly(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "park-readonly", store)
	room, c := f.open("r1")
	room.remove(c)

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	f.broker.evictIdle(time.Now().Add(ParkIdle + time.Second))
	if f.broker.parkedCount() != 1 {
		t.Fatal("evicted while read-only")
	}
	readonly.Leave()
	f.broker.evictIdle(time.Now().Add(ParkIdle + time.Second))
	if f.broker.parkedCount() != 0 {
		t.Fatal("not evicted after Leave")
	}
}

func TestSuspendFlushesThenCheckpointsOpenAndParkedRooms(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "park-suspend", store)
	var flushes, savesAtFlush atomic.Int32
	opts, _ := LookupOptionsForTest(f.kind)
	opts.FlushDirty = func(context.Context) error {
		flushes.Add(1)
		savesAtFlush.Store(int32(len(store.rows)))
		return nil
	}
	unregisterRoomKindForTest(f.kind)
	RegisterRoomKindWith(f.kind, opts)

	open, _ := f.open("open")
	parkedRoom, c := f.open("parked")
	parkedEpoch := parkedRoom.DocEpoch()
	parkedRoom.remove(c)

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	f.broker.Suspend(context.Background(), "test")

	if flushes.Load() != 1 || savesAtFlush.Load() != 0 {
		t.Fatalf("flushes=%d savesAtFlush=%d; want one flush before any save", flushes.Load(), savesAtFlush.Load())
	}
	cp, found, _ := store.Load(f.kind, "open")
	if !found || cp.Epoch != open.DocEpoch() || cp.Fingerprint != "fp-1" {
		t.Fatalf("open room checkpoint = %+v found=%v", cp, found)
	}
	cp, found, _ = store.Load(f.kind, "parked")
	if !found || cp.Epoch != parkedEpoch {
		t.Fatalf("parked room checkpoint = %+v found=%v", cp, found)
	}
	if f.broker.roomCount() != 1 || f.broker.parkedCount() != 1 {
		t.Fatalf("rooms=%d parked=%d after Suspend; want both kept", f.broker.roomCount(), f.broker.parkedCount())
	}
}

func TestSuspendAfterFailedFlushStillCheckpoints(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "park-suspend-fail", store)
	opts, _ := LookupOptionsForTest(f.kind)
	opts.FlushDirty = func(context.Context) error { return errors.New("exporter broke") }
	unregisterRoomKindForTest(f.kind)
	RegisterRoomKindWith(f.kind, opts)

	room, _ := f.open("r1")
	f.broker.Suspend(context.Background(), "test")
	cp, found, _ := store.Load(f.kind, "r1")
	if !found || cp.Epoch != room.DocEpoch() || cp.Fingerprint != "fp-1" {
		t.Fatalf("checkpoint = %+v found=%v; want it stored with the current fingerprint", cp, found)
	}
}

func TestDropRoomClosesParkedAndDeletesCheckpoint(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "park-drop", store)
	room, c := f.open("r1")
	handle := room.serverDoc.(*ycrdtHandle)
	room.remove(c)
	_ = store.Save(f.kind, "r1", Checkpoint{Epoch: 1, Fingerprint: "fp-1", State: []byte{1}})

	if err := f.broker.DropRoom(f.kind, "r1"); err != nil {
		t.Fatalf("DropRoom: %v", err)
	}
	if f.broker.parkedCount() != 0 || !handle.isClosed() || f.evictCount() != 1 {
		t.Fatal("the parked document was not evicted")
	}
	if _, found, _ := store.Load(f.kind, "r1"); found {
		t.Fatal("the checkpoint was not deleted")
	}
	if err := f.broker.DropRoom(f.kind, "r1"); err != nil {
		t.Fatalf("second DropRoom: %v", err)
	}
}
