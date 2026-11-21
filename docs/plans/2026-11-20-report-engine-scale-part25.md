# Report Engine Scale, Part 25: The Stored Cash Months (task 9d-2a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Books keeps a stored cash-basis month table (`books_cash_basis_balances`: one row for each account and month, the totals of the posted `books_cash_basis_lines` rows), current after every post, payment, void, replacement and correction, with its verify and its rebuild.

**Architecture:** `syncCashLineRows` (`server/cash_basis.go`) is the one writer of `books_cash_basis_lines`. It now collects the (account, month) of each row it deletes or writes and, in the same transaction, makes each of those stored months equal the cash rows again (`refreshCashBalance`: a sum over the index `(account, date)`; the row is deleted when the month has no cash row). `verifyCashBalances` compares the table with `GROUP BY account, month` of the cash rows; `rebuildBalances` repairs the months after it repairs the cash rows. No report reads the table in this part: task 9d-2b (the next part) makes the Cash Balance Sheet read it.

**Tech Stack:** Go (`books/server`, package `books`), PocketBase JS migration, Go test `TestStatementsScale` (run by the driver only).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (sections "State after plan part 23", item 9d-2, and "State after plan part 25"). Bar (human, 2026-11-20): the Balance Sheet as of a day, Accrual and Cash, 2 s at 500k lines. After task 9e-1 the Cash run (BS 2) is 1,508 ms at 100k, about 8 s projected at 500k; the Accrual run (BS 1) is 456 ms.

## Global Constraints

- Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Books: `/Users/nas/tmp/tinycld/books-purchases/books`, branch `books/p5-report-engine-scale`. Written against books `63cdf3cb`, core `ad568afa`.
- No core change. No client change: no screen reads the table, so `tinycld/books/collections.ts` does not declare it.
- The migration is an in-place edit of `pb-migrations/2100000021_create_books_bill_payments.js`. Reason: Books has no release (no git tag, version 0.1.0), so every database starts empty (CLAUDE.md, "Unreleased migrations may be edited in place"); the table sits in the file that creates the cash rows it sums, and its `down` drops it with them. Do not add a new migration file.
- Money is integer minor units, at most 2^53-1 (`maxStoredAmount`, `server/posting.go`).
- A month with no posted cash row has NO stored row (a later report counts the stored months of an account to decide "active").
- The stored months are written with `tx.Save` / `tx.Delete` inside the transaction of the write that changed the cash rows, never with raw SQL.
- The implementer starts no scale run: it is a `NEEDS-RUN` for the driver (step 8). No timing probe.
- This task changes 9 code files and the ledger record (about 470 lines with tests); 5 of the files change by 1 to 10 lines. It is one task, because the table, its writer and its verify cannot be tested apart.
- Evidence of the planner's build: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part25-scratch/` (the two new Go files, the migration after the change, `green.log`). The whole Books Go package passed with this change through a Go overlay.

## Review Focus

- A month that loses its last cash row (a voided payment that was the only cash row of an account in that month): the stored row must go, not stay with 0 / 0. Pinned by `TestCashBalances_FollowThePostsPaymentsAndVoids` (software, April).
- A stored month with no cash row and totals 0 / 0: `verifyCashBalances` must report it (unlike `verifyBalances`, which accepts a zero ledger month). Pinned by `TestVerifyCashBalances_ReportsEachDriftAndTheRebuildRepairsIt`.
- A cash row deleted past the hooks: the month drifts until the rebuild; `rebuildBalances` must repair the cash rows first, then the months. Pinned by `TestCashBalances_ARowDeletedPastTheHooksIsADrift` and by the order in `rebuildBalances`.
- Replacements and corrections (a bill payment correction, a payment correction that drops an invoice, an invoice correction, a bill correction, credit memos, vendor credits, refunds, write-offs): every existing cash-basis test ends in `requireNoCashDrift`, which now also compares the stored months with the cash rows through its own SQL.
- A document delete: the relations `bill`, `invoice`, … on `books_cash_basis_lines` are `cascadeDelete: true`, and a cascade does not pass through `syncCashLineRows`. The reviewer checks that no test of the package fails `requireNoDrift` (it now verifies the cash months); a document that has cash rows has a posted entry, and Books deletes only drafts.

