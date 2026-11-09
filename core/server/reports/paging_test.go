package reports

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
)

const feedingsPaged = `{"version":4,"source":"zoo.feedings","columns":[
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
	return `{"version":4,"source":"zoo.feedings","columns":[
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
const pagedBySpecies = `{"version":4,"source":"zoo.feedings","columns":[
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

// Since J1a a page counts group, opening, detail and subtotal rows (R3), not
// only detail rows, so a page of 2 ends earlier than it did; a group row and
// its opening stay on one page, and Page.Size still counts the details (R4).
func TestAGroupedReportCountsEveryRowKindOnItsPages(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	pages := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: 2})
	if len(pages) != 6 {
		t.Fatalf("%d pages", len(pages))
	}
	requireRows(t, pages[0], "kind", "group", "opening", "total")
	requireRows(t, pages[0], "id", `g:["lion","USD"]`, `opening:["lion","USD"]`, `total:["USD"]`)
	requireRows(t, pages[1], "kind", "detail", "detail", "total")
	requireOrderedRows(t, pagedBySpecies, pages[1], "note", "f1", "f2", nil)
	requireRows(t, pages[2], "kind", "detail", "subtotal", "total")
	requireRows(t, pages[2], "id", "d:"+z.id("f7"), `s:["lion","USD"]`, `total:["USD"]`)
	requireRows(t, pages[3], "id", `g:["zebra","USD"]`, `opening:["zebra","USD"]`, `total:["USD"]`)
	requireRows(t, pages[4], "kind", "detail", "detail", "total")
	requireOrderedRows(t, pagedBySpecies, pages[4], "note", "f3", "f4", nil)
	requireRows(t, pages[5], "id", `s:["zebra","USD"]`, `total:["USD"]`)
	for i, want := range []int{0, 2, 1, 0, 2, 0} {
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
// on from page 1's 3500 (f1 1000, f2 2500) to 3600 at f7. A page of 4 holds
// lion's group row, its opening, f1 and f2 (J1a: every row kind counts).
func TestARunningSumContinuesOnPageTwo(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	pages := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: 4})
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
	// Since J1a a page counts lion's and zebra's group and subtotal rows too
	// (R3), so the 6,004 details and those 4 rows fill the pages.
	if want := (6004 + 4 + 999) / 1000; len(pages) != want || details != 6004 || lionTotals != 1 {
		t.Fatalf("pages = %d (want %d), details = %d, lion subtotals = %d", len(pages), want, details, lionTotals)
	}
	// The page count alone matches a details-only count too: page 1's group
	// row is one of its 1,000 counted rows.
	if n := countedRows(pages[0].Rows); n != 1000 {
		t.Fatalf("page 1 holds %d counted rows, want 1000", n)
	}
}

// A page holds at most 1,000 counted rows (group, opening, detail and
// subtotal, J1a); Page.Size is the details among them (R4).
func TestAGroupedPageHoldsAtMostAThousandRows(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 1200)
	res := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 5000}, nil)
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
		}
	}
	// Page 1 is lion's group row and 999 of lion's 1,202 details: 1,000
	// counted rows, 999 of them details.
	if n := countedRows(res.Rows); n != 1000 || details != 999 || res.Page == nil || res.Page.Size != 999 || res.Page.Next == "" {
		t.Fatalf("%d counted rows, %d details, page = %+v", n, details, res.Page)
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
		"another sort":     {strings.Replace(pagedBySpecies, `"sort":[{"col":"note"}]`, `"sort":[{"col":"note","dir":"desc"}]`, 1), Options{}},
		"another grouping": {byNote, Options{}},
		"a viewer sort":    {pagedBySpecies, Options{Sort: []ViewSort{{Col: "cost", Dir: "desc"}}}},
		// Since J1a the fold print in the cursor's fingerprint refuses it.
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

// Since J1a an open grouped report pages its group rows, so the group cap is
// gone: with rowCap 3 every group and its detail still shows, with no notice.
func TestAnOpenGroupedReportHasNoGroupCap(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 3 })
	const byNote = `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"note"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", byNote, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "group", "detail", "group", "detail", "group", "detail", "total")
	requireRows(t, res, "label", "f1", "f1", "f2", "f2", "f3", "f3", "f4", "f4", "Total")
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	if res.Page == nil || res.Page.Next != "" {
		t.Fatalf("page = %+v", res.Page)
	}
}

func testShape() [][]orderTerm {
	return [][]orderTerm{{{sql: "(a.k0 IS NULL)"}, {sql: "a.k0"}}, {{sql: "(x IS NULL)"}, {sql: "x", desc: true}, {sql: "(a.k1 IS NULL)"}, {sql: "a.k1"}}}
}

func TestAGroupCursorReadsBackWhatItWrote(t *testing.T) {
	keys := []sortKey{{name: "s0", col: "note", typ: Text}, {name: "__id"}}
	fp := groupFingerprint("species/;note/", levelShape(testShape()), foldPrint(&expansion{open: map[int]bool{1: true}}), keys)
	at := groupPos{key: []any{"lion", "f1", "USD"}, path: [][]any{{int64(0), "lion"}, {int64(0), int64(12), int64(0), "f1"}}, mode: cursorIn}
	text := groupCursorOf(fp, keys, at, map[string]any{"s0": "f2", "__id": "abc"})
	cur, err := readGroupCursor(fp, testShape(), keys, 2, 1, text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cur.Group, at.key) || !reflect.DeepEqual(cur.Path, at.path) || cur.Mode != cursorIn || !reflect.DeepEqual(cur.Values, []any{"f2", "abc"}) {
		t.Fatalf("cursor = %+v", cur)
	}
	after := groupCursorOf(fp, keys, groupPos{key: []any{"lion", "USD"}, path: at.path[:1], mode: cursorAfter}, nil)
	if cur, err := readGroupCursor(fp, testShape(), keys, 2, 1, after); err != nil || cur.Values != nil || cur.Mode != cursorAfter {
		t.Fatalf("after cursor = %+v, %v", cur, err)
	}
}

// The fingerprint ties a group cursor to the fold state (J1b): a fold, an
// unfold, Expand all or Collapse all is a new run from page 1.
func TestAGroupFingerprintChangesWithTheFoldState(t *testing.T) {
	keys := []sortKey{{name: "__id"}}
	shape := levelShape(testShape())
	base := &expansion{open: map[int]bool{1: true}, set: map[string]bool{}, closed: map[string]bool{}}
	folded := &expansion{open: map[int]bool{1: true}, set: map[string]bool{}, closed: map[string]bool{`["lion","USD"]`: true}}
	all := &expansion{all: true, open: map[int]bool{}, set: map[string]bool{}, closed: map[string]bool{}}
	prints := map[string]bool{}
	for _, e := range []*expansion{base, folded, all} {
		prints[groupFingerprint("species/", shape, foldPrint(e), keys)] = true
	}
	if len(prints) != 3 {
		t.Fatalf("fingerprints = %v, want three", prints)
	}
	// The same folds listed in another order are the same state.
	a := &expansion{open: map[int]bool{}, set: map[string]bool{`["a"]`: true, `["b"]`: true}, closed: map[string]bool{}}
	b := &expansion{open: map[int]bool{}, set: map[string]bool{`["b"]`: true, `["a"]`: true}, closed: map[string]bool{}}
	if foldPrint(a) != foldPrint(b) {
		t.Fatal("the fold print depends on the keys' order")
	}
}

func TestAMalformedGroupCursorIsRefused(t *testing.T) {
	keys := []sortKey{{name: "__id"}}
	fp := groupFingerprint("species/;note/", levelShape(testShape()), "", keys)
	good := func(c map[string]any) string {
		raw, _ := json.Marshal(c)
		return encodeCursor(string(raw))
	}
	cases := map[string]map[string]any{
		"another fingerprint":    {"k": "000000000000", "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}, "m": "after"},
		"no mode":                {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}},
		"a mode it never writes": {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}, "m": "before"},
		"path longer than group": {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}, []any{0, 1, 0, "f1"}}, "m": "after"},
		"a level of wrong width": {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0}}, "m": "after"},
		"values outside a leaf":  {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, "lion"}}, "m": "in", "v": []any{"x"}},
		"values with mode after": {"k": fp, "g": []any{"lion", "f1", "USD"}, "p": []any{[]any{0, "lion"}, []any{0, 1, 0, "f1"}}, "m": "after", "v": []any{"x"}},
		"an object in the group": {"k": fp, "g": []any{map[string]any{}, "USD"}, "p": []any{[]any{0, "lion"}}, "m": "after"},
		"an object in the path":  {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{0, map[string]any{}}}, "m": "after"},
		"values of wrong length": inLeaf(fp, []any{"x", "y"}),
		"a number for __id":      inLeaf(fp, []any{5}),
		// An "(x IS NULL)" term's value is a number (0 or 1), never text or
		// blank: such a cursor would compare its level's rows by a wrong kind
		// and read a wrong page (review B, deferral 1).
		"a string at an IS NULL term":       {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{"0", "lion"}}, "m": "after"},
		"a blank at an IS NULL term":        {"k": fp, "g": []any{"lion", "USD"}, "p": []any{[]any{nil, "lion"}}, "m": "after"},
		"a string at an inner IS NULL term": {"k": fp, "g": []any{"lion", "f1", "USD"}, "p": []any{[]any{0, "lion"}, []any{0, 1, "0", "f1"}}, "m": "in", "v": []any{"x"}},
	}
	// The cases differ from a cursor the reader takes in one thing only.
	if _, err := readGroupCursor(fp, testShape(), keys, 2, 1, good(inLeaf(fp, []any{"x"}))); err != nil {
		t.Fatalf("a well-formed cursor in a leaf: %v", err)
	}
	for name, c := range cases {
		if _, err := readGroupCursor(fp, testShape(), keys, 2, 1, good(c)); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	raw, _ := json.Marshal(inLeaf(fp, []any{"x"}))
	if _, err := readGroupCursor(fp, testShape(), keys, 2, 1, encodeCursor(string(raw)+"{}")); err == nil {
		t.Error("trailing data after the cursor: taken")
	}
}

