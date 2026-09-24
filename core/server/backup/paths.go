package backup

import (
	"path/filepath"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
)

// A restore keeps every byte it stages outside pb_data: the live database is
// never touched, so a restore that fails, or a process killed halfway through
// one, leaves the organization exactly as it was.
func restoreDir(app core.App) string            { return filepath.Join(LedgerPath(app), "restore") }
func pendingDir(app core.App, id string) string { return filepath.Join(restoreDir(app), "pending", id) }
func preBackupPath(app core.App, id string) string {
	return filepath.Join(restoreDir(app), "pre", id+".age")
}
func previousDir(app core.App, id string) string {
	return filepath.Join(restoreDir(app), "previous", id)
}
func armedPath(app core.App) string   { return filepath.Join(restoreDir(app), "armed") }
func swappedPath(app core.App) string { return filepath.Join(restoreDir(app), "swapped") }

// The boot swap runs before an app exists, so it derives the same paths from the
// data dir alone. LedgerPath is the parent of pb_data in every deployment shape.
func restoreDirOf(dataDir string) string  { return filepath.Join(filepath.Dir(dataDir), "restore") }
func armedPathOf(dataDir string) string   { return filepath.Join(restoreDirOf(dataDir), "armed") }
func swappedPathOf(dataDir string) string { return filepath.Join(restoreDirOf(dataDir), "swapped") }

// armed is the marker a restore leaves behind for the process that boots next.
// It carries the manifest because that process finalizes the ledger row and has
// no other way to know what it is now running.
type armed struct {
	ID       string          `json:"id"`
	Pending  string          `json:"pending"`
	Pre      string          `json:"pre"`
	Manifest format.Manifest `json:"manifest"`
}
