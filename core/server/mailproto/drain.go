package mailproto

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"

	"tinycld.org/core/shutdown"
)

// drainBound bounds how long a stop lets open sessions finish before it cuts
// them. A hung client must not hold up the app's terminate, and the terminate
// chain has one deadline in all, about 8 s after a SIGTERM (see
// tinycld.org/core/shutdown), which can cut this shorter; 5 s is also
// readonly's tailStopBound. All the
// servers a process runs drain at the same time (see bindStop), so the bound
// is spent once, not once per server. Under a supervisor the listeners closed
// when the drain began, so the sessions have had the whole HTTP drain to end
// and few are left by now.
//
// A var, not a const, so tests can shorten it.
var drainBound = 5 * time.Second

// endGrace is how long a connection whose session has ended its work may
// still read before it is closed. The server can still need bytes the client
// already sent: an IMAP APPEND reads the CRLF after its literal once the
// session's Append has returned, and answers only when it has it. Closing at
// once there would drop the reply to a stored message, and the client would
// store it again. It is a time, not a count of reads, so a client cannot keep
// the connection open with a stream of commands.
const endGrace = 500 * time.Millisecond

// terminatePriority runs the servers' stop before the terminate handlers
// bound at the default priority, the background runner's among them. A write
// a session finishes during the drain can then still start its follow-up
// jobs, which a stopped runner would drop.
const terminatePriority = -1

// callKind says what a session call does, which decides what a stop may do
// to its connection.
type callKind int

const (
	// callCut can be cut: it only reads, or it is still reading what the
	// client sends. Cutting its connection makes it fail at its next network
	// read or write, before it stores anything.
	callCut callKind = iota
	// callIdle is an IMAP IDLE: it waits for the client and ends when its
	// connection closes.
	callIdle
	// callWrite writes to the database and must run to the end, with its
	// connection open to carry the reply: a client that does not get the
	// reply sends the write again.
	callWrite
)

// admitPolicy says until when a session call is admitted.
type admitPolicy int

const (
	// admitUntilDrain refuses the call once the drain begins: it starts new
	// work (an IMAP command, an SMTP transaction).
	admitUntilDrain admitPolicy = iota
	// admitUntilClosed admits the call during the drain: it continues work
	// begun before it (RCPT and DATA after MAIL FROM).
	admitUntilClosed
	// admitAlways never refuses the call: it is part of the reply to a call
	// already admitted (IMAP's Poll before a tagged OK).
	admitAlways
)

// drain stops one protocol server and returns only when no session of it can
// use the database any more:
//
//  1. Refuse new connections and sessions, and close the listeners.
//  2. Refuse new work (IMAP: BYE and NO [UNAVAILABLE]; SMTP: 421). For IMAP,
//     end at once the sessions with no command in progress: an IMAP client
//     holds its connection open (most often in IDLE), so waiting for it to
//     leave would always cost the whole bound.
//  3. Let the work in progress finish, up to the bound.
//  4. Cut every connection whose session is not writing, refuse every later
//     call, and wait for the writes to finish and every session to close.
//     A write has no bound of its own: the database is still open, and a
//     write cut in the middle could be half done. It cannot outlast the
//     terminate's deadline, though: past it the process is killed, and
//     returning first lets the finalizer close the database before the
//     kill.
//
// The drain counts the open connections, sessions and calls in active. It
// closes connections at the network connection, under the TLS layer, and
// never through the protocol library's own Close: go-smtp's Conn.Close runs
// the session's Logout, which must run on the session's own goroutine, not
// beside a Data call in progress.
type drain struct {
	label string
	bound time.Duration
	// endIdle ends the sessions with no command in progress as soon as the
	// drain begins (IMAP). SMTP leaves them: a submission client quits when
	// its transaction is done, and a session between MAIL FROM and DATA has
	// no command in progress either.
	endIdle bool
	// closeServer closes the protocol server once nothing is left open.
	closeServer func()

	mu        sync.Mutex
	draining  bool
	closed    bool
	listeners []net.Listener
	conns     map[*trackedConn]struct{}
	// active counts the open connections, sessions and calls; quiet closes
	// when it reaches zero once the drain has begun.
	active      int
	quiet       chan struct{}
	quietClosed bool

	stopOnce sync.Once
	served   sync.WaitGroup
	byes     sync.WaitGroup
}

func newDrain(label string, endIdle bool) *drain {
	return &drain{
		label:   label,
		bound:   drainBound,
		endIdle: endIdle,
		conns:   map[*trackedConn]struct{}{},
	}
}

