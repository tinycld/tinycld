package reports

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// A statement in the zoo: sections pick first-level groups by key, computed
// rows read the sections' subtotals by name.
const zooStatement = `{"version":1,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species"}],
	%s
	"layout":[
		{"section":"cats","title":"Lions","key":"lion"},
		{"section":"stripes","title":"Zebras","key":"zebra"},
		{"section":"apes","title":"Monkeys","key":"monkey"},
		{"row":"gap","label":"Lions less zebras","formula":{"op":"sub","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}},
		{"row":"all","label":"All animals","formula":{"op":"add","args":[{"subtotal":"gap"},{"op":"mul","args":[{"subtotal":"stripes"},{"value":2}]},{"subtotal":"apes"}]}}]}`

func TestLayoutSectionsAndComputedRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooStatement, ""), Options{}, nil)
	requireRows(t, res, "kind", "group", "subtotal", "group", "subtotal", "group", "subtotal", "computed", "computed")
	requireRows(t, res, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Monkeys", "Total Monkeys", "Lions less zebras", "All animals")
	// Ann sees no monkey: its section is empty and counts as zero.
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(1000), int64(1000), nil, nil, int64(2500), int64(4500))
	requireRows(t, res, "grams", int64(1300), int64(1300), int64(400), int64(400), nil, nil, int64(900), int64(1700))
	// A text column has no figure on a computed row, nor on an empty
	// section.
	requireRows(t, res, "species", "lion", "lion", "zebra", "zebra", nil, nil, nil, nil)
	if res.Rows[6].ID != `c:gap:["USD"]` {
		t.Fatalf("computed row id = %q", res.Rows[6].ID)
	}

	open := z.run(t, "ann", fmt.Sprintf(zooStatement, ""), Options{ExpandAll: true}, nil)
	requireRows(t, open, "kind", "group", "detail", "detail", "subtotal", "group", "detail", "detail", "subtotal", "group", "subtotal", "computed", "computed")
}

func TestLayoutComputedRowsSplitThePivot(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", fmt.Sprintf(zooStatement, `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"},`), Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Monkeys", "Total Monkeys", "Lions less zebras", "All animals")
	requireRows(t, res, "cost.p0", int64(1000), int64(1000), int64(700), int64(700), nil, nil, int64(300), int64(1700))
	requireRows(t, res, "cost.p2", int64(0), int64(0), int64(300), int64(300), nil, nil, int64(-300), int64(300))
}

func TestLayoutErrors(t *testing.T) {
	z := newZoo(t)
	base := `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"layout":[{"section":"cats","title":"Lions","key":"lion"},%s]}`
	cases := map[string]struct {
		row, at, want string
		status        int
	}{
		"unknown section": {`{"row":"r","label":"R","formula":{"subtotal":"dogs"}}`, "layout.1.formula", "no section or earlier row", 422},
		"later row":       {`{"row":"r","label":"R","formula":{"subtotal":"s"}},{"row":"s","label":"S","formula":{"value":1}}`, "layout.1.formula", "no section or earlier row", 422},
		"a column":        {`{"row":"r","label":"R","formula":{"col":"cost"}}`, "layout.1.formula", "reads sections' subtotals", 400},
		"not a number":    {`{"row":"r","label":"R","formula":{"value":"x"}}`, "layout.1.formula", "is a number", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", fmt.Sprintf(base, c.row), Options{}, nil)
			requireSpecError(t, err, c.status, c.at, c.want)
			spec, _, _ := Load([]byte(fmt.Sprintf(base, c.row)))
			requireSpecError(t, Validate(z.app, spec), c.status, c.at, c.want)
		})
	}
}

// layoutOf is a one-group statement over cost with the given layout items.
func layoutOf(items string) string {
	return `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"layout":[` + items + `]}`
}

const lions = `{"section":"cats","title":"Lions","key":"lion"}`

func TestLayoutComputedRowsKeepTheColumnType(t *testing.T) {
	z := newZoo(t)
	third := layoutOf(lions + `,{"row":"r","label":"R","formula":{"op":"div","scale":0,"args":[{"subtotal":"cats"},{"value":3}]}}`)
	res := z.run(t, "ann", third, Options{}, nil)
	// 3500 / 3 rounds to whole cents.
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(1167))

	scaled := layoutOf(lions + `,{"row":"r","label":"R","formula":{"op":"div","scale":2,"args":[{"subtotal":"cats"},{"value":3}]}}`)
	_, err := z.try(t, "ann", scaled, Options{}, nil)
	requireSpecError(t, err, 400, "layout.1.formula.scale", "use scale 0")

	product := layoutOf(lions + `,{"row":"r","label":"R","formula":{"op":"mul","args":[{"subtotal":"cats"},{"subtotal":"cats"}]}}`)
	_, err = z.try(t, "ann", product, Options{}, nil)
	requireSpecError(t, err, 400, "layout.1.formula", "an amount times an amount")

	fraction := layoutOf(lions + `,{"row":"r","label":"R","formula":{"op":"mul","args":[{"subtotal":"cats"},{"value":0.333}]}}`)
	res = z.run(t, "ann", fraction, Options{}, nil)
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(1166))
}

func TestLayoutRefusesAHugeExpansion(t *testing.T) {
	z := newZoo(t)
	items := lions + `,{"row":"r0","label":"R","formula":{"subtotal":"cats"}}`
	for i := 1; i < 30; i++ {
		items += fmt.Sprintf(`,{"row":"r%d","label":"R","formula":{"op":"add","args":[{"subtotal":"r%d"},{"subtotal":"r%d"}]}}`, i, i-1, i-1)
	}
	start := time.Now()
	_, err := z.try(t, "ann", layoutOf(items), Options{}, nil)
	requireSpecError(t, err, 400, "layout.10", "too large")
	if time.Since(start) > 2*time.Second {
		t.Fatalf("refusal took %v", time.Since(start))
	}
}

func TestLayoutBoolSectionKey(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"active","path":[{"link":"animal"},{"field":"active"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"active"}],"layout":[
		{"section":"on","title":"Active","key":true},
		{"row":"r","label":"R","formula":{"subtotal":"on"}}]}`, Options{}, nil)
	requireRows(t, res, "cost", int64(4500), int64(4500), int64(4500))
}

