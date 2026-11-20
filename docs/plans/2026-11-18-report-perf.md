# Report Engine Performance Implementation Plan — part 1 (tasks 1 and 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Report runs on a book with 500,000 journal lines meet the time bars of the A/R Aging spec §9 (each run ≤ 1,000 ms, CSV ≤ 5,000 ms, L ≤ 15 × S), with every other report's result unchanged.

**Architecture:** The time is in the SQL that core's report engine writes for a union source and its group levels, and in the two Books views. This part adds the measurement harness that gives each later task its target, and the first engine change: a level computes each distinct aggregate one time. Parts after this one change how `w` and the levels are written (row values once per row, roll-up from the finest level, one statement for page and Total, filters pushed into union parts, a one-evaluation export) and then rework the Books views. Each engine change has a test-only switch (`legacySQL`) that writes the SQL of before it, so a cross-check test compares the old and the new result on fictional data.

**Tech Stack:** Go, SQLite (PocketBase fork, dbx), core report engine `tinycld.org/core/reports`, Books server tests (`tinycld.org/packages/books`).

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/ar-aging-design.md` (§9 Performance acceptance; §4 Data for the views). Root-cause profile: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-ar-aging/cp/cp3-perf-rootcause.md`. Outline of all 10 tasks, with the budget and the stop rule: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/plan-outline.md`.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-perf`); Books is `books/` (branch `books/p5-report-perf`). Task 1 is a Books task; task 2 is a core task.

## Global Constraints

- Time bars (spec §9, verbatim): "runs 1 to 6: 1,000 ms or less each; run 7: 5,000 ms or less; each run at L takes no more than 15 times its time at S". "A bar is never changed and a run is never removed to pass."
- The human (2026-11-18): the 1 s whole-book bar at 500k lines stays. When the measurements show a bar is not attainable after all reasonable work, the work stops for the human with the evidence. It does not weaken the bar.
- Every plan bar of spec §9 stays (no `SCAN` of `books_journal_lines` or `books_journal_entries`, no `AUTOMATIC … INDEX` on a large table, and the others).
- Core names no package: no Books slug, collection, or route in `tinycld/`. Core tests use the fictional zoo (`zoo.meals`, `zoo.lines`).
- Every other shipped report's result stays identical: the existing core and Books report tests are the guard. A test that compares SQL text may change; a test that compares a result may not.
- Each engine change keeps a `legacySQL` switch and a cross-check test (old result = new result, byte for byte, on the zoo).
- No migration other than `books/pb-migrations/2100000026_create_books_receivable_items.js` changes (unreleased; edited in place).
- Spec §8: no change to `books_invoices.balance`, `books_party_balances`, or payment posting; no stored receivable-items table.
- The S and L runs start only with an environment variable (`BOOKS_SCALE=1`, `BOOKS_PROFILE=<file>`); `go test` with none of them stays fast.
- Go server logging and style per the workspace `CLAUDE.md`; comments say why, not what.

## Review Focus

1. A formula reads the same aggregate two times and the argument holds a lookup: the values stay as before (task 2: `TestAnAggregateOfALookupReadTwiceKeepsTheResult`), and a lookup whose `where` is unknown on a row is named in the notice at both reads (task 2: `TestAnAggregateReadTwiceNamesAnUnknownWhereAtEachRead`; the probe goes through `compiler.probe`, so the guard counts `compiler.probed`).
2. One argument through two operators (`sum`, `max`) or one operator at two scales (`avg` scale 0 and scale 2): each keeps its own value of `a<L>` (task 2: `TestAggregatesThatDifferStayApart`).
3. A reader who sees no row (zoo user `eve`): the change returns the same empty result as before (task 2: `TestAnAggregateComputedOnceKeepsTheResult` runs every zoo user).
4. The profile test with no environment variable must not write the S dataset: it skips before any write (task 1, step 2).
5. The profile at L on a run that passes core's 10 s limit (run 5 did in the failed scale run): the run's error goes in the table (`FAILED: …`), its statements are still recorded and timed directly with no limit, and the file is written after each run (task 1: `arProfileExec`, `arProfileStatements`; no small-size test can make a run pass the limit, so the L NEEDS-RUN of step 6 is its check).
6. A column that reads its own aggregate (`sum(col C)` inside C) is refused today; a kept aggregate must not let it pass when another column computed that aggregate first (task 2: `readsVisiting`; the existing "reads itself" tests of the package are the guard, run in step 5).

