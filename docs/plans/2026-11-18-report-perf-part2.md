# Report Engine Performance Implementation Plan — part 2 (tasks 3 and 11)

> **Dropped by the human 2026-11-19: not needed for the §9 bars.** The A/R Aging scale run with the export in one page (`2026-11-19-report-perf-export.md`) meets every bar without these tasks. Do not execute this plan.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Report runs on a book with 500,000 journal lines meet the time bars of the A/R Aging spec §9 (each run ≤ 1,000 ms, CSV ≤ 5,000 ms, L ≤ 15 × S), with every other report's result unchanged.

**Architecture:** Part 1 added the measurement harness and made a level compute each distinct aggregate one time. This part makes `w` compute the argument of a level's aggregate one time for each row (task 3), and finds and removes the time a run's statements take inside a run above the time of the same statements sent directly (task 11, which the harness of part 1 printed as "outside SQL"). Each engine change keeps a `legacySQL` switch and a cross-check on the fictional zoo.

**Tech Stack:** Go, SQLite (PocketBase fork, dbx, modernc.org/sqlite v1.57.0), core report engine `tinycld.org/core/reports`, Books server tests (`tinycld.org/packages/books`).

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/ar-aging-design.md` (§9 Performance acceptance). Root-cause profile: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-ar-aging/cp/cp3-perf-rootcause.md`. Outline of all 11 tasks, with the budget and the stop rule: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/plan-outline.md`. Part 1 (tasks 1 and 2): `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-18-report-perf.md`.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-perf`); Books is `books/` (branch `books/p5-report-perf`). Task 3 is a core task. Task 11 starts in Books (the measure) and its fix is a core task or a Books task, by its cause.

**Order:** task 3 runs after task 2 (it changes `levelCtx.aggregate` as task 2 leaves it). Task 11 runs after task 1 and before task 5. Tasks 4 to 10 are in a later part.

## Global Constraints

- Time bars (spec §9, verbatim): "runs 1 to 6: 1,000 ms or less each; run 7: 5,000 ms or less; each run at L takes no more than 15 times its time at S". "A bar is never changed and a run is never removed to pass."
- The human (2026-11-18): the 1 s whole-book bar at 500k lines stays. When the measurements show a bar is not attainable after all reasonable work, the work stops for the human with the evidence. It does not weaken the bar.
- Every plan bar of spec §9 stays (no `SCAN` of `books_journal_lines` or `books_journal_entries`, no `AUTOMATIC … INDEX` on a large table, "No view or subquery over a large table is built whole (`MATERIALIZE`, `CO-ROUTINE`) and then scanned inside a loop", and the others).
- Core names no package: no Books slug, collection, or route in `tinycld/`. Core tests use the fictional zoo (`zoo.meals`).
- Every other shipped report's result stays identical: the existing core and Books report tests are the guard. A test that compares SQL text may change; a test that compares a result may not.
- Each engine change keeps a `legacySQL` switch, one test that proves the switch writes the old SQL, and a cross-check test (old result = new result, byte for byte, on the zoo).
- No migration changes in this part.
- The S and L runs start only with an environment variable; `go test` with none of them stays fast.
- Go server logging and style per the workspace `CLAUDE.md`; comments say why, not what.

## Review Focus

1. A reader who sees no row (zoo user `eve`), and every other zoo user: the result with row values computed once is the result of before (task 3: `TestRowValuesComputedOnceKeepTheResult`).
2. A param with another value (As of another day): the value kept for a row is named by the formula node, not by the bound value, so the second run reads its own day (task 3: `TestRowValuesComputedOnceKeepTheResult`, the `asOf` run).
3. A statement with no group level (a probe), and the detail statement of an opened group, also when a group has a `having` (it then compiles levels to find the shown groups, `shownLevels`): neither gets an `x` value or a `LIMIT -1`, which would stop SQLite from pushing a filter into `w` (task 3: `TestAStatementWithNoLevelKeepsItsRowsPlain`, `TestTheDetailStatementOfAHavingKeepsItsRowsPlain`).
4. One formula node read by aggregates of every level (4 levels in A/R Aging): one `x` value in `w`, not one for each level (task 3: `TestAnAggregateArgumentIsComputedOncePerRow` counts `julianday(` in the whole statement).
5. The in-run measure on a run whose statement count changes between calls (a statement cache, a first-call ship): the test stops with the two counts, it does not pair the wrong statements (task 11: the `len(times) != len(long)` check in `TestARAgingInRunCost`).

---

### Task 3: Row values once per row (core)

Expected saving at S, run 1 (prof-agg.txt of the root cause): `julianday` goes from about 20 calls for each row, level and statement to 2 for each row and statement: about 90 ms of statement 4 and 47 ms of statement 5. Measure with task 1's harness after the change and write the two numbers in the report.

