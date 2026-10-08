package realtime

import (
	"context"
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
)

func TestTakeCredentialsReadsSubprotocols(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/realtime/boards/p1", nil)
	r.Header.Set("Sec-WebSocket-Protocol", Protocol+", "+AuthProtocolPrefix+"jwt.a.b")

	creds := takeCredentials(r)
	if creds.authToken != "jwt.a.b" || creds.shareSession != "" {
		t.Fatalf("got %+v", creds)
	}

	r.Header.Set("Sec-WebSocket-Protocol", Protocol+", "+ShareProtocolPrefix+"share.c.d")
	if got := takeCredentials(r).shareSession; got != "share.c.d" {
		t.Fatalf("share session = %q", got)
	}
}

func TestTakeCredentialsStripsLegacyQueryFromTheLoggedURL(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/realtime/boards/p1?token=secret&share_session=s2&keep=1", nil)

	creds := takeCredentials(r)
	if creds.authToken != "secret" || creds.shareSession != "s2" {
		t.Fatalf("got %+v", creds)
	}
	if got := r.URL.RequestURI(); got != "/api/realtime/boards/p1?keep=1" {
		t.Fatalf("request URI still carries a credential: %q", got)
	}
}

func TestTakeCredentialsPrefersTheSubprotocol(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/realtime/boards/p1?token=old", nil)
	r.Header.Set("Sec-WebSocket-Protocol", Protocol+", "+AuthProtocolPrefix+"new")

	if got := takeCredentials(r).authToken; got != "new" {
		t.Fatalf("auth token = %q", got)
	}
	if got := r.URL.RequestURI(); got != "/api/realtime/boards/p1" {
		t.Fatalf("request URI = %q", got)
	}
}

// The whole handshake through PocketBase's router: the auth token offered as
// a subprotocol admits the user, and the server answers with Protocol, which
// a browser requires before it opens the socket.
func TestConnectAuthenticatesBySubprotocol(t *testing.T) {
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

	user, err := app.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/realtime/test/room"

	dial := func(protocols ...string) (*websocket.Conn, *http.Response, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: protocols})
	}

	conn, _, err := dial(Protocol, AuthProtocolPrefix+token)
	if err != nil {
		t.Fatalf("dial with the auth subprotocol: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	if conn.Subprotocol() != Protocol {
		t.Fatalf("server chose subprotocol %q; want %q", conn.Subprotocol(), Protocol)
	}

	_, res, err := dial(Protocol)
	if err == nil {
		t.Fatal("a connection without a credential was admitted")
	}
	if res == nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response without a credential = %v; want 401", res)
	}
}
