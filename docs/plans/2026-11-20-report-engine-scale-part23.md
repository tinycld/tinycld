# Report engine scale, part 23: the Cash Balance Sheet reads the stored months and only the cash rows (task 9d-1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Balance Sheet on Cash reads the history before the as-of month from the stored months (the ledger part) and from the cash-basis rows alone (the cash part), so it reads no journal line of an earlier month.

**Architecture:** The cash basis is the ledger plus the cash-basis rows (`books.basis_lines`: layer `ledger` + layer `cash`). The stored months (`books_account_balances`) are the sums of the ledger lines, so they are correct on both methods. The spec stops holding the stored-month lookups to Accrual, and every Cash lookup of the history before a month reads only layer `cash`. The as-of month is read from its lines on both methods, as now. No table, no migration, no core change.

**Tech Stack:** Go (`books/server`, package `books`), core report engine (`tinycld.org/core/reports`), Go test `TestStatementsScale` (run by the driver only).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (section "State after plan part 23") and `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-20-report-engine-scale-part21.md` (its header and the Interfaces block of its task). Bar (human, 2026-11-20): the Balance Sheet as of a day, Accrual and Cash, 2 s or less at 500,000 lines.

**This part holds task 9d-1 only.** Tasks 9d-2, 9e-1 and 9e-2 are in the outline; they are not written.

## Evidence (100,000 lines, the statements book; probe files in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part23-scratch`)

- Each Balance Sheet run is one long statement (statement 5). Compile is the time of `EXPLAIN <statement>`; run is the rest.
- BS 1 Accrual: 953 ms = 266 ms compile + 687 ms run. BS 2 Cash: 4363 ms = 263 ms compile + 4100 ms run (`probe-before.txt`).
- The Cash run time is the lookups of the lines before a month (`bsBefore`, `bsBalanceAt`, `bsActivity`, `profitAndLossSum`): each is a SCAN of `books_journal_lines` and of `books_cash_basis_lines` (`scale-task9c2/st-plans.md`, 16 SCAN pairs). On Accrual the same lookups hold `method = cash`, which is false, and read nothing.
- With the change of this task, built in an overlay (`spec.diff`): BS 2 Cash statement 1832 ms (278 ms compile + 1554 ms run), BS 1 Accrual 914 ms (`probe-after.txt`). The machine was not quiet at the end of that run (1-minute load 9.4): the numbers are an indication; step 7 gives the record.

## Global Constraints

