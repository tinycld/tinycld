package output

import "testing"

func TestStripMarks(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"no tags", "budget review", "budget review"},
		{"one highlight", "the <mark>budget</mark> review", "the budget review"},
		{
			"several highlights",
			"<mark>budget</mark> and <mark>draft</mark>",
			"budget and draft",
		},
		{"empty", "", ""},
		// Only the server's own <mark> tags are stripped. Other angle-bracket
		// text is record content — a file literally named "a<b>c" must survive
		// intact rather than being half-eaten by an over-broad pattern.
		{"leaves other markup alone", "a <b>c</b> d", "a <b>c</b> d"},
	}
	for _, tc := range cases {
		if got := StripMarks(tc.in); got != tc.want {
			t.Errorf("%s: StripMarks(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}
