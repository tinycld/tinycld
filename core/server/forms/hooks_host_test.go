package forms

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

const bugDef = `{"version":1,"pages":[{"key":"main","title":"Bug"},{"key":"repro","title":"Repro"}],"fields":[
{"key":"name","kind":"builtin","page":"main","label":"Name","width":"full","required":true,"hidden":false},
{"key":"notes","kind":"builtin","page":"main","label":"Notes","width":"full","required":false,"hidden":false},
{"key":"due","kind":"builtin","page":"main","label":"Due","width":"full","required":false,"hidden":false},
{"key":"owner","kind":"builtin","page":"main","label":"Owner","width":"full","required":false,"hidden":false},
{"key":"tasks","kind":"builtin","page":"main","label":"Tasks","width":"full","required":false,"hidden":false},
{"key":"comments","kind":"builtin","page":"main","label":"Comments","width":"full","required":false,"hidden":false},
{"key":"severity","kind":"select","page":"main","label":"Severity","width":"third","required":false,"hidden":false,"display":"dropdown","multiple":false,"options":[{"value":"low","label":"Low"},{"value":"high","label":"High"}]},
{"key":"steps","kind":"textarea","page":"repro","label":"Steps","width":"full","required":false,"hidden":false},
{"key":"nick","kind":"text","page":"main","label":"Nick","width":"full","required":false,"hidden":false,"maxLength":5},
{"key":"tags","kind":"select","page":"main","label":"Tags","width":"full","required":false,"hidden":false,"display":"dropdown","multiple":true,"options":[{"value":"a","label":"A"}]},
{"key":"assignee","kind":"user","page":"main","label":"Assignee","width":"full","required":false,"hidden":false},
{"key":"flag","kind":"toggle","page":"main","label":"Flag","width":"full","required":false,"hidden":false},
{"key":"todo","kind":"checklist","page":"main","label":"Todo","width":"full","required":false,"hidden":false},
{"key":"body","kind":"markdown","page":"main","label":"Body","width":"full","required":false,"hidden":false,"collaborative":true,"maxLength":5},
{"key":"count","kind":"number","page":"main","label":"Count","width":"full","required":false,"hidden":false,"min":0,"max":10}
],"rules":[{"key":"r","match":"all","when":[{"field":"severity","op":"is","value":"high"}],"actions":[{"action":"require","field":"steps"}]}],
"defaults":{},"submit":{"required":[]}}`

func (e *env) widget(t *testing.T, formID string, fields string) (*core.Record, error) {
	t.Helper()
	col, _ := e.app.FindCollectionByNameOrId("gizmos_widgets")
	r := core.NewRecord(col)
	r.Set("name", "w")
	if formID != "" {
		r.Set("form", formID)
	}
	if fields != "" {
		r.Set("fields", []byte(fields))
	}
	return r, e.app.Save(r)
}

func TestHostHook_FillsTheDefaultFormWhenEmpty(t *testing.T) {
	e := newTestEnv(t)
	def := e.newForm(t, "widget", widgetDefault, true)
	w, err := e.widget(t, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if w.GetString("form") != def.Id {
		t.Fatalf("form = %q, want the default %q", w.GetString("form"), def.Id)
	}
}

func TestHostHook_RejectsAFormOfAnotherHost(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	col, _ := e.app.FindCollectionByNameOrId("forms")
	other := core.NewRecord(col)
	other.Set("host", "gizmos:other")
	other.Set("name", "o")
	other.Set("slug", "o")
	other.Set("order", 1)
	other.Set("definition", []byte(`{"version":1,"pages":[{"key":"main","title":"Other"}],"fields":[{"key":"name","kind":"builtin","page":"main","label":"Name","width":"full","required":true,"hidden":false}],"rules":[],"defaults":{},"submit":{"required":[]}}`))
	if err := e.app.Save(other); err != nil {
		t.Fatal(err)
	}
	_, err := e.widget(t, other.Id, "")
	if err == nil || !strings.Contains(err.Error(), "validation_unknown_form") && !strings.Contains(err.Error(), "form:") {
		t.Fatalf("got %v", err)
	}
}

func TestHostHook_RejectsUnknownKeyAndWrongKind(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	if _, err := e.widget(t, bug.Id, `{"ghost":1}`); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("unknown key: got %v", err)
	}
	if _, err := e.widget(t, bug.Id, `{"count":"many"}`); err == nil || !strings.Contains(err.Error(), "count") {
		t.Fatalf("wrong kind: got %v", err)
	}
	if _, err := e.widget(t, bug.Id, `{"severity":"nope"}`); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("bad option: got %v", err)
	}
	if _, err := e.widget(t, bug.Id, `{"count":11}`); err == nil || !strings.Contains(err.Error(), "count") {
		t.Fatalf("over max: got %v", err)
	}
}

