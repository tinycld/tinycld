# Report Engine Scale, Part 27: A Page's Detail Rows Find Their Open Group Through an Index (task 9b-3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The detail statement of a grouped report run reads its detail rows once and finds each row's open group through an index on the open groups, whatever the group key is, so that a run with many rows and a link group key (Transaction Detail with no Accounts filter) no longer reads every row once for each open group.

**Architecture:** The core report engine joins a page's detail rows (`d`) to the open innermost groups (`gord`, a materialized `json_each` over one bound list) to get each row's place in document order. Two changes to the one join, in one new function `leafFrom`: `CROSS JOIN` (SQLite keeps the written order of a CROSS JOIN, so the rows are the outer loop), and a unary `+` before each key on the row's side (it takes the column affinity off the term, and only a term with no affinity can use the automatic index SQLite builds on `gord`). A test switch `legacySQL.plannedLeafJoin` writes the join as before, so tests compare the two results.

**Tech Stack:** Go; core report engine (`tinycld.org/core/reports`, SQLite through `modernc.org/sqlite` v1.57.0); Books Go tests (`tinycld.org/packages/books`) for one plan test and the scale measures (run by the driver only).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`, section "State after the part 27 planner", and the hand-off note it names, `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part27.md`. Bar (human, 2026-11-20): Transaction Detail run "TD 2 all accounts, year" 2 s at 500k lines; its median is 1.691 s at 100k (`scale-task9d2b/st-results.md`), of which statement 6 (the page) is 1.594 s (`scale-task9d2b/st-plans.md`).

## The evidence this plan is written from

All in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part27-scratch/`. Plans only; no time of the change was measured.

