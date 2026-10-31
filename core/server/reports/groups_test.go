package reports

import (
	"fmt"
	"reflect"
	"testing"
)

const feedingsBySpecies = `{"version":1,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"bySpecies","col":"species","subtotals":true}],
	"sort":[{"col":"note"}]}`

func TestGroupsLoadLazily(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsBySpecies, Options{}, nil)
	requireRows(t, res, "kind", "group", "group", "total")
	requireRows(t, res, "label", "lion", "zebra", "Total")
	requireRows(t, res, "cost", int64(3500), int64(1000), int64(4500))
	lion := res.Rows[0]
	if !reflect.DeepEqual(lion.GroupKey, []any{"lion", "USD"}) || lion.Expanded || lion.ID != `g:["lion","USD"]` {
		t.Fatalf("lion = %+v", lion)
	}

	open := z.run(t, "ann", feedingsBySpecies, Options{Expand: [][]any{lion.GroupKey}}, nil)
	requireRows(t, open, "kind", "group", "detail", "detail", "subtotal", "group", "total")
	requireRows(t, open, "note", nil, "f1", "f2", nil, nil, nil)
	requireRows(t, open, "label", "lion", "lion", "lion", "Total lion", "zebra", "Total")
	if !open.Rows[0].Expanded || open.Rows[1].Section != lion.ID || open.Rows[1].Depth != 1 || open.Rows[3].Section != lion.ID {
		t.Fatalf("rows = %+v", open.Rows[:4])
	}
}

// A run expands at most as many groups as it can show rows.
func TestExpandTakesAtMostTheRowCapOfKeys(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 2 })
	keys := [][]any{{"lion", "USD"}, {"zebra", "USD"}}
	z.run(t, "ann", feedingsBySpecies, Options{Expand: keys}, nil)
	_, err := z.try(t, "ann", feedingsBySpecies, Options{Expand: append(keys, []any{"monkey", "USD"})}, nil)
	requireSpecError(t, err, 400, "options.expand", "at most 2 groups")
}

// sixLevels groups feedings six deep; a key of its last level holds six
// group values and the cost's currency.
const sixLevels = `{"version":1,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"grams","path":[{"field":"grams"}]},
	{"id":"cur","path":[{"field":"currency"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g0","col":"species"},{"id":"g1","col":"name"},{"id":"g2","col":"note"},
		{"id":"g3","col":"fed"},{"id":"g4","col":"grams"},{"id":"g5","col":"cur"}]}`

func deepKeys(n int) [][]any {
	keys := make([][]any, n)
	for i := range keys {
		keys[i] = []any{"lion", "Leo", fmt.Sprintf("f%d", i), "2026-01-05", float64(500), "USD", "USD"}
	}
	return keys
}

// The keys an expand binds stay under SQLite's limit of 32,766 variables:
// 5,000 keys of 7 values each would bind 35,000, a 400 here and not a 500
// from the database. 1,200 keys run: more keys than SQLite's expression
// depth of 1,000 still compile.
func TestExpandBindsAtMost30000Values(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", sixLevels, Options{Expand: deepKeys(5000)}, nil)
	requireSpecError(t, err, 400, "options.expand", "30000 values")
	z.run(t, "ann", sixLevels, Options{Expand: deepKeys(1200)}, nil)
}

// An expanded key matches by IS, so a blank group key opens too.
func TestExpandOpensABlankGroup(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.animals","columns":[
		{"id":"region","path":[{"link":"site"},{"field":"region"}]},
		{"id":"name","path":[{"field":"name"}]}],
		"groups":[{"id":"g","col":"region"}],"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{Expand: [][]any{{nil}}}, nil)
	requireRows(t, res, "kind", "group", "group", "group", "detail", "total")
	requireRows(t, res, "name", nil, nil, nil, "Kit", nil)
}

