package realtime

import (
	"bytes"
	"context"
	"sync"
	"time"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("realtime")

// sendBufferSize is the number of frames buffered per client. A slow
// reader that backs up past this many frames is dropped — that is, the
// broker prefers to disconnect a stuck client over blocking the room.
const sendBufferSize = 64

// Room holds the set of currently-connected clients for one (kind, id)
// pair plus the fan-out routing logic. Members access mu via add, remove,
// and the routing methods.
type Room struct {
	broker *Broker
	key    roomKey
	opts   RoomKindOptions

	// serverDoc is the broker's authoritative mirror of the room's
	// document state. Non-nil iff the room kind registered a
	// RuntimeProvider. When non-nil, every inbound MsgDocUpdate is
	// applied here before fan-out, and MsgSyncRequest replies are
	// served from EncodeStateAsUpdate. Closed exactly once on
	// teardown after the OnEmpty callback (if any) returns.
	serverDoc DocHandle

	mu sync.Mutex
	// draining flips true — under mu, atomically with the membership
	// delete that emptied the room — the moment teardown is committed.
	// From then on add refuses new members, because everything that
	// follows (OnEmpty's final flush, per-kind state teardown, closing
	// serverDoc, removeRoom) assumes nobody is inside. A client admitted
	// in that window would land in a half-torn-down room: its persistence
	// hooks already deregistered, its server doc about to close — every
	// edit silently unpersisted and sync served from nothing. Broker.join
	// retries until removeRoom frees the key, then builds a fresh room.
	draining bool
	// epoch names the document incarnation serverDoc holds. Minted when the
	// document was seeded from the derived source and carried across every
	// park, reopen and checkpoint of that same document; sent to each client
	// in its hello so a client can tell a rebuilt document from a reopened
	// one. Written once in newRoom, read afterwards.
	epoch int64
	// openedFrom records where serverDoc's content came from, for logs and
	// tests: "parked", "checkpoint", "seed" or "relay".
	openedFrom string
	members    map[*Client]struct{}
}

func newRoom(b *Broker, key roomKey, opts RoomKindOptions, parked *parkedDoc) *Room {
	r := &Room{
		broker:     b,
		key:        key,
		opts:       opts,
		members:    map[*Client]struct{}{},
		openedFrom: "relay",
	}
	if opts.RuntimeProvider == nil {
		if parked != nil {
			// The kind lost its runtime between park and reopen (a test
			// re-registered it). Nothing can use the document now.
			parked.close(key)
		}
		return r
	}
	var floor int64
	if parked != nil {
		if r.adoptParked(parked) {
			if opts.OnRoomCreate != nil {
				opts.OnRoomCreate(key.id, r.serverDoc, r)
			}
			return r
		}
		// A client may still hold the parked incarnation; the new epoch
		// must differ from it even inside the same millisecond.
		floor = parked.epoch
	}
	handle, err := opts.RuntimeProvider.NewDoc(key.id)
	if err != nil {
		// Construction failure falls back to pure-relay
		// behavior: clients can still join and fan out frames
		// among themselves; only the server-side mirror (and
		// therefore persistence) is disabled for this room.
		log.Error(
			"DocRuntime.NewDoc failed; falling back to pure relay",
			"kind", key.kind, "roomID", key.id, "err", err,
		)
		return r
	}
	handle, restored := r.openDoc(handle, floor)
	if handle == nil {
		return r
	}
	r.serverDoc = handle
	if opts.OnRoomCreate != nil {
		opts.OnRoomCreate(key.id, handle, r)
	}
	if restored && opts.OnDocUpdate != nil {
		// A checkpoint may hold edits a failed flush never wrote to the
		// derived file. Marking the room dirty gets them there on the
		// normal save schedule; for a checkpoint that matched the file it
		// costs one export.
		opts.OnDocUpdate(key.id)
	}
	return r
}

// adoptParked reuses a parked document when the derived source still has
// the fingerprint the document was parked under. Reports false when the
// parked document was stale and has been closed, so the caller builds and
// seeds a fresh one.
func (r *Room) adoptParked(p *parkedDoc) bool {
	fp, err := r.opts.Fingerprint(r.key.id)
	if err == nil && fp == p.fingerprint {
		r.serverDoc = p.handle
		r.epoch = p.epoch
		r.openedFrom = "parked"
		log.Info("room reopened from its parked document",
			"kind", r.key.kind, "roomID", r.key.id, "epoch", r.epoch)
		return true
	}
	if err != nil {
		log.Warn("fingerprint failed; dropping the parked document",
			"kind", r.key.kind, "roomID", r.key.id, "err", err)
	} else {
		log.Info("source changed while the document was parked; re-seeding",
			"kind", r.key.kind, "roomID", r.key.id)
	}
	p.close(r.key)
	if r.opts.Checkpoints != nil {
		if err := r.opts.Checkpoints.Delete(r.key.kind, r.key.id); err != nil {
			log.Warn("checkpoint delete failed", "kind", r.key.kind, "roomID", r.key.id, "err", err)
		}
	}
	return false
}

// openDoc gives a fresh handle its content: the stored checkpoint when its
// fingerprint still matches the derived source, else a seed from that
// source under a new epoch above floor and above any stored one. Returns
// the handle to use (a replacement when a failed checkpoint apply tainted
// the first one; nil when even that failed) and whether the content came
// from a checkpoint.
func (r *Room) openDoc(handle DocHandle, floor int64) (DocHandle, bool) {
	var (
		previous = floor
		found    bool
		cp       Checkpoint
		fp       string
	)
	if r.opts.Checkpoints != nil && r.opts.Fingerprint != nil {
		var err error
		fp, err = r.opts.Fingerprint(r.key.id)
		if err != nil {
			log.Warn("fingerprint failed; the room will be seeded",
				"kind", r.key.kind, "roomID", r.key.id, "err", err)
		} else {
			cp, found, err = r.opts.Checkpoints.Load(r.key.kind, r.key.id)
			if err != nil {
				log.Error("checkpoint load failed; the room will be seeded",
					"kind", r.key.kind, "roomID", r.key.id, "err", err)
				found = false
			}
		}
		if found {
			previous = max(previous, cp.Epoch)
			if cp.Fingerprint == fp {
				if restoreCheckpoint(handle, cp.State) {
					r.epoch = cp.Epoch
					r.openedFrom = "checkpoint"
					log.Info("room opened from its checkpoint",
						"kind", r.key.kind, "roomID", r.key.id, "epoch", r.epoch, "bytes", len(cp.State))
					return handle, true
				}
				// The apply may have left part of the state behind; a
				// seed on top of it would duplicate content.
				log.Error("checkpoint did not apply cleanly; re-seeding on a fresh document",
					"kind", r.key.kind, "roomID", r.key.id)
				closeHandle(r.key, handle)
				var err error
				handle, err = r.opts.RuntimeProvider.NewDoc(r.key.id)
				if err != nil {
					log.Error("DocRuntime.NewDoc failed after a bad checkpoint; falling back to pure relay",
						"kind", r.key.kind, "roomID", r.key.id, "err", err)
					return nil, false
				}
			} else {
				log.Info("source changed since the checkpoint; re-seeding",
					"kind", r.key.kind, "roomID", r.key.id)
			}
			if err := r.opts.Checkpoints.Delete(r.key.kind, r.key.id); err != nil {
				log.Warn("checkpoint delete failed", "kind", r.key.kind, "roomID", r.key.id, "err", err)
			}
		}
	}
	if err := r.opts.RuntimeProvider.Seed(context.Background(), r.key.id, handle); err != nil {
		log.Warn("seed failed; room continues with what the seed wrote",
			"kind", r.key.kind, "roomID", r.key.id, "err", err)
	}
	r.epoch = MintEpoch(previous)
	r.openedFrom = "seed"
	return handle, false
}

// DocEpoch names the incarnation of this room's server-side document, or
// 0 for a pure-relay room.
func (r *Room) DocEpoch() int64 {
	if r.serverDoc == nil {
		return 0
	}
	return r.epoch
}

// add admits a client, reporting whether the room accepted it. A false
// means the room's last member just left and teardown is in progress;
// the caller must wait for the key to free and join a fresh room.
func (r *Room) add(c *Client) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.draining {
		return false
	}
	c.room = r
	c.send = make(chan []byte, sendBufferSize)
	r.members[c] = struct{}{}
	return true
}

