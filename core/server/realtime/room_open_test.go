package realtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	ycrdt "github.com/skyterra/y-crdt"
)

// ycrdtHandle is a DocHandle over a real y-crdt document, so the open
// path's checkpoint apply and state-vector verification run against the
// real decoder rather than a stub that accepts any bytes.
type ycrdtHandle struct {
	mu     sync.Mutex
	doc    *ycrdt.Doc
	closed bool
}

func (h *ycrdtHandle) ApplyUpdate(payload []byte) (err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return errors.New("closed")
	}
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("apply panicked")
		}
	}()
	ycrdt.ApplyUpdate(h.doc, payload, nil)
	return nil
}

func (h *ycrdtHandle) EncodeStateAsUpdate() ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, errors.New("closed")
	}
	return ycrdt.EncodeStateAsUpdate(h.doc, nil), nil
}

func (h *ycrdtHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	return nil
}

func (h *ycrdtHandle) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func (h *ycrdtHandle) text() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.doc.GetText("t").ToString()
}

// ycrdtRuntime seeds "t" with seedText and counts what the broker asked of
// it.
type ycrdtRuntime struct {
	mu       sync.Mutex
	seedText string
	failSeed error
	newDocs  int
	seeds    int
	last     *ycrdtHandle
}

func (r *ycrdtRuntime) NewDoc(roomID string) (DocHandle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.newDocs++
	r.last = &ycrdtHandle{doc: ycrdt.NewDoc(roomID, false, nil, nil, false)}
	return r.last, nil
}

func (r *ycrdtRuntime) Seed(_ context.Context, _ string, handle DocHandle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seeds++
	if r.failSeed != nil {
		return r.failSeed
	}
	h := handle.(*ycrdtHandle)
	h.doc.GetText("t").Insert(0, r.seedText, nil)
	return nil
}

func (r *ycrdtRuntime) counts() (newDocs, seeds int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.newDocs, r.seeds
}

// encodedDocWithText builds a standalone document holding text and returns
// its full state, the shape a checkpoint stores.
func encodedDocWithText(text string) []byte {
	doc := ycrdt.NewDoc("standalone", false, nil, nil, false)
	doc.GetText("t").Insert(0, text, nil)
	return ycrdt.EncodeStateAsUpdate(doc, nil)
}

// fixedFingerprint is a FingerprintFn tests can change between opens.
type fixedFingerprint struct {
	mu  sync.Mutex
	val string
	err error
}

func (f *fixedFingerprint) fn(string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.val, f.err
}

func (f *fixedFingerprint) set(v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.val = v
}

type openFixture struct {
	kind    string
	broker  *Broker
	rt      *ycrdtRuntime
	store   *MemoryCheckpointStore
	fp      *fixedFingerprint
	dirty   []string
	dirtyMu sync.Mutex
	evicted []string
	creates int
	empties int
}

func newOpenFixture(t *testing.T, kind string, store CheckpointStore) *openFixture {
	t.Helper()
	f := &openFixture{
		kind:   kind,
		broker: NewBroker(),
		rt:     &ycrdtRuntime{seedText: "seeded"},
		fp:     &fixedFingerprint{val: "fp-1"},
	}
	if ms, ok := store.(*MemoryCheckpointStore); ok {
		f.store = ms
	}
	opts := RoomKindOptions{
		Authorize:       allowAllAuth,
		RuntimeProvider: f.rt,
		Fingerprint:     f.fp.fn,
		OnDocUpdate: func(id string) {
			f.dirtyMu.Lock()
			defer f.dirtyMu.Unlock()
			f.dirty = append(f.dirty, id)
		},
		OnEvict: func(id string) {
			f.dirtyMu.Lock()
			defer f.dirtyMu.Unlock()
			f.evicted = append(f.evicted, id)
		},
		OnRoomCreate: func(string, DocHandle, *Room) {
			f.dirtyMu.Lock()
			defer f.dirtyMu.Unlock()
			f.creates++
		},
		OnEmpty: func(string) {
			f.dirtyMu.Lock()
			defer f.dirtyMu.Unlock()
			f.empties++
		},
	}
	if store != nil {
		opts.Checkpoints = store
	}
	RegisterRoomKindWith(kind, opts)
	t.Cleanup(func() {
		unregisterRoomKindForTest(kind)
		f.broker.Close()
	})
	return f
}

func (f *openFixture) open(roomID string) (*Room, *Client) {
	c := &Client{joinedAt: time.Now()}
	f.broker.join(f.kind, roomID, c)
	return f.broker.lookupRoomForTest(f.kind, roomID), c
}

