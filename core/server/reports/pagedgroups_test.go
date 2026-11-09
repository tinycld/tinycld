package reports

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// isTotalStatement is the Total statement of a paged grouped run: its body
// ends reading o0 alone (totalPass).
func isTotalStatement(sql string) bool {
	return strings.HasSuffix(strings.TrimSpace(sql), " FROM o0")
}

// J1a: the Total is computed over every row the filter keeps, in a statement
// of its own, so a page of group rows never decides it.
func TestAGroupedRunReadsItsTotalInAStatementOfItsOwn(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", feedingsBySpecies, Options{}, nil)
	totals := 0
	for _, st := range *seen {
		if isTotalStatement(st.sql) {
			totals++
		}
	}
	if totals != 1 {
		t.Fatalf("%d Total statements, want 1", totals)
	}
	if last := res.Rows[len(res.Rows)-1]; last.Kind != "total" || last.Cells["cost"] != int64(4500) {
		t.Fatalf("total = %+v", last)
	}
}

// A pivot and a layout report still read their Total with their rows: a
// pivot has no pages, and a layout's computed rows read its sections.
func TestAPivotAndALayoutKeepOneGroupStatement(t *testing.T) {
	for name, spec := range map[string]string{"pivot": fmt.Sprintf(costBySpeciesAndMonth, ""), "layout": fmt.Sprintf(zooStatement, "")} {
		z := newZoo(t)
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{}, nil)
		for _, st := range *seen {
			if isTotalStatement(st.sql) {
				t.Fatalf("%s: a Total statement of its own", name)
			}
		}
	}
}

// byNoteClosed groups feedings by note, every group closed: one group row per
// feeding. bulkFeedings adds n more (notes n00001…).
const byNoteClosed = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"note","subtotals":true}],"sort":[{"col":"note"}]}`

// mergedGroupLabels is the group rows' labels of pages joined as the client
// joins them.
func mergedGroupLabels(t *testing.T, pages []*Result) []string {
	t.Helper()
	return groupRowLabels(&Result{Rows: mergedRows(t, pages)})
}

func lastTotal(t *testing.T, res *Result) Row {
	t.Helper()
	last := res.Rows[len(res.Rows)-1]
	if last.Kind != KindTotal {
		t.Fatalf("page ends in %+v, want the Total", last)
	}
	return last
}

// J1a: closed groups page in document order with no repeat and no gap, each
// page holds exactly Limit group rows (a closed group counts as its one row),
// and every page ends in the whole report's Total.
func TestClosedGroupsPageThroughEveryGroup(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 7)
	whole := z.run(t, "ann", byNoteClosed, Options{Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", byNoteClosed, Options{Limit: 3})
	want := groupRowLabels(whole)
	if len(want) != 11 || len(pages) != 4 {
		t.Fatalf("%d groups on %d pages", len(want), len(pages))
	}
	for i, p := range pages {
		if n := len(groupRowLabels(p)); i < 3 && n != 3 {
			t.Fatalf("page %d holds %d group rows, want 3", i+1, n)
		}
		if got, want := lastTotal(t, p).Cells["cost"], lastTotal(t, whole).Cells["cost"]; got != want {
			t.Fatalf("page %d total = %v, want %v", i+1, got, want)
		}
		if (p.Page.Next == "") != (i == len(pages)-1) {
			t.Fatalf("page %d next = %q", i+1, p.Page.Next)
		}
	}
	if got := mergedGroupLabels(t, pages); !slices.Equal(got, want) {
		t.Fatalf("paged groups %v\nwant %v", got, want)
	}
}

// Two levels, the inner closed: an open outer group whose inner groups span
// a page boundary goes on on the next page, and its subtotal follows them.
func TestTwoLevelsPageAcrossAnOpenOuterGroup(t *testing.T) {
	z := newZoo(t)
	whole := z.run(t, "ann", byRegionThenName, Options{Limit: 1000}, nil)
	for limit := 1; limit <= 4; limit++ {
		pages := groupedPages(t, z, "ann", byRegionThenName, Options{Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages\n%v\nwhole\n%v", limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
		}
		requireCut(t, pages, whole, limit)
	}
}

// having hides groups before the keyset: a hidden group is never counted and
// never shown, at a seam or anywhere else.
func TestHavingHidesGroupsBeforeTheKeyset(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 6)
	spec := strings.Replace(byNoteClosed, `"subtotals":true}`, `"subtotals":true,"having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1}]}}`, 1)
	whole := z.run(t, "ann", spec, Options{Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", spec, Options{Limit: 1})
	if got := mergedGroupLabels(t, pages); !slices.Equal(got, groupRowLabels(whole)) || len(got) != 4 {
		t.Fatalf("groups %v, want the four feedings over 1 (f1–f4)", got)
	}
	requireCut(t, pages, whole, 1)
}

// fill groups page as base groups do: a fill record with no rows is a group
// row in its place.
func TestFillGroupsPageInTheirPlace(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	raw := animalFill(`{}`, "")
	whole := z.run(t, "ann", raw, Options{Expand: [][]any{}, Limit: 1000}, nil)
	pages := groupedPages(t, z, "ann", raw, Options{Expand: [][]any{}, Limit: 1})
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages\n%v\nwhole\n%v", rowsOf(&Result{Rows: got}, "label"), rowsOf(whole, "label"))
	}
	requireCut(t, pages, whole, 1)
}

