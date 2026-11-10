package reports

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

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

// sharesOf is the share cell of each row of the kinds (every kind when none),
// by label.
func sharesOf(res *Result, kinds ...string) map[string]any {
	out := map[string]any{}
	for _, r := range res.Rows {
		if len(kinds) == 0 || slices.Contains(kinds, r.Kind) {
			out[r.Label] = r.Cells["share"]
		}
	}
	return out
}

func requireShares(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !maps.EqualFunc(got, want, func(a, b any) bool { return reflect.DeepEqual(a, b) }) {
		t.Fatalf("shares = %v\nwant     %v", got, want)
	}
}

func TestPercentOfSectionOnEveryRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, gapRow), Options{ExpandAll: true}, nil)
	// Every row divides its own cost by the Lions total, 3500.
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 100.0, "f1": 28.6, "f2": 71.4, "Total Lions": 100.0,
		"Zebras": 28.6, "f3": 20.0, "f4": 8.6, "Total Zebras": 28.6,
		"Lions less zebras": 71.4,
	})
	stripes := strings.ReplaceAll(shareOfLions, `"cats"`, `"stripes"`)
	res = z.run(t, "ann", percentStatement(stripes, bySpecies, sortByNote, gapRow), Options{ExpandAll: true}, nil)
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 350.0, "f1": 100.0, "f2": 250.0, "Total Lions": 350.0,
		"Zebras": 100.0, "f3": 70.0, "f4": 30.0, "Total Zebras": 100.0,
		"Lions less zebras": 250.0,
	})
}

// A section's total holds its within rows, so the percent divides by it.
func TestPercentOfSectionDividesByTheTotalWithItsWithinRows(t *testing.T) {
	z := newZoo(t)
	within := `,{"row":"fees","label":"Lion fees","within":"cats","formula":{"subtotal":"stripes"}}` + gapRow
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, within), Options{ExpandAll: true}, nil)
	// Lions 3500 + fees 1000 = 4500.
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 100.0, "f1": 22.2, "f2": 55.6, "Lion fees": 22.2, "Total Lions": 100.0,
		"Zebras": 22.2, "f3": 15.6, "f4": 6.7, "Total Zebras": 22.2,
		"Lions less zebras": 77.8,
	})
}

func TestPercentOfSectionOnASecondLevelSubtotal(t *testing.T) {
	z := newZoo(t)
	twoLevels := `[{"id":"g","col":"species"},{"id":"n","col":"note","subtotals":true}]`
	res := z.run(t, "ann", percentStatement(shareOfLions, twoLevels, sortByNote, ""), Options{ExpandAll: true}, nil)
	// The second-level group rows (f1 … f4) and their subtotals, with the
	// sections' own rows, each divide by the Lions total.
	requireShares(t, sharesOf(res, KindGroup, KindSubtotal), map[string]any{
		"Lions": 100.0, "f1": 28.6, "f2": 71.4,
		"Total f1": 28.6, "Total f2": 71.4, "Total Lions": 100.0,
		"Zebras": 28.6, "f3": 20.0, "f4": 8.6,
		"Total f3": 20.0, "Total f4": 8.6, "Total Zebras": 28.6,
	})
}

func TestPercentOfAnEmptySectionHasNoFigure(t *testing.T) {
	z := newZoo(t)
	// Ann sees no monkey: the Monkeys total is 0.
	apes := strings.ReplaceAll(shareOfLions, `"cats"`, `"apes"`)
	monkeys := `,{"section":"apes","title":"Monkeys","key":"monkey"}` + gapRow
	// lionShare in the same report shows the run fills figures at all.
	withLions := apes + `},{"id":"lionShare","formula":` + shareOfLions
	res := z.run(t, "ann", percentStatement(withLions, bySpecies, sortByNote, monkeys), Options{}, nil)
	for label, share := range sharesOf(res) {
		if share != nil {
			t.Fatalf("%s share = %v, want blank", label, share)
		}
	}
	if got := res.Rows[0].Cells["lionShare"]; got != 100.0 {
		t.Fatalf("Lions lionShare = %v, want 100.0", got)
	}
}

// On a pivot report the percent reads the cell column's total across the
// pivot (D10).
func TestPercentOfSectionOnAPivotReport(t *testing.T) {
	z := newZoo(t)
	pivot := `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"},`
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, pivot, gapRow), Options{}, nil)
	requireShares(t, sharesOf(res), map[string]any{
		"Lions": 100.0, "Total Lions": 100.0, "Zebras": 28.6, "Total Zebras": 28.6, "Lions less zebras": 71.4,
	})
	requireRows(t, res, "cost.p1", int64(2500), int64(2500), int64(0), int64(0), int64(2500))
}

func TestPercentOfSectionDividesByItsOwnCurrencysTotal(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f9", "zoo_feedings", map[string]any{
		"animal": z.recs["leo"].Id, "cost": 1000, "currency": "EUR", "fed": "2026-01-07 00:00:00.000Z", "grams": 5, "note": "f9", "due": "2026-01-07",
	})
	res := z.run(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, gapRow), Options{}, nil)
	var lions []any
	var zebrasUSD any
	for _, r := range res.Rows {
		switch {
		case r.Kind == KindGroup && r.Label == "Lions":
			lions = append(lions, r.Cells["share"])
		case r.Kind == KindGroup && r.Label == "Zebras" && strings.Contains(r.ID, `"USD"`):
			zebrasUSD = r.Cells["share"]
		}
	}
	// EUR Lions 1000 of 1000, USD Lions 3500 of 3500: each its own block's.
	if !reflect.DeepEqual(lions, []any{100.0, 100.0}) || zebrasUSD != 28.6 {
		t.Fatalf("lions = %v, USD zebras = %v", lions, zebrasUSD)
	}
}

func TestPercentOfSectionReadsTheTotalWhenTheSectionIsFolded(t *testing.T) {
	z := newZoo(t)
	raw := percentStatement(shareOfLions, bySpecies, sortByNote, gapRow)
	want := map[string]any{"Lions": 100.0, "Total Lions": 100.0, "Zebras": 28.6, "Total Zebras": 28.6, "Lions less zebras": 71.4}
	folded := z.run(t, "ann", raw, Options{}, nil)
	requireShares(t, sharesOf(folded), want)
	open := z.run(t, "ann", raw, Options{ExpandAll: true}, nil)
	requireShares(t, sharesOf(open, KindGroup, KindSubtotal, KindComputed), want)
}

// A percent multiplies its part by 100, so a part near the int64 limit
// overflows even though the section total still fits.
func TestPercentOfSectionReportsAnAmountTooLarge(t *testing.T) {
	z := newZoo(t)
	if _, err := z.app.DB().NewQuery("UPDATE zoo_feedings SET cost = 4611686018427387904 WHERE note = 'f1'").Execute(); err != nil {
		t.Fatal(err)
	}
	_, err := z.try(t, "ann", percentStatement(shareOfLions, bySpecies, sortByNote, gapRow), Options{ExpandAll: true}, nil)
	if !errors.Is(err, ErrAmountTooLarge) {
		t.Fatalf("err = %v, want ErrAmountTooLarge", err)
	}
}
