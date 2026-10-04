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
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/cron"
	"tinycld.org/core/logging"
)

var log = logging.ForPackage("readonly")

const (
	Code              = "read_only"
	RetryAfterSeconds = 2
	message           = "The server is updating. Try again in a moment."
)

var (
	active atomic.Bool

	// mu orders Enter and Leave with the channel that WaitInactive blocks on.
	// left is closed when the mode is left; Enter makes a new one.
	mu   sync.Mutex
	left = closedChan()
)

func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func Enter() {
	mu.Lock()
	defer mu.Unlock()
	if active.CompareAndSwap(false, true) {
		left = make(chan struct{})
		log.Info("read-only mode on: writes are refused until the mode is left")
	}
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

// Register binds the middleware, the cron guard and the SIGUSR2 trigger. Bind
// it before any middleware that reports 5xx responses: a refused write is
// expected during a pause and must not reach error reporting.
func Register(app core.App) {
	guardCron(app.Cron())
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.BindFunc(Middleware)
		return e.Next()
	})
	watchSignal()
}

// Middleware refuses every unsafe request to /api/ while the mode is on.
// POST /api/realtime is let through: it only sets which topics an open SSE
// stream carries and writes no record, and a client that reconnects during
// the pause needs it to subscribe again.
func Middleware(re *core.RequestEvent) error {
	if !active.Load() || safe(re.Request.Method) {
		return re.Next()
	}
	path := re.Request.URL.Path
	if !strings.HasPrefix(path, "/api/") || path == "/api/realtime" {
		return re.Next()
	}
	re.Response.Header().Set("Retry-After", strconv.Itoa(RetryAfterSeconds))
	return re.JSON(http.StatusServiceUnavailable, map[string]string{"code": Code, "message": message})
}

func safe(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
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
