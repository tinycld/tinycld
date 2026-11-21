# Report Engine Scale, Part 19: A Rows Rollup Under Layout Sections (task 9c-2a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core accepts a column with `total: "rows"` and a `rollup` in a report with a layout, when the report has a group below the section group, so a statement can show a looked-up balance on each account row and add those rows in its sections.

**Architecture:** One refusal in `Validate` goes away. The engine already computes the shape: the rollup is the cell of the innermost group's rows, each level above adds the rows shown under it (`rowsTotal`, `childSum`), a layout section is the first group's row, and a computed row reads the sections. A new refusal stays for a layout whose section group is the only group: there the rollup would be the section's own cell, each section would carry its own currency, and a computed row that adds two sections fails at run time (`layout.N.formula: amounts in different currencies cannot be combined`).

**Tech Stack:** Go, core report engine `tinycld.org/core/reports` (module at `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`). Tests run on the fictional ledger zoo (`zoo.ledger_lines`, `zoo.ledger_months`). Core names no package.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`, sections "State after plan part 14" (the 9c-0 contract and its "Refused" line) and "State after plan part 19" (what this part found, and tasks 9c-2a2 and 9c-2b that follow it).

## Global Constraints

- This is a core task: it edits `/Users/nas/tmp/tinycld/books-purchases/tinycld` only. No file of the task names a feature package, its slug, its collections or its routes.
- Work on branch `books/p5-report-engine-scale` of the core repo. Do not push.
- The refusal texts are exact; tests compare them:
  - `a rollup does not go with total "rows" in a report with a pivot`
  - `with total "rows" and a layout a rollup is the cell of the rows under a section: the report needs a group below the section group`
- A sum over rows of two currencies is blank with no currency code, and the run carries a `mixedCurrency` notice that names the column. No test may accept a figure that adds two currencies.
- No timing probe and no scale run in this task.
- Read a file with the Read tool before its first Edit. Never `sed -i` or a scripted replace.

## Review Focus

1. A section whose first-group key has no base row in the period (the zoo's `equity` type has no line of July): the section is empty and a fill of the group below it gives it no row, also when the account has stored months. The section and its subtotal then show 0 in the report's currency, and a computed row that reads the section omits those balances: a wrong figure, not a blank. This task does NOT change that; it is task 9c-2a2 (outline). No spec may use the pair under a layout before 9c-2a2 is built. The tests here use only sections that have a base row, and no test pins the empty section.
2. A layout whose section group is the only group, with the pair: refused at `Validate` with the new text (Step 1 pins it). Expected by a spec writer: a clear refusal, not a run-time error from a computed row.
3. A section over account rows of two currencies: the section, its subtotal and every computed row that reads it are blank with no code, and the notice names the columns (Step 2, second test).
4. A section whose shown rows are all blank in a column (Liabilities debit: 2000 has a credit only) shows 0 in the report's currency, not blank (Step 2, first test). A section whose rows are ALL hidden by a having is not run here: task 9c-2a2's test pins it.
5. Levels between the section group and the innermost group with `blank: "skip"` or `hideEmpty` (a statement outline of four levels): not run here. The pair on four levels is run by task 9c-2b's cross-check on a real statement; if it fails there, the fix is a core task before 9c-2b goes on.
6. A fill of the group below the section with no `where` that ties the record to its section fills it under every section, and each section adds it. Not refused and not pinned here; the tests use a fill tied by `{"col":"kind"}`. Task 9c-2b ties each fill and its cross-check pins it.

---

### Task 1 (9c-2a): A rows rollup is accepted under layout sections

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/bind.go:376-377`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/spec.go:210-218` (the `Rollup` comment)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rollup_test.go:97-110`
- Create: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rollup_rows_layout_test.go`
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md:1570` (the `columns[].rollup` row)

**Interfaces:**
- Consumes (test helpers of `rollup_rows_test.go`, same package, unchanged): `rowsRollupSpec(groups string) string` (tokens `NET`, `BEFORE_D`, `BEFORE_C` in `groups`; columns `account`, `kind`, `day`, `debit`, `credit`, `gap`; the spec text ends in `}`), `rowsRollupHaving` (const), `rowsRollupZoo(t) *zoo`, `rowsRollupLines(res) []rowsRollupLine`, `requireRowsRollupLines(t, got, want)`, `type rowsRollupLine struct{ Kind, Label string; Debit, Credit, Gap, Code any }`. From `run_test.go`: `(z *zoo) run(t, user, raw string, opts Options, params map[string]string) *Result`. From the bind tests: `requireSpecError(t, err, 400, at, text)`, `withZooRegistry(t)`, `loadSpec(t, raw)`.
- Produces: `Validate` and `Run` accept `Column{Formula, Total: TotalRows, Rollup}` in a spec with `Layout` and two or more `Groups` and no `Pivot`. Test helper `rowsRollupLayoutSpec(having string) string` and consts `rowsRollupSections`, `rowsRollupUnderSections`. No new exported name.

