//go:build unix

package coreserver

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/drainhooks"
	"tinycld.org/core/supervise"
)

// acceptSignal is an inherited listener that reports the client address of
// each connection it accepts, so a test knows the server holds a connection
// before it starts a drain.
type acceptSignal struct {
	net.Listener
	accepted chan string
}

func newAcceptSignal(t *testing.T) *acceptSignal {
	return &acceptSignal{Listener: loopbackListener(t), accepted: make(chan string, 64)}
}

// Accept never blocks on the report: a full channel only loses reports no
// test waits for, while a blocked Accept would stop the server accepting.
func (l *acceptSignal) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		select {
		case l.accepted <- c.RemoteAddr().String():
		default:
		}
	}
	return c, err
}

// dialAccepted connects to l and returns once l has accepted the connection.
func dialAccepted(t *testing.T, l *acceptSignal) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	deadline := time.After(5 * time.Second)
	for {
		select {
		case addr := <-l.accepted:
			if addr == c.LocalAddr().String() {
				return c
			}
		case <-deadline:
			t.Fatal("the server did not accept the connection")
		}
	}
}

// bootstrappedApp is a real PocketBase app, so apis.Serve can run on it.
func bootstrappedApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	return app
}

// terminateOnDrain makes the drain's self-terminate trigger OnTerminate, as
// PocketBase does on the SIGTERM the real one raises. The returned channel
// closes once every terminate handler has run.
func terminateOnDrain(t *testing.T, app core.App) <-chan struct{} {
	t.Helper()
	terminated := make(chan struct{})
	stubSelfTerminate(t, func() error {
		go func() {
			app.OnTerminate().Trigger(&core.TerminateEvent{App: app}, func(*core.TerminateEvent) error { return nil })
			close(terminated)
		}()
		return nil
	})
	return terminated
}

// serveApp runs apis.Serve on app, as the serve command does, without the
// installer (it would open a browser on an app with no superuser).
func serveApp(t *testing.T, app core.App, cfg apis.ServeConfig) <-chan error {
	t.Helper()
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.InstallerFunc = nil
		return e.Next()
	})
	served := make(chan error, 1)
	go func() { served <- apis.Serve(app, cfg) }()
	return served
}

// holdDrainBegin registers a drain-begin handler, after the ones already
// registered, that blocks until the returned release is called. The test
// then sees the state the drain has right after its begin handlers ran.
func holdDrainBegin(t *testing.T) (begun <-chan struct{}, release func()) {
	t.Helper()
	b, hold := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(hold) }) }
	t.Cleanup(release)
	drainhooks.OnBegin("acme-hold", func() {
		close(b)
		<-hold
	})
	return b, release
}

func waitClosed(t *testing.T, ch <-chan struct{}, limit time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(limit):
		t.Fatalf("timed out after %s waiting for %s", limit, what)
	}
}

func waitServed(t *testing.T, served <-chan error) {
	t.Helper()
	select {
	case err := <-served:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the drain")
	}
}

// waitRefused waits for a connect to addr to be refused. The test holds the
// only copy of the listener, so a server that stopped accepting refuses.
func waitRefused(t *testing.T, addr, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if errors.Is(err, syscall.ECONNREFUSED) {
			return
		}
		if c != nil {
			c.Close()
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still accepts after the drain began", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// openRealtimeStream opens a realtime stream and reads up to its PB_CONNECT
// event. The returned channel gets the stream's end.
func openRealtimeStream(t *testing.T, addr string) <-chan error {
	t.Helper()
	// The timeout bounds the whole stream, so a stream that never ends
	// fails the test instead of hanging it.
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get("http://" + addr + "/api/realtime")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	r := bufio.NewReader(resp.Body)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("the realtime stream ended before PB_CONNECT: %v", err)
		}
		if strings.TrimSpace(line) == "event:PB_CONNECT" {
			break
		}
	}
	ended := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, r)
		ended <- err
	}()
	return ended
}

