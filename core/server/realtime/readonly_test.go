package realtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/readonly"
)

// startConnectServer serves the production upgrade handler, handleConnect,
// for one room kind. The test stands in for PocketBase's auth middleware by
// setting re.Auth to a fixed user, so the upgrade reaches the read-only
// check and the websocket accept exactly as a real request would.
func startConnectServer(t *testing.T, kind string) string {
	t.Helper()
	RegisterRoomKindWith(kind, RoomKindOptions{Authorize: allowAllAuth})
	t.Cleanup(func() { unregisterRoomKindForTest(kind) })

	user := core.NewRecord(core.NewAuthCollection("users"))
	user.Id = "u1"
	broker := NewBroker()
	opts := Options{
		IdleTimeout:   defaultIdleTimeout,
		MaxFrameBytes: defaultMaxFrameBytes,
		PingInterval:  defaultPingInterval,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/realtime/{roomKind}/{roomID}", func(w http.ResponseWriter, r *http.Request) {
		re := &core.RequestEvent{Auth: user}
		re.Response = w
		re.Request = r
		if err := handleConnect(broker, opts, re); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/realtime/" + kind + "/room-1"
}

// A websocket upgrade is a GET, so the read-only middleware lets it through;
// the upgrade handler must refuse it itself, or a client opens a connection
// whose frames write journal rows during the pause.
func TestUpgradeRefusedWhileReadOnlyAndAcceptedAfterLeave(t *testing.T) {
	url := startConnectServer(t, "test-readonly-upgrade")
	t.Cleanup(readonly.Leave)

	readonly.Enter()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, res, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("upgrade during read-only mode succeeded; want it refused")
	}
	if res == nil {
		t.Fatalf("upgrade during read-only mode: no HTTP response: %v", err)
	}
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("upgrade during read-only mode: status %d, want 503", res.StatusCode)
	}
	if got := res.Header.Get("Retry-After"); got != "2" {
		t.Fatalf("upgrade during read-only mode: Retry-After %q, want 2", got)
	}

	readonly.Leave()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("upgrade after Leave: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read assign frame after Leave: %v", err)
	}
	if len(data) < frameOverhead || MessageType(data[clientIDLen]) != MsgAssignID {
		t.Fatal("first frame after Leave is not MsgAssignID")
	}
}

// A connection opened before the pause would otherwise keep writing journal
// rows: its frames are not HTTP requests, so nothing else can refuse them.
func TestOpenConnectionClosedWhenReadOnlyStarts(t *testing.T) {
	registerReadOnlyHooks()
	opts := startTestServerWithOpts(t, NewBroker(), RoomKindOptions{})
	c := dialClient(t, opts, "room-1", "u1")
	t.Cleanup(readonly.Leave)

	readonly.Enter()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := c.conn.Read(ctx)
	if err == nil {
		t.Fatal("read a frame after Enter; want the connection closed")
	}
	if got := websocket.CloseStatus(err); got != websocket.StatusGoingAway {
		t.Fatalf("close status %v (err %v), want StatusGoingAway", got, err)
	}
}

// OnEnter fns run in their own goroutine and can start after a Leave, so a
// late run must not close connections opened once writes are accepted again.
func TestCloseAllLeavesConnectionsOpenWhenNotReadOnly(t *testing.T) {
	opts := startTestServerWithOpts(t, NewBroker(), RoomKindOptions{})
	c1 := dialClient(t, opts, "room-1", "u1")
	c2 := dialClient(t, opts, "room-1", "u2")

	closeAllRealtimeConns()

	writeFrame(t, c1, MsgAwarenessUpdate, []byte{0x01})
	if typ, _ := readFrame(t, c2, 2*time.Second); typ != MsgAwarenessUpdate {
		t.Fatalf("peer received %v, want the awareness update", typ)
	}
}

// A frame that slips in between Enter and the connection close must not be
// journaled: the journal row is a database write during the pause.
func TestDocUpdateNotJournaledWhileReadOnly(t *testing.T) {
	j := &recordingJournal{}
	kind := "test-kind-readonly-route"
	RegisterRoomKindWith(kind, RoomKindOptions{
		Authorize:       allowAllAuth,
		RuntimeProvider: stubDocRuntime{},
		Journal:         j,
	})
	t.Cleanup(func() { unregisterRoomKindForTest(kind) })
	t.Cleanup(readonly.Leave)

	b := NewBroker()
	c1 := &Client{joinedAt: time.Now()}
	c2 := &Client{joinedAt: time.Now()}
	b.join(kind, "room-1", c1)
	b.join(kind, "room-1", c2)
	room := b.lookupRoomForTest(kind, "room-1")

	frame := make([]byte, frameOverhead+1)
	frame[clientIDLen] = byte(MsgDocUpdate)
	frame[frameOverhead] = 0xAA

	readonly.Enter()
	room.route(c1, frame)

	j.mu.Lock()
	appends := len(j.appends)
	j.mu.Unlock()
	if appends != 0 {
		t.Fatalf("appends during read-only = %d, want 0", appends)
	}
	select {
	case <-c2.send:
		t.Fatal("peer received a doc update the server did not journal")
	default:
	}

	readonly.Leave()
	room.route(c1, frame)
	j.mu.Lock()
	appends = len(j.appends)
	j.mu.Unlock()
	if appends != 1 {
		t.Fatalf("appends after Leave = %d, want 1", appends)
	}
}
