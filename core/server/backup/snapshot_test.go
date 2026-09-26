package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newNoAttachApp boots an app whose connections cannot ATTACH — the way a
// HOSTED tenant is opened.
//
// core.NoAttachDBConnect sets SQLITE_LIMIT_ATTACHED to 0 on every connection in
// the pool, because $app exposes raw SQL to sandboxed JS and an ATTACH against
// an absolute path is a read/write primitive for every other tenant's database.
// hosting/tenantmain passes it for both the tenant app and its aux app, so this
// is the configuration every hosted backup actually runs under — and the one
// the snapshot has to work under.
func newNoAttachApp(t *testing.T) *tests.TestApp {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "noattach")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{
		DataDir:       dir,
		EncryptionEnv: "pb_test_env",
		DBConnect:     core.NoAttachDBConnect,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	return app
}

// A snapshot must work on a tenant whose ATTACH is restricted.
//
// This is the regression test for a bug that made EVERY hosted backup fail:
// SQLite implements VACUUM INTO by attaching the destination internally, so a
// connection with SQLITE_LIMIT_ATTACHED = 0 rejects the statement with "too
// many attached databases - max 0 (1)". The two requirements are both correct
// and were simply never exercised together — the engine's own tests open a
// default app, and the limit is set only by the hosted tenant.
func TestVacuumInto_WorksWithAttachRestricted(t *testing.T) {
	app := newNoAttachApp(t)

	// The restriction has to actually be in force, or this test would pass by
	// testing nothing. ATTACH must be refused before the snapshot is tried.
	if _, err := app.NonconcurrentDB().NewQuery("ATTACH DATABASE ':memory:' AS probe").Execute(); err == nil {
		t.Fatal("ATTACH is permitted on this app; the test is not exercising the restricted configuration")
	}

	dest := filepath.Join(t.TempDir(), "snap.db")
	if err := vacuumInto(app, dest); err != nil {
		t.Fatalf("vacuumInto under SQLITE_LIMIT_ATTACHED = 0: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("the snapshot was not written: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("the snapshot is empty")
	}
}

// The snapshot must LEAVE the restriction in place. Raising the limit for the
// duration of one VACUUM INTO is the fix; leaving it raised would hand every
// subsequent query on that connection the cross-tenant ATTACH primitive the
// limit exists to remove — and a pooled connection is reused indefinitely.
func TestVacuumInto_LeavesAttachRestricted(t *testing.T) {
	app := newNoAttachApp(t)

	if err := vacuumInto(app, filepath.Join(t.TempDir(), "snap.db")); err != nil {
		t.Fatalf("vacuumInto: %v", err)
	}

	// Every connection in the pool, not just one: the snapshot borrows a
	// connection from the pool and gives it back, so a restriction restored on
	// the wrong connection would leave the borrowed one permissive and the
	// probe would only find it by chance. Looping past the pool's width makes
	// the check exhaustive in practice.
	for i := 0; i < 32; i++ {
		if _, err := app.NonconcurrentDB().NewQuery("ATTACH DATABASE ':memory:' AS probe").Execute(); err == nil {
			_, _ = app.NonconcurrentDB().NewQuery("DETACH DATABASE probe").Execute()
			t.Fatalf("ATTACH is permitted after a snapshot (attempt %d); untrusted JS could now read every other tenant's database", i)
		}
	}
}

// A default app has no restriction to work around, and the snapshot must still
// work — this is the single-tenant and docker path.
func TestVacuumInto_WorksWithoutTheRestriction(t *testing.T) {
	app := newTestApp(t)

	dest := filepath.Join(t.TempDir(), "snap.db")
	if err := vacuumInto(app, dest); err != nil {
		t.Fatalf("vacuumInto on a default app: %v", err)
	}
	if info, err := os.Stat(dest); err != nil || info.Size() == 0 {
		t.Fatalf("the snapshot was not written: %v", err)
	}
}

// A restore that FAILS must not leave a permissive connection in the pool.
//
// The deferred restore is the whole ATTACH guard, and the release is not part of
// it: database/sql's Conn.Close returns the driver connection to the pool rather
// than destroying it. So if the restore fails, the connection that just ran
// VACUUM INTO at ATTACH=1 goes straight back into circulation, and the next piece
// of package JS to borrow it can ATTACH any file on disk — every other tenant's
// database and the control plane's.
//
// This is the only test that can reach that branch: a live connection's limit
// always sets, so the failure is injected through setAttachLimit. Without the
// injection the discard-and-reprime path would be the one piece of the guard
// nothing ever exercised.
//
// Both halves are asserted, because either alone is insufficient. The discard
// alone trades a permissive connection for an UNPRIMED one, since
// NoAttachDBConnect primes only at open and the pool's replacement is opened
// lazily; the reprime alone leaves the permissive connection in the pool.
func TestVacuumInto_FailedRestoreLeavesNoPermissiveConnection(t *testing.T) {
	app := newNoAttachApp(t)

	// Fail only the RESTORE. The first call raises the limit so VACUUM INTO can
	// run — the snapshot has to get as far as needing the restore, or this test
	// asserts nothing — and every later call reports failure.
	real := setAttachLimit
	var calls int
	setAttachLimit = func(conn *sql.Conn, limit int) (int, error) {
		calls++
		if calls == 1 {
			return real(conn, limit)
		}
		return 0, errors.New("injected: the ATTACH limit could not be restored")
	}
	t.Cleanup(func() { setAttachLimit = real })

	err := vacuumInto(app, filepath.Join(t.TempDir(), "snap.db"))
	if err == nil {
		t.Fatal("a failed restore must be surfaced, not swallowed: an operator has to learn the pool was disturbed")
	}
	if !strings.Contains(err.Error(), "restore the ATTACH limit") {
		t.Fatalf("error = %v, want it to name the failed restore", err)
	}
	if calls < 2 {
		t.Fatalf("setAttachLimit called %d times; the snapshot never reached the restore, so the branch under test did not run", calls)
	}

	// The pool must be restricted again — every connection in it, including the
	// one the pool opened LAZILY to replace the discarded one. Sequential probes
	// cannot prove that: the pool keeps handing back the same primed idle
	// connection and never reaches the replacement, so a missing reprime stays
	// green. Holding the pool's full width at once forces every connection,
	// old and new, to be checked out and probed.
	sqlDB := app.NonconcurrentDB().(*dbx.DB).DB()
	var held []*sql.Conn
	defer func() {
		for _, c := range held {
			_ = c.Close()
		}
	}()
	for i := 0; i < noAttachPoolWidth; i++ {
		conn, cerr := sqlDB.Conn(context.Background())
		if cerr != nil {
			t.Fatalf("checking out connection %d: %v", i, cerr)
		}
		held = append(held, conn)
		if _, perr := conn.ExecContext(context.Background(), "ATTACH DATABASE ':memory:' AS probe"); perr == nil {
			_, _ = conn.ExecContext(context.Background(), "DETACH DATABASE probe")
			t.Fatalf("connection %d permits ATTACH after a failed restore; either the permissive connection went back into the pool or its replacement was never primed", i)
		}
	}
}

// noAttachPoolWidth mirrors core.NoAttachDBConnect's SetMaxOpenConns: holding
// this many connections at once is what makes the probe above exhaustive.
const noAttachPoolWidth = 24
