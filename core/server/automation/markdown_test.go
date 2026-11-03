package automation

import "testing"

func TestEscapeMarkdown(t *testing.T) {
	cases := map[string]string{
		"plain words":                         "plain words",
		"![x](https://tracker.example/p.png)": `\!\[x\]\(https://tracker\.example/p\.png\)`,
		"# Heading":                           `\# Heading`,
		"*bold* and _it_":                     `\*bold\* and \_it\_`,
		"<img src=x>":                         `\<img src=x\>`,
		"a\\b":                                `a\\b`,
		"line one\nline two":                  "line one\nline two",
	}
	for in, want := range cases {
		if got := EscapeMarkdown(in); got != want {
			t.Errorf("EscapeMarkdown(%q) = %q, want %q", in, got, want)
		}
	}
}
