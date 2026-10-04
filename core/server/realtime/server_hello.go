package realtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// ServerHelloFn is invoked by the broker once per inbound connection,
// immediately after a client is admitted to a room and assigned its
// routing ID, but before the y-protocols sync handshake runs. The
// returned bytes are sent as a MsgServerHello frame to that client only.
//
// roomID is the broker's room identifier (same string the kind's
// Authorize handler received). conn is the freshly-joined client; the
// callback may inspect conn.IDBytes() if the payload needs to reference
// the assigned ID. Returning a non-nil error logs and skips the frame
// (the connection continues; consumers must defensively render with no
// hello payload).
//
// A nil ServerHelloFn (the default) contributes no payload of its own. A
// room with a server-side document still gets a hello that carries the
// document epoch (see withDocEpoch); a pure-relay kind gets no frame.
type ServerHelloFn func(roomID string, conn *Client) ([]byte, error)

// makeServerHelloFrame builds a MsgServerHello frame addressed to the
// given client. Wire shape: clientID(16) || msgType(1) || payload.
// Sender-ID prefix is the recipient's own ID — the routing layer
// doesn't care, but reusing the recipient's ID makes the frame
// self-consistent for any future protocol change that does validate
// sender identity.
func makeServerHelloFrame(id [clientIDLen]byte, payload []byte) []byte {
	frame := make([]byte, frameOverhead+len(payload))
	copy(frame[:clientIDLen], id[:])
	frame[clientIDLen] = byte(MsgServerHello)
	copy(frame[frameOverhead:], payload)
	return frame
}

// DocEpochHelloKey is the field the broker adds to every hello of a room
// that has a server-side document. Its value names the document
// incarnation; the client discards its local document when the value it
// synced under changes. See Checkpoint.Epoch.
const DocEpochHelloKey = "docEpoch"

// withDocEpoch merges the room's epoch into the kind's hello payload. A
// kind without an OnConnect handler has no payload of its own, so the
// hello becomes {"docEpoch": N}; a kind's JSON object gains the field. A
// payload that is not a JSON object is an error: the epoch cannot be
// carried and the hello is better skipped than sent without it.
func withDocEpoch(payload []byte, epoch int64) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(payload)) > 0 {
		if err := json.Unmarshal(payload, &fields); err != nil {
			return nil, fmt.Errorf("realtime: hello payload is not a JSON object: %w", err)
		}
	}
	fields[DocEpochHelloKey] = json.RawMessage(strconv.FormatInt(epoch, 10))
	return json.Marshal(fields)
}
