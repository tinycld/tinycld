# Report Engine Query Scaling Implementation Plan — part 12 (task 9a: the scale measure of the Trial Balance, the Balance Sheet and Transaction Detail)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A measure that times the Trial Balance, the Balance Sheet and Transaction Detail by Account at 10,000, 100,000 and 500,000 journal lines and holds each run to the human's bar, with its 100,000-line baseline recorded, so that the next part can choose the change with the largest measured gain.

**Architecture:** One new Go test file in Books, beside the General Ledger's scale harness. It uses that harness's book (`writeGLScaleBook`) and adds cash-basis rows to it, runs the three shipped reports through `reports.Run`, and writes the times and the query plans to files. It changes no report and no engine code. The changes that make the runs fast are tasks 9b to 9d (the outline, section "State after plan part 12"); they are not in this part, because no time could be measured when this part was written (the machine was not quiet).

**Tech Stack:** Go, SQLite (PocketBase fork), core report engine `tinycld.org/core/reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause). The human's decision for this task (2026-11-19 and 2026-11-20): the Trial Balance, the Balance Sheet and Transaction Detail are fixed in this unit; at 500,000 journal lines a run filtered to one month takes 1 s or less and a filtered year run 2 s or less (the General Ledger's bar, `glMonthBar` and `glYearBar` in `books/server/report_general_ledger_scale_test.go`). Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`. The times from before this unit: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/other-reports-scale.txt` (Trial Balance 5.2 s and Balance Sheet 6.5 s at 500k; Transaction Detail 2 s at 10k and refused at 100k).

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Books is `books/` (branch `books/p5-report-engine-scale`, at 7912df04 when this part was written); core is `tinycld/` (same branch name, a6759c72). Task 9a is a Books task: it edits nothing under `tinycld/`.

## Global Constraints

- The bar is the human's and is never lowered: at 500,000 lines `glMonthBar` is 1 s and `glYearBar` is 2 s (median of 5). No task changes a bar, removes a run, or changes the scale book to pass.
- How the bar maps to the three reports (this part's decision; the hand-back names it so the human sees it):
  - Transaction Detail has a period and an Accounts filter, as the General Ledger has. Its runs are the General Ledger's five: the three with an Accounts filter carry the bar (month 1 s, year 2 s), the two with no Accounts filter are recorded with no bar. A sixth run (busy account, month, Cash method) carries the month bar.
  - A statement "as of" a day has no period: its figure is the whole history up to that day. A Trial Balance with an Accounts filter is held to the month bar (1 s). The Trial Balance of every account and the Balance Sheet (it has no Accounts filter, and it reads two days: the as-of day and the same day a year earlier) are held to the year bar (2 s), on Accrual and on Cash.
- The measure is expected to fail its bars at 500k until tasks 9b to 9d are built. This task runs the 100k size only, where no bar is held.
- A timing run starts only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the 1-minute load (`sysctl -n vm.loadavg`) is under 4. An implementer starts no timing run: it is a `NEEDS-RUN` for the driver.
- Books Go tests read the migrations from disk. Go code is formatted with `gofmt`.
- The record of each measure goes in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)".

## Review Focus

1. A month with no line (the small book has none in its last month): a run with a bar that shows no group row measures nothing and must fail, not pass fast. Test: `TestStatementsScale` fails a run with a bar and no group row at every size; `TestStatementsScaleBook` demands group rows for each run whose period holds a line.
2. A run core refuses (its time limit, 10 s): it is recorded as refused, fails its bar at 500k, and its statements are not timed again (each would take the time limit). Code: the `refused` map of `TestStatementsScale`.
3. A Cash run on a book with no cash-basis rows reads an empty table and proves nothing. Test: `TestStatementsScaleBook` demands cash rows, and that they add to zero (the Balance Sheet stays in balance on Cash).
4. A bar or a run removed without notice. Test: `TestStatementsScaleBars` pins the eleven runs and their bars.
5. The plans of the union source: a read of `books_cash_basis_lines` or of `books_account_balances` must show in `st-plans.md`, not only `books_journal_lines`. Code: the three table names in the plan filter of `TestStatementsScale`.

---

### Task 9a: The scale measure of the three reports, and its 100k baseline (Books)

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_statements_scale_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (the end of section "Performance (p5-report-engine-scale)", which is the end of the file)

**Interfaces:**
- Consumes (existing Books test code, package `books`): `writeGLScaleBook(t *testing.T, size arSize) glScaleBook`, `glScaleSizes() []arSize`, `glPeriod(from, to string) map[string]any`, the consts `glMonthBar`, `glYearBar` (`report_general_ledger_scale_test.go`, `report_general_ledger_test.go`); `arSmall`, `arMedian(fn func()) time.Duration`, `explainStatements(t testing.TB, app core.App, statements []string) []planStep`, `arProfileTime(t *testing.T, b arBook, sql string) (time.Duration, int)` (`report_ar_aging_scale_test.go`, `report_ar_aging_profile_test.go`); `recordStatements(t testing.TB, app *tests.TestApp, seen func(string), fn func())` (`sales_document_deferral_test.go`); `trialBalanceShipped()`, `balanceSheetShipped()`, `transactionDetailShipped()` (each returns `reports.Shipped`).
- Produces (for tasks 9b to 9d): `writeStatementsScaleBook(t *testing.T, size arSize) glScaleBook`; `type stScaleRun struct{ name string; shipped func() reports.Shipped; params map[string]any; bar time.Duration }`; `stScaleRuns(b glScaleBook) []stScaleRun`; `(r stScaleRun) exec(t *testing.T, b glScaleBook) (*reports.Result, error)`; `stScaleStatements(t *testing.T, b glScaleBook, r stScaleRun) []string`; `stGroupRows(res *reports.Result) int`; the const `stCashEvery = 4`; the tests `TestStatementsScaleBook`, `TestStatementsScaleBars`, `TestStatementsScale` (`BOOKS_SCALE=1`; `BOOKS_SCALE_OUT=<dir>` writes `st-results.md` and `st-plans.md`). Run names: `TB 1 all accounts, as of`, `TB 2 busy account, as of`, `TB 3 three accounts, as of`, `BS 1 accrual, as of`, `BS 2 cash, as of`, `TD 1 all accounts, month`, `TD 2 all accounts, year`, `TD 3 busy account, month`, `TD 4 small account, month`, `TD 5 three accounts, year`, `TD 6 busy account, month, cash`.

Facts the code depends on (checked on this tree):
- The bulk writer of the scale book writes no row in `books_cash_basis_lines`, so a Cash run would read an empty table. `writeStatementsScaleBook` adds the rows with one raw INSERT. The table's other columns (`bill`, `invoice`, `payment_entry`, `vendor`, and the columns later migrations add) have the default `''`.
- The small book (`arSmall`) has its last posted line on 2026-10-29 and its run day is 2026-11-18, so its month runs have no line and show only the Total row. That is why the book check asks for group rows only where the period holds a line.
- The Trial Balance reads `books.journal_lines`; the Balance Sheet and Transaction Detail read the union source `books.basis_lines` (parts `books_journal_lines` and `books_cash_basis_lines`).

- [ ] **Step 1: Write the test file, with the book that has no cash rows yet**

Create `/Users/nas/tmp/tinycld/books-purchases/books/server/report_statements_scale_test.go`:

```go
package books

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/reports"
)

