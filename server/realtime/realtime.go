// Package realtime is a generic in-memory WebSocket room broker for
// Yjs collaboration. It fans out opaque binary frames between connected
// clients of the same room. Document state, awareness, and the
// y-protocols sync handshake all flow over the same connection,
// multiplexed by a 1-byte message-type tag.
//
// The broker does not interpret payloads. Each room kind plugs in its
// own Go authorize handler via RegisterRoomKind. Sheets is the first
// consumer; other packages (mail, calendar, drive) can use the same
// primitive for their own collaboration features.
//
// State is fully in-memory: when the last client of a room disconnects,
// the room is gone. There is no durability layer here — that is by
// design. Consumers who need persistent collaborative state must layer
// it on top (e.g. by exporting periodic snapshots to durable storage).
//
// Authorization invariant: the only server-enforced gate is the
// per-room-kind Authorize callback at connection time. Once admitted,
// peers can send any wire frames the protocol supports, and the broker
// fans them out unchanged. Yjs has no built-in granular access control
// (no field/key-level ACL inside a doc), so consumers that need
// finer-grained edit authorization must enforce it at a higher layer
// (typically the durability layer that decides what to persist).
//   ref: https://discuss.yjs.dev/t/common-concepts-best-practices/2436
package realtime

import (
	"sync"
	"time"
)

// MessageType identifies the kind of payload a frame carries. The broker
// only uses this for routing decisions; the payload bytes themselves are
// opaque.
type MessageType byte

const (
	// MsgDocUpdate carries a Yjs document update. Fanned out to every
	// other client in the room.
	MsgDocUpdate MessageType = 0x01
	// MsgAwarenessUpdate carries a y-protocols awareness update. Fanned
	// out to every other client in the room.
	MsgAwarenessUpdate MessageType = 0x02
	// MsgSyncRequest is sent by a joining client asking for current doc
	// state. Forwarded to one current peer (longest-connected).
	MsgSyncRequest MessageType = 0x03
	// MsgSyncReply carries the response to a SyncRequest. Forwarded only
	// to the original requester.
	MsgSyncReply MessageType = 0x04
	// MsgAssignID is the very first frame the server sends to a freshly
	// connected client. Its payload is empty; the routing-ID prefix
	// IS the assignment. Subsequent inbound frames from this client
	// must use the same prefix or the connection is closed.
	MsgAssignID MessageType = 0x05
	// MsgServerHello is sent by the server immediately after MsgAssignID
	// (and before sync) when the room kind has registered an OnConnect
	// ServerHelloFn. The payload is opaque JSON-typed bytes the kind
	// itself defines; clients must stash it on the room handle and
	// surface it to consumers (e.g. text uses it for {readOnly,
	// importWarnings}). Skipped entirely when OnConnect is nil.
	MsgServerHello MessageType = 0x06
	// MsgServerSlot is broadcast by the server to every member of a room
	// to surface room-wide state (e.g. saveStatus indicators) without
	// using y-protocols awareness. The payload is opaque JSON-typed bytes
	// the room kind defines; clients route it via onServerSlot, never
	// applying it to doc/awareness. See Room.PublishServerSlot.
	MsgServerSlot MessageType = 0x07
)

// clientIDLen is the fixed 16-byte UUID prefix on every wire frame.
// IDs are assigned by the server (sent as the first frame after
// connect via MsgAssignID); the server enforces that subsequent
// inbound frames use the assigned prefix, so peers cannot impersonate
// each other on the wire.
const clientIDLen = 16

// frameOverhead is clientID + msgType.
const frameOverhead = clientIDLen + 1

// Broker holds all currently-active rooms. One Broker instance per process
// is the expected deployment. New rooms spring into existence on first
// joiner and disappear when their last client leaves.
type Broker struct {
	mu    sync.Mutex
	rooms map[roomKey]*Room
}

type roomKey struct {
	kind string
	id   string
}

// NewBroker returns a freshly initialized Broker.
func NewBroker() *Broker {
	return &Broker{rooms: map[roomKey]*Room{}}
}

// join admits a Client to the named room, creating the room if necessary.
// The caller is responsible for having already authorized the connection.
// Once join returns, the client may send and receive frames; on
// disconnect, the caller must invoke (*Client).leave to remove it.
//
// On the first join into a (kind, id), the broker constructs the Room
// with whatever RoomKindOptions the kind registered. A kind that
// registered via the legacy RegisterRoomKind has no DocRuntime and no
// hooks, so the room behaves as a pure relay (the prior behavior).
func (b *Broker) join(kind, id string, c *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := roomKey{kind, id}
	room, ok := b.rooms[key]
	if !ok {
		// optionsFor returning ErrUnknownRoomKind is normally
		// impossible here because handleConnect already authorized
		// against the same registry. If it does happen (e.g. the
		// kind was unregistered between authorize and join), we
		// fall back to a zero-options room — no server doc, no
		// hooks — which is the safest behavior.
		opts, _ := optionsFor(kind)
		room = newRoom(b, key, opts)
		b.rooms[key] = room
	}
	room.add(c)
}

// removeRoom is called by a Room when its last client leaves. It clears
// the entry from the broker's map so that a subsequent join in that room
// starts from scratch.
func (b *Broker) removeRoom(key roomKey) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room, ok := b.rooms[key]; ok && room.isEmpty() {
		delete(b.rooms, key)
	}
}

// roomCount returns the number of active rooms. Used in tests.
func (b *Broker) roomCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.rooms)
}

