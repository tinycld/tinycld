# Report Engine Query Scaling Implementation Plan — part 15 (task 9c-0b, part a: the Total of a report whose rows are all fill rows)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a run reads no base row and the first group fills in records, the Total row of a report with a `total: "rows"` column is a row of the statement: it adds the rows shown, with the currency rules of a run that has base rows.

**Architecture:** The total is level 0 of the group statement. Its CTE `a0` groups `w` by the currency keys, so with no base row it has no row, and the assembler then makes a Total with no cells. The fill records of level 1 already sit under one parent row in the sources' currencies (`parentKeys`, `noRowCurrencies`). The fix adds that same one row to `a0` (totals of no rows, as a fill row has), only when level 0 adds the rows below it and every currency key has a source currency; `rs0` then joins to it as to any other total. A source with no currency of its own names none for that row, while a fill row may carry a code of its own (a looked-up amount), so such a Total would add two currencies as one: it stays as it is today, with no cells. No other level, no assembler code and no client code changes.

**Tech Stack:** Go, SQLite (PocketBase), the report engine in `core/server/reports`.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-plan-part15.md` (section "Blocker") with its proof `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part15-scratch/probe.log`. Outline of all tasks: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md`.

This part holds part (a) of task 9c-0b only. Part (b) (the opening row's label names its month) is designed in `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-plan-part16.md` and is written as part 16.

## Global Constraints

- Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Core is `/Users/nas/tmp/tinycld/books-purchases/tinycld`. Branch `books/p5-report-engine-scale`. This is a core task: it edits only files under `tinycld/core/server/reports` and `tinycld/docs/packages.md`.
- Core names no package: no feature slug, collection or route in code, tests, comments or docs. Tests use the zoo sources (`zoo.*`).
- The figures of every existing test stay. The whole `reports` package passes.
- A total over two currencies stays blank, with no currency code and the `mixedCurrency` notice (R3).
- A report with no `total: "rows"` cell on its total keeps its Total over no rows as it is today (no cells). So does a report whose source has no currency of its own.
- Comments say why, not what. No timing probe in this task.
- Lane rules: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md`.

## Review Focus

- A source with no currency of its own: each account row shows its months' own code (USD, EUR, GBP), so a Total that added them would show three currencies as one figure with no code. The Total keeps no cells. Pinned by subtest "no source currency".
- A stored month in another currency than the source's: the account row is blank, so the Total is blank with no code and the run names the column. Pinned by subtest "two currencies".
- A fill report with a plain `aggregate: "sum"` column over no rows: its Total keeps no cells and its id. Pinned by subtest "a sum column keeps its total".
- A run with base rows: `NOT EXISTS (SELECT 1 FROM w)` is false, so `a0` has no extra row and no total is doubled. Pinned by the existing `TestARowsColumnShowsItsRollupOnTheInnermostGroup` (step 6).
- A report with no currency key (no money column) over no rows: `a0` groups by nothing and already has its one row, so it must not get a second. Pinned by subtest "no currency key".
- A fill that matches no record, over no rows: no account row shows, and the Total reads 0 in the source's currency (before this task it had no cells). Pinned by subtest "no fill record".

Not pinned by a test of this task (the reviewer probed it by hand, it holds): the paged path (`Options{Limit: 1, ExpandAll: true}` and its cursor) shows the same Total on each page.

## File Structure

- Modify `core/server/reports/fill.go`: two new methods of `level`, next to `fillRows` and `parentKeys`, which hold the rules they repeat.
- Modify `core/server/reports/levels.go` (`base`): one more leg of `a0`.
- Modify `core/server/reports/rollup_rows_test.go`: the zoo test and its helper.
- Modify `docs/packages.md` (the `groups[].fill` row): the rule in one sentence.

---

### Task 1: The Total adds the fill rows when the run reads no base row

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/rollup_rows_test.go` (imports; a helper and a test before `TestARowsTotalAddsTheRollupsOfTheLevelBelow`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/fill.go` (two methods before `parentKeys`)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/core/server/reports/levels.go` (`base`, after the `lv.fill` block)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md:1544`

**Interfaces:**
- Consumes (they exist, do not change them): `rowsRollupSpec(groups string) string`, `rowsRollupFilled`, `rowsRollupHaving`, `rowsRollupZoo(t) *zoo`, `rowsRollupLines(res) []rowsRollupLine`, `requireRowsRollupLines`, `namesMixed(res *Result, col string) bool` (all in `rollup_rows_test.go`); `lookupRegistered(key string)`, `RegisterSource` (as `withFeedingsCurrency` in `emptycurrency_test.go` uses them); `(c *compiler) noRowCurrencies(s *stmt) string` (`rows.go`); `(c *compiler) nextFill(n int) bool` (`fill.go`); `level.childCells []string`, `level.aggs []string`, `level.r *rowsCTE` with `r.s *stmt`.
- Produces: `func (lv *level) totalOverFillRows() bool`, `func (lv *level) noRowTotal() string`, test helper `func withLedgerCurrency(t *testing.T, code string)`. For task 9c-1 (Books): a Trial Balance spec with `Total: "rows"` + `Rollup` + `Fill` has a full Total row for an as-of month with no posted line; this holds only when every currency key of the report has a source currency (`noRowCurrency` not nil), so the planner of 9c-1 runs the probe of `handoff-plan-part15.md` again on the built core and checks the Total of the empty month before it writes the Books task.

- [ ] **Step 1: Write the failing test**

In `rollup_rows_test.go` change the import block to:

```go
import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)
```

Add this before the comment `// Above the innermost group every row adds the rows shown under it: a type's`:

```go
// withLedgerCurrency gives zoo.ledger_lines a home currency, as a package
// whose lines name a per-row currency field also does. newZoo's cleanup
// resets the registry.
func withLedgerCurrency(t *testing.T, code string) {
	t.Helper()
	src := *lookupRegistered("zoo.ledger_lines")
	src.Currency = func(core.App, *core.RequestInfo) string { return code }
	RegisterSource(src)
}

// With no line in the period every account row is a fill row, and the Total
// still adds the rows shown: it is a row of the statement, in the source's
// currency, as the account rows are. A source with no currency of its own
// names none for the Total, while each account row shows its months' code:
// the Total would add three currencies as one, so it keeps no cells (R3).
func TestARowsTotalAddsTheRollupsWhenNoBaseRowPasses(t *testing.T) {
	noLine := func(having string) string {
		spec := rowsRollupSpec(strings.ReplaceAll(rowsRollupFilled, "HAVING", having))
		return strings.Replace(spec, `"2026-07-01"`, `"2026-08-01"`, 1)
	}
	t.Run("the source's currency", func(t *testing.T) {
		z := rowsRollupZoo(t)
		withLedgerCurrency(t, "USD")
		res := z.run(t, "ann", noLine(rowsRollupHaving), Options{}, nil)
		requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{
			{KindGroup, "1000", int64(500), nil, int64(500), "USD"},
			{KindGroup, "1300", int64(300), nil, int64(300), "USD"},
			{KindTotal, "Total", int64(800), int64(0), int64(800), "USD"},
		})
	})
	t.Run("two currencies", func(t *testing.T) {
		z := rowsRollupZoo(t)
		withLedgerCurrency(t, "USD")
		res := z.run(t, "ann", noLine(""), Options{}, nil)
		total := res.Rows[len(res.Rows)-1]
		if _, has := total.Cells["debit"]; total.Kind != KindTotal || !has || total.Cells["debit"] != nil || total.Cells["debit@currency"] != nil {
			t.Fatalf("Total = %s %#v, want a blank debit cell with no code", total.Kind, total.Cells)
		}
		if !namesMixed(res, "debit") {
			t.Fatalf("notices = %+v, want mixedCurrency naming debit", res.Notices)
		}
	})
	t.Run("no source currency", func(t *testing.T) {
		res := rowsRollupZoo(t).run(t, "ann", noLine(""), Options{}, nil)
		total := res.Rows[len(res.Rows)-1]
		if total.Kind != KindTotal || len(total.Cells) != 0 {
			t.Fatalf("Total = %s %#v, want no cells", total.Kind, total.Cells)
		}
	})
	// A fill that matches no record shows no row; the Total adds none.
	t.Run("no fill record", func(t *testing.T) {
		z := rowsRollupZoo(t)
		withLedgerCurrency(t, "USD")
		spec := strings.Replace(noLine(""), `"fill":{}`, `"fill":{"where":{"op":"eq","args":[{"path":[{"field":"number"}]},{"value":"none"}]}}`, 1)
		res := z.run(t, "ann", spec, Options{}, nil)
		requireRowsRollupLines(t, rowsRollupLines(res), []rowsRollupLine{{KindTotal, "Total", int64(0), int64(0), int64(0), "USD"}})
	})
	// A report with no currency key has its one total already.
	t.Run("no currency key", func(t *testing.T) {
		const spec = `{"version":4,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"grams","path":[{"field":"grams"}],"total":"rows"}],
			"filter":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"no such note"}]},
			"groups":[{"id":"g","col":"animal","fill":{}}]}`
		res := newZoo(t).run(t, "ann", spec, Options{}, nil)
		var totals []Row
		for _, r := range res.Rows {
			if r.Kind == KindTotal {
				totals = append(totals, r)
			}
		}
		if len(totals) != 1 || totals[0].ID != "total" || totals[0].Cells["grams"] != int64(0) {
			t.Fatalf("totals = %#v, want one, id total, grams 0", totals)
		}
	})
	// A sum column adds no rows of the level below, so its Total over no rows
	// stays as it was: no cells.
	t.Run("a sum column keeps its total", func(t *testing.T) {
		z := newZoo(t)
		withEmptyAnimals(t, z)
		withFeedingsCurrency(t)
		none := `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"none"}]}`
		res := z.run(t, "ann", animalFill(`{}`, none), Options{}, nil)
		total := res.Rows[len(res.Rows)-1]
		if total.ID != `total:["GBP"]` || len(total.Cells) != 0 {
			t.Fatalf("Total = %s %#v, want total:[\"GBP\"] with no cells", total.ID, total.Cells)
		}
	})
}
```

