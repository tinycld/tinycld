# Report Engine Query Scaling Implementation Plan — part 1 (task 1, the baseline and the stored-summary decision)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A General Ledger run with an Accounts filter takes 1 s or less on a book with 500,000 journal lines, with every report's result unchanged.

**Architecture:** This part adds the measure (a scale book and five General Ledger runs at 10,000, 100,000 and 500,000 journal lines), records the baseline, and records the decision on a stored summary. Part 2 makes the changes the baseline points at: the filter bounds the base table through its index (core), the beginning balance reads the stored monthly balances (Books, with one new core formula operator), and a sum lookup is read as one grouped aggregate (core). Each engine change keeps a `legacySQL` switch and a cross-check on the fictional zoo.

**Tech Stack:** Go, SQLite (PocketBase fork, dbx, modernc.org/sqlite; SQLite 3.54), core report engine `tinycld.org/core/reports`, Books server tests (`tinycld.org/packages/books`).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-perf/from-p2-write-off-report-sql.md` (the root cause that made this unit; section "Why report runs, and not other steps, slowed down so much" and "App (contributing…)"). The unit's five requirements are in "Unit requirements" below. Outline of all 7 tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`. The General Ledger's design: `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/general-ledger-design.md`.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`); Books is `books/` (same branch name). Task 1 is a Books task.

## Unit requirements