// remove drops a client from the room and releases the empty room back
// to the broker. broadcastLeave should be called separately by the
// transport layer once the client's send loop has stopped, so that
// remaining members get a synthetic "this user left" frame.
//
// When this drops the last member, the room invokes its OnEmpty
// callback (synchronously) before closing the server-side DocHandle.
// OnEmpty is allowed to take time (e.g. a final persistence flush);
// the broker's removeRoom call is what frees the room key. Until then
// the draining flag keeps a quick rejoin out of this room — join spins
// until the key frees and then constructs a fresh room, whose bootstrap
// re-reads exactly the state the final flush wrote.
func (r *Room) remove(c *Client) {
	r.mu.Lock()
	delete(r.members, c)
	empty := len(r.members) == 0
	if empty {
		// Committing to teardown and refusing further joins is ONE
		// locked step. Without it, `empty` goes stale the moment the
		// lock drops: a rejoin (a page reload reconnects while OnEmpty's
		// final flush is still writing) would land in this room, and the
		// teardown below would then rip the doc and the persistence
		// hooks out from under a live member.
		r.draining = true
	}
	// Closed while STILL HOLDING r.mu, together with the delete above.
	//
	// A sender only ever reaches a client it found in r.members, and it
	// finds that under this same lock — so closing here makes "removed
	// from the room" and "channel closed" one atomic step, and no sender
	// can be holding a reference that is about to go stale. Closing after
	// the unlock left a window that fanOut walked straight into: it
	// snapshots its peers under the lock and releases it before
	// delivering, so a client removed in between had its channel closed
	// under a send already in flight, panicking the whole process with
	// "send on closed channel". deliver's select/default does not help —
	// that guards a FULL channel; a send to a CLOSED one panics either
	// way.
	close(c.send)
	r.mu.Unlock()
	if empty {
		if r.opts.OnEmpty != nil {
			r.opts.OnEmpty(r.key.id)
		}
		if r.serverDoc != nil {
			r.releaseDoc()
			r.serverDoc = nil
		}
		r.broker.removeRoom(r.key)
	}
}