func TestLayoutRefusesBadSectionKeys(t *testing.T) {
	z := newZoo(t)
	object := layoutOf(`{"section":"a","title":"A","key":{"a":1}}`)
	_, err := z.try(t, "ann", object, Options{}, nil)
	requireSpecError(t, err, 400, "layout.0.key", "a section key is")
	spec, _, _ := Load([]byte(object))
	requireSpecError(t, Validate(z.app, spec), 400, "layout.0.key", "a section key is")

	twice := layoutOf(lions + `,{"section":"again","title":"Again","key":"lion"}`)
	_, err = z.try(t, "ann", twice, Options{}, nil)
	requireSpecError(t, err, 400, "layout.1.key", "same group")
	spec, _, _ = Load([]byte(twice))
	requireSpecError(t, Validate(z.app, spec), 400, "layout.1.key", "same group")
}

func TestLayoutComputedRowsWithNothingVisible(t *testing.T) {
	z := newZoo(t)
	row := `{"row":"r","label":"R","formula":{"op":"add","args":[{"subtotal":"cats"},{"value":5}]}}`
	noRows := `"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"no such note"}]},`
	empty := layoutOf(lions + "," + row)
	empty = strings.Replace(empty, `"groups"`, noRows+`"groups"`, 1)
	res := z.run(t, "ann", empty, Options{}, nil)
	requireRows(t, res, "kind", "group", "subtotal", "computed")
	requireRows(t, res, "cost", nil, nil, int64(5))

	// Without a currency column the row reads the same.
	plain := strings.Replace(empty, `"id":"cost","path":[{"field":"cost"}]`, `"id":"cost","path":[{"field":"grams"}]`, 1)
	res = z.run(t, "ann", plain, Options{}, nil)
	requireRows(t, res, "cost", nil, nil, int64(5))
}

func TestLayoutRefusesExists(t *testing.T) {
	z := newZoo(t)
	spec := layoutOf(lions + `,{"row":"r","label":"R","formula":{"op":"exists","link":[{"link":"animal"}],"where":{"value":true}}}`)
	_, err := z.try(t, "ann", spec, Options{}, nil)
	requireSpecError(t, err, 400, "layout.1.formula", "reads sections' subtotals")
}

