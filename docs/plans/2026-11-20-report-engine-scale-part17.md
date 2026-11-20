# Report Engine Scale, Part 17: The Trial Balance Reads The Stored Months (Go) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Books Trial Balance reads the months before the as-of day's month from the stored month balances (`books_account_balances`) and only that month's posted lines, so its cost does not grow with the book's history.

**Architecture:** The report's rows are the posted lines of the as-of day's month. Debit and Credit carry a `Rollup` that adds a lookup over the stored months to the month's lines; `Total: "rows"` adds the account rows shown; `Fill` gives a row to an account with stored months and no line in the month. An opened account shows one opening row, "Balance before <month>", then the month's lines. A cross-check test runs the old line-read spec beside the new one.

**Tech Stack:** Go (Books server package `tinycld.org/packages/books`, module at `/Users/nas/tmp/tinycld/books-purchases/books/server`), core report engine `tinycld.org/core/reports` (built: rollup with a rows total, the Total over fill rows, the opening label with its month).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (the last three "State after…" sections) and `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-plan-part15.md` (the 9c-1 list). Human decision (2026-11-20): an opened account shows "Balance before <month>" plus the as-of month's lines; the drill-down still lists every line.

**This part holds task 9c-1a only.** Task 9c-1b (the e2e change, the Books help line, the 100k NEEDS-RUN and the ledger step) is the next part; it is listed at the end.

## Global Constraints

- Trees: core `d61fe27d`, books `6135abe7`, branch `books/p5-report-engine-scale` in both. This task changes Books only; it changes no core file.
- Bars at 500,000 lines (human, 2026-11-20): Trial Balance with an Accounts filter 1 s or less; every account 2 s or less. This task makes no timing run.
- The figures do not change: every account row, its currency codes and the Total equal those of the line read, for every as-of day, Rows choice and account list.
- Keep the stored-months lookup out of a column's `Formula`: core refuses a rows total over a looked-up amount. It goes in `Rollup` and in `Opening.Cells`.
- A `labelBucket` needs a Date label value: `LabelValue` is the `asOf` param.
- The shipped revision becomes 3. Not live: no upgrade path and no old-format reader.
- Rules of the lane: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md`. Change files with the Edit or Write tool. The driver commits unless the step says to.

## Review Focus

1. An as-of day in a month with no posted line: every account row and the Total still show, in USD. Test: `TestTrialBalanceOfAMonthWithNoLine`.
2. An as-of day in the middle of a month: lines after the day do not count, lines of earlier months come from the stored months. Test: `TestTrialBalanceStoredMonthsEqualTheLines` (2026-03-05, 2026-03-19), `TestTrialBalanceOpenedAccountRows`.
3. Income and expense of earlier fiscal years on Retained earnings, with a fiscal year that starts in July: `TestTrialBalanceStoredMonthsEqualTheLines` (fiscal start 1 and 7).
4. An account with no posted line, and an inactive account with and without lines, under each Rows choice: the cross-check book's Idle, Closed and Dormant accounts.
5. An as-of day before the first posted line (2024-12-31): no row, and the same Total as the line read.

---

### Task 1 (9c-1a): Books: the Trial Balance reads the stored months

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance.go` (the whole file is replaced)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance_test.go`
- Create: `/Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance_stored_test.go`
- Create: `/Users/nas/tmp/tinycld/books-purchases/books/server/testdata/trial_balance_spec.golden.json`

**Interfaces:**
- Consumes (core, built): `reports.Column{Formula, Total: reports.TotalRows, Rollup}`; `reports.Fill{Where}`; `reports.Opening{Label, LabelValue *Node, LabelBucket string, Cells}`; `reports.LabelValueToken` (`"{value}"`); `reports.KindOpening`; `Row.LabelTemplate`, `Row.LabelType`, `Row.LabelValue`; `reports.Month`.
- Consumes (Books, exist): `glSum(source, value, where...)`, `glStoredNet()`, `glNet()`, `firstLine`, `firstLineFY`, `tdDrillTarget`, `tdAccount`, `accountsParam`, `typeRankCase`, `lineDescriptionFormula`, `entryOpenPath`, `profitAndLossTypes`, `op`, `fld`, `lit`, `prm`, `rec`, `col`, `link`, `mustJSON`; test helpers `newTBBook`, `runTB`, `rowFor`, `totalOf`, `requireSplit`, `moneyCell`, `labelsOf`, `idsOf`, `mkRetainedEarnings`, `saveSettings`, `saveOrFail`, `ln`, `tbToday`.
- Produces (for 9c-1b and 9c-2): `trialBalanceBeforeLabel` (`"Balance before {value}"`), `tbStoredWhere() []*reports.Node`, `tbEarlierPLWhere() []*reports.Node`, `onRetainedEarnings(x *reports.Node) *reports.Node`, `trialBalanceBefore() *reports.Node`, `trialBalanceNet() *reports.Node`, `tbStoredCount(where ...*reports.Node) *reports.Node`, `trialBalanceActivity() *reports.Node`, `debitSide(x)`, `creditSide(x)`; shipped revision 3; test helpers `trialBalanceSpecFromLines() *reports.Spec`, `runTBSpec(t, b tbBook, spec, params, opts) *reports.Result`, `tbFigures(res) string`, `newTBStoredBook(t) tbStoredBook`.

What the reader sees: the account rows and the Total are as before. An opened account shows first a row "Balance before Nov 2026" (the month of the As of day, in the reader's words; core's client puts the month in place of `{value}`), then the posted lines of that month up to the day. Lines of earlier months are no longer listed there; a press on the Debit or Credit figure still opens Transaction Detail by Account with every line. The opened Retained earnings row no longer lists prior-year lines: its opening row carries them.

- [ ] **Step 1: Write the new test file**

Create `/Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance_stored_test.go`:

```go
package books

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/reports"
)

