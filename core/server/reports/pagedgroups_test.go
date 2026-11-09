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
