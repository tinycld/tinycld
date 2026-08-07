package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// tierValues maps the fixture's tier names to this implementation's constants.
// The fixture names tiers rather than numbers so the two languages can agree on
// the RANKING without also having to agree on arbitrary point values.
var tierValues = map[string]int{
	"exactTitle":      tierExactTitle,
	"titlePrefix":     tierTitlePrefix,
	"allTermsInTitle": tierAllTermsInTitle,
	"titleSubstring":  tierTitleSubstring,
	"secondaryMatch":  tierSecondaryMatch,
	"noVisibleMatch":  tierNoVisibleMatch,
}

type scoreFixture struct {
	Cases []struct {
		Why     string   `json:"why"`
		Include []string `json:"include"`
		Row     Row      `json:"row"`
		Tier    string   `json:"tier"`
	} `json:"cases"`
}

// TestScoreRowAgainstSharedFixture is the anti-drift guard: the TypeScript
// scorer asserts the same file, so a change to either implementation that is not
// mirrored fails here or there.
func TestScoreRowAgainstSharedFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "score-cases.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture scoreFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("fixture has no cases — a silently empty fixture asserts nothing")
	}

	for _, c := range fixture.Cases {
		want, ok := tierValues[c.Tier]
		if !ok {
			t.Errorf("case %q names unknown tier %q", c.Why, c.Tier)
			continue
		}
		if got := ScoreRow(c.Include, c.Row); got != want {
			t.Errorf("%s: ScoreRow(%v, %+v) = %d, want %s (%d)",
				c.Why, c.Include, c.Row, got, c.Tier, want)
		}
	}
}

func TestScoreTiersAreStrictlyOrdered(t *testing.T) {
	// The tiers only mean anything if they are ranked; equal values would make
	// the fixture pass while ordering silently collapsed.
	ordered := []int{
		tierExactTitle, tierTitlePrefix, tierAllTermsInTitle,
		tierTitleSubstring, tierSecondaryMatch, tierNoVisibleMatch,
	}
	for i := 1; i < len(ordered); i++ {
		if ordered[i-1] <= ordered[i] {
			t.Fatalf("tier %d (%d) must outrank tier %d (%d)", i-1, ordered[i-1], i, ordered[i])
		}
	}
}

func TestSortRowsTieBreaksOnTitleLengthThenPackageOrder(t *testing.T) {
	order := map[string]int{"mail": 5, "drive": 12}

	// Same tier, different title lengths: the tighter match wins.
	rows := []Row{
		{Slug: "drive", ID: "1", Title: "budget review long"},
		{Slug: "drive", ID: "2", Title: "budget rev"},
	}
	sortRows(rows, []string{"budget"}, order)
	if rows[0].ID != "2" {
		t.Fatalf("shorter title should sort first, got %+v", titlesOf(rows))
	}

	// Same tier and identical titles: nav.order decides, so ordering does not
	// depend on which source answered first.
	rows = []Row{
		{Slug: "drive", ID: "d", Title: "budget"},
		{Slug: "mail", ID: "m", Title: "budget"},
	}
	sortRows(rows, []string{"budget"}, order)
	if rows[0].Slug != "mail" {
		t.Fatalf("lower nav.order should sort first, got %s", rows[0].Slug)
	}
}

func TestNormalizeFoldsPunctuationToSpace(t *testing.T) {
	// Deleting punctuation instead would collapse 'budget-2026' to
	// 'budget2026', which then would not match 'budget 2026'.
	if got := normalize("budget-2026"); got != "budget 2026" {
		t.Errorf("normalize(budget-2026) = %q, want %q", got, "budget 2026")
	}
	if got := normalize("  Q3 — Budget!  "); got != "q3 budget" {
		t.Errorf("normalize = %q, want %q", got, "q3 budget")
	}
}
