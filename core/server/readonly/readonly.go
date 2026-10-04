// Package readonly lets a server stop accepting writes for a short time while
// it keeps serving reads. A supervising process uses it when a second server
// process is about to migrate the same database: the first one must not write
// against a schema it does not know, but its users should keep reading.
//
// The mode is process-wide. It is switched on by Enter (or SIGUSR2) and off
// only by Leave: a signal can open the pause but never close it, so a stray
// or repeated signal cannot re-open writes in the middle of a migration.
package readonly

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/cron"
	"tinycld.org/core/logging"
)

var log = logging.ForPackage("readonly")

const (
	Code              = "read_only"
	RetryAfterSeconds = 2
	message           = "The server is updating. Try again in a moment."

	// TailWait bounds how long a write that follows a request already
	// accepted may wait in WhenWritable for the mode to turn off: longer
	// than a normal swap, short enough that a stuck mode cannot pile up
	// goroutines for ever.
	TailWait = 2 * time.Minute
)

var (
	active atomic.Bool

	// mu orders Enter and Leave with the channel that WaitInactive blocks on,
	// and guards onEnterFns.
	// left is closed when the mode is left; Enter makes a new one.
	mu         sync.Mutex
	left       = closedChan()
	onEnterFns []func()
)

func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func Enter() {
	mu.Lock()
	if !active.CompareAndSwap(false, true) {
		mu.Unlock()
		return
	}
	left = make(chan struct{})
	fns := append([]func(){}, onEnterFns...)
	mu.Unlock()

	log.Info("read-only mode on: writes are refused until the mode is left")
	for _, fn := range fns {
		runOnEnter(fn)
	}
}

// runOnEnter runs fn, recovering a panic rather than letting it crash the
// process: this happens exactly while an upgrade has the server read-only, so
// a panicking writer must not take the whole process down with it, and other
// registered fns must still run.
func runOnEnter(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("an OnEnter fn panicked", "panic", r)
		}
	}()
	fn()
}

// OnEnter registers fn to run each time read-only mode starts (a false->true
// transition) — not on a repeated Enter while already active. For a writer
// that holds a long-lived connection and needs to react (pause, flush,
// disconnect) as soon as the mode turns on, rather than poll Active or block
// in WhenWritable.
//
// fn runs synchronously, before Enter returns and before the caller's own
// next step: a caller that enters the mode to take a backup starts the backup
// as soon as Enter returns, so a writer that must land its last state ahead
// of the backup has only this window. fn bounds its own wait. The
// collaborative-document broker uses it to flush and store every document.
//
// A fn registered while the mode is already active runs only at the next
// transition, not immediately. The mode's mutex is NOT held while fn runs,
// so fn may call Active and WaitInactive.
func OnEnter(fn func()) {
	mu.Lock()
	defer mu.Unlock()
	onEnterFns = append(onEnterFns, fn)
}

// resetOnEnterForTest clears every registration. OnEnter is process-wide like
// the rest of this package's state, so a test that registers one must call
// this in t.Cleanup to avoid leaking it into later tests.
func resetOnEnterForTest() {
	mu.Lock()
	defer mu.Unlock()
	onEnterFns = nil
}

func Leave() {
	mu.Lock()
	defer mu.Unlock()
	if active.CompareAndSwap(true, false) {
		close(left)
		log.Info("read-only mode off: writes are accepted again")
	}
}

func Active() bool { return active.Load() }

// WaitInactive returns nil once the mode is off, or ctx's error if ctx ends
// first. A background worker that writes outside a request,
// which the middleware cannot refuse, calls it before each write cycle when
// it should pause rather than skip the cycle.
func WaitInactive(ctx context.Context) error {
	for {
		if !active.Load() {
			return nil
		}
		mu.Lock()
		ch := left
		mu.Unlock()
		select {
		case <-ch:
			// The mode can be entered again before this waiter wakes, so the
			// loop checks it once more.
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// WhenWritable waits until read-only mode is off (or ctx ends), then runs fn.
// For a write that follows a request already accepted (a tail effect such as
// a notification or audit row) rather than one the middleware could refuse
// outright. Returns ctx's error without running fn if ctx ends first.
//
// The wait and the run of fn are not atomic: the mode can be entered again in
// the window between the wait returning and fn running, so fn itself is not
// guaranteed to run only while the mode is off.
func WhenWritable(ctx context.Context, fn func() error) error {
	if err := WaitInactive(ctx); err != nil {
		return err
	}
	return fn()
}

// Register binds the middleware, the cron guard, the end of tail waits at
// terminate (see TailContext) and the SIGUSR2 trigger. Bind
// it before any middleware that reports 5xx responses: a refused write is
// expected during a pause and must not reach error reporting.
func Register(app core.App) {
	guardCron(app.Cron())
	bindTails(app)
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.BindFunc(Middleware)
		return e.Next()
	})
	watchSignal()
}

// Middleware refuses every unsafe request on every path while the mode is on:
// the API, and also DAV writes (/caldav, /carddav, /dav/drive, a package's own
// DAV prefix), which do not go through /api/ but still write. POST
// /api/realtime is let through: it only sets which topics an open SSE stream
// carries and writes no record, and a client that reconnects during the pause
// needs it to subscribe again.
func Middleware(re *core.RequestEvent) error {
	if !active.Load() || safe(re.Request.Method) {
		return re.Next()
	}
	if re.Request.URL.Path == "/api/realtime" {
		return re.Next()
	}
	re.Response.Header().Set("Retry-After", strconv.Itoa(RetryAfterSeconds))
	return re.JSON(http.StatusServiceUnavailable, map[string]string{"code": Code, "message": message})
}

// safe reports a method that never writes: GET, HEAD, OPTIONS, and the DAV
// read methods PROPFIND and REPORT.
func safe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, "PROPFIND", "REPORT":
		return true
	default:
		return false
	}
}

// guardCron makes the scheduler skip every due job while the mode is on: a
// cron job writes without a request, so the middleware cannot refuse it. The
// guard is on the scheduler, not on each job, so it also covers PocketBase's
// own jobs and jobs that packages or JS hooks add. A skipped tick is not made
// up; the job runs at its next due time after Leave.
func guardCron(c *cron.Cron) {
	c.SetSkip(skipCronTick)
}

func skipCronTick(jobID string) bool {
	if !active.Load() {
		return false
	}
	log.Debug("cron job skipped: read-only", "job", jobID)
	return true
}
