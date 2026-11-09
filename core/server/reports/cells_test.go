package reports

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// cur and pri are the cost on or before the as-of day and a year earlier;
// chg and pct are derived from them.
const zooCells = `{"version":4,"source":"zoo.feedings",
	"params":[{"id":"asOf","type":"asOf","label":"As of"}],
	"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cur","formula":{"op":"coalesce","args":[{"op":"sum","args":[{"op":"if","args":[{"op":"lte","args":[{"path":[{"field":"fed"}]},{"param":"asOf"}]},{"path":[{"field":"cost"}]},{"value":null}]}]},{"value":0}]}},
	{"id":"pri","formula":{"op":"coalesce","args":[{"op":"sum","args":[{"op":"if","args":[{"op":"lte","args":[{"path":[{"field":"fed"}]},` + yearBefore + `]},{"path":[{"field":"cost"}]},{"value":null}]}]},{"value":0}]}},
	{"id":"chg","formula":{"op":"sub","args":[{"col":"cur"},{"col":"pri"}]}},
	{"id":"pct","format":"percent","formula":{"op":"if","args":[{"op":"eq","args":[{"col":"pri"},{"value":0}]},{"value":null},
		{"op":"mul","args":[{"op":"div","scale":4,"args":[{"col":"chg"},{"op":"max","args":[{"col":"pri"},{"op":"sub","args":[{"value":0},{"col":"pri"}]}]}]},{"value":100}]}]}},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	"layout":[
		{"section":"cats","title":"Lions","key":"lion"},
		{"section":"stripes","title":"Zebras","key":"zebra"}%s]}`

const yearBefore = `{"op":"addMonths","args":[{"param":"asOf"},{"value":-12}]}`

// sumCostTo is a lookup of the cost of every feeding the reader lists on or
// before day (a node), with an extra condition.
func sumCostTo(day, extra string) string {
	where := `{"op":"lte","args":[{"path":[{"field":"fed"}]},` + day + `]}`
	if extra != "" {
		where = `{"op":"and","args":[` + where + `,` + extra + `]}`
	}
	return `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","where":` + where + `,"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]}`
}

var asOf2027 = map[string]string{"asOf": `"2027-02-15"`}

func requireNear(t *testing.T, got any, want float64) {
	t.Helper()
	f, ok := got.(float64)
	if !ok {
		if i, isInt := got.(int64); isInt {
			f, ok = float64(i), true
		}
	}
	if !ok || math.Abs(f-want) > 0.005 {
		t.Fatalf("got %#v, want %.2f", got, want)
	}
}

func TestCellsByColumnAtTwoParamDays(t *testing.T) {
	z := newZoo(t)
	row := `,{"row":"fed","label":"Fed","formula":{"value":7},"cells":{"cur":` + sumCostTo(`{"param":"asOf"}`, "") + `,"pri":` + sumCostTo(yearBefore, "") + `}}`
	res := z.run(t, "ann", fmt.Sprintf(zooCells, row), Options{}, asOf2027)
	fed := res.Rows[len(res.Rows)-1]
	// Ann's feedings: 4500 by 2027-02-15; 4200 by 2026-02-15 (f4 is in March).
	if fed.Cells["cur"] != int64(4500) || fed.Cells["pri"] != int64(4200) {
		t.Fatalf("fed = %+v", fed.Cells)
	}
	// A number column cells does not name takes formula.
	if fed.Cells["grams"] != int64(7) {
		t.Fatalf("grams = %#v", fed.Cells["grams"])
	}
	// Derived from the row's own cells: 4500 − 4200, and 300 / 4200.
	if fed.Cells["chg"] != int64(300) {
		t.Fatalf("chg = %#v", fed.Cells["chg"])
	}
	requireNear(t, fed.Cells["pct"], 7.14)
}

