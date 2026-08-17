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

// quoteTerms strips FTS5 operator characters and returns each remaining word
// as a quoted prefix term. Shared by the include and exclude paths so their
// escaping cannot drift apart — this is the trust boundary: everything past
// this function is validated FTS5 syntax, so both callers must go through it.
func quoteTerms(input string) []string {
	cleaned := fts5SpecialChars.ReplaceAllString(input, " ")
	terms := strings.Fields(cleaned)
	quoted := make([]string, len(terms))
	for i, term := range terms {
		term = strings.ReplaceAll(term, `"`, `""`)
		quoted[i] = `"` + term + `"*`
	}
	return quoted
}

// SanitizeQuery escapes special FTS5 characters and turns the input into
// AND-ed prefix terms for safe search-as-you-type MATCH queries. Returns "" when
// the input has no usable terms.
//
// Prefix matches (term*) let search-as-you-type find partial words like
// "joh" -> "john", "johnson". FTS5's bare phrase syntax ("joh") is an exact
// token match and would only fire when the user types a whole indexed word.
func SanitizeQuery(input string) string {
	return strings.Join(quoteTerms(input), " ")
}

// SanitizeQueryWithExclusions builds an FTS5 MATCH expression requiring every
// include term and rejecting every exclude term.
//
// Both sides may be raw, unsplit query-string values (the /api/{slug}/search
// route passes q.Get("q")/q.Get("not") straight through) — quoteTerms is the
// only trust boundary and must treat the input as hostile. An exclude-only
// query returns "" rather than a bare NOT, which FTS5 rejects: there is no
// result set to subtract from.
func SanitizeQueryWithExclusions(include, exclude string) string {
	base := SanitizeQuery(include)
	if base == "" {
		return ""
	}

	for _, term := range quoteTerms(exclude) {
		base += " NOT " + term
	}
	return base
}

// htmlTagRegex strips HTML tags for plain-text indexing of editor fields.
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// stripHTML removes HTML tags from a string for plain-text indexing.
func stripHTML(s string) string {
	return strings.TrimSpace(htmlTagRegex.ReplaceAllString(s, " "))
}
