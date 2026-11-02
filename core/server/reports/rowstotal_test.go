package reports

import (
	"fmt"
	"strings"
	"testing"
)

const speciesRows = `{"version":2,"source":"zoo.feedings","params":[{"id":"min","type":"number"}],"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"},
	{"id":"all","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species","having":{"op":"gte","args":[
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"min"}]}}],
	"sort":[{"col":"species"}]}`

func TestRowsTotalsAddOnlyShownRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", speciesRows, Options{}, nil)
	requireRows(t, res, "shown", int64(3500), int64(1000), int64(4500))
	res = z.run(t, "ann", speciesRows, Options{}, map[string]string{"min": "1500"})
	requireRows(t, res, "label", "lion", "Total")
	requireRows(t, res, "shown", int64(3500), int64(3500))
	requireRows(t, res, "all", int64(3500), int64(4500))
}

// Each currency's total adds that currency's shown rows.
func TestRowsTotalsAddOnlyShownRowsPerCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 100, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
	res := z.run(t, "ann", speciesRows, Options{}, map[string]string{"min": "1500"})
	requireRows(t, res, "label", "Total", "lion", "Total")
	requireRows(t, res, "shown", int64(0), int64(3500), int64(3500))
	requireRows(t, res, "all", int64(100), int64(3500), int64(4500))
}

// A level above the innermost adds its shown child groups.
func TestRowsTotalsAddShownChildGroups(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"},
		{"id":"all","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"name","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}}],
		"sort":[{"col":"species"}]}`, Options{Expand: [][]any{{"lion", "USD"}, {"zebra", "USD"}}}, nil)
	requireRows(t, res, "label", "lion", "Leo", "zebra", "Total")
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(0), int64(3500))
	requireRows(t, res, "all", int64(3500), int64(3500), int64(1000), int64(4500))
}

// With no groups the total adds the detail cells.
func TestRowsTotalWithoutGroupsAddsTheDetailCells(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"big","formula":{"op":"if","args":[{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":1000}]},{"path":[{"field":"cost"}]},{"value":0}]},"total":"rows"}],
		"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "big", int64(0), int64(2500), int64(0), int64(0), int64(2500))
}

func TestRowsTotalsOnPivotCells(t *testing.T) {
	z := newZoo(t)
	pivoted := speciesRows[:len(speciesRows)-1] + `,"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"shown"}}`
	res := z.run(t, "ann", pivoted, Options{}, map[string]string{"min": "1500"})
	requireRows(t, res, "shown.p0", int64(1000), int64(1000))
	requireRows(t, res, "shown.p1", int64(2500), int64(2500))
	requireRows(t, res, "shown.p2", int64(0), int64(0))
}

func TestALayoutSectionAddsItsShownRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"name","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}}],
		"layout":[{"section":"l","title":"Lions","key":"lion"},{"section":"z","title":"Zebras","key":"zebra"},
			{"row":"d","label":"Diff","formula":{"op":"sub","args":[{"subtotal":"l"},{"subtotal":"z"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Diff")
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(0), int64(0), int64(3500))
}

// A number path column with no aggregate adds its rows too (F17): the
// innermost group sums its base rows, the levels above their shown groups.
func TestRowsTotalOnANumberWithoutAnAggregate(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"total":"rows"}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "grams", int64(1300), int64(400), int64(1700))
}

// A number with no aggregate that adds its rows is a sort key on groups too.
func TestGroupsSortByANumberThatAddsItsRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"total":"rows"}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"grams"}]}`, Options{}, nil)
	requireRows(t, res, "label", "zebra", "lion", "Total")
	requireRows(t, res, "grams", int64(400), int64(1300), int64(1700))
}

// A filled-in record with no rows adds nothing.
func TestAFillRowAddsNothing(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"total":"rows"},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"animal","fill":{"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Lia"}]}}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"name"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Lia", "Zed", "Total")
	requireRows(t, res, "grams", int64(1300), int64(0), int64(400), int64(1700))
	requireRows(t, res, "shown", int64(3500), int64(0), int64(1000), int64(4500))
}

// A having may read a "rows" column: a group's sum adds the child groups that
// pass their own having, so the zebra group (its one animal hidden) adds 0
// and hides too; no row under it reaches the result.
func TestAHavingReadsARowsColumn(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species","having":{"op":"gt","args":[{"col":"shown"},{"value":0}]}},
			{"id":"h","col":"name","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}}],
		"sort":[{"col":"species"},{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "note", nil, nil, "f1", "f2", nil)
	requireRows(t, res, "kind", "group", "group", "detail", "detail", "total")
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(1000), int64(2500), int64(3500))
	// The having is on the outer group only; its sum still adds the level
	// below, also in the statement of the detail rows.
	res = z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species","having":{"op":"gt","args":[{"col":"shown"},{"value":1500}]}},{"id":"h","col":"name"}],
		"sort":[{"col":"species"},{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "note", nil, nil, "f1", "f2", nil)
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(1000), int64(2500), int64(3500))
}

// Sorted by a "rows" column, groups sort by the sum they show: zebra (1000
// shown) before lion (its one animal, 3500, hidden).
func TestARowsColumnSortsGroupsByWhatTheyShow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"name","having":{"op":"lt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}}],
		"sort":[{"col":"shown","dir":"desc"}]}`, Options{}, nil)
	requireRows(t, res, "label", "zebra", "lion", "Total")
	requireRows(t, res, "shown", int64(1000), int64(0), int64(1000))
}

