# Report Engine Scale, Part 4: The Detail Statement Reads the Period's Rows Once

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A General Ledger run with an Accounts filter takes 1 s or less on a book with 500,000 journal lines, with every report's result unchanged.

**Architecture:** The detail statement of a report with sections (the longest statement of every General Ledger run) names the report's rows `w` in several CTEs: a section's fills (`f1`), its aggregates (`a1`) and the sorted rows (`ws`). SQLite computes an inline CTE once for each read, so the period's lines are read once for each read of `w`. This part makes `w` `MATERIALIZED` in such a statement, as the group statement already has it. A detail page with no section CTEs keeps `w` inline, so it still stops at its `LIMIT`. The rule is by statement shape, so it also marks the Total row's statement (`totalPass`, `run.go:1026`, body `SELECT * … FROM o0`: its level CTEs `a0`, `a1` read `w`) and, on a grouped report, the read of the rows shown now (`readShownNow`) and the probe of the conditions (`probeConditions`). The Total statement has a unit case and its time is read in step 7; the other two have no test and no measurement of their own (there the cost is one copy of `w`, which their `a<m>` CTE reads whole in both forms).

**Tech Stack:** Go, SQLite (PocketBase fork), core report engine `tinycld.org/core/reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause). The unit's five requirements, the baseline and the stored-summary decision: part 1, `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-19-report-engine-scale.md`. Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (its last section gives the task list this part follows). The measurements this part rests on: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part4b.md`, section "Result".

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`); Books is `books/` (same branch name). Task 5c is a core task: it may edit `tinycld/core`. Its last step changes one Books file, the ledger record.

**Order:** parts 1 to 3 are done (core c7d45a8c, books 0ef61668). This part has one task, 5c. Tasks 6, 5, 5d and 7 are the next part (section "What is left" at the end).

## Global Constraints

- The bar is never lowered: runs 3, 4 and 5 of `TestGLScale` (the runs with an Accounts filter) take 1 s or less at 500,000 lines, median of 5. No task changes a bar, removes a run, or changes the scale book to pass.
- Every report's rows stay the same. A change to the engine's SQL has a switch in `legacySQL` (`core/server/reports/legacy.go`), one test that proves the switch writes the old SQL, and a cross-check that runs a report with the switch on and off and compares the two results byte for byte (`requireSameAsLegacy`).
- Core names no package: core tests use the fictional zoo only (`zoo.ledger_lines`, `zoo_ledger_lines`); no Books slug, collection or route in `tinycld/core`.
- Each change to SQL is recorded with its plan and its time before and after in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)", under "Changes".
- The 100k scale run takes more than 4 minutes with its build, and the 500k run about 13 minutes: an implementer starts neither. Each is a `NEEDS-RUN` for the driver.
- The whole core `reports` package takes about 95 s. Run it once, at the end of the task.
- Go code is formatted with `gofmt`. Server logging goes through the package's `log`.

## Review Focus

1. A plain detail page (no groups, a sort and a `LIMIT`) whose body reads `w` twice (the page and an `EXISTS` probe): `w` must stay inline, or every page computes every row of the report before it returns 50. Test: task 5c, `TestMaterializeSharedMarksADetailStatementsRowsItsCTEsReadTwice`, cases "the page and a probe" and "one CTE and the page"; the existing `TestMaterializeSharedMarksRowsReadTwice` case "detail with a probe" must still pass unchanged.
2. A detail statement whose only CTE over `w` is the sorted rows `ws`, read two times by the body: it is not marked (one CTE read). This is the behavior of before; the test case "the sorted rows only" pins it so that a later change is a decision, not an accident.
3. A report with no Accounts filter (runs 1 and 2: `w` is every line of the period): a materialized `w` holds all of them in a temporary table. The unit gives these runs no bar, but they must not get slower. No test; step 7: it compares the 100k medians of runs 1 and 2 with the last record.
4. A reader with no grant, a mixed-currency book, a page seam inside a section and between sections, a view sort, All dates: the pages are the same with `w` computed once. Test: task 5c, `TestDetailRowsComputedOnceKeepTheResult` (every page of every run of `ledgerRuns()`, through `ledgerPages`, on a plain and a mixed book; the first page for reader `bob`).
5. A group statement with the switch on: the switch turns off the new rule only; the group statement keeps its `MATERIALIZED`. Test: the last check of `TestMaterializeSharedMarksADetailStatementsRowsItsCTEsReadTwice`.

---

### Task 5c: A detail statement with sections computes `w` once (core)

**Why:** measured at 100,000 lines on this tree (hand-off note, "Result"): `w AS MATERIALIZED` alone takes statement 6 of run 5 from 870 ms to about 600 to 766 ms, and of run 3 from 208 ms to 136 to 141 ms. The plan then reads `books_journal_lines` by `(account=? AND date>? AND date<?)` once, not 7 times.

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/legacy.go` (the struct `legacySQL`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rows.go:647-675` (`materializeShared`)
- Create (test): `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rowsonce_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (section "Changes", step 8)

**Interfaces:**
- Consumes (all in the tree; read each definition before use):
  - `materializeShared(ctes []string, body string) []string` and `var rowsRead` (`rows.go`); its one caller is `readRows` (`run.go:606`), which every read statement goes through.
  - `var legacySQL struct{ repeatAggregates, noFilterBounds bool }` (`legacy.go`).
  - Test helpers: `newZoo(t)`, `withBalanceZoo(t, z)` (`balance_shape_test.go`), `withLedgerScale(t, z, n)`, `withLedgerLines(t, z, n, mixed)`, `runLedger(z, params, opts) (*Result, error)`, `ledgerRuns() []ledgerRun`, `ledgerPeriod` (`ledger_scale_test.go`), `ledgerAccounts(ids ...int) string`, `var rowsWhere` (`bounds_test.go`), `observeReads(t) *[]readStatement`, `rawStatement(tb, sql, params) (string, []any)`, `readPool(t, z) *sql.DB` (`statement_test.go`), `queryPlan(t, db, stmt, args) []string` and `balanceSpec() string` (`balance_shape_test.go`), `requireSameAsLegacy(t, z, user, spec, opts, params, off *bool)` (`aggonce_test.go`).
- Produces: `legacySQL.inlineDetailRows bool`; test helper `periodReads(t *testing.T, z *zoo, seen []readStatement) (least, most int)`. The rule reads the statement's body (the text after the WITH clause): a body with no `__level` is not a group statement, and after this task its `w` starts `w AS MATERIALIZED (` when two or more of its other CTEs read `w`. That covers the detail statement and the Total statement. The test helper `periodReads` reads the whole SQL, so it sees the detail statement only (the Total statement's CTEs name `__level`); the unit case "the Total row's levels" covers the Total statement. Also consumed: `ledgerPages(t, z, r) []*Result`, `ledgerCanonical(t, z, pages) string` (`ledger_scale_test.go`).

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/rowsonce_test.go`:

```go
package reports

