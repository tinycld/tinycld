package coreserver

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

var errNoBufferSpace = &net.OpError{Op: "write", Net: "tcp", Err: os.NewSyscallError("write", syscall.ENOBUFS)}

// writeStep is one answer of the fake connection: it takes n bytes, then
// returns err.
type writeStep struct {
	n   int
	err error
}

// scriptedConn answers each Write from its steps in order; after the last
// step it takes everything.
type scriptedConn struct {
	net.Conn
	steps []writeStep
	calls int
	got   bytes.Buffer
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	c.calls++
	if len(c.steps) == 0 {
		return c.got.Write(p)
	}
	step := c.steps[0]
	c.steps = c.steps[1:]
	c.got.Write(p[:step.n])
	return step.n, step.err
}

func (c *scriptedConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
}

func retryConnOver(c net.Conn) (*writeRetryConn, *[]time.Duration) {
	waits := &[]time.Duration{}
	return &writeRetryConn{Conn: c, sleep: func(d time.Duration) { *waits = append(*waits, d) }}, waits
}

func TestWriteRetrySendsTheRestAfterNoBufferSpace(t *testing.T) {
	under := &scriptedConn{steps: []writeStep{
		{n: 4, err: errNoBufferSpace},
		{n: 0, err: errNoBufferSpace},
		{n: 3, err: errNoBufferSpace},
	}}
	c, waits := retryConnOver(under)

	n, err := c.Write([]byte("0123456789"))
	if err != nil || n != 10 {
		t.Fatalf("Write = %d, %v; want 10, nil", n, err)
	}
	if got := under.got.String(); got != "0123456789" {
		t.Fatalf("the connection got %q, want each byte once and in order", got)
	}
	if len(*waits) != 3 {
		t.Fatalf("waited %d times, want one wait for each failed write", len(*waits))
	}
}

func TestWriteRetryReturnsOtherErrorsAtOnce(t *testing.T) {
	under := &scriptedConn{steps: []writeStep{{n: 2, err: syscall.EPIPE}}}
	c, waits := retryConnOver(under)

	n, err := c.Write([]byte("0123456789"))
	if n != 2 || !errors.Is(err, syscall.EPIPE) {
		t.Fatalf("Write = %d, %v; want 2, EPIPE", n, err)
	}
	if under.calls != 1 || len(*waits) != 0 {
		t.Fatalf("calls = %d, waits = %d; want one write and no wait", under.calls, len(*waits))
	}
}

func TestWriteRetryGivesUpWhenNoWriteMakesProgress(t *testing.T) {
	steps := make([]writeStep, writeRetryAttempts+5)
	for i := range steps {
		steps[i] = writeStep{err: errNoBufferSpace}
	}
	under := &scriptedConn{steps: steps}
	c, waits := retryConnOver(under)

	n, err := c.Write([]byte("0123456789"))
	if n != 0 || !errors.Is(err, syscall.ENOBUFS) {
		t.Fatalf("Write = %d, %v; want 0, ENOBUFS", n, err)
	}
	if under.calls != writeRetryAttempts+1 {
		t.Fatalf("calls = %d, want the first write and %d retries", under.calls, writeRetryAttempts)
	}
	for _, d := range *waits {
		if d > writeRetryMaxWait {
			t.Fatalf("waited %v, more than the largest wait %v", d, writeRetryMaxWait)
		}
	}
}

func TestWriteRetryListenerWrapsAcceptedConnections(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := newWriteRetryListener(raw)
	defer l.Close()

	client, err := net.Dial("tcp", raw.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	c, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, ok := c.(*writeRetryConn); !ok {
		t.Fatalf("Accept returned %T, want a *writeRetryConn", c)
	}

	// The half-close must reach the TCP connection: the client then reads
	// the data and a clean end.
	if _, err := c.Write([]byte("done")); err != nil {
		t.Fatal(err)
	}
	if err := c.(*writeRetryConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(client)
	if err != nil || string(got) != "done" {
		t.Fatalf("client read %q, %v; want \"done\" and a clean end", got, err)
	}
}