1. Measure filtered and unfiltered General Ledger runs at 10k, 100k and 500k journal lines; a filtered run is ≤ 1 s at 500k lines. → task 1 (measure, baseline), task 7 (the run after the changes).
2. Before any query change, decide with evidence whether the General Ledger and other growing reports read a stored summary; record the decision and its timings in the plan. → section "Decision" below; task 4 builds it.
3. Core `server/reports` pushes account and date filters into the base read, so a filtered run reads only its rows. → task 2 (and task 6 for a lookup's where).
4. Per-account correlated sum/count subqueries replaced by one grouped aggregate joined back, each change checked with EXPLAIN and timings. → task 5.
5. Live re-run topics narrowed. → NOT in this plan: it needs a change-signal redesign and is its own unit (section "Requirement 5" below).

## Global Constraints

- The bar is never lowered: runs 3, 4 and 5 of `TestGLScale` (the runs with an Accounts filter) take 1 s or less at 500,000 lines, median of 5. No task changes a bar, removes a run, or changes the scale book to pass.
- Every report's rows stay the same. An engine change is proven by a cross-check (new SQL against the `legacySQL` switch) on the zoo, and one test proves the switch writes the old SQL.
- Core names no package: core tests use the fictional zoo (`zoo.meals`, `zoo.lines`) only; no Books slug, collection or route in `tinycld/core`.
- Raw SQL test writes skip PocketBase validation: bound every value (ids are exactly 15 characters).
- Server logging goes through `logging.ForPackage`; no `"books: "` prefixes in new log lines.
- Each change to SQL is recorded with its EXPLAIN plan and its time before and after in `books/docs/plans/general-ledger-ledger.md`, section "Performance (p5-report-engine-scale)".
- The 500k run takes about 13 minutes: an implementer never starts it. It is a `NEEDS-RUN` for the driver.

## Review Focus

1. A filter value that names a hidden or blank account: a bound on the stored column must not drop or add a row (the filter itself still decides). Owner: task 2 (part 2), test on the zoo with a hidden linked row and a blank link.
2. A day stored with a time after it (a PocketBase date column, `2026-11-18 10:00:00.000Z`) on the last day of a period: the upper bound keeps it. Owner: task 2 (part 2), test with such a row on the range's last day.
3. A period that starts in the middle of a month, in a fiscal year that starts in the middle of a calendar year: the beginning balance from stored months plus month-to-date lines equals the one from the lines. Owner: task 4 (part 2).
4. A void, a reversal and a refused post: the stored months still equal the lines (`verifyBalances` finds no drift), so the beginning balance does not go stale. Owner: task 4 (part 2).
5. A scale book that posting could not have written (balance rows missing or wrong) measures the wrong thing and hides a wrong beginning balance. Owner: task 1, `TestGLScaleBook` (stored balances agree with the lines; a filtered run shows only the picked accounts).

---

## Baseline (planner's run, 2026-11-19, core 4690f72e, books 381fe701, task 1's harness, 107 accounts)

Command: `BOOKS_SCALE=1 BOOKS_SCALE_OUT=<dir> go test -run 'TestGLScale$' -timeout 60m -v .` in `books/server` (758 s). Output kept in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-baseline.log` and `scale-baseline/gl-results.md`, `gl-plans.md`.

| run | 10,000 lines | 100,000 lines | 500,000 lines |
|---|---|---|---|
| 1 all accounts, month | 467 ms | 4,328 ms | refused after 19.3 s (time limit) |
| 2 all accounts, year | 572 ms | 5,208 ms | refused after 19.6 s |
| 3 busy account, month (filtered) | 326 ms | 2,527 ms | 13,772 ms |
| 4 small account, month (filtered) | 142 ms | 1,434 ms | 7,219 ms |
| 5 three accounts, year (filtered) | 401 ms | 3,472 ms | refused after 17.2 s |

Every run grows in line with the journal lines. No filtered run meets the 1 s bar at 500k; the nearest is 7 times over.

Where the time goes (run 3 at 500k, `gl-plans.md`): statement 4 (the section rows) is 2.4 s, statement 5 (the Total row) is 3.9 s and statement 6 (the page of detail rows) is 7.2 s, the longest of each run (3.1 s for the 9 rows of run 4, 10.9 s in run 5). Statements 4 and 5 read `books_journal_lines` with `SCAN` 3 and 9 times (the rows `w`, the Total row's lookups, the earlier-years P&L lookups) and with `SEARCH … idx_books_jlines_account_date (account>? AND account<?)` 3 and 4 times (the beginning balance of each section: three subqueries for one sum). Statement 6 reads it with `SCAN` 12 times and with that `SEARCH` 6 times. Attributed with the 10k profile (`profile-gl-10k.txt`, the same statement):

| read in statement 6 | plan | removed by |
|---|---|---|
| the rows `w`, read again in `wg3`, `a1`, `f1` and the parts of the page's row set (`kx`, `p`, `e`) | `SCAN books_journal_lines`, most of the 12 (7 in the 10k profile) | task 2 (the bound makes each a `SEARCH` by account and day) |
| the earlier-years P&L lookups of `lk1` | `SCAN`, 2 | task 4 (they read stored months) |
| the beginning balance of each section in `lk1` | 3 × `SEARCH (account>? AND account<?)` | task 4 (stored months), task 5 (one read for the three values) |
| the beginning balance read again for each detail row (the running balance's start) | 3 × `SEARCH (account>? AND account<?)`, once for each row of the page | task 4, task 5 (detail rows keyed by group keys) |
| the Split of each line | `SEARCH … idx_books_jlines_entry`, bounded by the page | none needed |

At 10k (the profile, 7 accounts) the beginning-balance lookups (`lk1`) are 64 to 79 ms of each long statement, the Total row (`o0`) 17 ms, the rows (`w`) 2.7 ms for each read.

## Decision: a stored summary (requirement 2)

**How p5-report-perf met its bar.** The A/R Aging read two views over the ledger. Its design study (`staging/p5-report-perf/stored-receivables-design.md`) measured that no engine change could meet 1 s at 500k while a run read every A/R movement, and stored `books_receivable_items`: one row per movement with the day its item is settled, refreshed per entry inside the write transaction, with verify and rebuild. A run then reads only the open items. Result at 500k: 55 to 510 ms for runs 1 to 6 (`perf-scale-stored-2.log`), from over 5 s.

**The same question for the General Ledger**, measured with direct SQL on the 500k book (`direct-sql-500k.txt`, 7 accounts, best of 3; the 10k figures are in `direct-sql-10k.txt`):

| read | 10k | 500k | plan |
|---|---|---|---|
| rows of one account and one month, as the engine writes the filter today | 2.0 ms | 81 ms | no search of `books_journal_lines` (the planner starts at the account and reads every line) |
| the same rows, with `account IN (…)` and the day range on the stored columns | 0.03 ms | 0.6 ms | `SEARCH idx_books_jlines_account_date (account=? AND date>? AND date<?)` |
| beginning balances, as today: 3 correlated subqueries per account | 13.4 ms | 1,056 ms | 3 × `SEARCH (account=? AND date<?)` |
| beginning balances, one grouped aggregate over the lines | 5.5 ms | 414 ms | `SCAN … USING INDEX idx_books_jlines_account_date` |
| the same, one account's keys pushed in | 1.2 ms | 84 ms | `SEARCH (account=? AND date<?)` |
| beginning balances from `books_account_balances` | 0.09 ms | 0.08 ms | `SCAN books_account_balances USING INDEX idx_books_balances_account_period` |
| earlier-years P&L sum from the lines, as today | 2.0 ms | 131 ms | `SEARCH l (account=?)` for each P&L account |
| earlier-years P&L sum from `books_account_balances` | 0.04 ms | 0.04 ms | `SEARCH g (account=? AND period<?)` |

**Decision.**

1. **Yes: the General Ledger's beginning balance reads a stored summary, and that summary exists.** `books_account_balances` holds one row per account and month (`debit_total`, `credit_total`), written by posting in the write transaction (`applyToBalances`, `books/server/balances.go:19`), with `verifyBalances`, `rebuildBalances` and the admin rebuild route. No new table and no new refresh hook. A beginning balance read from the lines costs time in line with the account's whole history: the best query form (one grouped aggregate, keys pushed) is still 84 ms for one busy account at 500k, in each of a run's three statements, and grows with every year of the book. From the stored months it is under 0.1 ms on this book (305 balance rows, 7 accounts) and does not grow with the journal lines: it grows with accounts × months. The beginning balance becomes: the stored months before the period's first month, plus the lines from the first day of that month to the day before the period (at most one month of one account, read through `(account, date)`). Built by task 3 (the `startOfMonth` operator) and task 4.
2. **The query changes are still needed, for the lines of the period itself.** The filter bound (task 2) takes the rows read from 81 ms to 0.6 ms per read. The grouped aggregate (task 5) and the day bounds in a lookup's where (task 6) serve the month-to-date lines and every other report's sum lookups. A stored summary cannot serve the period's own lines: the report lists them.

   **What is not proven.** The direct SQL explains about 1.3 s on a book of 7 accounts; run 3 on 107 accounts takes 13.8 s, in three statements that each read the rows and the lookups many times. The table of statement 6 above names a task for each read of `books_journal_lines`, but no measure yet shows that tasks 2 to 6 together reach 1 s. The plan therefore measures after each change, not only at the end: tasks 2, 4, 5 and 6 each end with the 100k run (`-run 'TestGLScale$/100k$'`, under 2 minutes) and record its times and plans, and task 7 is the 500k run. When a filtered run is over 1 s after task 6, task 7 records the profile of the longest statement and hands back `STOPPED`; no task changes the bar.
3. **Other growing reports.** The Trial Balance reads `books.journal_lines` with per-account sums of the same kind; it gains from tasks 2, 5 and 6 with no change of its own, and it can read `books_account_balances` the way task 4 does. The Balance Sheet, the Profit and Loss and the Transaction Detail read `books.basis_lines` (cash or accrual); the stored months hold the accrual basis only, so a stored summary for them needs a cash-basis month table. Measured on the same scale books (one run each, best of 2; `staging/p5-report-engine-scale/other-reports-scale.txt`, probe `other-reports-probe_test.go.txt`):

   | report | 10k | 100k | 500k |
   |---|---|---|---|
   | Trial Balance, as of the last day | 105 ms | 960 ms | 5,225 ms |
   | Balance Sheet, as of the last day | 166 ms | 1,280 ms | 6,496 ms |
   | Profit and Loss, month | 19 ms | 80 ms | 354 ms |
   | Profit and Loss, year | 23 ms | 103 ms | 483 ms |
   | Transaction Detail, month | 2,082 ms | refused (10 s limit) | refused |
   | Transaction Detail, month, one account | 809 ms | refused (10 s limit) | refused |

   Decisions:
   - **Trial Balance: yes, a stored summary.** It is a balance as of a day for every account, the same read as the General Ledger's beginning balance (414 ms from the lines against 0.08 ms from stored months, table above), and it grows in line with the book. It can read `books_account_balances` with the pieces task 3 and task 4 make (`startOfMonth`, `month_start`). This unit's bar is the General Ledger; the Trial Balance change is not in its five requirements and is not planned here.
   - **Balance Sheet: yes for the accrual basis, and a cash-basis month table is needed for the cash basis.** Same growth, same kind of read. The stored months hold the accrual basis only.
   - **Profit and Loss: no.** 483 ms for a year at 500k: under 1 s with no change.
   - **Transaction Detail: no stored summary; its fault is the query.** It lists lines, as the General Ledger's period does, so a summary cannot serve it. It takes 2 s at 10k and is refused at 100k, also with one account. That is a defect at every size, not only a scaling one. Tasks 2, 5 and 6 change the engine for every report, but this plan has not measured what they do for this report and gives it no bar.

   The Trial Balance and Balance Sheet change, the cash-basis month table and the Transaction Detail defect are reported to the driver in the hand-back: the human decides whether each gets a unit.

## Requirement 5 (live re-run topics): its own unit

The unit's own text allows this split when the work needs a redesign of the change signal ("split into its own unit if it needs a pbtsdb/change-signal redesign"). It does, and the redesign holds a decision only the human can make: what a signed-in client may learn from a topic. This plan does not hold it; the hand-back says so. Evidence:
- Any signed-in client may subscribe to any topic, and the empty message is by design: "Any signed-in client may subscribe to any topic" (`collectionOf`, `changesignal.go:70`); "The payload is empty, but the timing of writes is still information" (`isSignedIn`, `:133`). A message or a topic key that names the account or the day of a post tells every signed-in user, also one with no Books access, which account moved and when. A narrower signal thus needs an access rule for topics, which the signal does not have.
- A topic is one collection: `changed(collection string)` schedules `topicsFor(collection)` (`core/server/changesignal/changesignal.go:97`); `Touch(app, collection)` takes a collection name only (`:146`).
- The message carries nothing: `const payload = "{}"` (`changesignal.go:27`).
- A run's topics are the collections its rule CTEs read (`(s *stmt) topics`, `core/server/reports/stmt.go:221`).
- The client re-runs on any message of a topic (`core/reports/hooks/useLiveTopics.ts`).

To re-run only the reports whose filter can include a post, the signal must say what changed (account, day), or topics must be split by a key, and a run must state its filter as topic keys. That changes the signal's payload or topic scheme, the engine's topic list and the client's subscription.

## Tasks of the whole plan

1. books · Scale harness and baseline (this part).
2. core · The filter bounds the base table (requirement 3).
3. core · The `startOfMonth` formula operator.
4. books · The beginning balance reads stored months (requirement 2).
5. core · A sum lookup read as one grouped aggregate (requirement 4).
6. core · Day bounds in a lookup's where.
7. books · Scale run and record (requirement 1's bar).

Tasks 2 to 7 are in part 2 (`2026-11-19-report-engine-scale-part2.md`); the outline holds their files, interfaces and order.

---

### Task 1: Scale harness and baseline (Books)

The measure every later task reads. A scale book is the A/R Aging bulk writer's book (`report_ar_aging_scale_test.go`) with its income lines spread over 100 income accounts (107 accounts in all, so the report has about as many sections as a real chart) and its monthly balances stored, because the bulk writer writes no `books_account_balances` rows. `TestGLScaleBook` runs in every test run on the 500-line book. `TestGLScale` starts only with `BOOKS_SCALE=1`.

**Files:**
- Create: `books/server/report_general_ledger_scale_test.go`
- Modify: `books/docs/plans/general-ledger-ledger.md` (append after the last line, the `## Final review` block)

**Interfaces:**
- Consumes (existing, `books/server/*_test.go`): `newARBook(t *testing.T) arBook`, `arJournalPrefix(t *testing.T, b arBook)`, `writeARDataset(t testing.TB, b arBook, size arSize) arDataset`, `arSmall`, `arS`, `arL`, `arMedian(fn func()) time.Duration`, `arProfileTime(t *testing.T, b arBook, sql string) (time.Duration, int)`, `explainStatements(t testing.TB, app core.App, statements []string) []planStep`, `recordStatements(t testing.TB, app *tests.TestApp, seen func(string), fn func())`, `runGLErr(t *testing.T, app core.App, reader *core.Record, params map[string]any, opts reports.Options) (*reports.Result, error)`, `glPeriod(from, to string) map[string]any`, `mkNumberedAccount(t testing.TB, app core.App, name, number, accountType, detailType string) *core.Record`, `verifyBalances(app core.App) ([]BalanceDrift, error)`.
- Produces (for tasks 4 and 7): const `glIncomeAccounts = 100`; `glScaleSize(name string, lines int) arSize`; `glScaleSizes() []arSize` (names `10k`, `100k`, `500k`); `type glScaleBook struct { arBook; lastDay string; incomes []*core.Record; lines int }`; `writeGLScaleBook(t *testing.T, size arSize) glScaleBook`; `type glScaleRun struct { name string; params map[string]any; filtered bool }`; `glScaleRuns(b glScaleBook) []glScaleRun` (five runs; 3, 4, 5 are `filtered`); `(r glScaleRun) exec(t *testing.T, b glScaleBook) (*reports.Result, error)`; `glScaleStatements(t *testing.T, b glScaleBook, r glScaleRun) []string`; `glSectionAccounts(res *reports.Result) []string`; tests `TestGLScaleBook`, `TestGLScale`; output files `gl-results.md`, `gl-plans.md` under `BOOKS_SCALE_OUT`.

- [ ] **Step 1: Write the harness with the failing book test**

Create `books/server/report_general_ledger_scale_test.go` with this content. In this step the transaction in `writeGLScaleBook` ends with `return nil` (marked): the balances are not stored yet, so `TestGLScaleBook` fails on its first check.

```go
package books

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/reports"
)

// The scale measure of the General Ledger (p5-report-engine-scale): the A/R
// Aging bulk writer's book (report_ar_aging_scale_test.go) with its income
// spread over glIncomeAccounts accounts and its monthly balances stored, and
// the runs of the report with and without an Accounts filter. The book check
// runs in every test run (TestGLScaleBook); the three sizes start only with
// BOOKS_SCALE=1 (TestGLScale).

// glIncomeAccounts is the number of income accounts the book's income lines
// are spread over, so that the report has about as many sections as a real
// chart of accounts.
const glIncomeAccounts = 100

// glScaleSize is the A/R Aging S book scaled to lines journal lines.
func glScaleSize(name string, lines int) arSize {
	f := func(n, least int) int { return max(n*lines/arS.lines, least) }
	return arSize{
		name: name, lines: lines, customers: f(arS.customers, 30), subCustomers: f(arS.subCustomers, 3),
		voidInvoices: f(arS.voidInvoices, 2), closedInvoices: f(arS.closedInvoices, 80), openInvoices: f(arS.openInvoices, 15),
		partPayments: f(arS.partPayments, 3), pairPayments: f(arS.pairPayments, 8), creditPayments: f(arS.creditPayments, 3),
		voidPayments: f(arS.voidPayments, 3), voidPaymentApps: f(arS.voidPaymentApps, 2),
		memos: f(arS.memos, 4), manual: f(arS.manual, 10),
	}
}

// glScaleSizes are the three sizes of the unit: 10,000, 100,000 and 500,000
// journal lines (the last is the A/R Aging L book).
func glScaleSizes() []arSize {
	l := arL
	l.name = "500k"
	return []arSize{glScaleSize("10k", 10000), glScaleSize("100k", 100000), l}
}

// glScaleBook is a written book: its accounts and the day of its runs.
type glScaleBook struct {
	arBook
	lastDay string
	// incomes are the glIncomeAccounts accounts the income lines moved to.
	incomes []*core.Record
	lines   int
}

// writeGLScaleBook writes size's book, moves each income line to one of
// glIncomeAccounts new income accounts (by its row number, so every month of
// every account has lines), and stores the monthly balances as posting does.
// The bulk writer writes no balance rows.
func writeGLScaleBook(t *testing.T, size arSize) glScaleBook {
	t.Helper()
	b := newARBook(t)
	arJournalPrefix(t, b)
	data := writeARDataset(t, b, size)
	book := glScaleBook{arBook: b, lastDay: data.lastDay}
	for i := range glIncomeAccounts {
		book.incomes = append(book.incomes, mkNumberedAccount(t, b.app, fmt.Sprintf("Income %03d", i), fmt.Sprintf("41%02d", i), "income", "service_income"))
	}
	err := b.app.RunInTransaction(func(tx core.App) error {
		for i, a := range book.incomes {
			if _, err := tx.DB().NewQuery("UPDATE books_journal_lines SET account = {:to} WHERE account = {:from} AND rowid % {:n} = {:i}").
				Bind(map[string]any{"to": a.Id, "from": b.income.Id, "n": glIncomeAccounts + 1, "i": i}).Execute(); err != nil {
				return err
			}
		}
		return nil // step 3 stores the monthly balances here
	})
	if err != nil {
		t.Fatalf("spread the income and store the balances: %v", err)
	}
	if err := b.app.DB().NewQuery("SELECT count(*) FROM books_journal_lines").Row(&book.lines); err != nil {
		t.Fatal(err)
	}
	return book
}

// glScaleRun is one run of the report.
type glScaleRun struct {
	name   string
	params map[string]any
	// filtered is a run with an Accounts filter: the unit's 1 s bar is its.
	filtered bool
}

// glScaleRuns are the runs of the measure: the default period (this month to
// date) and the year to date, with no Accounts filter, with one busy account
// (Accounts receivable: a line of every invoice, payment and credit memo),
// with one small account (one of the income accounts) and with three accounts.
func glScaleRuns(b glScaleBook) []glScaleRun {
	month := glPeriod(b.lastDay[:8]+"01", b.lastDay)
	year := glPeriod(b.lastDay[:5]+"01-01", b.lastDay)
	three := []string{b.receivable.Id, b.incomes[0].Id, b.docs.checking.Id}
	return []glScaleRun{
		{name: "1 all accounts, month", params: map[string]any{"period": month}},
		{name: "2 all accounts, year", params: map[string]any{"period": year}},
		{name: "3 busy account, month", params: map[string]any{"period": month, "accounts": []string{b.receivable.Id}}, filtered: true},
		// Rows: All, so the section shows also in a month with no line of it.
		{name: "4 small account, month", params: map[string]any{"period": month, "rows": "all", "accounts": []string{b.incomes[0].Id}}, filtered: true},
		{name: "5 three accounts, year", params: map[string]any{"period": year, "accounts": three}, filtered: true},
	}
}

func (r glScaleRun) exec(t *testing.T, b glScaleBook) (*reports.Result, error) {
	t.Helper()
	return runGLErr(t, b.app, b.reader, r.params, reports.Options{Today: b.lastDay})
}

// glScaleStatements are the distinct SELECTs of r, in order.
func glScaleStatements(t *testing.T, b glScaleBook, r glScaleRun) []string {
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

// glSectionAccounts are the account ids of the sections of res, in order.
func glSectionAccounts(res *reports.Result) []string {
	var out []string
	for _, row := range res.Rows {
		if row.Kind != reports.KindGroup || len(row.GroupKey) == 0 {
			continue
		}
		id, _ := row.GroupKey[0].(string)
		out = append(out, id)
	}
	return out
}

// TestGLScaleBook: the scale book is a ledger posting could have written (its
// stored monthly balances agree with its lines, the income is spread), and a
// run with an Accounts filter shows the picked accounts' sections only.
func TestGLScaleBook(t *testing.T) {
	b := writeGLScaleBook(t, arSmall)
	drifts, err := verifyBalances(b.app)
	if err != nil {
		t.Fatalf("verifyBalances: %v", err)
	}
	if len(drifts) > 0 {
		t.Fatalf("the stored balances differ from the lines in %d rows, first %+v", len(drifts), drifts[0])
	}
	var moved int
	if err := b.app.DB().NewQuery("SELECT count(DISTINCT account) FROM books_journal_lines WHERE account IN (SELECT id FROM books_accounts WHERE name LIKE 'Income ___')").Row(&moved); err != nil {
		t.Fatal(err)
	}
	if moved < glIncomeAccounts/2 {
		t.Fatalf("income lines are on %d of the %d income accounts, want half or more", moved, glIncomeAccounts)
	}
	for _, run := range glScaleRuns(b) {
		res, err := run.exec(t, b)
		if err != nil {
			t.Fatalf("run %s: %v", run.name, err)
		}
		got := glSectionAccounts(res)
		if len(got) == 0 {
			t.Fatalf("run %s has no section", run.name)
		}
		if !run.filtered {
			continue
		}
		picked, _ := run.params["accounts"].([]string)
		for _, id := range got {
			if !slices.Contains(picked, id) {
				t.Fatalf("run %s shows a section of account %s, which it did not pick (%v)", run.name, id, picked)
			}
		}
	}
}

// TestGLScale measures each run at 10,000, 100,000 and 500,000 journal lines
// (median of 5) and holds the unit's bar: a run with an Accounts filter takes
// 1 s or less at 500,000 lines. A run core refuses (its time limit) is
// recorded as refused; a refused filtered run fails the bar. With
// BOOKS_SCALE_OUT=<dir> it writes gl-results.md (the times) and gl-plans.md
// (each statement of each run at 500,000 lines: its time, its rows, its plan).
func TestGLScale(t *testing.T) {
	if os.Getenv("BOOKS_SCALE") != "1" {
		t.Skip("BOOKS_SCALE=1 starts the General Ledger runs at 10k, 100k and 500k lines")
	}
	const bar = time.Second
	var results, plans strings.Builder
	fmt.Fprintf(&results, "# General Ledger scale results\n\n| lines | run | median of 5 | sections on the page |\n|---|---|---|---|\n")
	for _, size := range glScaleSizes() {
		t.Run(size.name, func(t *testing.T) {
			started := time.Now()
			b := writeGLScaleBook(t, size)
			t.Logf("%s: wrote %d lines in %s", size.name, b.lines, time.Since(started).Round(time.Second))
			last := size.name == "500k"
			for _, run := range glScaleRuns(b) {
				began := time.Now()
				res, err := run.exec(t, b)
				took := time.Since(began)
				if err != nil {
					fmt.Fprintf(&results, "| %d | %s | refused after %s: %v | n/a |\n", b.lines, run.name, took.Round(time.Millisecond), err)
					t.Logf("%s run %s: refused after %s: %v", size.name, run.name, took.Round(time.Millisecond), err)
					if last && run.filtered {
						t.Errorf("run %s at %s lines was refused: %v", run.name, size.name, err)
					}
					continue
				}
				median := arMedian(func() { _, _ = run.exec(t, b) })
				fmt.Fprintf(&results, "| %d | %s | %s | %d |\n", b.lines, run.name, median.Round(time.Millisecond), len(glSectionAccounts(res)))
				t.Logf("%s run %s: median %s", size.name, run.name, median.Round(time.Millisecond))
				if last && run.filtered && median > bar {
					t.Errorf("run %s at %s lines: median %s, want %s or less", run.name, size.name, median, bar)
				}
			}
			if !last {
				return
			}
			for _, run := range glScaleRuns(b) {
				statements := glScaleStatements(t, b, run)
				steps := explainStatements(t, b.app, statements)
				fmt.Fprintf(&plans, "\n## Run %s\n", run.name)
				for i, s := range statements {
					if len(s) < 1000 {
						continue
					}
					d, n := arProfileTime(t, b.arBook, s)
					fmt.Fprintf(&plans, "\n### statement %d: %s, %d rows, %d chars\n\n```\n", i, d.Round(100*time.Microsecond), n, len(s))
					for _, step := range steps {
						if step.Statement == i && strings.Contains(step.Detail, "books_journal_lines") {
							fmt.Fprintf(&plans, "%s\n", step.Detail)
						}
					}
					fmt.Fprintf(&plans, "```\n")
				}
			}
		})
	}
	out := os.Getenv("BOOKS_SCALE_OUT")
	if out == "" {
		return
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("make %s: %v", out, err)
	}
	for name, text := range map[string]string{"gl-results.md": results.String(), "gl-plans.md": plans.String()} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