func TestTwoGroupLevels(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"site","path":[{"link":"animal"},{"link":"site"},{"field":"name"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"bySite","col":"site","subtotals":true},{"id":"bySpecies","col":"species"}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "bob", spec, Options{}, nil)
	requireRows(t, res, "label", "North", "South", "Total")
	requireRows(t, res, "cost", int64(150), int64(1000), int64(1150))

	north := res.Rows[0].GroupKey
	res = z.run(t, "bob", spec, Options{Expand: [][]any{north}}, nil)
	requireRows(t, res, "label", "North", "monkey", "Total North", "South", "Total")
	requireRows(t, res, "site", "North", "North", "North", "South", nil)
	if res.Rows[1].Depth != 1 || res.Rows[1].Expanded || !reflect.DeepEqual(res.Rows[1].GroupKey, []any{"North", "monkey", "USD"}) {
		t.Fatalf("monkey = %+v", res.Rows[1])
	}

	res = z.run(t, "bob", spec, Options{Expand: [][]any{north, {"North", "monkey", "USD"}}}, nil)
	requireRows(t, res, "label", "North", "monkey", "North", "Total North", "South", "Total")
	requireRows(t, res, "note", nil, nil, "f5", nil, nil, nil)
	if res.Rows[2].Depth != 2 {
		t.Fatalf("detail depth = %d", res.Rows[2].Depth)
	}

	all := z.run(t, "bob", spec, Options{ExpandAll: true}, nil)
	requireRows(t, all, "kind", "group", "group", "detail", "subtotal", "group", "group", "detail", "detail", "subtotal", "total")
	requireRows(t, all, "note", nil, nil, "f5", nil, nil, nil, "f3", "f4", nil, nil)
}

func TestAToManyTotalCountsEachParentOnce(t *testing.T) {
	z := newZoo(t)
	// Each feeding row carries its animal's keeper count. Leo has two
	// feedings: adding the per-row counts would count leo's keeper twice.
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"site","path":[{"link":"animal"},{"link":"site"},{"field":"name"}]},
		{"id":"keepers","path":[{"link":"animal"},{"link":"keepers","mode":"aggregate"},{"field":"id"}],"aggregate":"count"},
		{"id":"grams","path":[{"link":"animal"},{"link":"feedings","mode":"aggregate"},{"field":"grams"}],"aggregate":"avg","scale":0}],
		"groups":[{"id":"bySite","col":"site"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireRows(t, res, "keepers", int64(1), int64(1), int64(1), int64(1), int64(1), int64(1), int64(2))
	// Leo's feedings average 650 g, zed's 200 g; together (1300 + 400) / 4.
	requireRows(t, res, "grams", int64(650), int64(650), int64(650), int64(200), int64(200), int64(200), int64(425))
}

func TestGroupsSortByTheirTotals(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"cost","dir":"%s"}]}`
	requireRows(t, z.run(t, "ann", fmt.Sprintf(spec, "asc"), Options{}, nil), "label", "zebra", "lion", "Total")
	requireRows(t, z.run(t, "ann", fmt.Sprintf(spec, "desc"), Options{}, nil), "label", "lion", "zebra", "Total")
}

func TestGroupByAMonthBucket(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"fed","path":[{"field":"fed"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"fed","bucket":"month"}]}`, Options{}, nil)
	requireRows(t, res, "label", "2026-01", "2026-02", "2026-03", "Total")
	requireRows(t, res, "cost", int64(1700), int64(2500), int64(300), int64(4500))
	// A bucketed group column has no single value on its group row.
	requireRows(t, res, "fed", nil, nil, nil, nil)
}

