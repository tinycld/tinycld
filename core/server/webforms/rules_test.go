package webforms

import (
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
)

const rulesDef = `{"version":1,"pages":[{"key":"main","title":"Main"}],"fields":[
{"key":"name","page":"main","label":"Name","kind":"text","required":true},
{"key":"age","page":"main","label":"Age","kind":"number"},
{"key":"ok","page":"main","label":"OK","kind":"toggle"},
{"key":"role","page":"main","label":"Role","kind":"select","options":[{"value":"dev","label":"Developer"}]},
{"key":"cv","page":"main","label":"CV","kind":"file"}]}`

func TestFormRuleWantsOnlyItsForm(t *testing.T) {
	f := newFixture(t, rulesDef)
	sub := storeForTest(t, f, map[string]any{"name": "Ada"})
	if !formRuleWants(f.app, map[string]any{"form": f.form.Id}, sub) {
		t.Fatal("a rule for this form wants its submission")
	}
	if formRuleWants(f.app, map[string]any{"form": "someotherform"}, sub) || formRuleWants(f.app, map[string]any{}, sub) {
		t.Fatal("a rule for another form, or for no form, must not")
	}
}

func TestFormFieldsMapsKindsAndValues(t *testing.T) {
	f := newFixture(t, rulesDef)
	sub := storeForTest(t, f, map[string]any{"name": "Ada", "age": 36, "ok": true, "role": "dev"}, upload{"files.cv", "cv.pdf", "application/pdf", []byte("%PDF")})
	got := map[string]automation.DynamicField{}
	for _, d := range formFields(f.app, map[string]any{"form": f.form.Id}, sub) {
		got[d.Key] = d
	}
	if got["name"].Value != "Ada" || got["name"].Type != "text" {
		t.Fatalf("name: %#v", got["name"])
	}
	if got["age"].Type != "number" || got["ok"].Type != "boolean" || got["role"].Type != "select" || got["role"].Options[0] != "dev" {
		t.Fatalf("kinds: %#v", got)
	}
	if got["cv"].Type != "text" || !strings.Contains(got["cv"].Value.(string), "cv.pdf") {
		t.Fatalf("a file field gives its file names: %#v", got["cv"])
	}
}

func TestFailureNoticeOncePerRulePerDay(t *testing.T) {
	f := newFixture(t, rulesDef)
	rlsUser(t, f.app, "boss@test.local", "admin")
	rules, _ := f.app.FindCollectionByNameOrId("rules")
	rule := core.NewRecord(rules)
	rule.Id = "rulefixture0001"
	rule.Set("name", "To the board")
	sub := storeForTest(t, f, map[string]any{"name": "Ada"})
	bad := automation.RunOutcome{Matched: true, Results: []automation.ActionResult{{Ref: "x", Status: "error"}}}

	onFormRun(f.app, rule, sub, bad)
	onFormRun(f.app, rule, sub, bad)
	onFormRun(f.app, rule, sub, automation.RunOutcome{Matched: true})

	n, _ := f.app.CountRecords("notifications", dbx.HashExp{"type": "core.web_form.delivery_failed"})
	if n != 1 {
		t.Fatalf("one notice for the admin, got %d", n)
	}
}