// releaseDoc parks the server document for a later reopen when the kind
// keeps checkpoints, and closes it otherwise. The fingerprint is read now,
// after OnEmpty's final flush, so it names the file that flush wrote.
func (r *Room) releaseDoc() {
	if r.opts.Checkpoints != nil && r.opts.Fingerprint != nil {
		fp, err := r.opts.Fingerprint(r.key.id)
		if err == nil {
			r.broker.park(r.key, &parkedDoc{
				handle:      r.serverDoc,
				epoch:       r.epoch,
				fingerprint: fp,
				parkedAt:    time.Now(),
				opts:        r.opts,
			})
			return
		}
		log.Warn("fingerprint failed; closing the document instead of parking it",
			"kind", r.key.kind, "roomID", r.key.id, "err", err)
	}
	closeHandle(r.key, r.serverDoc)
}

// closeHandle closes a server document and logs a failure; the broker has
// nothing better to do with one.
func closeHandle(key roomKey, handle DocHandle) {
	if err := handle.Close(); err != nil {
		log.Warn("DocHandle.Close failed", "kind", key.kind, "roomID", key.id, "err", err)
	}
}

func (r *Room) isEmpty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members) == 0
}

// HasWriter reports whether any current member of the room is
// authorized to write (i.e. `ReadOnly()` returns false). Used by
// downstream consumers — text's editEvent buffer in particular — to
// suppress per-frame audience-only producer work when the only
// connected peers are read-only viewers.
//
// Holds r.mu for the membership read. The ReadOnly flag itself is a
// pure-read accessor on Client (set once by OnConnect) and safe to
// call under the room mutex.
func (r *Room) HasWriter() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for c := range r.members {
		if !c.ReadOnly() {
			return true
		}
	}
	return false
}

