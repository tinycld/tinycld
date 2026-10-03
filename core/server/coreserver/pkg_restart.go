package coreserver

import (
	"os"
	"path/filepath"
	"strings"

	"tinycld.org/core/listeners"
	"tinycld.org/core/readonly"
)

const restartExitCode = 75

// restartMarkerPath is where requestRestart records that a restart was asked
// for. It lives under the STATE dir (resolveStateDir()) so it persists across
// the per-build symlink swap rather than in the swapped dir.
//
// It sits BESIDE pb_data, not inside it. A restore's boot swap renames pb_data
// away as a whole, and a restart requested to apply that restore is exactly when
// the marker is written — inside pb_data it would be carried off with the data
// the swap sets aside. Nothing reads the marker today (the entrypoint keys on
// exit 75 alone), so this is a record of intent rather than a protocol, but its
// location must not depend on which restart it is.
func restartMarkerPath() string {
	return filepath.Join(resolveStateDir(), ".restart-requested")
}

// exitProcess is os.Exit behind a seam, so a test can watch an unsupervised
// restart without ending the test binary.
var exitProcess = os.Exit

// requestRestart asks for this process to be replaced by one running the
// activated build, and reports whether that is under way.
//
// Without a supervisor it exits 75 and does not return. Under a supervisor it
// sends a restart message and returns true: this process keeps serving, read-
// only, until the supervisor drains it. cold asks the supervisor to stop this
// process before it starts the next one, for a restart whose new process must
// not run beside this one. In dev mode nothing restarts, and it returns false.
func requestRestart(cold bool) (underway bool) {
	if isDevelopment() {
		srvLog.Info("restart requested (dev mode — restart manually)")
		return false
	}

	// Write a restart marker so the entrypoint knows this was intentional
	markerPath := restartMarkerPath()
	if err := os.WriteFile(markerPath, []byte("restart"), 0o644); err != nil {
		srvLog.Warn("failed to write restart marker", "path", markerPath, "err", err)
	}

	if listeners.Supervised() {
		// The next process may migrate the database while this one still
		// serves, so this one must not write from here on.
		readonly.Enter()
		if askSupervisorToRestart(cold) {
			return true
		}
	}

	srvLog.Info("requesting restart via exit code 75")
	exitProcess(restartExitCode)
	return true
}

// isDevelopment returns true when running via `go run` (temp dir binary).
// It is a variable so a test, whose binary also runs from the temp dir, can
// reach the restart paths a production binary takes.
var isDevelopment = func() bool {
	ex, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.HasPrefix(ex, os.TempDir())
}