---

### Task 1: Measurement harness and baseline (Books)

Expected saving: none. It gives tasks 2 to 9 their per-statement targets.

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_ar_aging_profile_test.go`
- Commit as they are (uncommitted work of checkpoint 3, now owned by this unit): `books/server/report_ar_aging_scale_test.go`, `books/server/testdata/ar_aging_stat1.json`, `books/pb-migrations/2100000026_create_books_receivable_items.js`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/ar-aging-ledger.md` (section `## Performance`)

**Interfaces:**
- Consumes (all in `report_ar_aging_scale_test.go`, read their definitions before use): `newARBook(t) arBook`, `arJournalPrefix(t, b)`, `writeARDataset(t testing.TB, b arBook, size arSize) arDataset`, sizes `arSmall`, `arS`, `arL`, `arRuns(b arBook, data arDataset) []arRun` (fields `name`; method `exec(t *testing.T, b arBook)`), `recordARRun(t *testing.T, b arBook, r arRun) []string` (the run's statements with their values written in), `explainStatements(t testing.TB, app core.App, statements []string) []planStep` (fields `Statement`, `ID`, `Parent`, `Detail`).
- Produces: `TestARAgingProfile`; `arProfileTime(t *testing.T, b arBook, sql string) (time.Duration, int)`; `arProfileFloor []string`; the output file format (a markdown table `| run | wall (median of 5) | statements | SQL time | outside SQL |`, then `FLOOR <time>  <sql>` lines, then for each run `######## RUN …`, `=== run <name> statement <i>: <time>, <rows> rows, <chars> chars`, `PLAN <id><-<parent> <detail>`, `CTE <name> cumulative <time> (<rows> rows)`); the ledger table "Baseline (p5-report-perf)".

- [ ] **Step 1: Write the profile test**

Create `books/server/report_ar_aging_profile_test.go`:

```go
package books

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// arProfileCTE finds the name of each CTE of a statement core wrote.
var arProfileCTE = regexp.MustCompile(`(?m)^(?:WITH )?(\w+) AS (?:MATERIALIZED )?\(`)

// arProfileExec runs r and returns its error. A run that passes core's time
// limit at L is a result to record, not a reason to end the profile.
func arProfileExec(t *testing.T, b arBook, r arRun) error {
	t.Helper()
	if r.export {
		r.exec(t, b)
		return nil
	}
	_, err := runARErr(t, b.app, b.reader, r.params, r.opts)
	return err
}

// arProfileStatements are the statements of r, as recordARRun reads them, for
// a run that may fail.
func arProfileStatements(t *testing.T, b arBook, r arRun) []string {
	t.Helper()
	var mu sync.Mutex
	seen := map[string]bool{}
	var out []string
	recordStatements(t, b.app, func(statement string) {
		s := strings.TrimSpace(statement)
		upper := strings.ToUpper(s)
		if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}, func() { _ = arProfileExec(t, b, r) })
	return out
}

// arProfileTime runs sql three times and returns the fastest time and the row
// count.
func arProfileTime(t *testing.T, b arBook, sql string) (time.Duration, int) {
	t.Helper()
	best, n := time.Duration(1<<62), 0
	for range 3 {
		started := time.Now()
		rows, err := b.app.DB().NewQuery(sql).Rows()
		if err != nil {
			t.Fatalf("profile statement: %v\n%.300s", err, sql)
		}
		n = 0
		for rows.Next() {
			n++
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		best = min(best, time.Since(started))
	}
	return best, n
}

// arProfileFloor are reads of the two views with no report over them: the
// least a run that reads the whole book can cost.
var arProfileFloor = []string{
	"SELECT count(*), sum(debit), sum(credit), sum(length(item)), sum(length(due_date)), sum(length(item_number)), sum(item_seq) FROM books_receivable_lines",
	"SELECT count(*), sum(debit), sum(credit), sum(length(item)), sum(length(due_date)), sum(length(item_number)), sum(item_seq) FROM books_receivable_applications",
	"SELECT count(*) FROM (SELECT customer, item, sum(debit - credit) FROM books_receivable_lines GROUP BY customer, item)",
	"SELECT count(*) FROM (SELECT customer, item, sum(debit - credit) FROM (SELECT customer, item, debit, credit FROM books_receivable_lines UNION ALL SELECT customer, item, debit, credit FROM books_receivable_applications) GROUP BY customer, item)",
}

// TestARAgingProfile writes, for each run of spec §9, where its time goes: the
// run's median wall time, each statement's time, rows and plan, and the
// cumulative time of each CTE of a long statement. It starts only with
// BOOKS_PROFILE=<output file>; BOOKS_PROFILE_SIZE is small, S (default) or L.
// It has no bar: TestARAgingScale holds the bars.
func TestARAgingProfile(t *testing.T) {
	out := os.Getenv("BOOKS_PROFILE")
	if out == "" {
		t.Skip("set BOOKS_PROFILE=<output file> to profile the A/R Aging runs")
	}
	size, ok := map[string]arSize{"small": arSmall, "": arS, "S": arS, "L": arL}[os.Getenv("BOOKS_PROFILE_SIZE")]
	if !ok {
		t.Fatalf("BOOKS_PROFILE_SIZE is small, S or L")
	}
	b := newARBook(t)
	arJournalPrefix(t, b)
	data := writeARDataset(t, b, size)
	var summary, detail strings.Builder
	fmt.Fprintf(&summary, "| run | wall (median of 5) | statements | SQL time | outside SQL |\n|---|---|---|---|---|\n")
	for _, sql := range arProfileFloor {
		d, _ := arProfileTime(t, b, sql)
		fmt.Fprintf(&detail, "FLOOR %s  %s\n", d.Round(10*time.Microsecond), sql)
	}
	for _, run := range arRuns(b, data) {
		failed := ""
		if err := arProfileExec(t, b, run); err != nil {
			failed = " FAILED: " + err.Error()
		}
		walls := make([]time.Duration, 5)
		for i := range walls {
			started := time.Now()
			_ = arProfileExec(t, b, run)
			walls[i] = time.Since(started)
		}
		slices.Sort(walls)
		statements := arProfileStatements(t, b, run)
		steps := explainStatements(t, b.app, statements)
		var sqlTime time.Duration
		fmt.Fprintf(&detail, "\n######## RUN %s: wall %s, %d statements\n", run.name, walls[2].Round(time.Millisecond), len(statements))
		for i, s := range statements {
			d, n := arProfileTime(t, b, s)
			sqlTime += d
			fmt.Fprintf(&detail, "\n=== run %s statement %d: %s, %d rows, %d chars\n", run.name, i, d.Round(10*time.Microsecond), n, len(s))
			for _, step := range steps {
				if step.Statement == i {
					fmt.Fprintf(&detail, "PLAN %d<-%d %s\n", step.ID, step.Parent, step.Detail)
				}
			}
			// The CTEs of a long statement, each read alone: the time up to it.
			// Not for the export, whose pages repeat run 1's statements.
			end := strings.LastIndex(s, ")\nSELECT ")
			if len(s) < 5000 || end < 0 || len(statements) > 10 {
				continue
			}
			head := s[:end+1]
			for _, m := range arProfileCTE.FindAllStringSubmatch(head, -1) {
				d, n := arProfileTime(t, b, head+"\nSELECT count(*) FROM "+m[1])
				fmt.Fprintf(&detail, "CTE %-6s cumulative %s (%d rows)\n", m[1], d.Round(10*time.Microsecond), n)
			}
		}
		fmt.Fprintf(&summary, "| %s%s | %s | %d | %s | %s |\n", run.name, failed, walls[2].Round(time.Millisecond), len(statements),
			sqlTime.Round(time.Millisecond), (walls[2] - sqlTime).Round(time.Millisecond))
		// Written after each run: a later run that ends the test loses nothing.
		if err := os.WriteFile(out, []byte(summary.String()+"\n"+detail.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
```

- [ ] **Step 2: Verify it skips with no variable and runs at the small size**

Run (in `/Users/nas/tmp/tinycld/books-purchases/books/server`):

```bash
go vet . && go test -run 'TestARAgingProfile$' -count=1 -v . 2>&1 | command grep -E 'SKIP|ok|FAIL'
```

Expected: `--- SKIP: TestARAgingProfile` and `ok`, in a few seconds (no dataset is written).

Run:

```bash
BOOKS_PROFILE=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-small.txt BOOKS_PROFILE_SIZE=small go test -run 'TestARAgingProfile$' -count=1 . && command grep -E '^\|' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-small.txt
```

Expected: `ok` in about 15 s; a table of 7 runs (`1 default` … `7 CSV export`), each wall time about 50 to 100 ms. At 500 lines the wall time of run 1 (about 50 ms) is the cost that does not grow with the data.

- [ ] **Step 3: Run the S baseline**

Run (about 5 minutes; when it passes 4 minutes on this machine hand back `NEEDS-RUN:` with this command instead):

```bash
BOOKS_PROFILE=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-S-baseline.txt go test -run 'TestARAgingProfile$' -count=1 -timeout 20m . && command grep -E '^\||^FLOOR' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-S-baseline.txt
```

Expected: `ok`; run 1 near 850 ms wall with 6 statements, SQL time near 660 ms and about 190 ms outside SQL; the FLOOR lines near 73, 30, 80 and 99 ms (the planner's run of this test on 2026-11-18: runs 1 to 7 = 850, 512, 852, 332, 865, 1,193, 3,618 ms). A run 1 time under 400 ms or over 1,600 ms means the tree is not the one the root cause measured: stop and report it.

- [ ] **Step 4: Record the baseline in the ledger**

In `books/docs/plans/ar-aging-ledger.md`, section `## Performance`, add after the line that starts `- failed run of 2026-11-18 18:33` a block with this form, filled from `profile-S-baseline.txt` (the table lines verbatim, then one line for each FLOOR query, then for run 1 statement 4 and statement 5 the `CTE` lines of `r0`, `w`, `a0`, `a1`, `a2`, `a3`):

```markdown
### Baseline (p5-report-perf)

Machine, date, commits (books, core): <output of `uname -sm`, `go version`, today, `git rev-parse --short HEAD` in books and in tinycld>. Size S. Harness: `TestARAgingProfile` (`BOOKS_PROFILE=<file>`), which runs each recorded statement directly, so a statement over core's 10 s run limit is still timed.

<the summary table>

Floor (the views with no report):
- <time> — `books_receivable_lines`, every column
- <time> — `books_receivable_applications`, every column
- <time> — `books_receivable_lines` grouped by customer and item
- <time> — both views, grouped by customer and item

Run 1, cumulative time by CTE: statement 4 — r0 <t>, w <t>, a0 <t>, a1 <t>, a2 <t>, a3 <t>, whole <t>; statement 5 — r0 <t>, w <t>, a0 <t>, whole <t>.

Targets at S for a whole-book run (L ≤ 1,000 ms; L was 12.3 × S in the failed run, and about 50 ms of a run does not grow): at most 75 ms of time that grows with the data, the time outside SQL included: union read ≤ 40 ms; joins and row values ≤ 12 ms; finest-level aggregation ≤ 15 ms; roll-ups and level joins ≤ 4 ms; outside SQL ≤ 4 ms; one statement that reads the source.

Outside SQL: <the "outside SQL" column of runs 1 and 4>. It grows with the data in run 1 and not in run 4, and no statement holds it: part 2 finds its cause before it sizes the one-statement task.
```

Every `<…>` above is a measured value you copy from the file or the command named; the block in the ledger holds no angle bracket.

- [ ] **Step 5: Check the package still passes and commit (Books)**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -count=1 .` (the whole package: the commit holds a migration change; `NEEDS-RUN` when it passes 4 minutes).
Expected: `ok` (the scale and profile tests skip; the small plan test and the bulk-writer test pass).

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_ar_aging_profile_test.go server/report_ar_aging_scale_test.go server/testdata/ar_aging_stat1.json pb-migrations/2100000026_create_books_receivable_items.js docs/plans/ar-aging-ledger.md
git commit -m "test(books): A/R Aging scale benchmark and per-statement profile"
```

- [ ] **Step 6: Hand the L baseline to the driver**

Hand back: `NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_PROFILE=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-L-baseline.txt BOOKS_PROFILE_SIZE=L go test -run 'TestARAgingProfile$' -count=1 -timeout 90m .` — needed from it: the table lines and the FLOOR lines, which the driver gives to the agent of task 10 for the ledger. Tasks 2 to 9 use the S numbers and do not wait for it.

---

### Task 2: An aggregate a level reads twice is computed once (core)

Expected saving at S, run 1: Books' `blankZero(sum(x))` is `if(eq(sum(x), 0), null, sum(x))`, which the engine writes as two aggregates of `a<L>` (`y2`, `y3`). The five age columns are 10 sums in each of 4 levels and 2 statements; this makes them 5. About 60 ms of statement 4 (434 ms) and 30 ms of statement 5 (194 ms). Measure with task 1's harness after the change and write the two numbers in the report.

Why SQLite does not do this itself: each compile of the argument binds its literals under new names (`{:rp7}`, `{:rp12}`), so the two aggregates are not the same text.

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go`
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/aggonce_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/levels.go` (imports; `type level`; `func (c *compiler) level`; `func (lc *levelCtx) aggregate`, near line 1400)

**Interfaces:**
- Consumes (existing; read each definition first): `newUnionZoo(t) *zoo` and source `zoo.meals` (`union_zoo_test.go`: ann reads feedings 1000 + 2500 + 700 + 300 and treats 40 + 25); `(z *zoo) run(t, user, spec string, opts Options, params map[string]string) *Result` and `requireRows` (`run_test.go`); `observeReads(t) *[]readStatement` (field `sql`; `statement_test.go`); `lookupIn`, `level.wheres`, `rowsCTE.wheres` (the probes of lookups' wheres).
- Produces: `legacySQL` (package var, a struct; field `repeatAggregates bool`; later tasks add one field each); `requireSameAsLegacy(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string, off *bool)`; `levelAggregates *regexp.Regexp` (submatch 1 the level number, 2 the rest of the `a<L> AS (` line); `requireInEachLevel(t *testing.T, seen []readStatement, text string, n int)`; consts `blankZeroMeals`, `differingMeals`, `mealsOfPet`, `lookupTwice`; `aggregateKey(op Aggregate, arg *Node, scale int) (string, bool)`; `(p *plan) columnsRead(n *Node, seen map[string]bool)`; `(lc *levelCtx) readsVisiting(arg *Node) bool`; `level.aggMemo map[string]expr`. Rule for every later switch: one test proves the switch writes the old SQL (as `TestAnAggregateALevelReadsTwiceIsComputedOnce` does with the flag on).

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/aggonce_test.go`:

```go
package reports

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// requireSameAsLegacy runs a report with one change of the performance work
// off (*off) and then on, and requires the same result, byte for byte.
func requireSameAsLegacy(t *testing.T, z *zoo, user, spec string, opts Options, params map[string]string, off *bool) {
	t.Helper()
	t.Cleanup(func() { *off = false })
	*off = true
	before, err := json.Marshal(z.run(t, user, spec, opts, params))
	if err != nil {
		t.Fatal(err)
	}
	*off = false
	after, err := json.Marshal(z.run(t, user, spec, opts, params))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("the result changed\nbefore: %s\nafter:  %s", before, after)
	}
}

// blankZeroMeals shows a kind's cost, blank when it is 0: the formula reads
// one sum two times.
const blankZeroMeals = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"paid","formula":{"op":"if","args":[
		{"op":"eq","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]},
		{"value":null},
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
	"groups":[{"id":"g","col":"type"}],"sort":[{"col":"type"}]}`

// levelAggregates finds a<L>, the aggregates of a level, in a statement.
var levelAggregates = regexp.MustCompile(`(?m)^a(\d+) AS \((.*)$`)

// requireInEachLevel requires text n times in every a<L> of every statement.
func requireInEachLevel(t *testing.T, seen []readStatement, text string, n int) {
	t.Helper()
	levels := 0
	for _, st := range seen {
		for _, m := range levelAggregates.FindAllStringSubmatch(st.sql, -1) {
			levels++
			if got := strings.Count(m[2], text); got != n {
				t.Errorf("a%s has %q %d times, want %d:\n%s", m[1], text, got, n, m[2])
			}
		}
	}
	if levels == 0 {
		t.Fatal("no statement has a level's aggregates")
	}
}

func TestAnAggregateALevelReadsTwiceIsComputedOnce(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", blankZeroMeals, Options{}, nil)
	requireRows(t, res, "paid", int64(4500), int64(65), int64(4565))
	requireInEachLevel(t, *seen, "sum(", 1)
	// The switch writes the SQL of before: the cross-checks compare with it.
	legacySQL.repeatAggregates = true
	t.Cleanup(func() { legacySQL.repeatAggregates = false })
	*seen = nil
	z.run(t, "ann", blankZeroMeals, Options{}, nil)
	requireInEachLevel(t, *seen, "sum(", 2)
}

// A sum of a lookup whose where is unknown (lookupwheremixed_test.go), read
// two times: the notice names the where at both reads.
func TestAnAggregateReadTwiceNamesAnUnknownWhereAtEachRead(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":4,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},{"id":"region","path":[{"field":"region"}]},
		{"id":"fed","formula":{"op":"if","args":[
			{"op":"eq","args":[{"op":"sum","args":[` + unknownWhere + `]},{"value":0}]},
			{"value":null},
			{"op":"sum","args":[` + unknownWhere + `]}]}}],"groups":[{"id":"r","col":"region"}],"sort":[{"col":"name"}]}`
	requireWhereNamed(t, z, spec, "columns.2.formula.args.0.args.0.args.0.where", "columns.2.formula.args.2.args.0.where")
	requireSameAsLegacy(t, z, "bob", spec, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
}

func TestAnAggregateComputedOnceKeepsTheResult(t *testing.T) {
	z := newUnionZoo(t)
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		requireSameAsLegacy(t, z, user, blankZeroMeals, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
	}
}

// differingMeals reads one argument through two operators and one operator
// at two scales: none of the four is another's value.
const differingMeals = `{"version":4,"source":"zoo.meals","columns":[
	{"id":"type","path":[{"field":"kind"}]},
	{"id":"most","formula":{"op":"max","args":[{"path":[{"field":"grams"}]}]}},
	{"id":"all","formula":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}},
	{"id":"mean0","formula":{"op":"avg","scale":0,"args":[{"path":[{"field":"grams"}]}]}},
	{"id":"mean2","formula":{"op":"avg","scale":2,"args":[{"path":[{"field":"grams"}]}]}}],
	"groups":[{"id":"g","col":"type"}],"sort":[{"col":"type"}]}`

func TestAggregatesThatDifferStayApart(t *testing.T) {
	z := newUnionZoo(t)
	seen := observeReads(t)
	res := z.run(t, "ann", differingMeals, Options{}, nil)
	// ann's feedings weigh 500, 800, 300 and 100 grams; a treat has none.
	for id, want := range map[string]string{"most": "[800 <nil> 800]", "all": "[1700 0 1700]", "mean0": "[425 <nil> 425]", "mean2": "[425 <nil> 425]"} {
		if got := fmt.Sprint(rowsOf(res, id)); got != want {
			t.Errorf("%s is %s, want %s", id, got, want)
		}
	}
	// One average for each scale, one max: the two averages are not merged.
	requireInEachLevel(t, *seen, "tc_div(", 2)
	requireInEachLevel(t, *seen, "max(", 1)
	requireSameAsLegacy(t, z, "ann", differingMeals, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
}

// mealsOfPet sums a pet's meals from the union: a lookup inside an aggregate.
const mealsOfPet = `{"op":"lookup","source":"zoo.meals","pick":"sum",
	"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"col":"pet"}]},
	"value":{"path":[{"field":"cost"}]}}`

const lookupTwice = `{"version":4,"source":"zoo.pets","columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"pet","path":[{"field":"id"}]},
	{"id":"fed","formula":{"op":"if","args":[
		{"op":"eq","args":[{"op":"sum","args":[` + mealsOfPet + `]},{"value":0}]},
		{"value":null},
		{"op":"sum","args":[` + mealsOfPet + `]}]}}],
	"groups":[{"id":"g","col":"name"}],"sort":[{"col":"name"}]}`

func TestAnAggregateOfALookupReadTwiceKeepsTheResult(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", lookupTwice, Options{}, nil)
	requireRows(t, res, "fed", int64(40), int64(3525), int64(1000), int64(4565))
	for _, user := range []string{"ann", "bob", "dan", "eve"} {
		requireSameAsLegacy(t, z, user, lookupTwice, Options{ExpandAll: true}, nil, &legacySQL.repeatAggregates)
	}
}
```

`requireSameAsLegacy` compares the whole `Result`, notices included, so a lost or moved notice of a lookup fails it.

- [ ] **Step 2: Run the tests to verify they fail**

Run (in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports`): `go test -run 'TestAnAggregate|TestAggregatesThatDiffer' -count=1 .`

Expected: a build failure, `undefined: legacySQL`. Then create `legacy.go` (step 3, first block) and run again. Expected:

```
--- FAIL: TestAnAggregateALevelReadsTwiceIsComputedOnce
    aggonce_test.go: a0 has "sum(" 2 times, want 1:
        SELECT ck0, COALESCE(sum(w.h4), 0) AS y0, COALESCE(sum(w.h4), 0) AS y1, count(*) AS __n FROM w GROUP BY ck0),
    aggonce_test.go: a1 has "sum(" 2 times, want 1:
```

The other three tests pass already (the switch changes nothing yet): they are the guards for step 3. This was run against the tree on 2026-11-18: red as above, and green with step 3; the whole `reports` package (98 s) passed with step 3 and no expectation changed.

- [ ] **Step 3: Implement**

Create `core/server/reports/legacy.go`:

```go
package reports

// legacySQL turns one change of the 2026-11 report performance work off, so a
// test runs a report with the SQL the engine wrote before it and compares the
// two results (requireSameAsLegacy). Only a test sets a field.
var legacySQL struct {
	// repeatAggregates writes an aggregate a level reads twice as two values
	// of a<L>, as before levelCtx.aggregate kept each one.
	repeatAggregates bool
}
```

In `core/server/reports/levels.go`, add `"encoding/json"` to the imports. In `type level`, after `aggNames map[string]bool`:

```go
	// aggMemo holds each aggregate a formula of the level read, by its
	// operator, scale and argument (aggregateKey): a second read of the same
	// aggregate gets the first one's value of a<L>.
	aggMemo map[string]expr
```

In `func (c *compiler) level`, add `aggMemo: map[string]expr{},` to the `&level{…}` literal (beside `aggNames: map[string]bool{}`).

Replace `func (lc *levelCtx) aggregate` whole with:

```go
// aggregateKey names an aggregate of a formula for aggMemo: two reads with
// the same operator, scale and argument are one aggregate.
func aggregateKey(op Aggregate, arg *Node, scale int) (string, bool) {
	raw, err := json.Marshal(arg)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%s|%d|%s", op, scale, raw), true
}

// columnsRead adds to seen the ids of the columns n reads, and of the columns
// those read.
func (p *plan) columnsRead(n *Node, seen map[string]bool) {
	if n == nil {
		return
	}
	if n.Col != "" && !seen[n.Col] {
		seen[n.Col] = true
		if cp := p.byID[n.Col]; cp != nil {
			p.columnsRead(cp.col.Formula, seen)
		}
	}
	p.columnsRead(n.Where, seen)
	for _, a := range n.Args {
		p.columnsRead(a, seen)
	}
}

// readsVisiting is true when arg reads a column that is being compiled: the
// compile refuses such a formula ("reads itself"), which a kept value would
// skip.
func (lc *levelCtx) readsVisiting(arg *Node) bool {
	read := map[string]bool{}
	lc.lv.c.p.columnsRead(arg, read)
	for id := range read {
		if lc.visiting[id] {
			return true
		}
	}
	return false
}

// aggregate adds an aggregate of a formula to a<L>, once per level: a formula
// that reads the same aggregate twice (a sum shown blank when it is 0) reads
// one value. SQLite does not find the two equal itself, as each compile binds
// its literals under new names. An argument whose compile added a condition
// probe (a lookup's where that may be unknown, compiler.probe) is compiled
// again at each read, so each read's probe keeps its own place in the spec.
func (lc *levelCtx) aggregate(op Aggregate, arg *Node, scale int, at string) (expr, error) {
	lv := lc.lv
	key, keyed := aggregateKey(op, arg, scale)
	if e, ok := lv.aggMemo[key]; ok && keyed && !legacySQL.repeatAggregates && !lc.readsVisiting(arg) {
		return e, nil
	}
	probes, levelWheres, rowWheres := len(lv.c.probed), len(lv.wheres), len(lv.r.wheres)
	e, err := lc.aggregateArg(lv.r, lc.visiting, false, op, arg, scale, at)
	if err != nil {
		return expr{}, err
	}
	cur, err := lv.c.totalCurrency(e, at+".args.0")
	if err != nil {
		return expr{}, err
	}
	name := fmt.Sprintf("y%d", len(lv.aggs))
	e = countedArg(op, e)
	total := expr{sql: lv.agg(name, aggregateSQL(op, e.sql, scale)), typ: aggregateType(op, e.typ, scale)}
	out := withCurrency(lv.mixedTotal(name, e, total), cur)
	if keyed && len(lv.c.probed) == probes && len(lv.wheres) == levelWheres && len(lv.r.wheres) == rowWheres {
		lv.aggMemo[key] = out
	}
	return out, nil
}
```

Do not change `level.agg`, `recompute`, `rowsTotal`, `toManyTotal` or `pivot.go`: their aggregates have names fixed by the column (`x<idx>`, `q<j>`), which other code reads by name.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go vet . && go test -run 'TestAnAggregate|TestAggregatesThatDiffer' -count=1 .`
Expected: `ok`.

- [ ] **Step 5: Run the report packages**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports && go test -count=1 .`
Expected: `ok`. A test that fails because it compares SQL text or a bound parameter's name (`rp<n>`: a memo hit binds fewer parameters, so later names move) is updated to the new text; a test that fails on a result (a cell, a row, a notice) is a defect of this task: fix the code, never the expectation.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'Report|ARAging' -count=1 .`
Expected: `ok` (the Books reports give the same results; the small plan test still meets the plan bars).

- [ ] **Step 6: Measure**

Run task 1's S profile to `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/profile-S-task2.txt` (the command of task 1 step 3 with that file name; `NEEDS-RUN` when it passes 4 minutes). In your report give, for run 1, statement 4 and statement 5 before (from `profile-S-baseline.txt`) and after, and the count of `sum(` in the `a3 AS (` line before and after. Expected: statement 4 lower by about 60 ms, statement 5 by about 30 ms. A saving under 20 ms in statement 4 is reported as such with the two `a3` lines; it does not fail the task.

- [ ] **Step 7: Commit (core)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/legacy.go core/server/reports/aggonce_test.go core/server/reports/levels.go
git commit -m "perf(reports): a level computes an aggregate it reads twice one time"
```

(Add any test file whose SQL-text expectation step 5 updated.)

---

## What is left (part 2)

Tasks 3 to 10 of the outline (`/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/plan-outline.md`): row values once per row; finest-level aggregation with roll-up; one statement for the page and the Total; filters and opened keys pushed into union parts; the one-evaluation export; the Books views and indexes; the scale run and the ledger record with the stop rule. Each adds its field to `legacySQL` and its cross-check with `requireSameAsLegacy`.
