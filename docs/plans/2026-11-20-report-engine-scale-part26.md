# Report Engine Scale, Part 26: The Cash Balance Sheet Reads the Stored Cash Months (task 9d-2b) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On Cash, the Balance Sheet reads the cash-basis rows of the months before a day's month from the stored cash months (`books_cash_basis_balances`), not from `books_cash_basis_lines`, and its figures do not change.

**Architecture:** A new report source `books.cash_basis_balances` sits over the table task 9d-2a built (books `eb7a2f67`); it has the fields of `books.account_balances`. The four layer-`cash` reads "before the month" of the Balance Sheet (`bsBefore`, `profitAndLossSum`, `bsActivity`, `firstBasisMonth`) change from `books.basis_lines` to that source, each still held to a Cash run by `onCash()`. The rows of the day's month are still read as lines. The scale book writes its cash rows by raw SQL, so it also writes the stored cash months.

**Tech Stack:** Go (`books/server`, package `books`), core report engine (`tinycld.org/core/reports`, not changed), Go test `TestStatementsScale` (run by the driver only).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`, section "State after plan part 25" (the 9d-2b design). Bar (human, 2026-11-20): the Balance Sheet as of a day, Accrual and Cash, 2 s at 500k lines. After task 9d-2a the Cash run (BS 2) is 1,465 ms at 100k (7 to 8 s projected at 500k); the Accrual run (BS 1) is 443 ms. The BS 2 statement of that run is in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9d2a/st-plans.md` ("run BS 2 cash, as of": 21 `SCAN books_cash_basis_lines`).

## Global Constraints

- Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Books: `/Users/nas/tmp/tinycld/books-purchases/books`, branch `books/p5-report-engine-scale`. Written against books `b9c05f4d` (code as at `eb7a2f67`; `b9c05f4d` adds only the ledger record of task 9d-2a), core `8969bdbd`. Every line number in this plan is of a file before this plan's edits; an edit moves the numbers below it.
- No core change. No client change. No migration change (the table exists since task 9d-2a).
- The Cash figures must not change. The cross-check (the stored read equals the line read: rows, figures and currency codes; several as-of days, a month with no line, a section with no line in the month, fiscal years from January and from July) must pass on Cash and on Accrual. A cross-check that fails is a defect of the change: do not change `balanceSheetSpecFromLines` or the expected figures to make it pass.
- The drill start holds on Cash (human, 2026-11-20): an account's drill starts on the 1st of the month of its first line on the run's method. `TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth` (`server/report_cash_basis_test.go:239`) must pass with no change.
- Every one of the four reads keeps `onCash()`: an Accrual run reads no stored cash month.
- A month with no posted cash row has no stored row (task 9d-2a). `bsActivity` counts the stored cash months, so a row with totals 0 / 0 would make an account "active".
- The implementer starts no scale run and takes no timing: the 100k run is a `NEEDS-RUN` for the driver (task 9d-2b-2, step 8).
- Run the whole Books Go package once only, in task 9d-2b-2 step 6. Other steps run named tests.
- Do not edit `books/docs` before task 9d-2b-2 step 9.
- Evidence of the planner's build: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part26-scratch/` (each changed file after the change, the new test file, `overlay-green.json`, `overlay-red.json`, `overlay-red1.json`, `red.log`, `red1.log`, `green.log`, `green-reports.log`). The whole Books Go package ran once with this change through a Go overlay (`green.log`): one failure, the revision literal of `TestBalanceSheetShippedUnpublished` (now in task 9d-2b-2 step 1). After that fix, and after `onCashLayer` was deleted, `go vet` and the report tests (`-run 'TestBalanceSheet|TestCashBasisReport|TestStatementsScaleBook$|TestStatementsScaleBars$|TestReportSources'`) passed (`green-reports.log`); the whole package was not run again.

## Review Focus