---

### Task 9d-2a: The stored cash months, kept current, with verify and rebuild

**Files:**
- Modify: `books/pb-migrations/2100000021_create_books_bill_payments.js` (new collection before `pbc_books_cash_tax`; the `down` list)
- Create: `books/server/cash_balances.go`
- Modify: `books/server/cash_basis.go` (`syncCashLineRows`, lines 484-524)
- Modify: `books/server/balances.go` (`rebuildBalances`, lines 234-290)
- Modify: `books/server/oauth_scopes.go` (line 89)
- Create: `books/server/cash_balances_test.go`
- Modify: `books/server/cash_basis_test.go` (`requireNoCashDrift`, line 177)
- Modify: `books/server/env_test.go` (`requireNoDrift`, line 377)
- Modify: `books/server/registrations_test.go` (`serverOnly`, line 31)
- Modify: `books/docs/plans/general-ledger-ledger.md` (the record, step 9)

**Interfaces:**
- Consumes (exist, do not change): `balanceKey{account, period string}`, `balanceTotals{debit, credit int; currency string}`, `BalanceDrift`, `firstDayOf(period string) string`, `balanceOverflow(tx core.App, accountID, period string) error` (all `server/balances.go`); `periodOf(day string) string`; `maxStoredAmount`; `cashRowDiff`, `cashLinesCollection` (`server/cash_basis.go`); test helpers `cashBook(t) *cashBillEnv` (fields `app`, `bill`, `office`, `software`, `payable`), `saveRecord`, `requireNoDrift`.
- Produces (task 9d-2b relies on these names):
  - collection `books_cash_basis_balances`: `account` (relation to `books_accounts`, required), `period` (`YYYY-MM`), `month_start` (`YYYY-MM-01`), `debit_total`, `credit_total` (int, 0 to 2^53-1), `currency` (`^[A-Z]{3}$`, required); unique index `(account, period)`; list and view rule READ, no client write.
  - `const cashBalancesCollection = "books_cash_basis_balances"`
  - `func cashMonthTotals(app core.App) (map[balanceKey]balanceTotals, error)`
  - `func refreshCashBalance(tx core.App, key balanceKey) error`
  - `func refreshCashBalances(tx core.App, months map[balanceKey]bool) error`
  - `func verifyCashBalances(app core.App) ([]BalanceDrift, error)`
  - `func rebuildCashBalances(tx core.App) (int, error)`
  - test helpers `requireCashMonthsEqualRows(t testing.TB, app core.App)` and `cashMonthOf(t testing.TB, app core.App, account *core.Record, period string) (debit, credit int, found bool)`; `requireNoDrift` and `requireNoCashDrift` now fail on a drifted cash month.

- [ ] **Step 1: Write the failing tests**

Create `books/server/cash_balances_test.go`:

```go
package books

import (
	"reflect"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// cashMonth is one stored cash month, or one month of the cash rows added up.
type cashMonth struct {
	Account string `db:"account"`
	Period  string `db:"period"`
	Start   string `db:"month_start"`
	Debit   int    `db:"debit"`
	Credit  int    `db:"credit"`
}

// requireCashMonthsEqualRows reads the stored cash months and the cash rows
// with its own SQL (not through verifyCashBalances) and fails when they
// differ: the stored months equal the posted cash rows added up by account
// and month, and a month with no cash row has no stored row.
func requireCashMonthsEqualRows(t testing.TB, app core.App) {
	t.Helper()
	var stored, want []cashMonth
	if err := app.DB().NewQuery(`
		SELECT account, period, month_start, CAST(debit_total AS INTEGER) AS debit, CAST(credit_total AS INTEGER) AS credit
		FROM books_cash_basis_balances ORDER BY account, period`).All(&stored); err != nil {
		t.Fatalf("read the stored cash months: %v", err)
	}
	if err := app.DB().NewQuery(`
		SELECT account, substr(date, 1, 7) AS period, substr(date, 1, 7) || '-01' AS month_start,
		       CAST(SUM(debit) AS INTEGER) AS debit, CAST(SUM(credit) AS INTEGER) AS credit
		FROM books_cash_basis_lines WHERE posted = 1
		GROUP BY account, substr(date, 1, 7) ORDER BY account, period`).All(&want); err != nil {
		t.Fatalf("add the cash rows per account and month: %v", err)
	}
	if !reflect.DeepEqual(stored, want) {
		t.Errorf("the stored cash months differ from the cash rows:\nstored %+v\nrows   %+v", stored, want)
	}
}

// cashMonthOf is the stored cash month of account for period: its debit and
// credit totals, and whether the row exists.
func cashMonthOf(t testing.TB, app core.App, account *core.Record, period string) (debit, credit int, found bool) {
	t.Helper()
	rows, err := app.FindAllRecords(cashBalancesCollection, dbx.HashExp{"account": account.Id, "period": period})
	if err != nil {
		t.Fatalf("read cash month %s %s: %v", account.GetString("name"), period, err)
	}
	if len(rows) == 0 {
		return 0, 0, false
	}
	if len(rows) > 1 {
		t.Fatalf("cash month %s %s has %d rows, want one", account.GetString("name"), period, len(rows))
	}
	return rows[0].GetInt("debit_total"), rows[0].GetInt("credit_total"), true
}

// The book of cashBook: bill 1 (700.00 office, 300.00 software) on March 10,
// paid 400.00 on April 15 (voided) and 250.00 on May 20; bill 2 (123.45
// office) on April 15, paid in full on May 20; bill 3 (9.99 software) on May
// 20, unpaid. Every post, payment and void wrote its months.
func TestCashBalances_FollowThePostsPaymentsAndVoids(t *testing.T) {
	ce := cashBook(t)
	requireCashMonthsEqualRows(t, ce.app)

	type month struct{ debit, credit int }
	for _, c := range []struct {
		what    string
		account *core.Record
		period  string
		want    month
	}{
		{"bill 1 taken out of office", ce.office, "2026-03", month{0, 70000}},
		{"bill 1 taken out of software", ce.software, "2026-03", month{0, 30000}},
		{"bill 1 taken out of payables", ce.payable, "2026-03", month{100000, 0}},
		{"bill 2 taken out of office", ce.office, "2026-04", month{0, 12345}},
		{"bill 2 taken out of payables", ce.payable, "2026-04", month{12345, 0}},
		{"the May payments bring office back", ce.office, "2026-05", month{17500 + 12345, 0}},
		{"software: bill 1's share back, bill 3 out", ce.software, "2026-05", month{7500, 999}},
		{"payables: bill 3 out, the two payments", ce.payable, "2026-05", month{999, 25000 + 12345}},
	} {
		debit, credit, found := cashMonthOf(t, ce.app, c.account, c.period)
		if !found || (month{debit, credit}) != c.want {
			t.Errorf("%s: cash month %s = %d / %d (found %v), want %d / %d", c.what, c.period, debit, credit, found, c.want.debit, c.want.credit)
		}
	}
	// The voided April payment was the only cash row of software in April: its
	// month row went with its rows.
	if debit, credit, found := cashMonthOf(t, ce.app, ce.software, "2026-04"); found {
		t.Errorf("software has a cash month for April (%d / %d) after the only payment of that month was voided", debit, credit)
	}
	if drifts, err := verifyCashBalances(ce.app); err != nil || len(drifts) != 0 {
		t.Errorf("verifyCashBalances on a book built through the write paths = %+v, err %v; want none", drifts, err)
	}
}

// verifyCashBalances names each kind of drift, and rebuildBalances repairs
// them all and counts them.
func TestVerifyCashBalances_ReportsEachDriftAndTheRebuildRepairsIt(t *testing.T) {
	ce := cashBook(t)
	exec := func(what, sql string, params dbx.Params) {
		t.Helper()
		if _, err := ce.app.DB().NewQuery(sql).Bind(params).Execute(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	exec("change a total", "UPDATE books_cash_basis_balances SET debit_total = debit_total + 1 WHERE account = {:a} AND period = '2026-05'", dbx.Params{"a": ce.office.Id})
	exec("delete a month", "DELETE FROM books_cash_basis_balances WHERE account = {:a} AND period = '2026-03'", dbx.Params{"a": ce.software.Id})
	exec("move a month's first day", "UPDATE books_cash_basis_balances SET month_start = '2026-04-02' WHERE account = {:a} AND period = '2026-04'", dbx.Params{"a": ce.payable.Id})
	// A stored month with no cash row, its totals zero: a report that counts
	// months would show the account as active.
	saveRecord(t, ce.app, cashBalancesCollection, map[string]any{
		"account": ce.software.Id, "period": "2026-04", "month_start": "2026-04-01", "currency": "USD",
	})

	drifts, err := verifyCashBalances(ce.app)
	if err != nil {
		t.Fatalf("verifyCashBalances: %v", err)
	}
	got := map[balanceKey]BalanceDrift{}
	for _, d := range drifts {
		got[balanceKey{d.Account, d.Period}] = d
	}
	want := map[balanceKey]BalanceDrift{
		{ce.office.Id, "2026-05"}:   {ce.office.Id, "2026-05", 29846, 0, 29845, 0, "2026-05-01"},
		{ce.software.Id, "2026-03"}: {ce.software.Id, "2026-03", 0, 0, 0, 30000, ""},
		{ce.payable.Id, "2026-04"}:  {ce.payable.Id, "2026-04", 12345, 0, 12345, 0, "2026-04-02"},
		{ce.software.Id, "2026-04"}: {ce.software.Id, "2026-04", 0, 0, 0, 0, "2026-04-01"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verifyCashBalances = %+v, want %+v", got, want)
	}

	fixed, err := rebuildBalances(ce.app)
	if err != nil {
		t.Fatalf("rebuildBalances: %v", err)
	}
	if fixed != 4 {
		t.Errorf("rebuildBalances fixed %d rows, want the 4 cash months", fixed)
	}
	requireCashMonthsEqualRows(t, ce.app)
	if drifts, err := verifyCashBalances(ce.app); err != nil || len(drifts) != 0 {
		t.Errorf("verifyCashBalances after the rebuild = %+v, err %v; want none", drifts, err)
	}
}

// A cash row deleted past the hooks is a drift of its month until the
// rebuild, which repairs the cash rows first and the months after.
func TestCashBalances_ARowDeletedPastTheHooksIsADrift(t *testing.T) {
	ce := cashBook(t)
	if _, err := ce.app.DB().NewQuery("DELETE FROM books_cash_basis_lines WHERE bill = {:b}").
		Bind(dbx.Params{"b": ce.bill.Id}).Execute(); err != nil {
		t.Fatalf("delete the bill's cash-basis rows: %v", err)
	}
	drifts, err := verifyCashBalances(ce.app)
	if err != nil || len(drifts) == 0 {
		t.Fatalf("verifyCashBalances after cash rows were deleted = %+v, err %v; want drifts", drifts, err)
	}
	if _, err := rebuildBalances(ce.app); err != nil {
		t.Fatalf("rebuildBalances: %v", err)
	}
	requireCashMonthsEqualRows(t, ce.app)
	requireNoDrift(t, ce.app)
}
```

