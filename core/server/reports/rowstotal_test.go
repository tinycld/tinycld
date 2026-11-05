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
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"big","formula":{"op":"if","args":[{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":1000}]},{"path":[{"field":"cost"}]},{"value":0}]},"total":"rows"}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "big", int64(0), int64(2500), int64(0), int64(0), int64(2500))
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

// R8/F17: a number path column with no aggregate adds its rows too: the
// innermost group sums its base rows' values (lion 500 + 800, zebra 300 +
// 100), the levels above their shown groups. A column with an aggregate (max)
// beside it computes over the innermost group's base rows instead (E2: lion
// 800, zebra 300), and the Total adds those (1100). Under two levels with a
// having that hides Zed (400 g), zebra adds no shown group (0), not its base
// rows.
func TestRowsTotalOnANumberWithoutAnAggregate(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"total":"rows"},
		{"id":"most","path":[{"field":"grams"}],"aggregate":"max","total":"rows"}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "grams", int64(1300), int64(400), int64(1700))
	requireRows(t, res, "most", int64(800), int64(300), int64(1100))

	res = z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"grams","path":[{"field":"grams"}],"total":"rows"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"name","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"grams"}]}]},{"value":500}]}}],
		"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "label", "lion", "zebra", "Total")
	requireRows(t, res, "grams", int64(1300), int64(0), int64(1300))
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

// R6: a fill record has no base rows, so on the innermost group each "rows"
// column is computed over no rows (E2): a sum, and a number with no aggregate
// (F17), are 0; a max is blank. The fill row adds those blank or zero cells
// to the Total, which is that of the other rows. The max over Leo's rows is
// 800, over Zed's 300.
func TestAFillRowAddsItsBlankOrZeroCells(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"grams","path":[{"field":"grams"}],"total":"rows"},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"},
		{"id":"most","path":[{"field":"grams"}],"aggregate":"max","total":"rows"}],
		"groups":[{"id":"g","col":"animal","fill":{"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Lia"}]}}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"name"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Lia", "Zed", "Total")
	requireRows(t, res, "grams", int64(1300), int64(0), int64(400), int64(1700))
	requireRows(t, res, "shown", int64(3500), int64(0), int64(1000), int64(4500))
	requireRows(t, res, "most", int64(800), nil, int64(300), int64(1100))
}