import (
	"strings"
	"testing"
)

// A detail statement whose section CTEs read w twice or more computes it
// once; one whose only reads are its page's keeps it inline.
func TestMaterializeSharedMarksADetailStatementsRowsItsCTEsReadTwice(t *testing.T) {
	const inline, once = "w AS (SELECT 1)", "w AS MATERIALIZED (SELECT 1)"
	section := []string{inline, "f1 AS (SELECT k0 FROM w GROUP BY k0)", "a1 AS (SELECT k0, count(*) FROM w GROUP BY k0)", "ws AS (SELECT w.* FROM w)"}
	cases := []struct {
		name, body, want string
		ctes             []string
	}{
		{"a section's CTEs", "SELECT * FROM ws LIMIT 5", once, section},
		// The Total statement: its level CTEs read w, its body reads the
		// Total row and names no __level.
		{"the Total row's levels", "SELECT * FROM o0", once, []string{inline, "a0 AS (SELECT 0 AS __level FROM w)", "a1 AS (SELECT k0 FROM w GROUP BY k0)", "o0 AS (SELECT * FROM a0)"}},
		{"the sorted rows only", "SELECT * FROM ws JOIN ws w2 LIMIT 5", inline, []string{inline, "ws AS (SELECT w.* FROM w)"}},
		{"one CTE and the page", "SELECT * FROM w JOIN f1 LIMIT 5", inline, []string{inline, "f1 AS (SELECT k0 FROM w GROUP BY k0)"}},
		{"the page and a probe", "SELECT * FROM w WHERE EXISTS (SELECT 1 FROM w WHERE x) LIMIT 5", inline, []string{inline}},
	}
	for _, c := range cases {
		if got := materializeShared(c.ctes, c.body); got[0] != c.want {
			t.Errorf("%s: %q, want %q", c.name, got[0], c.want)
		}
	}
	// The switch writes the SQL of before: the cross-check compares with it.
	legacySQL.inlineDetailRows = true
	t.Cleanup(func() { legacySQL.inlineDetailRows = false })
	if got := materializeShared(section, "SELECT * FROM ws LIMIT 5"); got[0] != inline {
		t.Errorf("with the switch on: %q, want %q", got[0], inline)
	}
	if got := materializeShared(section, "SELECT __level FROM a1"); got[0] != once {
		t.Errorf("a group statement with the switch on: %q, want %q", got[0], once)
	}
}