// serve runs serve on ln in its own goroutine. The stop closes ln and waits
// for serve to return.
func (d *drain) serve(ln net.Listener, label, addr string, serve func(net.Listener) error) {
	d.mu.Lock()
	d.listeners = append(d.listeners, ln)
	d.mu.Unlock()
	d.served.Add(1)
	go func() {
		defer d.served.Done()
		logServeExit(label, addr, serve(ln))
	}()
}

// bind binds the stop to app's terminate and returns it. The stop runs once:
// a later call waits for the first to finish. Its waits end at app's
// terminate deadline.
func (d *drain) bind(app core.App) func() {
	stop := func() {
		d.stopOnce.Do(func() { d.shutdown(shutdown.Context(app)) })
	}
	bindStop(app, stop)
	return stop
}

func (d *drain) track(c net.Conn) *trackedConn {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.draining {
		return nil
	}
	tc := &trackedConn{Conn: c, d: d}
	d.conns[tc] = struct{}{}
	d.active++
	return tc
}

func (d *drain) untrack(tc *trackedConn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.conns, tc)
	d.release()
}

// release ends one unit of active. Hold d.mu.
func (d *drain) release() {
	d.active--
	if d.active == 0 && d.quiet != nil && !d.quietClosed {
		close(d.quiet)
		d.quietClosed = true
	}
}

// beginSession admits a new session on tc, or refuses it once the drain has
// begun. bye, when not nil, ends the session from another goroutine with the
// protocol's own goodbye; the drain calls it for an IMAP session with no
// command in progress.
func (d *drain) beginSession(tc *trackedConn, bye func()) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.draining {
		return false
	}
	d.active++
	if tc != nil {
		tc.bye = bye
	}
	return true
}

func (d *drain) endSession() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.release()
}

// enter admits one session call on tc, or returns nil when the drain refuses
// it.
func (d *drain) enter(tc *trackedConn, kind callKind, policy admitPolicy) *call {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch policy {
	case admitUntilDrain:
		if d.draining {
			return nil
		}
	case admitUntilClosed:
		if d.closed {
			return nil
		}
	}
	d.active++
	c := &call{d: d, tc: tc}
	if tc != nil {
		tc.calls++
		switch kind {
		case callIdle:
			tc.idles++
		case callWrite:
			tc.writes++
			c.writing = true
		}
		c.idle = kind == callIdle
	}
	return c
}

// call is one session call the drain admitted.
type call struct {
	d       *drain
	tc      *trackedConn
	idle    bool
	writing bool
}

// write marks the call as writing from now on: it has read all the client
// sends for it and is storing it, so a stop must let it finish and keep its
// connection open for the reply.
func (c *call) write() {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	if c.writing || c.tc == nil {
		return
	}
	c.writing = true
	c.tc.writes++
}

func (c *call) leave() {
	c.d.mu.Lock()
	defer c.d.mu.Unlock()
	if c.tc != nil {
		c.tc.calls--
		if c.idle {
			c.tc.idles--
		}
		if c.writing {
			c.tc.writes--
		}
	}
	c.d.release()
}

// readEnd says how a read on tc may go once its session has no call in
// progress: on as usual, or until deadline, or not at all.
func (d *drain) readEnd(tc *trackedConn) (end bool, deadline time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ending := d.closed || (d.draining && d.endIdle)
	if !ending || tc.calls > 0 {
		return false, time.Time{}
	}
	now := time.Now()
	if tc.graceUntil.IsZero() {
		tc.graceUntil = now.Add(endGrace)
	}
	if !now.Before(tc.graceUntil) {
		return true, time.Time{}
	}
	return false, tc.graceUntil
}