How it works. An aggregate of a level compiles its argument on a base row (`rowsCtx` with `inAggregate`), as SQL over the columns of `w` (`w.h3`, `w.c1`). Today that SQL is written inside each `a<L>`, so every level computes it again for every row. After this task `w` computes it as a column `x<n>`, and each level writes `sum(w.x<n>)`. A `diffDays` inside such an argument is an `x` column of its own, so the five age columns of an aging report read one age.

Two facts found with real commands (2026-11-18), which the code depends on:

- Each compile of one formula node binds its literals under new names (`{:rp7}`, `{:rp12}`), so the SQL of two compiles of one node differs. An `x` value is therefore named by the JSON of its node (`rowsCTE.once(key, sql)`), not by its SQL. The outline wrote `once(sql string)`; the signature here replaces it.
- SQLite merges a subquery into the select that reads it and writes the subquery's expression into every place that reads the column: `sqlite3 :memory: "create table t(a,b); explain with w as (select a, b from t), w2 as (select w.*, julianday(a) as x0 from w) select sum(case when x0>1 then b end), sum(case when x0>2 then b end) from w2;"` shows `julianday` 2 times; with `… from w limit -1)` it shows it 1 time and the plan is `CO-ROUTINE w2`. So a select whose `x` values a later layer reads ends in `LIMIT -1`.

