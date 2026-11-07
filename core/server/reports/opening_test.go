package reports

import (
	"slices"
	"strings"
	"testing"
)

// beforePeriod is the grams of this group's animal fed before the period: a
// sum lookup read on the group's level (its record), cut at the period's start.
const beforePeriod = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings",
	"where":{"op":"and","args":[
		{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
		{"op":"lt","args":[{"path":[{"field":"due"}]},{"param":"period","bound":"from"}]}]},
	"value":{"path":[{"field":"grams"}]},"pick":"sum"},{"value":0}]}`

// gramsByAnimal groups ann's feedings in the period by animal, each animal's
// group opening onto the grams fed before the period. group adds group parts
// (fill, having) after the opening.
func gramsByAnimal(cells, group string) string {
	return `{"version":3,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"day","path":[{"field":"due"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"filter":{"op":"between","args":[{"path":[{"field":"due"}]},{"param":"period"}]},
		"groups":[{"id":"g","col":"animal","open":true,"subtotals":true,
			"opening":{"label":"Before the period","cells":` + cells + `}` + group + `}],
		"sort":[{"col":"note"}]}`
}

var fromFebruary = map[string]string{"period": `{"from":"2026-02-01","to":"2026-12-31"}`}

// rowLabelled is the first row with the label, failing the test without one.
func rowLabelled(t *testing.T, res *Result, label string) (int, Row) {
	t.Helper()
	i := slices.IndexFunc(res.Rows, func(r Row) bool { return r.Label == label })
	if i < 0 {
		t.Fatalf("no row %q in %v", label, rowsOf(res, "label"))
	}
	return i, res.Rows[i]
}

func TestAnOpeningRowIsFirstUnderEachOpenGroup(t *testing.T) {
	z := newZoo(t)
	spec := gramsByAnimal(`{"grams":`+beforePeriod+`}`, "")
	res := z.run(t, "ann", spec, Options{}, fromFebruary)
	requireRows(t, res, "kind", "group", "opening", "detail", "subtotal", "group", "opening", "detail", "subtotal", "total")
	requireOrderedRows(t, spec, res, "note", nil, nil, "f2", nil, nil, nil, "f4", nil, nil)
	// Leo: f1 (500) is before the period; Zed: f3 (300).
	requireOrderedRows(t, spec, res, "grams", int64(800), int64(500), int64(800), int64(800), int64(100), int64(300), int64(100), int64(100), int64(900))
	for _, i := range []int{1, 5} {
		group, opening := res.Rows[i-1], res.Rows[i]
		want := "opening:" + keyString(group.GroupKey)
		if opening.ID != want || opening.Depth != 1 || opening.Section != group.ID || opening.Label != "Before the period" ||
			keyString(opening.GroupKey) != keyString(group.GroupKey) {
			t.Fatalf("opening row %d = %+v, want id %s under %s", i, opening, want, group.ID)
		}
	}
}

func TestAClosedGroupHasNoOpeningRow(t *testing.T) {
	z := newZoo(t)
	spec := gramsByAnimal(`{"grams":`+beforePeriod+`}`, "")
	closed := z.run(t, "ann", spec, Options{Expand: [][]any{}}, fromFebruary)
	requireRows(t, closed, "kind", "group", "group", "total")
	_, zed := rowLabelled(t, closed, "Zed")
	open := z.run(t, "ann", spec, Options{Expand: [][]any{zed.GroupKey}}, fromFebruary)
	requireRows(t, open, "kind", "group", "group", "opening", "detail", "subtotal", "total")
	requireRows(t, open, "label", "Leo", "Zed", "Before the period", "Zed", "Total Zed", "Total")
}

// A fill record has no rows in the period, yet its group opens onto its
// opening row and subtotal (R5d). Kit's feeding f6 is not one ann may read,
// so the lookup finds nothing and the opening shows 0.
func TestAFillGroupWithNoRowsOpensOntoItsOpeningRow(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	fill := `,"fill":{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}`
	res := z.run(t, "ann", gramsByAnimal(`{"grams":`+beforePeriod+`}`, fill), Options{}, fromFebruary)
	i, kit := rowLabelled(t, res, "Kit")
	if !kit.CanExpand || !kit.Expanded {
		t.Fatalf("Kit = %+v", kit)
	}
	if got := []string{res.Rows[i+1].Kind, res.Rows[i+2].Kind}; !slices.Equal(got, []string{KindOpening, KindSubtotal}) {
		t.Fatalf("under Kit: %v", got)
	}
	if res.Rows[i+1].Cells["grams"] != int64(0) || res.Rows[i+2].Label != "Total Kit" {
		t.Fatalf("Kit's opening = %+v, then %+v", res.Rows[i+1], res.Rows[i+2])
	}
	_, leo := rowLabelled(t, res, "Leo")
	if leo.Kind != KindGroup {
		t.Fatalf("Leo = %+v", leo)
	}
}

// Regression guard: Zed's 100 grams in the period fail the having, so Zed's
// group is hidden, and with it its opening row. A hidden group row is never
// emitted, so this guards the order of the emitted rows, not a new check.
func TestRegressionGuardAGroupItsHavingHidesHasNoOpeningRow(t *testing.T) {
	z := newZoo(t)
	having := `,"having":{"op":"gte","args":[{"op":"sum","args":[{"path":[{"field":"grams"}]}]},{"value":500}]}`
	res := z.run(t, "ann", gramsByAnimal(`{"grams":`+beforePeriod+`}`, having), Options{}, fromFebruary)
	requireRows(t, res, "kind", "group", "opening", "detail", "subtotal", "total")
	requireRows(t, res, "label", "Leo", "Before the period", "Leo", "Total Leo", "Total")
}

// sectionsWithOpening is a layout report whose sections (the species groups)
// each open onto an opening row whose grams cell is the section's own total
// of grams, read on the group's level.
const sectionsWithOpening = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species","opening":{"label":"Start","cells":{"grams":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}}}],
	"sort":[{"col":"note"}],
	"layout":[
		{"section":"cats","title":"Lions","key":"lion"},
		{"section":"stripes","title":"Zebras","key":"zebra"},
		{"section":"apes","title":"Monkeys","key":"monkey"},
		{"row":"gap","label":"Lions less zebras","formula":{"op":"sub","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}]}`

// An open layout section starts with its opening row, the empty Monkeys
// section too; a closed section has none, and a computed row never has one.
func TestAnOpeningRowIsFirstUnderEachOpenLayoutSection(t *testing.T) {
	z := newZoo(t)
	closed := z.run(t, "ann", sectionsWithOpening, Options{}, nil)
	requireRows(t, closed, "kind", "group", "subtotal", "group", "subtotal", "group", "subtotal", "computed")
	open := z.run(t, "ann", sectionsWithOpening, Options{ExpandAll: true}, nil)
	requireRows(t, open, "kind",
		"group", "opening", "detail", "detail", "subtotal",
		"group", "opening", "detail", "detail", "subtotal",
		"group", "opening", "subtotal",
		"computed")
	requireRows(t, open, "label",
		"Lions", "Start", "lion", "lion", "Total Lions",
		"Zebras", "Start", "zebra", "zebra", "Total Zebras",
		"Monkeys", "Start", "Total Monkeys",
		"Lions less zebras")
	requireOrderedRows(t, sectionsWithOpening, open, "grams",
		int64(1300), int64(1300), int64(500), int64(800), int64(1300),
		int64(400), int64(400), int64(300), int64(100), int64(400),
		nil, nil, nil,
		int64(900))
	for _, i := range []int{1, 6, 11} {
		section, opening := open.Rows[i-1], open.Rows[i]
		if want := "opening:" + keyString(section.GroupKey); opening.ID != want || opening.Depth != 1 || opening.Section != section.ID {
			t.Fatalf("opening row %d = %+v, want id %s under %s", i, opening, want, section.ID)
		}
		if _, ok := opening.Cells["cost"]; ok {
			t.Fatalf("cost has an opening cell: %#v", opening.Cells)
		}
	}
}

// An opening on the inner level (animals inside species) puts an opening row
// first under each open animal group, at depth 2, reading that animal's
// record; the outer species groups have none.
func TestAnOpeningRowOnAnInnerLevelIsFirstUnderEachOpenInnerGroup(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":3,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[
			{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"filter":{"op":"between","args":[{"path":[{"field":"due"}]},{"param":"period"}]},
		"groups":[{"id":"s","col":"species","open":true},
			{"id":"g","col":"animal","open":true,"subtotals":true,"opening":{"label":"Before the period","cells":{"grams":` + beforePeriod + `}}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, fromFebruary)
	requireRows(t, res, "kind",
		"group", "group", "opening", "detail", "subtotal",
		"group", "group", "opening", "detail", "subtotal",
		"total")
	requireRows(t, res, "label",
		"lion", "Leo", "Before the period", "lion", "Total Leo",
		"zebra", "Zed", "Before the period", "zebra", "Total Zed",
		"Total")
	// Leo: f1 (500) is before the period, f2 (800) in it; Zed: f3 (300), f4 (100).
	requireOrderedRows(t, spec, res, "grams",
		int64(800), int64(800), int64(500), int64(800), int64(800),
		int64(100), int64(100), int64(300), int64(100), int64(100),
		int64(900))
	for _, i := range []int{2, 7} {
		group, opening := res.Rows[i-1], res.Rows[i]
		want := "opening:" + keyString(group.GroupKey)
		if group.Depth != 1 || opening.ID != want || opening.Depth != 2 || opening.Section != group.ID ||
			keyString(opening.GroupKey) != keyString(group.GroupKey) {
			t.Fatalf("opening row %d = %+v, want id %s at depth 2 under %+v", i, opening, want, group)
		}
	}
}

// An opening cell reads the group's record, a lookup and a param; a column the
// opening names no cell for is blank.
func TestOpeningCellsReadARecordALookupAndAParam(t *testing.T) {
	z := newZoo(t)
	cells := `{"note":{"record":"g","path":[{"field":"name"}]},
		"day":{"param":"period","bound":"from"},
		"grams":` + beforePeriod + `}`
	res := z.run(t, "ann", gramsByAnimal(cells, ""), Options{}, fromFebruary)
	opening := res.Rows[1]
	if opening.Kind != KindOpening {
		t.Fatalf("row 1 = %+v", opening)
	}
	if opening.Cells["note"] != "Leo" || opening.Cells["day"] != "2026-02-01" || opening.Cells["grams"] != int64(500) {
		t.Fatalf("cells = %#v", opening.Cells)
	}
	if _, ok := opening.Cells["cost"]; ok {
		t.Fatalf("cost has a cell: %#v", opening.Cells)
	}
}

// leoCosts is the sum of every cost of this group's animal, whichever
// currency each feeding is in.
const leoCosts = `{"op":"lookup","source":"zoo.feedings",
	"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
	"value":{"path":[{"field":"cost"}]},"pick":"sum"}`

// openingOverTwoCurrencies runs a report whose bal column is the base row's
// cost, and whose opening cell in bal is the formula cell. Leo has a feeding
// in EUR (f7) that the filter keeps off the base rows, so only a lookup meets
// it: the column's own cells never mix currencies, and no total has two.
func openingOverTwoCurrencies(t *testing.T, cell string) *Result {
	t.Helper()
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("leo"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"bal","path":[{"field":"cost"}]}],
		"filter":{"op":"ne","args":[{"path":[{"field":"note"}]},{"value":"f7"}]},
		"groups":[{"id":"g","col":"animal","open":true,"opening":{"label":"All feedings","cells":{"bal":` + cell + `}}}],
		"sort":[{"col":"note"}]}`
	return z.run(t, "ann", spec, Options{}, nil)
}

// requireBlankMixedOpening checks that Leo's opening cell in bal is blank, that
// Zed's (all USD) is 1000, and that the run names bal in its mixedCurrency
// notice (R3).
func requireBlankMixedOpening(t *testing.T, res *Result) {
	t.Helper()
	_, leo := rowLabelled(t, res, "Leo")
	i, _ := rowLabelled(t, res, "Zed")
	leoOpening, zedOpening := res.Rows[1], res.Rows[i+1]
	if leo.Kind != KindGroup || leoOpening.Kind != KindOpening || leoOpening.Cells["bal"] != nil {
		t.Fatalf("Leo's opening = %+v", leoOpening)
	}
	if zedOpening.Kind != KindOpening || zedOpening.Cells["bal"] != int64(1000) {
		t.Fatalf("Zed's opening = %+v", zedOpening)
	}
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "bal") }) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// A sum lookup that meets two currencies has no value in an opening cell, and
// the run says so (R3).
func TestAnOpeningSumOverTwoCurrenciesIsBlankWithANotice(t *testing.T) {
	requireBlankMixedOpening(t, openingOverTwoCurrencies(t, leoCosts))
}

// A coalesce over a mixed sum does not turn it into its fallback: the cell
// stays blank, never 0, with the notice (R3).
func TestAnOpeningCoalesceOverAMixedSumIsBlankWithANotice(t *testing.T) {
	requireBlankMixedOpening(t, openingOverTwoCurrencies(t, `{"op":"coalesce","args":[`+leoCosts+`,{"value":0}]}`))
}

// An opening cell in the cost column adds up the animal's costs before the
// period: a sum lookup of zoo.feedings.cost, whose currency each feeding
// carries, combines with the column's cost (R24) and carries its currency.
func TestAnOpeningCellAddsUpItsColumnsAmounts(t *testing.T) {
	z := newZoo(t)
	const costBefore = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
			{"op":"lt","args":[{"path":[{"field":"due"}]},{"param":"period","bound":"from"}]}]},
		"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]}`
	spec := gramsByAnimal(`{"cost":`+costBefore+`}`, "")
	res := z.run(t, "ann", spec, Options{}, fromFebruary)
	requireRows(t, res, "kind", "group", "opening", "detail", "subtotal", "group", "opening", "detail", "subtotal", "total")
	// Leo: f1 (1000) is before the period, f2 (2500) in it; Zed: f3 (700), f4 (300).
	requireOrderedRows(t, spec, res, "cost", int64(2500), int64(1000), int64(2500), int64(2500), int64(300), int64(700), int64(300), int64(300), int64(2800))
	requireRows(t, res, "cost@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
}

func TestOpeningIsRefusedWhereItCannotShow(t *testing.T) {
	app := withZooRegistry(t)
	withOpening := func(opening string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"animal","opening":` + opening + `}]}`
	}
	cases := map[string]struct {
		spec     string
		status   int
		at, want string
	}{
		"the first column": {withOpening(`{"label":"Start","cells":{"animal":{"value":null}}}`), 400,
			"groups.0.opening.cells.animal", "the opening row's label fills the first column"},
		"an unknown column": {withOpening(`{"label":"Start","cells":{"nosuch":{"value":1}}}`), 422,
			"groups.0.opening.cells.nosuch", `the report has no column "nosuch"`},
		"another type": {withOpening(`{"label":"Start","cells":{"note":{"value":5}}}`), 400,
			"groups.0.opening.cells.note", "an opening cell shows in its column: it must be"},
		// The animal's fee carries its fee_currency; cost carries its row's.
		"another currency": {withOpening(`{"label":"Start","cells":{"cost":{"record":"g","path":[{"field":"fee"}]}}}`), 400,
			"groups.0.opening.cells.cost", "an opening cell's amount is in another currency than its column's"},
		"no label": {withOpening(`{"label":""}`), 400,
			"groups.0.opening.label", "an opening row has a label of 1 to 200 characters"},
		"a long label": {withOpening(`{"label":"` + strings.Repeat("x", 201) + `"}`), 400,
			"groups.0.opening.label", "an opening row has a label of 1 to 200 characters"},
		"a child row column": {`{"version":3,"source":"zoo.animals","columns":[
			{"id":"name","path":[{"field":"name"}]},
			{"id":"notes","path":[{"link":"feedings","mode":"nest"},{"field":"note"}]}],
			"groups":[{"id":"g","col":"name","opening":{"label":"Start","cells":{"notes":{"value":null}}}}]}`, 400,
			"groups.0.opening.cells.notes", "a child row column has no opening cell"},
		"a pivot's last level": {`{"version":3,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"animal","opening":{"label":"Start"}}],
			"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams"}}`, 400,
			"groups.0.opening", "this level never opens, so it has no opening row"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, Validate(app, loadSpec(t, c.spec)), c.status, c.at, c.want)
		})
	}
}