// A blank group key ((none)) pages in its place once.
func TestABlankGroupPagesInItsPlace(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.animals","columns":[
		{"id":"region","path":[{"link":"site"},{"field":"region"}]},
		{"id":"w","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"region"}]}`
	whole := z.run(t, "ann", spec, Options{Limit: 1000}, nil)
	if !slices.Contains(groupRowLabels(whole), "(none)") {
		t.Fatalf("no blank group: %v", groupRowLabels(whole))
	}
	pages := groupedPages(t, z, "ann", spec, Options{Limit: 1})
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages %v\nwhole %v", rowsOf(&Result{Rows: got}, "label"), rowsOf(whole, "label"))
	}
	requireCut(t, pages, whole, 1)
}

// Keyset on values (R1): a group added before the cursor, or the cursor's own
// group removed, between pages skips no group after the cursor.
func TestAGroupAddedOrRemovedBetweenPagesSkipsNoOther(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 4)
	first := z.run(t, "ann", byNoteClosed, Options{Limit: 3}, nil)
	z.feeding(t, "a0", z.recs["leo"], 5, "2026-04-02", 1) // note "a0" sorts first
	// f1 is before the cursor; f3 is the cursor's own group.
	if _, err := z.app.DB().NewQuery("DELETE FROM zoo_feedings WHERE note IN ('f1', 'f3')").Execute(); err != nil {
		t.Fatal(err)
	}
	rest := groupedPages(t, z, "ann", byNoteClosed, Options{Limit: 3, Cursor: first.Page.Next})
	got := mergedGroupLabels(t, append([]*Result{first}, rest...))
	want := []string{"f1", "f2", "f3", "f4", "n00001", "n00002", "n00003", "n00004"}
	if !slices.Equal(got, want) {
		t.Fatalf("groups %v, want %v", got, want)
	}
}

// J1a, every group closed: page 1 holds exactly N group rows.
func TestEveryGroupClosedPageOneHoldsExactlyLimitGroupRows(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 20)
	for _, n := range []int{1, 5, 17} {
		res := z.run(t, "ann", byNoteClosed, Options{Limit: n}, nil)
		if got := len(groupRowLabels(res)); got != n || res.Page.Next == "" || res.Page.Size != 0 {
			t.Fatalf("limit %d: %d group rows, page %+v", n, got, res.Page)
		}
	}
}

// The group-row cap is gone for a grouped report: 6,004 groups page to their
// end with no rowCap notice.
func TestASixThousandGroupReportReturnsEveryGroup(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 6000)
	pages := groupedPages(t, z, "ann", byNoteClosed, Options{Limit: 1000})
	labels := []string{}
	for i, p := range pages {
		for _, n := range p.Notices {
			if n.Code == "rowCap" {
				t.Fatalf("page %d says %q", i+1, n.Message)
			}
		}
		labels = append(labels, groupRowLabels(p)...)
	}
	if len(labels) != 6004 || len(pages) != 7 || !slices.IsSorted(labels) {
		t.Fatalf("%d groups on %d pages, sorted %v", len(labels), len(pages), slices.IsSorted(labels))
	}
}