// A realtime stream never goes idle, so a drain that waits for it waits
// its whole budget, and its client keeps listening to a server that will
// miss every event the next one sends. The stream must end when the drain
// begins, so the client reconnects at once, and a realtime connect that
// reaches this server after that must be turned away rather than become a
// stream that holds the drain again.
func TestSupervisedDrainEndsRealtimeStreamsAtOnce(t *testing.T) {
	l := newAcceptSignal(t)
	parent := supervisedFixtureWith(t, map[string]net.Listener{supervise.ListenerHTTP: l})
	addr := l.Addr().String()
	app := bootstrappedApp(t)
	terminated := terminateOnDrain(t, app)
	registerSupervised(app)
	begun, release := holdDrainBegin(t)
	served := serveApp(t, app, apis.ServeConfig{HttpAddr: addr})
	if got := recvLine(t, parent, bufio.NewReader(parent)); got != `{"type":"ready","version":1}` {
		t.Fatalf("control message = %q", got)
	}

	stream := openRealtimeStream(t, addr)
	late := dialAccepted(t, l)

	if err := supervise.Send(parent, supervise.Msg{Type: supervise.MsgDrain}); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, begun, 5*time.Second, "the drain to begin")
	select {
	case err := <-stream:
		if err != nil {
			t.Fatalf("the realtime stream ended with %v, want a clean end", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the realtime stream was still open after the drain began")
	}

	io.WriteString(late, "GET /api/realtime HTTP/1.1\r\nHost: test\r\n\r\n")
	late.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(late), nil)
	if err != nil {
		t.Fatalf("a realtime connect after the drain began got no response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !resp.Close {
		t.Fatalf("a realtime connect after the drain began got %d close=%v, want 503 and Connection: close", resp.StatusCode, resp.Close)
	}

	release()
	// Far less than the drain's budget, which is what a stream left open
	// would cost.
	waitClosed(t, terminated, supervise.ChildDrainTimeout/3, "the drain to finish")
	waitServed(t, served)
}

// The :80 redirect server must drain like the main server: stop accepting
// when the drain begins, and answer a request it already accepted. Shut
// down with the main server's terminate path instead, it keeps accepting
// for the whole drain and then gives its requests one second.
func TestSupervisedDrainFinishesAnInFlightRedirect(t *testing.T) {
	https := loopbackListener(t)
	redirect := newAcceptSignal(t)
	parent := supervisedFixtureWith(t, map[string]net.Listener{
		supervise.ListenerHTTPS:        https,
		supervise.ListenerHTTPRedirect: redirect,
	})
	app := bootstrappedApp(t)
	terminated := terminateOnDrain(t, app)
	registerSupervised(app)
	begun, release := holdDrainBegin(t)
	served := serveApp(t, app, apis.ServeConfig{
		HttpAddr:           redirect.Addr().String(),
		HttpsAddr:          https.Addr().String(),
		CertificateDomains: []string{"example.test"},
	})
	if got := recvLine(t, parent, bufio.NewReader(parent)); got != `{"type":"ready","version":1}` {
		t.Fatalf("control message = %q", got)
	}

	inFlight := dialAccepted(t, redirect)
	io.WriteString(inFlight, "GET /page HTTP/1.1\r\n")

	if err := supervise.Send(parent, supervise.Msg{Type: supervise.MsgDrain}); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, begun, 5*time.Second, "the drain to begin")
	waitRefused(t, redirect.Addr().String(), "the redirect server")
	release()
	select {
	case <-terminated:
		t.Fatal("the drain finished with a redirect request in flight")
	default:
	}

	io.WriteString(inFlight, "Host: example.test\r\n\r\n")
	inFlight.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(inFlight), nil)
	if err != nil {
		t.Fatalf("the redirect request in flight got no response: %v", err)
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode/100 != 3 || loc != "https://example.test/page" {
		t.Fatalf("redirect response = %d to %q", resp.StatusCode, loc)
	}
	waitClosed(t, terminated, 5*time.Second, "the drain to finish")
	waitServed(t, served)
}
