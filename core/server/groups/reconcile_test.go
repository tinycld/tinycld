package groups

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// Reconcile must repair both directions: a missing derived row is inserted, a
// stale derived row (no grant, or user no longer a member) is deleted, and a
// clean state is left alone.
func TestReconcileRepairsBothDirections(t *testing.T) {
	app := newZooApp(t)
	alice, bob := zooUser(t, app, "alice@x.test"), zooUser(t, app, "bob@x.test")
	g := zooGroup(t, app, "keepers")
	zooMember(t, app, g, alice)
	grant := zooGrant(t, app, "bronx", g, "viewer")

	// Simulate a crash between hook and commit: delete alice's derived row
	// with hooks bypassed, and forge a stale row for bob who is not a member.
	rows, err := app.FindRecordsByFilter("zoo_keepers", `user = {:u}`, "", 0, 0, map[string]any{"u": alice.Id})
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected alice's derived row, got %d rows (err %v)", len(rows), err)
	}
	if _, err := app.DB().NewQuery("DELETE FROM zoo_keepers WHERE id = {:id}").Bind(map[string]any{"id": rows[0].Id}).Execute(); err != nil {
		t.Fatal(err)
	}
	col, _ := app.FindCollectionByNameOrId("zoo_keepers")
	stale := core.NewRecord(col)
	stale.Set("zoo", "bronx")
	stale.Set("group", g.Id)
	stale.Set("user", bob.Id)
	stale.Set("role", "viewer")
	if err := app.Save(stale); err != nil {
		t.Fatal(err)
	}
	// Role drift on the grant must also be repaired.
	if _, err := app.DB().NewQuery("UPDATE zoo_keepers SET role = 'editor' WHERE id = {:id}").Bind(map[string]any{"id": grant.Id}).Execute(); err != nil {
		t.Fatal(err)
	}

	if err := Reconcile(app); err != nil {
		t.Fatal(err)
	}
	got := derivedRows(t, app, "bronx")
	if len(got) != 1 || got[alice.Id] != "editor" {
		t.Fatalf("after reconcile derived = %v, want only alice as editor", got)
	}

	// Idempotent: a second pass changes nothing.
	if err := Reconcile(app); err != nil {
		t.Fatal(err)
	}
	if again := derivedRows(t, app, "bronx"); len(again) != 1 || again[alice.Id] != "editor" {
		t.Fatalf("second reconcile changed state: %v", again)
	}
}
