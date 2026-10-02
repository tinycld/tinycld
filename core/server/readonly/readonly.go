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
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var log = logging.ForPackage("readonly")

const (
	Code              = "read_only"
	RetryAfterSeconds = 2
	message           = "The server is updating. Try again in a moment."
)

var active atomic.Bool

func Enter() {
	if active.CompareAndSwap(false, true) {
		log.Info("read-only mode on: writes are refused until the mode is left")
	}
}

func Leave() {
	if active.CompareAndSwap(true, false) {
		log.Info("read-only mode off: writes are accepted again")
	}
}

func Active() bool { return active.Load() }

// Register binds the middleware and the SIGUSR2 trigger. Bind it before any
// middleware that reports 5xx responses: a refused write is expected during a
// pause and must not reach error reporting.
func Register(app core.App) {
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