func (f *openFixture) dirtyCount() int {
	f.dirtyMu.Lock()
	defer f.dirtyMu.Unlock()
	return len(f.dirty)
}

func (f *openFixture) evictCount() int {
	f.dirtyMu.Lock()
	defer f.dirtyMu.Unlock()
	return len(f.evicted)
}

func (f *openFixture) createCount() int {
	f.dirtyMu.Lock()
	defer f.dirtyMu.Unlock()
	return f.creates
}

func TestNewRoomSeedsWhenNoCheckpoint(t *testing.T) {
	f := newOpenFixture(t, "open-seed", NewMemoryCheckpointStore())
	room, _ := f.open("r1")
	if room.openedFrom != "seed" {
		t.Fatalf("openedFrom = %q; want seed", room.openedFrom)
	}
	if room.DocEpoch() <= 0 {
		t.Fatalf("epoch = %d; want > 0", room.DocEpoch())
	}
	if _, seeds := f.rt.counts(); seeds != 1 {
		t.Fatalf("seeds = %d; want 1", seeds)
	}
	if got := room.serverDoc.(*ycrdtHandle).text(); got != "seeded" {
		t.Fatalf("text = %q; want seeded", got)
	}
	if f.dirtyCount() != 0 {
		t.Fatal("a seeded room was marked dirty")
	}
}

func TestNewRoomAppliesCheckpointAndSkipsSeed(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "open-ckpt", store)
	_ = store.Save(f.kind, "r1", Checkpoint{Epoch: 123, Fingerprint: "fp-1", State: encodedDocWithText("from-checkpoint")})

	room, _ := f.open("r1")
	if room.openedFrom != "checkpoint" {
		t.Fatalf("openedFrom = %q; want checkpoint", room.openedFrom)
	}
	if room.DocEpoch() != 123 {
		t.Fatalf("epoch = %d; want the stored 123", room.DocEpoch())
	}
	if _, seeds := f.rt.counts(); seeds != 0 {
		t.Fatalf("seeds = %d; want 0", seeds)
	}
	if got := room.serverDoc.(*ycrdtHandle).text(); got != "from-checkpoint" {
		t.Fatalf("text = %q; want from-checkpoint", got)
	}
	// The state may carry edits a failed flush never wrote: the room is
	// marked dirty once so the save schedule exports it.
	if f.dirtyCount() != 1 {
		t.Fatalf("dirty marks = %d; want 1", f.dirtyCount())
	}
}

func TestNewRoomReseedsOnFingerprintMismatch(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "open-stale", store)
	_ = store.Save(f.kind, "r1", Checkpoint{Epoch: 123, Fingerprint: "fp-old", State: encodedDocWithText("stale")})

	room, _ := f.open("r1")
	if room.openedFrom != "seed" {
		t.Fatalf("openedFrom = %q; want seed", room.openedFrom)
	}
	if room.DocEpoch() <= 123 {
		t.Fatalf("epoch = %d; want > the stale 123", room.DocEpoch())
	}
	if got := room.serverDoc.(*ycrdtHandle).text(); got != "seeded" {
		t.Fatalf("text = %q; want seeded only", got)
	}
	if _, found, _ := store.Load(f.kind, "r1"); found {
		t.Fatal("the stale checkpoint was not deleted")
	}
}

func TestNewRoomReseedsOnCorruptCheckpoint(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "open-corrupt", store)
	// Half of a real state: the decoder reads some of it and stops.
	state := encodedDocWithText("a longer text so the update has many bytes")
	_ = store.Save(f.kind, "r1", Checkpoint{Epoch: 5, Fingerprint: "fp-1", State: state[:len(state)/2]})

	room, _ := f.open("r1")
	if room.openedFrom != "seed" {
		t.Fatalf("openedFrom = %q; want seed", room.openedFrom)
	}
	newDocs, seeds := f.rt.counts()
	if newDocs != 2 || seeds != 1 {
		t.Fatalf("newDocs=%d seeds=%d; want a fresh document (2) seeded once", newDocs, seeds)
	}
	if got := room.serverDoc.(*ycrdtHandle).text(); got != "seeded" {
		t.Fatalf("text = %q; want seeded on a clean document", got)
	}
	if _, found, _ := store.Load(f.kind, "r1"); found {
		t.Fatal("the corrupt checkpoint was not deleted")
	}
}