`withEmptyAnimals`, `withFeedingsCurrency` and `animalFill` exist in the package's tests (`TestFillWithNoRowsIsInTheSourceCurrency` in `emptycurrency_test.go` uses the three); do not change them.

What the figures are: the spec's filter keeps lines from 2026-08-01, and the zoo has none (its lines are July's). The stored months are June's: cash (1000) 500 USD, idle (1300) 300 USD, bank 900 EUR, petty 250 GBP. With the having (net is not 0), 1000 and 1300 show; bank and petty are in another currency than USD, so the having cannot read their net and they are hidden; the accounts with no month have net 0 and are hidden. Total: debit 500 + 300, credit 0 (a sum of blank cells), gap 800.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestARowsTotalAddsTheRollupsWhenNoBaseRowPasses'`

Expected: FAIL in three subtests, PASS in three (add `-v` to see each). "the source's currency" and "no fill record" print `rows =` with a last row `{Kind:"total", Label:"Total", Debit:interface {}(nil), ...}`; "two currencies" prints `Total = total map[string]interface {}{}, want a blank debit cell with no code`. "no source currency", "no currency key" and "a sum column keeps its total" pass now and must still pass after the fix: they pin what the fix must not move.

- [ ] **Step 3: Add the two methods**

In `fill.go`, before the comment `// parentKeys selects the parent rows of level n: the currencies for level 1`:

```go
// totalOverFillRows is true for the total (level 0) of a report that splits by
// currency, whose first group fills in records, and that adds the rows below
// it (a "rows" cell). Its rows are grouped by the currency keys, so a run that
// reads no base row gives it no row, while the fill records still show under
// the sources' currencies (parentKeys). A currency key with no source
// currency names none on that row: a fill row may then carry a code of its
// own, and the total would add two currencies as one (R3), so that total
// stays as it is, with no cells.
func (lv *level) totalOverFillRows() bool {
	if lv.n != 0 || len(lv.c.currencies) == 0 || len(lv.childCells) == 0 || !lv.c.nextFill(0) {
		return false
	}
	return !slices.ContainsFunc(lv.c.currencies, func(ck string) bool { return lv.c.noRowCurrency(ck) == nil })
}

// noRowTotal is a0's one row when the report reads no base row: the currency
// keys the fill records sit under (noRowCurrencies), and the level's totals
// of no rows, as a fill row's are (fillRows). The level then joins the sums
// of its child rows (rs0) to it as to any other total.
func (lv *level) noRowTotal() string {
	empty := fmt.Sprintf("SELECT %s FROM w WHERE 0", strings.Join(append(slices.Clone(lv.aggs), "count(*) AS __n"), ", "))
	return fmt.Sprintf("SELECT %s, e.* FROM (%s) e WHERE NOT EXISTS (SELECT 1 FROM w)", lv.c.noRowCurrencies(lv.r.s), empty)
}
```