// lookupRoomForTest returns the room handle for (kind, id) or nil if no
// such room exists. Used by tests that need to call methods on the
// broker's Room directly (e.g. PublishServerSlot). Production code must
// not rely on this — joiners always go through the broker's join path.
func (b *Broker) lookupRoomForTest(kind, id string) *Room {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rooms[roomKey{kind, id}]
}

// Client is one connected WebSocket. The transport-specific read/write
// loop lives in register.go and calls into Client to forward frames into
// the broker; the broker pushes frames out via Client.send.
type Client struct {
	id       [clientIDLen]byte
	room     *Room
	joinedAt time.Time

	// authID is the connection's identity for room-kind logic. For an
	// authenticated PB user it's the user record id; for an anonymous
	// share-session visitor it's the anon id. Empty in tests that bypass
	// the production handleConnect path. Consumers reach for this in
	// OnConnect / ServerHelloFn when the per-connection hello payload
	// depends on the user's identity (e.g. role-based read-only flags).
	authID string

	// displayName is the human-facing name for this connection, set for
	// anonymous share-session visitors ("Anon <Animal>"). Empty for
	// authenticated users — their name is resolved from their record
	// elsewhere. Consumers use it to seed presence/awareness for anons.
	displayName string

	// shareRole is the share-link role ("editor", ...) for an anonymous
	// share-session visitor; empty for authenticated users. Lets a
	// kind's OnConnect decide read-only state for anons without re-
	// resolving a drive_shares row (which an anon doesn't have).
	shareRole string

	// readOnly caches the connection's write authorization, resolved once
	// by the room kind's OnConnect handler (which has DB access) rather
	// than re-querying on every inbound frame. The broker's WritePredicate
	// reads this in the hot route path, so it must be a pure field access.
	// Defaults false; OnConnect calls SetReadOnly to set the real value
	// before the first MsgDocUpdate can arrive (OnConnect runs during the
	// connection handshake, before the read loop processes frames).
	readOnly bool

	// send buffers frames the broker has decided this client should
	// receive. The transport reader pulls from this channel and writes
	// to the WebSocket. Buffer size is bounded so a slow client does
	// not pin memory; if the buffer overflows, the client is dropped.
	send chan []byte
}

// id of the client. Mostly used in tests; the byte prefix on each wire
// frame is the canonical identity at runtime.
func (c *Client) IDBytes() [clientIDLen]byte { return c.id }

// AuthID returns the identity associated with this connection (PB user
// record id, or anon id for share-session visitors). Empty for test
// clients constructed without going through the production handleConnect
// path.
func (c *Client) AuthID() string { return c.authID }

// DisplayName returns the human-facing name for an anonymous share-
// session visitor ("Anon <Animal>"), or empty for authenticated users.
func (c *Client) DisplayName() string { return c.displayName }

// ShareRole returns the share-link role for an anonymous share-session
// visitor (e.g. "editor"), or empty for authenticated users.
func (c *Client) ShareRole() string { return c.shareRole }

// IsAnonymous reports whether this connection is an anonymous share-
// session visitor rather than an authenticated PB user.
func (c *Client) IsAnonymous() bool { return c.displayName != "" }

// ReadOnly reports whether this connection is barred from writing,
// as resolved by the room kind's OnConnect handler. The broker's
// WritePredicate consults this on every inbound MsgDocUpdate; it is a
// pure field read (no DB) so the hot path stays cheap.
func (c *Client) ReadOnly() bool { return c.readOnly }

// SetReadOnly records the connection's write authorization. Intended to
// be called once from a room kind's OnConnect (ServerHelloFn) handler,
// which runs during the handshake before any frame is routed.
func (c *Client) SetReadOnly(ro bool) { c.readOnly = ro }

// NewClientForTest constructs a Client suitable for passing to a
// ServerHelloFn or similar callback in consumer-package tests. Only
// the auth id is populated; transport state is omitted. Production
// code must not call this.
func NewClientForTest(authID string) *Client {
	return &Client{authID: authID}
}

// NewAnonClientForTest constructs an anonymous share-session Client for
// use in consumer-package tests. Sets displayName (which makes
// IsAnonymous() return true) and shareRole. Production code must not
// call this.
func NewAnonClientForTest(shareRole, displayName string) *Client {
	return &Client{shareRole: shareRole, displayName: displayName}
}

// RouteFrameForTest drives a single wire frame through the broker's
// route() pipeline as if it had arrived over the WebSocket transport,
// using whatever options the kind has registered. Joins `from` into
// (kind, roomID) — creating the room if needed, applying the same
// RuntimeProvider / Journal / OnRoomCreate / etc. bootstrap as a real
// connection — then routes the frame.
//
// Returns the Room so consumer test packages can call PublishServerSlot
// or other exported Room methods. Production code must not call this —
// real connections always flow through handleConnect.
//
// Intended use: consumer packages that wire their own RoomKindOptions
// (e.g. text's Register) use this to verify the full broker route()
// path — not just the validator function in isolation — applies the
// kind's hooks correctly. Pairs with NewClientForTest /
// NewAnonClientForTest for constructing the `from` argument.
func (b *Broker) RouteFrameForTest(kind, roomID string, from *Client, frame []byte) *Room {
	b.join(kind, roomID, from)
	room := b.lookupRoomForTest(kind, roomID)
	if room == nil {
		return nil
	}
	room.route(from, frame)
	return room
}
