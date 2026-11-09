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
	var sawNorth, sawSouth, sawLeo bool
	for i, r := range res.Rows {
		if r.Kind != "group" {
			continue
		}
		switch r.Label {
		case "North":
			sawNorth = true
			if !r.Expanded {
				t.Fatalf("north closed: the open level must stay open when collapse is sent")
			}
		case "South":
			sawSouth = true
			if r.Expanded {
				t.Fatalf("south open: collapse must close it")
			}
		case "Leo":
			sawLeo = true
			if !r.Expanded {
				t.Fatalf("Leo closed: expand must open a group of a closed level")
			}
			if i+1 >= len(res.Rows) || res.Rows[i+1].Kind != "detail" {
				t.Fatalf("Leo has no detail rows: %v", rowsOf(res, "kind"))
			}
		}
	}
	if !sawNorth || !sawSouth || !sawLeo {
		t.Fatalf("north %v, south %v, Leo %v: rows %v", sawNorth, sawSouth, sawLeo, rowsOf(res, "label"))
	}
}

// A fold of the blank group closes that group only: a nil key matches the
// (none) group by IS, and every other group of the open level stays open.
func TestCollapseClosesTheBlankGroup(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.animals","columns":[
		{"id":"region","path":[{"link":"site"},{"field":"region"}]},
		{"id":"w","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"region","open":true}]}`
	open := z.run(t, "ann", spec, Options{}, nil)
	res := z.run(t, "ann", spec, Options{Collapse: [][]any{{nil}}}, nil)
	var blank, others int
	for _, r := range res.Rows {
		if r.Kind != "group" {
			continue
		}
		if r.GroupKey[0] == nil {
			blank++
			if r.Expanded {
				t.Fatalf("blank group open: %+v", r)
			}
			continue
		}
		others++
		if !r.Expanded {
			t.Fatalf("%s closed: only the blank group is folded", r.Label)
		}
	}
	if blank != 1 || others == 0 || len(res.Rows) >= len(open.Rows) {
		t.Fatalf("blank %d, others %d, rows %v (open %v)", blank, others, rowsOf(res, "label"), rowsOf(open, "label"))
	}
}

// A pivot keeps one group statement and its row cap. A folded species hides
// its notes and their animals too, though the note level is open: the
// animals under it are not read, so they do not count toward the cap: the 7
// level rows are the Total, the 2 species, zebra's 2 notes and their 2
// animals. Before, lion's 2 animals came too, past the cap.
func TestAFoldedGrandparentsRowsDoNotCountTowardTheRowCap(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 7 })
	const spec = `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"s","col":"species","open":true},{"id":"n","col":"note","open":true},{"id":"a","col":"animal"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`
	res := z.run(t, "ann", spec, Options{Collapse: [][]any{{"lion", "USD"}}}, nil)
	if len(res.Notices) != 0 || len(groupRowLabels(res)) != 6 {
		t.Fatalf("groups %v, notices %+v", groupRowLabels(res), res.Notices)
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
