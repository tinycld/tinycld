package forms

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/rlstest"
)

type env struct {
	app  *tests.TestApp
	defs *Defs
	host *Host
}

// newTestEnv builds the fictional gizmos_widgets host collection, applies
// core's migrations (for `forms`) and binds the forms hooks.
func newTestEnv(t *testing.T) *env {
	t.Helper()
	app := rlstest.NewAssembledApp(t)
	forms, err := app.FindCollectionByNameOrId("forms")
	if err != nil {
		t.Fatal(err)
	}
	widgets := core.NewBaseCollection("gizmos_widgets")
	widgets.Fields.Add(
		&core.TextField{Name: "name", Required: true, Max: 200},
		&core.TextField{Name: "notes"},
		&core.DateField{Name: "due"},
		&core.TextField{Name: "owner"},
		&core.TextField{Name: "position"},
		&core.RelationField{Name: "form", CollectionId: forms.Id, MaxSelect: 1},
		&core.JSONField{Name: "fields", MaxSize: 100000},
	)
	if err := app.Save(widgets); err != nil {
		t.Fatal(err)
	}
	defs, err := LoadHosts(filepath.Join("testdata", "form_hosts.json"))
	if err != nil {
		t.Fatal(err)
	}
	BindHooks(app, defs)
	return &env{app: app, defs: defs, host: defs.Host("gizmos:widget")}
}

func (e *env) newForm(t *testing.T, slug string, definition string, isDefault bool) *core.Record {
	t.Helper()
	col, _ := e.app.FindCollectionByNameOrId("forms")
	r := core.NewRecord(col)
	r.Set("host", e.host.Key)
	r.Set("name", slug)
	r.Set("slug", slug)
	r.Set("definition", []byte(definition))
	r.Set("is_default", isDefault)
	r.Set("order", 1)
	if err := e.app.Save(r); err != nil {
		t.Fatalf("save form %s: %v", slug, err)
	}
	return r
}

func (e *env) saveFormErr(t *testing.T, slug string, definition string) error {
	t.Helper()
	col, _ := e.app.FindCollectionByNameOrId("forms")
	r := core.NewRecord(col)
	r.Set("host", e.host.Key)
	r.Set("name", slug)
	r.Set("slug", slug)
	r.Set("definition", []byte(definition))
	r.Set("order", 1)
	return e.app.Save(r)
}

const widgetDefault = `{"version":1,"pages":[{"key":"main","title":"Widget"}],"fields":[
{"key":"name","kind":"builtin","page":"main","label":"Name","width":"full","required":true,"hidden":false},
{"key":"notes","kind":"builtin","page":"main","label":"Notes","width":"full","required":false,"hidden":false},
{"key":"due","kind":"builtin","page":"main","label":"Due","width":"full","required":false,"hidden":false},
{"key":"owner","kind":"builtin","page":"main","label":"Owner","width":"full","required":false,"hidden":false},
{"key":"tasks","kind":"builtin","page":"main","label":"Tasks","width":"full","required":false,"hidden":false},
{"key":"comments","kind":"builtin","page":"main","label":"Comments","width":"full","required":false,"hidden":false}
],"rules":[],"defaults":{},"submit":{"required":[]}}`

func withField(def, field string) string {
	return strings.Replace(def, `{"key":"comments"`, field+`,{"key":"comments"`, 1)
}

func TestFormsHook_AcceptsTheDefaultDefinition(t *testing.T) {
	e := newTestEnv(t)
	e.newForm(t, "widget", widgetDefault, true)
}

func TestFormsHook_RejectsUnknownHost(t *testing.T) {
	e := newTestEnv(t)
	col, _ := e.app.FindCollectionByNameOrId("forms")
	r := core.NewRecord(col)
	r.Set("host", "gizmos:ghost")
	r.Set("name", "x")
	r.Set("slug", "x")
	r.Set("definition", []byte(widgetDefault))
	err := e.app.Save(r)
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("want a host error, got %v", err)
	}
}

