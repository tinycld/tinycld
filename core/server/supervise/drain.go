package supervise

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// unreadGrace is how long a connection that has sent no request yet holds a
// drain. It is the 5 seconds after which http.Server.Shutdown treats such a
// connection as idle and closes it; waiting longer would gain nothing.
const unreadGrace = 5 * time.Second

// drainPoll is how often Drain checks whether the connections it waits for
// have finished: short next to a request, so a drain ends soon after its
// last one.
const drainPoll = 10 * time.Millisecond

// Drainer stops an http.Server without dropping a request it accepted.
//
// http.Server.Shutdown closes a connection without a response when it reads
// the connection's request after the shutdown began, and that includes the
// first request of a connection accepted just before. Under a supervisor the
// next server is already accepting on the same socket, so a swap under load
// would fail a request every time. Drain instead stops accepting itself,
// turns keep-alives off so each HTTP/1 connection closes after its current
// request, waits for those connections to finish, and only then calls
// Shutdown. HTTP/2 connections are left to Shutdown, which ends them with a
// GOAWAY that lets their streams finish.
type Drainer struct {
	srv *http.Server

	mu    sync.Mutex
	conns map[net.Conn]*drainConn
	ls    []*drainListener
}

type drainConn struct {
	accepted time.Time
	state    http.ConnState
	h2       bool
}

// NewDrainer wraps srv.ConnState (keeping any hook already set) so it can
// follow each connection. Call it before srv serves.
func NewDrainer(srv *http.Server) *Drainer {
	d := &Drainer{srv: srv, conns: map[net.Conn]*drainConn{}}
	prev := srv.ConnState
	srv.ConnState = func(c net.Conn, st http.ConnState) {
		d.track(c, st)
		if prev != nil {
			prev(c, st)
		}
	}
	return d
}

// Listener wraps l so Drain can stop accepting on it. Serve on the returned
// listener.
func (d *Drainer) Listener(l net.Listener) net.Listener {
	dl := &drainListener{Listener: l, d: d, stopped: make(chan struct{}), closed: make(chan struct{})}
	d.mu.Lock()
	d.ls = append(d.ls, dl)
	d.mu.Unlock()
	return dl
}

// StopAccepting stops accepting on every listener and turns keep-alives off,
// so each HTTP/1 connection closes after its current request. Drain does
// this first. Call it on its own to stop new requests before work that must
// not race them, such as ending long-lived responses whose clients will
// reconnect at once: with keep-alives on, a client could send its reconnect
// on the same connection and land on this server again.
func (d *Drainer) StopAccepting() {
	d.mu.Lock()
	ls := d.ls
	d.mu.Unlock()
	for _, l := range ls {
		l.stop()
	}
	d.srv.SetKeepAlivesEnabled(false)
}

// Drain stops accepting, waits (within ctx) for every HTTP/1 connection
// accepted so far to finish its request, then shuts the server down.
func (d *Drainer) Drain(ctx context.Context) error {
	d.StopAccepting()

	tick := time.NewTicker(drainPoll)
	defer tick.Stop()
	for d.pending() {
		select {
		case <-ctx.Done():
			return d.srv.Shutdown(ctx)
		case <-tick.C:
		}
	}
	return d.srv.Shutdown(ctx)
}

// pending reports whether a connection still has a request Shutdown could
// drop: one that is being served, one accepted recently that has not sent
// its request yet, or one a listener accepted but has not handed over yet.
func (d *Drainer) pending() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, l := range d.ls {
		if l.handingOff.Load() > 0 {
			return true
		}
	}
	for _, c := range d.conns {
		if c.h2 {
			continue
		}
		switch c.state {
		case http.StateActive:
			return true
		case http.StateNew:
			if time.Since(c.accepted) < unreadGrace {
				return true
			}
		}
	}
	return false
}

func (d *Drainer) accepted(c net.Conn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conns[c] = &drainConn{accepted: time.Now(), state: http.StateNew}
}

func (d *Drainer) track(c net.Conn, st http.ConnState) {
	// A TLS server hands the hook the *tls.Conn; the listener saw the raw
	// connection under it.
	raw := c
	if nc, ok := c.(interface{ NetConn() net.Conn }); ok {
		raw = nc.NetConn()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	dc, ok := d.conns[raw]
	if !ok {
		return // not accepted through a Drainer listener
	}
	if st == http.StateClosed || st == http.StateHijacked {
		delete(d.conns, raw)
		return
	}
	dc.state = st
	if tc, ok := c.(*tls.Conn); ok && st != http.StateNew && tc.ConnectionState().NegotiatedProtocol == "h2" {
		dc.h2 = true
	}
}

// drainListener records each connection it accepts. Once stopped it closes
// the listener it wraps but blocks further Accepts until Close: Serve
// returning before Shutdown would end the server's run before the drain
// finished.
type drainListener struct {
	net.Listener
	d *Drainer

	stopOnce, closeOnce sync.Once
	stopped, closed     chan struct{}
	stopErr             error

	// handingOff counts Accepts between the listener under this one and
	// the Drainer recording the connection. A connection in that window is
	// in no map yet, so a drain that looked only at the map would shut
	// down and drop its request. Counting calls, rather than waiting for an
	// Accept to fail after the stop, keeps a drain from waiting its whole
	// budget on a listener no Serve is accepting on.
	handingOff atomic.Int32
}

func (l *drainListener) Accept() (net.Conn, error) {
	l.handingOff.Add(1)
	c, err := l.Listener.Accept()
	if err != nil {
		l.handingOff.Add(-1)
		select {
		case <-l.stopped:
			<-l.closed
			return nil, net.ErrClosed
		default:
			return nil, err
		}
	}
	l.d.accepted(c)
	l.handingOff.Add(-1)
	return c, nil
}

func (l *drainListener) stop() {
	l.stopOnce.Do(func() {
		close(l.stopped)
		l.stopErr = l.Listener.Close()
	})
}

func (l *drainListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	l.stop()
	return l.stopErr
}
