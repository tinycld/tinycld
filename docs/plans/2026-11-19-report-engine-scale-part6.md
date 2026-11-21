# Report Engine Scale, Part 6: A Detail Row's Own Lookups Are Computed for the Page's Rows

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The detail statement of a report computes the lookups of a detail row (the General Ledger's Split and name lookups) for the rows of the page only, not for every row of the period, with every report's result unchanged.

**Architecture:** Today the detail select computes every cell of every row, and the page is cut last (`WHERE <cursor> ORDER BY … LIMIT n` around it). This part writes the detail statement as two selects. The select below the cut holds each row with its sort keys, its probe marks, its running values and percents (named `ww<n>`), and only the hoisted layers one of those reads; a layer computed once for each group key stays there too. The select above the cut adds the other hoisted layers and the cells, over the page's rows, and puts the rows in the same order again. When no layer would move above the cut, the statement is written as before.

**Tech Stack:** Go, SQLite (PocketBase fork), core report engine `tinycld.org/core/reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause). The unit's requirements and the baseline: part 1, `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-19-report-engine-scale.md`. Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`. The measurement this part rests on: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part6-scratch/100k/parts.md`, read with `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part6.md` (the layers of the final detail select, the decision table).

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`); Books is `books/` (same branch name). Task 5d is a core task: it may edit `tinycld/core`. Its last step changes one Books file, the ledger record.

**Order:** parts 1 to 5 are done (core 1bd18afe, books b35a77d4). This part: task 5d only. Task 7 is the next part.

**What the measurement says (100k lines, quiet machine, the detail statement only, best of 3):**

| | run 5 three accounts, year | run 3 busy account, month | run 2 all accounts, year |
| --- | --- | --- | --- |
| rows before the cut | 9,316 | 604 | 18,570 |
| as is | 536 ms | 110 ms | 1,061 ms |
| no lookup of each row (`wh0 wh1 wh2`) | 267 ms | 88 ms | 504 ms |
| nothing hoisted, no windows | 132 ms | 57 ms | 256 ms |
| prepare only | 60 ms | 59 ms | 59 ms |

The lookups of each row are half of the statement in runs 5 and 2. This task removes that half (all but 201 rows of it). It does not remove the rest: the keyed layers, the running balance, the rows and the sort (run 5: about 270 ms of the statement stay). Alone it does not bring run 5 under the bar at 500,000 lines; the run of step 7 says how far it goes.

## Global Constraints

- The bar is never lowered: runs 3, 4 and 5 of `TestGLScale` (the runs with an Accounts filter) take 1 s or less at 500,000 lines, median of 5. No task changes a bar, removes a run, or changes the scale book to pass.
- Every report's rows stay the same. A change to the engine's SQL has a switch in `legacySQL` (`core/server/reports/legacy.go`), one test that proves the switch writes the old SQL, and a cross-check that runs a report with the switch on and off and compares the two results byte for byte (`requireSameAsLegacy`).
- Core names no package: core tests use the fictional zoo only (`zoo.ledger_lines`); no Books slug, collection or route in `tinycld/core`.
- Each change to SQL is recorded with its plan and its time before and after in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)", under "Changes".
- The 100k scale run takes more than 4 minutes with its build: an implementer does not start it. It is a `NEEDS-RUN` for the driver, on a quiet machine.
- The whole core `reports` package takes about 105 to 130 s. Run it once, at the end of the task.
- Go code is formatted with `gofmt`. Server logging goes through the package's `log`.
- Change files with the Edit tool only (a hook blocks scripted edits).

## Review Focus

