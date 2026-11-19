# Report Engine Performance Implementation Plan — part 3 (task 4)

> **Dropped by the human 2026-11-19: not needed for the §9 bars.** The A/R Aging scale run with the export in one page (`2026-11-19-report-perf-export.md`) meets every bar without this task or the unplanned tasks 5 to 10. Do not execute this plan.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Report runs on a book with 500,000 journal lines meet the time bars of the A/R Aging spec §9 (each run ≤ 1,000 ms, CSV ≤ 5,000 ms, L ≤ 15 × S), with every other report's result unchanged.

**Architecture:** Parts 1 and 2 made a level compute each distinct aggregate one time and made `w` compute an aggregate's argument one time for each row. This part makes a statement total `w` one time: the deepest group level totals `w`, and each level above it totals the level below it (a roll-up). `w` then has one reader and is not written to a temp table. The change keeps a `legacySQL` switch and a cross-check on the fictional zoo.

**Tech Stack:** Go, SQLite (PocketBase fork, dbx, modernc.org/sqlite v1.57.0), core report engine `tinycld.org/core/reports`.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/ar-aging-design.md` (§9 Performance acceptance). Root-cause profile: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-ar-aging/cp/cp3-perf-rootcause.md`. Outline of all 11 tasks, with the budget and the stop rule: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/plan-outline.md`. Part 1 (tasks 1 and 2): `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-18-report-perf.md`. Part 2 (tasks 3 and 11): `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-18-report-perf-part2.md`.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-perf`); Books is `books/` (branch `books/p5-report-perf`). Task 4 is a core task.

**Order:** task 4 runs after tasks 2 and 3 (it changes `level.aggs` and `levelCtx.aggregate` as they leave them). Tasks 5 to 10 are in a later part; task 5 runs after this task and after task 11.

## Global Constraints

- Time bars (spec §9, verbatim): "runs 1 to 6: 1,000 ms or less each; run 7: 5,000 ms or less; each run at L takes no more than 15 times its time at S". "A bar is never changed and a run is never removed to pass."
- The human (2026-11-18): the 1 s whole-book bar at 500k lines stays. When the measurements show a bar is not attainable after all reasonable work, the work stops for the human with the evidence. It does not weaken the bar.
- Every plan bar of spec §9 stays (no `SCAN` of `books_journal_lines` or `books_journal_entries`, no `AUTOMATIC … INDEX` on a large table, "No view or subquery over a large table is built whole (`MATERIALIZE`, `CO-ROUTINE`) and then scanned inside a loop", and the others).
- Core names no package: no Books slug, collection, or route in `tinycld/`. Core tests use the fictional zoo (`zoo.meals`, `zoo.sites`, `zoo.feedings`).
- Every other shipped report's result stays identical: the existing core and Books report tests are the guard. A test that compares SQL text may change; a test that compares a result may not.
- Each engine change keeps a `legacySQL` switch, one test that proves the switch writes the old SQL, and a cross-check test (old result = new result, byte for byte, on the zoo).
- No migration changes in this part.
- The S and L runs start only with an environment variable; `go test` with none of them stays fast.
- Go server logging and style per the workspace `CLAUDE.md`; comments say why, not what.

## Review Focus

