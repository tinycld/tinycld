package reports

import (
	"slices"
	"strings"
	"testing"
)

// splitRead is the text of the Split column's lookups in a statement: they
// match the looked-up line's entry with the row's.
const splitRead = `.entry]]`

// detailStatement is the statement of seen that reads a page of detail
// rows: the one that orders by the detail row's id.
func detailStatement(t *testing.T, seen []readStatement) string {
	t.Helper()
	for _, st := range seen {
		if strings.Contains(st.sql, "d.__id LIMIT ") {
			return st.sql
		}
	}
	t.Fatal("no statement reads a page of detail rows")
	return ""
}

// The Split lookups read nothing a sort key or the running balance needs, so
// they are computed over the rows the page cut left. The page is a CTE of its
// own (wpage, materialized: SQLite would otherwise flatten it under the
// lookups); it holds the running balance, ww0, and the layers it reads. The
// layers above the cut are a second materialized CTE (wcells) over it.
func TestARowsOwnLookupsAreComputedForThePagesRows(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	seen := observeReads(t)
	z.run(t, "ann", balanceSpec(), Options{Today: "2026-07-15", Limit: 3}, map[string]string{"period": glJuly})
	sql := detailStatement(t, *seen)
	var st readStatement
	found := false
	for _, s := range *seen {
		if s.sql == sql {
			st, found = s, true
		}
	}
	if !found {
		t.Fatal("no observed statement is the detail statement")
	}
	page := strings.Index(sql, "wpage AS MATERIALIZED (")
	cells := strings.Index(sql, ", wcells AS MATERIALIZED (")
	end := strings.LastIndex(sql, ") d ORDER BY d.__g, ")
	if page < 0 || cells < page || end < cells {
		t.Fatalf("the page and the cells above it are not two materialized CTEs:\n%s", sql)
	}
	pageText, cellsText := sql[page:cells], sql[cells:end]
	if !strings.Contains(pageText, "d.__id LIMIT ") {
		t.Fatalf("the page is not cut in its CTE:\n%s", sql)
	}
	if !strings.Contains(pageText, " AS ww0") {
		t.Fatalf("the running balance is not computed below the page cut:\n%s", sql)
	}
	if !strings.Contains(pageText, " AS wh") {
		t.Fatalf("the layers the running balance reads are not below the page cut:\n%s", sql)
	}
	if strings.Contains(pageText, splitRead) {
		t.Fatalf("a Split lookup is computed below the page cut:\n%s", sql)
	}
	if !strings.Contains(cellsText, splitRead) || !strings.Contains(cellsText, "wpage") {
		t.Fatalf("the Split lookups are not computed over the page:\n%s", sql)
	}
	if !strings.Contains(sql[end:], ") d ORDER BY d.__g, ") {
		t.Fatalf("the page's rows are not put in order again:\n%s", sql)
	}
	// The text above is not enough: SQLite flattens a CTE it is not told to
	// materialize. In the plan tree, the Split lookups lie under wcells and
	// none of them under wpage.
	nodes := explainTree(t, z, st)
	pageNode := planNodeNamed(t, nodes, "MATERIALIZE wpage")
	cellsNode := planNodeNamed(t, nodes, "MATERIALIZE wcells")
	lookups := splitLookupNodes(nodes)
	if len(lookups) == 0 {
		t.Fatalf("the plan has no Split lookup: %+v", nodes)
	}
	for id := range lookups {
		if under(nodes, id, pageNode) {
			t.Fatalf("the Split lookup of plan node %d is under MATERIALIZE wpage: %+v", id, nodes)
		}
		if !under(nodes, id, cellsNode) {
			t.Fatalf("the Split lookup of plan node %d is not under MATERIALIZE wcells: %+v", id, nodes)
		}
	}
}

// pageAndCells splits a statement into the text of its page CTE and the text
// of the CTE of the layers above the cut and the cells select after it.
func pageAndCells(t *testing.T, sql string) (page, cells string) {
	t.Helper()
	p := strings.Index(sql, pageCTE+" AS MATERIALIZED (")
	c := strings.Index(sql, ", "+cellsCTE+" AS MATERIALIZED (")
	if p < 0 || c < p {
		t.Fatalf("the page and the cells above it are not two materialized CTEs:\n%s", sql)
	}
	return sql[p:c], sql[c:]
}