Only a level's aggregate asks for `x` values (`rowsCtx.once`). A detail row's `sum(x)` (one row) and a pivot cell do not: a statement with no level keeps the `w` of today. The detail statement of a report with a `having` does compile levels on its own `rowsCTE` (`shownLevels`, `run.go`, called by `detailPass`); A/R Aging has one, so the added statements of runs 5 and 6 are such statements. Its `w` is read under a filter on the opened keys and stops at a LIMIT, so `shownLevels` marks the `rowsCTE` `plain`, and `once` then returns the SQL unchanged.

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go` (one field)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rows.go` (`type rowsCTE`; `func (c *compiler) rows`; `func (r *rowsCTE) render`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/detail.go` (imports; `type rowsCtx`; `func (rc *rowsCtx) aggregate`; new `onceValue`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/formula.go` (`case "diffDays"`, near line 956; `func (ctxBase) aggregateArg`, near line 1291)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/levels.go` (`func (lc *levelCtx) aggregate`, as task 2 wrote it)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go` (`func (c *compiler) shownLevels`, near line 1425)
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rowonce_test.go`

**Interfaces:**
- Consumes (task 2, in the tree when this task starts; read each definition first): `legacySQL` (`legacy.go`); `requireSameAsLegacy(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string, off *bool)`, `levelAggregates *regexp.Regexp`, `requireInEachLevel(t *testing.T, seen []readStatement, text string, n int)`, consts `blankZeroMeals`, `differingMeals`, `lookupTwice` (`aggonce_test.go`); `levelCtx.aggregate` with `level.aggMemo`. Existing: `newUnionZoo(t) *zoo` and source `zoo.meals` with fields `kind`, `cost`, `fed` (a Date) (`union_zoo_test.go`); `(z *zoo) run(t, user, spec string, opts Options, params map[string]string) *Result` (a param value is JSON text: `"asOf": "\"2026-01-05\""`); `observeReads(t) *[]readStatement` (field `sql`).
- Produces: `legacySQL.rowValuesPerLevel bool`; `(r *rowsCTE) once(key, sql string) string` (returns `w.x<n>`, or `sql` itself when it is one column of `w`); `type onceValue struct{ name, sql string; layer int }`; `rowsCTE.xs []onceValue`, `rowsCTE.onceBy map[string]string`, `rowsCTE.plain bool` (set by `shownLevels`: `once` returns its SQL unchanged); `rowsCtx.once bool`; `(rc *rowsCtx) onceValue(n *Node, sql string) string`; `aggregateArg(r *rowsCTE, visiting map[string]bool, idText, once bool, op Aggregate, arg *Node, scale int, at string)` (one new parameter, `once`); the form of `w` with `x` values: `w AS (SELECT w.*, <layer 1 x values> FROM (SELECT w.*, <layer 0 x values> FROM (<the select of today>) w LIMIT -1) w)`; test const `agedMeals`, `rowsCTEText *regexp.Regexp`. Task 4 reads `w.x<n>` as the argument of the aggregates it rolls up.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/rowonce_test.go`:

```go
package reports

import (
	"regexp"
	"strings"
	"testing"
)

// agedMeals totals a kind's cost by the age of the meal on a day, as an aging
// report does: each age column reads the same days between two dates.
const agedMeals = `{"version":4,"source":"zoo.meals",
	"params":[{"id":"asOf","type":"date","label":"As of","default":"2026-03-01"}],
	"columns":[
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"recent","formula":{"op":"sum","args":[{"op":"if","args":[
		{"op":"lte","args":[{"op":"diffDays","args":[{"path":[{"field":"fed"}]},{"param":"asOf"}]},{"value":30}]},
		{"path":[{"field":"cost"}]},{"value":0}]}]}},
	{"id":"old","formula":{"op":"sum","args":[{"op":"if","args":[
		{"op":"gt","args":[{"op":"diffDays","args":[{"path":[{"field":"fed"}]},{"param":"asOf"}]},{"value":30}]},
		{"path":[{"field":"cost"}]},{"value":0}]}]}},
	{"id":"all","formula":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}],
	"groups":[{"id":"g","col":"type"}],"sort":[{"col":"type"}]}`

// rowsCTEText finds w in a statement.
var rowsCTEText = regexp.MustCompile(`(?ms)^w AS (?:MATERIALIZED )?\((.*?)\),\n\w+ AS `)

func TestAnAggregateArgumentIsComputedOncePerRow(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", agedMeals, Options{}, nil)
	levels := 0
	for _, st := range *seen {
		if !levelAggregates.MatchString(st.sql) {
			continue
		}
		levels++
		m := rowsCTEText.FindStringSubmatch(st.sql)
		if m == nil {
			t.Fatalf("no w in:\n%s", st.sql)
		}
		// The days are one value of w, and each age column's amount one more.
		if got := strings.Count(m[1], "julianday("); got != 2 {
			t.Errorf("w calls julianday %d times, want 2 (one diffDays):\n%s", got, m[1])
		}
		if got := strings.Count(st.sql, "julianday("); got != 2 {
			t.Errorf("the statement calls julianday %d times, want 2", got)
		}
		if !strings.Contains(m[1], "\nLIMIT -1) w") {
			t.Errorf("the select of the days is not kept apart (LIMIT -1):\n%s", m[1])
		}
	}
	if levels == 0 {
		t.Fatal("no statement has a level's aggregates")
	}
	requireInEachLevel(t, *seen, "julianday(", 0)
	requireInEachLevel(t, *seen, "CASE", 0)
	// The switch writes the SQL of before: the cross-checks compare with it.
	legacySQL.rowValuesPerLevel = true
	t.Cleanup(func() { legacySQL.rowValuesPerLevel = false })
	*seen = nil
	z.run(t, "ann", agedMeals, Options{}, nil)
	requireInEachLevel(t, *seen, "julianday(", 4)
}

// A statement with no group level (the detail rows, a probe) totals nothing:
// its w stays one plain select, so SQLite can still push a filter into it and
// stop at a LIMIT.
func TestAStatementWithNoLevelKeepsItsRowsPlain(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", agedMeals, Options{ExpandAll: true}, nil)
	if len(res.Rows) < 6 {
		t.Fatalf("the run shows %d rows, want the detail rows too", len(res.Rows))
	}
	plain := 0
	for _, st := range *seen {
		if levelAggregates.MatchString(st.sql) {
			continue
		}
		plain++
		if strings.Contains(st.sql, " AS x0") || strings.Contains(st.sql, "LIMIT -1") {
			t.Errorf("a statement with no level has an x value:\n%s", st.sql)
		}
	}
	if plain == 0 {
		t.Fatal("no statement without a level ran")
	}
}

// agedMealsHaving hides a kind with no recent cost: the detail statement then
// compiles the group's level (shownLevels) to find the kinds the having keeps.
var agedMealsHaving = strings.Replace(agedMeals, `"groups":[{"id":"g","col":"type"}]`,
	`"groups":[{"id":"g","col":"type","having":{"op":"gte","args":[{"col":"recent"},{"value":0}]}}]`, 1)

func TestTheDetailStatementOfAHavingKeepsItsRowsPlain(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", agedMealsHaving, Options{ExpandAll: true}, nil)
	if len(res.Rows) < 6 {
		t.Fatalf("the run shows %d rows, want the detail rows too", len(res.Rows))
	}
	details := 0
	for _, st := range *seen {
		if !strings.Contains(st.sql, "ws AS (") {
			continue
		}
		details++
		if strings.Contains(st.sql, " AS x0") || strings.Contains(st.sql, "LIMIT -1") {
			t.Errorf("the detail statement has an x value:\n%s", st.sql)
		}
	}
	if details == 0 {
		t.Fatal("no detail statement read the shown groups (ws)")
	}
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		requireSameAsLegacy(t, z, user, agedMealsHaving, Options{ExpandAll: true}, nil, &legacySQL.rowValuesPerLevel)
	}
}

func TestRowValuesComputedOnceKeepTheResult(t *testing.T) {
	z := newUnionZoo(t)
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		for _, spec := range []string{agedMeals, blankZeroMeals, differingMeals, lookupTwice} {
			requireSameAsLegacy(t, z, user, spec, Options{ExpandAll: true}, nil, &legacySQL.rowValuesPerLevel)
		}
		requireSameAsLegacy(t, z, user, agedMeals, Options{ExpandAll: true}, map[string]string{"asOf": `"2026-01-05"`}, &legacySQL.rowValuesPerLevel)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports`): `go test -run 'TestAnAggregateArgument|TestAStatementWithNoLevel|TestRowValues' -count=1 .`

