//go:build !no_default_driver

package core

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pocketbase/dbx"
	sqlite "modernc.org/sqlite"
)

// sqliteLimitAttached is SQLITE_LIMIT_ATTACHED, the maximum number of databases
// that may be ATTACHed to a connection. Zero makes ATTACH fail outright.
//
// https://www.sqlite.org/c3ref/c_limit_attached.html
const sqliteLimitAttached = 7

// noAttachMaxConns bounds the connection pool so every connection can be
// primed up front. SQLITE_LIMIT_ATTACHED is a property of a connection, not of
// a database, and database/sql opens connections lazily — so the only way to
// guarantee no unrestricted connection is ever handed out is to cap the pool
// and prime the whole cap before the pool is used.
//
// PocketBase already runs its non-concurrent DB at 1 connection and its
// concurrent DB at a small multiple; this cap sits above normal tenant load.
const noAttachMaxConns = 24

// NoAttachDBConnect opens a database whose connections cannot ATTACH another
// file. Use it for any app that executes untrusted JS.
//
// Withholding the $os/$filesystem/$http bindings from a sandboxed VM does not
// take away its file access, because $app stays bound and $app exposes raw SQL
// (db, nonconcurrentDB, concurrentDB, auxDB, runInTransaction). ATTACH DATABASE
// against an absolute path is then a read and write primitive for anything the
// process user can reach — in a multi-tenant deployment, every other tenant's
// data.db and the control plane's.
//
// A host may also separate tenants by uid, and should. That is a second line
// rather than a substitute: it is absent on developer machines, absent when the
// router runs unprivileged, and it fails outright for any pair of tenants that
// end up sharing a uid.
func NoAttachDBConnect(dbPath string) (*dbx.DB, error) {
	db, err := DefaultDBConnect(dbPath)
	if err != nil {
		return nil, err
	}

	if err := restrictAttach(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// restrictAttach caps the pool and sets SQLITE_LIMIT_ATTACHED to 0 on every
// connection in it.
//
// Two pool properties make this sound, and both are load-bearing:
//
//   - Connections are held open simultaneously while being primed. Releasing
//     each before opening the next would let database/sql hand the same
//     connection back repeatedly, priming one connection N times and leaving
//     the rest untouched.
//   - The idle pool is pinned so a primed connection is never discarded.
//     database/sql closes idle connections on its own schedule and opens fresh,
//     unprimed ones on demand; with the defaults, priming N connections and
//     releasing them leaves only some primed, and the pool serves an
//     unrestricted connection a few acquisitions later. Pinning is what makes
//     "primed once at open" mean "primed for the life of the pool".
//
// The limit itself persists across a connection's return to and reuse from the
// pool, so no re-priming is needed.
func restrictAttach(db *dbx.DB) error {
	sqlDB := db.DB()
	sqlDB.SetMaxOpenConns(noAttachMaxConns)
	sqlDB.SetMaxIdleConns(noAttachMaxConns)
	sqlDB.SetConnMaxIdleTime(0)
	sqlDB.SetConnMaxLifetime(0)

	ctx := context.Background()
	conns := make([]*sql.Conn, 0, noAttachMaxConns)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for i := 0; i < noAttachMaxConns; i++ {
		c, err := sqlDB.Conn(ctx)
		if err != nil {
			return fmt.Errorf("open connection %d to restrict ATTACH: %w", i, err)
		}
		conns = append(conns, c)
		if _, err := sqlite.Limit(c, sqliteLimitAttached, 0); err != nil {
			return fmt.Errorf("set SQLITE_LIMIT_ATTACHED on connection %d: %w", i, err)
		}
	}

	// Prove it took effect rather than trusting the call: a driver change that
	// silently stopped honouring the limit would otherwise reopen the hole with
	// every test still green.
	if err := assertAttachBlocked(ctx, conns[0]); err != nil {
		return err
	}
	return nil
}

// assertAttachBlocked verifies the restriction on a live connection. It targets
// a path that does not exist: if ATTACH is refused we get the limit error, and
// if it is permitted SQLite happily creates the file — which is itself the
// failure, so either outcome is unambiguous.
func assertAttachBlocked(ctx context.Context, c *sql.Conn) error {
	_, err := c.ExecContext(ctx, "ATTACH DATABASE ':memory:' AS tinycld_attach_probe")
	if err == nil {
		_, _ = c.ExecContext(ctx, "DETACH DATABASE tinycld_attach_probe")
		return fmt.Errorf("ATTACH DATABASE is still permitted after setting " +
			"SQLITE_LIMIT_ATTACHED: untrusted JS could read every other tenant's database")
	}
	return nil
}
