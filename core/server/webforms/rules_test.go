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
	sub := storeForTest(t, f, map[string]any{"name": "Ada", "age": 36, "ok": true, "role": "dev"}, upload{"files.cv", "cv.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n")})
	got := map[string]automation.DynamicField{}
	for _, d := range formFields(f.app, "", map[string]any{"form": f.form.Id}, sub) {
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

func autoReplyRule(t *testing.T, f *fixture, enabled bool) (*core.Record, error) {
	t.Helper()
	owner, _ := f.app.FindFirstRecordByFilter("users", "role = 'admin'")
	if owner == nil {
		rlsUser(t, f.app, "admin2@test.local", "admin")
		owner, _ = f.app.FindFirstRecordByFilter("users", "role = 'admin'")
	}
	rules, _ := f.app.FindCollectionByNameOrId("rules")
	r := core.NewRecord(rules)
	r.Set("name", "Auto reply")
	r.Set("scope", "org")
	r.Set("owner", owner.Id)
	r.Set("trigger", FormSubmittedTrigger)
	r.Set("trigger_config", map[string]any{"form": f.form.Id})
	r.Set("actions", `[{"ref":"core:send-email","params":{"to":"{{fields.email}}","subject":"Thanks"}}]`)
	r.Set("enabled", enabled)
	return r, f.app.Save(r)
}

func TestAutoReplyNeedsProofOfWork(t *testing.T) {
	f := newFixture(t, rulesDef)
	if _, err := autoReplyRule(t, f, true); err == nil || !strings.Contains(err.Error(), "actions") {
		t.Fatalf("an enabled auto-reply without proof of work is refused, got %v", err)
	}
	if _, err := autoReplyRule(t, f, false); err != nil {
		t.Fatalf("a disabled one may be saved: %v", err)
	}
	f.pub.Set("require_pow", true)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	if _, err := autoReplyRule(t, f, true); err != nil {
		t.Fatalf("with proof of work it is allowed: %v", err)
	}
	f.pub.Set("require_pow", false)
	if err := f.app.Save(f.pub); err == nil || !strings.Contains(err.Error(), "require_pow") {
		t.Fatalf("proof of work cannot be turned off while the rule is on, got %v", err)
	}
}

func TestEmailToAFixedAddressIsAllowed(t *testing.T) {
	f := newFixture(t, rulesDef)
	r, _ := autoReplyRule(t, f, false)
	r.Set("actions", `[{"ref":"core:send-email","params":{"to":"sales@example.com","subject":"New lead {{fields.name}}"}}]`)
	r.Set("enabled", true)
	if err := f.app.Save(r); err != nil {
		t.Fatalf("a fixed recipient is safe: %v", err)
	}
}

func TestAutoReplyThroughAPackageSendActionNeedsProofOfWork(t *testing.T) {
	f := newFixture(t, rulesDef)
	r, _ := autoReplyRule(t, f, false)
	r.Set("actions", `[{"ref":"gizmos:dispatch","params":{"recipient":"{{ fields.name }}","note":"hi"}}]`)
	r.Set("enabled", true)
	if err := f.app.Save(r); err == nil || !strings.Contains(err.Error(), "actions") {
		t.Fatalf("any recipient param filled from the form needs proof of work, got %v", err)
	}
	r.Set("actions", `[{"ref":"gizmos:dispatch","params":{"recipient":"ops@example.com","note":"{{fields.name}}"}}]`)
	if err := f.app.Save(r); err != nil {
		t.Fatalf("a fixed recipient is safe even when another param reads the form: %v", err)
	}
}

func TestFormScopeLimitsTheDryRunToItsForm(t *testing.T) {
	f := newFixture(t, rulesDef)
	storeForTest(t, f, map[string]any{"name": "Mine"})
	other := secondPublication(t, f, 90)
	storeForTest(t, other, map[string]any{"name": "Theirs"})

	filter, params := formScope(map[string]any{"form": f.form.Id})
	rows, err := f.app.FindRecordsByFilter("form_submissions", filter, "", 0, 0, params)
	if err != nil || len(rows) != 1 || rows[0].GetString("form") != f.form.Id {
		t.Fatalf("the scope reads this form's submissions only: %d %v", len(rows), err)
	}
	if filter, _ := formScope(map[string]any{}); filter != "id = ''" {
		t.Fatalf("a rule with no form reads nothing, got %q", filter)
	}
}

// Deleting the publication and creating it again without proof of work must
// not get around the safety rule while an auto-reply destination is on.
func TestRecreatedPublicationStillNeedsProofOfWork(t *testing.T) {
	f := newFixture(t, rulesDef)
	f.pub.Set("require_pow", true)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	if _, err := autoReplyRule(t, f, true); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Delete(f.pub); err != nil {
		t.Fatal(err)
	}
	pubs, _ := f.app.FindCollectionByNameOrId("form_publications")
	again := core.NewRecord(pubs)
	again.Set("form", f.form.Id)
	again.Set("enabled", true)
	again.Set("require_pow", false)
	err := f.app.Save(again)
	if err == nil || !strings.Contains(err.Error(), "require_pow") || !strings.Contains(err.Error(), "Auto reply") {
		t.Fatalf("the new publication is refused and names the rule, got %v", err)
	}
}