1. `plan100k.md` (the tree as is, the 100k book, full `EXPLAIN QUERY PLAN`): TD 2's statement 6 has `SCAN gord` then `SEARCH t1 USING AUTOMATIC PARTIAL COVERING INDEX (currency=?)`. `t1` is the union of the two line tables with the day bound. The index holds the currency only (one value), so each of the 105 open sections reads every row of the period. This is the plan of the small book too.
2. The same file, General Ledger run 2 (all accounts, year): `SCAN gord` then `SEARCH d USING AUTOMATIC PARTIAL COVERING INDEX (k0=? AND ck0=?)`. The index holds the section key: the detail rows are read once, to build it. **General Ledger run 2 does not have the sections x period rows join.** Its 2.733 s at 500k is elsewhere. The change still alters its statement (the index of all detail rows is no longer built), so its measure is run again.
3. Why SQLite builds no index on `gord` for some keys (the cause of the first design's failure, hand-off note "First design, REFUTED"): a term `d.note IS gord.g0` compares with the TEXT affinity of the column `note`; an index on `gord.g0` (a `json_extract` value, no affinity) cannot serve a comparison that has an affinity. A key that is an expression (`NULLIF(t2.id, '')`) has no affinity, so that term could use the index. With `+d.k0 IS gord.g0` every term can. Shown on the 8,000-open-groups run of `TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce`: `CROSS JOIN` alone 3.78 s (`SCAN zoo_feedings`, `SCAN gord`); `CROSS JOIN` with the `+` 0.19 s (`SCAN zoo_feedings`, `BLOOM FILTER ON gord (g0=? AND g1=?)`, `SEARCH gord USING AUTOMATIC PARTIAL COVERING INDEX (g0=? AND g1=?)`); the plain JOIN 0.21 s.
4. `plan100k-after.md` (the change, the 100k book): TD 2 `SCAN t1`, `BLOOM FILTER ON gord (g0=? AND g1=?)`, `SEARCH gord USING AUTOMATIC PARTIAL COVERING INDEX (g0=? AND g1=?)`; General Ledger run 2 the same with `SCAN d`.
5. The planner's build: `core/run.go`, `core/legacy.go`, `core/leaforder_test.go` (each file after the change), `books/` (the two Books scale test files after the change), `books_td2_test.go` (the new Books test), overlays `overlay-red.json` / `overlay-green.json` (core) and `overlay-books-red2.json` / `overlay-books-green2.json` (Books). `pkg3.log`: the whole core `reports` package with the change and the new tests (`ok`, 133 s). `books-pkg.log`: the whole Books Go package with the core change and the first form of the Books test (`ok`, 411 s); the final Books test files ran with the named tests only (`TestTransactionDetailReadsItsPeriodRowsOnce`, `TestStatementsScaleBook`, `TestStatementsScaleBars`, `TestGLScaleBook`, `TestGLScaleBars`: `ok`).

## Global Constraints

- Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Core: `/Users/nas/tmp/tinycld/books-purchases/tinycld`; Books: `/Users/nas/tmp/tinycld/books-purchases/books`; both on branch `books/p5-report-engine-scale`. Written against core `39fb32b8`, books `1001e3ae`. Every line number in this plan is of a file before this plan's edits.
- This is a core task: the implementer edits core `core/server/reports/` and Books `server/` and `docs/plans/`. Books builds against the core tree of the workspace, so the Books tests see the core change with no publish step.
- Core names no package: no Books name (`books`, `journal`, `account`, a collection or a report name) in core code, core tests or core comments. The core tests use the zoo fixture only.
- The result of every report run must not change: the same rows, in the same order, on the same pages. `requireSameAsLegacy` and the page comparison prove it; a difference is a defect of the change. Do not change an expected value of an existing test.
- Never raise a time limit to pass (`TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce`, limit 2 s for each statement, and the new union test with the same limit). A failure there is the quadratic read.
- The strings `"JOIN gord ON"` and `"gord AS MATERIALIZED ("` stay in the SQL: `leaforder_test.go:51`, `:55` and `export_onepage_test.go:22` find the statement by them. `CROSS JOIN gord ON` still holds `JOIN gord ON`.
- No migration, no client change, no new index.
- The implementer starts no scale run and takes no timing: the 100k runs are a `NEEDS-RUN` for the driver (step 11). A timing is valid only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the 1-minute load is under 4.
- Run the whole core `reports` package once (step 7) and the whole Books Go package once (step 8). Other steps run named tests.
- Do not edit `books/docs` before step 12.
- Steps 9, 10 and 13 commit. When the implementer's prompt does not allow commits, hand the file list of the step to the driver in place of the commit. Never push.

## Review Focus

- A group key of each stored kind must still find its group now that the comparison has no column affinity: a text column, a whole number, a number with a fraction (2.5), a month of a date, a bool read through a link. Pinned by `TestTheOpenGroupJoinMatchesAKeyOfEveryKind` and `TestTheOpenGroupJoinOrderKeepsTheResult`.
- A blank key (a row whose note is blank; a row whose linked record the reader cannot read) must still fall under the blank group, and one key in two currencies must stay two groups. Pinned by `TestTheOpenGroupJoinOrderKeepsTheResult` (four readers, `addOddTreats`).
- A run cut into pages (a cursor inside a group and after a group) must give the same pages. Pinned by `TestTheOpenGroupJoinOrderKeepsEveryPage`.
- Many open groups: SQLite's plan of this join changes with the number of rows, so a plan test on a small book does not prove the large one. Pinned on a collection by the existing `TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce` (8,000 open groups, 2 s) and on a union by `TestAGroupedRunOfAUnionWithManyOpenGroupsReadsItsRowsOnce` (a guard: its key is a field, and it passes before the change too). No core test has many open groups on a link key (the zoo's bulk rows are of one animal): that case is held by the small plan tests, and at 100k Books lines by the planner's `plan100k-after.md` and by the plan lines the 100k run of step 11 writes.
- Accepted, not pinned: the join now depends on SQLite's automatic indexes. With `PRAGMA automatic_index = OFF` every row would be compared with every open group. Checked: no Go file of core `core/server`, of the app's `server` or of Books `server` names `automatic_index` (`command grep -rlniE 'automatic_index' --include='*.go'`, no match). The reviewer checks that no such pragma is added.
- Accepted, not pinned: SQLite may read a very small `gord` with no index (a scan for each row) when its cost estimate says so. The work is then rows x a few groups; the two 8,000-group tests hold the case that matters.

---

### Task 9b-3: A page's detail rows find their open group through an index