func TestHostHook_RuleRequiredOnCreate(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	if _, err := e.widget(t, bug.Id, `{"severity":"high"}`); err == nil || !strings.Contains(err.Error(), "steps") {
		t.Fatalf("high severity without steps must be refused, got %v", err)
	}
	if _, err := e.widget(t, bug.Id, `{"severity":"high","steps":"1. open"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.widget(t, bug.Id, `{"severity":"low"}`); err != nil {
		t.Fatal(err)
	}
}

func TestHostHook_UpdateCannotClearARequiredField(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"severity":"high","steps":"1. open"}`)
	if err != nil {
		t.Fatal(err)
	}
	w.Set("fields", []byte(`{"severity":"high","steps":""}`))
	if err := e.app.Save(w); err == nil || !strings.Contains(err.Error(), "steps") {
		t.Fatalf("clearing steps must be refused, got %v", err)
	}
	w.Set("fields", []byte(`{"severity":"high","steps":"1. open","count":3}`))
	if err := e.app.Save(w); err != nil {
		t.Fatal(err)
	}
}

func TestHostHook_KeepsValuesOfAnotherFormAfterTypeChange(t *testing.T) {
	e := newTestEnv(t)
	plain := e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"severity":"low","count":2}`)
	if err != nil {
		t.Fatal(err)
	}
	w.Set("form", plain.Id)
	if err := e.app.Save(w); err != nil {
		t.Fatalf("a type change must keep orphan keys, got %v", err)
	}
}

func TestHostHook_RejectsBuiltinKeysInJSON(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	for _, key := range []string{"tasks", "notes"} {
		if _, err := e.widget(t, bug.Id, `{"`+key+`":"anything"}`); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("builtin %s never travels in the json column, got %v", key, err)
		}
	}
}

func TestDefaultFormIDFrom(t *testing.T) {
	if id, err := defaultFormIDFrom(nil, sql.ErrNoRows); id != "" || err != nil {
		t.Fatalf("no rows must mean no default, got %q %v", id, err)
	}
	if _, err := defaultFormIDFrom(nil, errors.New("boom")); err == nil {
		t.Fatal("a database error must propagate")
	}
}

func TestHostHook_RejectsEmptyFormWhenNoDefaultButFormsExist(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "a", widgetDefault, false)
	_, err := e.widget(t, "", "")
	if err == nil || !strings.Contains(err.Error(), "form") {
		t.Fatalf("got %v", err)
	}
}

func TestHostHook_ChecksKindOfCollaborativeCustomField(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	if _, err := e.widget(t, bug.Id, `{"body":3}`); err == nil || !strings.Contains(err.Error(), "body") {
		t.Fatalf("number in a text field, got %v", err)
	}
	if _, err := e.widget(t, bug.Id, `{"body":"ok"}`); err != nil {
		t.Fatal(err)
	}
}

func TestHostHook_RuleMadeRequiredByAnUpdateIsEnforced(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"severity":"low"}`)
	if err != nil {
		t.Fatal(err)
	}
	w.Set("fields", []byte(`{"severity":"low","count":1}`))
	if err := e.app.Save(w); err != nil {
		t.Fatalf("unrelated update must pass, got %v", err)
	}
	w.Set("fields", []byte(`{"severity":"high"}`))
	if err := e.app.Save(w); err == nil || !strings.Contains(err.Error(), "steps") {
		t.Fatalf("becoming required must be enforced, got %v", err)
	}
}

func TestHostHook_CheckKindBranches(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	cases := map[string]string{
		"tags":     `{"tags":"a"}`,
		"assignee": `{"assignee":5}`,
		"flag":     `{"flag":"yes"}`,
		"todo":     `{"todo":[{"title":"x"}]}`,
		"nick":     `{"nick":"toolong"}`,
	}
	for key, fields := range cases {
		if _, err := e.widget(t, bug.Id, fields); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("%s: got %v", key, err)
		}
	}
}

func (e *env) editForm(t *testing.T, form *core.Record, from, to string) {
	t.Helper()
	def := form.GetString("definition")
	if !strings.Contains(def, from) {
		t.Fatalf("form definition lacks %s", from)
	}
	form.Set("definition", []byte(strings.Replace(def, from, to, 1)))
	if err := e.app.Save(form); err != nil {
		t.Fatal(err)
	}
}