- Trees: core `04ce7268` plus this plan; books `91470dde`. Branch `books/p5-report-engine-scale` in both. Start only when `git -C /Users/nas/tmp/tinycld/books-purchases/books status --short` prints nothing.
- This task changes Books only: four Go files in `books/server` and the ledger file. It changes no core file, no migration, no TypeScript.
- The figures of the Balance Sheet do not change on either method. The cross-check tests (`TestBalanceSheetStoredMonthsEqualTheLines`, `TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows`) and `TestCashBasisReport_*` stay as they are and stay green.
- The implementer starts no scale run: it is a `NEEDS-RUN` for the driver (step 7).
- A timing run starts only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4.
- Rules of the lane: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md`. Change files with the Edit tool. Read a file with the Read tool right before its first Edit.
- No bar is held at 100,000 lines. The bar is at 500,000 lines: 2 s for both methods.

## Review Focus

1. A book with cash-basis rows before the as-of month (an unpaid bill of an earlier month, a payment of a later month): the Cash figures of every account, Retained earnings and Net income must equal a run that adds every line. Test: `TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows` (unchanged; seven days, both methods).
2. A stored month that drifted from the ledger now changes the Cash figures too (before this task only the Accrual figures). Test: step 1 (`TestBalanceSheetReadsTheStoredMonths`, the Cash assertion).
3. An account with cash-basis rows and no ledger line before the as-of month (Rows: Active): it must still show. `bsActivity` counts the cash rows before the month on Cash. Test: the cross-check test above runs `rows: active` on Cash.
4. A cash-basis row dated in the as-of month must count once: it is read by the month's lines (`basisFilter`), never by the before-month lookup (`date < startOfMonth`). Test: the cross-check test above, days `2026-03-10` and `2026-04-15` (the bill and its payment are of those months).
5. The Prior year day on Cash (`bsBalanceAt(dPrior)`): its before-month part reads the stored months and the cash rows before the prior day's month. Test: the cross-check test above, days `2027-03-20` to `2027-04-20`.

---

## File Structure

| File | Change |
|---|---|
| `books/server/report_balance_sheet.go` | The stored-month lookups run on both methods; the Cash lookups of the history read layer `cash` only; `profitAndLossSum` has three parts; `onAccrual` is removed; `onCashLayer` is new; `Revision: 5` |
| `books/server/report_balance_sheet_stored_test.go` | `TestBalanceSheetReadsTheStoredMonths`: Cash follows the stored months too |
| `books/server/report_balance_sheet_test.go` | The pinned revision is 5 (lines 246-247 and 1167) |
| `books/server/report_cash_basis_test.go` | The pinned revision is 5 (line 327) |
| `books/docs/plans/general-ledger-ledger.md` | Append the record "Task 9d-1" |

### Task 1 (9d-1): Books: the Cash Balance Sheet reads the stored months and only the cash rows

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet.go` (lines 43-49 and 348-439)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet_stored_test.go` (lines 243-280)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet_test.go` (lines 246-247 and 1167)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_cash_basis_test.go` (line 327)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (append at the end)

**Interfaces:**
- Consumes (they exist): `glSum(source string, value *reports.Node, where ...*reports.Node) *reports.Node` (`report_general_ledger.go:219`), `basisFilter()` (`report_spec_helpers.go:321`), `tbStoredCount(where ...*reports.Node)` (`report_trial_balance.go:110`), `tdAccount()`, `op`, `fld`, `lit`, `prm`, `link`, `mustJSON`.
- Produces: `onCashLayer() *reports.Node` (true for a cash-basis row of `books.basis_lines`); `onAccrual` no longer exists; `bsStoredBefore(x)` has no method term; `bsBefore`, `bsBalanceAt`, `bsActivity`, `profitAndLossSum` keep their signatures; `balanceSheetShipped().Revision == 5`.

- [ ] **Step 1: Write the failing test**

In `report_balance_sheet_stored_test.go`, replace the comment above `TestBalanceSheetReadsTheStoredMonths` (lines 243-244) with:

```go
// The months before the as-of day's month are read from the stored months and
// that month from its lines, on both methods: the cash basis is the ledger and
// the cash-basis rows, and the stored months are the ledger's.
```

In the same test, replace its last `if` (lines 276-279) with:

```go
	if c, p, r := figures("cash"); c != cashCurrent+10 || p != cashPrior+5 || r != cashRetained-5 {
		t.Errorf("Cash after three earlier stored months rose by 5: Operating %d / %d, Retained earnings %d; want %d / %d, %d: Cash reads the stored months for the ledger part",
			c, p, r, cashCurrent+10, cashPrior+5, cashRetained-5)
	}