```

- [ ] **Step 2: Run the book test to verify it fails**

Run, in `/Users/nas/tmp/tinycld/books-purchases/books/server`: `go test -run 'TestGLScaleBook$' -count=1 .`
Expected: FAIL with `the stored balances differ from the lines in 282 rows, first {Account:… Period:… StoredDebit:0 StoredCredit:0 …}` (the row count may differ by a few; the stored side is 0).

- [ ] **Step 3: Store the monthly balances**

In `writeGLScaleBook`, replace the line `return nil // step 3 stores the monthly balances here` with:

```go
		_, err := tx.DB().NewQuery(`INSERT INTO books_account_balances (id, account, period, debit_total, credit_total, currency, created, updated)
SELECT 'b' || substr(lower(hex(randomblob(8))), 1, 14), account, substr(date, 1, 7), sum(debit), sum(credit), min(currency), '', ''
FROM books_journal_lines WHERE posted IS true GROUP BY account, substr(date, 1, 7)`).Execute()
		return err
```

(The id is 15 characters, as a PocketBase id is. Do not call `rebuildBalances` here: on a bulk-written 100k book it runs for over 4 minutes, because it also rebuilds each document's balance.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go vet . && go test -run 'TestGLScaleBook$|TestGLScale$' -count=1 -v .`
Expected: `--- PASS: TestGLScaleBook`, `--- SKIP: TestGLScale` (`BOOKS_SCALE=1 starts the General Ledger runs…`), `ok`.

Then the neighbours that share the helpers: `go test -run 'GeneralLedger|ARAging' -count=1 .`
Expected: `ok`.

Do NOT start `BOOKS_SCALE=1`: the run takes about 13 minutes. The baseline below is the planner's run of this same file on this tree.

- [ ] **Step 5: Record the baseline in the ledger**

Append to `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md`, after its last line:

```markdown

## Performance (p5-report-engine-scale)

Measure: `BOOKS_SCALE=1 BOOKS_SCALE_OUT=<dir> go test -run 'TestGLScale$' -timeout 60m -v .` in `books/server` (`report_general_ledger_scale_test.go`). The book is the A/R Aging bulk writer's book with its income spread over 100 income accounts (107 accounts) and its monthly balances stored. Bar: a run with an Accounts filter (runs 3, 4, 5) takes 1 s or less at 500,000 journal lines, median of 5.

### Baseline (2026-11-19, core 4690f72e, books 381fe701)

| run | 10,000 lines | 100,000 lines | 500,000 lines |
|---|---|---|---|
| 1 all accounts, month | 467 ms | 4,328 ms | refused after 19.3 s (time limit) |
| 2 all accounts, year | 572 ms | 5,208 ms | refused after 19.6 s |
| 3 busy account, month (filtered) | 326 ms | 2,527 ms | 13,772 ms |
| 4 small account, month (filtered) | 142 ms | 1,434 ms | 7,219 ms |
| 5 three accounts, year (filtered) | 401 ms | 3,472 ms | refused after 17.2 s |

Every run grows in line with the journal lines. Run 3 at 500,000 lines: statement 4 (the section rows) takes 2.4 s, statement 5 (the Total row) 3.9 s and statement 6 (the page of detail rows) 7.2 s. They read `books_journal_lines` by `SCAN` 3, 9 and 12 times (the report's rows, read again by each part of the statement; the earlier-years P&L lookups) and by `SEARCH … idx_books_jlines_account_date (account>? AND account<?)` 3, 4 and 6 times (the beginning balance: three subqueries for one sum, read again for each detail row of the page).

### Decision: the beginning balance reads the stored monthly balances

Direct SQL on the 500,000-line book (7 accounts, best of 3):

| read | time | plan |
|---|---|---|
| rows of one account and one month, as the engine writes the filter | 81 ms | no search of `books_journal_lines` |
| the same rows, bounds on the stored `account` and `date` | 0.6 ms | `SEARCH idx_books_jlines_account_date (account=? AND date>? AND date<?)` |
| beginning balances, 3 correlated subqueries per account (today) | 1,056 ms | 3 × `SEARCH (account=? AND date<?)` |
| beginning balances, one grouped aggregate over the lines | 414 ms | `SCAN … USING INDEX idx_books_jlines_account_date` |
| the same, one account's keys pushed in | 84 ms | `SEARCH (account=? AND date<?)` |
| beginning balances from `books_account_balances` | 0.08 ms | `SCAN books_account_balances USING INDEX idx_books_balances_account_period` |
| earlier-years P&L sum from the lines (today) | 131 ms | `SEARCH l (account=?)` per P&L account |
| earlier-years P&L sum from `books_account_balances` | 0.04 ms | `SEARCH g (account=? AND period<?)` |

A beginning balance read from the lines costs time in line with the account's whole history, whatever the query form. `books_account_balances` (one row per account and month, written by posting, with verify and rebuild) answers it at any size. The beginning balance becomes the stored months before the period's first month plus the lines of that month before the period. The filter bounds and the grouped lookup are still needed for the period's own lines. Plan: `tinycld/docs/plans/2026-11-19-report-engine-scale.md`.

### Changes

(Each later task of the plan adds its change here: the SQL before and after, its plan, its time.)
```

- [ ] **Step 6: Commit (Books repo)**

Only when your prompt tells you to commit. When it does not, name the two files in your hand-back; the driver commits.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_general_ledger_scale_test.go docs/plans/general-ledger-ledger.md
git commit -m "test(books): General Ledger scale measure and its baseline"
```

---

## What is left (part 2)

Tasks 2 to 7, from the outline (`plan-outline.md`, sections "Tasks", "Order and dependencies" and "Facts for the planner of part 2"). Part 2's planner compiles each task's code with `go test -overlay` on scratch copies, as this part did for task 1 (red and green both run).