const nickField = `{"key":"nick","kind":"text","page":"main","label":"Nick","width":"full","required":false,"hidden":false,"maxLength":5},`

func TestHostHook_UpdateAfterAFieldIsDeletedFromTheForm(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"severity":"low","nick":"abc"}`)
	if err != nil {
		t.Fatal(err)
	}
	e.editForm(t, bug, nickField, "")
	w.Set("name", "renamed")
	if err := e.app.Save(w); err != nil {
		t.Fatalf("an unrelated update after a field delete must pass, got %v", err)
	}
	w.Set("fields", []byte(`{"severity":"low","nick":"abcd"}`))
	if err := e.app.Save(w); err == nil || !strings.Contains(err.Error(), "nick") {
		t.Fatalf("changing the deleted field's value must be refused, got %v", err)
	}
}

func TestHostHook_UpdateAfterAnOptionIsRemoved(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"severity":"low"}`)
	if err != nil {
		t.Fatal(err)
	}
	e.editForm(t, bug, `{"value":"low","label":"Low"},`, "")
	w.Set("fields", []byte(`{"severity":"low","count":4}`))
	if err := e.app.Save(w); err != nil {
		t.Fatalf("updating another key after an option removal must pass, got %v", err)
	}
	w.Set("fields", []byte(`{"severity":["low"],"count":4}`))
	if err := e.app.Save(w); err == nil || !strings.Contains(err.Error(), "severity") {
		t.Fatalf("changing the key to a value that is not an option must be refused, got %v", err)
	}
}

func TestHostHook_UpdateAfterALimitIsTightened(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"count":9}`)
	if err != nil {
		t.Fatal(err)
	}
	e.editForm(t, bug, `"min":0,"max":10`, `"min":0,"max":5`)
	w.Set("name", "renamed")
	if err := e.app.Save(w); err != nil {
		t.Fatalf("an unrelated update after a tighter max must pass, got %v", err)
	}
	w.Set("fields", []byte(`{"count":8}`))
	if err := e.app.Save(w); err == nil || !strings.Contains(err.Error(), "count") {
		t.Fatalf("a changed value is checked against the new max, got %v", err)
	}
}

func TestHostHook_FormAddsARequiredFieldThenAnUnrelatedUpdatePasses(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"severity":"low"}`)
	if err != nil {
		t.Fatal(err)
	}
	e.editForm(t, bug, `"label":"Flag","width":"full","required":false`, `"label":"Flag","width":"full","required":true`)
	e.editForm(t, bug, nickField, nickField+`{"key":"code","kind":"text","page":"main","label":"Code","width":"full","required":true,"hidden":false},`)
	w.Set("name", "renamed")
	if err := e.app.Save(w); err != nil {
		t.Fatalf("an unrelated update must pass when the form gained a required field, got %v", err)
	}
	if _, err := e.widget(t, bug.Id, `{"severity":"low"}`); err == nil || !strings.Contains(err.Error(), "code") {
		t.Fatalf("a new record must fill the new required field, got %v", err)
	}
}

func TestHostHook_RequiredBuiltinColumnIsEnforced(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	ruleRequired := strings.Replace(bugDef, `"actions":[{"action":"require","field":"steps"}]`, `"actions":[{"action":"require","field":"due"}]`, 1)
	byRule := e.newForm(t, "by_rule", ruleRequired, false)
	if _, err := e.widget(t, byRule.Id, `{"severity":"high"}`); err == nil || !strings.Contains(err.Error(), "due") {
		t.Fatalf("a rule-required built-in must be filled on create, got %v", err)
	}
	adminRequired := strings.Replace(bugDef, `"label":"Due","width":"full","required":false`, `"label":"Due","width":"full","required":true`, 1)
	byAdmin := e.newForm(t, "by_admin", adminRequired, false)
	if _, err := e.widget(t, byAdmin.Id, ""); err == nil || !strings.Contains(err.Error(), "due") {
		t.Fatalf("an admin-required built-in must be filled on create, got %v", err)
	}
	col, _ := e.app.FindCollectionByNameOrId("gizmos_widgets")
	w := core.NewRecord(col)
	w.Set("name", "w")
	w.Set("form", byAdmin.Id)
	w.Set("due", "2026-03-01 00:00:00.000Z")
	if err := e.app.Save(w); err != nil {
		t.Fatal(err)
	}
	w.Set("due", "")
	if err := e.app.Save(w); err == nil || !strings.Contains(err.Error(), "due") {
		t.Fatalf("an update that clears a required built-in must be refused, got %v", err)
	}
}