// planNode is a node of SQLite's plan tree: its id, its parent's, its text.
type planNode struct {
	id, parent int
	detail     string
}

// explainTree is the nodes of SQLite's plan of one observed statement.
func explainTree(t *testing.T, z *zoo, st readStatement) []planNode {
	t.Helper()
	rows, err := z.app.DB().NewQuery("EXPLAIN QUERY PLAN " + st.sql).Bind(st.params).Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []planNode
	for rows.Next() {
		var n planNode
		var unused int
		if err := rows.Scan(&n.id, &n.parent, &unused, &n.detail); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// explainPlan is the lines of SQLite's plan of one observed statement.
func explainPlan(t *testing.T, z *zoo, st readStatement) []string {
	t.Helper()
	var out []string
	for _, n := range explainTree(t, z, st) {
		out = append(out, n.detail)
	}
	return out
}

// loopPlan is explainPlan without the subtrees of a LIST SUBQUERY: SQLite
// builds such a list once for the statement (the IN over gord of leafCut),
// so a scan inside it reads no table again for each row.
func loopPlan(t *testing.T, z *zoo, st readStatement) []string {
	t.Helper()
	nodes := explainTree(t, z, st)
	var lists []int
	for _, n := range nodes {
		if strings.HasPrefix(n.detail, "LIST SUBQUERY") {
			lists = append(lists, n.id)
		}
	}
	var out []string
	for _, n := range nodes {
		if !slices.ContainsFunc(lists, func(root int) bool { return under(nodes, n.id, root) }) {
			out = append(out, n.detail)
		}
	}
	return out
}

// under reports whether the node id lies in the subtree of root.
func under(nodes []planNode, id, root int) bool {
	parent := map[int]int{}
	for _, n := range nodes {
		parent[n.id] = n.parent
	}
	for ; id != 0; id = parent[id] {
		if id == root {
			return true
		}
	}
	return false
}

// planNodeNamed is the id of the plan node whose text is detail.
func planNodeNamed(t *testing.T, nodes []planNode, detail string) int {
	t.Helper()
	for _, n := range nodes {
		if n.detail == detail {
			return n.id
		}
	}
	t.Fatalf("the plan has no %q", detail)
	return 0
}

// splitLookupNodes is the scalar subquery nodes of the plan that read the
// looked-up line's entry (the Split column's lookups): the nearest scalar
// subquery above each search by the entry index.
func splitLookupNodes(nodes []planNode) map[int]bool {
	byID := map[int]planNode{}
	for _, n := range nodes {
		byID[n.id] = n
	}
	out := map[int]bool{}
	for _, n := range nodes {
		if !strings.Contains(n.detail, "_entry (entry") {
			continue
		}
		for a, ok := byID[n.parent]; ok; a, ok = byID[a.parent] {
			if strings.Contains(a.detail, "SCALAR SUBQUERY") {
				out[a.id] = true
				break
			}
		}
	}
	return out
}

// SQLite flattens a layer select into the select that reads it, and copies
// the expression of a hoisted value into each place that reads it (the Split
// cell reads its count twice; a beginning balance is read by several cells).
// Each copy is a scalar subquery of the plan, run once more. The paged
// statement must hold no more of them than the old one, which computes each
// hoisted value once.
func TestAHoistedValueTheCellsReadSeveralTimesIsComputedOnce(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	t.Cleanup(func() { legacySQL.cellsBeforeThePage = false })
	subqueries := func(old bool) int {
		legacySQL.cellsBeforeThePage = old
		seen := observeReads(t)
		z.run(t, "ann", balanceSpec(), Options{Today: "2026-07-15", Limit: 3}, map[string]string{"period": glJuly})
		sql := detailStatement(t, *seen)
		for _, st := range *seen {
			if st.sql == sql {
				n := 0
				for _, line := range explainPlan(t, z, st) {
					if strings.Contains(line, "SCALAR SUBQUERY") {
						n++
					}
				}
				return n
			}
		}
		return -1
	}
	old, paged := subqueries(true), subqueries(false)
	if old <= 0 {
		t.Fatalf("the old statement has %d scalar subqueries: the case compares nothing", old)
	}
	if paged > old {
		t.Fatalf("the paged statement has %d scalar subqueries, the old one %d", paged, old)
	}
}

// A detail page with no groups reads w once, under its LIMIT: w stays inline,
// also when the cells are computed above the page cut (the CTEs of the cut
// sit in the body's own select, where materializeShared does not count them).
func TestAPagedDetailWithNoGroupsKeepsTheRowsInline(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"date","label":"Date","path":[{"field":"date"}]},
		{"id":"split","label":"Split","formula":` + strings.ReplaceAll(glSplit, "COUNT", glCount) + `},
		{"id":"run","label":"Run","formula":{"op":"runningSum","args":[` + glNet + `]}}],
		"sort":[{"formula":{"path":[{"field":"date"}]}},{"formula":{"path":[{"field":"position"}]}}]}`
	seen := observeReads(t)
	z.run(t, "ann", spec, Options{Today: "2026-07-15", Limit: 2}, nil)
	sql := detailStatement(t, *seen)
	if !strings.Contains(sql, "wcells AS MATERIALIZED (") || !strings.Contains(sql, splitRead) {
		t.Fatalf("the cells are not computed above the page cut:\n%s", sql)
	}
	if strings.Contains(sql, "w AS MATERIALIZED (") {
		t.Fatalf("a detail page with no groups materializes w:\n%s", sql)
	}
}

func TestCellsBeforeThePageIsTheOldDetailSelect(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	legacySQL.cellsBeforeThePage = true
	t.Cleanup(func() { legacySQL.cellsBeforeThePage = false })
	seen := observeReads(t)
	z.run(t, "ann", balanceSpec(), Options{Today: "2026-07-15", Limit: 3}, map[string]string{"period": glJuly})
	sql := detailStatement(t, *seen)
	if strings.Contains(sql, "ww0") || !strings.Contains(sql, splitRead) {
		t.Fatalf("the old detail select has no named running value:\n%s", sql)
	}
	if strings.Contains(sql, "FROM (SELECT * FROM (") {
		t.Fatalf("the old detail select cuts the page last:\n%s", sql)
	}
}

// A report with a running value and no lookup of its own has nothing to
// compute above the cut: its detail statement is the old one.
func TestADetailSelectWithNoLookupOfItsOwnIsWrittenAsBefore(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"date","label":"Date","path":[{"field":"date"}]},
		{"id":"run","label":"Run","formula":{"op":"runningSum","args":[` + glNet + `]}}],
		"sort":[{"formula":{"path":[{"field":"date"}]}},{"formula":{"path":[{"field":"position"}]}}]}`
	t.Cleanup(func() { legacySQL.cellsBeforeThePage = false })
	statement := func(old bool) string {
		legacySQL.cellsBeforeThePage = old
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{Today: "2026-07-15", Limit: 2}, nil)
		// The reader's rule above w binds names that differ in each run.
		sql := detailStatement(t, *seen)
		return sql[strings.Index(sql, "w AS ("):]
	}
	before, after := statement(true), statement(false)
	if before != after || !strings.Contains(after, " OVER (") {
		t.Fatalf("the detail statement changed:\nbefore: %s\nafter:  %s", before, after)
	}
}