In `books/server/cash_basis_test.go`, `requireNoCashDrift` gains one line, so every existing cash-basis test (posts, payments, voids, payment corrections, the invoice correction, the bill correction, credit memos, vendor credits, refunds, write-offs) also compares the stored months with the cash rows:

```go
	if len(drifts) != 0 {
		t.Errorf("the cash-basis rows drift from a full rebuild: %+v", drifts)
	}
	requireCashMonthsEqualRows(t, app)
}
```

In `books/server/env_test.go`, `requireNoDrift` gains the verify, after the receivables block (also change its comment's first sentence to end "…, a stored receivable row disagrees with the book, or a stored cash month disagrees with the cash-basis rows."):

```go
	if len(receivables) > 0 {
		t.Fatalf("receivable rows drifted from the book: %+v", receivables)
	}
	cashMonths, err := verifyCashBalances(app)
	if err != nil {
		t.Fatalf("verify cash months: %v", err)
	}
	if len(cashMonths) > 0 {
		t.Fatalf("stored cash months drifted from the cash-basis rows: %+v", cashMonths)
	}
}
```

In `books/server/registrations_test.go`, the `serverOnly` map (line 31) gains the collection, in name order:

```go
	"books_cash_basis_balances":     true,
	"books_cash_basis_lines":        true,
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 -run 'CashBalances' .`
Expected: FAIL at build: `undefined: cashBalancesCollection`, `undefined: verifyCashBalances`.

- [ ] **Step 3: Add the collection to the migration**

In `books/pb-migrations/2100000021_create_books_bill_payments.js`, insert this block between the `app.save(...)` of `books_cash_basis_lines` and the `app.save(...)` of `books_cash_basis_tax_lines` (`relation`, `READ`, `MAX_STORED`, `AUTODATES` and `accounts` exist in the file):

```js
        // The stored cash months: for each account and month, the totals of the
        // posted books_cash_basis_lines rows (server/cash_balances.go). The server
        // keeps them with the rows. Readers list them; no client writes them.
        app.save(
            new Collection({
                id: 'pbc_books_cash_months',
                name: 'books_cash_basis_balances',
                type: 'base',
                listRule: READ,
                viewRule: READ,
                createRule: null,
                updateRule: null,
                deleteRule: null,
                fields: [
                    relation('account', accounts.id, { required: true }),
                    { name: 'period', type: 'text', required: true, max: 7, pattern: '^\\d{4}-\\d{2}$' },
                    // The first day of the month, so a report compares the row with a day.
                    { name: 'month_start', type: 'text', required: true, max: 10, pattern: '^\\d{4}-\\d{2}-01$' },
                    { name: 'debit_total', type: 'number', onlyInt: true, min: 0, max: MAX_STORED },
                    { name: 'credit_total', type: 'number', onlyInt: true, min: 0, max: MAX_STORED },
                    { name: 'currency', type: 'text', required: true, pattern: '^[A-Z]{3}$' },
                    ...AUTODATES,
                ],
                indexes: [
                    'CREATE UNIQUE INDEX idx_books_cash_balances_account_period ON books_cash_basis_balances (account, period)',
                ],
            })
        )
```

In the same file's `down` function, the list of names gains the table first:

```js
        for (const name of [
            'books_cash_basis_balances',
            'books_cash_basis_tax_lines',
```

Also extend the file's header comment, section `CASH BASIS`, with one sentence: `books_cash_basis_balances holds the totals of the cash-basis rows per account and month (server/cash_balances.go).`

In `books/server/oauth_scopes.go` (line 89), a token reads the table as it reads the rows:

```go
			"books_cash_basis_lines":           ro,
			"books_cash_basis_balances":        ro,
```

- [ ] **Step 4: Write `books/server/cash_balances.go`**

```go
package books

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// The stored cash months. books_cash_basis_balances holds, for each account
// and month, the debit and credit totals of the posted books_cash_basis_lines
// rows of that month, as books_account_balances holds the ledger's. A Cash
// report reads the months before its period here, so that part of its cost
// does not grow with the cash rows. syncCashLineRows, the one writer of the
// cash rows, refreshes the months of the rows it deletes and writes, in the
// same transaction; rebuildBalances repairs every month (rebuildCashBalances);
// verifyCashBalances is the check. A month with no cash row has no stored row.

const cashBalancesCollection = "books_cash_basis_balances"

// cashMonthSum is the posted cash rows of one account and month, added up.
type cashMonthSum struct {
	Rows     int    `db:"n"`
	Debit    int    `db:"debit"`
	Credit   int    `db:"credit"`
	Currency string `db:"currency"`
}

// cashMonthTotals sums every posted cash-basis row by account and month: what
// the stored cash months must equal.
func cashMonthTotals(app core.App) (map[balanceKey]balanceTotals, error) {
	var rows []struct {
		Account  string `db:"account"`
		Period   string `db:"period"`
		Debit    int    `db:"debit"`
		Credit   int    `db:"credit"`
		Currency string `db:"currency"`
	}
	err := app.DB().NewQuery(`
		SELECT account, substr(date, 1, 7) AS period,
		       CAST(SUM(debit) AS INTEGER) AS debit,
		       CAST(SUM(credit) AS INTEGER) AS credit,
		       MAX(currency) AS currency
		FROM books_cash_basis_lines
		WHERE posted = 1
		GROUP BY account, substr(date, 1, 7)`).All(&rows)
	if err != nil {
		return nil, fmt.Errorf("books: sum the cash-basis rows: %w", err)
	}
	out := make(map[balanceKey]balanceTotals, len(rows))
	for _, r := range rows {
		out[balanceKey{r.Account, r.Period}] = balanceTotals{r.Debit, r.Credit, r.Currency}
	}
	return out, nil
}

// refreshCashBalance makes the stored cash month of key equal the posted cash
// rows of that account and month: it writes the row, or deletes it when the
// month has no cash row left. It reads the rows again instead of adding a
// difference, so a month that had drifted is right after its next write.
func refreshCashBalance(tx core.App, key balanceKey) error {
	var sum cashMonthSum
	err := tx.DB().NewQuery(`
		SELECT COUNT(*) AS n,
		       COALESCE(CAST(SUM(debit) AS INTEGER), 0) AS debit,
		       COALESCE(CAST(SUM(credit) AS INTEGER), 0) AS credit,
		       COALESCE(MAX(currency), '') AS currency
		FROM books_cash_basis_lines
		WHERE account = {:account} AND posted = 1 AND date >= {:from} AND date <= {:to}`).
		Bind(dbx.Params{"account": key.account, "from": firstDayOf(key.period), "to": key.period + "-31"}).One(&sum)
	if err != nil {
		return fmt.Errorf("books: sum the cash-basis rows of %s %s: %w", key.account, key.period, err)
	}
	row, err := tx.FindFirstRecordByFilter(cashBalancesCollection,
		"account = {:account} && period = {:period}", dbx.Params{"account": key.account, "period": key.period})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("books: read cash month %s %s: %w", key.account, key.period, err)
	}
	if sum.Rows == 0 {
		if row == nil {
			return nil
		}
		if err := tx.Delete(row); err != nil {
			return fmt.Errorf("books: delete cash month %s %s: %w", key.account, key.period, err)
		}
		return nil
	}
	if sum.Debit > maxStoredAmount || sum.Credit > maxStoredAmount {
		return balanceOverflow(tx, key.account, key.period)
	}
	if row == nil {
		col, err := tx.FindCachedCollectionByNameOrId(cashBalancesCollection)
		if err != nil {
			return err
		}
		row = core.NewRecord(col)
		row.Set("account", key.account)
		row.Set("period", key.period)
	}
	// A cash row copies its journal line's currency; one written with none
	// takes its account's, because the stored month must name one.
	if sum.Currency == "" {
		account, err := tx.FindRecordById("books_accounts", key.account)
		if err != nil {
			return fmt.Errorf("books: read the account of cash month %s %s: %w", key.account, key.period, err)
		}
		sum.Currency = account.GetString("currency")
	}
	row.Set("month_start", firstDayOf(key.period))
	row.Set("debit_total", sum.Debit)
	row.Set("credit_total", sum.Credit)
	row.Set("currency", sum.Currency)
	if err := tx.Save(row); err != nil {
		return fmt.Errorf("books: save cash month %s %s: %w", key.account, key.period, err)
	}
	return nil
}

// refreshCashBalances refreshes each month of months, in account then month
// order, so two writers take the rows in the same order.
func refreshCashBalances(tx core.App, months map[balanceKey]bool) error {
	keys := make([]balanceKey, 0, len(months))
	for k := range months {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].account != keys[j].account {
			return keys[i].account < keys[j].account
		}
		return keys[i].period < keys[j].period
	})
	for _, k := range keys {
		if err := refreshCashBalance(tx, k); err != nil {
			return err
		}
	}
	return nil
}

// verifyCashBalances reports every stored cash month that disagrees with the
// posted cash rows of its account and month: other totals, a month_start that
// is not the month's first day, a stored row whose month has no cash row, and
// a month with cash rows and no stored row (its stored fields are zero). The
// drift's Ledger fields hold the cash rows' sums.
func verifyCashBalances(app core.App) ([]BalanceDrift, error) {
	want, err := cashMonthTotals(app)
	if err != nil {
		return nil, err
	}
	stored, err := app.FindAllRecords(cashBalancesCollection)
	if err != nil {
		return nil, fmt.Errorf("books: read the cash months: %w", err)
	}
	var drifts []BalanceDrift
	seen := map[balanceKey]bool{}
	for _, row := range stored {
		key := balanceKey{row.GetString("account"), row.GetString("period")}
		seen[key] = true
		w, hasRows := want[key]
		if !hasRows || row.GetInt("debit_total") != w.debit || row.GetInt("credit_total") != w.credit ||
			row.GetString("month_start") != firstDayOf(key.period) {
			drifts = append(drifts, BalanceDrift{key.account, key.period,
				row.GetInt("debit_total"), row.GetInt("credit_total"), w.debit, w.credit, row.GetString("month_start")})
		}
	}
	for key, w := range want {
		if !seen[key] {
			drifts = append(drifts, BalanceDrift{key.account, key.period, 0, 0, w.debit, w.credit, ""})
		}
	}
	sort.Slice(drifts, func(i, j int) bool {
		if drifts[i].Account != drifts[j].Account {
			return drifts[i].Account < drifts[j].Account
		}
		return drifts[i].Period < drifts[j].Period
	})
	return drifts, nil
}

// rebuildCashBalances makes every stored cash month equal its cash rows,
// touching only the months that differ, and returns how many it fixed. It
// runs after rebuildCashBasis, which puts the cash rows right first.
func rebuildCashBalances(tx core.App) (int, error) {
	drifts, err := verifyCashBalances(tx)
	if err != nil {
		return 0, err
	}
	for _, d := range drifts {
		if err := refreshCashBalance(tx, balanceKey{d.Account, d.Period}); err != nil {
			return 0, err
		}
	}
	return len(drifts), nil
}
```

- [ ] **Step 5: The writer and the rebuild call it**

In `books/server/cash_basis.go`, replace the whole of `syncCashLineRows` (its comment too) with:

```go
// syncCashLineRows makes the document's stored books_cash_basis_lines rows
// equal the derived ones, and refreshes the stored cash month
// (cash_balances.go) of every row it deletes or writes.
func syncCashLineRows(tx core.App, d cashDeferral, docID string) (int, error) {
	stray, missing, err := cashRowDiff(tx, d, docID)
	if err != nil {
		return 0, err
	}
	months := map[balanceKey]bool{}
	for _, r := range stray {
		months[balanceKey{r.GetString("account"), periodOf(r.GetString("date"))}] = true
		if err := tx.Delete(r); err != nil {
			return 0, fmt.Errorf("books: delete a cash-basis row of %s %s: %w", d.Kind, docID, err)
		}
	}
	if len(missing) == 0 {
		return len(stray), refreshCashBalances(tx, months)
	}
	col, err := tx.FindCachedCollectionByNameOrId(cashLinesCollection)
	if err != nil {
		return 0, err
	}
	for _, w := range missing {
		months[balanceKey{w.Account, periodOf(w.Date)}] = true
		r := core.NewRecord(col)
		r.Set("document_kind", d.Kind)
		r.Set(d.Kind, docID)
		r.Set("entry", w.Entry)
		r.Set("payment_entry", w.PaymentEntry)
		r.Set("account", w.Account)
		r.Set("vendor", w.Vendor)
		r.Set("customer", w.Customer)
		r.Set("debit", w.Debit)
		r.Set("credit", w.Credit)
		r.Set("date", w.Date)
		r.Set("posted", true)
		r.Set("memo", w.Memo)
		r.Set("currency", w.Currency)
		r.Set("position", w.Position)
		if err := tx.Save(r); err != nil {
			return 0, fmt.Errorf("books: write a cash-basis row of %s %s: %w", d.Kind, docID, err)
		}
	}
	if err := refreshCashBalances(tx, months); err != nil {
		return 0, err
	}
	return len(stray) + len(missing), nil
}
```

Also add one sentence to the file's header comment, after "Nothing else writes the rows.": `Each write of a row refreshes the stored cash month of its account (cash_balances.go).`

In `books/server/balances.go`, `rebuildBalances`: after the `rebuildCashBasis` block and before `drifts, err := verifyBalances(tx)`, add the months; then count them:

```go
		cash, err := rebuildCashBasis(tx)
		if err != nil {
			return err
		}
		cashMonths, err := rebuildCashBalances(tx)
		if err != nil {
			return err
		}
		drifts, err := verifyBalances(tx)
```

```go
		fixed = len(drifts) + documents + cash + cashMonths
```

In the comment above `rebuildBalances`, change "and every cash-basis row equal what the entries and payments give (rebuildCashBasis; its verify part is verifyCashBasis)" to "every cash-basis row equal what the entries and payments give (rebuildCashBasis; its verify part is verifyCashBasis), and every stored cash month equal its cash rows (rebuildCashBalances; its verify part is verifyCashBalances)", and "month rows, document balances and cash-basis rows it fixed" to "month rows, document balances, cash-basis rows and cash months it fixed".

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -count=1 -run 'CashBalances|CashBasis|RebuildBalances|TestRegister|OAuth|Rebuild' .`
Expected: `go vet` prints nothing; `ok  tinycld.org/packages/books`. With `-v -run CashBalances`, three tests PASS: `TestCashBalances_FollowThePostsPaymentsAndVoids`, `TestVerifyCashBalances_ReportsEachDriftAndTheRebuildRepairsIt`, `TestCashBalances_ARowDeletedPastTheHooksIsADrift`.

- [ ] **Step 7: Run the Books Go package once, and biome on the migration**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -count=1 .`
Expected: `ok  tinycld.org/packages/books`. A failure in a test that calls `requireNoDrift` or `requireNoCashDrift` names a write path that changes cash rows and does not pass through `syncCashLineRows`: find that path and make it call `refreshCashBalances` in its transaction; do not weaken the helper.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check pb-migrations/2100000021_create_books_bill_payments.js`
Expected: no error.

- [ ] **Step 8: Hand the 100k run to the driver**

The scale book and the report specs are not changed by this task, so the figures must stay as after task 9e-1; the run is the record that they do. Hand back:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9d2a go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
```

Needed from it: PASS; `st-results.md` (BS 1, BS 2, the TB and TD medians, no run refused).

- [ ] **Step 9: Write the ledger record**

Read the end of `books/docs/plans/general-ledger-ledger.md` (the record `#### Task 9e-1: …`, line 745 at books `63cdf3cb`) and append after it, in the same form, `#### Task 9d-2a: the stored cash months (books `server/cash_balances.go`)` with: the table and its key; the one writer (`syncCashLineRows`) and that it reads the month's rows again (no difference is added); a month with no cash row has no row; verify and rebuild (`verifyCashBalances`, `rebuildCashBalances` inside `rebuildBalances`, after `rebuildCashBasis`); `requireNoDrift` and `requireNoCashDrift` check it; the migration is an in-place edit of `2100000021` and why; no report reads the table yet (task 9d-2b); the 100k figures from step 8 (`scale-task9d2a/st-results.md`) beside those of task 9e-1.

- [ ] **Step 10: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add pb-migrations/2100000021_create_books_bill_payments.js server/cash_balances.go server/cash_balances_test.go server/cash_basis.go server/cash_basis_test.go server/balances.go server/oauth_scopes.go server/env_test.go server/registrations_test.go docs/plans/general-ledger-ledger.md
git commit -m "feat(ledger): stored cash-basis months, kept current with the cash rows"
```