**Files:**
- Modify: `tinycld/core/server/reports/run.go` (lines 1538, 1546, and `leafJoin` at 1778-1789)
- Modify: `tinycld/core/server/reports/legacy.go` (the last field of `legacySQL`, lines 27-30)
- Modify (test): `tinycld/core/server/reports/leaforder_test.go` (imports, lines 3-8; new code after line 64, the end of the file)
- Modify (test): `books/server/report_general_ledger_scale_test.go` (`TestGLScale` line 351; a new helper before `writeGLScaleOut`, line 364)
- Modify (test): `books/server/report_statements_scale_test.go` (`TestStatementsScale` line 356; a new test at the end of the file)
- Modify: `books/docs/plans/general-ledger-ledger.md` (a new record at the end, after the record of task 9d-2b, line 773)

**Interfaces:**
- Consumes (exist, do not change): core test helpers `newZoo(t)`, `newUnionZoo(t)` (source `zoo.meals`: the feedings and the treats), `bulkFeedings(t, z, n)`, `observeReads(t) *[]readStatement`, `explainPlan(t, z, st) []string`, `requireSameAsLegacy(t, z, user, spec, opts, params, off *bool)`, `groupedPages`, `mergedRows`, `withLimits`, `z.run`, `z.try`, `z.rec`, `z.feeding`, the specs `byNoteClosed` and `zooStatement`, and `mealDays`. Books test helpers `writeStatementsScaleBook(t, arSmall) glScaleBook`, `stScaleRuns(b) []stScaleRun`, `stScaleStatements(t, b, run) []string`, `explainStatements(t, app, statements) []planStep` (`planStep` has `Statement int` and `Detail string`).
- Produces: core `func (c *compiler) leafFrom(sel string) string`, `func (c *compiler) leafJoin(row string) string` (was `leafJoin()`), the test switch `legacySQL.plannedLeafJoin bool`. Books test helper `scaleOpenGroupStep(detail string) bool`. No later task consumes them.

- [ ] **Step 1: Add the test switch (core)**

In `tinycld/core/server/reports/legacy.go`, add the field after `noPartBounds bool` (line 29), before the closing brace:

```go
	// noPartBounds reads a union source through its rule CTE with no bound
	// inside its parts, as before partBounds wrote them.
	noPartBounds bool
	// plannedLeafJoin joins a detail row to its open group with a plain JOIN
	// on the bare keys, whose order SQLite's planner picks, as before leafFrom
	// fixed the order with CROSS JOIN and took the keys' affinity off.
	plannedLeafJoin bool
}
```

Nothing reads the field yet; it is here so that the tests of step 2 compile.

- [ ] **Step 2: Write the failing core tests**

In `tinycld/core/server/reports/leaforder_test.go`, add `"encoding/json"` to the imports:

```go
import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)
```

Append at the end of the file (after `TestTheDetailStatementComputesItsOpenGroupsOnce`):

```go
// mealsByAnimal is the union's meals of a period, one open section an animal:
// a day bound alone, no link bound.
const mealsByAnimal = `{"version":4,"source":"zoo.meals","params":[{"id":"days","type":"dateRange","label":"Days","default":{"preset":"allDates"}}],
	"columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"days"}]},
	"groups":[{"id":"g","col":"animal","subtotals":true}],"sort":[{"col":"animal"}]}`

// mealsByNote groups the union's meals by their note: a note may be blank, and
// one note may hold meals of two currencies (two groups).
const mealsByNote = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"note","subtotals":true}],"sort":[{"col":"note"}]}`

// addOddTreats gives ann a treat with a blank note and a treat in a second
// currency whose note another treat has.
func addOddTreats(t *testing.T, z *zoo) {
	t.Helper()
	z.rec(t, "t5", "zoo_treats", map[string]any{"pet": z.id("kit"), "price": 5, "currency": "USD", "given": "2026-01-09 00:00:00.000Z", "label": ""})
	z.rec(t, "t6", "zoo_treats", map[string]any{"pet": z.id("kit"), "price": 7, "currency": "EUR", "given": "2026-01-10 00:00:00.000Z", "label": "t1"})
}