// HasOtherWriter reports whether any current member of the room OTHER
// THAN `excluding` is authorized to write. Used by audience-only
// producer paths to skip work when the only writer in the room is the
// sender themselves — solo author edits don't need to journal
// activity-feed events for nobody else to read.
//
// Pass the sender of the inbound frame as `excluding`. A nil
// `excluding` behaves like HasWriter.
func (r *Room) HasOtherWriter(excluding *Client) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for c := range r.members {
		if c == excluding {
			continue
		}
		if !c.ReadOnly() {
			return true
		}
	}
	return false
}

// route handles a single frame the transport layer just read off `from`'s
// WebSocket. The frame is the full wire bytes (clientID || msgType || payload).
// The broker decides who else should receive it.
func (r *Room) route(from *Client, frame []byte) {
	if len(frame) < frameOverhead {
		return
	}
	msgType := MessageType(frame[clientIDLen])
	switch msgType {
	case MsgDocUpdate:
		// Server-side write gate: drop mutations from connections the
		// room kind deems read-only. Without this, "read-only" is only a
		// client-side UI flag a crafted client could ignore. Silent drop
		// (not a connection close) so a benign client with a stale flag
		// isn't disconnected.
		if r.opts.WritePredicate != nil && !r.opts.WritePredicate(from, r.key.id) {
			log.Warn("dropped MsgDocUpdate from read-only connection",
				"kind", r.key.kind, "roomID", r.key.id, "authID", from.authID)
			return
		}
		payload := frame[frameOverhead:]
		limit := r.opts.MaxUpdateBytes
		if limit == 0 {
			limit = DefaultMaxUpdateBytes
		}
		if len(payload) > limit {
			log.Warn(
				"MsgDocUpdate exceeds cap; dropping",
				"kind", r.key.kind, "roomID", r.key.id,
				"size", len(payload), "cap", limit,
			)
			return
		}
		// Content-level reject: the kind's validator inspects the
		// decoded update structure and refuses frames that mutate
		// protected Y.Doc roots (see ProtectedYjsRootKeys). Runs after
		// the size + write-permission gates so the cheap checks fire
		// first. A non-nil error drops the frame silently — the
		// sender's local Y.Doc retains the edit, but it never reaches
		// the journal, the server mirror, or any peer.
		if r.opts.UpdateContentValidator != nil {
			if err := r.opts.UpdateContentValidator(r.key.id, payload); err != nil {
				log.Warn(
					"UpdateContentValidator rejected MsgDocUpdate; dropping",
					"kind", r.key.kind, "roomID", r.key.id, "err", err,
				)
				return
			}
		}
		// Apply to the server-side mirror first so a malformed update
		// fails fast and we don't fan out a corrupt frame to peers.
		// If no server mirror is configured, the broker is in pure-
		// relay mode for this kind and we just forward.
		if r.serverDoc != nil {
			if err := r.serverDoc.ApplyUpdate(payload); err != nil {
				log.Warn(
					"ApplyUpdate rejected an inbound MsgDocUpdate; dropping frame",
					"kind", r.key.kind, "roomID", r.key.id, "err", err,
				)
				return
			}
		}
		r.fanOut(from, frame)
		if r.opts.OnDocUpdate != nil {
			r.opts.OnDocUpdate(r.key.id)
		}
		if r.opts.OnDocUpdateContent != nil {
			r.opts.OnDocUpdateContent(r.key.id, from, payload)
		}
	case MsgAwarenessUpdate:
		r.fanOut(from, frame)
	case MsgAwarenessHello:
		// Announce-only: record the sender's awareness clientID so
		// broadcastLeave can name their slot, then stop. Deliberately NOT
		// fanned out — peers learn each other's slots from the awareness
		// payloads themselves.
		//
		// A malformed varuint is ignored rather than fatal: the only cost
		// is falling back to the legacy zero-length leave frame for this
		// connection. Do NOT give this switch a `default:` that closes the
		// connection — a newer client talking to an older broker relies on
		// unknown frames being dropped silently.
		if id, ok := readVarUint(frame[frameOverhead:]); ok {
			from.setYjsClientID(id)
		}
	case MsgSyncRequest:
		// If a server-side mirror is configured, the server is the
		// source of truth: build a SyncReply directly from the
		// mirror and send it to the requester. Skip the peer-bounce
		// path entirely.
		if r.serverDoc != nil {
			state, err := r.serverDoc.EncodeStateAsUpdate()
			if err != nil {
				log.Warn(
					"EncodeStateAsUpdate failed; falling back to peer bounce",
					"kind", r.key.kind, "roomID", r.key.id, "err", err,
				)
			} else {
				deliver(from, makeServerSyncReply(state))
				return
			}
		}
		// Fall back to the legacy pure-relay path: forward to one
		// current peer (longest-connected). Picked and delivered in ONE
		// locked step: a peer chosen under the lock and written to after
		// releasing it may have been removed (and its channel closed) in
		// between, which panics the process.
		if r.deliverToSyncPeer(from, frame) {
			return
		}
		// No peer: the requester is alone. Send back an empty reply so
		// the client knows to fall back to its bootstrap path
		// (e.g. parsing the .xlsx for sheets).
		deliver(from, makeEmptySyncReply())
	case MsgSyncReply:
		// SyncReply targets one specific client by ID. The first
		// clientID prefix in the frame is the *replying* peer (whoever
		// they are); the recipient is identified by an additional
		// 16-byte target ID immediately after the message-type tag.
		if len(frame) < frameOverhead+clientIDLen {
			return
		}
		var target [clientIDLen]byte
		copy(target[:], frame[frameOverhead:frameOverhead+clientIDLen])
		// Strip the routing target from the frame the recipient sees;
		// the recipient only needs the sender ID + reply payload.
		// Single allocation + two copies into pre-sized buffer
		// instead of two appends.
		stripped := make([]byte, len(frame)-clientIDLen)
		copy(stripped, frame[:frameOverhead])
		copy(stripped[frameOverhead:], frame[frameOverhead+clientIDLen:])
		r.deliverByID(target, stripped)
	}
}

