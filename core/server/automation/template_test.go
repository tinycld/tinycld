// tinycld/core/server/automation/template_test.go
package automation

import "testing"

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