// trialBalanceSpecFromLines is the Trial Balance as it was before it read the
// stored months: every figure is a sum over the account's posted lines up to
// the as-of day, and an earlier year's income or expense line is grouped onto
// the retained-earnings account. The stored read must show what it shows.
func trialBalanceSpecFromLines() *reports.Spec {
	net := op("sub", fld("debit"), fld("credit"))
	priorPL := op("and",
		op("in", fld("account", "type"), lit(profitAndLossTypes())),
		op("lt", op("fiscalYear", fld("date")), op("fiscalYear", prm("asOf"))),
	)
	reAccount := &reports.Node{
		Op:     "lookup",
		Source: "books.accounts",
		Where:  op("eq", fld("detail_type"), lit("retained_earnings")),
		Value:  mustJSON(fld("id")),
		Pick:   "min",
	}
	spec := trialBalanceSpec()
	spec.Filter = op("and",
		op("eq", fld("posted"), lit(true)),
		op("lte", fld("date"), prm("asOf")),
		op("in", col("account"), prm("accounts")),
	)
	for i := range spec.Columns {
		c := &spec.Columns[i]
		c.Drill, c.Rollup = nil, nil
		switch c.ID {
		case "account":
			c.Path, c.Formula = nil, op("if", priorPL, reAccount, link("account"))
		case "debit":
			c.Formula = op("if",
				op("eq", op("count", fld("id")), lit(0)),
				lit(nil),
				op("if", op("gte", op("sum", net), lit(0)), op("sum", net), lit(nil)),
			)
		case "credit":
			c.Formula = op("if", op("lt", op("sum", net), lit(0)), op("sub", lit(0), op("sum", net)), lit(nil))
		}
	}
	g := &spec.Groups[0]
	g.Opening = nil
	g.Fill = &reports.Fill{Where: op("and",
		op("eq", fld("inactive"), lit(false)),
		op("in", fld("id"), prm("accounts")),
	)}
	g.Having = op("or",
		op("eq", prm("rows"), lit("all")),
		op("and", op("eq", prm("rows"), lit("active")), op("gt", op("count", fld("id")), lit(0))),
		op("ne", op("sum", net), lit(0)),
	)
	return spec
}

