# Report percent of a section and period pivots (P1–P4) — outline

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. The tasks are in the parts below; this file is the outline every part shares.

**Scope:** checkpoint 0 of helix unit p5-profit-and-loss: P1 `percentOfSection`, P2 `pivot.range`, P3 `pivot.order: "spec"`, P4 bucket labels in words, with their builder controls, help, docs and e2e.

**Goal:** Core's report engine, spec (still version 4), client view, builder, help and docs gain a percent of a layout section's total, a pivot over every bucket of a date range, pivot columns before their total, and bucket labels typed so the view writes them in words.

**Verdict: MAJOR — the lane holds `exclusive` (taken 2026-11-09).** No migration, no generator change, no removed export, no spec version bump (every field optional at version 4; `Load` decodes strictly, so an older server refuses a spec that uses one). P4 changes the text of every bucketed label the view shows ("2026-01" → "Jan 2026"); P3 changes a pivot result's column order when set; P1/P2 edit the shared engine files. Core merge pre-approved on green checks (spec header, ledger header) unless the runner reports an out-of-plan file, a deferral, a rebuttal or STOPPED.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/profit-and-loss-design.md` §4 (every column of every row; "Core tests"; the verdict). Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-profit-and-loss/cp/cp0.md` (Checkpoint 0: Files, Interfaces, "Tests it must add", Do).

**Build order (spec §4):** P4 → P3 → P2 → P1 → builder and help.

**Architecture.**
- *P4.* `registry.go` gains five label types (`Month`, `Quarter`, `Year`, `FiscalYear`, `Day`) that are never a field's type. A bucketed pivot column (`pivotLabel`, pivot.go) and a bucketed group row (`group`, assemble.go) carry `labelType` = the bucket's type and `labelValue` = the raw key; `Label` text and `PivotColumn.Key` / group key stay the raw key. The client's `typedLabelText` writes a bucket type in words (`bucketWords`, dates.ts), so headings (`columnName`), group and subtotal labels (`rowLabelText`) and the column measure read words.
- *P3.* `Pivot.Order == "spec"`: `columns()` (assemble.go) puts the pivot columns immediately before their cell column instead of after every column. `c.p.cols` keeps the spec's order (it already does; fit.go's stale comment is corrected). The client's `expandPivots` (table-lines.ts) reads the result order: when the cell column follows its pivot columns, an arranged cell naming it expands to the pivot columns then the cell column.
- *P2.* `Pivot.Range` names a `dateRange` param. When it resolves to both bounds, `pivotPass` builds the bucket series in Go (`bucketSeries`, pivot.go) from `from`..`to` with no SQL pass, keeps the first `maxColumns` and counts the rest for the `pivotCap` notice. Sums over a bucket with no row are already 0 (`aggregateSQL` `COALESCE(sum(…), 0)`); min/max/avg stay blank. Unset or All dates: today's pass.
- *P1.* `{ op: "percentOfSection", args: [{ col }], subtotal, scale }` is a column's whole formula in a layout report. It compiles as a window op (so sort, having, label, rows-total and nesting refusals apply) to a NULL placeholder of type Percent; after the statements run, `assemble` reads each named section's total per currency block from the same raw rows `layoutRows` draws the section from (with its `within` rows, or the empty-section leg), and `cells` fills the column on every detail, group, subtotal, section and computed row: the row's own value of the argument column (on a pivot report, the cell column = the row's total across the pivot) ÷ that total × 100 at `scale`, rounded by `tc_pct`'s Go function (`sqlPct`). A computed row and a section total thus divide their own cell.
- *Builder.* Formula palette: "Percent of a section's total" (only on a layout report; section picker writes `subtotal`). Pivot section: "Show every <bucket> of <date range param>" (writes `range`) and "Total after the pivot columns" (writes `order: "spec"`). Draft round-trip keeps every new field; a node the editor cannot write stays read-only.

**Tech stack:** Go 1.26 (PocketBase, SQLite), TypeScript (React Native + web), zod v4, vitest, Playwright.

