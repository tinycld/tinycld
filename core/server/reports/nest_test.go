package reports

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const sitesWithAnimals = `{"version":4,"source":"zoo.sites","columns":[
	{"id":"site","path":[{"field":"name"}]},
	{"id":"animal","path":[{"link":"animals","mode":"nest"},{"field":"name"}]},
	{"id":"owner","path":[{"link":"animals","mode":"nest"},{"link":"owner"},{"field":"name"}]},
	{"id":"fed","path":[{"link":"animals","mode":"nest"},{"link":"feedings","mode":"nest"},{"field":"fed"}]}],
	"sort":[{"col":"site"}]}`

func TestNestedChildRowsHangUnderTheirParent(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	requireOrderedRows(t, sitesWithAnimals, res, "label", "North", "Leo", "2026-01-05", "2026-02-10", "South", "Zed", "2026-01-20", "2026-03-03", "Total")
	requireOrderedRows(t, sitesWithAnimals, res, "owner", nil, "ann", nil, nil, nil, "bob", nil, nil, nil)
	north, leo := res.Rows[0], res.Rows[1]
	if leo.Parent != north.ID || leo.Depth != 1 || leo.ID != north.ID+"/"+z.id("leo") {
		t.Fatalf("leo = %+v", leo)
	}
	if fed := res.Rows[2]; fed.Parent != leo.ID || fed.Depth != 2 {
		t.Fatalf("feeding = %+v", fed)
	}
	// Parent rows have no value for a child row column.
	if _, ok := north.Cells["animal"]; !ok || north.Cells["animal"] != nil {
		t.Fatalf("north cells = %v", north.Cells)
	}
}

func TestNestedRowsTheReaderCannotSeeAreLeftOut(t *testing.T) {
	z := newZoo(t)
	// Bob may list mo and zed; in the north he sees mo, not leo.
	res := z.run(t, "bob", sitesWithAnimals, Options{}, nil)
	requireOrderedRows(t, sitesWithAnimals, res, "label", "North", "Mo", "2026-02-14", "South", "Zed", "2026-01-20", "2026-03-03", "Total")
}

func TestNestedRowsStopAtTheRowCap(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 1 })
	res := z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	codes := map[string]bool{}
	for _, n := range res.Notices {
		codes[n.Code] = true
	}
	if !codes["nestCap"] {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

func labelsOf(res *Result) []string {
	out := make([]string, len(res.Rows))
	for i, r := range res.Rows {
		out[i] = r.Label
	}
	return out
}

func TestNestedRowsKeepTheLowestParentsAtTheRowCap(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 1 })
	res := z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	// A level keeps its first rowCap rows in parent id order: one animal, then
	// one feeding of that animal.
	want := []string{"North", "South", "Total"}
	if z.id("north") < z.id("south") {
		want = []string{"North", "Leo", "2026-01-05", "South", "Total"}
	} else {
		want = []string{"North", "South", "Zed", "2026-01-20", "Total"}
	}
	if got := labelsOf(res); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	var caps []Notice
	for _, n := range res.Notices {
		if n.Code == "nestCap" {
			caps = append(caps, n)
		}
	}
	if len(caps) != 1 || caps[0].Count != 1 {
		t.Fatalf("nestCap notices = %+v", caps)
	}
}

func TestNestedRowsOfAPagedReportBelongToTheirOwnPage(t *testing.T) {
	z := newZoo(t)
	first := z.run(t, "ann", sitesWithAnimals, Options{Limit: 1}, nil)
	if first.Page == nil || first.Page.Next == "" {
		t.Fatalf("page = %+v", first.Page)
	}
	if got, want := labelsOf(first), []string{"North", "Leo", "2026-01-05", "2026-02-10", "Total"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first page = %v, want %v", got, want)
	}
	second := z.run(t, "ann", sitesWithAnimals, Options{Limit: 1, Cursor: first.Page.Next}, nil)
	if got, want := labelsOf(second), []string{"South", "Zed", "2026-01-20", "2026-03-03", "Total"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second page = %v, want %v", got, want)
	}
}

func TestNestedRowsOfAGroupedReportSitUnderTheirDetailRow(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[
		{"id":"region","path":[{"field":"region"}]},
		{"id":"site","path":[{"field":"name"}]},
		{"id":"animal","path":[{"link":"animals","mode":"nest"},{"field":"name"}]}],
		"sort":[{"col":"site"}],
		"groups":[{"id":"byRegion","col":"region"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	var got []string
	for _, r := range res.Rows {
		if r.Kind != KindDetail {
			continue
		}
		parent := ""
		if r.Parent != "" {
			parent = "child"
		}
		got = append(got, fmt.Sprintf("%s:%d:%s", r.Label, r.Depth, parent))
	}
	want := []string{"North:1:", "Leo:2:child", "South:1:", "Zed:2:child"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("detail rows = %v, want %v", got, want)
	}
	for _, r := range res.Rows {
		if r.Parent != "" && !strings.HasPrefix(r.ID, r.Parent+"/") {
			t.Fatalf("child %s is not under %s", r.ID, r.Parent)
		}
	}
}

// A nest level binds its parents as one parameter, so it reads any number
// of them: past SQLite's 32,766 variables, and past any row cap a run sets
// (the run's rows bound the parents; the level has no bound of its own).
func TestANestLevelReadsAnyNumberOfParents(t *testing.T) {
	z := newZoo(t)
	spec, _, err := Load([]byte(sitesWithAnimals))
	if err != nil {
		t.Fatal(err)
	}
	p, specErr := bind(z.app, spec)
	if specErr != nil {
		t.Fatal(specErr)
	}
	rn := &runner{
		ctx: context.Background(), app: z.app, c: newCompiler(p, z.app, z.as("ann"), nil),
		res: &Result{Rows: []Row{}, Notices: []Notice{}}, topics: map[string]bool{},
	}
	parents := []any{z.id("north")}
	for i := range 40000 {
		parents = append(parents, fmt.Sprintf("p%d", i))
	}
	rows, err := rn.nestLevel(0, parents)
	if err != nil {
		t.Fatalf("nestLevel: %v", err)
	}
	var names []string
	for _, row := range rows {
		if row["__parent"] != z.id("north") {
			t.Fatalf("row under %v", row["__parent"])
		}
		names = append(names, fmt.Sprint(row["v1"]))
	}
	// Ann lists Leo, the one North animal she may see.
	if !reflect.DeepEqual(names, []string{"Leo"}) {
		t.Fatalf("children = %v", names)
	}
}