// shutdown runs the stop. Every wait in it ends when end does: the session
// wait after d.bound or end, and the wait for the writes at end.
func (d *drain) shutdown(end context.Context) {
	d.mu.Lock()
	d.draining = true
	d.quiet = make(chan struct{})
	if d.active == 0 {
		close(d.quiet)
		d.quietClosed = true
	}
	listeners := d.listeners
	var byes []func()
	if d.endIdle {
		for tc := range d.conns {
			if tc.bye != nil && tc.calls == tc.idles {
				byes = append(byes, tc.bye)
			}
		}
	}
	d.mu.Unlock()

	mailLog.Info("shutting down server", "server", d.label)
	for _, ln := range listeners {
		// The server's own Close closes them again later; a second close only
		// reports that it is already closed.
		_ = ln.Close()
	}
	// A goodbye writes to the client, and a stuck client blocks the write
	// until the cut below closes its connection, so each runs on its own.
	for _, bye := range byes {
		d.byes.Add(1)
		go func() {
			defer d.byes.Done()
			bye()
		}()
	}

	sessions, cancel := context.WithTimeout(end, d.bound)
	defer cancel()
	select {
	case <-d.quiet:
	case <-sessions.Done():
		mailLog.Warn("cutting the sessions still open after the drain", "server", d.label,
			"bound", d.bound, "cause", context.Cause(sessions))
	}

	d.mu.Lock()
	d.closed = true
	var cut []*trackedConn
	writes := 0
	for tc := range d.conns {
		if tc.writes == 0 {
			cut = append(cut, tc)
		} else {
			writes += tc.writes
		}
	}
	d.mu.Unlock()
	for _, tc := range cut {
		tc.cut()
	}
	if writes > 0 {
		mailLog.Info("waiting for the writes in progress before the database closes",
			"server", d.label, "writes", writes)
	}

	select {
	case <-d.quiet:
	case <-end.Done():
		// The writes still run, so the server is not closed under them: the
		// process ends next, which closes their connections.
		d.mu.Lock()
		writes = 0
		for tc := range d.conns {
			writes += tc.writes
		}
		d.mu.Unlock()
		mailLog.Error("closing the app under mail writes that did not finish by the terminate's deadline",
			"server", d.label, "writes", writes, "cause", context.Cause(end))
		return
	}
	// The listeners closed at the start, so every serve loop has returned or
	// is returning. Wait for them before the server's Close: go-imap's Serve
	// counts itself in a WaitGroup that its Close waits on, outside any lock,
	// so a Close that can run before Serve has counted itself races (a stop
	// right after the start).
	d.served.Wait()
	if d.closeServer != nil {
		d.closeServer()
	}
	d.byes.Wait()
}

// trackedConn is one network connection a drain tracks. Its fields after d
// are guarded by d.mu.
type trackedConn struct {
	net.Conn
	d         *drain
	closeOnce sync.Once

	// calls counts the session calls in progress; idles and writes count
	// those that are IMAP IDLEs and writes. More than one call runs at once
	// only around IDLE, whose call runs beside the connection's reads.
	calls, idles, writes int
	bye                  func()
	graceUntil           time.Time
}

// Close is the protocol server's close: the connection then leaves the
// drain.
func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() { c.d.untrack(c) })
	return err
}

// cut is the drain's close. It leaves the connection tracked until the
// server's own Close, which comes after the session has closed.
func (c *trackedConn) cut() {
	_ = c.Conn.Close()
}

func (c *trackedConn) Read(p []byte) (int, error) {
	end, deadline := c.d.readEnd(c)
	if end {
		return 0, io.EOF
	}
	if !deadline.IsZero() {
		_ = c.Conn.SetReadDeadline(deadline)
	}
	return c.Conn.Read(p)
}

// trackedOf finds the tracked connection under c, under any TLS layer. It
// returns nil for a connection no drain tracks.
func trackedOf(c net.Conn) *trackedConn {
	for {
		switch conn := c.(type) {
		case *trackedConn:
			return conn
		case *tls.Conn:
			c = conn.NetConn()
		default:
			return nil
		}
	}
}

const stopsStoreKey = "tinycld.mailproto.stops"

// stopGroup holds the stops of every server mailproto started for one app,
// and runs them together at the app's terminate.
type stopGroup struct {
	mu    sync.Mutex
	stops []func()
}

// bindStop runs stop at app's terminate, before the database closes. The
// group lives in app.Store(), so every value that shares the app's base finds
// the same one, and one terminate handler runs all of an app's stops at the
// same time.
func bindStop(app core.App, stop func()) {
	// Outside the GetOrSet below: Register makes its own store value.
	shutdown.Register(app)
	v := app.Store().GetOrSet(stopsStoreKey, func() any {
		g := &stopGroup{}
		app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
			Priority: terminatePriority,
			Func: func(e *core.TerminateEvent) error {
				g.run()
				return e.Next()
			},
		})
		return g
	})
	g := v.(*stopGroup)
	g.mu.Lock()
	g.stops = append(g.stops, stop)
	g.mu.Unlock()
}

func (g *stopGroup) run() {
	g.mu.Lock()
	stops := g.stops
	g.stops = nil
	g.mu.Unlock()
	var wg sync.WaitGroup
	for _, stop := range stops {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stop()
		}()
	}
	wg.Wait()
}
