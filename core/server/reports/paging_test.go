package reports

import (
	"encoding/base64"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
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

// pagedBySpecies groups ann's feedings by species, each open group starting with
// an opening row and ending in its subtotal; running restarts in each group.
const pagedBySpecies = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}}],
	"groups":[{"id":"g","col":"species","open":true,"subtotals":true,"opening":{"label":"Start"}}],
	"sort":[{"col":"note"}]}`

// withF7 adds a third lion feeding, so lion has f1, f2, f7 and zebra f3, f4.
func withF7(t *testing.T, z *zoo) {
	t.Helper()
	z.feeding(t, "f7", z.recs["leo"], 100, "2026-01-07", 10)
}

// groupedPages walks every page of raw as user with opts.
func groupedPages(t *testing.T, z *zoo, user, raw string, opts Options) []*Result {
	t.Helper()
	var pages []*Result
	for range 1000 {
		res := z.run(t, user, raw, opts, nil)
		pages = append(pages, res)
		if res.Page == nil || res.Page.Next == "" {
			return pages
		}
		opts.Cursor = res.Page.Next
	}
	t.Fatal("paging never ended")
	return nil
}

// mergedRows joins pages as the client does (R10): each page's rows in order,
// an earlier page's trailing total and computed rows dropped, since a later
// page gives them again (a total or computed row inside a page's slice keeps
// its place there and comes on no other page); it fails the test when a row
// is on two pages (R9e).
func mergedRows(t *testing.T, pages []*Result) []Row {
	t.Helper()
	var out []Row
	seen := map[string]bool{}
	for i, p := range pages {
		rows := p.Rows
		for i < len(pages)-1 && len(rows) > 0 && isEnd(rows[len(rows)-1]) {
			rows = rows[:len(rows)-1]
		}
		for _, r := range rows {
			if seen[r.ID] {
				t.Fatalf("row %s is on two pages", r.ID)
			}
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}

func TestAGroupedReportPagesAcrossAGroupBoundary(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	pages := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: 2})
	if len(pages) != 3 {
		t.Fatalf("%d pages", len(pages))
	}
	requireRows(t, pages[0], "kind", "group", "opening", "detail", "detail", "total")
	requireOrderedRows(t, pagedBySpecies, pages[0], "note", nil, nil, "f1", "f2", nil)
	requireRows(t, pages[1], "kind", "detail", "subtotal", "group", "opening", "detail", "total")
	requireOrderedRows(t, pagedBySpecies, pages[1], "note", "f7", nil, nil, nil, "f3", nil)
	if ids := []string{pages[1].Rows[1].ID, pages[1].Rows[2].ID, pages[1].Rows[3].ID}; !slices.Equal(ids, []string{`s:["lion","USD"]`, `g:["zebra","USD"]`, `opening:["zebra","USD"]`}) {
		t.Fatalf("page 2 ids = %v", ids)
	}
	requireRows(t, pages[2], "kind", "detail", "subtotal", "total")
	requireOrderedRows(t, pagedBySpecies, pages[2], "note", "f4", nil, nil)
	for i, want := range []int{2, 2, 1} {
		if pages[i].Page.Size != want {
			t.Fatalf("page %d size = %d", i+1, pages[i].Page.Size)
		}
		// Every page ends in the total of every row.
		if total := pages[i].Rows[len(pages[i].Rows)-1]; total.Cells["cost"] != int64(4600) {
			t.Fatalf("page %d total = %+v", i+1, total)
		}
	}
}

// Window values are computed before the page is cut: lion's running sum goes
// on from page 1's 3500 (f1 1000, f2 2500) to 3600 at f7.
func TestARunningSumContinuesOnPageTwo(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	pages := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: 2})
	requireDetails(t, pages[0], "running", int64(1000), int64(3500))
	if first := pages[1].Rows[0]; first.Cells["note"] != "f7" || first.Cells["running"] != int64(3600) {
		t.Fatalf("page 2 starts with %+v", first)
	}
}

// Pages of one row each join into exactly the one-page run, and a feeding
// added between pages repeats no row already shown.
func TestAGroupedPageSeamRepeatsNoRow(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	whole := z.run(t, "ann", pagedBySpecies, Options{Limit: 1000}, nil)
	if got := mergedRows(t, groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: 1})); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages = %v\nwhole = %v", got, whole.Rows)
	}
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 2}, nil)
	if first.Page == nil {
		t.Fatal("a grouped run gave no page")
	}
	z.feeding(t, "f8", z.recs["zed"], 50, "2026-03-20", 5)
	rest := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: 2, Cursor: first.Page.Next})
	merged := mergedRows(t, append([]*Result{first}, rest...))
	requireRows(t, &Result{Rows: merged}, "note", nil, nil, "f1", "f2", "f7", nil, nil, nil, "f3", "f4", "f8", nil, nil)
}

// An expanded report in two currencies keeps each currency's Total after its
// own block: a page holds a total inside its slice in its place, and ends in
// the totals after its slice.
func TestAGroupedPageKeepsEachCurrencyTotalInItsPlace(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 100)
	whole := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 1000}, nil)
	requireRows(t, whole, "kind", "group", "detail", "subtotal", "total", "group", "detail", "detail", "subtotal", "group", "detail", "detail", "subtotal", "total")
	requireRows(t, whole, "note", nil, "f7", nil, nil, nil, "f1", "f2", nil, nil, "f3", "f4", nil, nil)
	pages := groupedPages(t, z, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 2})
	if len(pages) != 3 {
		t.Fatalf("%d pages", len(pages))
	}
	requireRows(t, pages[0], "id", `g:["lion","EUR"]`, "d:"+z.id("f7"), `s:["lion","EUR"]`, `total:["EUR"]`, `g:["lion","USD"]`, "d:"+z.id("f1"), `total:["USD"]`)
	requireRows(t, pages[1], "id", "d:"+z.id("f2"), `s:["lion","USD"]`, `g:["zebra","USD"]`, "d:"+z.id("f3"), `total:["USD"]`)
	requireRows(t, pages[2], "id", "d:"+z.id("f4"), `s:["zebra","USD"]`, `total:["USD"]`)
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages = %v\nwhole = %v", got, whole.Rows)
	}
}

// bulkFeedings adds n feedings of leo (cost 1, grams 1, notes n00001…) in one
// statement: n saves would take minutes. The report reads them as any other.
func bulkFeedings(t *testing.T, z *zoo, n int) {
	t.Helper()
	_, err := z.app.DB().NewQuery(`WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < {:n})
		INSERT INTO zoo_feedings (id, animal, cost, currency, grams, fed, note, due)
		SELECT printf('bulk%011d', i), {:leo}, 1, 'USD', 1, '2026-04-01 00:00:00.000Z', printf('n%05d', i), '2026-04-01' FROM seq`).
		Bind(dbx.Params{"n": n, "leo": z.id("leo")}).Execute()
	if err != nil {
		t.Fatal(err)
	}
}

// The grouped 5,000-row cap is gone: 6,004 detail rows page to the end.
func TestASixThousandRowGroupedReportPagesToItsEnd(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 6000)
	pages := groupedPages(t, z, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 1000})
	details, lionTotals := 0, 0
	for i, p := range pages {
		if len(p.Notices) != 0 {
			t.Fatalf("page %d notices = %+v", i+1, p.Notices)
		}
		for _, r := range p.Rows {
			switch {
			case r.Kind == KindDetail:
				details++
			case r.ID == `s:["lion","USD"]`:
				lionTotals++
				if r.Cells["cost"] != int64(9500) {
					t.Fatalf("lion's subtotal = %+v", r)
				}
			}
		}
	}
	if len(pages) != 7 || details != 6004 || lionTotals != 1 {
		t.Fatalf("pages = %d, details = %d, lion subtotals = %d", len(pages), details, lionTotals)
	}
}

func TestAGroupedPageHoldsAtMostAThousandRows(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 1200)
	res := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 5000}, nil)
	if res.Page == nil || res.Page.Size != 1000 || res.Page.Next == "" {
		t.Fatalf("page = %+v", res.Page)
	}
}

func TestAGroupedCursorIsRefusedAfterASpecChangeOrASortChange(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 1}, nil)
	if first.Page == nil {
		t.Fatal("a grouped run gave no page")
	}
	byNote := strings.Replace(pagedBySpecies, `"col":"species","open"`, `"col":"note","open"`, 1)
	cases := map[string]struct {
		spec string
		opts Options
	}{
		"another sort":      {strings.Replace(pagedBySpecies, `"sort":[{"col":"note"}]`, `"sort":[{"col":"note","dir":"desc"}]`, 1), Options{}},
		"another grouping":  {byNote, Options{}},
		"a viewer sort":     {pagedBySpecies, Options{Sort: []ViewSort{{Col: "cost", Dir: "desc"}}}},
		"its group is shut": {pagedBySpecies, Options{Expand: [][]any{{"zebra", "USD"}}}},
	}
	for name, c := range cases {
		c.opts.Limit, c.opts.Cursor = 1, first.Page.Next
		_, err := z.try(t, "ann", c.spec, c.opts, nil)
		if err == nil {
			t.Fatalf("%s: the cursor was taken", name)
		}
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
	}
}

// Group rows keep their cap: with rowCap 3 the group statement keeps the
// total and the first two groups, says so, and pages only their details.
func TestGroupRowsStillStopAtTheGroupCap(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 3 })
	const byNote = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"note"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", byNote, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "group", "detail", "total")
	requireRows(t, res, "label", "f1", "f1", "f2", "f2", "Total")
	if len(res.Notices) != 1 || res.Notices[0].Code != "rowCap" || res.Notices[0].Count != 3 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	if res.Page == nil || res.Page.Next != "" {
		t.Fatalf("page = %+v", res.Page)
	}
}