// gordPlans is the plan lines of the grouped detail statement of a run that
// name the open groups (gord) or the detail rows' base (t1).
func gordPlans(t *testing.T, z *zoo, seen []readStatement) []string {
	t.Helper()
	var lines []string
	found := 0
	for _, st := range seen {
		if !strings.Contains(st.sql, "JOIN gord ON") {
			continue
		}
		found++
		for _, line := range explainPlan(t, z, st) {
			if strings.Contains(line, "gord") || strings.HasSuffix(line, " t1") || strings.Contains(line, " t1 ") {
				lines = append(lines, line)
			}
		}
	}
	if found != 1 {
		t.Fatalf("%d grouped detail statements, want 1", found)
	}
	return lines
}

// The detail statement reads its rows once and finds each row's open group by
// its keys. Left to SQLite's planner, a base that is a union (a subquery) was
// read once for each open group, through an index SQLite built on the rows'
// currency: rows x open groups. The engine fixes the order with CROSS JOIN.
func TestAGroupedDetailStatementReadsItsRowsOnce(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", mealsByAnimal, Options{ExpandAll: true}, mealDays)
	plan := strings.Join(gordPlans(t, z, *seen), "\n")
	if strings.Contains(plan, "SCAN gord") || strings.Contains(plan, "SEARCH t1") {
		t.Fatalf("the rows are read once for each open group:\n%s", plan)
	}
	if !strings.Contains(plan, "SCAN t1") || !strings.Contains(plan, "SEARCH gord USING AUTOMATIC") {
		t.Fatalf("the rows do not find their group by its keys:\n%s", plan)
	}
	legacySQL.plannedLeafJoin = true
	t.Cleanup(func() { legacySQL.plannedLeafJoin = false })
	*seen = nil
	z.run(t, "ann", mealsByAnimal, Options{ExpandAll: true}, mealDays)
	for _, st := range *seen {
		if strings.Contains(st.sql, "CROSS JOIN gord") || strings.Contains(st.sql, "+d.k0 IS gord.g0") {
			t.Fatalf("with the switch on, the join is not the planner's:\n%s", st.sql)
		}
	}
}

// Every reader gets the same rows whichever side drives the join: a union and
// a collection, paged groups and a layout, a period and all dates, a group
// with a blank key and one note in two currencies.
func TestTheOpenGroupJoinOrderKeepsTheResult(t *testing.T) {
	z := newUnionZoo(t)
	addOddTreats(t, z)
	open := Options{ExpandAll: true}
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		for _, params := range []map[string]string{mealDays, nil} {
			requireSameAsLegacy(t, z, user, mealsByAnimal, open, params, &legacySQL.plannedLeafJoin)
		}
		requireSameAsLegacy(t, z, user, mealsByNote, open, nil, &legacySQL.plannedLeafJoin)
		requireSameAsLegacy(t, z, user, byNoteClosed, open, nil, &legacySQL.plannedLeafJoin)
		requireSameAsLegacy(t, z, user, fmt.Sprintf(zooStatement, ""), open, nil, &legacySQL.plannedLeafJoin)
	}
	// ann reads f1-f4 and t1, t3, t5 (blank note), t6 (EUR): the blank note is
	// a group of its own, and note t1 is two groups, one a currency.
	res := z.run(t, "ann", mealsByNote, open, nil)
	groups, details := 0, 0
	for _, r := range res.Rows {
		switch r.Kind {
		case KindGroup:
			groups++
		case KindDetail:
			details++
		}
	}
	if groups != 8 || details != 8 {
		t.Fatalf("%d groups and %d detail rows, want 8 and 8", groups, details)
	}
}

// A base that is a collection finds each row's group through an index on the
// open groups too. A key that is a stored text column compares with the
// column's affinity, and SQLite builds no index on gord for such a term: the
// rows were then read against every open group. The unary + of leafFrom takes
// the affinity off.
func TestAGroupedDetailStatementOfACollectionFindsItsGroupByAnIndex(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", byNoteClosed, Options{ExpandAll: true}, nil)
	plan := strings.Join(gordPlans(t, z, *seen), "\n")
	if strings.Contains(plan, "SCAN gord") || !strings.Contains(plan, "SEARCH gord USING AUTOMATIC") {
		t.Fatalf("the rows do not find their group by its keys:\n%s", plan)
	}
	for _, key := range []string{"g0=?", "g1=?"} {
		if !strings.Contains(plan, key) {
			t.Fatalf("the index on the open groups has no %s:\n%s", key, plan)
		}
	}
}