func TestLayoutRowCapKeepsTheComputedRows(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 2 })
	res := z.run(t, "ann", fmt.Sprintf(zooStatement, ""), Options{}, nil)
	requireRows(t, res, "kind", "group", "subtotal", "group", "subtotal", "group", "subtotal", "computed", "computed")
	requireRows(t, res, "cost", int64(3500), int64(3500), nil, nil, nil, nil, int64(2500), int64(4500))
	if len(res.Notices) != 1 || res.Notices[0].Code != "rowCap" {
		t.Fatalf("notices = %+v", res.Notices)
	}

	withLimits(t, func(l *limits) { l.rowCap = 1 })
	res = z.run(t, "ann", fmt.Sprintf(zooStatement, ""), Options{}, nil)
	requireRows(t, res, "cost", nil, nil, nil, nil, nil, nil, int64(2500), int64(4500))
	if len(res.Notices) != 1 || res.Notices[0].Code != "rowCap" {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

func TestLayoutRowCapKeepsComputedRowsInEveryCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("zed"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	withLimits(t, func(l *limits) { l.rowCap = 4 })
	res := z.run(t, "ann", fmt.Sprintf(zooStatement, ""), Options{}, nil)
	var got []any
	for _, r := range res.Rows {
		if r.Kind == KindComputed {
			got = append(got, r.ID, r.Cells["cost"])
		}
	}
	want := []any{`c:gap:["EUR"]`, int64(-400), `c:all:["EUR"]`, int64(400), `c:gap:["USD"]`, int64(2500), `c:all:["USD"]`, int64(4500)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("computed rows = %#v\nwant %#v", got, want)
	}
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "rowCap" }) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// wideLayout is nine doubling rows (the last has 1,021 nodes) and then n rows
// that each read the last.
func wideLayout(n int) string {
	items := lions + `,{"row":"r0","label":"R","formula":{"subtotal":"cats"}}`
	for i := 1; i <= 8; i++ {
		items += fmt.Sprintf(`,{"row":"r%d","label":"R","formula":{"op":"add","args":[{"subtotal":"r%d"},{"subtotal":"r%d"}]}}`, i, i-1, i-1)
	}
	for i := 0; i < n; i++ {
		items += fmt.Sprintf(`,{"row":"w%d","label":"W","formula":{"subtotal":"r8"}}`, i)
	}
	return layoutOf(items)
}

func TestLayoutRefusesAWideExpansion(t *testing.T) {
	z := newZoo(t)
	for _, n := range []int{40, 190} {
		raw := wideLayout(n)
		start := time.Now()
		_, err := z.try(t, "ann", raw, Options{}, nil)
		requireTooLarge(t, err)
		spec, _, _ := Load([]byte(raw))
		requireTooLarge(t, Validate(z.app, spec))
		if time.Since(start) > 2*time.Second {
			t.Fatalf("n=%d took %v", n, time.Since(start))
		}
	}
}

func requireTooLarge(t *testing.T, err error) {
	t.Helper()
	var se *SpecError
	if !errors.As(err, &se) || se.Status != 400 {
		t.Fatalf("err = %v, want a 400 SpecError", err)
	}
	for _, p := range se.Problems {
		if strings.Contains(p.Error(), "too large") {
			return
		}
	}
	t.Fatalf("err = %v, want too large", err)
}

func TestLayoutStatementWithAPivotStillRuns(t *testing.T) {
	z := newZoo(t)
	items := lions + `,{"section":"stripes","title":"Z","key":"zebra"}`
	for i := 0; i < 30; i++ {
		items += fmt.Sprintf(`,{"row":"c%d","label":"C","formula":{"op":"sub","args":[{"subtotal":"cats"},{"subtotal":"stripes"}]}}`, i)
	}
	raw := strings.Replace(layoutOf(items), `"groups"`, `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"},"groups"`, 1)
	res := z.run(t, "ann", raw, Options{}, nil)
	requireRows(t, res, "cost.p0", append([]any{int64(1000), int64(1000), int64(700), int64(700)}, repeat(int64(300), 30)...)...)
}