1. A reader who sees no row, on a report with no amount (so no currency key): the Total is one row with a count of 0 and a blank min, as the total of no rows was (task 4: `TestTheRollUpOfNoRowsIsTheTotalOfNoRows`).
2. A sum of numbers that may hold fractions, an average, a count of distinct values: the value of a group is not made of the values of its parts, so that level totals `w` itself and `w` is still computed one time for the levels that read it, also when the levels above it roll up (task 4: `TestALevelWhoseValuesDoNotRollUpTotalsTheRows`, `TestLevelsAboveALevelThatTotalsTheRowsStillRollUp`).
3. A sum lookup that meets two currencies inside a total (the level's mixed check), and a group with fill records (rows with no base row, added after the roll-up): the cells, the blank values and the notice are those of before (task 4: `TestAMixedCheckAndAFillRollUpWithTheSameResult`).
4. The detail statement of a report with a `having` (it compiles levels only to find the shown groups, under a filter on the opened keys): no roll-up there, so SQLite can still push the filter into `w` (task 4: `TestTheDetailStatementTotalsTheRowsItself`).
5. One aggregate that two levels read, and one that a level reads two times: one value of the roll-up, and a row's `diffDays` is still computed one time for the whole statement (task 4: `TestLevelsRollUpFromOneTotalOfTheRows`, the `agedMeals` run).

---

### Task 4: Total the rows one time, roll up the levels (core)

Expected saving at S, run 1 (root cause, statement 4): the GROUP BYs of levels 0 to 2 over `w` (50 + 64 + 73 ms) become GROUP BYs over the rows of the level below (about 18,000, then about 200, then 1), and `w` is not written to a temp table (a part of 157 − 115 ms, and of the `sqlite3Malloc` / `mmap` samples of part 2's task 11). Measure with task 1's harness after the change.

How it works. Today each level `L` of a statement has `a<L> AS (SELECT <keys>, <aggregates> FROM w GROUP BY <keys>)`, so `w` is read one time for each level and SQLite writes it to a temp table (`w AS MATERIALIZED`, `materializeShared` in `rows.go`). After this task:

```
w   AS (…)                                                        -- inline: one reader
ag2 AS MATERIALIZED (SELECT k0, k1, ck0, <aggregates of w> AS z<i>, count(*) AS __n FROM w GROUP BY k0, k1, ck0)
ag1 AS MATERIALIZED (SELECT k0, ck0, <roll-up of z<i>> AS z<i>, COALESCE(sum(__n), 0) AS __n FROM ag2 GROUP BY k0, ck0)
ag0 AS MATERIALIZED (SELECT ck0, <roll-up of z<i>> AS z<i>, COALESCE(sum(__n), 0) AS __n FROM ag1 GROUP BY ck0)
a0  AS (SELECT ck0, z0 AS y0, __n FROM ag0)
a1  AS (SELECT k0, ck0, z0 AS y0, __n FROM ag1)
a2  AS (SELECT k0, k1, ck0, z0 AS y0, __n FROM ag2)
```

A level's keys are the keys of the level below it less one (`k0 … k<L-1>` and the currency keys), so each `ag<L>` groups the rows of the `ag` under it. Each `ag` has every aggregate that any level of the chain reads, one column `z<i>` for each distinct operator and argument. The outline wrote one `ag` at the finest level that every level reads; the chain here replaces it, because the finest level of an aging report has about as many rows as `w` has (18,000 of 27,500 at S).

Which aggregates roll up (`rollsUp`): `count` (the sum of the counts), `min`, `max`, and a `sum` whose argument is `Money`. Amounts are whole cents, so their sum is exact in any order. A `sum` of another number may hold fractions, and a sum of floating-point fractions depends on the order; `avg` and `countDistinct` are not made of their parts' values. Two things the roll-up relies on, as the totals of before did in another form: a stored `Money` value is whole cents (formula results are rounded with `tc_round`; a stored fraction could differ in its last bit, and an integer overflow could be raised at another partial sum), and a `min`/`max` compares with the BINARY collation (the engine writes no other; a view column that declares a collation would compare with it in `w` and not in `ag<L>`). A level with one aggregate that does not roll up (also a pivot cell, `q<j>`) totals `w` as today. A to-many total (`d<L>`) is a CTE of its own that reads `w` as today. When fewer than two levels of a statement roll up, nothing changes. When some reader of `w` stays (a level that does not roll up, a `d<L>`, a fill keyset), `materializeShared` still counts two or more reads and materializes `w` as today.

Facts found with real commands on 2026-11-18 (the tree with tasks 2 and 3 applied through `go test -overlay`), which the code depends on:

- `lv.agg(name, sql)` has seven callers: `levelCtx.aggregate`, `recompute` (two), `rowsTotal`, `shownKey` (the label `min(w.kl<i>)`), `mixedTotal` (`max(CASE WHEN … THEN 1 END)`), and `pivot.go` (two). Six of them write `aggregateSQL(op, arg, scale)` or text equal to it; this task gives them `aggOf`, which also keeps the operator and the argument. `pivot.go` keeps `agg`.
- The mixed check of a total is a `max`, so it rolls up with its sum (the outline kept it on `w`; a level with a mixed check would then read `w` a second time for nothing).
- The page statement and the Total statement both hold the CTEs of every level; SQLite does not compute a CTE that nothing reads. The body of the Total statement is `SELECT * FROM o0`, so there the chain runs from `w` to `ag0`.
- `fillRows` (`fill.go`) reads `w` for the keys with no base row and for the totals of no rows. It keeps both reads: the totals of no rows are the same from `w` and from the roll-up.
- Three tests compare SQL text that this task changes (step 5 updates them, no other test of the `reports` package failed): `TestAnAggregateALevelReadsTwiceIsComputedOnce` (`aggonce_test.go`), `TestAnAggregateArgumentIsComputedOncePerRow` (`rowonce_test.go`), `TestGroupStatementMaterializesTheRowsItReadsTwice` (`layout_cost_test.go`).
- The Books small plan test and report tests (`go test -run 'Report|ARAging'` in `books/server`) pass with this change.

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go` (one field)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/levels.go` (`type level`; `func (lv *level) agg`; new `levelAgg`, `rollsUp`, `aggOf`, `rollupSQL`, `aggregateCTEs`, `aggregateRows`; `shownKey`; `base`; `rowsTotal`; `recompute`; `mixedTotal`; `levelCtx.aggregate`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/fill.go` (imports; `func (lv *level) fillRows`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/run.go` (`func (c *compiler) pagedGroupStatement`; `func (c *compiler) groupStatement`)
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rollup_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/aggonce_test.go`, `rowonce_test.go`, `layout_cost_test.go` (one test each, step 5)

**Interfaces:**
- Consumes (tasks 2 and 3, in the tree when this task starts; read each definition first): `legacySQL` (`legacy.go`); `levelCtx.aggregate` as task 3 left it (it calls `lv.agg(name, aggregateSQL(op, e.sql, scale))` after `lv.r.once`); `requireSameAsLegacy(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string, off *bool)`, `levelAggregates *regexp.Regexp`, consts `blankZeroMeals`, `differingMeals`, `lookupTwice` (`aggonce_test.go`); const `agedMeals`, var `agedMealsHaving` (`rowonce_test.go`). Existing: `rowsRead *regexp.Regexp` (`rows.go`: a read of `w`); `aggregateSQL(op Aggregate, arg string, scale int) string` (`formula.go`); `compiledLevel{lv *level; …}` (`run.go`); `newUnionZoo(t) *zoo`, `newZoo(t) *zoo`; `(z *zoo) run(t, user, spec string, opts Options, params map[string]string) *Result`; `observeReads(t) *[]readStatement` (field `sql`); `requireRows(t, res, column string, want ...any)`; const `mixedFees` and `requireMixedFeesBlank(t, z, name, spec string)` (`lookup_test.go`); const `threeFills` (`fill_test.go`).
- Produces: `legacySQL.aggregateEachLevel bool`; `type levelAgg struct{ name, sql string; op Aggregate; arg string }`; `level.aggs []levelAgg`, `level.rollFrom string`, `level.rolled map[string]string`; `rollsUp(op Aggregate, typ FieldType) bool`; `(lv *level) aggOf(name string, op Aggregate, arg string, typ FieldType, scale int) string`; `(lv *level) rollsUp() bool`; `rollupSQL(op Aggregate, z string) string`; `(c *compiler) aggregateCTEs(levels []compiledLevel) []string` (the `ag<L>` CTEs, deepest first; nil when fewer than two levels roll up; it must run before any `lv.render` of those levels); `(lv *level) aggregateRows() string`; test helpers `levelStatements(t *testing.T, seen []readStatement) []string`, `rollupCTE *regexp.Regexp`, consts `rolledMeals`, `mixedFeeTotals`, vars `weighedMeals`, `countedMeals`, `partlyRolledMeals`. Task 5 reads `a0`/`o0` in the page statement: they then read `ag0`, the top of the same chain, with no second total of `w`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/rollup_test.go`:

```go
package reports

import (
	"regexp"
	"strings"
	"testing"
)

// rolledMeals groups the meals by kind, then by note, with a sum of amounts, a
// min, a max and a count: every level's values are made of the values of the
// level below it.
const rolledMeals = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"what","path":[{"field":"note"}]},
	{"id":"paid","formula":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}},
	{"id":"first","formula":{"op":"min","args":[{"path":[{"field":"fed"}]}]}},
	{"id":"last","formula":{"op":"max","args":[{"path":[{"field":"fed"}]}]}},
	{"id":"times","formula":{"op":"count","args":[{"path":[{"field":"fed"}]}]}}],
	"groups":[{"id":"g","col":"type"},{"id":"n","col":"what"}],"sort":[{"col":"type"},{"col":"what"}]}`

// weighedMeals adds up grams, a number that may hold a fraction: its sum is
// not rolled up.
var weighedMeals = strings.Replace(rolledMeals,
	`{"id":"paid","formula":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}`,
	`{"id":"paid","formula":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}`, 1)

// countedMeals has no amount, so no currency key: the Total of a reader with
// no row is one row of a count 0 and a blank min.
var countedMeals = strings.Replace(rolledMeals,
	`{"id":"paid","formula":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}},`, "", 1)

// partlyRolledMeals adds the grams of the rows under each group: the
// innermost level totals that number from w itself, beside the levels above
// it, which roll up.
var partlyRolledMeals = strings.Replace(rolledMeals,
	`{"id":"times",`,
	`{"id":"wt","path":[{"field":"grams"}],"aggregate":"sum","total":"rows"},
	{"id":"times",`, 1)

// rollupCTE finds ag<L>, a level's roll-up, in a statement.
var rollupCTE = regexp.MustCompile(`(?m)^ag\d+ AS `)

// levelStatements are the statements of a run that hold a level's aggregates.
func levelStatements(t *testing.T, seen []readStatement) []string {
	t.Helper()
	var out []string
	for _, st := range seen {
		if levelAggregates.MatchString(st.sql) {
			out = append(out, st.sql)
		}
	}
	if len(out) == 0 {
		t.Fatal("no statement has a level's aggregates")
	}
	return out
}

func TestLevelsRollUpFromOneTotalOfTheRows(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", rolledMeals, Options{ExpandAll: true}, nil)
	for _, sql := range levelStatements(t, *seen) {
		if got := len(rowsRead.FindAllString(sql, -1)); got != 1 {
			t.Errorf("the statement reads w %d times, want 1:\n%s", got, sql)
		}
		if strings.Contains(sql, "w AS MATERIALIZED") {
			t.Errorf("w is materialized for its one reader:\n%s", sql)
		}
		for _, want := range []string{
			"\nag2 AS MATERIALIZED (SELECT k0, k1, ck0, ",
			" FROM w GROUP BY k0, k1, ck0),\nag1 AS MATERIALIZED (SELECT k0, ck0, ",
			" FROM ag2 GROUP BY k0, ck0),\nag0 AS MATERIALIZED (SELECT ck0, ",
			" FROM ag1 GROUP BY ck0),\n",
			", __n FROM ag0),\n", ", __n FROM ag1),\n", ", __n FROM ag2),\n",
		} {
			if !strings.Contains(sql, want) {
				t.Errorf("the statement has no %q:\n%s", want, sql)
			}
		}
	}
	// One aggregate two levels read, or one level reads twice, is one value of
	// the roll-up, and a row's days are computed one time for all of it.
	*seen = nil
	z.run(t, "ann", agedMeals, Options{}, nil)
	for _, sql := range levelStatements(t, *seen) {
		if got := strings.Count(sql, "julianday("); got != 2 {
			t.Errorf("the statement calls julianday %d times, want 2 (one diffDays):\n%s", got, sql)
		}
		if got := strings.Count(sql, " FROM w GROUP BY "); got != 1 {
			t.Errorf("w is totalled %d times, want 1:\n%s", got, sql)
		}
		if got := strings.Count(sql, "sum(w."); got != 3 {
			t.Errorf("the roll-up has %d sums of w, want 3 (recent, old, all):\n%s", got, sql)
		}
	}
	// The switch writes the SQL of before: the cross-checks compare with it.
	legacySQL.aggregateEachLevel = true
	t.Cleanup(func() { legacySQL.aggregateEachLevel = false })
	*seen = nil
	z.run(t, "ann", rolledMeals, Options{ExpandAll: true}, nil)
	materialized := false
	for _, sql := range levelStatements(t, *seen) {
		if rollupCTE.MatchString(sql) || strings.Count(sql, " FROM w GROUP BY ") != 3 {
			t.Errorf("the switch does not write a total of w for each level:\n%s", sql)
		}
		materialized = materialized || strings.Contains(sql, "w AS MATERIALIZED (")
	}
	if !materialized {
		t.Error("the switch does not materialize w for the levels that read it")
	}
}

// A statement where the innermost level totals w itself and the levels above
// it roll up: w has two readers, so it is computed one time for both.
func TestLevelsAboveALevelThatTotalsTheRowsStillRollUp(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", partlyRolledMeals, Options{ExpandAll: true}, nil)
	materialized := false
	for _, sql := range levelStatements(t, *seen) {
		if got := len(rollupCTE.FindAllString(sql, -1)); got != 2 {
			t.Errorf("the statement has %d roll-ups, want 2 (levels 1 and 0):\n%s", got, sql)
		}
		if got := strings.Count(sql, " FROM w GROUP BY "); got != 2 {
			t.Errorf("w is totalled %d times, want 2 (level 2, and the roll-up of level 1):\n%s", got, sql)
		}
		materialized = materialized || strings.Contains(sql, "w AS MATERIALIZED (")
	}
	if !materialized {
		t.Error("no statement materializes w for its two readers")
	}
}

// A level with a value that is not made of its parts' values (an average, a
// sum of numbers that may hold fractions) totals the rows itself.
func TestALevelWhoseValuesDoNotRollUpTotalsTheRows(t *testing.T) {
	z := newUnionZoo(t)
	for name, spec := range map[string]string{"average": differingMeals, "number sum": weighedMeals} {
		seen := observeReads(t)
		z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
		materialized := false
		for _, sql := range levelStatements(t, *seen) {
			if rollupCTE.MatchString(sql) {
				t.Errorf("%s: a level rolls up:\n%s", name, sql)
			}
			materialized = materialized || strings.Contains(sql, "w AS MATERIALIZED (")
		}
		// The page statement's levels each read w: it is computed one time.
		if !materialized {
			t.Errorf("%s: no statement materializes w for the levels that read it", name)
		}
	}
}

func TestRolledUpLevelsKeepTheResult(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", rolledMeals, Options{}, nil)
	requireRows(t, res, "paid", int64(4500), int64(65), int64(4565))
	requireRows(t, res, "times", int64(4), int64(2), int64(6))
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		for _, spec := range []string{rolledMeals, partlyRolledMeals, weighedMeals, countedMeals, agedMeals, agedMealsHaving, blankZeroMeals, differingMeals, lookupTwice} {
			for _, opts := range []Options{{}, {ExpandAll: true}} {
				requireSameAsLegacy(t, z, user, spec, opts, nil, &legacySQL.aggregateEachLevel)
			}
		}
	}
}

// mixedFeeTotals totals, for each region, a sum lookup that meets two
// currencies: the level's mixed check is a max, which rolls up with its sum.
const mixedFeeTotals = `{"version":4,"source":"zoo.sites","columns":[
	{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},
	{"id":"usdFees","formula":{"op":"sum","args":[` + mixedFees + `]}}],
	"groups":[{"id":"r","col":"region"}],"sort":[{"col":"name"}]}`

// A mixed check, and a level with fill records (its rows with no base row
// come after the roll-up), keep their results.
func TestAMixedCheckAndAFillRollUpWithTheSameResult(t *testing.T) {
	z := newZoo(t)
	seen := observeReads(t)
	requireMixedFeesBlank(t, z, "rolled up", mixedFeeTotals)
	rolled := false
	for _, sql := range levelStatements(t, *seen) {
		rolled = rolled || regexp.MustCompile(`(?m)^ag1 AS .*max\(CASE WHEN `).MatchString(sql)
	}
	if !rolled {
		t.Error("the mixed check is not a value of the roll-up")
	}
	*seen = nil
	z.run(t, "ann", threeFills, Options{ExpandAll: true}, nil)
	rolled = false
	for _, sql := range levelStatements(t, *seen) {
		rolled = rolled || rollupCTE.MatchString(sql) && strings.Contains(sql, " EXCEPT SELECT ")
	}
	if !rolled {
		t.Error("no statement has both a roll-up and a level's fill rows")
	}
	for _, user := range []string{"ann", "bob"} {
		for _, spec := range []string{mixedFeeTotals, threeFills} {
			for _, opts := range []Options{{}, {ExpandAll: true}} {
				requireSameAsLegacy(t, z, user, spec, opts, nil, &legacySQL.aggregateEachLevel)
			}
		}
	}
}

// The detail statement of a report with a having compiles levels only to
// find the shown groups, under a filter on the opened keys: they total w
// themselves, so SQLite can still push that filter into w.
func TestTheDetailStatementTotalsTheRowsItself(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	z.run(t, "ann", agedMealsHaving, Options{ExpandAll: true}, nil)
	details := 0
	for _, st := range *seen {
		if !strings.Contains(st.sql, "ws AS (") {
			continue
		}
		details++
		if rollupCTE.MatchString(st.sql) {
			t.Errorf("the detail statement has a roll-up:\n%s", st.sql)
		}
	}
	if details == 0 {
		t.Fatal("no detail statement read the shown groups (ws)")
	}
}

// The Total of a reader who sees no row: the roll-up of no rows is the total
// of no rows (a count 0, a blank min), one row when no amount gives the
// report a currency.
func TestTheRollUpOfNoRowsIsTheTotalOfNoRows(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	res := z.run(t, "eve", countedMeals, Options{}, nil)
	if got := len(res.Rows); got != 1 {
		t.Fatalf("eve sees %d rows, want the Total alone", got)
	}
	for _, sql := range levelStatements(t, *seen) {
		if !rollupCTE.MatchString(sql) {
			t.Errorf("the statement does not roll up:\n%s", sql)
		}
	}
	requireRows(t, res, "times", int64(0))
	requireRows(t, res, "first", nil)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run (in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports`): `go test -run 'TestLevelsRollUp|TestLevelsAboveALevel|TestALevelWhoseValues|TestRolledUp|TestAMixedCheck|TestTheDetailStatementTotals|TestTheRollUpOfNoRows' -count=1 .`

Expected: a build failure, `legacySQL.aggregateEachLevel undefined`. Add the field to `legacy.go` (step 3, first block) and run again. Expected:

```
--- FAIL: TestLevelsRollUpFromOneTotalOfTheRows
    rollup_test.go: the statement reads w 3 times, want 1:
--- FAIL: TestAMixedCheckAndAFillRollUpWithTheSameResult
    rollup_test.go: the mixed check is not a value of the roll-up
```

`TestLevelsAboveALevelThatTotalsTheRowsStillRollUp` and `TestTheRollUpOfNoRowsIsTheTotalOfNoRows` fail too (`the statement has 0 roll-ups, want 2`, `the statement does not roll up`). `TestALevelWhoseValuesDoNotRollUpTotalsTheRows`, `TestRolledUpLevelsKeepTheResult` and `TestTheDetailStatementTotalsTheRowsItself` pass already (the switch changes nothing yet): they are the guards for step 3. The planner ran the tests green with step 3 on 2026-11-18 (tasks 2 and 3 applied through `go test -overlay`); the red was not run, because the switch gives the same SQL (the last block of `TestLevelsRollUpFromOneTotalOfTheRows` requires 3 totals of `w` with the switch on, and passes).

- [ ] **Step 3: Implement**

In `legacy.go`, add to the `legacySQL` struct, after `rowValuesPerLevel bool`:

```go
	// aggregateEachLevel makes every level total w itself, as before the
	// levels rolled up from one total of w (compiler.aggregateCTEs).
	aggregateEachLevel bool
```

In `levels.go`, in `type level`, replace the line `aggs     []string // a<L> values, "expr AS name"` with:

```go
	aggs     []levelAgg // a<L> values
	// rollFrom is the CTE a<L> takes its values from when the level rolls up
	// (compiler.aggregateCTEs): ag<L>, which holds one row for each of the
	// level's rows already. rolled names each value's column there. "" for a
	// level that totals w itself.
	rollFrom string
	rolled   map[string]string
```

Replace `func (lv *level) agg` whole (its comment stays) and add the rest after it:

```go
func (lv *level) agg(name, sql string) string {
	if !lv.aggNames[name] {
		lv.aggNames[name] = true
		lv.aggs = append(lv.aggs, levelAgg{name: name, sql: sql})
	}
	return "a." + name
}

// levelAgg is one value of a<L>: sql totals w. op and arg (the argument, over
// w) are set for a value that rolls up; op is "" for one that every level
// totals from w itself.
type levelAgg struct {
	name, sql string
	op        Aggregate
	arg       string
}

// rollsUp is true for an aggregate whose value over a group is the same
// aggregate of its values over the group's parts: a count (the sum of the
// counts), a min, a max, and a sum of amounts. Amounts are whole cents, so
// their sum is exact in any order; a sum of other numbers may hold fractions,
// whose sum depends on the order they are added in. An average and a count of
// distinct values are not made of their parts' values. A min or a max of the
// parts' values compares with BINARY, the one collation the engine writes.
func rollsUp(op Aggregate, typ FieldType) bool {
	switch op {
	case Count, Min, Max:
		return true
	case Sum:
		return typ == Money
	}
	return false
}

// aggOf adds the aggregate op of arg (a value of w, of type typ) to a<L>, as
// agg does, and keeps what a roll-up needs when the aggregate rolls up.
func (lv *level) aggOf(name string, op Aggregate, arg string, typ FieldType, scale int) string {
	added := !lv.aggNames[name]
	ref := lv.agg(name, aggregateSQL(op, arg, scale))
	if added && rollsUp(op, typ) {
		a := &lv.aggs[len(lv.aggs)-1]
		a.op, a.arg = op, arg
	}
	return ref
}

// rollsUp is true when every value of a<L> rolls up.
func (lv *level) rollsUp() bool {
	return !slices.ContainsFunc(lv.aggs, func(a levelAgg) bool { return a.op == "" })
}

// rollupSQL is the aggregate op over the values z of a finer level.
func rollupSQL(op Aggregate, z string) string {
	if op == Min || op == Max {
		return string(op) + "(" + z + ")"
	}
	return "COALESCE(sum(" + z + "), 0)"
}

// aggregateCTEs makes the levels that roll up read one total of w: ag<top>
// totals w by the keys of the deepest such level, with every value any of
// them reads, and each level above it totals the ag of the level below it
// (ag<L>). a<L> then selects its values from ag<L>. A level with a value that
// does not roll up totals w as before, and so does a statement with fewer
// than two levels that roll up. The CTEs go before the levels' own. Each ag
// is materialized: a<L> and the ag above it both read it, and SQLite would
// total w again for each.
func (c *compiler) aggregateCTEs(levels []compiledLevel) []string {
	if legacySQL.aggregateEachLevel {
		return nil
	}
	var rolling []*level
	for _, l := range levels {
		if l.lv.rollsUp() {
			rolling = append(rolling, l.lv)
		}
	}
	if len(rolling) < 2 {
		return nil
	}
	type column struct {
		name, arg string
		op        Aggregate
	}
	var cols []column
	byKey := map[string]string{}
	for _, lv := range rolling {
		lv.rolled = map[string]string{}
		for _, a := range lv.aggs {
			key := string(a.op) + "|" + a.arg
			z, ok := byKey[key]
			if !ok {
				z = fmt.Sprintf("z%d", len(cols))
				byKey[key] = z
				cols = append(cols, column{name: z, arg: a.arg, op: a.op})
			}
			lv.rolled[a.name] = z
		}
	}
	ctes := make([]string, 0, len(rolling))
	from := "w"
	for _, lv := range slices.Backward(rolling) {
		sel := slices.Clone(lv.keys)
		for _, col := range cols {
			value := rollupSQL(col.op, col.name)
			if from == "w" {
				value = aggregateSQL(col.op, col.arg, 0)
			}
			sel = append(sel, value+" AS "+col.name)
		}
		n := "COALESCE(sum(__n), 0) AS __n"
		if from == "w" {
			n = "count(*) AS __n"
		}
		lv.rollFrom = fmt.Sprintf("ag%d", lv.n)
		ctes = append(ctes, fmt.Sprintf("%s AS MATERIALIZED (SELECT %s FROM %s%s)", lv.rollFrom, strings.Join(append(sel, n), ", "), from, groupBy(lv.keys)))
		from = lv.rollFrom
	}
	return ctes
}

// aggregateRows selects a<L>'s rows: the level's keys, its values and __n,
// the count of the base rows of each group, which also keeps the select of a
// level with no aggregate valid.
func (lv *level) aggregateRows() string {
	cols := slices.Clone(lv.keys)
	if lv.rollFrom != "" {
		for _, a := range lv.aggs {
			cols = append(cols, lv.rolled[a.name]+" AS "+a.name)
		}
		return fmt.Sprintf("SELECT %s FROM %s", strings.Join(append(cols, "__n"), ", "), lv.rollFrom)
	}
	for _, a := range lv.aggs {
		cols = append(cols, a.sql+" AS "+a.name)
	}
	return fmt.Sprintf("SELECT %s FROM w%s", strings.Join(append(cols, "count(*) AS __n"), ", "), groupBy(lv.keys))
}
```

`aggregateSQL` reads its scale only for `avg`, which does not roll up, so the `0` there changes nothing. The names `z<i>` and `ag<L>` are free (`w` has `x<n>`; a level's CTEs are `a`, `d`, `f`, `g`, `gr`, `il`, `lk`, `o`, `rs`, `rt`, `s` with the level number).

The callers, each one line (the text before `→` is in the tree, the text after it replaces it):

In `func (lv *level) shownKey`:
`return lv.agg(fmt.Sprintf("kl%d", i), fmt.Sprintf("min(w.kl%d)", i))` → `return lv.aggOf(fmt.Sprintf("kl%d", i), Min, fmt.Sprintf("w.kl%d", i), Text, 0)`

In `func (lc *levelCtx) rowsTotal`:
`total := expr{sql: lv.agg(name, "COALESCE(sum("+d.sql+"), 0)"), typ: d.typ}` → `total := expr{sql: lv.aggOf(name, Sum, d.sql, d.typ, 0), typ: d.typ}`

In `func (lc *levelCtx) recompute`, the `case cp.path != nil && cp.col.Aggregate != "":` line:
`return withCurrency(expr{sql: lv.agg(name, aggregateSQL(cp.col.Aggregate, fmt.Sprintf("w.c%d", idx), cp.scale)),` → `return withCurrency(expr{sql: lv.aggOf(name, cp.col.Aggregate, fmt.Sprintf("w.c%d", idx), cp.path.field.Type, cp.scale),`

In `func (lc *levelCtx) recompute`, near its end:
`total := expr{sql: lv.agg(name, aggregateSQL(cp.col.Aggregate, e.sql, cp.scale)), typ: aggregateType(cp.col.Aggregate, e.typ, cp.scale)}` → `total := expr{sql: lv.aggOf(name, cp.col.Aggregate, e.sql, e.typ, cp.scale), typ: aggregateType(cp.col.Aggregate, e.typ, cp.scale)}`

In `func (lv *level) mixedTotal`:
`total.mixed = lv.agg(name+"m", fmt.Sprintf("max(CASE WHEN %s THEN 1 END)", blank)) + " IS NOT NULL"` → `total.mixed = lv.aggOf(name+"m", Max, fmt.Sprintf("CASE WHEN %s THEN 1 END", blank), Number, 0) + " IS NOT NULL"`

In `func (lc *levelCtx) aggregate` (as task 3 left it):
`total := expr{sql: lv.agg(name, aggregateSQL(op, e.sql, scale)), typ: aggregateType(op, e.typ, scale)}` → `total := expr{sql: lv.aggOf(name, op, e.sql, e.typ, scale), typ: aggregateType(op, e.typ, scale)}`

Each of the six writes the same SQL for `a<L>` as before (`aggregateSQL(Sum, x, 0)` is `COALESCE(sum(x), 0)`, `aggregateSQL(Min, x, 0)` is `min(x)`). Do not change `toManyTotal` or `pivot.go`.

In `func (lv *level) base`, replace the three lines from `keyCols := slices.Clone(lv.keys)` to `rows := fmt.Sprintf("SELECT %s FROM w%s", …)` (and the comment between them, which is now on `aggregateRows`) with:

```go
	rows := lv.aggregateRows()
```

In `fill.go`, `func (lv *level) fillRows`, replace the line `empty := fmt.Sprintf("SELECT %s FROM w WHERE 0", strings.Join(append(slices.Clone(lv.aggs), "count(*) AS __n"), ", "))` with:

```go
	// The totals of no rows are read from w also when the level rolls up: the
	// roll-up of no rows gives the same values, and w is named here anyway.
	aggs := make([]string, 0, len(lv.aggs)+1)
	for _, a := range lv.aggs {
		aggs = append(aggs, a.sql+" AS "+a.name)
	}
	empty := fmt.Sprintf("SELECT %s FROM w WHERE 0", strings.Join(append(aggs, "count(*) AS __n"), ", "))
```

and remove `"slices"` from the imports of `fill.go` (`go vet` names it when another use remains: then keep it).

In `run.go`, `func (c *compiler) pagedGroupStatement`, between the loop that sets `l.lv.paged = true` and the loop that renders the levels:

```go
	// The roll-up names where each a<L> reads, so it comes before the render.
	levelCTEs = c.aggregateCTEs(levels)
```

In `run.go`, `func (c *compiler) groupStatement`, after the line `unread := c.unreadCells(levels, computed)`:

```go
	ctes = append(ctes, c.aggregateCTEs(levels)...)
```

Do not call `aggregateCTEs` in `shownLevels`: the detail statement's levels total `w` themselves (Review Focus 4).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go vet . && go test -run 'TestLevelsRollUp|TestLevelsAboveALevel|TestALevelWhoseValues|TestRolledUp|TestAMixedCheck|TestTheDetailStatementTotals|TestTheRollUpOfNoRows' -count=1 .`
Expected: `ok`. (Run against the tree with tasks 2, 3 and 4 through `go test -overlay` on 2026-11-18: green.)

- [ ] **Step 5: Update the three tests that compare the SQL of before, and run the report packages**

`aggonce_test.go`, `TestAnAggregateALevelReadsTwiceIsComputedOnce`, and `rowonce_test.go`, `TestAnAggregateArgumentIsComputedOncePerRow`: each counts text in `a<L>`, which now selects from `ag<L>`. In each, add after the first line of that test, `z := newUnionZoo(t)`:

```go
	// Each level totals w here, so a<L> shows the level's aggregates; the
	// roll-up has its own test (TestLevelsRollUpFromOneTotalOfTheRows).
	legacySQL.aggregateEachLevel = true
	t.Cleanup(func() { legacySQL.aggregateEachLevel = false })
```

`layout_cost_test.go`: replace `TestGroupStatementMaterializesTheRowsItReadsTwice` and its comment with:

```go
// A grouped statement whose levels roll up reads w one time, for the total of
// its deepest level: w stays inline, and that total is materialized for the
// levels that read it.
func TestGroupStatementTotalsTheRowsOnce(t *testing.T) {
	m := measureRun(t, balanceSheetZoo(t, 40), benchBalanceSheet())
	if got := len(rowsRead.FindAllString(m.sql, -1)); got != 1 || strings.Contains(m.sql, "w AS MATERIALIZED (") {
		t.Fatalf("the group statement reads w %d times, want 1 with w inline:\n%s", got, m.sql)
	}
	if !regexp.MustCompile(`(?m)^ag\d+ AS MATERIALIZED \(`).MatchString(m.sql) {
		t.Fatalf("the group statement does not materialize the total its levels read:\n%s", m.sql)
	}
}
```

(`layout_cost_test.go` imports `regexp` and `strings` already. `TestMaterializeSharedMarksRowsReadTwice` in the same file, which tests `materializeShared` with written CTEs, does not change.)

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -count=1 .`
Expected: `ok` in about 105 s (run on 2026-11-18 with tasks 2, 3 and 4 and the three updates: `ok`, no other expectation changed). Another test that fails because it compares SQL text is updated to the new text; a test that fails on a result (a cell, a row, a notice) is a defect of this task: fix the code, never the expectation.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'Report|ARAging' -count=1 .`
Expected: `ok` (run on 2026-11-18 with tasks 2, 3 and 4: `ok`; the small plan test meets the plan bars with `ag<L>` materialized). A plan-bar failure that names a `MATERIALIZE ag<L>` or a `CO-ROUTINE` step inside a loop is a defect of this task: report the plan lines and stop.

- [ ] **Step 6: Measure**

Run task 1's S profile to `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-S-task4.txt` (the command of part 1, task 1, step 3 with that file name; `NEEDS-RUN` when it passes 4 minutes). In your report give, for run 1, statement 4 (the page) and statement 5 (the Total) before (from `profile-S-task3.txt`) and after; the count of ` FROM w GROUP BY ` in statement 4 before and after (expected 4 → 1); whether statement 4 has `w AS MATERIALIZED` (expected: no); and the cumulative time of each `ag<L>` from the profile's CTE lines. Expected: statement 4 lower by 150 ms or more. Statement 5 may rise by some tens of ms (it now groups by the deepest level's keys before it rolls up to the Total): task 5 removes that statement, so a rise there does not fail the task; report it. A saving under 60 ms in statement 4 is reported as such with the plan lines of statement 4; it does not fail the task. When statement 4 still has `w AS MATERIALIZED`, name the second reader of `w` (a level with `op == ""` in its `aggs`, a `d<L>`, a fill) in the report: the driver gives it to the planner of the next part.

- [ ] **Step 7: Commit (core)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/legacy.go core/server/reports/levels.go core/server/reports/fill.go core/server/reports/run.go core/server/reports/rollup_test.go core/server/reports/aggonce_test.go core/server/reports/rowonce_test.go core/server/reports/layout_cost_test.go
git commit -m "perf(reports): a statement totals its rows one time and rolls the levels up"
```

(Add any other test file whose SQL-text expectation step 5 updated.)

---

## What is left (part 4)

Tasks 5 to 10 of the outline: one statement for the page and the Total (the page statement holds `a0`/`o0` already and reads `ag0` after this part); filters and opened keys pushed into union parts; the one-evaluation export; the Books views and indexes; the scale run and the ledger record with the stop rule. Each engine task adds its field to `legacySQL`, one test that the switch writes the old SQL, and its cross-check with `requireSameAsLegacy`.
