package apis

// Fork-only: the test lives in package apis (not apis_test) because it
// exercises the unexported serveHTTPRedirect. It builds the app with
// core.NewBaseApp instead of tests.NewTestApp: the tests package imports
// apis, so importing it here would be a cycle.

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// getBody polls addr until it responds (the redirect server starts on its
// own goroutine) and returns the response body, or fails the test after 5s.
//
// Each attempt gets a fraction of the 5s, so one that hangs (a connection
// made before the server serves, then lost) leaves time for the next; with
// the whole 5s per attempt, the first hung one would fail the test.
func getBody(t *testing.T, addr string) string {
	t.Helper()

	client := &http.Client{Timeout: time.Second}
	var res *http.Response
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if res, err = client.Get("http://" + addr + "/"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return string(body)
}

func handlerReturning(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})
}

func TestServeHTTPRedirectUsesTheInjectedListener(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	SetRedirectListener(app, l)
	go serveHTTPRedirect(app, "203.0.113.1:80", handlerReturning("redirect"))

	if body := getBody(t, l.Addr().String()); body != "redirect" {
		t.Fatalf("body = %q", body)
	}
}

// A second serveHTTPRedirect call on the same app (e.g. Serve runs again
// after a restart) must shut the previous server down rather than leak it:
// the first listener should stop answering once the second call's server
// has taken over the store's "current server" slot.
func TestServeHTTPRedirectSecondCallStopsThePreviousServer(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{})
	l1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	SetRedirectListener(app, l1)
	go serveHTTPRedirect(app, "203.0.113.1:80", handlerReturning("first"))

	if body := getBody(t, l1.Addr().String()); body != "first" {
		t.Fatalf("first body = %q", body)
	}

	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	SetRedirectListener(app, l2)
	go serveHTTPRedirect(app, "203.0.113.2:80", handlerReturning("second"))

	if body := getBody(t, l2.Addr().String()); body != "second" {
		t.Fatalf("second body = %q", body)
	}

	// The first server should now be shut down: give it a moment to stop,
	// then confirm l1 no longer accepts connections.
	deadline := time.Now().Add(5 * time.Second)
	client := &http.Client{Timeout: 200 * time.Millisecond}
	for time.Now().Before(deadline) {
		if _, err := client.Get("http://" + l1.Addr().String() + "/"); err != nil {
			return // first server is down, as expected
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("first redirect server is still serving after a second serveHTTPRedirect call")
}

// SetRedirectListener called after serveHTTPRedirect already read the store
// (i.e. too late to take effect) must be visible, not silent.
func TestSetRedirectListenerWarnsWhenCalledTooLate(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{})

	var buf bytes.Buffer
	prevDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prevDefault)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	SetRedirectListener(app, l)
	go serveHTTPRedirect(app, "203.0.113.1:80", handlerReturning("redirect"))
	getBody(t, l.Addr().String()) // wait for serveHTTPRedirect to have read the store

	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	SetRedirectListener(app, l2) // too late: serveHTTPRedirect already read the store

	if !bytes.Contains(buf.Bytes(), []byte("SetRedirectListener called after")) {
		t.Fatalf("expected a late-call warning, got log output: %s", buf.String())
	}
}

// countingListener counts the connections it hands out.
type countingListener struct {
	net.Listener
	accepted atomic.Int32
}

func (l *countingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.accepted.Add(1)
	}
	return c, err
}

// The redirect server must serve on the listener the hook returns, so a
// caller can follow the server's connections (to drain it) from the start.
func TestServeHTTPRedirectServesOnTheHookListener(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	SetRedirectListener(app, l)

	type hooked struct {
		srv *http.Server
		l   *countingListener
	}
	calls := make(chan hooked, 1)
	SetRedirectServerHook(app, func(srv *http.Server, in net.Listener) net.Listener {
		if in != l {
			t.Errorf("the hook got listener %v, want the injected %v", in.Addr(), l.Addr())
		}
		h := hooked{srv: srv, l: &countingListener{Listener: in}}
		calls <- h
		return h.l
	})
	go serveHTTPRedirect(app, "203.0.113.1:80", handlerReturning("redirect"))

	if body := getBody(t, l.Addr().String()); body != "redirect" {
		t.Fatalf("body = %q", body)
	}
	var h hooked
	select {
	case h = <-calls:
	default:
		t.Fatal("the redirect server never called the hook")
	}
	if h.l.accepted.Load() == 0 {
		t.Fatal("the redirect server did not serve on the hook's listener")
	}
	if h.srv != app.Store().Get(redirectServerStoreKey) {
		t.Fatal("the hook did not get the redirect server")
	}
}