// fanOut writes frame to every member of the room except `from`.
//
// Delivery happens UNDER r.mu rather than to a snapshot taken under it.
// The lock is what makes a member's send channel safe to use: remove()
// deletes from r.members and closes that channel as one locked step, so a
// client reached from inside this loop is guaranteed not to be mid-close.
// Snapshotting and then delivering unlocked reintroduces exactly the race
// this pairing exists to close — a peer removed between the two panics the
// process on "send on closed channel". deliver never blocks (it drops on a
// full buffer), so holding the lock across the loop costs a few buffered
// writes, not a wait on any client.
//
// Peers that could NOT take the frame are collected and repaired afterwards,
// outside the lock — see resyncStarved, which re-checks membership under it
// before writing to any of them, for the same reason the loop above holds it.
func (r *Room) fanOut(from *Client, frame []byte) {
	var starved []*Client
	r.mu.Lock()
	for c := range r.members {
		if c == from {
			continue
		}
		if !deliver(c, frame) {
			starved = append(starved, c)
		}
	}
	r.mu.Unlock()

	if len(starved) > 0 {
		r.resyncStarved(starved, frame)
	}
}

// resyncStarved repairs peers whose send buffer was full when a frame was
// fanned out to them.
//
// A dropped frame is not merely a late frame. Yjs emits each change as a delta
// exactly once and nothing retransmits it, so a peer that misses one is
// permanently short of that edit — it keeps editing and rendering a document
// that silently disagrees with everyone else's, and the divergence is invisible
// until someone reads the record back. Sending the mirror's whole state closes
// that: a CRDT merging state it already holds is a no-op, so this is safe to
// send to a peer that was only briefly behind.
//
// Runs with r.mu RELEASED. EncodeStateAsUpdate takes the document's own lock,
// and the flush path already takes that lock before touching the room, so
// calling it under r.mu would invert the order.
//
// Only document updates are repairable this way. A dropped awareness frame is
// ephemeral — the next publish supersedes it — so those keep the old behavior.
func (r *Room) resyncStarved(starved []*Client, frame []byte) {
	if r.serverDoc == nil || len(frame) < frameOverhead {
		return
	}
	if MessageType(frame[clientIDLen]) != MsgDocUpdate {
		return
	}
	state, err := r.serverDoc.EncodeStateAsUpdate()
	if err != nil {
		log.Error(
			"EncodeStateAsUpdate failed; a peer is left missing an update",
			"kind", r.key.kind, "roomID", r.key.id, "err", err,
		)
		return
	}
	catchUp := makeServerSyncReply(state)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range starved {
		// Re-check membership: the client may have left while the lock was
		// released, and its send channel is closed under this same lock.
		if _, ok := r.members[c]; !ok {
			continue
		}
		if !deliver(c, catchUp) {
			// Still full. The transport's liveness checks will close this
			// connection, and its reconnect resyncs from scratch.
			log.Warn(
				"peer too far behind to catch up; leaving it to the transport",
				"kind", r.key.kind, "roomID", r.key.id,
			)
		}
	}
}