// inLeaf is a group cursor inside the leaf group lion/f1 with values v.
func inLeaf(fp string, v []any) map[string]any {
	return map[string]any{"k": fp, "g": []any{"lion", "f1", "USD"}, "p": []any{[]any{0, "lion"}, []any{0, 1, 0, "f1"}}, "m": "in", "v": v}
}

func TestAfterSQLIsTrueOnlyForARowAfterTheValues(t *testing.T) {
	z := newZoo(t)
	s := newStmt(z.app, nil)
	cols := []orderTerm{{sql: "(x IS NULL)"}, {sql: "x", desc: true}, {sql: "y"}}
	type row struct {
		x    any
		y    string
		want bool
	}
	cases := []struct {
		values []any
		rows   []row
	}{
		{[]any{int64(0), int64(5), "m"}, []row{{int64(6), "a", false}, {int64(5), "a", false}, {int64(5), "m", false}, {int64(5), "z", true}, {int64(4), "a", true}, {nil, "a", true}}},
		// A blank cursor value: only a blank row ties on it; a set row sorts before.
		{[]any{int64(1), nil, "m"}, []row{{int64(3), "z", false}, {nil, "a", false}, {nil, "m", false}, {nil, "z", true}}},
	}
	for _, c := range cases {
		cond := afterSQL(s, cols, c.values)
		for _, r := range c.rows {
			// No coalesce: NOT afterSQL must count a row, so the term is never NULL.
			var after, notAfter any
			q := fmt.Sprintf("SELECT (%s), NOT (%s) FROM (SELECT {:x} AS x, {:y} AS y)", cond, cond)
			params := dbx.Params{"x": r.x, "y": r.y}
			for k, v := range s.params {
				params[k] = v
			}
			if err := z.app.DB().NewQuery(q).Bind(params).Row(&after, &notAfter); err != nil {
				t.Fatal(err)
			}
			if after == nil || notAfter == nil || (after == int64(1)) != r.want {
				t.Errorf("values %v, x=%v y=%v: after = %v, not after = %v, want %v", c.values, r.x, r.y, after, notAfter, r.want)
			}
		}
	}
}
