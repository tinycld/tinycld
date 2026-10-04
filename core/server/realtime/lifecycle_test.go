package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/drainhooks"
	"tinycld.org/core/readonly"
)

// lifecycleFixture registers a checkpointing kind on a broker and binds the
// lifecycle hooks on a test app.
type lifecycleFixture struct {
	app    *tests.TestApp
	broker *Broker
	store  *MemoryCheckpointStore
	rt     *stubRuntime
	opts   dialOpts
}

func newLifecycleFixture(t *testing.T, broker *Broker) *lifecycleFixture {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	drainhooks.ResetForTest()
	resetLifecycleForTest()
	t.Cleanup(func() {
		drainhooks.ResetForTest()
		resetLifecycleForTest()
	})
	registerLifecycle(app)
	// The process-wide hooks act on the shared broker; point it at this one.
	swapSharedBrokerForTest(t, broker)

	f := &lifecycleFixture{app: app, broker: broker, store: NewMemoryCheckpointStore(), rt: newStubRuntime()}
	f.opts = startTestServerWithOpts(t, broker, RoomKindOptions{
		RuntimeProvider: f.rt,
		Checkpoints:     f.store,
		Fingerprint:     func(string) (string, error) { return "fp", nil },
	})
	return f
}

func expectClose(t *testing.T, conn *websocket.Conn, code websocket.StatusCode) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	var ce websocket.CloseError
	if !errors.As(err, &ce) {
		t.Fatalf("expected a close, got %v", err)
	}
	if ce.Code != code {
		t.Fatalf("close code = %d (%q); want %d", ce.Code, ce.Reason, code)
	}
}

func TestDrainBeginSuspendsThenClosesGoingAway(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)
	f := newLifecycleFixture(t, broker)
	a := dialClient(t, f.opts, "room-d", "alice")
	room := waitForRoomMembers(t, broker, "test", "room-d", 1, time.Second)

	drainhooks.RunBegin()

	expectClose(t, a.conn, websocket.StatusGoingAway)
	cp, found, _ := f.store.Load("test", "room-d")
	if !found || cp.Epoch != room.DocEpoch() {
		t.Fatalf("checkpoint = %+v found=%v; want the room's state stored before the close", cp, found)
	}
}

func TestDrainBeginSkipsSuspendWhileReadOnly(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)
	f := newLifecycleFixture(t, broker)
	a := dialClient(t, f.opts, "room-ro", "alice")
	waitForRoomMembers(t, broker, "test", "room-ro", 1, time.Second)

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	// Enter stored the state; the drain must not write again under the pause.
	_ = f.store.Delete("test", "room-ro")
	drainhooks.RunBegin()

	expectClose(t, a.conn, websocket.StatusGoingAway)
	if _, found, _ := f.store.Load("test", "room-ro"); found {
		t.Fatal("the drain wrote a checkpoint while read-only")
	}
}

func TestConnectionRefusedWhileDraining(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)
	f := newLifecycleFixture(t, broker)
	drainhooks.RunBegin()

	hdr := http.Header{}
	hdr.Set("X-Test-User", "alice")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, f.opts.url+"late", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	expectClose(t, conn, websocket.StatusGoingAway)
	if broker.roomCount() != 0 {
		t.Fatal("a draining process admitted a client")
	}
}

func TestUpgradeAnswers503WhileDraining(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	drainhooks.ResetForTest()
	resetLifecycleForTest()
	t.Cleanup(func() {
		drainhooks.ResetForTest()
		resetLifecycleForTest()
	})
	resetRegistry()
	RegisterRoomKindWith("test", RoomKindOptions{Authorize: allowAllAuth})
	Register(app, Options{})
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	var mux http.Handler
	err = app.OnServe().Trigger(&core.ServeEvent{App: app, Router: router}, func(e *core.ServeEvent) error {
		mux, err = e.Router.BuildMux()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	get := func() *http.Response {
		res, err := srv.Client().Get(srv.URL + "/api/realtime/test/room")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	if res := get(); res.StatusCode == http.StatusServiceUnavailable {
		t.Fatal("refused before any drain")
	}
	drainhooks.RunBegin()
	res := get()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status while draining = %d; want 503", res.StatusCode)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatal("no Retry-After on the refusal")
	}
}

func TestTerminateSuspends(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)
	f := newLifecycleFixture(t, broker)
	_ = dialClient(t, f.opts, "room-t", "alice")
	room := waitForRoomMembers(t, broker, "test", "room-t", 1, time.Second)

	err := f.app.OnTerminate().Trigger(&core.TerminateEvent{App: f.app}, func(*core.TerminateEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	cp, found, _ := f.store.Load("test", "room-t")
	if !found || cp.Epoch != room.DocEpoch() {
		t.Fatalf("checkpoint after terminate = %+v found=%v", cp, found)
	}
}

func TestReadOnlyEnterSuspends(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)
	f := newLifecycleFixture(t, broker)
	_ = dialClient(t, f.opts, "room-e", "alice")
	room := waitForRoomMembers(t, broker, "test", "room-e", 1, time.Second)

	readonly.Enter()
	t.Cleanup(readonly.Leave)
	// Enter runs the hook synchronously, so the row exists on return.
	cp, found, _ := f.store.Load("test", "room-e")
	if !found || cp.Epoch != room.DocEpoch() {
		t.Fatalf("checkpoint after Enter = %+v found=%v", cp, found)
	}
	if broker.roomCount() != 1 {
		t.Fatal("the room was closed by the pause")
	}
}

// swapSharedBrokerForTest makes the process-wide hooks (read-only enter,
// terminate) act on broker for the test's duration.
func swapSharedBrokerForTest(t *testing.T, broker *Broker) {
	t.Helper()
	processBrokerMu.Lock()
	prev := processBroker
	processBroker = broker
	processBrokerMu.Unlock()
	t.Cleanup(func() {
		processBrokerMu.Lock()
		processBroker = prev
		processBrokerMu.Unlock()
	})
}

var _ = strings.TrimSpace