func TestFormsHook_RejectsMissingBuiltin(t *testing.T) {
	e := newTestEnv(t)
	def := strings.Replace(widgetDefault, `{"key":"due","kind":"builtin","page":"main","label":"Due","width":"full","required":false,"hidden":false},`, "", 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "missing built-in field due") {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_RejectsCustomKeyThatIsAColumn(t *testing.T) {
	e := newTestEnv(t)
	def := withField(widgetDefault, `{"key":"position","kind":"text","page":"main","label":"P","width":"full","required":false,"hidden":false}`)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "position is a column") {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_RejectsDuplicateOptionValue(t *testing.T) {
	e := newTestEnv(t)
	def := withField(widgetDefault, `{"key":"severity","kind":"select","page":"main","label":"Severity","width":"full","required":false,"hidden":false,"display":"dropdown","multiple":false,"options":[{"value":"low","label":"Low"},{"value":"low","label":"Also low"}]}`)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "duplicate option value low") {
		t.Fatalf("want a duplicate option error, got %v", err)
	}
	var verrs validation.Errors
	if !errors.As(err, &verrs) {
		t.Fatalf("want validation errors, got %T", err)
	}
	if _, ok := verrs["definition.fields.5.options.1.value"]; !ok {
		t.Fatalf("want the error at definition.fields.5.options.1.value, got %v", verrs)
	}
}

func TestFormsHook_RejectsDuplicateSetOptionsValue(t *testing.T) {
	e := newTestEnv(t)
	def := strings.Replace(withField(widgetDefault, `{"key":"severity","kind":"select","page":"main","label":"Severity","width":"full","required":false,"hidden":false,"display":"dropdown","multiple":false,"options":[{"value":"low","label":"Low"}]}`),
		`"rules":[]`,
		`"rules":[{"key":"r","match":"all","when":[{"field":"due","op":"is_empty"}],"actions":[{"action":"set_options","field":"severity","options":[{"value":"a","label":"A"},{"value":"a","label":"Also A"}]}]}]`, 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "duplicate option value a") {
		t.Fatalf("want a duplicate option error, got %v", err)
	}
	if keys := errorKeys(t, err); len(keys) != 1 || keys[0] != "definition.rules.0.actions.0.options.1.value" {
		t.Fatalf("want the error at the second option's value, got %v", keys)
	}
}

func TestFormsHook_RejectsMinimumAboveMaximum(t *testing.T) {
	e := newTestEnv(t)
	number := func(min, max string) string {
		return withField(widgetDefault, `{"key":"count","kind":"number","page":"main","label":"Count","width":"full","required":false,"hidden":false,"min":`+min+`,"max":`+max+`}`)
	}
	err := e.saveFormErr(t, "bad", number("5", "2"))
	if err == nil || !strings.Contains(err.Error(), "minimum 5 is above maximum 2") {
		t.Fatalf("want a min/max error, got %v", err)
	}
	if keys := errorKeys(t, err); len(keys) != 1 || keys[0] != "definition.fields.5.min" {
		t.Fatalf("want the error at the field's minimum, got %v", keys)
	}
	if err := e.saveFormErr(t, "ok", number("2", "2")); err != nil {
		t.Fatalf("an equal minimum and maximum is fine: %v", err)
	}
}

func TestFormsHook_RejectsHiddenRequiredBuiltin(t *testing.T) {
	e := newTestEnv(t)
	def := strings.Replace(widgetDefault, `"label":"Name","width":"full","required":true,"hidden":false`, `"label":"Name","width":"full","required":true,"hidden":true`, 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "cannot be hidden") {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_RejectsRuleOnMissingField(t *testing.T) {
	e := newTestEnv(t)
	def := strings.Replace(widgetDefault, `"rules":[]`, `"rules":[{"key":"r","match":"all","when":[{"field":"ghost","op":"is","value":"x"}],"actions":[{"action":"hide","field":"due"}]}]`, 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), `"ghost" is not a field of this form`) {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_RejectsBadOperatorForKind(t *testing.T) {
	e := newTestEnv(t)
	def := strings.Replace(widgetDefault, `"rules":[]`, `"rules":[{"key":"r","match":"all","when":[{"field":"due","op":"contains","value":"x"}],"actions":[{"action":"hide","field":"owner"}]}]`, 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "contains is not valid for a date field") {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_OneDefaultPerHost(t *testing.T) {
	e := newTestEnv(t)
	a := e.newForm(t, "a", widgetDefault, true)
	b := e.newForm(t, "b", widgetDefault, true)
	a, _ = e.app.FindRecordById("forms", a.Id)
	if a.GetBool("is_default") {
		t.Fatal("setting b as default must clear a")
	}
	b.Set("is_default", false)
	if err := e.app.Save(b); err == nil || !strings.Contains(err.Error(), "another form") {
		t.Fatalf("unsetting the only default must be refused, got %v", err)
	}
}

func TestFormsHook_DeleteGuards(t *testing.T) {
	e := newTestEnv(t)
	a := e.newForm(t, "a", widgetDefault, true)
	if err := e.app.Delete(a); err == nil || !strings.Contains(err.Error(), "default") {
		t.Fatalf("deleting the default must be refused, got %v", err)
	}
	b := e.newForm(t, "b", widgetDefault, false)
	widgets, _ := e.app.FindCollectionByNameOrId("gizmos_widgets")
	w := core.NewRecord(widgets)
	w.Set("name", "w")
	w.Set("form", b.Id)
	if err := e.app.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Delete(b); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("deleting a form with records must be refused, got %v", err)
	}
	if err := e.app.Delete(w); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Delete(b); err != nil {
		t.Fatalf("an unused non-default form deletes: %v", err)
	}
}

func TestFormsHook_RejectsDuplicateRuleKey(t *testing.T) {
	e := newTestEnv(t)
	rule := `{"key":"r","match":"all","when":[{"field":"name","op":"is_empty"}],"actions":[{"action":"hide","field":"due"}]}`
	def := strings.Replace(widgetDefault, `"rules":[]`, `"rules":[`+rule+`,`+rule+`]`, 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), `rule key "r" is used twice`) {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_RejectsNonTextContainsValue(t *testing.T) {
	e := newTestEnv(t)
	def := strings.Replace(widgetDefault, `"rules":[]`, `"rules":[{"key":"r","match":"all","when":[{"field":"name","op":"contains","value":5}],"actions":[{"action":"hide","field":"due"}]}]`, 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "contains needs a text value") {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_RejectsNonTextStartsWithValue(t *testing.T) {
	e := newTestEnv(t)
	def := strings.Replace(widgetDefault, `"rules":[]`, `"rules":[{"key":"r","match":"all","when":[{"field":"name","op":"starts_with"}],"actions":[{"action":"hide","field":"due"}]}]`, 1)
	err := e.saveFormErr(t, "bad", def)
	if err == nil || !strings.Contains(err.Error(), "starts_with needs a text value") {
		t.Fatalf("got %v", err)
	}
}

func TestFormsHook_RejectsBadIsAnyOfValue(t *testing.T) {
	e := newTestEnv(t)
	def := withField(widgetDefault, `{"key":"sev","kind":"select","page":"main","label":"S","width":"full","required":false,"hidden":false,"options":[{"value":"a","label":"A"}]}`)
	for _, v := range []string{`[]`, `"a"`, `[1]`} {
		bad := strings.Replace(def, `"rules":[]`, `"rules":[{"key":"r","match":"all","when":[{"field":"sev","op":"is_any_of","value":`+v+`}],"actions":[{"action":"hide","field":"due"}]}]`, 1)
		err := e.saveFormErr(t, "bad", bad)
		if err == nil || !strings.Contains(err.Error(), "is_any_of needs a list") {
			t.Fatalf("value %s: got %v", v, err)
		}
	}
}

func TestFormsHook_SettlingSiblingFiresUpdateHook(t *testing.T) {
	e := newTestEnv(t)
	calls := 0
	e.app.OnRecordUpdate("forms").BindFunc(func(ev *core.RecordEvent) error {
		calls++
		return ev.Next()
	})
	a := e.newForm(t, "a", widgetDefault, true)
	e.newForm(t, "b", widgetDefault, true)
	if calls < 1 {
		t.Fatalf("sibling clear must fire the update hook, calls=%d", calls)
	}
	a, _ = e.app.FindRecordById("forms", a.Id)
	if a.GetBool("is_default") {
		t.Fatal("a must no longer be default")
	}
}

func TestFormsHook_DeleteToleratesAbsentHostCollection(t *testing.T) {
	e := newTestEnv(t)
	if _, err := e.app.FindCollectionByNameOrId("gizmos_others"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want an absent collection (sql.ErrNoRows), got %v", err)
	}
	col, _ := e.app.FindCollectionByNameOrId("forms")
	r := core.NewRecord(col)
	r.Set("host", "gizmos:other")
	r.Set("name", "o")
	r.Set("slug", "o")
	r.Set("definition", []byte(`{"version":1,"pages":[{"key":"main","title":"M"}],"fields":[{"key":"name","kind":"builtin","page":"main","label":"Name","width":"full","required":true,"hidden":false}],"rules":[],"defaults":{},"submit":{"required":[]}}`))
	r.Set("order", 1)
	if err := e.app.Save(r); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Delete(r); err != nil {
		t.Fatalf("delete with absent host collection: %v", err)
	}
}

func TestFormsHook_HostCannotChange(t *testing.T) {
	e := newTestEnv(t)
	f := e.newForm(t, "widget", widgetDefault, false)
	f.Set("host", "gizmos:other")
	err := e.app.Save(f)
	if err == nil {
		t.Fatal("moving a form to another host must be refused")
	}
	if keys := errorKeys(t, err); len(keys) != 1 || keys[0] != "host" {
		t.Fatalf("want a host error, got %v", err)
	}
}

func TestFormsHook_DefaultCannotBeArchived(t *testing.T) {
	e := newTestEnv(t)
	f := e.newForm(t, "widget", widgetDefault, true)
	f.Set("archived", true)
	err := e.app.Save(f)
	if err == nil {
		t.Fatal("archiving the default must be refused")
	}
	if keys := errorKeys(t, err); len(keys) != 1 || keys[0] != "archived" {
		t.Fatalf("want an archived error, got %v", err)
	}
	other := e.newForm(t, "other", widgetDefault, false)
	other.Set("archived", true)
	if err := e.app.Save(other); err != nil {
		t.Fatalf("a non-default form archives: %v", err)
	}
}

func TestFormsHook_SettlingSkipsTheSiblingsDefinition(t *testing.T) {
	e := newTestEnv(t)
	a := e.newForm(t, "a", widgetDefault, true)
	// The host gains a built-in after a was saved, so a's stored definition no
	// longer validates; clearing its default flag must still go through.
	e.withBuiltins(t, append(append([]Builtin{}, e.host.Builtins...), Builtin{Key: "extra", Label: "Extra", Kind: "text", Storage: "column"}), func() {
		def := withField(widgetDefault, `{"key":"extra","kind":"builtin","page":"main","label":"Extra","width":"full","required":false,"hidden":false}`)
		e.newForm(t, "b", def, true)
		a, _ = e.app.FindRecordById("forms", a.Id)
		if a.GetBool("is_default") {
			t.Fatal("a must no longer be default")
		}
	})
}

func TestFormsHook_AFailedSettleRollsBackTheSave(t *testing.T) {
	e := newTestEnv(t)
	a := e.newForm(t, "a", widgetDefault, true)
	e.app.OnRecordUpdate("forms").BindFunc(func(ev *core.RecordEvent) error {
		if isSettling(ev.Context) {
			return errors.New("sibling clear failed")
		}
		return ev.Next()
	})
	err := e.saveFormErr(t, "b", widgetDefault)
	if err != nil {
		t.Fatalf("a non-default save does not settle: %v", err)
	}
	b, _ := e.app.FindFirstRecordByFilter("forms", "slug = 'b'")
	b.Set("is_default", true)
	if err := e.app.Save(b); err == nil || !strings.Contains(err.Error(), "sibling clear failed") {
		t.Fatalf("want the settle error, got %v", err)
	}
	b, _ = e.app.FindRecordById("forms", b.Id)
	a, _ = e.app.FindRecordById("forms", a.Id)
	if b.GetBool("is_default") || !a.GetBool("is_default") {
		t.Fatalf("the failed settle must roll back: a=%v b=%v", a.GetBool("is_default"), b.GetBool("is_default"))
	}
	col, _ := e.app.FindCollectionByNameOrId("forms")
	c := core.NewRecord(col)
	c.Set("host", e.host.Key)
	c.Set("name", "c")
	c.Set("slug", "c")
	c.Set("definition", []byte(widgetDefault))
	c.Set("is_default", true)
	if err := e.app.Save(c); err == nil {
		t.Fatal("want the settle error on create")
	}
	if _, err := e.app.FindFirstRecordByFilter("forms", "slug = 'c'"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("a create whose settle failed must not persist, got %v", err)
	}
}

// withBuiltins swaps the host's declared built-ins for the length of a step:
// the hooks hold the same *Host, so a form saved inside the step is checked
// against the swapped list, the way it was when an older package version
// declared it.
func (e *env) withBuiltins(t *testing.T, builtins []Builtin, step func()) {
	t.Helper()
	saved := e.host.Builtins
	e.host.Builtins = builtins
	defer func() { e.host.Builtins = saved }()
	step()
}
