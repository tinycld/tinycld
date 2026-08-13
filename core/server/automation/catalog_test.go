// tinycld/core/server/automation/catalog_test.go
package automation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/rlstest"
)

func catalogApp(t *testing.T) (*tests.TestApp, *Engine) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	users, _ := app.FindCollectionByNameOrId("users")

	folders := core.NewBaseCollection("cat_folders")
	folders.Fields.Add(&core.TextField{Name: "name"})
	if err := app.Save(folders); err != nil {
		t.Fatal(err)
	}
	items := core.NewBaseCollection("cat_items")
	items.Fields.Add(&core.TextField{Name: "subject"})
	items.Fields.Add(&core.BoolField{Name: "has_attachments"})
	items.Fields.Add(&core.SelectField{Name: "status", Values: []string{"new", "done"}, MaxSelect: 1})
	items.Fields.Add(&core.RelationField{Name: "folder", CollectionId: folders.Id, MaxSelect: 1})
	items.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1})
	if err := app.Save(items); err != nil {
		t.Fatal(err)
	}

	defs := &Defs{Packages: []PackageDefs{
		{Slug: "core", Triggers: []TriggerDef{{ID: "manual", Label: "Run manually", Synthetic: "manual"}},
			Actions: []ActionDef{{ID: "notify", Label: "Notify", Kind: "native",
				Params: []ParamDef{{Key: "title", Type: "text"}}}}},
		{Slug: "cat", Triggers: []TriggerDef{{
			ID: "item-created", Label: "An item is created", Collection: "cat_items", On: "create",
			Fields: []FieldRef{{Key: "subject"}, {Key: "has_attachments"}, {Key: "folder"}, {Key: "status"}},
		}},
			Actions: []ActionDef{{ID: "set-folder", Label: "Move to folder", Kind: "record-op",
				Collection: "cat_items",
				Op:         RecordOp{Type: "update", Target: "trigger-record", Set: map[string]SetValue{"folder": {Param: "folder"}}},
				Params:     []ParamDef{{Key: "folder", Field: "folder"}}}}},
	}}
	return app, NewEngine(app, defs)
}

func TestCatalogResolution(t *testing.T) {
	app, eng := catalogApp(t)
	res := eng.buildCatalog(app)

	var item *catalogTrigger
	for i := range res.Triggers {
		if res.Triggers[i].Ref == "cat:item-created" {
			item = &res.Triggers[i]
		}
	}
	if item == nil {
		t.Fatal("trigger missing from catalog")
	}
	byKey := map[string]catalogField{}
	for _, f := range item.Fields {
		byKey[f.Key] = f
	}
	if byKey["subject"].Type != "text" || byKey["has_attachments"].Type != "boolean" {
		t.Fatalf("basic types: %+v", byKey)
	}
	if byKey["has_attachments"].Label != "Has attachments" {
		t.Fatalf("humanized label: %q", byKey["has_attachments"].Label)
	}
	if byKey["status"].Type != "select" || len(byKey["status"].Options) != 2 {
		t.Fatalf("select options: %+v", byKey["status"])
	}
	f := byKey["folder"]
	if f.Type != "relation" || f.RelationTarget != "cat_folders" || f.DisplayField != "name" {
		t.Fatalf("relation resolution: %+v", f)
	}
}

func TestCatalogActionAvailability(t *testing.T) {
	app, eng := catalogApp(t)
	res := eng.buildCatalog(app)
	get := func(ref string) catalogAction {
		for _, a := range res.Actions {
			if a.Ref == ref {
				return a
			}
		}
		t.Fatalf("action %s missing", ref)
		return catalogAction{}
	}
	notify := get("core:notify")
	if notify.Available {
		t.Fatal("native action without a handler must be unavailable")
	}
	// Template is a pure type-derived flag (server has no notion of "which
	// trigger will this rule use" — that's a UI-time choice), so a novel
	// text param is always templatable regardless of handler registration.
	if !notify.Params[0].Template {
		t.Fatalf("text param must be marked templatable: %+v", notify.Params[0])
	}
	RegisterAction("core:notify", func(a core.App, req ActionRequest) error { return nil })
	if !eng.buildCatalog(app).Actions[actionIndex(eng.buildCatalog(app).Actions, "core:notify")].Available {
		t.Fatal("registered native action must be available")
	}
	sf := get("cat:set-folder")
	if !sf.Available || sf.OpTarget != "trigger-record" || sf.Params[0].Field.RelationTarget != "cat_folders" {
		t.Fatalf("record-op resolution: %+v", sf)
	}
	if sf.Params[0].Template {
		t.Fatalf("relation-typed param must not be marked templatable: %+v", sf.Params[0])
	}
}

