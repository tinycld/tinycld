package reports

import (
	"reflect"
	"testing"
)

// plus shows a feeding's cost on detail rows and, on group, subtotal and
// total rows, its rollup: the cost added up, plus 1. again reads plus, so on
// those rows it reads the rollup too. Without the rollup plus is blank there
// (it reads a bare path).
func TestARollupShowsOnGroupSubtotalAndTotalRows(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"plus","formula":{"op":"add","args":[{"path":[{"field":"cost"}]},{"value":0}]},
			"rollup":{"op":"add","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1}]}},
		{"id":"again","formula":{"op":"add","args":[{"col":"plus"},{"value":0}]}}],
		"groups":[{"id":"g","col":"animal","subtotals":true}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "subtotal", "group", "detail", "detail", "subtotal", "total")
	requireOrderedRows(t, spec, res, "cost", int64(3500), int64(1000), int64(2500), int64(3500), int64(1000), int64(700), int64(300), int64(1000), int64(4500))
	requireOrderedRows(t, spec, res, "plus", int64(3501), int64(1000), int64(2500), int64(3501), int64(1001), int64(700), int64(300), int64(1001), int64(4501))
	requireOrderedRows(t, spec, res, "again", int64(3501), int64(1000), int64(2500), int64(3501), int64(1001), int64(700), int64(300), int64(1001), int64(4501))
	requireRows(t, res, "plus@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
}

// A rollup that reads the group's record has no record on the total row,
// so it is blank there as a whole (R6).
func TestARollupThatReadsARecordIsBlankOnTheTotal(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum",
			"rollup":{"op":"add","args":[{"record":"g","path":[{"field":"weight"}]},{"op":"sum","args":[{"path":[{"field":"grams"}]}]}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	// Leo weighs 190 (500 + 800 grams), Zed 300 (300 + 100 grams).
	requireOrderedRows(t, spec, res, "grams", int64(1490), int64(500), int64(800), int64(700), int64(300), int64(100), nil)
}

func TestARollupReadsLookupsAndParams(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","params":[{"id":"extra","type":"number"}],"columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum",
			"rollup":{"op":"add","args":[{"op":"sum","args":[{"path":[{"field":"grams"}]}]},{"param":"extra"},
				{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},
					"value":{"path":[{"field":"weight"}]}}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, map[string]string{"extra": `5`})
	// Each group and the total: its grams, 5, and Leo's 190.
	requireOrderedRows(t, spec, res, "grams", int64(1495), int64(500), int64(800), int64(595), int64(300), int64(100), int64(1895))
}

// A rollup adds a looked-up cost to the group's total cost: both read
// zoo.feedings.cost, whose currency is each feeding's own, so they combine
// (R24) and the rollup passes checkInColumn. f1 cost 1000; Leo's feedings
// total 3500, Zed's 1000, all 4500.
func TestARollupAddsALookedUpAmountOfItsColumnsField(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum",
			"rollup":{"op":"add","args":[
				{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","pick":"sum",
					"where":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]},
					"value":{"path":[{"field":"cost"}]}},{"value":0}]},
				{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireOrderedRows(t, spec, res, "cost", int64(4500), int64(1000), int64(2500), int64(2000), int64(700), int64(300), int64(5500))
	requireRows(t, res, "cost@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
}

func TestARollupIsRefusedWithRowsTotals(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum",
		"total":"rows","rollup":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}]}`))
	requireSpecError(t, err, 400, "columns.0.rollup", `a rollup replaces the column's totals: it does not go with total "rows"`)
}

func TestARollupIsRefusedOnAChildRowOrPivotCellColumn(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"notes","path":[{"link":"feedings","mode":"nest"},{"field":"note"}],"rollup":{"value":null}}]}`))
	requireSpecError(t, err, 400, "columns.1.rollup", "a child row column has no totals to replace")

	err = Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[{"id":"month","path":[{"field":"fed"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","rollup":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}],
		"groups":[{"id":"m","col":"month","bucket":"month"}],"pivot":{"path":[{"link":"animal"}],"cell":"grams"}}`))
	requireSpecError(t, err, 400, "columns.1.rollup", "a pivot's cell column is totalled by its pivot columns")
}

// A group row shows its key in the column the group groups by, so a rollup
// there would replace the key.
func TestARollupIsRefusedOnAColumnAGroupGroupsBy(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
		{"id":"weight","path":[{"field":"grams"}],"rollup":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}],
		"groups":[{"id":"g","col":"weight"}]}`))
	requireSpecError(t, err, 400, "columns.2.rollup", "a column a group is grouped by cannot have a rollup")
}

// A rollup shows in its column's cells: it has the column's type, and an
// amount is in the column's currency (prices are in the animals' fixed
// currency, costs in each feeding's own).
func TestARollupMustHaveItsColumnsTypeAndCurrency(t *testing.T) {
	app := withZooRegistry(t)
	cost := func(rollup string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","rollup":` + rollup + `}]}`
	}
	err := Validate(app, loadSpec(t, cost(`{"value":"x"}`)))
	requireSpecError(t, err, 400, "columns.0.rollup", "a rollup shows in its column: it must be money")
	err = Validate(app, loadSpec(t, cost(`{"op":"lookup","source":"zoo.animals","pick":"sum","value":{"path":[{"field":"price"}]}}`)))
	requireSpecError(t, err, 400, "columns.0.rollup", "a rollup's amount is in another currency than its column's")
	if err := Validate(app, loadSpec(t, cost(`{"value":null}`))); err != nil {
		t.Fatalf("a blank rollup: %v", err)
	}
}

// A rollup over a sum lookup that meets two currencies is blank, never the
// coalesce's 0, and a notice names its column (R3, P2). f3 is in EUR, so the
// cost of every feeding ann sees mixes USD and EUR. The report leaves f3 out,
// so its own rows are all USD and the only notice is the lookup's.
func TestARollupOverAMixedSumIsBlankWithANotice(t *testing.T) {
	z := newZoo(t)
	f3 := z.recs["f3"]
	f3.Set("currency", "EUR")
	if err := z.app.Save(f3); err != nil {
		t.Fatal(err)
	}
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum",
			"rollup":{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","pick":"sum",
				"value":{"path":[{"field":"cost"}]}},{"value":0}]}}],
		"filter":{"op":"ne","args":[{"path":[{"field":"note"}]},{"value":"f3"}]},
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "total")
	kinds, costs := rowsOf(res, "kind"), rowsOf(res, "cost")
	for i, kind := range kinds {
		if kind != "detail" && costs[i] != nil {
			t.Fatalf("a %s row's cost = %v, want blank (kinds %v, costs %v)", kind, costs[i], kinds, costs)
		}
	}
	want := []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"cost"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// A sort on a column with a rollup orders the groups by the rollup: grams
// has no aggregate, so without its rollup the groups would keep their label
// order (Leo, Zed). Leo's rollup is 1300, Zed's 400.
func TestASortOnARollupOrdersTheGroups(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],
			"rollup":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"grams"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "grams", int64(400), int64(1300), int64(1700))
}
