package pbs

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	gopbs "github.com/osshield/gopbs/pbs"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/logging"
)

// PBS connections fail now and then in ways the error alone does not explain:
// a reset during the protocol upgrade, a connect that times out, a session that
// goes silent. Each operation therefore records what every connection it opened
// did, and logs that record when the operation fails, followed by a probe that
// shows whether the server answers right after. A finished operation logs its
// timings at Info, so a failure can be compared with the normal case.
//
// The probe runs in the background: against a server that does not answer it
// takes tens of seconds, and the failed operation must not wait for it. A
// process that exits right after an operation calls WaitForProbes first.

var log = logging.ForPackage("backup")

// probeDelays is when each probe attempt starts, from the end of the failed
// operation. Tests set it to zeros.
var probeDelays = []time.Duration{0, 5 * time.Second, 10 * time.Second}

// probeTimeout bounds one probe attempt: the connect and the TLS handshake.
const probeTimeout = 10 * time.Second

// probesRunning counts the probes still in the background.
var probesRunning sync.WaitGroup

// WaitForProbes waits up to timeout for the probes that failed operations
// started, and reports whether they all finished. A process that exits right
// after an operation (a DR job) calls it, or the probe's record is lost.
func WaitForProbes(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		probesRunning.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

type trackerKey struct{}

// opTracker records the connections of one repository operation.
type opTracker struct {
	op      string
	server  string
	started time.Time

	mu          sync.Mutex
	conns       []*trackedConn
	failedDials []dialFailure
}

type dialFailure struct {
	Addr   string  `json:"addr"`
	DialMS float64 `json:"dial_ms"`
	Error  string  `json:"error"`
}

// observe starts recording op. The returned context carries the tracker to
// Config.Dial; end logs the outcome and must be called with the operation's
// final error.
func (r *Repository) observe(ctx context.Context, op string) (context.Context, func(err error)) {
	t := &opTracker{op: op, server: r.server, started: time.Now()}
	return context.WithValue(ctx, trackerKey{}, t), t.end
}

// dial is the PBS client's Config.Dial: a plain connect that registers the
// connection with the operation's tracker.
func dial(ctx context.Context, network, addr string) (net.Conn, error) {
	start := time.Now()
	var d net.Dialer
	c, err := d.DialContext(ctx, network, addr)
	t, _ := ctx.Value(trackerKey{}).(*opTracker)
	if t == nil {
		return c, err
	}
	took := time.Since(start)
	t.mu.Lock()
	defer t.mu.Unlock()
	if err != nil {
		t.failedDials = append(t.failedDials, dialFailure{Addr: addr, DialMS: ms(took), Error: err.Error()})
		return nil, err
	}
	tc := &trackedConn{Conn: c, opened: time.Now(), dialTook: took}
	t.conns = append(t.conns, tc)
	return tc, nil
}

func (t *opTracker) end(err error) {
	elapsed := time.Since(t.started)
	t.mu.Lock()
	conns := make([]connReport, len(t.conns))
	for i, c := range t.conns {
		conns[i] = c.report()
	}
	failed := append([]dialFailure(nil), t.failedDials...)
	t.mu.Unlock()

	attrs := []any{"op", t.op, "server", t.server, "elapsed_ms", ms(elapsed), "conns", conns}
	if len(failed) > 0 {
		attrs = append(attrs, "failed_dials", failed)
	}
	switch {
	case err == nil:
		log.Info("pbs: operation finished", attrs...)
	case errors.Is(err, context.Canceled):
		// The caller gave up (a shutdown, a cancelled job): not the link.
		log.Info("pbs: operation canceled", append(attrs, "error", err.Error())...)
	default:
		class := classify(err)
		log.Warn("pbs: operation failed", append(attrs, "error", err.Error(), "error_class", class)...)
		if probes(class) {
			probesRunning.Add(1)
			go func() {
				defer probesRunning.Done()
				log.Warn("pbs: probe after a failure", "op", t.op, "server", t.server, "attempts", probe(t.server))
			}()
		}
	}
}

// classify names the kind of failure, so the log can be grouped by it.
func classify(err error) string {
	var opErr *net.OpError
	isDial := errors.As(err, &opErr) && opErr.Op == "dial"
	var netErr net.Error
	switch {
	case errors.Is(err, format.ErrStalled):
		return "stalled"
	case errors.Is(err, gopbs.ErrFingerprint):
		return "fingerprint"
	case errors.Is(err, gopbs.ErrAuth):
		return "auth"
	case isDial && errors.Is(err, syscall.ECONNREFUSED):
		return "dial-refused"
	case isDial && (errors.Is(err, syscall.ETIMEDOUT) || errors.As(err, &netErr) && netErr.Timeout()):
		return "dial-timeout"
	case isDial:
		return "dial-other"
	case errors.Is(err, syscall.ECONNRESET):
		return "reset"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return "eof"
	case strings.Contains(err.Error(), "client connection lost"):
		return "connection-lost"
	case strings.Contains(err.Error(), "tls:"):
		return "tls"
	}
	return "other"
}

// probes reports whether a failure of class says something about the path to
// the server, so that a probe is worth its time.
func probes(class string) bool {
	switch class {
	case "fingerprint", "auth", "other":
		return false
	}
	return true
}

type probeAttempt struct {
	AfterMS   float64 `json:"after_ms"`
	ConnectMS float64 `json:"connect_ms"`
	TLSMS     float64 `json:"tls_ms,omitempty"`
	Local     string  `json:"local,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// probe connects to server and completes a TLS handshake once per probeDelays
// entry. It checks reachability only: it sends no credentials and reads no
// data, so the certificate is not verified.
func probe(server string) []probeAttempt {
	start := time.Now()
	out := make([]probeAttempt, 0, len(probeDelays))
	for _, delay := range probeDelays {
		if wait := delay - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}
		out = append(out, probeOnce(server, time.Since(start)))
	}
	return out
}

func probeOnce(server string, after time.Duration) probeAttempt {
	a := probeAttempt{AfterMS: ms(after)}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	start := time.Now()
	var d net.Dialer
	raw, err := d.DialContext(ctx, "tcp", server)
	a.ConnectMS = ms(time.Since(start))
	if err != nil {
		a.Error = err.Error()
		return a
	}
	defer raw.Close()
	a.Local = raw.LocalAddr().String()
	host, _, _ := net.SplitHostPort(server)
	conn := tls.Client(raw, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	start = time.Now()
	err = conn.HandshakeContext(ctx)
	a.TLSMS = ms(time.Since(start))
	if err != nil {
		a.Error = err.Error()
	}
	return a
}

// trackedConn is one TCP connection of an operation, counting what moved on it
// and keeping the first I/O error with the socket's state at that moment.
type trackedConn struct {
	net.Conn
	opened   time.Time
	dialTook time.Duration

	read, written       atomic.Int64
	firstRead           atomic.Int64 // unix nanos; 0 until the first byte arrives
	lastRead, lastWrite atomic.Int64 // unix nanos
	closedAt            atomic.Int64 // unix nanos; 0 while open

	mu       sync.Mutex
	ioErr    string
	ioErrOp  string
	ioErrAt  time.Time
	tcpAtErr *tcpInfo
	tcpAtEnd *tcpInfo
}

func (c *trackedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		now := time.Now().UnixNano()
		c.read.Add(int64(n))
		c.lastRead.Store(now)
		c.firstRead.CompareAndSwap(0, now)
	}
	if err != nil {
		c.noteErr("read", err)
	}
	return n, err
}

func (c *trackedConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.written.Add(int64(n))
		c.lastWrite.Store(time.Now().UnixNano())
	}
	if err != nil {
		c.noteErr("write", err)
	}
	return n, err
}

// Close keeps the socket's state as it was at the end: a session that went
// silent and was closed by the client's ping timeout shows it there.
func (c *trackedConn) Close() error {
	if c.closedAt.CompareAndSwap(0, time.Now().UnixNano()) {
		info := readTCPInfo(c.Conn)
		c.mu.Lock()
		c.tcpAtEnd = info
		c.mu.Unlock()
	}
	return c.Conn.Close()
}

// noteErr keeps the first error that came from the network, with the socket's
// state read while the socket is still open. An error after this side closed
// the connection is the close, not the network.
func (c *trackedConn) noteErr(op string, err error) {
	if c.closedAt.Load() != 0 && errors.Is(err, net.ErrClosed) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ioErr != "" {
		return
	}
	c.ioErr, c.ioErrOp, c.ioErrAt = err.Error(), op, time.Now()
	c.tcpAtErr = readTCPInfo(c.Conn)
}

// connReport is a connection as the log shows it. Times are milliseconds from
// the moment the connection opened, so one record reads as a timeline.
type connReport struct {
	Local        string   `json:"local"`
	Remote       string   `json:"remote"`
	DialMS       float64  `json:"dial_ms"`
	ReadBytes    int64    `json:"read_bytes"`
	WrittenBytes int64    `json:"written_bytes"`
	FirstByteMS  *float64 `json:"first_byte_ms,omitempty"`
	LastReadMS   *float64 `json:"last_read_ms,omitempty"`
	LastWriteMS  *float64 `json:"last_write_ms,omitempty"`
	ClosedMS     *float64 `json:"closed_ms,omitempty"`
	IOError      *string  `json:"io_error,omitempty"`
	IOErrorOp    string   `json:"io_error_op,omitempty"`
	IOErrorMS    *float64 `json:"io_error_ms,omitempty"`
	TCPAtError   *tcpInfo `json:"tcp_at_error,omitempty"`
	TCPAtClose   *tcpInfo `json:"tcp_at_close,omitempty"`
}

func (c *trackedConn) report() connReport {
	since := func(nanos int64) *float64 {
		if nanos == 0 {
			return nil
		}
		v := ms(time.Unix(0, nanos).Sub(c.opened))
		return &v
	}
	r := connReport{
		Local:        c.LocalAddr().String(),
		Remote:       c.RemoteAddr().String(),
		DialMS:       ms(c.dialTook),
		ReadBytes:    c.read.Load(),
		WrittenBytes: c.written.Load(),
		FirstByteMS:  since(c.firstRead.Load()),
		LastReadMS:   since(c.lastRead.Load()),
		LastWriteMS:  since(c.lastWrite.Load()),
		ClosedMS:     since(c.closedAt.Load()),
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ioErr != "" {
		e := c.ioErr
		r.IOError, r.IOErrorOp, r.IOErrorMS = &e, c.ioErrOp, since(c.ioErrAt.UnixNano())
	}
	r.TCPAtError, r.TCPAtClose = c.tcpAtErr, c.tcpAtEnd
	return r
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
