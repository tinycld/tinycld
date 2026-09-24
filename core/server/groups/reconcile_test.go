package groups

import (
	"errors"
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

// TestReconcileReportsErrorsButFinishesOtherTables proves reconcileTable
// keeps going past a bad grant instead of aborting the table, and Reconcile
// keeps going past a bad table instead of aborting the run — but the failure
// still surfaces in the return value rather than being swallowed. A table
// whose expansion errors for one grant must not stop a different, healthy
// table from being fully repaired in the same call.
func TestReconcileReportsErrorsButFinishesOtherTables(t *testing.T) {
	app := newZooApp(t)

	// A second grant table, same shape as zoo_keepers, registered the same
	// way a package would from its own Register(app).
	users, _ := app.FindCollectionByNameOrId("users")
	groupsCol, _ := app.FindCollectionByNameOrId("groups")
	vets := core.NewBaseCollection("zoo_vets")
	vets.Fields.Add(&core.TextField{Name: "zoo", Required: true})
	vets.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, CascadeDelete: true, MaxSelect: 1})
	vets.Fields.Add(&core.RelationField{Name: "group", CollectionId: groupsCol.Id, CascadeDelete: true, MaxSelect: 1})
	vets.Fields.Add(&core.SelectField{Name: "role", Required: true, MaxSelect: 1, Values: []string{"owner", "editor", "viewer"}})
	vets.AddIndex("idx_zv_unique", true, "`zoo`, `user`, `group`", "")
	if err := app.Save(vets); err != nil {
		t.Fatalf("save zoo_vets: %v", err)
	}
	RegisterGrantTable(GrantTable{Collection: "zoo_vets", ResourceField: "zoo"})

	// zoo_keepers: a healthy table with a repairable missing row.
	alice := zooUser(t, app, "alice@x.test")
	g := zooGroup(t, app, "keepers")
	zooMember(t, app, g, alice)
	zooGrant(t, app, "bronx", g, "viewer")
	rows, err := app.FindRecordsByFilter("zoo_keepers", `user = {:u}`, "", 0, 0, map[string]any{"u": alice.Id})
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected alice's zoo_keepers row, got %d rows (err %v)", len(rows), err)
	}
	if _, err := app.DB().NewQuery("DELETE FROM zoo_keepers WHERE id = {:id}").Bind(map[string]any{"id": rows[0].Id}).Execute(); err != nil {
		t.Fatal(err)
	}

	// zoo_vets: a grant whose expansion will fail every time reconcile tries
	// to (re-)insert its derived row.
	bob := zooUser(t, app, "bob@x.test")
	g2 := zooGroup(t, app, "vets")
	zooMember(t, app, g2, bob)
	vetsCol, _ := app.FindCollectionByNameOrId("zoo_vets")
	grant := core.NewRecord(vetsCol)
	grant.Set("zoo", "sd")
	grant.Set("group", g2.Id)
	grant.Set("role", "viewer")
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	// The grant's own creation goes through fine (isGrant, not isDerived);
	// only the derived-row insert that reconcile drives must fail, so the
	// handler only errors on rows carrying both user and group.
	app.OnRecordCreate("zoo_vets").BindFunc(func(e *core.RecordEvent) error {
		if isDerived(e.Record) {
			return errors.New("simulated derived-row insert failure")
		}
		return e.Next()
	})

	if err := Reconcile(app); err == nil {
		t.Fatal("expected Reconcile to report the zoo_vets expansion failure")
	}

	// zoo_keepers must still have been fully repaired despite zoo_vets failing.
	if got := derivedRows(t, app, "bronx"); len(got) != 1 || got[alice.Id] != "viewer" {
		t.Fatalf("zoo_keepers should be fully repaired, got %v", got)
	}
}