`fill.go` already imports `fmt`, `slices` and `strings`. Level 0's keys are the currency keys in `c.currencies` order, which is the order `noRowCurrencies` writes, so the leg's columns line up with `a0`'s first select (keys, aggregates, `__n`).

- [ ] **Step 4: Add the leg to `a0`**

In `levels.go`, func `base`, the block is now:

```go
	if lv.fill != "" {
		// The keyset is one CTE, which the level below reads by name.
		ctes = append(ctes, fmt.Sprintf("%s AS (%s)", lv.c.fillSets[lv.n], lv.fill))
		rows += "\nUNION ALL\n" + lv.fillRows()
	}
```

Add right after it:

```go
	if lv.totalOverFillRows() {
		rows += "\nUNION ALL\n" + lv.noRowTotal()
	}
```

`base` runs when the statement is rendered, after every level compiled, so `lv.childCells` is complete here. `base` has three callers: the two `render` calls in `run.go` (the paged and the unpaged statement), and `shownLevels` (`run.go`), which builds levels 1 and deeper only, so the new leg never runs there.

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet ./reports/ && gofmt -l reports/ && go test ./reports/ -run 'TestARowsTotalAddsTheRollupsWhenNoBaseRowPasses'`

Expected: no output from `go vet` and `gofmt -l`, then `ok  	tinycld.org/core/reports`.

- [ ] **Step 6: Run the tests of the rules that must not move**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/ -run 'TestARows|TestATotalOverNoRowsIsInTheSourceCurrency|TestFillWithNoRows|TestAnEmptyLayout|TestFillRowsTotalNoRows'`

Expected: `ok  	tinycld.org/core/reports`. A failure of `TestATotalOverNoRowsIsInTheSourceCurrency` or `TestFillWithNoRowsIsInTheSourceCurrency` means `totalOverFillRows` is true for a report it must not touch: check its four conditions, do not change those tests.

- [ ] **Step 7: Say it in the package docs**

In `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/packages.md` the row is:

```
| `groups[].fill` | `{ where? }` lists the target's records that have no rows, on any level, under each parent group. |
```

Replace it with:

```
| `groups[].fill` | `{ where? }` lists the target's records that have no rows, on any level, under each parent group. When a run reads no row at all, the first level's fill records sit under one Total in the source's currency; a `total: "rows"` column's cell on that Total adds the rows shown, as in a run with rows. A source with no currency of its own names none there, so that Total has no cells. |
```

- [ ] **Step 8: Run the whole package once**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test ./reports/`

Expected: `ok  	tinycld.org/core/reports` (about 2 minutes).

- [ ] **Step 9: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/fill.go core/server/reports/levels.go core/server/reports/rollup_rows_test.go docs/packages.md
git commit -m "fix(reports): the total adds its fill rows when the run reads no base row"
```
