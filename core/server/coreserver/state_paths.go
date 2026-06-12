package coreserver

import (
	"os"
	"path/filepath"
)

// resolveStateDir returns the root under which mutable runtime state lives
// (pb_data, releases, builds). It is intentionally SEPARATE from
// resolveServerDir() (the binary's own dir): the binary's dir is swapped
// atomically per build via the `current` symlink, but state must persist
// across swaps, so it lives outside the swapped subtree.
//
// TINYCLD_STATE_DIR pins it (the production layout sets it to /workspace).
// When unset it defaults to resolveServerDir() so deployments that still keep
// state under the binary's dir (pre-relocation) keep working unchanged.
func resolveStateDir() string {
	if d := os.Getenv("TINYCLD_STATE_DIR"); d != "" {
		return d
	}
	return resolveServerDir()
}

func statePbDataDir() string   { return filepath.Join(resolveStateDir(), "pb_data") }
func stateReleasesDir() string { return filepath.Join(resolveStateDir(), "releases") }
func stateBuildsDir() string   { return filepath.Join(resolveStateDir(), "builds") }
