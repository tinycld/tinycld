# Report engine scale, part 24: the Balance Sheet drill reads its first day from the stored months (task 9e-1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Balance Sheet figure's drill gets the first day of its period from the account's stored months, not from a `min(date)` over the account's lines, on Accrual and on Cash.

**Architecture:** The drill of an account row opens Transaction Detail by Account for a period `From..d`. `From` only has to be on or before the account's first line: no earlier line exists, so an earlier `From` lists the same lines. `From` becomes the first day of the account's first stored month (`books_account_balances`: one row for each month with a posted ledger line) that starts on or before `d`. On Cash the first cash-basis row of the account (layer `cash` of `books.basis_lines`) is read too, cut to the first day of its month, because no stored month holds a cash-basis row; the earlier of the two days is used. On Accrual that second lookup holds `method = cash` and reads no row. No table, no migration, no core change.

**Tech Stack:** Go (`books/server`, package `books`), core report engine (`tinycld.org/core/reports`), Playwright (one e2e pin), Go test `TestStatementsScale` (run by the driver only).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (section "State after plan part 23", item 9e-1) and `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-20-report-engine-scale-part23.md` (its header). Human decision (2026-11-20): the drilled period may start on the 1st of the first line's month; it lists the same lines, because no earlier line exists. Bar (human, 2026-11-20): the Balance Sheet as of a day, Accrual and Cash, 2 s or less at 500,000 lines.

**This part holds task 9e-1 only.** Tasks 9d-2 and 9e-2 are in the outline; they are not written.

## Evidence

