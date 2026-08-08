package output

import "regexp"

var markTags = regexp.MustCompile(`</?mark>`)

// StripMarks removes the server's <mark> highlight tags so a match snippet
// reads cleanly in a terminal. Only the tabular formats call it: --json emits
// the raw response, tags included, because a scripted caller may want to know
// which span matched.
//
// Shared here rather than per-package: mail and drive each carried an identical
// private copy, and every package contributing a highlighted search row needs
// the same behavior.
func StripMarks(s string) string {
	return markTags.ReplaceAllString(s, "")
}