// A formula over a group column is computed where the column is a key and
// blank where it is not (the total row), never computed from a blank.
func TestAFormulaOverAGroupColumnIsBlankWhereThereIsNoKey(t *testing.T) {
	z := newZoo(t)
	byGrams := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"grams","path":[{"field":"grams"}]},
		{"id":"g2","formula":{"op":"mul","args":[{"col":"grams"},{"value":2}]}}],
		"groups":[{"id":"g","col":"grams"}]}`, Options{}, nil)
	requireRows(t, byGrams, "g2", int64(200), int64(600), int64(1000), int64(1600), nil)

	byBand := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"band","formula":{"op":"div","args":[{"path":[{"field":"grams"}]},{"value":400}],"scale":0}},
		{"id":"band2","formula":{"op":"mul","args":[{"col":"band"},{"value":2}]}}],
		"groups":[{"id":"g","col":"band"}]}`, Options{}, nil)
	requireRows(t, byBand, "band2", int64(0), int64(2), int64(4), nil)
}

// A formula column groups like a field: its value per row is the key, and
// its group row shows the key.
func TestGroupByAFormulaColumn(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"bucket","formula":{"op":"div","args":[{"path":[{"field":"grams"}]},{"value":400}],"scale":0}},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"bucket","subtotals":true}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	// ann's grams 500, 800, 300 and 100 divide to 1, 2, 1 and 0.
	requireRows(t, res, "label", "0", "1", "2", "Total")
	requireRows(t, res, "bucket", int64(0), int64(1), int64(2), nil)
	requireRows(t, res, "cost", int64(300), int64(1700), int64(2500), int64(4500))

	open := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, open, "kind", "group", "detail", "subtotal", "group", "detail", "detail", "subtotal", "group", "detail", "subtotal", "total")
	requireRows(t, open, "note", nil, "f4", nil, nil, "f1", "f3", nil, nil, "f2", nil, nil)
	requireRows(t, open, "bucket", int64(0), int64(0), int64(0), int64(1), int64(1), int64(1), int64(1), int64(2), int64(2), int64(2), nil)
}

