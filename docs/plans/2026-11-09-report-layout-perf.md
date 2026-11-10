# Report Layout Statement Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Balance-Sheet-shaped layout report runs in tens of milliseconds instead of seconds: its statement reads the lines rule about 5 times instead of 1,495 and is under 100 KB instead of 1 MB; a group the reader opens shows at once that it is loading.

**Architecture:** In the layout compiler (core/server/reports/layout.go) each distinct lookup that reads no layout row is compiled once into a one-row `MATERIALIZED` CTE `lx<n>` (value, mixed check and currency in one scan of the looked-up rows); one more one-row CTE `lx` collects all their columns, and each computed row CROSS JOINs `lx` (one join, whatever the number of lookups) and reads by column. Each computed-row cell is compiled once and reused (with the sections, lookups and probes it needs replayed into the reader). The report-rows CTE `w` is `MATERIALIZED` when a statement reads it more than once. The rule CTEs stay inline (no global B: a materialized rule CTE has no index, and correlated per-row lookups lose `idx_zoo_ledger_lines_account_date`). On the client, `withPendingOpen` (core/reports/lib/rows.ts) marks a closed group the current expansion asks open while the run is updating; it reads as open (`aria-expanded`) and an `ActivityIndicator` takes the chevron's place in the fixed caret slot, so the label does not move. A new core migration indexes `org_pkg_access(user, pkg)` (Task 7, a separate later PR).

**Tech Stack:** Go 1.26 + PocketBase fork + modernc SQLite (>= 3.35, honours `MATERIALIZED`); TypeScript, React Native / react-native-web, FlashList v2, vitest, Playwright.

**Spec:** the measurements and the approved scope below. Source files (outside the repo, read them for detail): `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-balance-sheet/perf/deep-dive.txt` (byte breakdown, benchmarks, planner and index audit), `perf/findings.md`, `perf/proto-a1scan-b-w.patch` (the measured prototype of A1s + B + W; its env switches must not ship).

Approved scope (human, 2026-11-09), copied:

1. A1s + W: each distinct lookup compiled once into a one-scan MATERIALIZED CTE (sum, currency-mixed check and currency in one scan), computed cells read it; the main row set `w` MATERIALIZED; derived columns and computed rows that read earlier rows stop re-compiling (layout.go:469-531), and the mixed-currency guard / fieldGuard pairwise min(currency) copies (formula.go:211-239, 308-313, 436-451) stop multiplying. No global B.
2. Index on org_pkg_access(user, pkg) — a core migration. Its own task at the END of the plan so it can be split out into a second PR if needed (a core migration needs the `exclusive` hold, which the lane does not have now). Decided 2026-11-09: it is split out; this PR builds Tasks 0-6, and Task 7 is a separate, later PR.
3. Spinner C: a row the user opened shows its chevron turned at once and an ActivityIndicator in the GroupCaret slot (ReportRow.tsx:197) until the run that includes it lands; derive per-row pending state in useReportView from the expansion store + isUpdating, pass through ReportTable to ReportRow. Web and native. Covered by a core e2e at desktop.
4. Keep the benchmark (layout_bench_test.go, tidy it) and add a regression test that counts rule-CTE reads / statement size for a balance-shaped layout report, so a regression fails CI. Bench must not slow `go test ./...`; the count test must be fast.
5. Measure at 30,000 lines whether day comparisons via substr() (stmt.go:235) block an index that would matter; change the comparison only if it measurably gains and stays correct for text days and date/autodate columns. The outcome is recorded either way.

How item 1 is met: after the hoist, the min(currency), count(DISTINCT) and sum subqueries are one scan each in `lx<n>`; the guards that `fieldGuard`, `mixedOver` and `compileFormula` still copy now copy `lx.c<n>` / `lx.m<n>` / `lx.u<n>` (5-6 bytes each), not a 700-byte subquery, so they no longer multiply the statement (prototype: 1,004,129 B -> 72,373 B). Task 2 makes each computed-row cell compile once in Go. A per-row CTE that would also remove the remaining text copies (A2 in the deep dive) is not built: measured bound at most 4-5 ms per run, and it moves derived columns of computed rows into a second select.

## Global Constraints

- Run Go from `core/server` with `GOWORK=off`: `cd /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-balance-sheet/core-perf/core/server && GOWORK=off go test ./reports/ -run '<Name>' -count=1`.
- No env switches, no `os.Getenv` in product code (the prototype's `PROTO_*` switches do not ship).
- Never materialize a rule CTE (`r<n>`). `w` only when the statement reads it two or more times.
- `go test ./...` must not run a benchmark; every new test in `reports` finishes in under ~5 s on a laptop.
- A lookup that reads a `{subtotal}`, a `{col}`, `cur.` or a section chunk `ls<k>` stays inline (it is per row and per currency).
- Same results: every existing test in `core/server/reports` passes unchanged (`GOWORK=off go test ./reports/ -count=1`, about 90 s; the driver runs the full suite).
- Client: no `useState`/`useEffect` for the pending state (compute during render); no raw hex (use `useThemeColor('muted-foreground')`); no component unit tests (behavior is covered by e2e); biome 4-space indent, single quotes, no semicolons.
- E2E: `E2E_PORT=7230 E2E_PORT_2=7231` before every e2e command; run one file only. Check the ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7230|7231)'`).
- Core migrations up to `2090000012_*.js` are released or unreleased per git tags; `1700000000_create_core_collections.js` (org_pkg_access) is in v0.10.0, so the index is a NEW migration.
- Make no commits unless the runner says so; the commit steps below name the files.

## Review Focus

1. A lookup whose where reads an asked param that is unset (`unset`), or a fiscal-year bound: the hoisted CTE binds the same params as the inline form; the run gives the same figures (Task 1 value test reads RE and NI current and prior against direct SQL).
2. A sum lookup whose rows hold two currencies: the cell is blank and the run names the condition in its mixedCurrency notice, through the hoisted CTE (Task 1 relies on the existing `emptycurrency`, `blankcurrency`, `valuecurrency`, `fieldcurrency` tests; Task 1 step 5 runs them by name).
3. A computed row that reads the same earlier row in many cells or many times (derived Change / % change, totals of totals): each cell compiles once and the statement is unchanged (Task 2 test asserts the compile count; the whole layout test set must still pass).
4. A row that reads many distinct lookups: SQLite joins at most 64 tables, so all hoisted lookups are read through one one-row CTE `lx` (one join); past `maxLookupColumns` a lookup is read inline as before; the row still runs (Task 1 tests: 70 lookups, the column cap lowered to 5, and a level having that reads a lookup, whose level CTEs are named `lk<L>`).
5. A group of an open-by-default level that the server sends closed while a live re-run is in progress would show the spinner during that re-run. `withPendingOpen` only marks rows while `isUpdating` and only toggle rows; Task 6 unit-tests the default, Expand-all, explicit-key and folded cases.

---

### Task 0: Tidy the layout benchmark harness

**Files:**
- Modify: `core/server/reports/statement_test.go:53` (`rawStatement` takes `testing.TB`)
- Add to git (exists, untracked): `core/server/reports/layout_bench_test.go` (tidy: use `rawStatement`, drop `benchRaw`, add `benchRunWith`)

**Interfaces:**
- Consumes: bench helpers in `layout_bench_test.go`: `newZoo(tb)`, `withBalanceZoo(tb, z)`, `withBenchLedger(tb, z, lines int)`, `benchBalanceSheet() string`, `measureRun(tb, z, raw string) benchStatement`, `benchRun(z, raw string) (*Result, error)`, `benchAsOf`.
- Produces: `rawStatement(tb testing.TB, stmt string, params dbx.Params) (string, []any)`; `benchRunWith(z *zoo, raw string, params map[string]json.RawMessage) (*Result, error)`; `benchRun(z *zoo, raw string)` now calls `benchRunWith` with `{"asOf": benchAsOf}`. Tasks 1, 3 and 4 use them.

- [ ] **Step 1: Make `rawStatement` take `testing.TB` and drop `benchRaw`**

In `statement_test.go:53` change the signature to `func rawStatement(tb testing.TB, stmt string, params dbx.Params) (string, []any)` and rename `t` to `tb` inside (it only calls `Helper`/`Fatalf`). In `layout_bench_test.go` delete `benchRaw` (lines 300-323) and in `measureRun` replace `stmt, args := benchRaw(tb, out.sql, out.params)` with `stmt, args := rawStatement(tb, out.sql, out.params)`. Keep the rest of the bench file: it is the harness the human asked to keep.

- [ ] **Step 2: Split `benchRun`**

