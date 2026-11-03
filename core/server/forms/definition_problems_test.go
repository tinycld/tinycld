package forms

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func problemsOf(t *testing.T, e *env, raw string) map[string]string {
	t.Helper()
	def, err := ParseDefinition([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k, v := range ValidateDefinition(def, e.host) {
		out[k] = v.Error()
	}
	return out
}

// minimalValidFor is a one-page definition with every built-in of the host.
func minimalValidFor(t *testing.T, e *env) string {
	t.Helper()
	type field struct {
		Key   string `json:"key"`
		Kind  string `json:"kind"`
		Page  string `json:"page"`
		Label string `json:"label"`
	}
	fields := []field{}
	for _, b := range e.host.Builtins {
		fields = append(fields, field{b.Key, "builtin", "main", b.Label})
	}
	raw, err := json.Marshal(map[string]any{
		"version": 1, "pages": []map[string]string{{"key": "main", "title": "Main"}}, "fields": fields,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestValidateDefinitionReportsEveryProblem(t *testing.T) {
	e := newTestEnv(t)
	got := problemsOf(t, e, `{"version":1,"pages":[{"key":"main","title":"Main"}],"fields":[
		{"key":"Email","kind":"text","page":"main","label":"Email"},
		{"key":"phone","kind":"telephone","page":"main","label":"Phone"},
		{"key":"notes2","kind":"text","page":"extra","label":"Notes"}]}`)
	for _, path := range []string{"definition.fields.0.key", "definition.fields.1.kind", "definition.fields.2.page"} {
		if got[path] == "" {
			t.Errorf("no problem at %s; got %v", path, got)
		}
	}
	if !strings.Contains(got["definition.fields.0.key"], `"Email"`) || !strings.Contains(got["definition.fields.0.key"], "lowercase") {
		t.Errorf("the key message names the value and the rule: %q", got["definition.fields.0.key"])
	}
	if !strings.Contains(got["definition.fields.1.kind"], "text") {
		t.Errorf("the kind message lists the allowed kinds: %q", got["definition.fields.1.kind"])
	}
	if !strings.Contains(got["definition.fields.2.page"], "main") {
		t.Errorf("the page message lists the pages: %q", got["definition.fields.2.page"])
	}
}

func TestValidateDefinitionNamesAllowedOperators(t *testing.T) {
	e := newTestEnv(t)
	var def map[string]any
	_ = json.Unmarshal([]byte(minimalValidFor(t, e)), &def)
	fields := def["fields"].([]any)
	def["fields"] = append(fields, map[string]any{"key": "when", "kind": "date", "page": "main", "label": "When"})
	def["rules"] = []any{map[string]any{"key": "r1", "match": "all",
		"when":    []any{map[string]any{"field": "when", "op": "contains", "value": "x"}},
		"actions": []any{map[string]any{"action": "show", "field": "when"}}}}
	raw, _ := json.Marshal(def)
	got := problemsOf(t, e, string(raw))
	msg := got["definition.rules.0.when.0.op"]
	if !strings.Contains(msg, "contains is not valid for a date field") || !strings.Contains(msg, "before") {
		t.Errorf("op message = %q (all: %v)", msg, got)
	}
}

func TestValidateDefinitionValidOneIsNil(t *testing.T) {
	e := newTestEnv(t)
	if got := problemsOf(t, e, minimalValidFor(t, e)); len(got) != 0 {
		t.Fatalf("a valid definition has no problems: %v", got)
	}
}

func withDef(t *testing.T, e *env, edit func(def map[string]any)) string {
	t.Helper()
	var def map[string]any
	if err := json.Unmarshal([]byte(minimalValidFor(t, e)), &def); err != nil {
		t.Fatal(err)
	}
	edit(def)
	raw, _ := json.Marshal(def)
	return string(raw)
}

// A required built-in that is present but hidden is one problem, not also a
// "missing built-in field".
func TestValidateDefinitionHiddenBuiltinIsNotAlsoMissing(t *testing.T) {
	e := newTestEnv(t)
	var required string
	for _, b := range e.host.Builtins {
		if b.Required {
			required = b.Key
			break
		}
	}
	if required == "" {
		t.Skip("the test host has no required built-in")
	}
	got := problemsOf(t, e, withDef(t, e, func(def map[string]any) {
		for _, f := range def["fields"].([]any) {
			if fm := f.(map[string]any); fm["key"] == required {
				fm["hidden"] = true
			}
		}
	}))
	if _, ok := got["definition.fields"]; ok {
		t.Fatalf("a hidden built-in is reported as missing too: %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("want only the hidden problem, got %v", got)
	}
}

// A rule that names a declared field whose own definition is wrong reports
// the field's problem only, not "not a field of this form".
func TestValidateDefinitionReferenceToAnInvalidFieldIsNotAProblem(t *testing.T) {
	e := newTestEnv(t)
	got := problemsOf(t, e, withDef(t, e, func(def map[string]any) {
		def["fields"] = append(def["fields"].([]any), map[string]any{"key": "phone", "kind": "telephone", "page": "main", "label": "Phone"})
		def["rules"] = []any{map[string]any{"key": "r1", "match": "all",
			"when":    []any{map[string]any{"field": "phone", "op": "is_empty"}},
			"actions": []any{map[string]any{"action": "require", "field": "phone"}}}}
		def["defaults"] = map[string]any{"phone": "x"}
		def["submit"] = map[string]any{"required": []any{"phone"}}
	}))
	if len(got) != 1 {
		t.Fatalf("want only the kind problem, got %v", got)
	}
	for k := range got {
		if !strings.HasSuffix(k, ".kind") {
			t.Fatalf("got %v", got)
		}
	}
}

// A cross-field problem keeps the path of the field it is about, even when
// an earlier field has its own problem.
func TestValidateDefinitionCrossFieldPathsKeepTheirIndex(t *testing.T) {
	e := newTestEnv(t)
	var at int
	got := problemsOf(t, e, withDef(t, e, func(def map[string]any) {
		fields := def["fields"].([]any)
		fields = append(fields, map[string]any{"key": "Bad", "kind": "text", "page": "main", "label": "Bad"})
		at = len(fields)
		fields = append(fields, map[string]any{"key": "total", "kind": "computed", "page": "main", "label": "Total",
			"decimals": 0, "expression": map[string]any{"mul": []any{map[string]any{"var": "ghost"}, 2}}})
		def["fields"] = fields
	}))
	want := "definition.fields." + strconv.Itoa(at) + ".expression"
	if got[want] == "" {
		t.Fatalf("no problem at %s; got %v", want, got)
	}
}

// The remaining refusals also say what is allowed.
func TestValidateDefinitionMessagesNameTheAllowedSet(t *testing.T) {
	e := newTestEnv(t)
	cases := []struct {
		name, want string
		edit       func(def map[string]any)
	}{
		{"rule action on an unknown field", "Fields: ", func(def map[string]any) {
			def["rules"] = []any{map[string]any{"key": "r1", "match": "all",
				"when":    []any{map[string]any{"field": "name", "op": "is_empty"}},
				"actions": []any{map[string]any{"action": "require", "field": "ghost"}}}}
		}},
		{"rule action on an unknown page", "Pages: main", func(def map[string]any) {
			def["rules"] = []any{map[string]any{"key": "r1", "match": "all",
				"when":    []any{map[string]any{"field": "name", "op": "is_empty"}},
				"actions": []any{map[string]any{"action": "show", "page": "nope"}}}}
		}},
		{"unknown column kind", "Kinds: ", func(def map[string]any) {
			def["fields"] = append(def["fields"].([]any), map[string]any{"key": "lines", "kind": "table", "page": "main", "label": "Lines",
				"columns": []any{map[string]any{"key": "c", "kind": "colour", "label": "C"}}})
		}},
		{"formula on an unknown field", "Fields: ", func(def map[string]any) {
			def["fields"] = append(def["fields"].([]any), map[string]any{"key": "total", "kind": "computed", "page": "main", "label": "Total",
				"decimals": 0, "expression": map[string]any{"mul": []any{map[string]any{"var": "ghost"}, 2}}})
		}},
		{"decimals out of range", "Use 0 to 6", func(def map[string]any) {
			def["fields"] = append(def["fields"].([]any), map[string]any{"key": "total", "kind": "computed", "page": "main", "label": "Total",
				"decimals": 9, "expression": 1})
		}},
	}
	for _, c := range cases {
		got := problemsOf(t, e, withDef(t, e, c.edit))
		found := false
		for _, msg := range got {
			if strings.Contains(msg, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no message contains %q: %v", c.name, c.want, got)
		}
	}
}

// An unknown kind lists the kinds this host allows, not every kind, so the
// next try is not refused for a kind the host does not take.
func TestValidateDefinitionUnknownKindListsTheHostKinds(t *testing.T) {
	host := &Host{Key: "gizmos:narrow", Label: "Narrow", Kinds: []string{"text", "number"}}
	def, err := ParseDefinition([]byte(`{"version":1,"pages":[{"key":"main","title":"Main"}],"fields":[{"key":"a","kind":"telephone","page":"main","label":"A"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	msg := ValidateDefinition(def, host)["definition.fields.0.kind"].Error()
	if !strings.Contains(msg, "Kinds: text, number") || strings.Contains(msg, "markdown") {
		t.Fatalf("msg = %q", msg)
	}
}
