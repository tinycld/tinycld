package coreserver

import (
	"os"
	"path/filepath"
	"strings"
)

const restartExitCode = 75

// requestRestart signals the entrypoint wrapper to restart the server process.
// In production (Docker/Dokku), the entrypoint.sh script watches for exit code 75
// and restarts the server. In development (go run), we just log a message.
//
// The legacy serverDir parameter is retained for caller compatibility; the
// restart marker now lives under the STATE dir (resolveStateDir()) so it
// persists across the per-build symlink swap rather than in the swapped dir.
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

func requestRestart(_ string) {
	if isDevelopment() {
		srvLog.Info("restart requested (dev mode — restart manually)")
		return
	}

	// Write a restart marker so the entrypoint knows this was intentional
	markerPath := restartMarkerPath()
	if err := os.WriteFile(markerPath, []byte("restart"), 0o644); err != nil {
		srvLog.Warn("failed to write restart marker", "path", markerPath, "err", err)
	}

	srvLog.Info("requesting restart via exit code 75")
	os.Exit(restartExitCode)
}

// isDevelopment returns true when running via `go run` (temp dir binary).
func isDevelopment() bool {
	ex, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.HasPrefix(ex, os.TempDir())
}