// A formula that reads a row's value adds its detail cells on the innermost
// group; a column that reads a "rows" column reads its sum on every level.
func TestARowsFormulaAddsItsDetailCellsUnderGroups(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"big","formula":{"op":"if","args":[{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":1000}]},{"path":[{"field":"cost"}]},{"value":0}]},"total":"rows"},
		{"id":"twice","formula":{"op":"mul","args":[{"col":"big"},{"value":2}]}}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "big", int64(2500), int64(0), int64(2500))
	requireRows(t, res, "twice", int64(5000), int64(0), int64(5000))
}

// A record's amount carries its own currency: its "rows" total adds the
// shown rows of each currency. The animal's group adds its two detail rows,
// each of which shows the animal's fee.
func TestARowsTotalOfACarriedAmountAddsEachCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"fee","formula":{"record":"g","path":[{"field":"fee"}]},"total":"rows"}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"fee"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Total", "Zed", "Total")
	requireRows(t, res, "fee", int64(1400), int64(1400), int64(600), int64(600))
	requireRows(t, res, "fee@currency", "EUR", "EUR", "USD", "USD")
}

// A rows total adds the shown cells whatever the column's aggregate: the
// grams average adds its detail cells, and a constant 1 counts its rows.
func TestRowsTotalsAddTheDetailCellsOfEveryColumnKind(t *testing.T) {
	z := newZoo(t)
	const cols = `"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"avg","path":[{"field":"grams"}],"aggregate":"avg","scale":0,"total":"rows"},
		{"id":"one","formula":{"value":1},"total":"rows"}]`
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings",`+cols+`,"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "avg", int64(500), int64(800), int64(300), int64(100), int64(1700))
	requireRows(t, res, "one", int64(1), int64(1), int64(1), int64(1), int64(4))
	res = z.run(t, "ann", `{"version":2,"source":"zoo.feedings",`+cols+`,"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "label", "lion", "zebra", "Total")
	requireRows(t, res, "avg", int64(1300), int64(400), int64(1700))
	requireRows(t, res, "one", int64(2), int64(2), int64(4))
}

// A middle group hidden by its own having adds nothing to its parent, whose
// having reads that sum and hides it in turn (South, its zebra 1000).
func TestARowsTotalAddsThroughThreeLevels(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"site","path":[{"link":"animal"},{"link":"site"},{"field":"name"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"site","having":{"op":"gt","args":[{"col":"shown"},{"value":0}]}},
			{"id":"h","col":"species","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}},
			{"id":"i","col":"name"}],
		"sort":[{"col":"site"},{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "group", "group", "detail", "detail", "total")
	requireRows(t, res, "note", nil, nil, nil, "f1", "f2", nil)
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(3500), int64(1000), int64(2500), int64(3500))
}

// More pivot values than SQLite joins in one select: every sum of a level is
// in one CTE, joined once.
func TestRowsTotalsOnMorePivotCellsThanAJoinHolds(t *testing.T) {
	z := newZoo(t)
	for i := range 70 {
		z.feeding(t, fmt.Sprintf("p%02d", i), z.recs["leo"], 1, "2026-04-01", 1)
	}
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"name"}],"sort":[{"col":"species"}],
		"pivot":{"path":[{"field":"note"}],"cell":"shown","maxColumns":100}}`, Options{}, nil)
	requireRows(t, res, "label", "lion", "zebra", "Total")
	requireRows(t, res, "shown.p0", int64(1000), int64(0), int64(1000))
	requireRows(t, res, "shown.p73", int64(1), int64(0), int64(1))
}

