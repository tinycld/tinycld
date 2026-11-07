package reports

import (
	"reflect"
	"testing"
)

// detailCells lists one cell of the detail rows only, in order.
func detailCells(res *Result, what string) []any {
	var out []any
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			out = append(out, r.Cells[what])
		}
	}
	return out
}

func requireDetails(t *testing.T, res *Result, what string, want ...any) {
	t.Helper()
	if got := detailCells(res, what); !reflect.DeepEqual(got, want) {
		t.Fatalf("detail %s = %#v\nwant %#v", what, got, want)
	}
}

func byView(col, dir string) Options {
	return Options{ExpandAll: true, Sort: []ViewSort{{Col: col, Dir: dir}}}
}

// The viewer's sort orders detail rows inside their groups; the groups keep
// the report's order (zebra's 1000 is less than lion's 3500, and still follows).
func TestAViewerSortReordersDetailRowsInsideTheirGroups(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsBySpecies, byView("cost", "asc"), nil)
	requireRows(t, res, "label", "lion", "lion", "lion", "Total lion", "zebra", "zebra", "zebra", "Total zebra", "Total")
	requireOrderedRows(t, feedingsBySpecies, res, "note", nil, "f1", "f2", nil, nil, "f4", "f3", nil, nil)
}

// A running sum runs in the report's order (fed, newest first): f4 300,
// f2 2800, f3 3500, f1 4500, whatever order the viewer shows the rows in.
func TestRunningValuesKeepTheSpecsOrderUnderAViewerSort(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsPaged, Options{Sort: []ViewSort{{Col: "note", Dir: "asc"}}}, nil)
	requireDetails(t, res, "note", "f1", "f2", "f3", "f4")
	requireDetails(t, res, "running", int64(4500), int64(2800), int64(3500), int64(300))
}

// A report whose detail rows are labelled by a formula sorts by that label when
// the viewer sorts its first column (R7); without one, by the first column's
// record label (Zed, then Leo), the report's order after it.
func TestAViewerSortOnTheFirstColumnSortsByTheDetailLabel(t *testing.T) {
	z := newZoo(t)
	const base = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"fed","path":[{"field":"fed"}]}],"sort":[{"col":"fed"}]`
	labelled := z.run(t, "ann", base+`,"detail":{"label":{"path":[{"field":"note"}]}}}`, Options{Sort: []ViewSort{{Col: "animal", Dir: "desc"}}}, nil)
	requireDetails(t, labelled, "note", "f4", "f3", "f2", "f1")
	requireRows(t, labelled, "label", "f4", "f3", "f2", "f1", "Total")
	plain := z.run(t, "ann", base+`}`, Options{Sort: []ViewSort{{Col: "animal", Dir: "desc"}}}, nil)
	requireDetails(t, plain, "note", "f3", "f4", "f1", "f2")
}

// An opening row stays first under its group, and the groups keep their order,
// whatever order the viewer sorts the detail rows in.
func TestAnOpeningRowStaysFirstUnderAViewerSort(t *testing.T) {
	z := newZoo(t)
	spec := gramsByAnimal(`{"grams":`+beforePeriod+`}`, "")
	allTime := map[string]string{"period": `{"from":"1900-01-01","to":"2999-12-31"}`}
	res := z.run(t, "ann", spec, byView("note", "desc"), allTime)
	requireRows(t, res, "kind", "group", "opening", "detail", "detail", "subtotal", "group", "opening", "detail", "detail", "subtotal", "total")
	requireOrderedRows(t, spec, res, "note", nil, nil, "f2", "f1", nil, nil, nil, "f4", "f3", nil, nil)
}

func TestARunningValueIsNotAViewerSortKey(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", feedingsPaged, Options{Sort: []ViewSort{{Col: "running", Dir: "asc"}}}, nil)
	requireSpecError(t, err, 400, "options.sort.0.col", "a running value or a percent is not a sort key")
}

func TestAViewerSortIsCheckedAtRunTime(t *testing.T) {
	z := newZoo(t)
	eleven := make([]ViewSort, 11)
	for i := range eleven {
		eleven[i] = ViewSort{Col: "note", Dir: "asc"}
	}
	const pivot = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`
	cases := map[string]struct {
		spec     string
		sort     []ViewSort
		status   int
		at, want string
	}{
		"an unknown column": {feedingsPaged, []ViewSort{{Col: "nosuch", Dir: "asc"}}, 422, "options.sort.0.col", `the report has no column "nosuch"`},
		"a bad direction":   {feedingsPaged, []ViewSort{{Col: "note", Dir: "up"}}, 400, "options.sort.0.dir", "dir is asc or desc"},
		"a column twice":    {feedingsPaged, []ViewSort{{Col: "note", Dir: "asc"}, {Col: "note", Dir: "desc"}}, 400, "options.sort.1.col", "a column sorts once"},
		"eleven keys":       {feedingsPaged, eleven, 400, "options.sort", "a run sorts by at most 10 columns"},
		"a child row":       {sitesWithAnimals, []ViewSort{{Col: "animal", Dir: "asc"}}, 400, "options.sort.0.col", "a child row column is not a sort key"},
		"a pivot report":    {pivot, []ViewSort{{Col: "species", Dir: "asc"}}, 400, "options.sort", "a pivot report has no opened rows to sort"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", c.spec, Options{Sort: c.sort}, nil)
			requireSpecError(t, err, c.status, c.at, c.want)
		})
	}
}

// A cursor is tied to the viewer's sort: another sort, or none, refuses it.
func TestACursorIsRefusedAfterTheViewerSortChanges(t *testing.T) {
	z := newZoo(t)
	desc := []ViewSort{{Col: "cost", Dir: "desc"}}
	first := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Sort: desc}, nil)
	requireDetails(t, first, "note", "f2", "f1")
	next := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Sort: desc, Cursor: first.Page.Next}, nil)
	requireDetails(t, next, "note", "f3", "f4")
	for name, sort := range map[string][]ViewSort{"another sort": {{Col: "cost", Dir: "asc"}}, "no sort": nil} {
		_, err := z.try(t, "ann", feedingsPaged, Options{Limit: 2, Sort: sort, Cursor: first.Page.Next}, nil)
		if err == nil {
			t.Fatalf("%s: the cursor was taken", name)
		}
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
	}
}

func sortableOf(res *Result) map[string]bool {
	out := map[string]bool{}
	for _, c := range res.Columns {
		out[c.ID] = c.Sortable
	}
	return out
}

func TestResultColumnsSayWhichSort(t *testing.T) {
	z := newZoo(t)
	paged := z.run(t, "ann", feedingsPaged, Options{}, nil)
	if got, want := sortableOf(paged), map[string]bool{"note": true, "fed": true, "cost": true, "running": false}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sortable = %v, want %v", got, want)
	}
	nested := z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	if got, want := sortableOf(nested), map[string]bool{"site": true, "animal": false, "owner": false, "fed": false}; !reflect.DeepEqual(got, want) {
		t.Fatalf("nested sortable = %v, want %v", got, want)
	}
	pivot := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`, Options{}, nil)
	for _, c := range pivot.Columns {
		if c.Sortable {
			t.Fatalf("a pivot report's column %s is sortable", c.ID)
		}
	}
}
