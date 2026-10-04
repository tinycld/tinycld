package format

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// ErrStalled is what a transfer reports when no bytes moved for StallDeadline.
// A presigned target that accepts the connection and then never reads is
// indistinguishable from a very slow one at the socket level, so the deadline is
// on PROGRESS rather than on the transfer as a whole: an archive can take hours
// and must not be cut off for taking them.
var ErrStalled = errors.New("backup: the transfer stalled")

// StallDeadline is how long a transfer may move no bytes at all before it is
// given up on. It is a package var so a test can shorten it; nothing else writes
// it.
//
// Without it a target that accepts and never reads parks the transfer goroutine
// AND the installjob interlock for the life of the process, so no backup,
// restore or package install can ever run again.
var StallDeadline = 5 * time.Minute

// The transport's own budgets. There is deliberately no overall Timeout: a
// backup of a large organization legitimately runs for hours, and a whole-client
// timeout would cut it off mid-archive. What is bounded is every phase in which
// a healthy peer answers promptly.
const (
	dialTimeout           = 30 * time.Second
	tlsHandshakeTimeout   = 30 * time.Second
	responseHeaderTimeout = 60 * time.Second
	expectContinueTimeout = 10 * time.Second
	idleConnTimeout       = 90 * time.Second
)

// NoRedirectClient never follows a redirect: a presigned URL that redirects
// would leak its signature to the next host.
func NoRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: dialTimeout}).DialContext,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ResponseHeaderTimeout: responseHeaderTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
			IdleConnTimeout:       idleConnTimeout,
			ForceAttemptHTTP2:     true,
		},
	}
}

// stallTimer is the subset of *time.Timer the guard needs. A test substitutes
// newStallTimer with a fake that fires only when told to, so the deadline is
// crossed by an explicit step rather than by outrunning a real sleep — the
// margin between a tick and the deadline is then exact, not a wall-clock bet.
type stallTimer interface {
	Reset(d time.Duration) bool
	Stop() bool
}

// newStallTimer starts a real timer; tests override this package var to
// control the guard's timer deterministically. Production behavior is
// unchanged: it is exactly time.AfterFunc under another name.
var newStallTimer = func(d time.Duration, f func()) stallTimer {
	return time.AfterFunc(d, f)
}

// stallGuard cancels a transfer's context when no bytes have moved for
// StallDeadline. Every Read or Write resets the timer, so a transfer that is
// merely slow is never touched.
//
// The guard cancels rather than returning an error from the middle of a Read:
// the goroutine that is stuck is inside the transport, not inside our code, and
// cancelling its context is the only thing that unsticks it.
type stallGuard struct {
	cancel context.CancelFunc

	mu      sync.Mutex
	timer   stallTimer
	stalled bool
	stopped bool
}

// newStallGuard derives a cancellable context from parent and starts the timer.
// The caller must call stop() once the transfer is over, or the timer holds a
// reference to the context for StallDeadline after it ends.
func newStallGuard(parent context.Context) (context.Context, *stallGuard) {
	ctx, cancel := transferContext(parent)
	g := &stallGuard{cancel: cancel}
	g.timer = newStallTimer(StallDeadline, g.fire)
	return ctx, g
}

func (g *stallGuard) fire() {
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return
	}
	g.stalled = true
	g.mu.Unlock()
	g.cancel()
}

// progressed restarts the deadline. Called on every byte movement, so it is kept
// cheap: Reset on a fired timer is a no-op for us because fire() has already
// cancelled and the transfer is unwinding.
func (g *stallGuard) progressed() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped || g.stalled {
		return
	}
	g.timer.Reset(StallDeadline)
}

func (g *stallGuard) stop() {
	g.mu.Lock()
	g.stopped = true
	g.timer.Stop()
	g.mu.Unlock()
	g.cancel()
}

// fired reports whether this guard is what ended the transfer. A read cancelled
// mid-body does NOT come back as context.Canceled — the transport wraps it in
// its own error type — so errors.Is on the read's error is not a usable test for
// "was this a stall". The guard's own state is.
func (g *stallGuard) fired() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stalled
}

// classify turns the cancellation the guard caused back into ErrStalled. Without
// it the caller sees "context canceled" and cannot tell a stalled transfer from
// a shutdown or an operator's cancel.
func (g *stallGuard) classify(err error) error {
	if err == nil {
		return nil
	}
	if g.fired() {
		return ErrStalled
	}
	return err
}

// Watchdog is the stall guard for a transfer whose bytes move inside another
// client — a repository's Put, or a reader it opened — where this package owns
// no socket and progress is only what the caller reports.
type Watchdog struct{ g *stallGuard }

// Watch derives a context that ends when parent does, at shutdown, at CancelAll,
// or once Progressed has not been called for StallDeadline. The caller must
// call Stop once the transfer is over.
func Watch(parent context.Context) (context.Context, *Watchdog) {
	ctx, g := newStallGuard(parent)
	return ctx, &Watchdog{g: g}
}

