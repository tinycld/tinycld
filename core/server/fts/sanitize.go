package fts

import (
	"regexp"
	"strings"
)

// fts5SpecialChars matches characters that have special meaning in FTS5 queries,
// plus the email punctuation `@` and `.`. The unicode61 tokenizer treats `@`/`.`
// as token boundaries when indexing, so an email like "a.b@example.com" is stored
// as the separate tokens [a, b, example, com]. If we leave `@`/`.` inside the
// *query* the whole address is wrapped as a single ordered phrase ("a@example.com"*),
// which only matches when those tokens are adjacent in that exact order — so a
// partial address that skips a token (e.g. "a@example.com" against stored
// "a.b@example.com") silently matches nothing. Splitting on `@`/`.` instead turns
// the query into independent AND-ed prefix terms, which matches regardless of the
// in-between tokens.
var fts5SpecialChars = regexp.MustCompile(`[":*^{}()\[\]~\-@.]`)

// SanitizeQuery escapes special FTS5 characters and turns the input into
// AND-ed prefix terms for safe search-as-you-type MATCH queries. Returns "" when
// the input has no usable terms.
func SanitizeQuery(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	cleaned := fts5SpecialChars.ReplaceAllString(input, " ")
	terms := strings.Fields(cleaned)
	if len(terms) == 0 {
		return ""
	}

	// Use prefix matches (term*) so search-as-you-type finds partial words
	// like "joh" -> "john", "johnson". FTS5's bare phrase syntax ("joh")
	// is an exact token match and would only fire when the user types a
	// whole indexed word. Each term is also quoted to keep any residual
	// punctuation from being interpreted as FTS5 operators.
	prefixed := make([]string, len(terms))
	for i, term := range terms {
		term = strings.ReplaceAll(term, `"`, `""`)
		prefixed[i] = `"` + term + `"*`
	}

	return strings.Join(prefixed, " ")
}

// htmlTagRegex strips HTML tags for plain-text indexing of editor fields.
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes HTML tags from a string for plain-text indexing.
func stripHTML(s string) string {
	return strings.TrimSpace(htmlTagRegex.ReplaceAllString(s, " "))
}