// deliverToSyncPeer sends frame to the longest-connected peer that isn't
// the requester, and reports whether such a peer existed. Choosing and
// sending are one locked step by design — see fanOut for why a peer picked
// under r.mu must not be written to after releasing it.
func (r *Room) deliverToSyncPeer(from *Client, frame []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	var best *Client
	for c := range r.members {
		if c == from {
			continue
		}
		if best == nil || c.joinedAt.Before(best.joinedAt) {
			best = c
		}
	}
	if best == nil {
		return false
	}
	deliver(best, frame)
	return true
}

// deliverByID sends frame to the room member whose id matches target.
func (r *Room) deliverByID(target [clientIDLen]byte, frame []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for c := range r.members {
		if bytes.Equal(c.id[:], target[:]) {
			deliver(c, frame)
			return
		}
	}
}

// broadcastLeave synthesizes a frame announcing that `c` has disconnected
// and fans it out to remaining members. Called by the transport once the
// client's read/write loop has finished.
//
// When the client announced its awareness clientID via MsgAwarenessHello,
// the frame carries a REAL y-protocols awareness removal payload naming
// that slot, so peers drop the avatar immediately. This is the whole point
// of the hello: an ungraceful disconnect (TCP reset, killed tab) never gets
// to send its own removal, and without a named slot a peer cannot tell
// which avatar the leave refers to.
//
// When it did not (an older client build), we fall back to the legacy
// zero-length payload. That frame is not actionable by the receiver — it
// leaves the ghost to y-protocols' own 30s reaper — but emitting it keeps
// the wire contract unchanged.
func (r *Room) broadcastLeave(c *Client) {
	var payload []byte
	if yjsID, ok := c.YjsClientID(); ok {
		payload = encodeAwarenessRemoval(yjsID, awarenessRemovalClock)
	}
	frame := make([]byte, frameOverhead+len(payload))
	copy(frame[:clientIDLen], c.id[:])
	frame[clientIDLen] = byte(MsgAwarenessUpdate)
	copy(frame[frameOverhead:], payload)
	r.fanOut(c, frame)
}

// awarenessRemovalClock is the clock the broker stamps on a synthesized
// removal. y-protocols accepts an incoming awareness entry when its clock
// is GREATER than the one the receiver holds for that slot, or when the
// clocks are equal and the state is null. The broker does not track
// per-slot clocks — it never parses the awareness payloads it fans out —
// so it stamps a value no live session will reach: an awareness clock
// increments once per local state write, and y-protocols' own renewal
// ticks at most once every 15s, so 2^31 is unreachable in any real
// session lifetime.
//
// The alternative — having the hello carry the client's current clock —
// is more precise but makes the hello stateful (it would have to be
// re-sent on every local state change to stay accurate), which is a worse
// trade for a frame whose only job is to name a slot.
const awarenessRemovalClock = 1 << 31

