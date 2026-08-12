// tinycld/core/server/automation/actions_test.go
package automation

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// actionApp builds: users (fixture), label_assignments-like target collection,
// a source collection, one user, one source record, and a personal rule record
// shape (plain base collection standing in for `rules` — executor only reads
// scope/owner via GetString).
func actionApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record, *Defs) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	users, _ := app.FindCollectionByNameOrId("users")

	src := core.NewBaseCollection("things")
	src.Fields.Add(&core.TextField{Name: "title"})
	src.Fields.Add(&core.TextField{Name: "status"})
	if err := app.Save(src); err != nil {
		t.Fatal(err)
	}
	tgt := core.NewBaseCollection("thing_labels")
	tgt.Fields.Add(&core.TextField{Name: "label"})
	tgt.Fields.Add(&core.TextField{Name: "record_id"})
	tgt.Fields.Add(&core.TextField{Name: "collection"})
	tgt.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1})
	if err := app.Save(tgt); err != nil {
		t.Fatal(err)
	}
	rulesCol := core.NewBaseCollection("fake_rules")
	rulesCol.Fields.Add(&core.TextField{Name: "scope"})
	rulesCol.Fields.Add(&core.TextField{Name: "owner"})
	if err := app.Save(rulesCol); err != nil {
		t.Fatal(err)
	}

	u, _ := app.FindFirstRecordByFilter("users", "id != ''")
	rec := core.NewRecord(src)
	rec.Set("title", "Invoice #7")
	rec.Set("status", "new")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	rule := core.NewRecord(rulesCol)
	rule.Set("scope", "org")
	rule.Set("owner", u.Id)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}

	defs := &Defs{Packages: []PackageDefs{{
		Slug: "core",
		Actions: []ActionDef{
			{
				ID: "apply-label", Kind: "record-op", Collection: "thing_labels",
				Op: RecordOp{Type: "create", Set: map[string]SetValue{
					"label":      {Param: "label"},
					"record_id":  {Context: "record-id"},
					"collection": {Context: "collection"},
					"user":       {Context: "owner"},
				}},
				Params: []ParamDef{{Key: "label", Field: "label"}},
			},
			{ID: "notify", Kind: "native", Params: []ParamDef{{Key: "title", Type: "text"}}},
		},
	}, {
		Slug: "things",
		Actions: []ActionDef{{
			ID: "set-status", Kind: "record-op", Collection: "things",
			Op:     RecordOp{Type: "update", Target: "trigger-record", Set: map[string]SetValue{"status": {Param: "status"}}},
			Params: []ParamDef{{Key: "status", Field: "status"}},
		}},
	}}}
	return app, rec, rule, defs
}

var openTrigger = TriggerDef{Collection: "things", On: "create"}

func TestRecordOpCreateWithContext(t *testing.T) {
	app, rec, rule, defs := actionApp(t)
	err := ExecuteAction(app, defs, "core:apply-label", map[string]any{"label": "Finance {{title}}"}, rule, openTrigger, rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	made, err := app.FindFirstRecordByFilter("thing_labels", "record_id = {:id}", map[string]any{"id": rec.Id})
	if err != nil {
		t.Fatal(err)
	}
	if made.GetString("label") != "Finance Invoice #7" {
		t.Fatalf("template param: %q", made.GetString("label"))
	}
	if made.GetString("collection") != "things" || made.GetString("user") != rule.GetString("owner") {
		t.Fatalf("context values: %+v", made.PublicExport())
	}
	if _, ok := takeEngineWrite(made.Id); !ok {
		t.Fatal("engine create must carry provenance")
	}
}

func TestRecordOpUpdateTriggerRecord(t *testing.T) {
	app, rec, rule, defs := actionApp(t)
	if err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 1); err != nil {
		t.Fatal(err)
	}
	fresh, _ := app.FindRecordById("things", rec.Id)
	if fresh.GetString("status") != "filed" {
		t.Fatal("update did not apply")
	}
	w, ok := takeEngineWrite(rec.Id)
	if !ok || w.Depth != 1 || w.RuleID != rule.Id {
		t.Fatalf("provenance: %+v %v", w, ok)
	}
}

func TestNativeDispatchAndMissingHandler(t *testing.T) {
	app, rec, rule, defs := actionApp(t)
	var got ActionRequest
	RegisterAction("core:notify", func(a core.App, req ActionRequest) error {
		got = req
		return nil
	})
	err := ExecuteAction(app, defs, "core:notify", map[string]any{"title": "Got {{title}}"}, rule, openTrigger, rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Params["title"] != "Got Invoice #7" {
		t.Fatalf("substituted params must reach handler: %+v", got.Params)
	}
	ResetRegistriesForTest()
	if err := ExecuteAction(app, defs, "core:notify", nil, rule, openTrigger, rec, 0); err == nil {
		t.Fatal("missing native handler must error")
	}
}

func TestTriggerRecordOpNeedsMatchingCollection(t *testing.T) {
	app, _, rule, defs := actionApp(t)
	if err := ExecuteAction(app, defs, "things:set-status", nil, rule, openTrigger, nil, 0); err == nil {
		t.Fatal("trigger-record op with nil record must error")
	}
}