// requirePagesSameAsLegacy runs spec page by page, each page with the switch
// on and off, and follows the cursor.
func requirePagesSameAsLegacy(t *testing.T, z *zoo, spec string, opts Options, params map[string]string) (pages int) {
	t.Helper()
	for range 100 {
		pages++
		requireSameAsLegacy(t, z, "ann", spec, opts, params, &legacySQL.cellsBeforeThePage)
		res := z.run(t, "ann", spec, opts, params)
		if res.Page == nil || res.Page.Next == "" {
			return pages
		}
		opts.Cursor = res.Page.Next
	}
	t.Fatal("the pages do not end")
	return pages
}

// accountLines is a lookup of the number of lines an account has, and
// accountDebit the sum of their debits: a row's own lookups on the accounts
// source.
const (
	accountLines = `{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"eq","args":[{"path":[{"link":"account"}]},{"outer":[{"field":"id"}]}]},
		"value":{"path":[{"field":"id"}]},"pick":"count"}`
	accountDebit = `{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"eq","args":[{"path":[{"link":"account"}]},{"outer":[{"field":"id"}]}]},
		"value":{"path":[{"field":"position"}]},"pick":"sum"}`
)

// groupedNest is a report whose detail pass takes its grouped path (a nest
// level keeps the page cut by the group cursor, groupedAfter, and not by the
// paged groups of balanceSpec): accounts grouped by type, each with its lines
// as child rows, a lookup cell nothing below the cut reads (lines), and a
// running value over another lookup (so a lookup stays below the cut).
const groupedNest = `{"version":4,"source":"zoo.ledger_accounts","columns":[
	{"id":"type","label":"Type","path":[{"field":"type"}]},
	{"id":"number","label":"No.","path":[{"field":"number"}]},
	{"id":"lines","label":"Lines","formula":` + accountLines + `},
	{"id":"run","label":"Run","formula":{"op":"runningSum","args":[` + accountDebit + `]}},
	{"id":"child","label":"Child","path":[{"link":"lines","mode":"nest"},{"field":"position"}]}],
	"sort":[{"col":"number"}],
	"groups":[{"id":"byType","col":"type"}]}`

