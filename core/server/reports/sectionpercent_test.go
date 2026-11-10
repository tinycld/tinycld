package reports

import "testing"

// percentStatement is ann's statement: Lions (f1 1000, f2 2500) and Zebras
// (f3 700, f4 300), labelled by note, with share = cost as a percent of a
// section. share may close its column and add more; extra adds top-level
// parts; layout adds layout items.
func percentStatement(share, groups, extra, layout string) string {
	return `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"share","formula":` + share + `}],
		"groups":` + groups + `,` + extra + `
		"layout":[{"section":"cats","title":"Lions","key":"lion"},{"section":"stripes","title":"Zebras","key":"zebra"}` + layout + `]}`
}

const (
	shareOfLions = `{"op":"percentOfSection","args":[{"col":"cost"}],"subtotal":"cats","scale":1}`
	bySpecies    = `[{"id":"g","col":"species"}]`
	sortByNote   = `"sort":[{"col":"note"}],`
	gapRow       = `,{"row":"gap","label":"Lions less zebras","formula":{"op":"sub","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}`
)

func TestPercentOfSectionIsAPercentColumn(t *testing.T) {
	z := newZoo(t)
	raw := percentStatement(shareOfLions, bySpecies, sortByNote, gapRow)
	if err := Validate(z.app, loadSpec(t, raw)); err != nil {
		t.Fatal(err)
	}
	res := z.run(t, "ann", raw, Options{}, nil)
	if c := res.Columns[3]; c.ID != "share" || c.Type != Percent {
		t.Fatalf("share column = %+v", c)
	}
}

func TestPercentOfSectionRefusals(t *testing.T) {
	z := newZoo(t)
	notLayout := `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"share","formula":` + shareOfLions + `}],"groups":[{"id":"g","col":"species"}]}`
	cases := map[string]struct {
		raw, at, want string
		status        int
	}{
		"outside a layout": {notLayout, "columns.2.formula", "only a report with a layout", 400},
		"unknown section": {percentStatement(`{"op":"percentOfSection","args":[{"col":"cost"}],"subtotal":"dogs","scale":1}`, bySpecies, "", ""),
			"columns.3.formula.subtotal", "no section", 422},
		"no section": {percentStatement(`{"op":"percentOfSection","args":[{"col":"cost"}],"scale":1}`, bySpecies, "", ""),
			"columns.3.formula.subtotal", "names its section", 400},
		"argument not a column": {percentStatement(`{"op":"percentOfSection","args":[{"value":5}],"subtotal":"cats","scale":1}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "takes one column", 400},
		"a null argument": {percentStatement(`{"op":"percentOfSection","args":[null],"subtotal":"cats","scale":1}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "takes one column", 400},
		"argument not a number": {percentStatement(`{"op":"percentOfSection","args":[{"col":"note"}],"subtotal":"cats","scale":1}`, bySpecies, "", ""),
			"columns.3.formula", "needs a number", 400},
		"inside an operator": {percentStatement(`{"op":"add","args":[`+shareOfLions+`,{"value":1}]}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "whole formula", 400},
		"inside a window op": {percentStatement(`{"op":"runningSum","args":[`+shareOfLions+`]}`, bySpecies, "", ""),
			"columns.3.formula.args.0", "whole formula", 400},
		"read by another column": {percentStatement(shareOfLions+`},{"id":"double","formula":{"op":"mul","args":[{"col":"share"},{"value":2}]}`, bySpecies, "", ""),
			"columns.4.formula.args.0", "no formula reads", 400},
		"with an aggregate": {percentStatement(shareOfLions+`,"aggregate":"sum"`, bySpecies, "", ""),
			"columns.3.aggregate", "no aggregate", 400},
		"the pivot's cell": {percentStatement(shareOfLions, bySpecies, `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"share"},`, ""),
			"pivot.cell", "a column with an aggregate", 400},
		"a computed row's cell": {percentStatement(shareOfLions, bySpecies, "", `,{"row":"r","label":"R","formula":{"subtotal":"cats"},"cells":{"share":{"value":1}}}`),
			"layout.2.cells.share", "no cell formula", 400},
		"subtotal beside another op": {percentStatement(`{"op":"add","subtotal":"cats","args":[{"col":"cost"},{"value":1}]}`, bySpecies, "", ""),
			"columns.3.formula", "only percentOfSection takes a subtotal", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", c.raw, Options{}, nil)
			requireSpecError(t, err, c.status, c.at, c.want)
		})
	}
}