// runTBSpec runs spec as reader with the posted params.
func runTBSpec(t *testing.T, b tbBook, spec *reports.Spec, params map[string]any, opts reports.Options) *reports.Result {
	t.Helper()
	loaded, info, err := reports.Load(mustJSON(spec))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	given := map[string]json.RawMessage{}
	for id, v := range params {
		given[id] = mustJSON(v)
	}
	res, err := reports.Run(context.Background(), b.app, reports.Request{
		Spec: loaded, Loaded: info, Params: given, Options: opts, Auth: b.reader,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res
}

// tbFigures writes each account row and the Total as one line: its kind,
// label, number, and the Debit and Credit figures with their currency codes.
// A cell the row does not have reads <nil>.
func tbFigures(res *reports.Result) string {
	var out strings.Builder
	for _, r := range res.Rows {
		if r.Kind != reports.KindGroup && r.Kind != reports.KindTotal {
			continue
		}
		fmt.Fprintf(&out, "%s|%s|%v|%v %v|%v %v\n", r.Kind, r.Label, r.Cells["number"],
			r.Cells["debit"], r.Cells["debit@currency"], r.Cells["credit"], r.Cells["credit@currency"])
	}
	return out.String()
}

// tbStoredBook has two fiscal years of posted lines: income and expense in
// 2025 (they fold onto Retained earnings in 2026), months with lines, and no
// line in May, October or November 2026. Idle has no line; Closed is inactive
// and has lines of 2025.
type tbStoredBook struct {
	tbBook
	checking, savings, income, rent, retained, idle, closed *core.Record
}

func newTBStoredBook(t *testing.T) tbStoredBook {
	t.Helper()
	b := tbStoredBook{tbBook: newTBBook(t)}
	b.checking = b.account(t, "Checking", "1000", "bank", "checking")
	b.savings = b.account(t, "Savings", "1010", "bank", "savings")
	b.idle = b.account(t, "Idle", "1020", "bank", "checking")
	b.closed = b.account(t, "Closed", "1030", "bank", "checking")
	b.retained = mkRetainedEarnings(t, b.app, "3900")
	b.income = b.account(t, "Consulting", "4000", "income", "service_income")
	b.rent = b.account(t, "Rent", "6000", "expense", "rent_lease")
	b.post(t, "2025-03-10", ln{account: b.checking, debit: 90000}, ln{account: b.income, credit: 90000})
	b.post(t, "2025-11-20", ln{account: b.rent, debit: 25000}, ln{account: b.checking, credit: 25000})
	b.post(t, "2025-12-31", ln{account: b.closed, debit: 700}, ln{account: b.checking, credit: 700})
	b.post(t, "2026-01-15", ln{account: b.checking, debit: 40000}, ln{account: b.income, credit: 40000})
	b.post(t, "2026-03-05", ln{account: b.rent, debit: 12000}, ln{account: b.checking, credit: 12000})
	b.post(t, "2026-03-20", ln{account: b.savings, debit: 30000}, ln{account: b.checking, credit: 30000})
	b.post(t, "2026-04-30", ln{account: b.checking, debit: 5000}, ln{account: b.savings, credit: 5000})
	b.post(t, "2026-06-01", ln{account: b.rent, debit: 12000}, ln{account: b.checking, credit: 12000})
	// Savings is back at 0 from September on: a row of zero with posted lines.
	b.post(t, "2026-09-09", ln{account: b.checking, debit: 25000}, ln{account: b.savings, credit: 25000})
	// Dormant is inactive with no line: no Rows choice shows it.
	dormant := b.account(t, "Dormant", "1040", "bank", "checking")
	for _, inactive := range []*core.Record{b.closed, dormant} {
		inactive.Set("inactive", true)
		saveOrFail(t, b.app, inactive)
	}
	return b
}

// The stored read shows what the line read shows: the same account rows in the
// same order, the same Debit and Credit with the same currency codes, and the
// same Total. The days are a month's first, middle and last day, a day in a
// month with no line (May, November), a day before the first line, and the
// last day of the first fiscal year; each with every Rows choice, for every
// account and for an account list.
func TestTrialBalanceStoredMonthsEqualTheLines(t *testing.T) {
	for _, fiscalStart := range []int{1, 7} {
		b := newTBStoredBook(t)
		saveSettings(t, b.app, map[string]any{"fiscal_year_start_month": fiscalStart})
		opts := reports.Options{Today: tbToday}
		some := idsOf(b.checking, b.rent, b.retained, b.idle)
		for _, asOf := range []string{"2024-12-31", "2025-12-31", "2026-03-01", "2026-03-05", "2026-03-19", "2026-03-31", "2026-05-31", "2026-06-30", "2026-07-01", tbToday} {
			for _, rows := range []string{"nonzero", "active", "all"} {
				for name, accounts := range map[string][]string{"every account": nil, "an account list": some} {
					params := map[string]any{"asOf": asOf, "rows": rows}
					if accounts != nil {
						params["accounts"] = accounts
					}
					stored := tbFigures(runTBSpec(t, b.tbBook, trialBalanceSpec(), params, opts))
					lines := tbFigures(runTBSpec(t, b.tbBook, trialBalanceSpecFromLines(), params, opts))
					if stored != lines {
						t.Errorf("fiscal start %d, as of %s, rows %s, %s:\nstored months\n%s\nlines\n%s", fiscalStart, asOf, rows, name, stored, lines)
					}
					if asOf == tbToday && rows == "nonzero" && !strings.Contains(stored, "total|Total|") {
						t.Errorf("fiscal start %d, %s: no Total row in\n%s", fiscalStart, name, stored)
					}
				}
			}
		}
	}
}

// A month with no posted line still has every figure: each account row comes
// from its stored months, and the Total adds them, in the book's currency.
func TestTrialBalanceOfAMonthWithNoLine(t *testing.T) {
	b := newTBStoredBook(t)
	res := runTB(t, b.app, b.reader, map[string]any{"asOf": "2026-05-31"}, reports.Options{Today: tbToday})
	requireSplit(t, rowFor(t, res, b.checking), int64(67300), nil)
	requireSplit(t, rowFor(t, res, b.savings), int64(25000), nil)
	requireSplit(t, rowFor(t, res, b.income), nil, int64(40000))
	requireSplit(t, rowFor(t, res, b.retained), nil, int64(65000))
	total := totalOf(t, res)
	requireSplit(t, total, int64(105000), int64(105000))
	for _, cell := range []string{"debit@currency", "credit@currency"} {
		if got := total.Cells[cell]; got != "USD" {
			t.Errorf("Total %s = %#v, want USD", cell, got)
		}
	}
}

// The months before the as-of day's month are read from the stored months and
// that month from its lines: a stored month's change shows on the account row
// only when the month is an earlier one.
func TestTrialBalanceReadsTheStoredMonths(t *testing.T) {
	b := newTBStoredBook(t)
	debit := func() int64 {
		t.Helper()
		res := runTB(t, b.app, b.reader, map[string]any{"asOf": "2026-06-30"}, reports.Options{Today: tbToday})
		v, ok := moneyCell(t, rowFor(t, res, b.checking), "debit")
		if !ok {
			t.Fatal("Checking has no Debit")
		}
		return v
	}
	bump := func(period string) {
		t.Helper()
		_, err := b.app.DB().NewQuery("UPDATE books_account_balances SET debit_total = debit_total + 5 WHERE account = {:a} AND period = {:p}").
			Bind(map[string]any{"a": b.checking.Id, "p": period}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
	before := debit()
	bump("2026-04")
	if got := debit(); got != before+5 {
		t.Errorf("after April's stored debit rose by 5, Checking = %d, want %d: April is read from its stored month", got, before+5)
	}
	bump("2026-06")
	if got := debit(); got != before+5 {
		t.Errorf("after June's stored debit rose by 5, Checking = %d, want %d: the as-of day's month is read from its lines", got, before+5)
	}
}

// An opened account shows one row for the months before the as-of day's month,
// labelled with that month, then the month's lines up to the day. The opening
// row's figure and the lines add up to the account's row.
func TestTrialBalanceOpenedAccountRows(t *testing.T) {
	b := newTBStoredBook(t)
	opts := reports.Options{Today: tbToday}
	params := map[string]any{"asOf": "2026-03-19", "rows": "all"}
	closed := runTB(t, b.app, b.reader, params, opts)
	opts.Expand = [][]any{rowFor(t, closed, b.checking).GroupKey, rowFor(t, closed, b.idle).GroupKey, rowFor(t, closed, b.retained).GroupKey}
	open := runTB(t, b.app, b.reader, params, opts)

	under := func(account *core.Record) []reports.Row {
		t.Helper()
		section := rowFor(t, open, account).ID
		var rows []reports.Row
		for _, r := range open.Rows {
			if r.Section == section && (r.Kind == reports.KindOpening || r.Kind == reports.KindDetail) {
				rows = append(rows, r)
			}
		}
		return rows
	}
	requireOpening := func(r reports.Row, debit, credit any) {
		t.Helper()
		if r.Kind != reports.KindOpening || r.LabelTemplate != "Balance before {value}" || r.LabelType != reports.Month || r.LabelValue != "2026-03" {
			t.Fatalf("first row = %s %q (template %q, type %q, value %#v); want the opening row \"Balance before {value}\" of month 2026-03",
				r.Kind, r.Label, r.LabelTemplate, r.LabelType, r.LabelValue)
		}
		if r.Open != "" {
			t.Errorf("the opening row opens %q, want no open", r.Open)
		}
		requireSplit(t, r, debit, credit)
	}

	// Checking: 90000 - 25000 - 700 + 40000 before March, then the line of
	// March 5. The line of March 20 is after the day.
	checking := under(b.checking)
	if len(checking) != 2 {
		t.Fatalf("Checking's opened rows = %q, want the opening row and one line", labelsOf(checking))
	}
	requireOpening(checking[0], int64(104300), nil)
	if !strings.HasPrefix(checking[1].Label, "2026-03-05 · ") {
		t.Errorf("Checking's line = %q, want the line of 2026-03-05", checking[1].Label)
	}
	requireSplit(t, checking[1], nil, int64(12000))
	requireSplit(t, rowFor(t, open, b.checking), int64(92300), nil)

	// Retained earnings has no line of its own: its opening row is the income
	// and expense of 2025, and no line shows under it.
	retained := under(b.retained)
	if len(retained) != 1 {
		t.Fatalf("Retained earnings' opened rows = %q, want the opening row only", labelsOf(retained))
	}
	requireOpening(retained[0], nil, int64(65000))

	// An account with no posted line has an opening row of 0.
	idle := under(b.idle)
	if len(idle) != 1 {
		t.Fatalf("Idle's opened rows = %q, want the opening row only", labelsOf(idle))
	}
	requireOpening(idle[0], int64(0), nil)
}
```

- [ ] **Step 2: Change the existing tests**

In `/Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance_test.go` make the changes of this diff with the Edit tool (three tests post their lines in November 2026, the as-of day's month, because an opened account now lists that month's lines only; the revision is 3; the golden is one file):

```diff
diff --git a/server/report_trial_balance_test.go b/server/report_trial_balance_test.go
index 56d2308a..132b89df 100644
--- a/server/report_trial_balance_test.go
+++ b/server/report_trial_balance_test.go
@@ -4,6 +4,7 @@ import (
 	"context"
 	"encoding/json"
 	"maps"
+	"os"
 	"reflect"
 	"slices"
 	"strings"
@@ -259,8 +260,8 @@ func TestTrialBalanceSpecLoads(t *testing.T) {
 	if shipped.Description != "Each account's balance on one day, as a debit or a credit." {
 		t.Errorf("description = %q", shipped.Description)
 	}
-	if shipped.Revision != 2 || !shipped.Unpublished {
-		t.Errorf("revision %d, unpublished %v; want revision 2 (detail.open), shipped unpublished", shipped.Revision, shipped.Unpublished)
+	if shipped.Revision != 3 || !shipped.Unpublished {
+		t.Errorf("revision %d, unpublished %v; want revision 3 (the stored months), shipped unpublished", shipped.Revision, shipped.Unpublished)
 	}
 	built, err := json.Marshal(trialBalanceSpec())
 	if err != nil {
@@ -269,27 +270,12 @@ func TestTrialBalanceSpecLoads(t *testing.T) {
 	if !jsonEqual(t, shipped.Spec, string(built)) {
 		t.Errorf("the shipped spec is not trialBalanceSpec()")
 	}
-	// The helpers moved to report_spec_helpers.go (general-ledger checkpoint 2);
-	// the Trial Balance's spec stays byte for byte what it was before the move.
-	// Report drill-down checkpoint 1 adds only detail.open (entryOpenPath()), the
-	// last key of detail, and moves accounts to accountsParam() unchanged.
-	if entryOpenPath() == nil {
-		t.Errorf("entryOpenPath() = nil, want the entry route formula on the Trial Balance's opened lines")
-	}
-	const detailEnd = `}},"display":{`
-	if strings.Count(trialBalanceGolden, detailEnd) != 1 {
-		t.Fatalf("the golden does not hold %s once", detailEnd)
-	}
-	golden := strings.Replace(trialBalanceGolden, detailEnd, `},"open":`+string(mustJSON(entryOpenPath()))+detailEnd[1:], 1)
-	// Report drill-down checkpoint 2 adds only the drill of Debit and Credit,
-	// the last key of each of those two columns (Debit first).
-	const figureEnd = `"total":"rows"}`
-	if strings.Count(golden, figureEnd) != 2 {
-		t.Fatalf("the golden does not hold %s twice (Debit and Credit)", figureEnd)
-	}
-	golden = strings.Replace(golden, figureEnd, `"total":"rows","drill":`+trialBalanceDebitDrillGolden+`}`, 1)
-	golden = strings.Replace(golden, figureEnd, `"total":"rows","drill":`+trialBalanceCreditDrillGolden+`}`, 1)
-	if got := string(mustJSON(trialBalanceSpec())); got != golden {
+	// The whole spec, as core reads it. A change to it is a new revision.
+	golden, err := os.ReadFile("testdata/trial_balance_spec.golden.json")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if got := string(built); got != strings.TrimSpace(string(golden)) {
 		t.Errorf("the Trial Balance spec changed:\n got %s\nwant %s", got, golden)
 	}
 
@@ -685,10 +671,12 @@ func TestTrialBalanceHeadingAndLabels(t *testing.T) {
 	design.Set("number", "6110")
 	saveOrFail(t, b.app, design)
 	mkRetainedEarnings(t, b.app, "3900")
-	march := b.postMemo(t, "2026-03-10", "March tools",
+	// Both entries are in the as-of day's month: an opened account lists the
+	// lines of that month only.
+	march := b.postMemo(t, "2026-11-02", "March tools",
 		memoLine{account: design, debit: 12345, memo: "Figma seats"},
 		memoLine{account: checking, credit: 12345})
-	april := b.postMemo(t, "2026-04-02", "Refund from vendor",
+	april := b.postMemo(t, "2026-11-03", "Refund from vendor",
 		memoLine{account: checking, debit: 500},
 		memoLine{account: design, credit: 500})
 	opts := reports.Options{Today: tbToday}
@@ -721,8 +709,8 @@ func TestTrialBalanceHeadingAndLabels(t *testing.T) {
 			}
 		}
 	}
-	figma := "2026-03-10 · " + march.GetString("number") + " · Figma seats"
-	refund := "2026-04-02 · " + april.GetString("number") + " · Refund from vendor"
+	figma := "2026-11-02 · " + march.GetString("number") + " · Figma seats"
+	refund := "2026-11-03 · " + april.GetString("number") + " · Refund from vendor"
 	if len(details) != 2 {
 		t.Fatalf("opened lines = %v, want %q and %q", slices.Collect(maps.Keys(details)), figma, refund)
 	}
@@ -765,14 +753,16 @@ func TestTrialBalanceDetailOrder(t *testing.T) {
 	checking := b.account(t, "Checking", "1000", "bank", "checking")
 	supplies := b.account(t, "Supplies", "6200", "expense", "office_general")
 	rent := b.account(t, "Rent", "6000", "expense", "rent_lease")
-	march := b.postLineWithID(t, "2026-03-10", "tbdetailorder01", "March", supplies, checking)
+	// The lines are in the as-of day's month: an opened account lists that
+	// month's lines only.
+	march := b.postLineWithID(t, "2026-11-03", "tbdetailorder01", "March", supplies, checking)
 	// Entries 2 to 8 post to other accounts, so the next two are numbered 9
 	// and 10.
 	for range 7 {
 		b.post(t, "2026-02-01", ln{account: rent, debit: 100}, ln{account: checking, credit: 100})
 	}
-	nine := b.postLineWithID(t, "2026-01-05", "tbdetailorder03", "January first", supplies, checking)
-	ten := b.postLineWithID(t, "2026-01-05", "tbdetailorder02", "January second", supplies, checking)
+	nine := b.postLineWithID(t, "2026-11-02", "tbdetailorder03", "January first", supplies, checking)
+	ten := b.postLineWithID(t, "2026-11-02", "tbdetailorder02", "January second", supplies, checking)
 	opts := reports.Options{Today: tbToday}
 
 	closed := runTB(t, b.app, b.reader, map[string]any{"asOf": tbToday}, opts)
@@ -786,9 +776,9 @@ func TestTrialBalanceDetailOrder(t *testing.T) {
 		}
 	}
 	want := []string{
-		"2026-01-05 · " + nine.GetString("number") + " · January first",
-		"2026-01-05 · " + ten.GetString("number") + " · January second",
-		"2026-03-10 · " + march.GetString("number") + " · March",
+		"2026-11-02 · " + nine.GetString("number") + " · January first",
+		"2026-11-02 · " + ten.GetString("number") + " · January second",
+		"2026-11-03 · " + march.GetString("number") + " · March",
 	}
 	if !slices.Equal(got, want) {
 		t.Fatalf("opened lines = %q\nwant (date, then entry number) %q", got, want)
@@ -852,7 +842,7 @@ func TestTrialBalanceShippedUnpublished(t *testing.T) {
 	if row.GetBool("published") || row.GetString("published_by") != "" {
 		t.Errorf("published = %v (by %q), want unpublished", row.GetBool("published"), row.GetString("published_by"))
 	}
-	if row.GetString("title") != "Trial Balance" || row.GetInt("shipped_revision") != 2 || row.GetString("source") != "books.journal_lines" {
+	if row.GetString("title") != "Trial Balance" || row.GetInt("shipped_revision") != 3 || row.GetString("source") != "books.journal_lines" {
 		t.Errorf("row = %v", row.PublicExport())
 	}
 }
@@ -863,7 +853,7 @@ func TestTrialBalanceLineOpens(t *testing.T) {
 	b := newTBBook(t)
 	checking := b.account(t, "Checking", "1000", "bank", "checking")
 	supplies := b.account(t, "Supplies", "6100", "expense", "office_general")
-	entry := b.postMemo(t, "2026-03-10", "Paper", memoLine{account: supplies, debit: 2500}, memoLine{account: checking, credit: 2500})
+	entry := b.postMemo(t, "2026-11-02", "Paper", memoLine{account: supplies, debit: 2500}, memoLine{account: checking, credit: 2500})
 	opts := reports.Options{Today: tbToday}
 
 	closed := runTB(t, b.app, b.reader, map[string]any{"asOf": tbToday}, opts)
```

Then delete the end of the same file, from the comment line that starts `// trialBalanceGolden is mustJSON(trialBalanceSpec()) at core spec version 4` to the last line: the constants `trialBalanceGolden`, `trialBalanceDebitDrillGolden` and `trialBalanceCreditDrillGolden` have no reader left. The line `const trialBalanceGolden = …` is 5378 characters, and the Read tool cuts a line above 2000 characters, so do not copy it from a Read. Make the deletion as three Edit calls, each with text the Read tool shows whole: (a) the comment block above the constant, replaced by nothing; (b) the block `const (` … `)` of the two drill constants after it, from a Read of those lines, replaced by nothing (if one of its lines is also cut, use the method of (c) for it); (c) the long line: print it with `command grep -n 'const trialBalanceGolden = ' /Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance_test.go`, and use the printed text without the `897:` prefix as the `old_string` of one Edit with an empty `new_string`. Check: `command grep -rn 'trialBalanceGolden\|trialBalanceDebitDrillGolden\|trialBalanceCreditDrillGolden' /Users/nas/tmp/tinycld/books-purchases/books/server` prints nothing, and `gofmt -l /Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance_test.go` prints nothing. The planner's build kept these three constants (an unused constant compiles); the deletion was not built.

- [ ] **Step 3: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet . && go test -count=1 -run 'TrialBalance' .`
Expected: `go vet` exits 0 (the new test file reads no name that the old spec lacks). The tests FAIL: `TestTrialBalanceSpecLoads` (revision 2, and no golden file), `TestTrialBalanceShippedUnpublished` (`shipped_revision` 2), `TestTrialBalanceOpenedAccountRows` (no opening row), `TestTrialBalanceReadsTheStoredMonths` (a stored month's change does not show). `TestTrialBalanceStoredMonthsEqualTheLines` and `TestTrialBalanceOfAMonthWithNoLine` pass before the change: they pin figures that must not move. (The planner ran the green side only; if another test is red here, read its message before you go on.)

- [ ] **Step 4: Replace the spec**

Replace the whole of `/Users/nas/tmp/tinycld/books-purchases/books/server/report_trial_balance.go` with:

```go
package books

import "tinycld.org/core/reports"

// trialBalanceKey names the shipped Trial Balance for good.
const trialBalanceKey = "books.trial_balance"

// trialBalanceBeforeLabel is the first row under an opened account: its
// balance at the start of the as-of day's month. Core puts the month in place
// of the token, in the reader's words ("Balance before Nov 2026").
const trialBalanceBeforeLabel = "Balance before " + reports.LabelValueToken

// trialBalanceShipped is the Trial Balance Books ships. It is unpublished:
// published reports are hidden from guests, and Books bookkeepers are guests,
// so Books shares it with every Books reader instead.
func trialBalanceShipped() reports.Shipped {
	return reports.Shipped{
		Key:         trialBalanceKey,
		Title:       "Trial Balance",
		Description: "Each account's balance on one day, as a debit or a credit.",
		Spec:        mustJSON(trialBalanceSpec()),
		// 2: opened lines open the transaction that posted them (detail.open),
		// and an account's Debit or Credit opens Transaction Detail by Account
		// (drill).
		// 3: the months before the as-of day's month are read from the stored
		// months; an opened account shows them as one row.
		Revision:    3,
		Unpublished: true,
	}
}

// trialBalanceDrill opens Transaction Detail by Account from an account row's
// Debit or Credit: its posted lines from its first one (an income or expense
// account's first in the as-of day's fiscal year) up to the as-of day, so the
// section total is the figure by the account's normal side. sign is the
// comparison ("gte" for Debit, "lt" for Credit) the column's own rollup puts
// on the net to choose its side: core writes a cell's drill whatever the cell
// shows, so the side that draws no figure gets no accounts and opens nothing.
// The retained-earnings row adds earlier years' income and expense of other
// accounts, so it opens nothing.
func trialBalanceDrill(sign string) *reports.Drill {
	asOf := prm("asOf")
	from := op("if",
		op("in", rec("byAccount", "type"), lit(profitAndLossTypes())),
		firstLineFY(asOf),
		firstLine(asOf),
	)
	return tdDrillTarget(map[string]map[string]reports.DrillValue{
		"byAccount": {
			"period": {Range: &reports.DrillRange{From: from, To: asOf}},
			"accounts": {Formula: op("if",
				op("eq", rec("byAccount", "detail_type"), lit("retained_earnings")),
				lit(nil),
				op("if", op(sign, trialBalanceNet(), lit(0)), tdAccount(), lit(nil)),
			)},
		},
	}, nil)
}

// tbStoredWhere picks the stored months of the row's account that count in its
// balance and end before the as-of day's month: every month of a balance-sheet
// account, and of an income or expense account the months of the as-of day's
// fiscal year. A fiscal year starts on the first day of a month, so every day
// of a month is in the fiscal year of the month's first day.
func tbStoredWhere() []*reports.Node {
	asOf := prm("asOf")
	return []*reports.Node{
		op("eq", link("account"), rec("byAccount", "id")),
		op("lt", fld("month_start"), op("startOfMonth", asOf)),
		op("or",
			op("not", op("in", fld("account", "type"), lit(profitAndLossTypes()))),
			op("gte", op("fiscalYear", fld("month_start")), op("fiscalYear", asOf)),
		),
	}
}

// tbEarlierPLWhere picks the stored income and expense months of the fiscal
// years before the as-of day's: they count on the retained-earnings account.
func tbEarlierPLWhere() []*reports.Node {
	return []*reports.Node{
		op("in", fld("account", "type"), lit(profitAndLossTypes())),
		op("lt", op("fiscalYear", fld("month_start")), op("fiscalYear", prm("asOf"))),
	}
}

// onRetainedEarnings is x on the retained-earnings account's row, 0 on every
// other row.
func onRetainedEarnings(x *reports.Node) *reports.Node {
	return op("if", op("eq", rec("byAccount", "detail_type"), lit("retained_earnings")), x, lit(0))
}

// trialBalanceBefore is the row's account's net at the start of the as-of
// day's month, from the stored months (books_account_balances) alone, so its
// cost does not grow with the account's history.
func trialBalanceBefore() *reports.Node {
	return op("add",
		glSum("books.account_balances", glStoredNet(), tbStoredWhere()...),
		onRetainedEarnings(glSum("books.account_balances", glStoredNet(), tbEarlierPLWhere()...)),
	)
}

// trialBalanceNet is the row's account's net on the as-of day: the stored
// months before the day's month and the posted lines of that month up to the
// day, which are the report's own rows.
func trialBalanceNet() *reports.Node {
	return op("add", trialBalanceBefore(), op("coalesce", op("sum", glNet()), lit(0)))
}

// tbStoredCount counts the stored months that pass where, 0 when none does.
func tbStoredCount(where ...*reports.Node) *reports.Node {
	return op("coalesce", &reports.Node{
		Op:     "lookup",
		Source: "books.account_balances",
		Where:  op("and", where...),
		Value:  mustJSON(fld("id")),
		Pick:   "count",
	}, lit(0))
}

// trialBalanceActivity is above 0 when the row's figure adds a posted line: a
// stored month exists only for a month with a posted line.
func trialBalanceActivity() *reports.Node {
	return op("add",
		op("add", tbStoredCount(tbStoredWhere()...), op("count", fld("id"))),
		onRetainedEarnings(tbStoredCount(tbEarlierPLWhere()...)),
	)
}

// debitSide is x when it is 0 or more; creditSide is x as a positive amount
// when it is below 0. Each is blank on the other side.
func debitSide(x *reports.Node) *reports.Node {
	return op("if", op("gte", x, lit(0)), x, lit(nil))
}

func creditSide(x *reports.Node) *reports.Node {
	return op("if", op("lt", x, lit(0)), op("sub", lit(0), x), lit(nil))
}

// trialBalanceSpec is one row per account: its net of the posted lines on or
// before the as-of day, by sign in Debit or Credit, in the chart order. Income
// and expense of earlier fiscal years count on the retained-earnings account.
// The report's rows are the posted lines of the as-of day's month only; the
// months before it come from the stored months. Paths start at a journal line.
func trialBalanceSpec() *reports.Spec {
	asOf := prm("asOf")
	net := trialBalanceNet()
	before := trialBalanceBefore()
	activity := trialBalanceActivity()
	return &reports.Spec{
		Version: reports.CurrentVersion,
		Source:  "books.journal_lines",
		Params: []reports.ParamDef{
			{ID: "asOf", Type: reports.ParamAsOf, Label: "As of", Default: mustJSON(map[string]string{"preset": "endOfLastMonth"})},
			{
				ID: "rows", Type: reports.ParamChoice, Label: "Rows",
				Options: []reports.Option{
					{Value: "nonzero", Label: "Non-zero"},
					{Value: "active", Label: "Active"},
					{Value: "all", Label: "All"},
				},
				Default: mustJSON("nonzero"),
			},
			accountsParam(),
		},
		Filter: op("and",
			op("eq", fld("posted"), lit(true)),
			op("gte", fld("date"), op("startOfMonth", asOf)),
			op("lte", fld("date"), asOf),
			op("in", col("account"), prm("accounts")),
		),
		Columns: []reports.Column{
			{ID: "account", Label: "Account", Path: reports.Path{{Link: "account"}}},
			// An opened line would repeat its account's number; leave it blank.
			{ID: "number", Label: "No.", Formula: rec("byAccount", "number"), Detail: reports.DetailBlank},
			// A line shows its own amount. The account's row shows the rollup,
			// and the Total adds the account rows shown. The stored months stay
			// out of Formula: core refuses a rows total over a looked-up amount.
			{
				ID: "debit", Label: "Debit", Total: reports.TotalRows,
				Formula: debitSide(glNet()),
				// An account with no posted line shows no amount.
				Rollup: op("if", op("eq", activity, lit(0)), lit(nil), debitSide(net)),
				Drill:  trialBalanceDrill("gte"),
			},
			{
				ID: "credit", Label: "Credit", Total: reports.TotalRows,
				Formula: creditSide(glNet()),
				Rollup:  creditSide(net),
				Drill:   trialBalanceDrill("lt"),
			},
		},
		Groups: []reports.Group{{
			ID:    "byAccount",
			Col:   "account",
			Label: rec("byAccount", "full_name"),
			// Every account may get a row: one with stored months and no line
			// in the as-of day's month has no row of its own to start from.
			Fill: &reports.Fill{Where: op("in", fld("id"), prm("accounts"))},
			Having: op("or",
				op("and",
					op("eq", prm("rows"), lit("all")),
					op("or", op("eq", rec("byAccount", "inactive"), lit(false)), op("gt", activity, lit(0))),
				),
				op("and", op("eq", prm("rows"), lit("active")), op("gt", activity, lit(0))),
				op("ne", net, lit(0)),
			),
			Opening: &reports.Opening{
				Label:       trialBalanceBeforeLabel,
				LabelValue:  asOf,
				LabelBucket: "month",
				Cells:       map[string]*reports.Node{"debit": debitSide(before), "credit": creditSide(before)},
			},
		}},
		Sort: []reports.SortKey{
			{Formula: typeRankCase(rec("byAccount", "type"))},
			{Col: "number"},
			{Formula: rec("byAccount", "full_name")},
			// Core applies a formula sort key that reads a row's fields to
			// detail rows only, so these two order the opened lines and leave
			// the groups alone. The entry's integer sequence, not its text
			// number, which sorts "10" before "9".
			{Formula: fld("date")},
			{Formula: fld("entry", "sequence")},
		},
		Detail: &reports.Detail{
			Label: op("concat",
				fld("date"), lit(" · "),
				fld("entry", "number"), lit(" · "),
				lineDescriptionFormula(),
			),
			Open: entryOpenPath(),
		},
		// A long account name wraps onto up to three lines on a phone rather
		// than losing its end to an ellipsis (column-fit decision 3; three
		// lines fit "Software Subscriptions:Design Tools" at 390 px).
		Display: &reports.Display{Columns: map[string]reports.ColumnFit{
			"account": {MinWidth: "5ch", WhiteSpace: "normal", LineClamp: 3},
			"number":  {MinWidth: "max-content"},
			"debit":   {MinWidth: "max-content"},
			"credit":  {MinWidth: "max-content"},
		}},
	}
}
```

- [ ] **Step 5: Add the golden**

The golden is `mustJSON(trialBalanceSpec())` and a newline, 34008 bytes. Copy the planner's file and check it:

```bash
mkdir -p /Users/nas/tmp/tinycld/books-purchases/books/server/testdata
cp /Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part17-scratch/trial_balance_spec.golden.json /Users/nas/tmp/tinycld/books-purchases/books/server/testdata/trial_balance_spec.golden.json
shasum -a 256 /Users/nas/tmp/tinycld/books-purchases/books/server/testdata/trial_balance_spec.golden.json
```

Expected: `0a2b526ff3a0f3f4a63cbdb1f041e30967afe8d08d7e9ab730c199efb539f461`. If `TestTrialBalanceSpecLoads` then fails on the golden, the spec of step 4 was not copied exactly: fix the spec, not the golden. If the scratch file is gone, the golden is the `got` text that the failing `TestTrialBalanceSpecLoads` prints, plus one newline; it must have the sha256 above.

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && gofmt -l . ; go vet . && go test -count=1 -run 'TrialBalance|Lifecycle|StatementsScaleBook|TransactionDetail' .`
Expected: `gofmt -l` prints nothing, `go vet` exits 0, `ok  tinycld.org/packages/books`. The planner's run of the whole package (`go test -count=1 .`) with this code: `ok  tinycld.org/packages/books 219.330s`.

- [ ] **Step 7: Commit (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add server/report_trial_balance.go server/report_trial_balance_test.go server/report_trial_balance_stored_test.go server/testdata/trial_balance_spec.golden.json
git commit -m "feat(reports): the Trial Balance reads the stored months"
```

---

## Not in this part: task 9c-1b (the next part)

- E2E `books/tests/e2e/trial-balance.spec.ts`, test "earlier fiscal years fold onto Retained earnings…" (lines 559-570): the opened Retained earnings row no longer lists the prior-year line; assert the opening row instead. `detailsUnder` (`books/tests/e2e/books-report-helpers.ts:363`) takes rows of kind `detail` only, and `openAccountRow` (line 372) waits for `detailsUnder(...).length`, so an account whose opened rows are the opening row alone needs its own wait; how `tbSnapshot` names an opening row was not read. One NEEDS-RUN command for the driver.
- Help `books/help/reports.md` line 35.
- The 100k NEEDS-RUN (`TestStatementsScale`) and the ledger step (`books/docs/plans/general-ledger-ledger.md`, the section "Trial Balance, Balance Sheet and Transaction Detail: the measure and its baseline"), in the form of part 13 steps 13 to 15.
- `pnpm exec tinycld-pkg typecheck` in books.
