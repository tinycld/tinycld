package coreserver

import (
	"log"
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
func requestRestart(_ string) {
	if isDevelopment() {
		log.Println("pkg_install: restart requested (dev mode — restart manually)")
		return
	}

	// Write a restart marker so the entrypoint knows this was intentional
	markerPath := filepath.Join(statePbDataDir(), ".restart-requested")
	if err := os.WriteFile(markerPath, []byte("restart"), 0o644); err != nil {
		log.Printf("pkg_install: failed to write restart marker: %v", err)
	}

	log.Println("pkg_install: requesting restart via exit code 75")
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