1. A sort key that reads a row's own lookup (a viewer sorts by the Split column; a spec sorts by a lookup). The lookup must be computed for every row, or the page holds the wrong rows. Expected: its layer stays below the cut, and the pages are the same. Tests: `TestCellsComputedForThePageKeepTheResult`, cases "a viewer sort" and "a sort key over a lookup"; `TestALookupASortKeyReadsStaysBelowThePageCut`.
2. A cell that adds a running value to a row's own lookup, a running value over a lookup, and a percent of a lookup. The running value reads every row, the lookup it reads too. Expected: the running value is computed below the cut over every row and goes on across pages. Test: `TestCellsComputedForThePageKeepTheResult`, case "a running value over a lookup".
3. A page of a report with sections reached through a cursor, and a report with no groups (keyset alone). Expected: each page the same as before, rows in the same order (the select above the cut orders again). Tests: cases "pages" and "no groups" of the same test, which follow the cursor to the last page.
4. A hoisted name that is the prefix of another (`w.wh1`, `w.wh10`). Expected: `w.wh10` is not a read of `w.wh1`. Test: `TestALookupASortKeyReadsStaysBelowThePageCut`, its last check.
5. A value read below the cut (a lookup's where, probed on each row) that reads a running value. Below the cut a running value has no name. Expected: the statement is written whole, as before. Test: `TestAValueBelowTheCutThatReadsARunningValueKeepsTheWholeSelect`. Not covered by a run: no zoo spec has such a where; the unit test pins the rule on `pagedCells`.
6. A report with a running value and no lookup of its own. Expected: the old statement, text for text. Test: `TestADetailSelectWithNoLookupOfItsOwnIsWrittenAsBefore`.

Not covered by a cross-check (the plan review found them; the planner reached its limit before it could build them): (a) groups with child rows (`nest`) or a layout, which page through `groupedAfter` and not through `pagedGroups` as `balanceSpec` does; (b) a lookup whose where reads a running-value column and that only a cell shows (a late layer that reads `w.ww<n>`). The next planner adds one case for each to the table of `TestCellsComputedForThePageKeepTheResult`, built once in the overlay, before this part runs.

---

### Task 5d: A detail row's own lookups are computed for the page's rows (core)

**Why:** `parts.md`: the final detail select computes the lookups of each row (`wh0`: a name; `wh1`, `wh2`: the Split) for all 9,316 rows of run 5 to return 201. Without them the statement takes 267 ms in place of 536 ms.

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go` (the struct `legacySQL`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/detail.go` (imports, the struct `windowFrame`, `(*windowFrame).from`, new functions before `lookupOf`, `windowSQL`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rows.go` (the struct `compiler`, after `detailSortMixed`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go` (`detailSelect`, `detailPass`, two new declarations after `detailSelect`)
- Create (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/pagedcells_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (step 8)

**Interfaces:**
- Consumes (all in the tree; read each definition before use): `windowFrame`, `hoistLayer`, `(*windowFrame).hoist`, `(*windowFrame).hoistLayer`, `(*recordJoins).any`, `(*recordJoins).from` (`record.go`), `windowSQL`, `(*compiler).detailSelect`, `(*runner).detailPass`, `orderBy` (`run.go`), `legacySQL`. Test helpers: `newZoo(t)`, `withBalanceZoo(t, z)`, `balanceSpec()`, `glJuly`, `glCount`, `glNet` (`balance_shape_test.go`), `observeReads(t) *[]readStatement` (`statement_test.go`), `requireSameAsLegacy(t, z, user, spec, opts, params, off *bool)` (`aggonce_test.go`), `z.run`, `ViewSort{Col, Dir}`.
- Produces: `legacySQL.cellsBeforeThePage bool`; `windowFrame.paged bool`, `windowFrame.windows []string`; `(f *windowFrame) rows(table, alias string) string`; `(f *windowFrame) layers(from, base, alias string, pick func(int) bool) string`; `(f *windowFrame) window(sql string) string`; `(f *windowFrame) inline(sql string) string`; `(f *windowFrame) earlyLayers(texts []string) []bool`; `readsHoisted(sql, name string) bool`; `var windowRef *regexp.Regexp`; `compiler.detailCells func(page, by string) string`; `type earlyCol struct{ at int; name string }`; `pagedCells(frame *windowFrame, table string, sel []string, early []earlyCol) (rows string, cells func(page, by string) string)`. `detailSelect` keeps its signature: its first result is the select the page is cut from. Test helpers: `detailStatement(t, seen []readStatement) string`, `requirePagesSameAsLegacy(t, z, spec, opts, params) int`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/pagedcells_test.go`:

```go
package reports

import (
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
// they are computed over the rows the page cut left: the select that cuts
// the page (it holds the running balance, ww0) is read inside the select
// that computes them.
func TestARowsOwnLookupsAreComputedForThePagesRows(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	seen := observeReads(t)
	z.run(t, "ann", balanceSpec(), Options{Today: "2026-07-15", Limit: 3}, map[string]string{"period": glJuly})
	sql := detailStatement(t, *seen)
	cut := strings.Index(sql, "d.__id LIMIT ")
	window := strings.Index(sql, " AS ww0")
	if window < 0 || window > cut {
		t.Fatalf("the running balance is not computed below the page cut:\n%s", sql)
	}
	from := strings.LastIndex(sql[:cut], "FROM (SELECT * FROM (")
	if from < 0 {
		t.Fatalf("the page is not cut inside another select:\n%s", sql)
	}
	if !strings.Contains(sql[:from], splitRead) {
		t.Fatalf("the Split lookups are not computed over the page:\n%s", sql)
	}
	if strings.Contains(sql[from:cut], splitRead) {
		t.Fatalf("a Split lookup is computed below the page cut:\n%s", sql)
	}
	if !strings.Contains(sql[from:cut], " AS wh") {
		t.Fatalf("the layers the running balance reads are not below the page cut:\n%s", sql)
	}
	if !strings.Contains(sql[cut:], ") d ORDER BY d.__g, ") {
		t.Fatalf("the page's rows are not put in order again:\n%s", sql)
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := requirePagesSameAsLegacy(t, z, c.spec, c.opts, c.params); got < c.pages {
				t.Fatalf("%d pages, want at least %d", got, c.pages)
			}
		})
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
```

- [ ] **Step 2: Run the tests to see them fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:
`go test -run 'TestARowsOwnLookups|TestCellsBeforeThePage|TestCellsComputedForThePage|TestALookupASortKeyReads|TestAValueBelowTheCut' -count=1 ./reports/`
Expected: `FAIL	tinycld.org/core/reports [build failed]`, with `legacySQL.cellsBeforeThePage undefined`, `unknown field paged in struct literal of type windowFrame`, `f.earlyLayers undefined`, `f.window undefined`, `undefined: pagedCells`, `undefined: earlyCol`.

A build failure is not a behavioral red: step 3b runs the red.

- [ ] **Step 3: Add the switch, and the frame's two halves**

`core/server/reports/legacy.go`, in the struct `legacySQL`, after `rangeLinkBounds bool`:

```go
	// cellsBeforeThePage computes every cell of a detail row in the select
	// the page is cut from, as before a row's own lookups moved above the cut
	// (pagedCells).
	cellsBeforeThePage bool
```

`core/server/reports/detail.go`:

Imports: add `"strconv"` between `"slices"` and `"strings"`.

In the struct `windowFrame`, after the field `lookups map[string]detailLookup`:

```go
	// paged is a frame whose select is cut to a page before its cells are
	// computed (pagedCells): a running value or a percent reads every row, so
	// it is kept in windows and read as w.ww<n>.
	paged   bool
	windows []string
```

Replace the head of `(*windowFrame).from`, from its `func` line down to (not including) the line `if len(h.keys) == 0 {`. The doc comment above `from` stays. Old:

```go
func (f *windowFrame) from(table, alias string) string {
	base := table + " " + alias
	from := base
	if len(f.hoisted) == 0 && table == alias && !f.recs.any() {
		from = table
	}
	// The records first: a hoisted value may read one.
	if f.recs.any() {
		from = f.recs.from(from)
	}
	for i, h := range f.hoisted {
```

New:

```go
func (f *windowFrame) from(table, alias string) string {
	return f.layers(f.rows(table, alias), table+" "+alias, alias, nil)
}

// rows is table read as alias, with the group records the frame joins.
func (f *windowFrame) rows(table, alias string) string {
	from := table + " " + alias
	if len(f.hoisted) == 0 && table == alias && !f.recs.any() {
		from = table
	}
	// The records first: a hoisted value may read one.
	if f.recs.any() {
		from = f.recs.from(from)
	}
	return from
}

// layers adds the hoisted layers that pick keeps (every layer when pick is
// nil) to from, in the order they were hoisted. base is the rows a keyed
// layer groups.
func (f *windowFrame) layers(from, base, alias string, pick func(int) bool) string {
	for i, h := range f.hoisted {
		if pick != nil && !pick(i) {
			continue
		}
```

The rest of the loop (the unkeyed `Sprintf`, the `wg%d` join that reads `base`, `return from`) is unchanged: `from`, `base`, `alias` and `i` keep their names, so `wg<i>` keeps the layer's own number when a layer is left out.

Before the comment `// lookupOf is the lookup compiled on the frame's rows under key, if any.` add:

```go
// window keeps a running value or a percent of a paged frame to compute over
// every row, below the page cut, and returns its reference; any other frame
// computes it in place.
func (f *windowFrame) window(sql string) string {
	if !f.paged {
		return sql
	}
	i := slices.Index(f.windows, sql)
	if i < 0 {
		i = len(f.windows)
		f.windows = append(f.windows, sql)
	}
	return fmt.Sprintf("w.ww%d", i)
}

// windowRef is a reference to a paged frame's running value or percent.
var windowRef = regexp.MustCompile(`\bw\.ww(\d+)\b`)

// inline writes each running value or percent back in the place that reads
// it: the form of a select that is not cut before its cells.
func (f *windowFrame) inline(sql string) string {
	return windowRef.ReplaceAllStringFunc(sql, func(ref string) string {
		n, _ := strconv.Atoi(strings.TrimPrefix(ref, "w.ww"))
		return f.windows[n]
	})
}

// readsHoisted reports whether sql reads the frame's hoisted value name on a
// row (w.wh12 does not read w.wh1).
func readsHoisted(sql, name string) bool {
	ref := "w." + name
	for at := 0; ; {
		i := strings.Index(sql[at:], ref)
		if i < 0 {
			return false
		}
		start, end := at+i, at+i+len(ref)
		before := start == 0 || !isNamePart(sql[start-1])
		after := end == len(sql) || !isNamePart(sql[end])
		if before && after {
			return true
		}
		at = end
	}
}

func isNamePart(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// earlyLayers marks the hoisted layers a paged select computes before its
// page is cut: a keyed layer (it reads every row of its group), a layer that
// one of texts reads (a sort key, a running value, a probe mark), and a layer
// that an early layer reads. The other layers are computed for the page's
// rows only.
func (f *windowFrame) earlyLayers(texts []string) []bool {
	early := make([]bool, len(f.hoisted))
	read := slices.Clone(texts)
	for i, h := range f.hoisted {
		if len(h.keys) > 0 {
			early[i] = true
			read = append(read, h.cols...)
		}
	}
	for changed := true; changed; {
		changed = false
		for i, h := range f.hoisted {
			if early[i] || !slices.ContainsFunc(h.names, func(name string) bool {
				return slices.ContainsFunc(read, func(sql string) bool { return readsHoisted(sql, name) })
			}) {
				continue
			}
			early[i], changed = true, true
			read = append(read, h.cols...)
		}
	}
	return early
}
```

In `windowSQL`, wrap the SQL of the three window values in `f.window(…)` (the `percentOfSection` placeholder `NULL` stays as it is):

```go
	case "runningSum":
		over := "OVER (" + partitionBy(f.parent) + " ORDER BY " + strings.Join(f.order, ", ") + " ROWS UNBOUNDED PRECEDING)"
		return withCurrency(expr{sql: f.window("SUM(" + num(e) + ") " + over), typ: sumType([]expr{e}), win: true}, e.cur), nil
	case "percentOfParent":
		return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.parent), scale)), typ: Percent, win: true}, nil
	}
	return expr{sql: f.window(fmt.Sprintf("tc_pct(%s, SUM(%s) OVER (%s), %d)", num(e), num(e), partitionBy(f.total), scale)), typ: Percent, win: true}, nil
```

Only the detail frame of `detailSelect` sets `paged` (step 4); a level's frame and a checking frame keep `paged` false, so `window` returns their SQL unchanged.

- [ ] **Step 3b: Run the behavioral red**

So that the tests build before `detailSelect` changes, add to `run.go` now, after the function `detailSelect`, the type `earlyCol` and the function `pagedCells` exactly as step 4 (d) gives them (the two declarations after the closing brace of `detailSelect`; they call only what step 3 added). Leave `detailSelect` and `detailPass` unchanged.

Run, in `core/server`: `go test -run 'TestADetailSelectWithNoLookup|TestARowsOwnLookups|TestCellsBeforeThePage|TestCellsComputedForThePage|TestALookupASortKeyReads|TestAValueBelowTheCut' -count=1 ./reports/`
Expected: `--- FAIL: TestARowsOwnLookupsAreComputedForThePagesRows` with "the running balance is not computed below the page cut" (no frame is paged yet). The other five pass: nothing writes another statement yet.

- [ ] **Step 4: Write the detail statement as two selects**

`core/server/reports/rows.go`, in the struct `compiler`, after the field `detailSortMixed []condProbe`:

```go
	// detailCells writes the statement of a page cut from the detail select
	// last built, when that select leaves cells to compute for the page's
	// rows only (pagedCells); nil when it holds every cell.
	detailCells func(page, by string) string
```

`core/server/reports/run.go`, in `detailSelect`:

(a) In the `windowFrame` literal, after `recs:   newRecordJoins(r.s, "w"),` add:

```go
		paged:  !legacySQL.cellsBeforeThePage,
```

(b) After the line `sel := []string{"w.__id AS __id"}` add:

```go
	// early names the values of sel that are read before the page is cut:
	// the sort keys and the probe marks.
	var early []earlyCol
	before := func(sql, name string) {
		early = append(early, earlyCol{at: len(sel), name: name})
		sel = append(sel, sql+" AS "+name)
	}
```

(c) In `addKey`, each of the four appends of a sort key value to `sel` goes through `before`:

| old | new |
| --- | --- |
| `sel = append(sel, key+" AS "+name)` (twice: the `sortLabel` branch and the `shownOrder` branch) | `before(key, name)` |
| `sel = append(sel, e.sql+" AS "+name)` | `before(e.sql, name)` |
| `sel = append(sel, fmt.Sprintf("(CASE WHEN %s THEN 1 ELSE 0 END) AS %s", m, mx))` | `before(fmt.Sprintf("(CASE WHEN %s THEN 1 ELSE 0 END)", m), mx)` |

(d) The end of the function. Old:

```go
		sel = append(sel, fmt.Sprintf("(CASE WHEN %s THEN 1 ELSE 0 END) AS %s", w.sql, mx))
		c.detailSortMixed = append(c.detailSortMixed, condProbe{at: w.at, sql: mx})
	}
	return "SELECT " + strings.Join(sel, ", ") + " FROM " + frame.from(table, "w"), order, nil
}
```

New:

```go
		before(fmt.Sprintf("(CASE WHEN %s THEN 1 ELSE 0 END)", w.sql), mx)
		c.detailSortMixed = append(c.detailSortMixed, condProbe{at: w.at, sql: mx})
	}
	rows, cells := pagedCells(frame, table, sel, early)
	c.detailCells = cells
	return rows, order, nil
}

// earlyCol is a value of the detail select that is read before the page is
// cut: its place in the select list and its name.
type earlyCol struct {
	at   int
	name string
}

// pagedCells writes the detail select (sel over table, read as w) so that a
// row's own lookups run for the rows of the page only. rows is the select the
// page is cut from: every row with its sort keys and probe marks (early), its
// running values and percents, and the hoisted layers one of those reads.
// cells writes the statement of a page cut from rows (page, ordered by by):
// the other layers and the cells, over the page's rows.
//
// cells is nil, and rows is the whole select, when no layer would move above
// the cut, or when a value read before the cut reads a running value (a
// lookup's where over one).
func pagedCells(frame *windowFrame, table string, sel []string, early []earlyCol) (rows string, cells func(page, by string) string) {
	whole := frame.inline("SELECT " + strings.Join(sel, ", ") + " FROM " + frame.from(table, "w"))
	if !frame.paged {
		return whole, nil
	}
	texts := slices.Clone(frame.windows)
	for _, e := range early {
		texts = append(texts, sel[e.at])
	}
	isEarly := frame.earlyLayers(texts)
	if !slices.Contains(isEarly, false) {
		return whole, nil
	}
	for i, h := range frame.hoisted {
		if isEarly[i] {
			texts = append(texts, h.cols...)
		}
	}
	if slices.ContainsFunc(texts, windowRef.MatchString) {
		return whole, nil
	}
	first, last := []string{"w.*"}, slices.Clone(sel)
	for _, e := range early {
		first = append(first, sel[e.at])
		last[e.at] = "w." + e.name + " AS " + e.name
	}
	for i, sql := range frame.windows {
		first = append(first, fmt.Sprintf("%s AS ww%d", sql, i))
	}
	rows = "SELECT " + strings.Join(first, ", ") + " FROM " +
		frame.layers(frame.rows(table, "w"), table+" w", "w", func(i int) bool { return isEarly[i] })
	return rows, func(page, by string) string {
		cols := last
		if strings.HasPrefix(by, "d.__g") {
			cols = append(slices.Clone(last), "w.__g AS __g")
		}
		from := frame.layers("("+page+") w", table+" w", "w", func(i int) bool { return !isEarly[i] })
		return fmt.Sprintf("SELECT * FROM (SELECT %s FROM %s) d ORDER BY %s", strings.Join(cols, ", "), from, by)
	}
}
```

How it reads: the select below the cut is `SELECT w.*, <sort keys>, <marks>, <running values AS ww<n>> FROM <the early layers>`. `w.*` carries the row's columns, its group records and the early layers' values up through the cut, so the cells above read them under the same names (`w.k0`, `w.rc0`, `w.wh3`), and a cell reads a running value as `w.ww<n>`. Every reader of the detail select before the cut (`detailPass`'s page, the mixed probes, `readShownNow`) reads only `__id`, the group keys, the sort keys and the marks, which that select holds. A frame with no window and no late layer writes the same text as before (`inline` changes nothing).

The other callers of `detailSelect` (three more in `run.go`, one in `relation_test.go`) need no change: the signature is the same.

In `detailPass`, between the line that builds `body` and the line `rows, err := rn.query(s, ctes, body)`, add the comment and the three lines of the `if`:

```go
	body := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY %s LIMIT %s", from, where, by, limitSQL(int64(limit+1)))
	// The cells that nothing below the cut reads are computed for the page.
	if c.detailCells != nil {
		body = c.detailCells(body, by)
	}
	rows, err := rn.query(s, ctes, body)
```

Update the doc comment of `detailPass`: replace the sentence (it spans two comment lines) "Window values are computed before the page is cut, so a running value goes on across pages." with "Window values, sort keys and what they read are computed before the page is cut, so a running value goes on across pages; a row's other lookups and its cells are computed for the page's rows (pagedCells)."

- [ ] **Step 5: Run the tests to see them pass**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:
`go vet ./reports/ && go test -run 'TestADetailSelectWithNoLookup|TestARowsOwnLookups|TestCellsBeforeThePage|TestCellsComputedForThePage|TestALookupASortKeyReads|TestAValueBelowTheCut' -count=1 ./reports/`
Expected: `ok  	tinycld.org/core/reports` (about 7 s; the planner's run of this code: 7.3 s).

When `TestCellsComputedForThePageKeepTheResult` fails with "the result changed", do not change the test: the two results print in full. A cell that differs is a value that must be below the cut and is not in `texts` (`pagedCells`); a row order that differs is the outer `ORDER BY`.

- [ ] **Step 6: Run the whole package and the Books ledger tests, and commit core**

Run once, in `core/server`: `gofmt -l ./reports/ && go test -count=1 ./reports/`
Expected: `gofmt` prints nothing; `ok  	tinycld.org/core/reports` in about 105 to 130 s (the planner's run of this code: 130.8 s, no failure; no existing test changed).

Run, in `/Users/nas/tmp/tinycld/books-purchases/books/server`: `go test -run 'GeneralLedger|Ledger|Guard' -count=1 .`
Expected: `ok  	tinycld.org/packages/books` (the planner's run: 13.9 s).

Commit only when your prompt permits commits; if it does not, name the five files in your report and the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/legacy.go core/server/reports/detail.go core/server/reports/rows.go core/server/reports/run.go core/server/reports/pagedcells_test.go
git commit -m "perf(reports): a detail row's own lookups run for the page's rows"
```

- [ ] **Step 7: Hand the 100k run to the driver**

Do not start the run. Hand back:

`NEEDS-RUN (quiet machine: no chrome-headless-shell process, 1-minute load under 4): mkdir -p /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5d && cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5d go test -run 'TestGLScale$/100k$' -timeout 20m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5d.log 2>&1`

What is needed from it: `scale-task5d/gl-results.md` and `scale-task5d/gl-plans.md`. The implementer's work ends here; step 8 is done by the agent that receives the run's files.

Expected (from `part6-scratch/100k/parts.md`, the line `no wh0 wh1 wh2`, plus 201 rows of lookups): the detail statement of run 5 from about 540 ms to about 270 to 300 ms, of run 2 from about 1,060 ms to about 500 to 600 ms, of run 3 from about 110 ms to about 90 ms; the medians of runs 5 and 2 lower by about that much; runs 1 and 4 within the run's variation (about 25%). The reads of `books_journal_lines` in the plan keep their forms (`entry=?` and `(entry>? AND entry<?)` for the Split lookups; the count of plan lines may change, because the statement has one more select).

When a run with an Accounts filter (3, 4, 5) is more than 25% above its record of task 5 (178, 140, 713 ms), hand back a second `NEEDS-RUN` (output to `scale-task5d-b`); only when both are above, hand back `STOPPED: the two selects slow run <n>` with the medians and that run's statement times. When the detail statement of run 5 is not lower by 150 ms or more, hand back `STOPPED: the lookups still run for every row` with the plan lines of run 5's detail statement. Do not change the rule to pass.

- [ ] **Step 8: Record the change in the Books ledger, and commit Books (the agent that receives the run)**

Read `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` from the heading `#### Task 5: a lookup on a known link searches the link and the day` to the end of the section, and append after the record of task 5, in the same form (read from the run's files, never typed from this plan):

```markdown
#### Task 5d: a detail row's own lookups run for the page's rows (core `server/reports/run.go`, <date>)

The detail statement computed every cell of every row of the period and cut the page last, so the Split and name lookups ran for each row of the period to show 200. The statement is now two selects: below the page cut, each row with its sort keys, its running balance and the beginning balance the running balance reads; above it, the row's own lookups and the cells, for the page's rows.

| run, 100k lines | before | after |
| --- | --- | --- |
| 1 all accounts, month | <task 5 after> | <median> |
| 2 all accounts, year | <task 5 after> | <median> |
| 3 busy account, month | <task 5 after> | <median> |
| 4 small account, month | <task 5 after> | <median> |
| 5 three accounts, year | <task 5 after> | <median> |

The detail statement: run 5 <time before> to <time after>, run 3 <time before> to <time after>, run 2 <time before> to <time after>. Plan, the detail statement of run 5, reads of `books_journal_lines`: <the plan lines and their counts, before and after>. What stays in the statement (measured before the change, `parts.md`): the rows, their records and the sort, the beginning balance for each account, and the running balance.
```

Where each `<…>` comes from:
- `<task 5 after>`: the "after" column of the task 5 record in the ledger.
- `<median>`: `scale-task5d/gl-results.md`, the row of that run, the column `median of 5`, in ms.
- The plan lines and the statement times: `scale-task5d/gl-plans.md` (after) and `scale-task5/gl-plans.md` (before), under `## 100k lines, run <n> …`, the statement with the most rows (`### statement <n>: <time>, <rows> rows, <chars> chars`). List them with `command grep -n -E '^## 100k lines, run (2|3|5)|^### statement|books_journal_lines' <file>`.

Commit only when your prompt permits commits; if it does not, the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs(books): record the paged cells of the detail statement at 100k lines"
```

---

## What is left (the next plan part)

- **7. books · Scale run and record.** The 500k `NEEDS-RUN`, the plan pins in `TestGLScaleBook`, the ledger record; `STOPPED` when a filtered run is over 1 s.
- After 5d the detail statement of run 5 still holds, by `parts.md`: about 130 ms for the rows, their records, the sort and the page; about 60 ms to prepare a statement of 43,000 chars (the same at any size, in each of the run's three long statements); the keyed layers (the beginning balance: `no wh3..wh5` is 60 to 75 ms lower); and the running balance. A cut for each is not planned here; the planner of task 7 reads the 5d run first.
