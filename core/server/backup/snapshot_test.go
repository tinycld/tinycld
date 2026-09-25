package backup

import (
	"os"
	"path/filepath"
	"testing"

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