Paths are relative to `/Users/nas/tmp/tinycld/books-purchases/tinycld` (core repo, branch `books/p5-profit-and-loss`, from main 95077d35). Go tests: `cd core/server && go test ./reports/ -run '<Regex>' -count=1`. Vitest: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run <file>` from the repo root. E2E: one file, ports checked first, `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts`.

## Renamed names

None. Every Produces name of the brief is kept: `percentOfSection` with `Node.Subtotal`; `Pivot.Range` / `range`; `Pivot.Order` / `order` value `"spec"` (Go constant `PivotOrderSpec`); `reports.Month`, `reports.Quarter`, `reports.Year`, `reports.FiscalYear`, `reports.Day`; `PivotColumn.LabelType` / `LabelValue`; client `FieldType` gains `'month' | 'quarter' | 'year' | 'fiscalYear' | 'day'`; `typedLabelText` writes the words.

## Books plan changes (for the driver; amend before checkpoint 1)

- **B1 — the by-month pivot's cell needs `aggregate`.** `bind.go` `groupsAndPivot` refuses a pivot cell column with no `aggregate` ("the pivot cell is a column with an aggregate, …"). Spec §5 writes `amount` = `coalesce( sum(signed), 0 )` with no `aggregate`, so the by-month spec as written is refused at `pivot.cell`. The engine already supports the same figures as `amount` = `{ formula: signed, aggregate: "sum" }` (a sum over no rows reads 0: `aggregateSQL` is `COALESCE(sum(…), 0)`). Checkpoint 0's fictional statement test (Task 9) uses that shape. Books must write `amount` so in the by-month spec (and may in the P&L spec, for one shape). This is not a missing capability, so core is not changed for it.
- **B2 — label text.** `Row.Label` and `ResultColumn.Label` of a bucketed label stay the raw key ("2026-01"); the words come from the client (R8). A Books Go test pins `labelType` and the key (brief D7).

## Global constraints

- Core names no package: Go tests use `zoo_*`; e2e use the `salesstub` package (its shipped reports live in `tests/scripts/reports-stub/server/register.go.txt`).
- "No version bump (Q12): every field is optional and added at spec version 4."
- "Each extension carries its builder round-trip (`lib/builder/draft.ts` keeps the field; a part the builder cannot edit stays read-only) and its row in `docs/packages.md` ("Fields added in version 4")."
- P4: "month "Jan 2026", quarter "Q1 2026", year "2026", fiscal year "FY 2026" (the fiscal year's end year), day in the short day form. The raw key stays the column's `pivot.key` and the group key."
- P2: "every bucket the range's `from`..`to` touches is a column, in order, whether or not a row falls in it (cells 0 for a summed amount, blank otherwise as today); at most `maxColumns`, then the `pivotCap` notice. On an unset range or All dates, the columns are the buckets that have rows."
- P1: "no figure when that total is 0 or blank … Type Percent."
- E2E: `test.describe('reports: period pivot', …)`, one file at a time on ports 7210/7211; no raw PocketBase writes (sales are added through the stub's UI, `addSale`); `page.goto` only through the existing helpers.
- Biome, no `any`, no biome-ignore, no `console.*`; comments say why; Go keeps the engine's style (SQL values bound with `s.bind`, refusals through `b.bad` / `invalid`).

## Review focus

1. A month key that is not in the bucket's form (a `labelValue` from an older server or a hand-built model): the view shows the key as it is, never "undefined 2026" (Task 2 test `bucketWords keeps a key it cannot read`).
2. A range far wider than `maxColumns` (a day bucket over 1900-01-01..2100-12-31): the engine builds at most `maxColumns` keys and counts the rest arithmetically, with no long loop (Task 5 test `TestBucketSeriesCountsWithoutBuildingPastTheCap`).
3. A range param on All dates (both bounds resolve to "") or a pivot with no `range` keeps today's columns, never an empty pivot (`params.go` `rangeValue` already refuses a `from` after its `to`) (Task 6 test `TestPeriodPivotOnAllDatesKeepsTodaysColumns`).
4. Two currencies in a layout report with a percent column: each currency block divides by its own section total (Task 8 test `TestPercentOfSectionDividesByItsOwnCurrencysTotal`).
5. A section folded closed: the percent still divides by the section's whole total, because the total is read from the statement rows, not from the rows on screen (Task 8 test `TestPercentOfSectionReadsTheTotalWhenTheSectionIsFolded`).

## File structure

| File | Change | Task |
|---|---|---|
| `core/server/reports/registry.go` | `Month`, `Quarter`, `Year`, `FiscalYear`, `Day` FieldType constants; `bucketType(bucket string) FieldType` | 1 |
| `core/server/reports/pivot.go` | `pivotLabel` types a bucketed column label | 1 |
| `core/server/reports/assemble.go` | `group` types a bucketed group row's label | 1 |
| `core/server/reports/run.go` | `PivotColumn` doc: LabelType/LabelValue also type a bucket | 1 |
| `core/server/reports/bucketlabel_test.go` (new) | P4 Go tests | 1 |
| `core/reports/lib/types.ts` | `BUCKET_LABEL_TYPES`, `BucketLabelType`, `isBucketLabelType`; `FIELD_TYPES` gains them | 2 |
| `core/reports/lib/dates.ts` | `bucketWords(type, key)` | 2 |
| `core/reports/lib/format.ts` | `typedLabelText` writes a bucket type in words | 2 |
| `core/reports/lib/builder/filter.ts` | `OPS_BY_TYPE` entries for the five label types (no conditions) | 2 |
| `core/reports/lib/__tests__/dates.test.ts`, `format.test.ts` | P4 vitest | 2 |
| `tests/e2e/reports-group-pivot.spec.ts` | lines 66–67: month headings "Jan 2026", "Feb 2026" | 2 |
| `core/server/reports/spec.go` | `Pivot.Range`, `Pivot.Order`, `PivotOrderSpec` | 3 (Order), 5 (Range) |
| `core/server/reports/bind.go` | `pivot.order` refusals | 3 |
| `core/server/reports/assemble.go` | `columns()` places pivot columns before their cell column under `order: "spec"` | 3 |
| `core/server/reports/fit.go` | comment at `fitRefusals` (label column is the spec's first column in both orders) | 3 |
| `core/server/reports/pivotorder_test.go` (new) | P3 Go tests | 3 |
| `core/reports/lib/spec.ts` | pivot `range`, `order`; `SpecNode.subtotal` doc names percentOfSection | 4 |
| `core/reports/lib/table-lines.ts` | `expandPivots` reads the result order | 4 |
| `core/reports/lib/__tests__/spec.test.ts`, `table-lines.test.ts`, `column-measure.test.ts` | P2/P3 client vitest; column-measure pins headings in words and the label column under `order: "spec"` (no code change in `column-measure.ts`, R10) | 4 |
| `core/server/reports/pivot.go` | `bucketSeries`, `bucketIndex`, `bucketKey`; `pivotPass` over a resolved range | 5, 6 |
| `core/server/reports/bind.go` | `pivot.range` refusals (`pivotRange`) | 5 |
| `core/server/reports/periodpivot_test.go` (new) | P2 Go tests | 5, 6 |
| `core/server/reports/pivot_test.go` | `TestPivotBucketOfARowWithNoDayHasNoTypedLabel` (a P4 case added during SDD, named here at gate 3) | 1 |
| `core/server/reports/formula.go` | `percentOfSection` window op; `subtotal` is a leaf only when `op` is empty, refused on any other op | 7 |
| `core/server/reports/detail.go` | `windowSQL`: `percentOfSection` placeholder (NULL, Percent, win) | 7 |
| `core/server/reports/rows.go` | `windowOp` includes `percentOfSection` | 7 |
| `core/server/reports/bind.go` | `plan.percents`, `sectionPercent`, `sectionPercents` (refusals and the walk over every spec formula) | 7 |
| `core/server/reports/layout.go` | `cellOf`: a percent column's cell is NULL in SQL; `sectionRow` (split out of `layoutRows`) | 7, 8 |
| `core/server/reports/assemble.go` | `assemble` reads `sectionBases`; `cells` fills percent columns; `runner.assembleErr` | 8 |
| `core/server/reports/run.go` | `runner.sectionBases`, `runner.assembleErr`; `run` and `openGroups` (now `([][]any, error)`) return `assembleErr` | 8 |
| `core/server/reports/section_percent.go` (new) | `sectionPercent` bind (7); `readSectionBases`, `sectionPercentCell` (8) | 7, 8 |
| `core/server/reports/sectionpercent_test.go` (new) | P1 Go tests | 7, 8 |
| `core/server/reports/fuzz_test.go` | `sectionPercentParity`, `periodPivotParity` in `FuzzRuleParity` | 9 |
| `core/server/reports/periodstatement_test.go` (new) | the fictional P&L-by-month statement on `zoo_*` (last Go task) | 9 |
| `core/reports/lib/builder/formula-parts.ts`, `formula-context.ts`, `formula-type.ts`, `formula-ops.ts` | "Percent of a section's total": part, arity, label, type, offer only on a layout report, operand a number column, `sectionPercentColumns` | 10 |
| `core/reports/lib/builder/__tests__/formula-ops.test.ts`, `formula-type.test.ts`, `formula-context.test.ts`, `formula-fixture.ts` | builder vitest for P1 | 10 |
| `core/reports/lib/builder/formula-describe.ts` | "<col> as % of <section>" | 11 |
| `core/reports/components/builder/ColumnsSection.tsx` | `offeredAggregates` offers no total for a `percentOfSection` column (the engine refuses one; draft issue `PERCENT_NO_TOTAL`); added during SDD, named here at gate 3 | 11 |
| `core/reports/components/builder/report-formula.tsx` | `SectionLeaf` (pick the section; writes `subtotal`); the sections come from `formulaContext`, which already reads the layout | 11 |
| `core/reports/lib/builder/draft.ts` | `cellColumnIds` leaves percent columns out (11); pivot draft `range`, `totalAfter`; read/write; `draftIssues` for a range that names no date range param (12) | 11, 12 |
| `core/reports/lib/builder/__tests__/formula-describe.test.ts`, `draft.test.ts` | P1 description and draft round trip (11); P2/P3 round trip and issues (12) | 11, 12 |
| `core/reports/lib/builder/pivot.ts` (new) | `pivotRangeChoices(draft, catalog)` | 12 |
| `core/reports/components/builder/PivotSection.tsx` | "Show every <bucket> of" select, "Total after the pivot columns" toggle | 12 |
| `core/reports/components/builder/parts.tsx` | `BucketSelect` `onValueChange` takes the value | 12 |
| `core/reports/lib/builder/__tests__/pivot.test.ts` (new) | `pivotRangeChoices` vitest | 12 |
| `tests/scripts/reports-stub/server/register.go.txt` | shipped "Stub period statement" and "Stub period pivot" | 13 |
| `tests/e2e/reports-period-pivot.spec.ts` (new) | `reports: period pivot` desktop + phone | 13 |
| `tests/e2e/reports-helpers.ts` | header comment lists every shipped stub report, the two new ones included | 13 |
| `core/help/reports.md` | "Show a row as a percent of a section", "Pivot by month across a whole period", the **Pivot** bullet names "Total after the pivot columns", pivot and group-by-date text names the words | 14 |
| `docs/packages.md` | "Fields added in version 4": `columns[].formula` `percentOfSection` (`subtotal`), `pivot.range`, `pivot.order`, bucket label types | 14 |
| `core/reports/lib/__tests__/statements-docs.test.ts` | the docs check gains the new headings and fields | 14 |

The brief's Files list (cp0.md) names `pivot_test.go`, `layout_test.go` and `formula_cases_test.go`. The plan puts its Go tests in new test files, one per extension (`bucketlabel_test.go`, `pivotorder_test.go`, `periodpivot_test.go`, `sectionpercent_test.go`, `periodstatement_test.go`), as the brief allows ("one test file per extension as the core plan chooses"). `layout_test.go` and `formula_cases_test.go` stay unchanged; `pivot_test.go` gains one P4 case (`TestPivotBucketOfARowWithNoDayHasNoTypedLabel`).

## Tasks

Part 1 (`2026-11-09-report-percent-and-period-pivots-part-1.md`): Tasks 1–8 (P4, P3, P2, P1 in the engine and client lib). Part 2 (`…-part-2.md`): Tasks 9–14 (parity and the statement-shape test, builder, e2e, help and docs). Part 2 split the builder P1 task in two (10 library, 11 editor), so the pivot builder is Task 12, the e2e Task 13 and the docs Task 14.

1. **P4 Go — bucket label types.** Produces `reports.Month = "month"`, `reports.Quarter = "quarter"`, `reports.Year = "year"`, `reports.FiscalYear = "fiscalYear"`, `reports.Day = "day"` (FieldType, never a field's type: `validateField` still refuses them); `bucketType(bucket string) FieldType`; a bucketed pivot column carries `PivotColumn{Key: <raw key>, LabelType: <bucket type>, LabelValue: <raw key>}`; a bucketed group row and its subtotal carry `Row.LabelType` = bucket type, `Row.LabelValue` = raw key; `Label` text and `GroupKey` unchanged. Fiscal year keys are `int64` end years.
2. **P4 client — words.** Produces `BUCKET_LABEL_TYPES = ['month','quarter','year','fiscalYear','day'] as const`, `type BucketLabelType`, `isBucketLabelType(type: FieldType): type is BucketLabelType` (types.ts); `FieldType` gains the five; `bucketWords(type: BucketLabelType, key: string | number): string` (dates.ts); `typedLabelText` writes them ("Jan 2026", "Q1 2026", "2026", "FY 2026", "Jan 5, 2026"); e2e `reports-group-pivot.spec.ts` headings in words.
3. **P3 Go — pivot order.** Produces `Pivot.Order string` (JSON `order`), `const PivotOrderSpec = "spec"`; refusals at `pivot.order` (`order is "spec"`; the cell column is not the first column); `columns()` order: spec columns with the pivot columns immediately before the cell column.
4. **P2/P3 client lib.** Produces pivot schema `range?: string`, `order?: 'spec'` in `lib/spec.ts`; `expandPivots` lays the pivot columns then the cell column when the result has the cell column after its pivot columns; pins in `column-measure.test.ts`.
5. **P2 Go — range binding and the bucket series.** Produces `Pivot.Range string` (JSON `range`); `bucketSeries(bucket, from, to string, fiscalStart, max int) (keys []any, total int)`; refusals at `pivot.range` (unknown param 422 `unknown_param`; not a `dateRange`; not over a date; no bucket).
6. **P2 Go — the pivot pass over the range.** Produces `pivotPass` building columns from `bucketSeries` when the range param resolves to both bounds (typed labels as Task 1; `pivotCap` notice with `LeftOut = total − max`); unchanged otherwise.
7. **P1 Go — bind and compile.** Produces `type sectionPercent struct { col, arg *colPlan; section string; scale int }`, `plan.percents map[string]*sectionPercent` (by column id), `(b *binder) sectionPercents(p *plan)`; formula op `percentOfSection` (window op, Percent, SQL NULL placeholder); refusals: outside a layout report, `subtotal` naming no section (422 `unknown_subtotal`), argument not a `{col}` of a number or amount column, not a column's whole formula (inside any other operator, window ones included), on an aggregate column, read by another formula, a pivot whose `cell` names it, `subtotal` beside another op.
8. **P1 Go — the figures.** Produces `(rn *runner) sectionRow(t *tree, item *LayoutItem, total map[string]any, byKey map[string]map[string]any) map[string]any` (split out of `layoutRows`), `(rn *runner) sectionsByKey(t *tree, ck string) map[string]map[string]any`, `runner.sectionBases map[string]any` (key: percent column id + `"|"` + currency key), `(rn *runner) readSectionBases(t *tree, total map[string]any)`, `(rn *runner) sectionPercentCell(sp *sectionPercent, row map[string]any) any`, `runner.assembleErr error`; every row kind but opening gets the percent; zero or blank total → nil.
9. **Parity and the statement shape (last Go task).** Produces `(z *zoo) sectionPercentParity`, `(z *zoo) periodPivotParity` called from `FuzzRuleParity`; `TestAFictionalProfitAndLossByMonthRunsOnTheMergedEngine` (periodstatement_test.go).
10. **Builder — "Percent of a section's total" (library).** Produces part `percentOfSection` in `WINDOWS` and `SCALED_PARTS`, `ARITY.percentOfSection = [1, 1]`, `PART_LABELS.percentOfSection = "Percent of a section's total"`, `ReportFormulaContext.sections: readonly { id: string; title: string }[]` and `onlyColumns?: boolean`, offered only at the root when `sections.length > 0` and windows are; operand offers number/amount columns only; `isEditableNode` accepts `subtotal` on `percentOfSection` only; `sectionPercentColumns(columns): ReadonlySet<string>` (formula-context.ts), which no other formula reads.
11. **Builder — section picker, description, draft.** Produces `SectionLeaf` test id `<testID>-section`; `describeNode` "<col> as % of <section title>"; `cellColumnIds` leaves percent columns out; the draft round trip of a `percentOfSection` column.
12. **Builder — pivot range and order.** Produces draft `pivot.range: string` ('' = none), `pivot.totalAfter: boolean`; `specPivot` writes `range` / `order: 'spec'`; `draftFromSpec` reads them; `pivotRangeChoices(draft, catalog): { value: string; label: string }[]` (labels "Show every month of Period"; empty unless the pivot is over a date with a bucket and the spec has a `dateRange` param); test ids `report-builder-pivot-range`, `report-builder-pivot-total-after`.
13. **E2E.** Produces shipped stub reports `salesstub.period_statement` ("Stub period statement": sections Open / Paid by a hidden status column, computed "All sales", `share` = `percentOfSection({col amount}, subtotal open, scale 1)`) and `salesstub.period_pivot` ("Stub period pivot": customers × months of `period`, `range: "period"`, `order: "spec"`); `tests/e2e/reports-period-pivot.spec.ts` (desktop: percent figures; month headings in words with the middle month 0.00 and the total after the months; phone 390×844: `waitForTableFit`, no sideways scroll, words).
14. **Help, docs and the docs check.** `core/help/reports.md` sections and sentences; `docs/packages.md` rows; `statements-docs.test.ts` gains them.

## Rulings

- **R1 — P1 is computed in Go after the statements, reading the section's total from the statement rows.** The spec's engine column puts the denominator in `detail.go` `windowSQL` ("read from the section's total") and computed rows / section totals in `layout.go`. A SQL denominator cannot read a section total that includes the section's `within` rows (computed legs), and a level CTE cannot read `o1` without a cycle when a "rows" total makes `o1` read `o2`. So `windowSQL` compiles and checks the op and returns a NULL placeholder; `assemble` reads each section's total (the same raw row `layoutRows` draws, `within` rows and the empty-section leg included) per currency block, and `cells` divides each row's own argument cell by it with `sqlPct` (the Go function behind `tc_pct`: half away from zero, NULL on a zero or blank whole). Cost if wrong: none to figures; a reviewer reading SQL finds the column NULL there. A later feature that needs the value in SQL (a sort on it) is refused today anyway (window ops are not sort keys).
- **R2 — `percentOfSection` is a column's whole formula, and nothing reads that column.** Bind refuses it inside any other operator (window ones included: the spec's "inside another window operator"), in any place but `columns[i].formula`, on a column with `aggregate`, and any `{col <percent column>}` in another formula (its value is filled after the statements, so a reader in SQL would see NULL). Cost if wrong: an author cannot write "share − 100"; that needs a derived column over the section total, which no spec asks for.
- **R3 — a percent column's cell on an opening row stays blank**, and `detail: "blank"` keeps it off detail rows (the fill writes a cell only where the row already has the column).
- **R4 — a range pivot's columns are exactly the range's buckets.** A row outside `from`..`to` (a filter that does not bound the date) falls in no month column but still counts in the cell column's total. Cost if wrong: such a spec's months do not add to its total; the P&L and every builder-made range pivot filter by the same param.
- **R5 — the bucket series is built in Go**, not by SQL: keys in the forms `bucketSQL` gives (`"2026-01-05"`, `"2026-01"`, `"2026-Q1"`, `"2026"` as text; fiscal year as `int64` end year, from the source's fiscal start month, `c.fiscal`). A pivot over a datetime field buckets by the stored text's first 10 characters, as `bucketSQL` does.
- **R6 — the cap counts arithmetically.** `bucketSeries` returns the first `max` keys and the total count; the notice's `LeftOut` is `total − max`. Cost if wrong: none.
- **R7 — `order: "spec"` is refused when the cell column is the spec's first column** (its pivot columns would come before the label). Cost if wrong: such a spec has no use.
- **R8 — the server keeps the raw key as label text.** `Row.Label`, `ResultColumn.Label` and `PivotColumn.Key` / group key stay "2026-01"; `labelType` + `labelValue` carry the type; only the client writes words (brief D7). Core has no export path today (no CSV/PDF in `core/reports` or `core/server/reports`); the view's text functions (`columnName`, `headingText`, `rowLabelText`) are the one place labels are written, so the words reach any later export. Existing Go tests that read `Label` "2026-01" stay green.
- **R9 — "every FieldType switch handles them".** Go: `validateField` keeps refusing the five as a field's type (pinned); `neverCut` / `typedLabelTypes` leave them out, so the label column's fit is unchanged (spec §4 "fit.go label column unchanged"). Client: `FieldType` gains them; `OPS_BY_TYPE` gives them no conditions; `TYPED_LABELS` in column-measure.ts is unchanged (a bucket label is measured as text, in words).
- **R10 — `column-measure.ts` needs no code change.** It measures a heading through `headingText` → `columnName` → `typedLabelText` (words after Task 2) and takes `columns[0]` as the label (unchanged by P3, R7). `column-measure.test.ts` pins both. Cost if wrong: a gate-3 reader sees the file listed in the brief unchanged; the pin explains why.
- **R11 — the e2e drives shipped stub reports.** Both e2e reports are shipped by the `salesstub` stub (published, revision 1), so the e2e needs no builder steps and tests the engine and view; the builder controls are pinned by builder vitest (Tasks 10–12). Sales are added through the stub's UI by a fresh invited member, who reads only their own sales, so the figures are exact.

## Open questions

None for the human. B1 is a Books plan amendment the driver makes (not a core gap).

## Coverage check

| Brief / spec item | Task |
|---|---|
| Delivers P1 / P2 / P3 / P4 engine, spec, client, builder, help, docs | 7–8 / 5–6 / 3 / 1 (engine); 4, 2 (client); 10, 11, 12 (builder); 14 (help, docs) |
| Files `spec.go` (`Pivot.Range`, `Pivot.Order`; `Node.Subtotal` reused) | 3, 5, 7 |
| Files `registry.go` (bucket FieldTypes) | 1 |
| Files `formula.go` (window op) | 7 |
| Files `detail.go` (`windowSQL`) | 7 |
| Files `layout.go` (computed rows and section totals divide their own cell) | 7, 8 |
| Files `pivot.go` (`pivotPass` series) | 5, 6 (and 1 for `pivotLabel`) |
| Files `bind.go` (refusals; `c.p.cols` order) | 3, 5, 7 (`c.p.cols` already keeps spec order; R in Task 3) |
| Files `assemble.go` (`columns()`, `display`/`pivotLabel` typed labels) | 1, 3, 8 |
| Files `fit.go` (label column unchanged) | 3 |
| Files `run.go` (`PivotColumn.LabelType`) | 1, 8 |
| Files client `spec.ts`, `types.ts`, `format.ts`, `dates.ts`, `table-lines.ts`, `column-measure.ts`, `builder/draft.ts` (+ `__tests__`) | 4, 2, 2, 2, 4, 4 (R10), 11 and 12 |
| Files builder formula editor + pivot editor components | 11 (`report-formula.tsx`), 12 (`PivotSection.tsx`, `parts.tsx`) |
| Files `core/help/reports.md`, `docs/packages.md` | 14 |
| Files e2e `reports-period-pivot.spec.ts` (new), `reports-group-pivot.spec.ts`, `reports-view.spec.ts`, every core e2e reading a bucketed label | 13, 2 (group-pivot :66–67). `reports-view.spec.ts:497` builds a month pivot but reads only the `pivotCap` notice text: unchanged. `grep -rn` over `tests/e2e` for `-01'`, `2026-`, `bucket`, `pivot` heading reads finds no other bucketed label read (`reports-wrap.spec.ts:616` builds days, reads no bucket). |
| Produces Go `percentOfSection` (`args [{col}]`, `subtotal`, `scale`) | 7, 8 |
| Produces `reports.Pivot.Range` / `range`; `reports.Pivot.Order` / `order` "spec" | 5; 3 |
| Produces `reports.Month`, `Quarter`, `Year`, `FiscalYear`, `Day` on `PivotColumn.LabelType` (key in `LabelValue`) and on a bucketed group row's `LabelType`; raw key in `PivotColumn.Key`; every FieldType switch handles them | 1 (R9) |
| Produces client `FieldType` additions, `ResultColumn.pivot.labelType`, `typedLabelText` words | 2 |
| Go P4: month, quarter, year, fiscal year (start 7: Jul 2026–Jun 2027 = FY 2027), day typed on pivot columns and group rows; raw keys unchanged | 1 |
| Go P3: order with `order: "spec"` and without; fit's label column unchanged in both | 3 |
| Go P2: Jan–Mar with empty Feb (sum 0, max null); mid-month range → Jan, Feb, Mar; quarter; wider than `maxColumns` + notice count; All dates and unset keep today's; refusals (not dateRange, no bucket, not over a date) | 5 (refusals, series), 6 (runs) |
| Go P1: detail, group, second-level subtotal, A's total (100.0), B's total, computed row; type percent; pivot report over the cell total (D10); zero/blank total → null; each refusal (outside layout; unknown section; argument not `{col}` of number; inside a window op; pivot cell names it; `subtotal` on another op) | 7 (refusals), 8 (figures) |
| Go fuzz rule parity covers `percentOfSection` and `pivot.range` | 9 |
| Go last task: fictional P&L-by-month statement on `zoo_*` | 9 |
| Vitest `spec.ts` (each new field parses; old spec still parses) | 4 |
| Vitest `draft.ts` round-trip (`percentOfSection` + `subtotal`, `range`, `order`; read-only part stays) | 11 (node), 12 (pivot) |
| Vitest `format.ts` bucket words in `en-US` | 2 |
| Vitest `table-lines.ts` `expandPivots` with `order: "spec"` | 4 |
| Builder vitest: "Percent of a section's total" writes `subtotal`, offered only on a layout report | 10 |
| Builder vitest: "Show every <bucket> of <date range param>" writes `range`, offered only on a bucketed date pivot with a `dateRange` param | 12 |
| Builder vitest: "Total after the pivot columns" writes `order: "spec"` | 12 |
| Docs: help sections, pivot section names "Total after the pivot columns", words in pivot / group-by-date text; packages.md table; docs check | 14 |
| E2E `reports: period pivot` desktop (percent; three months in words, middle 0.00, total after) and phone (fit, no sideways scroll, words) | 13 |
| Spec §4 table P1 columns: engine, schema (`Node.subtotal` on the op; operator in the formula table), client (`spec.ts` operator; `format.ts` percent as today), builder, help + packages.md | 7–8, 7 + 14 (packages.md operator row), 4 (spec.ts doc), 10–11, 14 |
| Spec §4 table P2 columns | 5–6, 5, 4, 12, 14 |
| Spec §4 table P3 columns (incl. `column-measure.ts`) | 3, 3, 4 (R10), 12, 14 |
| Spec §4 table P4 columns (engine `assemble.go` `display`/`pivotLabel`, `run.go`; client types/format/dates; help) | 1, 2, 14 |
| Do step 1 build order P4 → P3 → P2 → P1 → builder and help | task order 1–14 |
| Do steps 2–5 (human review, SDD, PR, merge, gate 2 Books fixture) | driver / runner |
