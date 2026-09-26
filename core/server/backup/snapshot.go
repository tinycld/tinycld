package backup

import (
	"context"
	"database/sql"
	"database/sql/driver"
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
	// An ordinary release. Conn.Close returns the driver connection to the pool
	// — it does not destroy it — so this is not part of the ATTACH guard; see
	// withAttachSlot, which owns that.
	defer conn.Close()

	return withAttachSlot(ctx, db.DB(), conn, func() error {
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
// # What keeps the opening from leaking
//
// The deferred restore is the guard, and it is the ONLY guard. Releasing the
// connection is not one: database/sql's Conn.Close returns the driver connection
// to the pool rather than destroying it, so a connection left at ATTACH=1 would
// go straight back into circulation and the next piece of package JS to borrow it
// could ATTACH any file on disk. (An earlier version of this comment claimed the
// close discarded the connection and that the pool would replace it with a
// freshly primed one. Both were wrong: the close releases, and
// NoAttachDBConnect primes only at open — a lazily opened replacement would be
// UNPRIMED, so relying on that would have been worse than relying on nothing.)
//
// So a failed restore is handled rather than merely reported. The driver
// connection is discarded by returning driver.ErrBadConn from Raw, which is the
// one documented way to tell database/sql not to reuse it, and then
// ReapplyNoAttachLimits re-primes the pool so the connection opened in its place
// carries the restriction. Without the second half the discard would trade one
// permissive connection for one unrestricted one.
//
// ReapplyNoAttachLimits is a no-op on a pool that was never restricted, so a
// single-tenant deployment pays nothing for this path.
func withAttachSlot(ctx context.Context, sqlDB *sql.DB, conn *sql.Conn, fn func() error) (err error) {
	prev, limitErr := setAttachLimit(conn, 1)
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
		_, rerr := setAttachLimit(conn, prev)
		if rerr == nil {
			return
		}
		// The restore failed, so this connection is still permissive. Get it out
		// of the pool and re-prime what replaces it — see the doc comment.
		discardErr := discardAndReprime(sqlDB, conn)

		// A failed restore is surfaced, but it must not mask fn's own error: that
		// is the one an operator needs, and this one is about the pool.
		if err == nil {
			err = fmt.Errorf("backup: snapshot could not restore the ATTACH limit: %w", rerr)
			if discardErr != nil {
				err = errors.Join(err, discardErr)
			}
			return
		}
		// fn already failed, so the pool's state has nowhere to be returned. It
		// is logged rather than dropped: a discard that ITSELF failed means a
		// connection may still be circulating at ATTACH=1, which is a security
		// fact an operator has to be able to find, and it would otherwise be
		// invisible behind whatever fn complained about.
		log.Error("a snapshot could not restore the ATTACH limit on its connection",
			"err", rerr, "discard", discardErr, "cause", err)
	}()

	return fn()
}

// setAttachLimit is the restore call, indirected so a test can make it FAIL.
//
// The discard-and-reprime path below only runs when the restore fails, and that
// failure cannot be provoked from outside — a live connection's limit always sets.
// Left untestable, the branch that keeps a permissive connection out of the pool
// would be the one piece of this guard nothing ever exercised.
var setAttachLimit = func(conn *sql.Conn, limit int) (int, error) {
	return sqlite.Limit(conn, sqliteLimitAttached, limit)
}

// discardAndReprime takes one permissive connection out of circulation and makes
// sure the pool's replacement for it is restricted again.
//
// Returning driver.ErrBadConn from Raw is the documented way to tell
// database/sql that a driver connection must not be reused: the subsequent
// Close then destroys it instead of releasing it. On its own that is only half
// the job — the pool opens a replacement lazily, and NoAttachDBConnect primes
// only at open time, so the replacement would carry SQLite's default limit and
// package JS could ATTACH. ReapplyNoAttachLimits re-primes every connection in
// the pool, and is a no-op on a pool that was never restricted.
func discardAndReprime(sqlDB *sql.DB, conn *sql.Conn) error {
	var errs []error
	// The error is expected and is the mechanism, not a fault: Raw returns
	// whatever the callback returned. Anything ELSE is worth reporting, because
	// it means the connection was not marked bad and may be reused permissive.
	if rawErr := conn.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(rawErr, driver.ErrBadConn) {
		errs = append(errs, fmt.Errorf(
			"backup: snapshot could not discard a connection whose ATTACH limit it failed to restore: %w", rawErr))
	}
	if reErr := core.ReapplyNoAttachLimits(sqlDB); reErr != nil {
		errs = append(errs, fmt.Errorf("backup: snapshot could not re-restrict the connection pool: %w", reErr))
	}
	return errors.Join(errs...)
}
