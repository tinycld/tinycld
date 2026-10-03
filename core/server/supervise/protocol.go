package supervise

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// Msg is one control message between the supervisor and a server child,
// sent as one JSON object per line over the inherited control socket.
type Msg struct {
	Type string `json:"type"` // MsgReady | MsgRestart | MsgDrain | MsgRestartAck
	// Cold, on a restart, asks the supervisor to stop this child before it
	// starts the next one instead of running the two side by side.
	Cold    bool `json:"cold,omitempty"`
	Version int  `json:"version"`
}

// ProtocolVersion is the version a sender states. It is informational: the
// supervisor is the image's baked binary and its children can be newer, so
// ready, restart, drain and restart-ack are permanent version-1 messages
// that both sides act on whatever version is stated. A later version may
// add message types or optional fields, which an older side ignores; it
// must not change what these four mean.
const ProtocolVersion = 1

// ControlFD is the inherited-fd name of the control socket.
const ControlFD = "control"

const (
	MsgReady   = "ready"   // child → supervisor: listeners set, routes bound
	MsgRestart = "restart" // child → supervisor: replace me with the activated build
	MsgDrain   = "drain"   // supervisor → child: stop accepting, finish, exit
	// MsgRestartAck tells a child its restart arrived. A child that gets
	// none exits 75 instead, the restart every supervisor handles.
	MsgRestartAck = "restart-ack" // supervisor → child
)

// The names the supervisor passes its own listeners under. A package cannot
// declare a port with one of these names (the generator refuses it).
const (
	ListenerHTTP         = "http"
	ListenerHTTPS        = "https"
	ListenerHTTPRedirect = "http-redirect"
)

// ChildDrainTimeout is how long a draining child lets in-flight requests
// finish before it cuts them. PocketBase's own shutdown allows one second,
// which cuts a large upload or a slow export short. Connections that never
// go idle (realtime streams) are cut at the end of this budget, and their
// clients reconnect to the new child.
const ChildDrainTimeout = 30 * time.Second

// ErrBadMessage is what Recv returns for a line that is not a message. The
// stream is still usable: the next line is read normally.
var ErrBadMessage = errors.New("supervise: bad control message")

// Send writes m as one line. A zero Version is sent as ProtocolVersion, so
// a sender always states the version it speaks. The line goes out in one
// Write, so concurrent senders that serialize their calls never interleave.
func Send(w io.Writer, m Msg) error {
	if m.Version == 0 {
		m.Version = ProtocolVersion
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// Recv reads the next message. A read error (io.EOF when the other side
// closed the socket) is returned as is; a line that does not parse wraps
// ErrBadMessage.
func Recv(r *bufio.Reader) (Msg, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Msg{}, err
	}
	var m Msg
	if err := json.Unmarshal(line, &m); err != nil {
		return Msg{}, fmt.Errorf("%w %q: %v", ErrBadMessage, line, err)
	}
	return m, nil
}