Zoo facts the expected rows come from (`balance_shape_test.go:78-85`, `rollup_rows_test.go:65-76`): accounts 1000 cash, 1100 bank, 1200 petty, 1300 idle (type `asset`), 2000 loan (`liability`), 3900 (`equity`), 4000 (`income`), 5000 (`expense`); stored months of June: cash 500 USD, bank 900 EUR, petty 250 GBP, idle 300 USD; every line is USD and is in July.

- [ ] **Step 1: Change the refusal test**

In `core/server/reports/rollup_test.go`, replace the whole function `TestARollupWithRowsTotalsIsRefusedWithAPivotOrALayout` and its two comment lines (lines 97 to 110) with:

```go
// A pivot's columns make their own figures from a column's totals; a rollup
// that shows on one level only is not defined there. A layout's sections add
// the rows under them, so the rollup needs a group below the section group: on
// a section row itself each section would carry its own currency, and a
// computed row could not add two of them.
func TestARollupWithRowsTotalsIsRefusedWithAPivotOrOnALayoutSection(t *testing.T) {
	app := withZooRegistry(t)
	const cost = `{"id":"cost","path":[{"field":"cost"}],"total":"rows","rollup":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}`
	err := Validate(app, loadSpec(t, `{"version":4,"source":"zoo.feedings","columns":[{"id":"species","path":[{"link":"animal"},{"field":"species"}]},`+cost+`],
		"groups":[{"id":"g","col":"species"}],"layout":[{"section":"cats","title":"Lions","key":"lion"}]}`))
	requireSpecError(t, err, 400, "columns.1.rollup",
		`with total "rows" and a layout a rollup is the cell of the rows under a section: the report needs a group below the section group`)
	err = Validate(app, loadSpec(t, `{"version":4,"source":"zoo.feedings","columns":[{"id":"month","path":[{"field":"fed"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},`+cost+`],
		"groups":[{"id":"m","col":"month","bucket":"month"}],"pivot":{"path":[{"link":"animal"}],"cell":"grams"}}`))
	requireSpecError(t, err, 400, "columns.2.rollup", `a rollup does not go with total "rows" in a report with a pivot`)
}
```

- [ ] **Step 2: Write the two layout tests**

Create `core/server/reports/rollup_rows_layout_test.go`:

```go
package reports

import (
	"slices"
	"strings"
	"testing"
)

// A statement by account type: the balance list's columns under layout
// sections, one per type, and a computed row that adds two sections. Each
// account fills in under its own type, so an account with a stored month and
// no line of July has a row.
const (
	rowsRollupSections = `,"layout":[
		{"section":"assets","title":"Assets","key":"asset"},
		{"section":"liabilities","title":"Liabilities","key":"liability"},
		{"row":"owed","label":"Assets and liabilities","formula":{"op":"add","args":[{"subtotal":"assets"},{"subtotal":"liabilities"}]}}]}`
	rowsRollupUnderSections = `[{"id":"k","col":"kind","open":true,"subtotals":true},
		{"id":"g","col":"account","fill":{"where":{"op":"eq","args":[{"path":[{"field":"type"}]},{"col":"kind"}]}},
		"label":{"record":"g","path":[{"field":"number"}]}HAVING,
		"opening":{"label":"Balance before July","cells":{"debit":BEFORE_D,"credit":BEFORE_C}}}]`
)

func rowsRollupLayoutSpec(having string) string {
	spec := rowsRollupSpec(strings.ReplaceAll(rowsRollupUnderSections, "HAVING", having))
	return strings.TrimSuffix(spec, "}") + rowsRollupSections
}

// Under a layout the account row shows the rollup, and its section, the
// section's subtotal and a computed row that reads the sections add the
// account rows shown: idle (a month, no line) counts in Assets. bank and petty
// are hidden by the having, as in the report with no layout.
func TestARowsRollupTotalsUnderLayoutSections(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupLayoutSpec(rowsRollupHaving), Options{ExpandAll: true}, nil)
	const before = "Balance before July"
	requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
		{KindGroup, "Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindOpening, before, int64(500), nil, nil, "USD"},
		{KindDetail, "1000", nil, int64(10000), int64(-10000), "USD"},
		{KindDetail, "1000", int64(3000), nil, int64(3000), "USD"},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindOpening, before, int64(300), nil, nil, "USD"},
		{KindSubtotal, "Total Assets", int64(300), int64(6500), int64(-6200), "USD"},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindOpening, before, nil, nil, nil, "USD"},
		{KindDetail, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindComputed, "Assets and liabilities", int64(300), int64(11000), int64(-10700), "USD"},
	})
}

// With no having bank (a month in EUR) and petty (GBP) show as blank rows
// under Assets. The section and its subtotal would add three currencies as
// one figure, so they are blank with no code, the computed row that reads the
// section is blank too, and the run names the columns. Liabilities, all in one
// currency, keeps its figures.
func TestARowsRollupSectionOverTwoCurrenciesIsBlank(t *testing.T) {
	z := rowsRollupZoo(t)
	res := z.run(t, "ann", rowsRollupLayoutSpec(""), Options{ExpandAll: true}, nil)
	var got []rowsRollupLine
	for _, line := range rowsRollupLines(res) {
		if line.Kind != KindDetail && line.Kind != KindOpening {
			got = append(got, line)
		}
	}
	requireRowsRollupLines(t, got, []rowsRollupLine{
		{KindGroup, "Assets", nil, nil, nil, nil},
		{KindGroup, "1000", nil, int64(6500), int64(-6500), "USD"},
		{KindGroup, "1100", nil, nil, nil, nil},
		{KindGroup, "1200", nil, nil, nil, nil},
		{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
		{KindSubtotal, "Total Assets", nil, nil, nil, nil},
		{KindGroup, "Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindGroup, "2000", nil, int64(4500), int64(-4500), "USD"},
		{KindSubtotal, "Total Liabilities", int64(0), int64(4500), int64(-4500), "USD"},
		{KindComputed, "Assets and liabilities", nil, nil, nil, nil},
	})
	for _, col := range []string{"debit", "credit"} {
		if !slices.ContainsFunc(res.Notices, func(n Notice) bool {
			return n.Code == "mixedCurrency" && slices.Contains(n.Columns, col)
		}) {
			t.Fatalf("notices = %+v, want mixedCurrency naming %s", res.Notices, col)
		}
	}
}
```

- [ ] **Step 3: Run the three tests to see them fail**

Run, from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:

```bash
go test ./reports -run 'TestARowsRollupTotalsUnderLayoutSections|TestARowsRollupSectionOverTwoCurrenciesIsBlank|TestARollupWithRowsTotalsIsRefusedWithAPivotOrOnALayoutSection' -count=1
```

Expected: FAIL, all three. The two layout tests fail with `Run: reports: columns.3.rollup: a rollup does not go with total "rows" in a report with a pivot or a layout; columns.4.rollup: …`. The refusal test fails with `problem at "columns.1.rollup" = "a rollup does not go with total \"rows\" in a report with a pivot or a layout", want it to contain "with total \"rows\" and a layout a rollup is the cell of the rows under a section: …"`.

- [ ] **Step 4: Change the refusal in `bind.go`**

In `core/server/reports/bind.go`, in the `if c.Rollup != nil` switch (line 376), replace:

```go
			case c.Total == TotalRows && (p.spec.Pivot != nil || len(p.spec.Layout) > 0):
				b.bad(at+".rollup", `a rollup does not go with total "rows" in a report with a pivot or a layout`)
```

with:

```go
			case c.Total == TotalRows && p.spec.Pivot != nil:
				b.bad(at+".rollup", `a rollup does not go with total "rows" in a report with a pivot`)
			case c.Total == TotalRows && len(p.spec.Layout) > 0 && len(p.spec.Groups) < 2:
				b.bad(at+".rollup", `with total "rows" and a layout a rollup is the cell of the rows under a section: the report needs a group below the section group`)
```

The case above it (`len(p.spec.Groups) == 0`) and the case below it (`cp.nest > 0`) stay as they are.

- [ ] **Step 5: Run the three tests to see them pass**

Run the command of Step 3.

Expected: `ok  	tinycld.org/core/reports`. If a row of the two layout tests differs, do not change the expected rows: they were produced by this change on this tree (core 7433b88b). Stop and report the difference.

- [ ] **Step 6: Say it in the `Rollup` comment and the package docs**

In `core/server/reports/spec.go`, in the comment of `Rollup`, replace the two lines 216 and 217

```go
	// another currency than the sum's, makes the sum blank. Such a report has
	// a group.
```

with the three lines

```go
	// another currency than the sum's, makes the sum blank. Such a report has
	// a group. With a layout it has a group below the section group: a section
	// and a computed row that reads it add the rows shown under the section.
```

In `docs/packages.md`, in the `columns[].rollup` row (line 1570), replace the text

`that pair needs a group and is refused in a report with a pivot or a layout.`

with

`that pair needs a group and is refused in a report with a pivot. In a report with a layout it needs a group below the section group: a section, its subtotal and a computed row that reads the section add the rows shown under it. A fill of a group below the section needs a where that ties the record to its section.`

These two text edits were not in the planner's build; Step 7's `gofmt -l` checks the comment.

- [ ] **Step 7: Format, vet, and run the engine's rollup and layout tests**

Run, from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server`:

```bash
gofmt -l reports; go vet ./reports && go test ./reports -run 'Rollup|Layout|RowsTotal' -count=1
```

Expected: no file name from `gofmt`, no output from `go vet`, `ok  	tinycld.org/core/reports`.

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/bind.go core/server/reports/spec.go core/server/reports/rollup_test.go core/server/reports/rollup_rows_layout_test.go docs/packages.md
git commit -m "feat(reports): a rows rollup totals under layout sections"
```

Then hand back, for the driver: `NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports -count=1` (about 125 s; expected `ok`). The planner ran it once with this change: `ok  	tinycld.org/core/reports	123.179s`.

---

## What comes after this part

Not in this plan; the outline's section "State after plan part 19" holds the detail.

- 9c-2a2 (core): a fill reaches a parent group that has no base row in the period. Without it a statement section (or a class under it) with no line in the as-of month shows no account, whatever its stored months hold.
- 9c-2b (books), blocked by 9c-2a2: the accrual Balance Sheet on the stored months.