// The scale measure of the Trial Balance, the Balance Sheet and Transaction
// Detail by Account (p5-report-engine-scale): the General Ledger's scale book
// with cash-basis rows (writeStatementsScaleBook) and its bar (glMonthBar, glYearBar). The book check runs
// in every test run (TestStatementsScaleBook); the three sizes start only with
// BOOKS_SCALE=1 (TestStatementsScale).
//
// How the bar maps (the human, 2026-11-20: at 500,000 lines a run filtered to
// one month takes 1 s or less, a filtered year run 2 s or less):
//   - Transaction Detail has a period and an Accounts filter, as the General
//     Ledger has: its runs and bars are the General Ledger's five (the runs
//     with an Accounts filter carry the bar; the two with none are recorded).
//   - A statement "as of" a day has no period: its figure is the whole history
//     up to that day. A Trial Balance with an Accounts filter is held to the
//     month bar. The Trial Balance of every account and the Balance Sheet (it
//     has no Accounts filter, and it reads two days: the as-of day and the
//     same day a year earlier) are held to the year bar.

// stCashEvery is the share of the book's entries that get cash-basis rows: one
// entry in stCashEvery.
const stCashEvery = 4

// writeStatementsScaleBook is the General Ledger's scale book with cash-basis
// rows, which the bulk writer does not write: for one entry in stCashEvery,
// each of its posted lines taken out again (debit and credit changed over) on
// the line's own day, as the cash basis takes out an unpaid invoice. An
// entry's cash rows add to zero, so a Cash run stays in balance, and a Cash
// run reads about a quarter more rows than an Accrual run.
func writeStatementsScaleBook(t *testing.T, size arSize) glScaleBook {
	t.Helper()
	return writeGLScaleBook(t, size)
}