// encodeAwarenessRemoval builds a y-protocols awareness update marking one
// client's slot as gone. Mirrors encodeAwarenessUpdate in
// y-protocols/awareness.js for the single-client, null-state case:
//
//	varUint(1)          // one entry follows
//	varUint(clientID)
//	varUint(clock)
//	varString("null")   // varUint(len) || utf8 bytes
//
// The literal "null" is what JSON.stringify(null) produces, and the
// receiving applyAwarenessUpdate takes its state===null branch on exactly
// that, calling states.delete(clientID). Pinned to real y-protocols output
// by TestEncodeAwarenessRemovalGolden.
func encodeAwarenessRemoval(clientID, clock uint64) []byte {
	const nullJSON = "null"
	out := make([]byte, 0, 16)
	out = appendVarUint(out, 1)
	out = appendVarUint(out, clientID)
	out = appendVarUint(out, clock)
	out = appendVarUint(out, uint64(len(nullJSON)))
	return append(out, nullJSON...)
}

// readVarUint decodes a lib0 varuint from the front of b. Returns false on
// a truncated or over-long encoding rather than panicking — the input is
// attacker-controlled.
func readVarUint(b []byte) (uint64, bool) {
	var v uint64
	var shift uint
	for i, by := range b {
		// A uint64 varuint is at most 10 bytes; anything longer is
		// malformed and would overflow the shift.
		if i >= 10 {
			return 0, false
		}
		v |= uint64(by&0x7F) << shift
		if by&0x80 == 0 {
			return v, true
		}
		shift += 7
	}
	return 0, false
}

// deliver pushes a frame to a client's send buffer, reporting whether it
// was accepted. If the buffer is full the frame is dropped — the read loop
// in register.go is expected to detect a stuck client via separate liveness
// checks.
//
// Callers must not treat a false as merely a slow client. For a document
// update it means that peer has permanently missed a change: Yjs emits each
// delta once, so nothing will resend it and the peer's document silently
// diverges from everyone else's. fanOut repairs that rather than ignoring it.
func deliver(c *Client, frame []byte) bool {
	select {
	case c.send <- frame:
		return true
	default:
		// Buffer overflow: drop. A persistently slow client will be
		// disconnected by the transport's idle/keepalive logic.
		return false
	}
}

// makeEmptySyncReply builds a "you are alone, no peers" reply. The
// route handler delivers it to the requester directly by pointer, so
// no target ID is needed inside the frame. The sender-ID prefix is
// left as 16 zero bytes; the client doesn't validate inbound sender
// IDs (only outbound), so this is a routing detail rather than an
// authenticated marker.
func makeEmptySyncReply() []byte {
	frame := make([]byte, frameOverhead)
	frame[clientIDLen] = byte(MsgSyncReply)
	return frame
}

// makeServerSyncReply builds a SyncReply frame whose payload is the
// server-side mirror's encoded state, wrapped in a y-protocols/sync
// "step2" envelope so the client's readSyncMessage handler dispatches
// it correctly.
//
// The y-protocols sync wire shape for step2 is:
//
//	varUint(messageYjsSyncStep2 == 1) || varUint8Array(updateBytes)
//
// Without this envelope, the raw update bytes are interpreted by the
// client as a y-protocols message — and for an empty server-side doc
// the leading byte happens to parse as messageYjsSyncStep1 (==0),
// triggering a degenerate decode path that throws "Unexpected end of
// array". Sender-ID prefix is left zero for the same reason as
// makeEmptySyncReply.
func makeServerSyncReply(state []byte) []byte {
	envelope := encodeSyncStep2(state)
	frame := make([]byte, frameOverhead+len(envelope))
	frame[clientIDLen] = byte(MsgSyncReply)
	copy(frame[frameOverhead:], envelope)
	return frame
}

// encodeSyncStep2 writes the y-protocols/sync step2 envelope: a
// varuint message-type tag (==1) followed by a length-prefixed copy
// of the update bytes. Mirrors writeSyncStep2 in y-protocols/sync.js.
func encodeSyncStep2(update []byte) []byte {
	const messageYjsSyncStep2 = 1
	out := make([]byte, 0, 1+varUintSize(uint64(len(update)))+len(update))
	out = appendVarUint(out, messageYjsSyncStep2)
	out = appendVarUint(out, uint64(len(update)))
	out = append(out, update...)
	return out
}