// Group keys of each stored kind: a number, a month of a date, a bool read
// through a link.
const (
	byGrams = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"grams","path":[{"field":"grams"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"grams","subtotals":true}],"sort":[{"col":"grams"}]}`
	byFedMonth = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"fed","path":[{"field":"fed"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"fed","bucket":"month","subtotals":true}]}`
	byActive = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"active","path":[{"link":"animal"},{"field":"active"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"active","subtotals":true}]}`
)

// The join compares a row's key with its group's with no column affinity. A
// key of every stored kind still finds its group: each run shows the detail
// rows the planner's join showed, and every feeding the reader may read.
func TestTheOpenGroupJoinMatchesAKeyOfEveryKind(t *testing.T) {
	z := newZoo(t)
	z.feeding(t, "half", z.recs["leo"], 3, "2026-03-05", 0)
	half := z.recs["half"]
	half.Set("grams", 2.5)
	if err := z.app.Save(half); err != nil {
		t.Fatal(err)
	}
	open := Options{ExpandAll: true}
	flat := 0
	for _, r := range z.run(t, "ann", `{"version":4,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}]}]}`, Options{}, nil).Rows {
		if r.Kind == KindDetail {
			flat++
		}
	}
	if flat == 0 {
		t.Fatal("ann reads no feeding")
	}
	for name, spec := range map[string]string{"a number": byGrams, "a month": byFedMonth, "a bool through a link": byActive} {
		t.Run(name, func(t *testing.T) {
			requireSameAsLegacy(t, z, "ann", spec, open, nil, &legacySQL.plannedLeafJoin)
			details := 0
			for _, r := range z.run(t, "ann", spec, open, nil).Rows {
				if r.Kind == KindDetail {
					details++
				}
			}
			if details != flat {
				t.Fatalf("%d detail rows under the open groups, want %d", details, flat)
			}
		})
	}
}

// A union with 8,000 open groups stays under the limit of
// TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce: SQLite's plan of this
// join changes with the number of rows, so the small plan tests above do not
// prove the large one. A failure is a result: never raise the limit to pass.
func TestAGroupedRunOfAUnionWithManyOpenGroupsReadsItsRowsOnce(t *testing.T) {
	z := newUnionZoo(t)
	bulkFeedings(t, z, 8000)
	withLimits(t, func(l *limits) {
		l.timeout = 2 * time.Second
		l.maxPage = 30000
	})
	res, err := z.try(t, "ann", mealsByNote, Options{ExpandAll: true, Limit: 30000}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
		}
	}
	if details != 8006 {
		t.Fatalf("%d detail rows, want 8006", details)
	}
}

// A run cut into pages gives the same pages whichever side drives the join.
func TestTheOpenGroupJoinOrderKeepsEveryPage(t *testing.T) {
	z := newUnionZoo(t)
	addOddTreats(t, z)
	for _, spec := range []string{mealsByNote, mealsByAnimal} {
		opts := Options{ExpandAll: true, Limit: 3}
		after, err := json.Marshal(mergedRows(t, groupedPages(t, z, "ann", spec, opts)))
		if err != nil {
			t.Fatal(err)
		}
		legacySQL.plannedLeafJoin = true
		t.Cleanup(func() { legacySQL.plannedLeafJoin = false })
		before, err := json.Marshal(mergedRows(t, groupedPages(t, z, "ann", spec, opts)))
		legacySQL.plannedLeafJoin = false
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("the pages changed\nbefore: %s\nafter:  %s", before, after)
		}
	}
}
```

- [ ] **Step 3: Write the failing Books test and the plan print**

In `books/server/report_general_ledger_scale_test.go`, add the helper before the comment `// writeGLScaleOut writes gl-results.md and gl-plans.md under BOOKS_SCALE_OUT` (line 364):

```go
// scaleOpenGroupStep is true for a plan step of the join of a page's detail
// rows to their open sections: a step that names the open sections (gord), or
// a read of the detail rows (t1 over a union source, d over computed rows).
func scaleOpenGroupStep(detail string) bool {
	if strings.Contains(detail, "gord") || detail == "SCAN t1" || detail == "SCAN d" {
		return true
	}
	return strings.HasPrefix(detail, "SEARCH t1 ") || strings.HasPrefix(detail, "SEARCH d ")
}
```

