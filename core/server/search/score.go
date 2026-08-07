package search

import (
	"sort"
	"strings"
)

// Match-quality tiers. Deliberately NOT BM25: FTS5 ranks weight terms by
// frequency within their own table's corpus, so scores from two packages are in
// different units and a perfect filename match can sort below a marginal mail
// hit. Match quality against visible text is unit-free and identical for every
// package.
//
// These values and the tiering below mirror core/lib/search/score.ts exactly.
// While both exist they are pinned to one another by a shared fixture
// (testdata/score-cases.json) asserted from both languages — two independent
// implementations of one ranking would otherwise diverge silently.
const (
	tierExactTitle      = 1000
	tierTitlePrefix     = 800
	tierAllTermsInTitle = 600
	tierTitleSubstring  = 400
	tierSecondaryMatch  = 200
	tierNoVisibleMatch  = 100
)

// normalize folds case and punctuation so 'Budget 2026' and 'budget-2026'
// compare equal. Punctuation becomes a space rather than being deleted, so
// 'budget-2026' and 'budget 2026' normalize identically instead of both
// collapsing to 'budget2026'.
func normalize(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	lastSpace := true
	for _, r := range strings.ToLower(value) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// ScoreRow scores how well a row matches the query, using only text the row
// displays. A hit the backend matched on invisible content (a mail body, file
// contents) still scores, but below anything with a visible match.
func ScoreRow(include []string, row Row) int {
	terms := make([]string, 0, len(include))
	for _, t := range include {
		if n := normalize(t); n != "" {
			terms = append(terms, n)
		}
	}
	if len(terms) == 0 {
		return tierNoVisibleMatch
	}

	title := normalize(row.Title)
	query := strings.Join(terms, " ")

	if title == query {
		return tierExactTitle
	}
	if strings.HasPrefix(title, query) {
		return tierTitlePrefix
	}

	titleWords := strings.Split(title, " ")
	everyTermPrefixesAWord := true
	for _, term := range terms {
		found := false
		for _, word := range titleWords {
			if strings.HasPrefix(word, term) {
				found = true
				break
			}
		}
		if !found {
			everyTermPrefixesAWord = false
			break
		}
	}
	if everyTermPrefixesAWord {
		return tierAllTermsInTitle
	}

	if strings.Contains(title, query) {
		return tierTitleSubstring
	}

	var secondaryParts []string
	if row.Subtitle != "" {
		secondaryParts = append(secondaryParts, row.Subtitle)
	}
	if row.Meta != "" {
		secondaryParts = append(secondaryParts, row.Meta)
	}
	secondary := normalize(strings.Join(secondaryParts, " "))
	if secondary != "" {
		for _, term := range terms {
			if strings.Contains(secondary, term) {
				return tierSecondaryMatch
			}
		}
	}

	return tierNoVisibleMatch
}

// sortRows orders a merged cross-package slice in place. Tie-breaks, in order:
// score, then shorter title (a tighter match), then the package's Order — so
// the result does not depend on which source answered first.
func sortRows(rows []Row, include []string, orderBySlug map[string]int) {
	scores := make(map[string]int, len(rows))
	key := func(r Row) string { return r.Slug + "\x00" + r.ID }
	for _, r := range rows {
		scores[key(r)] = ScoreRow(include, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if sa, sb := scores[key(a)], scores[key(b)]; sa != sb {
			return sa > sb
		}
		if len(a.Title) != len(b.Title) {
			return len(a.Title) < len(b.Title)
		}
		return orderBySlug[a.Slug] < orderBySlug[b.Slug]
	})
}
