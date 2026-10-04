package realtime

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"tinycld.org/core/drainhooks"
	"tinycld.org/core/readonly"
)

// Document edits arrive as websocket frames, not HTTP requests, so the
// server's lifecycle events reach the broker only through these hooks:
//
//   - read-only enter: Suspend, so the pause's backup or the next build's
//     migration sees every flushed file and the state of every document.
//     Sockets stay open; edits made during the pause live in the open
//     rooms and in the clients, which resend them to whichever process
//     serves them next.
//   - drain begin: Suspend (unless read-only already did), then close every
//     connection with a going-away code. Accepting has already stopped, so
//     a client's reconnect reaches the next process, whose hello carries
//     the same epoch and whose sync accepts the client's diff.
//   - terminate: Suspend for a plain stop with no supervisor.
//
// A drain's Suspend is bounded: drainhooks.RunBegin runs inside the HTTP
// drain budget, so the flushes get suspendBudget and no more.

const (
	suspendBudget      = 10 * time.Second
	readOnlySuspendMax = 15 * time.Second
	drainCloseReason   = "server restarting; reconnect"
	// realtimeSuspendPriority runs the terminate hook ahead of the drain's
	// shutdown (coreserver's drainShutdownPriority, -10000), so a plain stop
	// stores every document before PocketBase cuts request contexts.
	realtimeSuspendPriority = -10001
)

var (
	lifecycleOnce sync.Once
	// draining flips when a drain begins; an upgrade that still reaches this
	// process (on a connection accepted before accepting stopped) is turned
	// away so the client reconnects to the next process.
	draining atomic.Bool

	// openConns maps each live room connection's Client to its websocket,
	// so a drain can close them all.
	openConnsMu sync.Mutex
	openConns   = map[*Client]*websocket.Conn{}
)

// registerLifecycle binds the hooks above. The read-only hook registers
// once per process (OnEnter registrations are never removed); the drain
// hook replaces itself by name; the terminate hook is per app.
func registerLifecycle(app core.App) {
	lifecycleOnce.Do(func() {
		readonly.OnEnter(func() {
			ctx, cancel := context.WithTimeout(context.Background(), readOnlySuspendMax)
			defer cancel()
			Suspend(ctx, "read-only")
		})
	})
	drainhooks.OnBegin("realtime-rooms", beginDrain)
	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id:       "tinycldRealtimeSuspend",
		Priority: realtimeSuspendPriority,
		Func: func(e *core.TerminateEvent) error {
			suspendForExit("terminate")
			return e.Next()
		},
	})
}

func beginDrain() {
	draining.Store(true)
	suspendForExit("drain")
	closeAllConns()
}

// suspendForExit stores every document unless the server is read-only, in
// which case the pause's own Suspend already did and nothing may write.
func suspendForExit(reason string) {
	if readonly.Active() {
		log.Info("documents were stored when read-only began; nothing to write now", "reason", reason)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), suspendBudget)
	defer cancel()
	Suspend(ctx, reason)
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

func closeAllConns() {
	openConnsMu.Lock()
	conns := make([]*websocket.Conn, 0, len(openConns))
	for _, conn := range openConns {
		conns = append(conns, conn)
	}
	openConnsMu.Unlock()
	if len(conns) == 0 {
		return
	}
	log.Info("drain: closing realtime connections; clients reconnect to the next server", "count", len(conns))
	for _, conn := range conns {
		go closeGoingAway(conn)
	}
}

// closeGoingAway runs in its own goroutine because Close waits for the
// peer's close frame, and a caller may be the connection's own read loop.
func closeGoingAway(conn *websocket.Conn) {
	_ = conn.Close(websocket.StatusGoingAway, drainCloseReason)
}

// refuseWhileDraining answers an upgrade that reached a draining process.
// Mirrors the read-only refusal shape so the client's backoff treats both
// the same way.
func refuseWhileDraining(re *core.RequestEvent) error {
	re.Response.Header().Set("Retry-After", strconv.Itoa(readonly.RetryAfterSeconds))
	return re.JSON(http.StatusServiceUnavailable, map[string]string{
		"code":    "retry_later",
		"message": "The server is restarting; reconnect.",
	})
}

// resetLifecycleForTest clears the draining flag. Tests only.
func resetLifecycleForTest() {
	draining.Store(false)
}