// stScaleRun is one run of one of the three reports.
type stScaleRun struct {
	name    string
	shipped func() reports.Shipped
	params  map[string]any
	// bar is the most the run's median may take at 500,000 lines; 0 for a
	// run with no bar.
	bar time.Duration
}

// stScaleRuns are the runs of the measure, on the day of the book's last line.
func stScaleRuns(b glScaleBook) []stScaleRun {
	month := glPeriod(b.lastDay[:8]+"01", b.lastDay)
	year := glPeriod(b.lastDay[:5]+"01-01", b.lastDay)
	busy := []string{b.receivable.Id}
	three := []string{b.receivable.Id, b.incomes[0].Id, b.docs.checking.Id}
	return []stScaleRun{
		{name: "TB 1 all accounts, as of", shipped: trialBalanceShipped, params: map[string]any{"asOf": b.lastDay}, bar: glYearBar},
		{name: "TB 2 busy account, as of", shipped: trialBalanceShipped, params: map[string]any{"asOf": b.lastDay, "accounts": busy}, bar: glMonthBar},
		{name: "TB 3 three accounts, as of", shipped: trialBalanceShipped, params: map[string]any{"asOf": b.lastDay, "accounts": three}, bar: glMonthBar},
		{name: "BS 1 accrual, as of", shipped: balanceSheetShipped, params: map[string]any{"asOf": b.lastDay}, bar: glYearBar},
		{name: "BS 2 cash, as of", shipped: balanceSheetShipped, params: map[string]any{"asOf": b.lastDay, "method": "cash"}, bar: glYearBar},
		{name: "TD 1 all accounts, month", shipped: transactionDetailShipped, params: map[string]any{"period": month}},
		{name: "TD 2 all accounts, year", shipped: transactionDetailShipped, params: map[string]any{"period": year}},
		{name: "TD 3 busy account, month", shipped: transactionDetailShipped, params: map[string]any{"period": month, "accounts": busy}, bar: glMonthBar},
		{name: "TD 4 small account, month", shipped: transactionDetailShipped, params: map[string]any{"period": month, "accounts": []string{b.incomes[0].Id}}, bar: glMonthBar},
		{name: "TD 5 three accounts, year", shipped: transactionDetailShipped, params: map[string]any{"period": year, "accounts": three}, bar: glYearBar},
		{name: "TD 6 busy account, month, cash", shipped: transactionDetailShipped, params: map[string]any{"period": month, "accounts": busy, "method": "cash"}, bar: glMonthBar},
	}
}

func (r stScaleRun) exec(t *testing.T, b glScaleBook) (*reports.Result, error) {
	t.Helper()
	spec, info, err := reports.Load(r.shipped().Spec)
	if err != nil {
		t.Fatalf("Load %s: %v", r.name, err)
	}
	given := map[string]json.RawMessage{}
	for id, v := range r.params {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("param %s: %v", id, err)
		}
		given[id] = raw
	}
	return reports.Run(context.Background(), b.app, reports.Request{
		Spec: spec, Loaded: info, Params: given, Options: reports.Options{Today: b.lastDay}, Auth: b.reader,
	})
}

// stScaleStatements are the distinct SELECTs of r, in order.
func stScaleStatements(t *testing.T, b glScaleBook, r stScaleRun) []string {
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
	}, func() { _, _ = r.exec(t, b) })
	return out
}

// stGroupRows counts the group rows of res: the accounts of a Trial Balance,
// the sections of Transaction Detail, the outline rows of a Balance Sheet.
func stGroupRows(res *reports.Result) int {
	n := 0
	for _, row := range res.Rows {
		if row.Kind == reports.KindGroup {
			n++
		}
	}
	return n
}

