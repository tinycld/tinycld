# Report Engine Query Scaling Implementation Plan — part 14 (task 9c-0: a "rows" column shows its rollup on the innermost group)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A report column with `total: "rows"` may carry a `rollup`: the rollup is the cell of the innermost group's rows, and every row above them adds the rows shown under it, so a balance list can show "stored months + lines" on each account row and still total.

**Architecture:** Today `Validate` refuses `rollup` with `total: "rows"`, and a rollup shows on every group, subtotal and total row. With this task `level.showsRollup` gives the rollup to the innermost group level only when the column is a "rows" column; the levels above keep `rowsTotal` (the sum of the child rows' cells, `childSum`). A rollup can be blank for two currencies where the detail cell never is, so `rowsTotal` always reads the child rows' mixed helper (`vmx<idx>`) for such a column: a child row that is blank for two currencies makes the sum blank, with the `mixedCurrency` notice (a cell that is blank for any other reason adds 0). A rollup can be an amount in its own currency (a looked-up amount's on each row, an amount param's), so `rowsCells` also sets that helper on a row whose code is not the code of the sum above it (`level.foreignRollup`): the row keeps its figure and code, and the sum is blank. The pair is refused with no group, with a pivot and with a layout.

**Tech Stack:** Go, SQLite (PocketBase fork), core report engine `tinycld.org/core/reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-plan-part14.md` (the two refusals that block the stored-months Trial Balance, and the design that was tried). The human's answer of 2026-11-20: an opened account in the Trial Balance and the Balance Sheet shows a "Balance before <month>" row plus the lines of the as-of month; the drill-down still lists every line. Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`.

**Repos:** workspace `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/` (branch `books/p5-report-engine-scale`, at f4296bb0 when this part was written). Task 9c-0 is a core task: it edits only `tinycld/core/server/reports` and `tinycld/docs/packages.md`. Books is not changed.

**Built before it was written:** the planner applied every code block of this part to the tree, ran the red and green runs the steps name and the whole `reports` package once (`ok tinycld.org/core/reports 123s`), and took the code out again. The diff is `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part14-scratch/9c-0-v2.patch`, the new test file `part14-scratch/rollup_rows_test.go`. An implementer may compare with them, and writes the tree from this plan.

**The second refusal of the note is not changed, on purpose.** The note's run 3 put the stored-months lookup in the column's `formula` (`this amount's currency is read per row, and the total has no currency key to group it by`). With the human's answer the column's `formula` is the plain line amount (the detail rows are the lines of the as-of month), the lookup is in the `rollup` and in the `opening` cells, and neither is read inside a total: `TestARowsColumnShowsItsRollupOnTheInnermostGroup` runs that exact shape (a sum lookup of a `SameCurrencyAs` source in the rollup, in the having and in the opening cells) and is not refused. `totalCurrency` stays as it is. That test proves it only for a rollup that adds a sum of the report's rows to the lookup: `add` then blanks a row of two codes. A rollup that is the looked-up amount alone is not refused either, and no row of it is blank; `TestARowsTotalOverLookedUpAmountsOfTwoCurrenciesIsBlank` is the guard of that shape, and `TestARowsTotalOverAnAmountParamInAnotherCurrencyIsBlank` of a rollup that is an amount param (steps 1 and 9).

## Global Constraints

- Core names no package: every core test, comment and message of this task names the zoo only (`zoo.ledger_lines`, `zoo.ledger_months`, `zoo.feedings`). No Books name, slug, collection or route appears under `tinycld/`.
- Core money rules stay sound (R3): a total over two currencies is refused or blank, never a figure. A sum that reads a row that is blank for two currencies, or a row in another currency than its own, is blank, with no currency code, and the run carries the `mixedCurrency` notice. A row that is blank for another reason adds 0.
- A report that does not pair `rollup` with `total: "rows"` compiles to the same SQL as before: `showsRollup` is true on every level for it.
- No timing run and no probe belongs to this task.
- Go code is formatted with `gofmt`. Comments say why, not what.

## Review Focus

1. An account row that is blank for two currencies (its months in EUR, a line in USD) under a Total of the rows shown: the Total is blank with no code, not the sum of the other rows. Test: `TestARowsTotalOverARollupOfTwoCurrenciesIsBlank/a month and a line` (step 1).
2. A filled account (no line in the period) whose months are in another currency than the lines: its row is blank and the Total is blank; with a having that reads its net the row is hidden and the run says so. Tests: `TestARowsTotalOverARollupOfTwoCurrenciesIsBlank/a month alone`, and the notice check of `TestARowsColumnShowsItsRollupOnTheInnermostGroup` (step 1).
3. A report with two group levels: the outer group and the Total add the rows shown, and do not show the rollup (which reads the inner group's record and would be blank). Test: `TestARowsTotalAddsTheRollupsOfTheLevelBelow` (step 1).
4. A column that reads the "rows" column by `{col}` on a level above the innermost group: it reads the sum, not a blank. Test: the `gap` column of the same three tests (step 1).
5. The pair where it has no defined cell: no group, a pivot, a layout. Each is refused at `Validate` with the column's `rollup` path. Tests: `TestARollupWithRowsTotalsNeedsAGroup`, `TestARollupWithRowsTotalsIsRefusedWithAPivotOrALayout` (step 2).
6. A rollup that is an amount in its own currency (a lookup alone, an amount param; test of the param: `TestARowsTotalOverAnAmountParamInAnotherCurrencyIsBlank`, rows of 50 EUR under a blank Total, 100 USD for a USD param). For the lookup: each account row shows its own figure and code (500 USD, 900 EUR, 250 GBP), no row is blank, and the Total is blank with no code and the notice, not 1950 "USD". With every month in USD the Total is 1950 USD and the run has no notice for the column. The helper of `rowsCells` is read by the rows above only: the row's own cell and code come from the level's cells (`compileLevels`), so they do not change. Test: `TestARowsTotalOverLookedUpAmountsOfTwoCurrenciesIsBlank` (step 1).

---

### Task 9c-0: A "rows" column shows its rollup on the innermost group

**Files:**
- Create: `tinycld/core/server/reports/rollup_rows_test.go`
- Modify: `tinycld/core/server/reports/rollup_test.go:84-89` (the refusal test is replaced)
- Modify: `tinycld/core/server/reports/bind.go:372-379` (`binder.columns`, the rollup switch)
- Modify: `tinycld/core/server/reports/levels.go:652-671` (`rowsCells`, and the new `foreignRollup` after it), `:951-957` (`levelCtx.col`), `:1036` (`rowsTotal`), `:1294` (`readsRowValue`)
- Modify: `tinycld/core/server/reports/spec.go:210-213` (the `Rollup` comment)
- Modify: `tinycld/docs/packages.md:1562` (the `columns[].rollup` row)

**Interfaces:**
- Consumes (existing test helpers, do not redefine): `newZoo`, `withBalanceZoo`, `glNet` (`balance_shape_test.go`); `withLedgerMonths(t, z, sameAs)`, `monthsOf` (`sharedcurrency_test.go`); `z.run`, `z.rec`, `z.id` (`run_test.go`); `withZooRegistry`, `loadSpec`, `requireSpecError` (`spec_test.go`).
- Consumes (existing engine code, do not redefine): `orSQL`, `expr.blankFor`, `currency.none`, `currency.sameRead` (`formula.go`); `mixedCell` (`run.go`); `level.detailCell`, `compiler.totalCurrency`, `levelCtx.currencySQL` (`levels.go`); `z.recs` (`zoo_test.go`, the records by key).
- Produces: `func (lv *level) showsRollup(cp *colPlan) bool` and `func (lv *level) foreignRollup(cp *colPlan, e expr) (string, error)` (core, unexported). For task 9c-1 (Books): a spec column `{Formula: <line amount>, Total: reports.TotalRows, Rollup: <formula on the group's level>}` passes `Validate` in a report with at least one group and no pivot and no layout; the rollup shows on the innermost group's rows, and the outer groups, subtotals and the Total add the rows shown. A sum over account rows in two currencies is blank with the `mixedCurrency` notice, so 9c-1 needs no guard of its own for it.

- [ ] **Step 1: Write the failing run tests**

Create `tinycld/core/server/reports/rollup_rows_test.go`:

```go
package reports

import (
	"slices"
	"strings"
	"testing"
)

// A balance list by account: a line's debit or credit on detail rows; on the
// account's row the stored months' net added to the lines' net, on its side
// (the rollup); on every row above it the sum of the rows shown under it
// (total "rows"). Its lines are July's; the stored months are June's.
// groups is the report's groups, with NET for an account's net and BEFORE_D,
// BEFORE_C for its stored months on each side.
func rowsRollupSpec(groups string) string {
	months := strings.ReplaceAll(monthsOf, "ACCOUNT", `{"record":"g","path":[{"field":"id"}]}`)
	net := `{"op":"add","args":[` + months + `,{"op":"coalesce","args":[{"op":"sum","args":[` + glNet + `]},{"value":0}]}]}`
	side := func(x string, credit bool) string {
		if credit {
			x = `{"op":"sub","args":[{"value":0},` + x + `]}`
		}
		return `{"op":"if","args":[{"op":"gt","args":[` + x + `,{"value":0}]},` + x + `,{"value":null}]}`
	}
	groups = strings.NewReplacer("NET", net, "BEFORE_D", side(months, false), "BEFORE_C", side(months, true)).Replace(groups)
	return `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"account","path":[{"link":"account"}],"labelField":"number"},
		{"id":"kind","path":[{"link":"account"},{"field":"type"}]},
		{"id":"day","path":[{"field":"date"}]},
		{"id":"debit","formula":` + side(glNet, false) + `,"total":"rows","rollup":` + side(net, false) + `},
		{"id":"credit","formula":` + side(glNet, true) + `,"total":"rows","rollup":` + side(net, true) + `},
		{"id":"gap","formula":{"op":"sub","args":[{"op":"coalesce","args":[{"col":"debit"},{"value":0}]},{"op":"coalesce","args":[{"col":"credit"},{"value":0}]}]}}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"gte","args":[{"path":[{"field":"date"}]},{"value":"2026-07-01"}]}]},
		"groups":` + groups + `,
		"sort":[{"col":"kind"},{"formula":{"record":"g","path":[{"field":"number"}]}},{"col":"day"}]}`
}

const (
	// One row per account, also for an account with no line of July (fill).
	rowsRollupFilled = `[{"id":"g","col":"account","fill":{},"label":{"record":"g","path":[{"field":"number"}]}HAVING,
		"opening":{"label":"Balance before July","cells":{"debit":BEFORE_D,"credit":BEFORE_C}}}]`
	rowsRollupHaving = `,"having":{"op":"ne","args":[NET,{"value":0}]}`
	// The accounts under their type.
	rowsRollupByKind = `[{"id":"k","col":"kind"},{"id":"g","col":"account","label":{"record":"g","path":[{"field":"number"}]}}]`
)

// rowsRollupLine is one row of the balance list: its kind and label, and its
// debit, credit and gap cells with the debit's currency code.
type rowsRollupLine struct {
	Kind, Label              string
	Debit, Credit, Gap, Code any
}

func rowsRollupLines(res *Result) []rowsRollupLine {
	var out []rowsRollupLine
	for _, r := range res.Rows {
		out = append(out, rowsRollupLine{r.Kind, r.Label, r.Cells["debit"], r.Cells["credit"], r.Cells["gap"], r.Cells["debit@currency"]})
	}
	return out
}

// rowsRollupZoo is the ledger zoo with its stored months (cash 500 USD, bank
// 900 EUR, petty 250 GBP) and one more: idle, 300 USD. idle has no line.
func rowsRollupZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	withBalanceZoo(t, z)
	withLedgerMonths(t, z, "zoo.ledger_lines.currency")
	z.rec(t, "idleJune", "zoo_ledger_months", map[string]any{
		"account": z.id("idle"), "first_day": "2026-06-01", "debit_total": 300, "credit_total": 0, "currency": "USD",
	})
	return z
}

func requireRowsRollupLines(t *testing.T, got, want []rowsRollupLine) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
	}
}

// The account row shows the rollup (the months before July and July's lines),
// its opening row the months, its detail rows each line, and the Total adds
// the account rows shown: idle (a month, no line) counts, and the accounts
// whose net is 0 are hidden and add nothing. bank and petty have a month in
// another currency than the lines': the having cannot read their net, so they
// are hidden and the run says so.
func TestARowsColumnShowsItsRollupOnTheInnermostGroup(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupSpec(strings.ReplaceAll(rowsRollupFilled, "HAVING", rowsRollupHaving)), Options{ExpandAll: true}, nil)
	const before = "Balance before July"
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindOpening, before, int64(500), nil, nil, "USD"},
		{KindDetail, "1000", nil, int64(10000), int64(-10000), "USD"},
		{KindDetail, "1000", int64(3000), nil, int64(3000), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindOpening, before, int64(300), nil, nil, "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindGroup, "4000", int64(1500), nil, int64(1500), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "4000", int64(1500), nil, int64(1500), "USD"},
		{KindGroup, "5000", int64(10000), nil, int64(10000), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "5000", int64(10000), nil, int64(10000), "USD"},
		{KindTotal, "Total", int64(11800), int64(11000), int64(800), "USD"},
	})
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
		return n.Code == "mixedCurrency" && slices.Contains(n.Conditions, "groups.0.having")
	}) {
		t.Fatalf("notices = %+v, want mixedCurrency naming groups.0.having", res.Notices)
	}
}

// Above the innermost group every row adds the rows shown under it: a type's
// row adds its accounts' rollups, the Total the types' rows. A column that
// reads a "rows" column with a rollup (gap) reads those cells on each level.
func TestARowsTotalAddsTheRollupsOfTheLevelBelow(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupSpec(rowsRollupByKind), Options{}, nil)
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "Asset", int64(0), int64(6500), int64(-6500), "USD"},
		{KindGroup, "Expense", int64(10000), int64(0), int64(10000), "USD"},
		{KindGroup, "Income", int64(1500), int64(0), int64(1500), "USD"},
		{KindGroup, "Liability", int64(0), int64(4500), int64(-4500), "USD"},
		{KindTotal, "Total", int64(11500), int64(11000), int64(500), "USD"},
	})
}

// A total over two currencies stays blank (R3). An account whose months are
// in another currency than the lines' has a blank rollup: with no line of its
// own (bank EUR, petty GBP), and with one (bank, a USD line of July). The
// Total adds the account rows, so one blank row makes it blank, with no
// currency code, and the run names the columns.
func TestARowsTotalOverARollupOfTwoCurrenciesIsBlank(t *testing.T) {
	for name, withLine := range map[string]bool{"a month alone": false, "a month and a line": true} {
		t.Run(name, func(t *testing.T) {
			z := rowsRollupZoo(t)
			if withLine {
				z.rec(t, "j4a", "zoo_ledger_lines", map[string]any{
					"entry": z.id("J3"), "account": z.id("bank"), "date": "2026-07-05",
					"debit": 100, "credit": 0, "currency": "USD", "position": 3, "posted": true,
				})
			}
			res := z.run(t, "ann", rowsRollupSpec(strings.ReplaceAll(rowsRollupFilled, "HAVING", "")), Options{}, nil)
			requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
				{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
				{KindGroup, "1100", nil, nil, nil, nil},
				{KindGroup, "1200", nil, nil, nil, nil},
				{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
				{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
				{KindGroup, "3900", nil, nil, int64(0), "USD"},
				{KindGroup, "4000", int64(1500), nil, int64(1500), "USD"},
				{KindGroup, "5000", int64(10000), nil, int64(10000), "USD"},
				{KindTotal, "Total", nil, nil, nil, nil},
			})
			if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
				return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "debit") && slices.Contains(n.Columns, "credit")
			}) {
				t.Fatalf("notices = %+v, want mixedCurrency naming debit and credit", res.Notices)
			}
		})
	}
}

// A rollup that is a looked-up amount alone carries its own currency on each
// row: no row is blank, and each shows its figure with its own code. The Total
// adds the account rows in the lines' currency, so a row in another one makes
// it blank, with no code, and the run names the column (R3). With every month
// in the lines' currency the Total is the sum.
func TestARowsTotalOverLookedUpAmountsOfTwoCurrenciesIsBlank(t *testing.T) {
	months := strings.ReplaceAll(monthsOf, "ACCOUNT", `{"record":"g","path":[{"field":"id"}]}`)
	spec := `{"version":4,"source":"zoo.ledger_lines","columns":[
		{"id":"account","path":[{"link":"account"}],"labelField":"number"},
		{"id":"debit","formula":{"path":[{"field":"debit"}]},"total":"rows","rollup":` + months + `}],
		"groups":[{"id":"g","col":"account","fill":{},"label":{"record":"g","path":[{"field":"number"}]}}],
		"sort":[{"formula":{"record":"g","path":[{"field":"number"}]}}]}`
	type line struct {
		Label       string
		Debit, Code any
	}
	lines := func(res *Result) []line {
		var out []line
		for _, r := range res.Rows {
			out = append(out, line{r.Label, r.Cells["debit"], r.Cells["debit@currency"]})
		}
		return out
	}
	names := func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "debit") }

	t.Run("two currencies", func(t *testing.T) {
		res := rowsRollupZoo(t).run(t, "ann", spec, Options{}, nil)
		want := []line{
			{"1000", int64(500), "USD"}, {"1100", int64(900), "EUR"}, {"1200", int64(250), "GBP"}, {"1300", int64(300), "USD"},
			{"2000", int64(0), "USD"}, {"3900", int64(0), "USD"}, {"4000", int64(0), "USD"}, {"5000", int64(0), "USD"},
			{"Total", nil, nil},
		}
		if got := lines(res); !slices.Equal(got, want) {
			t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
		}
		if !slices.ContainsFunc(res.Notices, names) {
			t.Fatalf("notices = %+v, want mixedCurrency naming debit", res.Notices)
		}
	})
	t.Run("one currency", func(t *testing.T) {
		z := rowsRollupZoo(t)
		for _, key := range []string{"bankJune", "pettyJune"} {
			z.recs[key].Set("currency", "USD")
			if err := z.app.Save(z.recs[key]); err != nil {
				t.Fatal(err)
			}
		}
		res := z.run(t, "ann", spec, Options{}, nil)
		want := []line{
			{"1000", int64(500), "USD"}, {"1100", int64(900), "USD"}, {"1200", int64(250), "USD"}, {"1300", int64(300), "USD"},
			{"2000", int64(0), "USD"}, {"3900", int64(0), "USD"}, {"4000", int64(0), "USD"}, {"5000", int64(0), "USD"},
			{"Total", int64(1950), "USD"},
		}
		if got := lines(res); !slices.Equal(got, want) {
			t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
		}
		if slices.ContainsFunc(res.Notices, names) {
			t.Fatalf("notices = %+v, want no mixedCurrency for debit", res.Notices)
		}
	})
}

// An amount param carries its own code too: a rollup that is the param shows
// it on each group row, and the Total adds those rows in the rows' currency
// only when the param is in it (R3).
func TestARowsTotalOverAnAmountParamInAnotherCurrencyIsBlank(t *testing.T) {
	const spec = `{"version":4,"source":"zoo.feedings","params":[{"id":"budget","type":"amount","label":"Budget"}],"columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"total":"rows","rollup":{"param":"budget"}}],
		"groups":[{"id":"g","col":"species"}]}`
	type line struct {
		Kind       string
		Cost, Code any
	}
	names := func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "cost") }
	for code, total := range map[string]line{"EUR": {KindTotal, nil, nil}, "USD": {KindTotal, int64(100), "USD"}} {
		t.Run(code, func(t *testing.T) {
			res := newZoo(t).run(t, "ann", spec, Options{}, map[string]string{"budget": `{"amount":50,"currency":"` + code + `"}`})
			var got []line
			for _, r := range res.Rows {
				got = append(got, line{r.Kind, r.Cells["cost"], r.Cells["cost@currency"]})
			}
			want := []line{{KindGroup, int64(50), code}, {KindGroup, int64(50), code}, total}
			if !slices.Equal(got, want) {
				t.Fatalf("rows =\n%#v\nwant\n%#v", got, want)
			}
			if blank := total.Cost == nil; slices.ContainsFunc(res.Notices, names) != blank {
				t.Fatalf("notices = %+v, want mixedCurrency naming cost: %v", res.Notices, blank)
			}
		})
	}
}
```