In the same file, `TestGLScale` (line 351) also prints those steps to `gl-plans.md`. Replace

```go
						if step.Statement == i && strings.Contains(step.Detail, "books_journal_lines") {
```

with

```go
						if step.Statement == i && (strings.Contains(step.Detail, "books_journal_lines") || scaleOpenGroupStep(step.Detail)) {
```

In `books/server/report_statements_scale_test.go`, `TestStatementsScale` (line 356) prints them to `st-plans.md`. Replace

```go
						if step.Statement == i && (strings.Contains(step.Detail, "books_journal_lines") || strings.Contains(step.Detail, "books_cash_basis_lines") || strings.Contains(step.Detail, "books_account_balances")) {
```

with

```go
						if step.Statement == i && (strings.Contains(step.Detail, "books_journal_lines") || strings.Contains(step.Detail, "books_cash_basis_lines") || strings.Contains(step.Detail, "books_account_balances") || scaleOpenGroupStep(step.Detail)) {
```

Append at the end of `books/server/report_statements_scale_test.go` (the file imports `strings` and `testing` already):

```go
// TestTransactionDetailReadsItsPeriodRowsOnce: the page statement of a run
// with no Accounts filter reads the period's rows once, and each row finds
// its open section through an index on the open sections. Read the other way
// (the open sections first), every section read every row of the period: a
// section's key is the id of a joined account, which no index on the rows
// holds.
func TestTransactionDetailReadsItsPeriodRowsOnce(t *testing.T) {
	b := writeStatementsScaleBook(t, arSmall)
	for _, run := range stScaleRuns(b) {
		if run.name != "TD 2 all accounts, year" {
			continue
		}
		var page []string
		for _, s := range stScaleStatements(t, b, run) {
			if strings.Contains(s, "JOIN gord ON") {
				page = append(page, s)
			}
		}
		if len(page) != 1 {
			t.Fatalf("%d page statements that join the open sections, want 1", len(page))
		}
		var steps []string
		for _, step := range explainStatements(t, b.app, page) {
			if scaleOpenGroupStep(step.Detail) {
				steps = append(steps, step.Detail)
			}
		}
		plan := strings.Join(steps, "\n")
		if strings.Contains(plan, "SCAN gord") || strings.Contains(plan, "SEARCH t1") {
			t.Fatalf("every open section reads the period's rows:\n%s", plan)
		}
		if !strings.Contains(plan, "SCAN t1") || !strings.Contains(plan, "SEARCH gord USING AUTOMATIC") {
			t.Fatalf("the rows do not find their section by its keys:\n%s", plan)
		}
		return
	}
	t.Fatal("the measure has no run TD 2 all accounts, year")
}
```

- [ ] **Step 4: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet ./reports/ && go test -count=1 -run 'TestAGroupedDetailStatement|TestTheOpenGroupJoin|TestAGroupedRunOfAUnionWithManyOpenGroups' ./reports/`

Expected: `go vet` prints nothing. Two tests FAIL, the other four pass (the switch changes nothing yet, and the plain JOIN is fast on a union whose key is a field):

```
--- FAIL: TestAGroupedDetailStatementReadsItsRowsOnce
    leaforder_test.go: the rows are read once for each open group:
--- FAIL: TestAGroupedDetailStatementOfACollectionFindsItsGroupByAnIndex
    leaforder_test.go: the rows do not find their group by its keys:
```

The first prints `SCAN gord` and `SEARCH t1 USING AUTOMATIC PARTIAL COVERING INDEX (currency=?)`; the second prints `SCAN gord`.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -count=1 -run 'TestTransactionDetailReadsItsPeriodRowsOnce$' .`

Expected: `go vet` prints nothing; FAIL:

```
--- FAIL: TestTransactionDetailReadsItsPeriodRowsOnce
    report_statements_scale_test.go: every open section reads the period's rows:
        MATERIALIZE gord
        SCAN gord
        SEARCH t1 USING AUTOMATIC PARTIAL COVERING INDEX (currency=?)
```

- [ ] **Step 5: Write the join (core)**

