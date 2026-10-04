package backup

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
)

// ErrStopping refuses a run on an app that is shutting down. It matches ErrBusy
// because a caller handles it the same way: nothing was attempted, and the run
// can be asked for again once a server is up.
var ErrStopping error = stoppingError{}

type stoppingError struct{}

func (stoppingError) Error() string        { return "backup: the server is shutting down" }
func (stoppingError) Is(target error) bool { return target == ErrBusy }

// runsKey files an app's runSet in the app's own store. The set belongs to the
// app rather than to the process because what it guards is the app: a run's
// terminal work writes through that app, so it is that app's close that must
// wait. Two apps in one process (as in tests) must not latch each other.
const runsKey = "tinycld.backup.runs"

// runSet counts the backup and restore runs an app has in flight, and latches
// shut once the app starts to stop so no run can begin after the wait did.
type runSet struct {
	mu       sync.Mutex
	stopping bool
	active   int
	idle     chan struct{} // made by a waiter; closed when active reaches 0
}

func runsOf(app core.App) *runSet {
	return app.Store().GetOrSet(runsKey, func() any { return &runSet{} }).(*runSet)
}

// trackRun registers a run before it claims anything, and returns what the run
// calls once it has done its last piece of work with the app.
func trackRun(app core.App) (finished func(), err error) {
	s := runsOf(app)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return nil, ErrStopping
	}
	s.active++
	return sync.OnceFunc(s.done), nil
}

func (s *runSet) done() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	if s.active == 0 && s.idle != nil {
		close(s.idle)
		s.idle = nil
	}
}

// StopAll is what an app's terminate hook calls before the app closes. It
// refuses every later run on app, cancels every transfer in flight, and waits
// until each of app's runs has finished its terminal work — the ledger row, the
// announcement and the callback — or ctx ends.
//
// The wait is what keeps a run's terminal work off a closed database: a
// cancelled run still writes through app on its way out. The error says how
// many runs were still going when ctx ended; those runs are left to the panic
// guard around their terminal work.
//
// The cancel is process-wide, as every transfer is bound to the process (see
// SetShutdown); the wait is for app's own runs.
func StopAll(ctx context.Context, app core.App) error {
	s := runsOf(app)
	s.mu.Lock()
	s.stopping = true
	if s.active == 0 {
		s.mu.Unlock()
		format.CancelAll()
		return nil
	}
	if s.idle == nil {
		s.idle = make(chan struct{})
	}
	idle := s.idle
	s.mu.Unlock()

	format.CancelAll()
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		n := s.active
		s.mu.Unlock()
		return fmt.Errorf("backup: %d backup or restore run(s) still running: %w", n, ctx.Err())
	}
}

// Arm gives app a fresh lifetime for its runs: a shutdown context every
// transfer and announcement derives from, and a run set that accepts runs.
// It is the one place both are set, and it undoes StopAll, so it belongs on the
// app's bootstrap — the point from which the app can run backups at all, and
// the point a failed restart returns to.
//
// Transfers still running on an earlier lifetime are not re-bound; StopAll has
// already waited for them by the time a stopped app is bootstrapped again.
func Arm(app core.App) {
	format.SetShutdown(context.Background())
	s := runsOf(app)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopping = false
}

// survive runs one step of a run's terminal work and turns a panic into a log
// line. The steps run inside the run's own deferred finalizer, past the recover
// that guards the run's body, so a panic here would otherwise end the process.
// The steps are guarded one by one so a failed announcement still lets the
// callback go out.
func survive(id, step string, fn func()) {
	defer func() {
		if p := recover(); p != nil {
			log.Error("a backup run's terminal step panicked", "id", id, "step", step, "panic", p,
				"stack", string(debug.Stack()))
		}
	}()
	fn()
}