// A pivot and a layout report keep the cap and its notice (a statement needs
// every section; a pivot's columns read every row).
func TestAPivotAndALayoutStillStopAtTheRowCap(t *testing.T) {
	for name, spec := range map[string]string{"pivot": fmt.Sprintf(costBySpeciesAndMonth, ""), "layout": fmt.Sprintf(zooStatement, "")} {
		z := newZoo(t)
		withLimits(t, func(l *limits) { l.rowCap = 1 })
		res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
		if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "rowCap" && n.Count == 1 }) {
			t.Fatalf("%s: notices %+v", name, res.Notices)
		}
	}
}

func TestAGroupCursorFromAnotherFoldStateIsRefused(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 4)
	first := z.run(t, "ann", byNoteClosed, Options{Limit: 2}, nil)
	for name, opts := range map[string]Options{
		"a fold":       {Collapse: [][]any{{"f1", "USD"}}},
		"an expand":    {Expand: [][]any{{"f1", "USD"}}},
		"expand all":   {ExpandAll: true},
		"another sort": {Sort: []ViewSort{{Col: "cost", Dir: "desc"}}},
	} {
		opts.Limit, opts.Cursor = 2, first.Page.Next
		_, err := z.try(t, "ann", byNoteClosed, opts, nil)
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
		_ = name
	}
}

// countedRows is the number of rows a page's limit counts (R3): group,
// opening, detail and subtotal rows.
func countedRows(rows []Row) int {
	n := 0
	for _, r := range rows {
		if !isEnd(r) {
			n++
		}
	}
	return n
}

// requireCut fails unless pages really paged: more than one page when the
// whole run counts more rows than limit, and no page counting more than limit
// (a group row and its opening share a page, so at limit 1 a page may hold
// both). A merged-equals-whole test with it fails if paging stops.
func requireCut(t *testing.T, pages []*Result, whole *Result, limit int) {
	t.Helper()
	if countedRows(whole.Rows) > limit && len(pages) < 2 {
		t.Fatalf("limit %d: %d counted rows on one page", limit, countedRows(whole.Rows))
	}
	for i, p := range pages {
		counted := slices.DeleteFunc(slices.Clone(p.Rows), isEnd)
		n := len(counted)
		if n == 0 || (n > limit && (n != 2 || counted[0].Kind != KindGroup || counted[1].Kind != KindOpening)) {
			t.Fatalf("limit %d: page %d counts %d rows", limit, i+1, n)
		}
	}
}

// Open groups: pages of every limit join into the one-page run, the open
// groups' details span page boundaries, and every page ends in the Total.
func TestOpenGroupsPageAtEveryLimit(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	whole := z.run(t, "ann", pagedBySpecies, Options{Limit: 1000}, nil)
	for limit := 1; limit <= 12; limit++ {
		pages := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages\n%v\nwhole\n%v", limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
		}
		requireCut(t, pages, whole, limit)
		for i, p := range pages {
			if lastTotal(t, p).Cells["cost"] != lastTotal(t, whole).Cells["cost"] {
				t.Fatalf("limit %d page %d: total differs", limit, i+1)
			}
		}
	}
}

// Two open levels page as one document.
func TestTwoOpenLevelsPageAsOneDocument(t *testing.T) {
	z := newZoo(t)
	raw := pagedAnimals(2)
	whole := z.run(t, "ann", raw, Options{Limit: 1000}, nil)
	for limit := 1; limit <= 9; limit++ {
		pages := groupedPages(t, z, "ann", raw, Options{Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages differ from the one-page run", limit)
		}
		requireCut(t, pages, whole, limit)
	}
}

// R3: a page counts group, opening, detail and subtotal rows; the group row
// and its opening share a page, also at limit 1.
func TestAGroupRowAndItsOpeningShareAPage(t *testing.T) {
	z := newZoo(t)
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 1}, nil)
	requireRows(t, first, "kind", "group", "opening", "total")
	second := z.run(t, "ann", pagedBySpecies, Options{Limit: 1, Cursor: first.Page.Next}, nil)
	requireRows(t, second, "kind", "detail", "total")
	third := z.run(t, "ann", pagedBySpecies, Options{Limit: 3}, nil)
	requireRows(t, third, "kind", "group", "opening", "detail", "total")
}