func TestDerivedColumnsOnASectionTotalWithAWithinRow(t *testing.T) {
	z := newZoo(t)
	f4 := `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f4"}]}`
	row := `,{"row":"adj","label":"Adjust","within":"stripes","cells":{"cur":` + sumCostTo(`{"param":"asOf"}`, f4) + `,"pri":` + sumCostTo(yearBefore, f4) + `}}`
	res := z.run(t, "ann", fmt.Sprintf(zooCells, row), Options{ExpandAll: true}, asOf2027)
	var adj, total *Row
	for i := range res.Rows {
		switch res.Rows[i].Label {
		case "Adjust":
			adj = &res.Rows[i]
		case "Total Zebras":
			total = &res.Rows[i]
		}
	}
	// Adjust: f4's 300 now, 0 a year earlier; Change from its own cells,
	// percent blank on a zero prior.
	if adj.Cells["cur"] != int64(300) || adj.Cells["pri"] != int64(0) || adj.Cells["chg"] != int64(300) || adj.Cells["pct"] != nil {
		t.Fatalf("adjust = %+v", adj.Cells)
	}
	// Zebras 1000 / 700 plus Adjust 300 / 0; Change and percent from the
	// total's own cells: 1300 − 700 = 600, 600 / 700.
	if total.Cells["cur"] != int64(1300) || total.Cells["pri"] != int64(700) || total.Cells["chg"] != int64(600) {
		t.Fatalf("total = %+v", total.Cells)
	}
	requireNear(t, total.Cells["pct"], 85.71)
}

// A top-level computed row with no cells: its percent comes from its own
// Change and Prior year (7.14), no longer from the sum of the sections'
// percents (0 + 42.86).
func TestATopLevelComputedRowsPercentIsFromItsOwnCells(t *testing.T) {
	z := newZoo(t)
	row := `,{"row":"both","label":"Both","formula":{"op":"add","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}`
	res := z.run(t, "ann", fmt.Sprintf(zooCells, row), Options{}, asOf2027)
	both := res.Rows[len(res.Rows)-1]
	if both.Cells["cur"] != int64(4500) || both.Cells["pri"] != int64(4200) || both.Cells["chg"] != int64(300) {
		t.Fatalf("both = %+v", both.Cells)
	}
	requireNear(t, both.Cells["pct"], 7.14)
}

func TestCellsRefusals(t *testing.T) {
	z := newZoo(t)
	cases := map[string]struct {
		row, at, want string
		status        int
	}{
		"neither":        {`,{"row":"r","label":"R"}`, "layout.2", "a label and a formula", 400},
		"first column":   {`,{"row":"r","label":"R","cells":{"species":{"value":1}}}`, "layout.2.cells.species", "the row's label fills the first column", 400},
		"text column":    {`,{"row":"r","label":"R","cells":{"note":{"value":1}}}`, "layout.2.cells.note", "number and amount columns", 400},
		"unknown column": {`,{"row":"r","label":"R","cells":{"nope":{"value":1}}}`, "layout.2.cells.nope", "nope", 422},
		// A cells formula is checked in its column, at its own path.
		"path read":    {`,{"row":"r","label":"R","cells":{"grams":{"path":[{"field":"grams"}]}}}`, "layout.2.formula.cells.grams.path", "a computed row reads sections' subtotals", 400},
		"column read":  {`,{"row":"r","label":"R","cells":{"grams":{"col":"cur"}}}`, "layout.2.formula.cells.grams", "would read itself", 400},
		"own section":  {`,{"row":"r","label":"R","within":"stripes","cells":{"grams":{"subtotal":"stripes"}}}`, "layout.2.formula.cells.grams", "it would read itself", 400},
		"text formula": {`,{"row":"r","label":"R","cells":{"grams":{"value":"x"}}}`, "layout.2.formula.cells.grams", "number and amount columns", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(zooCells, c.row)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			requireSpecError(t, Validate(z.app, spec), c.status, c.at, c.want)
		})
	}
}