// Many to-many totals and "rows" columns: the level's joins are split so no
// select joins more tables than SQLite allows.
func TestManyToManyTotalsAndRowsColumns(t *testing.T) {
	z := newZoo(t)
	cols := []string{`{"id":"region","path":[{"field":"region"}]}`, `{"id":"name","path":[{"field":"name"}]}`}
	for i := range 70 {
		cols = append(cols, fmt.Sprintf(`{"id":"w%d","path":[{"link":"animals","mode":"aggregate"},{"field":"weight"}],"aggregate":"sum","total":"rows"}`, i))
	}
	res := z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[`+strings.Join(cols, ",")+`],
		"groups":[{"id":"g","col":"region"},{"id":"h","col":"name"}],"sort":[{"col":"region"}]}`, Options{}, nil)
	requireRows(t, res, "label", "North", "South", "Total")
	requireRows(t, res, "w0", int64(190), int64(300), int64(490))
	requireRows(t, res, "w69", int64(190), int64(300), int64(490))
}

// More recompute to-many totals than one select may join, beside a "rows"
// column: the level's joins are split into chunks. The having hides the North
// site (190), so the rows total adds South only while the recompute total
// adds both.
func TestManyToManyTotalsJoinInChunks(t *testing.T) {
	z := newZoo(t)
	cols := []string{`{"id":"region","path":[{"field":"region"}]}`, `{"id":"name","path":[{"field":"name"}]}`,
		`{"id":"shown","path":[{"link":"animals","mode":"aggregate"},{"field":"weight"}],"aggregate":"sum","total":"rows"}`}
	for i := range 65 {
		cols = append(cols, fmt.Sprintf(`{"id":"w%d","path":[{"link":"animals","mode":"aggregate"},{"field":"weight"}],"aggregate":"sum"}`, i))
	}
	res := z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[`+strings.Join(cols, ",")+`],
		"groups":[{"id":"g","col":"region"},{"id":"h","col":"name","having":{"op":"gt","args":[{"col":"w0"},{"value":200}]}}],
		"sort":[{"col":"region"}]}`, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "group", "group", "detail", "total")
	requireRows(t, res, "name", nil, nil, "South", "South", nil)
	requireRows(t, res, "shown", int64(0), int64(300), int64(300), int64(300), int64(300))
	requireRows(t, res, "w0", int64(190), int64(300), int64(300), int64(300), int64(490))
	requireRows(t, res, "w64", int64(190), int64(300), int64(300), int64(300), int64(490))
}

func TestRowsTotalIsRefusedWhereItMeansNothing(t *testing.T) {
	app := withZooRegistry(t)
	for _, c := range []struct{ spec, at, want string }{
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"c","path":[{"field":"cost"}],"aggregate":"sum","total":"some"}]}`,
			"columns.0.total", "total is recompute or rows"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}],"total":"rows"}]}`,
			"columns.0.total", "only a number or an amount column adds the rows shown"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"},
			{"id":"p","formula":{"op":"percentOfTotal","scale":2,"args":[{"col":"c"}]},"total":"rows"}],"groups":[{"id":"g","col":"n"}]}`,
			"columns.2.total", "is not added up"},
		{`{"version":2,"source":"zoo.sites","columns":[{"id":"n","path":[{"field":"name"}]},
			{"id":"w","path":[{"link":"animals","mode":"nest"},{"field":"weight"}],"total":"rows"}]}`,
			"columns.1.total", "child rows are not totalled"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}
