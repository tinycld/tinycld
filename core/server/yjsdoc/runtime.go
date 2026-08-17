// Package yjsdoc is the generic server-side Yjs document runtime: a per-room
// registry of native-Go Y.Docs implementing realtime.DocRuntime, plus the
// ProseMirror bridge and update-inspection helpers a room kind needs to persist
// what its collaborators type.
//
// It is the reusable core of the machinery text/server grew first. Anything
// text-specific (docx import warnings, authorship stamping, suggestion maps,
// edit-event buffering) deliberately stayed behind in that package; what lives
// here is what a second consumer — cards' board rooms — needed verbatim.
//
// Backed by github.com/skyterra/y-crdt. Consumers refer to yjsdoc.Doc rather
// than importing y-crdt directly, which keeps the dependency inside core: a
// feature package can register a document room without adding a single external
// module to its own go.mod.
package yjsdoc

import (
	"fmt"
	"sync"
	"time"

	ycrdt "github.com/skyterra/y-crdt"

	"tinycld.org/core/logging"
	"tinycld.org/core/realtime"
)

var log = logging.ForPackage("yjsdoc")

// Doc is a Yjs document. Aliased rather than wrapped so a consumer can pass it
// to the bridge helpers in this package without importing y-crdt, while text/
// could still adopt this runtime later without rewriting its own y-crdt calls.
type Doc = ycrdt.Doc

// Janitor / TTL knobs. Vars, not consts, so tests can drop them to
// milliseconds and observe eviction without sleeping.
var (
	// JanitorInterval is how often the reaper scans for idle docs.
	JanitorInterval = 5 * time.Minute
	// MaxIdleDuration bounds memory: a doc with no activity for this long is
	// closed. Rooms that are still live are exempt — see evictIdleDocs.
	MaxIdleDuration = 30 * time.Minute
)

// MaxApplyUpdateBytes bounds a single inbound update. y-crdt allocates per
// message, so a hostile 100 MiB frame would exhaust memory before the recover
// guard could fire. Real edits are kilobytes; 1 MiB leaves ample headroom.
const MaxApplyUpdateBytes = 1 * 1024 * 1024

// now is the clock the runtime and janitor read. Replaced in tests.
var now = time.Now

// BootstrapFn seeds a freshly created document. It runs synchronously inside
// NewDoc, before the broker can serve a SyncReply, so the first joiner already
// sees populated content.
type BootstrapFn func(roomID string, doc *Doc) error

// Runtime is a process-wide registry of server-side Y.Docs, one per active
// room. It satisfies realtime.DocRuntime.
type Runtime struct {
	// mu guards the maps below. RWMutex because the accessors are read on
	// every inbound frame while writers (NewDoc, closeDoc) are rare.
	mu      sync.RWMutex
	docs    map[string]*Doc
	handles map[string]*Handle
	rooms   map[string]*realtime.Room
	// live marks rooms that still have occupants. Separate from rooms so a
	// caller without a *realtime.Room can still express liveness.
	live      map[string]bool
	bootstrap BootstrapFn

	stop           chan struct{}
	stopOnce       sync.Once
	janitorOnce    sync.Once
	janitorDone    chan struct{}
	janitorStartMu sync.Mutex
	janitorStarted bool
}

// NewRuntime returns an empty registry. Call StartJanitor to enable idle
// eviction, and Stop at shutdown.
func NewRuntime() *Runtime {
	return &Runtime{
		docs:        make(map[string]*Doc),
		handles:     make(map[string]*Handle),
		rooms:       make(map[string]*realtime.Room),
		live:        make(map[string]bool),
		stop:        make(chan struct{}),
		janitorDone: make(chan struct{}),
	}
}

// SetBootstrap registers the seeding hook. Passing nil clears it.
func (r *Runtime) SetBootstrap(hook BootstrapFn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bootstrap = hook
}

// NoteRoom associates a broker room with a roomID. The runtime needs it for
// two things: publishing server-originated updates, and knowing that a room is
// still live so the janitor does not evict its document out from under it.
//
// Pass nil when the room goes away.
func (r *Runtime) NoteRoom(roomID string, room *realtime.Room) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if room == nil {
		delete(r.rooms, roomID)
		delete(r.live, roomID)
		return
	}
	r.rooms[roomID] = room
	r.live[roomID] = true
}

// MarkLive records that a room is occupied without supplying a *realtime.Room.
// The janitor consults only this flag, so a caller that has no Room to hand
// (and tests, which cannot construct one — Room's fields are unexported) can
// still express liveness.
func (r *Runtime) MarkLive(roomID string, live bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if live {
		r.live[roomID] = true
		return
	}
	delete(r.live, roomID)
}

// RoomFor returns the room registered for roomID, or nil.
func (r *Runtime) RoomFor(roomID string) *realtime.Room {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.rooms[roomID]
}

// HandleFor returns the handle for roomID, or nil. Callers that mutate the doc
// must go through Handle.WithDoc so their writes serialize against the broker's
// concurrent ApplyUpdate / EncodeStateAsUpdate.
func (r *Runtime) HandleFor(roomID string) *Handle {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handles[roomID]
}

