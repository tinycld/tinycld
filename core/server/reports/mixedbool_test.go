package reports

import (
	"fmt"
	"slices"
	"testing"
)

// positive is a condition that a sum is above 0: a Bool over it is unknown
// (blank) where the sum met two currencies.
func positive(sum string) string {
	return `{"op":"gt","args":[` + sum + `,{"value":0}]}`
}

// namesColumn is true when the run's mixedCurrency notice names column id.
func namesColumn(res *Result, id string) bool {
	return slices.ContainsFunc(res.Notices, func(n Notice) bool {
		return n.Code == "mixedCurrency" && slices.Contains(n.Columns, id)
	})
}

// requireBlankNamed checks that every row of kind shows no value in column
// id, that there is at least one, and that the notice names id (R3).
func requireBlankNamed(t *testing.T, res *Result, kind, id string) {
	t.Helper()
	seen := 0
	for _, r := range res.Rows {
		if r.Kind != kind {
			continue
		}
		seen++
		if v, ok := r.Cells[id]; ok && v != nil {
			t.Fatalf("%s row %q shows %s = %v, want blank", kind, r.Label, id, v)
		}
	}
	if seen == 0 {
		t.Fatalf("no %s row: %s", kind, rowKinds(res))
	}
	if !namesColumn(res, id) {
		t.Fatalf("the notice does not name %s: %+v", id, res.Notices)
	}
}

func rowKinds(res *Result) string {
	out := ""
	for _, r := range res.Rows {
		out += fmt.Sprintf("%s:%q ", r.Kind, r.Label)
	}
	return out
}

// A Bool column over a sum that met two currencies is blank on a detail row,
// and the notice names it. The column reads the row's cost, so a group and
// the total have no value there and do not name it.
func TestABoolDetailCellOverAMixedSumIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"pos","formula":{"op":"gt","args":[{"path":[{"field":"grams"}]},` + allCosts + `]}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireBlankNamed(t, res, KindDetail, "pos")
}

// A Bool column over the group's own mixed sum is blank on that group's row
// and named; a group whose sum is in one currency shows its value.
func TestABoolGroupCellOverAMixedSumIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"pos","formula":` + positive(leoCosts) + `}],
		"groups":[{"id":"g","col":"animal"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	_, leo := rowLabelled(t, res, "Leo")
	_, zed := rowLabelled(t, res, "Zed")
	if leo.Kind != KindGroup || leo.Cells["pos"] != nil {
		t.Fatalf("Leo = %+v", leo)
	}
	if zed.Kind != KindGroup || zed.Cells["pos"] != true {
		t.Fatalf("Zed = %+v", zed)
	}
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			t.Fatalf("a detail row %q is shown; the test needs the group rows alone", r.Label)
		}
	}
	if !namesColumn(res, "pos") {
		t.Fatalf("the notice does not name pos: %+v", res.Notices)
	}
}

// A Bool column over a mixed sum is blank on the total row and named. The
// having hides every group, so the total is the one row that shows it.
func TestABoolTotalCellOverAMixedSumIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"pos","formula":` + positive(allCosts) + `}],
		"groups":[{"id":"g","col":"animal","having":{"op":"lt","args":[{"col":"grams"},{"value":0}]}}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "kind", "total")
	requireBlankNamed(t, res, KindTotal, "pos")
}

// A Bool opening cell over a mixed sum is blank and named. The column's own
// cells read the row's grams, so the opening is the one cell blank for
// currencies.
func TestABoolOpeningCellOverAMixedSumIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"pos","formula":{"op":"gt","args":[{"path":[{"field":"grams"}]},{"value":0}]}}],
		"groups":[{"id":"g","col":"animal","open":true,"opening":{"label":"All feedings","cells":{"pos":` + positive(leoCosts) + `}}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	i, leo := rowLabelled(t, res, "Leo")
	if leo.Kind != KindGroup || res.Rows[i+1].Kind != KindOpening || res.Rows[i+1].Cells["pos"] != nil {
		t.Fatalf("Leo's opening = %+v", res.Rows[i+1])
	}
	j, _ := rowLabelled(t, res, "Zed")
	if res.Rows[j+1].Kind != KindOpening || res.Rows[j+1].Cells["pos"] != true {
		t.Fatalf("Zed's opening = %+v", res.Rows[j+1])
	}
	if !namesColumn(res, "pos") {
		t.Fatalf("the notice does not name pos: %+v", res.Notices)
	}
}

// A Bool column over a mixed sum is blank on a layout's section row and
// named. A layout opens collapsed, and a computed row has no Bool cell, so
// the section row is the one that shows it.
func TestABoolLayoutSectionCellOverAMixedSumIsNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"pos","formula":` + positive(allCosts) + `}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"lions","title":"Lions","key":"lion"},
			{"row":"price","label":"Double","formula":{"op":"mul","args":[{"subtotal":"lions"},{"value":2}]}}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireBlankNamed(t, res, KindGroup, "pos")
}

// A condition another term decides is not blank: an or with a true term
// shows true over a mixed sum, and the notice does not name its column.
func TestABoolCellAnotherTermDecidesIsNotNamed(t *testing.T) {
	z := newZoo(t)
	withEuroFeeding(t, z)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"any","formula":{"op":"or","args":[` + positive(allCosts) + `,{"op":"gt","args":[{"path":[{"field":"grams"}]},{"value":0}]}]}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	for _, r := range res.Rows {
		if r.Kind == KindDetail && r.Cells["any"] != true {
			t.Fatalf("detail %q any = %v, want true", r.Label, r.Cells["any"])
		}
	}
	if namesColumn(res, "any") {
		t.Fatalf("the notice names any, whose cells all show: %+v", res.Notices)
	}
}
