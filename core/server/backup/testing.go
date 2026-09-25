package backup

import "tinycld.org/core/backup/format"

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

	rebuilderMu.Lock()
	rebuilder = nil
	rebuilderMu.Unlock()

	restartMu.Lock()
	restartFn = func() {}
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
