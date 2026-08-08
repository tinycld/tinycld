package main

import (
	"regexp"
	"strings"
)

// A Go port of core/lib/search/parse-query.ts. Both implementations are pinned
// to testdata/query-grammar.json so `tinycld search "mail: budget -draft"`
// resolves to the same chips and terms the web palette would produce.
//
// The web parser additionally returns a `remainder` used to render its chip
// input box. Nothing in a terminal consumes it, so it is not ported and not
// part of the shared fixture.

// Grouping and quoting characters, stripped per-token rather than supported:
// every backend already strips them, and passing a half-typed expression
// through to FTS5 turns it into a parse error rather than a search.
var operatorChars = regexp.MustCompile(`[&|!"'()]`)

// Bare boolean operator words are dropped only when a token is EXACTLY one of
// these — never a substring — so a hyphenated literal like
// "plan-NOT-final.docx" keeps NOT embedded and literal.
var operatorWord = regexp.MustCompile(`^(AND|OR|NOT)$`)

var leadingHyphens = regexp.MustCompile(`^-+`)

// parsedQuery is the grammar's output: which packages to search, which terms
// must match, and which must not.
type parsedQuery struct {
	Chips   []string
	Include []string
	Exclude []string
}

// parseQuery splits palette/CLI input into scope chips and include/exclude
// terms.
//
// Chips are recognized ONLY from a leading, contiguous run of `pkg:` tokens at
// the very start of the input. This mirrors the web palette, where chips render
// as their own pinned row before the text field and so can only ever be a
// prefix. A `pkg:`-shaped token typed after free text is ordinary literal text
// — never silently promoted to a chip, and never dropped from the query.
//
// A word becomes a chip only when followed by `:` AND naming an installed
// package, so "mail" alone stays searchable text and an email titled
// "mail server migration" remains findable.
func parseQuery(input string, installedSlugs []string) parsedQuery {
	slugSet := make(map[string]struct{}, len(installedSlugs))
	for _, s := range installedSlugs {
		slugSet[strings.ToLower(s)] = struct{}{}
	}

	var out parsedQuery
	scanningChips := true

	for _, rawToken := range strings.Fields(input) {
		token := strings.TrimSpace(operatorChars.ReplaceAllString(rawToken, " "))
		if token == "" {
			continue
		}

		// Stripping operator characters can split one raw token into several
		// (`"quoted phrase"` becomes `quoted phrase`), so re-split before
		// classifying. The TS parser gets this for free by trimming a token
		// that whitespace-splitting already separated; here the replacement
		// introduces the spaces, so the walk has to happen explicitly.
		for _, part := range strings.Fields(token) {
			if scanningChips {
				if candidate, ok := strings.CutSuffix(part, ":"); ok {
					lowered := strings.ToLower(candidate)
					if _, known := slugSet[lowered]; known {
						if !contains(out.Chips, lowered) {
							out.Chips = append(out.Chips, lowered)
						}
						continue
					}
				}
				// The first token that is not a chip ends the leading run for
				// good; every later `pkg:`-shaped token is literal text.
				scanningChips = false
			}

			if operatorWord.MatchString(part) {
				continue
			}

			if literal, ok := strings.CutSuffix(part, ":"); ok {
				// Either not a package name, or a package name typed after the
				// leading run ended — keep it as text, minus the colon.
				if literal != "" {
					out.Include = append(out.Include, literal)
				}
				continue
			}

			// A leading hyphen negates only when a term follows it. Because we
			// split on whitespace, any hyphen still inside a token is mid-token
			// by construction (budget-2026) and stays literal.
			if strings.HasPrefix(part, "-") {
				if term := leadingHyphens.ReplaceAllString(part, ""); term != "" {
					out.Exclude = append(out.Exclude, term)
				}
				continue
			}

			out.Include = append(out.Include, part)
		}
	}

	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
