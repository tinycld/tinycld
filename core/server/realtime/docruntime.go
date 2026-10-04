package realtime

import "context"

// DocRuntime is the contract a room kind implements when it wants the
// broker to maintain a long-lived server-side document mirror per room.
//
// Most room kinds don't need this — the broker is happy to be a pure
// relay that fans out opaque frames. But room kinds that need the
// server to persist or otherwise inspect document state register a
// DocRuntime via RoomKindOptions, and the broker then keeps a DocHandle
// alive for each active room of that kind, applying every inbound
// MsgDocUpdate frame to it.
//
// Construction and seeding are separate steps because the broker decides
// where a room's content comes from. When it holds a parked document or a
// stored checkpoint whose fingerprint still matches the derived source, the
// content is that state and Seed is never called; only a room with no usable
// state is seeded from the kind's derived source (docx, xlsx, records).
//
// DocRuntime intentionally does not depend on any specific CRDT
// library. The sheets package implements it on top of yjs running in a
// goja VM; a hypothetical future room kind could implement it on top
// of automerge, plain JSON, etc.
type DocRuntime interface {
	// NewDoc creates an EMPTY server-side document mirror for a newly
	// created room: patchers installed, registries updated, no content.
	// roomID is the broker's stable identifier for the room (e.g. a
	// drive_items.id for sheets). Returns an opaque handle the broker
	// will retain for the room's lifetime, or an error if construction
	// failed (in which case the room falls back to pure-relay behavior —
	// joining clients still succeed).
	NewDoc(roomID string) (DocHandle, error)

	// Seed populates a document NewDoc returned from the kind's derived
	// source. The broker calls it only when it has no parked document and
	// no checkpoint that matches the source. The ctx bounds the work;
	// document parsers honour it. An error is logged and the room
	// continues with whatever the seed managed to write — an empty
	// document still lets clients connect and edit.
	Seed(ctx context.Context, roomID string, handle DocHandle) error
}

// DocHandle is the broker's view of a single room's server-side
// document mirror. The broker calls ApplyUpdate for every inbound
// MsgDocUpdate and for a checkpoint it restores, EncodeStateAsUpdate
// when a new joiner needs to be bootstrapped and when it checkpoints the
// room, and Close exactly once when the document is finally dropped.
//
// Implementations must be safe to call from multiple goroutines, since
// the broker may receive updates from different connections
// concurrently. Most implementations will simply hold an internal
// mutex. ApplyUpdate must accept a payload of up to MaxCheckpointBytes:
// the broker already caps inbound client frames, and a checkpoint is a
// whole document.
type DocHandle interface {
	// ApplyUpdate folds an incoming yjs update payload (the bytes
	// after the frame's clientID + msgType prefix) into the
	// server-side mirror. A non-nil error means the update was
	// rejected — the broker logs and drops the frame; it does not
	// fan out a corrupt update to peers.
	ApplyUpdate(payload []byte) error

	// EncodeStateAsUpdate returns the bytes a new joiner needs to
	// catch up to the current state. The broker wraps these bytes in
	// a MsgSyncReply frame and sends them to the requester.
	EncodeStateAsUpdate() ([]byte, error)

	// Close releases any runtime resources tied to this room. Called
	// exactly once, after the room has emptied and either its parked
	// document was evicted or the kind keeps no checkpoints.
	Close() error
}
