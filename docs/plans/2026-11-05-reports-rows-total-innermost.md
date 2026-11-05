# Reports: `total: "rows"` on the innermost group — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Tests-first from a different agent: a test writer writes and runs every test RED, an implementer makes them green without editing tests, a reviewer compares the code with the plan text cited below.

**Goal:** A `total: "rows"` column's innermost group cell is the column computed over that group's base rows; the levels above, layout sections and the report Total add the cells of the rows they show (after `having`), per currency.

**Architecture:** One change in `core/server/reports/levels.go` `rowsTotal` (and the matching branch in `pivot.go` `pivotCells`): on the innermost group level (`lv.n == len(groups) && lv.n >= 1`), a column with an aggregate (its `aggregate` field, or an aggregate operator in its formula) uses `recompute`; a column with no aggregate keeps the sum of its detail cells (F17). Level 0 with no groups keeps the sum of detail cells. The levels above already add their children (`childSum`), so the fix reaches them, layout sections and the Total through `rowsCells`.

**Tech Stack:** Go, SQLite (PocketBase), `go test`.

**Spec:** `docs/plans/2026-11-01-report-engine-v2.md` line 93 (E2) and line 155 (R8, F17); R6 (line 151) for fill rows.

## Global Constraints

- Spec E2: "a `rows` column's group, subtotal and total cells are the sum of the cells of the rows directly under them that the result shows (after `having`), per currency. The innermost group computes from its base rows. With no groups, the total adds the detail cells. Pivot cells of a `rows` cell column follow the same rule. Layout sections add their shown rows; computed rows are unchanged. A number or amount column with no aggregate also adds its rows: its detail cells, and per group the sum of its base rows (F17)."
- Spec R8: "allowed on any number or amount column, with or without an aggregate (F17: with none, a group adds its base rows' values), that is not a running value or percent; with no groups it adds the detail cells."
- Spec R6: fill rows "add nothing to `recompute` totals, add their (blank or zero) cells to `rows` totals".
- Core must not name a package: tests use the fictional zoo registry only (`zoo_test.go`). No real package name in core.
- Go: gofmt, `go vet ./reports/`. Comments explain why.

## Review Focus

1. A formula column whose aggregate is inside the formula (`if(sum(amount) > 0, sum(amount))`, no `aggregate` field): innermost group = the formula over the group's base rows, not the sum of per-row cells.
2. A fill row (record group with `fill`, no base rows) on the innermost level: the formula over no rows (blank for the `if` form, 0 for a `sum` aggregate), and it adds that to the parent/Total (blank adds nothing).
3. `having` on the innermost group hides a group: the Total and the level above add nothing for it.
4. Multi-currency: each currency's group rows and Total add only that currency's shown rows.
5. A column with `aggregate: "avg"` and `total: "rows"`: the innermost group is the average of its base rows; the Total adds the shown group cells.

---

### Task 1 (test writer): tests RED

**Files:**
- Modify: `core/server/reports/rowstotal_test.go`
- Modify: `core/server/reports/run_test.go` (new helper beside `requireRows`)
- Modify: the reports test files whose ordered detail-row asserts switch to the helper
- Create (if needed): a test file for the helper's own tests, e.g. `core/server/reports/orderedrows_test.go`

**Interfaces (produces):**
- `requireOrderedRows(t testing.TB, spec string, res *Result, what string, want ...any)` — asserts `rowsOf(res, what)` equals `want` in order, and fails the test first when the spec (JSON) has no `sort` entry, with a message saying a report with no sort returns detail rows in record-id order. Test files only.

