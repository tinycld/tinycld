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

// stateRollbackRecordPath mirrors supervise's State.rollbackRecordPath: the
// JSON record ({"build", "rolled_to", "at"}) the supervisor leaves when it
// rolls a failed build back. It is beside pb_data, not in it, so a restore
// swap that moves pb_data aside cannot carry it away.
func stateRollbackRecordPath() string { return filepath.Join(resolveStateDir(), ".rollback-pending") }

// stateUnrestoredDir mirrors supervise's State.unrestoredDir: one dir per
// build whose backup a rollback could not restore,
// <build>/{data.db,unrestored.json}. Only an operator removes it.
func stateUnrestoredDir() string { return filepath.Join(resolveStateDir(), "unrestored") }
