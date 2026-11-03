package automation

import (
	"testing"

	"tinycld.org/core/markdown"
)

func TestEscapeMarkdown(t *testing.T) {
	cases := map[string]string{
		"plain words":                         "plain words",
		"![x](https://tracker.example/p.png)": `\!\[x\]\(https\://tracker\.example/p\.png\)`,
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

// An escaped value parses back to exactly the text it was, with no mark: no
// image, no link, and no autolink of a bare address.
func TestEscapedMarkdownReadsAsPlainText(t *testing.T) {
	for _, in := range []string{
		"![x](https://tracker.example/p.png)",
		"[click](javascript:alert(1))",
		"see https://tracker.example/p.png now",
		"see www.tracker.example now",
		"write to a@tracker.example",
		"<https://tracker.example>",
	} {
		pm := markdown.ToPM(EscapeMarkdown(in))
		para := pm.Content[0]
		if len(para.Content) != 1 || para.Content[0].Text != in || len(para.Content[0].Marks) != 0 {
			t.Errorf("%q parses to %+v, want one unmarked text node", in, para.Content)
		}
	}
}
