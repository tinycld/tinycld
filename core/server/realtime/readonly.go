package realtime

import (
	"sync"

	"github.com/coder/websocket"
	"tinycld.org/core/readonly"
)

// Document edits arrive as websocket frames, not HTTP requests, so the
// read-only middleware never sees them; every journal append would write
// during the pause. The broker therefore stops them itself: it refuses new
// upgrades, closes the open connections when the pause starts, and refuses a
// frame that slips in before the close. A closed client reconnects with
// backoff (core/lib/realtime/client.ts) and, once writes are accepted again,
// resyncs and sends the edits it queued while disconnected.

const readOnlyCloseReason = "server is read-only; reconnect shortly"

var (
	readOnlyHooksOnce sync.Once

	// openConns maps each live room connection's Client to its websocket,
	// so a pause can close them all and a refused frame can close its sender.
	openConnsMu sync.Mutex
	openConns   = map[*Client]*websocket.Conn{}
)

// registerReadOnlyHooks registers the pause hook once per process: OnEnter
// registrations are process-wide and never removed, so a second Register (a
// test, or two compositions in one process) must not add a second one.
func registerReadOnlyHooks() {
	readOnlyHooksOnce.Do(func() {
		readonly.OnEnter(closeAllRealtimeConns)
	})
}

// trackConn records conn as client's connection until the returned func runs.
func trackConn(client *Client, conn *websocket.Conn) func() {
	openConnsMu.Lock()
	openConns[client] = conn
	openConnsMu.Unlock()
	return func() {
		openConnsMu.Lock()
		delete(openConns, client)
		openConnsMu.Unlock()
	}
}

// closeAllRealtimeConns closes every open room connection with a going-away
// code. It runs from readonly.OnEnter in its own goroutine, which can start
// after a later Leave, so it re-checks the mode and closes nothing once
// writes are accepted again. Closing a connection twice is harmless, so
// overlapping runs are safe.
func closeAllRealtimeConns() {
	if !readonly.Active() {
		return
	}
	openConnsMu.Lock()
	conns := make([]*websocket.Conn, 0, len(openConns))
	for _, conn := range openConns {
		conns = append(conns, conn)
	}
	openConnsMu.Unlock()
	if len(conns) == 0 {
		return
	}
	log.Info("read-only mode: closing realtime connections; clients reconnect when writes resume",
		"count", len(conns))
	for _, conn := range conns {
		if !readonly.Active() {
			return
		}
		go closeGoingAway(conn)
	}
}

// closeClientForReadOnly closes client's connection, if it has one. Clients
// built directly by tests have none.
func closeClientForReadOnly(client *Client) {
	openConnsMu.Lock()
	conn := openConns[client]
	openConnsMu.Unlock()
	if conn != nil {
		go closeGoingAway(conn)
	}
}

// closeGoingAway runs in its own goroutine because Close waits for the
// peer's close frame, and a caller may be the connection's own read loop.
func closeGoingAway(conn *websocket.Conn) {
	_ = conn.Close(websocket.StatusGoingAway, readOnlyCloseReason)
}