In `tinycld/core/server/reports/run.go`, replace `leafJoin` (lines 1778-1789) with the two functions:

```go
// leafJoin matches a detail row to its open group by each key value with IS,
// so a blank key matches a blank one, as a group row's key reads. row is what
// stands before a key's name on the row's side: "d." or "+d.".
func (c *compiler) leafJoin(row string) string {
	on := make([]string, 0, len(c.p.groups)+len(c.currencies))
	for i := range c.p.groups {
		on = append(on, fmt.Sprintf("%sk%d IS gord.g%d", row, i, i))
	}
	for j, ck := range c.currencies {
		on = append(on, fmt.Sprintf("%s%s IS gord.g%d", row, ck, len(c.p.groups)+j))
	}
	return strings.Join(on, " AND ")
}

// leafFrom is the detail rows (sel) with each row's open group: __g, the
// group's place in document order. The rows are read once, and each finds its
// group through the index SQLite builds on gord (an automatic index):
//   - CROSS JOIN fixes the join order in SQLite. Left to the planner, the
//     open groups were read first, and a key that is not a stored column of
//     the rows (a link's id, read from the joined row) is in no index of the
//     rows: every open group read every row.
//   - The unary + takes the column affinity off a key. A term that compares
//     with a column's affinity cannot use an index on gord, whose values (out
//     of json_extract) have none. A key and its group's value have the same
//     storage class, so the comparison gives the same answer without it.
func (c *compiler) leafFrom(sel string) string {
	join, row := "CROSS JOIN", "+d."
	if legacySQL.plannedLeafJoin {
		join, row = "JOIN", "d."
	}
	return fmt.Sprintf("(SELECT d.*, gord.__g AS __g FROM (%s) d %s gord ON %s) d", sel, join, c.leafJoin(row))
}
```

In `detailPass`, both grouped cases write the join through it. Line 1538 (`case paged:`) and line 1546 (`case grouped:`) are the same line; replace each

```go
		from = fmt.Sprintf("(SELECT d.*, gord.__g AS __g FROM (%s) d JOIN gord ON %s) d", sel, c.leafJoin())
```

with

```go
		from = c.leafFrom(sel)
```

`leafJoin` has no other caller (`command grep -rn 'leafJoin(' /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/` shows lines 1538, 1546 and 1780 before the edit). With the switch on, the SQL is the SQL of before, character for character.

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet ./reports/ && go test -count=1 -run 'TestAGroupedDetailStatement|TestTheOpenGroupJoin|TestAGroupedRunOf|TestTheDetailStatementComputesItsOpenGroupsOnce' -v ./reports/`

Expected: `go vet` prints nothing; PASS for `TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce` (not changed; it must stay under its 2 s limit), `TestAGroupedRunOfAUnionWithManyOpenGroupsReadsItsRowsOnce`, `TestTheDetailStatementComputesItsOpenGroupsOnce`, `TestAGroupedDetailStatementReadsItsRowsOnce`, `TestAGroupedDetailStatementOfACollectionFindsItsGroupByAnIndex`, `TestTheOpenGroupJoinOrderKeepsTheResult`, `TestTheOpenGroupJoinMatchesAKeyOfEveryKind` (three subtests), `TestTheOpenGroupJoinOrderKeepsEveryPage`.

If `TestAGroupedRunOfManyOpenGroupsReadsItsDetailsOnce` fails on its time limit, the `+` is missing from a term (SQLite then builds no index on gord): read the statement's SQL, do not raise the limit.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -run 'TestTransactionDetailReadsItsPeriodRowsOnce$' .`

Expected: `ok  tinycld.org/packages/books`.

- [ ] **Step 7: Run the core reports package once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -count=1 ./reports/`

Expected: `ok  tinycld.org/core/reports` (about 130 s). A failure of a test that compares rows or pages is a result the join changed: find the key whose value and group value differ in storage class; do not change the expected rows.

- [ ] **Step 8: Run the Books Go package once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -timeout 30m .`

Expected: `ok  tinycld.org/packages/books` (about 6 minutes). Every Books report with sections runs through the changed join. A failure of a test that compares rows is a result the join changed: find the section key whose value and group value differ in storage class; do not change the expected rows.

