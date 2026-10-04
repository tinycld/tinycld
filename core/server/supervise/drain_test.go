//go:build unix

package supervise

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func (d *Drainer) tracked() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.conns)
}

// drainServer serves h on a loopback listener through a Drainer.
func drainServer(t *testing.T, h http.Handler) (*Drainer, string, chan error) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	d := NewDrainer(srv)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(d.Listener(l)) }()
	t.Cleanup(func() { srv.Close() })
	return d, l.Addr().String(), served
}

func startDrain(d *Drainer, budget time.Duration) chan error {
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		defer cancel()
		done <- d.Drain(ctx)
	}()
	return done
}

func waitRefused(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("the server still accepts after the drain began")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
}

// A connection accepted before the drain whose request arrives after it
// began must be answered. http.Server.Shutdown alone closes it with no
// response; under a supervisor that is a failed request on every swap.
func TestDrainAnswersAConnectionAcceptedBeforeIt(t *testing.T) {
	d, addr, _ := drainServer(t, okHandler())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waitFor(t, 5*time.Second, "the connection to be accepted", func() bool { return d.tracked() == 1 })

	drained := startDrain(d, 10*time.Second)
	waitRefused(t, addr)
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("the request on a connection accepted before the drain got no response: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" || !resp.Close {
		t.Fatalf("response = %q close=%v, want ok and Connection: close", body, resp.Close)
	}
	if err := <-drained; err != nil {
		t.Fatalf("Drain = %v", err)
	}
}

func TestDrainFinishesInFlightRequests(t *testing.T) {
	inFlight, release := make(chan struct{}), make(chan struct{})
	d, addr, served := drainServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight)
		<-release
		io.WriteString(w, "slow")
	}))
	slow := make(chan error, 1)
	go func() {
		body, err := get(addr)
		if err == nil && body != "slow" {
			err = errors.New("body " + body)
		}
		slow <- err
	}()
	<-inFlight

	drained := startDrain(d, 10*time.Second)
	waitRefused(t, addr)
	select {
	case err := <-drained:
		t.Fatalf("Drain returned %v with a request in flight", err)
	default:
	}
	close(release)
	if err := <-slow; err != nil {
		t.Fatalf("the in-flight request was cut: %v", err)
	}
	if err := <-drained; err != nil {
		t.Fatalf("Drain = %v", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve = %v, want ErrServerClosed", err)
	}
}

// A client that connects and never sends a request (a browser's
// speculative connection) must not hold the drain for its whole budget.
func TestDrainDoesNotWaitForASilentConnection(t *testing.T) {
	d, addr, _ := drainServer(t, okHandler())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waitFor(t, 5*time.Second, "the connection to be accepted", func() bool { return d.tracked() == 1 })

	start := time.Now()
	if err := <-startDrain(d, 30*time.Second); err != nil {
		t.Fatalf("Drain = %v", err)
	}
	if took := time.Since(start); took > unreadGrace+3*time.Second {
		t.Fatalf("Drain waited %s for a connection that sent nothing", took)
	}
}

func TestDrainIdleKeepAliveConnectionIsClosed(t *testing.T) {
	d, addr, _ := drainServer(t, okHandler())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	io.ReadAll(resp.Body)

	if err := <-startDrain(d, 10*time.Second); err != nil {
		t.Fatalf("Drain = %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := r.ReadByte(); err == nil || strings.Contains(err.Error(), "timeout") {
		t.Fatalf("idle keep-alive connection still open after the drain: %v", err)
	}
}

// slowHandOff is a listener whose Accept holds each connection it accepted
// until release closes: the window between the kernel's accept and the
// Drainer seeing the connection.
type slowHandOff struct {
	net.Listener
	accepted chan net.Conn
	release  chan struct{}
	closed   chan struct{}
	once     sync.Once
}

func (l *slowHandOff) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.accepted <- c
	<-l.release
	return c, nil
}

func (l *slowHandOff) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

// A connection the listener accepted just before the drain stopped it, but
// had not yet handed to the server, must still be served. A drain that
// checks for pending connections before that hand-off sees none, shuts
// down, and the process exits with the request unread.
func TestDrainServesAConnectionInTheAcceptWindow(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &slowHandOff{Listener: inner, accepted: make(chan net.Conn, 1), release: make(chan struct{}), closed: make(chan struct{})}
	srv := &http.Server{Handler: okHandler()}
	d := NewDrainer(srv)
	go srv.Serve(d.Listener(l))
	t.Cleanup(func() { srv.Close() })

	conn, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
	var serverSide net.Conn
	select {
	case serverSide = <-l.accepted:
	case <-time.After(5 * time.Second):
		t.Fatal("the listener did not accept the connection")
	}

	// Once Drain returns the process exits: its connections go with it.
	drained := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := d.Drain(ctx)
		serverSide.Close()
		drained <- err
	}()
	select {
	case <-l.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the drain did not stop the listener")
	}
	close(l.release)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("the connection accepted as the drain began got no response: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("response = %q, want ok", body)
	}
	if err := <-drained; err != nil {
		t.Fatalf("Drain = %v", err)
	}
}
