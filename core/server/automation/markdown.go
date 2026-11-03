package automation

import "strings"

// markdownPunctuation is every ASCII character CommonMark lets a backslash
// escape that can start markup: emphasis, links, images, headings, lists,
// tables, HTML and code.
const markdownPunctuation = "\\`*_{}[]()#+-.!|<>~"

// EscapeMarkdown makes a value read as the text it is when put into
// markdown: a visitor's "![x](https://…)" must not become an image the
// reader's browser fetches. Line breaks stay.
func EscapeMarkdown(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 128 && strings.ContainsRune(markdownPunctuation, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