Split `benchRun` into `benchRunWith(z *zoo, raw string, params map[string]json.RawMessage) (*Result, error)` (the body of today's `benchRun`, with `Params: params`) and `benchRun(z *zoo, raw string)`, which calls it with `{"asOf": benchAsOf}`. Run refuses a param the spec does not declare (params.go:156), so a spec without `asOf` uses `benchRunWith(z, raw, nil)`.

- [ ] **Step 3: Check it builds and the benchmark still runs**

Run: `GOWORK=off go vet ./reports/ && LAYOUT_BENCH_LINES=300 GOWORK=off go test ./reports/ -run '^$' -bench 'BenchmarkLayoutStatement/balanceSheet/' -benchtime 1x`
Expected: no vet output; the benchmark prints `stmt-bytes` about 1,004,129 and `rule-reads` 1,495 (the baseline). `GOWORK=off go test ./reports/ -run 'Statement' -count=1` gives `ok`.

- [ ] **Step 4: Commit**

```bash
git add core/server/reports/statement_test.go core/server/reports/layout_bench_test.go
git commit -m "test(reports): keep the layout statement benchmark"
```

---

### Task 1: Hoist each layout lookup into one MATERIALIZED one-scan CTE, with the regression test

**Files:**
- Modify: `core/server/reports/formula.go:47-62` (add `scan` to `expr`)
- Modify: `core/server/reports/lookup.go:270-289` (`summedLookup` builds `scan`)
- Modify: `core/server/reports/layout.go:28` (init), `:147` (return), `:172` (reset), `:226-228` (join cap comment), `:245-255` (`joins`), `:261-300` (struct), `:704-709` (`lookup`)
- Create: `core/server/reports/layout_cost_test.go`

**Interfaces:**
- Consumes: Task 0's `rawStatement`, `benchRun`, `measureRun`; `lookupIn(outer fctx, r *rowsCTE, n *Node, at string) (expr, error)` (lookup.go:107); `(e expr) guarded(mixed, value string) expr` (lookup.go:248); `(p *plan) lookupOf(n *Node, at string) (*Node, error)` (lookup.go:40); `condProbe{at, sql string}` (run.go:974); `layoutOf`, `lions` (layout_test.go:83, 90); `observeReads` (statement_test.go:332).
- Produces: `expr.scan string`; `layoutCtx.lookups map[*Node]layoutLookup`, `layoutCtx.lookupCTEs []string`, `layoutCtx.lkUsed []string`; `layoutCtx.lookupCols []string`; `type layoutLookup struct{ name string; e expr; wheres []condProbe }`; `(lc *layoutCtx) lookupsRoom() bool`; `(lc *layoutCtx) lookupRowCTEs() []string`; the CTEs `lx<n>` (one per lookup) and `lx` (one row, columns `u<n>`, `m<n>`, `c<n>`, `s<n>`, `sh<n>`), joined as `CROSS JOIN lx`; `(lc *layoutCtx) useLookup(name string)`; `readsSubtotal(n *Node, p *plan) bool`; `readsLayoutRow(sql string) bool`; `var maxLookupColumns`; test helpers `balanceSheetZoo(tb testing.TB, lines int) *zoo`, `manyLookups(count int) string`, `manyFigure`, `manyWant` in layout_cost_test.go.

- [ ] **Step 1: Write the regression and value tests**

Create `core/server/reports/layout_cost_test.go`:

```go
package reports

import (
	"fmt"
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

// manyWant is the sum the lookups of manyLookups give, by SQL over the rows.
func manyWant(t *testing.T, z *zoo, count int) int64 {
	t.Helper()
	var want int64
	for i := range count {
		var v int64
		if err := z.app.DB().NewQuery("SELECT COALESCE(SUM(grams), 0) FROM zoo_feedings WHERE grams >= {:i}").
			Bind(dbx.Params{"i": i}).Row(&v); err != nil {
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
// (levels.go:456) and share the WITH list with the layout's CTEs, so the
// hoisted lookups must not take those names.
func TestALayoutWithALookupInALevelHavingRuns(t *testing.T) {
	z := newZoo(t)
	having := `{"op":"gt","args":[{"op":"lookup","source":"zoo.feedings","value":{"path":[{"field":"grams"}]},"pick":"sum"},{"value":0}]}`
	raw := strings.Replace(manyLookups(3), `"groups":[{"id":"g","col":"species"}]`,
		`"groups":[{"id":"g","col":"species","having":`+having+`}]`, 1)
	res, err := z.try(t, "ann", raw, Options{}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := manyFigure(t, res), manyWant(t, z, 3); got != want || want == 0 {
		t.Fatalf("Many = %d, the rows give %d", got, want)
	}
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
```

Helpers used: `z.try` (run_test.go:26) returns the run's error; `observeReads` (statement_test.go:332); `toInt` (assemble.go:246) reads a cell as int64; `grams` is a Number field of `zoo.feedings` (zoo_test.go:188), so these lookups take the non-`scan` branch of Step 4 (no carried currency) and test it too. 70 lookups of about 7 nodes each are about 490 nodes, under `maxFormulaNodes` (1000, formula.go:302). If a cap refuses first (the error names `maxFormulaNodes` or a layout cap), lower the lookups' size, never the count of 70. If `Validate` refuses a `having` in a layout report, find the shape that puts a level `lk<L>` CTE in a layout statement (`grep -n "lv.pre\|lookupsIn" core/server/reports/*.go`) and use it; if none can, say so in the test's comment and keep the test as a guard. Check `manyWant`'s SQL against how `zoo_feedings.grams` is stored (zoo_test.go:188); if the lookup reads only the reader's rows, use the same where as the reader's rule.

The inline path for a lookup that reads `cur.` (Step 4 `readsLayoutRow`): find a spec that reaches it. Put a temporary `t.Log` where `lookup` returns early on `readsLayoutRow`, run `GOWORK=off go test ./reports/ -run 'Layout|Currency|AmountParam' -count=1 -v 2>&1 | grep <your marker>`, and name one test that hits it in the Step 5 `-run` list; then remove the log. If no test hits it, write `TestALayoutLookupThatReadsTheRowsCurrencyStaysInline` from a layout lookup whose value carries the computed row's currency key, and check its figure; if no spec can produce `cur.` in a layout lookup (the reviewer found that `inLevel(layoutCtx)` keeps `cur.key` off a lookup), write that in the `readsLayoutRow` comment ("a guard: no spec reaches it today") and keep `TestReadsLayoutRow`.

- [ ] **Step 2: Run the tests to see them fail**

Run: `GOWORK=off go test ./reports/ -run 'TestABalanceSheetShapedLayout|TestALayoutRowReadingManyLookups|TestALookupPastTheColumnCap|TestALayoutWithALookupInALevelHaving|TestReadsLayoutRow' -count=1 -v`
Expected: build failure "undefined: maxLookupColumns" and "undefined: readsLayoutRow". To see the count test red too, comment out `TestALookupPastTheColumnCapIsReadInline` and `TestReadsLayoutRow` for this one run: `...ReadsTheLinesRuleOncePerLookup` FAIL ("reads the lines rule 1495 times" or near); `...LookupsMatchTheLines`, `...ManyLookupsStillRuns` and `...LevelHavingRuns` PASS (they guard the figures, the 64-table limit and the CTE names through the change). Restore the two tests.

- [ ] **Step 3: Add `scan` to `expr` and build it in `summedLookup`**

In `formula.go`, after the `link storedLink` field of `expr` (line 61), add:

```go
	// scan is a sum lookup's value, mixed check and currency as one select
	// over the looked-up rows (columns u, m, c), for a context that computes
	// the lookup once in a CTE (layoutCtx.lookup). "" for any other value.
	scan string
```

In `lookup.go` `summedLookup`, after `out = out.guarded(mixed, sum)` (line 282), add:

```go
	if inner == "" {
		// One read of the matching rows: sum of no rows is NULL, count 0 and
		// min NULL, as the three subqueries above give.
		hit := "CASE WHEN " + v.sql + " IS NOT NULL THEN " + v.cur.sql + " END"
		out.scan = fmt.Sprintf("SELECT sum(%s) AS u, (count(DISTINCT %s) > 1) AS m, min(%s) AS c\n%s\nWHERE %s",
			v.sql, hit, hit, from, where)
	}
```

- [ ] **Step 4: Hoist the lookups in the layout compiler**

In `layout.go`:

(a) In `layoutCtx` (after `wheres []condProbe`, line 299) add:

```go
	// lookups are the layout's lookups that read no layout row, by node, each
	// computed once in its CTE lx<n> (lookupCTEs). One more CTE, lx, holds
	// all their columns in one row (lookupCols), so a computed row joins one
	// table for any number of lookups and stays under SQLite's 64 tables in a
	// join. lkUsed is ["lx"] when the computed row being compiled reads it.
	lookups    map[*Node]layoutLookup
	lookupCTEs []string
	lookupCols []string
	lkUsed     []string
```

and after the struct:

```go
// layoutLookup is a layout lookup computed once: e reads its columns of lx,
// wheres are the probes its where needs on every row that reads it, so the
// mixedCurrency notice still names the condition.
type layoutLookup struct {
	name   string
	e      expr
	wheres []condProbe
}

func (lc *layoutCtx) useLookup(name string) {
	if !slices.Contains(lc.lkUsed, name) {
		lc.lkUsed = append(lc.lkUsed, name)
	}
}

// maxLookupColumns keeps lx's one select under SQLite's 2,000 columns
// (SQLITE_MAX_COLUMN); a lookup past it is read inline, as before the hoist.
// A var so a test can reach the fallback with a few lookups.
var maxLookupColumns = 1000

// lookupsRoom is true while lx can take one more lookup's columns (at most 5).
func (lc *layoutCtx) lookupsRoom() bool { return len(lc.lookupCols)+5 <= maxLookupColumns }

// lookupRowCTEs are the lookups' CTEs and lx, the one row of their columns.
func (lc *layoutCtx) lookupRowCTEs() []string {
	if len(lc.lookupCols) == 0 {
		return nil
	}
	return append(slices.Clone(lc.lookupCTEs), "lx AS MATERIALIZED (SELECT "+strings.Join(lc.lookupCols, ", ")+")")
}
```

Rule: the prefix `lx` must be free. `lk<L>` is taken by levels.go:456 (the level CTE of a having that reads a lookup) and sits in the same WITH list as the layout CTEs (run.go:1121-1124), so `lk<n>` would give a duplicate name. Before writing the code, `grep -n '"lx\|lx%d\| lx ' core/server/reports/*.go` must show no hit; if it shows one, pick another free two-letter prefix and use it everywhere this plan says `lx`.

(b) Line 28: add `lookups: map[*Node]layoutLookup{}` to the `layoutCtx` literal.

(c) Line 147: `return append(lc.lookupRowCTEs(), lc.chunkCTEs()...), legs, nil`.

(d) Line 172: `lc.used, lc.wheres, lc.current, lc.lkUsed = nil, nil, item, nil`.

(e) Lines 226-228 stay as they are: lookups add one join at most (`lx`), so `maxJoinedChunks` (60) plus `cur` plus `lx` stays under 64 and no spec that ran before is refused. Add one sentence to the `maxJoinedChunks` comment: the select also joins `cur` and at most `lx`.

(f) In `joins()` after the chunk loop (line 253):

```go
	for _, name := range lc.lkUsed {
		fmt.Fprintf(&out, " CROSS JOIN %s", name)
	}
```

(g) Replace `lookup` (lines 704-709):

```go
// lookup reads its source's rule CTE in the layout statement. There is no
// report row to read: a column inside it is refused as above, and a param
// resolves in the lookup itself. A lookup that reads no layout row is the
// same on every computed row and currency, so it is computed once, in its
// own CTE, and each cell reads its columns: the cells and guards that copy
// it copy a column name, not its subqueries.
func (lc *layoutCtx) lookup(n *Node, at string) (expr, error) {
	if h, ok := lc.lookups[n]; ok {
		lc.useLookup(h.name)
		lc.wheres = append(lc.wheres, h.wheres...)
		return h.e, nil
	}
	before := len(lc.wheres)
	e, err := lookupIn(lc, &rowsCTE{c: lc.c, s: lc.s}, n, at)
	if err != nil || readsSubtotal(n, lc.c.p) || !lc.lookupsRoom() {
		return e, err
	}
	for _, sql := range []string{e.sql, e.mixed, e.unguarded, e.cur.sql, e.shown, e.scan} {
		if readsLayoutRow(sql) {
			return e, nil
		}
	}
	k := len(lc.lookupCTEs)
	name := fmt.Sprintf("lx%d", k)
	// pick puts column col of lx<k> into lx as <col><k> and returns its read.
	pick := func(col string) string {
		lc.lookupCols = append(lc.lookupCols, fmt.Sprintf("(SELECT %s FROM %s) AS %s%d", col, name, col, k))
		return fmt.Sprintf("lx.%s%d", col, k)
	}
	h := e
	h.scan = ""
	if e.scan != "" && e.cur.sql != "" {
		h.mixed, h.unguarded, h.cur.sql = pick("m"), pick("u"), pick("c")
		h = h.guarded(h.mixed, h.unguarded)
		lc.lookupCTEs = append(lc.lookupCTEs, fmt.Sprintf("%s AS MATERIALIZED (%s)", name, e.scan))
	} else {
		var cols []string
		ref := func(f *string, col string) {
			if *f == "" {
				return
			}
			cols = append(cols, *f+" AS "+col)
			*f = pick(col)
		}
		if h.mixed != "" && h.unguarded != "" {
			// sql is the guard over the two: read them, not a third copy.
			ref(&h.mixed, "m")
			ref(&h.unguarded, "u")
			h = h.guarded(h.mixed, h.unguarded)
		} else {
			ref(&h.sql, "s")
		}
		ref(&h.cur.sql, "c")
		ref(&h.shown, "sh")
		lc.lookupCTEs = append(lc.lookupCTEs, fmt.Sprintf("%s AS MATERIALIZED (SELECT %s)", name, strings.Join(cols, ", ")))
	}
	lc.lookups[n] = layoutLookup{name: "lx", e: h, wheres: slices.Clone(lc.wheres[before:])}
	lc.useLookup("lx")
	return h, nil
}

// readsSubtotal is true when a lookup's where or value reads a layout row
// (a {subtotal}) or a column, which differ by computed row.
func readsSubtotal(n *Node, p *plan) bool {
	if n == nil {
		return false
	}
	if n.Subtotal != "" || n.Col != "" {
		return true
	}
	if n.Op == "lookup" {
		if v, err := p.lookupOf(n, ""); err == nil && readsSubtotal(v, p) {
			return true
		}
	}
	if readsSubtotal(n.Where, p) {
		return true
	}
	return slices.ContainsFunc(n.Args, func(a *Node) bool { return readsSubtotal(a, p) })
}

// layoutRowRef finds a read of the computed row's currency (cur.) or of a
// section chunk (ls<k>.) in compiled SQL.
var layoutRowRef = regexp.MustCompile(`\b(?:cur|ls\d+)\.`)

// readsLayoutRow is true when compiled SQL reads the computed row's own
// currency or a section chunk, so it cannot leave the row's select.
func readsLayoutRow(sql string) bool { return layoutRowRef.MatchString(sql) }
```

Add `"regexp"` to the imports of layout.go. Note why the `else` branch exists: a lookup that is not a carried-currency sum (count, min, max, a sum with an inner mixed check, a pickedAmount) has no `scan`; it is still computed once, as `SELECT <its subqueries> AS s|m|u|c|sh`, so its copies are column names too. `lx` reads each `lx<n>` up to three times by scalar subquery, so `lx<n>` is `MATERIALIZED` (one scan), and `lx` is `MATERIALIZED` so the legs read its one row, not the subqueries again. The `lx<n>` CTEs read `FROM r<n> ` so `stmt.topics` still finds their rule (stmt.go:184).

- [ ] **Step 5: Run the tests and the currency and layout tests by name**

Run: `GOWORK=off go test ./reports/ -run 'TestABalanceSheetShapedLayout|TestALayoutRowReadingManyLookups|TestALookupPastTheColumnCap|TestALayoutWithALookupInALevelHaving|TestReadsLayoutRow|LevelLookup|Having|AmountParam|Layout|Within|Currency|Ragged|HideEmpty|ABalanceShaped|Statement|Cells|LookupHoist' -count=1`
Expected: `ok`. If a statement golden (`statement_test.go` or `ledger` golden) fails only because the SQL text changed, read the golden's comment: update the golden text only if the test pins text, not figures; never change an expected figure.

- [ ] **Step 6: Check the size at 300 lines with the benchmark**

Run: `LAYOUT_BENCH_LINES=300 GOWORK=off go test ./reports/ -run '^$' -bench 'BenchmarkLayoutStatement/balanceSheet/' -benchtime 1x`
Expected: `stmt-bytes` about 72,000, `rule-reads` 5 (baseline 1,004,129 and 1,495).

- [ ] **Step 7: Commit**

```bash
git add core/server/reports/formula.go core/server/reports/lookup.go core/server/reports/layout.go core/server/reports/layout_cost_test.go
git commit -m "perf(reports): compute each layout lookup once, in one scan"
```

---

### Task 2: Compile each computed-row cell once and reuse it

**Files:**
- Modify: `core/server/reports/layout.go:172` (reset), `:261-300` (struct), `:304-307` (`probeWhere`), `:414-431` (`cellOf`), `:536-550` (`subtotal` chunk use), Task 1's `lookup` and `useLookup`
- Test: `core/server/reports/layout_cost_test.go`

**Interfaces:**
- Consumes: Task 1's `layoutCtx.lkUsed`, `useLookup(name string)`, `layoutLookup.wheres`.
- Produces: `type cellKey struct{ item *LayoutItem; cell string }`; `type layoutDeps struct{ chunks []int; wheres []condProbe; lookups []string }`; `type cellMemo struct{ e expr; deps layoutDeps }`; `layoutCtx.cells map[cellKey]cellMemo`; `layoutCtx.recording []*layoutDeps`; `(lc *layoutCtx) useChunk(k int)`; `(lc *layoutCtx) addWhere(p condProbe)`; `(lc *layoutCtx) replay(d layoutDeps)`.

- [ ] **Step 1: Write the failing test**

Append to `layout_cost_test.go`:

```go
// A computed row that reads an earlier row many times, and derived columns
// that read a row's own cells, compile the earlier cell once: the compiler
// reuses it, so the nodes it compiles stay far under what the caps count
// (each read counted in full).
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
```

- [ ] **Step 2: Run it to see it fail**

Run: `GOWORK=off go test ./reports/ -run TestLayoutCompilesEachEarlierRowCellOnce -count=1 -v`
Expected: FAIL, "compiled N formula nodes for M counted" with N close to M.

- [ ] **Step 3: Record what a cell needs, and reuse it**

In `layout.go`:

(a) Struct fields (after `lkUsed`):

```go
	// cells holds each computed row's cell in each column once compiled, with
	// what its select must join and probe (deps); recording are the cells
	// being compiled now, innermost last, each collecting its deps.
	cells     map[cellKey]cellMemo
	recording []*layoutDeps
```

and types:

```go
// cellKey is one computed row's cell in one result column ("" is the check
// pass; pv<j> a pivot cell). A row's cell compiles the same in its own leg
// and inlined into a later reader (inline sets the row's own place), so one
// compile serves both.
type cellKey struct {
	item *LayoutItem
	cell string
}

// layoutDeps is what a compiled cell needs from the select that reads it:
// the section chunks and hoisted lookups to join, and the lookup wheres to
// probe on its rows.
type layoutDeps struct {
	chunks  []int
	wheres  []condProbe
	lookups []string
}

type cellMemo struct {
	e    expr
	deps layoutDeps
}

func (lc *layoutCtx) useChunk(k int) {
	for _, d := range lc.recording {
		d.chunks = append(d.chunks, k)
	}
	if !slices.Contains(lc.used, k) {
		lc.used = append(lc.used, k)
	}
}

func (lc *layoutCtx) addWhere(p condProbe) {
	for _, d := range lc.recording {
		d.wheres = append(d.wheres, p)
	}
	lc.wheres = append(lc.wheres, p)
}

// replay gives the select being compiled what a reused cell needs, and the
// cells being compiled around it too.
func (lc *layoutCtx) replay(d layoutDeps) {
	for _, k := range d.chunks {
		lc.useChunk(k)
	}
	for _, p := range d.wheres {
		lc.addWhere(p)
	}
	for _, name := range d.lookups {
		lc.useLookup(name)
	}
}
```

(b) Make every write to `used`, `wheres` and `lkUsed` go through these helpers so recorders see it:
- `probeWhere`: `lc.addWhere(condProbe{at: at, sql: sql}); return true`.
- `subtotal` lines 548-550: `lc.useChunk(slot.chunk)`.
- Task 1's `lookup` cache hit: replace `lc.wheres = append(lc.wheres, h.wheres...)` with `for _, p := range h.wheres { lc.addWhere(p) }`. The first compile's wheres already went through `probeWhere` (so `addWhere`); keep `slices.Clone(lc.wheres[before:])`.
- `useLookup`: also append `name` to every recorder: `for _, d := range lc.recording { d.lookups = append(d.lookups, name) }` before the `slices.Contains` check.

(c) `computedLeg` line 172 stays as Task 1 left it; `lc.recording` is empty between legs. Init `cells: map[cellKey]cellMemo{}` in the `layoutCtx` literal (line 28).

(d) Rename the body of `cellOf` to `compileCell` and make `cellOf` the memo:

```go
// cellOf is item's cell in the column being compiled (lc.cell), compiled
// once: a later read reuses it and replays what it joins and probes.
func (lc *layoutCtx) cellOf(item *LayoutItem, at string) (expr, error) {
	key := cellKey{item: item, cell: lc.cell}
	if m, ok := lc.cells[key]; ok {
		lc.replay(m.deps)
		return m.e, nil
	}
	deps := &layoutDeps{}
	lc.recording = append(lc.recording, deps)
	e, err := lc.compileCell(item, at)
	lc.recording = lc.recording[:len(lc.recording)-1]
	if err != nil {
		return expr{}, err
	}
	lc.cells[key] = cellMemo{e: e, deps: *deps}
	return e, nil
}

// compileCell is item's cell in the column being compiled: its cells
// formula for that column, a derived column's formula over the row's own
// cells, else its formula, else blank.
func (lc *layoutCtx) compileCell(item *LayoutItem, at string) (expr, error) {
	// (the former body of cellOf, lines 418-430, unchanged)
}
```

Why it is safe: the key's column fixes `lc.typ`, `lc.column`, `lc.idx`, `lc.pivotIdx` (all set from the column in `computedLeg` and `derivedCtx.col`); `inline` sets `lc.current` and `lc.within` to the row's own place, which equal what its own leg had (`withinAt` is cloned right after its leg); a section total or the empty-section item is a fresh pointer that no reader inlines. A failed compile is not cached. A reused cell's probes keep the `at` of its first compile. That is the row's own leg (legs compile in layout order), with one exception: when a derived column reads a later column, the later cell is first compiled inside `derivedCtx` with the derived formula's `at`, so a mixedCurrency notice for it names that path. Figures do not change; accept it.

- [ ] **Step 4: Run the test and the layout tests**

Run: `GOWORK=off go test ./reports/ -run 'TestLayoutCompilesEachEarlierRowCellOnce|Layout|Within|Cells|ABalanceSheetShaped|Currency|Ragged|HideEmpty' -count=1`
Expected: `ok`. `cells_test.go:253` (`layoutNodes > 2*layoutCounted`) still holds: the memo only lowers `layoutNodes`.

- [ ] **Step 5: Commit**

```bash
git add core/server/reports/layout.go core/server/reports/layout_cost_test.go
git commit -m "perf(reports): compile each computed-row cell once"
```

---

### Task 3: Materialize the report rows `w` when a statement reads them twice

**Files:**
- Modify: `core/server/reports/rows.go:598-607` (add `materializeShared` beside `render`)
- Modify: `core/server/reports/run.go:503-506` (`readRows` calls it)
- Test: `core/server/reports/layout_cost_test.go`

**Interfaces:**
- Consumes: `readRows(ctx, app, s *stmt, ctes []string, body string, topics map[string][]string)` (run.go:502); `observeReads(t) *[]readStatement` (statement_test.go:332, `.sql`); Task 1's `balanceSheetZoo`, `measureRun`.
- Produces: `materializeShared(ctes []string, body string) []string` (acts only on a body that names `__level`).

Before Step 3, confirm the group statement's body names `__level` (layout and level legs select `... AS __level`): `grep -n "__level" core/server/reports/levels.go | head -5`. If `__level` sits only in a CTE, test for it across `ctes` and `body` instead.

- [ ] **Step 1: Write the failing tests**

Append to `layout_cost_test.go`:

```go
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
```

(`benchRunWith` is from Task 0; `Run` refuses an undeclared param, so the flat spec passes `nil`.)

- [ ] **Step 2: Run them to see them fail**

Run: `GOWORK=off go test ./reports/ -run 'TestMaterializeShared|TestGroupStatementMaterializes|TestDetailStatementKeeps' -count=1`
Expected: build failure "undefined: materializeShared".

- [ ] **Step 3: Implement**

In `rows.go` after `render()`:

```go
// rowsRead finds a read of the report rows CTE w (not ws or w2).
var rowsRead = regexp.MustCompile(`(?:FROM|JOIN) w\b`)

// materializeShared makes w MATERIALIZED when a group statement (its body
// names __level) reads it twice or more (one read per group level, the
// fills): SQLite otherwise computes the rows, and reads the rule under them,
// once per read. A detail page keeps w inline, also when a probe reads it a
// second time, so it can stop at its LIMIT. The rule CTEs are never
// materialized: a correlated lookup must keep searching the table by its
// index.
func materializeShared(ctes []string, body string) []string {
	if !strings.Contains(body, "__level") {
		return ctes
	}
	for i, cte := range ctes {
		if !strings.HasPrefix(cte, "w AS (") {
			continue
		}
		reads := len(rowsRead.FindAllString(body, -1))
		for j, other := range ctes {
			if j != i {
				reads += len(rowsRead.FindAllString(other, -1))
			}
		}
		if reads > 1 {
			out := slices.Clone(ctes)
			out[i] = "w AS MATERIALIZED (" + strings.TrimPrefix(cte, "w AS (")
			return out
		}
	}
	return ctes
}
```

(add `regexp` / `slices` imports if rows.go lacks them). In `run.go` `readRows`, first line of the body: `ctes = materializeShared(ctes, body)` before `s.with(ctes...)`.

- [ ] **Step 4: Run the tests, the plan test and the paged tests**

Run: `GOWORK=off go test ./reports/ -run 'TestMaterializeShared|TestGroupStatementMaterializes|TestDetailStatementKeeps|TestABalanceShapedReportOverTwoThousandLinesSearchesLinesPerRow|Paged|Pivot|Fill|Statement' -count=1`
Expected: `ok` (the over-2,000-lines plan test must pass: w is not a rule CTE).

- [ ] **Step 5: Commit**

```bash
git add core/server/reports/rows.go core/server/reports/run.go core/server/reports/layout_cost_test.go
git commit -m "perf(reports): materialize the report rows when a statement reads them twice"
```

---

### Task 4: Measure day comparisons through substr() against a sargable form at 30,000 lines

**Files:**
- Modify: `core/server/reports/layout_bench_test.go` (add `benchPeriodReport`, `sargableDays`, `timeStatement`, `BenchmarkDayComparison`)
- Modify: `docs/plans/2026-11-09-report-layout-perf.md` (fill the Results section at the end)
- Modify: `core/server/reports/stmt.go:229-231` (why-comment on `fieldSQL`, if the gate fails)

**Interfaces:**
- Consumes: `withBalanceZoo`, `withBenchLedger`, `measureRun`, `rawStatement`, `queryPlan(t *testing.T, db *sql.DB, stmt string, args []any) []string` (balance_shape_test.go:658; change its first parameter to `testing.TB`).
- Produces: `sargableDays(sql string) string`, `dayForm(col, op, rhs string) string` (test-side; Task 5 copies the forms into product code), the gate result.

Gate (decided before measuring): Task 5 runs only if, at 30,000 lines, the sargable form with a `(date)` or `(posted, date)` index has `sql-ms` <= 0.75 x the substr form's `sql-ms` with no new index, AND `EXPLAIN QUERY PLAN` of the sargable statement names that index, AND both forms return the same rows. The gate is read for each filter (`between`, `lte`, `gte`); Task 5 runs if `between` and `lte` pass it (the General Ledger and Balance Sheet reads) and no filter gets slower than 1.1 x its substr form. Record all nine rows per size.

- [ ] **Step 1: Write the benchmark**

Append to `layout_bench_test.go`:

```go
// dayFilters are the day comparisons a ledger report makes: a period
// (General Ledger), on or before a day (Balance Sheet), on or after a day.
var dayFilters = map[string]string{
	"between": `{"op":"between","args":[{"path":[{"field":"date"}]},{"param":"period"}]}`,
	"lte":     `{"op":"lte","args":[{"path":[{"field":"date"}]},{"param":"asOf"}]}`,
	"gte":     `{"op":"gte","args":[{"path":[{"field":"date"}]},{"param":"asOf"}]}`,
}

// The bounds are Date params, as in the Balance Sheet: Task 5 rewrites a
// comparison only when its bound is a Date, so a text literal here would
// measure a form that does not ship.

// benchPeriodReport is a report on the ledger: posted lines that pass the
// day filter, totalled by account.
func benchPeriodReport(filter string) string {
	net := `{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]}`
	return `{"version":4,"source":"zoo.ledger_lines",
	"params":[{"id":"period","type":"dateRange","label":"Period"},{"id":"asOf","type":"date","label":"As of"}],
	"filter":{"op":"and","args":[{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
		` + dayFilters[filter] + `]},
	"columns":[{"id":"account","path":[{"link":"account"}]},
		{"id":"net","formula":{"op":"sum","args":[` + net + `]}}],
	"groups":[{"id":"byAccount","col":"account"}]}`
}

// substrDay is a text day read through substr() (fieldSQL), maybe inside
// between's own substr(), before its comparison operator.
var substrDay = regexp.MustCompile("(?:substr\\()?NULLIF\\(substr\\((`[^`]+`\\.`[^`]+`), 1, 10\\), ''\\)(?:, 1, 10\\))? (<=|<|>=|>) ")

// sargableDays rewrites each day comparison of a raw statement into the
// form a bare-column comparison would take (dayForm), so one statement is
// timed both ways. The right side ends at the comparison's closing
// parenthesis or at an AND at its own depth.
func sargableDays(sql string) string {
	var out strings.Builder
	for {
		loc := substrDay.FindStringSubmatchIndex(sql)
		if loc == nil {
			out.WriteString(sql)
			return out.String()
		}
		col, op := sql[loc[2]:loc[3]], sql[loc[4]:loc[5]]
		rest := sql[loc[1]:]
		depth, end := 0, len(rest)
		for i, r := range rest {
			if r == '(' {
				depth++
			}
			if r == ')' {
				if depth == 0 {
					end = i
					break
				}
				depth--
			}
			if depth == 0 && strings.HasPrefix(rest[i:], " AND ") {
				end = i
				break
			}
		}
		out.WriteString(sql[:loc[0]])
		out.WriteString(dayForm(col, op, rest[:end]))
		sql = rest[end:]
	}
}

// dayForm is col op rhs on a bare day column, as the substr() form is on
// every stored day (a text day, a PocketBase date "YYYY-MM-DD hh:mm:ss.sssZ")
// and on a blank one, which reads NULL ('' sorts first, so the blank is
// made NULL, not false, and a not() over it stays NULL).
func dayForm(col, op, rhs string) string {
	known := "(" + col + " <> '' OR NULL)"
	switch op {
	case "<":
		return "(" + col + " < " + rhs + " AND " + known + ")"
	case "<=":
		return "(" + col + " < date(" + rhs + ", '+1 day') AND " + known + ")"
	case ">":
		return "(" + col + " >= date(" + rhs + ", '+1 day') OR (" + col + " = '' AND NULL))"
	}
	return "(" + col + " >= " + rhs + " OR (" + col + " = '' AND NULL))"
}

// timeStatement prepares and runs a raw statement on db, returning both
// times and the rows as text.
func timeStatement(tb testing.TB, db *sql.DB, stmt string, args []any) (prepare, run time.Duration, rows []string) {
	tb.Helper()
	t0 := time.Now()
	prepared, err := db.Prepare(stmt)
	if err != nil {
		tb.Fatal(err)
	}
	defer prepared.Close()
	prepare = time.Since(t0)
	t1 := time.Now()
	res, err := prepared.Query(args...)
	if err != nil {
		tb.Fatal(err)
	}
	defer res.Close()
	cols, _ := res.Columns()
	for res.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := res.Scan(ptrs...); err != nil {
			tb.Fatal(err)
		}
		rows = append(rows, fmt.Sprint(vals...))
	}
	if err := res.Err(); err != nil {
		tb.Fatal(err)
	}
	return prepare, time.Since(t1), rows
}

//	LAYOUT_BENCH_LINES=30000 GOWORK=off go test ./reports/ -run '^$' -bench BenchmarkDayComparison -benchtime 1x
func BenchmarkDayComparison(b *testing.B) {
	timeout := runLimits.timeout
	runLimits.timeout = 30 * time.Minute
	defer func() { runLimits.timeout = timeout }()
	for _, lines := range benchSizes() {
		for _, filter := range []string{"between", "lte", "gte"} {
		for _, index := range []string{"", "date", "posted, date"} {
			b.Run(fmt.Sprintf("lines=%d/filter=%s/index=%s", lines, filter, index), func(b *testing.B) {
				z := newZoo(b)
				withBalanceZoo(b, z)
				withBenchLedger(b, z, lines)
				if index != "" {
					if _, err := z.app.DB().NewQuery("CREATE INDEX idx_bench_lines_day ON zoo_ledger_lines (" + index + ")").Execute(); err != nil {
						b.Fatal(err)
					}
					if _, err := z.app.DB().NewQuery("ANALYZE").Execute(); err != nil {
						b.Fatal(err)
					}
				}
				var group benchStatement
				observeRead = func(sql string, params dbx.Params) {
					if strings.Contains(sql, "__level") && len(sql) > group.bytes {
						group.bytes, group.sql, group.params = len(sql), sql, params
					}
				}
				params := map[string]json.RawMessage{
					"period": json.RawMessage(`{"from":"2026-03-01","to":"2026-03-31"}`),
					"asOf":   json.RawMessage(`"2026-03-15"`),
				}
				if _, err := benchRunWith(z, benchPeriodReport(filter), params); err != nil {
					b.Fatal(err)
				}
				observeRead = nil
				raw, args := rawStatement(b, group.sql, group.params)
				sargable := sargableDays(raw)
				if sargable == raw {
					b.Fatal("the statement has no day comparison to rewrite")
				}
				pool, ok := z.app.ConcurrentDB().(*dbx.DB)
				if !ok {
					b.Fatalf("read pool is %T", z.app.ConcurrentDB())
				}
				db := pool.DB()
				_, substrRun, substrRows := timeStatement(b, db, raw, args)
				_, sargRun, sargRows := timeStatement(b, db, sargable, args)
				if strings.Join(substrRows, "\n") != strings.Join(sargRows, "\n") {
					b.Fatalf("the two forms disagree:\n%v\n%v", substrRows, sargRows)
				}
				plan := strings.Join(queryPlan(b, db, sargable, args), "\n")
				used := 0.0
				if strings.Contains(plan, "idx_bench_lines_day") {
					used = 1
				}
				b.ReportMetric(float64(substrRun.Microseconds())/1000, "substr-ms")
				b.ReportMetric(float64(sargRun.Microseconds())/1000, "sargable-ms")
				b.ReportMetric(used, "index-used")
				b.ReportMetric(float64(len(substrRows)), "rows")
			})
		}
		}
	}
}
```

(gofmt indents the inner loop.) `benchRunWith` is from Task 0. Change `queryPlan`'s first parameter to `testing.TB` (balance_shape_test.go:658). Add the `database/sql` import. Make `measureRun` use `timeStatement` for its own prepare and run timing so the two share one helper.

- [ ] **Step 2: Check the rewrite on a small ledger**

Run: `LAYOUT_BENCH_LINES=300 GOWORK=off go test ./reports/ -run '^$' -bench BenchmarkDayComparison -benchtime 1x`
Expected: three sub-benchmarks, no "disagree" failure, `rows` > 0.

- [ ] **Step 3: Measure at 30,000 lines**

Run: `LAYOUT_BENCH_LINES=30000 GOWORK=off go test ./reports/ -run '^$' -bench BenchmarkDayComparison -benchtime 1x -timeout 30m`
This takes a few minutes. If it runs longer than 4 minutes, hand back `NEEDS-RUN: <this command>`. Also run the same with `LAYOUT_BENCH_LINES=3000` to see the trend.

- [ ] **Step 4: Record the outcome**

Fill the "Results" section at the end of this plan file with the table (lines, index, substr-ms, sargable-ms, index-used) and the verdict against the gate. If the gate fails, replace the `fieldSQL` comment in stmt.go (lines 229-230) with:

```go
// fieldSQL reads a field from an alias. A blank date, select or relation is
// NULL, so a blank and a hidden linked row read the same. A day is compared
// through substr(), which no index serves: measured (BenchmarkDayComparison,
// 2026-11) a bare-column form with a date index gained nothing that matters,
// because every read of a collection also runs its rule's id list over the
// whole table.
```

and mark Task 5 "Skipped: gate not met" in this plan. If the gate passes, leave `fieldSQL`'s comment and go on to Task 5.

- [ ] **Step 5: Commit**

```bash
git add core/server/reports/layout_bench_test.go core/server/reports/balance_shape_test.go core/server/reports/stmt.go docs/plans/2026-11-09-report-layout-perf.md
git commit -m "test(reports): measure day comparisons against a sargable form"
```

---

### Task 5: Compare days on the bare column (only if Task 4's gate passed)

Skipped: gate not met (see Results).

Skip this task, and say so in the Results section, when Task 4's gate failed.

**Files:**
- Modify: `core/server/reports/formula.go:47-62` (`expr.day`), `:836-843` (lt/lte/gt/gte), `:1067-1077` (between with a range)
- Modify: `core/server/reports/stmt.go:291-294` (`scope.value` sets `day`)
- Create: `core/server/reports/daycompare_test.go`

**Interfaces:**
- Consumes: Task 4's `dayForm` forms (copy them into product code; the bench keeps its own copy for the measurement).
- Produces: `expr.day string`; `dayCompare(op string, args []expr) string`; `dayFormSQL(col, op, rhs string) string` (ops `lt`, `lte`, `gt`, `gte`).

- [ ] **Step 1: Write the failing test**

Create `daycompare_test.go`. It runs every comparison over three stored forms (a text day `2026-03-15`, a PocketBase date `2026-03-15 13:45:00.000Z`, a blank `''`) and NULL, and checks the compiled condition against the substr() form, also under `not`:

```go
package reports

import (
	"testing"
)

// A day compared on the bare column gives what substr() gave on every
// stored form of a day and on a blank one (NULL, also under not).
func TestDayComparisonMatchesTheDayOfEveryStoredForm(t *testing.T) {
	z := newZoo(t)
	db := z.app.DB()
	for _, stored := range []any{"2026-03-15", "2026-03-15 13:45:00.000Z", "", nil} {
		for _, bound := range []string{"2026-03-14", "2026-03-15", "2026-03-16"} {
			for _, op := range []string{"lt", "lte", "gt", "gte"} {
				old := "NULLIF(substr({:v}, 1, 10), '') " + comparisons[op] + " {:b}"
				bare := dayFormSQL("{:v}", op, "{:b}")
				for _, wrap := range []string{"%s", "NOT (%s)"} {
					var a, b *int64
					q := "SELECT (" + sprintf(wrap, old) + "), (" + sprintf(wrap, bare) + ")"
					if err := db.NewQuery(q).Bind(map[string]any{"v": stored, "b": bound}).Row(&a, &b); err != nil {
						t.Fatal(err)
					}
					if (a == nil) != (b == nil) || (a != nil && *a != *b) {
						t.Errorf("%v %s %s (%s): substr %v, bare %v", stored, op, bound, wrap, deref(a), deref(b))
					}
				}
			}
		}
	}
}
```

with small local helpers `sprintf := fmt.Sprintf` (import `fmt`) and `deref(p *int64) any` (nil or the value); `Bind` takes `dbx.Params`. Add a second test that compiles a report filtered by `lte` on `zoo.ledger_lines.date` and checks the statement contains "` < date(`" and no "substr(" around that field, and a third over `zoo.feedings.fed` (a PocketBase date column, zoo_test.go:189; `due` beside it is a text day): a report filtered by `{"op":"lte","args":[{"path":[{"field":"fed"}]},{"param":"asOf"}]}` returns the same detail rows as the same report filtered by `{"op":"lte","args":[{"op":"addMonths","args":[{"path":[{"field":"fed"}]},{"value":0}]},{"param":"asOf"}]}` (an operator over the field keeps the substr() form, since `day` is set only on a bare leaf), for an asOf on a day that has a feeding.

Add two more tests, because `day` rides on a copied `expr` after its `sql` is wrapped, and only `bareDay` keeps such a copy on the substr() form:
- `TestADayLevelHavingKeepsItsFigures`: a report grouped by `date` with `"bucket":"month"` and a level `having` of `{"op":"gte","args":[{"col":"date"},{"value":"2026-03-01"}]}`, and a label over the date column; the statement has no bare-column day form for the bucketed column (`!strings.Contains(stmt, "< date(")` on that column's alias), and the rows are the same as the run before Task 5 (pin the expected group keys in the test).
- `TestABucketedDayIsComparedThroughItsBucket`: a filter `{"op":"lte","args":[{"op":"bucket","args":[{"path":[{"field":"date"}]},{"value":"month"}]},{"value":"2026-03-01"}]}` (or the bucket operator the spec has; find it with `grep -n '"bucket"\|bucketSQL' core/server/reports/*.go`) returns the same rows as substr() gave (pinned).

- [ ] **Step 2: Run to see it fail**

Run: `GOWORK=off go test ./reports/ -run TestDayComparison -count=1`
Expected: build failure "undefined: dayFormSQL".

- [ ] **Step 3: Implement**

`formula.go`, `expr` field:

```go
	// day is, on a Date field read straight from its column, that column
	// (fieldSQL reads it through substr()); a comparison with a day uses the
	// bare column so an index on it can serve the range. "" otherwise.
	day string
```

`stmt.go`:

```go
// value reads a to-one path's field in this scope.
func (sc *scope) value(p *pathPlan) expr {
	alias := sc.join(p.hops)
	raw := fieldSQL(alias, p.field)
	e := pathExpr(raw, p)
	if p.field.Type == Date && e.sql == raw {
		e.day = q(alias, p.field.Column)
	}
	return e
}
```

`formula.go`, in the `lt/lte/gt/gte` case before the generic return: `if sql := dayCompare(n.Op, args); sql != "" { return expr{sql: sql, typ: Bool}, nil }`, and:

```go
var flippedOps = map[string]string{"lt": "gt", "lte": "gte", "gt": "lt", "gte": "lte"}

// dayCompare is a comparison of a day field with a day on the bare column
// (dayFormSQL), "" when neither or both sides are a day field or the other
// side is not a day: a date-time bound is compared as before.
func dayCompare(op string, args []expr) string {
	day, bound := bareDay(args[0]), bareDay(args[1])
	if day == "" {
		day, bound, op = bound, day, flippedOps[op]
		args = []expr{args[1], args[0]}
	}
	if day == "" || bound != "" || args[1].typ != Date {
		return ""
	}
	return dayFormSQL(day, op, args[1].sql)
}

// bareDay is e's day column while e still reads it as fieldSQL wrote it: an
// expr copied and wrapped (a bucket, a label, a level's having) keeps its
// day field but not this sql, and keeps the substr() form.
func bareDay(e expr) string {
	if e.day == "" || e.sql != "NULLIF(substr("+e.day+", 1, 10), '')" {
		return ""
	}
	return e.day
}

// dayFormSQL is col op rhs on a bare day column, the same as the substr()
// form on a text day, a stored date-time and a blank (NULL, also under not).
func dayFormSQL(col, op, rhs string) string {
	known := "(" + col + " <> '' OR NULL)"
	switch op {
	case "lt":
		return "(" + col + " < " + rhs + " AND " + known + ")"
	case "lte":
		return "(" + col + " < date(" + rhs + ", '+1 day') AND " + known + ")"
	case "gt":
		return "(" + col + " >= date(" + rhs + ", '+1 day') OR (" + col + " = '' AND NULL))"
	}
	return "(" + col + " >= " + rhs + " OR (" + col + " = '' AND NULL))"
}
```

In `between` (range case, line 1076): `if d := bareDay(x); d != "" { return expr{sql: "(" + dayFormSQL(d, "gte", args[1].lo) + " AND " + dayFormSQL(d, "lte", args[1].hi) + ")", typ: Bool}, nil }` before the substr form. Confirm the exact text fieldSQL writes for a Date (`grep -n "NULLIF(substr" core/server/reports/stmt.go`) and make `bareDay` match it byte for byte.

- [ ] **Step 4: Run the new tests and the whole date test set**

Run: `GOWORK=off go test ./reports/ -run 'TestDayComparison|Date|Period|Between|ABalanceShaped|Ledger|TextDay|AllDates' -count=1`
Expected: `ok`.

- [ ] **Step 5: Re-measure and commit**

Run Task 4 step 3 again with the product change; record the new run time of `BenchmarkLayoutStatement/trialBalance` and the period report in the Results section.

```bash
git add core/server/reports/formula.go core/server/reports/stmt.go core/server/reports/daycompare_test.go docs/plans/2026-11-09-report-layout-perf.md
git commit -m "perf(reports): compare days on the bare column"
```

---

### Task 6: A group the reader opens shows its caret turned and a spinner until its run lands

**Files:**
- Modify: `core/reports/lib/rows.ts:7-13` (`VisibleRow.isPending`), `:71-79` (`visibleRows` sets `isPending: false`), add `PendingSpec` and `withPendingOpen`
- Test: `core/reports/lib/__tests__/rows.test.ts`
- Modify: `core/reports/lib/__tests__/column-measure.test.ts`, `sticky.test.ts`, `table-lines.test.ts` (`isPending: false` in hand-built `VisibleRow` literals)
- Modify: `core/reports/hooks/useReportView.ts:259` (rows), `:292`
- Modify: `core/reports/components/ReportRow.tsx:1-4` (imports), `:197-201` (`GroupCaret`), `:252-280` (`GroupToggleLabel`)
- Modify: `tests/e2e/reports-groups-v2.spec.ts` (new test)
- Modify: `core/help/reports.md:133-135` (one sentence)

**Interfaces:**
- Consumes: `Expansion`, `visibleRows`, `toggleGroup` (rows.ts); `run.isUpdating` (lib/run-state.ts:83, `query.isPlaceholderData && !refusal`); `expansion` from `useReportUIStore(s => s.expansion[reportKey]) ?? DEFAULT_EXPANSION` (useReportView.ts:161); e2e helpers `holdRuns(page)`, `detailLabels(page)` (reports-helpers.ts), `rowOf(page, label)` (report-view-helpers.ts).
- Produces: `VisibleRow.isPending: boolean`; `type PendingSpec = { groups?: readonly { open?: boolean }[] | null }`; `withPendingOpen(rows: VisibleRow[], expansion: Expansion, spec: PendingSpec | null, isUpdating: boolean): VisibleRow[]`; test ids `report-pending-<rowId>` (and `report-sticky-pending-<rowId>` on the stuck copy).

The rows reach `ReportRow` through `ReportTable`'s `rows` prop (FlashList `data` and `extraData.rows`), so no new prop is threaded: `isPending` travels on each `VisibleRow`. A hand-built `ReportViewModel` (Books static references) gets `isPending: false` from `visibleRows`.

Accepted, not a bug: a group of an open-by-default level that the server sends closed shows the spinner while a realtime re-run is on its way (Review Focus 5); the spinner goes when that run lands.

Order: the e2e and unit tests are written first and committed alone (commit A), the change after (commit B), so the driver sees both red at A and green at B (Step 7). No step runs vitest, tsc, biome or the e2e in the worktree.

- [ ] **Step 0: Write the e2e test**

In `tests/e2e/reports-groups-v2.spec.ts`, import `holdRuns` and `detailLabels` from `./reports-helpers`, and add inside `test.describe('group levels', ...)`:

```ts
    test('a group opened while its run is on its way turns its arrow at once and shows a spinner until the rows land', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptpend')
        try {
            const stem = uniqueName('Pend')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            const team = rowOf(p, `${stem} A`)
            await expect(team).toHaveAttribute('aria-expanded', 'false')
            await expect(detailLabels(p)).toHaveCount(0)
            const id = ((await team.getAttribute('data-testid')) ?? '').replace('report-row-', '')
            const label = team.getByText(`${stem} A`, { exact: true })
            const labelX = async () => (await label.boundingBox())?.x ?? null
            // Read the label's place once the first run has landed and the row is still.
            await expect.poll(labelX).not.toBeNull()
            const before = await labelX()
            const release = await holdRuns(p)
            await team.click()
            await expect(team).toHaveAttribute('aria-expanded', 'true')
            await expect(team).toHaveAttribute('aria-busy', 'true')
            await expect(p.getByTestId(`report-pending-${id}`)).toBeVisible()
            await expect(detailLabels(p)).toHaveCount(0)
            // The spinner sits in the caret's slot: the label does not move.
            await expect.poll(labelX).toBe(before)
            release()
            await p.unrouteAll({ behavior: 'wait' })
            await expect(detailLabels(p)).toHaveCount(1)
            await expect(p.getByTestId(`report-pending-${id}`)).toHaveCount(0)
            await expect(team).not.toHaveAttribute('aria-busy', 'true')
        } finally {
            await close()
        }
    })
```

A phone run is not added: the phone path draws the same `GroupToggleLabel` from the same `withPendingOpen` rows (no phone-only code).

- [ ] **Step 1: Write the failing unit tests**

Add `withPendingOpen` to the import list of `rows.test.ts` and append:

```ts
describe('withPendingOpen', () => {
    const closed = group('g:c', ['c'])
    const open = group('g:a', ['a'], { expanded: true })
    const shown = visibleRows([open, closed, total])

    it('marks a closed group the expansion opens by key, while the run updates', () => {
        const next = withPendingOpen(shown, { all: false, keys: [['c']], collapsed: [] }, null, true)
        expect(next[1]).toMatchObject({ isOpen: true, isPending: true })
        expect(next[0].isPending).toBe(false)
    })

    it('marks nothing once the run has landed', () => {
        const next = withPendingOpen(shown, { all: false, keys: [['c']], collapsed: [] }, null, false)
        expect(next).toBe(shown)
    })

    it('under Expand all marks every closed group that is not folded', () => {
        expect(withPendingOpen(shown, EXPAND_ALL, null, true)[1].isPending).toBe(true)
        const folded = { all: true, keys: [], collapsed: [['c']] }
        expect(withPendingOpen(shown, folded, null, true)[1].isPending).toBe(false)
    })

    it('under the open levels marks a closed group of an open level that is not folded', () => {
        const spec = { groups: [{ open: true }] }
        const unfolded = { ...DEFAULT_EXPANSION, collapsed: [] }
        expect(withPendingOpen(shown, unfolded, spec, true)[1].isPending).toBe(true)
        const folded = { ...DEFAULT_EXPANSION, collapsed: [['c']] }
        expect(withPendingOpen(shown, folded, spec, true)[1].isPending).toBe(false)
        expect(withPendingOpen(shown, DEFAULT_EXPANSION, { groups: [{}] }, true)[1].isPending).toBe(
            false
        )
    })

    // toggleGroup reads row.expanded, so the pending row carries expanded: a
    // second press while the run is on its way closes it, as on an open group.
    it('lets a second press close a pending group', () => {
        const expansion = { all: false, keys: [['c']], collapsed: [], isDefault: false }
        const next = withPendingOpen(shown, expansion, null, true)
        expect(next[1].row.expanded).toBe(true)
        const closedAgain = toggleGroup(expansion, next[1].row, [open, closed, total])
        expect(closedAgain.keys).toEqual([])
        expect(withPendingOpen(shown, closedAgain, null, true)[1].isPending).toBe(false)
    })

    it('never marks a row that cannot open', () => {
        const fixed = visibleRows([group('g:d', ['d'], { canExpand: false })])
        expect(withPendingOpen(fixed, EXPAND_ALL, null, true)[0].isPending).toBe(false)
    })
})
```

- [ ] **Step 2: Commit the tests alone (commit A)**

```bash
git add tests/e2e/reports-groups-v2.spec.ts core/reports/lib/__tests__/rows.test.ts
git commit -m "test(reports): a group opened while its run is on its way shows a spinner"
```

Note its sha (`git rev-parse HEAD`) for Step 7. The red runs of these tests are in Step 7 (the driver runs them).

- [ ] **Step 3: Implement the helper**

In `rows.ts`, add `isPending: boolean` to `VisibleRow` with the doc comment `/** A closed group the reader opened whose run has not landed yet: drawn open, with a spinner. */`, set `isPending: false` in `visibleRows`, and add after `toggleGroup`:

```ts
/** What withPendingOpen reads of the spec: which group levels open by default. */
export type PendingSpec = { groups?: readonly { open?: boolean }[] | null }

// Whether the expansion asks this closed group open, as the engine reads it
// (expansionOptions, R2): every group but the folds under Expand all; under the
// open levels, a group of an open level that is not folded or one opened by its
// key; otherwise one opened by its key.
function asksOpen(expansion: Expansion, row: ResultRow, spec: PendingSpec | null): boolean {
    const key = row.groupKey ?? []
    if (hasKey(expansion.collapsed, key)) return false
    if (expansion.all) return true
    if (hasKey(expansion.keys, key)) return true
    if (!expansion.isDefault) return false
    const level = row.level ?? row.depth + 1
    return spec?.groups?.[level - 1]?.open === true
}

/**
 * While a run is on its way (isUpdating), a closed group the expansion asks open is
 * drawn open and pending, so the press shows at once; the run that lands draws it
 * open with its rows. With no run on its way the rows are returned unchanged.
 */
export function withPendingOpen(
    rows: VisibleRow[],
    expansion: Expansion,
    spec: PendingSpec | null,
    isUpdating: boolean
): VisibleRow[] {
    if (!isUpdating) return rows
    return rows.map(item =>
        item.isToggle && !item.isOpen && asksOpen(expansion, item.row, spec)
            ? { ...item, row: { ...item.row, expanded: true }, isOpen: true, isPending: true }
            : item
    )
}
```

The pending row's `row.expanded` is `true` because `toggleGroup` (rows.ts:161) decides open or close from `row.expanded`: without it a second press while the run is on its way adds the same key again and the press does nothing. With it, the second press closes the group, `asksOpen` is false, and the spinner goes. The sticky copy of a pending group header draws the same `VisibleRow`, so it shows the same spinner (test id `report-sticky-pending-<id>`).

Add `isPending: false` to every hand-built `VisibleRow` literal in the unit tests, or tsc fails: `core/reports/lib/__tests__/column-measure.test.ts:49`, `core/reports/lib/__tests__/sticky.test.ts:9`, `core/reports/lib/__tests__/table-lines.test.ts:112, 776, 784, 800, 1068` (confirm the list with `grep -rn "isToggle:" core/reports --include='*.ts' --include='*.tsx'`; fix each hit that builds a `VisibleRow`).

The unit tests pass at commit B (Step 7; the existing `toMatchObject` cases still pass with the new field).

- [ ] **Step 4: Use it in the view and draw it**

`useReportView.ts` line 259:

```ts
    const shownRows = useMemo(() => visibleRows(rows, viewSpec), [rows, viewSpec])
    // The group the reader just opened shows open at once, with a spinner, until
    // the run that opens it lands (findings: a slow statement left the press silent).
    const pendingRows = useMemo(
        () => withPendingOpen(shownRows, expansion, viewSpec, run.isUpdating),
        [shownRows, expansion, viewSpec, run.isUpdating]
    )
```

import `withPendingOpen` from `../lib/rows`, and return `rows: pendingRows` (line 292). `ReportSpec` must satisfy `PendingSpec` (its groups carry `open?: boolean`, spec.ts:165); if tsc refuses, pass `viewSpec` typed as `PendingSpec | null`.

`ReportRow.tsx`: import `ActivityIndicator` from `react-native`, then:

```tsx
// The caret slot is CARET_SIZE square and the label starts CARET_ROOM after the
// indent (column-measure.ts:155, 206), so the spinner takes the chevron's place in
// that slot: a wider caret would push the label out of the measured column.
const SPINNER_SCALE = CARET_SIZE / 20

function GroupCaret({ isOpen, pendingId }: { isOpen: boolean; pendingId: string | null }) {
    const color = useThemeColor('muted-foreground')
    const Icon = isOpen ? ChevronDown : ChevronRight
    if (pendingId !== null) return <PendingSpinner testID={pendingId} color={color} />
    return <Icon size={CARET_SIZE} color={color} />
}

// ActivityIndicator "small" is 20 px on web and native; scaled into the slot.
function PendingSpinner({ testID, color }: { testID: string; color: string }) {
    return (
        <View
            style={{ width: CARET_SIZE, height: CARET_SIZE }}
            className="items-center justify-center"
        >
            <ActivityIndicator
                testID={testID}
                size="small"
                color={color}
                style={{ transform: [{ scale: SPINNER_SCALE }] }}
            />
        </View>
    )
}

const pendingId = (item: VisibleRow, isCopy: boolean) =>
    item.isPending ? `${isCopy ? 'report-sticky-pending' : 'report-pending'}-${item.row.id}` : null
```

In `GroupToggleLabel`: `aria-busy={item.isPending}` on the `Pressable`, and `<GroupCaret isOpen={item.isOpen} pendingId={pendingId(item, isCopy)} />`. The spinner's test id must not start with `report-row-` (the specs select rows by that prefix).

- [ ] **Step 5: Help line**

In `core/help/reports.md`, in the "Fold groups" paragraph (line 133), after the sentence that starts "**Collapse all** closes the groups" add: "While the rows of a group you opened load, a spinner shows in place of its arrow. Select the row again to close it." No generate step: no new topic, only a line.

- [ ] **Step 6: Commit the change (commit B)**

```bash
git add core/reports/lib/rows.ts core/reports/lib/__tests__/column-measure.test.ts core/reports/lib/__tests__/sticky.test.ts core/reports/lib/__tests__/table-lines.test.ts core/reports/hooks/useReportView.ts core/reports/components/ReportRow.tsx core/help/reports.md
git commit -m "feat(reports): show a spinner on a group while its rows load"
```

- [ ] **Step 7: Hand back the runs to the driver**

The worktree has no `node_modules`, and the e2e needs `../salesstub` beside the app (tests/e2e/helpers.ts:93-95, `skipWithoutReportsStub`); without it the whole `group levels` describe is skipped, not failed. So vitest, tsc, biome and the e2e run in the workspace `/Users/nas/tmp/tinycld/books`, and only the driver runs them (they start servers and change the checkout of that workspace's core). The implementer does not run them and does not touch `/Users/nas/tmp/tinycld/books`. Hand back, with the two shas filled in:

```
NEEDS-RUN: (in /Users/nas/tmp/tinycld/books/tinycld; ports first: lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7230|7231)' must print nothing)
  RED at commit A (<sha A>):
    git checkout --detach <sha A> && pnpm run packages:generate
    ../node_modules/.bin/vitest run core/reports/lib/__tests__/rows.test.ts
      expected: FAIL, withPendingOpen is not exported
    E2E_PORT=7230 E2E_PORT_2=7231 pnpm exec tinycld-pkg test:e2e -- reports-groups-v2.spec.ts -g "spinner"
      expected: 1 failed: aria-expanded stays "false" while the run is held (a "skipped" result is a failure of the run)
  GREEN at commit B (<sha B>):
    git checkout --detach <sha B> && pnpm run packages:generate
    ../node_modules/.bin/vitest run core/reports/lib/__tests__/
    pnpm exec tinycld-pkg typecheck
    ../node_modules/.bin/biome check core/reports/lib/rows.ts core/reports/lib/__tests__/rows.test.ts core/reports/lib/__tests__/column-measure.test.ts core/reports/lib/__tests__/sticky.test.ts core/reports/lib/__tests__/table-lines.test.ts core/reports/hooks/useReportView.ts core/reports/components/ReportRow.tsx tests/e2e/reports-groups-v2.spec.ts
    E2E_PORT=7230 E2E_PORT_2=7231 pnpm exec tinycld-pkg test:e2e -- reports-groups-v2.spec.ts -g "spinner"
      expected: vitest ok, no tsc or biome errors, e2e "1 passed" (not skipped)
  then: git checkout main
```

The e2e server builds the Go server of that checkout, so the run includes Tasks 0-5 as well. A failure goes back to the implementer as a fix round; the driver does not re-run to green.

---

### Task 7: Index org_pkg_access(user, pkg) (NOT IN THIS PR: a separate, later PR)

Human decision (2026-11-09): this PR builds Tasks 0-6 only. Task 7 goes in a separate, later PR, because it adds a core migration, which needs the lane's `exclusive` hold. The runner of this PR skips Task 7; Tasks 0-6 do not depend on it. The text below stays as the plan of that later PR.

**Files:**
- Create: `core/server/pb_migrations/2090000013_org_pkg_access_user_pkg_index.js`
- Create: `core/server/pkgaccess/index_test.go`

**Interfaces:**
- Consumes: `rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))` (tinycld.org/core/rlstest, as groups/rules_test.go:16).
- Produces: index `idx_org_pkg_access_user_pkg` on `org_pkg_access (user, pkg)`.

Before you write: `ls core/server/pb_migrations | tail -3` and use the next free number after the highest (2090000012 at plan time). `1700000000_create_core_collections.js` is in tag v0.10.0 (released), so the index cannot be added there. The index is not UNIQUE: nothing guarantees one row per (user, pkg) today, and a unique index would fail the migration on a database with a duplicate.

- [ ] **Step 1: Write the failing test**

```go
package pkgaccess

import (
	"strings"
	"testing"

	"tinycld.org/core/rlstest"
)

// Every package rule reads a user's access row by user and package (the
// books_access view, CanWrite): an index serves that read instead of a scan
// of the table for each rule evaluation.
func TestOrgPkgAccessHasAUserPkgIndex(t *testing.T) {
	app := rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
	col, err := app.FindCollectionByNameOrId("org_pkg_access")
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE INDEX `idx_org_pkg_access_user_pkg` ON `org_pkg_access` (`user`, `pkg`)"
	if !strings.Contains(strings.Join(col.Indexes, "\n"), want) {
		t.Fatalf("indexes = %v", col.Indexes)
	}
	rows, err := app.DB().NewQuery("EXPLAIN QUERY PLAN SELECT access FROM org_pkg_access WHERE user = 'u' AND pkg = 'p'").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if !strings.Contains(strings.Join(plan, "\n"), "idx_org_pkg_access_user_pkg") {
		t.Fatalf("the read does not use the index: %v", plan)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `GOWORK=off go test ./pkgaccess/ -run TestOrgPkgAccessHasAUserPkgIndex -count=1`
Expected: FAIL "indexes = [...]".

- [ ] **Step 3: Write the migration**

```js
/// <reference path="../pb_data/types.d.ts" />
// org_pkg_access is read by user and package in every package's access rule
// (a books_access-style view evaluates it once per rule read, which a report
// statement does several times). With no index each read scans the table.
// Not unique: nothing has kept one row per (user, pkg). A new migration
// because 1700000000_create_core_collections.js is released.
migrate(
    app => {
        const col = app.findCollectionByNameOrId('org_pkg_access')
        col.indexes = [
            ...col.indexes,
            'CREATE INDEX `idx_org_pkg_access_user_pkg` ON `org_pkg_access` (`user`, `pkg`)',
        ]
        app.save(col)
    },
    app => {
        const col = app.findCollectionByNameOrId('org_pkg_access')
        col.indexes = col.indexes.filter(i => !i.includes('idx_org_pkg_access_user_pkg'))
        app.save(col)
    }
)
```

- [ ] **Step 4: Run the test and the access tests**

Run: `GOWORK=off go test ./pkgaccess/ ./groups/ ./rlstest/ -count=1`
Expected: `ok` for each. Then `grep -rn "org_pkg_access" core/server/coreserver/*_test.go | grep -i index` to make sure no golden lists the collection's indexes; update one only if it pins the index list.

- [ ] **Step 5: Commit**

```bash
git add core/server/pb_migrations/2090000013_org_pkg_access_user_pkg_index.js core/server/pkgaccess/index_test.go
git commit -m "perf(pkgaccess): index org_pkg_access by user and package"
```

---

## Results

(Filled by Task 4, and by Task 5 if it runs: the day comparison table at 3,000 and 30,000 lines, the gate verdict, and the `BenchmarkLayoutStatement` figures after Tasks 1-3.)

### Day comparison (Task 4), M4 Pro

Report: posted ledger lines filtered by one day comparison, totalled by account. The benchmark uses the period 2025-03-01..2025-03-31 (the generated ledger has no line in March 2026, so the brief's 2026 period matched nothing and compared nothing); `asOf` is 2026-03-15. Both forms return the same rows in every case.

| lines | filter | index | substr-ms | sargable-ms | index-used | rows |
|---|---|---|---|---|---|---|
| 300 | between | none | 0.539 | 0.497 | 0 | 19 |
| 300 | between | date | 0.557 | 0.693 | 1 | 19 |
| 300 | between | posted, date | 0.802 | 0.715 | 1 | 19 |
| 300 | lte | none | 0.822 | 0.810 | 0 | 33 |
| 300 | lte | date | 0.822 | 1.063 | 0 | 33 |
| 300 | lte | posted, date | 1.099 | 1.087 | 0 | 33 |
| 300 | gte | none | 0.677 | 0.653 | 0 | 34 |
| 300 | gte | date | 0.675 | 0.876 | 1 | 34 |
| 300 | gte | posted, date | 0.969 | 0.907 | 1 | 34 |
| 3,000 | between | none | 4.315 | 4.013 | 0 | 18 |
| 3,000 | between | date | 4.344 | 3.162 | 1 | 18 |
| 3,000 | between | posted, date | 4.559 | 3.166 | 1 | 18 |
| 3,000 | lte | none | 6.670 | 6.569 | 0 | 47 |
| 3,000 | lte | date | 6.676 | 7.135 | 0 | 47 |
| 3,000 | lte | posted, date | 6.953 | 6.824 | 0 | 47 |
| 3,000 | gte | none | 4.709 | 4.586 | 0 | 19 |
| 3,000 | gte | date | 4.656 | 4.059 | 1 | 19 |
| 3,000 | gte | posted, date | 4.766 | 4.061 | 1 | 19 |
| 30,000 | between | none | 48.31 | 42.70 | 0 | 30 |
| 30,000 | between | date | 44.90 | 29.03 | 1 | 30 |
| 30,000 | between | posted, date | 45.15 | 28.59 | 1 | 30 |
| 30,000 | lte | none | 69.95 | 69.73 | 0 | 152 |
| 30,000 | lte | date | 70.50 | 77.03 | 0 | 152 |
| 30,000 | lte | posted, date | 73.64 | 71.91 | 0 | 152 |
| 30,000 | gte | none | 51.30 | 50.07 | 0 | 64 |
| 30,000 | gte | date | 51.35 | 41.52 | 1 | 64 |
| 30,000 | gte | posted, date | 51.22 | 41.52 | 1 | 64 |

Gate at 30,000 lines (sargable with an index <= 0.75 x substr with no new index, plan names the index, same rows):
- between: 29.03 / 48.31 = 0.60 (date), 28.59 / 48.31 = 0.59 (posted, date); index named; passes.
- lte: 77.03 / 69.95 = 1.10 (date), 71.91 / 69.95 = 1.03 (posted, date); the plan does not name the index (the `<=` bound is rewritten to `< date(asOf, '+1 day')`, which SQLite does not use the index for here); fails, and with the date index it is no faster than substr.
- gte: 41.52 / 51.30 = 0.81; index named; fails 0.75.

Verdict: gate not met (Task 5 needs between and lte to pass; lte fails). Task 5 is skipped, and `fieldSQL`'s comment records why.

### BenchmarkLayoutStatement after Tasks 1-3

Baseline (perf/deep-dive.txt sec 3): balanceSheet 1,004,129 B, 1,495 rule reads, 2,029 / 7,675 / 71,181 ms at 300 / 3,000 / 30,000; trialBalance 3,959 B, 1 read, 6.5 / 22.3 / 194 ms.

| statement | lines | ns/op | stmt-bytes | rule-reads | sql-ms |
|---|---|---|---|---|---|
| balanceSheet | 300 | 53,614,917 | 72,606 | 5 | 5.549 |
| balanceSheet | 3,000 | 90,048,208 | 72,606 | 5 | 42.44 |
| balanceSheet | 30,000 | 511,728,708 | 72,606 | 5 | 451.7 |
| balanceSheetLevels | 300 | 38,912,208 | 26,727 | 1 | 3.512 |
| balanceSheetLevels | 3,000 | 64,345,583 | 26,727 | 1 | 21.36 |
| balanceSheetLevels | 30,000 | 364,537,750 | 26,727 | 1 | 227.2 |
| trialBalance | 300 | 6,940,458 | 3,972 | 1 | 0.867 |
| trialBalance | 3,000 | 21,931,334 | 3,972 | 1 | 6.983 |
| trialBalance | 30,000 | 204,284,417 | 3,972 | 1 | 77.57 |

Run times: BenchmarkDayComparison 300 lines 2.8 s, 3,000 lines 2.3 s, 30,000 lines 8.4 s; BenchmarkLayoutStatement (all three sizes) 7.0 s. `sql-ms` now includes scanning the rows to text (`timeStatement`), a few ms at most.