- [ ] **Step 2: Replace the refusal test**

In `tinycld/core/server/reports/rollup_test.go`, replace the whole function `TestARollupIsRefusedWithRowsTotals` (lines 84-89) with:

```go
// With total "rows" a rollup is the cell of the innermost group's rows; a
// report with no group has no such row, so its rollup would never show.
func TestARollupWithRowsTotalsNeedsAGroup(t *testing.T) {
	app := withZooRegistry(t)
	const columns = `"columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],
		"total":"rows","rollup":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}]`
	err := Validate(app, loadSpec(t, `{"version":4,"source":"zoo.feedings",`+columns+`}`))
	requireSpecError(t, err, 400, "columns.1.rollup", `with total "rows" a rollup is the cell of the innermost group's rows: the report needs a group`)
	if err := Validate(app, loadSpec(t, `{"version":4,"source":"zoo.feedings",`+columns+`,"groups":[{"id":"g","col":"animal"}]}`)); err != nil {
		t.Fatalf("Validate with a group = %v", err)
	}
}

// A pivot's columns and a layout's sections make their own figures from a
// column's totals; a rollup that shows on one level only is not defined there.
func TestARollupWithRowsTotalsIsRefusedWithAPivotOrALayout(t *testing.T) {
	app := withZooRegistry(t)
	const refusal = `a rollup does not go with total "rows" in a report with a pivot or a layout`
	const cost = `{"id":"cost","path":[{"field":"cost"}],"total":"rows","rollup":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}`
	err := Validate(app, loadSpec(t, `{"version":4,"source":"zoo.feedings","columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},`+cost+`],
		"groups":[{"id":"g","col":"species"}],"layout":[{"section":"cats","title":"Lions","key":"lion"}]}`))
	requireSpecError(t, err, 400, "columns.1.rollup", refusal)
	err = Validate(app, loadSpec(t, `{"version":4,"source":"zoo.feedings","columns":[{"id":"month","path":[{"field":"fed"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},`+cost+`],
		"groups":[{"id":"m","col":"month","bucket":"month"}],"pivot":{"path":[{"link":"animal"}],"cell":"grams"}}`))
	requireSpecError(t, err, 400, "columns.2.rollup", refusal)
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:

`go test ./reports -run 'RowsColumnShowsItsRollup|RowsTotalAddsTheRollups|RowsTotalOver|RollupWithRowsTotals' -count=1`

Expected: FAIL. The five run tests fail in `z.run` with `a rollup replaces the column's totals: it does not go with total "rows"` (at `columns.3.rollup`; `columns.1.rollup` in `TestARowsTotalOverLookedUpAmountsOfTwoCurrenciesIsBlank` and `TestARowsTotalOverAnAmountParamInAnotherCurrencyIsBlank`); the two Validate tests fail because the message is the old one.

- [ ] **Step 4: Validate accepts the pair where it has a cell**

In `tinycld/core/server/reports/bind.go`, in `binder.columns`, replace:

```go
			case c.Total == TotalRows:
				b.bad(at+".rollup", `a rollup replaces the column's totals: it does not go with total "rows"`)
```

with:

```go
			case c.Total == TotalRows && len(p.spec.Groups) == 0:
				b.bad(at+".rollup", `with total "rows" a rollup is the cell of the innermost group's rows: the report needs a group`)
			case c.Total == TotalRows && (p.spec.Pivot != nil || len(p.spec.Layout) > 0):
				b.bad(at+".rollup", `a rollup does not go with total "rows" in a report with a pivot or a layout`)
```

- [ ] **Step 5: The rollup shows on the innermost group only**

In `tinycld/core/server/reports/levels.go`, in `levelCtx.col`, replace:

```go
	if cp.col.Rollup != nil {
		return lc.rollup(cp)
	}
	if cp.col.Total == TotalRows {
		return lc.rowsTotal(cp)
	}
	return lc.recompute(cp, at)
}
```

with:

```go
	if lc.lv.showsRollup(cp) {
		return lc.rollup(cp)
	}
	if cp.col.Total == TotalRows {
		return lc.rowsTotal(cp)
	}
	return lc.recompute(cp, at)
}