// periodReads counts, in each detail statement of seen that has the report's
// rows, the plan steps that read the ledger lines by account and day range:
// the reads of the period's own rows. It returns the smallest and the largest
// count, and fails when no detail statement has the rows. The Total statement
// is left out on purpose (its level CTEs name __level): the unit case "the
// Total row's levels" covers it.
func periodReads(t *testing.T, z *zoo, seen []readStatement) (least, most int) {
	t.Helper()
	db := readPool(t, z)
	least = -1
	for _, st := range seen {
		if strings.Contains(st.sql, "__level") || !rowsWhere.MatchString(st.sql) {
			continue
		}
		raw, args := rawStatement(t, st.sql, st.params)
		n := 0
		for _, step := range queryPlan(t, db, raw, args) {
			if strings.Contains(step, "zoo_ledger_lines USING INDEX idx_zoo_ledger_lines_account_date (account=? AND date>? AND date<?)") {
				n++
			}
		}
		if least < 0 || n < least {
			least = n
		}
		most = max(most, n)
	}
	if least < 0 {
		t.Fatal("no detail statement has the report's rows")
	}
	return least, most
}

// The detail statement of a report with sections reads the period's rows in
// each section CTE: the lines table is read once for them, not once per CTE.
func TestADetailStatementWithSectionsReadsThePeriodsRowsOnce(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerScale(t, z, 40)
	params := map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2, 4)}
	opts := Options{Today: "2025-07-15", ExpandAll: true}
	seen := observeReads(t)
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if least, most := periodReads(t, z, *seen); least != 1 || most != 1 {
		t.Fatalf("a detail statement reads the period's rows %d to %d times, want once", least, most)
	}
	legacySQL.inlineDetailRows = true
	t.Cleanup(func() { legacySQL.inlineDetailRows = false })
	*seen = nil
	if _, err := runLedger(z, params, opts); err != nil {
		t.Fatal(err)
	}
	if least, _ := periodReads(t, z, *seen); least < 2 {
		t.Fatalf("with the switch on, a detail statement reads the period's rows %d times, want 2 or more", least)
	}
}

