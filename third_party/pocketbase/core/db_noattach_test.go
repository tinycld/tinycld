//go:build !no_default_driver

package core

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A tenant's JS runs with $app bound, and $app reaches raw SQL through
// db()/nonconcurrentDB()/concurrentDB()/auxDB(). Withholding the $os/$filesystem
// bindings therefore does not remove the tenant's file access: ATTACH DATABASE
// against an absolute path is a read/write primitive for anything the process
// user can reach, including a sibling tenant's data.db.
//
// These tests pin the connector that closes it.

func TestNoAttachDBConnect_BlocksAttach(t *testing.T) {
	dir := t.TempDir()

	victim := filepath.Join(dir, "victim.db")
	vdb, err := DefaultDBConnect(victim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vdb.NewQuery("CREATE TABLE secrets (v TEXT)").Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := vdb.NewQuery("INSERT INTO secrets VALUES ('OTHER-TENANT-SECRET')").Execute(); err != nil {
		t.Fatal(err)
	}
	vdb.Close()

	db, err := NoAttachDBConnect(filepath.Join(dir, "attacker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.NewQuery("CREATE TABLE t (x INT)").Execute(); err != nil {
		t.Fatalf("ordinary DDL must still work: %v", err)
	}

	// The exploit, verbatim.
	_, err = db.NewQuery("ATTACH DATABASE '" + victim + "' AS stolen").Execute()
	if err == nil {
		t.Fatal("ATTACH DATABASE succeeded: a sandboxed tenant can read every " +
			"other tenant's database through $app's raw-SQL surface")
	}
	if !strings.Contains(err.Error(), "too many attached") {
		t.Logf("ATTACH blocked with an unexpected error (still blocked): %v", err)
	}
}

// The limit is per-connection, so it has to hold on every connection the pool
// opens — not just the first. A tenant that keeps issuing queries until the
// pool grows must not find an unrestricted connection waiting.
func TestNoAttachDBConnect_HoldsAcrossPooledConnections(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim.db")
	vdb, err := DefaultDBConnect(victim)
	if err != nil {
		t.Fatal(err)
	}
	vdb.NewQuery("CREATE TABLE s (v TEXT)").Execute()
	vdb.Close()

	db, err := NoAttachDBConnect(filepath.Join(dir, "attacker.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.NewQuery("CREATE TABLE t (x INT)").Execute(); err != nil {
		t.Fatal(err)
	}

	// Force several distinct connections to exist concurrently, then try the
	// attack on each.
	var conns []*sql.Conn
	for i := 0; i < 5; i++ {
		c, err := db.DB().Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		_, err := c.ExecContext(t.Context(), "ATTACH DATABASE '"+victim+"' AS stolen")
		if err == nil {
			t.Fatalf("connection %d permitted ATTACH: the limit is not applied to "+
				"every pooled connection", i)
		}
		c.Close()
	}

	// The interesting case: connections that have been released and re-acquired.
	// database/sql discards idle connections on its own schedule and opens fresh
	// ones on demand, so priming at open time only holds if the pool is pinned —
	// without that, an unrestricted connection surfaces a few acquisitions later.
	for i := 0; i < 20; i++ {
		c, err := db.DB().Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.ExecContext(t.Context(), "ATTACH DATABASE '"+victim+"' AS stolen")
		if err == nil {
			_, _ = c.ExecContext(t.Context(), "DETACH DATABASE stolen")
			c.Close()
			t.Fatalf("acquisition %d got an unprimed connection: the pool replaced "+
				"a primed connection with a fresh unrestricted one", i)
		}
		c.Close()
	}
}

// The restriction must not leak onto databases that did not ask for it — the
// control plane and any non-tenant app keep the stock connector.
func TestDefaultDBConnect_StillAllowsAttach(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "other.db")
	odb, err := DefaultDBConnect(other)
	if err != nil {
		t.Fatal(err)
	}
	odb.NewQuery("CREATE TABLE x (v TEXT)").Execute()
	odb.Close()

	db, err := DefaultDBConnect(filepath.Join(dir, "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.NewQuery("ATTACH DATABASE '" + other + "' AS o").Execute(); err != nil {
		t.Fatalf("the default connector must keep stock behaviour, got: %v", err)
	}
}

// A path containing the marker as ordinary text must not silently disable the
// restriction, and must not be mistaken for an opt-in either.
func TestNoAttachDBConnect_PathWithQueryLikeText(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, "weird name.db")

	db, err := NoAttachDBConnect(odd)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.NewQuery("CREATE TABLE t (x INT)").Execute(); err != nil {
		t.Fatalf("connector failed on a path with a space: %v", err)
	}
	if _, err := os.Stat(odd); err != nil {
		t.Fatalf("database was not created at the requested path: %v", err)
	}

	victim := filepath.Join(dir, "victim.db")
	if _, err := db.NewQuery("ATTACH DATABASE '" + victim + "' AS v").Execute(); err == nil {
		t.Fatal("ATTACH permitted on a path containing a space")
	}
}