- A stored cash month that is also the day's month must not count twice: that month is read from its rows. Pinned by `TestBalanceSheetReadsTheStoredCashMonths` (the April figure, after April's stored month rose).
- An Accrual run after a stored cash month changed: no figure may move. Pinned by the same test.
- An opened account on Cash: the "Balance before" row must hold the stored cash months, and with the month's ledger lines and cash rows it must add to the account's figure. The line read has no such row, so the cross-check does not see it. Pinned by `TestBalanceSheetCashOpenedAccountRows`.
- An account whose only activity is a stored cash month, with Rows "Active": a row on Cash, none on Accrual, none with Rows "Non-zero". Pinned by `TestBalanceSheetCashMonthMakesAnAccountActive`.
- An account with cash months and no ledger month on or before the day (its ledger months start later, or were lost): the drill start must come from the cash months. Pinned by `TestBalanceSheetCashDrillStartsAtTheFirstStoredCashMonth` and by the unchanged `TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth`.
- Accepted, not pinned: a drill start can differ from before in one case. Before, the Cash start was the month of the first cash row dated on or before the day; now it is the first stored cash month whose first day is on or before the day. An account whose first cash row is in the day's month but after the day, with no ledger month and no earlier cash month, got a range from the day to the day and now gets the 1st of the day's month to the day. Both ranges hold no line, the ledger months already behave so (`balanceDrill`'s comment), and the write paths do not make such an account (a cash row's account has a ledger line in the same or an earlier month). The reviewer checks that no other start moved.
- Currency: a stored cash month names `MAX(currency)` of its rows, or its account's currency when the rows name none (`cash_balances.go:105`); the row read gave the rows' own code. The codes are equal on every book the write paths make (one currency, each cash row copies its line's). The cross-checks compare the codes.
- A book whose cash rows did not pass through `syncCashLineRows` (the scale book's bulk insert) has no stored cash month, and a Cash Balance Sheet of it would silently show the Accrual history. Pinned by `TestStatementsScaleBook` (`requireCashMonthsEqualRows`, `verifyCashBalances`). Checked: no other test of the package writes `books_cash_basis_lines` by `INSERT` or `UPDATE`.

---

### Task 9d-2b-1: The report source `books.cash_basis_balances`, and the scale book's stored cash months

**Files:**
- Modify: `books/server/report_sources.go` (after the `books.account_balances` source, lines 101-117)
- Modify: `books/server/report_sources_test.go` (`booksSourceKeys` line 49, `expectedCatalog` line 299, the currency list line 637)
- Modify: `books/server/report_statements_scale_test.go` (`writeStatementsScaleBook` lines 45-62, `TestStatementsScaleBook` line 174)

**Interfaces:**
- Consumes (exist since task 9d-2a, do not change): `const cashBalancesCollection = "books_cash_basis_balances"` and `verifyCashBalances(app core.App) ([]BalanceDrift, error)` (`server/cash_balances.go`); test helper `requireCashMonthsEqualRows(t testing.TB, app core.App)` (`server/cash_balances_test.go:24`). Table columns: `id`, `account`, `period` (`YYYY-MM`), `month_start` (`YYYY-MM-01`), `debit_total`, `credit_total`, `currency`, `created`, `updated`.
- Produces (task 9d-2b-2 relies on these): report source key `books.cash_basis_balances` with fields `period` (Text), `month_start` (Date, text day), `debit_total`, `credit_total` (Money, currency field `currency`), `currency` (Text, `SameCurrencyAs: "books.journal_lines.currency"`) and the to-one link `account` to `books.accounts`. `writeStatementsScaleBook` returns a book whose stored cash months equal its cash rows.

- [ ] **Step 1: Write the failing tests**

In `books/server/report_sources_test.go`:

In `booksSourceKeys` (line 49), add the key after `"books.basis_lines",` (the list is sorted):

```go
	"books.basis_lines",
	"books.cash_basis_balances",
	"books.customers",
```

In `expectedCatalog`, after the `books.account_balances` entry (lines 299-307), add:

```go
		{
			key: "books.cash_basis_balances", collection: "books_cash_basis_balances", label: "Monthly cash-basis balances",
			fields: []wantField{
				{key: "period", typ: reports.Text, aggs: reports.MinMax, label: "Month"},
				{key: "month_start", typ: reports.Date, textDay: true, aggs: reports.MinMax, label: "Month start"},
				money("debit_total"), money("credit_total"), text("currency"),
			},
			links: []wantLink{toOne("account", "books.accounts", "account")},
		},
```

In `TestReportSourcesCatalog`, the list of the sources that have the Books currency (line 637) gets the key:

```go
				"books.journal_entries", "books.journal_lines", "books.basis_lines", "books.account_balances",
				"books.cash_basis_balances",
				"books.sales_documents", "books.sales_tax_lines", "books.sales_lines", "books.products",
```

In `books/server/report_statements_scale_test.go`, in `TestStatementsScaleBook`, after the check of `cashRows` and `cashNet` (the `if cashRows == 0 || cashNet != 0 { … }` block that ends at line 176), add:

```go
	// The stored cash months equal the cash rows, or a Cash Balance Sheet of
	// this book would not be the book's.
	requireCashMonthsEqualRows(t, b.app)
	if drifts, err := verifyCashBalances(b.app); err != nil || len(drifts) != 0 {
		t.Fatalf("verifyCashBalances on the scale book = %d drifts, err %v; want none", len(drifts), err)
	}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -run 'TestReportSourcesCatalog$|TestStatementsScaleBook$' .`
Expected: FAIL. `TestReportSourcesCatalog`: `source keys = […], want […]` (the new key is missing). `TestStatementsScaleBook` (expected from reading the code; the planner ran only the catalog failure, `red1.log`): `the stored cash months differ from the cash rows` with an empty `stored` list, then `verifyCashBalances on the scale book = <n> drifts`.

- [ ] **Step 3: Add the source**

In `books/server/report_sources.go`, after the `books.account_balances` source (its closing `},` at line 117) and before `books.customers`, add:

```go
		{
			// The cash-basis rows added up by account and month
			// (cash_balances.go): what a Cash statement reads for the months
			// before its day's month, beside the ledger's stored months.
			Key: "books.cash_basis_balances", Collection: cashBalancesCollection, Label: "Monthly cash-basis balances",
			Fields: []reports.Field{
				{Key: "period", Label: "Month", Type: reports.Text, Aggregates: reports.MinMax},
				{Key: "month_start", Label: "Month start", Type: reports.Date, TextDay: true, Aggregates: reports.MinMax},
				{Key: "debit_total", Label: "Debit total", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
				{Key: "credit_total", Label: "Credit total", Type: reports.Money, Aggregates: reports.SumAll, Currency: "currency"},
				// A cash row carries its journal line's currency, and so does its
				// month, so a report adds stored cash months to lines.
				{Key: "currency", Label: "Currency", Type: reports.Text, SameCurrencyAs: "books.journal_lines.currency"},
			},
			Links: []reports.Link{
				{Key: "account", Label: "Account", Target: "books.accounts", Cardinality: reports.ToOne, Field: "account"},
			},
		},
```

`books.accounts` gets no back link to it: no report walks from an account to its cash months.

- [ ] **Step 4: The scale book writes its stored cash months**

In `books/server/report_statements_scale_test.go`, replace `writeStatementsScaleBook` and the last sentence of its comment (lines 45-62) with:

```go
// writeStatementsScaleBook is the General Ledger's scale book with cash-basis
// rows, which the bulk writer does not write: for one entry in stCashEvery,
// each of its posted lines taken out again (debit and credit changed over) on
// the line's own day, as the cash basis takes out an unpaid invoice. An
// entry's cash rows add to zero, so a Cash run stays in balance, and a Cash
// run reads about a quarter more rows than an Accrual run. The bulk insert
// does not pass through syncCashLineRows, so the book's stored cash months
// (books_cash_basis_balances) are written here too, as that writer keeps them:
// one row for each account and month with a cash row.
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
	_, err = b.app.DB().NewQuery(`INSERT INTO books_cash_basis_balances (id, account, period, month_start, debit_total, credit_total, currency, created, updated)
SELECT 'm' || substr(lower(hex(randomblob(8))), 1, 14), account, substr(date, 1, 7), substr(date, 1, 7) || '-01', SUM(debit), SUM(credit), MAX(currency), '', ''
FROM books_cash_basis_lines WHERE posted IS true GROUP BY account, substr(date, 1, 7)`).Execute()
	if err != nil {
		t.Fatalf("write the stored cash months: %v", err)
	}
	return b
}
```

The first `INSERT` is not changed. An id is 15 characters (`'m'` and 14 hex digits), as PocketBase ids are.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -count=1 -run 'TestReportSources|TestStatementsScaleBook$|TestStatementsScaleBars$|TestCashBasisReport_ShippedSpecs$' .`
Expected: `go vet` prints nothing; `ok  tinycld.org/packages/books`. (`TestCashBasisReport_ShippedSpecs` runs core `CheckSources` over every Books source, the new one too.)

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_sources.go server/report_sources_test.go server/report_statements_scale_test.go
git commit -m "feat(reports): report source over the stored cash-basis months"
```

---

### Task 9d-2b-2: The Cash Balance Sheet reads the stored cash months

**Files:**
- Create: `books/server/report_balance_sheet_cash_months_test.go`
- Modify: `books/server/report_balance_sheet.go` (`Revision` lines 43-53, `profitAndLossSum` lines 353-373, `bsStoredBefore` and `bsBefore` lines 404-423, `bsActivity` lines 433-449)
- Modify: `books/server/report_spec_helpers.go` (`firstBasisMonth`, lines 85-105)
- Modify: `books/server/report_balance_sheet_test.go` (the two revision checks, lines 246-247 and 1167)
- Modify: `books/server/report_cash_basis_test.go` (the revision table, lines 391-393)
- Modify: `books/docs/plans/general-ledger-ledger.md` (the record, step 9)

**Interfaces:**
- Consumes from task 9d-2b-1: report source `books.cash_basis_balances` (fields `month_start`, `debit_total`, `credit_total`, `id`; link `account`).
- Consumes (exist, do not change): `glSum(source string, value *reports.Node, where ...*reports.Node) *reports.Node` (`server/report_general_ledger.go:219`, a `sum` lookup, 0 when no row), `tbStoredCount(where ...*reports.Node)` (`server/report_trial_balance.go:110`), `bsSigned(debit, credit string)`, `onCash()`, `tdAccount()`, `bsAccountLines`, `basisFilter`, `op`, `fld`, `link`, `lit`, `prm`, `mustJSON`. Test helpers: `paidBook(t) (cashReportBook, bill, payment)`, `newCashReportBook`, `cashReportBook.bs(t, asOf, method)`, `postedBillOn`, `payOn`, `pay`, `voidBillPaymentAs`, `mkBill`, `mkBillLine`, `accountLine`, `mustPostBill`, `newPaymentEnv`, `newInvoice`, `mkInvoiceLine`, `item`, `mustPostInvoice`, `mkApplication`, `mustPostPayment`, `requireNoCashDrift`, `registerBooksSources`, `saveSettings`, `saveRecord`, `runBS`, `runTBSpec`, `tbBook{app, reader}`, `balanceSheetSpecFromLines`, `bsFigures`, `bsAccount`, `bsFindAccount`, `bsRow`, `bsFigure`, `bsWireKey`, `moneyCell`, `labelsOf`, `requireDrill`, `rangeJSON`, `idList`, `cashDay2` (`"2026-04-15"`).
- Produces: `bsCashStoredBefore(x *reports.Node) []*reports.Node` (`server/report_balance_sheet.go`); the shipped Balance Sheet at `Revision: 7`. No later task of this unit uses a new name.

- [ ] **Step 1: Write the tests**

Create `books/server/report_balance_sheet_cash_months_test.go`:

```go
package books

import (
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/reports"
)

// The Cash Balance Sheet reads the cash-basis rows of the months before a
// day's month from the stored cash months (books_cash_basis_balances), and the
// rows of the day's month as lines. These tests hold its figures to the line
// read and prove each of its four reads of the table.

// newBSCashBillBook is a book of bills over two fiscal years, built through
// the write paths: a bill of 2025-08-12 (700.00 office, 300.00 software), a
// payment of 2025-09-20 that is voided, a payment of 2025-11-05 that a
// correction of 2026-02-10 replaces, an unpaid bill of 2026-01-15 (postage), a
// bill of 2026-03-10 that a correction replaces (200.00 software), and a
// payment of 2026-04-15 on the correction. No line is dated October or
// December 2025, or May 2026 and later.
func newBSCashBillBook(t *testing.T) cashReportBook {
	t.Helper()
	b := newCashReportBook(t)
	first := b.postedBillOn(t, b.summit, "2025-08-12", billAmount{b.office, 70000}, billAmount{b.software, 30000})
	voided := b.payOn(t, b.summit, "2025-09-20", payLine{first, 40000})
	if err := voidBillPaymentAs(b.app, voided.Id, b.users.bookkeeper.Id); err != nil {
		t.Fatalf("void a payment: %v", err)
	}
	replaced := b.payOn(t, b.summit, "2025-11-05", payLine{first, 25000})
	b.pay(t, b.summit, map[string]any{"date": "2026-02-10", "replaces": replaced.Id}, payLine{first, 30000})
	b.postedBillOn(t, b.officeSupply, "2026-01-15", billAmount{b.postage, 5000})
	wrong := b.postedBillOn(t, b.officeSupply, "2026-03-10", billAmount{b.office, 12345})
	draft := mkBill(t, b.app, b.officeSupply, map[string]any{"date": "2026-03-10", "due_date": "2026-03-10", "replaces": wrong.Id})
	mkBillLine(t, b.app, draft, 0, accountLine(b.software, 20000))
	corrected := mustPostBill(t, b.app, draft.Id, b.users.bookkeeper.Id)
	b.payOn(t, b.officeSupply, "2026-04-15", payLine{corrected, 10000})
	requireNoCashDrift(t, b.app)
	return b
}

// bsCashBillDays are a day in each month with a line, the days of the void and
// of the two corrections, a day before the cash rows of its own month
// (2026-04-10: the payment is of April 15), days in months with no line
// (October and December 2025, May 2026 and later), the first and last day of
// both fiscal years, and days whose prior day is a day with a line
// (2026-09-20, 2026-11-05, 2027-02-10).
var bsCashBillDays = []string{
	"2025-08-31", "2025-09-20", "2025-10-31", "2025-12-31", "2026-01-01", "2026-01-31", "2026-02-10",
	"2026-03-10", "2026-04-10", "2026-04-30", "2026-05-31", "2026-06-30", "2026-07-01", "2026-09-20", "2026-11-05", "2027-02-10",
}

// newBSCashInvoiceBook is a book of invoices built through the write paths: an
// invoice of 2026-02-03 (500.00), a payment of 300.00 on 2026-03-12, an
// invoice correction (450.00) that takes over the payment, and an unpaid
// invoice of 2026-05-06. No line is dated April 2026.
func newBSCashInvoiceBook(t *testing.T) tbBook {
	t.Helper()
	pe := newPaymentEnv(t)
	registerBooksSources(t)
	invoice := func(fields map[string]any, total int64) *core.Record {
		t.Helper()
		draft, err := newInvoice(pe.app, pe.customer, fields)
		if err != nil {
			t.Fatalf("save a draft invoice: %v", err)
		}
		mkInvoiceLine(t, pe.app, draft, 0, item(pe.service, 1, total, false))
		return mustPostInvoice(t, pe.app, draft.Id, pe.users.bookkeeper.Id)
	}
	first := invoice(map[string]any{"date": "2026-02-03", "due_date": "2026-03-03"}, 50000)
	p := pe.mkPayment(t, map[string]any{"date": "2026-03-12", "amount": 30000, "deposit_to": pe.checking.Id})
	mkApplication(t, pe.app, p, first, 0, 30000)
	mustPostPayment(t, pe.app, p.Id, pe.users.bookkeeper.Id)
	invoice(map[string]any{"date": "2026-02-03", "due_date": "2026-03-03", "replaces": first.Id}, 45000)
	invoice(map[string]any{"date": "2026-05-06", "due_date": "2026-06-05"}, 8000)
	requireNoCashDrift(t, pe.app)
	return tbBook{app: pe.app, reader: pe.users.readOnly}
}

// bsCashInvoiceDays hold a day before the cash rows of its own month
// (2026-03-05: the payment is of March 12) and a month with no line (April).
var bsCashInvoiceDays = []string{
	"2026-02-03", "2026-02-28", "2026-03-05", "2026-03-12", "2026-03-31", "2026-04-30", "2026-05-31", "2026-06-30", "2026-07-01",
	"2027-03-12", "2027-05-06",
}

// requireBSStoredEqualsLines runs the Balance Sheet and its line read
// (balanceSheetSpecFromLines) on each method, day and Rows choice, and fails
// when a row, a figure or a currency code differs. The book must have
// cash-basis rows and stored cash months: on the last day the Cash run differs
// from the Accrual run, and its Total Assets names the currency in both
// columns.
func requireBSStoredEqualsLines(t *testing.T, what string, book tbBook, days []string) {
	t.Helper()
	if n, err := book.app.CountRecords(cashBalancesCollection); err != nil || n == 0 {
		t.Fatalf("%s: %d stored cash months (err %v), want some", what, n, err)
	}
	last := map[string]string{}
	for _, method := range []string{"accrual", "cash"} {
		for _, asOf := range days {
			for _, rows := range []string{"nonzero", "active"} {
				params := map[string]any{"asOf": asOf, "rows": rows, "method": method}
				opts := reports.Options{Today: asOf}
				stored := bsFigures(runTBSpec(t, book, balanceSheetSpec(), params, opts))
				lines := bsFigures(runTBSpec(t, book, balanceSheetSpecFromLines(), params, opts))
				if stored != lines {
					t.Errorf("%s, %s, as of %s, rows %s:\nstored months\n%s\nlines\n%s", what, method, asOf, rows, stored, lines)
				}
				last[method] = stored
			}
		}
	}
	if last["cash"] == last["accrual"] {
		t.Errorf("%s: the Cash run equals the Accrual run on the last day, so the book proves nothing", what)
	}
	named := false
	for _, line := range strings.Split(last["cash"], "\n") {
		if strings.Contains(line, "|Total Assets|") {
			named = strings.Count(line, " USD") == 2
		}
	}
	if !named {
		t.Errorf("%s: Total Assets on Cash does not show USD on Current and Prior year in\n%s", what, last["cash"])
	}
}

// The stored read shows what the line read shows on a book whose cash-basis
// rows come from posts, payments, a void, a replaced payment and a corrected
// bill, with the fiscal year starting in January and in July.
func TestBalanceSheetCashMonthsEqualTheLines_Bills(t *testing.T) {
	for _, fiscalStart := range []int{1, 7} {
		b := newBSCashBillBook(t)
		saveSettings(t, b.app, map[string]any{"fiscal_year_start_month": fiscalStart})
		requireBSStoredEqualsLines(t, map[int]string{1: "fiscal start 1", 7: "fiscal start 7"}[fiscalStart],
			tbBook{app: b.app, reader: b.reader}, bsCashBillDays)
	}
}

// The same on a book with an invoice correction that takes over a payment.
func TestBalanceSheetCashMonthsEqualTheLines_InvoiceCorrection(t *testing.T) {
	for _, fiscalStart := range []int{1, 7} {
		book := newBSCashInvoiceBook(t)
		saveSettings(t, book.app, map[string]any{"fiscal_year_start_month": fiscalStart})
		requireBSStoredEqualsLines(t, map[int]string{1: "fiscal start 1", 7: "fiscal start 7"}[fiscalStart], book, bsCashInvoiceDays)
	}
}

// bumpCashMonth adds 7 to the stored debit of one cash month, past the hooks.
func bumpCashMonth(t *testing.T, app core.App, account *core.Record, period string) {
	t.Helper()
	res, err := app.DB().NewQuery("UPDATE books_cash_basis_balances SET debit_total = debit_total + 7 WHERE account = {:a} AND period = {:p}").
		Bind(dbx.Params{"a": account.Id, "p": period}).Execute()
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("cash month %s %s: %d rows changed, want 1", account.GetString("name"), period, n)
	}
}

// On Cash the months before a day's month are read from the stored cash
// months, and the day's month from its rows; an Accrual run reads no stored
// cash month. paidBook: bills of March 10, a payment of April 15.
func TestBalanceSheetReadsTheStoredCashMonths(t *testing.T) {
	b, _, _ := paidBook(t)
	type figures struct{ payable, payablePrior, retained, netIncomePrior int64 }
	cell := func(r reports.Row, col string) int64 {
		t.Helper()
		v, _ := moneyCell(t, r, col)
		return v
	}
	// As of 2027-06-30 the prior day is 2026-06-30: 2026 income and expense are
	// Retained earnings on the day and Net income on the prior day.
	read := func(method string) figures {
		t.Helper()
		res := b.bs(t, "2027-06-30", method)
		payable := bsAccount(t, res, b.payable)
		return figures{
			cell(payable, "current"), cell(payable, "prior"),
			cell(bsRow(t, res, "Retained earnings"), "current"), cell(bsRow(t, res, "Net income"), "prior"),
		}
	}
	accrual, cash := read("accrual"), read("cash")
	april := bsFigure(t, b.bs(t, "2026-04-30", "cash"), b.payable)

	bumpCashMonth(t, b.app, b.payable, "2026-04")
	bumpCashMonth(t, b.app, b.office, "2026-03")

	// A debit lowers Accounts Payable (credit-normal) and raises an expense.
	want := figures{cash.payable - 7, cash.payablePrior - 7, cash.retained - 7, cash.netIncomePrior - 7}
	if got := read("cash"); got != want {
		t.Errorf("Cash after two earlier stored cash months rose by 7 = %+v, want %+v (before: %+v): the months before a day's month are read from books_cash_basis_balances", got, want, cash)
	}
	if got := read("accrual"); got != accrual {
		t.Errorf("Accrual after two stored cash months rose by 7 = %+v, want %+v: an Accrual run reads no cash month", got, accrual)
	}
	if got := bsFigure(t, b.bs(t, "2026-04-30", "cash"), b.payable); got != april {
		t.Errorf("Cash as of 2026-04-30, Accounts Payable = %d, want %d: April is the day's month, read from its rows, not from its stored cash month", got, april)
	}
}

// With Rows "Active", an account whose only activity is a stored cash month
// before the day's month has a row on Cash and none on Accrual.
func TestBalanceSheetCashMonthMakesAnAccountActive(t *testing.T) {
	b, _, _ := paidBook(t)
	saveRecord(t, b.app, cashBalancesCollection, map[string]any{
		"account": b.savings1010.Id, "period": "2026-03", "month_start": "2026-03-01", "currency": "USD",
	})
	shown := func(method, rows string) bool {
		t.Helper()
		res := runBS(t, b.app, b.reader, map[string]any{"asOf": "2026-06-30", "rows": rows, "method": method}, reports.Options{Today: "2026-06-30"})
		_, ok := bsFindAccount(res, b.savings1010)
		return ok
	}
	if !shown("cash", "active") {
		t.Error("Cash, Rows Active: 1010 Business Savings has no row, want one (it has a stored cash month before June)")
	}
	if shown("accrual", "active") {
		t.Error("Accrual, Rows Active: 1010 Business Savings has a row, want none (a cash month is no Accrual activity)")
	}
	if shown("cash", "nonzero") {
		t.Error("Cash, Rows Non-zero: 1010 Business Savings has a row, want none (its figures are 0)")
	}
}

// An opened account on Cash: the row for the months before the day's month
// holds the stored months and the stored cash months, then come the month's
// ledger lines and cash-basis rows; together they are the account's figure.
// paidBook: Accounts Payable holds 1,050.00 at the end of March on Accrual and
// nothing on Cash (both bills are unpaid, so the cash basis takes them out).
func TestBalanceSheetCashOpenedAccountRows(t *testing.T) {
	b, _, _ := paidBook(t)
	for _, c := range []struct {
		method           string
		opening, account int64
		lines            int
	}{
		// April: the payment's line (−400.00).
		{"accrual", 105000, 65000, 1},
		// April: the payment's line and its cash-basis share row (+400.00).
		{"cash", 0, 0, 2},
	} {
		params := map[string]any{"asOf": "2026-04-20", "rows": "active", "method": c.method}
		opts := reports.Options{Today: "2026-04-20"}
		closed := runBS(t, b.app, b.reader, params, opts)
		opts.Collapse = [][]any{}
		opts.Expand = [][]any{bsWireKey(t, bsAccount(t, closed, b.payable).GroupKey)}
		open := runBS(t, b.app, b.reader, params, opts)
		account := bsAccount(t, open, b.payable)
		var under []reports.Row
		for _, r := range open.Rows {
			if r.Section == account.ID && (r.Kind == reports.KindOpening || r.Kind == reports.KindDetail) {
				under = append(under, r)
			}
		}
		if len(under) != c.lines+1 || under[0].Kind != reports.KindOpening {
			t.Fatalf("%s: Accounts Payable's opened rows = %q, want the opening row and %d lines", c.method, labelsOf(under), c.lines)
		}
		sum := int64(0)
		for _, r := range under {
			v, _ := moneyCell(t, r, "current")
			sum += v
		}
		opening, _ := moneyCell(t, under[0], "current")
		figure, _ := moneyCell(t, account, "current")
		if opening != c.opening || figure != c.account || sum != figure {
			t.Errorf("%s: opening row %d, account row %d, opened rows add to %d; want %d, %d, and the rows to add to the account row",
				c.method, opening, figure, sum, c.opening, c.account)
		}
	}
}

// On Cash a drill starts on the first day of the account's first stored month,
// ledger or cash. With Accounts Payable's ledger months and its March cash
// month gone, that is April: the day comes from the stored cash months, not
// from the cash-basis rows (which start on March 10).
func TestBalanceSheetCashDrillStartsAtTheFirstStoredCashMonth(t *testing.T) {
	b, _, _ := paidBook(t)
	exec := func(what, sql string) {
		t.Helper()
		if _, err := b.app.DB().NewQuery(sql).Bind(dbx.Params{"a": b.payable.Id}).Execute(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	exec("delete the stored months of Accounts Payable", "DELETE FROM books_account_balances WHERE account = {:a}")
	requireDrill(t, bsAccount(t, b.bs(t, cashDay2, "cash"), b.payable), "current", map[string]string{
		"period": rangeJSON("2026-03-01", cashDay2), "method": `"cash"`, "accounts": idList(b.payable.Id)})
	exec("delete the March cash month of Accounts Payable", "DELETE FROM books_cash_basis_balances WHERE account = {:a} AND period = '2026-03'")
	requireDrill(t, bsAccount(t, b.bs(t, cashDay2, "cash"), b.payable), "current", map[string]string{
		"period": rangeJSON("2026-04-01", cashDay2), "method": `"cash"`, "accounts": idList(b.payable.Id)})
}
```

Write the minus sign in the two comments of `TestBalanceSheetCashOpenedAccountRows` as the character U+2212, as the other Books tests do, or as a plain `-`; do not write a `\u` escape.

In `books/server/report_balance_sheet_test.go` (lines 246-247), the revision is 7:

```go
	if shipped.Revision != 7 || !shipped.Unpublished {
		t.Errorf("revision %d, unpublished %v; want revision 7 (the stored cash months on Cash), shipped unpublished", shipped.Revision, shipped.Unpublished)
```

In the same file, `TestBalanceSheetShippedUnpublished` (line 1167) reads the stored row's revision; it is 7 too:

```go
	if row.GetString("title") != "Balance Sheet" || row.GetInt("shipped_revision") != 7 || row.GetString("source") != "books.basis_lines" {
```

In `books/server/report_cash_basis_test.go` (lines 391-393):

```go
		// 3 for the Method param, 4 for the stored months, 5 for the stored
		// months on Cash, 6 for the drill's first day from the stored months,
		// 7 for the stored cash months on Cash.
		{balanceSheetShipped(), 7, "asOf"},
```

- [ ] **Step 2: Run the tests to verify which fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -run 'TestBalanceSheetCash|TestBalanceSheetReadsTheStoredCashMonths$|TestBalanceSheetSpecLoads$|TestCashBasisReport_ShippedSpecs$' .`
Expected: FAIL, with these and no other failures:
- `TestBalanceSheetReadsTheStoredCashMonths`: `Cash after two earlier stored cash months rose by 7 = {payable:0 payablePrior:0 retained:-40000 netIncomePrior:-40000}, want {payable:-7 payablePrior:-7 retained:-40007 netIncomePrior:-40007}`.
- `TestBalanceSheetCashMonthMakesAnAccountActive`: `Cash, Rows Active: 1010 Business Savings has no row`.
- `TestBalanceSheetCashDrillStartsAtTheFirstStoredCashMonth`: `current@drill period = {"from":"2026-03-01","to":"2026-04-15"}, want {"from":"2026-04-01","to":"2026-04-15"}`.
- `TestBalanceSheetSpecLoads` and `TestCashBasisReport_ShippedSpecs`: `revision 6`, want 7 (the planner saw the second in `red.log`; the first is expected from reading). (`TestBalanceSheetShippedUnpublished` is not in this run; it fails on the revision too until step 3.)

`TestBalanceSheetCashMonthsEqualTheLines_Bills`, `TestBalanceSheetCashMonthsEqualTheLines_InvoiceCorrection` and `TestBalanceSheetCashOpenedAccountRows` PASS now: they hold the figures, which this task must not change. When one of them fails here, stop and report it: the book or the line read is wrong, not the change.

- [ ] **Step 3: The four reads**

In `books/server/report_balance_sheet.go`, replace `profitAndLossSum` and its comment (lines 353-373) with:

```go
// profitAndLossSum adds credits − debits of the posted lines on or before x
// that match which, on the run's method; 0 when none does. which names the
// day it reads: a line's date, or a stored month's first day (a fiscal year
// starts on the first day of a month, so every day of a month is in the
// fiscal year of that first day). The ledger lines of the months before x's
// month come from the stored months on both methods; on Cash the cash-basis
// rows of those months are added, from the stored cash months; x's month comes
// from its lines.
func profitAndLossSum(x *reports.Node, which func(day string) *reports.Node) *reports.Node {
	monthStart := op("startOfMonth", x)
	net := op("sub", fld("credit"), fld("debit"))
	storedNet := op("sub", fld("credit_total"), fld("debit_total"))
	return op("add",
		glSum("books.account_balances", storedNet,
			op("lt", fld("month_start"), monthStart), which("month_start")),
		glSum("books.cash_basis_balances", storedNet,
			onCash(), op("lt", fld("month_start"), monthStart), which("month_start")),
		glSum("books.basis_lines", net,
			basisFilter(), op("eq", fld("posted"), lit(true)), op("lte", fld("date"), x),
			op("gte", fld("date"), monthStart), which("date")),
	)
}
```

Replace `bsStoredBefore`, `bsBefore` and their comments (lines 404-423) with:

```go
// bsStoredBefore are the stored months of the row's account before the month
// of day x: the ledger's (books.account_balances), which both methods read, or
// the cash-basis rows' (books.cash_basis_balances). Both sources have the same
// fields.
func bsStoredBefore(x *reports.Node) []*reports.Node {
	return []*reports.Node{
		op("eq", link("account"), tdAccount()),
		op("lt", fld("month_start"), op("startOfMonth", x)),
	}
}

// bsCashStoredBefore are the stored cash months of the row's account before
// the month of day x, on a Cash run; no row on Accrual.
func bsCashStoredBefore(x *reports.Node) []*reports.Node {
	return append([]*reports.Node{onCash()}, bsStoredBefore(x)...)
}

// bsBefore is the row's account's balance at the start of the month of day x:
// the stored months (books_account_balances) and, on Cash, the stored cash
// months (books_cash_basis_balances), so its cost does not grow with the
// account's ledger lines or its cash-basis rows.
func bsBefore(x *reports.Node) *reports.Node {
	return op("add",
		glSum("books.account_balances", bsSigned("debit_total", "credit_total"), bsStoredBefore(x)...),
		glSum("books.cash_basis_balances", bsSigned("debit_total", "credit_total"), bsCashStoredBefore(x)...),
	)
}
```

`bsBalanceAt` (below `bsBefore`) is not changed: it adds the lines of x's month to `bsBefore`, and `bsAccountLines` stays for it.

`onCashLayer` has no caller after this step (with the new `firstBasisMonth` below; `command grep -n 'onCashLayer' *.go` in `books/server` then shows only its definition). Delete it and its comment (lines 390-392):

```go
// onCashLayer is true for a cash-basis row of books.basis_lines. A lookup
// that holds it reads no ledger line.
func onCashLayer() *reports.Node { return op("eq", fld("layer"), lit("cash")) }
```

Replace `bsActivity` and its comment (lines 433-449) with:

```go
// bsActivity is above 0 when the row's account has a posted line on or before
// day x, the day of the report's rows: a stored month exists only for a month
// with a posted ledger line; on Cash the stored cash months before x's month
// count too (a stored cash month exists only for a month with a cash-basis
// row).
func bsActivity(x *reports.Node) *reports.Node {
	earlier := &reports.Node{
		Op:     "lookup",
		Source: "books.cash_basis_balances",
		Where:  op("and", bsCashStoredBefore(x)...),
		Value:  mustJSON(fld("id")),
		Pick:   "count",
	}
	return op("add",
		op("add", tbStoredCount(bsStoredBefore(x)...), op("coalesce", earlier, lit(0))),
		op("count", fld("date")),
	)
}
```

In `balanceSheetShipped` (lines 51-53), add the revision note and raise the revision:

```go
		// 6: a drill's period starts on the first day of the account's first
		// month (read from the stored months), not on its first line's day.
		// 7: on Cash the cash-basis rows of the months before a day's month
		// come from the stored cash months (books_cash_basis_balances), not
		// from the rows.
		Revision:    7,
```

In `books/server/report_spec_helpers.go`, replace `firstBasisMonth` and its comment (lines 85-105) with:

```go
// firstBasisMonth is the first day of the first month, on or before day d, in
// which the account row has a posted line on the run's method: where a Balance
// Sheet drill's range starts, so it holds every line a balance on d sums. It
// reads the account's stored months (books_account_balances: one row for each
// month with a posted ledger line), not its lines, so its cost does not grow
// with them; on Cash also its stored cash months (books_cash_basis_balances:
// one row for each month with a cash-basis row). No value when the account
// has neither.
func firstBasisMonth(d *reports.Node) *reports.Node {
	firstMonth := func(source string, more ...*reports.Node) *reports.Node {
		where := append([]*reports.Node{op("eq", link("account"), tdAccount()), op("lte", fld("month_start"), d)}, more...)
		return &reports.Node{
			Op:     "lookup",
			Source: source,
			Where:  op("and", where...),
			Value:  mustJSON(fld("month_start")),
			Pick:   "min",
		}
	}
	stored := firstMonth("books.account_balances")
	cash := firstMonth("books.cash_basis_balances", onCash())
	// The earlier of the two; either may have no value. Core's min takes
	// numbers only, so the days are compared.
	a, b := op("coalesce", stored, cash), op("coalesce", cash, stored)
	return op("if", op("lte", a, b), a, b)
}
```

`firstLineWhere` stays: `firstLine` and `firstLineFY` use it.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -count=1 -run 'TestBalanceSheet|TestCashBasisReport|TestStatementsScaleBook$' .`
Expected: `go vet` prints nothing; `ok  tinycld.org/packages/books`. This run holds the run topics (`TestBalanceSheetTopics`), the two older cross-checks (`TestBalanceSheetStoredMonthsEqualTheLines`, `TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows`) and the drill start (`TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth`), none of them changed.

- [ ] **Step 5: Check that no read of the lines before the month is left**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && command grep -n '"books.basis_lines"\|onCashLayer' report_balance_sheet.go report_spec_helpers.go`
Expected: three lines, all in `report_balance_sheet.go`: the spec's `Source`, the third `glSum` of `profitAndLossSum`, and the `glSum` of `bsBalanceAt`. The two lookups read the day's month only (`date >= startOfMonth`). No line in `report_spec_helpers.go`, and no `onCashLayer`.

- [ ] **Step 6: Run the Books Go package once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 .`
Expected: `ok  tinycld.org/packages/books` (about 6 minutes). A failure in a Cash report test names a book whose cash rows have no stored cash month: find the write that made the rows and why it did not pass through `syncCashLineRows`; do not change the report back.

- [ ] **Step 7: Commit the code**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_balance_sheet.go server/report_spec_helpers.go server/report_balance_sheet_cash_months_test.go server/report_balance_sheet_test.go server/report_cash_basis_test.go
git commit -m "perf(reports): the Cash Balance Sheet reads the stored cash months"
```

The run of step 8 is then of a committed tree; write that commit's hash in the ledger record.

- [ ] **Step 8: Hand the 100k run to the driver**

Hand back (the driver runs it and sends an agent back for steps 9 and 10 with the two files):

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9d2b go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
```

Needed from it: PASS; `st-results.md` (BS 1 and BS 2 medians, the TB and TD medians, no run refused; the group rows (10) and rows (20) of BS 1 and BS 2 as before); from `st-plans.md`, section "run BS 2 cash, as of": the count of `SCAN books_cash_basis_lines` (21 after task 9d-2a) and the lines that name `books_cash_basis_balances`. Before: `scale-task9d2a/st-results.md` (BS 1 443 ms, BS 2 1.465 s).

- [ ] **Step 9: Write the ledger record**

Read the end of `books/docs/plans/general-ledger-ledger.md` (the record `#### Task 9d-2a: the stored cash months …`, line 756 at books `b9c05f4d`) and append after it, in the same form, `#### Task 9d-2b: the Cash Balance Sheet reads the stored cash months (2026-11-20)` with: the source `books.cash_basis_balances`; the four reads that changed (`bsBefore`, `profitAndLossSum`, `bsActivity`, `firstBasisMonth`) and that each keeps `onCash()`; the day's month is still read from its rows; Revision 7; the scale book writes its stored cash months; the tests that hold the figures (the two cross-checks on a bill book and an invoice book, both fiscal starts) and the four that prove the reads; the 100k figures from step 8 beside those of task 9d-2a in a table of the same columns, the count of `SCAN books_cash_basis_lines` in statement 5 of BS 2 before (21) and after, and what is left for the 2 s bar at 500k (task 9e-2, decided from a 500k run).

- [ ] **Step 10: Commit the record**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add docs/plans/general-ledger-ledger.md
git commit -m "docs: record the Cash Balance Sheet on the stored cash months and its 100k run"
```