// Every run of the ledger fixture returns the same pages, the first and each
// one after it, with w computed once in the detail statement and with w
// inline.
func TestDetailRowsComputedOnceKeepTheResult(t *testing.T) {
	t.Cleanup(func() { legacySQL.inlineDetailRows = false })
	for _, mixed := range []bool{false, true} {
		z := newZoo(t)
		withBalanceZoo(t, z)
		withLedgerLines(t, z, 45, mixed)
		for _, r := range ledgerRuns() {
			legacySQL.inlineDetailRows = true
			before := ledgerCanonical(t, z, ledgerPages(t, z, r))
			legacySQL.inlineDetailRows = false
			if after := ledgerCanonical(t, z, ledgerPages(t, z, r)); after != before {
				t.Fatalf("%s (mixed %v): the pages changed\nbefore: %s\nafter:  %s", r.name, mixed, before, after)
			}
		}
		requireSameAsLegacy(t, z, "bob", balanceSpec(), Options{Today: "2025-07-15", ExpandAll: true},
			map[string]string{"period": ledgerPeriod, "accounts": ledgerAccounts(0, 2)}, &legacySQL.inlineDetailRows)
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:
`go test -run 'TestMaterializeSharedMarksADetailStatements|TestADetailStatementWithSections|TestDetailRowsComputedOnce' -count=1 ./reports/`

Expected: the build fails with `legacySQL.inlineDetailRows undefined`. A build failure is not yet a behavioral red, so add the field alone (step 3) and run again.

- [ ] **Step 3: Add the switch**

In `core/server/reports/legacy.go`, after the field `noFilterBounds bool`:

```go
	// inlineDetailRows keeps w inline in every detail statement, as before
	// materializeShared marked the ones whose CTEs read it twice or more.
	inlineDetailRows bool
```

Run the command of step 2 again. Expected: FAIL, with
- `TestMaterializeSharedMarksADetailStatementsRowsItsCTEsReadTwice`: `a section's CTEs: "w AS (SELECT 1)", want "w AS MATERIALIZED (SELECT 1)"`, and the same for `the Total row's levels`;
- `TestADetailStatementWithSectionsReadsThePeriodsRowsOnce`: `a detail statement reads the period's rows <n> to <n> times, want once`, with n of 2 or more;
- `TestDetailRowsComputedOnceKeepTheResult`: PASS (the switch changes nothing yet).

- [ ] **Step 4: Change `materializeShared`**

In `core/server/reports/rows.go`, replace the comment and the start of `materializeShared` (from the line `// materializeShared makes w MATERIALIZED when a group statement` through the line `if reads > 1 {`) with:

```go
// materializeShared makes w MATERIALIZED when a statement reads it twice or
// more: SQLite otherwise computes the rows, and reads the rule under them,
// once per read. A group statement (its body names __level) counts every
// read (one per group level, the fills). Any other statement (a detail page,
// the Total row's statement, a read of the rows shown now, a probe of the
// conditions) counts only the reads of its other CTEs (a section's fills,
// its aggregates, the sorted rows, the levels under the Total row): two of
// them mean the statement computes every row of w before its page, so one
// copy costs less than a read each. The count includes a CTE the body does
// not reach; there the cost is the copy alone. A detail page whose only
// reads are its body's (the page, a probe) keeps w inline, so it can stop at
// its LIMIT. The rule CTEs are never materialized: a correlated lookup must
// keep searching the table by its index.
func materializeShared(ctes []string, body string) []string {
	grouped := strings.Contains(body, "__level")
	if !grouped && legacySQL.inlineDetailRows {
		return ctes
	}
	for i, cte := range ctes {
		if !strings.HasPrefix(cte, "w AS (") {
			continue
		}
		reads := 0
		if grouped {
			reads = len(rowsRead.FindAllString(body, -1))
		}
		for j, other := range ctes {
			if j != i {
				reads += len(rowsRead.FindAllString(other, -1))
			}
		}
		if reads > 1 {
```

The rest of the function (the clone, the `MATERIALIZED` line, the two returns) stays as it is.

- [ ] **Step 5: Run the tests to see them pass**

Run, in `core/server`:
`go vet ./reports/ && go test -run 'TestMaterializeShared|TestADetailStatementWithSections|TestDetailRowsComputedOnce|TestGroupStatementMaterializes' -count=1 ./reports/`

Expected: `ok  	tinycld.org/core/reports` (about 8 s). `TestMaterializeSharedMarksRowsReadTwice` (`layout_cost_test.go`, unchanged) is in the run: its case "detail with a probe" proves a plain page keeps `w` inline.

- [ ] **Step 6: Run the whole package once, and commit core**

Run, in `core/server`: `gofmt -l ./reports/ && go test -count=1 ./reports/`

Expected: `gofmt` prints nothing; `ok  	tinycld.org/core/reports` in about 95 s. A test that fails because it reads a detail statement's `w AS (` text is fixed at the test's pattern (`w AS (?:MATERIALIZED )?\(`, as `rowsWhere` in `bounds_test.go` has it), never by a change to the rule.

Commit only when your prompt permits commits (lane rule 7); if it does not, name the three files in your report and the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/legacy.go core/server/reports/rows.go core/server/reports/rowsonce_test.go
git commit -m "perf(reports): a detail statement with sections computes its rows once"
```

- [ ] **Step 7: Hand the 100k run to the driver**

Do not start it. Hand back:

`NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5c go test -run 'TestGLScale$/100k$' -timeout 20m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task5c.log 2>&1` (make the directory `scale-task5c` first).

What is needed from it: `scale-task5c/gl-results.md` (the five medians) and `scale-task5c/gl-plans.md` (for runs 1, 2, 3 and 5: the time of each long statement, the Total statement and the detail statement among them, and the plan lines of `books_journal_lines`).

The implementer's work ends here. Step 8 and the comparison below are done by the agent that receives the run's files.

Compare with the last record (ledger, "Task 4", the column "after": run 1 730 ms, run 2 2,157 ms, run 3 334 ms, run 4 194 ms, run 5 1,205 ms; statement times of before: `scale-task4/gl-plans.md` in the staging folder):
- Runs 3, 4 and 5 are expected lower (run 5 by about 100 to 270 ms).
- One 100k run varies by up to about 25% (the hand-off note has one statement at 597 and at 766 ms). When run 1 or run 2 is more than 25% above its last record, hand back a second `NEEDS-RUN` of the same command (output to `scale-task5c-b`). Only when both runs are above, hand back `STOPPED: w MATERIALIZED slows a run with no Accounts filter` with the medians and the times of that run's Total and detail statements from the two `gl-plans.md` files, before and after. Do not change the rule to pass.

- [ ] **Step 8: Record the change in the Books ledger, and commit Books (the agent that receives the run)**

Read `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` from the heading `### Changes` to the end of the section, and append after the record of task 4, in the same form as that record (read from the run's files, never typed from this plan):

```markdown
#### Task 5c: a detail statement with sections computes its rows once (core `server/reports/rows.go`, <date>)

The detail statement read the period's lines once for each CTE that names `w` (the section's fills, its aggregates, the sorted rows). `w` is now `MATERIALIZED` there, as in the group statement, and in the Total row's statement (its level CTEs read `w`). A detail page with no section CTEs keeps `w` inline.

| run, 100k lines | before | after |
| --- | --- | --- |
| 1 all accounts, month | 730 ms | <median> |
| 2 all accounts, year | 2,157 ms | <median> |
| 3 busy account, month | 334 ms | <median> |
| 4 small account, month | 194 ms | <median> |
| 5 three accounts, year | 1,205 ms | <median> |

Plan, the detail statement of run 5, reads of `books_journal_lines` by `(account=? AND date>? AND date<?)`: before <count>; after <count>. The Total statement of run 5: before <count>, <time>; after <count>, <time>.
```

Where each `<…>` comes from:
- `<median>`: `scale-task5c/gl-results.md`, the row of that run (`| 100000 | 5 three accounts, year | … |`), the column `median of 5`, written in ms as the task 4 record has it (`1.205s` is 1,205 ms).
- `<count>` and `<time>` "after": `scale-task5c/gl-plans.md`, under the heading `## 100k lines, run 5 three accounts, year`. Find the statements with `command grep -n -E '^## 100k lines, run 5|^### statement|account=\? AND date>\? AND date<\?' <file>` and count the lines under each statement. Each statement has a heading `### statement <n>: <time>, <rows> rows, <chars> chars`: the detail statement is statement 6 (201 rows), the Total statement is statement 5 (1 row).
- "before": the same reads of `scale-task4/gl-plans.md`. Never type a count from this plan.
- The "before" column of the table is the task 4 record's "after" column: copy it from the ledger, and correct the figures above when they differ.

Commit only when your prompt permits commits; if it does not, the driver commits:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs(books): record the once-computed detail rows at 100k lines"
```

---

## What is left (the next plan part)

Part 5 of the plan is written from the outline (`plan-outline.md`, its last section) and the hand-off note `handoff-backend-planner-part4b.md`:

- **6. core · Day bounds in a lookup's where**, then **5. core · The lookup's link bound is an equality when the outer value is not NULL** (a `CASE` over two forms of the subquery; no grouped CTE). Design facts, read from the code: the note's section "Facts for the design of tasks 5 and 6". Together they cut statement 6 by 8% (run 5) to 20% (run 3) before task 5c; with task 5c, to 490 to 516 ms (run 5) and 90 to 98 ms (run 3) at 100k.
- **5d. core · A detail row's lookups are computed for the page's rows only.** Not designed: its planner first times the parts of the final detail select (the note's section "First thing to measure"). After 5c, 6 and 5 the final select is about 430 ms of run 5's 513 ms at 100k, so run 5 does not reach the bar without it.
- **7. books · Scale run and record.** The 500k `NEEDS-RUN`, the plan pins in `TestGLScaleBook`, the ledger record; `STOPPED` when a filtered run is over 1 s.
- Then the human's two decisions (large reports do not live-refresh; the Trial Balance, the Balance Sheet and the Transaction Detail reach the same bar).

Facts for the next planner (built by this part's planner, 2026-11-19):

- Task 5c was built in an overlay on this tree: the three new tests pass (after the review's changes too), the whole core `reports` package passes (92 s, run before the review's changes to the test file and the comment), and the Books probe (`part4b-scratch/probe_test.go`, 500 lines) shows `w AS MATERIALIZED (` in the detail statement of runs 3 and 5, the SQL of the probe's variant "w materialized". Scratch copies: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part4c-scratch/` (`rows.go`, `legacy.go`, `rowsonce_test.go`, `overlay-5c.json`).
- The zoo ledger fixture serves tasks 6 and 5 as it is: `glOpening` (`balance_shape_test.go:162`) has a sum lookup with `eq(account link, record id)` and `lt(date, <param bound>)`, and `zoo_ledger_lines` has the index `idx_zoo_ledger_lines_account_date`.
