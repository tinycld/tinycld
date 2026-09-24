package backup

import (
	"fmt"
	"os"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// vacuumInto writes a consistent copy of the live database through the open
// connection. It does not shell out: the sqlite3 CLI is absent from a single
// binary and from a confined process whose PATH is /usr/bin:/bin.
//
// VACUUM INTO cannot run inside a transaction, so the statement goes to the
// underlying *sql.DB rather than through dbx's query builder — dbx wraps a
// builder Execute on the writer connection and SQLite rejects the statement
// with "cannot VACUUM from within a transaction".
func vacuumInto(app core.App, dest string) error {
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(dest, "'") {
		return fmt.Errorf("backup: snapshot path must not contain a quote")
	}
	db, ok := app.NonconcurrentDB().(*dbx.DB)
	if !ok {
		return fmt.Errorf("backup: snapshot needs the writer connection, got %T", app.NonconcurrentDB())
	}
	if _, err := db.DB().Exec("VACUUM INTO '" + dest + "'"); err != nil {
		return fmt.Errorf("backup: snapshot: %w", err)
	}
	return nil
}