// rowLabeled is the result row with label, or fails.
func rowLabeled(t *testing.T, res *Result, label string) Row {
	t.Helper()
	for _, r := range res.Rows {
		if r.Label == label {
			return r
		}
	}
	t.Fatalf("no row %q", label)
	return Row{}
}

func addOf(a, b string) string { return `{"op":"add","args":[` + a + `,` + b + `]}` }

// A statement-sized layout (a balance sheet's shape): within rows in both
// sections whose cells add two lookups, the section totals, a total of both
// sections, and a later total that reads it. The derived Change and percent
// columns must not count toward the rows that read these rows.
func TestCellsOfAStatementSizedLayoutRun(t *testing.T) {
	z := newZoo(t)
	f4 := `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f4"}]}`
	curF4, priF4 := sumCostTo(`{"param":"asOf"}`, f4), sumCostTo(yearBefore, f4)
	curAll, priAll := sumCostTo(`{"param":"asOf"}`, ""), sumCostTo(yearBefore, "")
	rows := `,{"row":"ca","label":"Cat adjust","within":"cats","cells":{"cur":` + addOf(curF4, priAll) + `,"pri":` + addOf(priF4, priF4) + `}}` +
		`,{"row":"re","label":"Retained","within":"stripes","cells":{"cur":` + addOf(curF4, curF4) + `,"pri":` + addOf(priF4, priF4) + `}}` +
		`,{"row":"ni","label":"Net","within":"stripes","cells":{"cur":{"op":"sub","args":[` + curAll + `,` + priAll + `]},"pri":` + addOf(priF4, priF4) + `}}` +
		`,{"row":"both","label":"Both","formula":` + addOf(`{"subtotal":"cats"}`, `{"subtotal":"stripes"}`) + `}` +
		`,{"row":"grand","label":"Grand","formula":` + addOf(`{"subtotal":"both"}`, `{"subtotal":"stripes"}`) + `}`
	res := z.run(t, "ann", fmt.Sprintf(zooCells, rows), Options{ExpandAll: true}, asOf2027)
	want := map[string][3]int64{
		// Lions 3500 / 3500 plus Cat adjust 300 + 4200 / 0.
		"Total Lions": {8000, 3500, 4500},
		// Zebras 1000 / 700 plus Retained 600 / 0 plus Net 300 / 0.
		"Total Zebras": {1900, 700, 1200},
		"Both":         {9900, 4200, 5700},
		"Grand":        {11800, 4900, 6900},
	}
	for label, w := range want {
		r := rowLabeled(t, res, label)
		if r.Cells["cur"] != w[0] || r.Cells["pri"] != w[1] || r.Cells["chg"] != w[2] {
			t.Fatalf("%s = %+v, want %v", label, r.Cells, w)
		}
	}
	requireNear(t, rowLabeled(t, res, "Total Zebras").Cells["pct"], 171.43)
	requireNear(t, rowLabeled(t, res, "Grand").Cells["pct"], 140.82)
}

// treeOf is a formula of 2^(depth+1)-1 nodes: adds, depth deep, of ones.
func treeOf(depth int) string {
	if depth == 0 {
		return `{"value":1}`
	}
	return addOf(treeOf(depth-1), treeOf(depth-1))
}

// The caps still count a derived cell through the derived columns it reads:
// pct reads pri three times and chg once (cur and pri), so its cell is about
// five of the row's cells. Cells of 511 nodes fit, but pct does not.
func TestCellsCapCountsNestedDerivedReads(t *testing.T) {
	z := newZoo(t)
	row := func(depth int) string {
		return `,{"row":"big","label":"Big","cells":{"cur":` + treeOf(depth) + `,"pri":` + treeOf(depth) + `}}`
	}
	res := z.run(t, "ann", fmt.Sprintf(zooCells, row(6)), Options{}, asOf2027)
	if big := res.Rows[len(res.Rows)-1]; big.Cells["cur"] != int64(64) || big.Cells["chg"] != int64(0) {
		t.Fatalf("big = %+v", big.Cells)
	}
	_, err := z.try(t, "ann", fmt.Sprintf(zooCells, row(8)), Options{}, asOf2027)
	requireSpecError(t, err, 400, "layout.2", "too large")
}