- [ ] **Step 9: Commit the core change**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/run.go core/server/reports/legacy.go core/server/reports/leaforder_test.go
git commit -m "perf(reports): a page's detail rows find their open group through an index"
```

- [ ] **Step 10: Commit the Books tests**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_statements_scale_test.go server/report_general_ledger_scale_test.go
git commit -m "test(reports): Transaction Detail reads its period rows once; the scale plans print the open-section join"
```

The runs of step 11 are then of committed trees; write both hashes in the ledger record.

- [ ] **Step 11: Hand the 100k runs to the driver**

Hand back (the driver runs them, one after the other, and sends an agent back for steps 12 and 13 with the four files):

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9b3 go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
NEEDS-RUN: (quiet machine, as above) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9b3-gl go test -run 'TestGLScale$/100k$' -count=1 -timeout 60m -v .
```

Needed from the first: PASS; `scale-task9b3/st-results.md` (every median, no run refused; the group rows and rows of each run as in `scale-task9d2b/st-results.md`); from `scale-task9b3/st-plans.md`, section "100k lines, run TD 2 all accounts, year", statement 6: its time (1.594 s before), and its open-section lines, which must be `SCAN t1` and `SEARCH gord USING AUTOMATIC PARTIAL COVERING INDEX (g0=? AND g1=?)`, with no `SCAN gord` and no `SEARCH t1`. Before, for every run: `scale-task9d2b/st-results.md` (TD 1 165 ms, TD 2 1.691 s, TD 5 142 ms).

Needed from the second: PASS; `scale-task9b3-gl/gl-results.md` (the five medians and the sections on the page, beside the last General Ledger 100k run, `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5d-b/gl-results.md`: run 1 310 ms, run 2 706 ms, run 3 167 ms, run 4 144 ms, run 5 377 ms; other core changes came between that run and this one, so a difference is not this task's alone); from `gl-plans.md`, run "2 all accounts, year", the statement that has a `gord` line: `SCAN d` and `SEARCH gord USING AUTOMATIC`, no `SEARCH d USING AUTOMATIC`.

A run with a higher median than before on a quiet machine is a result: record it, and do not change the code in this step.

- [ ] **Step 12: Write the ledger record**

Read the end of `books/docs/plans/general-ledger-ledger.md` (the record `#### Task 9d-2b: the Cash Balance Sheet reads the stored cash months (2026-11-20)`, line 773 at books `1001e3ae`) and append after it, in the same form, `#### Task 9b-3: a page's detail rows find their open section through an index (core `server/reports/run.go`, 2026-11-20)` with:

- The cause, from this plan's section "The evidence this plan is written from", items 1 and 3: with no Accounts filter the page statement read the open sections first and, for each, every row of the period (`SCAN gord`, `SEARCH t1 USING AUTOMATIC PARTIAL COVERING INDEX (currency=?)`), because a section's key is the id of the joined account; the scan of the line tables by day (statements 4 and 5, 46 and 37 ms) was not the cost, so no `(date)` index was added.
- The change: core `leafFrom` writes `CROSS JOIN gord` with a unary `+` on each key of the row's side; why each of the two is needed (the join order; the affinity of a stored column, which kept SQLite from an index on gord and made `CROSS JOIN` alone slow on a collection: 3.78 s against 0.19 s on 8,000 open groups).
- General Ledger run 2 did not have the sections x rows join (its plan built one index of all detail rows by the section key); its statement changed all the same, so it was measured again.
- The tests: the core tests of step 2 by name, and Books `TestTransactionDetailReadsItsPeriodRowsOnce`.
- The 100k figures of step 11 beside those before, in a table of the columns `| lines | run | before | after |`: every run of `st-results.md` (before: `scale-task9d2b/st-results.md`) and every run of `gl-results.md`; statement 6 of TD 2 before (1.594 s) and after; the open-section plan lines of TD 2 and of General Ledger run 2 after. The core and Books hashes of step 9 and step 10, and whether the machine was quiet (the load at the start).
- What is left for the 2 s bars at 500k: TD 2 and General Ledger run 2 projected from the 100k figures (say that it is a projection), decided from a 500k run.

- [ ] **Step 13: Commit the record**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs: record the open-section join of a page's detail rows and its 100k runs"
```