func TestGroupLabelsUseSelectLabelsAndNone(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.animals","columns":[
		{"id":"region","path":[{"link":"site"},{"field":"region"}]},
		{"id":"weight","path":[{"field":"weight"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"region"}]}`, Options{}, nil)
	requireRows(t, res, "label", "North", "South", "(none)", "Total")
	requireRows(t, res, "weight", int64(190), int64(300), int64(4), int64(494))
}

func TestAmountsInDifferentCurrenciesAreTotalledApart(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	res := z.run(t, "ann", feedingsBySpecies, Options{}, nil)
	requireRows(t, res, "label", "zebra", "Total", "lion", "zebra", "Total")
	requireRows(t, res, "cost@currency", "EUR", "EUR", "USD", "USD", "USD")
	requireRows(t, res, "cost", int64(400), int64(400), int64(3500), int64(1000), int64(4500))
	if !res.Columns[2].MixedCurrency || res.Columns[2].CurrencyCell != "cost@currency" {
		t.Fatalf("cost column = %+v", res.Columns[2])
	}
	if len(res.Notices) != 1 || res.Notices[0].Code != "mixedCurrency" || !reflect.DeepEqual(res.Notices[0].Columns, []string{"cost"}) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// An amount read through a formula keeps its currency: its totals group by
// it, as a path column's do, and its cells say which currency they hold.
func TestAFormulaOverAmountsTotalsEachCurrencyApart(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"t","formula":{"op":"sum","args":[{"op":"add","args":[{"path":[{"field":"cost"}]},{"value":0}]}]}}],
		"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "kind", "detail", "detail", "detail", "detail", "detail", "total", "total")
	requireRows(t, res, "t", int64(1000), int64(2500), int64(700), int64(300), int64(400), int64(400), int64(4500))
	requireRows(t, res, "t@currency", "USD", "USD", "USD", "USD", "EUR", "EUR", "USD")
	if c := res.Columns[1]; c.CurrencyCell != "t@currency" || !c.MixedCurrency {
		t.Fatalf("t column = %+v", c)
	}
}

// Two amounts of different currency sources never combine into one; a
// fixed currency combines with itself and names its code.
func TestAFormulaRefusesToCombineCurrencies(t *testing.T) {
	z := newZoo(t)
	mixed := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"cost","path":[{"field":"cost"}]},
		{"id":"x","formula":{"op":"add","args":[{"col":"cost"},{"path":[{"link":"animal"},{"field":"price"}]}]}}]}`
	_, err := z.try(t, "ann", mixed, Options{}, nil)
	requireSpecError(t, err, 400, "columns.1.formula", "amounts in different currencies cannot be combined")
	requireSpecError(t, Validate(z.app, loadSpec(t, mixed)), 400, "columns.1.formula", "amounts in different currencies cannot be combined")
	for name, op := range map[string]string{
		"min":      `{"op":"min","args":[{"col":"cost"},{"path":[{"link":"animal"},{"field":"price"}]}]}`,
		"if":       `{"op":"if","args":[{"value":true},{"col":"cost"},{"path":[{"link":"animal"},{"field":"price"}]}]}`,
		"coalesce": `{"op":"coalesce","args":[{"col":"cost"},{"path":[{"link":"animal"},{"field":"price"}]}]}`,
		"case":     `{"op":"case","args":[{"value":true},{"col":"cost"},{"path":[{"link":"animal"},{"field":"price"}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{"version":1,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}]},{"id":"x","formula":` + op + `}]}`
			requireSpecError(t, Validate(z.app, loadSpec(t, raw)), 400, "columns.1.formula", "amounts in different currencies")
		})
	}
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"same","formula":{"op":"sub","args":[{"path":[{"field":"cost"}]},{"path":[{"field":"cost"}]}]}},
		{"id":"price","formula":{"op":"add","args":[{"path":[{"link":"animal"},{"field":"price"}]},{"path":[{"link":"animal"},{"field":"price"}]}]}}]}`, Options{}, nil)
	if c := res.Columns[0]; c.CurrencyCell != "same@currency" {
		t.Fatalf("same column = %+v", c)
	}
	if c := res.Columns[1]; c.Currency != "USD" || c.CurrencyCell != "" {
		t.Fatalf("price column = %+v", c)
	}
}

// A nested amount carries its currency: the column names the cell, and each
// child row holds it.
func TestANestedAmountCarriesItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"cost","path":[{"link":"feedings","mode":"nest"},{"field":"cost"}]}],
		"sort":[{"col":"name"}]}`, Options{}, nil)
	if c := res.Columns[1]; c.CurrencyCell != "cost@currency" {
		t.Fatalf("cost column = %+v", c)
	}
	requireRows(t, res, "label", "Kit", "Leo", "1000", "2500", "Zed", "300", "700", "Total")
	requireRows(t, res, "cost@currency", nil, nil, "USD", "USD", nil, "USD", "USD", nil)
}

func TestGroupRowsStopAtTheRowCap(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 2 })
	res := z.run(t, "ann", feedingsBySpecies, Options{}, nil)
	if len(res.Notices) != 1 || res.Notices[0].Code != "rowCap" || res.Notices[0].Count != 2 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

func TestRunningValuesAndPercents(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}},
		{"id":"ofParent","formula":{"op":"percentOfParent","args":[{"col":"cost"}],"scale":1}},
		{"id":"ofTotal","formula":{"op":"percentOfTotal","args":[{"col":"cost"}],"scale":1}}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "note", nil, "f1", "f2", nil, "f3", "f4", nil)
	// Running sums restart in each group; group rows run across groups.
	requireRows(t, res, "running", int64(3500), int64(1000), int64(3500), int64(4500), int64(700), int64(1000), int64(4500))
	requireRows(t, res, "ofParent", 77.8, 28.6, 71.4, 22.2, 70.0, 30.0, 100.0)
	requireRows(t, res, "ofTotal", 77.8, 22.2, 55.6, 22.2, 15.6, 6.7, 100.0)
}

func TestARunningValueIsNotASortKey(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}}],
		"sort":[{"col":"running"}]}`, Options{}, nil)
	requireSpecError(t, err, 400, "sort.0.col", "not a sort key")
}