// A having may read a "rows" column: a group's sum adds the child groups that
// pass their own having, so the zebra group (its one animal hidden) adds 0
// and hides too; no row under it reaches the result.
func TestAHavingReadsARowsColumn(t *testing.T) {
	z := newZoo(t)
	const both = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species","having":{"op":"gt","args":[{"col":"shown"},{"value":0}]}},
			{"id":"h","col":"name","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	res := z.run(t, "ann", both, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, both, res, "note", nil, nil, "f1", "f2", nil)
	requireRows(t, res, "kind", "group", "group", "detail", "detail", "total")
	requireOrderedRows(t, both, res, "shown", int64(3500), int64(3500), int64(1000), int64(2500), int64(3500))
	// The having is on the outer group only; its sum still adds the level
	// below, also in the statement of the detail rows.
	const outer = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species","having":{"op":"gt","args":[{"col":"shown"},{"value":1500}]}},{"id":"h","col":"name"}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	res = z.run(t, "ann", outer, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, outer, res, "note", nil, nil, "f1", "f2", nil)
	requireOrderedRows(t, outer, res, "shown", int64(3500), int64(3500), int64(1000), int64(2500), int64(3500))
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

// E2/R8 for each column kind. With no groups the Total adds the detail cells:
// the grams average of one row is its grams (1700 in all). Grouped, the
// innermost group computes the average over its base rows (lion (500 + 800) /
// 2 = 650, zebra (300 + 100) / 2 = 200), not the sum of its detail cells, and
// the Total adds the shown group cells (850). A constant 1 has no aggregate
// (F17): each group adds its base rows' values, so it counts them.
func TestRowsTotalsOfEveryColumnKind(t *testing.T) {
	z := newZoo(t)
	const cols = `"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"avg","path":[{"field":"grams"}],"aggregate":"avg","scale":0,"total":"rows"},
		{"id":"one","formula":{"value":1},"total":"rows"}]`
	const flat = `{"version":2,"source":"zoo.feedings",` + cols + `,"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", flat, Options{}, nil)
	requireOrderedRows(t, flat, res, "avg", int64(500), int64(800), int64(300), int64(100), int64(1700))
	requireRows(t, res, "one", int64(1), int64(1), int64(1), int64(1), int64(4))
	res = z.run(t, "ann", `{"version":2,"source":"zoo.feedings",`+cols+`,"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "label", "lion", "zebra", "Total")
	requireRows(t, res, "avg", int64(650), int64(200), int64(850))
	requireRows(t, res, "one", int64(2), int64(2), int64(4))
}

// A middle group hidden by its own having adds nothing to its parent, whose
// having reads that sum and hides it in turn (South, its zebra 1000).
func TestARowsTotalAddsThroughThreeLevels(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"site","path":[{"link":"animal"},{"link":"site"},{"field":"name"}]},
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"site","having":{"op":"gt","args":[{"col":"shown"},{"value":0}]}},
			{"id":"h","col":"species","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}},
			{"id":"i","col":"name"}],
		"sort":[{"col":"site"},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "group", "group", "detail", "detail", "total")
	requireOrderedRows(t, spec, res, "note", nil, nil, nil, "f1", "f2", nil)
	requireOrderedRows(t, spec, res, "shown", int64(3500), int64(3500), int64(3500), int64(1000), int64(2500), int64(3500))
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

// Seventy "rows" columns, each adding a to-many total of the same parent row:
// every group row and the total add the rows under them, column by column.
func TestManyRowsColumnsOverOneParentAddTheirRows(t *testing.T) {
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

// Sixty-five recompute to-many totals of one parent row beside a "rows"
// column, under a having. The having hides the North site (190), so the rows
// total adds South only while the recompute totals add both. The totals of
// one parent row share one CTE per level (statement_test.go); more distinct
// parents than one select may join are tested by
// TestSidesPastTheJoinLimitAreJoinedInChunks.
func TestManyRecomputeTotalsOverOneParentUnderAHaving(t *testing.T) {
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

// ledgerSides are a trial balance's two columns over the feedings' signed
// cost: a row shows its net on the side its sign puts it, the other side
// blank. The aggregate is inside each formula (no aggregate field), as a
// ledger writes it.
const ledgerSides = `
	{"id":"debit","formula":{"op":"if","args":[{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]},
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":null}]},"total":"rows"},
	{"id":"credit","formula":{"op":"if","args":[{"op":"lt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]},
		{"op":"mul","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":-1}]},{"value":null}]},"total":"rows"}`

// withLedger makes ann's feedings a balanced ledger of signed USD costs, each
// animal with rows on both sides:
//
//	Leo  f1 1000, f2 2500, c1 -500   net  3000
//	Lia  c3  200, c4 -500            net  -300
//	Zed  f3  700, f4  300, c2 -3700  net -2700
//
// The nets add to 0, so the debits (3000) equal the credits (300 + 2700).
// The sum of a group's rows' sides differs from its net (Leo's rows: 3500
// debit, 500 credit). Eli (from withEmptyAnimals) has no feeding.
func withLedger(t *testing.T, z *zoo) {
	t.Helper()
	withEmptyAnimals(t, z)
	z.rec(t, "kLia", "zoo_keepers", map[string]any{"user": z.id("ann"), "animal": z.id("lia")})
	z.feeding(t, "c1", z.recs["leo"], -500, "2026-02-20", 10)
	z.feeding(t, "c2", z.recs["zed"], -3700, "2026-03-10", 10)
	z.feeding(t, "c3", z.recs["lia"], 200, "2026-01-12", 10)
	z.feeding(t, "c4", z.recs["lia"], -500, "2026-02-12", 10)
}

const ledgerByAnimal = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"note","path":[{"field":"note"}]},` + ledgerSides + `],
	"groups":[{"id":"g","col":"animal"%s}],
	"sort":[{"formula":{"record":"g","path":[{"field":"name"}]}},{"col":"note"}]}`

// E2: the innermost group computes a "rows" column from its base rows, so a
// group shows its net on one side only; the Total adds the shown group cells
// (Debit total = Credit total for a balanced ledger). Detail rows keep their
// own sides.
func TestARowsTotalComputesTheInnermostGroupFromItsBaseRows(t *testing.T) {
	z := newZoo(t)
	withLedger(t, z)
	spec := fmt.Sprintf(ledgerByAnimal, "")
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Lia", "Zed", "Total")
	requireRows(t, res, "debit", int64(3000), nil, nil, int64(3000))
	requireRows(t, res, "credit", nil, int64(300), int64(2700), int64(3000))

	open := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, spec, open, "note", nil, "c1", "f1", "f2", nil, "c3", "c4", nil, "c2", "f3", "f4", nil)
	requireOrderedRows(t, spec, open, "debit", int64(3000), nil, int64(1000), int64(2500),
		nil, int64(200), nil, nil, nil, int64(700), int64(300), int64(3000))
	requireOrderedRows(t, spec, open, "credit", nil, int64(500), nil, nil,
		int64(300), nil, int64(500), int64(2700), int64(3700), nil, nil, int64(3000))
}

// R6: a fill row has no base rows, so each side's formula over no rows is
// blank (a sum of no rows is 0, which is neither side), and a blank adds
// nothing to the Total.
func TestARowsTotalFillRowIsBlankOnBothSides(t *testing.T) {
	z := newZoo(t)
	withLedger(t, z)
	res := z.run(t, "ann", fmt.Sprintf(ledgerByAnimal,
		`,"fill":{"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Eli"}]}}`), Options{}, nil)
	requireRows(t, res, "label", "Eli", "Leo", "Lia", "Zed", "Total")
	requireRows(t, res, "debit", nil, int64(3000), nil, nil, int64(3000))
	requireRows(t, res, "credit", nil, nil, int64(300), int64(2700), int64(3000))
}

// E2/E3: a group hidden by its having adds nothing to the Total, which adds
// the shown group cells only; a recompute column (net) still totals every
// base row. The having hides Leo, the one debit.
func TestARowsTotalAddsNothingForAGroupItsHavingHides(t *testing.T) {
	z := newZoo(t)
	withLedger(t, z)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"net","path":[{"field":"cost"}],"aggregate":"sum"},`+ledgerSides+`],
		"groups":[{"id":"g","col":"animal","having":{"op":"lt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"name"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Lia", "Zed", "Total")
	requireRows(t, res, "net", int64(-300), int64(-2700), int64(0))
	requireRows(t, res, "debit", nil, nil, int64(0))
	requireRows(t, res, "credit", int64(300), int64(2700), int64(3000))
}

// E2: each currency's groups compute from that currency's base rows, and each
// currency's Total adds that currency's shown rows. In EUR Leo has 500 and
// -200 (net 300 debit) and Zed -300 (300 credit).
func TestARowsTotalComputesEachCurrencyApart(t *testing.T) {
	z := newZoo(t)
	withLedger(t, z)
	for _, f := range []struct {
		key, animal string
		cost        int
	}{{"e1", "leo", 500}, {"e2", "leo", -200}, {"e3", "zed", -300}} {
		z.rec(t, f.key, "zoo_feedings", map[string]any{"animal": z.id(f.animal), "cost": f.cost, "currency": "EUR",
			"fed": "2026-04-01 00:00:00.000Z", "grams": 10, "note": f.key, "due": "2026-04-01"})
	}
	res := z.run(t, "ann", fmt.Sprintf(ledgerByAnimal, ""), Options{}, nil)
	requireRows(t, res, "label", "Leo", "Zed", "Total", "Leo", "Lia", "Zed", "Total")
	requireRows(t, res, "debit", int64(300), nil, int64(300), int64(3000), nil, nil, int64(3000))
	requireRows(t, res, "credit", nil, int64(300), int64(300), nil, int64(300), int64(2700), int64(3000))
	ids := []string{res.Rows[2].ID, res.Rows[6].ID}
	if ids[0] != `total:["EUR"]` || ids[1] != `total:["USD"]` {
		t.Fatalf("total ids = %v", ids)
	}
}

// E2: under two group levels the innermost (animal) computes from its base
// rows and the outer level (species) adds its shown child groups: lion shows
// Leo's 3000 debit and Lia's 300 credit, not its own net (2700 debit).
func TestARowsTotalOuterLevelAddsItsChildGroups(t *testing.T) {
	z := newZoo(t)
	withLedger(t, z)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}]},`+ledgerSides+`],
		"groups":[{"id":"s","col":"species"},{"id":"a","col":"animal"}],
		"sort":[{"col":"species"},{"formula":{"record":"a","path":[{"field":"name"}]}}]}`,
		Options{Expand: [][]any{{"lion", "USD"}, {"zebra", "USD"}}}, nil)
	requireRows(t, res, "label", "lion", "Leo", "Lia", "zebra", "Zed", "Total")
	requireRows(t, res, "debit", int64(3000), int64(3000), nil, int64(0), nil, int64(3000))
	requireRows(t, res, "credit", int64(300), nil, int64(300), int64(2700), int64(2700), int64(3000))
}

