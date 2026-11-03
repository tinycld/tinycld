package forms

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// errorKeys returns the field keys of a hook's validation error.
func errorKeys(t *testing.T, err error) []string {
	t.Helper()
	var verrs validation.Errors
	if !errors.As(err, &verrs) {
		t.Fatalf("want validation errors, got %T %v", err, err)
	}
	keys := make([]string, 0, len(verrs))
	for k := range verrs {
		keys = append(keys, k)
	}
	return keys
}

func TestFormsHook_KeysErrorsByTheirPathInTheDefinition(t *testing.T) {
	e := newTestEnv(t)
	cases := map[string]string{
		"definition.fields.5.key": withField(widgetDefault, `{"key":"position","kind":"text","page":"main","label":"P","width":"full","required":false,"hidden":false}`),
		"definition.rules.0.when.0.op": strings.Replace(widgetDefault, `"rules":[]`,
			`"rules":[{"key":"r","match":"all","when":[{"field":"due","op":"contains","value":"x"}],"actions":[{"action":"hide","field":"owner"}]}]`, 1),
		"definition.rules.0.actions.0.field": strings.Replace(widgetDefault, `"rules":[]`,
			`"rules":[{"key":"r","match":"all","when":[{"field":"due","op":"is_empty"}],"actions":[{"action":"hide","field":"ghost"}]}]`, 1),
		"definition.fields":            strings.Replace(widgetDefault, `{"key":"due","kind":"builtin","page":"main","label":"Due","width":"full","required":false,"hidden":false},`, "", 1),
		"definition.defaults.due":      strings.Replace(widgetDefault, `"defaults":{}`, `"defaults":{"due":{"var":"x"}}`, 1),
		"definition.submit.required.0": strings.Replace(widgetDefault, `"submit":{"required":[]}`, `"submit":{"required":["ghost"]}`, 1),
	}
	for want, def := range cases {
		err := e.saveFormErr(t, "bad", def)
		if keys := errorKeys(t, err); len(keys) != 1 || keys[0] != want {
			t.Errorf("want key %s, got %v (%v)", want, keys, err)
		}
	}
}

func TestFormsHook_DefaultAndSetValueExpressions(t *testing.T) {
	e := newTestEnv(t)
	withDefault := func(expr string) string {
		return strings.Replace(widgetDefault, `"defaults":{}`, `"defaults":{"due":`+expr+`}`, 1)
	}
	withSetValue := func(expr string) string {
		return strings.Replace(widgetDefault, `"rules":[]`,
			`"rules":[{"key":"r","match":"all","when":[{"field":"name","op":"is_empty"}],"actions":[{"action":"set_value","field":"due","value":`+expr+`}]}]`, 1)
	}
	valid := []string{`"2026-01-31"`, `3`, `true`, `null`, `["a","b"]`, `{"today":[]}`, `{"me":[]}`,
		`{"addDays":[{"today":[]},21]}`, `{"addDays":["2026-01-31",-3]}`}
	invalid := []string{`{"var":"x"}`, `{"today":[1]}`, `{"addDays":[{"me":[]},2]}`, `{"addDays":[{"today":[]},1.5]}`,
		`{"addDays":["soon",2]}`, `{"today":[],"me":[]}`, `[1]`, `{"addDays":[{"addDays":[{"today":[]},1]},1]}`}
	for i, expr := range valid {
		if err := e.saveFormErr(t, fmt.Sprintf("d%d", i), withDefault(expr)); err != nil {
			t.Errorf("default %s: %v", expr, err)
		}
		if err := e.saveFormErr(t, fmt.Sprintf("s%d", i), withSetValue(expr)); err != nil {
			t.Errorf("set_value %s: %v", expr, err)
		}
	}
	for _, expr := range invalid {
		if err := e.saveFormErr(t, "bad", withDefault(expr)); err == nil || !strings.Contains(err.Error(), "supported expression") {
			t.Errorf("default %s must be refused, got %v", expr, err)
		}
		if err := e.saveFormErr(t, "bad", withSetValue(expr)); err == nil || !strings.Contains(err.Error(), "supported expression") {
			t.Errorf("set_value %s must be refused, got %v", expr, err)
		}
	}
}

func TestFormsHook_RejectsARuleThatHidesARequiredBuiltin(t *testing.T) {
	e := newTestEnv(t)
	hideField := strings.Replace(widgetDefault, `"rules":[]`,
		`"rules":[{"key":"r","match":"all","when":[{"field":"due","op":"is_empty"}],"actions":[{"action":"hide","field":"name"}]}]`, 1)
	if err := e.saveFormErr(t, "bad", hideField); err == nil || !strings.Contains(err.Error(), "cannot hide required field name") {
		t.Fatalf("got %v", err)
	}
	twoPages := strings.Replace(widgetDefault, `"pages":[{"key":"main","title":"Widget"}]`, `"pages":[{"key":"main","title":"Widget"},{"key":"more","title":"More"}]`, 1)
	hidePage := strings.Replace(twoPages, `"rules":[]`,
		`"rules":[{"key":"r","match":"all","when":[{"field":"due","op":"is_empty"}],"actions":[{"action":"hide","page":"main"}]}]`, 1)
	if err := e.saveFormErr(t, "bad", hidePage); err == nil || !strings.Contains(err.Error(), "page main holds required field name") {
		t.Fatalf("got %v", err)
	}
	hideOther := strings.Replace(twoPages, `"rules":[]`,
		`"rules":[{"key":"r","match":"all","when":[{"field":"due","op":"is_empty"}],"actions":[{"action":"hide","page":"more"},{"action":"hide","field":"owner"}]}]`, 1)
	if err := e.saveFormErr(t, "ok", hideOther); err != nil {
		t.Fatalf("hiding a page or field with no required built-in is fine: %v", err)
	}
}

func TestFormsHook_ValidatesTheDefinitionOnUpdate(t *testing.T) {
	e := newTestEnv(t)
	f := e.newForm(t, "widget", widgetDefault, true)
	f.Set("definition", []byte(strings.Replace(widgetDefault, `"version":1`, `"version":2`, 1)))
	if err := e.app.Save(f); err == nil || !strings.Contains(err.Error(), "version 2 is not supported") {
		t.Fatalf("an invalid definition update must be refused, got %v", err)
	}
}

func TestValidateDefinition_HostKinds(t *testing.T) {
	host := &Host{Key: "gizmos:note", Label: "Note", Kinds: []string{"text", "file"}}
	def, err := ParseDefinition([]byte(`{"version":1,"pages":[{"key":"main","title":"Main"}],` +
		`"fields":[{"key":"who","page":"main","label":"Who","kind":"user"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	errs := ValidateDefinition(def, host)
	if errs == nil || !strings.Contains(errs.Error(), "not allowed") {
		t.Fatalf("want kind rejection, got %v", errs)
	}
}

func TestParseDefinition_FileLimits(t *testing.T) {
	def, err := ParseDefinition([]byte(`{"version":1,"pages":[{"key":"main","title":"Main"}],` +
		`"fields":[{"key":"cv","page":"main","label":"CV","kind":"file","accept":["application/pdf"],"maxSizeMb":5}]}`))
	if err != nil {
		t.Fatal(err)
	}
	f := def.Field("cv")
	if len(f.Accept) != 1 || f.Accept[0] != "application/pdf" || f.MaxSizeMb == nil || *f.MaxSizeMb != 5 {
		t.Fatalf("file limits not decoded: %+v", f)
	}
}
