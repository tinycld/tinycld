package coreserver

import (
	"errors"
	"net"
	"syscall"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// A write that fails with ENOBUFS is tried again this many times in a row
// without progress, with a wait that doubles from the first to the largest.
const (
	writeRetryAttempts  = 10
	writeRetryFirstWait = time.Millisecond
	writeRetryMaxWait   = 50 * time.Millisecond
)

// writeRetryListener gives each accepted connection a Write that tries again
// on ENOBUFS.
//
// macOS fails a TCP write with ENOBUFS when the system mbuf pool is empty.
// Linux answers EAGAIN there, which Go's poller waits on, so only macOS shows
// the error. The pool empties for a moment when many clients load large
// bundles over loopback at one time. net/http marks the connection dead on
// the first failed write, so nothing above the connection can recover: the
// response ends short and the browser reports ERR_CONTENT_LENGTH_MISMATCH.
// The retry must therefore be here, below net/http and below TLS.
type writeRetryListener struct {
	net.Listener
	sleep func(time.Duration)
}

// registerWriteRetry binds on every platform, so that the count of OnServe
// handlers does not depend on the platform; the handler does work on macOS
// only.
func registerWriteRetry(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := serveWithWriteRetry(e); err != nil {
			return err
		}
		return e.Next()
	})
}

func newWriteRetryListener(l net.Listener) *writeRetryListener {
	return &writeRetryListener{Listener: l, sleep: time.Sleep}
}

func (l *writeRetryListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &writeRetryConn{Conn: c, sleep: l.sleep}, nil
}

// writeRetryConn hides the *net.TCPConn under it, so net/http does not use
// sendfile on it. A sendfile can fail with ENOBUFS too, and could not be
// tried again from here.
type writeRetryConn struct {
	net.Conn
	sleep func(time.Duration)
}

func (c *writeRetryConn) Write(p []byte) (int, error) {
	written := 0
	failures := 0
	wait := writeRetryFirstWait
	for {
		n, err := c.Conn.Write(p[written:])
		written += n
		if err == nil || !errors.Is(err, syscall.ENOBUFS) {
			return written, err
		}
		if n > 0 {
			failures = 0
			wait = writeRetryFirstWait
		}
		failures++
		if failures > writeRetryAttempts {
			srvLog.Warn("a write found no buffer space after every retry", "remote", c.RemoteAddr().String(), "written", written, "size", len(p))
			return written, err
		}
		c.sleep(wait)
		wait = min(wait*2, writeRetryMaxWait)
	}
}

// CloseWrite keeps net/http's half-close before a full close, which the
// wrapper would otherwise hide.
func (c *writeRetryConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}