Expected: a build failure, `legacySQL.rowValuesPerLevel undefined`. Add the field to `legacy.go` (step 3, first block) and run again. Expected:

```
--- FAIL: TestAnAggregateArgumentIsComputedOncePerRow
    rowonce_test.go: w calls julianday 0 times, want 2 (one diffDays):
```

`TestAStatementWithNoLevelKeepsItsRowsPlain`, `TestTheDetailStatementOfAHavingKeepsItsRowsPlain` and `TestRowValuesComputedOnceKeepTheResult` pass already (the switch changes nothing yet): they are the guards for step 3. In step 3, write the `plain` lines (`rows.go` field and `once`'s first `if`, `run.go`) last, and run `TestTheDetailStatementOfAHavingKeepsItsRowsPlain` just before them: expected `the detail statement has an x value`. The planner did not run that red; when the test is green without the `plain` lines, say so in your report and keep the lines.

- [ ] **Step 3: Implement**

In `legacy.go`, add to the `legacySQL` struct, after `repeatAggregates bool`:

```go
	// rowValuesPerLevel writes an aggregate's argument inside every a<L> that
	// totals it, as before w computed it one time per row (rowsCTE.once).
	rowValuesPerLevel bool
```

In `rows.go`, in `type rowsCTE`, after the field `wheres []rowWhere`, add the two fields; then add the type, the two patterns and `once` after the struct:

```go
	// xs are the values w computes one time per row for the levels'
	// aggregates (once), and onceBy their names by key.
	xs     []onceValue
	onceBy map[string]string
	// plain keeps w one select with no x value: the detail statement, whose
	// levels (shownLevels) only find the groups a having hides.
	plain bool
```

```go
// onceValue is a value of w computed over w's own columns: x<n>. layer is 0
// for a value that reads no other x value, else one more than the highest
// layer it reads.
type onceValue struct {
	name, sql string
	layer     int
}

// onceRead finds a read of an x value of w.
var onceRead = regexp.MustCompile(`\bw\.(x\d+)\b`)

// bareRead is a value that is one column of w: it needs no x value.
var bareRead = regexp.MustCompile(`^w\.\w+$`)

// once puts a value of a base row in w as x<n> and returns its reference, so
// w computes it one time per row where every level that totals it would
// compute it again. key names the value by the formula node it compiles:
// each compile of one node binds its literals under new names, so the SQL of
// two compiles differs.
func (r *rowsCTE) once(key, sql string) string {
	if r.plain || bareRead.MatchString(sql) {
		return sql
	}
	if name, ok := r.onceBy[key]; ok {
		return "w." + name
	}
	x := onceValue{name: fmt.Sprintf("x%d", len(r.xs)), sql: sql}
	for _, m := range onceRead.FindAllStringSubmatch(sql, -1) {
		for _, read := range r.xs {
			if read.name == m[1] {
				x.layer = max(x.layer, read.layer+1)
			}
		}
	}
	r.xs = append(r.xs, x)
	r.onceBy[key] = x.name
	return "w." + x.name
}
```

In `func (c *compiler) rows`, the `&rowsCTE{…}` literal gains `onceBy: map[string]string{}` (after `byExpr: map[string]string{}`).

Replace `func (r *rowsCTE) render` whole with:

```go
func (r *rowsCTE) render() string {
	cols := make([]string, len(r.order))
	for i, name := range r.order {
		cols[i] = r.raw[name] + " AS " + name
	}
	sql := "SELECT " + strings.Join(cols, ", ") + "\n" + r.root.render()
	if r.filter != "" {
		sql += "\nWHERE " + r.filter
	}
	// The x values, one select for each layer over the one before. A select
	// whose x values a later layer reads ends in LIMIT -1: SQLite then keeps
	// it as it is, where it would merge it into the select above and write
	// the value into every expression that reads it.
	layers := 0
	for _, x := range r.xs {
		layers = max(layers, x.layer+1)
	}
	for layer := range layers {
		var xs []string
		for _, x := range r.xs {
			if x.layer == layer {
				xs = append(xs, x.sql+" AS "+x.name)
			}
		}
		if layer > 0 {
			sql += "\nLIMIT -1"
		}
		sql = "SELECT w.*, " + strings.Join(xs, ", ") + "\nFROM (" + sql + ") w"
	}
	return "w AS (" + sql + ")"
}
```

The names `x<n>` are free in `w` (its columns are `__id`, `k`, `kl`, `ck`, `pk`, `c`, `h`, `p`, `cs`, `cn`, `n`); `x<idx>` in `levels.go` is a value of `a<L>`, another table.

In `detail.go`, add `"encoding/json"` to the imports. In `type rowsCtx`, after `inAggregate bool`:

```go
	// once is set inside an aggregate of a group level: w computes a value of
	// the row one time (rowsCTE.once), and each level reads it.
	once bool
```

In `func (rc *rowsCtx) aggregate`, the call becomes `rc.aggregateArg(rc.r, rc.visiting, rc.idText, false, op, arg, scale, at)` (a detail row totals one row: no `x` value). Add before `func (rc *rowsCtx) window`:

```go
// onceValue is sql, the value of node n on a base row, computed one time per
// row in w when the context is a level's aggregate; elsewhere it is sql.
func (rc *rowsCtx) onceValue(n *Node, sql string) string {
	if !rc.once {
		return sql
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return sql
	}
	return rc.r.once("row|"+string(raw), sql)
}
```

In `formula.go`, `func (ctxBase) aggregateArg` gets the parameter `once` and passes it on; its first two lines become:

```go
func (ctxBase) aggregateArg(r *rowsCTE, visiting map[string]bool, idText, once bool, op Aggregate, arg *Node, scale int, at string) (expr, error) {
	inner := &rowsCtx{r: r, inAggregate: true, once: once, visiting: visiting, idText: idText}
```

In `case "diffDays":` of `compileNode`, replace the `return expr{sql: "CAST(julianday(…` line with:

```go
		days := "CAST(julianday(substr(" + args[1].sql + ", 1, 10)) - julianday(substr(" + args[0].sql + ", 1, 10)) AS INTEGER)"
		// Inside a level's aggregate the days are a value of the row: the
		// aggregates that read them (one for each age range) share one.
		if o, ok := c.(interface{ onceValue(*Node, string) string }); ok {
			days = o.onceValue(n, days)
		}
		return expr{sql: days, typ: Number}, nil
```

Only `diffDays` is taken out from under an `if`: it cannot fail, so computing it on a row the `if` would skip changes nothing. Do not do this for another operator.

In `levels.go`, `func (lc *levelCtx) aggregate` (as task 2 wrote it): the `aggregateArg` call gets the new argument, and the argument's SQL becomes an `x` value before `countedArg`:

```go
	e, err := lc.aggregateArg(lv.r, lc.visiting, false, !legacySQL.rowValuesPerLevel, op, arg, scale, at)
```

```go
	name := fmt.Sprintf("y%d", len(lv.aggs))
	if raw, err := json.Marshal(arg); err == nil && !legacySQL.rowValuesPerLevel {
		// Every level that totals the argument reads the one value of w.
		e.sql = lv.r.once("arg|"+string(raw), e.sql)
	}
	e = countedArg(op, e)
```

(The first line and the last line of that block exist; the `if` goes between them.)

In `run.go`, `func (c *compiler) shownLevels`, after the line `c.fillSets = map[int]string{}`:

```go
	// The detail statement reads w under a filter on the opened keys and
	// stops at its LIMIT: its w stays one plain select.
	r.plain = true
```

Do not change `recompute` (`levels.go`, its own `rowsCtx`), `pivot.go`, `rowsTotal` or `toManyTotal`: they keep `once` false.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go vet . && go test -run 'TestAnAggregate|TestAggregatesThatDiffer|TestAStatementWithNoLevel|TestTheDetailStatement|TestRowValues' -count=1 .`
Expected: `ok`. (Run against the tree with tasks 2 and 3 through `go test -overlay` on 2026-11-18: green.)

- [ ] **Step 5: Run the report packages**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -count=1 .`
Expected: `ok` in about 95 s (run on 2026-11-18 with tasks 2 and 3: `ok`, no expectation changed). A test that fails because it compares SQL text is updated to the new text; a test that fails on a result (a cell, a row, a notice) is a defect of this task: fix the code, never the expectation.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'Report|ARAging' -count=1 .`
Expected: `ok` (run on 2026-11-18 with tasks 2 and 3: `ok`; the small plan test still meets the plan bars with the `CO-ROUTINE` of the `LIMIT -1` select, which is read one time and not inside a loop).

- [ ] **Step 6: Measure**

Run task 1's S profile to `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-S-task3.txt` (the command of part 1, task 1, step 3 with that file name; `NEEDS-RUN` when it passes 4 minutes). In your report give, for run 1, statement 4 and statement 5 before (from `profile-S-task2.txt`) and after, and the count of `julianday(` in statement 4 before and after. Expected: statement 4 lower by about 90 ms, statement 5 by about 47 ms. A saving under 30 ms in statement 4 is reported as such with the `w AS` text of statement 4; it does not fail the task. Give also the time of the statement that run 5 and run 6 each add to run 1, before and after: it must not rise, and its text has no ` AS x0` and no `LIMIT -1` (a rise or an `x` value there is a defect of this task). In the plan lines of statement 4, a `CO-ROUTINE` step inside a loop (a parent step that is a `SEARCH` or a correlated subquery) breaks a plan bar: report it and stop.

- [ ] **Step 7: Commit (core)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/legacy.go core/server/reports/rows.go core/server/reports/detail.go core/server/reports/formula.go core/server/reports/levels.go core/server/reports/run.go core/server/reports/rowonce_test.go
git commit -m "perf(reports): w computes a level's aggregate argument one time per row"
```

(Add any test file whose SQL-text expectation step 5 updated.)

---

### Task 11: The cost a run adds to its statements (Books measure; fix by its cause)

Runs after task 1 and before task 5. Target (outline): the time of a run that grows with the data and is not in its statements' direct times is at most 4 ms at S.

What is known (planner's measurements at S on 2026-11-18, same tree, a loaded machine; none is a guess):

- Part 1's harness printed 193 ms "outside SQL" for run 1. That column is wall (median of 5 runs) minus the sum of the statements' direct times (best of 3, the text with its values written in).
- The time is not Go code. A CPU profile of 10 × run 1: of `reports.Run`'s 7.86 s, 7.67 s is inside `sqlite3_step`; Go's part (prepare, scan, assemble, the `EXPLAIN`s of the change topics) is 19 ms a run and does not grow.
- With the same statistic on both sides (median of 5), run 1 is 1,012 ms wall against 773 ms for its statements sent directly; run 4 is 310 ms against 302 ms.
- Timed inside the run from the statement log, the page statement took 875 ms (direct: 481 ms) and the Total statement 315 ms (direct: 234 ms).
- A deadline context is not the cause: the direct text with `WithContext` (a 10 s timeout) takes the same time as with none (479 against 481 ms; 215 against 234 ms).
- The profile of the run: 25% of the samples are in `sqlite3Malloc` and 20% in the `mmap` system call under `pcache1Alloc` (the page cache of the temp B-trees of `MATERIALIZE`); the profiler caught 86% of the wall time.

So the difference left between a run and the direct text is: the run binds its values (`NewQuery(sql).Bind(s.params)`, `core/server/reports/run.go` `readRows`), the direct text has them written in. The cause is not yet known. Step 4 finds it.

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_ar_aging_inrun_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-ledger.md` (section `## Performance`)
- Modify (the fix, when step 4 names a cause in core): the file step 4 names in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/`, with a zoo test beside it.

**Interfaces:**
- Consumes (all in `books/server`, read each definition first): `newARBook(t) arBook`, `arJournalPrefix(t, b)` (`receivable_items_test.go`), `writeARDataset(t testing.TB, b arBook, size arSize) arDataset`, sizes `arSmall`, `arS`, `arL`, `arRuns(b arBook, data arDataset) []arRun` (index 0 is run 1, index 3 is run 4; `name`; `exec(t, b)`), `recordARRun(t *testing.T, b arBook, r arRun) []string` (`report_ar_aging_scale_test.go`); `recordStatements(t testing.TB, app *tests.TestApp, seen func(statement string), fn func())` (`sales_document_deferral_test.go`; `seen` is called when a statement ends, and may run on another goroutine).
- Produces: `TestARAgingInRunCost` (starts only with `BOOKS_PROFILE_INRUN=<file>`); `arInRunTimes(t *testing.T, b arBook, r arRun) []time.Duration`; `arMedianOf(n int, fn func() time.Duration) time.Duration`; const `arLongStatement`; the output lines `INRUN <run>: in-run <t>, direct <t>, bound cost <t>, outside the long statements <t>`. No interface for other tasks.

- [ ] **Step 1: Write the measure**

Create `books/server/report_ar_aging_inrun_test.go`:

```go
package books

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// arLongStatement is the least length of a statement that reads the report's
// source: the page, the Total and the detail statements. The others (settings,
// schema, EXPLAIN) take under a millisecond.
const arLongStatement = 5000

// arInRunTimes runs r one time and returns, for each long statement in the
// order the run sent them, the time from the log line of the statement before
// it (the EXPLAIN of its change topics) to its own log line. dbx writes the
// log line when the query returns its rows handle, so this is the statement's
// time with its values bound up to its first row; the read of the rows after
// that is in "outside the long statements".
func arInRunTimes(t *testing.T, b arBook, r arRun) []time.Duration {
	t.Helper()
	var mu sync.Mutex
	var out []time.Duration
	last := time.Now()
	recordStatements(t, b.app, func(statement string) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		s := strings.TrimSpace(statement)
		if len(s) >= arLongStatement && strings.HasPrefix(s, "WITH") {
			out = append(out, now.Sub(last))
		}
		last = now
	}, func() { r.exec(t, b) })
	return out
}

// arMedianOf is the median of n calls of fn after one warm-up call.
func arMedianOf(n int, fn func() time.Duration) time.Duration {
	fn()
	d := make([]time.Duration, n)
	for i := range d {
		d[i] = fn()
	}
	slices.Sort(d)
	return d[n/2]
}

// TestARAgingInRunCost writes, for runs 1 and 4 of spec §9, the time of each
// long statement inside the run (values bound, as core sends it) beside the
// time of the same text sent directly with its values written in, which is
// what TestARAgingProfile times. It starts only with
// BOOKS_PROFILE_INRUN=<output file>; BOOKS_PROFILE_SIZE is small, S (default)
// or L. It has no bar.
func TestARAgingInRunCost(t *testing.T) {
	out := os.Getenv("BOOKS_PROFILE_INRUN")
	if out == "" {
		t.Skip("set BOOKS_PROFILE_INRUN=<output file> to compare a run's statements with their direct times")
	}
	size, ok := map[string]arSize{"small": arSmall, "": arS, "S": arS, "L": arL}[os.Getenv("BOOKS_PROFILE_SIZE")]
	if !ok {
		t.Fatalf("BOOKS_PROFILE_SIZE is small, S or L")
	}
	b := newARBook(t)
	arJournalPrefix(t, b)
	data := writeARDataset(t, b, size)
	runs := arRuns(b, data)
	var text strings.Builder
	for _, run := range []arRun{runs[0], runs[3]} {
		var long []string
		for _, s := range recordARRun(t, b, run) {
			if len(s) >= arLongStatement && strings.HasPrefix(s, "WITH") {
				long = append(long, s)
			}
		}
		wall := arMedianOf(5, func() time.Duration {
			started := time.Now()
			run.exec(t, b)
			return time.Since(started)
		})
		fmt.Fprintf(&text, "INRUN %s: wall %s, %d long statements\n", run.name, wall.Round(time.Millisecond), len(long))
		var inRunSum, directSum time.Duration
		for i, s := range long {
			inRun := arMedianOf(5, func() time.Duration {
				times := arInRunTimes(t, b, run)
				if len(times) != len(long) {
					t.Fatalf("run %s sent %d long statements, recorded %d", run.name, len(times), len(long))
				}
				return times[i]
			})
			direct := arMedianOf(5, func() time.Duration {
				started := time.Now()
				rows, err := b.app.DB().NewQuery(s).Rows()
				if err != nil {
					t.Fatalf("direct statement: %v\n%.300s", err, s)
				}
				for rows.Next() {
				}
				if err := rows.Close(); err != nil {
					t.Fatal(err)
				}
				return time.Since(started)
			})
			inRunSum += inRun
			directSum += direct
			fmt.Fprintf(&text, "INRUN %s statement %d (%d chars): in-run %s, direct %s\n", run.name, i, len(s),
				inRun.Round(time.Millisecond), direct.Round(time.Millisecond))
		}
		fmt.Fprintf(&text, "INRUN %s: in-run %s, direct %s, bound cost %s, outside the long statements %s\n", run.name,
			inRunSum.Round(time.Millisecond), directSum.Round(time.Millisecond), (inRunSum - directSum).Round(time.Millisecond),
			(wall - inRunSum).Round(time.Millisecond))
	}
	if err := os.WriteFile(out, []byte(text.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Verify it skips with no variable and runs at the small size**

Run (in `/Users/nas/tmp/tinycld/books-purchases/books/server`):

```bash
go vet . && go test -run 'TestARAgingInRunCost$' -count=1 -v . 2>&1 | command grep -E 'SKIP|ok|FAIL'
```

Expected: `--- SKIP: TestARAgingInRunCost` and `ok`.

```bash
BOOKS_PROFILE_INRUN=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/inrun-small.txt BOOKS_PROFILE_SIZE=small go test -run 'TestARAgingInRunCost$' -count=1 . && command grep INRUN /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/inrun-small.txt
```

Expected: `ok` in about 5 s, and 8 lines. The planner's run (2026-11-18): run 1 wall 47 ms; statement 0 in-run 34 ms, direct 32 ms; statement 1 in-run 5 ms, direct 3 ms; `bound cost 4ms, outside the long statements 8ms`. At 500 lines there is no bound cost: this is the cost that does not grow.

- [ ] **Step 3: Run it at S: the failing measure**

```bash
BOOKS_PROFILE_INRUN=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/inrun-S-before.txt go test -run 'TestARAgingInRunCost$' -count=1 -timeout 10m . && command grep INRUN /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/inrun-S-before.txt
```

Expected: `ok` in about 1 minute. Run 1's `bound cost` is far above 4 ms (the planner measured about 240 ms from medians); run 4's is under 15 ms. This line is the red of the task. When run 1's `bound cost` is under 15 ms on your tree, an earlier task removed the cause: record the lines in the ledger (step 6), make no fix, and commit the test.

- [ ] **Step 4: Find the cause**

Work from a copy of the statement and its values; change no repo file to find the cause (a scratch test through `go test -overlay`, files in the scratchpad). Check these in order, and stop at the first that explains the cost. Write each check and its numbers in your report.

0. **The order of the statements.** A direct time is one statement sent 6 times in a row; a run sends its statements in turn. Send the statements `recordARRun` returns for run 1 (values written in) one time each, in the run's order, on `b.app.DB()`, and take the median of 5 such passes for each long statement. When the page statement takes its in-run time there, binding is not the cause: go to check 3. When it takes its direct time, go on with check 1.
1. **The plan with bound values.** Get the page statement of run 1 as core sends it: in a scratch copy of `core/server/reports/run.go`, in `readRows` before `app.DB().NewQuery(sql).Bind(s.params)`, run `EXPLAIN QUERY PLAN ` + `sql` with the same `Bind(s.params)` and write the `detail` column of its rows to a file. Compare with the plan of the direct text (`explainStatements(t, b.app, statements)` in the Books test file gives it). A step that differs (a `MATERIALIZE` in place of a `CO-ROUTINE`, a `SCAN` in place of a `SEARCH`, another index) is the cause: SQLite plans a comparison or an `IN` with a bound value without knowing the value.
2. **Which value.** When the plans are the same, send the page statement directly with all values written in but one group bound, one group at a time (the As of day; the customers list; the `Today` day; the numbers): the group whose binding brings the time from the direct time to the in-run time holds the cause.
3. **The connection.** When no binding changes the time, time the bound statement two times in a row on `app.DB()` and on `app.NonconcurrentDB()`: a first call slower than the second, or a pool difference, is a page-cache or temp-store setting of the connection the run uses.

- [ ] **Step 5: Remove the cause, test first**

The fix is in the place the cause names, and starts from a failing test:

- Cause in how core writes or binds a value (checks 1 and 2): the fix is in core `reports`, as `limitSQL` (`stmt.go`) is the precedent for a value written as a literal. Only a value the engine has typed and bounded is written as a literal (an integer; a day that passed `isDay`); a text a reader typed is never written into SQL. The red is a zoo test in `core/server/reports` that reads the statement with `observeReads` and requires the new form; the cross-check is `requireSameAsLegacy` with a new field of `legacySQL`, and one test proves the switch writes the old form (the rule of part 1).
- Cause in the connection (check 3): the fix is where the connection is set up; name the file and the setting in your report before you change it, because it is outside `reports`: hand back `STOPPED: <the setting and the measured times>` when it is a setting of the PocketBase fork under `third_party/`.
- Cause is a value that must not be written as a literal (a record id of the customers list, a text): hand back `STOPPED: <the value group, its bound time and its written-in time>`. Do not write it into SQL.
- No check explains the cost: hand back `STOPPED: bound cost of run 1 is <t> at S and checks 0 to 3 do not explain it`, with the four checks' numbers. Do not guess a fix.

Before any core edit, write in your report the cause (the check and its numbers) and the SQL form of before and after.

After the fix, run step 3's command to `inrun-S-after.txt`. Expected: run 1's `bound cost` at or under 15 ms (the noise of the measure at S), and `outside the long statements` within 15 ms of its small-size value (step 2). S alone does not prove the 4 ms target: hand back `NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_PROFILE_INRUN=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/inrun-L-after.txt BOOKS_PROFILE_SIZE=L go test -run 'TestARAgingInRunCost$' -count=1 -timeout 60m .` — needed from it: the `INRUN` lines; run 1's `bound cost` at L is 50 ms or less (4 ms × 12.3). When it is more, the task is not done: report the lines. The same NEEDS-RUN applies when step 3 found the cost already under 15 ms. Then run the report packages as task 3 step 5 does (core `go test -count=1 .` in `core/server/reports`; Books `go test -run 'Report|ARAging' -count=1 .`): both `ok`, no result expectation changed.

- [ ] **Step 6: Record and commit**

In `books/docs/plans/ar-aging-ledger.md`, section `## Performance`, add under the block "Baseline (p5-report-perf)":

```markdown
### Cost a run adds to its statements (p5-report-perf, task 11)

The harness column "outside SQL" was wall (median) minus the statements' direct times (best of 3, values written in). It is not Go time: a CPU profile of run 1 has 97% of the run inside sqlite3_step.

Before (S): <the two summary INRUN lines of inrun-S-before.txt>
Cause: <the check of step 4 that explained it, with its numbers>
Fix: <the commit and one line>
After (S): <the two summary INRUN lines of inrun-S-after.txt>
After (L): <the two summary INRUN lines of inrun-L-after.txt, when the driver has run it>
```

Every `<…>` is a measured value or a commit you copy; the block in the ledger holds no angle bracket.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_ar_aging_inrun_test.go docs/plans/ar-aging-ledger.md
git commit -m "test(books): the time a run adds to its statements"
```

A core fix is its own commit in `/Users/nas/tmp/tinycld/books-purchases/tinycld` (`git add` the files of the fix and its test by path; message `perf(reports): <the cause removed>`).

---

## What is left (part 3)

Tasks 4 to 10 of the outline: finest-level aggregation with roll-up (reads `w.x<n>` of task 3); one statement for the page and the Total; filters and opened keys pushed into union parts; the one-evaluation export; the Books views and indexes; the scale run and the ledger record with the stop rule. Each adds its field to `legacySQL`, one test that the switch writes the old SQL, and its cross-check with `requireSameAsLegacy`.