func TestHostHook_RequiredRowsAndCollaborativeBuiltinsAreSkipped(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	def := strings.Replace(bugDef, `"label":"Tasks","width":"full","required":false`, `"label":"Tasks","width":"full","required":true`, 1)
	def = strings.Replace(def, `"label":"Notes","width":"full","required":false`, `"label":"Notes","width":"full","required":true`, 1)
	form := e.newForm(t, "rows", def, false)
	if _, err := e.widget(t, form.Id, ""); err != nil {
		t.Fatalf("rows and collaborative built-ins carry no value in the write, got %v", err)
	}
}

func TestHostHook_AcceptsAValueFromASetOptionsRule(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	def := strings.Replace(bugDef, `"actions":[{"action":"require","field":"steps"}]`, `"actions":[{"action":"set_options","field":"tags","options":[{"value":"z","label":"Z"}]}]`, 1)
	form := e.newForm(t, "high_sets_tags", def, false)
	if _, err := e.widget(t, form.Id, `{"severity":"high","tags":["z"]}`); err != nil {
		t.Fatalf("a value from a set_options list must pass, got %v", err)
	}
	if _, err := e.widget(t, form.Id, `{"severity":"low","tags":["z"]}`); err == nil || !strings.Contains(err.Error(), "tags") {
		t.Fatalf("with the rule inactive the field's own options apply, got %v", err)
	}
}