- [ ] **Step 1: Acceptance tests in `rowstotal_test.go`** (fictional zoo ledger: feedings with signed `cost`, grouped by the `animal` record group, or a new zoo source in `zoo_test.go` if feedings cannot hold the shape). Columns: `Debit = if(sum(cost) > 0, sum(cost))`, `Credit = if(sum(cost) < 0, -sum(cost))`, both `total: "rows"`. Cases:
  - each group row shows its net on one side only (a group with +500 and -200 shows Debit 300, Credit blank);
  - a filled group with no base rows (`fill`) shows blank on both;
  - a group hidden by `having` adds nothing to the Total;
  - the Total adds the shown group cells; for a balanced ledger Debit total = Credit total;
  - multi-currency rows total per currency (one Total per currency);
  - two group levels: the innermost computes from its base rows, the outer level adds its shown child groups (not its own base rows' net).
- [ ] **Step 2: Pivot and layout under the rule:** a `rows` pivot cell column on the innermost level computes over the base rows with that pivot value; a layout section over one group level shows the column over its base rows and the section total adds its shown rows.
- [ ] **Step 3: Fix the tests that pin the old rule** (read each against E2/R8): `TestRowsTotalOnANumberWithoutAnAggregate` and `TestAFillRowAddsNothing`, plus any other (e.g. the `avg` assertion in `TestRowsTotalsAddTheDetailCellsOfEveryColumnKind`) whose expected values contradict "the innermost group computes from its base rows". F17 cases (no aggregate) keep the sum of base rows' values; no-groups totals keep the sum of detail cells. Comment each changed test with the spec rule it asserts.
- [ ] **Step 4: Ordered-rows helper and its tests:** add `requireOrderedRows`; test it with a fake `testing.TB` that records failure: it fails on a spec with no sort, passes with one. Switch existing `requireRows` calls that assert the ORDER of detail rows in `core/server/reports/*_test.go` to it (only those; group-only or single-row asserts stay).
- [ ] **Step 5: Run RED:** `cd core/server && go test ./reports/ -run '<new and changed tests>'`. Expected: the acceptance and changed tests fail on values (not compile errors in non-test code); the helper tests and switched calls pass. Record the failing output.
- [ ] **Step 6: Commit** `test(reports): pin the innermost rows-total rule and ordered detail rows`.

### Task 2 (implementer): green, without editing tests

**Files:**
- Modify: `core/server/reports/levels.go` (`rowsTotal`, ~645–671, and its comment)
- Modify: `core/server/reports/pivot.go` (`pivotCells`, ~120–137) if the pivot test is red
- Modify: `docs/packages.md` (`columns[].total`, ~1290), `core/help/reports.md` "Totals"

- [ ] **Step 1:** In `rowsTotal`, on the innermost group level (`lv.n >= 1 && lv.n == len(lv.c.p.groups)`), return `lc.recompute(cp, cp.at)` when the column has an aggregate (`cp.col.Aggregate != ""`, or an aggregate operator anywhere in its formula — `aggregateOp`); otherwise keep `COALESCE(sum(<detail cell>), 0)` (F17). Keep the `detailCell` call first so refusals (`nest`, running value, non-number) stay. Level 0 with no groups keeps the detail-cell sum. Keep the currency of the recompute result.
- [ ] **Step 2:** In `pivotCells`, apply the same rule on the innermost level (drop the detail-cell override for a column with an aggregate).
- [ ] **Step 3:** Docs: `columns[].total` row and help "Totals" say the lowest group level computes the column from its own rows, and the levels above and the Total add the rows shown.
- [ ] **Step 4:** `cd core/server && go test ./reports/... && go build ./... && go vet ./reports/ && gofmt -l reports`; `pnpm run checks` from the repo root.
- [ ] **Step 5: Commit** `fix(reports): rows total computes the innermost group over its base rows`.

### Task 3 (reviewer)

- [ ] Compare `rowsTotal` and `pivotCells` with the spec text at `docs/plans/2026-11-01-report-engine-v2.md` lines 93 and 155 (not only the tests): innermost group, F17, no groups, pivots, layout sections, having, fill rows, currencies. Check the helper refuses a spec with no sort and the switched asserts are order asserts.
