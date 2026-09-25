package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	sqlite "modernc.org/sqlite"
)

// sqliteLimitAttached is SQLITE_LIMIT_ATTACHED, the number of databases a
// connection may have ATTACHed. It is restated here rather than imported
// because core does not export it, and the constant is the C API's.
//
// https://www.sqlite.org/c3ref/c_limit_attached.html
const sqliteLimitAttached = 7

// vacuumInto writes a consistent copy of the live database through the open
// connection. It does not shell out: the sqlite3 CLI is absent from a single
// binary and from a confined process whose PATH is /usr/bin:/bin.
//
// VACUUM INTO cannot run inside a transaction, so the statement goes to the
// underlying *sql.DB rather than through dbx's query builder — dbx wraps a
// builder Execute on the writer connection and SQLite rejects the statement
// with "cannot VACUUM from within a transaction".
//
// It runs on ONE borrowed connection rather than on the pool, because a HOSTED
// tenant's connections cannot ATTACH and VACUUM INTO needs to — see
// withAttachSlot.
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

	ctx := context.Background()
	conn, err := db.DB().Conn(ctx)
	if err != nil {
		return fmt.Errorf("backup: snapshot could not take the writer connection: %w", err)
	}
	// Closed, not just released: see withAttachSlot's last paragraph.
	defer conn.Close()

	return withAttachSlot(ctx, conn, func() error {
		if _, err := conn.ExecContext(ctx, "VACUUM INTO '"+dest+"'"); err != nil {
			return fmt.Errorf("backup: snapshot: %w", err)
		}
		return nil
	})
}

// withAttachSlot runs fn with ONE attach slot available on conn, restoring the
// connection's previous limit afterwards.
//
// This exists because two correct requirements collide. A hosted tenant is
// opened with core.NoAttachDBConnect, which sets SQLITE_LIMIT_ATTACHED to 0 on
// every pooled connection: $app hands sandboxed package JS raw SQL, and an
// ATTACH against an absolute path would be a read and write primitive for every
// other tenant's database and the control plane's. And SQLite implements VACUUM
// INTO by attaching the destination internally, so on such a connection the
// statement fails outright — "too many attached databases - max 0 (1)". Before
// this, every backup on a hosted tenant failed, and only there: the engine's own
// tests and every single-tenant deployment run with the default limit.
//
// Raising the limit to exactly 1, on one connection, for the duration of one
// statement is the narrowest opening that lets the snapshot run. It is not a
// hole in the restriction that matters: the window is this function's body,
// which executes one VACUUM INTO and no user SQL of any kind, so there is no
// point inside it at which package JS could issue an ATTACH.
//
// The limit is restored even on failure, and the caller CLOSES the connection
// rather than returning it to the pool — belt and braces, because the pool
// reuses a connection indefinitely and a restore that silently failed would
// leave one permissive connection circulating forever. Closing it means the
// pool replaces it with a freshly primed one instead.
func withAttachSlot(ctx context.Context, conn *sql.Conn, fn func() error) (err error) {
	prev, limitErr := sqlite.Limit(conn, sqliteLimitAttached, 1)
	if limitErr != nil {
		// A dead connection cannot run the snapshot either, so say so rather
		// than letting VACUUM INTO report the same thing less clearly.
		if errors.Is(limitErr, sql.ErrConnDone) {
			return fmt.Errorf("backup: snapshot lost its connection: %w", limitErr)
		}
		// Any other failure to READ the limit is not a reason to skip the
		// snapshot: a deployment that never restricted ATTACH runs at SQLite's
		// default of 10 and the statement below works regardless. Try it and
		// let its own error speak.
		return fn()
	}
	defer func() {
		// Restoring the PREVIOUS value, not zero: a single-tenant deployment
		// runs at the default, and pinning it to 0 here would take ATTACH away
		// from a deployment that never restricted it.
		//
		// A failed restore is surfaced rather than logged and dropped, but it
		// must not mask fn's own error — that is the one an operator needs.
		// Either way the caller closes the connection, so a connection stuck
		// permissive never returns to the pool.
		if _, rerr := sqlite.Limit(conn, sqliteLimitAttached, prev); rerr != nil && err == nil {
			err = fmt.Errorf("backup: snapshot could not restore the ATTACH limit: %w", rerr)
		}
	}()

	return fn()
}
