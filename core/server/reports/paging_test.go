package reports

import (
	"encoding/base64"
	"slices"
	"testing"
)

const feedingsPaged = `{"version":1,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}}],
	"sort":[{"col":"fed","dir":"desc"}]}`

func TestDetailRowsPageByKeyset(t *testing.T) {
	z := newZoo(t)
	// A feeding with no date sorts last, in either direction.
	z.rec(t, "f8", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 50, "currency": "USD", "note": "f8"})

	first := z.run(t, "ann", feedingsPaged, Options{Limit: 2}, nil)
	requireOrderedRows(t, feedingsPaged, first, "note", "f4", "f2", nil)
	if first.Page == nil || first.Page.Size != 2 || first.Page.Next == "" {
		t.Fatalf("page = %+v", first.Page)
	}
	// Every page ends in the total of every row, not of the page.
	requireOrderedRows(t, feedingsPaged, first, "cost", int64(300), int64(2500), int64(4550))

	second := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	requireOrderedRows(t, feedingsPaged, second, "note", "f3", "f1", nil)
	// A running sum runs over every row, across pages.
	requireOrderedRows(t, feedingsPaged, second, "running", int64(3500), int64(4500), int64(4550))

	third := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Cursor: second.Page.Next}, nil)
	requireRows(t, third, "note", "f8", nil)
	if third.Page.Next != "" {
		t.Fatalf("the last page has a next cursor: %+v", third.Page)
	}
}

func TestAPageStartsAfterItsCursorWhenRowsChange(t *testing.T) {
	z := newZoo(t)
	first := z.run(t, "ann", feedingsPaged, Options{Limit: 2}, nil)
	// A row added before the cursor's place (a newer date, in descending
	// order) would shift an offset-based page, so f2 would show twice.
	z.feeding(t, "f9", z.recs["leo"], 10, "2026-03-31", 1)
	second := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	requireOrderedRows(t, feedingsPaged, second, "note", "f3", "f1", nil)
}

func TestACursorThisReportDidNotGiveIsRefused(t *testing.T) {
	z := newZoo(t)
	for _, cursor := range []string{"not base64!", "WyJ4Il0", "W3siYSI6MX0sImlkIl0"} {
		_, err := z.try(t, "ann", feedingsPaged, Options{Cursor: cursor}, nil)
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
	}
}

func TestAPageHoldsAtMostTheLargestPage(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.maxPage = 3 })
	res := z.run(t, "ann", feedingsPaged, Options{Limit: 50}, nil)
	if res.Page.Size != 3 || res.Page.Next == "" {
		t.Fatalf("page = %+v", res.Page)
	}
}

// pageNotes walks every page of raw with limit 1 and lists the detail notes.
func pageNotes(t *testing.T, z *zoo, raw string) []any {
	t.Helper()
	var notes []any
	cursor := ""
	for range 20 {
		res := z.run(t, "ann", raw, Options{Limit: 1, Cursor: cursor}, nil)
		notes = append(notes, res.Rows[0].Cells["note"])
		if res.Page.Next == "" {
			return notes
		}
		cursor = res.Page.Next
	}
	t.Fatal("paging never ended")
	return nil
}

func fullNotes(t *testing.T, z *zoo, raw string) []any {
	t.Helper()
	res := z.run(t, "ann", raw, Options{}, nil)
	notes := rowsOf(res, "note")
	return notes[:len(notes)-1]
}

func feedingsSortedBy(sort string) string {
	return `{"version":1,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"sort":` + sort + `}`
}

func TestACursorInsideARunOfBlankRowsContinuesInIdOrder(t *testing.T) {
	for _, dir := range []string{"asc", "desc"} {
		z := newZoo(t)
		z.rec(t, "f8", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 50, "currency": "USD", "note": "f8"})
		z.rec(t, "f10", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 60, "currency": "USD", "note": "f10"})
		raw := feedingsSortedBy(`[{"col":"fed","dir":"` + dir + `"}]`)
		got := pageNotes(t, z, raw)
		if want := fullNotes(t, z, raw); !slices.Equal(got, want) || len(got) != 6 {
			t.Fatalf("%s: pages = %v, full = %v", dir, got, want)
		}
	}
}

func TestATieOnTheSortColumnsIsBrokenByTheId(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "f9", z.recs["leo"], 10, "2026-01-05", 1)
	z.feeding(t, "f10", z.recs["leo"], 10, "2026-01-05", 1)
	raw := feedingsSortedBy(`[{"col":"fed"}]`)
	got := pageNotes(t, z, raw)
	if want := fullNotes(t, z, raw); !slices.Equal(got, want) || len(got) != 6 {
		t.Fatalf("pages = %v, full = %v", got, want)
	}
}

func TestPagesOfAMixedDirectionSortEqualTheFullRun(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f8", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 700, "currency": "USD", "note": "f8"})
	z.feeding(t, "f9", z.recs["leo"], 700, "2026-03-03", 1)
	z.feeding(t, "f10", z.recs["leo"], 700, "2026-02-01", 1)
	raw := feedingsSortedBy(`[{"col":"cost","dir":"asc"},{"col":"fed","dir":"desc"}]`)
	got := pageNotes(t, z, raw)
	if want := fullNotes(t, z, raw); !slices.Equal(got, want) || len(got) != 7 {
		t.Fatalf("pages = %v, full = %v", got, want)
	}
}

func TestARowRemovedBetweenPagesSkipsNoOther(t *testing.T) {
	z := newZoo(t)
	first := z.run(t, "ann", feedingsPaged, Options{Limit: 2}, nil)
	if err := z.app.Delete(z.recs["f4"]); err != nil {
		t.Fatal(err)
	}
	second := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	requireOrderedRows(t, feedingsPaged, second, "note", "f3", "f1", nil)
}

func encodeCursor(raw string) string { return base64.RawURLEncoding.EncodeToString([]byte(raw)) }

func TestAMalformedCursorOfTheRightShapeIsRefused(t *testing.T) {
	z := newZoo(t)
	first := z.run(t, "ann", feedingsPaged, Options{Limit: 1}, nil)
	raw, _ := base64.RawURLEncoding.DecodeString(first.Page.Next)
	other := z.run(t, "ann", feedingsSortedBy(`[{"col":"cost","dir":"asc"}]`), Options{Limit: 1}, nil)
	cursors := map[string]string{
		"wrong id type":  encodeCursor(`{"k":"x","v":["2026-03-03",7]}`),
		"bool value":     encodeCursor(`{"k":"x","v":[true,"a"]}`),
		"trailing data":  encodeCursor(string(raw) + " []"),
		"another report": other.Page.Next,
	}
	for what, c := range cursors {
		_, err := z.try(t, "ann", feedingsPaged, Options{Cursor: c}, nil)
		if err == nil {
			t.Fatalf("%s: a cursor this report did not give was taken", what)
		}
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
	}
}

func TestAGroupedReportIsNotPaged(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", feedingsBySpecies, Options{Cursor: "abc"}, nil)
	requireSpecError(t, err, 400, "options.cursor", "a grouped report is not paged")
}