// showsRollup is true where a column's cell on this level is its rollup: on
// every level, or, for a "rows" column, on the innermost group only. The
// levels above that group add its cells (rowsTotal), so a rollup that reads
// the group's record still totals.
func (lv *level) showsRollup(cp *colPlan) bool {
	if cp.col.Rollup == nil {
		return false
	}
	return cp.col.Total != TotalRows || lv.n == len(lv.c.p.groups)
}
```

In the same file, in `readsRowValue`, replace:

```go
		if cp != nil && cp.col.Rollup != nil {
			seen[n.Col] = true
```

with:

```go
		if cp != nil && lv.showsRollup(cp) {
			seen[n.Col] = true
```

(Above the innermost group a "rows" column with a rollup then falls to the `cp.col.Total == TotalRows` line below it, which says the column has a value on every level.)

- [ ] **Step 6: Run the tests: the two money tests still fail**

Run the command of step 3.

Expected: `TestARowsColumnShowsItsRollupOnTheInnermostGroup`, `TestARowsTotalAddsTheRollupsOfTheLevelBelow`, the two Validate tests and the subtest `TestARowsTotalOverLookedUpAmountsOfTwoCurrenciesIsBlank/one_currency` PASS. `TestARowsTotalOverARollupOfTwoCurrenciesIsBlank` FAILS in both subtests: the Total row is `Debit 11800, Credit 11000, Gap 800, "USD"` in place of blank. This is the unsound figure (a sum that read a row blank for two currencies as 0); step 7 removes it. `TestARowsTotalOverLookedUpAmountsOfTwoCurrenciesIsBlank/two_currencies` FAILS: the Total row is `Debit 1950, Code "USD"` in place of blank (EUR and GBP cents added as USD), and `TestARowsTotalOverAnAmountParamInAnotherCurrencyIsBlank/EUR` FAILS: the Total row is `Cost 100, Code "USD"` over two rows of 50 EUR (its subtest `USD` passes); step 8 removes both.

- [ ] **Step 7: A blank rollup makes the sum above it blank**

In `tinycld/core/server/reports/levels.go`, in `rowsTotal`, replace:

```go
		if d.blankFor() != "" {
			// A child row that is blank for mixed currencies makes its sum
			// blank too (rowsCells gives each child row its vmx helper).
```

with:

```go
		if d.blankFor() != "" || cp.col.Rollup != nil {
			// A child row that is blank for mixed currencies makes its sum
			// blank too (rowsCells gives each child row its vmx helper). A
			// rollup can be blank where the detail cell never is.
```

(`rowsCells` writes `vmx<idx>` for every "rows" column on every level, `NULL` where the cell cannot be blank, so the read is always valid.)

- [ ] **Step 8: A rollup in another currency makes the sum above it blank**

In `tinycld/core/server/reports/levels.go`, in `rowsCells`, replace:

```go
		out = append(out, fmt.Sprintf("%s AS v%d", e.sql, idx), mixedCell(e, idx))
	}
	return out, nil
}
```

with:

```go
		foreign, err := lv.foreignRollup(cp, e)
		if err != nil {
			return nil, err
		}
		// The helper is for the rows above only: the row's own cell is not
		// blank, and keeps its figure and code.
		e.mixed, e.condMixed = orSQL(e.blankFor(), foreign), ""
		out = append(out, fmt.Sprintf("%s AS v%d", e.sql, idx), mixedCell(e, idx))
	}
	return out, nil
}