// stPeriodHasLines is true when r has no period (a run "as of" a day) or the
// book has a posted journal line dated within r's period.
func stPeriodHasLines(t *testing.T, b glScaleBook, r stScaleRun) bool {
	t.Helper()
	period, ok := r.params["period"].(map[string]any)
	if !ok {
		return true
	}
	var n int
	if err := b.app.DB().NewQuery("SELECT count(*) FROM books_journal_lines WHERE posted IS true AND date >= {:from} AND date <= {:to}").
		Bind(map[string]any{"from": period["from"], "to": period["to"]}).Row(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// TestStatementsScaleBook: the book has cash-basis rows that add to zero, and
// every run of the measure answers on it. A run "as of" a day and a run whose
// period holds a line show group rows (the small book has no line in its last
// month; the sizes of TestStatementsScale have).
func TestStatementsScaleBook(t *testing.T) {
	b := writeStatementsScaleBook(t, arSmall)
	var cashRows, cashNet int64
	if err := b.app.DB().NewQuery("SELECT count(*), COALESCE(sum(debit - credit), 0) FROM books_cash_basis_lines").Row(&cashRows, &cashNet); err != nil {
		t.Fatal(err)
	}
	if cashRows == 0 || cashNet != 0 {
		t.Fatalf("the book has %d cash-basis rows with a net of %d, want some rows and a net of 0", cashRows, cashNet)
	}
	for _, run := range stScaleRuns(b) {
		res, err := run.exec(t, b)
		if err != nil {
			t.Fatalf("run %s: %v", run.name, err)
		}
		if stPeriodHasLines(t, b, run) && stGroupRows(res) == 0 {
			t.Errorf("run %s has no group row (%d rows)", run.name, len(res.Rows))
		}
	}
}

// TestStatementsScaleBars pins each run's bar, so a change of a bar or of the
// run list is a change of this test (the bar is the human's, 2026-11-20).
func TestStatementsScaleBars(t *testing.T) {
	account := func() *core.Record { return core.NewRecord(core.NewBaseCollection("accounts")) }
	b := glScaleBook{lastDay: "2026-11-20", incomes: []*core.Record{account()}}
	b.receivable = account()
	b.docs = &paymentEnv{ledgerEnv: &ledgerEnv{checking: account()}}
	want := map[string]time.Duration{
		"TB 1 all accounts, as of":       2 * time.Second,
		"TB 2 busy account, as of":       time.Second,
		"TB 3 three accounts, as of":     time.Second,
		"BS 1 accrual, as of":            2 * time.Second,
		"BS 2 cash, as of":               2 * time.Second,
		"TD 1 all accounts, month":       0,
		"TD 2 all accounts, year":        0,
		"TD 3 busy account, month":       time.Second,
		"TD 4 small account, month":      time.Second,
		"TD 5 three accounts, year":      2 * time.Second,
		"TD 6 busy account, month, cash": time.Second,
	}
	runs := stScaleRuns(b)
	if len(runs) != len(want) {
		t.Fatalf("the measure has %d runs, want %d", len(runs), len(want))
	}
	for _, run := range runs {
		bar, ok := want[run.name]
		if !ok {
			t.Errorf("run %q has no bar in this test", run.name)
			continue
		}
		if run.bar != bar {
			t.Errorf("run %q has the bar %s, want %s", run.name, run.bar, bar)
		}
	}
}

// TestStatementsScale measures each run at 10,000, 100,000 and 500,000 journal
// lines (median of 5) and holds the bar at 500,000 lines. A run core refuses
// (its time limit) is recorded as refused; a refused run with a bar fails it.
// With BOOKS_SCALE_OUT=<dir> it writes st-results.md (the times) and
// st-plans.md (each long statement of each run at each size: its time, its
// rows, its reads of the line tables).
func TestStatementsScale(t *testing.T) {
	if os.Getenv("BOOKS_SCALE") != "1" {
		t.Skip("BOOKS_SCALE=1 starts the Trial Balance, Balance Sheet and Transaction Detail runs at 10k, 100k and 500k lines")
	}
	var results, plans strings.Builder
	fmt.Fprintf(&results, "# Trial Balance, Balance Sheet and Transaction Detail scale results\n\n| lines | run | median of 5 | group rows | rows |\n|---|---|---|---|---|\n")
	for _, size := range glScaleSizes() {
		t.Run(size.name, func(t *testing.T) {
			started := time.Now()
			b := writeStatementsScaleBook(t, size)
			t.Logf("%s: wrote %d lines in %s", size.name, b.lines, time.Since(started).Round(time.Second))
			last := size.name == "500k"
			refused := map[string]bool{}
			for _, run := range stScaleRuns(b) {
				began := time.Now()
				res, err := run.exec(t, b)
				took := time.Since(began)
				if err != nil {
					refused[run.name] = true
					fmt.Fprintf(&results, "| %d | %s | refused after %s: %v | n/a | n/a |\n", b.lines, run.name, took.Round(time.Millisecond), err)
					t.Logf("%s run %s: refused after %s: %v", size.name, run.name, took.Round(time.Millisecond), err)
					if last && run.bar > 0 {
						t.Errorf("run %s at %s lines was refused: %v", run.name, size.name, err)
					}
					continue
				}
				median := arMedian(func() { _, _ = run.exec(t, b) })
				fmt.Fprintf(&results, "| %d | %s | %s | %d | %d |\n", b.lines, run.name, median.Round(time.Millisecond), stGroupRows(res), len(res.Rows))
				t.Logf("%s run %s: median %s", size.name, run.name, median.Round(time.Millisecond))
				if run.bar > 0 && stGroupRows(res) == 0 {
					t.Errorf("run %s at %s lines has no group row: its time measures nothing", run.name, size.name)
				}
				if last && run.bar > 0 && median > run.bar {
					t.Errorf("run %s at %s lines: median %s, want %s or less", run.name, size.name, median, run.bar)
				}
			}
			// A refused run's statements would each take the time limit
			// again: its plans are read with no time.
			for _, run := range stScaleRuns(b) {
				statements := stScaleStatements(t, b, run)
				steps := explainStatements(t, b.app, statements)
				fmt.Fprintf(&plans, "\n## %s lines, run %s\n", size.name, run.name)
				for i, s := range statements {
					if len(s) < 1000 {
						continue
					}
					if refused[run.name] {
						fmt.Fprintf(&plans, "\n### statement %d: not timed (the run was refused), %d chars\n\n```\n", i, len(s))
					} else {
						d, n := arProfileTime(t, b.arBook, s)
						fmt.Fprintf(&plans, "\n### statement %d: %s, %d rows, %d chars\n\n```\n", i, d.Round(100*time.Microsecond), n, len(s))
					}
					for _, step := range steps {
						if step.Statement == i && (strings.Contains(step.Detail, "books_journal_lines") || strings.Contains(step.Detail, "books_cash_basis_lines") || strings.Contains(step.Detail, "books_account_balances")) {
							fmt.Fprintf(&plans, "%s\n", step.Detail)
						}
					}
					fmt.Fprintf(&plans, "```\n")
				}
			}
		})
		// After each size, so a timeout in a later size keeps the earlier results.
		writeStatementsScaleOut(t, results.String(), plans.String())
	}
}

// writeStatementsScaleOut writes st-results.md and st-plans.md under
// BOOKS_SCALE_OUT (nothing when it is unset).
func writeStatementsScaleOut(t *testing.T, results, plans string) {
	t.Helper()
	out := os.Getenv("BOOKS_SCALE_OUT")
	if out == "" {
		return
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("make %s: %v", out, err)
	}
	for name, text := range map[string]string{"st-results.md": results, "st-plans.md": plans} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
```

Then format it: `gofmt -w /Users/nas/tmp/tinycld/books-purchases/books/server/report_statements_scale_test.go` (it wraps nothing; it only aligns).

- [ ] **Step 2: Run the book check to see it fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/books/server`:

```bash
go test -run 'TestStatementsScaleBook$' -count=1 .
```

Expected: FAIL with `the book has 0 cash-basis rows with a net of 0, want some rows and a net of 0`.

- [ ] **Step 3: Write the cash-basis rows**

In the same file, replace the whole function `writeStatementsScaleBook` (keep its comment) with:

```go
func writeStatementsScaleBook(t *testing.T, size arSize) glScaleBook {
	t.Helper()
	b := writeGLScaleBook(t, size)
	_, err := b.app.DB().NewQuery(`INSERT INTO books_cash_basis_lines (id, document_kind, entry, account, customer, debit, credit, date, posted, memo, currency, position, created, updated)
SELECT 'c' || substr(lower(hex(randomblob(8))), 1, 14), 'invoice', l.entry, l.account, l.customer, l.credit, l.debit, l.date, true, '', l.currency, l.position, '', ''
FROM books_journal_lines l JOIN books_journal_entries e ON e.id = l.entry
WHERE l.posted IS true AND e.rowid % {:n} = 0`).Bind(map[string]any{"n": stCashEvery}).Execute()
	if err != nil {
		t.Fatalf("write the cash-basis rows: %v", err)
	}
	return b
}
```

- [ ] **Step 4: Run the three tests and the vet**

Run, in `/Users/nas/tmp/tinycld/books-purchases/books/server`:

```bash
gofmt -l report_statements_scale_test.go && go vet . && go test -run 'TestStatementsScale|TestGLScaleBook$|TestGLScaleBars$' -count=1 -v .
```

Expected: `gofmt -l` prints nothing; vet prints nothing; `--- PASS: TestStatementsScaleBook`, `--- PASS: TestStatementsScaleBars`, `--- SKIP: TestStatementsScale` (the `BOOKS_SCALE=1` message), `--- PASS: TestGLScaleBook`, `--- PASS: TestGLScaleBars`, and `ok  	tinycld.org/packages/books`.

- [ ] **Step 5: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_statements_scale_test.go
git commit -m "test: scale measure of the Trial Balance, Balance Sheet and Transaction Detail"
```

- [ ] **Step 6: Hand the 100k baseline run to the driver**

Do not start it. Hand back:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9a go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 30m -v . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9a.log 2>&1
```

What is needed from it: the exit code, and the files `scale-task9a/st-results.md` and `scale-task9a/st-plans.md`. The run writes the 100k book, then each run once and five times more; a refused run takes 10 s and is run twice (once for its time, once for its statements). Expected: exit 0 (no time bar is held at 100k), eleven result lines, a "refused" line for each Transaction Detail run core stops. An exit 1 with `has no group row: its time measures nothing` is a `STOPPED` hand-back with that line (the run's period holds no line of its accounts, so the run list must change before any record); steps 7 and 8 do not run then.

- [ ] **Step 7: Record the baseline in the ledger**

With the files of step 6, append to the end of `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (the end of section "Performance (p5-report-engine-scale)"):

```markdown
### Trial Balance, Balance Sheet and Transaction Detail: the measure and its baseline

The measure is `TestStatementsScale` (`server/report_statements_scale_test.go`): the General Ledger's scale book with cash-basis rows for one entry in four, and eleven runs. The bar at 500,000 lines (the human, 2026-11-20) maps so: Transaction Detail's runs with an Accounts filter take 1 s or less for a month and 2 s or less for a year (the General Ledger's runs; the two runs with no Accounts filter have no bar); a Trial Balance with an Accounts filter takes 1 s or less; the Trial Balance of every account and the Balance Sheet (no Accounts filter, two days) take 2 s or less, on Accrual and on Cash. `TestStatementsScaleBars` pins the runs and the bars.

Baseline at 100,000 lines (median of 5, core <core commit>, books <books commit>, before any change for these reports):

<the table of scale-task9a/st-results.md: its two table header lines and its eleven rows, as they are; not the `#` title line>

Before this unit (`other-reports-scale.txt`, 2026-11-19): Trial Balance 960 ms and Balance Sheet 1,280 ms at 100k (5,225 ms and 6,496 ms at 500k); Transaction Detail refused at 100k, also with one account.

Reads of the line tables in the longest statement of each report (`scale-task9a/st-plans.md`):

<for run "TB 1 all accounts, as of", run "BS 1 accrual, as of" and run "TD 3 busy account, month": the heading line of its longest statement ("### statement N: …") and the plan lines under it, as they are>
```

Fill the three `<…>` places from the files and from `git rev-parse --short HEAD` in `tinycld/` and in `books/`; copy lines, do not round or reword them. The longest statement of a run is the one with the largest time in its heading. A refused run has no timed statement: take the one with the largest `chars`.

- [ ] **Step 8: Commit the record**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs: baseline of the Trial Balance, Balance Sheet and Transaction Detail at 100k"
```

---

## What is left (the next plan part)

Part 13 is `2026-11-20-report-engine-scale-part13.md`, written from the outline (`plan-outline.md`, section "State after plan part 12") and from this part's Interfaces block. Its planner reads `scale-task9a/st-results.md` and `st-plans.md` first and plans the change with the largest measured gain. The candidate changes (not measured; the outline gives the detail): 9b, core's filter bounds reach each part of a union source (Transaction Detail and the Balance Sheet read `books.basis_lines`, which the bounds of task 2 skip); 9c, the Trial Balance and the Balance Sheet read the stored months (`books_account_balances.month_start`) for the months before the as-of month, as the General Ledger's beginning balance does; 9d, a stored month table for the cash-basis rows, only if the Cash runs stay over the bar after 9b and 9c. Each change task ends with a 100k `NEEDS-RUN` of `TestStatementsScale` and a ledger step; the last task runs 500k and holds the bars.