```

The shipped revision is pinned in three places. Change each:

- `report_balance_sheet_test.go` lines 246-247 (`TestBalanceSheetSpecLoads`):

```go
	if shipped.Revision != 5 || !shipped.Unpublished {
		t.Errorf("revision %d, unpublished %v; want revision 5 (the stored months on Cash too), shipped unpublished", shipped.Revision, shipped.Unpublished)
```

- `report_balance_sheet_test.go` line 1167 (`TestBalanceSheetShippedUnpublished`): `row.GetInt("shipped_revision") != 4` becomes `row.GetInt("shipped_revision") != 5`.
- `report_cash_basis_test.go` line 327 (`TestCashBasisReport_ShippedSpecs`): `{balanceSheetShipped(), 4, "asOf"},` becomes `{balanceSheetShipped(), 5, "asOf"},`, and the comment of line 326 becomes `// 3 for the Method param, 4 for the stored months, 5 for the stored months on Cash.`

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'TestBalanceSheetReadsTheStoredMonths$|TestBalanceSheetSpecLoads$|TestBalanceSheetShippedUnpublished$|TestCashBasisReport_ShippedSpecs$' -count=1 .`
Expected: FAIL, four tests. `TestBalanceSheetReadsTheStoredMonths` prints "Cash after three earlier stored months rose by 5: Operating 110300 / 133000, Retained earnings 110000; want 110310 / 133005, 109995: Cash reads the stored months for the ledger part". `TestBalanceSheetSpecLoads` prints "revision 4, unpublished true; want revision 5 …". The other two fail on revision 4.

- [ ] **Step 3: Change the spec**

In `report_balance_sheet.go` (line numbers are of the file before this step; find each function by name):

(a) In `balanceSheetShipped`, add a comment line after the comment of revision 4 and set the revision:

```go
		// 5: on Cash too the months before a day's month come from the stored
		// months; only the cash-basis rows of those months are read as lines.
		Revision:    5,
```

(b) Replace the comment and the body of `profitAndLossSum` (lines 348-365) with:

```go
// profitAndLossSum adds credits − debits of the posted lines on or before x
// that match which, on the run's method; 0 when none does. which names the
// day it reads: a line's date, or a stored month's first day (a fiscal year
// starts on the first day of a month, so every day of a month is in the
// fiscal year of that first day). The ledger lines of the months before x's
// month come from the stored months on both methods; on Cash the cash-basis
// rows of those months are added; x's month comes from its lines.
func profitAndLossSum(x *reports.Node, which func(day string) *reports.Node) *reports.Node {
	monthStart := op("startOfMonth", x)
	net := op("sub", fld("credit"), fld("debit"))
	return op("add",
		glSum("books.account_balances", op("sub", fld("credit_total"), fld("debit_total")),
			op("lt", fld("month_start"), monthStart), which("month_start")),
		glSum("books.basis_lines", net,
			onCash(), onCashLayer(), op("eq", fld("posted"), lit(true)),
			op("lt", fld("date"), monthStart), which("date")),
		glSum("books.basis_lines", net,
			basisFilter(), op("eq", fld("posted"), lit(true)), op("lte", fld("date"), x),
			op("gte", fld("date"), monthStart), which("date")),
	)
}
```

(c) Replace `onCash`, `onAccrual` and their comment (lines 378-382) with:

```go
// onCash is true on a Cash run. A lookup that holds it reads no row on
// Accrual.
func onCash() *reports.Node { return op("eq", prm("method"), lit("cash")) }

// onCashLayer is true for a cash-basis row of books.basis_lines. A lookup
// that holds it reads no ledger line.
func onCashLayer() *reports.Node { return op("eq", fld("layer"), lit("cash")) }
```

(d) Replace `bsStoredBefore` and `bsBefore` with their comments (lines 394-414) with:

```go
// bsStoredBefore are the stored months of the row's account before the month
// of day x. They hold the ledger lines, which both methods read.
func bsStoredBefore(x *reports.Node) []*reports.Node {
	return []*reports.Node{
		op("eq", link("account"), tdAccount()),
		op("lt", fld("month_start"), op("startOfMonth", x)),
	}
}

// bsBefore is the row's account's balance at the start of the month of day x:
// the stored months (books_account_balances), so its cost does not grow with
// the account's ledger lines, and on Cash the account's cash-basis rows before
// that month, which no stored month holds.
func bsBefore(x *reports.Node) *reports.Node {
	return op("add",
		glSum("books.account_balances", bsSigned("debit_total", "credit_total"), bsStoredBefore(x)...),
		glSum("books.basis_lines", bsSigned("debit", "credit"),
			bsAccountLines(onCash(), onCashLayer(), op("lt", fld("date"), op("startOfMonth", x)))...),
	)
}
```

(e) In `bsActivity`, replace the `Where` line of `earlier` with:

```go
		Where:  op("and", bsAccountLines(onCash(), onCashLayer(), op("lt", fld("date"), op("startOfMonth", x)))...),
```

and replace the comment of `bsActivity` (lines 424-426) with:

```go
// bsActivity is above 0 when the row's account has a posted line on or before
// day x, the day of the report's rows: a stored month exists only for a month
// with a posted ledger line; on Cash the cash-basis rows before x's month
// count too.
```

`bsBalanceAt`, `bsAccountLines` and `bsSigned` do not change. The whole file after the change is `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part23-scratch/report_balance_sheet.go.txt` (it has the code of (b) to (e) and `Revision: 5`, without the new comments): compare, do not copy it over the file.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -run 'TestBalanceSheet|TestCashBasisReport|TestStatementsScaleBook|TestStatementsScaleBars' -count=1 .`
Expected: `go vet` prints nothing (a remaining `onAccrual` is a compile error: remove the use); the test line is `ok  tinycld.org/packages/books`. This takes about 75 s. If `TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows` fails, the diff it prints names the method, the day and the row: a Cash row that differs means a lookup reads a cash-basis row twice or not at all (Review Focus 4).

- [ ] **Step 5: Run the package's report tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'Report|Shipped|TrialBalance|GeneralLedger|ProfitAndLoss|TransactionDetail' -count=1 .`
Expected: `ok`. This run proves the other reports still build and pass.

- [ ] **Step 6: Commit (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add server/report_balance_sheet.go server/report_balance_sheet_stored_test.go server/report_balance_sheet_test.go server/report_cash_basis_test.go && git commit -m "perf(reports): the Cash Balance Sheet reads the stored months and only the cash rows"
```

- [ ] **Step 7: Hand the 100k run to the driver**

Hand back this line and stop until the driver gives the two files:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9d1 go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
```

Needed from it: `scale-task9d1/st-results.md` (the rows "BS 1 accrual, as of" and "BS 2 cash, as of") and `scale-task9d1/st-plans.md` (the sections of those two runs). Expected: BS 2 Cash about 2 s (it was 4.551 s), BS 1 Accrual about 1 s (it was 972 ms). The run fails no bar at 100k. If BS 2 is not under 3 s, hand back `STOPPED: the Cash Balance Sheet took <time> at 100k` with the two sections of `st-plans.md`.

- [ ] **Step 8: Record the result in the ledger (Books)**

Read the end of `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (the record "Task 9c-2" at line 590 is the pattern), then append with the Edit tool. Write the heading and the first paragraph as they are here; fill the table from `scale-task9d1/st-results.md`, and the last paragraph from `scale-task9d1/st-plans.md`:

```markdown
#### Task 9d-1: the Cash Balance Sheet reads the stored months (books `server/report_balance_sheet.go`, 2026-11-20)

The cash basis is the ledger and the cash-basis rows. The stored months are the ledger's, so a Cash run reads them too; of the months before a day's month it reads as lines only the cash-basis rows (layer `cash`). Before this task a Cash run added every journal line of the book for each lookup (4.551 s at 100,000 lines: 0.26 s compile, the rest SCANs of both line tables).

| lines | run | before (task 9c-2) | after |
|---|---|---|---|
| 100000 | BS 1 accrual, as of | 972ms | <median of the run> |
| 100000 | BS 2 cash, as of | 4.551s | <median of the run> |

<One paragraph: the count of "SCAN books_journal_lines" and of "SCAN books_cash_basis_lines" in the BS 2 section of st-plans.md, and the statement's size in chars. The bar is 2 s at 500,000 lines for both methods; tasks 9d-2, 9e-1 and 9e-2 of the plan outline are what is left for it.>
```

The three `<…>` parts are values the run gives: write the values, not the brackets.

- [ ] **Step 9: Commit the record (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add docs/plans/general-ledger-ledger.md && git commit -m "docs: the Cash Balance Sheet at 100k after it reads the stored months"
```
