package reports

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOpenGroupsOpenWhenTheRunHasNoExpansion(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","open":true}],"sort":[{"col":"note"}]}`
	first := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, first, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	if !first.Rows[0].Expanded {
		t.Fatalf("lion = %+v", first.Rows[0])
	}
	closed := z.run(t, "ann", spec, Options{Expand: [][]any{}}, nil)
	requireRows(t, closed, "kind", "group", "group", "total")
	one := z.run(t, "ann", spec, Options{Expand: [][]any{first.Rows[3].GroupKey}}, nil)
	requireRows(t, one, "kind", "group", "group", "detail", "detail", "total")
}

func TestOpenFirstLevelLeavesTheSecondClosed(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","open":true},{"id":"h","col":"note"}],"sort":[{"col":"note"}]}`
	first := z.run(t, "ann", spec, Options{}, nil)
	for _, r := range first.Rows {
		if r.Kind == KindDetail {
			t.Fatalf("a detail row shows with the second level closed: %+v", r)
		}
	}
	if len(first.Rows) < 3 || first.Rows[1].Kind != KindGroup || first.Rows[1].Depth != 1 {
		t.Fatalf("level 1 did not open onto level 2: %+v", first.Rows)
	}
	all := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	if all.Rows[0].Expanded != true || len(all.Rows) <= len(first.Rows) {
		t.Fatalf("expandAll must open every level: %d rows", len(all.Rows))
	}
}

const openTwoLevels = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species","open":%t},{"id":"h","col":"note","open":%t}],"sort":[{"col":"note"}]}`

func TestOpenUnderAClosedAncestorFetchesNothing(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 3 })
	res := z.run(t, "ann", fmt.Sprintf(openTwoLevels, false, true), Options{}, nil)
	requireRows(t, res, "kind", "group", "group", "total")
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

func TestOpenStopsAtTheFirstClosedLevel(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"fed","path":[{"field":"fed"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","open":true},{"id":"h","col":"fed"},{"id":"i","col":"note","open":true}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	for _, r := range res.Rows {
		if r.Kind == KindDetail || (r.Kind == KindGroup && r.Depth == 2) {
			t.Fatalf("a row shows under the closed level: %+v", r)
		}
	}
}

func TestOpenIsIgnoredWhenTheRunSendsKeys(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(openTwoLevels, true, true), Options{Expand: [][]any{{"nothing"}}}, nil)
	requireRows(t, res, "kind", "group", "group", "total")
}

// Ruling R2: collapse (even empty) keeps the open levels; expand without
// collapse still replaces them (TestOpenIsIgnoredWhenTheRunSendsKeys).
func TestOpenLevelsStayOpenWhenTheRunSendsCollapse(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", pagedBySpecies, Options{Expand: [][]any{}, Collapse: [][]any{}}, nil)
	requireRows(t, res, "kind", "group", "opening", "detail", "detail", "subtotal", "group", "opening", "detail", "detail", "subtotal", "total")
}

func TestOpenWithHavingFillAndLayout(t *testing.T) {
	z := newZoo(t)
	having := strings.Replace(speciesHaving, `"groups":[{"id":"g","col":"species",`, `"groups":[{"id":"g","col":"species","open":true,`, 1)
	res := z.run(t, "ann", having, Options{}, map[string]string{"min": "1500"})
	requireRows(t, res, "label", "lion", "lion", "lion", "Total")

	fill := strings.Replace(animalFill(`{}`, ""), `"col":"animal",`, `"col":"animal","open":true,`, 1)
	closed := z.run(t, "ann", fill, Options{Expand: [][]any{}}, nil)
	opened := z.run(t, "ann", fill, Options{}, nil)
	if len(opened.Rows) <= len(closed.Rows) {
		t.Fatalf("open fill group did not open: %d vs %d rows", len(opened.Rows), len(closed.Rows))
	}

	layout := strings.Replace(layoutOf(lions), `"col":"species"`, `"col":"species","open":true`, 1)
	res = z.run(t, "ann", layout, Options{}, nil)
	if len(res.Rows) == 0 || res.Rows[0].Kind != KindGroup {
		t.Fatalf("layout rows = %+v", res.Rows)
	}
}

func TestOpenOptionsDecodeEmptyExpandAsAChoice(t *testing.T) {
	var absent, empty Options
	if err := json.Unmarshal([]byte(`{}`), &absent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"expand":[]}`), &empty); err != nil {
		t.Fatal(err)
	}
	if absent.Expand != nil || empty.Expand == nil {
		t.Fatalf("absent = %#v, empty = %#v", absent.Expand, empty.Expand)
	}
}