// E2: a "rows" pivot cell on the innermost group is the column's aggregate
// over the base rows with that pivot value; the Total adds the shown group
// cells. A second January feeding of Leo (200 g) makes lion's January max
// (500) differ from the sum of its cells (700); a month with no feeding is
// blank (max of no rows).
func TestARowsPivotCellComputesTheInnermostGroupFromItsBaseRows(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "p1", z.recs["leo"], 100, "2026-01-25", 200)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"most","path":[{"field":"grams"}],"aggregate":"max","total":"rows"}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}],
		"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"most"}}`, Options{}, nil)
	requireRows(t, res, "label", "lion", "zebra", "Total")
	requireRows(t, res, "most.p0", int64(500), int64(300), int64(800))
	requireRows(t, res, "most.p1", int64(800), nil, int64(800))
	requireRows(t, res, "most.p2", nil, int64(100), int64(100))
}

// E2: a layout section over the innermost group shows the column computed
// over its base rows (lions 1000 + 2500 - 500 + 200 - 500 = 2700 debit;
// zebras -2700); over two levels the section adds its shown rows (Leo's 3000
// debit, Lia's 300 credit). A computed row reads the sections as they show.
func TestALayoutSectionOfARowsColumnUnderTheRule(t *testing.T) {
	z := newZoo(t)
	withLedger(t, z)
	const layout = `"layout":[{"section":"l","title":"Lions","key":"lion"},{"section":"z","title":"Zebras","key":"zebra"},
		{"row":"b","label":"Both","formula":{"op":"add","args":[{"subtotal":"l"},{"subtotal":"z"}]}}]}`
	one := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},`+ledgerSides+`],
		"groups":[{"id":"g","col":"species"}],`+layout, Options{}, nil)
	requireRows(t, one, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Both")
	requireRows(t, one, "debit", int64(2700), int64(2700), nil, nil, int64(2700))
	requireRows(t, one, "credit", nil, nil, int64(2700), int64(2700), int64(2700))

	two := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"animal","path":[{"link":"animal"}]},`+ledgerSides+`],
		"groups":[{"id":"g","col":"species"},{"id":"a","col":"animal"}],`+layout, Options{}, nil)
	requireRows(t, two, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Both")
	requireRows(t, two, "debit", int64(3000), int64(3000), int64(0), int64(0), int64(3000))
	requireRows(t, two, "credit", int64(300), int64(300), int64(2700), int64(2700), int64(3000))
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

// The to-many totals of one parent row share one CTE per level, each parent's
// values counted once. Sites East and West have the same animals' weights, so
// a CTE that lost the parent would count one of them once instead of twice.
// Site Empty has no animals: its sum is 0 and its min, max and average are
// blank beside the others. Price is an amount in the animals' fixed currency.
// The second report fills in the sites with no animal (a fill row) on a level
// whose two feeding totals share one CTE: their sum is 0 and their max blank.
func TestToManyTotalsOfOneParentRowShareOneCTE(t *testing.T) {
	z := newZoo(t)
	ann := z.recs["ann"]
	east := z.rec(t, "east", "zoo_sites", map[string]any{"name": "East", "region": "north"})
	west := z.rec(t, "west", "zoo_sites", map[string]any{"name": "West", "region": "north"})
	z.rec(t, "empty", "zoo_sites", map[string]any{"name": "Empty", "region": "north"})
	for _, a := range []struct {
		key           string
		site          string
		weight, price int
	}{{"ada", east.Id, 10, 1000}, {"bea", east.Id, 30, 3000}, {"cid", west.Id, 10, 1000}, {"dee", west.Id, 30, 3000}} {
		z.rec(t, a.key, "zoo_animals", map[string]any{"name": a.key, "species": "owl", "site": a.site, "owner": ann.Id, "weight": a.weight, "price": a.price})
	}
	total := func(id, aggregate, field string) string {
		return fmt.Sprintf(`{"id":%q,"path":[{"link":"animals","mode":"aggregate"},{"field":%q}],"aggregate":%q}`, id, field, aggregate)
	}
	res := z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[{"id":"region","path":[{"field":"region"}]},`+
		strings.Join([]string{total("sum", "sum", "weight"), total("min", "min", "weight"), total("max", "max", "weight"),
			strings.Replace(total("avg", "avg", "weight"), `"avg"}`, `"avg","scale":0}`, 1), total("price", "sum", "price")}, ",")+`],
		"groups":[{"id":"g","col":"region"}],"sort":[{"col":"region"}]}`, Options{}, nil)
	requireRows(t, res, "label", "North", "South", "Total")
	// North: North (Leo 190), East (10, 30), West (10, 30), Empty (none).
	requireRows(t, res, "sum", int64(270), int64(300), int64(570))
	requireRows(t, res, "min", int64(10), int64(300), int64(10))
	requireRows(t, res, "max", int64(190), int64(300), int64(300))
	// The average of every animal under the row: 270 / 5, 300 / 1, 570 / 6.
	requireRows(t, res, "avg", int64(54), int64(300), int64(95))
	requireRows(t, res, "price", int64(508000), int64(120000), int64(628000))
	// A fixed currency is the column's, not a cell's.
	if c := res.Columns[5]; c.ID != "price" || c.Currency != "USD" || c.CurrencyCell != "" {
		t.Fatalf("price column = %+v", c)
	}

	// Ann keeps Leo and Zed, so she reads only their feedings.
	filled := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[{"id":"site","path":[{"link":"site"}]},
		{"id":"grams","path":[{"link":"feedings","mode":"aggregate"},{"field":"grams"}],"aggregate":"sum"},
		{"id":"most","path":[{"link":"feedings","mode":"aggregate"},{"field":"grams"}],"aggregate":"max"}],
		"groups":[{"id":"g","col":"site","fill":{}}],"sort":[{"col":"site"}]}`, Options{}, nil)
	requireRows(t, filled, "label", "East", "Empty", "North", "South", "West", "(none)", "Total")
	requireRows(t, filled, "grams", int64(0), int64(0), int64(1300), int64(400), int64(0), int64(0), int64(1700))
	requireRows(t, filled, "most", nil, nil, int64(800), int64(300), nil, nil, int64(800))
}