// appendVarUint writes lib0-compatible varuint encoding (7 bits per
// byte little-endian, MSB set on continuation). lib0/encoding's
// writeVarUint produces the same bytes.
func appendVarUint(dst []byte, n uint64) []byte {
	for n >= 0x80 {
		dst = append(dst, byte(n)|0x80)
		n >>= 7
	}
	return append(dst, byte(n))
}

// varUintSize returns how many bytes appendVarUint would write for n.
func varUintSize(n uint64) int {
	size := 1
	for n >= 0x80 {
		n >>= 7
		size++
	}
	return size
}

// serverSlotID is the reserved 16-byte client-ID under which the broker
// itself publishes room-wide state via PublishServerSlot. Placed at
// 0xFF... so it cannot collide with crypto/rand-generated client IDs
// (which are uniformly distributed across the full 16-byte space, but
// the probability of randomly hitting all-0xFF except a 16-bit suffix
// is 2^-112 — well below any realistic deployment).
//
// Clients that render presence avatars must filter out this ID (and
// any future reserved IDs in the same prefix range).
var serverSlotID = [clientIDLen]byte{
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x01,
}

// IsReservedClientID reports whether the given 16-byte ID is in the
// reserved range used for server-published slots. UI consumers (e.g.
// presence-avatar renderers) must skip slots with reserved IDs.
func IsReservedClientID(id [clientIDLen]byte) bool {
	for i := 0; i < clientIDLen-2; i++ {
		if id[i] != 0xFF {
			return false
		}
	}
	return true
}

// PublishServerSlot broadcasts a server-originated state payload to
// every member of the room. The frame is constructed with the reserved
// serverSlotID as its sender prefix and tagged MsgServerSlot, then
// fanned out via the existing broadcaster.
//
// Used by consumers (text, future packages) that need to surface
// room-wide server state — e.g. saveStatus indicators. Clients route
// this frame to a dedicated onServerSlot callback rather than the
// y-protocols awareness layer, so payload format is consumer-defined.
//
// MsgServerSlot is the routing-level signal that the frame is
// server-originated; the serverSlotID sender prefix is kept for
// self-consistency and so UI presence renderers can also filter it
// out via IsReservedClientID.
func (r *Room) PublishServerSlot(payload []byte) {
	frame := make([]byte, frameOverhead+len(payload))
	copy(frame[:clientIDLen], serverSlotID[:])
	frame[clientIDLen] = byte(MsgServerSlot)
	copy(frame[frameOverhead:], payload)
	// fanOut(nil, frame) — passing nil as `from` excludes nobody;
	// every member receives the frame.
	r.fanOut(nil, frame)
}

// PublishDocUpdate broadcasts a server-originated Yjs update to every
// member of the room and marks the room dirty, so the save schedule
// carries the change to the derived file. The frame uses serverSlotID as
// its sender prefix and MsgDocUpdate as its type, so clients integrate it
// into their Y.Doc the same way they integrate any other update.
//
// Skips:
//   - UpdateContentValidator — the validator's purpose is to reject
//     client writes to protected roots; the server is the writer here.
//   - WritePredicate — server-originated updates are never read-only.
//   - The server-side serverDoc.ApplyUpdate — the caller has already
//     mutated the doc to produce these bytes (the bytes ARE the delta
//     of that mutation); re-applying would be a no-op via Yjs's
//     idempotency, but skipping saves the cycle.
//
// Used by consumers (text's authorship stamper and edit events) that
// write metadata into the live Y.Doc and need peers to converge on it.
// Returns nil; the error result stays so a caller's handling of a
// broadcast failure keeps compiling should one become possible again.
func (r *Room) PublishDocUpdate(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	frame := make([]byte, frameOverhead+len(payload))
	copy(frame[:clientIDLen], serverSlotID[:])
	frame[clientIDLen] = byte(MsgDocUpdate)
	copy(frame[frameOverhead:], payload)
	// fanOut(nil, frame) — passing nil as `from` excludes nobody;
	// every member receives the frame.
	r.fanOut(nil, frame)
	if r.opts.OnDocUpdate != nil {
		r.opts.OnDocUpdate(r.key.id)
	}
	return nil
}