// runningLookup is a lookup that reads a running value (run, over the line's
// position) and is shown by a cell only.
const runningLookup = `{"version":4,"source":"zoo.ledger_lines","columns":[
	{"id":"date","label":"Date","path":[{"field":"date"}]},
	{"id":"run","label":"Run","formula":{"op":"runningSum","args":[{"path":[{"field":"position"}]}]}},
	{"id":"below","label":"Below","formula":{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"lt","args":[{"path":[{"field":"position"}]},{"col":"run"}]},
		"value":{"path":[{"field":"id"}]},"pick":"count"}}],
	"sort":[{"formula":{"path":[{"field":"date"}]}},{"formula":{"path":[{"field":"position"}]}}]}`

// markedByRunningLookup is runningLookup with a where that also reads a sum
// that meets two currencies (with withMixedCash's lines): such a where is
// probed below the cut, on every row, to name the lookup in the run's notice,
// and the probe reads the running value. The lines cell is a layer nothing
// below the cut reads, which would move above it: the fallback is what keeps
// the select whole. (A sort key over a lookup of a running
// value is refused: a running value is not a sort key.)
const markedByRunningLookup = `{"version":4,"source":"zoo.ledger_lines","columns":[
	{"id":"date","label":"Date","path":[{"field":"date"}]},
	{"id":"lines","label":"Lines","formula":` + glCount + `},
	{"id":"run","label":"Run","formula":{"op":"runningSum","args":[{"path":[{"field":"position"}]}]}},
	{"id":"below","label":"Below","formula":{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"or","args":[
			{"op":"lt","args":[{"path":[{"field":"position"}]},{"col":"run"}]},
			{"op":"gt","args":[{"op":"lookup","source":"zoo.ledger_lines","pick":"sum","value":{"path":[{"field":"debit"}]}},{"value":0}]}]},
		"value":{"path":[{"field":"id"}]},"pick":"count"}}],
	"sort":[{"formula":{"path":[{"field":"date"}]}},{"formula":{"path":[{"field":"position"}]}}]}`

