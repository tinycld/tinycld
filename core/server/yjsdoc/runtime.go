// Package yjsdoc is the generic server-side Yjs document runtime: a per-room
// registry of native-Go Y.Docs implementing realtime.DocRuntime, plus the
// ProseMirror bridge and update-inspection helpers a room kind needs to persist
// what its collaborators type.
//
// It is the reusable core of the machinery text/server grew first. Anything
// text-specific (docx import warnings, authorship stamping, suggestion maps,
// edit-event buffering) deliberately stayed behind in that package; what lives
// here is what a second consumer — boards' board rooms — needed verbatim.
//
// Backed by github.com/skyterra/y-crdt. Consumers refer to yjsdoc.Doc rather
// than importing y-crdt directly, which keeps the dependency inside core: a
// feature package can register a document room without adding a single external
// module to its own go.mod.
package yjsdoc

import (
	"context"
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

// now is the clock the runtime reads for LastActivity. Replaced in tests.
var now = time.Now

// BootstrapFn seeds a document from the kind's derived source. The broker
// calls Seed, and so this hook, only when it has no parked document and no
// checkpoint that matches the source; it runs before the broker serves the
// first SyncReply, so the first joiner already sees populated content.
//
// The ctx bounds the seeding work: document parsers honour it, so a
// pathological source file is abandoned rather than stalling the first
// joiner.
type BootstrapFn func(ctx context.Context, roomID string, doc *Doc) error

// Runtime is a process-wide registry of server-side Y.Docs, one per
// document the broker holds (open or parked). It satisfies
// realtime.DocRuntime. The broker owns a document's lifetime: it parks the
// document when its room empties, evicts it after realtime.ParkIdle, and
// closes it through the handle, which removes it from here.
type Runtime struct {
	// mu guards the maps below. RWMutex because the accessors are read on
	// every inbound frame while writers (NewDoc, closeDoc) are rare.
	mu        sync.RWMutex
	docs      map[string]*Doc
	handles   map[string]*Handle
	rooms     map[string]*realtime.Room
	bootstrap BootstrapFn
}

// NewRuntime returns an empty registry.
func NewRuntime() *Runtime {
	return &Runtime{
		docs:    make(map[string]*Doc),
		handles: make(map[string]*Handle),
		rooms:   make(map[string]*realtime.Room),
	}
}

// SetBootstrap registers the seeding hook. Passing nil clears it.
func (r *Runtime) SetBootstrap(hook BootstrapFn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bootstrap = hook
}

// NoteRoom associates a broker room with a roomID, for publishing
// server-originated updates. Pass nil when the room goes away.
func (r *Runtime) NoteRoom(roomID string, room *realtime.Room) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if room == nil {
		delete(r.rooms, roomID)
		return
	}
	r.rooms[roomID] = room
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

// NewDoc satisfies realtime.DocRuntime: an empty document with the patcher
// installed. Content arrives through Seed or through the broker applying a
// checkpoint.
func (r *Runtime) NewDoc(roomID string) (realtime.DocHandle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.docs[roomID]; exists {
		return nil, fmt.Errorf("yjsdoc: room %s already has a document", roomID)
	}
	doc := ycrdt.NewDoc(roomID, false, nil, nil, false)
	InstallPatcher(doc)
	handle := &Handle{runtime: r, id: roomID, doc: doc, lastActivity: now()}
	r.docs[roomID] = doc
	r.handles[roomID] = handle
	return handle, nil
}

// Seed satisfies realtime.DocRuntime: it runs the bootstrap hook on the
// document. The error is returned for the broker to log; the document stays
// usable with whatever the hook wrote, because an empty document still lets
// clients connect and edit, whereas refusing the room takes the feature down
// for everyone in it.
func (r *Runtime) Seed(ctx context.Context, roomID string, handle realtime.DocHandle) error {
	r.mu.RLock()
	hook := r.bootstrap
	r.mu.RUnlock()
	if hook == nil {
		return nil
	}
	h, ok := handle.(*Handle)
	if !ok {
		return fmt.Errorf("yjsdoc: seed of a handle this runtime did not create for room %s", roomID)
	}
	return h.WithDoc(func(doc *Doc) error { return hook(ctx, roomID, doc) })
}

// closeDoc drops a room's entries. Reports whether the room was registered.
func (r *Runtime) closeDoc(roomID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, existed := r.docs[roomID]
	delete(r.docs, roomID)
	delete(r.handles, roomID)
	delete(r.rooms, roomID)
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
// WithDoc time.
func (h *Handle) LastActivity() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastActivity
}

// ApplyUpdate folds an inbound update into the mirror. The broker caps
// inbound client frames itself; a checkpoint it restores is a whole
// document and may be far larger than any frame.
//
// y-crdt logs and returns on malformed input rather than surfacing an error;
// the recover guard is insurance against that contract changing, so hostile
// input cannot take down the broker goroutine.
func (h *Handle) ApplyUpdate(payload []byte) error {
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