func TestHostHook_DateValuesMatchTheStoredShape(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	def := withField(widgetDefault, `{"key":"when","kind":"date","page":"main","label":"When","width":"full","required":false,"hidden":false}`)
	form := e.newForm(t, "dated", def, false)
	for _, ok := range []string{`"2026-01-31"`, `"2026-01-31T09:30"`} {
		if _, err := e.widget(t, form.Id, `{"when":`+ok+`}`); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{`"tomorrow"`, `"2026-1-31"`, `"2026-01-31 09:30"`, `5`} {
		if _, err := e.widget(t, form.Id, `{"when":`+bad+`}`); err == nil || !strings.Contains(err.Error(), "when") {
			t.Fatalf("%s must be refused, got %v", bad, err)
		}
	}
}

// openWidgets lets an anonymous request write gizmos_widgets, so the
// request-only hooks run through the real record API.
func (e *env) openWidgets(t *testing.T) {
	t.Helper()
	e.openCollection(t, "gizmos_widgets")
}

func (e *env) openCollection(t *testing.T, name string) {
	t.Helper()
	col, err := e.app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	open := ""
	col.CreateRule = &open
	col.UpdateRule = &open
	if err := e.app.Save(col); err != nil {
		t.Fatal(err)
	}
}

func (e *env) storedFields(t *testing.T, id string) map[string]any {
	t.Helper()
	rec, err := e.app.FindRecordById("gizmos_widgets", id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := fieldsOf(rec, "fields")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (e *env) request(t *testing.T, method, url, payload string) {
	t.Helper()
	sc := tests.ApiScenario{
		Method:                method,
		URL:                   url,
		Body:                  strings.NewReader(payload),
		ExpectedStatus:        200,
		ExpectedContent:       []string{`"id":`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return e.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}

func TestHostHook_UpdateRequestKeepsTheRoomsCollaborativeValue(t *testing.T) {
	e := newTestEnv(t)
	e.openWidgets(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"body":"room"}`)
	if err != nil {
		t.Fatal(err)
	}
	e.request(t, http.MethodPatch, "/api/collections/gizmos_widgets/records/"+w.Id, `{"fields":{"body":"stale","count":3}}`)
	got := e.storedFields(t, w.Id)
	if got["body"] != "room" {
		t.Fatalf("a client patch overwrote the room's text: body = %v", got["body"])
	}
	if got["count"] != float64(3) {
		t.Fatalf("the rest of the patch must still write: count = %v", got["count"])
	}
}

func TestHostHook_UpdateRequestCannotAddACollaborativeValue(t *testing.T) {
	e := newTestEnv(t)
	e.openWidgets(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"count":1}`)
	if err != nil {
		t.Fatal(err)
	}
	e.request(t, http.MethodPatch, "/api/collections/gizmos_widgets/records/"+w.Id, `{"fields":{"body":"late","count":2}}`)
	got := e.storedFields(t, w.Id)
	if _, has := got["body"]; has {
		t.Fatalf("a key the room never wrote must stay absent, got body = %v", got["body"])
	}
}

func TestHostHook_NonRequestSaveWritesTheCollaborativeValue(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	w, err := e.widget(t, bug.Id, `{"body":"old"}`)
	if err != nil {
		t.Fatal(err)
	}
	w.Set("fields", []byte(`{"body":"flush"}`))
	if err := e.app.Save(w); err != nil {
		t.Fatal(err)
	}
	if got := e.storedFields(t, w.Id); got["body"] != "flush" {
		t.Fatalf("the room's flush must write: body = %v", got["body"])
	}
}

func TestHostHook_CreateRequestKeepsTheCollaborativeValue(t *testing.T) {
	e := newTestEnv(t)
	e.openWidgets(t)
	e.newForm(t, "widget", widgetDefault, true)
	bug := e.newForm(t, "bug", bugDef, false)
	e.request(t, http.MethodPost, "/api/collections/gizmos_widgets/records", `{"id":"wcreate00000001","name":"w","form":"`+bug.Id+`","fields":{"body":"first"}}`)
	if got := e.storedFields(t, "wcreate00000001"); got["body"] != "first" {
		t.Fatalf("create is not room-owned: body = %v", got["body"])
	}
}

// withSerial gives the fictional widget host a `serial` text column and a
// text built-in for it limited to 40 characters, and a default form with it.
func (e *env) withSerial(t *testing.T) {
	t.Helper()
	widgets, err := e.app.FindCollectionByNameOrId("gizmos_widgets")
	if err != nil {
		t.Fatal(err)
	}
	widgets.Fields.Add(&core.TextField{Name: "serial"})
	if err := e.app.Save(widgets); err != nil {
		t.Fatal(err)
	}
	limit := 40
	e.host.Columns = append(e.host.Columns, "serial")
	e.host.Builtins = append(e.host.Builtins, Builtin{Key: "serial", Label: "Serial", Kind: "text", Storage: "column", MaxLength: &limit})
	e.newForm(t, "widget", withField(widgetDefault, `{"key":"serial","kind":"builtin","page":"main","label":"Serial","width":"full","required":false,"hidden":false}`), true)
}

func (e *env) serialWidget(t *testing.T, serial string) (*core.Record, error) {
	t.Helper()
	col, err := e.app.FindCollectionByNameOrId("gizmos_widgets")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", "w")
	r.Set("serial", serial)
	return r, e.app.Save(r)
}

func TestHostHook_TextBuiltinMaxLengthRefusesALongerValue(t *testing.T) {
	e := newTestEnv(t)
	e.withSerial(t)
	_, err := e.serialWidget(t, strings.Repeat("x", 41))
	var verrs validation.Errors
	if !errors.As(err, &verrs) {
		t.Fatalf("want validation errors, got %v", err)
	}
	got, ok := verrs["serial"]
	if !ok || got.Error() != "Serial is too long" {
		t.Fatalf("want serial refused with %q, got %v", "Serial is too long", verrs)
	}
	if verr, ok := got.(validation.Error); !ok || verr.Code() != "validation_wrong_kind" {
		t.Fatalf("want code validation_wrong_kind, got %#v", got)
	}
	w, err := e.serialWidget(t, strings.Repeat("x", 40))
	if err != nil {
		t.Fatalf("40 characters must save, got %v", err)
	}
	stored, err := e.app.FindRecordById("gizmos_widgets", w.Id)
	if err != nil || stored.GetString("serial") != strings.Repeat("x", 40) {
		t.Fatalf("stored serial = %q, err %v", stored.GetString("serial"), err)
	}
}

func TestHostHook_TextBuiltinMaxLengthLeavesAnUnchangedStoredValue(t *testing.T) {
	e := newTestEnv(t)
	e.withSerial(t)
	w, err := e.serialWidget(t, "short")
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 41)
	if _, err := e.app.DB().NewQuery("UPDATE gizmos_widgets SET serial = {:v} WHERE id = {:id}").
		Bind(dbx.Params{"v": long, "id": w.Id}).Execute(); err != nil {
		t.Fatal(err)
	}
	w, err = e.app.FindRecordById("gizmos_widgets", w.Id)
	if err != nil || w.GetString("serial") != long {
		t.Fatalf("raw write did not land: %q %v", w.GetString("serial"), err)
	}
	w.Set("name", "renamed")
	if err := e.app.Save(w); err != nil {
		t.Fatalf("an update that leaves the stored serial unchanged must pass, got %v", err)
	}
	w.Set("serial", strings.Repeat("y", 41))
	if err := e.app.Save(w); err == nil || !strings.Contains(err.Error(), "serial") {
		t.Fatalf("changing serial to 41 characters must be refused, got %v", err)
	}
}
