package backup

import (
	"sync"

	"tinycld.org/core/backup/format"
)

// restoreWatcher lets a test outside this package bound the lifetime of a restore
// goroutine. StartRestore calls it before the goroutine starts and calls what it
// returns when the goroutine ends, so a test can wait for every restore it began
// before its app's database is closed under one.
//
// It is a seam rather than an exported WaitGroup because nothing in production
// waits: a restore's whole point is to outlive the request that asked for it.
var (
	watcherMu      sync.RWMutex
	restoreWatcher func() func()
)

// SetRestoreWatcher installs the seam. Pass nil to remove it.
func SetRestoreWatcher(fn func() func()) {
	watcherMu.Lock()
	defer watcherMu.Unlock()
	restoreWatcher = fn
}

// watchRestore reports the goroutine's start and returns its completion callback.
func watchRestore() func() {
	watcherMu.RLock()
	fn := restoreWatcher
	watcherMu.RUnlock()
	if fn == nil {
		return func() {}
	}
	return fn()
}

// ResetForTesting returns every process-wide seam in this package to its
// zero state.
//
// It is exported, and lives in a non-test file, because the state it clears is
// not reachable from another package's tests any other way — and leaving it set
// is not a harmless leak. A test that calls coreserver's Register installs the
// REAL restart function, which ends the process; the next test in that package
// to reach a restore would then kill the test binary. `restoring` left true puts
// every later request behind the maintenance 503. A registered rebuilder left
// behind would try to run a build.
//
// Call it from a t.Cleanup in any test that registers a composition or exercises
// a restore.
func ResetForTesting() {
	restoring.Store(false)

	watcherMu.Lock()
	restoreWatcher = nil
	watcherMu.Unlock()

	rebuilderMu.Lock()
	rebuilder = nil
	rebuilderMu.Unlock()

	restartMu.Lock()
	restartFn = func() bool { return true }
	restarted = false
	restartMu.Unlock()

	// A restore that never finished leaves its source registered for a URL swap,
	// so a later SwapSource would reach a dead restore's reader.
	waitingMu.Lock()
	waiting = map[string]*format.RangeSource{}
	waitingMu.Unlock()

	// Register binds the process's lifetime to every transfer. A cancelled one
	// left bound would kill every later transfer in the binary.
	format.ResetShutdownForTesting()

	resetDailyLimitForTesting()
}