// A chain of rows whose cells each read the row before: every row's cells are
// checked once, by its own leg, so the compile grows with the rows, not
// exponentially with the reads (two per row here).
func TestCellsChainOfRowsChecksEachRowOnce(t *testing.T) {
	z := newZoo(t)
	rows := []string{`{"row":"r0","label":"R0","cells":{"cur":` + sumCostTo(`{"param":"asOf"}`, "") + `,"pri":` + sumCostTo(yearBefore, "") + `}}`}
	for i := 1; i < 18; i++ {
		prev := fmt.Sprintf(`{"subtotal":"r%d"}`, i-1)
		rows = append(rows, fmt.Sprintf(`{"row":"r%d","label":"R%d","cells":{"cur":%s,"pri":%s}}`, i, i, prev, prev))
	}
	raw := fmt.Sprintf(zooCells, ","+strings.Join(rows, ","))
	requireCompileWithinCaps(t, z, raw)
	res := z.run(t, "ann", raw, Options{}, asOf2027)
	last := rowLabeled(t, res, "R17")
	if last.Cells["cur"] != int64(4500) || last.Cells["pri"] != int64(4200) || last.Cells["chg"] != int64(300) {
		t.Fatalf("R17 = %+v", last.Cells)
	}
}

// requireCompileWithinCaps validates raw as Validate does, and fails if it is
// refused or if the layout compiled more formula nodes than twice what its
// caps counted: the caps count every cell compiled in a column, and the
// check pass compiles each formula once more on no column, at no more than
// its counted size. A compile the caps do not count (the work that made
// Validate slow) breaks the bound by orders of magnitude, on any machine.
func requireCompileWithinCaps(t *testing.T, z *zoo, raw string) {
	t.Helper()
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, serr := bind(z.app, spec)
	if serr != nil {
		t.Fatalf("bind: %v", serr)
	}
	c := newCompiler(p, nil, nil, nil)
	if err := c.compileAll(); err != nil {
		t.Fatalf("compile: %v", err)
	}
	if c.layoutCounted == 0 || c.layoutNodes > 2*c.layoutCounted {
		t.Fatalf("compiled %d formula nodes; the caps counted %d", c.layoutNodes, c.layoutCounted)
	}
}

// A row with a large formula and a one-node cell in grams, read 998 times by
// the grams cell of each of ten readers. The caps count each reader's cell
// with the row at its size in grams (about 2,000 nodes, 20,000 in all), and a
// cells formula is compiled only in its column, so the compile stays near
// that count. A check of the cells formulas with the row inlined by its
// formula compiled about 5 M nodes, uncounted.
func TestCellsManyReadersOfARowWithASmallCell(t *testing.T) {
	z := newZoo(t)
	reads := strings.TrimSuffix(strings.Repeat(`{"subtotal":"big"},`, 998), ",")
	rows := []string{`{"row":"big","label":"Big","formula":` + treeOf(8) + `,"cells":{"grams":{"value":1}}}`}
	for i := range 10 {
		rows = append(rows, fmt.Sprintf(`{"row":"r%d","label":"R%d","cells":{"grams":{"op":"add","args":[%s]}}}`, i, i, reads))
	}
	raw := `{"version":4,"source":"zoo.feedings",
		"columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"cats","title":"Lions","key":"lion"},` + strings.Join(rows, ",") + `]}`
	requireCompileWithinCaps(t, z, raw)
	res := z.run(t, "ann", raw, Options{}, nil)
	if big := rowLabeled(t, res, "Big"); big.Cells["grams"] != int64(1) {
		t.Fatalf("Big = %+v", big.Cells)
	}
	if last := rowLabeled(t, res, "R9"); last.Cells["grams"] != int64(998) {
		t.Fatalf("R9 = %+v", last.Cells)
	}
}