func TestNewRoomTreatsFingerprintErrorAsMismatch(t *testing.T) {
	store := NewMemoryCheckpointStore()
	f := newOpenFixture(t, "open-fperr", store)
	_ = store.Save(f.kind, "r1", Checkpoint{Epoch: 9, Fingerprint: "fp-1", State: encodedDocWithText("x")})
	f.fp.err = errors.New("db down")

	room, _ := f.open("r1")
	if room.openedFrom != "seed" || room.DocEpoch() <= 9 {
		t.Fatalf("openedFrom=%q epoch=%d; want a seed under a new epoch", room.openedFrom, room.DocEpoch())
	}
}

func TestNewRoomNilStoreAlwaysSeedsAndClosesOnEmpty(t *testing.T) {
	f := newOpenFixture(t, "open-nostore", nil)
	room, c := f.open("r1")
	handle := room.serverDoc.(*ycrdtHandle)
	room.remove(c)
	if !handle.isClosed() {
		t.Fatal("the document was not closed when the room emptied")
	}
	if f.broker.parkedCount() != 0 {
		t.Fatal("a document was parked without a checkpoint store")
	}
	f.open("r1")
	if _, seeds := f.rt.counts(); seeds != 2 {
		t.Fatalf("seeds = %d; want 2 (one per open)", seeds)
	}
}

// countingStore fails the test on any call: the inbound route path must
// never touch storage.
type countingStore struct {
	MemoryCheckpointStore
	calls int
	mu    sync.Mutex
}

func (s *countingStore) Load(kind, id string) (Checkpoint, bool, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.MemoryCheckpointStore.Load(kind, id)
}

func (s *countingStore) Save(kind, id string, cp Checkpoint) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.MemoryCheckpointStore.Save(kind, id, cp)
}

func (s *countingStore) Delete(kind, id string) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.MemoryCheckpointStore.Delete(kind, id)
}

func TestRouteDocUpdateNeverTouchesStore(t *testing.T) {
	store := &countingStore{MemoryCheckpointStore: *NewMemoryCheckpointStore()}
	f := newOpenFixture(t, "open-route", store)
	room, c := f.open("r1")
	store.mu.Lock()
	before := store.calls
	store.mu.Unlock()

	// A client edit against the server's incarnation.
	client := ycrdt.NewDoc("client", false, nil, nil, false)
	state, _ := room.serverDoc.EncodeStateAsUpdate()
	ycrdt.ApplyUpdate(client, state, nil)
	sv := ycrdt.EncodeStateVector(client, nil, ycrdt.NewUpdateEncoderV1())
	client.GetText("t").Insert(0, "edit:", nil)
	update := ycrdt.EncodeStateAsUpdate(client, sv)

	frame := make([]byte, frameOverhead+len(update))
	frame[clientIDLen] = byte(MsgDocUpdate)
	copy(frame[frameOverhead:], update)
	room.route(c, frame)

	if got := room.serverDoc.(*ycrdtHandle).text(); got != "edit:seeded" {
		t.Fatalf("text = %q; want edit:seeded", got)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.calls != before {
		t.Fatalf("the route path made %d store calls", store.calls-before)
	}
}

// A document the client built with garbage collection on emits GC structs
// for deleted ranges; the server must integrate them.
func TestRouteAppliesAnUpdateWithGCStructs(t *testing.T) {
	f := newOpenFixture(t, "open-gc", NewMemoryCheckpointStore())
	room, c := f.open("r1")

	client := ycrdt.NewDoc("client", true, func(*ycrdt.Item) bool { return true }, nil, false)
	state, _ := room.serverDoc.EncodeStateAsUpdate()
	ycrdt.ApplyUpdate(client, state, nil)
	sv := ycrdt.EncodeStateVector(client, nil, ycrdt.NewUpdateEncoderV1())
	text := client.GetText("t")
	text.Insert(0, "temporary ", nil)
	text.Delete(0, len("temporary "))
	text.Insert(0, "kept ", nil)
	update := ycrdt.EncodeStateAsUpdate(client, sv)

	frame := make([]byte, frameOverhead+len(update))
	frame[clientIDLen] = byte(MsgDocUpdate)
	copy(frame[frameOverhead:], update)
	room.route(c, frame)

	if got := room.serverDoc.(*ycrdtHandle).text(); got != "kept seeded" {
		t.Fatalf("text = %q; want kept seeded", got)
	}
	if strings.Contains(got(room), "temporary") {
		t.Fatal("deleted text reappeared")
	}
}

func got(room *Room) string { return room.serverDoc.(*ycrdtHandle).text() }