// foreignRollup is true on a row of this level whose cell e of a "rows"
// column is a rollup in another currency than the one the rows above add it
// in, "" when no row can be. A rollup can be in a currency of its own (a
// looked-up amount's on each row, an amount param's), and the rows above have
// one code for their sum, so adding such a row would show EUR cents as USD
// (R3). A missing code is not a second currency (currencyGuard).
func (lv *level) foreignRollup(cp *colPlan, e expr) (string, error) {
	if !lv.showsRollup(cp) || e.typ != Money || e.cur.none() {
		return "", nil
	}
	at := cp.at + ".total"
	d, err := lv.detailCell(cp, at)
	if err != nil {
		return "", err
	}
	cur, err := lv.c.totalCurrency(d, at)
	if err != nil {
		return "", err
	}
	// A param's code is known only in the run, so it is compared on each row.
	if cur.none() || e.cur.param == "" && cur.param == "" && e.cur.sameRead(cur) {
		return "", nil
	}
	lc := &levelCtx{lv: lv, visiting: lv.visiting, rows: lv.sumsFrame}
	own, sum := lc.currencySQL(e.cur), lc.currencySQL(cur)
	return fmt.Sprintf("((%s) IS NOT NULL AND %s IS NOT %s AND %s IS NOT NULL AND %s IS NOT NULL)", e.sql, own, sum, own, sum), nil
}
```

(`rowsCells` is called only for a level whose rows the level above adds, and only for "rows" columns, so `showsRollup` is true here on the innermost group alone. The cell and the code the row itself shows are the level's cells of `compileLevels`, which this does not change.)

- [ ] **Step 9: Run the tests to verify they pass**

Run the command of step 3. Expected: `ok tinycld.org/core/reports`.

- [ ] **Step 10: Say it in the spec comment and the reference**

In `tinycld/core/server/reports/spec.go`, in `Column`, replace:

```go
	// Formula.
	Rollup *Node `json:"rollup,omitempty"`