// NewDoc satisfies realtime.DocRuntime.
//
// A bootstrap failure is logged rather than fatal: an empty document still lets
// clients connect and edit, whereas refusing the room takes the feature down
// for everyone in it.
func (r *Runtime) NewDoc(roomID string) (realtime.DocHandle, error) {
	r.mu.Lock()
	if _, exists := r.docs[roomID]; exists {
		r.mu.Unlock()
		return nil, fmt.Errorf("yjsdoc: room %s already has a document", roomID)
	}
	doc := ycrdt.NewDoc(roomID, false, nil, nil, false)
	InstallPatcher(doc)
	handle := &Handle{runtime: r, id: roomID, doc: doc, lastActivity: now()}
	r.docs[roomID] = doc
	r.handles[roomID] = handle
	hook := r.bootstrap
	r.mu.Unlock()

	if hook != nil {
		if err := hook(roomID, doc); err != nil {
			log.Warn("bootstrap failed; room continues with an empty document",
				"roomID", roomID, "err", err)
		}
	}
	return handle, nil
}

// StartJanitor launches the idle-eviction goroutine. Idempotent.
func (r *Runtime) StartJanitor() {
	r.janitorOnce.Do(func() {
		r.janitorStartMu.Lock()
		r.janitorStarted = true
		r.janitorStartMu.Unlock()
		go r.janitorLoop()
	})
}

// Stop halts the janitor and waits for it to exit. Safe if never started.
func (r *Runtime) Stop() {
	r.stopOnce.Do(func() { close(r.stop) })
	r.janitorStartMu.Lock()
	started := r.janitorStarted
	r.janitorStartMu.Unlock()
	if started {
		<-r.janitorDone
	}
}

func (r *Runtime) janitorLoop() {
	defer close(r.janitorDone)
	ticker := time.NewTicker(JanitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.evictIdleDocs()
		}
	}
}

// evictIdleDocs closes documents that have been quiet longer than
// MaxIdleDuration AND whose room is no longer live.
//
// The liveness exemption is the one behavioral difference from text's janitor,
// and it matters for a room that carries presence alongside its document: a
// board can sit open with people connected for hours without anyone editing a
// description, and evicting the doc under the live room would strand the next
// edit against a closed handle.
//
// Lock ordering: Close takes the handle mutex and then the runtime mutex, so
// the handle pointers are snapshotted under RLock and inspected after release.
func (r *Runtime) evictIdleDocs() {
	cutoff := now().Add(-MaxIdleDuration)
	r.mu.RLock()
	type candidate struct {
		handle *Handle
		live   bool
	}
	snapshot := make([]candidate, 0, len(r.handles))
	for id, h := range r.handles {
		snapshot = append(snapshot, candidate{handle: h, live: r.live[id]})
	}
	r.mu.RUnlock()

	for _, c := range snapshot {
		if c.live {
			continue
		}
		if c.handle.LastActivity().Before(cutoff) {
			_ = c.handle.Close()
		}
	}
}

// closeDoc drops a room's entries. Reports whether the room was registered.
func (r *Runtime) closeDoc(roomID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, existed := r.docs[roomID]
	delete(r.docs, roomID)
	delete(r.handles, roomID)
	delete(r.rooms, roomID)
	delete(r.live, roomID)
	return existed
}

// Handle is one room's server-side document mirror. It satisfies
// realtime.DocHandle and is safe for concurrent use.
type Handle struct {
	runtime *Runtime
	id      string

	mu           sync.Mutex
	doc          *Doc // nil after Close
	closed       bool
	lastActivity time.Time
}

// RoomID returns the broker's identifier for this handle's room.
func (h *Handle) RoomID() string { return h.id }

// LastActivity reports the most recent ApplyUpdate / EncodeStateAsUpdate /
// WithDoc time, for the janitor.
func (h *Handle) LastActivity() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastActivity
}

// ApplyUpdate folds an inbound update into the mirror.
//
// y-crdt logs and returns on malformed input rather than surfacing an error;
// the recover guard is insurance against that contract changing, so hostile
// input cannot take down the broker goroutine.
func (h *Handle) ApplyUpdate(payload []byte) error {
	if len(payload) > MaxApplyUpdateBytes {
		return fmt.Errorf("yjsdoc: update of %d bytes exceeds the %d cap for room %s",
			len(payload), MaxApplyUpdateBytes, h.id)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.doc == nil {
		return fmt.Errorf("yjsdoc: update applied to closed room %s", h.id)
	}
	h.lastActivity = now()

	var applyErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				applyErr = fmt.Errorf("yjsdoc: update panicked for room %s: %v", h.id, rec)
			}
		}()
		ycrdt.ApplyUpdate(h.doc, payload, nil)
	}()
	return applyErr
}

// EncodeStateAsUpdate returns the bytes a joining client needs to catch up.
func (h *Handle) EncodeStateAsUpdate() ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.doc == nil {
		return nil, fmt.Errorf("yjsdoc: state requested from closed room %s", h.id)
	}
	h.lastActivity = now()

	var state []byte
	var encErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				encErr = fmt.Errorf("yjsdoc: encode panicked for room %s: %v", h.id, rec)
			}
		}()
		state = ycrdt.EncodeStateAsUpdate(h.doc, nil)
	}()
	return state, encErr
}

// WithDoc runs fn with exclusive access to the document, holding the same mutex
// as ApplyUpdate and EncodeStateAsUpdate.
//
// This is how a consumer reads or mutates document content without racing the
// broker — the flush path snapshots every fragment through it. Do not retain
// the *Doc beyond fn: once Close runs, the pointer is dead.
func (h *Handle) WithDoc(fn func(doc *Doc) error) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.doc == nil {
		return fmt.Errorf("yjsdoc: document access on closed room %s", h.id)
	}
	h.lastActivity = now()
	return fn(h.doc)
}

// Close releases the document. Idempotent.
func (h *Handle) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.doc = nil
	h.mu.Unlock()

	h.runtime.closeDoc(h.id)
	return nil
}