- At 100,000 lines the two drills of the Accrual run (`balanceDrill` → `firstBasisLine`, a `min(date)` over the account's lines for the as-of day and for the prior-year day) cost about 0.5 s of the 0.69 s run (`part23-scratch/probe-before.txt`; outline, "State after plan part 23").
- Record before this task (`scale-task9d1/st-results.md`, 100,000 lines): BS 1 accrual 940 ms, BS 2 cash 1.965 s.
- The code of this plan was built once against books `627e24c7` through a Go overlay (`/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part24-scratch`: the five Go files after the change, `red.log`, `green.log`, `green-wide.log`). No timing was taken: step 9 gives the record.
- Core's `min` operator takes numbers only (`Validate: … min needs numbers`), so the earlier of two days is written with `if` and `lte`.

## Global Constraints

- Trees: core `25d6ecac` plus this plan; books `5b4f3bff` (or later, when it only adds ledger docs). Branch `books/p5-report-engine-scale` in both. Start only when `git -C /Users/nas/tmp/tinycld/books-purchases/books status --short` prints nothing.
- This task changes Books only: five Go files in `books/server`, one e2e spec and the ledger file. It changes no core file, no migration, no app TypeScript.
- The figures of the Balance Sheet do not change. Only the `from` day of a drill changes: from the first line's day to the 1st of that line's month. The `to` day, the method and the account of a drill do not change.
- The Trial Balance drill (`firstLine`, `firstLineFY`) does not change in this task.
- The implementer starts no e2e run and no scale run: each is a `NEEDS-RUN` for the driver (steps 7 and 9). No timing probe.
- A timing run starts only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4.
- Rules of the lane: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md`. Change files with the Edit tool. Read a file with the Read tool right before its first Edit.
- No bar is held at 100,000 lines. The bar is at 500,000 lines: 2 s for both methods.

## Review Focus

1. An account whose first line is not on the 1st of its month: the drill starts on the 1st and lists exactly the lines of a range from the first line's own day; the drilled total is the figure. Test: step 1, `TestDrillBalanceSheet` (Operating, first line June 10; Current and Prior year).
2. On Cash, the same, with cash-basis rows among the lines. Test: step 1, `TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth` (Accounts Payable, six lines; Business Checking, one line).
3. On Cash, an account whose cash-basis rows start in a month with no stored month of its own (a payment's share dated before the document's line): the range must still start on or before that row. Test: step 1, the same test, its last part (the stored months of Accounts Payable deleted; the range still starts on March 1).
4. A day `d` before the account's first line, in the same month (the month is stored, no line is on or before `d`): the range is the 1st of the month to `d`, an empty run, where it was `d` to `d`. A reader sees a period of some days with no line, not a wrong line. Test: step 1, `TestDrillBalanceSheet` (Reserve, Prior year).
5. An account with no stored month and no cash-basis row on or before `d`: the range stays `d` to `d`. Test: `TestDrillBalanceSheet` (Savings, Prior year; unchanged assertion).

---

## File Structure

| File | Change |
|---|---|
| `books/server/report_spec_helpers.go` | `firstBasisLine` is replaced by `firstBasisMonth` |
| `books/server/report_balance_sheet.go` | `balanceDrill` uses `firstBasisMonth`; its comment; `Revision: 6` |
| `books/server/report_transaction_detail_test.go` | `tdLineIDs`, `tdFrom`; `TestDrillBalanceSheet` pins the 1st of the month and cross-checks the lines |
| `books/server/report_cash_basis_test.go` | New `TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth`; the pinned revision is 6 |
| `books/server/report_balance_sheet_test.go` | The pinned revision is 6 (two places) |
| `books/tests/e2e/report-drill-down.spec.ts` | The Balance Sheet drill's period is this month to date, not today alone |
| `books/docs/plans/general-ledger-ledger.md` | Append the record "Task 9e-1" |

### Task 1 (9e-1): Books: the Balance Sheet drill reads its first day from the stored months

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_spec_helpers.go` (lines 85-91)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet.go` (lines 49-51 and 199-213)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_transaction_detail_test.go` (line 1017 and lines 1202-1268)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_cash_basis_test.go` (imports, before line 235, lines 326-327)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet_test.go` (lines 246-247 and 1167)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/report-drill-down.spec.ts` (lines 805-843)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (append at the end)

**Interfaces:**
- Consumes (they exist): `firstLineWhere(source string, d *reports.Node, more ...*reports.Node) *reports.Node` (`report_spec_helpers.go:100`: the account row's posted rows of `source` on or before `d` that pass `more`, `Pick: "min"` of `date`), `onCash()`, `onCashLayer()` (`report_balance_sheet.go:385`, `389`), `tdAccount()`, `tdDrillTarget`, `methodDrill()`, `tdAccountOf()`, `op`, `fld`, `link`, `mustJSON`. Tests: `newDrillBook`, `drillBook.bs`, `drillTD`, `requireDrill`, `rangeJSON`, `idList`, `accrualMethod`, `runTD`, `glPeriod`, `sectionFor` (its `.lines` are `[]reports.Row`), `glSections`, `sectionLabels`, `tdSectionTotal`, `bsAccount`, `requireFigures`, `plAccount`, `paidBook`, `cashReportBook.bs`, `bsFigure`, `requireHeaderLines`, `cashDay1` (`2026-03-10`), `cashDay2` (`2026-04-15`), `plToday`, `drillToday`. E2E (all in `report-drill-down.spec.ts`): `firstOfMonth(day)`, `monthToDateWords(today)`, `customPeriodChip(from, to)`, `urlPeriod(page)`.
- Produces: `firstBasisMonth(d *reports.Node) *reports.Node` (`report_spec_helpers.go`); `firstBasisLine` no longer exists; `balanceDrill(d)` keeps its signature; `balanceSheetShipped().Revision == 6`; test helpers `tdLineIDs(t *testing.T, res *reports.Result, account *core.Record) []string` and `tdFrom(t *testing.T, b drillBook, account *core.Record, from, to string) *reports.Result`.

- [ ] **Step 1: Write the failing tests**

(a) In `report_transaction_detail_test.go`, add above `func idList` (line 1017):

```go
// tdLineIDs are the ids of the line rows of the account's section, in order.
func tdLineIDs(t *testing.T, res *reports.Result, account *core.Record) []string {
	t.Helper()
	var out []string
	for _, l := range sectionFor(t, res, account).lines {
		out = append(out, l.ID)
	}
	return out
}

// tdFrom runs Transaction Detail by Account for one account from day from to
// day to on Accrual: the run a Balance Sheet drill opened when its range
// started on the account's first line.
func tdFrom(t *testing.T, b drillBook, account *core.Record, from, to string) *reports.Result {
	t.Helper()
	return runTD(t, b.app, b.reader, map[string]any{
		"period": glPeriod(from, to), "accounts": []string{account.Id},
	}, reports.Options{Today: drillToday})
}
```

(b) In the same file, replace the comment above `TestDrillBalanceSheet` (lines 1202-1204) with:

```go
// Current and Prior year of an account row drill to the account's lines from
// the first day of the month of its first posted line up to the column's day:
// the same lines as a range from the first line's own day, and the drilled
// total equals the figure. Nothing else on the Balance Sheet drills.
```

(c) In `TestDrillBalanceSheet`, replace these two lines (1230-1231):

```go
	requireDrill(t, row, "current", map[string]string{"period": rangeJSON("2025-06-10", asOf), "method": accrualMethod, "accounts": idList(bank.Id)})
	current := drillTD(t, b.app, b.reader, res, row.ID, "current")
```

with:

```go
	// Operating's first line is of June 10: the range starts on June 1.
	requireDrill(t, row, "current", map[string]string{"period": rangeJSON("2025-06-01", asOf), "method": accrualMethod, "accounts": idList(bank.Id)})
	current := drillTD(t, b.app, b.reader, res, row.ID, "current")
	if got, want := tdLineIDs(t, current, bank), tdLineIDs(t, tdFrom(t, b, bank, "2025-06-10", asOf), bank); !slices.Equal(got, want) {
		t.Errorf("Current drills to lines %v, want the lines of a range from the first line's day %v", got, want)
	}
```

(d) Replace these two lines (1242-1243):

```go
	requireDrill(t, row, "prior", map[string]string{"period": rangeJSON("2025-06-10", priorDay), "method": accrualMethod, "accounts": idList(bank.Id)})
	prior := drillTD(t, b.app, b.reader, res, row.ID, "prior")
```

with:

```go
	requireDrill(t, row, "prior", map[string]string{"period": rangeJSON("2025-06-01", priorDay), "method": accrualMethod, "accounts": idList(bank.Id)})
	prior := drillTD(t, b.app, b.reader, res, row.ID, "prior")
	if got, want := tdLineIDs(t, prior, bank), tdLineIDs(t, tdFrom(t, b, bank, "2025-06-10", priorDay), bank); !slices.Equal(got, want) {
		t.Errorf("Prior year drills to lines %v, want the lines of a range from the first line's day %v", got, want)
	}
```

The assertions that follow each (the total equals the figure; 3 lines and 2 lines; the Savings part with `rangeJSON(priorDay, priorDay)`) stay as they are.

(e) In the same test, add above the line `requireNoDrill(t, row, "change")` (line 1267):

```go
	// Reserve's first line is of October 20, 2025. On a prior-year day before
	// it in the same month, the month is stored and no line is on or before the
	// day: the range starts on the 1st and the transaction report is empty.
	reserve := plAccount(t, b.app, "Reserve", "1040", "bank", "savings", "")
	b.post(t, "2025-10-20", ln{reserve, 600, 0}, ln{owner, 0, 600})
	mid := b.bs(t, "2026-10-15")
	rv := bsAccount(t, mid, reserve)
	requireFigures(t, rv, 600, 0)
	requireDrill(t, rv, "prior", map[string]string{"period": rangeJSON("2025-10-01", "2025-10-15"), "method": accrualMethod, "accounts": idList(reserve.Id)})
	if secs := glSections(t, drillTD(t, b.app, b.reader, mid, rv.ID, "prior")); len(secs) != 0 {
		t.Errorf("Reserve's Prior year 0.00 drills to sections %v, want none", sectionLabels(secs))
	}
	requireDrill(t, rv, "current", map[string]string{"period": rangeJSON("2025-10-01", "2026-10-15"), "method": accrualMethod, "accounts": idList(reserve.Id)})

```

(f) In `report_cash_basis_test.go`, add `"slices"` to the imports, between `"reflect"` and `"strings"`. Add this test above the comment `// A bill dated in the prior fiscal year and paid in the current one counts in` (line 235):

```go
// On Cash an account's Current drills from the first day of the month of its
// first line on the cash basis, a cash-basis row included, and lists the same
// lines as a range from that line's own day.
func TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth(t *testing.T) {
	b, _, _ := paidBook(t)
	const cashMethod = `"cash"`
	tdOnCash := func(account *core.Record, from string) *reports.Result {
		return runTD(t, b.app, b.reader, map[string]any{
			"period": glPeriod(from, cashDay2), "method": "cash", "accounts": []string{account.Id},
		}, reports.Options{Today: plToday})
	}
	requireSameLines := func(what string, res *reports.Result, account *core.Record, firstDay string, wantLines int) {
		t.Helper()
		row := bsAccount(t, res, account)
		td := drillTD(t, b.app, b.reader, res, row.ID, "current")
		requireHeaderLines(t, what+": the drilled Transaction Detail", td, "Cash basis")
		got, want := tdLineIDs(t, td, account), tdLineIDs(t, tdOnCash(account, firstDay), account)
		if len(got) != wantLines || !slices.Equal(got, want) {
			t.Errorf("%s drills to lines %v, want the %d lines of a range from the first line's day %s: %v", what, got, wantLines, firstDay, want)
		}
		if total, figure := tdSectionTotal(t, td, account), bsFigure(t, res, account); total != figure {
			t.Errorf("%s drills to a total of %d, want the figure %d", what, total, figure)
		}
	}

	cash := b.bs(t, cashDay2, "cash")
	// Accounts Payable's first line is of March 10 (the bills).
	requireDrill(t, bsAccount(t, cash, b.payable), "current", map[string]string{
		"period": rangeJSON("2026-03-01", cashDay2), "method": cashMethod, "accounts": idList(b.payable.Id)})
	requireSameLines("Accounts Payable", cash, b.payable, cashDay1, 6)
	// Business Checking's first line is the payment of April 15.
	requireDrill(t, bsAccount(t, cash, b.checking1000), "current", map[string]string{
		"period": rangeJSON("2026-04-01", cashDay2), "method": cashMethod, "accounts": idList(b.checking1000.Id)})
	requireSameLines("Business Checking", cash, b.checking1000, cashDay2, 1)

	// An account whose cash-basis rows start in a month with no ledger line of
	// its own: with Accounts Payable's stored months gone, only its cash-basis
	// rows give the first month.
	if _, err := b.app.DB().NewQuery("DELETE FROM books_account_balances WHERE account = {:a}").Bind(dbx.Params{"a": b.payable.Id}).Execute(); err != nil {
		t.Fatalf("delete the stored months of Accounts Payable: %v", err)
	}
	cashOnly := b.bs(t, cashDay2, "cash")
	requireDrill(t, bsAccount(t, cashOnly, b.payable), "current", map[string]string{
		"period": rangeJSON("2026-03-01", cashDay2), "method": cashMethod, "accounts": idList(b.payable.Id)})
}

```

(g) The shipped revision is pinned in three places. Change each:

- `report_cash_basis_test.go` lines 326-327 (`TestCashBasisReport_ShippedSpecs`) become:

```go
		// 3 for the Method param, 4 for the stored months, 5 for the stored
		// months on Cash, 6 for the drill's first day from the stored months.
		{balanceSheetShipped(), 6, "asOf"},
```

- `report_balance_sheet_test.go` lines 246-247 (`TestBalanceSheetSpecLoads`) become:

```go
	if shipped.Revision != 6 || !shipped.Unpublished {
		t.Errorf("revision %d, unpublished %v; want revision 6 (the drill's first day from the stored months), shipped unpublished", shipped.Revision, shipped.Unpublished)
```

- `report_balance_sheet_test.go` line 1167 (`TestBalanceSheetShippedUnpublished`): `row.GetInt("shipped_revision") != 5` becomes `row.GetInt("shipped_revision") != 6`.

- [ ] **Step 2: Run the tests to verify they fail**

The output of a failure of `TestBalanceSheetShippedUnpublished` prints the whole stored spec (tens of thousands of characters). Send the output to a file and read only short lines:

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'TestDrillBalanceSheet$|TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth$|TestBalanceSheetSpecLoads$|TestBalanceSheetShippedUnpublished$|TestCashBasisReport_ShippedSpecs$' -count=1 . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/task9e1-red.log 2>&1; command grep -E '^--- FAIL|_test.go:[0-9]+: ' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/task9e1-red.log | cut -c1-300`

Expected: FAIL, five tests. Among the lines:

```
report_transaction_detail_test.go: 1010 Operating (group) current@drill period = {"from":"2025-06-10","to":"2026-10-31"}, want {"from":"2025-06-01","to":"2026-10-31"}
report_transaction_detail_test.go: 1010 Operating (group) prior@drill period = {"from":"2025-06-10","to":"2025-10-31"}, want {"from":"2025-06-01","to":"2025-10-31"}
report_transaction_detail_test.go: 1040 Reserve (group) prior@drill period = {"from":"2025-10-15","to":"2025-10-15"}, want {"from":"2025-10-01","to":"2025-10-15"}
report_transaction_detail_test.go: 1040 Reserve (group) current@drill period = {"from":"2025-10-20","to":"2026-10-15"}, want {"from":"2025-10-01","to":"2026-10-15"}
report_cash_basis_test.go: 2000 Accounts Payable (group) current@drill period = {"from":"2026-03-10","to":"2026-04-15"}, want {"from":"2026-03-01","to":"2026-04-15"}
report_cash_basis_test.go: 1000 Business Checking (group) current@drill period = {"from":"2026-04-15","to":"2026-04-15"}, want {"from":"2026-04-01","to":"2026-04-15"}
report_balance_sheet_test.go: revision 5, unpublished true; want revision 6 (the drill's first day from the stored months), shipped unpublished
report_cash_basis_test.go: revision 5, want 6 (the Method param's, or a later one)
```

No line says "drills to lines": the cross-checks pass before the change too (the two ranges list the same lines).

- [ ] **Step 3: Change the spec**

(a) In `report_spec_helpers.go`, replace `firstBasisLine` and its comment (lines 85-91) with:

```go
// firstBasisMonth is the first day of the first month, on or before day d, in
// which the account row has a posted line on the run's method: where a Balance
// Sheet drill's range starts, so it holds every line a balance on d sums. It
// reads the account's stored months (books_account_balances: one row for each
// month with a posted ledger line), not its lines, so its cost does not grow
// with them; on Cash also the month of the account's first cash-basis row,
// which no stored month holds. No value when the account has neither.
func firstBasisMonth(d *reports.Node) *reports.Node {
	stored := &reports.Node{
		Op:     "lookup",
		Source: "books.account_balances",
		Where:  op("and", op("eq", link("account"), tdAccount()), op("lte", fld("month_start"), d)),
		Value:  mustJSON(fld("month_start")),
		Pick:   "min",
	}
	cash := op("startOfMonth", firstLineWhere("books.basis_lines", d, onCash(), onCashLayer()))
	// The earlier of the two; either may have no value. Core's min takes
	// numbers only, so the days are compared.
	a, b := op("coalesce", stored, cash), op("coalesce", cash, stored)
	return op("if", op("lte", a, b), a, b)
}
```

`firstLine`, `firstLineFY` and `firstLineWhere` do not change.

(b) In `report_balance_sheet.go`, in `balanceSheetShipped`, add two comment lines after the comment of revision 5 and set the revision:

```go
		// 6: a drill's period starts on the first day of the account's first
		// month (read from the stored months), not on its first line's day.
		Revision:    6,
```

(c) In the same file, replace the comment of `balanceDrill` and the function (lines 199-213) with:

```go
// balanceDrill opens Transaction Detail by Account from an account row's
// balance on day d on the run's method: the account's posted lines from the
// first day of its first month up to d, so the section total is the balance.
// An account with no posted line on or before d still opens (its 0.00
// figure), an empty run: the range is then d to d, or the 1st of d's month to
// d when the account's first line is later in that month. Class, kind and
// section rows do not drill: their groups are no account type.
func balanceDrill(d *reports.Node) *reports.Drill {
	return tdDrillTarget(map[string]map[string]reports.DrillValue{
		"byAccount": {
			"period":   {Range: &reports.DrillRange{From: op("coalesce", firstBasisMonth(d), d), To: d}},
			"method":   methodDrill(),
			"accounts": tdAccountOf(),
		},
	}, nil)
}
```

The five Go files after the change are in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part24-scratch` (same file names): compare, do not copy one over a file.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -run 'TestDrillBalanceSheet$|TestCashBasisReport_BalanceSheetDrillStartsOnTheFirstOfTheMonth$|TestBalanceSheetSpecLoads$|TestBalanceSheetShippedUnpublished$|TestCashBasisReport_ShippedSpecs$' -count=1 .`
Expected: `go vet` prints nothing (a remaining `firstBasisLine` is a compile error: remove the use); the test line is `ok  tinycld.org/packages/books` (about 5 s).

- [ ] **Step 5: Run the package's report tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go test -run 'TestBalanceSheet|TestCashBasisReport|TestDrill|TestStatementsScaleBook|TestStatementsScaleBars|Report|Shipped|TrialBalance|GeneralLedger|ProfitAndLoss|TransactionDetail' -count=1 . > /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/task9e1-green.log 2>&1; command grep -E '^--- FAIL|^ok|^FAIL' /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/task9e1-green.log | cut -c1-300`
Expected: one line, `ok  tinycld.org/packages/books` (about 100 s). It proves the other reports still build and pass, and the cross-check tests of the figures (`TestBalanceSheetStoredMonthsEqualTheLines`, `…WithCashRows`) stay green.

- [ ] **Step 6: Change the e2e pin**

The test `a Balance Sheet and a Trial Balance figure open the account up to the day` (`books/tests/e2e/report-drill-down.spec.ts`, line 805) posts the bank account's two lines today and pins the drilled period to today alone. The period now starts on the 1st of this month. Replace lines 839-843:

```ts
        expect(urlPeriod(page)).toBe(`${today}..${today}`)
        await expect(page.getByTestId('report-document-period')).toHaveText(dayWords(today))
        await expect(page.getByTestId('report-param-period')).toHaveText(
            customPeriodChip(today, today)
        )
```

with:

```ts
        // The account's first line is of today: the period starts on the 1st of this month.
        expect(urlPeriod(page)).toBe(`${firstOfMonth(today)}..${today}`)
        await expect(page.getByTestId('report-document-period')).toHaveText(
            monthToDateWords(today)
        )
        await expect(page.getByTestId('report-param-period')).toHaveText(
            customPeriodChip(firstOfMonth(today), today)
        )
```

`firstOfMonth`, `monthToDateWords` and `customPeriodChip` are functions of this file; each gives the one-day form when today is the 1st. `dayWords` stays imported (other tests use it). The assertions after these lines (the Accounts chip, the total 325.00, the back link, the Trial Balance part) do not change.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check tests/e2e/report-drill-down.spec.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck`
Expected: biome reports no error for the file; the typecheck ends with no error.

- [ ] **Step 7: Commit (Books), then hand the e2e run to the driver**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add server/report_spec_helpers.go server/report_balance_sheet.go server/report_transaction_detail_test.go server/report_cash_basis_test.go server/report_balance_sheet_test.go tests/e2e/report-drill-down.spec.ts && git commit -m "perf(reports): the Balance Sheet drill reads its first day from the stored months"
```

Hand back this line and stop until the driver gives the result:

```
NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7210|7211) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7210 E2E_PORT_2=7211 E2E_IMAP_PORT=1210 E2E_IMAP_PORT_2=1211 pnpm exec tinycld-pkg test:e2e books/tests/e2e/report-drill-down.spec.ts
```

Needed from it: the `Running N tests` line (N is the count of tests of that one file) and the pass/fail line. Expected: every test passes. If `a Balance Sheet and a Trial Balance figure open the account up to the day` fails on the period, fix the cause in this task (the pin of step 6 or the spec of step 3); do not run it again without a change.

- [ ] **Step 8: Hand the 100k run to the driver**

Hand back this line and stop until the driver gives the two files:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9e1 go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
```

Needed from it: `scale-task9e1/st-results.md` (the rows "BS 1 accrual, as of" and "BS 2 cash, as of") and `scale-task9e1/st-plans.md` (the sections of those two runs). Expected: BS 1 accrual lower than its 940 ms (the two line lookups of the drills are gone on Accrual; about 0.5 s was their cost); BS 2 cash not higher than its 1.965 s. The run fails no bar at 100k. If BS 1 is not lower than 940 ms or BS 2 is higher than 2.2 s, hand back `STOPPED: the Balance Sheet took <BS 1 time> / <BS 2 time> at 100k after 9e-1` with the two sections of `st-plans.md`.

- [ ] **Step 9: Record the result in the ledger (Books)**

Read the end of `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (the record "Task 9d-1" is the pattern), then append with the Edit tool. Write the heading and the first paragraph as they are here; fill the table from `scale-task9e1/st-results.md`, and the last paragraph from `scale-task9e1/st-plans.md`:

```markdown
#### Task 9e-1: the Balance Sheet drill reads its first day from the stored months (books `server/report_spec_helpers.go`, 2026-11-20)

A figure's drill opens Transaction Detail by Account from the account's first line up to the column's day. The first day was a `min(date)` over the account's lines, once for Current and once for Prior year (about 0.5 s of the Accrual run at 100,000 lines). It is now the first day of the account's first stored month; on Cash the month of the account's first cash-basis row is read too. The drilled period starts on the 1st of the first line's month (human decision, 2026-11-20) and lists the same lines.

| lines | run | before (task 9d-1) | after |
|---|---|---|---|
| 100000 | BS 1 accrual, as of | 940ms | <median of the run> |
| 100000 | BS 2 cash, as of | 1.965s | <median of the run> |

<One paragraph: the count of "SCAN books_journal_lines" and of "SCAN books_cash_basis_lines" in the BS 1 and the BS 2 sections of st-plans.md (each was 21 in each section after task 9d-1), and the statement's size in chars. The bar is 2 s at 500,000 lines for both methods; tasks 9d-2 and 9e-2 of the plan outline are what is left for it, each decided from a 500k run.>
```

The three `<…>` parts are values the run gives: write the values, not the brackets.

- [ ] **Step 10: Commit the record (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add docs/plans/general-ledger-ledger.md && git commit -m "docs: the Balance Sheet at 100k after its drill reads the stored months"
```
