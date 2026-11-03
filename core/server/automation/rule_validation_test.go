package automation

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func validationDefs() *Defs {
	return &Defs{Packages: []PackageDefs{{Slug: "gizmos", Actions: []ActionDef{{ID: "file", Kind: "native", Params: []ParamDef{
		{Key: "title", Type: "text", Required: true},
		{Key: "note", Type: "text"},
		{Key: "ticket", Type: "relation", RelationTarget: "tickets"},
	}}}}}}
}

func saveRule(t *testing.T, app core.App, owner, actions string) (*core.Record, error) {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("rules")
	r := core.NewRecord(col)
	r.Set("name", "Check")
	r.Set("scope", "personal")
	r.Set("owner", owner)
	r.Set("trigger", "tickets:ticket-created")
	r.Set("actions", actions)
	r.Set("enabled", true)
	return r, app.Save(r)
}

func TestRuleValidationRefusesUnknownActionParamAndMissingRequired(t *testing.T) {
	app, _, u := engineApp(t)
	BindRuleValidation(app, validationDefs())
	_, err := saveRule(t, app, u.Id, `[
		{"ref":"gizmos:fly","params":{}},
		{"ref":"gizmos:file","params":{"title":"","colour":"red"}}]`)
	if err == nil {
		t.Fatal("an invalid rule is refused")
	}
	msg := err.Error()
	for _, want := range []string{"actions.0.ref", "gizmos:file", "actions.1.params.colour", "actions.1.params.title", "actions.1.params.ticket"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
}

func TestRuleValidationAcceptsAValidRule(t *testing.T) {
	app, _, u := engineApp(t)
	BindRuleValidation(app, validationDefs())
	if _, err := saveRule(t, app, u.Id, `[{"ref":"gizmos:file","params":{"title":"Hi {{title}}","ticket":"abc123"}}]`); err != nil {
		t.Fatalf("a valid rule saves: %v", err)
	}
}

// A rule whose package was uninstalled can still be turned off: an update
// that leaves the actions alone is not checked.
func TestRuleValidationSkipsAnUpdateThatKeepsTheActions(t *testing.T) {
	app, _, u := engineApp(t)
	r, err := saveRule(t, app, u.Id, `[{"ref":"gone:thing","params":{}}]`)
	if err != nil {
		t.Fatal(err)
	}
	BindRuleValidation(app, validationDefs())
	r.Set("enabled", false)
	if err := app.Save(r); err != nil {
		t.Fatalf("turning off a rule of an uninstalled package works: %v", err)
	}
	r.Set("actions", `[{"ref":"gone:thing","params":{"x":1}}]`)
	if err := app.Save(r); err == nil {
		t.Fatal("changing its actions is checked")
	}
}

func TestRuleValidationNamesNoParamsAsNone(t *testing.T) {
	app, _, u := engineApp(t)
	defs := &Defs{Packages: []PackageDefs{{Slug: "gizmos", Actions: []ActionDef{{ID: "ping", Kind: "native"}}}}}
	BindRuleValidation(app, defs)
	_, err := saveRule(t, app, u.Id, `[{"ref":"gizmos:ping","params":{"x":1}}]`)
	if err == nil || !strings.Contains(err.Error(), "Params: none") {
		t.Fatalf("got %v", err)
	}
}

// A param the package no longer declares, which the rule already held, does
// not block an edit: the app cannot show or remove it. A new unknown param
// is still refused.
func TestRuleValidationKeepsAStoredParamThePackageDropped(t *testing.T) {
	app, _, u := engineApp(t)
	r, err := saveRule(t, app, u.Id, `[{"ref":"gizmos:file","params":{"title":"A","ticket":"t1","colour":"red"}}]`)
	if err != nil {
		t.Fatal(err)
	}
	BindRuleValidation(app, validationDefs())
	r.Set("actions", `[{"ref":"gizmos:file","params":{"title":"B","ticket":"t1","colour":"red"}}]`)
	if err := app.Save(r); err != nil {
		t.Fatalf("an edit that keeps the dropped param saves: %v", err)
	}
	r.Set("actions", `[{"ref":"gizmos:file","params":{"title":"B","ticket":"t1","colour":"red","size":"xl"}}]`)
	if err := app.Save(r); err == nil || !strings.Contains(err.Error(), "actions.0.params.size") || strings.Contains(err.Error(), "colour") {
		t.Fatalf("only the new unknown param is refused, got %v", err)
	}
}
