# Report engine scale, part 21: the Balance Sheet reads the stored months (task 9c-2b, Go) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On Accrual the Balance Sheet reads the months before the as-of day's month (and before the prior day's month) from `books_account_balances`, so its cost does not grow with the book's history; every figure stays what the line read shows.

**Architecture:** The report's rows become the posted lines of the as-of day's month only (both methods). Each account row's Current and Prior year are a `rollup` with `total: "rows"`: the balance before the month plus the month's lines. "Before the month" is a lookup of the stored months on Accrual and a lookup of the account's lines on Cash (no stored month holds the cash rows; a cash month table is task 9d). The lookups carry the method as a term of their `where` (`onAccrual()`, `onCash()`), so one spec serves both methods with no `if` around a lookup. The account group gets a `fill` (an account with no line in the month still shows, under its own section, class and kind) and an `opening` row ("Balance before <month>"). Retained earnings and Net income read stored months plus the lines of their day's month on Accrual.

**Tech Stack:** Go, `tinycld.org/core/reports` (core 67f5f903: rollup with `total: "rows"` under layout sections, a fill that reaches a parent with no base row, opening label with a month). Books at 726c1d3f, branch `books/p5-report-engine-scale`.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/balance-sheet-design.md` (the figures, unchanged); `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`, sections "State after plan part 19", "State after plan part 20" and "State after plan part 21"; `rulings.md` #83. The pattern is the Trial Balance: `books/server/report_trial_balance.go`, `report_trial_balance_stored_test.go`.

## Global Constraints

- Work in `/Users/nas/tmp/tinycld/books-purchases/books` only. No core change: when a core gap shows, stop and report it with the failing command and its output.
- Every Balance Sheet figure, row and currency code stays what the line read shows, on Accrual and on Cash, for every as-of day (the cross-check test of Task 1 is the proof).
- HUMAN decision: an opened account shows "Balance before <month>" (`trialBalanceBeforeLabel`) and then the lines of the as-of day's month; the drill-down (`balanceDrill`) still lists every line and is not changed.
- Bar: Balance Sheet on Accrual at 500k lines at most 2 s (now 1,373 ms at 100k). No timing probes in this task: the driver runs `TestStatementsScale` (part 22).
- No backward compatibility: the shipped spec is replaced; `Revision` goes from 3 to 4.
- Money stays integer cents; currency codes come from the engine (`<col>@currency`), never from a test's own text.
- Run Go tests from `/Users/nas/tmp/tinycld/books-purchases/books/server` with `go test -run '<names>' .`; the whole package once at the end of the task (about 5 minutes).

## Review Focus

1. A day in a month with no posted line (May or November 2026 in the test book): every account row, section and total still shows with its figure and `USD`. Pinned by `TestBalanceSheetOfAMonthWithNoLine` and the days `2026-05-31` and `bsToday` of the cross-check.
2. A class with no line in the as-of month (Fixed assets, Long-term liabilities: lines of 2025 only): the class row, its subtotal and its account show under the right section. Pinned by the cross-check (it compares every row, in order) and by `Total Fixed assets` in `TestBalanceSheetOfAMonthWithNoLine`.
3. The prior day inside a month with lines before and after it (as of 2026-03-19, prior day 2025-03-19, lines on 2025-03-10, -12 and -25): Prior year counts the lines up to the 19th only. Pinned by the day `2026-03-19` of the cross-check and by `TestBalanceSheetOpenedAccountRows`.
4. An account back at zero with posted lines (Netted), an account with no line (Idle), an inactive account with a balance (Closed), on Rows = Active and Non-zero. Pinned by the cross-check on both Rows choices.
5. Cash: no figure changes, and a changed stored month does not move a Cash figure. Pinned by the `cash` runs of the cross-check and the last check of `TestBalanceSheetReadsTheStoredMonths`. A book with cash-basis rows (a part-paid bill) is pinned by `TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows`, on both methods, with days a year later for the Prior year column. The existing `TestCashBasisReport_*` tests stay green, unchanged but for the Balance Sheet's revision number.

---

## File Structure

| File | Change |
|---|---|
| `server/report_balance_sheet.go` | The spec: month filter, `current`/`prior` rollups, `fill` and `opening` on `byAccount`, stored-month lookups (`bsSigned`, `onCash`, `onAccrual`, `bsAccountLines`, `bsStoredBefore`, `bsBefore`, `bsBalanceAt`, `bsActivity`), `profitAndLossSum` on stored months, `Revision: 4` |
| `server/report_sources.go` | `books.basis_lines.currency` is the same currency as `books.journal_lines.currency`, so the engine adds stored months to basis lines |
| `server/report_balance_sheet_stored_test.go` (new) | The line-read spec, the cross-check, and the stored-read tests |
| `server/report_balance_sheet_test.go` | The two opened-lines tests post their lines in the as-of day's month and check the opening row; revision 4 |
| `server/report_cash_basis_test.go` | The Balance Sheet's shipped revision is 4 |

Part 22 (not in this file) holds the rest of 9c-2b: the e2e change, the help text, the 100k scale run and the ledger record.

### Task 1: The Balance Sheet spec reads the stored months

**Files:**
- Create: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet_stored_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_sources.go` (the `currency` field of `basisLinesSource`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet_test.go` (`TestBalanceSheetOpenedLines`, `TestBalanceSheetPagesOpenedLines`, two revision pins)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_cash_basis_test.go` (`TestCashBasisReport_ShippedSpecs`: revision 4)

**Interfaces:**
- Consumes (exist, do not change): `glSum(source string, value *reports.Node, where ...*reports.Node) *reports.Node` and `tbStoredCount(where ...*reports.Node) *reports.Node` and `trialBalanceBeforeLabel` (Trial Balance / General Ledger files); `basisFilter()`, `tdAccount()`, `creditNormalTypes()`, `profitAndLossTypes()`, `op`, `fld`, `link`, `lit`, `prm`, `col` (`report_spec_helpers.go`); test helpers `newBSBook`, `runBS`, `runTBSpec`, `bsAccount`, `bsRow`, `amt`, `requireFigures`, `bsWireKey`, `labelsOf`, `mkRetainedEarnings`, `saveOrFail`, `ln`.
- Produces: `bsSigned(debit, credit string) *reports.Node`; `onCash() *reports.Node`, `onAccrual() *reports.Node`; `bsBefore(x *reports.Node) *reports.Node`; `bsBalanceAt(x *reports.Node) *reports.Node`; `bsActivity(x *reports.Node) *reports.Node`; `profitAndLossSum(x *reports.Node, which func(day string) *reports.Node) *reports.Node`; test helpers `balanceSheetSpecFromLines() *reports.Spec`, `bsFigures(res *reports.Result) string`, `newBSStoredBook(t) bsStoredBook`. An opened account's rows: one row of kind `opening` (row id prefix `opening:`, label template `Balance before {value}`, month value), then the as-of month's lines; part 22's e2e reads this.

Facts the code depends on (each seen in a run of this plan's build, core 67f5f903):
- The engine refuses `add` of a `books.account_balances` amount and a `books.basis_lines` amount (`columns.1.rollup.args.0: amounts in different currencies cannot be combined`) until `books.basis_lines.currency` declares `SameCurrencyAs: "books.journal_lines.currency"`.
- A group key posted back in `Options.Expand` must go through JSON (`bsWireKey`), or the run answers `options.expand.0: a group key holds text, numbers, true, false or null`.
- The fill has no tie to `side`, `cls` or `knd` in its `where`: those groups read only the account, so each account sits under its own parents (core `docs/packages.md`, `groups[].fill`). The cross-check compares the whole outline, so a wrong parent fails it.

- [ ] **Step 1: Write the failing tests**

Create `server/report_balance_sheet_stored_test.go` with exactly this content:

```go
package books

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/reports"
)

// bsLinesSum is credits − debits of the posted lines on or before x that match
// which, read from the lines alone.
func bsLinesSum(x, which *reports.Node) *reports.Node {
	return glSum("books.basis_lines", op("sub", fld("credit"), fld("debit")),
		basisFilter(), op("eq", fld("posted"), lit(true)), op("lte", fld("date"), x), which)
}

// balanceSheetSpecFromLines is the Balance Sheet as it was before it read the
// stored months: its rows are every posted line up to the as-of day, each
// figure is a sum over them, and Retained earnings and Net income read every
// line. The stored read must show what it shows.
func balanceSheetSpecFromLines() *reports.Spec {
	d := prm("asOf")
	dPrior := op("addMonths", prm("asOf"), lit(-12))
	signed := bsSigned("debit", "credit")
	pl := op("in", fld("account", "type"), lit(profitAndLossTypes()))
	retained := func(x *reports.Node) *reports.Node {
		return bsLinesSum(x, op("or",
			op("eq", fld("account", "detail_type"), lit("retained_earnings")),
			op("and", pl, op("lt", op("fiscalYear", fld("date")), op("fiscalYear", x))),
		))
	}
	netIncome := func(x *reports.Node) *reports.Node {
		return bsLinesSum(x, op("and", pl, op("gte", op("fiscalYear", fld("date")), op("fiscalYear", x))))
	}
	spec := balanceSheetSpec()
	spec.Filter = op("and",
		basisFilter(),
		op("eq", fld("posted"), lit(true)),
		op("lte", fld("date"), d),
		op("not", pl),
		op("ne", fld("account", "detail_type"), lit("retained_earnings")),
	)
	for i := range spec.Columns {
		c := &spec.Columns[i]
		switch c.ID {
		case "current":
			c.Total, c.Rollup, c.Drill = "", nil, nil
			c.Formula = op("coalesce", op("sum", signed), lit(0))
		case "prior":
			c.Total, c.Rollup, c.Drill = "", nil, nil
			c.Formula = op("coalesce", op("sum", op("if", op("lte", fld("date"), dPrior), signed, lit(nil))), lit(0))
		}
	}
	g := &spec.Groups[len(spec.Groups)-1]
	g.Fill, g.Opening = nil, nil
	g.Having = op("or",
		op("eq", prm("rows"), lit("active")),
		op("ne", col("current"), lit(0)),
		op("ne", col("prior"), lit(0)),
	)
	for i := range spec.Layout {
		switch item := &spec.Layout[i]; item.Row {
		case "retainedEarnings":
			item.Cells = map[string]*reports.Node{"current": retained(d), "prior": retained(dPrior)}
		case "netIncome":
			item.Cells = map[string]*reports.Node{"current": netIncome(d), "prior": netIncome(dPrior)}
		}
	}
	return spec
}

// bsFigures writes every row as one line: its kind, level, label, and the
// four figures, Current and Prior year with their currency codes. A cell the
// row does not have reads <nil>.
func bsFigures(res *reports.Result) string {
	var out strings.Builder
	for _, r := range res.Rows {
		fmt.Fprintf(&out, "%s|%d|%s|%v %v|%v %v|%v|%v\n", r.Kind, r.Level, r.Label,
			r.Cells["current"], r.Cells["current@currency"], r.Cells["prior"], r.Cells["prior@currency"],
			r.Cells["change"], r.Cells["pct"])
	}
	return out.String()
}

// bsStoredBook has three fiscal years of posted lines. Income and expense of
// 2024 and 2025 fold into Retained earnings; Retained earnings has lines of
// its own (June 2025, March 2026). No line is dated May, October or November
// 2026. Equipment (the only fixed asset) and Loan (the only long-term
// liability) have lines of 2025 only, so in 2026 their class has no line in
// the month. Netted is back at 0 from April 2026 on; Idle has no line; Closed
// is inactive with a balance.
type bsStoredBook struct {
	bsBook
	checking, savings, receivable, equipment, visa, loan, owner, retained, income, rent, netted, idle, closed *core.Record
}

func newBSStoredBook(t *testing.T) bsStoredBook {
	t.Helper()
	b := bsStoredBook{bsBook: newBSBook(t)}
	b.checking = b.account(t, "Operating", "1000", "bank", "checking")
	b.savings = b.account(t, "Reserve", "1010", "bank", "savings")
	b.netted = b.account(t, "Netted", "1020", "bank", "checking")
	b.idle = b.account(t, "Idle", "1030", "bank", "checking")
	b.closed = b.account(t, "Closed", "1040", "bank", "checking")
	b.receivable = b.account(t, "Receivables", "1100", "accounts_receivable", "accounts_receivable")
	b.equipment = b.account(t, "Equipment", "1500", "fixed_asset", "machinery_equipment")
	b.visa = b.account(t, "Visa", "2100", "credit_card", "credit_card")
	b.loan = b.account(t, "Bank loan", "2700", "long_term_liability", "notes_payable")
	b.owner = b.account(t, "Owner's equity", "3000", "equity", "owners_equity")
	b.retained = mkRetainedEarnings(t, b.app, "3900")
	b.income = b.account(t, "Fees", "4000", "income", "service_income")
	b.rent = b.account(t, "Office rent", "6000", "expense", "rent_lease")
	b.post(t, "2024-08-10", ln{b.checking, 50000, 0}, ln{b.income, 0, 50000})
	b.post(t, "2024-12-05", ln{b.rent, 8000, 0}, ln{b.checking, 0, 8000})
	b.post(t, "2025-02-01", ln{b.checking, 30000, 0}, ln{b.owner, 0, 30000})
	b.post(t, "2025-03-10", ln{b.equipment, 12000, 0}, ln{b.loan, 0, 12000})
	b.post(t, "2025-03-12", ln{b.receivable, 90000, 0}, ln{b.income, 0, 90000})
	b.post(t, "2025-03-25", ln{b.checking, 60000, 0}, ln{b.receivable, 0, 60000})
	b.post(t, "2025-06-15", ln{b.checking, 1000, 0}, ln{b.retained, 0, 1000})
	b.post(t, "2025-11-20", ln{b.rent, 25000, 0}, ln{b.visa, 0, 25000})
	b.post(t, "2025-12-31", ln{b.closed, 700, 0}, ln{b.checking, 0, 700})
	b.post(t, "2026-01-15", ln{b.checking, 40000, 0}, ln{b.income, 0, 40000})
	b.post(t, "2026-03-05", ln{b.rent, 12000, 0}, ln{b.checking, 0, 12000})
	b.post(t, "2026-03-10", ln{b.checking, 2000, 0}, ln{b.retained, 0, 2000})
	b.post(t, "2026-03-15", ln{b.netted, 5000, 0}, ln{b.owner, 0, 5000})
	b.post(t, "2026-03-20", ln{b.savings, 30000, 0}, ln{b.checking, 0, 30000})
	b.post(t, "2026-04-02", ln{b.owner, 5000, 0}, ln{b.netted, 0, 5000})
	b.post(t, "2026-04-30", ln{b.visa, 10000, 0}, ln{b.checking, 0, 10000})
	b.post(t, "2026-06-01", ln{b.rent, 12000, 0}, ln{b.checking, 0, 12000})
	b.post(t, "2026-09-09", ln{b.checking, 25000, 0}, ln{b.savings, 0, 25000})
	b.closed.Set("inactive", true)
	saveOrFail(t, b.app, b.closed)
	return b
}

// bsStoredDays are a month's first, middle and last day, days in a month with
// no line (May, November 2026), a day before the first line, a day whose prior
// day is in a month with lines (2026-03-19 → 2025-03-19, between two lines of
// that month), and the last and first day of a fiscal year.
var bsStoredDays = []string{
	"2024-07-31", "2024-12-31", "2025-06-30", "2025-12-31", "2026-01-01", "2026-03-01", "2026-03-05",
	"2026-03-19", "2026-03-31", "2026-04-30", "2026-05-31", "2026-06-30", "2026-07-01", bsToday,
}

// The stored read shows what the line read shows: the same rows in the same
// order (sections, classes, kinds, accounts, subtotals, Retained earnings,
// Net income, Total liabilities and equity), the same Current, Prior year,
// Change and % change, and the same currency codes; on each method, with the
// fiscal year starting in January and in July, and with each Rows choice.
func TestBalanceSheetStoredMonthsEqualTheLines(t *testing.T) {
	for _, fiscalStart := range []int{1, 7} {
		b := newBSStoredBook(t)
		b.fiscalStart(t, fiscalStart)
		for _, method := range []string{"accrual", "cash"} {
			for _, asOf := range bsStoredDays {
				for _, rows := range []string{"nonzero", "active"} {
					params := map[string]any{"asOf": asOf, "rows": rows, "method": method}
					opts := reports.Options{Today: bsToday}
					stored := bsFigures(runTBSpec(t, b.tbBook, balanceSheetSpec(), params, opts))
					lines := bsFigures(runTBSpec(t, b.tbBook, balanceSheetSpecFromLines(), params, opts))
					if stored != lines {
						t.Errorf("fiscal start %d, %s, as of %s, rows %s:\nstored months\n%s\nlines\n%s", fiscalStart, method, asOf, rows, stored, lines)
					}
					// Two rows with no code are equal too: from the first year
					// on, Total Assets names the currency in both columns.
					if asOf < "2024-12-31" {
						continue
					}
					named := false
					for _, line := range strings.Split(stored, "\n") {
						if strings.Contains(line, "|Total Assets|") {
							named = strings.Count(line, " USD") == 2
						}
					}
					if !named {
						t.Errorf("fiscal start %d, %s, as of %s, rows %s: Total Assets does not show USD on Current and Prior year in\n%s", fiscalStart, method, asOf, rows, stored)
					}
				}
			}
		}
	}
}

// The same on a book with cash-basis rows (a bill of cashDay1, part paid on
// cashDay2): on Accrual the cash rows count nowhere; on Cash they count in the
// balance before the month, in the month's lines, in the Prior year column a
// year later, and in Retained earnings and Net income.
func TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows(t *testing.T) {
	b, _, _ := paidBook(t)
	book := tbBook{app: b.app, reader: b.reader}
	last := map[string]string{}
	for _, method := range []string{"accrual", "cash"} {
		for _, asOf := range []string{"2026-03-10", "2026-03-31", "2026-04-15", "2026-04-30", "2027-03-20", "2027-04-10", "2027-04-20"} {
			for _, rows := range []string{"nonzero", "active"} {
				params := map[string]any{"asOf": asOf, "rows": rows, "method": method}
				opts := reports.Options{Today: asOf}
				stored := bsFigures(runTBSpec(t, book, balanceSheetSpec(), params, opts))
				lines := bsFigures(runTBSpec(t, book, balanceSheetSpecFromLines(), params, opts))
				if stored != lines {
					t.Errorf("%s, as of %s, rows %s:\nstored months\n%s\nlines\n%s", method, asOf, rows, stored, lines)
				}
				last[method] = stored
			}
		}
	}
	if last["cash"] == last["accrual"] {
		t.Error("the Cash run equals the Accrual run on the last day: the book has no cash-basis row, so this test proves nothing")
	}
}

// In a month with no posted line every figure still shows, in the book's
// currency: each account row comes from its stored months, under its own
// section, class and kind, and the sections add them.
func TestBalanceSheetOfAMonthWithNoLine(t *testing.T) {
	b := newBSStoredBook(t)
	res := runBS(t, b.app, b.reader, map[string]any{"asOf": "2026-05-31"}, reports.Options{Today: bsToday})
	// Operating: 50000 − 8000 + 30000 + 60000 + 1000 − 700 at the end of 2025
	// (132300), then + 40000 − 12000 + 2000 − 30000 − 10000. A year earlier:
	// 50000 − 8000 + 30000 + 60000.
	requireFigures(t, bsAccount(t, res, b.checking), 122300, 132000)
	requireFigures(t, bsAccount(t, res, b.equipment), 12000, 12000)
	requireFigures(t, bsRow(t, res, "Total Fixed assets"), 12000, 12000)
	requireFigures(t, bsAccount(t, res, b.loan), 12000, 12000)
	requireFigures(t, bsAccount(t, res, b.visa), 15000, 0)
	// Retained earnings: 1000 + 2000 of its own, and 2024 and 2025 income less
	// expense (50000 − 8000 + 90000 − 25000). A year earlier: 2024 only.
	requireFigures(t, bsRow(t, res, "Retained earnings"), 110000, 42000)
	// Net income: 40000 − 12000 in 2026; 90000 by 2025-05-31.
	requireFigures(t, bsRow(t, res, "Net income"), 28000, 90000)
	assets, le := bsRow(t, res, "Total Assets"), bsRow(t, res, "Total liabilities and equity")
	requireFigures(t, le, amt(t, assets, "current"), amt(t, assets, "prior"))
	for _, r := range []reports.Row{assets, le, bsRow(t, res, "Total Fixed assets")} {
		for _, cell := range []string{"current@currency", "prior@currency"} {
			if got := r.Cells[cell]; got != "USD" {
				t.Errorf("%s %s = %#v, want USD", r.Label, cell, got)
			}
		}
	}
}

// On Accrual the months before the as-of day's month are read from the stored
// months and that month from its lines; on Cash no stored month is read.
func TestBalanceSheetReadsTheStoredMonths(t *testing.T) {
	b := newBSStoredBook(t)
	figures := func(method string) (current, prior, retained int64) {
		t.Helper()
		res := runBS(t, b.app, b.reader, map[string]any{"asOf": "2026-06-30", "method": method}, reports.Options{Today: bsToday})
		row := bsAccount(t, res, b.checking)
		return amt(t, row, "current"), amt(t, row, "prior"), amt(t, bsRow(t, res, "Retained earnings"), "current")
	}
	bump := func(account *core.Record, period string) {
		t.Helper()
		_, err := b.app.DB().NewQuery("UPDATE books_account_balances SET debit_total = debit_total + 5 WHERE account = {:a} AND period = {:p}").
			Bind(map[string]any{"a": account.Id, "p": period}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
	current, prior, retained := figures("accrual")
	cashCurrent, cashPrior, cashRetained := figures("cash")
	bump(b.checking, "2026-04")
	bump(b.checking, "2025-03")
	bump(b.rent, "2025-11")
	// April 2026 and March 2025 are before June 2026; March 2025 is before
	// June 2025 too. Rent's debit of 2025 lowers Retained earnings.
	if c, p, r := figures("accrual"); c != current+10 || p != prior+5 || r != retained-5 {
		t.Errorf("after three earlier stored months rose by 5: Operating %d / %d, Retained earnings %d; want %d / %d, %d",
			c, p, r, current+10, prior+5, retained-5)
	}
	bump(b.checking, "2026-06")
	if c, _, _ := figures("accrual"); c != current+10 {
		t.Errorf("after June's stored debit rose by 5, Operating = %d, want %d: the as-of day's month is read from its lines", c, current+10)
	}
	if c, p, r := figures("cash"); c != cashCurrent || p != cashPrior || r != cashRetained {
		t.Errorf("Cash after the stored months changed: Operating %d / %d, Retained earnings %d; want %d / %d, %d: Cash reads the lines",
			c, p, r, cashCurrent, cashPrior, cashRetained)
	}
}

// An opened account shows one row for the months before the as-of day's
// month, labelled with that month, then the month's lines up to the day. The
// opening row and the lines add up to the account's row in both columns.
func TestBalanceSheetOpenedAccountRows(t *testing.T) {
	b := newBSStoredBook(t)
	for _, method := range []string{"accrual", "cash"} {
		opts := reports.Options{Today: bsToday}
		params := map[string]any{"asOf": "2026-03-19", "rows": "active", "method": method}
		closed := runBS(t, b.app, b.reader, params, opts)
		opts.Collapse = [][]any{}
		opts.Expand = [][]any{bsWireKey(t, bsAccount(t, closed, b.checking).GroupKey), bsWireKey(t, bsAccount(t, closed, b.equipment).GroupKey)}
		open := runBS(t, b.app, b.reader, params, opts)

		under := func(account *core.Record) []reports.Row {
			t.Helper()
			var rows []reports.Row
			for _, r := range open.Rows {
				if r.Section == bsAccount(t, open, account).ID && (r.Kind == reports.KindOpening || r.Kind == reports.KindDetail) {
					rows = append(rows, r)
				}
			}
			return rows
		}
		requireOpening := func(r reports.Row, current, prior int64) {
			t.Helper()
			if r.Kind != reports.KindOpening || r.LabelTemplate != "Balance before {value}" || r.LabelType != reports.Month || r.LabelValue != "2026-03" {
				t.Fatalf("%s: first row = %s %q (template %q, type %q, value %#v); want the opening row \"Balance before {value}\" of month 2026-03",
					method, r.Kind, r.Label, r.LabelTemplate, r.LabelType, r.LabelValue)
			}
			requireFigures(t, r, current, prior)
			for _, cell := range []string{"change", "pct"} {
				if got := r.Cells[cell]; got != nil {
					t.Errorf("%s: the opening row's %s = %#v, want none", method, cell, got)
				}
			}
		}

		// Operating: 132300 at the end of 2025, + 40000 in January; then the
		// lines of March 5 and March 10 (March 20 is after the day). A year
		// earlier (2025-03-19): 50000 − 8000 + 30000, before the line of March 25.
		checking := under(b.checking)
		if len(checking) != 3 {
			t.Fatalf("%s: Operating's opened rows = %q, want the opening row and two lines", method, labelsOf(checking))
		}
		requireOpening(checking[0], 172300, 72000)
		for i, want := range []struct {
			day    string
			amount int64
		}{{"2026-03-05", -12000}, {"2026-03-10", 2000}} {
			line := checking[i+1]
			if !strings.HasPrefix(line.Label, want.day+" · ") {
				t.Errorf("%s: Operating's line %d = %q, want the line of %s", method, i+1, line.Label, want.day)
			}
			if got := amt(t, line, "current"); got != want.amount {
				t.Errorf("%s: line of %s current = %d, want %d", method, want.day, got, want.amount)
			}
			if got := amt(t, line, "prior"); got != 0 {
				t.Errorf("%s: line of %s prior = %d, want 0: the line is after the prior day", method, want.day, got)
			}
		}
		requireFigures(t, bsAccount(t, open, b.checking), 162300, 72000)

		// Equipment has no line in March 2026: the opening row only. Its prior
		// figure is the line of 2025-03-10, in the prior day's month.
		equipment := under(b.equipment)
		if len(equipment) != 1 {
			t.Fatalf("%s: Equipment's opened rows = %q, want the opening row only", method, labelsOf(equipment))
		}
		requireOpening(equipment[0], 12000, 12000)
	}
}
```

- [ ] **Step 2: Run the tests to see them fail**

Run (from `/Users/nas/tmp/tinycld/books-purchases/books/server`): `go test -run 'TestBalanceSheetStoredMonthsEqualTheLines|TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows|TestBalanceSheetOfAMonthWithNoLine|TestBalanceSheetReadsTheStoredMonths|TestBalanceSheetOpenedAccountRows' .`
Expected: FAIL, the package does not build: `undefined: bsSigned`.

- [ ] **Step 3: Change the spec and the source**

Apply this diff to `server/report_balance_sheet.go` and `server/report_sources.go` with the Edit tool (hunk by hunk; the result must equal the `+` side exactly):

```diff
diff --git a/server/report_balance_sheet.go b/server/report_balance_sheet.go
index 3cc2dd11..5e1724cb 100644
--- a/server/report_balance_sheet.go
+++ b/server/report_balance_sheet.go
@@ -43,7 +43,10 @@ func balanceSheetShipped() reports.Shipped {
 		// 2: an account's Current and Prior year open Transaction Detail by
 		// Account (drill).
 		// 3: the Method param (Accrual or Cash) over books.basis_lines.
-		Revision:    3,
+		// 4: the months before the as-of day's month (and before the prior
+		// day's month) are read from the stored months on Accrual; an opened
+		// account shows them as one row, then the as-of month's lines.
+		Revision:    4,
 		Unpublished: true,
 	}
 }
@@ -60,11 +63,7 @@ func balanceSheetSpec() *reports.Spec {
 	d := prm("asOf")
 	dPrior := op("addMonths", prm("asOf"), lit(-12))
 	accountType := fld("account", "type")
-	signed := op("if",
-		op("in", accountType, lit(creditNormalTypes())),
-		op("sub", fld("credit"), fld("debit")),
-		op("sub", fld("debit"), fld("credit")),
-	)
+	signed := bsSigned("debit", "credit")
 	return &reports.Spec{
 		Version: reports.CurrentVersion,
 		Source:  "books.basis_lines",
@@ -83,20 +82,29 @@ func balanceSheetSpec() *reports.Spec {
 		Filter: op("and",
 			basisFilter(),
 			op("eq", fld("posted"), lit(true)),
+			op("gte", fld("date"), op("startOfMonth", d)),
 			op("lte", fld("date"), d),
 			op("not", op("in", accountType, lit(profitAndLossTypes()))),
 			op("ne", fld("account", "detail_type"), lit("retained_earnings")),
 		),
 		Columns: []reports.Column{
 			{ID: "account", Label: "Account", Path: reports.Path{{Link: "account"}}},
-			{ID: "current", Label: "Current", Heading: d, Formula: op("coalesce", op("sum", signed), lit(0)), Drill: balanceDrill(d)},
+			// A line shows its own amount. The account's row shows the rollup, and
+			// every row above adds the account rows shown. The stored months stay
+			// out of Formula: core refuses a rows total over a looked-up amount.
 			{
-				ID: "prior", Label: "Prior year", Heading: dPrior,
-				Formula: op("coalesce",
-					op("sum", op("if", op("lte", fld("date"), dPrior), signed, lit(nil))),
-					lit(0),
-				),
-				Drill: balanceDrill(dPrior),
+				ID: "current", Label: "Current", Heading: d, Total: reports.TotalRows,
+				Formula: signed,
+				Rollup:  op("add", bsBefore(d), op("coalesce", op("sum", signed), lit(0))),
+				Drill:   balanceDrill(d),
+			},
+			// The report's rows are the as-of month's lines, all after the prior
+			// day, so a line's Prior year amount is 0.
+			{
+				ID: "prior", Label: "Prior year", Heading: dPrior, Total: reports.TotalRows,
+				Formula: op("coalesce", op("if", op("lte", fld("date"), dPrior), signed, lit(nil)), lit(0)),
+				Rollup:  bsBalanceAt(dPrior),
+				Drill:   balanceDrill(dPrior),
 			},
 			{ID: "change", Label: "Change", Detail: reports.DetailBlank, Formula: op("sub", col("current"), col("prior"))},
 			{ID: "pct", Label: "% change", Format: "percent", Detail: reports.DetailBlank, Formula: percentChange()},
@@ -123,11 +131,25 @@ func balanceSheetSpec() *reports.Spec {
 			},
 			{
 				ID: "byAccount", Col: "account", Label: accountLabel(func(n ...string) *reports.Node { return rec("byAccount", n...) }),
+				// Every balance-sheet account may get a row: one with earlier
+				// months and no line in the as-of day's month has no row of its
+				// own to start from. Each sits under its own section, class and
+				// kind, which read only the account.
+				Fill: &reports.Fill{Where: op("and",
+					op("not", op("in", fld("type"), lit(profitAndLossTypes()))),
+					op("ne", fld("detail_type"), lit("retained_earnings")),
+				)},
 				Having: op("or",
-					op("eq", prm("rows"), lit("active")),
+					op("and", op("eq", prm("rows"), lit("active")), op("gt", bsActivity(d), lit(0))),
 					op("ne", col("current"), lit(0)),
 					op("ne", col("prior"), lit(0)),
 				),
+				Opening: &reports.Opening{
+					Label:       trialBalanceBeforeLabel,
+					LabelValue:  d,
+					LabelBucket: "month",
+					Cells:       map[string]*reports.Node{"current": bsBefore(d), "prior": bsBalanceAt(dPrior)},
+				},
 			},
 		},
 		Layout: []reports.LayoutItem{
@@ -298,34 +320,118 @@ func placementLabelCase(rank *reports.Node, level func(placement) (int, string))
 // retained-earnings account's posted lines on or before x, plus every income
 // and expense line dated before the fiscal year that holds x.
 func retainedEarningsAt(x *reports.Node) *reports.Node {
-	return profitAndLossSum(x, op("or",
-		op("eq", fld("account", "detail_type"), lit("retained_earnings")),
-		op("and",
-			op("in", fld("account", "type"), lit(profitAndLossTypes())),
-			op("lt", op("fiscalYear", fld("date")), op("fiscalYear", x)),
-		),
-	))
+	return profitAndLossSum(x, func(day string) *reports.Node {
+		return op("or",
+			op("eq", fld("account", "detail_type"), lit("retained_earnings")),
+			op("and",
+				op("in", fld("account", "type"), lit(profitAndLossTypes())),
+				op("lt", op("fiscalYear", fld(day)), op("fiscalYear", x)),
+			),
+		)
+	})
 }
 
 // netIncomeAt is Net income on day x (credits − debits, so a profit reads
 // positive): the income and expense lines dated in the fiscal year that
 // holds x, on or before x.
 func netIncomeAt(x *reports.Node) *reports.Node {
-	return profitAndLossSum(x, op("and",
-		op("in", fld("account", "type"), lit(profitAndLossTypes())),
-		op("gte", op("fiscalYear", fld("date")), op("fiscalYear", x)),
-	))
+	return profitAndLossSum(x, func(day string) *reports.Node {
+		return op("and",
+			op("in", fld("account", "type"), lit(profitAndLossTypes())),
+			op("gte", op("fiscalYear", fld(day)), op("fiscalYear", x)),
+		)
+	})
 }
 
 // profitAndLossSum adds credits − debits of the posted lines on or before x
 // that match which, on the run's method (the statement's source and
-// condition); 0 when none does.
-func profitAndLossSum(x, which *reports.Node) *reports.Node {
-	return op("coalesce", &reports.Node{
+// condition); 0 when none does. which names the day it reads: a line's date,
+// or a stored month's first day (a fiscal year starts on the first day of a
+// month, so every day of a month is in the fiscal year of that first day). On
+// Accrual the months before x's month come from the stored months and only
+// x's month from its lines; on Cash every line is read, because no stored
+// month holds the cash rows.
+func profitAndLossSum(x *reports.Node, which func(day string) *reports.Node) *reports.Node {
+	monthStart := op("startOfMonth", x)
+	return op("add",
+		glSum("books.account_balances", op("sub", fld("credit_total"), fld("debit_total")),
+			onAccrual(), op("lt", fld("month_start"), monthStart), which("month_start")),
+		glSum("books.basis_lines", op("sub", fld("credit"), fld("debit")),
+			basisFilter(), op("eq", fld("posted"), lit(true)), op("lte", fld("date"), x),
+			op("or", onCash(), op("gte", fld("date"), monthStart)), which("date")),
+	)
+}
+
+// bsSigned is debit and credit by the account's normal side: credits less
+// debits on a credit-normal account. It reads a line (debit, credit) or a
+// stored month (debit_total, credit_total); both link to their account.
+func bsSigned(debit, credit string) *reports.Node {
+	return op("if",
+		op("in", fld("account", "type"), lit(creditNormalTypes())),
+		op("sub", fld(credit), fld(debit)),
+		op("sub", fld(debit), fld(credit)),
+	)
+}
+
+// onCash and onAccrual are true on a run of that method. A lookup that holds
+// one reads no row on the other method.
+func onCash() *reports.Node { return op("eq", prm("method"), lit("cash")) }
+
+func onAccrual() *reports.Node { return op("ne", prm("method"), lit("cash")) }
+
+// bsAccountLines are the posted lines of the row's account that the run's
+// method reads and that pass more.
+func bsAccountLines(more ...*reports.Node) []*reports.Node {
+	return append([]*reports.Node{
+		op("eq", link("account"), tdAccount()),
+		basisFilter(),
+		op("eq", fld("posted"), lit(true)),
+	}, more...)
+}
+
+// bsStoredBefore are the stored months of the row's account before the month
+// of day x. They hold ledger lines only, so only an Accrual run reads them.
+func bsStoredBefore(x *reports.Node) []*reports.Node {
+	return []*reports.Node{
+		op("eq", link("account"), tdAccount()),
+		op("lt", fld("month_start"), op("startOfMonth", x)),
+		onAccrual(),
+	}
+}
+
+// bsBefore is the row's account's balance at the start of the month of day x:
+// on Accrual from the stored months (books_account_balances), so its cost does
+// not grow with the account's history; on Cash from the lines, because no
+// stored month holds the cash rows.
+func bsBefore(x *reports.Node) *reports.Node {
+	return op("add",
+		glSum("books.account_balances", bsSigned("debit_total", "credit_total"), bsStoredBefore(x)...),
+		glSum("books.basis_lines", bsSigned("debit", "credit"),
+			bsAccountLines(onCash(), op("lt", fld("date"), op("startOfMonth", x)))...),
+	)
+}
+
+// bsBalanceAt is the row's account's balance on day x when the report's rows
+// are not the lines of x's month: the balance before that month and the
+// month's lines up to x.
+func bsBalanceAt(x *reports.Node) *reports.Node {
+	return op("add", bsBefore(x), glSum("books.basis_lines", bsSigned("debit", "credit"),
+		bsAccountLines(op("gte", fld("date"), op("startOfMonth", x)), op("lte", fld("date"), x))...))
+}
+
+// bsActivity is above 0 when the row's account has a posted line on or before
+// day x, the day of the report's rows: a stored month exists only for a month
+// with a posted line.
+func bsActivity(x *reports.Node) *reports.Node {
+	earlier := &reports.Node{
 		Op:     "lookup",
 		Source: "books.basis_lines",
-		Where:  op("and", basisFilter(), op("eq", fld("posted"), lit(true)), op("lte", fld("date"), x), which),
-		Value:  mustJSON(op("sub", fld("credit"), fld("debit"))),
-		Pick:   "sum",
-	}, lit(0))
+		Where:  op("and", bsAccountLines(onCash(), op("lt", fld("date"), op("startOfMonth", x)))...),
+		Value:  mustJSON(fld("date")),
+		Pick:   "count",
+	}
+	return op("add",
+		op("add", tbStoredCount(bsStoredBefore(x)...), op("coalesce", earlier, lit(0))),
+		op("count", fld("date")),
+	)
 }
diff --git a/server/report_sources.go b/server/report_sources.go
index fcc057d2..27bbf491 100644
--- a/server/report_sources.go
+++ b/server/report_sources.go
@@ -246,7 +246,9 @@ func basisLinesSource() reports.Source {
 			{Key: "memo", Label: "Memo", Type: reports.Text},
 			{Key: "date", Label: "Date", Type: reports.Date, TextDay: true, Aggregates: reports.MinMax},
 			{Key: "posted", Label: "Posted", Type: reports.Bool},
-			{Key: "currency", Label: "Currency", Type: reports.Text},
+			// Both parts carry their journal line's currency, so a statement adds
+			// the stored months (books.account_balances) to these lines.
+			{Key: "currency", Label: "Currency", Type: reports.Text, SameCurrencyAs: "books.journal_lines.currency"},
 			{Key: "position", Label: "Position", Type: reports.Number},
 			{Key: "layer", Label: "Layer", Type: reports.Select, Options: []reports.Option{
 				{Value: "ledger", Label: "Ledger"},
```

Why each part is as it is:
- `Filter` gains `date >= startOfMonth(asOf)` on both methods, so an opened account reads the same on Accrual and on Cash: the opening row, then the month's lines.
- `current` and `prior` keep the stored months out of `Formula`: core refuses a rows total over a looked-up amount. `Rollup` is the account row's cell; every row above adds the account rows shown.
- A lookup with `onAccrual()` reads no row on Cash, and one with `onCash()` none on Accrual, so `bsBefore` is right on both with no `if`.
- `prior` has no rows of its own (the report's rows are after the prior day), so it is `bsBalanceAt(dPrior)`: before the prior day's month, plus that month's lines up to the prior day.
- The opening row's `prior` is the account's whole Prior year, because the lines below it each show 0 there: the opening row and the lines add up to the account row in both columns.
- Rows = Active keeps its meaning (the account has a posted line on or before the day): `bsActivity` counts the stored months, the earlier cash-run lines and the month's rows.

- [ ] **Step 4: Run the new tests to see them pass**

Run: `go test -run 'TestBalanceSheetStoredMonthsEqualTheLines|TestBalanceSheetOfAMonthWithNoLine|TestBalanceSheetReadsTheStoredMonths|TestBalanceSheetOpenedAccountRows' .`
Expected: `ok  tinycld.org/packages/books` (about 60 s; the cross-check is 2 x 2 x 14 x 2 pairs of runs).

- [ ] **Step 5: Run the old Balance Sheet tests to see the two that must change**

Run: `go test -run 'TestBalanceSheet|TestCashBasisReport' .`
Expected: FAIL in exactly five tests. Two list opened lines: `TestBalanceSheetOpenedLines` (`Operating's opened lines = []`) and `TestBalanceSheetPagesOpenedLines` (`the 8 lines came on 0 page(s)`): their lines are dated before November 2026, the month of `bsToday`, so they are now in the opening row. Three pin the shipped revision 3: `TestBalanceSheetSpecLoads` (`revision 4, unpublished true; want revision 3`), `TestBalanceSheetShippedUnpublished` (`shipped_revision:4`), `TestCashBasisReport_ShippedSpecs` (`revision 4, want 3`). Every other test passes unchanged, the other `TestCashBasisReport_*` tests too. Any other failure: stop and report it.

- [ ] **Step 6: Move the two tests' lines into the as-of day's month, and pin revision 4**

Apply this diff to `server/report_balance_sheet_test.go` and `server/report_cash_basis_test.go` with the Edit tool, then run `go test -run 'TestBalanceSheet|TestCashBasisReport' .` again. Expected: `ok  tinycld.org/packages/books`.

```diff
diff --git a/server/report_balance_sheet_test.go b/server/report_balance_sheet_test.go
index 3e53ede0..63fa44f2 100644
--- a/server/report_balance_sheet_test.go
+++ b/server/report_balance_sheet_test.go
@@ -243,8 +243,8 @@ func TestBalanceSheetSpecLoads(t *testing.T) {
 	if shipped.Description != "What the business owns and owes on one day, beside the same day a year earlier." {
 		t.Errorf("description = %q", shipped.Description)
 	}
-	if shipped.Revision != 3 || !shipped.Unpublished {
-		t.Errorf("revision %d, unpublished %v; want revision 3 (the Method param), shipped unpublished", shipped.Revision, shipped.Unpublished)
+	if shipped.Revision != 4 || !shipped.Unpublished {
+		t.Errorf("revision %d, unpublished %v; want revision 4 (the stored months), shipped unpublished", shipped.Revision, shipped.Unpublished)
 	}
 	built, err := json.Marshal(balanceSheetSpec())
 	if err != nil {
@@ -1164,7 +1164,7 @@ func TestBalanceSheetShippedUnpublished(t *testing.T) {
 	if row.GetBool("published") || row.GetString("published_by") != "" {
 		t.Errorf("published = %v (by %q), want unpublished", row.GetBool("published"), row.GetString("published_by"))
 	}
-	if row.GetString("title") != "Balance Sheet" || row.GetInt("shipped_revision") != 3 || row.GetString("source") != "books.basis_lines" {
+	if row.GetString("title") != "Balance Sheet" || row.GetInt("shipped_revision") != 4 || row.GetString("source") != "books.basis_lines" {
 		t.Errorf("row = %v", row.PublicExport())
 	}
 	if err := reconcileBooksReportShares(app); err != nil {
@@ -1258,21 +1258,25 @@ func TestBalanceSheetOpenedLines(t *testing.T) {
 	owner := b.account(t, "Owner's equity", "3000", "equity", "owners_equity")
 	supplies := b.account(t, "Supplies", "6200", "expense", "office_general")
 	mkRetainedEarnings(t, b.app, "3900")
-	// On or before the prior-year day (2025-11-04).
-	e1 := b.postIDs(t, "2025-10-01", "Owner deposit",
+	// On or before the prior-year day (2025-11-04), and one in an earlier
+	// month of this year: both are in the opening row, not listed.
+	b.postIDs(t, "2025-10-01", "Owner deposit",
 		idLine{id: "bsopenline00003", account: checking, debit: 1000, memo: "Before the prior day"},
 		idLine{account: owner, credit: 1000})
+	b.postIDs(t, "2026-03-10", "March deposit",
+		idLine{id: "bsopenline00004", account: checking, debit: 150},
+		idLine{account: owner, credit: 150})
 	// Same day, posted in this order: e2 has the lower sequence, e3 the lower id.
-	e2 := b.postIDs(t, "2026-03-10", "March deposit",
+	e2 := b.postIDs(t, "2026-11-02", "November deposit",
 		idLine{id: "bsopenline00001", account: checking, debit: 2500},
 		idLine{account: owner, credit: 2500})
-	e3 := b.postIDs(t, "2026-03-10", "Card payment entry",
+	e3 := b.postIDs(t, "2026-11-02", "Card payment entry",
 		idLine{id: "bsopenline00000", account: checking, credit: 400, memo: "Card payment"},
 		idLine{account: owner, debit: 400})
-	e4 := b.postIDs(t, "2026-01-05", "January entry",
-		idLine{id: "bsopenline00002", account: checking, debit: 300, memo: "January"},
+	e4 := b.postIDs(t, "2026-11-01", "First entry",
+		idLine{id: "bsopenline00002", account: checking, debit: 300, memo: "First"},
 		idLine{account: owner, credit: 300})
-	e5 := b.postIDs(t, "2026-04-01", "Card purchase",
+	e5 := b.postIDs(t, "2026-11-03", "Card purchase",
 		idLine{account: supplies, debit: 4500},
 		idLine{account: visa, credit: 4500})
 
@@ -1305,13 +1309,25 @@ func TestBalanceSheetOpenedLines(t *testing.T) {
 		current, prior int64
 	}
 	wants := []want{
-		{bsLineLabel("2025-10-01", e1, "Before the prior day"), 1000, 1000},
-		{bsLineLabel("2026-01-05", e4, "January"), 300, 0},
+		{bsLineLabel("2026-11-01", e4, "First"), 300, 0},
 		// No line memo: the entry's.
-		{bsLineLabel("2026-03-10", e2, "March deposit"), 2500, 0},
+		{bsLineLabel("2026-11-02", e2, "November deposit"), 2500, 0},
 		// A bank account credited reads negative by its normal side.
-		{bsLineLabel("2026-03-10", e3, "Card payment"), -400, 0},
+		{bsLineLabel("2026-11-02", e3, "Card payment"), -400, 0},
 	}
+	// The months before November are one row: 1000 + 150, of which 1000 is on
+	// or before the prior-year day. It and the lines add up to the account row.
+	var opening []reports.Row
+	for _, r := range open.Rows {
+		if r.Kind == reports.KindOpening && r.Section == bsAccount(t, open, checking).ID {
+			opening = append(opening, r)
+		}
+	}
+	if len(opening) != 1 || opening[0].LabelTemplate != "Balance before {value}" || opening[0].LabelValue != "2026-11" {
+		t.Fatalf("Operating's opening rows = %#v, want one \"Balance before {value}\" of month 2026-11", opening)
+	}
+	requireFigures(t, opening[0], 1150, 1000)
+	requireFigures(t, bsAccount(t, open, checking), 3550, 1000)
 	var got []string
 	for _, l := range lines {
 		got = append(got, l.Label)
@@ -1328,8 +1344,8 @@ func TestBalanceSheetOpenedLines(t *testing.T) {
 	}
 
 	visaLines := bsLines(open, bsAccount(t, open, visa))
-	if len(visaLines) != 1 || visaLines[0].Label != bsLineLabel("2026-04-01", e5, "Card purchase") {
-		t.Fatalf("Visa's opened lines = %q, want one labelled %q", bsLabels(&reports.Result{Rows: visaLines}), bsLineLabel("2026-04-01", e5, "Card purchase"))
+	if len(visaLines) != 1 || visaLines[0].Label != bsLineLabel("2026-11-03", e5, "Card purchase") {
+		t.Fatalf("Visa's opened lines = %q, want one labelled %q", bsLabels(&reports.Result{Rows: visaLines}), bsLineLabel("2026-11-03", e5, "Card purchase"))
 	}
 	// A credit card credited 45.00 reads 45.00 (a liability's normal side).
 	requireFigures(t, visaLines[0], 4500, 0)
@@ -1356,8 +1372,10 @@ func TestBalanceSheetPagesOpenedLines(t *testing.T) {
 	owner := b.account(t, "Owner's equity", "3000", "equity", "owners_equity")
 	mkRetainedEarnings(t, b.app, "3900")
 	var want []string
+	// The lines are in the as-of day's month (two a day, November 1 to 4):
+	// an earlier month's lines are one opening row.
 	for k := 1; k <= 8; k++ {
-		date := fmt.Sprintf("2026-08-0%d", k)
+		date := fmt.Sprintf("2026-11-0%d", (k+1)/2)
 		memo := fmt.Sprintf("Line %d", k)
 		entry := b.postIDs(t, date, "Paging",
 			idLine{id: fmt.Sprintf("bspageline0000%d", 9-k), account: petty, debit: 100 * k, memo: memo},
diff --git a/server/report_cash_basis_test.go b/server/report_cash_basis_test.go
index ed78491f..304d5605 100644
--- a/server/report_cash_basis_test.go
+++ b/server/report_cash_basis_test.go
@@ -323,13 +323,14 @@ func TestCashBasisReport_ShippedSpecs(t *testing.T) {
 	}{
 		{profitAndLossShipped(), 3, "period"},
 		{profitAndLossByMonthShipped(), 3, "period"},
-		{balanceSheetShipped(), 3, "asOf"},
+		// 3 for the Method param, 4 for the stored months.
+		{balanceSheetShipped(), 4, "asOf"},
 		{transactionDetailShipped(), 2, "period"},
 	}
 	for _, c := range cases {
 		t.Run(c.shipped.Title, func(t *testing.T) {
 			if c.shipped.Revision != c.revision {
-				t.Errorf("revision %d, want %d (one more for the Method param)", c.shipped.Revision, c.revision)
+				t.Errorf("revision %d, want %d (the Method param's, or a later one)", c.shipped.Revision, c.revision)
 			}
 			if strings.Contains(strings.ToLower(c.shipped.Description), "accrual basis") {
 				t.Errorf("description %q still says \"accrual basis\"", c.shipped.Description)
```

- [ ] **Step 7: Prove the cross-check can fail**

Make each edit below alone, run the named test, see it FAIL, and undo the edit with the Edit tool:
1. In `bsStoredBefore`, change `op("lt", fld("month_start"), ...)` to `op("lte", ...)`. Run `go test -run 'TestBalanceSheetStoredMonthsEqualTheLines' .`: FAIL (the as-of month counts twice).
2. In `report_sources.go`, take `SameCurrencyAs` off the `currency` field of `basisLinesSource`. Run `go test -run 'TestBalanceSheetSpecLoads' .`: FAIL with `amounts in different currencies cannot be combined`.
3. In `profitAndLossSum`, drop `onAccrual()` from the stored lookup. Run `go test -run 'TestBalanceSheetStoredMonthsEqualTheLines' .`: FAIL on a `cash` run (Retained earnings counts the stored months and the lines).
4. In `bsBefore`, drop `onCash()` from the lines lookup. Run `go test -run 'TestBalanceSheetStoredMonthsEqualTheLines$' .`: FAIL on an `accrual` run (the earlier months count twice).
5. In `bsAccountLines`, drop `basisFilter()`. Run `go test -run 'TestBalanceSheetStoredMonthsEqualTheLinesWithCashRows' .`: FAIL on an `accrual` run (the Prior year of 2027 counts the cash rows).

Write the five outputs' failing lines in your report. A mutation that does not fail its test is a finding: report it, do not weaken the step.

- [ ] **Step 8: Run the package and commit**

Run: `gofmt -l . ; go vet . ; go test .`
Expected: no file from `gofmt`, no finding from `go vet`, `ok  tinycld.org/packages/books`.

Commit only when your dispatch prompt permits it; else name the files and the message and leave the commit to the driver.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_balance_sheet.go server/report_sources.go server/report_balance_sheet_stored_test.go server/report_balance_sheet_test.go server/report_cash_basis_test.go
git commit -m "feat(reports): the Balance Sheet reads the stored months on Accrual"
```

## Self-review

- Coverage of 9c-2b in this part: the spec, the Method branch, the Prior year lookups, Retained earnings and Net income on stored months, the fill, the opening row, the cross-check file (figures and currency codes, 14 days, a month with no line, a class with no line in the month, both methods, both Rows choices, two fiscal starts), the changed tests. No golden file exists for the Balance Sheet spec (`testdata` has none), so none changes.
- Left for part 22: the e2e change (`books/tests/e2e/books-report-helpers.ts` `reportLinesUnder` stops at the opening row, so `balance-sheet.spec.ts` "pressing an account opens its posted lines" fails until it skips that row), the help text (`books/help/reports.md`, "Read the Balance Sheet"), the 100k `TestStatementsScale` NEEDS-RUN, the ledger record.