// TestCatalogDeclaredAllowlistFiltersHiddenFields covers the same
// system/hidden filter exposedFields applies to an OPEN trigger (see
// resolvableColumns), now proven for a DECLARED allowlist too: a trigger
// def that names a hidden field (tokenKey, on the auth-collection-backed
// users table) must not publish it into the catalog, even though the field
// exists and would otherwise resolve to a usable "text" type. Regression
// test for the declared branch skipping the hidden/system filter the open
// branch already applied.
func TestCatalogDeclaredAllowlistFiltersHiddenFields(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ResetRegistriesForTest)
	t.Cleanup(func() { app.Cleanup() })

	defs := &Defs{Packages: []PackageDefs{
		{Slug: "auth", Triggers: []TriggerDef{{
			ID: "user-created", Label: "A user is created", Collection: "users", On: "create",
			Fields: []FieldRef{{Key: "tokenKey"}, {Key: "name"}},
		}}},
	}}
	eng := NewEngine(app, defs)
	res := eng.buildCatalog(app)

	var trig *catalogTrigger
	for i := range res.Triggers {
		if res.Triggers[i].Ref == "auth:user-created" {
			trig = &res.Triggers[i]
		}
	}
	if trig == nil {
		t.Fatal("trigger missing from catalog")
	}
	for _, f := range trig.Fields {
		if f.Key == "tokenKey" {
			t.Fatalf("hidden field tokenKey must not be published in the catalog, got fields: %+v", trig.Fields)
		}
	}
	found := false
	for _, f := range trig.Fields {
		if f.Key == "name" {
			found = true
		}
	}
	if !found {
		t.Fatalf("regular declared field 'name' should still resolve, got: %+v", trig.Fields)
	}
}

func actionIndex(actions []catalogAction, ref string) int {
	for i, a := range actions {
		if a.Ref == ref {
			return i
		}
	}
	return -1
}

// catalogSyncApp mirrors runsApp's rlstest idiom (runs_test.go): apply the
// real shipped migrations rather than hand-building bare collections, so the
// reconcile test proves syncCatalog against the actual automation_catalog
// schema — including the username-index fixture reconciliation actions_test.go
// / runs_test.go both need before 1820000000 (users_username_required) applies.
func catalogSyncApp(t *testing.T) (*tests.TestApp, *Defs) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	app := rlstest.NewApp(t)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	var kept types.JSONArray[string]
	for _, idx := range users.Indexes {
		if !strings.Contains(idx, "username") {
			kept = append(kept, idx)
		}
	}
	users.Indexes = kept
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatalf("drop fixture username index: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))

	defs := &Defs{Packages: []PackageDefs{
		{Slug: "core", Triggers: []TriggerDef{{ID: "manual", Label: "Run manually", Synthetic: "manual"}},
			Actions: []ActionDef{{ID: "notify", Label: "Notify", Kind: "native",
				Params: []ParamDef{{Key: "title", Type: "text"}}}}},
	}}
	return app, defs
}

func TestSyncCatalogReconciles(t *testing.T) {
	app, defs := catalogSyncApp(t)
	eng := NewEngine(app, defs)

	eng.syncCatalog()

	rows, err := app.FindRecordsByFilter("automation_catalog", "", "ref", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// one row per trigger + action: core:manual (trigger), core:notify (action)
	if len(rows) != 2 {
		t.Fatalf("expected 2 catalog rows, got %d: %+v", len(rows), rows)
	}

	var notifyRow *core.Record
	for _, r := range rows {
		if r.GetString("ref") == "core:notify" {
			notifyRow = r
		}
	}
	if notifyRow == nil {
		t.Fatal("core:notify row missing")
	}
	if notifyRow.GetBool("available") {
		t.Fatal("core:notify should be unavailable before the handler registers")
	}
	var decoded catalogAction
	if err := json.Unmarshal([]byte(notifyRow.GetString("definition")), &decoded); err != nil {
		t.Fatalf("definition did not round-trip: %v", err)
	}
	if decoded.Ref != "core:notify" || decoded.Available {
		t.Fatalf("decoded definition mismatch: %+v", decoded)
	}

	// Register the handler and re-sync: the existing row updates (available
	// flips), no duplicate row is created (unique ref).
	RegisterAction("core:notify", func(a core.App, req ActionRequest) error { return nil })
	eng.syncCatalog()

	rows, err = app.FindRecordsByFilter("automation_catalog", "ref = 'core:notify'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly one core:notify row after re-sync, got %d", len(rows))
	}
	if !rows[0].GetBool("available") {
		t.Fatal("core:notify should be available after the handler registers")
	}

	// Remove the action from defs (a second Engine sharing the app, fewer
	// defs) and re-sync: the stale row must be deleted.
	fewerDefs := &Defs{Packages: []PackageDefs{
		{Slug: "core", Triggers: []TriggerDef{{ID: "manual", Label: "Run manually", Synthetic: "manual"}}},
	}}
	eng2 := NewEngine(app, fewerDefs)
	eng2.syncCatalog()

	rows, err = app.FindRecordsByFilter("automation_catalog", "ref = 'core:notify'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected core:notify row to be deleted as stale, got %d rows", len(rows))
	}

	rows, err = app.FindRecordsByFilter("automation_catalog", "", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only core:manual to remain, got %d rows: %+v", len(rows), rows)
	}
}