```

with:

```go
	// Formula. With Total "rows" it is the cell of the innermost group's rows
	// only: every row above them adds the rows shown under it, so a rollup
	// that reads the group's record (a looked-up balance) still totals. A
	// blank cell adds 0; a cell that is blank for two currencies, or is in
	// another currency than the sum's, makes the sum blank. Such a report has
	// a group.
	Rollup *Node `json:"rollup,omitempty"`
```

In `tinycld/docs/packages.md`, in the `columns[].rollup` row (line 1562), replace:

```
(a group's record on the Total). Refused with `total: "rows"`, on a child row column,
```

with:

```
(a group's record on the Total). With `total: "rows"` the rollup is the cell of the innermost group's rows only, and every row above them adds the rows shown under it (a blank cell adds 0; a cell that is blank for two currencies, or is in another currency than the sum's, makes that sum blank); that pair needs a group and is refused in a report with a pivot or a layout. Refused on a child row column,
```

- [ ] **Step 11: Format, vet and run the package**

Run, in `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:

`gofmt -l reports/ ; go vet ./reports && go test ./reports -count=1`

Expected: `gofmt` prints nothing, `go vet` prints nothing, `ok tinycld.org/core/reports` (about 3 minutes).

- [ ] **Step 12: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/rollup_rows_test.go core/server/reports/rollup_test.go core/server/reports/bind.go core/server/reports/levels.go core/server/reports/spec.go docs/packages.md
git commit -m "feat(reports): a rows column shows its rollup on the innermost group, and the rows above add it"
```
