package forms

import (
	"encoding/json"
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