// A viewer sort orders the details inside their groups; a page that ends
// inside a group goes on in the viewer's order.
func TestAViewerSortPagesInsideItsGroups(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	opts := Options{Sort: []ViewSort{{Col: "cost", Dir: "desc"}}}
	opts.Limit = 1000
	whole := z.run(t, "ann", pagedBySpecies, opts, nil)
	opts.Limit = 3
	pages := groupedPages(t, z, "ann", pagedBySpecies, opts)
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages\n%v\nwhole\n%v", rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
	}
	requireCut(t, pages, whole, opts.Limit)
}

// Two currencies: each currency's Total keeps its place on the page that
// holds its block's end, and every page ends in the Totals after its slice.
func TestAPagedGroupPageKeepsEachCurrencyTotalInItsPlace(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 100)
	whole := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 1000}, nil)
	requireRows(t, whole, "kind", "group", "detail", "subtotal", "total", "group", "detail", "detail", "subtotal", "group", "detail", "detail", "subtotal", "total")
	requireRows(t, whole, "note", nil, "f7", nil, nil, nil, "f1", "f2", nil, nil, "f3", "f4", nil, nil)
	for limit := 1; limit <= 8; limit++ {
		pages := groupedPages(t, z, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages\n%v\nwhole\n%v", limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
		}
		requireCut(t, pages, whole, limit)
	}
}

// Closed groups in two currencies page through both currency blocks: the
// EUR block's Total keeps its place between them (Task 4 reviewer).
func TestClosedGroupsInTwoCurrenciesPageThroughBothBlocks(t *testing.T) {
	z := newZoo(t)
	withEuroLeo(t, z, 100)
	whole := z.run(t, "ann", feedingsBySpecies, Options{Limit: 1000}, nil)
	requireRows(t, whole, "id", `g:["lion","EUR"]`, `total:["EUR"]`, `g:["lion","USD"]`, `g:["zebra","USD"]`, `total:["USD"]`)
	for limit := 1; limit <= 4; limit++ {
		pages := groupedPages(t, z, "ann", feedingsBySpecies, Options{Limit: limit})
		if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
			t.Fatalf("limit %d: pages\n%v\nwhole\n%v", limit, rowsOf(&Result{Rows: got}, "id"), rowsOf(whole, "id"))
		}
		requireCut(t, pages, whole, limit)
	}
}

// A page that ended after a detail row of a group goes on from the next group
// when that group was removed between pages, never with a refusal: a refused
// next page would fail on every Retry (R8).
func TestAPageWhoseOpenGroupWasRemovedGoesOnFromTheNextGroup(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 3}, nil)
	requireRows(t, first, "kind", "group", "opening", "detail", "total")
	if _, err := z.app.DB().NewQuery("DELETE FROM zoo_feedings WHERE note IN ('f1', 'f2', 'f7')").Execute(); err != nil {
		t.Fatal(err)
	}
	rest := groupedPages(t, z, "ann", pagedBySpecies, Options{Limit: 3, Cursor: first.Page.Next})
	requireRows(t, &Result{Rows: mergedRows(t, rest)}, "id", `g:["zebra","USD"]`, `opening:["zebra","USD"]`, "d:"+z.id("f3"), "d:"+z.id("f4"), `s:["zebra","USD"]`, `total:["USD"]`)
}

// A fold under an open level pages like any closed group.
func TestAFoldedGroupPagesAsOneRow(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	opts := Options{Collapse: [][]any{{"lion", "USD"}}, Limit: 1000}
	whole := z.run(t, "ann", pagedBySpecies, opts, nil)
	opts.Limit = 2
	pages := groupedPages(t, z, "ann", pagedBySpecies, opts)
	if got := mergedRows(t, pages); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages differ from the one-page run")
	}
	requireCut(t, pages, whole, opts.Limit)
}

// checkTopics (TestMain) sees the page statement and the Total statement of
// a cursor page; this test makes sure both are sent on a cursor page.
func TestAPagedGroupRunSendsAPageAndATotalStatement(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 4)
	first := z.run(t, "ann", byNoteClosed, Options{Limit: 2}, nil)
	seen := observeReads(t)
	z.run(t, "ann", byNoteClosed, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	kinds := map[string]int{}
	for _, st := range *seen {
		switch {
		case isTotalStatement(st.sql):
			kinds["total"]++
		case strings.Contains(st.sql, "__p1"):
			kinds["page"]++
		}
	}
	if kinds["total"] != 1 || kinds["page"] != 1 {
		t.Fatalf("statements %v", kinds)
	}
}