func repeat(v any, n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// manySections is a statement with the given sections, a pivot, and rows
// rows that each add up 51 of the sections (or all of them, when fewer).
func manySections(sections, rows int) string {
	var items []string
	for i := 0; i < sections; i++ {
		items = append(items, fmt.Sprintf(`{"section":"s%d","title":"S%d","key":"k%d"}`, i, i, i))
	}
	items[0] = `{"section":"s0","title":"Lions","key":"lion"}`
	items[1] = `{"section":"s1","title":"Zebras","key":"zebra"}`
	for r := 0; r < rows; r++ {
		args := make([]string, min(51, sections))
		for a := range args {
			args[a] = fmt.Sprintf(`{"subtotal":"s%d"}`, (r*7+a)%sections)
		}
		items = append(items, fmt.Sprintf(`{"row":"r%d","label":"R","formula":{"op":"add","args":[%s]}}`, r, strings.Join(args, ",")))
	}
	return strings.Replace(layoutOf(strings.Join(items, ",")), `"groups"`, `"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"},"groups"`, 1)
}

// layoutStatement compiles a spec's group statement as Validate does, with
// three pivot values.
func layoutStatement(t *testing.T, app core.App, raw string) string {
	t.Helper()
	p, specErr := bind(app, loadSpec(t, raw))
	if specErr != nil {
		t.Fatal(specErr)
	}
	c := newCompiler(p, nil, nil, nil)
	s := newStmt(nil, nil)
	r, err := c.rows(s)
	if err != nil {
		t.Fatal(err)
	}
	ctes, body, err := c.groupStatement(s, r, &expansion{}, make([]any, 3), 1)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(ctes, ",\n") + "\n" + body
}

// Each section's cells are read from o1 once, in its chunk's CTE, however
// many computed rows and {subtotal} leaves read them: twice the rows add no
// read. A statement that re-read the sections per leaf would grow with the
// leaves and pass the time limit.
func TestLayoutManySectionsReadsEachSectionOnce(t *testing.T) {
	app := withZooRegistry(t)
	for _, rows := range []int{29, 58} {
		sql := layoutStatement(t, app, manySections(142, rows))
		// 142 sections, each with 4 cells: cost and three pivot cells.
		if n := strings.Count(sql, "MAX(CASE WHEN k0 IS"); n != 142*4 {
			t.Fatalf("%d rows: %d section cell reads, want %d", rows, n, 142*4)
		}
		if n := strings.Count(sql, "FROM o1 GROUP BY"); n != 1 {
			t.Fatalf("%d rows: %d section chunks, want 1", rows, n)
		}
		// Every leaf reads its section's cell from the chunk.
		if n := strings.Count(sql, "COALESCE(ls0.s"); n != rows*51*4 {
			t.Fatalf("%d rows: %d leaf reads, want %d", rows, n, rows*51*4)
		}
	}
}

// The chunk's cells are the sections' subtotals, by column and pivot value.
func TestLayoutSectionChunkComputesItsRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", manySections(10, 3), Options{}, nil)
	// Row 0 adds sections 0..9, which include the lion and the zebra.
	var first Row
	for _, r := range res.Rows {
		if r.ID == `c:r0:["USD"]` {
			first = r
		}
	}
	if first.Cells["cost"] != int64(4500) || first.Cells["cost.p0"] != int64(1700) {
		t.Fatalf("row r0 = %+v", first.Cells)
	}
}

func TestLayoutSectionIdsThatDifferInCase(t *testing.T) {
	z := newZoo(t)
	raw := layoutOf(`{"section":"L","title":"Lions","key":"lion"},{"section":"l","title":"Zebras","key":"zebra"},
		{"row":"big","label":"Big L","formula":{"subtotal":"L"}},
		{"row":"small","label":"Small l","formula":{"subtotal":"l"}}`)
	res := z.run(t, "ann", raw, Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Big L", "Small l")
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(1000), int64(1000), int64(3500), int64(1000))
}

func TestLayoutValidateCountsTheWidestPivot(t *testing.T) {
	z := newZoo(t)
	doubling := lions + `,{"row":"r0","label":"R","formula":{"subtotal":"cats"}}`
	for i := 1; i <= 6; i++ {
		doubling += fmt.Sprintf(`,{"row":"r%d","label":"R","formula":{"op":"add","args":[{"subtotal":"r%d"},{"subtotal":"r%d"}]}}`, i, i-1, i-1)
	}
	with := func(maxColumns int) string {
		return strings.Replace(layoutOf(doubling), `"groups"`, fmt.Sprintf(`"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost","maxColumns":%d},"groups"`, maxColumns), 1)
	}
	narrow, _, _ := Load([]byte(with(1)))
	if err := Validate(z.app, narrow); err != nil {
		t.Fatalf("one pivot column: %v", err)
	}
	wide, _, _ := Load([]byte(with(100)))
	requireTooLarge(t, Validate(z.app, wide))
}
