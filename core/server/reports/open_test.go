package reports

import "testing"

func TestOpenGroupsOpenWhenTheRunHasNoExpansion(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[
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
	spec := `{"version":2,"source":"zoo.feedings","columns":[
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
