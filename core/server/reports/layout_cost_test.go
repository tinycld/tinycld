package reports

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
)

// balanceSheetZoo is the balance fixture with lines ledger lines
// (withBenchLedger), the data of the layout benchmark.
func balanceSheetZoo(tb testing.TB, lines int) *zoo {
	tb.Helper()
	z := newZoo(tb)
	withBalanceZoo(tb, z)
	withBenchLedger(tb, z, lines)
	return z
}

// A Balance-Sheet-shaped layout reads its lines' rule once for its rows and
// once for each distinct lookup (Retained earnings and Net income, current
// and prior), each lookup in one scan. Before the hoist the statement copied
// every lookup's three subqueries into each guard and cell that read it:
// 1,495 reads of the rule and 1 MB of SQL, two seconds at 300 lines.
func TestABalanceSheetShapedLayoutReadsTheLinesRuleOncePerLookup(t *testing.T) {
	z := balanceSheetZoo(t, 40)
	m := measureRun(t, z, benchBalanceSheet())
	if m.ruleReads > 5 {
		t.Fatalf("the statement reads the lines rule %d times, want at most 5", m.ruleReads)
	}
	if m.bytes > 100_000 {
		t.Fatalf("the statement is %d bytes, want at most 100,000", m.bytes)
	}
	if n := strings.Count(m.sql, " AS MATERIALIZED (SELECT sum("); n != 4 {
		t.Fatalf("%d one-scan lookup CTEs, want 4", n)
	}
}

