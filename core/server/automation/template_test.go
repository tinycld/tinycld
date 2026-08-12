// tinycld/core/server/automation/template_test.go
package automation

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestSubstituteTemplates(t *testing.T) {
	_, r := evalRecord(t)
	trig := TriggerDef{Fields: []FieldRef{{Key: "subject"}, {Key: "sender"}}}

	cases := []struct{ in, want string }{
		{"Re: {{subject}}", "Re: Invoice #42 attached"},
		{"{{ subject }} from {{sender}}", "Invoice #42 attached from billing@ACME.com"},
		{"{{size}}", ""},        // not exposed by the trigger's allowlist
		{"{{nonexistent}}", ""}, // unknown field
		{"no placeholders", "no placeholders"},
		{"{{subject", "{{subject"}, // unterminated stays verbatim
	}
	for _, tc := range cases {
		if got := SubstituteTemplates(tc.in, r, trig); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}

	// Empty Fields = all non-system columns exposed.
	open := TriggerDef{}
	if got := SubstituteTemplates("{{size}}", r, open); got != "1500" {
		t.Fatalf("open trigger exposes all columns, got %q", got)
	}
	if got := SubstituteTemplates("{{subject}}", nil, trig); got != "{{subject}}" {
		t.Fatalf("nil record leaves input unchanged, got %q", got)
	}
}

func TestSystemAndHiddenFieldsNeverSubstitute(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	u, err := app.FindFirstRecordByFilter("users", "id != ''")
	if err != nil {
		t.Fatal(err)
	}
	// Open trigger (all columns): hidden/system auth fields must never substitute,
	// even if they have values. They should become empty strings.
	got := SubstituteTemplates("{{tokenKey}}x{{password}}", u, TriggerDef{})
	if got != "x" {
		t.Fatalf("system/hidden fields must not substitute, got %q want 'x'", got)
	}
	// Even an explicit allowlist naming system/hidden fields must not leak.
	curated := TriggerDef{Fields: []FieldRef{{Key: "tokenKey"}, {Key: "name"}}}
	gotAllowlist := SubstituteTemplates("{{tokenKey}}|{{name}}", u, curated)
	// tokenKey should have been filtered out (it's system/hidden), so it becomes empty.
	if !strings.HasPrefix(gotAllowlist, "|") {
		t.Fatalf("tokenKey (system field) must not substitute in curated allowlist, got %q", gotAllowlist)
	}
	// The name field should still substitute (it's a regular field).
	expectedName := u.GetString("name")
	if !strings.Contains(gotAllowlist, expectedName) {
		t.Fatalf("name field should substitute in allowlist, got %q", gotAllowlist)
	}
}
