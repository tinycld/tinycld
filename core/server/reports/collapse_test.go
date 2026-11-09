package reports

import (
	"fmt"
	"slices"
	"testing"
)

// byRegionThenName groups animals by their site's region (open) and then by
// name (closed by default): expand opens a name group, collapse closes a
// region.
const byRegionThenName = `{"version":4,"source":"zoo.animals","columns":[
	{"id":"region","path":[{"link":"site"},{"field":"region"}]},
	{"id":"name","path":[{"field":"name"}]},
	{"id":"weight","path":[{"field":"weight"}],"aggregate":"sum"}],
	"groups":[{"id":"r","col":"region","open":true,"subtotals":true},{"id":"n","col":"name","subtotals":true}],
	"sort":[{"col":"name"}]}`

// A fold under a level that is open by default closes exactly that group:
// its row keeps its cells, it has no children and no subtotal, and every
// other group of the level stays open.
func TestCollapseClosesAGroupUnderAnOpenLevel(t *testing.T) {
	z := newZoo(t)
	open := z.run(t, "ann", pagedBySpecies, Options{}, nil)
	requireRows(t, open, "kind", "group", "opening", "detail", "detail", "subtotal", "group", "opening", "detail", "detail", "subtotal", "total")
	res := z.run(t, "ann", pagedBySpecies, Options{Collapse: [][]any{{"lion", "USD"}}}, nil)
	requireRows(t, res, "kind", "group", "group", "opening", "detail", "detail", "subtotal", "total")
	lion := res.Rows[0]
	if lion.Label != "lion" || lion.Expanded || !lion.CanExpand || lion.Cells["cost"] != int64(3500) {
		t.Fatalf("lion = %+v", lion)
	}
}

func TestCollapseClosesAGroupUnderExpandAll(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Collapse: [][]any{{"zebra", "USD"}}}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "subtotal", "group", "total")
	if zebra := res.Rows[4]; zebra.Label != "zebra" || zebra.Expanded || zebra.Cells["cost"] != int64(1000) {
		t.Fatalf("zebra = %+v", zebra)
	}
}

// Ruling R2: a run that sends collapse keeps the open levels open, so expand
// adds a group of a closed level and collapse closes a group of an open one,
// in one run.
func TestCollapseAndExpandTogetherOnTwoLevels(t *testing.T) {
	z := newZoo(t)
	whole := z.run(t, "ann", byRegionThenName, Options{ExpandAll: true}, nil)
	var north, south []any
	// region is a select: its groups read their option labels.
	for _, r := range whole.Rows {
		if r.Kind == "group" && r.Depth == 0 && r.Label == "North" {
			north = r.GroupKey
		}
		if r.Kind == "group" && r.Depth == 0 && r.Label == "South" {
			south = r.GroupKey
		}
	}
	if north == nil || south == nil {
		t.Fatalf("regions = %v", rowsOf(whole, "label"))
	}
	// weight is not money, so the keys carry no currency key.
	leo := append(slices.Clone(north), "Leo")
	res := z.run(t, "ann", byRegionThenName, Options{Expand: [][]any{leo}, Collapse: [][]any{south}}, nil)
	for _, r := range res.Rows {
		switch {
		case r.Kind == "group" && r.Label == "North" && !r.Expanded:
			t.Fatalf("north closed: the open level must stay open when collapse is sent")
		case r.Kind == "group" && r.Label == "South" && r.Expanded:
			t.Fatalf("south open: collapse must close it")
		case r.Kind == "group" && r.Label == "Leo" && !r.Expanded:
			t.Fatalf("Leo closed: expand must open a group of a closed level")
		}
	}
}

func TestCollapseTakesAtMostTheFoldCapOfKeys(t *testing.T) {
	z := newZoo(t)
	z.run(t, "ann", sixLevels, Options{Collapse: deepKeys(runLimits.foldCap)}, nil)
	_, err := z.try(t, "ann", sixLevels, Options{Collapse: deepKeys(runLimits.foldCap + 1)}, nil)
	requireSpecError(t, err, 400, "options.collapse", fmt.Sprintf("at most %d groups", runLimits.foldCap))
}

func TestCollapseRefusesAKeyThatIsNotAGroupKey(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", feedingsBySpecies, Options{Collapse: [][]any{{}}}, nil)
	requireSpecError(t, err, 400, "options.collapse.0", "a group key is a group row's groupKey")
	_, err = z.try(t, "ann", feedingsBySpecies, Options{Collapse: [][]any{{map[string]any{"a": 1}}}}, nil)
	requireSpecError(t, err, 400, "options.collapse.0", "a group key holds text, numbers, true, false or null")
}