// The hoisted lookups give the figures the lines give: Retained earnings and
// Net income, as of the day and a year before, summed by SQL over the lines.
func TestABalanceSheetShapedLayoutLookupsMatchTheLines(t *testing.T) {
	z := balanceSheetZoo(t, 40)
	res, err := benchRun(z, benchBalanceSheet())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cell := func(row, col string) int64 {
		for _, r := range res.Rows {
			// A computed row's id is c:<row>, with a currency suffix when
			// the report has currency keys (assemble.go currencySuffix).
			if r.ID == "c:"+row || strings.HasPrefix(r.ID, "c:"+row+":") {
				return toInt(r.Cells[col])
			}
		}
		t.Fatalf("no row %s in %d rows", row, len(res.Rows))
		return 0
	}
	sum := func(where string, day, year string) int64 {
		var v int64
		err := z.app.DB().NewQuery("SELECT COALESCE(SUM(l.credit - l.debit), 0) FROM zoo_ledger_lines l " +
			"JOIN zoo_ledger_accounts a ON a.id = l.account WHERE l.posted = 1 AND l.date <= {:day} AND " + where).
			Bind(dbx.Params{"day": day, "year": year}).Row(&v)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// zoo.ledger_lines declares no FiscalStartMonth, so its fiscal year is the
	// calendar year and fiscalYear(day) is the day's year.
	retained := `(a.detail_type = 'retained_earnings' OR (a.type IN ('income', 'expense') AND substr(l.date, 1, 4) < {:year}))`
	net := `a.type IN ('income', 'expense') AND substr(l.date, 1, 4) >= {:year}`
	nonzero := false
	for _, c := range []struct{ row, where, col, day, year string }{
		{"retainedEarnings", retained, "current", "2026-06-30", "2026"},
		{"retainedEarnings", retained, "prior", "2025-06-30", "2025"},
		{"netIncome", net, "current", "2026-06-30", "2026"},
		{"netIncome", net, "prior", "2025-06-30", "2025"},
	} {
		want := sum(c.where, c.day, c.year)
		nonzero = nonzero || want != 0
		if got := cell(c.row, c.col); got != want {
			t.Errorf("%s %s = %d, the lines give %d", c.row, c.col, got, want)
		}
	}
	if !nonzero {
		t.Fatal("every expected figure is 0: the fixture checks nothing")
	}
}

// manyLookups is a layout row Many that adds count lookups of grams, each with
// its own where, so each is a distinct lookup.
func manyLookups(count int) string {
	var args []string
	for i := range count {
		args = append(args, fmt.Sprintf(`{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","where":{"op":"gte","args":[{"path":[{"field":"grams"}]},{"value":%d}]},"value":{"path":[{"field":"grams"}]},"pick":"sum"},{"value":0}]}`, i))
	}
	return layoutOf(lions + `,{"row":"many","label":"Many","formula":{"op":"add","args":[` + strings.Join(args, ",") + `]}}`)
}

// manyFigure is the cost cell of the row Many.
func manyFigure(t *testing.T, res *Result) int64 {
	t.Helper()
	for _, r := range res.Rows {
		if r.ID == "c:many" || strings.HasPrefix(r.ID, "c:many:") {
			return toInt(r.Cells["cost"])
		}
	}
	t.Fatal("no row many")
	return 0
}

// manyWant is the sum the lookups of manyLookups give, by SQL over the rows
// ann may read: zoo_feedings' rule shows the feedings of the animals she keeps.
func manyWant(t *testing.T, z *zoo, count int) int64 {
	t.Helper()
	var want int64
	for i := range count {
		var v int64
		if err := z.app.DB().NewQuery("SELECT COALESCE(SUM(grams), 0) FROM zoo_feedings WHERE grams >= {:i} " +
			"AND animal IN (SELECT animal FROM zoo_keepers WHERE user = {:ann})").
			Bind(dbx.Params{"i": i, "ann": z.id("ann")}).Row(&v); err != nil {
			t.Fatal(err)
		}
		want += v
	}
	return want
}

// SQLite joins at most 64 tables: a computed row reads every hoisted lookup
// through the one row of lx, so a row reading 70 lookups still runs. One join
// per lookup would make 70 + cur = 71 tables, and fail.
func TestALayoutRowReadingManyLookupsStillRuns(t *testing.T) {
	z := newZoo(t)
	res, err := z.try(t, "ann", manyLookups(70), Options{}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := manyFigure(t, res), manyWant(t, z, 70); got != want || want == 0 {
		t.Fatalf("Many = %d, the rows give %d", got, want)
	}
}

// Past maxLookupColumns a lookup is read inline, as before the hoist, and
// gives the same figure.
func TestALookupPastTheColumnCapIsReadInline(t *testing.T) {
	old := maxLookupColumns
	maxLookupColumns = 5 // room for exactly one lookup (lookupsRoom asks for 5)
	t.Cleanup(func() { maxLookupColumns = old })
	z := newZoo(t)
	seen := observeReads(t)
	res, err := z.try(t, "ann", manyLookups(3), Options{}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := manyFigure(t, res), manyWant(t, z, 3); got != want || want == 0 {
		t.Fatalf("Many = %d, the rows give %d", got, want)
	}
	hoisted := false
	for _, st := range *seen {
		hoisted = hoisted || strings.Contains(st.sql, "lx0 AS MATERIALIZED")
		if strings.Contains(st.sql, "lx1 AS") {
			t.Fatalf("a second lookup was hoisted past the cap:\n%s", st.sql)
		}
	}
	if !hoisted {
		t.Fatal("the first lookup was not hoisted")
	}
}

// The level CTEs of a having that reads a lookup are named lk<L>
// (levels.go lv.pre) and share the WITH list with the layout's CTEs, so the
// hoisted lookups must not take those names. A layout refuses a having on
// its first group (bind.go layout), so the having is on a second group.
func TestALayoutWithALookupInALevelHavingRuns(t *testing.T) {
	z := newZoo(t)
	having := `{"op":"gt","args":[{"op":"lookup","source":"zoo.feedings","value":{"path":[{"field":"grams"}]},"pick":"sum"},{"value":0}]}`
	raw := strings.Replace(manyLookups(3), `"groups":[{"id":"g","col":"species"}]`,
		`"groups":[{"id":"g","col":"species"},{"id":"a","col":"animal","having":`+having+`}]`, 1)
	raw = strings.Replace(raw, `{"id":"cost",`, `{"id":"animal","path":[{"link":"animal"},{"field":"name"}]},{"id":"cost",`, 1)
	seen := observeReads(t)
	res, err := z.try(t, "ann", raw, Options{}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := manyFigure(t, res), manyWant(t, z, 3); got != want || want == 0 {
		t.Fatalf("Many = %d, the rows give %d", got, want)
	}
	level := regexp.MustCompile(`\blk\d+ AS MATERIALIZED`)
	for _, st := range *seen {
		if level.MatchString(st.sql) && strings.Contains(st.sql, "lx0 AS MATERIALIZED") {
			return
		}
	}
	t.Fatal("no statement holds both a level lookup CTE and a hoisted layout lookup")
}

func TestReadsLayoutRow(t *testing.T) {
	for sql, want := range map[string]bool{
		"cur.ck0": true, "ls3.s0_v1": true, "lx.c0": false, "lb4.v": false, "rcur.x": false,
	} {
		if got := readsLayoutRow(sql); got != want {
			t.Errorf("readsLayoutRow(%q) = %v, want %v", sql, got, want)
		}
	}
}

// A computed row that reads an earlier row many times compiles the earlier
// cell once: the compiler reuses it, so the nodes it compiles stay far under
// what the caps count (each read counted in full). The derived-column case is
// the next test.
func TestLayoutCompilesEachEarlierRowCellOnce(t *testing.T) {
	z := newZoo(t)
	// a is 20 nodes: each reader counts 1 + 20*20 nodes (about 4,030 in all),
	// while with the memo the compiler visits about 460.
	leaves := make([]string, 19)
	for i := range leaves {
		leaves[i] = `{"subtotal":"cats"}`
	}
	base := `{"row":"a","label":"A","formula":{"op":"add","args":[` + strings.Join(leaves, ",") + `]}}`
	items := []string{lions, base}
	for r := range 10 {
		args := make([]string, 20)
		for i := range args {
			args[i] = `{"subtotal":"a"}`
		}
		items = append(items, fmt.Sprintf(`{"row":"r%d","label":"R","formula":{"op":"add","args":[%s]}}`, r, strings.Join(args, ",")))
	}
	spec, _, err := Load([]byte(layoutOf(strings.Join(items, ","))))
	if err != nil {
		t.Fatal(err)
	}
	p, serr := bind(z.app, spec)
	if serr != nil {
		t.Fatal(serr)
	}
	c := newCompiler(p, nil, nil, nil)
	if err := c.compileAll(); err != nil {
		t.Fatal(err)
	}
	if c.layoutNodes*4 > c.layoutCounted {
		t.Fatalf("compiled %d formula nodes for %d counted: earlier rows are compiled again", c.layoutNodes, c.layoutCounted)
	}
}

// The same with derived columns: each row's Change and % change read its cur
// and pri cells (the memo is seeded from inside the derived column), and every
// reader reads an earlier row 10 times. The figures check behavior, not only
// the compile count: a is 9 Zebras totals (1000 / 700), a reader is 10 of a.
func TestLayoutCompilesEachEarlierRowCellOnceWithDerivedColumns(t *testing.T) {
	z := newZoo(t)
	reads := func(n int, id string) string {
		args := make([]string, n)
		for i := range args {
			args[i] = `{"subtotal":"` + id + `"}`
		}
		return `{"op":"add","args":[` + strings.Join(args, ",") + `]}`
	}
	rows := []string{`{"row":"a","label":"A","formula":` + reads(9, "stripes") + `}`}
	for r := range 4 {
		rows = append(rows, fmt.Sprintf(`{"row":"r%d","label":"R%d","formula":%s}`, r, r, reads(10, "a")))
	}
	raw := fmt.Sprintf(zooCells, ","+strings.Join(rows, ","))
	spec, _, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	p, serr := bind(z.app, spec)
	if serr != nil {
		t.Fatal(serr)
	}
	c := newCompiler(p, nil, nil, nil)
	if err := c.compileAll(); err != nil {
		t.Fatal(err)
	}
	if c.layoutNodes*4 > c.layoutCounted {
		t.Fatalf("compiled %d formula nodes for %d counted: earlier rows are compiled again", c.layoutNodes, c.layoutCounted)
	}
	res := z.run(t, "ann", raw, Options{}, asOf2027)
	last := rowLabeled(t, res, "R3")
	// 10 * 9 * 1000 and 10 * 9 * 700; Change is 27,000.
	if last.Cells["cur"] != int64(90000) || last.Cells["pri"] != int64(63000) || last.Cells["chg"] != int64(27000) {
		t.Fatalf("R3 = %+v", last.Cells)
	}
}

func TestMaterializeSharedMarksRowsReadTwice(t *testing.T) {
	cases := []struct {
		name, body, want string
		ctes             []string
	}{
		{"two levels", "SELECT __level FROM a0", "w AS MATERIALIZED (SELECT 1)", []string{"w AS (SELECT 1)", "a0 AS (SELECT k FROM w)", "a1 AS (SELECT k FROM w GROUP BY k)"}},
		{"one read", "SELECT __level FROM w LIMIT 5", "w AS (SELECT 1)", []string{"w AS (SELECT 1)"}},
		{"ws is not w", "SELECT __level FROM ws JOIN ws2", "w AS (SELECT 1)", []string{"w AS (SELECT 1)", "ws AS (SELECT w.* FROM w)"}},
		// The __level gate: a body with no __level (a detail page, also one
		// whose probe reads w a second time) keeps w inline.
		{"detail with a probe", "SELECT * FROM w WHERE EXISTS (SELECT 1 FROM w WHERE x) LIMIT 5", "w AS (SELECT 1)", []string{"w AS (SELECT 1)"}},
	}
	for _, c := range cases {
		got := materializeShared(c.ctes, c.body)
		if got[0] != c.want {
			t.Errorf("%s: %q, want %q", c.name, got[0], c.want)
		}
	}
}

// A grouped statement reads w once per level: SQLite computes it once.
func TestGroupStatementMaterializesTheRowsItReadsTwice(t *testing.T) {
	m := measureRun(t, balanceSheetZoo(t, 40), benchBalanceSheet())
	if !strings.Contains(m.sql, "w AS MATERIALIZED (") {
		t.Fatal("the group statement reads w per level but does not materialize it")
	}
}

// A detail page reads w once, under its LIMIT: it stays inline, so SQLite
// can stop at the page.
func TestDetailStatementKeepsTheRowsInline(t *testing.T) {
	z := balanceSheetZoo(t, 40)
	seen := observeReads(t)
	if _, err := benchRunWith(z, `{"version":4,"source":"zoo.ledger_lines","columns":[{"id":"date","path":[{"field":"date"}]}]}`, nil); err != nil {
		t.Fatal(err)
	}
	for _, st := range *seen {
		if strings.Contains(st.sql, "w AS MATERIALIZED") {
			t.Fatalf("a flat report materializes w:\n%s", st.sql)
		}
	}
	if len(*seen) == 0 {
		t.Fatal("no statement observed")
	}
}