// Progressed restarts the deadline. Call it only when bytes actually moved: a
// callback that repeats the same count is not progress.
func (w *Watchdog) Progressed() { w.g.progressed() }

func (w *Watchdog) Stop() { w.g.stop() }

// Err turns the cancellation the watchdog caused into ErrStalled, so the ledger
// says "stalled" rather than whatever the other client wrapped the cancel in.
func (w *Watchdog) Err(err error) error { return w.g.classify(err) }

// Reader counts every read that returns bytes as progress.
func (w *Watchdog) Reader(r io.Reader) io.Reader { return &watchedReader{r: r, w: w} }

type watchedReader struct {
	r io.Reader
	w *Watchdog
}

func (r *watchedReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.w.Progressed()
	}
	return n, err
}

// Lifetime is a transfer's context without a stall deadline, for a call whose
// progress the caller cannot see: it still ends at shutdown and at CancelAll.
func Lifetime(parent context.Context) (context.Context, context.CancelFunc) {
	return transferContext(parent)
}

// Shutdown is the process-wide parent every transfer derives from, so a graceful
// stop cancels the ones in flight instead of letting them hold the installjob
// interlock until the process is killed.
//
// It is a package var rather than a parameter because the transports are built
// from an HTTP handler, which has no reason to know about process lifetime, and
// because a transfer started before the shutdown must be cancelled by it too.
var (
	shutdownMu  sync.Mutex
	shutdownCtx = context.Background()
	shutdownAll context.CancelFunc
)

// SetShutdown binds the process's lifetime. Every transfer started afterwards is
// cancelled when ctx is done.
func SetShutdown(ctx context.Context) {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	shutdownCtx, shutdownAll = context.WithCancel(ctx)
}

// CancelAll cancels every transfer in flight. Safe to call more than once, and
// safe to call when nothing registered a shutdown.
func CancelAll() {
	shutdownMu.Lock()
	cancel := shutdownAll
	shutdownMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ResetShutdownForTesting drops the bound lifetime. A test that cancels the
// process-wide context would otherwise kill every transfer in every later test
// in the binary.
func ResetShutdownForTesting() {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	shutdownCtx, shutdownAll = context.Background(), nil
}

// ManualStallTimerForTesting is a Watchdog's timer driven by explicit steps
// instead of real time: CheckDeadline is the test's stand-in for the
// deadline elapsing, so a test proves a repository Put (or any Watch caller)
// survives many progress ticks without racing a real sleep against
// StallDeadline. It fires only when nothing has reset it (via Progressed)
// since the previous check.
type ManualStallTimerForTesting struct {
	mu       sync.Mutex
	fire     func()
	active   bool
	touched  bool
	fireOnce sync.Once
}

func (m *ManualStallTimerForTesting) Reset(time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	wasActive := m.active
	m.active, m.touched = true, true
	return wasActive
}

func (m *ManualStallTimerForTesting) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	wasActive := m.active
	m.active = false
	return wasActive
}

// CheckDeadline fires the timer exactly when it has not been touched (armed
// or reset) since the previous check.
func (m *ManualStallTimerForTesting) CheckDeadline() {
	m.mu.Lock()
	crossed := m.active && !m.touched
	m.touched = false
	m.mu.Unlock()
	if crossed {
		m.fireOnce.Do(m.fire)
	}
}

// InstallManualStallTimerForTesting replaces the real timer every new
// Watch/Watchdog uses with a *ManualStallTimerForTesting, and returns a
// restore func the caller runs via t.Cleanup. The manual timer for a
// specific Watch call is only known once that call creates it — call Watch
// after this, then read the timer via the callback's argument.
func InstallManualStallTimerForTesting(onCreate func(*ManualStallTimerForTesting)) (restore func()) {
	prev := newStallTimer
	newStallTimer = func(_ time.Duration, fire func()) stallTimer {
		mt := &ManualStallTimerForTesting{fire: fire, active: true, touched: true}
		if onCreate != nil {
			onCreate(mt)
		}
		return mt
	}
	return func() { newStallTimer = prev }
}

// transferContext merges a caller's context with the process lifetime: whichever
// ends first ends the transfer.
func transferContext(parent context.Context) (context.Context, context.CancelFunc) {
	shutdownMu.Lock()
	shutdown := shutdownCtx
	shutdownMu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	if shutdown.Done() == nil {
		return ctx, cancel
	}
	// AfterFunc runs cancel on its own goroutine even when shutdown has already
	// ended, so a transfer begun after a stop would start on a live context and
	// race its own cancellation. It must not start at all.
	if shutdown.Err() != nil {
		cancel()
		return ctx, cancel
	}
	stop := context.AfterFunc(shutdown, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}