func TestCellsComputedForThePageKeepTheResult(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	july := map[string]string{"period": glJuly, "rows": `"all"`}
	opts := Options{Today: "2026-07-15"}
	paged := Options{Today: "2026-07-15", Limit: 3}
	// A cell that adds a running value to a row's own lookup, and a percent
	// of one.
	mixed := strings.Replace(balanceSpec(), `{"id":"debit","label":"Debit"`,
		`{"id":"lines","label":"Lines","formula":{"op":"add","args":[`+glCount+`,{"op":"runningSum","args":[`+glCount+`]}]}},
		 {"id":"share","label":"Share","formula":{"op":"percentOfParent","scale":2,"args":[`+glCount+`]}},
		 {"id":"debit","label":"Debit"`, 1)
	// A sort key that reads a row's own lookup: the lookup stays below the cut.
	sorted := strings.Replace(balanceSpec(), `{"formula":{"path":[{"field":"position"}]}}`,
		`{"formula":`+glCount+`,"dir":"desc"},{"formula":{"path":[{"field":"position"}]}}`, 1)
	// No groups: the page is cut by the keyset alone.
	flat := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"date","label":"Date","path":[{"field":"date"}]},
		{"id":"lines","label":"Lines","formula":` + glCount + `},
		{"id":"run","label":"Run","formula":{"op":"runningSum","args":[` + glNet + `]}}],
		"sort":[{"formula":{"path":[{"field":"date"}]}},{"formula":{"path":[{"field":"position"}]}}]}`
	cases := []struct {
		name, spec string
		opts       Options
		params     map[string]string
		pages      int
	}{
		{"one page", balanceSpec(), opts, july, 1},
		{"pages", balanceSpec(), paged, july, 2},
		{"a viewer sort", balanceSpec(), Options{Today: "2026-07-15", Limit: 3, Sort: []ViewSort{{Col: "split", Dir: "desc"}}}, july, 2},
		{"a running value over a lookup", mixed, paged, july, 2},
		{"a sort key over a lookup", sorted, paged, july, 2},
		{"no groups", flat, Options{Today: "2026-07-15", Limit: 2}, nil, 2},
		{"groups with child rows", groupedNest, Options{Today: "2026-07-15", Limit: 2, ExpandAll: true}, nil, 2},
		{"a lookup of a running value", runningLookup, Options{Today: "2026-07-15", Limit: 2}, nil, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := requirePagesSameAsLegacy(t, z, c.spec, c.opts, c.params); got < c.pages {
				t.Fatalf("%d pages, want at least %d", got, c.pages)
			}
		})
	}
}

// The detail pass of a report with groups and child rows is not the paged
// groups' (pagedGroups is false with a nest): it reads the page after the
// group cursor (groupedAfter), and its page, cut by the keyset in that
// cursor, still has its cells computed over the page's rows. The second page
// proves the cursor was followed: its statement reads rows after the group
// (d.__g >) and has the two-select form.
func TestGroupsWithChildRowsComputeTheirCellsForThePage(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	opts := Options{Today: "2026-07-15", Limit: 2, ExpandAll: true}
	first := z.run(t, "ann", groupedNest, opts, nil)
	if first.Page == nil || first.Page.Next == "" {
		t.Fatalf("the run has one page: %+v", first.Page)
	}
	opts.Cursor = first.Page.Next
	seen := observeReads(t)
	z.run(t, "ann", groupedNest, opts, nil)
	sql := detailStatement(t, *seen)
	if !strings.Contains(sql, "d.__g > ") || !strings.Contains(sql, " AS __n0") {
		t.Fatalf("the page does not follow the group cursor (groupedAfter):\n%s", sql)
	}
	page, _ := pageAndCells(t, sql)
	if !strings.Contains(sql, ") d ORDER BY d.__g, ") {
		t.Fatalf("the page's rows are not put in order again:\n%s", sql)
	}
	if !strings.Contains(page, " AS ww0") || !strings.Contains(page, " OVER (") {
		t.Fatalf("the running value is not computed below the page cut:\n%s", sql)
	}
	legacySQL.cellsBeforeThePage = true
	t.Cleanup(func() { legacySQL.cellsBeforeThePage = false })
	seen = observeReads(t)
	z.run(t, "ann", groupedNest, opts, nil)
	if old := detailStatement(t, *seen); strings.Contains(old, pageCTE) {
		t.Fatalf("the old detail select cuts the page last:\n%s", old)
	}
}

// A lookup whose where reads a running value, shown by a cell only, has its
// layer (wh0, which reads the running value as w.ww0) above the cut: only the
// running value itself is computed below it, over every row. (A where that a
// probe or sort key below the cut reads keeps the select whole: the unit test
// of pagedCells.)
func TestALookupOfARunningValueIsComputedAboveTheCutOverTheRunningValue(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	seen := observeReads(t)
	z.run(t, "ann", runningLookup, Options{Today: "2026-07-15", Limit: 2}, nil)
	sql := detailStatement(t, *seen)
	page, cells := pageAndCells(t, sql)
	if !strings.Contains(page, " AS ww0") || !strings.Contains(page, " OVER (") {
		t.Fatalf("the running value is not computed below the page cut:\n%s", sql)
	}
	if !strings.Contains(cells, "w.ww0 AS wh0") {
		t.Fatalf("the lookup's layer does not read the running value above the cut:\n%s", sql)
	}
	if strings.Contains(page, " AS wh0") {
		t.Fatalf("the lookup's layer is computed below the page cut:\n%s", sql)
	}
}

// A lookup whose where reads a running value and a sum that meets two
// currencies is probed below the cut (its mark is read before the page is
// cut), and the probe reads the running value: no running value can be named
// there, so the statement is the whole select, and its pages are still the
// old select's.
func TestAProbedLookupOfARunningValueKeepsTheWholeSelect(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withMixedCash(t, z)
	opts := Options{Today: "2026-07-15", Limit: 2}
	seen := observeReads(t)
	z.run(t, "ann", markedByRunningLookup, opts, nil)
	sql := detailStatement(t, *seen)
	if strings.Contains(sql, "FROM (SELECT * FROM (") || strings.Contains(sql, " AS ww0") {
		t.Fatalf("the select is cut, or names a running value:\n%s", sql)
	}
	if !strings.Contains(sql, " OVER (") || !strings.Contains(sql, " AS __wmx0") {
		t.Fatalf("the running value or the probe mark is not in the statement:\n%s", sql)
	}
	if got := requirePagesSameAsLegacy(t, z, markedByRunningLookup, opts, nil); got < 2 {
		t.Fatalf("%d pages, want at least 2", got)
	}
}

// A sort key that reads a lookup keeps that lookup, and the lookup it reads,
// below the page cut; a keyed layer stays there too; a layer nothing below
// the cut reads moves above.
func TestALookupASortKeyReadsStaysBelowThePageCut(t *testing.T) {
	f := &windowFrame{paged: true}
	key := f.hoist("w", "(SELECT 1)")
	f.hoist("w", "(SELECT 2)")
	of := f.hoist("w", "(SELECT "+key+" + 1)")
	f.hoistLayer("w", []string{"k0"}, "(SELECT 3)")
	for i := len(f.hoistedAs); i < 11; i++ {
		f.hoist("w", "(SELECT 'filler' || "+string(rune('a'+i))+")")
	}
	got := f.earlyLayers([]string{of + " AS s0"})
	want := []bool{true, false, true, true, false, false, false, false, false, false, false}
	if len(got) != len(want) {
		t.Fatalf("%d layers, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("layer %d early = %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
	// w.wh10 is not a read of w.wh1.
	if f.earlyLayers([]string{"w.wh10 AS s0"})[1] {
		t.Fatal("w.wh10 was read as w.wh1")
	}
}

// A lookup's where that reads a running value is probed below the cut, where
// no running value is named: the select stays whole.
func TestAValueBelowTheCutThatReadsARunningValueKeepsTheWholeSelect(t *testing.T) {
	f := &windowFrame{paged: true}
	run := f.window("SUM(w.c0) OVER (ORDER BY w.__id)")
	f.hoist("w", "(SELECT 1)")
	sel := []string{"w.__id AS __id", "w.wh0 AS v0", "(CASE WHEN " + run + " > 1 THEN 1 ELSE 0 END) AS __wmx0"}
	rows, cells := pagedCells(f, "w", sel, []earlyCol{{at: 2, name: "__wmx0"}})
	if cells != nil || strings.Contains(rows, "ww0") || !strings.Contains(rows, "SUM(w.c0) OVER (ORDER BY w.__id) > 1") {
		t.Fatalf("the select was cut, or a running value has no SQL:\n%s", rows)
	}
}
