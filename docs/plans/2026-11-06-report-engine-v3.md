# Report Engine v3 (Checkpoint 0 of p5-general-ledger) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core report-engine, report-view and report-builder extensions F1–F11 that the approved General Ledger design needs: `sum`/`count` lookups, an outer-row leaf inside a lookup, date-range bounds, the All dates preset, opening rows, rollup formulas, paged grouped reports, a viewer sort, spec format version 3 with a full builder round-trip, tables that fit and never clip at every width, and a list that starts at the top when the sort changes.

**Architecture:** The Go engine (`core/server/reports`, module `tinycld.org/core`) gains optional spec fields (spec version 3, the 2→3 step is the identity): `Node.Pick` `sum`/`count`, `Node.Outer`, `Node.Bound`, `Group.Opening`, `Column.Rollup`, a run option `Options.Sort`, the range preset `allDates`, a row kind `opening`, and keyset pages for grouped reports (the group pass stays whole and capped; the detail pass pages in document order). The TypeScript side (`core/reports`) reads v3 strictly, round-trips every v3 field through the builder draft, edits the new formula parts and level parts in the builder, draws the opening row, a heading sort control and grouped pages, and replaces the desktop sideways scroller with one fit rule for every width (narrow the text columns, then wrap). Core e2e drives the `salesstub` stub package, which ships one more report that uses the v3 parts.

**Tech Stack:** Go (PocketBase fork, `tinycld.org/core/reports`, SQLite), TypeScript, React Native + react-native-web (Expo Router), Uniwind classes, react-hook-form + zod v4, TanStack Query 5 (`useInfiniteQuery`), FlashList v2 (nathanstitt fork), lucide-react-native, Playwright, vitest, Biome.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/general-ledger-design.md` §4 (binding: every column of every row F1–F11) and §5 "The General Ledger spec" (the shape the engine must run), with the checkpoint brief `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/cp/cp0.md` (its "Spec", "Tests it must add" and "Interfaces" blocks are requirements). Prior core plans in this repo: `docs/plans/2026-11-01-report-engine-v2.md` (the model; its rulings R1–R31 stand unless a ruling below changes one), `docs/plans/2026-10-30-report-engine.md`, `docs/plans/2026-10-31-report-builder.md`.

---

## For review

This section is for the human before anything runs. Core UI has no design gate, so the view and builder per width are written out here.

### Verdict: MAJOR — lane `purchases` holds `exclusive` until the core PR merges

- **F9** changes the spec contract every report unit writes: `reports.CurrentVersion` becomes 3, and a v3 spec sent to an older server is refused by its version (`core/server/reports/spec.go:356-358`).
- **F5** adds a row kind (`opening`) that every client of `/api/reports/run` must draw; `RowKind` is a closed union in `core/reports/lib/types.ts:21` and `ReportRow.tsx:14-28` keys styles by it.
- **F7** changes the run contract: a grouped run now pages (`page.next` set, `options.cursor` accepted), where today it is refused (`run.go:309-311`, `"a grouped report is not paged"`), and its 5,000-detail cap goes.
- **F8** adds a run option (`options.sort`) and a result-column flag (`sortable`).
- **F10** changes how every report table fits at desktop width and on phones: no sideways scroll, no frozen label (`ReportTable.tsx:169-214`, `ReportRow.tsx:264`); text columns narrow before a row wraps, so a phone report with few columns can draw one line where v2 drew two (R21, Q3).
- No core migration, no generator change, no removed or renamed export other than `FrozenLabel` (internal to `core/reports/components`, imported only by `ReportTable.tsx:23`) and `ReportTableLayout.scrollsSideways` (`report-table-layout.ts:17`).
- One PR on core branch `books/p5-general-ledger`, merged before Books checkpoint 1. Books' Trial Balance test changes one assertion after the merge (brief P3); nothing else in Books breaks, because every v3 field is optional.

### The spec format: version 3

`CurrentVersion` becomes 3. Every new field is optional. The 2→3 upgrade step is the identity, so every stored v1 or v2 spec loads unchanged and with no "upgraded" notice (v2 R1: `sameExceptVersion`, `spec.go:332`). The client schema accepts versions 1–3 and writes 3.

A v3 spec that uses every new part (fictional `zoo.*` sources):

```jsonc
{
  "version": 3,
  "source": "zoo.feedings",
  "params": [ { "id": "period", "type": "dateRange", "label": "Period", "default": { "preset": "allDates" } } ],  // F4
  "columns": [
    { "id": "animal", "label": "Animal", "path": [{ "link": "animal" }] },
    { "id": "fed", "label": "Fed", "path": [{ "field": "fed" }] },
    { "id": "cost", "label": "Cost", "path": [{ "field": "cost" }], "aggregate": "sum" },
    // F1 count + F2 outer: feedings of this row's animal on this row's day.
    { "id": "sameDay", "label": "Same day", "formula": { "op": "lookup", "source": "zoo.feedings",
        "where": { "op": "and", "args": [
          { "op": "eq", "args": [{ "path": [{ "link": "animal" }] }, { "outer": [{ "link": "animal" }] }] },
          { "op": "eq", "args": [{ "path": [{ "field": "fed" }] }, { "outer": [{ "field": "fed" }] }] } ] },
        "value": { "path": [{ "field": "id" }] }, "pick": "count" } },
    // F1 sum + F3 bound: the animal's cost before the period, then a running sum; F6 its rollup.
    { "id": "balance", "label": "Balance",
      "formula": { "op": "add", "args": [
        { "op": "coalesce", "args": [ { "op": "lookup", "source": "zoo.feedings",
            "where": { "op": "and", "args": [
              { "op": "eq", "args": [{ "path": [{ "link": "animal" }] }, { "record": "byAnimal", "path": [{ "field": "id" }] }] },
              { "op": "lt", "args": [{ "path": [{ "field": "fed" }] }, { "param": "period", "bound": "from" }] } ] },
            "value": { "path": [{ "field": "cost" }] }, "pick": "sum" }, { "value": 0 } ] },
        { "op": "runningSum", "args": [{ "path": [{ "field": "cost" }] }] } ] },
      "rollup": { "op": "add", "args": [ { "op": "sum", "args": [{ "path": [{ "field": "cost" }] }] }, { "value": 0 } ] } }
  ],
  "filter": { "op": "between", "args": [{ "path": [{ "field": "fed" }] }, { "param": "period" }] },
  "groups": [ { "id": "byAnimal", "col": "animal", "open": true, "subtotals": true,
    // F5: the first row under each open group.
    "opening": { "label": "Before the period", "cells": { "balance": { "op": "coalesce", "args": [
      { "op": "lookup", "source": "zoo.feedings", "where": { "op": "and", "args": [
          { "op": "eq", "args": [{ "path": [{ "link": "animal" }] }, { "record": "byAnimal", "path": [{ "field": "id" }] }] },
          { "op": "lt", "args": [{ "path": [{ "field": "fed" }] }, { "param": "period", "bound": "from" }] } ] },
        "value": { "path": [{ "field": "cost" }] }, "pick": "sum" }, { "value": 0 } ] } } } } ],
  "sort": [ { "col": "fed" } ]
}
```

A run body gains `options.sort` (F8): `{ "options": { "sort": [ { "col": "cost", "dir": "desc" } ], "cursor": "…", "limit": 200 } }`.

**New fields, by part:**

| Field (JSON) | Go type | TS zod (`core/reports/lib/spec.ts`, `run-client.ts`) | Where Validate checks it | Refusal messages (`at` path) |
|---|---|---|---|---|
| `Node.pick` adds `"sum"`, `"count"` | `Pick string` (exists, `spec.go:150`) | `pick: z.enum(['min','max','sum','count']).optional()` | `lookupIn` (`lookup.go:102`), reached by `bind` | `.pick`: "pick is min, max, sum or count"; `.value`: "pick sum adds numbers or amounts" |
| `Node.outer` | `Outer Path \`json:"outer,omitempty"\`` | `outer: z.array(specHopSchema).optional()` | `compileNode` (`formula.go:230`) via `fctx.outerPath` | at: "outer reads the report's row: use it inside a lookup"; `.outer`: "outer reads one value of the report's row: a to-many link is not one"; with another kind: `errNodeKind` (text gains "outer") |
| `Node.bound` | `Bound string \`json:"bound,omitempty"\`` | `bound: z.enum(['from','to']).optional()` | `compileNode` param case | at: "bound is from or to"; "bound goes with a param"; "bound reads a date range's start or end" |
| range preset `allDates` | `rangePresets` entry (`params.go:38-42`) | `RANGE_PRESETS` entry (`lib/params.ts:11-23`) | `rangeValue` (`params.go:366`) | existing "the preset is one of …" |
| `Group.opening` | `Opening *Opening \`json:"opening,omitempty"\``; `type Opening struct { Label string \`json:"label"\`; Cells map[string]*Node \`json:"cells,omitempty"\` }` | `opening: z.strictObject({ label: z.string().min(1).max(200), cells: z.record(z.string(), specNodeSchema).optional() }).optional()` | `bind.groupsAndPivot` (`bind.go:713`) + compile on the level | `groups.N.opening.label`: "an opening row has a label of 1 to 200 characters"; `…cells.<col>` (422 `unknown_column`): "the report has no column %q"; `…cells.<first col>`: "the opening row's label fills the first column"; `…cells.<col>`: "an opening cell shows in its column: it must be %s"; "an opening cell's amount is in another currency than its column's"; `groups.N.opening` on a pivot's last level: "this level never opens, so it has no opening row" |
| `Column.rollup` | `Rollup *Node \`json:"rollup,omitempty"\`` | `rollup: specNodeSchema.optional()` | `bind.columns` (`bind.go:274`) + `levelCtx.col` | `columns.N.rollup`: "a rollup replaces the column's totals: it does not go with total \"rows\""; "a child row column has no totals to replace"; "a pivot's cell column is totalled by its pivot columns"; "a rollup shows in its column: it must be %s"; "a rollup's amount is in another currency than its column's" |
| `options.sort` | `Options.Sort []ViewSort \`json:"sort,omitempty"\``; `type ViewSort struct { Col string \`json:"col"\`; Dir string \`json:"dir,omitempty"\` }` | `RunOptions.sort?: ViewSort[]`; `type ViewSort = { col: string; dir: 'asc' \| 'desc' }` | `viewSortPlans` (new, `run.go`) at run time (it is a run input, not a spec part) | `options.sort`: "a run sorts by at most 10 columns"; `options.sort.N.col` (422 `unknown_column`): "the report has no column %q"; `…col`: "a column sorts once"; `…dir`: "dir is asc or desc"; `…col`: "a running value or a percent is not a sort key" (existing `notASortKey`); "a child row column is not a sort key"; `options.sort`: "a pivot report has no opened rows to sort" |
| `ResultColumn.sortable` | `Sortable bool \`json:"sortable,omitempty"\`` | `sortable?: boolean` (`lib/types.ts` `ResultColumn`) | — (result) | — |
| row kind `opening` | `KindOpening = "opening"` (`run.go:108-114`) | `RowKind` adds `'opening'` | — | — |
| `version` 3 | `CurrentVersion = 3`, `upgradeSteps[2]` identity | `CURRENT_SPEC_VERSION = 3` | `upgrade` (`spec.go:338`) | existing "version 4 is newer than this server reads (3)" |

Every new formula slot (`opening.cells`, `rollup`, `outer` paths) joins the walkers that every formula slot is in today: alias resolution (`resolveAliases`, `alias.go`), lookup decoding (`binder.lookups`, `lookup.go:46-84`), formula size limits (`binder.formulaSizes`, `bind.go:399`), and `keptParams` on the client (`draft.ts:797`).

### Engine behaviour, per extension

- **F1 `sum` / `count` lookups** (`lookup.go` `lookupIn`, `pickedAmount`): `count` is the number of matching rows whose `value` is not blank, 0 when none match (type Number, no currency). `sum` adds `value` over the matching rows (Number or Money only), NULL when none match. A money `sum` carries the matched rows' currency: one currency → that currency; more than one → no value (NULL) and the `mixedCurrency` notice naming the column (R3). `min`/`max` are unchanged.
- **F2 `outer`** (`formula.go` dispatch, `lookup.go` `lookupCtx`, `levels.go` `lookupReadsRowValue`): `{ "outer": <path> }` compiles, inside a lookup's `where` or `value`, as the report row's value of that to-one path from the base source — what a `{path}` means in the context the outermost lookup sits in (a detail row's value, a joined row in the filter, blank on a group/subtotal/total row as a bare path is). In a lookup inside a lookup it still reads the report's row (R4). It makes the lookup correlated. Refused outside a lookup and on a to-many path.
- **F3 `bound`** (`formula.go`, `params.go` `paramExpr`): `{ "param": id, "bound": "from" | "to" }` is a `Date` value, the range's first or last day. On an unset range or All dates it is a blank date (NULL) — never an "unset" condition, so `lt([date], from)` matches no row (R2; this gives the General Ledger's 0.00 beginning balance under All dates).
- **F4 `allDates`** (`params.go` `rangePresets`, `rangeValue`, `rangeDays`, `paramExpr`; `between`): resolves with no bounds (`from` = `to` = ""). `between(x, {param})` on it is true for every row, a blank day included, in a filter, a having, a formula and a lookup (R1). The run's resolved param has no `from`/`to`, and the header prints "All dates".
- **F5 opening rows** (`levels.go`, `run.go` `groupStatement`, `assemble.go` `groupWithLabel`): for each group level with `opening`, each `opening.cells` formula compiles on that level (a `levelCtx`: aggregates over the group's base rows, `record` leaves of this and enclosing groups, lookups, params) as extra cells `op<idx>` of `o<L>`. When a group row of that level is expanded, the row right after it is `{ id: "opening:" + keyString(groupKey), kind: "opening", depth: n, label: opening.label, section: <group row id>, groupKey, cells }` with only the cells `opening.cells` names (absent cells blank; a money cell's currency in `<col>@currency` as on group rows). A level with `opening` lets every group of that level open even with no base rows (a fill group included), so its opening row and subtotal show (R5). A closed group and a group its having hides show none.
- **F6 `rollup`** (`levels.go` `levelCtx.col`): a column with `rollup` shows, on group, subtotal and total rows (and layout sections), its rollup formula compiled on that level instead of `recompute`/`rowsTotal`; detail rows keep `path`/`formula`. A rollup that reads a row value the level has none of — a `record` leaf of a group the row is not inside (the Total), a bare path — is blank as a whole (`readsRowValue`, `levels.go:874`), so the Total's balance is blank (R6).
- **F7 paged grouped reports** (`run.go` `run`, `detailPass`; `paging.go`; `assemble.go`): the group pass runs whole (capped at 5,000 group rows, its `rowCap` notice unchanged). The detail pass of a grouped report pages: it reads the expanded innermost groups' detail rows in document order — the group order the group pass gave (bound as a JSON list, the `expansion.where` pattern, `run.go:970-1030`), then the detail order (F8 viewer keys, the spec's keys, `__id`) — up to `options.limit` (default 200, held at 1,000) rows after the cursor. Window values are computed before the keyset (`SELECT * FROM (<select with windows>) d WHERE <keyset>`, as today, `run.go:707`), so running values continue across pages. A page holds its detail rows plus the group, opening and subtotal rows that fall between them in document order; every page ends with the Total (and layout computed) rows, as ungrouped pages do; `page.next` carries the group path and the detail keyset (R9). The grouped 5,000-detail cap goes (`run.go:693`). The cursor's fingerprint covers the group levels, the viewer sort and the detail keys, so a cursor is refused after a spec change or a sort change.
- **F8 viewer sort** (`run.go` `detailSelect`, `Options.Sort`; `paging.go` fingerprint; `assemble.go` `columns`): `options.sort` keys order detail rows first, inside their groups; group order is unchanged; the window frame keeps the spec's order (`frame.order`, `run.go:770-807`), so running values keep the spec's order. Sorting by the first (label) column sorts by the row's label — `detail.label` when the spec has one (R7). `ResultColumn.sortable` is true for a column the run would accept as a viewer sort key.
- **F9 version 3**: `CurrentVersion = 3`, identity step, strict decode of every new field, aliases and lookups resolved in every new slot, `Validate` for every new field (table above). A Go test stores a spec with every v3 field and loads it back unchanged.
- **F10** (client only): see "Report table layout per width".
- **F11** (client only): `rowsKeyOf` includes `options.sort` (R11).

### Client lib changes

- `lib/spec.ts`: v3 zod (table above); `CURRENT_SPEC_VERSION = 3`.
- `lib/types.ts`: `RowKind` adds `'opening'`; `ResultColumn.sortable?: boolean`.
- `lib/params.ts` (not `lib/dates.ts`, R18): `RANGE_PRESETS` adds `'allDates'` after `'last12Months'`; `PRESET_LABELS.allDates = 'All dates'`; `periodWords` returns `'All dates'` for a resolved date range that is set and has no `from`/`to`; `emptyMessage` reads "No rows for all dates" there.
- `lib/view-url.ts`: `ViewOptions.sort: ViewSort | null`; key `s=<col>:<dir>` (`dir` `asc` | `desc`; a value of another shape is ignored, as a bad `p-` value is); `serializeViewOptions` leaves `s` out when there is no sort; `reportLink` keeps it.
- `lib/run-client.ts`: `RunOptions.sort?: ViewSort[]`; `rowsKeyOf` appends `|sort:<JSON>` when `options.sort` is non-empty and is unchanged otherwise (R11).
- `lib/rows.ts`: `mergePages` merges grouped pages: every row of each page in order, an earlier page's total and computed rows dropped, a row whose id an earlier page already gave dropped (R10).
- `lib/table-lines.ts`: `tableLines({ columns, width, breakpoint, layouts })` returns `{ kind: 'one-line', widths }` or `{ kind: 'blocks', root, notice? }`; new `fitOneLine(columns, width, breakpoint)`; `rowLayout(item, layout, labelId)` drops blank lines on a figure-only row (R14).
- `lib/builder/formula-parts.ts`, `formula-ops.ts`, `formula-type.ts`, `formula-describe.ts`, `formula-context.ts`: parts `outer` and `bound`, pick choices, the report row's source in the context (`rowSource`).
- `lib/builder/draft.ts`: column `rollup: FormulaDraft | null`; group level `opening: { label, cells: Record<colId, FormulaDraft> } | null`; `specFromDraft` writes version 3.
- `components/report-table-layout.ts`: per-column widths (`TEXT_FLOOR`), `scrollsSideways` and `fitsOnOneLine` go.

### View and builder UI, per width

Widths are core's breakpoints (`core/components/workspace/useBreakpoint.ts:5-9`): `mobile` < 768, `tablet` 768–1023, `desktop` ≥ 1024.

- **Opening row** (all widths): drawn like a detail row at its depth (no fold toggle, no rule above), its label in `text-muted-foreground`, its figures in the column tracks as on any row. Test id `report-row-opening:<group key JSON>` (`labelId`, `ReportRow.tsx:143-144`). Not a sticky boundary (`lib/sticky.ts`).
- **Heading sort control** (all widths, one-line and wrapped headings): a column whose result column is `sortable` gets its heading wrapped in a React Native `Pressable` (R15) with `accessibilityRole="button"`, `accessibilityLabel="Sort by <column label>"`, test id `report-sort-<col>`, and a lucide icon after the heading text in `useThemeColor('muted-foreground')`: `ArrowUpDown` (no sort), `ArrowUp` (asc), `ArrowDown` (desc). A press cycles asc → desc → the report's order and writes `s=` (router `setParams`), which starts the list at the top (F11). A running-value column's heading is plain text. Same control on tablet and phone; on phone the press target is the heading cell (min height 44).
- **Sort refused** (R8, R22): a run refused with a problem under `options.sort` shows the message and a "Clear sort" button in place of "Edit report"; pressing it removes `s=` and the report runs in its own order.
- **Period chip** (`ParamPeriodControl.tsx`): the preset pills follow `presetsFor` (`ParamPeriodControl.tsx:118`), so "All dates" appears after "Last 12 months" with no component change other than its test; chip text "Period: All dates"; document header "All dates".
- **Grouped paging**: the list's `onEndReached` already calls `loadMore` (`ReportTable.tsx:262`); grouped pages now carry `page.next`, so the same call appends sections. The loading row shows under the list as today.
- **Builder, formula editor** (all widths; menus are popovers on desktop/tablet and sheets on phone, core `PickMenu`): a lookup's pick menu lists Smallest, Largest, Add up, Count (test ids `<id>-pick` items `min|max|sum|count`). Inside a lookup the part menu adds "This row's field" (part `outer`), whose field picker lists the report source's fields and to-one paths, shown as "This row's <field>". Where a date-range param exists, the part menu adds "Period start" / "Period end" style parts: one part `bound` whose picker lists "<param label> start" and "<param label> end" for each date-range param.
- **Builder, Group and sort** (each level card, stacked vertically at every width): a switch "Start each group with a row" (test id `report-builder-group-N-opening`); when on, a "Row label" text input (zod: 1–200 characters, "Enter a label") and one row per column after the first: "<column label>: No value" or a formula (the report formula editor), each with "Add a value" / "Remove".
- **Builder, Columns** (each column card): "On group and total rows show" choice — "The column's total" (default) | "A formula" (test id `report-builder-column-N-rollup`); "A formula" opens the report formula editor (totals, group fields, lookups, asked values offered). Hidden while Total is "Add the rows shown" (they cannot go together).
- **Builder, Arrange**: the Desktop tab already edits like Phone and Tablet (`ArrangeSection.tsx:28-36`); only its caption changes (no "one line" wording) and a test pins it.
- **Builder round-trip**: a spec part the builder cannot show (an unknown node shape) stays `fixed` and read-only and is saved unchanged (v2 R17, spec "R15").

### Report table layout per width (F10)

One rule for every width; desktop no longer scrolls sideways, and the label column is no longer frozen.

1. **Authored layout for this breakpoint** (`display.layouts` entry whose `width` is the current breakpoint): used as today (v2 R24) — see Q1.
2. **One line, narrowed if needed** (`fitOneLine`): the label column starts at its minimum (220 desktop/tablet, 168 phone); figure columns (number, money, percent, date, datetime, bool) keep their width (128 desktop/tablet, 108 phone); text columns (text, select, relation) start at the same width. If the sum exceeds the table's width, first the label narrows toward `TEXT_FLOOR`, then the text columns narrow from the widest down (water-fill: one cap lowered until the sum fits), each to `TEXT_FLOOR` = 8 average characters (8 × 7 pt) + the cell's 24 pt padding = 80 pt. A narrowed cell ends in "…" (`numberOfLines={1}`). If it fits, the row is one line with those widths; the label column is still `flex: 1` and takes any width left.
3. **Nearest narrower authored layout**: desktop → tablet → mobile (from the brief: at 1024 the breakpoint is desktop, `useBreakpoint.ts:6`, and `tableLines` picks by breakpoint only, `table-lines.ts:294`).
4. **`autoLines`** (`table-lines.ts:203`): the label across line 1, the other columns in tracks of ≥ 104 below.

A column is never cut at the view's edge; the table never scrolls sideways (the `Sideways` scroller and `FrozenLabel` are deleted). In a wrapped layout, a row that is not a detail row (group, opening, subtotal, total, computed) drops each line whose cells are all blank on that row, so a phone "Beginning balance" row is its label line and then its figures line (R14). Example, the General Ledger (label, 5 text columns — Type, No., Name, Description, Split — and 3 money columns): its natural line is 220 + 8 × 128 = 1244 pt, so at 1280 it fits or fits after the label narrows; fully narrowed it is 80 + 5 × 80 + 3 × 128 = 864 pt, so a desktop table of 864 pt or more draws one narrowed line; at tablet width (768–1023) its authored tablet layout applies (step 1); at 390 its authored phone layout.

### Help and docs changes

- `core/help/reports.md`: "Look up a value" gains Add up and Count and "This row's <field>"; "Run a report" gains the Period's All dates and "<param> start / end" in formulas; new section "Opening rows"; "Totals" gains "On group and total rows show a formula"; new section "Sort a report" (heading press cycle, the link keeps it, running values keep the report's order); "Narrow screens" becomes "When columns do not fit" (narrow text columns, then wrap; Arrange per width including Desktop; no sideways scroll); "Formulas" lists the new parts.
- `docs/packages.md` (`### Report sources`, line 1111; "What a report returns", line 1267; "Spec version 2", line 1290): spec version 3 and its fields in the spec table; `options.sort` and `ResultColumn.sortable`; the `opening` row kind; grouped results page (`page.next`, `options.limit`, the group cap stays, running values continue).

### Rulings

- **R1. All dates.** `allDates` resolves to a set date range with `from` = `to` = "". `paramExpr` gives `expr{rng: true, all: true}`, and `between` on it is `1` in every context, so every row matches, a blank day included (the same rows an unset range matches in a filter, `formula.go:419-443`, but also in a formula, where an unset range is not a match). `ResolvedParam` keeps `Unset: false` and omits `from`/`to` (`omitempty`, `params.go:187-188`); the client reads that shape as All dates. Reason: spec F4 "a `between` on it matches every row"; `rangeValue` today requires two days (`params.go:377`). **Cost if wrong:** a ledger under All dates drops lines with a blank date (they would then never show anywhere); one branch in `between`.
- **R2. A bound of an open range is a blank Date, not an unset condition.** Reason: the GL opening lookup `lt([date], {param period, bound from})` must match no line under All dates so the beginning balance is 0.00 (spec §3 "With All dates the beginning balance is 0.00"); an unset condition would match every row (`formula.go:442`). **Cost if wrong:** every beginning balance under All dates is the whole history — the Books spec's own pinned test fails.
- **R3. `sum`/`count` details.** `count` is `count(<value>)` (rows with a non-blank value), 0 on no match; `sum` is `sum(<value>)`, NULL on no match, refused on a value that is not a number or an amount; a money `sum` reads `(SELECT count(DISTINCT <cur>) …)` and is NULL when it is above 1, its currency `(SELECT min(<cur>) …)`; on a base row inside a total the currency is the lookup's registered key, as `pickedAmount` does (`lookup.go:180`). The notice: when a column cell (detail, group, opening, rollup) reads a sum lookup that met two currencies on any row of the result, the run adds the notice code `mixedCurrency` with message "A looked-up total mixes currencies, so it shows no value." and `Columns` naming those columns; if the totals' `mixedCurrency` notice (`assemble.go:116`) is also due, its `Columns` are merged into one notice and the first message is kept. The check is a helper cell `vmx<idx>` (1 where the lookup met two currencies), left out by `unreadHelpers` when no leg has one (`run.go:584-609`). The same notice, through the same `vmx<idx>` cell (`expr.mixed`), names a column whose amount R24's run-time guard blanked: two reads of one currency field that hold different codes on a row (Task 2b). Lookups in a filter, having, sort or label give no notice. **Cost if wrong:** a mixed sum shows a wrong figure or a misleading notice; local to `lookup.go` and the cell helpers.
- **R4. `outer` compiles through the context the lookup sits in.** `fctx` gains `outerPath(p Path, at string) (expr, error)` (not `outer`: `lookupCtx` already has a field `outer`); `ctxBase.outerPath` refuses ("outer reads the report's row: use it inside a lookup"); `lookupCtx.outerPath` binds the path on the report's base source (`bindValuePath(app, c.p.base, p, at, pathValue)`), refuses a to-many hop, and returns `outerContext.path(p)` where `outerContext` is the first context outside every enclosing lookup. Aliases in an outer path resolve against the base source (`resolveAliases`). `lookupReadsRowValue` (`levels.go:927`) treats an outer leaf as a row value. Reason: lookupCtx already routes `col`/`record`/`param` to `outer` (`lookup.go:229-256`); `levelCtx.path` is NULL (`levels.go:977-983`), which gives "blank on group rows". **Cost if wrong:** a nested lookup's outer leaf reads the wrong source (a 422 or a wrong Split); one function.
- **R5. Opening rows.** (a) Cells compile on the opening's own level (`levelCtx`), helper cells `op<idx>`; (b) a cell on the first column is refused (the label fills it); (c) a cell's type must equal its column's detail type (a `{ "value": null }` cell is allowed), and a money cell must combine with the column's currency (`combined`, `formula.go:95`), else refused (messages in the table) — the row is drawn with the column's format; (d) a level with `opening` overrides the "no base rows cannot open" rule (`assemble.go:302-304`) and the "all children hidden" rule (`assemble.go:306-308`) for that level; (e) opening on a pivot's innermost level is refused (it never opens, `assemble.go:299`); layout sections may have one. Reason: spec F5 "also under a group with no detail rows, a fill group included" — today such a group cannot open, so no row could show; the GL shows "Beginning balance" on accounts with no line in the period (spec §3). **Cost if wrong:** a GL account with only a beginning balance shows a closed row with no balance; or a refused spec where a looser type rule was wanted (one check).
- **R6. Rollup.** Compiled in `levelCtx.col` before the `total` switch (`levels.go:634-643`); blank as a whole when `readsRowValue(rollup)` is true on that level; refused with `total: "rows"`, on a child-row column (`cp.nest > 0`), on the pivot's cell column; type and currency checked as R5(c). Subtotal rows carry the group row's cells (`assemble.go:325-329`), so they show the rollup too. **Cost if wrong:** the Total's balance shows the sum of nets instead of blank (spec F6 "a record leaf on a row with no record (the Total) makes the cell blank").
- **R7. Viewer sort.** Applies to detail rows of any report with detail rows (grouped or not); refused on a pivot report (no detail rows). Keys go before the spec's keys in the statement's `ORDER BY` and the cursor, never into `frame.order`. A key on the first column sorts by the detail label formula when the spec has `detail.label`, else by the first column's value; on a relation column it sorts by the record's label then id, as spec sort keys do (`c.sortLabel`, `run.go:791-796`). `ResultColumn.sortable` = not a running value or percent, not a child-row column, not a pivot report, and for the first column the same test on `detail.label`. At most 10 keys (the spec's sort-key limit); the client writes one. Reason: spec §3/§5 make "Date" — the detail label (`detail: { label: [date] }`), not a column — a sort key under the "Account / Date" heading. **Cost if wrong:** pressing "Account / Date" in the GL does nothing visible (sorts by account inside an account); one branch.
- **R8. A refused sort offers "Clear sort".** A run refused (`kind: 'invalid' | 'missing'`) with a problem whose path starts `options.sort` shows the refusal state with the problem's message and a "Clear sort" button (test id `report-clear-sort`) in place of "Edit report" (R22); pressing it removes `s=` (`router.setParams`), which runs the report in its own order. Reason: a copied link outlives column edits; "Edit report" would not remove the sort from the link; an automatic clear would need an effect that writes the URL (banned pattern). **Cost if wrong:** one `ProblemsBody` action set in `ReportView.tsx` and one derived value (`sortProblem`) in `useReportView`.
- **R9. Grouped pages.** (a) Group order: from the group pass, the open innermost groups' keys in assembled order, bound as one JSON list `[[ordinal, k0…kN, currencies…], …]` and joined with the expansion key pattern (`run.go:970-1030`); (b) the detail statement orders by `(ordinal, viewer keys, spec keys, __id)`; (c) cursor JSON `{ k: fingerprint, g: <group key>, v: [...] }`; the next page finds `g` in the fresh group order; a group that is gone refuses the cursor with the existing "the cursor is not one this report gave" (the client's Retry refetches from page 1, `useReportRun.ts:82`); (d) the fingerprint hashes the group levels (`col`, `bucket`), the viewer keys and the detail keys; (e) a page's rows are the document slice after the previous page's last detail row up to and including this page's last detail row, then the Total/computed rows; the last page runs to the end of the document; no row is repeated across pages; (f) the group pass still runs once per page (its totals are whole-report figures). **Cost if wrong:** a seam duplicates or skips a section heading — caught by the Task 8 tests and the e2e.
- **R10. `mergePages`** keeps every row of every page in order, drops each earlier page's `total` and `computed` rows, and drops a row whose id it already holds. The ungrouped behaviour is the special case (earlier pages hold only detail rows plus totals). **Cost if wrong:** a duplicated opening or subtotal row at a seam.
- **R11. `rowsKeyOf`** stays byte-identical without a sort (so no existing list resets), and appends `|sort:<JSON of options.sort>` with one. **Cost if wrong:** a sort change keeps the scroll position in the middle of a reordered list (Q10 of the spec).
- **R12. F10 widths and floor.** Natural widths are today's (`report-table-layout.ts:6-9`: label min 220/168, other columns 128/108), so a table that fits today is unchanged; `TEXT_FLOOR` = 80 (8 × 7 pt average character at `text-sm` + `px-3` both sides); "figure" = `number`, `money`, `percent`, `date`, `datetime`, `bool` (aligned or fixed-length values an ellipsis would ruin); the label narrows first, then text columns by water-fill from the widest. Content is not measured (a measured width would change the layout as pages load). Consequence for the GL (spec §5 says "F10 picks [the tablet layout] when the one line does not fit" at 1024): narrowed, its line is 864 pt, so at a 1024 window it draws one narrowed line, not the tablet layout; the tablet layout shows at tablet width and on a desktop table under 864 pt (a builder drawer open). This contradicts spec §5 and the phase-3 reference (`populated-1024` wraps to the tablet layout), so it is open question Q2; the plan builds the recommended answer. **Cost if wrong:** text columns ellipsize earlier or later than a designer wants; constants only.
- **R13. Breakpoint order of layouts.** At each breakpoint: (1) its own authored layout, (2) the narrowed one line, (3) the nearest narrower authored layout, (4) `autoLines`. Tablet and phone follow the same order (today they skip step 3). **Cost if wrong:** see Q1.
- **R14. Figure-only rows in wrapped layouts.** `rowLayout` prunes, for a row whose kind is not `detail`, every cell block whose column has no value on the row (not the label), and then every stack line and side block left with no cell; detail rows keep every line so a field sits at the same place in each row. Today's rule (a group its subtotal follows draws only its label) stays. **Cost if wrong:** a phone opening row is 4 lines tall (the carried phase-3 finding).
- **R15. Sort control parts.** React Native `Pressable` with `accessibilityRole="button"`, as the group fold toggle already is (`ReportRow.tsx:146-163`; core has no `ui/pressable`), lucide `ArrowUpDown` / `ArrowUp` / `ArrowDown` at size 12, colour from `useThemeColor('muted-foreground')`. The heading text keeps `numberOfLines={1}`; the icon is outside the text so the ellipsis never eats it. **Cost if wrong:** if another press target, icon or colour is wanted, one heading component in `ReportTable.tsx` and the e2e's `role="button"` / "Sort by <column>" assertions change; no run or URL contract changes.
- **R16. Builder wording.** Pick menu: "Smallest", "Largest", "Add up", "Count"; part labels "This row's field" (menu) / "This row's <field label>" (shown); bound picker "<param label> start" / "<param label> end"; Group and sort "Start each group with a row", "Row label"; Columns "On group and total rows show", "The column's total", "A formula". `formula-describe.ts` says "add up …" / "count …", "this row's <field>", "<param> start/end". **Cost if wrong:** wording churn only: the strings in `formula-parts.ts`, `formula-describe.ts` and the builder parts, the help topic, and the builder vitest and e2e text assertions; no behaviour changes.
- **R17. E2E fixture.** The `salesstub` stub ships a third report "Stub team balance" (`salesstub.team_balance`, v3, revision 1, `Unpublished: true`): base `salesstub.sales`; params `period` (dateRange, default `thisFiscalYear`) and `teams` (choice from `salesstub.teams`, multiple); a record group on `team` (`open`, `subtotals`, `opening` "Before the period" with a `balance` cell: a `sum` lookup over the team's sales before `{param period, bound from}`); nine columns: `customer` (first), `team` (`detail: "blank"`), `day`, `status`, `amount` (sum), `cost` (sum), `quantity` (sum), `sameDay` (a `count` lookup with `outer` day and team), `balance` (opening + `runningSum(amount)`, `rollup` opening + `coalesce(sum(amount), 0)`); sort `day`, `customer`; a `display.layouts` tablet entry that leaves `team` out. `team` is there because the group's `col` must name a column (the group row names the team; detail rows leave the cell blank). `cost` and `quantity` are there to make the narrowed line 80 + 2 × 80 + 6 × 128 = 1008, wider than the 944 a 1024 window gives (Q2), so the 1024 e2e shows the wrap to the tablet arrangement, and that arrangement has a second line of figures. Grouped paging in e2e shrinks the page by rewriting `options.limit` to 5 on `POST /api/reports/run` with `page.route` (the precedent is `reports-source-params.spec.ts:363-378`), so 12 seeded sales page across a group seam. Reason: building these formulas through the builder in every e2e would be slow; the builder controls get one e2e of their own (Task 13a "the builder counts a lookup on this row, …") and builder vitest. **Cost if wrong:** if the stub's data does not reach a seam (12 sales on fewer than two pages, or no group boundary inside a page), the paged and opening-row e2e pass without testing F7 at a seam; the fix is the stub spec or the seeded sales in Task 13a's files only. A shipped report that `Validate` refuses is left out at boot, so a stub defect shows as a missing hub row, not a silent pass.
- **R18. Brief file list vs the code.** (a) Detail order is in `run.go` `detailSelect` (`run.go:765-807`), not `detail.go` (which holds `rowsCtx` and `windowFrame`); `detail.go` changes only if a viewer key needs a `rowsCtx` change. (b) Totals are level 0 in `levels.go`; `rows.go` is the rows CTE and changes only for the outer leaf's currency, if at all. (c) The range presets and `periodWords` are in `lib/params.ts:11-48,390-399`, not `lib/dates.ts`; `dates.ts` does not change. (d) `paramLeaf` is in `options.go:171`; F3 lands in `formula.go` and `params.go`. (e) `tests/e2e/reports-view.spec.ts:111` is the title-ellipsis helper (`element.scrollWidth > element.clientWidth` on the title), not a table scroll; no view test relies on the sideways scroller, so that file is re-run, not changed. The only sideways-scroll e2e is `reports-wrap.spec.ts:385`. (f) The builder's Desktop tab already exists (`ArrangeSection.tsx:28-36`). (g) Tests that pin version 2 change with F9: `v2_test.go:14-54`, `spec_test.go:107`, `saved_test.go:65`; `paging_test.go:168` (`TestAGroupedReportIsNotPaged`) is replaced by F7's tests; `params_test.go:19` (`TestRangePresetsMatchTheShell`) gains `allDates`. **Cost if wrong:** an implementer edits a file the plan does not list (out-of-plan file at review).
- **R19. Opening rows and sticky groups.** An opening row is not a sticky boundary; it scrolls under the stuck group row like a detail row. **Cost if wrong:** a stuck opening row covers the first line rows.
- **R20. Numbers and money in tests** are integer cents (`zoo` costs are cents, `zoo_test.go:253`), as every engine test writes them. **Cost if wrong:** test-only: a test that writes decimal money expects figures 100 times too small and fails at once, before any engine code is blamed; no product code reads this ruling.
- **R21. F10 narrows at every width, phones included (the built default; the human decides in Q3).** Spec F10 says "at every width"; so a report with no phone layout whose narrowed line fits at 390 now draws one line, where v2 wrapped it to two (label, then figures). Task 13 moves the existing `table-lines.test` phone cases from 390 to 360 so they still test wrapping, and adds a 390 case for the narrowed line. **Cost if wrong:** see Q3.
- **R22. "Clear sort" takes the place of "Edit report", not "Retry".** A refused run of kind `invalid` / `missing` shows "Edit report" today, not Retry (R8 corrected). Clear sort is a `ProblemsBody` action set in `ReportView.tsx` when `ReportViewModel.sortProblem` is set; `ReportStates.tsx` does not change. **Cost if wrong:** one action in the refusal body.
- **R23. A rollup or opening formula that adds a blank must coalesce it.** `add` passes a blank through (`formula.go:558-577`), so the spec §5 rollup `add(opening, sum(net))` gives a blank ending balance on an account with no line in the period. Task 13a's stub and Task 22 write `add(opening, coalesce(sum(net), 0))`. **The Books plan must use the same form** (spec §5 `balance` rollup) before checkpoint 2. **Cost if wrong:** the GL shows no ending balance on an account with a beginning balance and no activity.
- **R24. Amounts read from one currency field combine; a run-time guard blanks a row where their codes differ (Task 2b).** A Books money field names a per-row `currency` column (`books/server/report_sources.go:15-16,77-78`). Today a looked-up amount carries `currency{sql, id: "lk:<node JSON>"}` (`lookup.go:177-181`), a base path amount `currency{key: ck<n>}` (`rows.go:350`), and `currency.same` (`formula.go:70-80`) matches only equal ids or equal keys, so `combined` refuses the GL's `opening` (two lookups added), `balance` (`add(opening, runningSum(net))`) and `rollup` (`add(opening, coalesce(sum(net), 0))`) at `Validate` with "amounts in different currencies cannot be combined" (review 2 probes 1–2). The rule: (a) a money value read from a source field whose currency is a per-row field carries `currency.field = "<source key>.<currency field key>"` (`currencyField(p *pathPlan)`), set in `rowsCTE.pathCurrency`, `joinCtx.amountCurrency`, `lookupCtx.path`, and kept by `pickedAmount` and `summedLookup`; `id` does not change, because `valueCur` registers each lookup's currency key by its id and two lookups of one field must keep two keys. (b) `same` is true for two currencies with the same non-empty `field`; the old comparison is `sameRead`, which `currencyGuard` keeps using, so comparisons, ratios and amount params are guarded as before. (c) `fieldGuard`, called by `operator` on every amount result: where its operands hold two or more different reads (`sameRead` false) of one field, the amount is `NULL` on a row where two of their codes are known and differ, `expr.mixed` is true there (so R3's `vmx<idx>` cell and `mixedCurrency` notice name the column), and its currency is `COALESCE(<codes>)` with id `"any:" + that SQL`: the first known code, so a lookup that matched nothing (no code) takes the row's. Where no read has a code the amount has no currency (an opening of 0 with no earlier lines shows no code). (d) Task 5's `checkInColumn` stays as written (`combined([]expr{d, e}, at)`): `same` now matches a rollup or opening cell of the column's field. Not changed: a record leaf's amount (no `field`), a fixed-currency amount, amounts of two different currency fields (still refused), and an amount made of two reads inside a total (its `COALESCE` currency has no key, so `totalCurrency` refuses it with "this amount's currency is read per row…"). This goes beyond spec §4's letter (it names no combining rule for per-row currencies) because §5's `opening`, `balance` and `rollup` need it. Task 22 runs its fictional ledger on a per-row `currency` field, so it fails if this rule is wrong. **Cost if wrong:** too loose, a EUR row is added to a USD lookup — the guard blanks that cell and the notice names it, so no wrong figure shows; too strict, the Books General Ledger spec is refused by `Validate` at checkpoint 2 and core is reopened. Local to `formula.go` (`same`, `fieldGuard`) and the four places that set `field`. Two known costs of the built rule: (1) `fieldGuard` and the args' `mixed` OR also read the code of an `if` branch that the row does not take. `combined` gives `if(RE, coalesce(lk2), 0)` lk2's currency (`formula.go:95-116`), so the GL's `opening` guards each account's lookup (lk1) against the prior-year P&L lookup (lk2) on every account, also where the `if` gives 0, and Task 2's argument loop ORs lk2's `mixed` into every account's opening. In a book whose prior-year P&L lines are in another currency than an account's lines, or in more than one, that account's beginning balance, running balance and rollup are blank and a `mixedCurrency` notice names Balance; a one-currency book is unchanged (Q4; "Names the Books plan depends on"). (2) On a detail row a lookup's code is a correlated subquery, not a `w` key, and the guard repeats it: a GL detail row evaluates the opening lookup's subqueries about 11 times (5 in the balance's guard, 5 in its `mixed` helper, 1 in its currency cell; the `if` of `byNormal` runs one branch). A `w` key per such lookup is not a small change: every key in `c.currencies` is a grouping key of every level and of the Total (`levels.go:63,72-76`, `assemble.go:94,147-151`), so a key for a lookup read on detail rows would split groups and the Total by the lookup's code (a lookup that matched nothing gives NULL, another USD: two Total rows). Task 22's time test bounds the cost: about 2,000 lines run under 4 s.

### Open questions

**Answered (2026-11-06):** the human approved this plan and accepted the recommended answer to each question: Q1 yes (an authored layout for the current width applies even when the one line fits); Q2 keep F10 as written (the General Ledger draws one narrowed line at a 1024 window; `populated-1024` is re-captured at the Books gate 2); Q3 narrow on phones too; Q4 a book whose journal lines are in more than one currency shows blank balances with the `mixedCurrency` notice, for now.

- **Q1. Does an authored layout for the current width apply even when the one line fits?** **Recommended: yes** — keep v2's approved rule (R24: "An author arrangement for desktop replaces the one line"), so an author who arranged a width gets that arrangement there; F10 only adds what happens when there is none. Alternative: use an authored layout only when the narrowed one line does not fit (the literal reading of spec F10 "if it still does not fit, wraps: the authored `display.layouts` entry for that width"); then a desktop arrangement shows only on narrow desktops. **Cost if wrong:** one condition in `tableLines` and two vitest cases; the General Ledger is unaffected (it authors no desktop layout, and its phone one line never fits).
- **Q2. At a 1024 desktop window, does the General Ledger draw one narrowed line or the tablet layout?** With F10's order (narrow text columns to the 8-character floor first, then wrap) and R12's widths, the GL's narrowed line is 864 pt (80 + 5 × 80 + 3 × 128). **The number, computed from the code: a line gets 944 pt at a 1024 window.** A 1024 window is the desktop breakpoint (`core/components/workspace/useBreakpoint.ts:6`, `width >= 1024`). The rail takes 64 (`core/components/workspace/rail-width.ts:3`, applied at `PackageRail.tsx:55`; the left inset is 0 on web). Reports has no package sidebar: `hasSidebar` needs the active package's manifest `sidebar` (`WorkspaceLayout.tsx:41`), and reports is a rail link (`PackageRail.tsx:113`), not a package. The report view adds no side padding (`core/reports/components/ReportView.tsx:158-159`; `ReportTable`'s root is `min-h-0 flex-1`), and the customize drawer is closed until opened. So the table is 1024 − 64 = 960 wide, and the line is 960 − the 16 pt web scroll bar gutter (`ReportTable.tsx:144,264`, Task 14's `SCROLL_GUTTER`) = 944. **Conclusion: the GL's narrowed line fits at 1024; it draws one narrowed line there, not the tablet layout.** The label takes 80 and its five text columns get (944 − 80 − 384) / 5 = 96 pt each, about 10 characters. Its line would stop fitting only below a 944 window (864 + 16 + 64), which is not a desktop width, so under the recommended answer the GL never shows its tablet layout on a desktop; it shows it at the tablet breakpoint (768–1023), where its own tablet layout applies. Task 13a's 1024 e2e pins the premise (the measured line is at least 864 and under the stub's 1008), and Task 14 Step 6 runs it as the check that the number holds; it stays in the suite (no separate probe spec is written). So the GL at 1024 is one line with Name, Description and Split cut to about 10 characters, while spec §5 ("F10 picks it when the one line does not fit") and the approved phase-3 reference (`populated-1024`) show the tablet layout. **Recommended: keep F10 as the spec words it (narrow first, then wrap)**, so at a 1024 window the GL draws one narrowed line (944 pt available, 864 needed) and never its tablet layout on a desktop window; the human decides whether that is acceptable against spec §5 and the `populated-1024` reference. If it is, the Books plan changes: `populated-1024` is re-captured at gate 2 as one narrowed line, and checkpoint 3's tablet assertion runs at a tablet width (768–1023), where the tablet layout applies by breakpoint. Alternatives: (b) a higher text floor (for example 14 characters, about 140 pt), so the GL wraps at 1024 and every other report wraps earlier too; (c) a new optional v3 field `display.layouts[].preferred` that makes an authored layout of a narrower breakpoint win over narrowing; only the GL would set it. **Cost if wrong:** with the recommended answer, if the human wanted the reference: the GL at 1024 (and at every desktop window, since its line fits from 944 up) shows Name, Description and Split cut to about 10 characters instead of the tablet layout, and Books checkpoints 2–3 capture and assert a layout the human did not approve — one constant (b) or one field (c), Task 13a's 1024 assertions, and a re-capture later. With (b) or (c), if the human wanted the spec's order: every report wraps earlier than needed, or the spec format gains a field only one report uses.
- **Q3. On a phone, does a report with no phone layout draw one narrowed line when that line fits, or keep v2's two-line wrap?** A phone line gets 374 pt (390 − the 16 pt gutter). Under F10 a report whose narrowed line fits there (for example label 80 + one money 108 + two text columns narrowed to 93) now draws one line per row, where v2 drew the label line and then the figures line. **Recommended: narrow on phones too** (R21, the built default), as spec F10 says: "at every width". Alternatives: (b) phones keep v2's wrap — `tableLines` uses `fitOneLine` only at desktop and tablet, and a phone with no phone layout goes straight to `autoLines`; (c) narrow on phones only to a higher floor (for example 12 characters), so only very short rows fit one line. **Cost if wrong:** with the recommended answer, if the human wanted v2's phone look: phone reports with few columns look different from the approved v2 phone screens (one line, text cut to about 8–11 characters) — one breakpoint condition in `tableLines`, Task 13's 390 cases, and a re-run of the 390-wide report specs. With (b), if the human wanted F10's rule: a phone row that fits one line takes two. Under either answer the e2e hold: `reports-wrap.spec.ts:504` gained a `cost` column (Task 13a), so it wraps at 390 either way (456 > 374), and the General Ledger is unaffected (its phone narrowed line is 80 + 5 × 80 + 3 × 108 = 804, and it authors a phone layout).
- **Q4. Is a book whose journal lines are in more than one currency (a currency setting changed after posting) a case the General Ledger must show figures for?** Under R24 as built, `fieldGuard` and the `mixed` OR also read the prior-year P&L lookup inside `opening`'s `if`, on every account (R24 cost 1). So in such a book the affected accounts show blank beginning, running and ending balances, and the `mixedCurrency` notice names Balance. **Recommended: no, for now** — a mixed book shows blank balances with the notice, which is honest, not wrong; the Books plan carries the limit ("Names the Books plan depends on"). Alternative: (b) `fieldGuard` and the `mixed` OR skip an `if`/`case` condition and guard only the reads that can reach the result (per branch, so lk2 is guarded only on the retained-earnings row). **Cost if wrong:** with the recommended answer, if the human wants figures for a mixed book: every account whose lines differ in currency from the prior-year P&L lines shows no balance until core gains (b), which changes `fieldGuard`, Task 2's argument loop and their tests (a core PR after this one). With (b), if the human did not need it: a non-trivial branch-aware guard in `formula.go` for `if` and `case` (each branch's reads guarded on its own, the condition's reads not guarded), more tests in Task 2b, and more review risk in this checkpoint, for a case no org has yet.

### Names the Books plan depends on

| Name (brief "Produces") | Status |
|---|---|
| `reports.Node.Pick` values `"sum"`, `"count"` | kept |
| `reports.Node.Outer` (`reports.Path`, JSON `outer`) | kept |
| `reports.Node.Bound` (`"from"` \| `"to"`, with `Param`; JSON `bound`) | kept |
| range preset `allDates` | kept |
| `reports.Group.Opening` (`*reports.Opening`, JSON `opening`), `reports.Opening{Label string, Cells map[string]*reports.Node}` | kept |
| `reports.Column.Rollup` (`*reports.Node`, JSON `rollup`) | kept |
| `reports.Options.Sort` (`[]reports.ViewSort`, JSON `sort`), `reports.ViewSort{Col, Dir string}` | kept |
| `reports.KindOpening = "opening"` | kept |
| `reports.CurrentVersion = 3` | kept |
| client row kind `'opening'`; row id `opening:<group key JSON>`; test id `report-row-opening:<group key JSON>` | kept |
| heading sort control test id `report-sort-<col>` | kept |
| view URL key `s=<col>:<dir>` | kept |
| `rowsKeyOf` includes `options.sort` | kept |
| **added** (Books' gate-2 fixture uses them): `ResultColumn.sortable`; `ReportViewModel.sort: ViewSort \| null`, `ReportViewModel.onSortColumn(col: string): void` and `ReportViewModel.onClearSort(): void` (the static view model passes the sort state to the heading); `LineLayout` one-line kind now `{ kind: 'one-line'; widths: Record<string, number> }`; `ReportViewModel.sortProblem` (R22). The Books scratch route `screens/helix-mockup.tsx` builds a static `ReportViewModel` and does not compile after Task 15 until it sets `sort`, `onSortColumn`, `onClearSort` and `sortProblem` (Books checkpoint 0 gate 2 step) | new, no rename |
| spec §5 `balance` rollup `byNormal(add(opening, sum(net)))` | **Books plan change:** `byNormal(add(opening, coalesce(sum(net), 0)))` (R23) |
| R24's guard on spec §5 `opening`, `balance` and `rollup` | **Books plan limit:** the Books General Ledger shows correct figures only where an org's journal lines share one currency. A book whose lines are in more than one currency (a currency setting changed after posting) shows blank beginning, running and ending balances on the affected accounts, with the `mixedCurrency` notice naming Balance (R24 cost 1, Q4) |

### Changes after the plan review

1. Per-row-currency amounts: new Task 2b (`currency.field`, `sameRead`, `fieldGuard`, `fieldcurrency_test.go`); R24 rewritten (run-time guard, `mixedCurrency` notice, beyond §4's letter, cost); Tasks 5 and 6 gain per-row `cost` tests; Task 22's `zoo_ledger_lines` gets a per-row `currency` field.
2. R21 and `reports-wrap.spec.ts:504`: the test gains a `cost` column (Task 13a) so it still wraps at 390; Task 21 re-runs ten more 390-wide report specs one file at a time.
3. Task 6 calls `lv.checkInColumn(...)`; its Interfaces line names the `*level` receiver, with no hedge.
4. New Task 13a writes the stub report, the v3 e2e spec and the `reports-wrap.spec.ts` changes before Task 14, each red for a named reason or with a mutation proof; Tasks 14, 15, 16a, 18 turn them green; Tasks 20 and 21 only run.
5. One desktop builder e2e (Task 13a) drives This row's field, Count, Range start, a rollup formula and the opening row with label and cell; the false claim at Task 18 is gone.
6. R15, R16, R17 and R20 each have a cost if wrong.
7. R21 stays the built default and is now open question Q3; the F10 verdict bullet says "at desktop width and on phones".
8. Q2 states the 1024 number from the code (944 pt, so one narrowed line), its recommended answer and cost match it; the 1024 e2e stays in the suite and asserts the one result.
9. The checks-only preflight task is now Step 0 of Task 1; its own heading is gone.
10. Task 16 is split: Task 16 (builder lib, with its vitest) and Task 16a (the two components, driven by the builder e2e).
11. The Task outline, File Structure and Coverage check are generated from the task bodies; the head says `fctx.outerPath`; R8, the view text and Review Focus 3 say Clear sort replaces "Edit report" (R22); Task 7's red reason is the build failure on `c.Sortable`; Task 13a's builder test ids are checked against the code that forms them.
12. Task 13 adds `lines(800, 'desktop', [DESKTOP_LAYOUT, TABLET_LAYOUT])` → `DESKTOP_LINES`.
13. The stub report is `salesstub.team_balance` ("Stub team balance"); Task 22's file and tests are `balance_shape_test.go` and `TestABalanceShaped…`; Global Constraint 1 names exactly the two allowed exceptions; the `general-ledger:` describe prefix stays because the lane's e2e filter needs it.

Round 2 (fix review):

14. Finding 1: core names are neutral: `gramsLedger` → `gramsByAnimal` (Task 6), `pagedLedger` → `pagedBySpecies` (Task 8), the e2e constant → `Q2_NARROWED_LINE` and its comments and header (Task 13a), the Task 2b test comment, and Task 22's file comment and commit message; the Global Constraint exceptions are unchanged.
15. Finding 2: Task 13a says Task 2b's `currency.field` lets amounts of one currency field combine (R24), not "one combining id".
16. Finding 3: R24's cost names the untaken `if` branch that the guard reads (option a); "Names the Books plan depends on" gains the one-currency limit; new open question Q4.
17. Finding 4: a `w` key per detail-row lookup is not a small change (every currency key groups every level and the Total), so R24 names the repeated subqueries as cost 2; Task 22 adds the two indexes and `TestABalanceShapedReportOverTwoThousandLinesRunsInTime` (2,000 lines under 4 s, a 5× margin over a measured `sqlite3` model).
18. Finding 5: R17 lists the stub's nine columns and why `team`, `cost` and `quantity` are there.
19. Finding 6: Task 13a computes the 390-wide line of every phone table in `reports-create`, `reports-formula-layout`, `reports-formulas-v2`, `reports-group-pivot` and `reports-share`; none asserts a wrap that now fits, so no test moves; Task 21 only runs (no edit allowance).

---

## Global Constraints

- Core names no package: no feature slug, scope, collection name or `/api/<slug>/` route in `core/`, `app/`, `cli/`, `scripts/`, `server/`. `PATH=$WS/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts` (in `$WS/tinycld`) stays at 0 references. Go tests use the fictional `zoo_*` collections (`core/server/reports/zoo_test.go`); e2e uses the `salesstub` stub under `tests/scripts/reports-stub/`. No "ledger", "account", "journal" or "books" in core code, test names, test data or stub reports, with exactly two exceptions: Task 22's fictional zoo collections and sources (`zoo_ledger_*`, `zoo.ledger_*`, with their own field names, labels and rows), and the e2e file name `tests/e2e/reports-ledger-v3.spec.ts`, which the checkpoint brief fixes. Names that existing core test fixtures already use (for example the `account` column of `core/reports/lib/__tests__/table-lines.test.ts`) may be reused by a test that extends that fixture; a task adds no new such name. The stub report is `salesstub.team_balance` ("Stub team balance"). Every new e2e `test.describe` title starts with `general-ledger:`: the brief requires it and the lane's e2e filter is `-g "general-ledger:"`; it is a title prefix, not a code name.
- Every screen supports web and native at desktop, tablet and phone widths; nothing new is web-only or native-only.
- Data rules: pbtsdb for collection data; `useMutation` from `@tinycld/core/lib/mutations`; no raw PocketBase writes in components, hooks or e2e (e2e reads rows in one `page.evaluate` snapshot; a `page.route` that rewrites a run's `options.limit` is a read); row types derive from queries; Zustand only for shared UI state; no `useState` + `useEffect` sync; every form uses zod through `zodResolver`; RN 0.88 `onPress` wraps `handleSubmit`: `onPress={() => void handleSubmit(fn)()}`; JSX stays minimal with `isVisible` props; semantic colour tokens or `useThemeColor` only, no raw hex; no `any`, no `biome-ignore`, no `console.*`.
- zod v4: a recursive schema is typed `z.ZodType<T, T>`.
- Go: only `errors.Is(err, sql.ErrNoRows)` means "not found"; any other error is a fault.
- Spec limits (engine) unchanged: 100 columns, 4 link hops, 10 sort keys (also the viewer sort's limit), 6 group levels, labels ≤ 200, formula depth 32 and 1000 nodes; page size default 200, at most 1,000; group rows capped at 5,000.
- Tests before code: each task's test step writes its tests first and proves them red for the named reason; the implementer makes them green and does not edit them.
- Front-end behaviour is tested by Playwright on `salesstub` at desktop and at phone 390×844 where the phone path runs different code; vitest only for `lib/` logic. Go keeps Go tests (`go test -tags no_ui ./reports/` from `core/server`).
- E2E: never `page.goto()` for in-app moves; `E2E_PORT=7210 E2E_PORT_2=7211` before every e2e command; check the ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`); one spec file at a time; no bumped timeouts, no forced-serial runs; every created record uses `uniqueName`; every new `test.describe` title starts with `general-ledger:`.
- Run tools from `$WS/node_modules/.bin` (`$WS` = `/Users/nas/tmp/tinycld/books-purchases`); never `pnpm install`.
- Commits: repo `$WS/tinycld`, branch `books/p5-general-ledger` (never switch); the driver commits; messages never mention Claude. Before each lint step, `PATH=$WS/node_modules/.bin:$PATH biome format --write <the task's new and changed files>`.

## Review Focus

1. A saved v2 report (and the shipped "Stub team totals") opened after this PR → runs with no "upgraded" notice; its next save stores version 3 (Task 1 `TestAV2SpecLoadsSilentlyAsVersion3`; Task 21 re-runs `reports-source-params.spec.ts`, which opens the shipped v2 "Stub team totals").
2. A grouped report whose reader scrolls while a sale is added in another browser → the live re-run refetches from page 1, no duplicated or missing section at a seam (Task 8 `TestAGroupedPageSeamRepeatsNoRow`; Task 12 `mergePages` dedupe).
3. A link with `s=<col>:<dir>` for a column the report no longer has → the refusal shows "Clear sort" in place of "Edit report" (R22), and one press runs the report (Task 7 refusal at `options.sort.0.col`; Task 15 `refusalSortProblem` vitest; Task 13a e2e "a link that sorts by a column the report lacks offers Clear sort", run green in Tasks 15 and 20).
4. Two-currency rows under a money `sum` lookup → no value and one `mixedCurrency` notice naming the column, merged with a totals notice when both apply (Task 2 `TestASumLookupOverTwoCurrenciesHasNoValueAndANotice`).
5. A desktop 1024–1280 wide with a 9-column report → every heading's right edge inside the table, no horizontal scroll, text cells ending in "…" (Task 13 vitest; Task 13a e2e in `reports-wrap.spec.ts` at 1100 and `reports-ledger-v3.spec.ts` at 1024, run green in Tasks 14, 20 and 21).

## Deferrals

Deferrals: none. Every cell of spec §4 (F1–F11), the carried figure-only-row finding, and every item of the brief's "Tests it must add" is placed in a task (Coverage check). Q1, Q2, Q3 and Q4 are questions, not deferrals: the plan builds each recommended answer.

## What was probed

- `core/server/reports/spec.go:15-31` — `CurrentVersion = 2`, `upgradeSteps{1: identity}`; `:311-335` `Load`/`loadWith` (strict decode, `UpgradedFrom` only when a step changed the doc); `:338-376` `upgrade` refuses a newer version "version %d is newer than this server reads (%d)"; structs `Column` `:94-117`, `Node` `:132-151` (`Pick` exists, min/max), `Group` `:161-180`.
- `core/server/reports/lookup.go:95-146` `lookupIn` (refuses pick other than min/max at `:102`; `(SELECT min|max(v) FROM … WHERE …)`), `:165-184` `pickedAmount`, `:217-312` `lookupCtx` (col/record/param route to `outer`; `path` reads the looked-up source).
- `core/server/reports/formula.go:186-213` `fctx` interface; `:230-315` `compileNode` dispatch; `:318` `errNodeKind`; `:419-443` unset conditions; `:669` `between`.
- `core/server/reports/params.go:38-44` presets; `:366-384` `rangeValue`; `:430-456` `rangeDays`; `:472-503` `paramExpr` (dateRange → `rng`, `lo`, `hi`); `:177-207` `ResolvedParam`. `options.go:171` `paramLeaf`.
- `core/server/reports/run.go:20-37` `Options` (no sort), `:108-114` row kinds, `:183` `runLimits{rowCap 5000, pageSize 200, maxPage 1000}`, `:306-350` `run` (grouped cursor refused at `:309-311`), `:424-446` `groupPass` + `capLevelRows`, `:476-573` `groupStatement`, `:672-726` `detailPass` (grouped: rowCap, no paging; window select then keyset outside at `:707`), `:742-807` `detailSortValue`/`detailSelect` (`frame.order` built from the spec sort), `:970-1030` `expansion.where` (binds key tuples as JSON).
- `core/server/reports/paging.go` — `cursor{k,v}`, `fingerprint` (cols, dirs, kinds), `readCursor`, `keysetAfter`.
- `core/server/reports/assemble.go:13-63` `columns`; `:92-118` `markCurrencies` (`mixedCurrency` notice); `:159-202` `assemble`; `:215-231` `group`; `:297-331` `groupWithLabel` (`canOpen` rules at `:299-308`, ids `g:<key>` / `s:<key>`); `:334-347` `detail`.
- `core/server/reports/levels.go:24-87` `level`; `:179-182` `cell`; `:625-775` `levelCtx.col`, `rowsTotal`, `recompute`; `:874-947` `readsRowValue`, `lookupReadsRowValue`; `:951-983` `levelCtx.record`/`path` (NULL above the group / on levels).
- `core/server/reports/` tests: `zoo_test.go` (`newZoo`, `z.feeding`, sources `zoo.sites`, `zoo.animals` with `fee`/`fee_currency`, `zoo.feedings`), `lookup_test.go`, `paging_test.go:168`, `params_test.go:19`, `v2_test.go:14,52`, `spec_test.go:107`, `saved_test.go:65`, `groups_test.go:38-45,293-300` (rowCap via `withLimits`), `fuzz_test.go:184` `FuzzRuleParity`, `formula_cases_test.go:351,379` over `core/lib/forms/fixtures/formula-cases.json` (lookup rows at `:571-592`).
- `core/reports/lib/spec.ts:9,21-58,109-147,182-205`; `types.ts:21,41-98`; `run-client.ts:17-51` (`rowsKeyOf` drops options); `rows.ts:37-50,147-158` (`mergePages` keeps only earlier detail rows); `view-url.ts:22-171` (keys `p-*`, `div1000`, `nocents`, `negative`, `red`, `hide`); `params.ts:11-48,97-102,390-410`; `table-lines.ts:203-222` (`autoLines`), `:287-298` (`tableLines`, desktop always one line), `:301-304` (`rowLayout`); `format.ts:108-114` (`RIGHT_TYPES`).
- `core/reports/components/ReportTable.tsx` (341 lines; `Sideways` `:169-214`, `OneLineHeading`/`FrozenLabel` `:91-111`, `fitsOnOneLine` `:301`, `onEndReached` `:262`); `report-table-layout.ts` (68 lines); `ReportRow.tsx` (424 lines; `ROW_RULE`/`ROW_WEIGHT` `:14-28`, `FrozenLabel` `:264`, `BlocksRow` `:332-368`, `labelId` `:143`); `ParamPeriodControl.tsx:118` (pills from `presetsFor`); `hooks/useReportRun.ts:14,30-35,60-61` (sends `limit: 200` on every page); `hooks/useReportView.ts:90-96,140-143,177` (body and URL).
- Builder: `lib/builder/formula-ops.ts:67-117` (`editable`), `:129-149` (`childContext` for lookups), `:228-251` (`partsAt`), `:335-345` (new lookup seeds `pick: 'min'`); `components/builder/report-formula.tsx:254-288` (pick menu, Smallest/Largest); `lib/builder/draft.ts:79-196` (draft schemas), `:578` `draftFromSpec`, `:884` `specFromDraft`; `components/builder/GroupLevelParts.tsx` (`GroupLabel`, `GroupFill`, `GroupHaving`), `ColumnsSection.tsx:200-223` (Total choice), `ArrangeSection.tsx:28-36` (Desktop tab).
- E2E: `tests/e2e/reports-wrap.spec.ts:385-433` (the only sideways test, `wheel(300, 0)` at `:416`), `:187-212` (desktop one line, `wideReport` 4 columns), `reports-view.spec.ts:111` (title helper), `reports-source-params.spec.ts:277-290` (phone, no sideways scroll), `:363-378` (`limit` rewrite precedent); helpers `tests/e2e/report-view-helpers.ts` (`rowOf`, `cellOf`, `openReportsHub`), `tests/e2e/reports-helpers.ts` (`uniqueName`, `addSale`, `addSales`, `loginAsShippedOwner`); stub `tests/scripts/reports-stub/server/register.go.txt` (two shipped reports, `:56`, `:79`).
- `core/components/workspace/useBreakpoint.ts:5-9` (1024 / 768).
- `core/help/reports.md` sections (`## Totals` :44 … `## Narrow screens` :103); `docs/packages.md:1111,1267,1290`.

---

## File Structure

Generated from the task bodies, which are binding. Every path is under `/Users/nas/tmp/tinycld/books-purchases/tinycld`; Go paths under `core/server/reports/`, TypeScript paths under `core/reports/` unless written in full.

| File | Change (task) | Tasks |
|---|---|---|
| `spec.go` | `CurrentVersion` 3, step 2, `Column.Rollup`, `Node.Outer`, `Node.Bound`, `Group.Opening`, `Opening`, `Opening.cellIDs` (1) | 1 |
| `run.go` | `Options.Sort`, `ViewSort`, `KindOpening` (1); `mixedCell`, `noMixedCell`, `mixedHelpers`, `helperCell`, the cells of `groupStatement` and `detailSelect`, the call in `run` (2); `groupStatement` opening cells, `helperCell`, `carriedSQL` (6); `Run` plans `options.sort`, `viewSortPlans`, viewer keys first in `detailSelect`, `ResultColumn.Sortable` (7); `Options` comments, `runner.page`, `run`, `detailPass`, `groupedPage`, `openGroups`, `groupLevels`, `leafOrder`, `leafJoin`, `groupedAfter` (8) | 1, 2, 6, 7, 8 |
| `alias.go` | aliases in `outer`, `rollup`, `opening.cells` (1) | 1 |
| `lookup.go` | decoder walk covers `rollup`, `opening.cells` (1); `lookupPicks`, pick check, `count`, `summedLookup` (2); `lookupCtx.path`, `pickedAmount`, `summedLookup` keep `currency.field` (2b); `lookupCtx.outerPath`, `outerToMany`, the leaf check in `lookupIn` (3); a lookup node with a bound (4) | 1, 2, 2b, 3, 4 |
| `bind.go` | `formulaSizes` covers `rollup`, `opening.cells` (1); rollup refusals (5); `groupsAndPivot` calls `binder.opening`, pivot's last level (6) | 1, 5, 6 |
| `formula.go` | `expr.mixed`, `orSQL`, the arg loop (2); `currency.field`, `same`, `sameRead`, `currencyGuard` dedupe, `fieldGuard`, its call in `operator` (2b); `fctx.outerPath`, `ctxBase.outerPath`, `nodeKinds`, `errNodeKind`, dispatch (3); `expr.all`, `between`, bound checks and dispatch (4) | 2, 2b, 3, 4 |
| `rows.go` | `currencyField`, `pathCurrency` (2b); `compiler.viewSort`, `compiler.sortable` (7) | 2b, 7 |
| `join.go` | `amountCurrency` sets `currency.field` (2b) | 2b |
| `layout.go` | computed legs carry `vmx<idx>` (2); computed legs carry `NULL` opening cells (6) | 2, 6 |
| `levels.go` | a blanked cell has no mixed check (2); `lookupReadsRowValue` (3); `levelCtx.col`, `levelCtx.rollup`, `level.checkInColumn`, `readsRowValue` (5); `level.openingCells` (6) | 2, 3, 5, 6 |
| `params.go` | `allDates`, `rangeDays`, `paramExpr`, `rangeBound` (4) | 4 |
| `assemble.go` | `markMixedLookups` (2); `groupWithLabel` emits the opening row and lets the level open, `runner.openingCells`, `runner.openingMixed` (6); `columns` sets `Sortable` (7); `runner.pageWindow` (8) | 2, 6, 7, 8 |
| `paging.go` | `cursor.Group`; `fingerprint`, `cursorOf`, `readCursor` take the group levels; `keysetCond` (8). Task 7 does not change it (a viewer key's cursor column is `view:<col>`). | 8 |
| `v3_test.go` (new) | `everyV3`, `v3Refusal`, `requireV3Refusal`, the load tests and `TestV3PartsAreValidated` (1); refusal rows (2, 3) | 1, 2, 3 |
| `v2_test.go`, `spec_test.go`, `saved_test.go` | version 2 → 3 assertions (1) | 1 |
| `lookup_test.go` | sum/count tests, the bad-pick message (2); outer tests (3) | 2, 3 |
| `formula_cases_test.go` | harness picks sum and count (2); harness `outerPath` (3); `paramDefs`, `paramValues`, `tableCtx.param` (4) | 2, 3, 4 |
| `formula_run_test.go` | one-row case count 59 → 63 (2), → 64 (3); params in the run harness, → 65 (4) | 2, 3, 4 |
| `core/lib/forms/fixtures/formula-cases.json` | four lookup cases (2); one outer case (3); one bound case (4) | 2, 3, 4 |
| `fieldcurrency_test.go` (new) | R24 tests (2b) | 2b |
| `params_test.go` | presets row, bound and All dates tests (4) | 4 |
| `rollup_test.go` (new) | F6 tests (5) | 5 |
| `opening_test.go` (new) | F5 tests (6) | 6 |
| `viewsort_test.go` (new) | F8 tests (7) | 7 |
| `paging_test.go` | `TestAGroupedReportIsNotPaged` deleted; grouped page tests (8) | 8 |
| `having_test.go` | the hidden-rows test reads a page, not the row cap (8) | 8 |
| `run_test.go` | comment of `TestTheRowCapNoticeIsGivenOnce` (8) | 8 |
| `fuzz_test.go` | `sumLookupParity`, `pagedGroupParity` branches (9) | 9 |
| `balance_shape_test.go` (new) | the General Ledger's spec shape on fictional `zoo_ledger_*` collections (22) | 22 |
| `lib/spec.ts` | v3 zod, `CURRENT_SPEC_VERSION = 3` (10) | 10 |
| `lib/types.ts` | `RowKind` `'opening'`, `ResultColumn.sortable` (10) | 10 |
| `lib/run-client.ts` | `ViewSort`, `RunOptions.sort` (10); `rowsKeyOf` (12); `refusalSortProblem` (15) | 10, 12, 15 |
| `lib/format.ts` | `cellType` reads the detail type on an opening row (10) | 10 |
| `components/ReportRow.tsx` | `opening` in `ROW_RULE`/`ROW_WEIGHT` (10); `oneLineLabelStyle`, `OneLineRow`, `StickyGroup`, `TableRowProps`, label tone, `FrozenLabel` deleted (14) | 10, 14 |
| `lib/__tests__/spec.test.ts`, `lib/__tests__/format.test.ts` | v3 schema tests, opening cell type (10) | 10 |
| `lib/params.ts` | `RANGE_PRESETS`, `PRESET_LABELS`, `periodWords`, `emptyMessage`, `emptyAction` (11) | 11 |
| `lib/view-url.ts` | `ViewOptions.sort`, `SORT_KEY`, `encodeSort`, `decodeSort` (11); `nextSort` (15) | 11, 15 |
| `lib/__tests__/params.test.ts` | All dates tests (11) | 11 |
| `lib/__tests__/view-url.test.ts` | sort in the URL (11); `nextSort` (15) | 11, 15 |
| `lib/rows.ts` | `mergePages` (12) | 12 |
| `hooks/useReportRun.ts` | comment only (12) | 12 |
| `lib/__tests__/run-client.test.ts` | `rowsKeyOf` (12); `refusalSortProblem` (15) | 12, 15 |
| `lib/__tests__/rows.test.ts` | grouped `mergePages` (12) | 12 |
| `lib/table-lines.ts` | `LineLayout`, `isFigureColumn`, `fitOneLine`, `tableLines`, `rowLayout` (13) | 13 |
| `components/report-table-layout.ts` | `TEXT_FLOOR`, `fitsOnOneLine` removed (13); `ReportTableLayout`, `reportTableLayout`, `layoutFor` deleted (14) | 13, 14 |
| `components/ReportTable.tsx` | the `tableLines` call without `oneLineFits` (13); no `Sideways`, widths from `layout.widths`, the gutter (14); heading sort control (15) | 13, 14, 15 |
| `lib/__tests__/table-lines.test.ts` | F10 fit order, figure-only rows (13) | 13 |
| `components/__tests__/report-table-layout.test.ts` | `fitOneLine`, `isFigureColumn` (13); `describe('reportTableLayout')` deleted (14) | 13, 14 |
| `hooks/useReportView.ts` | sort in the body and the model, `sortProblem` (15) | 15 |
| `components/ReportView.tsx` | sort props to the table; Clear sort action (15). `components/ReportStates.tsx` does not change (R22). | 15 |
| `lib/builder/formula-parts.ts`, `formula-ops.ts`, `formula-type.ts`, `formula-describe.ts`, `formula-context.ts` | parts `outer`, `bound`, `LOOKUP_PICKS`, `boundChoices`, `rowSource`, editing, types, read-back (16) | 16 |
| `lib/builder/__tests__/formula-fixture.ts`, `formula-ops.test.ts`, `formula-describe.test.ts`, `formula-type.test.ts` | `rowSource` on the fixtures; v3 part tests (16) | 16 |
| `components/builder/report-formula.tsx`, `report-formula-paths.tsx` | pick menu from `LOOKUP_PICKS`, `BoundLeaf`, `OuterLeaf` (16a) | 16a |
| `lib/builder/draft.ts` | opening and rollup drafts, v3 round trip, problems (17) | 17 |
| `lib/builder/__tests__/draft.test.ts` | v3 draft tests (17); desktop arrangement tests (18) | 17, 18 |
| `components/builder/GroupLevelParts.tsx`, `GroupSortSection.tsx`, `ColumnsSection.tsx`, `ArrangeSection.tsx` | `GroupOpening`, `OpeningCell`, `TextAction`; `ColumnRollup`; Arrange caption (18) | 18 |
| `core/help/reports.md`, `docs/packages.md` | help and docs (19) | 19 |
| `tests/scripts/reports-stub/server/register.go.txt` | third shipped report `salesstub.team_balance` (13a) | 13a |
| `tests/e2e/reports-ledger-v3.spec.ts` (new; the brief fixes the name) | the v3 e2e (13a) | 13a |
| `tests/e2e/reports-wrap.spec.ts` | header comment; `:385-433` replaced by the narrowing test; `:504-546` gets a `cost` column (13a) | 13a |

Tasks 20 and 21 plan no file (a defect they find is fixed in its owning task's file). Not changed by any task: `detail.go`, `lib/dates.ts`, `components/ParamPeriodControl.tsx`, `components/ReportStates.tsx` (R18, R22).

---

## Task outline

Generated from the task bodies, which are binding: where this outline and a body differ, the body wins. Go paths are under `core/server/reports/`, TypeScript paths under `core/reports/`.

**Run order (25 tasks):** 1, 2, 2b, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 13a, 14, 15, 16, 16a, 17, 18, 19, 20, 21, 22. Every task's test step writes its tests first and proves them red for the reason named (a test green at once gets a mutation proof); every Go task runs `cd $WS/tinycld/core/server && go test -tags no_ui -count=1 ./reports/` green before its commit; every TS task runs `tinycld-pkg check` (biome, tsc, all vitest).

### Task 1 — F9: spec version 3 and the new fields (Go)
- **F:** F9 (engine, schema) · **Files:** `spec.go`, `run.go` (types only), `alias.go`, `lookup.go` (decoder walk), `bind.go` (`formulaSizes`), `v3_test.go` (new), `v2_test.go`, `spec_test.go`, `saved_test.go`.
- **Step 0 (preflight, no file):** the branch is `books/p5-general-ledger` at or after `aa16a677`, the tree is clean but for the plan, and the Go reports suite, `tinycld-pkg check` and `check-core-isolation.ts` are green; a red baseline is fixed at its root first.
- **Tests first:** `TestAV2SpecLoadsSilentlyAsVersion3`; `TestAVersion4SpecIsRefused`; `TestASpecWithEveryV3FieldLoadsAndIsStoredUnchanged`; `TestV3SlotsResolveAliasesAndCountSize`; the table `TestV3PartsAreValidated`; v2/spec/saved version assertions moved to 3.
- **Red:** fails to build: `undefined: ViewSort`, `undefined: KindOpening`, `…Opening undefined`, `…Rollup undefined`, `…Outer undefined`.
- **Produces:** `const CurrentVersion = 3`; `upgradeSteps[2]` (identity); `Column.Rollup *Node`; `Node.Outer Path`; `Node.Bound string`; `Group.Opening *Opening`; `type Opening struct{ Label string; Cells map[string]*Node }`; `(o *Opening) cellIDs() []string`; `Options.Sort []ViewSort`; `type ViewSort struct{ Col, Dir string }`; `KindOpening = "opening"`; in `v3_test.go`: `everyV3`, `v3Refusal`, `requireV3Refusal`, `TestV3PartsAreValidated` (rows added by Tasks 2 and 3).

### Task 2 — F1: `sum` and `count` lookups (Go)
- **F:** F1 (engine) · **Files:** `lookup.go`, `formula.go`, `run.go`, `layout.go`, `levels.go`, `assemble.go`, `lookup_test.go`, `formula_cases_test.go`, `formula_run_test.go`, `core/lib/forms/fixtures/formula-cases.json`, `v3_test.go`.
- **Tests first:** `TestASumLookupAddsTheMatchingRows`; `TestASumLookupOverTwoCurrenciesHasNoValueAndANotice`; `TestAnEmptyMatchIsBlankForSumAndZeroForCount`; `TestACountLookupCountsRowsWithAValue`; `TestPickSumNeedsANumberOrAnAmount`; formula-table cases "lookup pick sum adds the matching values", "lookup pick count counts them", "lookup pick sum with no match is empty", "lookup pick count with no match is zero"; `v3_test.go` rows.
- **Red:** `pick is min or max` (in the run and table cases), the problem wants "min, max, sum or count", and `ran 59 detail rows`.
- **Produces:** pick values `"sum"`, `"count"`; `lookupPicks`; `expr.mixed string`; `orSQL(a, b string) string`; `summedLookup(…)`; `mixedCell(e expr, idx int) string` (`vmx<idx>`), `noMixedCell`, `mixedHelpers(idx int) []string`; `(rn *runner) markMixedLookups(…)`; the notice "A looked-up total mixes currencies, so it shows no value.".

### Task 2b — R24: amounts read from one currency field combine, guarded per row (Go)
- **F:** F1, F5, F6 money (engine) · **Files:** `formula.go`, `rows.go`, `join.go`, `lookup.go`, `fieldcurrency_test.go` (new).
- **Tests first:** `TestALookedUpAmountCombinesWithAmountsOfItsField`; `TestALookedUpAmountCombinesWithATotalOfItsField`; `TestALookedUpAmountInAnotherCurrencyThanItsRowIsBlank`; `TestAmountsOfTwoCurrencyFieldsStillDoNotCombine` (green at once: it pins what does not change).
- **Red:** `columns.1.formula: amounts in different currencies cannot be combined` (and `columns.3.formula: …` for the total).
- **Produces:** `currency.field string` (`"<source key>.<currency field key>"`); `currencyField(p *pathPlan) string`; `(a currency) sameRead(b currency) bool` (today's `same`); `same` also true for one non-empty `field`; `fieldGuard(c fctx, e expr, values []expr) expr`.

### Task 3 — F2: the `outer` leaf (Go)
- **F:** F2 (engine) · **Files:** `formula.go`, `lookup.go`, `levels.go`, `formula_cases_test.go`, `formula_run_test.go`, `lookup_test.go`, `core/lib/forms/fixtures/formula-cases.json`, `v3_test.go`.
- **Tests first:** `TestAnOuterLeafCorrelatesALookupToItsRow`; `TestAnOuterLeafInANestedLookupReadsTheReportRow`; `TestAnOuterLeafIsBlankOnGroupRows`; `TestAnOuterLeafIsRefusedOutsideALookup`; `TestAnOuterLeafIsRefusedOnAToManyPath`; formula-table case "outer reads the report row inside a lookup"; `v3_test.go` rows.
- **Red:** `a formula node is one of op, col, path, param, value, subtotal or record` (an outer-only node is no kind yet), and `ran 63 detail rows`.
- **Produces:** `fctx.outerPath(p Path, at string) (expr, error)` (the brief's `fctx.outer`); `ctxBase.outerPath` (refuses outside a lookup); `lookupCtx.outerPath` (refuses a to-many hop); `errNodeKind` names `outer`; `outerToMany(base *Source, p Path) bool`.

### Task 4 — F3 date-range bounds and F4 All dates (Go)
- **F:** F3, F4 (engine) · **Files:** `params.go`, `formula.go`, `lookup.go`, `params_test.go`, `formula_cases_test.go`, `formula_run_test.go`, `core/lib/forms/fixtures/formula-cases.json`.
- **Tests first:** `TestABoundReadsARangesFirstAndLastDay`; `TestABoundOfAnOpenRangeHasNoValue`; `TestAllDatesMatchesEveryRowAndResolvesWithNoBounds`; `TestABoundIsRefusedOffADateRange` (all in `params_test.go`); `TestRangePresetsMatchTheShell` gains `allDates`; formula-table case "param bound from reads the range's start".
- **Red:** `allDates` falls to `last12Months`; `the preset is one of [today … last12Months]`; `bound` is ignored, so the node is the whole range and no cell takes it; the refusals get `err = <nil>`; `ran 64 detail rows`.
- **Produces:** preset `"allDates"` (last in `rangePresets`); a resolved All dates `paramValue` with empty `from`/`to`, not unset; `expr.all bool`; `rangeBound(e expr, bound, at string) (expr, error)`; formula-table keys `paramDefs`, `paramValues`.

### Task 5 — F6: rollup formulas (Go)
- **F:** F6 (engine) · **Files:** `levels.go`, `bind.go`, `rollup_test.go` (new).
- **Tests first:** `TestARollupShowsOnGroupSubtotalAndTotalRows`; `TestARollupThatReadsARecordIsBlankOnTheTotal`; `TestARollupReadsLookupsAndParams`; `TestARollupAddsALookedUpAmountOfItsColumnsField` (per-row-currency `cost`, R24); `TestARollupIsRefusedWithRowsTotals`; `TestARollupIsRefusedOnAChildRowOrPivotCellColumn`; `TestARollupMustHaveItsColumnsTypeAndCurrency`.
- **Red:** the rollup is ignored: group cells show the plain total or a blank, and every refusal test gets `err = <nil>`.
- **Produces:** `(lc *levelCtx) rollup(cp *colPlan) (expr, error)`; `(lv *level) checkInColumn(cp *colPlan, e expr, at, what string) error` (Task 6 calls it with "an opening cell"); bind refusals at `columns.N.rollup`.

### Task 6 — F5: opening rows (Go)
- **F:** F5 (engine) · **Files:** `bind.go`, `levels.go`, `run.go`, `layout.go`, `assemble.go`, `opening_test.go` (new).
- **Tests first:** `TestAnOpeningRowIsFirstUnderEachOpenGroup`; `TestAClosedGroupHasNoOpeningRow`; `TestAFillGroupWithNoRowsOpensOntoItsOpeningRow`; `TestAGroupItsHavingHidesHasNoOpeningRow`; `TestOpeningCellsReadARecordALookupAndAParam`; `TestAnOpeningSumOverTwoCurrenciesIsBlankWithANotice`; `TestAnOpeningCellAddsUpItsColumnsAmounts` (per-row-currency `cost`, R24); `TestOpeningIsRefusedWhereItCannotShow`.
- **Red:** no `opening` row (`kind = [group detail subtotal …]`); the refusals get `err = <nil>`.
- **Produces:** `(lv *level) openingCells() ([]string, error)` (cells `op<idx>`, `opcur<idx>`, `opmx<idx>`); `carriedSQL(s *stmt, e expr) string`; `(rn *runner) openingCells(…)`, `(rn *runner) openingMixed(…)`; rows `Row{ID: "opening:" + keyString(key), Kind: KindOpening, …}` right after an expanded group row.

### Task 7 — F8: viewer sort (Go)
- **F:** F8 (engine) · **Files:** `rows.go`, `run.go`, `assemble.go`, `viewsort_test.go` (new). `paging.go` does not change.
- **Tests first:** `TestAViewerSortReordersDetailRowsInsideTheirGroups`; `TestRunningValuesKeepTheSpecsOrderUnderAViewerSort`; `TestAViewerSortOnTheFirstColumnSortsByTheDetailLabel`; `TestARunningValueIsNotAViewerSortKey`; `TestAViewerSortIsCheckedAtRunTime`; `TestACursorIsRefusedAfterTheViewerSortChanges`; `TestResultColumnsSayWhichSort`.
- **Red:** fails to build on `c.Sortable undefined`; after the field alone, detail rows keep the spec's order and the refusals get `err = <nil>`.
- **Produces:** `viewSortPlans(c *compiler, keys []ViewSort) ([]sortPlan, error)`; `compiler.viewSort []sortPlan`; `ResultColumn.Sortable bool` (JSON `sortable`); viewer keys `vs<i>` before the spec's keys.

### Task 8 — F7: paged grouped reports (Go)
- **F:** F7 (engine), F8 cursor (grouped) · **Files:** `paging.go`, `run.go`, `assemble.go`, `paging_test.go`, `having_test.go`, `run_test.go` (comment).
- **Tests first:** `TestAGroupedReportPagesAcrossAGroupBoundary`; `TestARunningSumContinuesOnPageTwo`; `TestAGroupedPageSeamRepeatsNoRow`; `TestASixThousandRowGroupedReportPagesToItsEnd`; `TestAGroupedPageHoldsAtMostAThousandRows`; `TestAGroupedCursorIsRefusedAfterASpecChangeOrASortChange`; `TestGroupRowsStillStopAtTheGroupCap` (all in `paging_test.go`); `TestHiddenRowsTakeNoPlaceOnAPage` (`having_test.go`). `TestAGroupedReportIsNotPaged` is deleted.
- **Red:** `1 pages` (a grouped run returns every detail and no `Page`); `options.cursor: a grouped report is not paged`; `page = <nil>`.
- **Produces:** `cursor{Key string; Group []any; Values []any}` (JSON `k`, `g`, `v`); `fingerprint(levels, keys)`, `cursorOf(levels, keys, row, group)`, `readCursor(levels, keys, text)`; `keysetCond(s, keys, values)`; `(rn *runner) openGroups(…)`, `detailPass(leaves)`, `pageWindow()`; a grouped `Result.Page`.

### Task 9 — Rule-parity fuzz: sum lookups and paged grouped reports (Go)
- **F:** F1, F7 (engine tests) · **Files:** `fuzz_test.go`.
- **Tests first:** `FuzzRuleParity` gains `sumLookupParity` (per feeding, a sum and a count lookup equal the sum and count over the rows the list API gives the reader) and `pagedGroupParity` (a grouped report's pages, joined as the client joins them, equal the one-page run).
- **Red:** green at once (Tasks 2 and 8 are in); Step 2 proves each branch red on a broken engine with a scratch overlay.
- **Produces:** nothing new.

### Task 10 — Client spec and result types v3
- **F:** F9 (schema), F5 (row kind), F8 (types), F1–F3/F5/F6 (schema) · **Files:** `lib/spec.ts`, `lib/types.ts`, `lib/run-client.ts`, `lib/format.ts`, `components/ReportRow.tsx`, `lib/__tests__/spec.test.ts`, `lib/__tests__/format.test.ts`.
- **Tests first:** spec.test 'reads a version 1, 2 and 3 spec and refuses version 4'; `describe('spec version 3')`: 'reads every v3 part as it is', 'reads a v3 spec with none of the new parts', 'reads an opening row with no cells', 'refuses a pick, a bound or an opening label the engine refuses', 'refuses an outer leaf that is not a path of hops, and an opening part it does not know'; format.test 'reads an opening row’s cell as the column’s detail type'.
- **Red:** `parseSpec` returns null for version 3 and the new parts; `CURRENT_SPEC_VERSION` is 2; `cellType(…, row('opening'))` is `'number'`.
- **Produces:** `CURRENT_SPEC_VERSION = 3`; `SpecNode.outer?`, `.bound?`, `.pick?` (`'min' | 'max' | 'sum' | 'count'`); `SpecGroup.opening?`; `SpecColumn.rollup?`; `RowKind` `'opening'`; `ResultColumn.sortable?`; `export type ViewSort = { col: string; dir: 'asc' | 'desc' }` (`lib/run-client.ts`); `RunOptions.sort?: ViewSort[]`.

### Task 11 — Client: All dates and the sort in the URL
- **F:** F4 (client), F8 (URL) · **Files:** `lib/params.ts`, `lib/view-url.ts`, `lib/__tests__/params.test.ts`, `lib/__tests__/view-url.test.ts`. `ParamPeriodControl.tsx` does not change (its pills follow `presetsFor`).
- **Tests first:** params.test `describe('All dates')`: 'is the last range preset, and only a range offers it', 'names the period on the chip, in the header and when nothing matches', 'keeps an unset range without words', 'offers no narrower period when All dates has no rows', 'reads All dates as a default'; view-url.test: 'reads a sort as <column>:<asc|desc> and ignores any other shape', 'writes the sort, and leaves it out with none', 'keeps the sort in a copied link', 'reads All dates from the address'.
- **Red:** `presetsFor` ends with `'last12Months'`; `periodWords` is null for a range with no days; `decodeSort`/`encodeSort`/`SORT_KEY` are not exported; `parseViewOptions(…).sort` is undefined.
- **Produces:** preset `'allDates'` ("All dates", the last range preset); `ViewOptions.sort: ViewSort | null`; `SORT_KEY = 's'`; `encodeSort(sort: ViewSort): string`; `decodeSort(raw: string | undefined): ViewSort | null`.

### Task 12 — Client: rows key with the sort, grouped pages merge
- **F:** F11, F7 (client) · **Files:** `lib/run-client.ts`, `lib/rows.ts`, `hooks/useReportRun.ts` (comment only), `lib/__tests__/run-client.test.ts`, `lib/__tests__/rows.test.ts`.
- **Tests first:** run-client.test 'adds the reader’s sort to the key, and only the sort'; rows.test (in `describe('mergePages')`) 'appends the next grouped page’s sections after the first page’s rows', 'keeps one copy of an opening or subtotal row two pages both give'.
- **Red:** the sorted key is `'report:r1'` (options ignored); the merged grouped rows drop page 1's group and opening rows, and the seam repeats `'d:1'` and `'s:a'`.
- **Produces:** `rowsKeyOf(body)` (byte-identical without a sort, `…|sort:<JSON>` with one, R11); `mergePages(pages)` (grouped-aware, R10).

### Task 13 — F10 lib: fit, never clip; figure-only rows
- **F:** F10 (client lib), F5 (wrap layout) · **Files:** `lib/table-lines.ts`, `components/report-table-layout.ts`, `components/ReportTable.tsx` (the `tableLines` call), `lib/__tests__/table-lines.test.ts`, `components/__tests__/report-table-layout.test.ts`.
- **Tests first:** report-table-layout.test `describe('fitOneLine')`: 'keeps every natural width where the line fits', 'narrows the label first', 'then lowers the text columns to one cap, the figures kept', 'does not fit below the floor', 'keeps a number, a percent, a date, a time and a yes/no at full width', 'uses the phone widths on a phone', 'fits a lone label down to the floor'; `describe('isFigureColumn')`. table-lines.test: existing cases moved (phone cases 390 → 360, R21) and 'draws one line with its natural widths wherever the columns fit'; `describe('F10: one line that fits, then the narrower arrangements')`: 'draws a desktop line that fits with its natural widths', 'narrows the label, then the text columns, before it wraps', 'uses the authored desktop arrangement where there is one, even when the line fits', 'wraps a desktop too narrow for the line to its own authored arrangement' (`lines(800, 'desktop', [DESKTOP_LAYOUT, TABLET_LAYOUT])` → `DESKTOP_LINES`), 'wraps a desktop too narrow for the narrowed line to the tablet arrangement, then the phone one, then automatically', 'wraps a tablet in the same order, never to a wider arrangement'; `describe('figure-only rows in a wrapped layout')`: 'draws an opening row as its label line, then its figures line', 'draws a subtotal without its blank text lines, and keeps a blank figure’s track', 'keeps every line of a detail row, and of any row with a value in every column'.
- **Red:** `fitOneLine`/`isFigureColumn` not exported, `TEXT_FLOOR` undefined; `tableLines` returns `{ kind: 'one-line' }` with no widths on every desktop; `rowLayout` returns an opening row's layout unchanged. 'wraps a desktop too narrow … to its own authored arrangement' is green at once and gets a mutation proof.
- **Produces:** `TEXT_FLOOR = 80` (`report-table-layout.ts`); `isFigureColumn(column: ResultColumn): boolean`; `fitOneLine(columns: readonly ResultColumn[], width: number, breakpoint: Breakpoint): Record<string, number> | null`; `LineLayout = { kind: 'one-line'; widths: Record<string, number> } | { kind: 'blocks'; root: Block; notice?: string }`; `tableLines({ columns, width, breakpoint, layouts })`; `rowLayout(item, layout, labelId)` pruned for every kind but `detail` (R14).

### Task 13a — E2E first: the stub team balance report, the v3 e2e spec, and `reports-wrap.spec.ts` changed to F10
- **F:** F1–F8, F10, F11 (e2e); F1/F2/F3/F5/F6 builder (e2e) · **Files:** `tests/scripts/reports-stub/server/register.go.txt`, `tests/e2e/reports-ledger-v3.spec.ts` (new), `tests/e2e/reports-wrap.spec.ts`.
- **Tests first** (`test.describe('general-ledger: report engine v3', …)`): 'an open group starts with its opening row, runs a balance from it and ends with its rollup'; 'a grouped report scrolled past its first page keeps its running balance'; 'a heading sorts the rows inside their groups, and a copied link keeps it'; 'a link that sorts by a column the report lacks offers Clear sort'; 'All dates shows every sale, says so in the header, and opens at zero'; 'on a desktop too narrow for its narrowed line a report wraps to its tablet arrangement inside the table' (1024: pins Q2's 944 between 864 and 1008); 'on a phone an opening row is its label and then its figures, and a heading still sorts'; 'the builder counts a lookup on this row, shows a formula on group and total rows, and starts each group with a row'. In `reports-wrap.spec.ts`: `:385-433` replaced by 'on a desktop a table wider than its view narrows its text columns and never scrolls sideways'; `:504-546` gets a `cost` column so it still wraps at 390 (Q3).
- **Red:** heading sort, phone sort: `report-sort-amount` not found (Task 15); Clear sort: `report-invalid` not found, the view does not send `s=` (Task 15); builder: `…-column-3-formula-0-1-part-outer` not found (Tasks 16, 16a, 18); wrap desktop: `outside` above 0, the `Sideways` scroller (Task 14). Green now, each with a mutation proof: opening row, paged balance, All dates, 1024, phone opening row part, phone blank slot.
- **Produces:** shipped report `salesstub.team_balance` "Stub team balance"; `openTeamBalance(page)`, `balanceOfOneTeam(page, prefix)`; the test ids Tasks 15, 16a and 18 must draw (`report-sort-<col>`, `report-clear-sort`, `<id>-outer`, `<id>-bound`, `<id>-pick-count`, `report-builder-column-N-rollup[-total|-custom|-formula]`, `report-builder-group-N-opening[-label|-cell-<col>-add|-cell-<col>-formula]`).

### Task 14 — F10/F5 view: the table without sideways scroll; the opening row
- **F:** F10 (view), F5 (view) · **Files:** `components/ReportTable.tsx`, `components/ReportRow.tsx`, `components/report-table-layout.ts`, `components/__tests__/report-table-layout.test.ts` (its `describe('reportTableLayout')` deleted). Tests run here: Task 13a's two e2e files.
- **Tests first:** none of its own (components only); Task 13a's 'on a desktop a table wider than its view narrows its text columns and never scrolls sideways' is red at its start.
- **Red → green:** that test turns green; the other tests of both files stay green; Step 6 runs the 1024 test as the check that Q2's 944 holds.
- **Produces:** `ReportTable` with no `Sideways`; `oneLineLabelStyle(layout, labelId)` (exported from `ReportRow.tsx`); `TableRowProps` without `oneLine` and `scrollX`; `OneLineRow`/`OneLineHeading` draw each column at `layout.widths[col]` with `numberOfLines={1}`; `FrozenLabel` deleted; an opening row's label in `text-muted-foreground`; `HeadingRowProps = { columns; layout; format }`.

### Task 15 — F8/F11 view: heading sort control, sort in the run and the model
- **F:** F8 (view), F11 (wiring) · **Files:** `lib/view-url.ts`, `lib/run-client.ts`, `hooks/useReportView.ts`, `components/ReportView.tsx`, `components/ReportTable.tsx`, `lib/__tests__/view-url.test.ts`, `lib/__tests__/run-client.test.ts`. `components/ReportStates.tsx` does not change (R22).
- **Tests first:** view-url.test `describe('nextSort')`: 'cycles a column ascending, descending, then off', 'starts another column ascending'; run-client.test `describe('refusalSortProblem')`: 'is the message of the first problem under options.sort', 'is null for any other refusal'. Task 13a's heading sort, Clear sort and phone tests are red at its start.
- **Red:** `nextSort is not a function`, `refusalSortProblem is not a function`; then (Step 8a) the three e2e turn green.
- **Produces:** `nextSort(current: ViewSort | null, col: string): ViewSort | null`; `refusalSortProblem(problems: readonly Problem[]): string | null`; `ReportViewModel.sort`, `.onSortColumn(col)`, `.onClearSort()`, `.sortProblem: string | null`; `ReportTableProps.sort`, `.onSortColumn`; run body `options.sort` = `[sort]` from the URL; test ids `report-sort-<col>` (label "Sort by <heading>"), `report-clear-sort` (in place of "Edit report").

### Task 16 — Builder formula parts (lib): Add up / Count, This row's field, Period start / end
- **F:** F1, F2, F3 (builder lib) · **Files:** `lib/builder/formula-parts.ts`, `formula-ops.ts`, `formula-type.ts`, `formula-describe.ts`, `formula-context.ts`, `lib/builder/__tests__/formula-fixture.ts`, `formula-ops.test.ts`, `formula-describe.test.ts`, `formula-type.test.ts`.
- **Tests first:** formula-ops.test `describe('the v3 lookup and leaf parts')`: 'picks a lookup match as Smallest, Largest, Add up or Count', "offers This row's field only inside a lookup that a report row reads", "edits This row's field inside a lookup and keeps one outside a lookup read-only", 'offers a start and an end for each date range param'; formula-type.test `describe('formulaType of the v3 parts')`: 'types a count as a number, a sum as its value, a bound as a date', "types This row's field from the report's source inside a lookup on another source"; formula-describe.test `describe('describeNode for the v3 parts')`: "says add up, count and this row's field", 'says a range start or end by its param'.
- **Red:** `LOOKUP_PICKS`, `boundChoices` undefined; `partOf({ outer })` is `'unknown'`; `partsAt` has no `'outer'`/`'bound'`; a count lookup types as `'relation'`; describe says "(smallest)" and "a part the editor cannot show".
- **Produces:** `type LookupPick`; `LOOKUP_PICKS` (Smallest, Largest, Add up, Count); parts `'outer'` ("This row's field") and `'bound'` ("Range start or end") in `REPORT_PARTS`; `boundChoices(params): { param; bound: 'from' | 'to'; label }[]` ("<param label> start" / "end"); `ReportFormulaContext.rowSource: string | null`; `FormulaTypes.rowSource?`.

### Task 16a — Builder formula leaves (UI): Add up / Count menu, This row's field, Range start or end
- **F:** F1, F2, F3 (builder UI) · **Files:** `components/builder/report-formula.tsx`, `components/builder/report-formula-paths.tsx`. Test run here: Task 13a's builder e2e.
- **Tests first:** none of its own (components only); the builder e2e is red at `report-builder-column-3-formula-0-1-outer` (no leaf draws `outer`).
- **Red → green:** the builder e2e passes the lookup and bound steps and then fails at `report-builder-column-2-rollup` (Task 18); `reports-formulas-v2.spec.ts` and `reports-formula-leaves.spec.ts` stay green.
- **Produces:** `OuterLeaf` (`<id>-outer`, a `ColumnPicker` of to-one paths); `BoundLeaf` (`<id>-bound`, a `PickMenu`, items `<param>-from` / `<param>-to`); lookup pick items `min|max|sum|count` under `<id>-pick`.

### Task 17 — Builder draft: v3 round-trip, opening and rollup drafts
- **F:** F9 (builder), F5, F6 (draft) · **Files:** `lib/builder/draft.ts`, `lib/builder/__tests__/draft.test.ts`.
- **Tests first:** `describe('spec version 3 in the builder')`: 'keeps every v3 part through the draft and writes version 3', 'keeps a rollup or an opening cell the editor cannot show read-only and saves it unchanged', '"Start each group with a row" writes the opening with its label and only the cells set', '"A formula" on group and total rows writes the column rollup, and the total takes it out', 'flags a rollup with totals of the rows shown, an empty opening label and an opening cell on the first column', 'keeps a param that only a rollup, an opening cell or a range bound reads'.
- **Red:** `openingDraft is not a function`; the round trip loses `rollup` and `opening`; `keptParams(byRollup)` is `[]`.
- **Produces:** `openingDraftSchema`; `type OpeningDraft = { label: string; cells: Record<string, FormulaDraft> }`; `ColumnDraft.rollup?: FormulaDraft | null`; `GroupLevelDraft.opening?: OpeningDraft | null`; `openingDraft(label)`; `setOpeningCell(opening, col, formula)`; `draftIssues` problems at `columns.N.rollup` and `groups.N.opening[.cells.<col>]`.

### Task 18 — Builder UI: Group and sort opening row, Columns rollup, Arrange desktop
- **F:** F5, F6, F10 (builder UI) · **Files:** `components/builder/GroupLevelParts.tsx`, `GroupSortSection.tsx`, `ColumnsSection.tsx`, `ArrangeSection.tsx`, `lib/builder/__tests__/draft.test.ts`. Test run here: Task 13a's builder e2e.
- **Tests first:** draft.test `describe('the desktop arrangement in the builder')`: 'starts a desktop layout that places every column', 'keeps a desktop layout through the draft as it keeps a tablet one', 'names a desktop problem at its layout as it does for a tablet' (green at once, R18f: a mutation proof in `displayIssues`). The builder e2e is red at `report-builder-column-2-rollup`.
- **Red → green:** the builder e2e passes to its end (the opening row "Brought forward" and the rollup cell in the view). Not driven by e2e: the opening cell's Remove and the Arrange caption.
- **Produces:** `GroupOpening({ builder, groups, index })` (`report-builder-group-N-opening`, `-opening-label`, `-opening-cell-<col>` with `-add`, `-remove`, `-formula`); `OpeningCell`, `TextAction`; `ColumnRollup` (`report-builder-column-N-rollup`, items `-total`, `-custom`, editor `-rollup-formula`); Arrange caption `report-builder-arrange-auto`.

### Task 19 — Help and package docs
- **F:** F1–F11 (help/docs) · **Files:** `core/help/reports.md`, `docs/packages.md`.
- **Tests first:** none (docs); Step 3 runs `pnpm run packages:generate` from `$WS/tinycld` and `tinycld-pkg check` (the help topic's frontmatter test).
- **Produces:** help sections "Opening rows", "Sort a report", "When columns do not fit"; docs "Spec version 3".

### Task 20 — Run the v3 e2e spec with every task in
- **F:** F1–F8, F10, F11 (e2e run) · **Files:** none (a defect is fixed in its owning task's file).
- **Run:** `tests/e2e/reports-ledger-v3.spec.ts` (written in Task 13a), expected PASS, 8 tests.
- **Produces:** nothing new.

### Task 21 — Re-run the existing report e2e under F10
- **F:** F10 (e2e run) · **Files:** none (Task 13a computed every phone table of these files; none asserts a wrap that F10 now fits; a failure is fixed in its owning task's file).
- **Run, one file at a time:** `reports-wrap.spec.ts` (changed in Task 13a); then `reports-view`, `reports-source-params`, `reports-scroll-anchor`, `reports-groups-v2`, `reports-filter`, `reports-create`, `reports-formula-layout`, `reports-formulas-v2`, `reports-group-pivot`, `reports-share` (`.spec.ts`), each expected PASS unchanged.
- **Produces:** nothing new.

### Task 22 — LAST: a fictional ledger on `zoo_*` with the General Ledger's spec shape (Go)
- **F:** F1–F8 together (engine acceptance) · **Files:** `balance_shape_test.go` (new).
- **Tests first:** `TestABalanceShapedSpecLoadsAndValidates`; `TestABalanceShapedReportRunsBalancesByNormalSide`; `TestABalanceShapedReportCarriesTheBalanceIntoALaterPeriod`; `TestABalanceShapedReportUnderAllDatesBeginsAtZero`; `TestABalanceShapedReportShowsQuietActiveGroupsOnAll`; `TestABalanceShapedReportOverTwoThousandLinesRunsInTime` (R24 cost 2: under 4 s) — over fictional `zoo_ledger_accounts`, `zoo_ledger_entries`, `zoo_ledger_lines` (debit and credit money with a per-row `currency` field, R24) and a spec of §5's shape (opening sum lookup with the retained-earnings fold, `bound from`, `byNormal`, `runningSum`, `rollup` with `coalesce`, the opening row, `having`, `fill`, the Split column with `count` and `min` lookups and `outer`, `detail.label`, the chart sort).
- **Red:** none expected once Tasks 1–8 and 2b hold; a failure is an engine defect, fixed in the owning task's file.
- **Produces:** nothing new.

---

## Coverage check

### Spec §4, every cell

| F | Engine (Go) | Spec schema | Client lib / view | Builder UI | Help / docs | E2E (written in 13a; run green in the task named) |
|---|---|---|---|---|---|---|
| F1 | 2, 2b (money sums combine, R24), 9 | 1 (Go), 10 (TS) | 10 (`lib/spec.ts`) | 16 (lib), 16a (UI) | 19 | builder "Count" (18) |
| F2 | 3 | 1, 10 | 10 | 16, 16a | 19 | builder "This row's field" (18) |
| F3 | 4 | 1, 10 | 10 | 16, 16a | 19 | builder "Day start" (16a) |
| F4 | 4 | 4 (preset list), 11 | 11 (`params.ts` presets, `periodWords`; `ParamPeriodControl` pills follow `presetsFor`) | — (spec: none) | 19 | All dates (13a) |
| F5 | 6 (money cells via 2b) | 1, 10 | 10 (row kind, `cellType`), 12 (`rows.ts` merge), 13 (wrap layout), 14 (`ReportRow`) | 17, 18 | 19 | opening row (13a); phone opening row (15); builder opening row (18) |
| F6 | 5 (money via 2b) | 1, 10 | 10 | 17, 18 | 19 | rollup (13a); builder rollup (18) |
| F7 | 8, 9 | — (spec: none) | 12 (`rows.ts` `mergePages`), 14 (`onEndReached` appends sections) | — | 19 | paged balance (13a) |
| F8 | 7, 8 (cursor) | — (run option: 1 Go, 10 TS) | 11 (`view-url.ts`), 15 (`nextSort`, `refusalSortProblem`, heading control, Clear sort) | — | 19 | heading sort, Clear sort (15) |
| F9 | 1 (+ `TestV3PartsAreValidated` rows in 2, 3) | 10 | — | 17 | 19 | — |
| F10 | — | — | 13 (`fitOneLine`, `tableLines`), 14 (`ReportTable`, `ReportRow`) | 18 (Desktop arrangement) | 19 | 1024 tablet arrangement (13a); wrap 1100 narrowed line (14) |
| F11 | — | — | 12 (`rowsKeyOf`), 15 (wiring) | — | — (spec: none) | heading sort (15) |
| carried: figure-only rows collapse | — | — | 13, 14 | — | 19 | phone opening row (15) |

### Tests it must add

| Test (brief) | Task |
|---|---|
| Go F1 sum/count; money keeps currency; two currencies → no value + `mixedCurrency`; empty → no value for sum, 0 for count | 2 |
| Go F2 outer correlates (two rows, two answers); refused outside a lookup and on a to-many path | 3 |
| Go F3 bound from/to on a closed range; no value on an open one | 4 |
| Go F4 allDates matches every row; resolves with no bounds | 4 |
| Go F5 opening rows on open, closed (none), fill, having-hidden (none) groups; cell reads a record leaf, a lookup, a param | 6 |
| Go F6 rollup on group, subtotal, total; record leaf blank on the Total; refused with `total: "rows"` | 5 |
| Go F7 pages across a group boundary; running sum continues on page 2; 5,000 cap gone (6,000 rows to the end); cursor refused after a spec change and a sort change | 8 |
| Go F8 detail order inside groups follows `options.sort`; group order unchanged; running-value column refused; running values unchanged | 7 |
| Go F7 `options.limit` > 1,000 held at 1,000; > 5,000 group rows stop at the group cap with its notice | 8 |
| Go F9 v2 loads as v3 (`UpgradedFrom` 0); version 4 refused; `Validate` checks every new field | 1 (the table; its rows in 2, 3; the other refusals in each task's own test file: 4, 5, 6, 7, 8) |
| Go shared formula table: lookup sum/count, outer, bound rows | 2, 3, 4 |
| Go rule-parity fuzz: sum lookups and paged grouped reports | 9 |
| Go last task: fictional ledger on `zoo_*` with §5's shape and the brief's numbers | 22 |
| Vitest `lib/spec.ts` v3 zod (new fields optional; v2 parses) | 10 |
| Vitest `builder/draft.ts` round-trip keeps every v3 field; unknown parts read-only | 17 |
| Vitest grouped paging in `run-client.ts` (appends sections; no duplicate opening/subtotal at a seam) — `mergePages` (`rows.ts`) is where pages merge (R10) | 12 |
| Vitest `view-url.ts` keeps `s=<col>:<dir>` | 11 |
| Vitest `rowsKeyOf` with and without `options.sort` | 12 |
| Vitest `tableLines`: fits; fits after narrowing; wraps to authored desktop; no desktop → tablet; no tablet → `autoLines` | 13 |
| Vitest a figure-only row in a wrap layout has no empty text line | 13 |
| Builder vitest: "Add up" / "Count" (F1) | 16 |
| Builder vitest: "This row's <field>" only inside a lookup (F2) | 16 |
| Builder vitest: "<param> start" / "<param> end" for a date-range param (F3) | 16 |
| Builder vitest: Group and sort "Start each group with a row" writes `groups[].opening` (F5) | 17 |
| Builder vitest: Columns "On group and total rows show …" writes `columns[].rollup` (F6) | 17 |
| Builder vitest: the layout editor edits the desktop width like tablet and mobile (F10) | 18 |
| E2E opening row first under an open group, with the rollup cell on its subtotal ('an open group starts with its opening row, …') | 13a |
| E2E grouped report scrolled past page 1, running value continuing | 13a |
| E2E heading sort (asc → desc → spec order; copied link keeps `s=`; no control on a running value; `role="button"`, "Sort by <column>") | 13a |
| E2E a link that sorts by a missing column offers Clear sort | 13a |
| E2E All dates in the header | 13a |
| E2E desktop too narrow for its narrowed line: wraps to its tablet arrangement inside the table, no sideways scroll (`reports-ledger-v3.spec.ts`, 1024) | 13a |
| E2E desktop wider than its view: one narrowed line, every heading inside the table, no sideways scroll (`reports-wrap.spec.ts`, 1100; replaces `:385`) | 13a |
| E2E phone: opening row is label line then figures, no empty lines; a heading still sorts | 13a |
| E2E builder: This row's field, Count, Range start, a rollup formula, "Start each group with a row" with label and cell, saved and seen in the view | 13a |
| Existing e2e `reports-wrap.spec.ts:504` gets a `cost` column so it wraps at 390 under either answer to Q3 | 13a |
| Existing e2e `reports-wrap.spec.ts` (whole file) re-run | 21 |
| Existing e2e `reports-view.spec.ts:111` (R18e: not a table-scroll test) and the other report specs re-run | 21 |
| `reports-source-params.spec.ts:277` re-run unchanged | 21 |

Tests the plan adds beyond the brief's list: Go R24 (`fieldcurrency_test.go`, `TestARollupAddsALookedUpAmountOfItsColumnsField`, `TestAnOpeningCellAddsUpItsColumnsAmounts`, `TestAnOpeningSumOverTwoCurrenciesIsBlankWithANotice`) — 2b, 5, 6; `TestHiddenRowsTakeNoPlaceOnAPage` — 8; vitest `nextSort`, `refusalSortProblem` — 15; vitest `cellType` on an opening row — 10; vitest `fitOneLine`, `isFigureColumn` — 13; Go `TestABalanceShapedReportOverTwoThousandLinesRunsInTime` (R24 cost 2) — 22. Each test is written in exactly one task; Tasks 14, 15, 16a, 18, 20 and 21 run e2e that Task 13a wrote.

## Tasks

### Task 1 — F9: spec version 3 and the new fields (Go)

Engine facts used: `CurrentVersion` and `upgradeSteps` (`spec.go:15-29`); `Column` (`spec.go:94-117`), `Node` (`spec.go:132-151`), `Group` (`spec.go:161-180`); `loadWith` gives `UpgradedFrom` only when a step changed the document (`spec.go:315-333`, `sameExceptVersion` `:378-392`); `upgrade` refuses a newer version (`spec.go:356-358`); `resolveAliases` (`alias.go:10-71`), `aliaser.node` (`alias.go:144-163`), `nodeTarget` (`alias.go:198-226`); the lookup decoder walk `binder.lookups` (`lookup.go:46-84`); `formulaSizes` (`bind.go:399-433`) and `nodeSize` (`bind.go:454-482`); `Options` (`run.go:20-37`), row kinds (`run.go:108-114`); test helpers `withZooRegistry`, `requireSpecError` (`spec_test.go:24-55`), `loadSpec` (`bind_test.go:11`), `mustLoad`, `jsonDoc` (`v2_test.go:110-134`), `loadStored` (`saved.go:123`).

**Files:**
- Modify: `core/server/reports/spec.go` (`CurrentVersion`, step 2, `Column.Rollup`, `Node.Outer`, `Node.Bound`, `Group.Opening`, `Opening`, `Opening.cellIDs`)
- Modify: `core/server/reports/run.go:20-37,108-114` (`Options.Sort`, `ViewSort`, `KindOpening`; types only)
- Modify: `core/server/reports/alias.go` (aliases in `outer`, `rollup`, `opening.cells`)
- Modify: `core/server/reports/lookup.go:46-84` (the decoder walk covers `rollup` and `opening.cells`)
- Modify: `core/server/reports/bind.go:399-433` (`formulaSizes` covers `rollup` and `opening.cells`)
- Create: `core/server/reports/v3_test.go`
- Modify: `core/server/reports/v2_test.go:12-55,83-108`, `core/server/reports/spec_test.go:107`, `core/server/reports/saved_test.go:65`

**Interfaces:**
- Consumes: nothing new.
- Produces: `const CurrentVersion = 3`; `upgradeSteps[2]` (identity); `Column.Rollup *Node` (JSON `rollup`); `Node.Outer Path` (JSON `outer`); `Node.Bound string` (JSON `bound`); `Group.Opening *Opening` (JSON `opening`); `type Opening struct { Label string \`json:"label"\`; Cells map[string]*Node \`json:"cells,omitempty"\` }`; `func (o *Opening) cellIDs() []string` (sorted column ids); `Options.Sort []ViewSort` (JSON `sort`); `type ViewSort struct { Col string \`json:"col"\`; Dir string \`json:"dir,omitempty"\` }`; `const KindOpening = "opening"`; in `v3_test.go`: `const everyV3`, `type v3Refusal struct { spec string; status int; at, want string }`, `requireV3Refusal(t, app, c)`, and the table `TestV3PartsAreValidated`, whose `cases` map Tasks 2 and 3 add rows to (Tasks 4–8 put their refusal tests in their own test files).

- [ ] **Step 0: Preflight — the branch and a green baseline**

Before any change, check the branch and that the suites are green:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git branch --show-current
git merge-base --is-ancestor aa16a677 HEAD && echo based-on-aa16a677
git status --short
(cd core/server && go test -tags no_ui -count=1 ./reports/)
pnpm exec tinycld-pkg check
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts
```
Expected: `books/p5-general-ledger`; `based-on-aa16a677`; `git status --short` lists nothing, or only this plan's file under `docs/plans/`; `ok  	tinycld.org/core/reports`; biome, tsc and vitest pass (exit 0); the isolation check reports 0 references. If the branch is another one, stop and report it (never switch branches). If a suite or the isolation check fails, find the root cause and fix it at its source before Step 1 (no re-runs, no skipped tests, no bumped timeouts), and append the failure and its fix to `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/retro-log.md` in one line. A fix outside `core/` or `tests/` is out of scope: stop and report it. This step changes no file.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/v3_test.go`:

```go
package reports

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// everyV3 uses every part version 3 adds (the plan's "For review" spec): a
// count lookup with outer leaves, a sum lookup with a range bound, the All
// dates preset, an opening row and a rollup.
const everyV3 = `{"version":3,"source":"zoo.feedings",
	"params":[{"id":"period","type":"dateRange","label":"Period","default":{"preset":"allDates"}}],
	"columns":[
		{"id":"animal","label":"Animal","path":[{"link":"animal"}]},
		{"id":"fed","label":"Fed","path":[{"field":"fed"}]},
		{"id":"cost","label":"Cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"sameDay","label":"Same day","formula":{"op":"lookup","source":"zoo.feedings",
			"where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"link":"animal"}]}]},
				{"op":"eq","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]}]},
			"value":{"path":[{"field":"id"}]},"pick":"count"}},
		{"id":"balance","label":"Balance","formula":{"op":"add","args":[
			{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings",
				"where":{"op":"and","args":[
					{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"byAnimal","path":[{"field":"id"}]}]},
					{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]}]},
				"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]},
			{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]},
			"rollup":{"op":"add","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]}}],
	"filter":{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]},
	"groups":[{"id":"byAnimal","col":"animal","open":true,"subtotals":true,
		"opening":{"label":"Before the period","cells":{"balance":{"op":"coalesce","args":[
			{"op":"lookup","source":"zoo.feedings","where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"byAnimal","path":[{"field":"id"}]}]},
				{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]}]},
			"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]}}}}],
	"sort":[{"col":"fed"}]}`

// A version 2 spec loads unchanged as version 3: the step is the identity,
// so the run says nothing about an upgrade and a save stores version 3.
func TestAV2SpecLoadsSilentlyAsVersion3(t *testing.T) {
	z := newZoo(t)
	raw := `{"version":2,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],"groups":[{"id":"g","col":"animal","open":true}]}`
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != 3 || info.UpgradedFrom != 0 {
		t.Fatalf("version %d, upgradedFrom %d", spec.Version, info.UpgradedFrom)
	}
	res := z.run(t, "ann", raw, Options{}, nil)
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	_, stored, err := loadStored([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(stored, &doc); err != nil || doc["version"] != float64(3) {
		t.Fatalf("stored = %s", stored)
	}
}

func TestAVersion4SpecIsRefused(t *testing.T) {
	_, _, err := Load([]byte(`{"version":4,"source":"zoo.feedings","columns":[]}`))
	requireSpecError(t, err, 400, "version", "version 4 is newer than this server reads (3)")
}

// Every v3 field decodes strictly and is stored as posted; a run's options
// carry the viewer sort.
func TestASpecWithEveryV3FieldLoadsAndIsStoredUnchanged(t *testing.T) {
	withZooRegistry(t)
	spec, info, stored := mustLoad(t, everyV3)
	if info.UpgradedFrom != 0 || len(info.Aliased) != 0 {
		t.Fatalf("info = %+v", info)
	}
	if !reflect.DeepEqual(jsonDoc(t, []byte(everyV3)), jsonDoc(t, stored)) {
		t.Fatalf("stored spec differs from the posted one:\n%s", stored)
	}
	for _, key := range []string{`"outer":[`, `"bound":"from"`, `"pick":"sum"`, `"pick":"count"`, `"opening":{`, `"rollup":{`, `"preset":"allDates"`} {
		if !strings.Contains(string(stored), key) {
			t.Fatalf("stored spec lost %s: %s", key, stored)
		}
	}
	if o := spec.Groups[0].Opening; o == nil || o.Label != "Before the period" || o.Cells["balance"] == nil {
		t.Fatalf("opening = %+v", o)
	}
	if spec.Columns[4].Rollup == nil || spec.Columns[3].Formula.Where.Args[0].Args[1].Outer[0].Link != "animal" {
		t.Fatalf("columns = %+v", spec.Columns)
	}
	var opts Options
	if err := json.Unmarshal([]byte(`{"sort":[{"col":"cost","dir":"desc"}]}`), &opts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(opts.Sort, []ViewSort{{Col: "cost", Dir: "desc"}}) {
		t.Fatalf("sort = %+v", opts.Sort)
	}
	if KindOpening != "opening" {
		t.Fatalf("KindOpening = %q", KindOpening)
	}
}

// An aliased key in an outer path, a rollup or an opening cell is replaced
// with its current key, and the formula-size guard counts the new slots.
func TestV3SlotsResolveAliasesAndCountSize(t *testing.T) {
	app := withZooRegistry(t)
	spec, info, _ := mustLoad(t, `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"heavier","formula":{"op":"lookup","source":"zoo.animals","pick":"count",
			"where":{"op":"gt","args":[{"path":[{"field":"mass"}]},{"outer":[{"link":"animal"},{"field":"mass"}]}]},
			"value":{"path":[{"field":"name"}]}}},
		{"id":"weight","formula":{"value":null},
			"rollup":{"op":"max","args":[{"path":[{"link":"animal"},{"field":"mass"}]}]}}],
		"groups":[{"id":"g","col":"animal","opening":{"label":"Before","cells":{"weight":{"record":"g","path":[{"field":"mass"}]}}}}]}`)
	for name, got := range map[string]string{
		"lookup where": spec.Columns[1].Formula.Where.Args[0].Path[0].Field,
		"outer":        spec.Columns[1].Formula.Where.Args[1].Outer[1].Field,
		"rollup":       spec.Columns[2].Rollup.Args[0].Path[1].Field,
		"opening cell": spec.Groups[0].Opening.Cells["weight"].Path[0].Field,
	} {
		if got != "weight" {
			t.Errorf("%s = %q, want weight", name, got)
		}
	}
	if len(info.Aliased) != 4 {
		t.Fatalf("aliased %d keys: %+v", len(info.Aliased), info.Aliased)
	}

	big := `{"op":"add","args":[` + strings.TrimSuffix(strings.Repeat(`{"value":1},`, 1000), ",") + `]}`
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","rollup":`+big+`}],
		"groups":[{"id":"g","col":"animal","opening":{"label":"Before","cells":{"cost":`+big+`}}}]}`))
	requireSpecError(t, err, 400, "columns.1.rollup", "at most 32 levels and 1000 nodes")
	requireSpecError(t, err, 400, "groups.0.opening.cells.cost", "at most 32 levels and 1000 nodes")
}

// v3Refusal is one spec a v3 part refuses: the status, the part's path and
// a piece of the message.
type v3Refusal struct {
	spec   string
	status int
	at     string
	want   string
}

// requireV3Refusal loads the spec and, when it loads, validates it: either
// step may refuse it.
func requireV3Refusal(t *testing.T, app core.App, c v3Refusal) {
	t.Helper()
	spec, _, err := Load([]byte(c.spec))
	if err == nil {
		err = Validate(app, spec)
	}
	requireSpecError(t, err, c.status, c.at, c.want)
}

// Every v3 part is checked where it is: Tasks 2–8 add their rows to cases.
func TestV3PartsAreValidated(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]v3Refusal{
		// F9: every new part decodes strictly.
		"an unknown key in an opening": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]}],
			"groups":[{"id":"g","col":"a","opening":{"label":"x","lable":"y"}}]}`, 400, "spec", `unknown field "lable"`},
		"an unknown key in a rollup": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"c","path":[{"field":"cost"}],"aggregate":"sum",
			"rollup":{"op":"sum","sql":"x"}}]}`, 400, "spec", `unknown field "sql"`},
		"a bound that is not text": {`{"version":3,"source":"zoo.feedings","params":[{"id":"p","type":"dateRange"}],
			"columns":[{"id":"x","formula":{"param":"p","bound":1}}]}`, 400, "spec", "cannot unmarshal"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { requireV3Refusal(t, app, c) })
	}
}
```

In `core/server/reports/v2_test.go`, delete `TestAV1SpecLoadsSilentlyAsVersion2` (lines 12-36) and `TestAVersion3SpecIsRefused` (lines 52-55): `v3_test.go` replaces them. In `TestASpecWithEveryV2PartIsKeptWhole`, a v2 spec is now stored as version 3, so replace:

```go
		if !reflect.DeepEqual(jsonDoc(t, []byte(c.raw)), jsonDoc(t, stored)) {
			t.Fatalf("stored spec differs from the posted one:\n%s", stored)
		}
```

with:

```go
		// Stored as version 3, every other part as posted.
		want := jsonDoc(t, []byte(c.raw)).(map[string]any)
		want["version"] = json.Number("3")
		if !reflect.DeepEqual(want, jsonDoc(t, stored)) {
			t.Fatalf("stored spec differs from the posted one:\n%s", stored)
		}
```

In `core/server/reports/spec_test.go:107`, replace the "newer version" case with:

```go
		"newer version":   {`{"version":4,"columns":[]}`, "version", "newer than this server reads (3)"},
```

In `core/server/reports/saved_test.go:65`, replace `{"version":3}` in the posted body with `{"version":4}`:

```go
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":4}}`, ann),
		http.StatusBadRequest, `"version":{"code":"invalid"`)
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'V3|V2|Version|LoadRefuses|ASpecIsCheckedAndSavedLoaded' ./reports/`
Expected: FAIL to build: `undefined: ViewSort`, `undefined: KindOpening`, `spec.Groups[0].Opening undefined`, `spec.Columns[4].Rollup undefined`, `...Outer undefined` (the fields and types do not exist; with them, Load would refuse `"version":3` as "newer than this server reads (2)").

- [ ] **Step 3: Implement the spec format**

In `spec.go`, add `"maps"` and `"slices"` to the imports, then replace lines 15-29 with:

```go
// CurrentVersion is the spec version this engine writes. Load upgrades an
// older spec one version at a time with upgradeSteps.
const CurrentVersion = 3

// upgradeSteps[v] turns a version v document into version v+1. upgrade sets
// the version, not each step. Add a step whenever the spec format changes,
// and raise CurrentVersion with it.
//
// 1 → 2 adds only optional parts (column totals, group having, labels, fill
// and open, formula sort keys, the detail label, options from a source, the
// record and lookup formulas, display layouts), so a version 1 document is
// already a valid version 2 document.
//
// 2 → 3 adds only optional parts too (sum and count lookups, outer leaves,
// date range bounds, the All dates preset, opening rows, rollups), so a
// version 2 document is already a valid version 3 document.
var upgradeSteps = map[int]upgradeStep{
	1: func(map[string]any) error { return nil },
	2: func(map[string]any) error { return nil },
}
```

In `Column`, after `Detail string ...`:

```go
	// Rollup is the column's cell on group, subtotal and total rows in place
	// of its total: a formula on the row's level. Detail rows keep Path or
	// Formula.
	Rollup *Node `json:"rollup,omitempty"`
```

In `Node`, replace the `Source`/`Pick` comment and add after `Pick`:

```go
	// Source and Pick belong to lookup: the source it reads, and "min" (the
	// default), "max", "sum" or "count" of the matching rows' value. A
	// lookup's Value is a formula node over that source, not a literal.
	Source string `json:"source,omitempty"`
	Pick   string `json:"pick,omitempty"`
	// Outer is a path on the report's row, read from inside a lookup: the
	// row the outermost lookup sits on. It correlates the lookup.
	Outer Path `json:"outer,omitempty"`
	// Bound reads a date range param's first ("from") or last ("to") day.
	Bound string `json:"bound,omitempty"`
```

In `Group`, after `Having`:

```go
	// Opening is a row shown first under each open group of this level.
	Opening *Opening `json:"opening,omitempty"`
```

After `type Fill`:

```go
// Opening is a group level's opening row: its label, and the cells it shows
// by column id, each a formula on the group's level. A column it does not
// name is blank on the row.
type Opening struct {
	Label string           `json:"label"`
	Cells map[string]*Node `json:"cells,omitempty"`
}

// cellIDs are the opening's cell column ids in order, so every walk over its
// cells reports problems in one order.
func (o *Opening) cellIDs() []string { return slices.Sorted(maps.Keys(o.Cells)) }
```

In `run.go`, add to `Options` after `Limit`:

```go
	// Sort orders the detail rows before the report's own sort keys: the
	// viewer's heading sort.
	Sort []ViewSort `json:"sort,omitempty"`
```

After `Options`:

```go
// ViewSort is one key of a run's viewer sort: a column, and "asc" (the
// default) or "desc".
type ViewSort struct {
	Col string `json:"col"`
	Dir string `json:"dir,omitempty"`
}
```

In the row kinds (`run.go:108-114`):

```go
const (
	KindDetail   = "detail"
	KindGroup    = "group"
	KindOpening  = "opening"
	KindSubtotal = "subtotal"
	KindComputed = "computed"
	KindTotal    = "total"
)
```

- [ ] **Step 4: Walk the new formula slots**

In `alias.go` `resolveAliases`, replace the formula loop and the group loop (lines 27-39) with:

```go
	for i := range spec.Columns {
		a.node(base, spec.Columns[i].Formula)
		a.node(base, spec.Columns[i].Rollup)
	}
	a.node(base, spec.Filter)
	for i := range spec.Groups {
		g := &spec.Groups[i]
		a.node(base, g.Label)
		a.node(base, g.Having)
		if g.Fill != nil {
			// A fill's where reads the records the group links to.
			a.node(a.targets[g.ID], g.Fill.Where)
		}
		if g.Opening != nil {
			for _, id := range g.Opening.cellIDs() {
				a.node(base, g.Opening.Cells[id])
			}
		}
	}
```

In `aliaser.node`, after `if n == nil { return }`:

```go
	// An outer path is on the report's row, whatever source the formula
	// around it reads.
	a.path(a.base, n.Outer)
```

In `aliaser.nodeTarget`, add a case before `case n.Path != nil:`:

```go
	case n.Outer != nil:
		return pathTarget(a.base, n.Outer)
```

In `lookup.go` `binder.lookups`, replace the column loop and the group loop with:

```go
	for i, c := range p.spec.Columns {
		walk(c.Formula, fmt.Sprintf("columns.%d.formula", i))
		walk(c.Rollup, fmt.Sprintf("columns.%d.rollup", i))
	}
	walk(p.spec.Filter, "filter")
	for i, g := range p.spec.Groups {
		walk(g.Label, fmt.Sprintf("groups.%d.label", i))
		if g.Fill != nil {
			walk(g.Fill.Where, fmt.Sprintf("groups.%d.fill.where", i))
		}
		walk(g.Having, fmt.Sprintf("groups.%d.having", i))
		if g.Opening != nil {
			for _, id := range g.Opening.cellIDs() {
				walk(g.Opening.Cells[id], fmt.Sprintf("groups.%d.opening.cells.%s", i, id))
			}
		}
	}
```

In `bind.go` `formulaSizes`, replace the column loop and the group loop with:

```go
	for _, cp := range p.cols {
		if cp.col.Formula != nil {
			check(cp.at, p.columnSize(cp, memo, map[string]bool{}))
		}
		if r := cp.col.Rollup; r != nil {
			check(cp.at+".rollup", p.nodeSize(r, memo, map[string]bool{}, true))
		}
	}
	if f := p.spec.Filter; f != nil {
		check("filter", p.nodeSize(f, memo, map[string]bool{}, true))
	}
	for i := range p.spec.Groups {
		if l := p.spec.Groups[i].Label; l != nil {
			check(fmt.Sprintf("groups.%d.label", i), p.nodeSize(l, memo, map[string]bool{}, true))
		}
		if f := p.spec.Groups[i].Fill; f != nil && f.Where != nil {
			check(fmt.Sprintf("groups.%d.fill.where", i), p.nodeSize(f.Where, memo, map[string]bool{}, true))
		}
		if h := p.spec.Groups[i].Having; h != nil {
			check(fmt.Sprintf("groups.%d.having", i), p.nodeSize(h, memo, map[string]bool{}, true))
		}
		if o := p.spec.Groups[i].Opening; o != nil {
			for _, id := range o.cellIDs() {
				if n := o.Cells[id]; n != nil {
					check(fmt.Sprintf("groups.%d.opening.cells.%s", i, id), p.nodeSize(n, memo, map[string]bool{}, true))
				}
			}
		}
	}
```

- [ ] **Step 5: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'V3|V2|Version|LoadRefuses|ASpecIsCheckedAndSavedLoaded' ./reports/`
Expected: PASS.

- [ ] **Step 6: Run the whole reports suite and gofmt**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go test -tags no_ui -count=1 ./reports/`
Expected: `gofmt -l` prints nothing; `ok  	tinycld.org/core/reports`.

- [ ] **Step 7: Hand over for commit**

The driver commits:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/spec.go core/server/reports/run.go core/server/reports/alias.go core/server/reports/lookup.go core/server/reports/bind.go core/server/reports/v3_test.go core/server/reports/v2_test.go core/server/reports/spec_test.go core/server/reports/saved_test.go
git commit -m "core(reports): spec version 3 with outer, bound, opening, rollup and the viewer sort fields"
```

---

### Task 2 — F1: `sum` and `count` lookups (Go)

Engine facts used: `lookupIn` refuses a pick other than min/max (`lookup.go:102`), compiles `(SELECT min|max(v) FROM … WHERE …)` (`lookup.go:136-145`), and reads a carried amount through `pickedAmount` (`lookup.go:165-184`), whose currency is the lookup's registered key on a base row inside a total (`lookup.go:180`, `valueCurrencyKey` `rows.go:301-319`); `expr` (`formula.go:14-45`) and the arg loop in `compileNode` (`formula.go:298-314`); a column cell's helper cells in the group statement (`run.go:498-511`), the detail select (`run.go:810-838`) and the layout's computed legs (`layout.go:74-88`); `unreadHelpers` drops a helper cell NULL in every leg (`run.go:584-609`); `markCurrencies` writes the totals' `mixedCurrency` notice (`assemble.go:92-118`) and `run` calls it (`run.go:333`); `recompute` blanks a formula that reads a row value (`levels.go:757-760`); the formula-table harness's own lookup (`formula_cases_test.go:144-167`) and `TestFormulaCasesThroughRun`'s count of one-row cases (`formula_run_test.go:227`).

**Files:**
- Modify: `core/server/reports/lookup.go` (`lookupPicks`, pick check, `count`, `summedLookup`)
- Modify: `core/server/reports/formula.go` (`expr.mixed`, `orSQL`, the arg loop)
- Modify: `core/server/reports/run.go` (`mixedCell`, `noMixedCell`, `mixedHelpers`, `helperCell`, the cells of `groupStatement` and `detailSelect`, the call in `run`)
- Modify: `core/server/reports/layout.go:74-88` (computed legs carry `vmx<idx>`)
- Modify: `core/server/reports/levels.go:757-760` (a blanked cell has no mixed check)
- Modify: `core/server/reports/assemble.go` (`markMixedLookups`)
- Modify: `core/server/reports/lookup_test.go` (new tests; the "a bad pick" case's message)
- Modify: `core/server/reports/formula_cases_test.go:144-167` (the harness picks sum and count)
- Modify: `core/server/reports/formula_run_test.go:227` (one-row case count)
- Modify: `core/lib/forms/fixtures/formula-cases.json` (four cases)
- Modify: `core/server/reports/v3_test.go` (rows)

**Interfaces:**
- Consumes: `Node.Pick` (Task 1).
- Produces: pick values `"sum"`, `"count"`; `var lookupPicks = []string{"min", "max", "sum", "count"}`; `expr.mixed string` (SQL that is true where a sum lookup met two currencies; `""` when it cannot); `orSQL(a, b string) string`; `summedLookup(outer fctx, r *rowsCTE, n *Node, v expr, sc *scope, where string, out expr) expr`; `mixedCell(e expr, idx int) string` (`… AS vmx<idx>`), `noMixedCell(idx int) string`; `mixedHelpers(idx int) []string` (the helper cell names whose 1 marks column `idx` mixed: `vmx<idx>`; Task 6 adds its opening cell's helper name here and to `helperCell`); `(rn *runner) markMixedLookups(rowSets ...[]map[string]any)`; notice message `"A looked-up total mixes currencies, so it shows no value."`.

- [ ] **Step 1: Write the failing Go tests**

Append to `core/server/reports/lookup_test.go` (add `"reflect"` to its imports):

```go
// sitesWithSums sums animals per site (a number), the fees of every animal
// but lions (an amount whose currency each row carries), and every price
// (an amount in the source's fixed currency).
const sitesWithSums = `{"version":3,"source":"zoo.sites","columns":[
	{"id":"name","path":[{"field":"name"}]},
	{"id":"site","path":[{"field":"id"}]},
	{"id":"weight","formula":{"op":"lookup","source":"zoo.animals","pick":"sum",
		"where":{"op":"eq","args":[{"path":[{"link":"site"}]},{"col":"site"}]},
		"value":{"path":[{"field":"weight"}]}}},
	{"id":"usdFees","formula":{"op":"lookup","source":"zoo.animals","pick":"sum",
		"where":{"op":"ne","args":[{"path":[{"field":"species"}]},{"value":"lion"}]},
		"value":{"path":[{"field":"fee"}]}}},
	{"id":"prices","formula":{"op":"lookup","source":"zoo.animals","pick":"sum","value":{"path":[{"field":"price"}]}}}],
	"sort":[{"col":"name"}]}`

// ann may list leo (north, 700 EUR), zed (south, 300 USD) and kit (no site,
// 50 USD); mo (north) is hidden from her.
func TestASumLookupAddsTheMatchingRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", sitesWithSums, Options{}, nil)
	requireOrderedRows(t, sitesWithSums, res, "label", "North", "South", "Total")
	// Correlated: each site adds its own animals; the total has no site.
	requireOrderedRows(t, sitesWithSums, res, "weight", int64(190), int64(300), nil)
	// One currency among the matched rows: the sum keeps it.
	requireRows(t, res, "usdFees", int64(350), int64(350), int64(350))
	requireRows(t, res, "usdFees@currency", "USD", "USD", "USD")
	requireRows(t, res, "prices", int64(625000), int64(625000), int64(625000))
	for _, c := range res.Columns {
		if c.ID == "prices" && (c.Type != Money || c.Currency != "USD") {
			t.Fatalf("prices column = %+v", c)
		}
		if c.ID == "weight" && c.Type != Number {
			t.Fatalf("weight column = %+v", c)
		}
	}
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// bob's animals but lions are zed (300 USD) and mo (20 GBP): their fees
// cannot be added, so the cell is blank and a notice names the column. When
// the totals' notice is due too, the two are one notice.
func TestASumLookupOverTwoCurrenciesHasNoValueAndANotice(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "bob", sitesWithSums, Options{}, nil)
	requireRows(t, res, "usdFees", nil, nil, nil)
	want := []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"usdFees"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v", res.Notices)
	}

	const animals = `{"version":3,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"fee","path":[{"field":"fee"}],"aggregate":"sum"},
		{"id":"allFees","formula":{"op":"lookup","source":"zoo.animals","pick":"sum","value":{"path":[{"field":"fee"}]}}}],
		"sort":[{"col":"name"}]}`
	res = z.run(t, "ann", animals, Options{}, nil)
	for i, v := range rowsOf(res, "allFees") {
		if v != nil {
			t.Fatalf("row %d allFees = %#v, want blank", i, v)
		}
	}
	want = []Notice{{Code: "mixedCurrency", Message: "Amounts in different currencies are totalled apart, one row per currency.", Columns: []string{"fee", "allFees"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

func TestAnEmptyMatchIsBlankForSumAndZeroForCount(t *testing.T) {
	z := newZoo(t)
	nobody := `"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Nobody"}]},"value":{"path":[{"field":"weight"}]}`
	spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"sum","formula":{"op":"lookup","source":"zoo.animals","pick":"sum",` + nobody + `}},
		{"id":"count","formula":{"op":"lookup","source":"zoo.animals","pick":"count",` + nobody + `}}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "sum", nil, nil, nil)
	requireRows(t, res, "count", int64(0), int64(0), int64(0))
}

// count counts the matching rows whose value is not blank: kit has no birth
// day, so ann's animals with one are leo and zed.
func TestACountLookupCountsRowsWithAValue(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":3,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"site","path":[{"field":"id"}]},
		{"id":"born","formula":{"op":"lookup","source":"zoo.animals","pick":"count","value":{"path":[{"field":"born"}]}}},
		{"id":"here","formula":{"op":"lookup","source":"zoo.animals","pick":"count",
			"where":{"op":"eq","args":[{"path":[{"link":"site"}]},{"col":"site"}]},"value":{"path":[{"field":"name"}]}}}],
		"sort":[{"col":"name"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "born", int64(2), int64(2), int64(2))
	requireOrderedRows(t, spec, res, "here", int64(1), int64(1), nil)
	for _, c := range res.Columns {
		if (c.ID == "born" || c.ID == "here") && c.Type != Number {
			t.Fatalf("%s column = %+v", c.ID, c)
		}
	}
}

func TestPickSumNeedsANumberOrAnAmount(t *testing.T) {
	app := withZooRegistry(t)
	lookup := func(pick, field string) string {
		return `{"version":3,"source":"zoo.sites","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals",
			"pick":"` + pick + `","value":{"path":[{"field":"` + field + `"}]}}}]}`
	}
	for _, field := range []string{"name", "born", "active"} {
		err := Validate(app, loadSpec(t, lookup("sum", field)))
		requireSpecError(t, err, 400, "columns.0.formula.value", "pick sum adds numbers or amounts")
	}
	if err := Validate(app, loadSpec(t, lookup("count", "name"))); err != nil {
		t.Fatalf("a count of names: %v", err)
	}
}
```

In `TestALookupIsRefusedWhereItCannotBeRead` (`lookup_test.go:220-221`), the pick message changes:

```go
		"a bad pick": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"id"}]},"pick":"avg"}}]}`,
			"columns.0.formula.pick", "min, max, sum or count"},
```

Add these rows to the `cases` map of `TestV3PartsAreValidated` in `v3_test.go`:

```go
		// F1: sum and count lookups.
		"a pick that is not min, max, sum or count": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup",
			"source":"zoo.animals","value":{"path":[{"field":"weight"}]},"pick":"avg"}}]}`, 400, "columns.0.formula.pick", "pick is min, max, sum or count"},
		"a sum of days": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup",
			"source":"zoo.animals","value":{"path":[{"field":"born"}]},"pick":"sum"}}]}`, 400, "columns.0.formula.value", "pick sum adds numbers or amounts"},
```

- [ ] **Step 2: Add the shared formula-table cases**

Append to `cases` in `core/lib/forms/fixtures/formula-cases.json`, after "lookup inside arithmetic" (report-only: no `forms` key; the forms readers skip them):

```json
        {
            "name": "lookup pick sum adds the matching values",
            "types": { "c": "text" },
            "values": { "c": "a" },
            "lookupTypes": { "code": "text", "rate": "number" },
            "lookupRows": [
                { "code": "a", "rate": 3 },
                { "code": "a", "rate": 1 },
                { "code": "b", "rate": 9 }
            ],
            "formula": {
                "op": "lookup",
                "source": "table",
                "where": { "op": "eq", "args": [{ "path": [{ "field": "code" }] }, { "col": "c" }] },
                "value": { "path": [{ "field": "rate" }] },
                "pick": "sum"
            },
            "expect": 4
        },
        {
            "name": "lookup pick count counts them",
            "types": { "c": "text" },
            "values": { "c": "a" },
            "lookupTypes": { "code": "text", "rate": "number" },
            "lookupRows": [
                { "code": "a", "rate": 3 },
                { "code": "a", "rate": 1 },
                { "code": "b", "rate": 9 }
            ],
            "formula": {
                "op": "lookup",
                "source": "table",
                "where": { "op": "eq", "args": [{ "path": [{ "field": "code" }] }, { "col": "c" }] },
                "value": { "path": [{ "field": "rate" }] },
                "pick": "count"
            },
            "expect": 2
        },
        {
            "name": "lookup pick sum with no match is empty",
            "types": { "c": "text" },
            "values": { "c": "z" },
            "lookupTypes": { "code": "text", "rate": "number" },
            "lookupRows": [{ "code": "a", "rate": 3 }],
            "formula": {
                "op": "lookup",
                "source": "table",
                "where": { "op": "eq", "args": [{ "path": [{ "field": "code" }] }, { "col": "c" }] },
                "value": { "path": [{ "field": "rate" }] },
                "pick": "sum"
            },
            "expect": null
        },
        {
            "name": "lookup pick count with no match is zero",
            "types": { "c": "text" },
            "values": { "c": "z" },
            "lookupTypes": { "code": "text", "rate": "number" },
            "lookupRows": [{ "code": "a", "rate": 3 }],
            "formula": {
                "op": "lookup",
                "source": "table",
                "where": { "op": "eq", "args": [{ "path": [{ "field": "code" }] }, { "col": "c" }] },
                "value": { "path": [{ "field": "rate" }] },
                "pick": "count"
            },
            "expect": 0
        }
```

In `formula_cases_test.go`, the harness's own lookup picks by name (add `"cmp"` to the imports). Replace lines 162-166:

```go
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	return expr{sql: fmt.Sprintf("(SELECT %s(%s) FROM (%s) l WHERE %s)", pick, v.sql, c.lookupTable(), where), typ: v.typ}, nil
```

with:

```go
	// pick names the SQL aggregate (min, max, sum, count); a count is a
	// number whatever it counts.
	pick, typ := cmp.Or(n.Pick, "min"), v.typ
	if pick == "count" {
		typ = Number
	}
	return expr{sql: fmt.Sprintf("(SELECT %s(%s) FROM (%s) l WHERE %s)", pick, v.sql, c.lookupTable(), where), typ: typ}, nil
```

In `formula_run_test.go:227`, the four new cases each run one detail row:

```go
	if detailRuns != 63 || totalRuns != 12 || recordRuns != 2 {
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'SumLookup|EmptyMatch|CountLookup|PickSum|LookupIsRefused|V3Parts|FormulaCases' ./reports/`
Expected: FAIL: `Run: reports: columns.2.formula.pick: pick is min or max` (and the same refusal in the formula-table run cases), `problem at "columns.0.formula.pick" = "pick is min or max", want it to contain "min, max, sum or count"`, and `ran 59 detail rows`.

- [ ] **Step 4: Implement sum and count in `lookup.go`**

Add `"slices"` to the imports. After `lookupValue`:

```go
// lookupPicks are a lookup's picks: the smallest or largest matching value,
// their sum, or the number of matching rows with a value.
var lookupPicks = []string{"min", "max", "sum", "count"}
```

Replace the pick check (`lookup.go:102-104`):

```go
	if n.Pick != "" && !slices.Contains(lookupPicks, n.Pick) {
		return expr{}, invalid(at+".pick", "pick is min, max, sum or count")
	}
```

Replace the body after the where (from `if v.typ == Money && v.cur.sql != "" {` at line 133 to the end of `lookupIn`) with:

```go
	switch n.Pick {
	case "count":
		// count skips a blank value; with no match it is 0.
		return expr{sql: fmt.Sprintf("(SELECT count(%s)\n%s\nWHERE %s)", v.sql, sc.render(), where), typ: Number, agg: out.agg, win: out.win}, nil
	case "sum":
		if v.typ != Number && v.typ != Money {
			return expr{}, invalid(at+".value", "pick sum adds numbers or amounts")
		}
		return summedLookup(outer, r, n, v, sc, where, out), nil
	}
	if v.typ == Money && v.cur.sql != "" {
		return pickedAmount(outer, r, n, v, sc, where, out), nil
	}
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	from := sc.render()
	out.sql = fmt.Sprintf("(SELECT %s(%s)\n%s\nWHERE %s)", pick, v.sql, from, where)
	if v.shown != "" {
		out.shown = pickedShown(n, v, from, where)
	}
	return out, nil
}

// summedLookup adds the value over the matching rows, blank when none match.
// An amount whose currency each row carries is added only in one currency:
// where the matching rows hold two, the sum is blank and mixed says so (R3).
// Its currency is the rows' one currency, read beside it; on a base row
// inside a total, the key valueCurrencies gave the lookup, as pickedAmount's.
func summedLookup(outer fctx, r *rowsCTE, n *Node, v expr, sc *scope, where string, out expr) expr {
	from := sc.render()
	out.target, out.options, out.shown = nil, nil, ""
	if v.typ != Money || v.cur.sql == "" {
		out.sql = fmt.Sprintf("(SELECT sum(%s)\n%s\nWHERE %s)", v.sql, from, where)
		return out
	}
	matched := where + " AND " + v.sql + " IS NOT NULL"
	out.mixed = fmt.Sprintf("((SELECT count(DISTINCT %s)\n%s\nWHERE %s) > 1)", v.cur.sql, from, matched)
	out.sql = fmt.Sprintf("(CASE WHEN %s THEN NULL ELSE (SELECT sum(%s)\n%s\nWHERE %s) END)", out.mixed, v.sql, from, where)
	id := lookupCurrencyID(n)
	out.cur = currency{sql: fmt.Sprintf("(SELECT min(%s)\n%s\nWHERE %s)", v.cur.sql, from, matched), id: id}
	if ck, ok := r.c.valueCur[id]; ok && !inLevel(outer) {
		out.cur = currency{key: ck, id: id}
	}
	return out
}
```

Update the comment above `lookupIn` (line 86-87) to: `// lookupIn compiles a lookup in a context whose rows CTE is r: the smallest (pick min, the default) or largest value of the node's value over the rows of its source that match its where, their sum (NULL when none match) or the number of them with a value (0 when none match).`

- [ ] **Step 5: Carry the mixed check to a cell and a notice**

In `formula.go` `expr`, after `text *string`:

```go
	// mixed is SQL that is true where a sum lookup in the value met amounts
	// in two currencies (R3), "" when none can.
	mixed string
```

After `num`:

```go
// orSQL joins two conditions with OR; an empty one is no condition.
func orSQL(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return "(" + a + " OR " + b + ")"
}
```

In `compileNode`, replace the loop after `operator` (lines 310-313):

```go
	for _, a := range args {
		e.agg = e.agg || a.agg
		e.win = e.win || a.win
		e.mixed = orSQL(e.mixed, a.mixed)
	}
```

In `run.go`, after `noCarriedCell`:

```go
// mixedCell is a column cell's helper vmx<idx>: 1 on a row where a sum
// lookup the cell reads met amounts in two currencies, NULL elsewhere.
func mixedCell(e expr, idx int) string {
	if e.mixed == "" {
		return noMixedCell(idx)
	}
	return fmt.Sprintf("(CASE WHEN %s THEN 1 END) AS vmx%d", e.mixed, idx)
}

func noMixedCell(idx int) string { return fmt.Sprintf("NULL AS vmx%d", idx) }

// mixedHelpers are the helper cells whose 1 marks column idx as one whose
// looked-up total met two currencies.
func mixedHelpers(idx int) []string { return []string{fmt.Sprintf("vmx%d", idx)} }
```

Replace `helperCell` (`run.go:582-584`):

```go
// helperCell matches a row's record label (vl<idx>), its label's currency
// (vc<idx>), a value's carried currency (vcur<idx>) and a looked-up total's
// mixed check (vmx<idx>).
var helperCell = regexp.MustCompile(`^(?:vl|vc|vcur|vmx)\d+$`)
```

In `groupStatement` (`run.go:506`):

```go
			carried = append(carried, c.carriedCell(s, cp, e, idx), mixedCell(e, idx))
```

In `detailSelect`, the child row column (`run.go:816`):

```go
			sel = append(sel, fmt.Sprintf("NULL AS v%d", idx), noCarriedCell(idx), noMixedCell(idx))
```

and the other columns (`run.go:830`):

```go
		sel = append(sel, fmt.Sprintf("%s AS v%d", cellSQL(e), idx), c.carriedCell(r.s, cp, e, idx), mixedCell(e, idx))
```

In `layout.go`, every leg of the union holds the same cells, so a computed row carries the helper too (`layout.go:78` and `:87`):

```go
				carried = append(carried, noCarriedCell(idx), noMixedCell(idx))
```

```go
			carried = append(carried, lc.carriedCell(cp, e, idx), mixedCell(e, idx))
```

In `levels.go` `recompute` (lines 757-759), a blanked formula has no value to be mixed:

```go
		if lv.readsRowValue(cp.col.Formula, false, map[string]bool{}) {
			e.sql, e.mixed = "NULL", ""
		}
```

In `assemble.go`, add `"slices"` to the imports and, after `markCurrencies`:

```go
// markMixedLookups names in a notice every column a row of which shows no
// value because a sum lookup met amounts in two currencies (R3). With the
// totals' notice due too, the two are one notice: its message, both columns.
func (rn *runner) markMixedLookups(rowSets ...[]map[string]any) {
	marked := func(name string) bool {
		return slices.ContainsFunc(rowSets, func(rows []map[string]any) bool {
			return slices.ContainsFunc(rows, func(row map[string]any) bool { return row[name] != nil })
		})
	}
	var cols []string
	for idx, cp := range rn.c.p.cols {
		if slices.ContainsFunc(mixedHelpers(idx), marked) {
			cols = append(cols, cp.col.ID)
		}
	}
	if len(cols) == 0 {
		return
	}
	for i := range rn.res.Notices {
		if n := &rn.res.Notices[i]; n.Code == "mixedCurrency" {
			for _, id := range cols {
				if !slices.Contains(n.Columns, id) {
					n.Columns = append(n.Columns, id)
				}
			}
			return
		}
	}
	rn.notice(Notice{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: cols})
}
```

In `run.go` `run`, after `rn.markCurrencies(groupRows)`:

```go
	rn.markMixedLookups(groupRows, details)
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'SumLookup|EmptyMatch|CountLookup|PickSum|LookupIsRefused|V3Parts|FormulaCases' ./reports/`
Expected: PASS.

- [ ] **Step 7: Run the whole reports suite, the forms tables and gofmt**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go test -tags no_ui -count=1 ./reports/ ./forms/
cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run core/lib/forms/__tests__/formula-cases.test.ts
```
Expected: `gofmt -l` prints nothing; both Go packages `ok`; the vitest file passes (the new cases carry no `forms` key).

- [ ] **Step 8: Hand over for commit**

The driver commits:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/lookup.go core/server/reports/formula.go core/server/reports/run.go core/server/reports/layout.go core/server/reports/levels.go core/server/reports/assemble.go core/server/reports/lookup_test.go core/server/reports/formula_cases_test.go core/server/reports/formula_run_test.go core/server/reports/v3_test.go core/lib/forms/fixtures/formula-cases.json
git commit -m "core(reports): sum and count lookups, blank with a notice over two currencies"
```

---

### Task 2b — R24: amounts read from one currency field combine, guarded per row (Go)

Engine facts used: `currency` and `same` (`formula.go:58-80`: equal ids, else equal keys, else the same fixed source); `combined` refuses two currencies that are not `same` (`formula.go:95-116`); `currencyGuard` dedupes its values' currencies with `same` (`formula.go:133`) and guards only a param, a comparison or a ratio (`:139`); `operator` calls `combined` then `currencyGuard` (`formula.go:434-441`); `compileNode` ORs the args' `mixed` into the result after `operator` (Task 2); a base path amount's currency is a key, `currency{key: ck<n>}` (`rows.go:346-355`); a filter path's is a key or `currency{sql, id: "row:"+sql}` (`join.go:97-109`); a looked-up path's is `currency{sql, id: "row:"+sql}` (`lookup.go:273-278`); `pickedAmount` (`lookup.go:177-182`) and Task 2's `summedLookup` give the lookup `id: lookupCurrencyID(n)` ("lk:" + node JSON), and `valueCur` registers one currency key per such id (`rows.go:301-328`), so two lookups of one field must keep two ids; `amountCurrencyOf` reads the currency field from `p.src` (`rows.go:361-369`); a cell's `<id>@currency` is its `vcur<idx>` when set, else its column's key (`assemble.go:391-399`); a formula column's currency is its detail cell's (`run.go:833-836`); `totalCurrency` refuses an SQL currency with no registered key (`levels.go:782-790`). The zoo (`zoo_test.go`): `zoo.feedings.cost` is money whose currency is the row's `currency` field (`:186`), every feeding "USD" (`:256`); ann reads f1 1000, f2 2500 (leo), f3 700, f4 300 (zed); `zoo.animals.fee` is money in `fee_currency` (`:170`).

R24 is the rule. Probe (scratch `go test -overlay` with Task 2's code applied, then this task's): the three run tests below fail on Task 2's code with the refusals Step 2 names; with this task's code all four pass and the whole reports suite passes (only `TestALookupIsRefusedWhereItCannotBeRead`'s pick message, which Task 2 itself changes, differed in the probe).

**Files:**
- Modify: `core/server/reports/formula.go` (`currency.field`, `same`, `sameRead`, `currencyGuard`'s dedupe, `fieldGuard`, the call in `operator`)
- Modify: `core/server/reports/rows.go:343-355` (`currencyField`, `pathCurrency`)
- Modify: `core/server/reports/join.go:97-109` (`amountCurrency`)
- Modify: `core/server/reports/lookup.go` (`lookupCtx.path`, `pickedAmount`, Task 2's `summedLookup`)
- Create: `core/server/reports/fieldcurrency_test.go`

**Interfaces:**
- Consumes: pick `"sum"`, `summedLookup`, `expr.mixed`, `orSQL`, the `vmx<idx>` cell and the lookup `mixedCurrency` notice (Task 2).
- Produces: `currency.field string` (`"<source key>.<currency field key>"`); `func currencyField(p *pathPlan) string`; `func (a currency) sameRead(b currency) bool` (today's `same`); `same` also true for one non-empty `field`; `func fieldGuard(c fctx, e expr, values []expr) expr` (an amount of two or more reads of one field: blank where two known codes differ, `mixed` there, currency `COALESCE(<codes>)` with id `"any:" + <that SQL>`). Task 5's `checkInColumn` and Task 6's opening cells rely on `same` matching a cell of the column's field; Task 22's ledger relies on all of it.

- [ ] **Step 1: Write the failing Go tests**

Create `core/server/reports/fieldcurrency_test.go`:

```go
package reports

import (
	"reflect"
	"testing"
)

// lookedUpCost is a sum lookup of zoo.feedings.cost (an amount whose currency
// each row carries) over the feedings whose note is note.
func lookedUpCost(note string) string {
	return `{"op":"lookup","source":"zoo.feedings","pick":"sum",
		"where":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"` + note + `"}]},
		"value":{"path":[{"field":"cost"}]}}`
}

func orZero(n string) string { return `{"op":"coalesce","args":[` + n + `,{"value":0}]}` }

// twoLookedUpCosts adds two different sum lookups of cost, each blank as 0.
var twoLookedUpCosts = `{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,` + orZero(lookedUpCost("f3")) + `]}`

// feedingsWithLookedUpCost adds looked-up costs to the row's own cost, to its
// running sum, and to each other. Every one reads zoo.feedings.cost, whose
// currency is the row's own currency field.
var feedingsWithLookedUpCost = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"plus","formula":{"op":"add","args":[` + lookedUpCost("f1") + `,{"path":[{"field":"cost"}]}]}},
	{"id":"balance","formula":{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]}},
	{"id":"two","formula":` + twoLookedUpCosts + `},
	{"id":"signed","formula":{"op":"if","args":[{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f2"}]},
		{"op":"sub","args":[{"value":0},{"op":"add","args":[` + twoLookedUpCosts + `,{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]}]},
		{"op":"add","args":[` + twoLookedUpCosts + `,{"op":"runningSum","args":[{"path":[{"field":"cost"}]}]}]}]}},
	{"id":"none","formula":{"op":"add","args":[` + orZero(lookedUpCost("nobody")) + `,{"path":[{"field":"cost"}]}]}},
	{"id":"largest","formula":{"op":"add","args":[{"op":"lookup","source":"zoo.feedings","pick":"max","value":{"path":[{"field":"cost"}]}},{"path":[{"field":"cost"}]}]}}],
	"sort":[{"col":"note"}]}`

// ann's feedings: f1 1000, f2 2500 (leo), f3 700, f4 300 (zed), all USD.
func TestALookedUpAmountCombinesWithAmountsOfItsField(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsWithLookedUpCost, Options{}, nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "note", "f1", "f2", "f3", "f4", nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "plus", int64(2000), int64(3500), int64(1700), int64(1300), nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "balance", int64(2000), int64(4500), int64(5200), int64(5500), nil)
	requireRows(t, res, "two", int64(1700), int64(1700), int64(1700), int64(1700), int64(1700))
	// Combined amounts combine again: signed reads two lookups and a running
	// sum twice, as a balance that flips its sign on some rows does.
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "signed", int64(2700), int64(-5200), int64(5900), int64(6200), nil)
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "largest", int64(3500), int64(5000), int64(3200), int64(2800), nil)
	// A lookup that matches nothing has no currency: the sum takes the row's.
	requireOrderedRows(t, feedingsWithLookedUpCost, res, "none", int64(1000), int64(2500), int64(700), int64(300), nil)
	requireRows(t, res, "none@currency", "USD", "USD", "USD", "USD", "USD")
	requireRows(t, res, "plus@currency", "USD", "USD", "USD", "USD", "USD")
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// On a group row a looked-up amount adds to the group's total of its field.
func TestALookedUpAmountCombinesWithATotalOfItsField(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"rolled","formula":{"op":"add","args":[` + orZero(lookedUpCost("f1")) + `,{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}},
		{"id":"both","formula":{"op":"add","args":[` + twoLookedUpCosts + `,` + orZero(`{"op":"sum","args":[{"path":[{"field":"cost"}]}]}`) + `]}}],
		"groups":[{"id":"bySpecies","col":"species"}],
		"sort":[{"col":"species"},{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, spec, res, "note", nil, "f1", "f2", nil, "f3", "f4", nil)
	requireOrderedRows(t, spec, res, "rolled", int64(4500), int64(2000), int64(3500), int64(2000), int64(1700), int64(1300), int64(5500))
	requireRows(t, res, "rolled@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
	requireOrderedRows(t, spec, res, "both", int64(5200), int64(2700), int64(4200), int64(2700), int64(2400), int64(2000), int64(6200))
}

// f3 is in EUR: its cost and the looked-up USD cost of f1 cannot be added, so
// that one cell is blank and a notice names the column.
func TestALookedUpAmountInAnotherCurrencyThanItsRowIsBlank(t *testing.T) {
	z := newZoo(t)
	f3 := z.recs["f3"]
	f3.Set("currency", "EUR")
	if err := z.app.Save(f3); err != nil {
		t.Fatal(err)
	}
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"plus","formula":{"op":"add","args":[` + lookedUpCost("f1") + `,{"path":[{"field":"cost"}]}]}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	// Two total rows, one per currency: a formula that reads a row's cost has
	// no value on either.
	requireOrderedRows(t, spec, res, "plus", int64(2000), int64(3500), nil, int64(1300), nil, nil)
	want := []Notice{{Code: "mixedCurrency", Message: "A looked-up total mixes currencies, so it shows no value.", Columns: []string{"plus"}}}
	if !reflect.DeepEqual(res.Notices, want) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// Amounts of two currency fields still do not combine: a cost and an
// animal's fee. This passes before and after the task; it pins that the rule
// stays one field wide.
func TestAmountsOfTwoCurrencyFieldsStillDoNotCombine(t *testing.T) {
	app := withZooRegistry(t)
	spec := `{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"add","args":[
		` + lookedUpCost("f1") + `,{"path":[{"link":"animal"},{"field":"fee"}]}]}}]}`
	requireSpecError(t, Validate(app, loadSpec(t, spec)), 400, "columns.0.formula", "amounts in different currencies cannot be combined")
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'LookedUp|TwoCurrencyFields' ./reports/`
Expected: FAIL: `TestALookedUpAmountCombinesWithAmountsOfItsField` and `TestALookedUpAmountInAnotherCurrencyThanItsRowIsBlank` with `Run: reports: columns.1.formula: amounts in different currencies cannot be combined`, `TestALookedUpAmountCombinesWithATotalOfItsField` with `Run: reports: columns.3.formula: amounts in different currencies cannot be combined`; `TestAmountsOfTwoCurrencyFieldsStillDoNotCombine` passes.

- [ ] **Step 3: Name the currency field in `formula.go`**

In `currency`, after `param string`:

```go
	// field names the source field a carried currency is read from,
	// "<source key>.<currency field key>" (R24): two amounts whose currencies
	// come from one field combine, wherever each is read (the base row, a
	// looked-up row, a total), and fieldGuard checks their codes on each row.
	// "" for a fixed currency, a record's, a param's and a section subtotal.
	field string
```

Replace `same` (`formula.go:68-80`) with:

```go
// same is true for one currency source: one currency field (R24), or one
// read of it (sameRead).
func (a currency) same(b currency) bool {
	if a.field != "" && a.field == b.field {
		return true
	}
	return a.sameRead(b)
}

// sameRead is true for one read of a currency: one carried value, a key
// equals itself, a fixed currency the same source's.
func (a currency) sameRead(b currency) bool {
	switch {
	case a.id != "" && a.id == b.id:
		return true
	case a.key != "" || b.key != "":
		return a.key == b.key
	case a.fixed != nil || b.fixed != nil:
		return a.fixed != nil && b.fixed != nil && a.fixed.Key == b.fixed.Key
	}
	return a.id == b.id
}
```

In `currencyGuard` (`formula.go:133`), two reads of one field are two currencies to check:

```go
		if v.cur.none() || slices.ContainsFunc(curs, v.cur.sameRead) {
```

In `operator` (`formula.go:434-438`):

```go
	if e.typ == Money {
		if e.cur, err = combined(args, at); err != nil {
			return expr{}, err
		}
		e = fieldGuard(c, e, args)
	}
```

After `currencyGuard`:

```go
// fieldGuard is an amount e made of values whose currencies are read from
// one currency field in two or more places (a looked-up row's and the base
// row's, say: R24). Where two of their codes are known and differ, e is
// blank and mixed is true, which the run names in a mixedCurrency notice
// (R3). Its currency is the first known code, so a lookup that matched
// nothing takes the row's. e is unchanged with one read or none.
func fieldGuard(c fctx, e expr, values []expr) expr {
	var reads []currency
	for _, v := range values {
		if v.cur.field == "" || v.cur.field != e.cur.field || slices.ContainsFunc(reads, v.cur.sameRead) {
			continue
		}
		reads = append(reads, v.cur)
	}
	if len(reads) < 2 {
		return e
	}
	codes := make([]string, len(reads))
	for i, cur := range reads {
		codes[i] = c.currencySQL(cur)
	}
	var differ []string
	for i, a := range codes {
		for _, b := range codes[i+1:] {
			differ = append(differ, fmt.Sprintf("(%s IS NOT NULL AND %s IS NOT NULL AND %s <> %s)", a, b, a, b))
		}
	}
	mixed := "(" + strings.Join(differ, " OR ") + ")"
	e.sql = "(CASE WHEN " + mixed + " THEN NULL ELSE " + e.sql + " END)"
	e.mixed = orSQL(e.mixed, mixed)
	// The id makes two different guarded amounts two reads (sameRead), so
	// combining them is guarded again.
	cur := "COALESCE(" + strings.Join(codes, ", ") + ")"
	e.cur = currency{sql: cur, id: "any:" + cur, field: e.cur.field}
	return e
}
```

- [ ] **Step 4: Set the field where an amount is read**

In `rows.go`, replace `pathCurrency` (`:343-355`) and add `currencyField` after it:

```go
// pathCurrency is the currency of a to-one path's amount: its currency
// field, kept in w as a currency key every total groups by and named for
// combining (currencyField), or its source's fixed currency.
func (r *rowsCTE) pathCurrency(p *pathPlan) currency {
	sql, fixed := amountCurrencyOf(r.root, p.hops, p)
	switch {
	case sql != "":
		return currency{key: r.currencyColumn(sql), field: currencyField(p)}
	case fixed != nil:
		return currency{fixed: fixed}
	}
	return currency{}
}

// currencyField names the currency field of p's amount for combining (R24):
// "<source key>.<currency field key>", "" for a value that is not an amount
// or an amount with no currency field.
func currencyField(p *pathPlan) string {
	if p.field.Type != Money || p.field.Currency == "" {
		return ""
	}
	return p.src.Key + "." + p.field.Currency
}
```

In `join.go` `amountCurrency` (`:100-104`):

```go
	case sql != "":
		if name, ok := j.r.byExpr[sql]; ok && j.sc == j.r.root {
			return currency{key: name, field: currencyField(bound)}
		}
		return currency{sql: sql, id: "row:" + sql, field: currencyField(bound)}
```

In `lookup.go` `lookupCtx.path` (`:274-275`):

```go
	case cur != "":
		e.cur = currency{sql: cur, id: "row:" + cur, field: currencyField(bound)}
```

In `pickedAmount` (`:179-182`), the looked-up amount keeps its field; its id stays the lookup's, which `valueCur` registers its key by:

```go
	out.cur = currency{sql: pickOne(v.cur.sql), id: id, field: v.cur.field}
	if ck, ok := r.c.valueCur[id]; ok && !inLevel(outer) {
		out.cur = currency{key: ck, id: id, field: v.cur.field}
	}
```

In Task 2's `summedLookup`, the same:

```go
	out.cur = currency{sql: fmt.Sprintf("(SELECT min(%s)\n%s\nWHERE %s)", v.cur.sql, from, matched), id: id, field: v.cur.field}
	if ck, ok := r.c.valueCur[id]; ok && !inLevel(outer) {
		out.cur = currency{key: ck, id: id, field: v.cur.field}
	}
```

- [ ] **Step 5: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'LookedUp|TwoCurrencyFields|SumLookup|Currency' ./reports/`
Expected: PASS.

- [ ] **Step 6: Run the whole reports suite, vet and gofmt**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: `gofmt -l` prints nothing; `ok  	tinycld.org/core/reports` (no existing test combines two reads of one field).

- [ ] **Step 7: Hand over for commit**

The driver commits:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/formula.go core/server/reports/rows.go core/server/reports/join.go core/server/reports/lookup.go core/server/reports/fieldcurrency_test.go
git commit -m "core(reports): amounts of one currency field combine, blank where a row's codes differ"
```

---

### Task 3 — F2: the `outer` leaf (Go)

Engine facts used: the `fctx` interface (`formula.go:186-213`), `compileNode`'s leaf dispatch (`formula.go:247-262`), `nodeKinds` and `errNodeKind` (`formula.go:317-331`), `ctxBase` (`formula.go:1046-1062`); every context but the test harness's `tableCtx` embeds `ctxBase` (`detail.go:12`, `join.go:47`, `layout.go:139`, `levels.go:626`, `lookup.go:218`; `fillCtx` embeds `*joinCtx`); `lookupCtx` has a field named `outer` (`lookup.go:219`), so the interface method cannot share that name; `lookupCtx` routes col/record/param to its outer context (`lookup.go:229-256`) and a nested lookup's outer context is the enclosing `lookupCtx` (`lookup.go:250`, `:300`); `bindValuePath` (`join.go:12-19`) and `binder.path`'s refusals (`bind.go:509-590`); `levelCtx.path` is NULL (`levels.go:977-983`); `lookupReadsRowValue` (`levels.go:927-947`); the harness's `tableLookupCtx` (`formula_cases_test.go:193-226`).

**Files:**
- Modify: `core/server/reports/formula.go` (`fctx.outerPath`, `nodeKinds`, `errNodeKind`, dispatch, `ctxBase.outerPath`)
- Modify: `core/server/reports/lookup.go` (`lookupCtx.outerPath`, `outerToMany`, the leaf check in `lookupIn`)
- Modify: `core/server/reports/levels.go:927-947` (`lookupReadsRowValue`)
- Modify: `core/server/reports/formula_cases_test.go` (harness `outerPath`)
- Modify: `core/server/reports/formula_run_test.go:227` (one-row case count)
- Modify: `core/server/reports/lookup_test.go` (new tests)
- Modify: `core/lib/forms/fixtures/formula-cases.json` (one case)
- Modify: `core/server/reports/v3_test.go` (rows)

**Interfaces:**
- Consumes: `Node.Outer` (Task 1); `pick: "count"` (Task 2).
- Produces: `fctx.outerPath(p Path, at string) (expr, error)` (the brief's `fctx.outer`: `lookupCtx` already has a field `outer`); `ctxBase.outerPath` refuses "outer reads the report's row: use it inside a lookup" at the node; `lookupCtx.outerPath` refuses a to-many hop at `<at>.outer` ("outer reads one value of the report's row: a to-many link is not one"); `errNodeKind` = "a formula node is one of op, col, path, outer, param, value, subtotal or record"; `outerToMany(base *Source, p Path) bool`.

- [ ] **Step 1: Write the failing Go tests**

Append to `core/server/reports/lookup_test.go`:

```go
// feedingsWithOuter reads the report's row from inside lookups: earlier
// counts ann's feedings fed before the row's (f1 01-05, f3 01-20, f2 02-10,
// f4 03-03), other is the other feeding of the row's animal, site the name of
// the row's animal's site, and nested counts the earlier feedings inside a
// lookup of another source.
const feedingsWithOuter = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"earlier","formula":{"op":"lookup","source":"zoo.feedings","pick":"count",
		"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]},
		"value":{"path":[{"field":"id"}]}}},
	{"id":"other","formula":{"op":"lookup","source":"zoo.feedings",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"link":"animal"}]}]},
			{"op":"ne","args":[{"path":[{"field":"note"}]},{"outer":[{"field":"note"}]}]}]},
		"value":{"path":[{"field":"note"}]}}},
	{"id":"site","formula":{"op":"lookup","source":"zoo.sites",
		"where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"outer":[{"link":"animal"},{"link":"site"}]}]},
		"value":{"path":[{"field":"name"}]}}},
	{"id":"nested","formula":{"op":"lookup","source":"zoo.sites",
		"where":{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]},
		"value":{"op":"lookup","source":"zoo.feedings","pick":"count",
			"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]},
			"value":{"path":[{"field":"id"}]}}}}],
	"sort":[{"col":"note"}]}`

// Each detail row looks up its own answer; the total row has no one row,
// so a lookup that reads it is blank there.
func TestAnOuterLeafCorrelatesALookupToItsRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsWithOuter, Options{}, nil)
	requireOrderedRows(t, feedingsWithOuter, res, "note", "f1", "f2", "f3", "f4", nil)
	requireOrderedRows(t, feedingsWithOuter, res, "earlier", int64(0), int64(2), int64(1), int64(3), nil)
	requireOrderedRows(t, feedingsWithOuter, res, "other", "f2", "f1", "f4", "f3", nil)
	requireOrderedRows(t, feedingsWithOuter, res, "site", "North", "North", "South", "South", nil)
}

// An outer leaf in a lookup inside a lookup reads the report's row, not the
// rows of the lookup around it (zoo.sites has no fed field).
func TestAnOuterLeafInANestedLookupReadsTheReportRow(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsWithOuter, Options{}, nil)
	requireOrderedRows(t, feedingsWithOuter, res, "nested", int64(0), int64(2), int64(1), int64(3), nil)
}

func TestAnOuterLeafIsBlankOnGroupRows(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"earlier","formula":{"op":"lookup","source":"zoo.feedings","pick":"count",
			"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"outer":[{"field":"fed"}]}]},
			"value":{"path":[{"field":"id"}]}}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireOrderedRows(t, spec, res, "earlier", nil, int64(0), int64(2), nil, int64(1), int64(3), nil)
}

func TestAnOuterLeafIsRefusedOutsideALookup(t *testing.T) {
	app := withZooRegistry(t)
	outer := `{"outer":[{"field":"note"}]}`
	grouped := func(part string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],` + part + `}`
	}
	cases := map[string]struct{ spec, at string }{
		"a column":      {`{"version":3,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},{"id":"x","formula":` + outer + `}]}`, "columns.1.formula"},
		"the filter":    {grouped(`"filter":{"op":"eq","args":[` + outer + `,{"value":"f1"}]}`), "filter.args.0"},
		"a having":      {grouped(`"groups":[{"id":"g","col":"animal","having":{"op":"eq","args":[` + outer + `,{"value":"f1"}]}}]`), "groups.0.having.args.0"},
		"a group label": {grouped(`"groups":[{"id":"g","col":"animal","label":` + outer + `}]`), "groups.0.label"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(app, loadSpec(t, c.spec))
			requireSpecError(t, err, 400, c.at, "outer reads the report's row: use it inside a lookup")
		})
	}
}

// A feeding's animal has many feedings: the row has no one value there.
func TestAnOuterLeafIsRefusedOnAToManyPath(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals",
		"where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"outer":[{"link":"animal"},{"link":"feedings"},{"field":"note"}]}]},
		"value":{"path":[{"field":"weight"}]}}}]}`))
	requireSpecError(t, err, 400, "columns.0.formula.where.args.1.outer", "outer reads one value of the report's row: a to-many link is not one")
}
```

Add these rows to the `cases` map of `TestV3PartsAreValidated` in `v3_test.go`:

```go
		// F2: the outer leaf.
		"an outer leaf with a value": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"outer":[{"field":"note"}],"value":1}}]}`,
			400, "columns.0.formula", "one of op, col, path, outer"},
		"an outer field the report's source does not have": {`{"version":3,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup",
			"source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"outer":[{"field":"nope"}]}]},
			"value":{"path":[{"field":"weight"}]}}}]}`, 422, "columns.0.formula.where.args.1.outer.0.field", `has no field "nope"`},
```

- [ ] **Step 2: Add the shared formula-table case**

Append to `cases` in `core/lib/forms/fixtures/formula-cases.json`, after "lookup pick count with no match is zero":

```json
        {
            "name": "outer reads the report row inside a lookup",
            "types": { "c": "text" },
            "values": { "c": "a" },
            "lookupTypes": { "code": "text", "rate": "number" },
            "lookupRows": [
                { "code": "a", "rate": 3 },
                { "code": "b", "rate": 9 }
            ],
            "formula": {
                "op": "lookup",
                "source": "table",
                "where": { "op": "eq", "args": [{ "path": [{ "field": "code" }] }, { "outer": [{ "field": "c" }] }] },
                "value": { "path": [{ "field": "rate" }] }
            },
            "expect": 3
        }
```

In `formula_cases_test.go`, the harness reads an outer leaf as the table's own column (after `func (c *tableCtx) exists`):

```go
// outerPath: the table has no paths; inside a lookup an outer leaf reads
// the table's row (tableLookupCtx).
func (c *tableCtx) outerPath(_ Path, at string) (expr, error) {
	return expr{}, invalid(at, "outer reads the report's row: use it inside a lookup")
}
```

and after `func (l *tableLookupCtx) path`:

```go
// outerPath reads one field of the table's row.
func (l *tableLookupCtx) outerPath(p Path, at string) (expr, error) {
	if len(p) != 1 || p[0].Field == "" {
		return expr{}, invalid(at, "an outer path is one field here")
	}
	return l.outer.col(p[0].Field, at)
}
```

In `formula_run_test.go:227`:

```go
	if detailRuns != 64 || totalRuns != 12 || recordRuns != 2 {
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'OuterLeaf|V3Parts|FormulaCases' ./reports/`
Expected: FAIL: `Run: reports: columns.1.formula.where.args.1: a formula node is one of op, col, path, param, value, subtotal or record` (an outer-only node is no kind yet), the refusal tests find that message where they want theirs, and `ran 63 detail rows`.

- [ ] **Step 4: Implement the leaf in `formula.go`**

Add to `fctx`, after `record`:

```go
	// outerPath compiles an outer leaf: a path on the report's row, read
	// from inside a lookup.
	outerPath(p Path, at string) (expr, error)
```

In `compileNode`'s leaf switch, before `case n.Path != nil:`:

```go
	case n.Outer != nil:
		return c.outerPath(n.Outer, at)
```

Replace `errNodeKind` and `nodeKinds`:

```go
// errNodeKind refuses a node that is not exactly one kind of node.
const errNodeKind = "a formula node is one of op, col, path, outer, param, value, subtotal or record"

func nodeKinds(n *Node) int {
	kinds := 0
	// A record leaf's path is part of it. (A lookup, whose value is a
	// formula, is compiled before nodes are counted.)
	path := n.Path != nil && n.Record == ""
	for _, set := range []bool{n.Op != "", n.Col != "", path, n.Outer != nil, n.Param != "", n.Value != nil, n.Subtotal != "", n.Record != ""} {
		if set {
			kinds++
		}
	}
	return kinds
}
```

Add to `ctxBase`:

```go
func (ctxBase) outerPath(_ Path, at string) (expr, error) {
	return expr{}, invalid(at, "outer reads the report's row: use it inside a lookup")
}
```

- [ ] **Step 5: Implement it in the lookup**

In `lookup.go` `lookupIn`, the leaf check (line 96):

```go
	if n.Col != "" || n.Path != nil || n.Outer != nil || n.Param != "" || n.Subtotal != "" || n.Record != "" {
		return expr{}, invalid(at, errNodeKind)
	}
```

After `lookupCtx.path`:

```go
// outerPath reads a path on the report's row: what a path means in the
// context the outermost lookup sits in (a detail row's value, a joined row
// in the filter, blank on a group, subtotal or total row). It is bound on
// the report's source, to one row (R4).
func (l *lookupCtx) outerPath(p Path, at string) (expr, error) {
	base := l.r.c.p.base
	if outerToMany(base, p) {
		return expr{}, invalid(at+".outer", "outer reads one value of the report's row: a to-many link is not one")
	}
	if _, err := bindValuePath(l.r.c.p.app, base, p, at+".outer", pathValue); err != nil {
		return expr{}, err
	}
	var row fctx = l
	for {
		inner, ok := row.(*lookupCtx)
		if !ok {
			break
		}
		row = inner.outer
	}
	return row.path(p, at+".outer")
}

// outerToMany is true when an outer path follows a to-many link from the
// report's source. A link it cannot follow is left to bindValuePath.
func outerToMany(base *Source, p Path) bool {
	src := base
	for _, h := range p {
		if h.Link == "" || src == nil {
			continue
		}
		l := src.link(h.Link)
		if l == nil {
			return false
		}
		if l.Cardinality != ToOne {
			return true
		}
		src = lookupRegistered(l.Target)
	}
	return false
}
```

In `levels.go` `lookupReadsRowValue`, the walk's switch reads an outer leaf as a row value (before `case n.Path != nil:`):

```go
		case n.Outer != nil:
			// The report's row: blank on a level's rows, as a path is.
			return !inAggregate
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'OuterLeaf|V3Parts|FormulaCases|Lookup' ./reports/`
Expected: PASS.

- [ ] **Step 7: Run the whole reports suite and gofmt**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go test -tags no_ui -count=1 ./reports/ ./forms/`
Expected: `gofmt -l` prints nothing; both packages `ok`.

- [ ] **Step 8: Hand over for commit**

The driver commits:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/formula.go core/server/reports/lookup.go core/server/reports/levels.go core/server/reports/formula_cases_test.go core/server/reports/formula_run_test.go core/server/reports/lookup_test.go core/server/reports/v3_test.go core/lib/forms/fixtures/formula-cases.json
git commit -m "core(reports): an outer leaf reads the report's row inside a lookup"
```

---

### Task 4 — F3 date-range bounds and F4 All dates (Go)

Engine facts used: `rangePresets` (`params.go:38-42`), `rangeValue` (`params.go:366-384`), `rangeDays` (`params.go:430-456`), `paramExpr` gives a date range `rng`, `lo`, `hi` and an unset one `lo = hi = "NULL"`, `unset` (`params.go:472-478`); `ResolvedParam` leaves out an empty `from`/`to` (`params.go:177-189`, `resolvedParams` `:192-207`); `between` compares `substr(x,1,10)` with `lo`/`hi` (`formula.go:884-906`); `operator` turns a condition on an unset param into `unsetCondition` in a filter only (`formula.go:441-444`); `compileNode`'s param case (`formula.go:256-257`); `paramLeaf` (`options.go:171-189`); the harness's `tableCtx.param` refuses every param (`formula_cases_test.go:84-86`) and `TestFormulaCasesThroughRun` builds a spec with no params (`formula_run_test.go:168-195`); `TestRangePresetsMatchTheShell` (`params_test.go:19-68`).

**Files:**
- Modify: `core/server/reports/params.go` (`allDates`, `rangeDays`, `paramExpr`, `rangeBound`)
- Modify: `core/server/reports/formula.go` (`expr.all`, `between`, the bound checks and dispatch)
- Modify: `core/server/reports/lookup.go:96` (a lookup node with a bound)
- Modify: `core/server/reports/params_test.go` (presets row, new tests)
- Modify: `core/server/reports/formula_cases_test.go` (`paramDefs`, `paramValues`, `tableCtx.param`)
- Modify: `core/server/reports/formula_run_test.go:168-227` (params in the run harness; one-row case count)
- Modify: `core/lib/forms/fixtures/formula-cases.json` (one case)

**Interfaces:**
- Consumes: `Node.Bound` (Task 1); `pick: "sum"`, `"count"` (Task 2).
- Produces: preset `"allDates"` (last in `rangePresets`); a resolved All dates `paramValue{typ: ParamDateRange}` with empty `from`/`to` and `unset` false; `expr.all bool` (a date range with no bounds); `rangeBound(e expr, bound, at string) (expr, error)`; messages "bound is from or to", "bound goes with a param", "bound reads a date range's start or end" (all at the node); formula-table case keys `paramDefs` (`[]ParamDef`) and `paramValues` (`map[string]json.RawMessage`).

- [ ] **Step 1: Write the failing Go tests**

In `params_test.go` `TestRangePresetsMatchTheShell`, add after the `last12Months` marchStart case:

```go
		{"allDates", july, "", ""},
```

Append to `params_test.go` (its imports already hold `reflect`):

```go
// boundSpec reads the period's first and last day, compares each feeding's
// day with the first, and sums ann's grams fed before it.
const boundSpec = `{"version":3,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],"columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"from","formula":{"param":"period","bound":"from"}},
	{"id":"to","formula":{"param":"period","bound":"to"}},
	{"id":"before","formula":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]}},
	{"id":"gramsBefore","formula":{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","pick":"sum",
		"where":{"op":"lt","args":[{"path":[{"field":"fed"}]},{"param":"period","bound":"from"}]},
		"value":{"path":[{"field":"grams"}]}},{"value":0}]}}],
	"sort":[{"col":"note"}]}`

func TestABoundReadsARangesFirstAndLastDay(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", boundSpec, Options{}, map[string]string{"period": `{"from":"2026-02-01","to":"2026-02-28"}`})
	requireRows(t, res, "from", "2026-02-01", "2026-02-01", "2026-02-01", "2026-02-01", "2026-02-01")
	requireRows(t, res, "to", "2026-02-28", "2026-02-28", "2026-02-28", "2026-02-28", "2026-02-28")
	// f1 01-05, f2 02-10, f3 01-20, f4 03-03; the total has no one day.
	requireOrderedRows(t, boundSpec, res, "before", true, false, true, false, nil)
	// f1's 500 and f3's 300 grams were fed before February.
	requireRows(t, res, "gramsBefore", int64(800), int64(800), int64(800), int64(800), int64(800))
	for _, c := range res.Columns {
		if (c.ID == "from" || c.ID == "to") && c.Type != Date {
			t.Fatalf("%s column = %+v", c.ID, c)
		}
	}
}

// An unset range and All dates have no first day: a bound is blank, and a
// comparison with it matches no row (R2), so nothing was fed before it.
func TestABoundOfAnOpenRangeHasNoValue(t *testing.T) {
	z := newZoo(t)
	for name, period := range map[string]string{"unset": `null`, "all dates": `{"preset":"allDates"}`} {
		t.Run(name, func(t *testing.T) {
			res := z.run(t, "ann", boundSpec, Options{}, map[string]string{"period": period})
			requireRows(t, res, "from", nil, nil, nil, nil, nil)
			requireRows(t, res, "to", nil, nil, nil, nil, nil)
			requireRows(t, res, "before", nil, nil, nil, nil, nil)
			requireRows(t, res, "gramsBefore", int64(0), int64(0), int64(0), int64(0), int64(0))
		})
	}
}

// All dates keeps every row, one with no day too, in a filter, a formula
// and a lookup, and resolves to a set range with no bounds.
func TestAllDatesMatchesEveryRowAndResolvesWithNoBounds(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 5, "currency": "USD", "grams": 1, "note": "f7"})
	inPeriod := `{"op":"between","args":[{"path":[{"field":"fed"}]},{"param":"period"}]}`
	spec := `{"version":3,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange","default":{"preset":"allDates"}}],
		"columns":[{"id":"note","path":[{"field":"note"}]},
		{"id":"inPeriod","formula":` + inPeriod + `},
		{"id":"counted","formula":{"op":"lookup","source":"zoo.feedings","pick":"count","where":` + inPeriod + `,"value":{"path":[{"field":"id"}]}}}],
		"filter":` + inPeriod + `,"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireOrderedRows(t, spec, res, "note", "f1", "f2", "f3", "f4", "f7", nil)
	requireRows(t, res, "inPeriod", true, true, true, true, true, nil)
	requireRows(t, res, "counted", int64(5), int64(5), int64(5), int64(5), int64(5), int64(5))
	if want := []ResolvedParam{{ID: "period", Type: ParamDateRange}}; !reflect.DeepEqual(res.Params, want) {
		t.Fatalf("params = %+v", res.Params)
	}
	// A set range drops the row with no day.
	res = z.run(t, "ann", spec, Options{}, map[string]string{"period": `{"from":"2026-01-01","to":"2026-01-31"}`})
	requireOrderedRows(t, spec, res, "note", "f1", "f3", nil)
	requireRows(t, res, "counted", int64(2), int64(2), int64(2))
}

func TestABoundIsRefusedOffADateRange(t *testing.T) {
	app := withZooRegistry(t)
	spec := func(formula string) string {
		return `{"version":3,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"},{"id":"day","type":"date"}],
			"columns":[{"id":"x","formula":` + formula + `}]}`
	}
	cases := map[string]struct{ formula, want string }{
		"a date param":       {`{"param":"day","bound":"from"}`, "bound reads a date range's start or end"},
		"a bound mid":        {`{"param":"period","bound":"mid"}`, "bound is from or to"},
		"a bound on a value": {`{"value":"2026-01-01","bound":"from"}`, "bound goes with a param"},
		"a bound on a lookup": {`{"op":"lookup","source":"zoo.animals","bound":"from","value":{"path":[{"field":"born"}]}}`,
			"one of op, col, path"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(app, loadSpec(t, spec(c.formula)))
			requireSpecError(t, err, 400, "columns.0.formula", c.want)
		})
	}
}
```

- [ ] **Step 2: Add the shared formula-table case and its harness**

Append to `cases` in `core/lib/forms/fixtures/formula-cases.json`, after "outer reads the report row inside a lookup":

```json
        {
            "name": "param bound from reads the range's start",
            "types": { "a": "date" },
            "values": { "a": "2026-01-31" },
            "paramDefs": [{ "id": "period", "type": "dateRange" }],
            "paramValues": { "period": { "from": "2026-02-01", "to": "2026-02-28" } },
            "formula": { "op": "lt", "args": [{ "col": "a" }, { "param": "period", "bound": "from" }] },
            "expect": true
        }
```

In `formula_cases_test.go`, add to `formulaCase` after `FiscalStart`:

```go
	// ParamDefs and ParamValues are the params a case's formula reads, as a
	// spec declares them and a run posts them.
	ParamDefs   []ParamDef                 `json:"paramDefs"`
	ParamValues map[string]json.RawMessage `json:"paramValues"`
```

add to `tableCtx` after `recordRow`:

```go
	// paramValues are the case's params as a run resolves them.
	paramValues map[string]paramValue
```

replace `tableCtx.param` (lines 84-86):

```go
func (c *tableCtx) param(id, at string) (expr, error) {
	v, ok := c.paramValues[id]
	if !ok {
		return expr{}, invalid(at, "unknown param "+id)
	}
	return paramExpr(c, v)
}
```

replace `tableLookupCtx.param` (line 209):

```go
func (l *tableLookupCtx) param(id, at string) (expr, error) { return l.outer.param(id, at) }
```

and in `runFormulaCase`, replace the `ctx := &tableCtx{…}` statement (lines 239-243):

```go
	values, perr := resolveParams(c.ParamDefs, c.ParamValues, anyDay)
	if perr != nil {
		t.Fatalf("params: %v", perr)
	}
	ctx := &tableCtx{
		types: c.Types, params: dbx.Params{}, fiscal: c.FiscalStart,
		lookupTypes: c.LookupTypes, lookupRows: c.LookupRows,
		recordTypes: c.RecordTypes, recordRow: c.Record,
		paramValues: values,
	}
```

In `formula_run_test.go` `TestFormulaCasesThroughRun`, the spec declares the case's params and each run posts them. Replace lines 172-179:

```go
			columns = append(columns, `{"id":"zz_out","formula":`+string(node)+`}`)
			raw := fmt.Sprintf(`{"version":1,"source":%q,"columns":[%s]}`, source, strings.Join(columns, ","))

			if c.RecordTypes != nil {
				recordRuns++
				columns = append(columns, `{"id":"zz_rec","path":[{"link":"rec"}]}`)
				raw = fmt.Sprintf(`{"version":2,"source":%q,"columns":[%s],"groups":[{"id":"g","col":"zz_rec"}]}`, source, strings.Join(columns, ","))
				res := z.run(t, "ann", raw, Options{Expand: [][]any{{"row0"}}}, nil)
```

with:

```go
			columns = append(columns, `{"id":"zz_out","formula":`+string(node)+`}`)
			params, given := "", map[string]string{}
			if c.ParamDefs != nil {
				defs, err := json.Marshal(c.ParamDefs)
				if err != nil {
					t.Fatal(err)
				}
				params = `"params":` + string(defs) + `,`
			}
			for id, v := range c.ParamValues {
				given[id] = string(v)
			}
			raw := fmt.Sprintf(`{"version":1,%s"source":%q,"columns":[%s]}`, params, source, strings.Join(columns, ","))

			if c.RecordTypes != nil {
				recordRuns++
				columns = append(columns, `{"id":"zz_rec","path":[{"link":"rec"}]}`)
				raw = fmt.Sprintf(`{"version":2,%s"source":%q,"columns":[%s],"groups":[{"id":"g","col":"zz_rec"}]}`, params, source, strings.Join(columns, ","))
				res := z.run(t, "ann", raw, Options{Expand: [][]any{{"row0"}}}, given)
```

then pass `given` in the two other runs: line 190 becomes `res := z.run(t, "ann", raw, Options{}, given)` and line 206 becomes `res, err := z.try(t, "ann", raw, Options{}, given)`. Line 227:

```go
	if detailRuns != 65 || totalRuns != 12 || recordRuns != 2 {
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'RangePresets|Bound|AllDates|FormulaCases' ./reports/`
Expected: FAIL: `allDates (fiscal start 7): 2025-08-01..2026-07-31, want ..` (it falls to `last12Months`); `Run: reports: params.period.default: the preset is one of [today … last12Months]`; the bound runs fail because `bound` is ignored and the node compiles as the whole date range, which no cell or comparison takes (a `Run:` or `compile:` error); the refusal cases get `err = <nil>` or another message; and `ran 64 detail rows`.

- [ ] **Step 4: Implement All dates in `params.go`**

Replace `rangePresets` (lines 38-42):

```go
	rangePresets = []string{
		"today", "thisMonth", "thisMonthToDate", "thisQuarter", "thisQuarterToDate",
		"thisFiscalYear", "thisFiscalYearToDate", "lastMonth", "lastQuarter",
		"lastFiscalYear", "last12Months", "allDates",
	}
```

In `rangeDays`, add a case before the `last12Months` fallthrough return:

```go
	case "allDates":
		// A set range with no bounds (R1): every row, a row with no day too.
		return "", ""
```

Replace the `ParamDateRange` case of `paramExpr`:

```go
	case ParamDateRange:
		switch {
		case v.unset:
			return expr{rng: true, lo: "NULL", hi: "NULL", unset: true}, nil
		case v.from == "" && v.to == "":
			return expr{rng: true, all: true, lo: "NULL", hi: "NULL"}, nil
		}
		return expr{rng: true, lo: c.bind(v.from), hi: c.bind(v.to)}, nil
```

After `paramExpr`:

```go
// rangeBound is a date range param's first (from) or last (to) day as a
// Date (F3). A range with no bounds (unset, All dates) has no such day: the
// value is blank, never an unset condition, so a comparison with it matches
// no row (R2).
func rangeBound(e expr, bound, at string) (expr, error) {
	if bound != "from" && bound != "to" {
		return expr{}, invalid(at, "bound is from or to")
	}
	if !e.rng {
		return expr{}, invalid(at, "bound reads a date range's start or end")
	}
	day := e.lo
	if bound == "to" {
		day = e.hi
	}
	return expr{sql: day, typ: Date}, nil
}
```

- [ ] **Step 5: Implement the bound and All dates in `formula.go` and the lookup**

In `expr`, after `lo, hi string`:

```go
	// all is a date range with no bounds (All dates): between it holds for
	// every row.
	all bool
```

In `compileNode`, after the `only lookup takes a source and a pick` check:

```go
	if n.Bound != "" && n.Param == "" {
		return expr{}, invalid(at, "bound goes with a param")
	}
```

and replace the param case of the leaf switch:

```go
	case n.Param != "":
		e, err := c.param(n.Param, at)
		if err != nil || n.Bound == "" {
			return e, err
		}
		return rangeBound(e, n.Bound, at)
```

In `between`, after the `a date range holds dates` check:

```go
		if args[1].all {
			// All dates: every row, a row with no day too (R1).
			return expr{sql: "1", typ: Bool}, nil
		}
```

In `lookup.go` `lookupIn`, the leaf check (line 96) also refuses a bound on the lookup node:

```go
	if n.Col != "" || n.Path != nil || n.Outer != nil || n.Param != "" || n.Subtotal != "" || n.Record != "" || n.Bound != "" {
		return expr{}, invalid(at, errNodeKind)
	}
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'RangePresets|ResolveParams|Bound|AllDates|FormulaCases|Between' ./reports/`
Expected: PASS.

- [ ] **Step 7: Run the whole reports suite, the forms tables and gofmt**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go test -tags no_ui -count=1 ./reports/ ./forms/
cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run core/lib/forms/__tests__/formula-cases.test.ts
```
Expected: `gofmt -l` prints nothing; both Go packages `ok`; the vitest file passes.

- [ ] **Step 8: Hand over for commit**

The driver commits:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/params.go core/server/reports/formula.go core/server/reports/lookup.go core/server/reports/params_test.go core/server/reports/formula_cases_test.go core/server/reports/formula_run_test.go core/lib/forms/fixtures/formula-cases.json
git commit -m "core(reports): date range bounds and the All dates preset"
```

---

### Task 5 — F6: rollup formulas (Go)

Engine facts used: `levelCtx.col` picks `rowsTotal` or `recompute` (`levels.go:634-643`); `recompute` enters `visiting` and blanks a formula that reads a row value (`levels.go:742-760`); `readsRowValue` (`levels.go:874-921`) is NULL for a record leaf above its group (`:883-886`) and a bare path (`:887-889`), and walks a formula column's formula (`:890-913`); `detailCell` compiles a column's detail-row value with a `rowsCtx` (`levels.go:683-700`); `combined` refuses amounts of two currencies (`formula.go:95-116`); `lv.cell` is every level's cell (`levels.go:179-182`), compiled by `groupStatement` for levels 0…N, which also records `c.levelTypes` (`run.go:490-511`); `compileDry` compiles the group statement, so Validate reaches level cells (`run.go:1048-1066`); `binder.columns` (`bind.go:274-308`, `cp.nest` set by `pathColumn` `:356-388`); the pivot cell check (`bind.go:772-775`); subtotal rows carry the group row's cells (`assemble.go:325-329`).

**Files:**
- Modify: `core/server/reports/levels.go` (`levelCtx.col`, `levelCtx.rollup`, `level.checkInColumn`, `readsRowValue`)
- Modify: `core/server/reports/bind.go:274-308,772-775` (rollup refusals)
- Create: `core/server/reports/rollup_test.go`

**Interfaces:**
- Consumes: `Column.Rollup` (Task 1); `pick: "count"`, `"sum"` (Task 2); `currency.same` true for two reads of one currency field, and `fieldGuard` (Task 2b, R24), so `checkInColumn`'s `combined([]expr{d, e}, at)` accepts a rollup that adds a lookup of the column's field to its total — no change here for that.
- Produces: `func (lc *levelCtx) rollup(cp *colPlan) (expr, error)`; `func (lv *level) checkInColumn(cp *colPlan, e expr, at, what string) error` (a method on `*level`, as it compiles the column's detail cell; Task 6 calls it with `what` = `"an opening cell"`; messages `"<what> shows in its column: it must be <type>"` and `"<what>'s amount is in another currency than its column's"`); bind refusals at `columns.N.rollup`: `a rollup replaces the column's totals: it does not go with total "rows"`, `a child row column has no totals to replace`, `a pivot's cell column is totalled by its pivot columns`.

- [ ] **Step 1: Write the failing Go tests**

Create `core/server/reports/rollup_test.go`:

```go
package reports

import "testing"

// plus shows a feeding's cost on detail rows and, on group, subtotal and
// total rows, its rollup: the cost added up, plus 1. again reads plus, so on
// those rows it reads the rollup too. Without the rollup plus is blank there
// (it reads a bare path).
func TestARollupShowsOnGroupSubtotalAndTotalRows(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"plus","formula":{"op":"add","args":[{"path":[{"field":"cost"}]},{"value":0}]},
			"rollup":{"op":"add","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1}]}},
		{"id":"again","formula":{"op":"add","args":[{"col":"plus"},{"value":0}]}}],
		"groups":[{"id":"g","col":"animal","subtotals":true}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "subtotal", "group", "detail", "detail", "subtotal", "total")
	requireOrderedRows(t, spec, res, "cost", int64(3500), int64(1000), int64(2500), int64(3500), int64(1000), int64(700), int64(300), int64(1000), int64(4500))
	requireOrderedRows(t, spec, res, "plus", int64(3501), int64(1000), int64(2500), int64(3501), int64(1001), int64(700), int64(300), int64(1001), int64(4501))
	requireOrderedRows(t, spec, res, "again", int64(3501), int64(1000), int64(2500), int64(3501), int64(1001), int64(700), int64(300), int64(1001), int64(4501))
	requireRows(t, res, "plus@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
}

// A rollup that reads the group's record has no record on the total row,
// so it is blank there as a whole (R6).
func TestARollupThatReadsARecordIsBlankOnTheTotal(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum",
			"rollup":{"op":"add","args":[{"record":"g","path":[{"field":"weight"}]},{"op":"sum","args":[{"path":[{"field":"grams"}]}]}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	// Leo weighs 190 (500 + 800 grams), Zed 300 (300 + 100 grams).
	requireOrderedRows(t, spec, res, "grams", int64(1490), int64(500), int64(800), int64(700), int64(300), int64(100), nil)
}

func TestARollupReadsLookupsAndParams(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","params":[{"id":"extra","type":"number"}],"columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum",
			"rollup":{"op":"add","args":[{"op":"sum","args":[{"path":[{"field":"grams"}]}]},{"param":"extra"},
				{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},
					"value":{"path":[{"field":"weight"}]}}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, map[string]string{"extra": `5`})
	// Each group and the total: its grams, 5, and Leo's 190.
	requireOrderedRows(t, spec, res, "grams", int64(1495), int64(500), int64(800), int64(595), int64(300), int64(100), int64(1895))
}

// A rollup adds a looked-up cost to the group's total cost: both read
// zoo.feedings.cost, whose currency is each feeding's own, so they combine
// (R24) and the rollup passes checkInColumn. f1 cost 1000; Leo's feedings
// total 3500, Zed's 1000, all 4500.
func TestARollupAddsALookedUpAmountOfItsColumnsField(t *testing.T) {
	z := newZoo(t)
	const spec = `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum",
			"rollup":{"op":"add","args":[
				{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings","pick":"sum",
					"where":{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f1"}]},
					"value":{"path":[{"field":"cost"}]}},{"value":0}]},
				{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
		"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	requireOrderedRows(t, spec, res, "cost", int64(4500), int64(1000), int64(2500), int64(2000), int64(700), int64(300), int64(5500))
	requireRows(t, res, "cost@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
}

func TestARollupIsRefusedWithRowsTotals(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum",
		"total":"rows","rollup":{"op":"sum","args":[{"path":[{"field":"cost"}]}]}}]}`))
	requireSpecError(t, err, 400, "columns.0.rollup", `a rollup replaces the column's totals: it does not go with total "rows"`)
}

func TestARollupIsRefusedOnAChildRowOrPivotCellColumn(t *testing.T) {
	app := withZooRegistry(t)
	err := Validate(app, loadSpec(t, `{"version":3,"source":"zoo.animals","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"notes","path":[{"link":"feedings","mode":"nest"},{"field":"note"}],"rollup":{"value":null}}]}`))
	requireSpecError(t, err, 400, "columns.1.rollup", "a child row column has no totals to replace")

	err = Validate(app, loadSpec(t, `{"version":3,"source":"zoo.feedings","columns":[{"id":"month","path":[{"field":"fed"}]},
		{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum","rollup":{"op":"sum","args":[{"path":[{"field":"grams"}]}]}}],
		"groups":[{"id":"m","col":"month","bucket":"month"}],"pivot":{"path":[{"link":"animal"}],"cell":"grams"}}`))
	requireSpecError(t, err, 400, "columns.1.rollup", "a pivot's cell column is totalled by its pivot columns")
}

// A rollup shows in its column's cells: it has the column's type, and an
// amount is in the column's currency (prices are in the animals' fixed
// currency, costs in each feeding's own).
func TestARollupMustHaveItsColumnsTypeAndCurrency(t *testing.T) {
	app := withZooRegistry(t)
	cost := func(rollup string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum","rollup":` + rollup + `}]}`
	}
	err := Validate(app, loadSpec(t, cost(`{"value":"x"}`)))
	requireSpecError(t, err, 400, "columns.0.rollup", "a rollup shows in its column: it must be money")
	err = Validate(app, loadSpec(t, cost(`{"op":"lookup","source":"zoo.animals","pick":"sum","value":{"path":[{"field":"price"}]}}`)))
	requireSpecError(t, err, 400, "columns.0.rollup", "a rollup's amount is in another currency than its column's")
	if err := Validate(app, loadSpec(t, cost(`{"value":null}`))); err != nil {
		t.Fatalf("a blank rollup: %v", err)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Rollup' ./reports/`
Expected: FAIL: the rollup is ignored, so `plus = []interface {}{interface {}(nil), int64(1000), …}` (the group cell is blank: plus reads a bare path), `grams = []interface {}{int64(1300), …}`, `cost = []interface {}{3500, 1000, 2500, 1000, 700, 300, 4500}` (the group rows show the plain total), and every refusal test fails with `err = <nil>, want a *SpecError`.

- [ ] **Step 3: Refuse a rollup where it cannot show**

In `bind.go` `binder.columns`, after the `if c.Path != nil { … } else if …` block (line 306):

```go
		if c.Rollup != nil {
			switch {
			case c.Total == TotalRows:
				b.bad(at+".rollup", `a rollup replaces the column's totals: it does not go with total "rows"`)
			case cp.nest > 0:
				b.bad(at+".rollup", "a child row column has no totals to replace")
			}
		}
```

In `groupsAndPivot`, after the pivot cell check (line 775):

```go
	if pp.cell != nil && pp.cell.col.Rollup != nil {
		b.bad(pp.cell.at+".rollup", "a pivot's cell column is totalled by its pivot columns")
	}
```

- [ ] **Step 4: Compile the rollup on each level**

In `levels.go`, replace `levelCtx.col`:

```go
func (lc *levelCtx) col(id, at string) (expr, error) {
	cp := lc.lv.c.p.byID[id]
	if cp == nil {
		return expr{}, unknownColumn(at, id)
	}
	if cp.col.Rollup != nil {
		return lc.rollup(cp)
	}
	if cp.col.Total == TotalRows {
		return lc.rowsTotal(cp)
	}
	return lc.recompute(cp, at)
}

// rollup is a column's cell on this level from its rollup formula (F6, R6),
// in place of its total. A rollup that reads a value the level's rows have
// none of (a record of a group the row is not inside, a bare path) is blank
// as a whole, as a column's formula is.
func (lc *levelCtx) rollup(cp *colPlan) (expr, error) {
	lv := lc.lv
	at := cp.at + ".rollup"
	done, err := enter(lc.visiting, cp.col.ID, at)
	if err != nil {
		return expr{}, err
	}
	defer done()
	e, err := compileFormula(&levelCtx{lv: lv, visiting: lc.visiting}, cp.col.Rollup, at)
	if err != nil {
		return expr{}, err
	}
	if err := lv.checkInColumn(cp, e, at, "a rollup"); err != nil {
		return expr{}, err
	}
	if lv.readsRowValue(cp.col.Rollup, false, map[string]bool{}) {
		e.sql, e.mixed = "NULL", ""
	}
	return e, nil
}

// checkInColumn refuses a level formula drawn in a column's cell (a rollup,
// an opening cell; what names it) whose value the column cannot show: its
// type is the column's detail type (a blank value fits any), and an amount
// is in the column's currency.
func (lv *level) checkInColumn(cp *colPlan, e expr, at, what string) error {
	d, err := (&rowsCtx{r: lv.r, visiting: map[string]bool{}, detail: &windowFrame{}}).col(cp.col.ID, cp.at)
	if err != nil {
		return err
	}
	if e.typ != "" && d.typ != "" && e.typ != d.typ {
		return invalid(at, fmt.Sprintf("%s shows in its column: it must be %s", what, d.typ))
	}
	if e.typ == Money {
		if _, err := combined([]expr{d, e}, at); err != nil {
			return invalid(at, what+"'s amount is in another currency than its column's")
		}
	}
	return nil
}
```

In `readsRowValue`, a column with a rollup is read as its rollup on a level's rows. Replace the start of the column branch (lines 890-892):

```go
	if n.Col != "" && !inAggregate && !seen[n.Col] {
		cp := lv.c.p.byID[n.Col]
		// A column with a rollup shows the rollup on the level's rows.
		if cp != nil && cp.col.Rollup != nil {
			seen[n.Col] = true
			defer delete(seen, n.Col)
			return lv.readsRowValue(cp.col.Rollup, false, seen)
		}
```

(the rest of the branch, from the `// A "rows" column adds the rows under each group` comment, is unchanged).

- [ ] **Step 5: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Rollup|V3' ./reports/`
Expected: PASS.

- [ ] **Step 6: Run the whole reports suite and gofmt**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l reports && go test -tags no_ui -count=1 ./reports/`
Expected: `gofmt -l` prints nothing; `ok  	tinycld.org/core/reports`.

- [ ] **Step 7: Hand over for commit**

The driver commits:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports/levels.go core/server/reports/bind.go core/server/reports/rollup_test.go
git commit -m "core(reports): a column's rollup formula on group, subtotal and total rows"
```

### Task 6: F5 — opening rows (Go)

Engine facts used: `groupStatement` builds one leg per level with the cells `v<idx>`, the pivot cells, the label cells and the carried currencies, in that order (`run.go:490-541`); `unreadHelpers` drops a helper cell that is `NULL` in every leg (`run.go:584-609`, regex `helperCell`); a computed layout leg lists the same cells (`layout.go:61-101`); `levelCtx` compiles on a level (`levels.go:625-1021`) and `readsRowValue` says a formula has no value on the level (`levels.go:874`); `groupWithLabel` decides `canOpen` and emits group and subtotal rows (`assemble.go:297-331`), and layout sections call it with `n = 1` (`layout.go:394`); `binder.groupsAndPivot` (`bind.go:713-777`); `binder.column` merges `unknownColumn` (422 `unknown_column`, "the report has no column %q", `join.go:29`). The zoo (`zoo_test.go`): ann reads f1 (leo, cost 1000, grams 500, 2026-01-05), f2 (leo, 2500, 800, 2026-02-10), f3 (zed, 700, 300, 2026-01-20), f4 (zed, 300, 100, 2026-03-03); `due` is the same day as a text day; `withEmptyAnimals` (`fill_test.go:10`) adds Lia and Eli, active, with no feedings.

**Files:**
- Modify: `core/server/reports/bind.go` (`groupsAndPivot`: call `b.opening`; new `binder.opening`; pivot's last level)
- Modify: `core/server/reports/levels.go` (new `level.openingCells`)
- Modify: `core/server/reports/run.go` (`groupStatement` appends the opening cells; `helperCell`; new `carriedSQL`)
- Modify: `core/server/reports/layout.go:94-97` (computed legs carry `NULL` opening cells)
- Modify: `core/server/reports/assemble.go` (`groupWithLabel` emits the opening row and lets the level open; new `runner.openingCells`, `runner.openingMixed`)
- Create: `core/server/reports/opening_test.go`

**Interfaces:**
- Consumes: `Group.Opening *Opening`, `Opening{Label string; Cells map[string]*Node}`, `KindOpening` (Task 1); pick `"sum"` and `expr.mixed` (Task 2); `{ "param", "bound" }` (Task 4); `func (lv *level) checkInColumn(cp *colPlan, e expr, at, what string) error` (Task 5: refuses a value whose type is not the column's detail type, "<what> shows in its column: it must be %s", or whose amount does not combine with the column's currency, "<what>'s amount is in another currency than its column's"; allows an untyped `{ "value": null }`); `currency.same` true for two reads of one currency field (Task 2b, R24), so an opening cell that looks up the column's own field passes `checkInColumn`.
- Produces: `(lv *level) openingCells() ([]string, error)` — cells `op<idx>`, `opcur<idx>`, `opmx<idx>` on every level leg; `carriedSQL(s *stmt, e expr) string`; `(rn *runner) openingCells(row map[string]any, opening *Opening) map[string]any`; `(rn *runner) openingMixed(groupRows []map[string]any) []string`; result rows `Row{ID: "opening:" + keyString(key), Kind: KindOpening, Depth: n, Label: opening.Label, Section: <group row id>, GroupKey: key, Cells}` right after an expanded group row of a level with `opening`. Task 8's page window skips the opening row of the cursor's group.

- [ ] **Step 1: Write the failing Go tests**

Create `core/server/reports/opening_test.go`:

```go
package reports

import (
	"slices"
	"strings"
	"testing"
)

// beforePeriod is the grams of this group's animal fed before the period: a
// sum lookup read on the group's level (its record), cut at the period's start.
const beforePeriod = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings",
	"where":{"op":"and","args":[
		{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
		{"op":"lt","args":[{"path":[{"field":"due"}]},{"param":"period","bound":"from"}]}]},
	"value":{"path":[{"field":"grams"}]},"pick":"sum"},{"value":0}]}`

// gramsByAnimal groups ann's feedings in the period by animal, each animal's
// group opening onto the grams fed before the period. group adds group parts
// (fill, having) after the opening.
func gramsByAnimal(cells, group string) string {
	return `{"version":3,"source":"zoo.feedings","params":[{"id":"period","type":"dateRange"}],
		"columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"day","path":[{"field":"due"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"filter":{"op":"between","args":[{"path":[{"field":"due"}]},{"param":"period"}]},
		"groups":[{"id":"g","col":"animal","open":true,"subtotals":true,
			"opening":{"label":"Before the period","cells":` + cells + `}` + group + `}],
		"sort":[{"col":"note"}]}`
}

var fromFebruary = map[string]string{"period": `{"from":"2026-02-01","to":"2026-12-31"}`}

// rowLabelled is the first row with the label, failing the test without one.
func rowLabelled(t *testing.T, res *Result, label string) (int, Row) {
	t.Helper()
	i := slices.IndexFunc(res.Rows, func(r Row) bool { return r.Label == label })
	if i < 0 {
		t.Fatalf("no row %q in %v", label, rowsOf(res, "label"))
	}
	return i, res.Rows[i]
}

func TestAnOpeningRowIsFirstUnderEachOpenGroup(t *testing.T) {
	z := newZoo(t)
	spec := gramsByAnimal(`{"grams":`+beforePeriod+`}`, "")
	res := z.run(t, "ann", spec, Options{}, fromFebruary)
	requireRows(t, res, "kind", "group", "opening", "detail", "subtotal", "group", "opening", "detail", "subtotal", "total")
	requireOrderedRows(t, spec, res, "note", nil, nil, "f2", nil, nil, nil, "f4", nil, nil)
	// Leo: f1 (500) is before the period; Zed: f3 (300).
	requireOrderedRows(t, spec, res, "grams", int64(800), int64(500), int64(800), int64(800), int64(100), int64(300), int64(100), int64(100), int64(900))
	for _, i := range []int{1, 5} {
		group, opening := res.Rows[i-1], res.Rows[i]
		want := "opening:" + keyString(group.GroupKey)
		if opening.ID != want || opening.Depth != 1 || opening.Section != group.ID || opening.Label != "Before the period" ||
			keyString(opening.GroupKey) != keyString(group.GroupKey) {
			t.Fatalf("opening row %d = %+v, want id %s under %s", i, opening, want, group.ID)
		}
	}
}

func TestAClosedGroupHasNoOpeningRow(t *testing.T) {
	z := newZoo(t)
	spec := gramsByAnimal(`{"grams":`+beforePeriod+`}`, "")
	closed := z.run(t, "ann", spec, Options{Expand: [][]any{}}, fromFebruary)
	requireRows(t, closed, "kind", "group", "group", "total")
	_, zed := rowLabelled(t, closed, "Zed")
	open := z.run(t, "ann", spec, Options{Expand: [][]any{zed.GroupKey}}, fromFebruary)
	requireRows(t, open, "kind", "group", "group", "opening", "detail", "subtotal", "total")
	requireRows(t, open, "label", "Leo", "Zed", "Before the period", "Zed", "Total Zed", "Total")
}

// A fill record has no rows in the period, yet its group opens onto its
// opening row and subtotal (R5d). Kit's feeding f6 is not one ann may read,
// so the lookup finds nothing and the opening shows 0.
func TestAFillGroupWithNoRowsOpensOntoItsOpeningRow(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	fill := `,"fill":{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}`
	res := z.run(t, "ann", gramsByAnimal(`{"grams":`+beforePeriod+`}`, fill), Options{}, fromFebruary)
	i, kit := rowLabelled(t, res, "Kit")
	if !kit.CanExpand || !kit.Expanded {
		t.Fatalf("Kit = %+v", kit)
	}
	if got := []string{res.Rows[i+1].Kind, res.Rows[i+2].Kind}; !slices.Equal(got, []string{KindOpening, KindSubtotal}) {
		t.Fatalf("under Kit: %v", got)
	}
	if res.Rows[i+1].Cells["grams"] != int64(0) || res.Rows[i+2].Label != "Total Kit" {
		t.Fatalf("Kit's opening = %+v, then %+v", res.Rows[i+1], res.Rows[i+2])
	}
	_, leo := rowLabelled(t, res, "Leo")
	if leo.Kind != KindGroup {
		t.Fatalf("Leo = %+v", leo)
	}
}

// Zed's 100 grams in the period fail the having: Zed's group is hidden, and
// with it its opening row.
func TestAGroupItsHavingHidesHasNoOpeningRow(t *testing.T) {
	z := newZoo(t)
	having := `,"having":{"op":"gte","args":[{"op":"sum","args":[{"path":[{"field":"grams"}]}]},{"value":500}]}`
	res := z.run(t, "ann", gramsByAnimal(`{"grams":`+beforePeriod+`}`, having), Options{}, fromFebruary)
	requireRows(t, res, "kind", "group", "opening", "detail", "subtotal", "total")
	requireRows(t, res, "label", "Leo", "Before the period", "Leo", "Total Leo", "Total")
}

// An opening cell reads the group's record, a lookup and a param; a column the
// opening names no cell for is blank.
func TestOpeningCellsReadARecordALookupAndAParam(t *testing.T) {
	z := newZoo(t)
	cells := `{"note":{"record":"g","path":[{"field":"name"}]},
		"day":{"param":"period","bound":"from"},
		"grams":` + beforePeriod + `}`
	res := z.run(t, "ann", gramsByAnimal(cells, ""), Options{}, fromFebruary)
	opening := res.Rows[1]
	if opening.Kind != KindOpening {
		t.Fatalf("row 1 = %+v", opening)
	}
	if opening.Cells["note"] != "Leo" || opening.Cells["day"] != "2026-02-01" || opening.Cells["grams"] != int64(500) {
		t.Fatalf("cells = %#v", opening.Cells)
	}
	if _, ok := opening.Cells["cost"]; ok {
		t.Fatalf("cost has a cell: %#v", opening.Cells)
	}
}

// A sum lookup that meets two currencies has no value in an opening cell, and
// the run says so (R3).
func TestAnOpeningSumOverTwoCurrenciesIsBlankWithANotice(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{
		"animal": z.id("leo"), "cost": 400, "currency": "EUR", "fed": "2026-03-10 00:00:00.000Z", "grams": 10, "note": "f7",
	})
	const costs = `{"op":"lookup","source":"zoo.feedings",
		"where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
		"value":{"path":[{"field":"cost"}]},"pick":"sum"}`
	spec := `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"bal","formula":` + costs + `}],
		"groups":[{"id":"g","col":"animal","open":true,"opening":{"label":"All feedings","cells":{"bal":` + costs + `}}}],
		"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	leo, zed := res.Rows[1], res.Rows[slices.IndexFunc(res.Rows, func(r Row) bool { return r.Label == "Zed" })+1]
	if leo.Kind != KindOpening || leo.Cells["bal"] != nil {
		t.Fatalf("Leo's opening = %+v", leo)
	}
	if zed.Kind != KindOpening || zed.Cells["bal"] != int64(1000) {
		t.Fatalf("Zed's opening = %+v", zed)
	}
	if !slices.ContainsFunc(res.Notices, func(n Notice) bool { return n.Code == "mixedCurrency" && slices.Contains(n.Columns, "bal") }) {
		t.Fatalf("notices = %+v", res.Notices)
	}
}

// An opening cell in the cost column adds up the animal's costs before the
// period: a sum lookup of zoo.feedings.cost, whose currency each feeding
// carries, combines with the column's cost (R24) and carries its currency.
func TestAnOpeningCellAddsUpItsColumnsAmounts(t *testing.T) {
	z := newZoo(t)
	const costBefore = `{"op":"coalesce","args":[{"op":"lookup","source":"zoo.feedings",
		"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
			{"op":"lt","args":[{"path":[{"field":"due"}]},{"param":"period","bound":"from"}]}]},
		"value":{"path":[{"field":"cost"}]},"pick":"sum"},{"value":0}]}`
	spec := gramsByAnimal(`{"cost":`+costBefore+`}`, "")
	res := z.run(t, "ann", spec, Options{}, fromFebruary)
	requireRows(t, res, "kind", "group", "opening", "detail", "subtotal", "group", "opening", "detail", "subtotal", "total")
	// Leo: f1 (1000) is before the period, f2 (2500) in it; Zed: f3 (700), f4 (300).
	requireOrderedRows(t, spec, res, "cost", int64(2500), int64(1000), int64(2500), int64(2500), int64(300), int64(700), int64(300), int64(300), int64(2800))
	requireRows(t, res, "cost@currency", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD", "USD")
}

func TestOpeningIsRefusedWhereItCannotShow(t *testing.T) {
	app := withZooRegistry(t)
	withOpening := func(opening string) string {
		return `{"version":3,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"note","path":[{"field":"note"}]},
			{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"animal","opening":` + opening + `}]}`
	}
	cases := map[string]struct {
		spec       string
		status     int
		at, want   string
	}{
		"the first column": {withOpening(`{"label":"Start","cells":{"animal":{"value":null}}}`), 400,
			"groups.0.opening.cells.animal", "the opening row's label fills the first column"},
		"an unknown column": {withOpening(`{"label":"Start","cells":{"nosuch":{"value":1}}}`), 422,
			"groups.0.opening.cells.nosuch", `the report has no column "nosuch"`},
		"another type": {withOpening(`{"label":"Start","cells":{"note":{"value":5}}}`), 400,
			"groups.0.opening.cells.note", "an opening cell shows in its column: it must be"},
		// The animal's fee carries its fee_currency; cost carries its row's.
		"another currency": {withOpening(`{"label":"Start","cells":{"cost":{"record":"g","path":[{"field":"fee"}]}}}`), 400,
			"groups.0.opening.cells.cost", "an opening cell's amount is in another currency than its column's"},
		"no label": {withOpening(`{"label":""}`), 400,
			"groups.0.opening.label", "an opening row has a label of 1 to 200 characters"},
		"a long label": {withOpening(`{"label":"` + strings.Repeat("x", 201) + `"}`), 400,
			"groups.0.opening.label", "an opening row has a label of 1 to 200 characters"},
		"a pivot's last level": {`{"version":3,"source":"zoo.feedings","columns":[
			{"id":"animal","path":[{"link":"animal"}]},
			{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"animal","opening":{"label":"Start"}}],
			"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"grams"}}`, 400,
			"groups.0.opening", "this level never opens, so it has no opening row"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			requireSpecError(t, Validate(app, loadSpec(t, c.spec)), c.status, c.at, c.want)
		})
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Opening' ./reports/`
Expected: FAIL. `TestAnOpeningRowIsFirstUnderEachOpenGroup` and `TestAnOpeningCellAddsUpItsColumnsAmounts`: `kind = []interface {}{"group", "detail", "subtotal", "group", "detail", "subtotal", "total"}` (no `opening` row: Task 1 decodes `opening`, nothing reads it); the refusal cases: `err = <nil>, want a *SpecError`.

- [ ] **Step 3: Check the opening at bind time**

In `bind.go`, add `"maps"` to the imports. In `groupsAndPivot`, after `p.groups = append(p.groups, &groupPlan{group: g, col: cp, at: at})`:

```go
		if g.Opening != nil {
			b.opening(p, g.Opening, at+".opening")
		}
```

At the end of `groupsAndPivot`, after `p.pivot = pp`:

```go
	// A pivot's innermost level never opens (its groups are the pivot's rows),
	// so an opening row there could never show (R5e).
	if last := len(spec.Groups) - 1; last >= 0 && spec.Groups[last].Opening != nil {
		b.bad(fmt.Sprintf("groups.%d.opening", last), "this level never opens, so it has no opening row")
	}
```

Add after `groupsAndPivot`:

```go
// opening checks a group's opening row: a label of 1 to maxLabel characters,
// and cells only in the report's columns after the first, whose cell the
// label fills. A cell's type and currency are checked where it compiles, on
// its level (level.openingCells).
func (b *binder) opening(p *plan, op *Opening, at string) {
	if n := len(op.Label); n < 1 || n > maxLabel {
		b.bad(at+".label", fmt.Sprintf("an opening row has a label of 1 to %d characters", maxLabel))
	}
	for _, id := range slices.Sorted(maps.Keys(op.Cells)) {
		cellAt := at + ".cells." + id
		cp := b.column(p, id, cellAt)
		if cp != nil && cp == p.cols[0] {
			b.bad(cellAt, "the opening row's label fills the first column")
		}
	}
}
```

- [ ] **Step 4: Compile the opening cells on their level**

In `run.go`, after `noCarriedCell`:

```go
// carriedSQL is the currency a value carries on a row (a record's or a
// looked-up row's own, or an amount param's code), "NULL" when it carries
// none.
func carriedSQL(s *stmt, e expr) string {
	switch {
	case e.cur.param != "":
		return s.bind(e.cur.param)
	case e.cur.sql != "":
		return e.cur.sql
	}
	return "NULL"
}
```

In `levels.go`, after `level.own`:

```go
// openingCells are the level's opening row cells, three per column in column
// order: op<idx>, the opening's formula for that column compiled on this level
// (R5a); opcur<idx>, the currency that value carries; opmx<idx>, 1 where a sum
// lookup in it met two currencies (R3). A column the opening names no cell
// for, and every column of a level with no opening, has NULL in all three, so
// each leg of the group statement has the same cells.
func (lv *level) openingCells() ([]string, error) {
	var opening *Opening
	at := ""
	if lv.n >= 1 {
		g := lv.c.p.groups[lv.n-1]
		opening, at = g.group.Opening, g.at+".opening.cells."
	}
	out := make([]string, 0, 3*len(lv.c.p.cols))
	for idx, cp := range lv.c.p.cols {
		var node *Node
		if opening != nil {
			node = opening.Cells[cp.col.ID]
		}
		if node == nil {
			out = append(out, fmt.Sprintf("NULL AS op%d", idx), fmt.Sprintf("NULL AS opcur%d", idx), fmt.Sprintf("NULL AS opmx%d", idx))
			continue
		}
		cellAt := at + cp.col.ID
		e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting}, node, cellAt)
		if err != nil {
			return nil, err
		}
		if err := lv.checkInColumn(cp, e, cellAt, "an opening cell"); err != nil {
			return nil, err
		}
		value, cur, mixed := cellSQL(e), carriedSQL(lv.r.s, e), "NULL"
		if e.mixed != "" {
			mixed = e.mixed
		}
		// A cell that reads a value of one row (a path, a record of a group
		// inside this one) has none on a group row, as a rollup has none (R6).
		if lv.readsRowValue(node, false, map[string]bool{}) {
			value, cur, mixed = "NULL", "NULL", "NULL"
		}
		out = append(out, fmt.Sprintf("%s AS op%d", value, idx), fmt.Sprintf("%s AS opcur%d", cur, idx), fmt.Sprintf("%s AS opmx%d", mixed, idx))
	}
	return out, nil
}
```

In `run.go` `groupStatement`, replace

```go
		// The labels, then the carried currencies, follow the pivot cells, as in
		// every leg of the union.
		cells = append(append(append(cells, pivotCells...), labels...), carried...)
```

with

```go
		opening, err := lv.openingCells()
		if err != nil {
			return nil, "", err
		}
		// The labels, the carried currencies, then the opening cells follow the
		// pivot cells, as in every leg of the union.
		cells = append(append(append(append(cells, pivotCells...), labels...), carried...), opening...)
```

Extend `helperCell` so a report with no opening sends no opening cells: add `op|opcur|opmx` to its alternation (after Task 2 it reads `^(?:vl|vc|vcur|vmx)\d+$`; it becomes `^(?:vl|vc|vcur|vmx|op|opcur|opmx)\d+$`), and its comment gains "an opening row's cells (op<idx>, opcur<idx>, opmx<idx>)".

In `layout.go` `layoutLegs`, after `cells = append(cells, carried...)`:

```go
		// A computed row has no opening row: its opening cells are blank.
		for idx := range c.p.cols {
			cells = append(cells, fmt.Sprintf("NULL AS op%d", idx), fmt.Sprintf("NULL AS opcur%d", idx), fmt.Sprintf("NULL AS opmx%d", idx))
		}
```

- [ ] **Step 5: Emit the opening row**

In `assemble.go`, replace the start of `groupWithLabel` up to and including the `rn.push(Row{ … KindGroup … })` call with:

```go
func (rn *runner) groupWithLabel(t *tree, row map[string]any, n int, section, id string, label rowLabel, key []any, subtotal bool) {
	p := rn.c.p
	opening := p.groups[n-1].group.Opening
	canOpen := n < len(p.groups) || p.pivot == nil
	// A level with an opening row opens even with nothing under it, so the
	// opening row and the subtotal show (R5d). Without one: a fill record has no
	// base rows and opens only onto the next level's fill records (a layout
	// section with no rows has no __n and keeps this rule), and a group whose
	// child groups the having hides, every one, opens onto nothing.
	if opening == nil {
		if v, ok := row["__n"]; ok && v != nil && toInt(v) == 0 && !rn.c.nextFill(n) {
			canOpen = false
		}
		if v, ok := row["__kids"]; ok && v != nil && toInt(v) == 0 {
			canOpen = false
		}
	}
	expanded := canOpen && rn.expand.has(key, n)
	rn.push(Row{
		ID: id, Kind: KindGroup, Depth: n - 1, Label: label.text, Section: section, GroupKey: key, Expanded: expanded, CanExpand: canOpen, Cells: rn.cells(row),
		LabelType: label.typ, LabelValue: label.value, LabelCurrency: label.currency,
	})
	if expanded && opening != nil {
		rn.push(Row{ID: "opening:" + keyString(key), Kind: KindOpening, Depth: n, Label: opening.Label, Section: id, GroupKey: key, Cells: rn.openingCells(row, opening)})
	}
```

(the `if expanded { … }` children block and the subtotal block that follow stay as they are).

Add after `detailCells`:

```go
// openingCells are an opening row's cells: each column its level's opening
// names, from the group row's op<idx>, typed as the column's detail cells, and
// a money cell's currency as a group row's (cells): the one it carries, else
// the column's currency key or fixed code. A column it names no cell for has
// none, so it is blank.
func (rn *runner) openingCells(row map[string]any, opening *Opening) map[string]any {
	out := map[string]any{}
	for idx, cp := range rn.c.p.cols {
		id := cp.col.ID
		if opening.Cells[id] == nil {
			continue
		}
		out[id] = rn.cellValue(cp, row[fmt.Sprintf("op%d", idx)], false)
		cur := rn.c.currencyOf[id]
		switch v := row[fmt.Sprintf("opcur%d", idx)]; {
		case v != nil:
			out[id+"@currency"] = v
		case cur.key != "":
			out[id+"@currency"] = row[cur.key]
		case cur.fixed != nil && rn.c.carried[id]:
			out[id+"@currency"] = rn.currencyOf(row, cur)
		}
	}
	return out
}

// openingMixed lists, in column order, the columns whose opening cell met two
// currencies in a sum lookup (opmx<idx> is 1) on a group row that opens.
func (rn *runner) openingMixed(groupRows []map[string]any) []string {
	var out []string
	for idx, cp := range rn.c.p.cols {
		name := fmt.Sprintf("opmx%d", idx)
		if slices.ContainsFunc(groupRows, func(row map[string]any) bool {
			n := int(toInt(row["__level"]))
			return row["__row"] == nil && n >= 1 && toInt(row[name]) == 1 && rn.expand.has(rn.rowKey(row, n), n)
		}) {
			out = append(out, cp.col.ID)
		}
	}
	return out
}
```

Add `"slices"` to the `assemble.go` imports. Then, where Task 2 collects the columns whose `vmx<idx>` is 1 for the lookup `mixedCurrency` notice (R3), add each id of `rn.openingMixed(groupRows)` that the list does not hold yet, before the notice is built, so a column named by both a cell and an opening cell is named once and the notice stays one notice.

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Opening' ./reports/`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l ./reports/ && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt output; PASS (every report test: a report with no opening sends no `op*` cells, `unreadHelpers`).

- [ ] **Step 7: Commit** (the driver runs it)

```bash
git add core/server/reports/bind.go core/server/reports/levels.go core/server/reports/run.go core/server/reports/layout.go core/server/reports/assemble.go core/server/reports/opening_test.go
git commit -m "feat(reports): opening rows under open groups"
```

---

### Task 7: F8 — viewer sort (Go)

Engine facts used: `Run` builds the compiler with `readerCompiler` (`run.go:204-242`); `detailSelect` compiles the spec's sort keys into `order` (the statement's `ORDER BY` and the cursor) and `frame.order` (the window order) in one loop (`run.go:781-808`); `detailSortValue` refuses a running value with `notASortKey(s.at)` and names a key's cursor column (`run.go:742-761`); `fingerprint` hashes each key's `col`, direction and value kind (`paging.go:36-42`); `runner.columns` builds `ResultColumn` (`assemble.go:13-59`); a detail label is `Spec.Detail.Label` (`assemble.go:337-339`). Test helpers: `feedingsPaged` (ungrouped, sort `fed` desc, `running` = runningSum(cost), `paging_test.go:9`), `feedingsBySpecies` (grouped by species, subtotals, sort `note`, `groups_test.go:10`), `sitesWithAnimals` (nested child rows, `nest_test.go:10`).

**Files:**
- Modify: `core/server/reports/rows.go` (`compiler.viewSort`, `compiler.sortable`)
- Modify: `core/server/reports/run.go` (`Run` plans `options.sort`; new `viewSortPlans`; `detailSelect` puts viewer keys first and records `sortable`; `ResultColumn.Sortable`)
- Modify: `core/server/reports/assemble.go` (`columns` sets `Sortable`)
- Create: `core/server/reports/viewsort_test.go`

`paging.go` does not change in this task: a viewer key's cursor column is `view:<col>`, so the existing `fingerprint` already tells a viewer key from a spec key on the same column.

**Interfaces:**
- Consumes: `Options.Sort []ViewSort`, `ViewSort{Col, Dir string}` (Task 1).
- Produces: `viewSortPlans(c *compiler, keys []ViewSort) ([]sortPlan, error)`; `compiler.viewSort []sortPlan`; `ResultColumn.Sortable bool` (JSON `sortable`); viewer keys named `vs<i>` in the detail select and ordered before the spec's keys in `order` (Task 8 puts `d.__g` before both).

- [ ] **Step 1: Write the failing Go tests**

Create `core/server/reports/viewsort_test.go`:

```go
package reports

import (
	"reflect"
	"testing"
)

// detailCells lists one cell of the detail rows only, in order.
func detailCells(res *Result, what string) []any {
	var out []any
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			out = append(out, r.Cells[what])
		}
	}
	return out
}

func requireDetails(t *testing.T, res *Result, what string, want ...any) {
	t.Helper()
	if got := detailCells(res, what); !reflect.DeepEqual(got, want) {
		t.Fatalf("detail %s = %#v\nwant %#v", what, got, want)
	}
}

func byView(col, dir string) Options {
	return Options{ExpandAll: true, Sort: []ViewSort{{Col: col, Dir: dir}}}
}

// The viewer's sort orders detail rows inside their groups; the groups keep
// the report's order (zebra's 1000 is less than lion's 3500, and still follows).
func TestAViewerSortReordersDetailRowsInsideTheirGroups(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsBySpecies, byView("cost", "asc"), nil)
	requireRows(t, res, "label", "lion", "lion", "lion", "Total lion", "zebra", "zebra", "zebra", "Total zebra", "Total")
	requireOrderedRows(t, feedingsBySpecies, res, "note", nil, "f1", "f2", nil, nil, "f4", "f3", nil, nil)
}

// A running sum runs in the report's order (fed, newest first): f4 300,
// f2 2800, f3 3500, f1 4500, whatever order the viewer shows the rows in.
func TestRunningValuesKeepTheSpecsOrderUnderAViewerSort(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsPaged, Options{Sort: []ViewSort{{Col: "note", Dir: "asc"}}}, nil)
	requireDetails(t, res, "note", "f1", "f2", "f3", "f4")
	requireDetails(t, res, "running", int64(4500), int64(2800), int64(3500), int64(300))
}

// A report whose detail rows are labelled by a formula sorts by that label when
// the viewer sorts its first column (R7); without one, by the first column's
// record label (Zed, then Leo), the report's order after it.
func TestAViewerSortOnTheFirstColumnSortsByTheDetailLabel(t *testing.T) {
	z := newZoo(t)
	const base = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"fed","path":[{"field":"fed"}]}],"sort":[{"col":"fed"}]`
	labelled := z.run(t, "ann", base+`,"detail":{"label":{"path":[{"field":"note"}]}}}`, Options{Sort: []ViewSort{{Col: "animal", Dir: "desc"}}}, nil)
	requireDetails(t, labelled, "note", "f4", "f3", "f2", "f1")
	requireRows(t, labelled, "label", "f4", "f3", "f2", "f1", "Total")
	plain := z.run(t, "ann", base+`}`, Options{Sort: []ViewSort{{Col: "animal", Dir: "desc"}}}, nil)
	requireDetails(t, plain, "note", "f3", "f4", "f1", "f2")
}

func TestARunningValueIsNotAViewerSortKey(t *testing.T) {
	z := newZoo(t)
	_, err := z.try(t, "ann", feedingsPaged, Options{Sort: []ViewSort{{Col: "running", Dir: "asc"}}}, nil)
	requireSpecError(t, err, 400, "options.sort.0.col", "a running value or a percent is not a sort key")
}

func TestAViewerSortIsCheckedAtRunTime(t *testing.T) {
	z := newZoo(t)
	eleven := make([]ViewSort, 11)
	for i := range eleven {
		eleven[i] = ViewSort{Col: "note", Dir: "asc"}
	}
	const pivot = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`
	cases := map[string]struct {
		spec     string
		sort     []ViewSort
		status   int
		at, want string
	}{
		"an unknown column": {feedingsPaged, []ViewSort{{Col: "nosuch", Dir: "asc"}}, 422, "options.sort.0.col", `the report has no column "nosuch"`},
		"a bad direction":   {feedingsPaged, []ViewSort{{Col: "note", Dir: "up"}}, 400, "options.sort.0.dir", "dir is asc or desc"},
		"a column twice":    {feedingsPaged, []ViewSort{{Col: "note", Dir: "asc"}, {Col: "note", Dir: "desc"}}, 400, "options.sort.1.col", "a column sorts once"},
		"eleven keys":       {feedingsPaged, eleven, 400, "options.sort", "a run sorts by at most 10 columns"},
		"a child row":       {sitesWithAnimals, []ViewSort{{Col: "animal", Dir: "asc"}}, 400, "options.sort.0.col", "a child row column is not a sort key"},
		"a pivot report":    {pivot, []ViewSort{{Col: "species", Dir: "asc"}}, 400, "options.sort", "a pivot report has no opened rows to sort"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := z.try(t, "ann", c.spec, Options{Sort: c.sort}, nil)
			requireSpecError(t, err, c.status, c.at, c.want)
		})
	}
}

// A cursor is tied to the viewer's sort: another sort, or none, refuses it.
func TestACursorIsRefusedAfterTheViewerSortChanges(t *testing.T) {
	z := newZoo(t)
	desc := []ViewSort{{Col: "cost", Dir: "desc"}}
	first := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Sort: desc}, nil)
	requireDetails(t, first, "note", "f2", "f1")
	next := z.run(t, "ann", feedingsPaged, Options{Limit: 2, Sort: desc, Cursor: first.Page.Next}, nil)
	requireDetails(t, next, "note", "f3", "f4")
	for name, sort := range map[string][]ViewSort{"another sort": {{Col: "cost", Dir: "asc"}}, "no sort": nil} {
		_, err := z.try(t, "ann", feedingsPaged, Options{Limit: 2, Sort: sort, Cursor: first.Page.Next}, nil)
		if err == nil {
			t.Fatalf("%s: the cursor was taken", name)
		}
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
	}
}

func sortableOf(res *Result) map[string]bool {
	out := map[string]bool{}
	for _, c := range res.Columns {
		out[c.ID] = c.Sortable
	}
	return out
}

func TestResultColumnsSayWhichSort(t *testing.T) {
	z := newZoo(t)
	paged := z.run(t, "ann", feedingsPaged, Options{}, nil)
	if got, want := sortableOf(paged), map[string]bool{"note": true, "fed": true, "cost": true, "running": false}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sortable = %v, want %v", got, want)
	}
	nested := z.run(t, "ann", sitesWithAnimals, Options{}, nil)
	if got, want := sortableOf(nested), map[string]bool{"site": true, "animal": false, "owner": false, "fed": false}; !reflect.DeepEqual(got, want) {
		t.Fatalf("nested sortable = %v, want %v", got, want)
	}
	pivot := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"cost"}}`, Options{}, nil)
	for _, c := range pivot.Columns {
		if c.Sortable {
			t.Fatalf("a pivot report's column %s is sortable", c.ID)
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'ViewerSort|WhichSort' ./reports/`
Expected: FAIL to build: `c.Sortable undefined (type ResultColumn has no field or method Sortable)`. After Step 3's field alone, the behaviour tests fail: `detail note = []interface {}{"f1", "f2", "f3", "f4"}` with the spec's order where the viewer's was asked, and the refusal cases `err = <nil>`.

- [ ] **Step 3: Add the result flag and the compiler fields**

In `run.go` `ResultColumn`, after `LabelCurrencyCell`:

```go
	// Sortable says a run accepts the column as a viewer sort key
	// (options.sort): not a running value or a percent, not a child row
	// column, not in a pivot report; the first column by its detail label
	// when the report has one.
	Sortable bool `json:"sortable,omitempty"`
```

In `rows.go` `compiler`, after `detailLabel`:

```go
	// viewSort are the run's viewer sort keys (options.sort), which order
	// detail rows before the spec's keys and never the window order.
	viewSort []sortPlan
	// sortable holds the columns a viewer may sort by, as detailSelect
	// compiled them.
	sortable map[string]bool
```

- [ ] **Step 4: Plan the viewer keys at run time**

In `run.go` `Run`, after `p, params := c.p, c.params`:

```go
	if c.viewSort, err = viewSortPlans(c, req.Options.Sort); err != nil {
		return nil, err
	}
```

Add after `notASortKey`:

```go
// viewSortPlans checks a run's viewer sort (options.sort) and plans its keys.
// It is a run input, not a spec part: a link outlives the report's columns,
// so each key is checked against the report as it is now (R7). A key on the
// first column of a report with a detail label sorts by that label. A running
// value is refused where it compiles (detailSortValue).
func viewSortPlans(c *compiler, keys []ViewSort) ([]sortPlan, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if len(keys) > maxSort {
		return nil, invalid("options.sort", fmt.Sprintf("a run sorts by at most %d columns", maxSort))
	}
	if c.p.pivot != nil {
		return nil, invalid("options.sort", "a pivot report has no opened rows to sort")
	}
	seen := map[string]bool{}
	out := make([]sortPlan, 0, len(keys))
	for i, k := range keys {
		at := fmt.Sprintf("options.sort.%d", i)
		cp := c.p.byID[k.Col]
		switch {
		case cp == nil:
			return nil, unknownColumn(at+".col", k.Col)
		case seen[k.Col]:
			return nil, invalid(at+".col", "a column sorts once")
		case k.Dir != "" && k.Dir != "asc" && k.Dir != "desc":
			return nil, invalid(at+".dir", "dir is asc or desc")
		case cp.nest > 0:
			return nil, invalid(at+".col", "a child row column is not a sort key")
		}
		seen[k.Col] = true
		s := sortPlan{col: cp, at: at + ".col", desc: k.Dir == "desc"}
		if d := c.p.spec.Detail; cp == c.p.cols[0] && d != nil && d.Label != nil {
			s = sortPlan{formula: d.Label, at: at + ".col", desc: s.desc}
		}
		out = append(out, s)
	}
	return out, nil
}
```

- [ ] **Step 5: Order detail rows by the viewer keys first**

In `run.go` `detailSelect`, replace the loop `for i, s := range c.p.sort { … }` (`run.go:781-806`) with:

```go
	// addKey selects one sort key's values as name and orders by them. A spec
	// key also orders the window frame; a viewer key does not, so running
	// values keep the report's order (R7), and its cursor column is view:<col>.
	addKey := func(s sortPlan, name string, spec bool) error {
		e, col, err := detailSortValue(&rowsCtx{r: r, visiting: visiting, detail: frame}, s)
		if err != nil {
			return err
		}
		inFrame := func(parts ...string) {
			if spec {
				frame.order = append(frame.order, parts...)
			}
		}
		if !spec {
			col = "view:" + col
		}
		// A record sorts by its label's typed value, then by its id so equal
		// labels keep one order. A label key holds a value of the label
		// field's type or a no-name record's text, so its cursor value may be
		// either (typ "").
		if ref, ok := c.sortLabel(labels, s, e); ok {
			key := labelOrder(r.s, ref.value)
			sel = append(sel, key+" AS "+name)
			order = append(order, sortKey{name: name, desc: s.desc, col: col + "@label"})
			inFrame("("+key+" IS NULL)", key+dirOf(s.desc))
			name = name + "_id"
		} else if key, ok := shownOrder(r.s.bind, e); ok {
			sel = append(sel, key+" AS "+name)
			order = append(order, sortKey{name: name, desc: s.desc, col: col + "@shown", typ: Text})
			inFrame("("+key+" IS NULL)", key+dirOf(s.desc))
			name = name + "_value"
		}
		sel = append(sel, e.sql+" AS "+name)
		order = append(order, sortKey{name: name, desc: s.desc, col: col, typ: e.typ})
		inFrame("("+e.sql+" IS NULL)", e.sql+dirOf(s.desc))
		return nil
	}
	for i, s := range c.viewSort {
		if err := addKey(s, fmt.Sprintf("vs%d", i), false); err != nil {
			return "", nil, err
		}
	}
	for i, s := range c.p.sort {
		if err := addKey(s, fmt.Sprintf("s%d", i), true); err != nil {
			return "", nil, err
		}
	}
```

In the same function, replace `c.detailTypes, c.formulaOptions = map[string]FieldType{}, map[string][]Option{}` with

```go
	c.detailTypes, c.formulaOptions, c.sortable = map[string]FieldType{}, map[string][]Option{}, map[string]bool{}
```

and after `c.detailTypes[cp.col.ID] = e.typ` (the non-nested branch) add

```go
		c.sortable[cp.col.ID] = !e.win
```

(a child row column is never added, so it reads false). In the `if d := c.p.spec.Detail; d != nil && d.Label != nil` block, after `sel = append(sel, ref.value+" AS __label", …)`:

```go
		// The first column sorts by the label: sortable when the label is.
		le, err := compileFormula(&rowsCtx{r: r, visiting: visiting, detail: frame}, d.Label, "detail.label")
		if err != nil {
			return "", nil, err
		}
		c.sortable[c.p.cols[0].col.ID] = !le.win
```

- [ ] **Step 6: Say which columns sort**

In `assemble.go` `columns`, before `rn.currencyOfColumn(cp, &rc)`:

```go
		rc.Sortable = c.p.pivot == nil && c.sortable[cp.col.ID]
```

- [ ] **Step 7: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'ViewerSort|WhichSort' ./reports/`
Expected: PASS.

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l ./reports/ && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt output; PASS (a run with no `options.sort` builds the same statement and the same cursor as before).

- [ ] **Step 8: Commit** (the driver runs it)

```bash
git add core/server/reports/rows.go core/server/reports/run.go core/server/reports/assemble.go core/server/reports/viewsort_test.go
git commit -m "feat(reports): viewer sort of detail rows (options.sort)"
```

---

### Task 8: F7 — paged grouped reports (Go)

Engine facts used: `run` refuses a cursor on a grouped report (`run.go:309-311`); the group pass runs whole and capped (`groupPass`, `capLevelRows`, `run.go:424-468`); `detailPass` caps a grouped report's details at `rowCap` and filters them by `expansion.where` at the innermost depth (`run.go:672-726`); `expansion.where` binds keys as one JSON list read by `json_each` (`run.go:970-1027`); `assemble` emits the document (`assemble.go:159-202`), with only `push` as a side effect, and layout sections are group rows `g:<key>` (`layout.go:394`); `cursor`, `fingerprint`, `cursorOf`, `readCursor`, `keysetAfter` (`paging.go`); `numberValue` (`formula.go:399`). Within a group a detail window's partition is the group keys (`run.go:770-773`), so a running sum restarts per innermost group and continues across pages inside it.

**Files:**
- Modify: `core/server/reports/paging.go` (`cursor.Group`; `fingerprint`, `cursorOf`, `readCursor` take the group levels; `keysetCond` split out of `keysetAfter`)
- Modify: `core/server/reports/run.go` (`Options` comments; `runner.page`; `run`; `detailPass`; new `groupedPage`, `openGroups`, `groupLevels`, `leafOrder`, `leafJoin`, `groupedAfter`)
- Modify: `core/server/reports/assemble.go` (new `runner.pageWindow`)
- Modify: `core/server/reports/paging_test.go` (delete `TestAGroupedReportIsNotPaged`; add the grouped tests)
- Modify: `core/server/reports/having_test.go:62-72` (the hidden-rows test reads a page, not the row cap)
- Modify: `core/server/reports/run_test.go:389-391` (comment of `TestTheRowCapNoticeIsGivenOnce`)

**Interfaces:**
- Consumes: viewer keys in `order` (Task 7); `KindOpening` rows `opening:<key>` right after their group row (Task 6).
- Produces: `cursor{Key string; Group []any; Values []any}` (JSON `k`, `g`, `v`); `fingerprint(levels string, keys []sortKey) string`; `cursorOf(levels string, keys []sortKey, row map[string]any, group []any) string`; `readCursor(levels string, keys []sortKey, text string) (cursor, error)`; `keysetCond(s *stmt, keys []sortKey, values []any) string`; `(rn *runner) openGroups(groupRows []map[string]any) [][]any`; `(rn *runner) detailPass(leaves [][]any)`; `(rn *runner) pageWindow()`; a grouped run's `Result.Page` (`Size` = its detail rows, `Next` while more remain). Task 9 walks these pages.

- [ ] **Step 1: Write the failing Go tests**

In `core/server/reports/paging_test.go`, delete `TestAGroupedReportIsNotPaged` (`paging_test.go:168-172`) and append (`requireDetails` is Task 7's helper in `viewsort_test.go`):

```go
// pagedBySpecies groups ann's feedings by species, each open group starting with
// an opening row and ending in its subtotal; running restarts in each group.
const pagedBySpecies = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"running","formula":{"op":"runningSum","args":[{"col":"cost"}]}}],
	"groups":[{"id":"g","col":"species","open":true,"subtotals":true,"opening":{"label":"Start"}}],
	"sort":[{"col":"note"}]}`

// withF7 adds a third lion feeding, so lion has f1, f2, f7 and zebra f3, f4.
func withF7(t *testing.T, z *zoo) {
	t.Helper()
	z.feeding(t, "f7", z.recs["leo"], 100, "2026-01-07", 10)
}

// groupedPages walks every page of raw as ann with opts.
func groupedPages(t *testing.T, z *zoo, raw string, opts Options) []*Result {
	t.Helper()
	var pages []*Result
	for range 100 {
		res := z.run(t, "ann", raw, opts, nil)
		pages = append(pages, res)
		if res.Page == nil || res.Page.Next == "" {
			return pages
		}
		opts.Cursor = res.Page.Next
	}
	t.Fatal("paging never ended")
	return nil
}

// mergedRows joins pages as the client does (R10): each page's rows in order,
// an earlier page's total and computed rows dropped; it fails the test when a
// row other than those is on two pages (R9e).
func mergedRows(t *testing.T, pages []*Result) []Row {
	t.Helper()
	var out []Row
	seen := map[string]bool{}
	for i, p := range pages {
		for _, r := range p.Rows {
			ends := r.Kind == KindTotal || r.Kind == KindComputed
			if ends && i < len(pages)-1 {
				continue
			}
			if seen[r.ID] {
				t.Fatalf("row %s is on two pages", r.ID)
			}
			seen[r.ID] = true
			out = append(out, r)
		}
	}
	return out
}

func TestAGroupedReportPagesAcrossAGroupBoundary(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	pages := groupedPages(t, z, pagedBySpecies, Options{Limit: 2})
	if len(pages) != 3 {
		t.Fatalf("%d pages", len(pages))
	}
	requireRows(t, pages[0], "kind", "group", "opening", "detail", "detail", "total")
	requireOrderedRows(t, pagedBySpecies, pages[0], "note", nil, nil, "f1", "f2", nil)
	requireRows(t, pages[1], "kind", "detail", "subtotal", "group", "opening", "detail", "total")
	requireOrderedRows(t, pagedBySpecies, pages[1], "note", "f7", nil, nil, nil, "f3", nil)
	if ids := []string{pages[1].Rows[1].ID, pages[1].Rows[2].ID, pages[1].Rows[3].ID}; !slices.Equal(ids, []string{`s:["lion","USD"]`, `g:["zebra","USD"]`, `opening:["zebra","USD"]`}) {
		t.Fatalf("page 2 ids = %v", ids)
	}
	requireRows(t, pages[2], "kind", "detail", "subtotal", "total")
	requireOrderedRows(t, pagedBySpecies, pages[2], "note", "f4", nil, nil)
	for i, want := range []int{2, 2, 1} {
		if pages[i].Page.Size != want {
			t.Fatalf("page %d size = %d", i+1, pages[i].Page.Size)
		}
		// Every page ends in the total of every row.
		if total := pages[i].Rows[len(pages[i].Rows)-1]; total.Cells["cost"] != int64(4600) {
			t.Fatalf("page %d total = %+v", i+1, total)
		}
	}
}

// Window values are computed before the page is cut: lion's running sum goes
// on from page 1's 3500 (f1 1000, f2 2500) to 3600 at f7.
func TestARunningSumContinuesOnPageTwo(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	pages := groupedPages(t, z, pagedBySpecies, Options{Limit: 2})
	requireDetails(t, pages[0], "running", int64(1000), int64(3500))
	if first := pages[1].Rows[0]; first.Cells["note"] != "f7" || first.Cells["running"] != int64(3600) {
		t.Fatalf("page 2 starts with %+v", first)
	}
}

// Pages of one row each join into exactly the one-page run, and a feeding
// added between pages repeats no row already shown.
func TestAGroupedPageSeamRepeatsNoRow(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	whole := z.run(t, "ann", pagedBySpecies, Options{Limit: 1000}, nil)
	if got := mergedRows(t, groupedPages(t, z, pagedBySpecies, Options{Limit: 1})); !reflect.DeepEqual(got, whole.Rows) {
		t.Fatalf("pages = %v\nwhole = %v", got, whole.Rows)
	}
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 2}, nil)
	z.feeding(t, "f8", z.recs["zed"], 50, "2026-03-20", 5)
	rest := groupedPages(t, z, pagedBySpecies, Options{Limit: 2, Cursor: first.Page.Next})
	merged := mergedRows(t, append([]*Result{first}, rest...))
	requireRows(t, &Result{Rows: merged}, "note", nil, nil, "f1", "f2", "f7", nil, nil, nil, "f3", "f4", "f8", nil, nil)
}

// bulkFeedings adds n feedings of leo (cost 1, grams 1, notes n00001…) in one
// statement: n saves would take minutes. The report reads them as any other.
func bulkFeedings(t *testing.T, z *zoo, n int) {
	t.Helper()
	_, err := z.app.DB().NewQuery(`WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < {:n})
		INSERT INTO zoo_feedings (id, animal, cost, currency, grams, fed, note, due)
		SELECT printf('bulk%011d', i), {:leo}, 1, 'USD', 1, '2026-04-01 00:00:00.000Z', printf('n%05d', i), '2026-04-01' FROM seq`).
		Bind(dbx.Params{"n": n, "leo": z.id("leo")}).Execute()
	if err != nil {
		t.Fatal(err)
	}
}

// The grouped 5,000-row cap is gone: 6,004 detail rows page to the end.
func TestASixThousandRowGroupedReportPagesToItsEnd(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 6000)
	pages := groupedPages(t, z, feedingsBySpecies, Options{ExpandAll: true, Limit: 1000})
	details, lionTotals := 0, 0
	for i, p := range pages {
		if len(p.Notices) != 0 {
			t.Fatalf("page %d notices = %+v", i+1, p.Notices)
		}
		for _, r := range p.Rows {
			switch {
			case r.Kind == KindDetail:
				details++
			case r.ID == `s:["lion","USD"]`:
				lionTotals++
				if r.Cells["cost"] != int64(9500) {
					t.Fatalf("lion's subtotal = %+v", r)
				}
			}
		}
	}
	if len(pages) != 7 || details != 6004 || lionTotals != 1 {
		t.Fatalf("pages = %d, details = %d, lion subtotals = %d", len(pages), details, lionTotals)
	}
}

func TestAGroupedPageHoldsAtMostAThousandRows(t *testing.T) {
	z := newZoo(t)
	bulkFeedings(t, z, 1200)
	res := z.run(t, "ann", feedingsBySpecies, Options{ExpandAll: true, Limit: 5000}, nil)
	if res.Page == nil || res.Page.Size != 1000 || res.Page.Next == "" {
		t.Fatalf("page = %+v", res.Page)
	}
}

func TestAGroupedCursorIsRefusedAfterASpecChangeOrASortChange(t *testing.T) {
	z := newZoo(t)
	withF7(t, z)
	first := z.run(t, "ann", pagedBySpecies, Options{Limit: 1}, nil)
	byNote := strings.Replace(pagedBySpecies, `"col":"species","open"`, `"col":"note","open"`, 1)
	cases := map[string]struct {
		spec string
		opts Options
	}{
		"another sort":         {strings.Replace(pagedBySpecies, `"sort":[{"col":"note"}]`, `"sort":[{"col":"note","dir":"desc"}]`, 1), Options{}},
		"another grouping":     {byNote, Options{}},
		"a viewer sort":        {pagedBySpecies, Options{Sort: []ViewSort{{Col: "cost", Dir: "desc"}}}},
		"its group is shut":    {pagedBySpecies, Options{Expand: [][]any{{"zebra", "USD"}}}},
	}
	for name, c := range cases {
		c.opts.Limit, c.opts.Cursor = 1, first.Page.Next
		_, err := z.try(t, "ann", c.spec, c.opts, nil)
		if err == nil {
			t.Fatalf("%s: the cursor was taken", name)
		}
		requireSpecError(t, err, 400, "options.cursor", "not one this report gave")
	}
}

// Group rows keep their cap: with rowCap 3 the group statement keeps the
// total and the first two groups, says so, and pages only their details.
func TestGroupRowsStillStopAtTheGroupCap(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.rowCap = 3 })
	const byNote = `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"note"}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", byNote, Options{ExpandAll: true}, nil)
	requireRows(t, res, "kind", "group", "detail", "group", "detail", "total")
	requireRows(t, res, "label", "f1", "f1", "f2", "f2", "Total")
	if len(res.Notices) != 1 || res.Notices[0].Code != "rowCap" || res.Notices[0].Count != 3 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	if res.Page == nil || res.Page.Next != "" {
		t.Fatalf("page = %+v", res.Page)
	}
}
```

The `paging_test.go` imports become `"encoding/base64"`, `"reflect"`, `"slices"`, `"strings"`, `"testing"` and `"github.com/pocketbase/dbx"`.

In `core/server/reports/having_test.go`, replace `TestHiddenRowsTakeNoPlaceUnderTheRowCap` with:

```go
// The rows of a hidden group take no place on a page: lion's two feedings
// sort first, and with them a page of 2 rows would hold no zebra row.
func TestHiddenRowsTakeNoPlaceOnAPage(t *testing.T) {
	z := newZoo(t)
	withLimits(t, func(l *limits) { l.pageSize = 2 })
	res := z.run(t, "ann", lionHidden, Options{ExpandAll: true}, nil)
	requireOrderedRows(t, lionHidden, res, "note", nil, "f3", "f4", nil)
	if len(res.Notices) != 0 || res.Page == nil || res.Page.Next != "" {
		t.Fatalf("notices = %+v, page = %+v", res.Notices, res.Page)
	}
}
```

In `core/server/reports/run_test.go` `TestTheRowCapNoticeIsGivenOnce`, the comment becomes `// The group statement (3 rows) passes the cap; the detail pass pages and has no cap.` (the assertion stays).

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Grouped|RunningSumContinues|SixThousand|GroupCap|HiddenRowsTakeNoPlace' ./reports/`
Expected: FAIL. `TestAGroupedReportPagesAcrossAGroupBoundary`: `1 pages` (a grouped run returns every detail and `Page` nil); the cursor tests: `Run: options.cursor: a grouped report is not paged`; `TestAGroupedPageHoldsAtMostAThousandRows`: `page = <nil>`; `TestHiddenRowsTakeNoPlaceOnAPage`: `page = <nil>`.

- [ ] **Step 3: Tie a cursor to the group levels and its group**

Replace `paging.go` from `type cursor struct` through `keysetAfter` with:

```go
// cursor is a page cursor: a fingerprint of the group levels and sort keys,
// the open innermost group the last row is in (a grouped report only), then
// the sort values of the last row and its id.
type cursor struct {
	Key    string `json:"k"`
	Group  []any  `json:"g,omitempty"`
	Values []any  `json:"v"`
}

// valueKind is the JSON type a sort key's value has in a cursor: n for a
// number, s for text, a for a column whose type is not known.
func valueKind(k sortKey) string {
	switch {
	case k.name == "__id":
		return "s"
	case k.typ == "":
		return "a"
	case isNumeric(k.typ):
		return "n"
	}
	return "s"
}

// fingerprint ties a cursor to the group levels (groupLevels, "" without
// groups) and the sort columns, directions and value types of the report and
// run that gave it (R9d).
func fingerprint(levels string, keys []sortKey) string {
	h := sha256.New()
	if levels != "" {
		h.Write([]byte("groups:" + levels + ";"))
	}
	for _, k := range keys {
		h.Write([]byte(k.col + "|" + dirOf(k.desc) + "|" + valueKind(k) + ";"))
	}
	return hex.EncodeToString(h.Sum(nil)[:6])
}

// cursorOf is the page cursor after row: its group (nil without groups), its
// sort values, then its id.
func cursorOf(levels string, keys []sortKey, row map[string]any, group []any) string {
	values := make([]any, len(keys))
	for i, k := range keys {
		values[i] = row[k.name]
	}
	b, _ := json.Marshal(cursor{Key: fingerprint(levels, keys), Group: group, Values: values})
	return base64.RawURLEncoding.EncodeToString(b)
}

func badCursor() *SpecError {
	return invalid("options.cursor", "the cursor is not one this report gave")
}

// readCursor decodes a cursor and checks it fits levels and keys: same
// fingerprint, a group exactly when the report has groups (scalars only), one
// value per key, each of the type its key holds, a text id last.
func readCursor(levels string, keys []sortKey, text string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return cursor{}, badCursor()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var c cursor
	if err := dec.Decode(&c); err != nil {
		return cursor{}, badCursor()
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return cursor{}, badCursor()
	}
	if c.Key != fingerprint(levels, keys) || len(c.Values) != len(keys) || (levels != "") != (len(c.Group) > 0) {
		return cursor{}, badCursor()
	}
	for i, v := range c.Group {
		switch x := v.(type) {
		case json.Number:
			c.Group[i] = numberValue(x)
		case string, bool, nil:
		default:
			return cursor{}, badCursor()
		}
	}
	for i, v := range c.Values {
		kind := valueKind(keys[i])
		switch x := v.(type) {
		case json.Number:
			if kind == "s" {
				return cursor{}, badCursor()
			}
			c.Values[i] = numberValue(x)
		case string:
			if kind == "n" {
				return cursor{}, badCursor()
			}
		case nil:
			if i == len(keys)-1 {
				return cursor{}, badCursor()
			}
		default:
			return cursor{}, badCursor()
		}
	}
	return c, nil
}

// keysetAfter selects the rows after a cursor in the detail order of a report
// without groups: a page starts where the last one ended, so a row added or
// removed between pages does not shift the rest.
func keysetAfter(s *stmt, keys []sortKey, text string) (string, error) {
	if text == "" {
		return "1", nil
	}
	c, err := readCursor("", keys, text)
	if err != nil {
		return "", err
	}
	return keysetCond(s, keys, c.Values), nil
}

// keysetCond is true for a row (d) after values in the order of keys. The
// values are bound, never inlined.
func keysetCond(s *stmt, keys []sortKey, values []any) string {
	last := len(keys) - 1
	cond := "d.__id > " + s.bind(values[last])
	for i := last - 1; i >= 0; i-- {
		col := "d." + keys[i].name
		if values[i] == nil {
			// NULLs sort last, so after a NULL come only NULLs.
			cond = "(" + col + " IS NULL AND " + cond + ")"
			continue
		}
		cmp := " > "
		if keys[i].desc {
			cmp = " < "
		}
		v := s.bind(values[i])
		cond = "(" + col + " IS NULL OR " + col + cmp + v + " OR (" + col + " = " + v + " AND " + cond + "))"
	}
	return cond
}
```

- [ ] **Step 4: Page the detail pass of a grouped report**

In `run.go` `Options`, the `Cursor`/`Limit` comment becomes:

```go
	// Cursor is the previous page's page.next; Limit is the page size (at
	// most maxPage). A grouped report pages its detail rows in document
	// order; its group rows come whole on every page (R9).
```

In `runner`, after `pivotLabels []rowLabel`:

```go
	// page is a grouped run's page: where its rows start and end in the
	// document (pageWindow); nil when the run did not page grouped details.
	page *groupedPage
```

Add after `runner`:

```go
// groupedPage is a grouped run's page: after is the open group the previous
// page's last detail row was in (nil on page 1), last this page's last
// detail row's id ("" on the last page).
type groupedPage struct {
	after []any
	last  string
}
```

Replace `run`'s beginning, from `func (rn *runner) run() error {` through the closing `}` of the `if rn.c.p.pivot == nil && (…)` block, with:

```go
func (rn *runner) run() error {
	// A pivot report has no detail rows to page.
	if rn.c.p.pivot != nil && rn.opts.Cursor != "" {
		return badCursor()
	}
	groupRows, err := rn.groupPass()
	if err != nil {
		return err
	}
	var details []map[string]any
	if rn.c.p.pivot == nil {
		var leaves [][]any
		grouped := len(rn.c.p.groups) > 0
		if grouped {
			leaves = rn.openGroups(groupRows)
		}
		// A cursor is read even with no open group, so one whose group shut
		// is refused rather than answered with no rows.
		if !grouped || len(leaves) > 0 || rn.opts.Cursor != "" {
			details, err = rn.detailPass(leaves)
			if err != nil {
				return err
			}
		}
	}
```

and after `rn.assemble(groupRows, details, nested)` add `rn.pageWindow()`.

Replace `detailPass` with:

```go
// detailPass runs the detail statement: one row per base row, a page at a
// time. Without groups a page follows the detail order; with groups it holds
// the rows of the open innermost groups (leaves) in document order: the
// groups' order, then the detail order (R9b). Window values are computed
// before the page is cut, so a running value goes on across pages. Rows under
// a hidden group are left out before any running value reads them.
func (rn *runner) detailPass(leaves [][]any) ([]map[string]any, error) {
	c := rn.c
	s := newStmt(rn.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		return nil, err
	}
	shown, m, err := c.shownLevels(r, rn.expand)
	if err != nil {
		return nil, err
	}
	table := "w"
	if m > 0 {
		table = "ws"
		shown = append(shown, "ws AS (SELECT w.* FROM w WHERE "+c.inShown(m, "w")+")")
	}
	sel, order, err := c.detailSelect(r, table)
	if err != nil {
		return nil, err
	}
	limit := runLimits.pageSize
	if rn.opts.Limit > 0 {
		limit = min(rn.opts.Limit, runLimits.maxPage)
	}
	ctes := append([]string{r.render()}, shown...)
	grouped := len(c.p.groups) > 0
	from, by, levels := "("+sel+") d", orderBy(order), ""
	var where string
	if grouped {
		levels = c.groupLevels()
		ctes = append(ctes, c.leafOrder(s, leaves))
		from = fmt.Sprintf("(SELECT d.*, gord.__g AS __g FROM (%s) d JOIN gord ON %s) d", sel, c.leafJoin())
		by = "d.__g, " + by
		rn.page = &groupedPage{}
		where, rn.page.after, err = groupedAfter(s, levels, order, rn.opts.Cursor, leaves)
	} else {
		where, err = keysetAfter(s, order, rn.opts.Cursor)
	}
	if err != nil {
		return nil, err
	}
	body := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY %s LIMIT %s", from, where, by, limitSQL(int64(limit+1)))
	rows, err := rn.query(s, ctes, body)
	if err != nil {
		return nil, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	rn.res.Page = &Page{Size: len(rows)}
	if more {
		last := rows[len(rows)-1]
		var group []any
		if grouped {
			group = rn.rowKey(last, len(c.p.groups))
			rn.page.last = fmt.Sprintf("d:%v", last["__id"])
		}
		rn.res.Page.Next = cursorOf(levels, order, last, group)
	}
	return rows, nil
}

// openGroups lists the open innermost groups in the order the report shows
// them (R9a): the group rows that an assembly with no detail rows marks
// expanded at the innermost depth. That assembly is thrown away.
func (rn *runner) openGroups(groupRows []map[string]any) [][]any {
	rows, notices := rn.res.Rows, rn.res.Notices
	rn.res.Rows, rn.res.Notices = []Row{}, slices.Clone(notices)
	rn.assemble(groupRows, nil, nil)
	depth := len(rn.c.p.groups) - 1
	var out [][]any
	for _, r := range rn.res.Rows {
		if r.Kind == KindGroup && r.Expanded && r.Depth == depth {
			out = append(out, r.GroupKey)
		}
	}
	rn.res.Rows, rn.res.Notices = rows, notices
	return out
}

// groupLevels names the report's group levels (column and bucket) for a
// grouped cursor's fingerprint: a cursor from another grouping is refused.
func (c *compiler) groupLevels() string {
	parts := make([]string, len(c.p.groups))
	for i, g := range c.p.groups {
		parts[i] = g.col.col.ID + "/" + g.group.Bucket
	}
	return strings.Join(parts, ";")
}

// leafOrder is the CTE gord: one row per open innermost group, __g its place
// in document order (json_each's index), g<j> its key values (group keys,
// then currency keys), bound as one JSON list as expansion.where binds keys.
func (c *compiler) leafOrder(s *stmt, leaves [][]any) string {
	encoded := make([]string, len(leaves))
	for i, key := range leaves {
		encoded[i] = keyString(key)
	}
	cols := []string{"key AS __g"}
	for j := range len(c.p.groups) + len(c.currencies) {
		cols = append(cols, fmt.Sprintf("json_extract(value, '$[%d]') AS g%d", j, j))
	}
	return fmt.Sprintf("gord AS (SELECT %s FROM json_each(%s))", strings.Join(cols, ", "), s.bind("["+strings.Join(encoded, ",")+"]"))
}

// leafJoin matches a detail row (d) to its open group by each key value with
// IS, so a blank key matches a blank one, as a group row's key reads.
func (c *compiler) leafJoin() string {
	on := make([]string, 0, len(c.p.groups)+len(c.currencies))
	for i := range c.p.groups {
		on = append(on, fmt.Sprintf("d.k%d IS gord.g%d", i, i))
	}
	for j, ck := range c.currencies {
		on = append(on, fmt.Sprintf("d.%s IS gord.g%d", ck, len(c.p.groups)+j))
	}
	return strings.Join(on, " AND ")
}

// groupedAfter selects the detail rows after a grouped report's cursor: the
// rows of a later open group, or of the cursor's group after its sort values
// (R9c), and returns the cursor's group, which the page starts after. A
// cursor whose group no longer opens is refused; the client then reads from
// page 1.
func groupedAfter(s *stmt, levels string, keys []sortKey, text string, leaves [][]any) (string, []any, error) {
	if text == "" {
		return "1", nil, nil
	}
	cur, err := readCursor(levels, keys, text)
	if err != nil {
		return "", nil, err
	}
	want := keyString(cur.Group)
	at := slices.IndexFunc(leaves, func(k []any) bool { return keyString(k) == want })
	if at < 0 {
		return "", nil, badCursor()
	}
	g := s.bind(int64(at))
	return fmt.Sprintf("(d.__g > %s OR (d.__g = %s AND %s))", g, g, keysetCond(s, keys, cur.Values)), cur.Group, nil
}
```

- [ ] **Step 5: Cut the page out of the document**

In `assemble.go`, after `push`:

```go
// pageWindow keeps a grouped page's slice of the document (R9e): the rows
// after the previous page's last detail row (after its group row and opening
// row; from the start on page 1), up to and including this page's last detail
// row and its child rows (to the end on the last page), then every total and
// computed row, as each page of a report without groups ends.
func (rn *runner) pageWindow() {
	if rn.page == nil {
		return
	}
	var body, ends []Row
	for _, r := range rn.res.Rows {
		if r.Kind == KindTotal || r.Kind == KindComputed {
			ends = append(ends, r)
		} else {
			body = append(body, r)
		}
	}
	start := 0
	if rn.page.after != nil {
		id := "g:" + keyString(rn.page.after)
		if i := slices.IndexFunc(body, func(r Row) bool { return r.ID == id }); i >= 0 {
			start = i + 1
			if start < len(body) && body[start].Kind == KindOpening && body[start].Section == id {
				start++
			}
		}
	}
	end := len(body)
	if rn.page.last != "" {
		if i := slices.IndexFunc(body, func(r Row) bool { return r.ID == rn.page.last }); i >= 0 {
			end = i + 1
			for end < len(body) && body[end].Parent != "" {
				end++
			}
		}
	}
	rn.res.Rows = append(slices.Clone(body[start:end]), ends...)
}
```

(`"slices"` is in the `assemble.go` imports since Task 6.)

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Grouped|RunningSumContinues|SixThousand|GroupCap|HiddenRowsTakeNoPlace|Paged|Cursor|Page' ./reports/`
Expected: PASS (the ungrouped paging tests too: their fingerprint input is unchanged).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l ./reports/ && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt output; PASS. A failure in a grouped test that expanded more than 200 detail rows with no `Limit` is a test that read the old cap: give it `Limit: 1000`, never a larger page.

- [ ] **Step 7: Commit** (the driver runs it)

```bash
git add core/server/reports/paging.go core/server/reports/run.go core/server/reports/assemble.go core/server/reports/paging_test.go core/server/reports/having_test.go core/server/reports/run_test.go
git commit -m "feat(reports): page grouped reports in document order"
```

---

### Task 9: Rule-parity fuzz — sum lookups and paged grouped reports (Go)

Engine facts used: `FuzzRuleParity` runs each seed against one grown zoo (`fuzz_test.go:184-205`; `growZoo` adds 20 animals, 25 keepers, 60 feedings with grams 0–899 in USD or EUR for ann, bob and carol) and calls `lookupParity` and `fillParity` per seed; `listExpanded` reads rows through PocketBase's list API (`fuzz_test.go:116-158`). A detail row's link cell holds the linked record's id (`engineExpanded`, `fuzz_test.go:178`).

**Files:**
- Modify: `core/server/reports/fuzz_test.go`

**Interfaces:**
- Consumes: pick `"sum"`/`"count"` (Task 2); `{ "outer": … }` (Task 3); opening rows (Task 6); grouped pages, `Page.Next` and the rule that only total and computed rows repeat across pages (Task 8).
- Produces: nothing new.

- [ ] **Step 1: Add the generator branches**

In `fuzz_test.go`, in `FuzzRuleParity`'s body after `z.fillParity(t, mux, user)`:

```go
		z.sumLookupParity(t, mux, user)
		z.pagedGroupParity(t, user, 1+rng.IntN(5))
```

Append:

```go
// listItems reads every field of a collection's rows through the list API
// as user.
func (z *zoo) listItems(t *testing.T, mux http.Handler, collection, user string) []map[string]any {
	t.Helper()
	q := url.Values{"perPage": {"500"}, "skipTotal": {"1"}}
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+collection+"/records?"+q.Encode(), nil)
	req.Header.Set("Authorization", z.token(t, user))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %s as %s: %d %s", collection, user, rec.Code, rec.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Items
}

// sumLookupParity: per feeding, a sum lookup of the grams of its animal's
// feedings, and a count of them, equal the sum and the count over the
// feedings the list API lists for the reader.
func (z *zoo) sumLookupParity(t *testing.T, mux http.Handler, user string) {
	t.Helper()
	const sameAnimal = `{"op":"eq","args":[{"path":[{"link":"animal"}]},{"outer":[{"link":"animal"}]}]}`
	res := z.run(t, user, `{"version":3,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"total","formula":{"op":"lookup","source":"zoo.feedings","where":`+sameAnimal+`,"value":{"path":[{"field":"grams"}]},"pick":"sum"}},
		{"id":"n","formula":{"op":"lookup","source":"zoo.feedings","where":`+sameAnimal+`,"value":{"path":[{"field":"id"}]},"pick":"count"}}]}`,
		Options{Limit: 1000}, nil)
	sums, counts := map[string]int64{}, map[string]int64{}
	for _, item := range z.listItems(t, mux, "zoo_feedings", user) {
		animal, _ := item["animal"].(string)
		grams, _ := item["grams"].(float64)
		sums[animal] += int64(grams)
		counts[animal]++
	}
	for _, row := range res.Rows {
		if row.Kind != KindDetail {
			continue
		}
		animal, _ := row.Cells["animal"].(string)
		if !reflect.DeepEqual(row.Cells["total"], sums[animal]) || !reflect.DeepEqual(row.Cells["n"], counts[animal]) {
			t.Fatalf("%s: animal %s: sum %v count %v, list API %d and %d", user, animal, row.Cells["total"], row.Cells["n"], sums[animal], counts[animal])
		}
	}
}

// pagedAnimals groups feedings by animal, each open group starting with the
// sum of its animal's grams; running restarts in each group.
const pagedAnimals = `{"version":3,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"},
	{"id":"running","formula":{"op":"runningSum","args":[{"col":"grams"}]}}],
	"groups":[{"id":"g","col":"animal","open":true,"subtotals":true,"opening":{"label":"All feedings","cells":{"grams":
		{"op":"lookup","source":"zoo.feedings","where":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"record":"g","path":[{"field":"id"}]}]},
		"value":{"path":[{"field":"grams"}]},"pick":"sum"}}}}],
	"sort":[{"col":"note"}]}`

// pagedGroupParity: the pages of a grouped report, joined as the client joins
// them (an earlier page's totals dropped), are exactly the one-page run, and
// no other row is on two pages.
func (z *zoo) pagedGroupParity(t *testing.T, user string, limit int) {
	t.Helper()
	whole := z.run(t, user, pagedAnimals, Options{Limit: 1000}, nil)
	var merged []Row
	seen := map[string]bool{}
	cursor := ""
	for page := 1; ; page++ {
		if page > 1000 {
			t.Fatalf("%s, limit %d: paging never ended", user, limit)
		}
		res := z.run(t, user, pagedAnimals, Options{Limit: limit, Cursor: cursor}, nil)
		last := res.Page == nil || res.Page.Next == ""
		for _, row := range res.Rows {
			if (row.Kind == KindTotal || row.Kind == KindComputed) && !last {
				continue
			}
			if seen[row.ID] {
				t.Fatalf("%s, limit %d: row %s is on two pages", user, limit, row.ID)
			}
			seen[row.ID] = true
			merged = append(merged, row)
		}
		if last {
			break
		}
		cursor = res.Page.Next
	}
	if !reflect.DeepEqual(merged, whole.Rows) {
		t.Fatalf("%s, limit %d: pages\n%v\nwhole\n%v", user, limit, merged, whole.Rows)
	}
}
```

- [ ] **Step 2: Prove the branches can fail**

Tasks 2 and 8 are done, so the new branches pass at once. Prove each one turns red on a broken engine, with an overlay on a scratch copy (the repo source is not touched):

```bash
WS=/Users/nas/tmp/tinycld/books-purchases
R=$WS/tinycld/core/server/reports
SCR=$(mktemp -d)
cp $R/assemble.go $SCR/assemble.go
# A seam that repeats the cursor's group row.
perl -0pi -e 's/start = i \+ 1\n/start = i\n/' $SCR/assemble.go
printf '{"Replace":{"%s":"%s"}}' "$R/assemble.go" "$SCR/assemble.go" > $SCR/overlay.json
cd $WS/tinycld/core/server && go test -tags no_ui -count=1 -overlay $SCR/overlay.json -run FuzzRuleParity ./reports/
```

Expected: FAIL with `row g:[…] is on two pages`.

```bash
cp $R/lookup.go $SCR/lookup.go
# A sum lookup that adds nothing up: Task 2's sum SQL reads the largest value.
perl -0pi -e 's/"sum\(/"max(/' $SCR/lookup.go
printf '{"Replace":{"%s":"%s"}}' "$R/lookup.go" "$SCR/lookup.go" > $SCR/overlay.json
cd $WS/tinycld/core/server && go test -tags no_ui -count=1 -overlay $SCR/overlay.json -run FuzzRuleParity ./reports/
rm -rf "$SCR"
```

Expected: FAIL with `animal …: sum … count …, list API … and …`. (If Task 2 builds the sum SQL with another literal than `"sum(`, edit that literal in the copy instead; the point is one wrong aggregate.)

- [ ] **Step 3: Run the fuzz seeds to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run FuzzRuleParity ./reports/`
Expected: PASS (40 seeds).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && gofmt -l ./reports/ && go vet -tags no_ui ./reports/ && go test -tags no_ui -count=1 ./reports/`
Expected: no gofmt output; PASS.

- [ ] **Step 4: Commit** (the driver runs it)

```bash
git add core/server/reports/fuzz_test.go
git commit -m "test(reports): rule parity for sum lookups and grouped pages"
```

### Task 10: Client spec and result types v3

Client facts used: the strict spec schema (`core/reports/lib/spec.ts:9,21-60,109-147,182-205`); `RowKind` is a closed union (`lib/types.ts:21`) that `ReportRow.tsx:14-28` keys `ROW_RULE`/`ROW_WEIGHT` by, so adding a kind is a compile error until both maps have it; `cellType` gives every non-detail row the column's `totalType` (`lib/format.ts:97-100`), but an opening cell has its column's detail type (R5c), so an opening row must read `column.type`; `RunOptions` (`lib/run-client.ts:17-24`) has no sort. The Go result and spec shapes are Task 1's (`Node.outer`, `Node.bound`, `pick` `sum`/`count`, `Group.opening`, `Column.rollup`, `ResultColumn.sortable`, row kind `opening`, `options.sort`).

**Files:**
- Modify: `core/reports/lib/spec.ts` (`CURRENT_SPEC_VERSION`, `SpecNode`, `specNodeSchema`, `specColumnSchema`, `specGroupSchema`)
- Modify: `core/reports/lib/types.ts` (`RowKind`, `ResultColumn.sortable`)
- Modify: `core/reports/lib/run-client.ts` (`ViewSort`, `RunOptions.sort`)
- Modify: `core/reports/lib/format.ts:97-100` (`cellType` reads the detail type on an opening row)
- Modify: `core/reports/components/ReportRow.tsx:14-28` (`opening` in `ROW_RULE`, `ROW_WEIGHT`)
- Test: `core/reports/lib/__tests__/spec.test.ts`, `core/reports/lib/__tests__/format.test.ts`

**Interfaces:**
- Consumes: Task 1's JSON field names and the spec table in the head ("New fields, by part").
- Produces: `CURRENT_SPEC_VERSION = 3`; `SpecNode.outer?: SpecHop[]`, `SpecNode.bound?: 'from' | 'to'`, `SpecNode.pick?: 'min' | 'max' | 'sum' | 'count'`; `SpecGroup.opening?: { label: string; cells?: Record<string, SpecNode> }`; `SpecColumn.rollup?: SpecNode`; `RowKind` adds `'opening'`; `ResultColumn.sortable?: boolean`; `export type ViewSort = { col: string; dir: 'asc' | 'desc' }` (in `lib/run-client.ts`); `RunOptions.sort?: ViewSort[]`.

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/__tests__/spec.test.ts`, replace the test `'reads a version 1 spec and a version 2 spec'` with:

```ts
    it('reads a version 1, 2 and 3 spec and refuses version 4', () => {
        expect(parseSpec({ ...FEEDINGS, version: 1 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 2 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 3 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 4 })).toBeNull()
        expect(CURRENT_SPEC_VERSION).toBe(3)
    })
```

Append at the end of the file:

```ts
// F1/F3 inside a lookup: the animal's cost before the period.
const BEFORE_PERIOD = {
    op: 'coalesce',
    args: [
        {
            op: 'lookup',
            source: 'zoo.feedings',
            where: {
                op: 'and',
                args: [
                    {
                        op: 'eq',
                        args: [
                            { path: [{ link: 'animal' }] },
                            { record: 'byAnimal', path: [{ field: 'id' }] },
                        ],
                    },
                    {
                        op: 'lt',
                        args: [{ path: [{ field: 'fed' }] }, { param: 'period', bound: 'from' }],
                    },
                ],
            },
            value: { path: [{ field: 'cost' }] },
            pick: 'sum',
        },
        { value: 0 },
    ],
}

// The head's v3 example: every new part once.
const V3 = {
    version: 3,
    source: 'zoo.feedings',
    params: [{ id: 'period', type: 'dateRange', label: 'Period', default: { preset: 'allDates' } }],
    columns: [
        { id: 'animal', label: 'Animal', path: [{ link: 'animal' }] },
        { id: 'fed', label: 'Fed', path: [{ field: 'fed' }] },
        { id: 'cost', label: 'Cost', path: [{ field: 'cost' }], aggregate: 'sum' },
        {
            id: 'sameDay',
            label: 'Same day',
            formula: {
                op: 'lookup',
                source: 'zoo.feedings',
                where: {
                    op: 'and',
                    args: [
                        {
                            op: 'eq',
                            args: [{ path: [{ link: 'animal' }] }, { outer: [{ link: 'animal' }] }],
                        },
                        {
                            op: 'eq',
                            args: [{ path: [{ field: 'fed' }] }, { outer: [{ field: 'fed' }] }],
                        },
                    ],
                },
                value: { path: [{ field: 'id' }] },
                pick: 'count',
            },
        },
        {
            id: 'balance',
            label: 'Balance',
            formula: {
                op: 'add',
                args: [BEFORE_PERIOD, { op: 'runningSum', args: [{ path: [{ field: 'cost' }] }] }],
            },
            rollup: {
                op: 'add',
                args: [{ op: 'sum', args: [{ path: [{ field: 'cost' }] }] }, { value: 0 }],
            },
        },
    ],
    filter: { op: 'between', args: [{ path: [{ field: 'fed' }] }, { param: 'period' }] },
    groups: [
        {
            id: 'byAnimal',
            col: 'animal',
            open: true,
            subtotals: true,
            opening: { label: 'Before the period', cells: { balance: BEFORE_PERIOD } },
        },
    ],
    sort: [{ col: 'fed' }],
}

describe('spec version 3', () => {
    it('reads every v3 part as it is', () => {
        expect(parseSpec(V3)).toEqual(V3)
    })

    it('reads a v3 spec with none of the new parts', () => {
        expect(parseSpec({ ...FEEDINGS, version: 3 })).toEqual({ ...FEEDINGS, version: 3 })
    })

    it('reads an opening row with no cells', () => {
        const groups = [{ id: 'g', col: 'species', opening: { label: 'Before' } }]
        expect(parseSpec({ ...FEEDINGS, version: 3, groups })?.groups?.[0]?.opening).toEqual({
            label: 'Before',
        })
    })

    it('refuses a pick, a bound or an opening label the engine refuses', () => {
        const lookup = (pick: string) => ({
            ...FEEDINGS,
            version: 3,
            columns: [
                {
                    id: 'x',
                    formula: {
                        op: 'lookup',
                        source: 'zoo.animals',
                        value: { path: [{ field: 'weight' }] },
                        pick,
                    },
                },
            ],
        })
        for (const pick of ['min', 'max', 'sum', 'count']) expect(parseSpec(lookup(pick))).not.toBeNull()
        expect(parseSpec(lookup('avg'))).toBeNull()

        const bound = (b: string) => ({
            ...FEEDINGS,
            version: 3,
            filter: {
                op: 'lt',
                args: [{ path: [{ field: 'fed' }] }, { param: 'period', bound: b }],
            },
        })
        expect(parseSpec(bound('to'))).not.toBeNull()
        expect(parseSpec(bound('mid'))).toBeNull()

        const opening = (label: string) => ({
            ...FEEDINGS,
            version: 3,
            groups: [{ id: 'g', col: 'species', opening: { label } }],
        })
        expect(parseSpec(opening(''))).toBeNull()
        expect(parseSpec(opening('x'.repeat(201)))).toBeNull()
        expect(parseSpec(opening('x'.repeat(200)))).not.toBeNull()
    })

    it('refuses an outer leaf that is not a path of hops, and an opening part it does not know', () => {
        const outer = (value: unknown) => ({
            ...FEEDINGS,
            version: 3,
            columns: [{ id: 'x', formula: { outer: value } }],
        })
        expect(parseSpec(outer([{ field: 'fed' }]))).not.toBeNull()
        expect(parseSpec(outer('fed'))).toBeNull()
        const groups = [{ id: 'g', col: 'species', opening: { label: 'Before', rows: 2 } }]
        expect(parseSpec({ ...FEEDINGS, version: 3, groups })).toBeNull()
    })
})
```

In `core/reports/lib/__tests__/format.test.ts`, add `cellType` to the import from `'../format'`, and append inside `describe('cellDisplay', …)` (after `'shows a count of a link column’s records as a number on total rows'`):

```ts
    // R5(c): an opening cell has its column's detail type, never the total type.
    it('reads an opening row’s cell as the column’s detail type', () => {
        const counted: ResultColumn = { ...ANIMAL, aggregate: 'count', totalType: 'number' }
        expect(cellType(counted, row('opening'))).toBe('relation')
        expect(cellType(counted, row('subtotal'))).toBe('number')
        expect(cellDisplay(MONEY, row('opening', { cost: 1250, 'cost@currency': 'USD' }), 1250, PLAIN).text).toBe(
            '12.50'
        )
    })
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/format.test.ts`
Expected: FAIL — `parseSpec` returns null for version 3 and for `outer`/`bound`/`opening`/`rollup`/`pick: 'sum'` (strict object, `version` max 2); `CURRENT_SPEC_VERSION` is 2; `cellType(counted, row('opening'))` is `'number'`.

- [ ] **Step 3: `lib/spec.ts`**

Set `export const CURRENT_SPEC_VERSION = 3`.

In `SpecNode`, replace the `pick` member and add two after it:

```ts
    /**
     * What a lookup takes from its matching rows' values: 'min' (the default) or
     * 'max' of them, 'sum' adds them, 'count' counts those with a value.
     */
    pick?: 'min' | 'max' | 'sum' | 'count'
    /**
     * Inside a lookup: the report row's value of this to-one path from the report's
     * own source (blank on a row with none, as a bare path is).
     */
    outer?: SpecHop[]
    /** With `param` on a date range: its first ('from') or last ('to') day. */
    bound?: 'from' | 'to'
```

In `specNodeSchema`, replace `pick: z.enum(['min', 'max']).optional(),` with:

```ts
        pick: z.enum(['min', 'max', 'sum', 'count']).optional(),
        outer: z.array(specHopSchema).optional(),
        bound: z.enum(['from', 'to']).optional(),
```

In `specColumnSchema`, after `detail: z.literal('blank').optional(),` add:

```ts
    // A formula shown on group, subtotal and total rows in place of the column's
    // totals; detail rows keep `path` or `formula`.
    rollup: specNodeSchema.optional(),
```

In `specGroupSchema`, after `open: z.boolean().optional(),` add:

```ts
    // A row first under each open group of this level: its label, and the cells
    // it shows, each a formula on the group (absent cells are blank).
    opening: z
        .strictObject({
            label: z.string().min(1).max(200),
            cells: z.record(z.string(), specNodeSchema).optional(),
        })
        .optional(),
```

- [ ] **Step 4: `lib/types.ts`, `lib/run-client.ts`, `lib/format.ts`, `ReportRow.tsx`**

`lib/types.ts:21`:

```ts
export type RowKind = 'detail' | 'group' | 'opening' | 'subtotal' | 'computed' | 'total'
```

In `ResultColumn`, after `labelCurrencyCell?: string`:

```ts
    /** The run accepts this column as a reader's sort key (options.sort). */
    sortable?: boolean
```

`lib/run-client.ts`, before `export interface RunOptions`:

```ts
/** One key of a reader's sort: a column of the result, ascending or descending. */
export type ViewSort = { col: string; dir: 'asc' | 'desc' }
```

and in `RunOptions`, after `limit?: number`:

```ts
    /** The reader's sort, before the report's own; detail rows only, inside their groups. */
    sort?: ViewSort[]
```

`lib/format.ts:97-100`:

```ts
/**
 * Group, subtotal, computed and total cells take the column's total type when it
 * has one; detail and opening cells have the column's own type (R5c).
 */
export function cellType(column: ResultColumn, row: ResultRow): FieldType {
    const isTotalRow = row.kind !== 'detail' && row.kind !== 'opening'
    return isTotalRow && column.totalType ? column.totalType : column.type
}
```

`components/ReportRow.tsx:14-28` — an opening row is drawn like a detail row (no rule above, normal weight):

```ts
const ROW_RULE: Record<RowKind, string> = {
    group: '',
    detail: '',
    opening: '',
    subtotal: 'border-t border-border',
    computed: 'border-t border-border',
    total: 'border-t-2 border-foreground',
}

const ROW_WEIGHT: Record<RowKind, string> = {
    group: 'font-semibold',
    detail: '',
    opening: '',
    subtotal: 'font-semibold',
    computed: 'font-semibold',
    total: 'font-semibold',
}
```

- [ ] **Step 5: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/format.test.ts`
Expected: PASS.

- [ ] **Step 6: Check and commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/spec.ts core/reports/lib/types.ts core/reports/lib/run-client.ts core/reports/lib/format.ts core/reports/components/ReportRow.tsx core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/format.test.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```

Expected: biome, tsc and every vitest pass (the builder's pick menu still compiles: it reads `pick` as a string). The driver commits:

```bash
git add core/reports/lib/spec.ts core/reports/lib/types.ts core/reports/lib/run-client.ts core/reports/lib/format.ts core/reports/components/ReportRow.tsx core/reports/lib/__tests__/spec.test.ts core/reports/lib/__tests__/format.test.ts
git commit -m "feat(core): the report client reads spec version 3, opening rows and sortable columns"
```

---

### Task 11: Client — All dates and the sort in the URL

Client facts used: `RANGE_PRESETS`, `PRESET_LABELS`, `presetsFor`, `valueText`, `periodWords`, `emptyMessage`, `emptyAction` (`core/reports/lib/params.ts:11-48,97-102,346-371,390-430`); a preset chip shows the resolved days only when the run gave both (`resolvedDates`, `params.ts:289-294`), so "Period: All dates" needs no chip change; the period pills follow `presetsFor` (`ParamPeriodControl.tsx:118-124`), so the control needs no change; the view URL (`lib/view-url.ts:22-171`) checks each key on its own and writes `undefined` for a default so `router.setParams` removes it. Task 4 resolves `allDates` with no `from`/`to` and `unset` false (R1).

**Files:**
- Modify: `core/reports/lib/params.ts` (`RANGE_PRESETS`, `PRESET_LABELS`, `periodWords`, `emptyMessage`, `emptyAction`)
- Modify: `core/reports/lib/view-url.ts` (`ViewOptions.sort`, `SORT_KEY`, `encodeSort`, `decodeSort`, parse and serialize)
- Test: `core/reports/lib/__tests__/params.test.ts`, `core/reports/lib/__tests__/view-url.test.ts`

**Interfaces:**
- Consumes: `ViewSort` (Task 10, `lib/run-client.ts`).
- Produces: preset `'allDates'` (label "All dates", the last range preset); `ViewOptions.sort: ViewSort | null`; `SORT_KEY = 's'`; `encodeSort(sort: ViewSort): string`; `decodeSort(raw: string | undefined): ViewSort | null`.

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/__tests__/params.test.ts`, add `presetsFor` to the import from `'../params'` and append:

```ts
describe('All dates', () => {
    // R1: the server resolves All dates as a set range with no days.
    const allDates = { id: 'period', type: 'dateRange' as const }

    it('is the last range preset, and only a range offers it', () => {
        const presets = presetsFor(PERIOD)
        expect(presets[presets.length - 1]).toBe('allDates')
        expect(presetsFor(AS_OF)).not.toContain('allDates')
    })

    it('names the period on the chip, in the header and when nothing matches', () => {
        expect(chipText(PERIOD, { kind: 'preset', preset: 'allDates' }, allDates)).toBe(
            'Period: All dates'
        )
        expect(periodWords(PERIOD, allDates)).toBe('All dates')
        expect(emptyMessage(PERIOD, allDates)).toBe('No rows for all dates')
    })

    it('keeps an unset range without words', () => {
        expect(periodWords(PERIOD, { ...allDates, unset: true })).toBeNull()
        expect(emptyMessage(PERIOD, { ...allDates, unset: true })).toBe('No rows')
    })

    it('offers no narrower period when All dates has no rows', () => {
        expect(emptyAction([PERIOD], { period: { kind: 'preset', preset: 'allDates' } })).toBeNull()
    })

    it('reads All dates as a default', () => {
        expect(defaultValueOf({ ...PERIOD, default: { preset: 'allDates' } })).toEqual({
            kind: 'preset',
            preset: 'allDates',
        })
    })
})
```

In `core/reports/lib/__tests__/view-url.test.ts`:

1. Import `decodeSort`, `encodeSort` and `SORT_KEY` with the others from `'../view-url'`.
2. In `'round-trips params and display options'`, add `sort: { col: 'cost', dir: 'desc' },` to `options` after `display: {…},` and add `s: 'cost:desc',` to the `toMatchObject` expectation.
3. Add `sort: null,` after `display: …` in each other `ViewOptions` literal: `'writes nothing for defaults…'` (and add `'s'` to its `arrayContaining` key list), `'writes an amount…'` (the `serializeViewOptions([BUDGET], {…})` call), `'keeps a whole group picked from a source…'` and `'builds a full link the reader can copy'`.
4. In `'falls back to the default for a value the report no longer offers'`, add `s: 'cost:sideways',` to the parsed search and `expect(parsed.sort).toBeNull()` after the `display` assertion.
5. Append inside `describe('the view URL', …)`:

```ts
    it('reads a sort as <column>:<asc|desc> and ignores any other shape', () => {
        expect(SORT_KEY).toBe('s')
        expect(parseViewOptions(DEFS, { s: 'amount:desc' }).sort).toEqual({
            col: 'amount',
            dir: 'desc',
        })
        expect(parseViewOptions(DEFS, { s: 'amount_q1.p0:asc' }).sort).toEqual({
            col: 'amount_q1.p0',
            dir: 'asc',
        })
        for (const bad of ['amount', 'amount:up', ':asc', 'amount:asc:desc', '<b>:asc', ''])
            expect(decodeSort(bad)).toBeNull()
        expect(decodeSort(undefined)).toBeNull()
        expect(encodeSort({ col: 'amount', dir: 'asc' })).toBe('amount:asc')
    })

    it('writes the sort, and leaves it out with none', () => {
        const sorted = serializeViewOptions(DEFS, {
            params: {},
            display: DEFAULT_DISPLAY,
            sort: { col: 'amount', dir: 'desc' },
        })
        expect(sorted.s).toBe('amount:desc')
        const plain = serializeViewOptions(DEFS, { params: {}, display: DEFAULT_DISPLAY, sort: null })
        expect('s' in plain).toBe(true)
        expect(plain.s).toBeUndefined()
    })

    it('keeps the sort in a copied link', () => {
        const link = reportLink('https://acme.example', 'r1', DEFS, {
            params: {},
            display: DEFAULT_DISPLAY,
            sort: { col: 'amount', dir: 'desc' },
        })
        expect(link).toBe('https://acme.example/a/reports/r1?s=amount%3Adesc')
    })

    it('reads All dates from the address', () => {
        expect(parseViewOptions(DEFS, { 'p-period': 'allDates' }).params.period).toEqual({
            kind: 'preset',
            preset: 'allDates',
        })
    })
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/params.test.ts core/reports/lib/__tests__/view-url.test.ts`
Expected: FAIL — `presetsFor(PERIOD)` ends with `'last12Months'`; `periodWords` is null for a range with no days; `decodeSort`/`encodeSort`/`SORT_KEY` are not exported (`TypeError: … is not a function`); `parseViewOptions(…).sort` is undefined.

- [ ] **Step 3: `lib/params.ts`**

Append `'allDates',` to `RANGE_PRESETS` after `'last12Months',`, and to `PRESET_LABELS` after `last12Months: 'Last 12 months',`:

```ts
    allDates: 'All dates',
```

Replace `periodWords` and `emptyMessage`:

```ts
/**
 * The period in words for the document header: the days the server ran. A set
 * date range with no days is All dates (the server resolves it with none, R1).
 */
export function periodWords(
    def: ParamDef | undefined,
    resolved: ResolvedParam | undefined
): string | null {
    if (!def || !resolved || resolved.unset) return null
    if (def.type === 'dateRange')
        return resolved.from && resolved.to
            ? rangeWords(resolved.from, resolved.to)
            : PRESET_LABELS.allDates
    if (typeof resolved.value !== 'string' || !isIsoDay(resolved.value)) return null
    return def.type === 'asOf' ? `As of ${dayWords(resolved.value)}` : dayWords(resolved.value)
}

export function emptyMessage(
    def: ParamDef | undefined,
    resolved: ResolvedParam | undefined
): string {
    const words = periodWords(def, resolved)
    if (!def || !words) return 'No rows'
    if (words === PRESET_LABELS.allDates) return 'No rows for all dates'
    if (def.type === 'dateRange') return `No rows for ${words}`
    if (def.type === 'asOf') return `No rows as of ${words.replace(/^As of /, '')}`
    return `No rows on ${words}`
}
```

In `emptyAction`, replace

```ts
    const current = effectiveValue(period, values)
    if (current?.kind === 'preset' && current.preset === target) return null
```

with

```ts
    // All dates is the widest period: a narrower one has no rows either.
    const current = effectiveValue(period, values)
    const isWidest =
        current?.kind === 'preset' && (current.preset === target || current.preset === 'allDates')
    if (isWidest) return null
```

- [ ] **Step 4: `lib/view-url.ts`**

Add `import type { ViewSort } from './run-client'` with the imports. After `const COLUMN_ID = …` add:

```ts
/** The reader's sort: `s=<column>:<asc|desc>` (F8). */
export const SORT_KEY = 's'
const SORT_SEPARATOR = ':'
```

Replace `ViewOptions`:

```ts
export interface ViewOptions {
    params: Record<string, ParamValue>
    display: DisplayOptions
    /** The reader's sort of the detail rows; null: the report's own order. */
    sort: ViewSort | null
}
```

After `parseHidden` add:

```ts
export function encodeSort(sort: ViewSort): string {
    return `${sort.col}${SORT_SEPARATOR}${sort.dir}`
}

// A sort of another shape is ignored, as a bad param value is: the report then
// runs in its own order. A column the report no longer has is the server's to
// refuse (the view offers Clear sort, R8).
export function decodeSort(raw: string | undefined): ViewSort | null {
    const [col, dir, extra] = (raw ?? '').split(SORT_SEPARATOR)
    if (!col || !COLUMN_ID.test(col) || extra !== undefined) return null
    if (dir !== 'asc' && dir !== 'desc') return null
    return { col, dir }
}
```

In `parseViewOptions`, add `sort: decodeSort(first(search[SORT_KEY])),` after the `display: {…},` member of the returned object. In `serializeViewOptions`, after `out.hide = …` add:

```ts
    out[SORT_KEY] = options.sort ? encodeSort(options.sort) : undefined
```

`hooks/useReportView.ts` spreads `options` into every write, so it keeps the sort with no change.

- [ ] **Step 5: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/params.test.ts core/reports/lib/__tests__/view-url.test.ts`
Expected: PASS.

- [ ] **Step 6: Check and commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/params.ts core/reports/lib/view-url.ts core/reports/lib/__tests__/params.test.ts core/reports/lib/__tests__/view-url.test.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```

Expected: PASS. The driver commits:

```bash
git add core/reports/lib/params.ts core/reports/lib/view-url.ts core/reports/lib/__tests__/params.test.ts core/reports/lib/__tests__/view-url.test.ts
git commit -m "feat(core): the All dates period and a reader's sort in the report link"
```

---

### Task 12: Client — rows key with the sort, grouped pages merge

Client facts used: `rowsKeyOf` keys a saved report on its id and a draft on its spec without `display`, and leaves every option out (`lib/run-client.ts:41-51`); its value is `ReportRun.shownRowsKey` (`hooks/useReportRun.ts:58,76`), which `ReportView.shownRunKey` (`ReportView.tsx:129-131`) turns into the list's `key` (`ReportTable.tsx:313`), so a new rows key starts the list at the top (F11); `mergePages` keeps only earlier pages' detail rows (`lib/rows.ts:143-158`), which drops a grouped page's group, opening and subtotal rows. Task 8 makes grouped runs page: each page holds its detail rows plus the group, opening and subtotal rows between them in document order and ends with the Total (R9e).

**Files:**
- Modify: `core/reports/lib/run-client.ts` (`rowsKeyOf`)
- Modify: `core/reports/lib/rows.ts` (`mergePages`)
- Modify: `core/reports/hooks/useReportRun.ts:37-38` (comment only)
- Test: `core/reports/lib/__tests__/run-client.test.ts`, `core/reports/lib/__tests__/rows.test.ts`

**Interfaces:**
- Consumes: `RunOptions.sort` (Task 10); row kind `'opening'` (Task 10).
- Produces: `rowsKeyOf(body)` — byte-identical without a sort, `…|sort:<JSON of options.sort>` with one (R11); `mergePages(pages)` — every row of each page in order, earlier pages' `total`/`computed` rows dropped, a repeated row id kept once (R10).

- [ ] **Step 1: Write the failing tests**

Append inside `describe('rowsKeyOf', …)` in `core/reports/lib/__tests__/run-client.test.ts`:

```ts
    // R11: no sort keeps today's key, so no list resets; each sort is its own list.
    it('adds the reader’s sort to the key, and only the sort', () => {
        const desc = [{ col: 'amount', dir: 'desc' as const }]
        expect(rowsKeyOf({ reportId: 'r1', params: {}, options })).toBe('report:r1')
        expect(rowsKeyOf({ reportId: 'r1', params: {}, options: { ...options, sort: [] } })).toBe(
            'report:r1'
        )
        const sorted = rowsKeyOf({ reportId: 'r1', params: {}, options: { ...options, sort: desc } })
        expect(sorted).toBe('report:r1|sort:[{"col":"amount","dir":"desc"}]')
        expect(
            rowsKeyOf({
                reportId: 'r1',
                params: {},
                options: { ...options, sort: [{ col: 'amount', dir: 'asc' }] },
            })
        ).not.toBe(sorted)
        expect(
            rowsKeyOf({
                reportId: 'r1',
                params: {},
                options: { ...options, sort: desc, expand: [['open']], cursor: 'c' },
            })
        ).toBe(sorted)

        const draft = rowsKeyOf({ spec, params: {}, options })
        expect(draft).toBe(`spec:${JSON.stringify(spec)}`)
        expect(rowsKeyOf({ spec, params: {}, options: { ...options, sort: desc } })).toBe(
            `${draft}|sort:[{"col":"amount","dir":"desc"}]`
        )
    })
```

Append inside `describe('mergePages', …)` in `core/reports/lib/__tests__/rows.test.ts` (it uses that block's `page` helper and the file's `group`, `detail`, `subtotal`, `total`):

```ts
    const opening = (key: string, section: string): ResultRow => ({
        id: `opening:["${key}"]`,
        kind: 'opening',
        depth: 1,
        label: 'Before the period',
        section,
        cells: { cost: 10 },
    })

    // R9/R10: a grouped page carries the sections between its detail rows.
    it('appends the next grouped page’s sections after the first page’s rows', () => {
        const first: RunResult = {
            ...page([], 0),
            rows: [
                group('g:a', ['a'], { expanded: true }),
                opening('a', 'g:a'),
                detail('d:1', 'g:a'),
                detail('d:2', 'g:a'),
                { ...total, cells: { cost: 300 } },
            ],
            page: { size: 2, next: 'c1' },
        }
        const second: RunResult = {
            ...page([], 0),
            rows: [
                detail('d:3', 'g:a'),
                subtotal('s:a', 'g:a'),
                group('g:b', ['b'], { expanded: true }),
                opening('b', 'g:b'),
                detail('d:4', 'g:b'),
                subtotal('s:b', 'g:b'),
                { ...total, cells: { cost: 400 } },
            ],
            page: { size: 2 },
        }
        const merged = mergePages([first, second])
        expect(merged.rows.map(r => r.id)).toEqual([
            'g:a',
            'opening:["a"]',
            'd:1',
            'd:2',
            'd:3',
            's:a',
            'g:b',
            'opening:["b"]',
            'd:4',
            's:b',
            'total',
        ])
        expect(merged.rows[merged.rows.length - 1]?.cells.cost).toBe(400)
        expect(merged.page).toEqual({ size: 2 })
    })

    it('keeps one copy of an opening or subtotal row two pages both give', () => {
        const first: RunResult = {
            ...page([], 0),
            rows: [group('g:a', ['a'], { expanded: true }), opening('a', 'g:a'), detail('d:1', 'g:a'), total],
        }
        const second: RunResult = {
            ...page([], 0),
            rows: [
                opening('a', 'g:a'),
                detail('d:1', 'g:a'),
                detail('d:2', 'g:a'),
                subtotal('s:a', 'g:a'),
                total,
            ],
        }
        const third: RunResult = { ...page([], 0), rows: [subtotal('s:a', 'g:a'), total] }
        expect(mergePages([first, second, third]).rows.map(r => r.id)).toEqual([
            'g:a',
            'opening:["a"]',
            'd:1',
            'd:2',
            's:a',
            'total',
        ])
    })
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/run-client.test.ts core/reports/lib/__tests__/rows.test.ts`
Expected: FAIL — the sorted key is `'report:r1'` (options are ignored); the merged grouped rows start `['d:1', 'd:2', 'd:3', …]` (page 1's group and opening rows dropped) and the seam test holds `'d:1'` and `'s:a'` twice.

- [ ] **Step 3: `rowsKeyOf`**

Replace the comment above `rowsKeyOf` and the function in `lib/run-client.ts`:

```ts
// What decides a run's rows, apart from its params (the server resolves those and
// returns them on the result): the saved report, or the draft spec without its
// display, which holds only line layouts per width, and the reader's sort. Other
// options are left out, so a group fold, the next page or a new day keeps the
// reader's place; a new sort is a new order, so its list starts at the top (F11).
// Without a sort the key is as it always was (R11). The param definitions
// (spec.params) stay in the key on purpose: a param-def edit is a design edit.
export function rowsKeyOf(body: RunBody): string {
    const sort = body.options.sort?.length ? `|sort:${JSON.stringify(body.options.sort)}` : ''
    if ('reportId' in body) return `report:${body.reportId}${sort}`
    const { display: _display, ...rowsSpec } = body.spec
    return `spec:${JSON.stringify(rowsSpec)}${sort}`
}
```

- [ ] **Step 4: `mergePages`**

Replace the comment above `mergePages` and the function in `lib/rows.ts` (and add `ResultRow` to the type import if it is not there; it is):

```ts
// Rows every page repeats with the newest figures: only the newest page's stay.
const WHOLE_REPORT_ROWS: ReadonlySet<ResultRow['kind']> = new Set(['total', 'computed'])

// Every report pages by keyset (R9). A page holds its detail rows and, in a grouped
// report, the group, opening and subtotal rows between them in document order, then
// the whole-report rows (total, computed). Pages are joined in order: each earlier
// page's whole-report rows go, since only the newest page's figures are current,
// and a row whose id an earlier page gave is kept once (R10), so a seam never shows
// a section's row twice.
export function mergePages(pages: readonly RunResult[]): RunResult {
    const last = pages[pages.length - 1]
    if (!last) return EMPTY_RESULT
    const seen = new Set<string>()
    const rows: ResultRow[] = []
    for (const page of pages) {
        const isLast = page === last
        for (const row of page.rows) {
            if (seen.has(row.id) || (!isLast && WHOLE_REPORT_ROWS.has(row.kind))) continue
            seen.add(row.id)
            rows.push(row)
        }
    }
    return {
        ...last,
        rows,
        notices: uniqueNotices(pages.flatMap(page => page.notices)),
    }
}
```

`'takes computed and total rows from the newest page only'` and `'keeps every page’s detail rows and the newest total'` still pass: the last page's `total` id is first seen there, because earlier pages skip theirs.

- [ ] **Step 5: `useReportRun.ts` comment**

Replace the comment at `hooks/useReportRun.ts:37-38`:

```ts
// Runs a report through POST /api/reports/run (R9). Every report pages by keyset; a
// grouped one pages its detail rows in document order with the section rows between
// them (mergePages joins the pages). The result's topics keep it live.
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/run-client.test.ts core/reports/lib/__tests__/rows.test.ts core/reports/lib/__tests__/run-state.test.ts`
Expected: PASS (`run-state.test.ts` merges through `mergePages` and is unchanged).

- [ ] **Step 7: Check and commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/run-client.ts core/reports/lib/rows.ts core/reports/hooks/useReportRun.ts core/reports/lib/__tests__/run-client.test.ts core/reports/lib/__tests__/rows.test.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```

Expected: PASS. The driver commits:

```bash
git add core/reports/lib/run-client.ts core/reports/lib/rows.ts core/reports/hooks/useReportRun.ts core/reports/lib/__tests__/run-client.test.ts core/reports/lib/__tests__/rows.test.ts
git commit -m "feat(core): a sort starts the report list at the top; grouped pages join without repeats"
```

---

### Task 13: F10 lib — fit, never clip; figure-only rows

Client facts used: `tableLines` picks an authored layout for the breakpoint, else one line on every desktop (the sideways scroller holds the rest) or where `oneLineFits`, else `autoLines` (`lib/table-lines.ts:286-298`); `rowLayout` draws a row with no cells as its label alone (`:300-304`); natural widths `LABEL_MIN_WIDTH` 220 / `PHONE_LABEL_WIDTH` 168, `AMOUNT_WIDTH` 128 / `PHONE_AMOUNT_WIDTH` 108 (`components/report-table-layout.ts:5-10`); `fitsOnOneLine` (`:42-52`) is the only fit test, called once by `ReportTable.tsx:301`; the layout block tree (`Block`, `stackOf`, `build`) is in `table-lines.ts:25-186`. Rules: head "Report table layout per width (F10)", R12 (widths, `TEXT_FLOOR` 80, figure types, the label narrows first, then text columns by water-fill), R13 (order), R14 (figure-only rows), Q1 recommended (an authored layout for the breakpoint applies even when the line fits).

`fitOneLine` lives in `lib/table-lines.ts` (head "Client lib changes"); the widths it reads, and `TEXT_FLOOR`, stay in `components/report-table-layout.ts`, a pure module (no React Native import), so the lib imports them from there. Its water-fill cases are in `components/__tests__/report-table-layout.test.ts`, next to the widths they pin. `reportTableLayout`, `ReportTableLayout` and `scrollsSideways` are deleted in Task 14 together with their only user, the sideways scroller in `ReportTable.tsx`; this task removes only `fitsOnOneLine`.

**Files:**
- Modify: `core/reports/lib/table-lines.ts` (`LineLayout`, `isFigureColumn`, `fitOneLine`, `tableLines`, `rowLayout`)
- Modify: `core/reports/components/report-table-layout.ts` (`TEXT_FLOOR`; `fitsOnOneLine` removed)
- Modify: `core/reports/components/ReportTable.tsx:296-305` (the `tableLines` call without `oneLineFits`)
- Test: `core/reports/lib/__tests__/table-lines.test.ts`, `core/reports/components/__tests__/report-table-layout.test.ts`

**Interfaces:**
- Consumes: `RowKind` `'opening'` (Task 10).
- Produces: `TEXT_FLOOR = 80` (`report-table-layout.ts`); `isFigureColumn(column: ResultColumn): boolean`; `fitOneLine(columns: readonly ResultColumn[], width: number, breakpoint: Breakpoint): Record<string, number> | null` (the label column's entry is its minimum width; it is `flex: 1` and takes what is left); `LineLayout = { kind: 'one-line'; widths: Record<string, number> } | { kind: 'blocks'; root: Block; notice?: string }`; `tableLines({ columns, width, breakpoint, layouts })`; `rowLayout(item, layout, labelId)` pruned for every kind but `detail` (R14).

- [ ] **Step 1: Write the failing fit tests**

In `core/reports/components/__tests__/report-table-layout.test.ts`, replace the import of `fitsOnOneLine` with `TEXT_FLOOR`, add `import { fitOneLine, isFigureColumn } from '../../lib/table-lines'`, and replace `describe('fitsOnOneLine', …)` with:

```ts
// A wide report: the label, five text columns and three amounts. Its natural line
// is 220 + 8 × 128 = 1244; narrowed to the floor it is 80 + 5 × 80 + 3 × 128 = 864.
const WIDE: ResultColumn[] = [
    { id: 'customer', label: 'Customer', type: 'text' },
    { id: 'status', label: 'Status', type: 'select' },
    { id: 'ref', label: 'Ref', type: 'text' },
    { id: 'team', label: 'Team', type: 'relation' },
    { id: 'note', label: 'Note', type: 'text' },
    { id: 'other', label: 'Other', type: 'text' },
    { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
    { id: 'cost', label: 'Cost', type: 'money', currency: 'USD' },
    { id: 'balance', label: 'Balance', type: 'money', currency: 'USD' },
]
const TEXT_IDS = ['status', 'ref', 'team', 'note', 'other']
const wideWidths = (label: number, text: number, amount: number) => ({
    customer: label,
    ...Object.fromEntries(TEXT_IDS.map(id => [id, text] as const)),
    amount,
    cost: amount,
    balance: amount,
})

describe('fitOneLine', () => {
    it('keeps every natural width where the line fits', () => {
        expect(fitOneLine(WIDE, 1244, 'desktop')).toEqual(
            wideWidths(LABEL_MIN_WIDTH, AMOUNT_WIDTH, AMOUNT_WIDTH)
        )
        expect(fitOneLine(WIDE, 1600, 'desktop')).toEqual(
            wideWidths(LABEL_MIN_WIDTH, AMOUNT_WIDTH, AMOUNT_WIDTH)
        )
    })

    it('narrows the label first', () => {
        expect(fitOneLine(WIDE, 1200, 'desktop')).toEqual(wideWidths(176, 128, 128))
    })

    it('then lowers the text columns to one cap, the figures kept', () => {
        // 1100 − 80 − 3 × 128 = 636 for five text columns: 127 each.
        expect(fitOneLine(WIDE, 1100, 'desktop')).toEqual(wideWidths(TEXT_FLOOR, 127, 128))
        expect(fitOneLine(WIDE, 864, 'desktop')).toEqual(wideWidths(TEXT_FLOOR, TEXT_FLOOR, 128))
    })

    it('does not fit below the floor', () => {
        expect(fitOneLine(WIDE, 863, 'desktop')).toBeNull()
    })

    it('keeps a number, a percent, a date, a time and a yes/no at full width', () => {
        const columns: ResultColumn[] = [
            { id: 'customer', label: 'Customer', type: 'text' },
            { id: 'count', label: 'Count', type: 'number' },
            { id: 'share', label: 'Share', type: 'percent' },
            { id: 'day', label: 'Day', type: 'date' },
            { id: 'booked', label: 'Booked', type: 'datetime' },
            { id: 'done', label: 'Done', type: 'bool' },
            { id: 'note', label: 'Note', type: 'text' },
        ]
        // 820 − 80 − 5 × 128 = 100 for the note.
        expect(fitOneLine(columns, 820, 'desktop')).toEqual({
            customer: TEXT_FLOOR,
            count: 128,
            share: 128,
            day: 128,
            booked: 128,
            done: 128,
            note: 100,
        })
        expect(fitOneLine(columns.slice(0, 6), 719, 'desktop')).toBeNull()
    })

    it('uses the phone widths on a phone', () => {
        const columns: ResultColumn[] = [
            { id: 'customer', label: 'Customer', type: 'text' },
            { id: 'ref', label: 'Ref', type: 'text' },
            { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
            { id: 'cost', label: 'Cost', type: 'money', currency: 'USD' },
        ]
        expect(fitOneLine(columns, 492, 'mobile')).toEqual({
            customer: PHONE_LABEL_WIDTH,
            ref: PHONE_AMOUNT_WIDTH,
            amount: PHONE_AMOUNT_WIDTH,
            cost: PHONE_AMOUNT_WIDTH,
        })
        // 390 − 80 − 2 × 108 = 94 for Ref.
        expect(fitOneLine(columns, 390, 'mobile')).toEqual({
            customer: TEXT_FLOOR,
            ref: 94,
            amount: 108,
            cost: 108,
        })
        expect(fitOneLine(columns, 375, 'mobile')).toBeNull()
    })

    it('fits a lone label down to the floor', () => {
        const label: ResultColumn[] = [{ id: 'customer', label: 'Customer', type: 'text' }]
        expect(fitOneLine(label, 100, 'desktop')).toEqual({ customer: 100 })
        expect(fitOneLine(label, 79, 'desktop')).toBeNull()
        expect(fitOneLine([], 10, 'desktop')).toEqual({})
    })
})

describe('isFigureColumn', () => {
    it('is a column whose values an ellipsis would ruin', () => {
        for (const type of ['number', 'money', 'percent', 'date', 'datetime', 'bool'] as const)
            expect(isFigureColumn({ id: 'x', label: 'X', type })).toBe(true)
        for (const type of ['text', 'select', 'relation'] as const)
            expect(isFigureColumn({ id: 'x', label: 'X', type })).toBe(false)
    })
})
```

- [ ] **Step 2: Write the failing layout tests**

In `core/reports/lib/__tests__/table-lines.test.ts`, delete the line `oneLineFits: …,` from every `tableLines({…})` call (18 calls; the option is gone). Then change these existing tests, whose widths the narrowed line now fits (COLS at 390 on a phone narrows to 80 + 94 + 2 × 108 = 390):

1. Replace `'keeps one line on desktop and wherever the columns fit'` with:

```ts
    it('draws one line with its natural widths wherever the columns fit', () => {
        const natural = { account: 220, number: 128, debit: 128, credit: 128 }
        expect(
            tableLines({ columns: COLS, width: 1280, breakpoint: 'desktop', layouts: [] })
        ).toEqual({ kind: 'one-line', widths: natural })
        expect(tableLines({ columns: COLS, width: 900, breakpoint: 'tablet', layouts: [] })).toEqual(
            { kind: 'one-line', widths: natural }
        )
    })
```

2. In `'wraps a phone row: …'`, `'uses the arrangement for its own width only, …'` (both the `auto` call and the `refused` call), `'a row with no figures draws only its label'` and `'keeps the wrapped lines of a detail row whose No. is blank'`, change `width: 390` to `width: 360` (360 − 80 − 216 = 64 is under the floor, so COLS still wraps). Leave every call that has an authored `mobile` layout, and the `PIVOTS` calls, at 390.
3. In `'uses the arrangement for its own width only, …'`, change the desktop expectation `.toEqual({ kind: 'one-line' })` to `.toMatchObject({ kind: 'one-line' })`.
4. Replace the second assertion of `'a row with no figures draws only its label'` (the `cells: { debit: 5 }` one) with:

```ts
        const full = { number: 'A1', debit: 5, credit: 0 }
        expect(rowLayout({ row, cells: full, isToggle: false, isOpen: false }, layout, 'account')).toBe(
            layout
        )
```

Add `fitOneLine` to the import from `'../table-lines'`, and append:

```ts
// F10: one line when it fits (narrowed if need be), else the nearest narrower
// arrangement, else the automatic lines (R13). WIDE narrows to 864 at best.
const WIDE: ResultColumn[] = [
    { id: 'customer', label: 'Customer', type: 'text' },
    { id: 'status', label: 'Status', type: 'select' },
    { id: 'ref', label: 'Ref', type: 'text' },
    { id: 'team', label: 'Team', type: 'relation' },
    { id: 'note', label: 'Note', type: 'text' },
    { id: 'other', label: 'Other', type: 'text' },
    { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
    { id: 'cost', label: 'Cost', type: 'money', currency: 'USD' },
    { id: 'balance', label: 'Balance', type: 'money', currency: 'USD' },
]
const DESKTOP_LAYOUT = {
    width: 'desktop' as const,
    tracks: 2,
    cells: [
        { col: 'customer', line: 1, span: 2 },
        { col: 'amount', line: 2 },
        { col: 'balance', line: 2 },
    ],
}
const TABLET_LAYOUT = {
    width: 'tablet' as const,
    tracks: 3,
    cells: [
        { col: 'customer', line: 1, span: 3 },
        { col: 'amount', line: 2 },
        { col: 'cost', line: 2 },
        { col: 'balance', line: 2 },
    ],
}
const MOBILE_LAYOUT = {
    width: 'mobile' as const,
    tracks: 2,
    cells: [
        { col: 'customer', line: 1, span: 2 },
        { col: 'balance', line: 2, start: 2 },
    ],
}
const label = { kind: 'cell', col: 'customer', justify: 'start' } as const
const money = (col: string) => ({ block: { kind: 'cell', col, justify: 'end' } as const, tracks: 1 })
const DESKTOP_LINES = {
    kind: 'blocks',
    root: { kind: 'stack', parts: [label, { kind: 'side', of: 2, parts: [money('amount'), money('balance')] }] },
}
const TABLET_LINES = {
    kind: 'blocks',
    root: {
        kind: 'stack',
        parts: [label, { kind: 'side', of: 3, parts: [money('amount'), money('cost'), money('balance')] }],
    },
}
const MOBILE_LINES = {
    kind: 'blocks',
    root: {
        kind: 'stack',
        parts: [
            label,
            { kind: 'side', of: 2, parts: [{ block: { kind: 'gap', lines: 1 }, tracks: 1 }, money('balance')] },
        ],
    },
}

describe('F10: one line that fits, then the narrower arrangements', () => {
    const lines = (width: number, breakpoint: 'desktop' | 'tablet' | 'mobile', layouts = [] as DisplayLayout[]) =>
        tableLines({ columns: WIDE, width, breakpoint, layouts })

    it('draws a desktop line that fits with its natural widths', () => {
        expect(lines(1300, 'desktop')).toEqual({
            kind: 'one-line',
            widths: fitOneLine(WIDE, 1300, 'desktop'),
        })
        expect(lines(1300, 'desktop')).toMatchObject({ widths: { customer: 220, note: 128, amount: 128 } })
    })

    it('narrows the label, then the text columns, before it wraps', () => {
        expect(lines(1100, 'desktop')).toEqual({
            kind: 'one-line',
            widths: {
                customer: 80,
                status: 127,
                ref: 127,
                team: 127,
                note: 127,
                other: 127,
                amount: 128,
                cost: 128,
                balance: 128,
            },
        })
        expect(lines(864, 'desktop')).toMatchObject({ kind: 'one-line', widths: { note: 80, amount: 128 } })
    })

    // Q1 (recommended): an author's arrangement for this width replaces the line.
    it('uses the authored desktop arrangement where there is one, even when the line fits', () => {
        expect(lines(1300, 'desktop', [DESKTOP_LAYOUT, TABLET_LAYOUT])).toEqual(DESKTOP_LINES)
    })

    // The brief's "desktop that wraps to an authored desktop layout": at 800 the
    // narrowed line (864) does not fit, and the desktop arrangement wins over the
    // tablet one. This holds under either answer to Q1.
    it('wraps a desktop too narrow for the line to its own authored arrangement', () => {
        expect(lines(800, 'desktop', [DESKTOP_LAYOUT, TABLET_LAYOUT])).toEqual(DESKTOP_LINES)
    })

    it('wraps a desktop too narrow for the narrowed line to the tablet arrangement, then the phone one, then automatically', () => {
        expect(lines(800, 'desktop', [MOBILE_LAYOUT, TABLET_LAYOUT])).toEqual(TABLET_LINES)
        expect(lines(800, 'desktop', [MOBILE_LAYOUT])).toEqual(MOBILE_LINES)
        const auto = lines(800, 'desktop')
        expect(auto.kind).toBe('blocks')
        expect(auto).toEqual(lines(800, 'tablet'))
        expect(auto).not.toEqual(TABLET_LINES)
    })

    it('wraps a tablet in the same order, never to a wider arrangement', () => {
        expect(lines(700, 'tablet', [DESKTOP_LAYOUT, MOBILE_LAYOUT])).toEqual(MOBILE_LINES)
        expect(lines(700, 'tablet', [DESKTOP_LAYOUT])).toEqual(lines(700, 'mobile'))
        expect(lines(1000, 'tablet', [DESKTOP_LAYOUT])).toMatchObject({ kind: 'one-line' })
    })
})

// R14: a row that is not a detail row draws only the lines it has a value on.
describe('figure-only rows in a wrapped layout', () => {
    const PHONE = {
        width: 'mobile' as const,
        tracks: 3,
        cells: [
            { col: 'customer', line: 1, span: 3 },
            { col: 'status', line: 2 },
            { col: 'ref', line: 2 },
            { col: 'team', line: 3, span: 3 },
            { col: 'amount', line: 4 },
            { col: 'cost', line: 4 },
            { col: 'balance', line: 4 },
        ],
    }
    const COLUMNS = WIDE.filter(c => c.id !== 'note' && c.id !== 'other')
    const layout = tableLines({ columns: COLUMNS, width: 390, breakpoint: 'mobile', layouts: [PHONE] })
    const item = (kind: ResultRow['kind'], cells: Record<string, CellValue>) => ({
        row: { id: `${kind}:1`, kind, depth: 1, label: 'Before the period', cells },
        cells,
        isToggle: false,
        isOpen: false,
    })
    const gap = { block: { kind: 'gap', lines: 1 } as const, tracks: 1 }

    it('draws an opening row as its label line, then its figures line', () => {
        expect(rowLayout(item('opening', { balance: 1200 }), layout, 'customer')).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [label, { kind: 'side', of: 3, parts: [gap, gap, money('balance')] }],
            },
        })
    })

    it('draws a subtotal without its blank text lines, and keeps a blank figure’s track', () => {
        const cells = { amount: 900, cost: 300, balance: null }
        expect(rowLayout(item('subtotal', cells), layout, 'customer')).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [label, { kind: 'side', of: 3, parts: [money('amount'), money('cost'), gap] }],
            },
        })
    })

    it('keeps every line of a detail row, and of any row with a value in every column', () => {
        expect(rowLayout(item('detail', { balance: 1200 }), layout, 'customer')).toBe(layout)
        const full = { status: 'open', ref: 'A1', team: 't1', amount: 1, cost: 2, balance: 3 }
        expect(rowLayout(item('total', full), layout, 'customer')).toBe(layout)
    })
})
```

Add `DisplayLayout` to the type import from `'../spec'`, and `CellValue, ResultRow` to the type import from `'../types'`.

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/table-lines.test.ts core/reports/components/__tests__/report-table-layout.test.ts`
Expected: FAIL — `fitOneLine`/`isFigureColumn` are not exported (`TypeError: … is not a function`), `TEXT_FLOOR` is undefined; `tableLines` returns `{ kind: 'one-line' }` with no widths on every desktop (and, with `oneLineFits` gone, wraps a tablet that fits); `rowLayout` returns the layout unchanged for an opening row. One new case passes already: 'wraps a desktop too narrow for the line to its own authored arrangement' (v2 already applies the author's arrangement for the breakpoint, `table-lines.ts:286-298`); it pins that F10 keeps that order. Show it red by a mutation: move the `own` check in the new `tableLines` (Step 5) below the `fitOneLine` fallbacks, see "expected { kind: 'blocks', root: … 'cost' … } to deeply equal { kind: 'blocks', … }" (the tablet lines), and revert.

- [ ] **Step 4: `report-table-layout.ts`**

Delete `fitsOnOneLine` (and its comment). After `export const INDENT = 16` add:

```ts
/**
 * The narrowest a text column (and the label) gets on one line (R12): 8 average
 * characters at text-sm (8 × 7) plus the cell's px-3 padding on both sides.
 * Contents are not measured, so the layout does not change as pages load.
 */
export const TEXT_FLOOR = 80
```

- [ ] **Step 5: `table-lines.ts`**

Replace the imports and `LineLayout`:

```ts
import type { Breakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import {
    AMOUNT_WIDTH,
    LABEL_MIN_WIDTH,
    PHONE_AMOUNT_WIDTH,
    PHONE_LABEL_WIDTH,
    TEXT_FLOOR,
} from '../components/report-table-layout'
import { isRightAligned } from './format'
import type { VisibleRow } from './rows'
import type { DisplayCell, DisplayLayout, DisplayWidth } from './spec'
import type { CellValue, FieldType, ResultColumn } from './types'
```

```ts
/** One line, each column at its width (the label's is its minimum: it takes what is left). */
export type LineLayout =
    | { kind: 'one-line'; widths: Record<string, number> }
    | { kind: 'blocks'; root: Block; notice?: string }
```

Replace `tableLines` and `rowLayout`:

```ts
// Aligned or fixed-length values an ellipsis would ruin: they keep their width (R12).
const FIGURE_TYPES: ReadonlySet<FieldType> = new Set([
    'number',
    'money',
    'percent',
    'date',
    'datetime',
    'bool',
])

export function isFigureColumn(column: ResultColumn): boolean {
    return FIGURE_TYPES.has(column.type)
}

/**
 * The widths of one line at this width (F10, R12), or null when it does not fit:
 * every column at its natural width; if that is too wide, the label narrows toward
 * TEXT_FLOOR, then the text columns are lowered to one cap (water-fill from the
 * widest), each to TEXT_FLOOR at the least. Figure columns keep their width.
 */
export function fitOneLine(
    columns: readonly ResultColumn[],
    width: number,
    breakpoint: Breakpoint
): Record<string, number> | null {
    const [label, ...others] = columns
    if (!label) return {}
    const isPhone = breakpoint === 'mobile'
    const labelMin = isPhone ? PHONE_LABEL_WIDTH : LABEL_MIN_WIDTH
    const cell = isPhone ? PHONE_AMOUNT_WIDTH : AMOUNT_WIDTH
    const figures = others.filter(isFigureColumn)
    const texts = others.filter(column => !isFigureColumn(column))
    const figureWidth = figures.length * cell
    const lineAt = (labelWidth: number, textWidth: number) => {
        const widths: Record<string, number> = { [label.id]: labelWidth }
        for (const column of others) widths[column.id] = isFigureColumn(column) ? cell : textWidth
        return widths
    }
    const natural = labelMin + figureWidth + texts.length * cell
    if (natural <= width) return lineAt(labelMin, cell)
    const labelWidth = Math.max(TEXT_FLOOR, labelMin - (natural - width))
    if (labelWidth + figureWidth + texts.length * cell <= width) return lineAt(labelWidth, cell)
    if (texts.length === 0) return null
    const cap = Math.floor((width - TEXT_FLOOR - figureWidth) / texts.length)
    if (cap < TEXT_FLOOR) return null
    return lineAt(TEXT_FLOOR, Math.min(cell, cap))
}

// The authored widths tried when this width has none and its line does not fit,
// nearest first (R13).
const NARROWER: Record<Breakpoint, readonly DisplayWidth[]> = {
    desktop: ['tablet', 'mobile'],
    tablet: ['mobile'],
    mobile: [],
}

/**
 * The lines of every row (and the heading) at this width (F10): the author's
 * arrangement for this width (Q1); else one line, narrowed if it must be; else the
 * nearest narrower arrangement; else the automatic lines. Never wider than `width`,
 * so the table never scrolls sideways.
 */
export function tableLines(input: {
    columns: readonly ResultColumn[]
    width: number
    breakpoint: Breakpoint
    layouts: readonly DisplayLayout[]
}): LineLayout {
    const { columns, width, breakpoint, layouts } = input
    const own = layouts.find(l => l.width === breakpoint)
    if (own) return authoredLines(own, columns, width)
    const widths = fitOneLine(columns, width, breakpoint)
    if (widths) return { kind: 'one-line', widths }
    const narrower = NARROWER[breakpoint]
        .map(w => layouts.find(l => l.width === w))
        .find(layout => layout !== undefined)
    if (narrower) return authoredLines(narrower, columns, width)
    return autoLines(columns, width)
}

const hasValue = (value: CellValue | undefined) => value !== undefined && value !== null
const LINE_GAP: Block = { kind: 'gap', lines: 1 }

function hasCell(block: Block): boolean {
    if (block.kind === 'cell') return true
    if (block.kind === 'gap') return false
    if (block.kind === 'stack') return block.parts.some(hasCell)
    return block.parts.some(part => hasCell(part.block))
}

function blockColumns(block: Block): string[] {
    if (block.kind === 'cell') return [block.col]
    if (block.kind === 'gap') return []
    if (block.kind === 'stack') return block.parts.flatMap(blockColumns)
    return block.parts.flatMap(part => blockColumns(part.block))
}

// The block without the cells `keeps` refuses: in a side block a refused cell
// leaves a gap, so the others keep their tracks; a stack line, or a side block,
// left with no cell goes (R14). Null when nothing is left.
function pruned(block: Block, keeps: (col: string) => boolean): Block | null {
    if (block.kind === 'gap') return block
    if (block.kind === 'cell') return keeps(block.col) ? block : null
    if (block.kind === 'stack') {
        const parts = block.parts.flatMap(part => {
            const kept = pruned(part, keeps)
            return kept && hasCell(kept) ? [kept] : []
        })
        return parts.length > 0 ? stackOf(parts) : null
    }
    const parts = block.parts.map(part => ({
        block: pruned(part.block, keeps) ?? LINE_GAP,
        tracks: part.tracks,
    }))
    return parts.some(part => hasCell(part.block)) ? { ...block, parts } : null
}

/**
 * A row's lines. A row with no figures (a group its subtotal follows) draws only
 * its label, across the row. A row that is not a detail row (group, opening,
 * subtotal, total, computed) draws only the columns it has a value in, so a phone
 * opening row is its label line, then its figures line (R14); a detail row keeps
 * every line, so a field sits at the same place in each row.
 */
export function rowLayout(item: VisibleRow, layout: LineLayout, labelId: string): LineLayout {
    if (layout.kind === 'one-line') return layout
    const labelOnly: Block = { kind: 'cell', col: labelId, justify: 'start' }
    if (Object.keys(item.cells).length === 0) return { kind: 'blocks', root: labelOnly }
    if (item.row.kind === 'detail') return layout
    const keeps = (col: string) => col === labelId || hasValue(item.cells[col])
    if (blockColumns(layout.root).every(keeps)) return layout
    return { ...layout, root: pruned(layout.root, keeps) ?? labelOnly }
}
```

- [ ] **Step 6: `ReportTable.tsx` calls the new `tableLines`**

Remove `fitsOnOneLine` from the import of `'./report-table-layout'` and replace `ReportTable.tsx:296-305` with:

```ts
    const amountCount = Math.max(columns.length - 1, 0)
    const oneLine = useMemo(
        () => reportTableLayout({ breakpoint, amountCount }),
        [breakpoint, amountCount]
    )
    const layout = useMemo(
        () => tableLines({ columns, width, breakpoint, layouts }),
        [columns, width, breakpoint, layouts]
    )
```

(The rows still draw one line at `oneLine.amountWidth` until Task 14 draws `layout.widths`; this step only keeps the app compiling.)

- [ ] **Step 7: Run the tests to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/table-lines.test.ts core/reports/components/__tests__/report-table-layout.test.ts`
Expected: PASS (the shared display-cell table and the placement tests are unchanged).

- [ ] **Step 8: Check and commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/table-lines.ts core/reports/components/report-table-layout.ts core/reports/components/ReportTable.tsx core/reports/lib/__tests__/table-lines.test.ts core/reports/components/__tests__/report-table-layout.test.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```

Expected: PASS. The driver commits:

```bash
git add core/reports/lib/table-lines.ts core/reports/components/report-table-layout.ts core/reports/components/ReportTable.tsx core/reports/lib/__tests__/table-lines.test.ts core/reports/components/__tests__/report-table-layout.test.ts
git commit -m "feat(core): report lines narrow their text columns before they wrap; figure-only rows drop blank lines"
```

---

### Task 13a — E2E first: the stub team balance report, the v3 e2e spec, and `reports-wrap.spec.ts` changed to F10

F1–F8, F10, F11 (e2e), F1/F2/F5/F6 builder (e2e). Why here: Tasks 14, 15, 16a and 18 change components only, and CLAUDE.md pins front-end behaviour with e2e, not vitest. So their e2e are written now, after the engine (Tasks 1–9) and the client lib (Tasks 10–13) are in, so the stub report loads, and each test is shown red for the reason a later task removes. A test that is already green at this point gets a mutation proof (Step 5). Tasks 14, 15, 16a and 18 turn the red ones green; Tasks 20 and 21 only run the files.

Facts used: the stub's sources and shipped reports (`tests/scripts/reports-stub/server/register.go.txt`: sales fields customer, amount, cost (money, currency field `currency`, `:22`), quantity, day (a PocketBase date), booked (a text day), status; link team; teams name, region); the stub is copied into the workspace by `tests/scripts/scaffold-reports-stub.ts`; a shipped report belongs to the seed admin, and stub sales are read by their owner only, so a spec signs in with `loginAsShippedOwner` and adds its own sales (`reports-source-params.spec.ts:24-33,90-95`); a run that fails `Validate` at boot leaves the report out and logs "shipped reports not reconciled" (`core/server/reports/shipped.go:148,161`); helpers `addTeam`, `addSale`, `addSales`, `listScroller`, `uniqueName`, `chooseParamOption`, `cellOf`, `rowOf`, `hubRow`, `openReportsHub`, `pressToolbar`, `startTeamSales`, `chooseOption`, `saveNewReport`, `SALES_SOURCE` (`tests/e2e/reports-helpers.ts`, `report-view-helpers.ts`), `createInvitedUser` (`tests/e2e/helpers.ts`); detail money cells read "12.00", total and subtotal cells may carry the currency sign (`reports-groups-v2.spec.ts:89,161`); refusal state `report-invalid` (`core/reports/components/ReportView.tsx:108`); period chip `report-param-<id>`, presets `report-period-preset-<preset>`, header `report-document-period`; a copied link is opened in a second context with `loginAs…` then `goto(link)` (`reports-view.spec.ts:170-193`); the run body's `options.limit` rewrite precedent (`reports-source-params.spec.ts:363-378`); the formula editor's test ids: a node's part menu `<id>-part` with one item per part (`chooseOption(p, '<id>-part', '<part>')`), a path picker `<id>-path`, a lookup's source `<id>-source` and pick `<id>-pick`, the read-back line `<formula>-summary`, children `<id>-0`, `<id>-1` (`reports-formulas-v2.spec.ts:34-52`, `reports-groups-v2.spec.ts:265-270`, `components/builder/report-formula.tsx:282-288`); a `PickMenu` row's test id is `${testID}-${item.id}` (`core/ui/pick-menu/index.tsx:23,32`). `reports-wrap.spec.ts` facts: the sideways test (`:385-433`: invited user, `seed(p, 1)` gives one sale "Wrapco 00", a report over sales whose first column is `customer` plus eight added columns — amount, cost, quantity, currency, day, booked, status, team › name — nine in all; its natural line is 220 + 8 × 128 = 1244 pt; fully narrowed 80 + 3 text × 80 (currency, status, team name) + 5 figures × 128 = 960 pt, so a 1100 window, a 1036 table and a 1020 line, draws one narrowed line (R12); the label narrows first, so "Wrapco 00" (about 91 pt with padding) ends in "…"); the phone blank-slot test (`:504-546`: `startTeamSales` + `groupWithTeamRegion`, `reports-helpers.ts:192-224`: customer (label), team (relation, text), amount (money), formula (text)).

The builder test's ids, checked against the code that forms them: a formula node's id is `${testID}-root` at the root and `${testID}-<path joined by ->` below it (`core/components/forms-builder/FormulaEditor.tsx:326-327`), its part menu is `<id>-part`, and `chooseOption(p, '<id>-part', '<part>')` clicks `<id>-part-<part>` (`tests/e2e/reports-helpers.ts:183-186`; `PickMenu` row `${testID}-${item.id}`, item id = the part key, `FormulaEditor.tsx:448-452`). So `outer` and `bound` are the part keys Task 16 adds to `REPORT_PARTS`, and `largest` is today's total part (`lib/builder/formula-parts.ts:30,166`, "Largest value of"), offered in a `{ kind: 'level' }` place as `sum` is in today's sort formula (`reports-groups-v2.spec.ts:265-270`). A lookup's where is child `0` and its value child `1` (`reports-formulas-v2.spec.ts:38-45`). The leaves are `<id>-outer` (Task 16a `OuterLeaf`, a `ColumnPicker`, so `pickIn` finds `report-builder-picker` › `report-builder-pick-field-<key>`, `components/builder/ColumnPicker.tsx:117,211`), `<id>-bound` (Task 16a `BoundLeaf`, a `PickMenu` whose label is the first choice, Task 16 `leafPart` → `boundChoices` "<param label> start"), and `<id>-pick-<pick>` (Task 16a, ids from `LOOKUP_PICKS` values). An asked `between` on Day makes a `dateRange` param labelled with the field's label, "Day" (`lib/builder/filter.ts:267-271`), so the bound reads "Day start". The opening row's label input is `report-builder-group-0-opening-label`: Task 18's `TextInput` gets `testID={\`${testID}-label\`}`, and `core/ui/form/TextInput.tsx:102` puts the test id on the input itself, so `fill` works; the switch is `report-builder-group-0-opening` and its cells `…-opening-cell-<col>-add` / `-formula` (Task 18 `GroupOpening`, `OpeningCell`); the level's open toggle is today's `report-builder-group-0-open` (`GroupSortSection.tsx:150`); the rollup menu is `report-builder-column-N-rollup` with rows `-total` / `-custom` and its editor `-rollup-formula` (Task 18 `ColumnRollup`).

The stub report has R17's nine columns: six for the v3 parts, the `team` link column the group needs (`detail: "blank"`, so it has no cell on detail rows; the group row names the team), and `cost` and `quantity`, which make the narrowed line wider than 944. Its narrowed line is label 80 + `team` and `status` 2 × 80 + six figure columns (day, amount, cost, quantity, sameDay, balance) × 128 = 1008. At a 1024 window a line gets 944 (Q2: 1024 − the 64 rail − the 16 web gutter), so under Q2's recommended answer the report wraps there to its authored tablet arrangement (R13 step 3), which leaves `team` out (a column left out of an arrangement is not shown at that width) and puts the figures on two lines. The General Ledger's narrowed line (864) fits in the same 944, which the test pins too, so a change of the rail or the gutter that moves Q2's answer fails here.

Names: the brief fixes the file name `tests/e2e/reports-ledger-v3.spec.ts` and the `general-ledger:` prefix of every new `test.describe` title (the lane's e2e filter is `-g "general-ledger:"`). Everything else in the stub and the spec has no accounting name (Global Constraints): the report is `salesstub.team_balance`, "Stub team balance".

The stub's `balance` and `rollup` add a `sum` lookup over `amount` to `runningSum(amount)` and to `sum(amount)`; `amount` is money with a per-row currency field. Task 2b's `currency.field` lets amounts of one currency field combine (R24), so `Validate` accepts them. If the hub has no "Stub team balance" after Step 1, that is a Task 2b/5/6 defect, fixed there; this task does not work around it.

The other phone tables that Task 21 re-runs, computed here so that Task 21 only runs (R21; a phone line is 390 − the 16 web gutter = 374; narrowed = label 80 + text column 80 each + figure column 108 each; v2 drew one line when 168 + 108 × (columns − 1) ≤ 390, `report-table-layout.ts:43-52`, `ReportTable.tsx:296-301`):
- `reports-scroll-anchor.spec.ts:448`: 512 narrowed, wraps; `reports-filter.spec.ts:605`: 404 narrowed, wraps. Both assert the wrap and still get it.
- `reports-create.spec.ts:144` (builder preview of a new report: `customer` only): 80 narrowed, natural 168; one line in v2 and v3. It asserts only that "Delta" shows.
- `reports-formula-layout.spec.ts:192` (`customer`, `status` select, `amount` money): 80 + 80 + 108 = 268 narrowed; natural 168 + 108 + 108 = 384 > 374, so the label narrows to 158 and the row is one line; v2 drew one line too (384 ≤ 390). It asserts cells by `cellOf` only.
- `reports-formulas-v2.spec.ts:192` (`customer`, a text lookup formula): 160 narrowed, natural 276; one line in v2 and v3. It asserts a cell's text only (`numberOfLines={1}` keeps the full text in the element).
- `reports-group-pivot.spec.ts:201` (`customer`, `status`, `amount`, pivot `amount.p0`, `amount.p1`; the cell column stays in the result, `assemble.go:16-57`): 80 + 80 + 3 × 108 = 484 > 374, wraps as in v2 (600 > 390). It asserts headings and cells only.
- `reports-share.spec.ts:172` draws no table (the hub's sheets); `:280` opens a report of `customer` and `amount` (188 narrowed, natural 276; one line in v2 and v3) and asserts only its title.

So no test in these files asserts a phone wrap that F10 now fits on one line, and none changes in this task or in Task 21.

**Files:**
- Modify: `tests/scripts/reports-stub/server/register.go.txt` (third shipped report, `teamOpening`, `"strings"` import)
- Create: `tests/e2e/reports-ledger-v3.spec.ts`
- Modify: `tests/e2e/reports-wrap.spec.ts` (header comment; `:385-433` replaced; `:504-546` gets a `cost` column)

**Interfaces:**
- Consumes: Task 2b's `currency.field`, which lets amounts of one currency field combine (R24); Task 4 preset `allDates`; Task 5 rollup; Task 6 opening rows (row id `opening:<group key JSON>`); Task 7 viewer sort; Task 8 grouped pages; Tasks 10–12 client (`RowKind` `opening`, `mergePages`, `s=<col>:<dir>`); Task 13 `tableLines`, pruned `rowLayout`.
- Produces: shipped report `salesstub.team_balance` "Stub team balance"; in the spec, `openTeamBalance(page)` and `balanceOfOneTeam(page, prefix)`. The test ids later tasks must draw: Task 15 `report-sort-<col>` (`role="button"`, label "Sort by <heading>"), `report-clear-sort`; Task 16 the parts `outer` (in a lookup) and `bound` (with a date range param) in a part menu; Task 16a `<id>-outer` (This row's field picker), `<id>-bound` (Range start or end, text "<param label> start") and the lookup pick item `<id>-pick-count`; Task 18 `report-builder-column-N-rollup` with items `-total` and `-custom`, `report-builder-column-N-rollup-formula`, `report-builder-group-N-opening`, `-opening-label`, `-opening-cell-<col>-add`, `-opening-cell-<col>-formula`.

- [ ] **Step 1: Add the stub report**

In `register.go.txt`, change the package comment's "and two shipped reports (one published, one not)" to "and three shipped reports (one published, two not)", and add before `return nil`:

```go
	// An unpublished shipped report over the spec v3 parts (R17): a team's sales
	// before the period open each team (an opening row and a `sum` lookup before
	// the period's start), a running balance continues from it, a `rollup` shows
	// the team's ending balance on its Total row, a `count` lookup reads the
	// row's own day and team (`outer`), and a tablet arrangement takes over when
	// the nine columns do not fit (tests/e2e/reports-ledger-v3.spec.ts).
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.team_balance", Title: "Stub team balance", Revision: 1, Unpublished: true,
		Description: "Each team's sales with a running balance from before the period.",
		Spec: json.RawMessage(strings.ReplaceAll(`{
			"version": 3,
			"source": "salesstub.sales",
			"params": [
				{"id": "period", "type": "dateRange", "label": "Period", "default": {"preset": "thisFiscalYear"}},
				{"id": "teams", "type": "choice", "label": "Teams", "multiple": true,
				 "optionsFrom": {"source": "salesstub.teams", "value": [{"field": "id"}], "label": [{"field": "name"}]}}
			],
			"columns": [
				{"id": "customer", "label": "Customer", "path": [{"field": "customer"}]},
				{"id": "team", "label": "Team", "path": [{"link": "team"}], "detail": "blank"},
				{"id": "day", "label": "Day", "path": [{"field": "day"}]},
				{"id": "status", "label": "Status", "path": [{"field": "status"}]},
				{"id": "amount", "label": "Amount", "path": [{"field": "amount"}], "aggregate": "sum"},
				{"id": "cost", "label": "Cost", "path": [{"field": "cost"}], "aggregate": "sum"},
				{"id": "quantity", "label": "Quantity", "path": [{"field": "quantity"}], "aggregate": "sum"},
				{"id": "sameDay", "label": "Same day", "formula": {"op": "lookup", "source": "salesstub.sales",
					"where": {"op": "and", "args": [
						{"op": "eq", "args": [{"path": [{"link": "team"}]}, {"outer": [{"link": "team"}]}]},
						{"op": "eq", "args": [{"path": [{"field": "day"}]}, {"outer": [{"field": "day"}]}]}]},
					"value": {"path": [{"field": "id"}]}, "pick": "count"}},
				{"id": "balance", "label": "Balance",
					"formula": {"op": "add", "args": [OPENING, {"op": "runningSum", "args": [{"path": [{"field": "amount"}]}]}]},
					"rollup": {"op": "add", "args": [OPENING, {"op": "coalesce", "args": [{"op": "sum", "args": [{"path": [{"field": "amount"}]}]}, {"value": 0}]}]}}
			],
			"filter": {"op": "and", "args": [
				{"op": "between", "args": [{"path": [{"field": "day"}]}, {"param": "period"}]},
				{"op": "in", "args": [{"path": [{"link": "team"}]}, {"param": "teams"}]}
			]},
			"groups": [{"id": "byTeam", "col": "team", "open": true, "subtotals": true,
				"label": {"record": "byTeam", "path": [{"field": "name"}]},
				"opening": {"label": "Before the period", "cells": {"balance": OPENING}}}],
			"sort": [{"col": "day"}, {"col": "customer"}],
			"display": {"layouts": [{"width": "tablet", "tracks": 12, "cells": [
				{"col": "customer", "line": 1, "span": 4},
				{"col": "day", "line": 1, "span": 2},
				{"col": "status", "line": 1, "span": 2},
				{"col": "amount", "line": 1, "span": 2},
				{"col": "balance", "line": 1, "span": 2},
				{"col": "cost", "line": 2, "span": 4},
				{"col": "quantity", "line": 2, "span": 4},
				{"col": "sameDay", "line": 2, "span": 4}
			]}]}
		}`, "OPENING", teamOpening)),
	})
```

Add below `stubHeading`:

```go
// teamOpening is a team's sales before the period's first day, where the
// report's running balance starts. It is 0 with no such sale, and on All dates,
// whose start has no value (spec v3 F3).
const teamOpening = `{"op": "coalesce", "args": [{"op": "lookup", "source": "salesstub.sales",
	"where": {"op": "and", "args": [
		{"op": "eq", "args": [{"path": [{"link": "team"}]}, {"record": "byTeam", "path": [{"field": "id"}]}]},
		{"op": "lt", "args": [{"path": [{"field": "day"}]}, {"param": "period", "bound": "from"}]}]},
	"value": {"path": [{"field": "amount"}]}, "pick": "sum"}, {"value": 0}]}`
```

Add `"strings"` to the imports. (The rollup wraps `sum(amount)` in `coalesce(…, 0)`: `add` carries a blank, `formula.go:558-577`, so a team with no sale in the period would otherwise show no ending balance.)

Re-emit the stub into the workspace:

Run: `cd /Users/nas/tmp/tinycld/books-purchases && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx tinycld/tests/scripts/scaffold-reports-stub.ts`
Expected: the stub's `server/register.go` now holds `salesstub.team_balance`; `cd /Users/nas/tmp/tinycld/books-purchases/salesstub/server && go vet ./...` prints nothing.

- [ ] **Step 2: Write the v3 e2e spec**

Create `tests/e2e/reports-ledger-v3.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addSale,
    addSales,
    addTeam,
    cellOf,
    chooseOption,
    chooseParamOption,
    hubRow,
    listScroller,
    loginAsShippedOwner,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startTeamSales,
    uniqueName,
} from './reports-helpers'

/**
 * Report engine v3 (spec version 3). Most tests read the stub's
 * shipped "Stub team balance" (salesstub.team_balance, tests/scripts/reports-stub/
 * server/register.go.txt): a team's sales before the period open each team in an
 * opening row ("Before the period"), a running balance continues from it, the
 * team's Total row shows a rollup (opening + the period's sales), and "Same day"
 * counts the team's sales on the row's own day. A shipped report belongs to the
 * seed's admin and stub sales are read only by their owner, so those tests sign in
 * as that user, add their own team and sales, and show only their team through the
 * Teams filter. The period is this fiscal year (the stub's sources start it in
 * January); each team has one sale in December of last year. The last test builds
 * the same parts in the builder as an invited user.
 */

const TEAM_BALANCE_TITLE = 'Stub team balance'
const YEAR = new Date().getFullYear()
const PHONE = { width: 390, height: 844 }
const OPENING = 'Before the period'
// Q2: a line at a 1024 window is 944 wide. The narrowed line that Q2 asks about
// (80 + 5 × 80 + 3 × 128) fits in it; this report's does not (80 + 2 × 80 + 6 × 128).
const Q2_NARROWED_LINE = 864
const TEAM_BALANCE_NARROWED_LINE = 1008

async function openTeamBalance(page: Page) {
    await openReportsHub(page)
    await page.getByTestId('reports-hub-tab-mine').click()
    await hubRow(page, TEAM_BALANCE_TITLE).click()
    await expect(page.getByTestId('report-title')).toHaveText(TEAM_BALANCE_TITLE)
}

/**
 * A team with a 10.00 sale last December and four sales this January: Ann (2.00)
 * and Bea (3.00) on one day, Cy (4.00), Dee (1.00). Opening 10.00; balances 12.00,
 * 15.00, 19.00, 20.00; the Total's balance 20.00.
 */
async function seedTeam(page: Page, team: string) {
    await openReportStub(page)
    await addTeam(page, team)
    const sales = [
        ['Old', 1000, `${YEAR - 1}-12-15`],
        ['Ann', 200, `${YEAR}-01-05`],
        ['Bea', 300, `${YEAR}-01-05`],
        ['Cy', 400, `${YEAR}-01-07`],
        ['Dee', 100, `${YEAR}-01-08`],
    ] as const
    for (const [customer, amount, day] of sales) {
        await addSale(page, { team, customer, amount, cost: 0, day })
    }
}

async function showTeams(page: Page, search: string, teams: string[]) {
    await chooseParamOption(page, 'teams', { from: 'add-filter', search, labels: teams })
}

/** Signs in, seeds one team and opens the report on it alone. */
async function balanceOfOneTeam(page: Page, prefix: string) {
    await loginAsShippedOwner(page)
    const team = uniqueName(prefix)
    await seedTeam(page, team)
    await openTeamBalance(page)
    await showTeams(page, team, [team])
    await expect(rowOf(page, 'Dee')).toBeVisible()
    return team
}

/** The rows' ids in screen order, read in one snapshot (a sticky copy counts once). */
function rowIds(page: Page) {
    return page.getByTestId('report-table').evaluate(table => {
        const ids = [...table.querySelectorAll('[data-testid^="report-row-"]')].map(
            el => el.getAttribute('data-testid')?.replace('report-row-', '') ?? ''
        )
        return [...new Set(ids)]
    })
}

function detailLabels(page: Page) {
    return page.getByTestId('report-table').locator('[data-testid^="report-row-d:"]')
}

const sortParam = (page: Page) => new URL(page.url()).searchParams.get('s')

async function pickIn(page: Page, trigger: string, key: string) {
    await page.getByTestId(trigger).click()
    await page.getByTestId('report-builder-picker').getByTestId(key).click()
}

test.describe('general-ledger: report engine v3', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an open group starts with its opening row, runs a balance from it and ends with its rollup', async ({
        page,
    }) => {
        const team = await balanceOfOneTeam(page, 'BalOpen')
        const ids = await rowIds(page)
        const group = ids.findIndex(id => id.startsWith('g:'))
        expect(group).toBeGreaterThanOrEqual(0)
        expect(ids[group + 1]).toMatch(/^opening:/)
        expect(ids.slice(group + 2, group + 6).every(id => id.startsWith('d:'))).toBe(true)
        expect(ids[group + 6]).toMatch(/^s:/)
        await expect(await cellOf(page, 'balance', OPENING)).toHaveText(/^\$?10\.00$/)
        await expect(await cellOf(page, 'balance', 'Ann')).toHaveText('12.00')
        await expect(await cellOf(page, 'balance', 'Bea')).toHaveText('15.00')
        await expect(await cellOf(page, 'balance', 'Cy')).toHaveText('19.00')
        await expect(await cellOf(page, 'balance', 'Dee')).toHaveText('20.00')
        // Count and This row's field: Ann and Bea share a day.
        await expect(await cellOf(page, 'sameDay', 'Ann')).toHaveText('2')
        await expect(await cellOf(page, 'sameDay', 'Cy')).toHaveText('1')
        // The rollup: opening + the period's sales; the amount keeps its sum.
        await expect(await cellOf(page, 'balance', `Total ${team}`)).toHaveText(/^\$?20\.00$/)
        await expect(await cellOf(page, 'amount', `Total ${team}`)).toHaveText(/^\$?10\.00$/)
    })

    test('a grouped report scrolled past its first page keeps its running balance', async ({
        page,
    }) => {
        await loginAsShippedOwner(page)
        const stem = uniqueName('BalPage')
        const teams = [`${stem} A`, `${stem} B`]
        await openReportStub(page)
        for (const [i, team] of teams.entries()) {
            await addTeam(page, team)
            await addSales(page, {
                team,
                customerPrefix: `${stem}${'AB'[i]}`,
                count: 6,
                firstDay: `${YEAR}-02-01`,
                firstAmount: 100 * (i + 1),
                cost: 0,
                quantity: 1,
            })
        }
        // Pages of 5 detail rows: 12 sales page across the first team's end.
        const cursors: string[] = []
        await page.route('**/api/reports/run', async route => {
            const body = route.request().postDataJSON()
            if (body.options?.cursor) cursors.push(body.options.cursor)
            await route.continue({
                postData: JSON.stringify({ ...body, options: { ...body.options, limit: 5 } }),
            })
        })
        await openTeamBalance(page)
        await showTeams(page, stem, teams)
        const firstDetail = detailLabels(page).first()
        await expect(firstDetail).toBeVisible()
        const scroller = await listScroller(firstDetail)
        await expect
            .poll(async () => {
                await scroller.evaluate(el => {
                    el.scrollTop = el.scrollHeight
                })
                return cursors.length
            })
            .toBeGreaterThan(0)
        // Each team's sixth sale: the running sum of six amounts, whichever page it is on.
        await expect(await cellOf(page, 'balance', `${stem}A 05`)).toHaveText('6.15')
        await expect(await cellOf(page, 'balance', `${stem}B 05`)).toHaveText('12.15')
        const ids = await rowIds(page)
        const openings = ids.filter(id => id.startsWith('opening:'))
        const subtotals = ids.filter(id => id.startsWith('s:'))
        expect(openings).toHaveLength(2)
        expect(subtotals).toHaveLength(2)
        expect(ids.filter(id => id.startsWith('total')).length).toBeLessThanOrEqual(1)
    })

    test('a heading sorts the rows inside their groups, and a copied link keeps it', async ({
        page,
        browser,
    }) => {
        await balanceOfOneTeam(page, 'BalSort')
        await expect(detailLabels(page)).toHaveText(['Ann', 'Bea', 'Cy', 'Dee'])
        const sort = page.getByTestId('report-sort-amount')
        await expect(sort).toHaveAttribute('role', 'button')
        await expect(sort).toHaveAccessibleName('Sort by Amount')
        // A running value is not a sort key.
        await expect(page.getByTestId('report-heading-balance')).toBeVisible()
        await expect(page.getByTestId('report-sort-balance')).toHaveCount(0)

        await sort.click()
        await expect.poll(() => sortParam(page)).toBe('amount:asc')
        await expect(detailLabels(page)).toHaveText(['Dee', 'Ann', 'Bea', 'Cy'])
        await sort.click()
        await expect.poll(() => sortParam(page)).toBe('amount:desc')
        await expect(detailLabels(page)).toHaveText(['Cy', 'Bea', 'Ann', 'Dee'])
        // The running balance keeps the report's own order.
        await expect(await cellOf(page, 'balance', 'Cy')).toHaveText('19.00')

        await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
        await pressToolbar(page, 'report-copy-link', 'Copy link')
        await expect(page.getByText('Link copied', { exact: true })).toBeVisible()
        const link = await page.evaluate(() => navigator.clipboard.readText())
        expect(new URL(link).searchParams.get('s')).toBe('amount:desc')
        const other = await browser.newContext()
        try {
            const q = await other.newPage()
            await loginAsShippedOwner(q)
            // Opening a pasted link is a full load by nature.
            await q.goto(link)
            await expect(detailLabels(q)).toHaveText(['Cy', 'Bea', 'Ann', 'Dee'])
        } finally {
            await other.close()
        }

        await sort.click()
        await expect.poll(() => sortParam(page)).toBeNull()
        await expect(detailLabels(page)).toHaveText(['Ann', 'Bea', 'Cy', 'Dee'])
    })

    test('a link that sorts by a column the report lacks offers Clear sort', async ({
        page,
        browser,
    }) => {
        await balanceOfOneTeam(page, 'BalBadSort')
        const link = new URL(page.url())
        link.searchParams.set('s', 'nosuch:asc')
        const other = await browser.newContext()
        try {
            const q = await other.newPage()
            await loginAsShippedOwner(q)
            // Opening a pasted link is a full load by nature.
            await q.goto(link.toString())
            await expect(q.getByTestId('report-invalid')).toContainText('nosuch')
            await q.getByTestId('report-clear-sort').click()
            await expect.poll(() => sortParam(q)).toBeNull()
            await expect(detailLabels(q)).toHaveText(['Ann', 'Bea', 'Cy', 'Dee'])
        } finally {
            await other.close()
        }
    })

    test('All dates shows every sale, says so in the header, and opens at zero', async ({
        page,
    }) => {
        await balanceOfOneTeam(page, 'BalAll')
        await page.getByTestId('report-param-period').click()
        await page.getByTestId('report-period-preset-allDates').click()
        await expect(page.getByTestId('report-param-period')).toHaveText('Period: All dates')
        await expect(page.getByTestId('report-document-period')).toHaveText('All dates')
        await expect(rowOf(page, 'Old')).toBeVisible()
        await expect(await cellOf(page, 'balance', OPENING)).toHaveText(/^\$?0\.00$/)
        await expect(await cellOf(page, 'balance', 'Old')).toHaveText('10.00')
        await expect(await cellOf(page, 'balance', 'Dee')).toHaveText('20.00')
    })

    // Q2, recommended answer (narrow first, then wrap): at a 1024 window the line is
    // 944 wide, so Q2's narrowed line (864) fits and this report's does
    // not. This report wraps to its tablet arrangement: eight headings (Team is left
    // out) on two lines, all inside the table, and nothing scrolls sideways.
    test('on a desktop too narrow for its narrowed line a report wraps to its tablet arrangement inside the table', async ({
        page,
    }) => {
        await balanceOfOneTeam(page, 'BalFit')
        await page.setViewportSize({ width: 1024, height: 720 })
        const measure = () =>
            page.getByTestId('report-table').evaluate(table => {
                const right = table.getBoundingClientRect().right
                const heading = table.querySelector('[data-testid="report-heading-row"]')
                const cells = [...table.querySelectorAll('[data-testid^="report-heading-"]')].filter(
                    el => el.getAttribute('data-testid') !== 'report-heading-row'
                )
                const tops = new Set(cells.map(el => Math.round(el.getBoundingClientRect().top)))
                return {
                    // The heading row keeps the list's scroll bar gutter, so its client
                    // width is the width a line's tracks get.
                    line: heading?.clientWidth ?? 0,
                    headings: cells.length,
                    lines: tops.size,
                    outside: cells.filter(el => el.getBoundingClientRect().right > right + 1).length,
                }
            })
        // At the first window (1280) the line is 1200; wait for the 1024 one.
        await expect.poll(async () => (await measure()).line).toBeLessThan(TEAM_BALANCE_NARROWED_LINE)
        await expect.poll(async () => (await measure()).headings).toBe(8)
        const fit = await measure()
        expect(fit.line).toBeGreaterThanOrEqual(Q2_NARROWED_LINE)
        expect(fit).toMatchObject({ lines: 2, outside: 0 })
        await expect(page.getByTestId('report-heading-team')).toHaveCount(0)
        const scroller = await listScroller(rowOf(page, 'Dee'))
        expect(await scroller.evaluate(el => el.scrollWidth === el.clientWidth)).toBe(true)
        expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
            1024
        )
    })

    test('on a phone an opening row is its label and then its figures, and a heading still sorts', async ({
        page,
    }) => {
        await balanceOfOneTeam(page, 'BalPhone')
        await page.setViewportSize(PHONE)
        const opening = rowOf(page, OPENING)
        await expect(opening).toBeVisible()
        const id = (await opening.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
        const balance = page.getByTestId(`report-cell-balance-${id}`)
        await expect(balance).toHaveText(/^\$?10\.00$/)
        // Blank cells are not drawn: no empty line between the label and the figures.
        for (const col of ['team', 'day', 'status', 'amount', 'cost', 'quantity', 'sameDay']) {
            await expect(page.getByTestId(`report-cell-${col}-${id}`)).toHaveCount(0)
        }
        const gap = async () => {
            const label = await opening.boundingBox()
            const figures = await balance.boundingBox()
            return label && figures ? figures.y - (label.y + label.height) : null
        }
        await expect.poll(gap).toBeGreaterThanOrEqual(-1)
        expect(await gap()).toBeLessThanOrEqual(8)
        await expect
            .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
            .toBeLessThanOrEqual(PHONE.width)

        await page.getByTestId('report-sort-amount').click()
        await expect.poll(() => sortParam(page)).toBe('amount:asc')
        await expect(detailLabels(page)).toHaveText(['Dee', 'Ann', 'Bea', 'Cy'])
    })

    // The builder's v3 controls, in the order the tasks add them: This row's field,
    // Count and Range start or end (Tasks 16, 16a), a formula on group and total rows
    // and a row that starts each group (Task 18). The asked period (this fiscal year)
    // keeps every sale, all dated this January.
    test('the builder counts a lookup on this row, shows a formula on group and total rows, and starts each group with a row', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptv3build')
        try {
            const team = uniqueName('Built')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Ada', amount: 1000, cost: 0, day: `${YEAR}-01-05` })
            await addSale(p, { team, customer: 'Abe', amount: 500, cost: 0, day: `${YEAR}-01-05` })
            await addSale(p, { team, customer: 'Cy', amount: 200, cost: 0, day: `${YEAR}-01-09` })
            await openReportsHub(p)
            // Columns customer (0), team (1) and the amount summed (2).
            await startTeamSales(p)

            // How many sales share this row's day: Count, with This row's field.
            await p.getByTestId('report-builder-add-formula').click()
            const f = 'report-builder-column-3-formula'
            await chooseOption(p, `${f}-root-part`, 'lookup')
            await chooseOption(p, `${f}-root-source`, SALES_SOURCE)
            await chooseOption(p, `${f}-0-part`, 'eq')
            await chooseOption(p, `${f}-0-0-part`, 'path')
            await pickIn(p, `${f}-0-0-path`, 'report-builder-pick-field-day')
            await chooseOption(p, `${f}-0-1-part`, 'outer')
            await pickIn(p, `${f}-0-1-outer`, 'report-builder-pick-field-day')
            await chooseOption(p, `${f}-1-part`, 'path')
            await pickIn(p, `${f}-1-path`, 'report-builder-pick-field-customer')
            await chooseOption(p, `${f}-root-pick`, 'count')
            await expect(p.getByTestId(`${f}-summary`)).toHaveText(
                "= count Customer in Stub sales where Day = this row's Day"
            )

            // Range start or end: an asked period offers its first day (column 4).
            await p.getByTestId('report-builder-section-filter').click()
            await pickIn(p, 'report-builder-filter-add-condition', 'report-builder-pick-field-day')
            await chooseOption(p, 'report-builder-filter-0-op', 'between')
            await p.getByTestId('report-builder-filter-0-ask').click()
            await chooseOption(p, 'report-builder-filter-0-default', 'thisFiscalYear')
            await p.getByTestId('report-builder-add-formula').click()
            const b = 'report-builder-column-4-formula'
            await chooseOption(p, `${b}-root-part`, 'bound')
            await expect(p.getByTestId(`${b}-root-bound`)).toContainText('Day start')

            // On group and total rows the amount shows its largest sale, not the sum.
            const r = 'report-builder-column-2-rollup'
            await chooseOption(p, r, 'custom')
            await chooseOption(p, `${r}-formula-root-part`, 'largest')
            await chooseOption(p, `${r}-formula-0-part`, 'path')
            await pickIn(p, `${r}-formula-0-path`, 'report-builder-pick-field-amount')

            // Open teams, each starting with a row that adds up the team's amounts.
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'team')
            await p.getByTestId('report-builder-group-0-open').click()
            const o = 'report-builder-group-0-opening'
            await p.getByTestId(o).click()
            await p.getByTestId(`${o}-label`).fill('Brought forward')
            await p.getByTestId(`${o}-cell-amount-add`).click()
            const cell = `${o}-cell-amount-formula`
            await chooseOption(p, `${cell}-root-part`, 'sum')
            await chooseOption(p, `${cell}-0-part`, 'path')
            await pickIn(p, `${cell}-0-path`, 'report-builder-pick-field-amount')
            await saveNewReport(p, uniqueName('Built v3'))

            const ids = await rowIds(p)
            const group = ids.findIndex(id => id.startsWith('g:'))
            expect(ids[group + 1]).toMatch(/^opening:/)
            await expect(await cellOf(p, 'amount', 'Brought forward')).toHaveText(/^\$?17\.00$/)
            await expect(await cellOf(p, 'amount', 'Ada')).toHaveText('10.00')
            await expect(await cellOf(p, 'amount', `Total ${team}`)).toHaveText(/^\$?10\.00$/)
            await expect(await cellOf(p, 'formula', 'Ada')).toHaveText('2')
            await expect(await cellOf(p, 'formula', 'Abe')).toHaveText('2')
            await expect(await cellOf(p, 'formula', 'Cy')).toHaveText('1')
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 3: Change `reports-wrap.spec.ts` to F10**

Replace the whole test `'on a desktop a table wider than its view scrolls sideways with its labels held at the left'` (`:385-433`) with:

```ts
    test('on a desktop a table wider than its view narrows its text columns and never scrolls sideways', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptside')
        try {
            await seed(p, 1)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            for (const keys of [
                ['amount'],
                ['cost'],
                ['quantity'],
                ['currency'],
                ['day'],
                ['booked'],
                ['status'],
                ['team', 'name'],
            ]) {
                await addColumn(p, keys)
            }
            await saveNewReport(p, uniqueName('Narrowed'))
            await p.setViewportSize({ width: 1100, height: 720 })
            const label = rowOf(p, 'Wrapco 00')
            await expect(label).toBeVisible()
            const table = p.getByTestId('report-table')
            // Every heading ends inside the table, on one line.
            const headings = () =>
                table.evaluate(element => {
                    const box = element.getBoundingClientRect()
                    const cells = [
                        ...element.querySelectorAll('[data-testid^="report-heading-"]'),
                    ].filter(el => el.getAttribute('data-testid') !== 'report-heading-row')
                    const tops = new Set(cells.map(el => Math.round(el.getBoundingClientRect().top)))
                    return {
                        count: cells.length,
                        outside: cells.filter(el => el.getBoundingClientRect().right > box.right + 1)
                            .length,
                        lines: tops.size,
                    }
                })
            await expect.poll(async () => (await headings()).outside).toBe(0)
            expect(await headings()).toMatchObject({ count: 9, lines: 1 })
            // Nothing moves sideways.
            const x = async (testId: string) => (await p.getByTestId(testId).boundingBox())?.x ?? 0
            const before = await x('report-heading-amount')
            await table.hover()
            await p.mouse.wheel(300, 0)
            expect(
                await table.evaluate(element =>
                    [element, ...element.querySelectorAll('*')].every(el => el.scrollLeft === 0)
                )
            ).toBe(true)
            expect(Math.abs((await x('report-heading-amount')) - before)).toBeLessThanOrEqual(1)
            // The label is cut short with "…"; the amount keeps its width.
            const text = label.getByText('Wrapco 00', { exact: true })
            expect(await text.evaluate(el => el.scrollWidth > el.clientWidth)).toBe(true)
            await expect
                .poll(async () =>
                    Math.round((await p.getByTestId('report-heading-amount').boundingBox())?.width ?? 0)
                )
                .toBe(128)
        } finally {
            await close()
        }
    })
```

In `'on a phone a column left blank on opened rows keeps its empty slot'` (`:504-546`), add a `cost` column after `await groupWithTeamRegion(p)`, so the row is still too wide for one narrowed line at 390 (label 80 + `team` and `formula` 2 × 80 + `amount` and `cost` 2 × 108 = 456, more than the 374 line):

```ts
            await groupWithTeamRegion(p)
            // A fifth column: with four, R21 fits the row on one narrowed line at 390
            // (80 + 108 + 2 × 93 = 374), and this test is about the wrapped rows.
            await addColumn(p, ['cost'])
            await expect(p.getByTestId('report-builder-column-3-detail')).toBeVisible()
```

(the `await expect(p.getByTestId('report-builder-column-3-detail')).toBeVisible()` line is the existing one; `formula` stays column 3 and `cost` is column 4). Why a column and not the one-line assertion: the test pins E2, the empty slot of a blank column in wrapped rows, which every wider phone report still draws; R21's one narrowed line on a phone is pinned by Task 13's `fitOneLine(…, 390, 'mobile')` cases, and the wrapped test then holds under either answer to Q3.

Update the file's header comment: replace "Desktop keeps one line per row and the heading row stays at the top." with "Desktop keeps one line per row, narrowing text columns before it wraps, never scrolls sideways, and the heading row stays at the top."

- [ ] **Step 4: Run the two files and see each test red for its reason**

Check the ports first: `lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'` (kill a leftover server only if it was started from `/Users/nas/tmp/tinycld/books-purchases`).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-ledger-v3.spec.ts`, then the same with `tests/e2e/reports-wrap.spec.ts`.

Expected, with Tasks 1–13 done (a test with "PASS" gets its mutation proof in Step 5):

| Test | Now | Red reason, or why it holds | Green after |
|---|---|---|---|
| v3: opening row, balance, rollup | PASS | engine Tasks 5–6, client Task 10 (`opening` kind) | — (Step 5) |
| v3: paged running balance | PASS | Task 8 pages, Task 12 `mergePages` | — (Step 5) |
| v3: heading sort and copied link | FAIL | `report-sort-amount`: element not found (no heading control) | Task 15 |
| v3: Clear sort | FAIL | `report-invalid` not found: the view does not send the URL's `s=` to the run, so nothing refuses it | Task 15 |
| v3: All dates | PASS | Tasks 4 and 11 | — (Step 5) |
| v3: 1024 tablet arrangement | PASS | Task 13 already wraps the 1008 line to the tablet arrangement, and a blocks layout has no sideways scroller | — (Step 5) |
| v3: phone opening row and sort | FAIL | at `report-sort-amount` (no heading control); the opening-row part before it holds (Task 13's pruned `rowLayout`) | Task 15 |
| v3: builder | FAIL | `report-builder-column-3-formula-0-1-part-outer` not found (no This row's field part) | Task 16, then 16a, then 18 |
| wrap: desktop narrows, never sideways | FAIL | `outside` stays above 0: the `Sideways` scroller still draws the label at 220 and every column at 128 (1244 in a 1036 table) | Task 14 |
| wrap: phone blank slot (with `cost`) | PASS | five columns still wrap at 390 | — (Step 5) |
| wrap: every other test | PASS | unchanged behaviour | — |

If the hub has no "Stub team balance", search `tmp/e2e-serve-7210.log` for "shipped reports not reconciled": the message names the spec part the engine refused; fix the engine task that refuses a valid part (for the money sums, Task 2b), or the stub spec if the part is wrong — never the test.

- [ ] **Step 5: Mutation proofs for the tests green now**

Each mutation is made, the one test is run with `-g "<its title>"` (same prefix as Step 4), the failure is seen, and the mutation is reverted (`git diff` shows only this task's three files after):

1. Opening row: delete `"opening": {…}` from the stub's group, re-emit the stub (Step 1's command). Expected: "expected 'd:…' to match /^opening:/".
2. Paged balance: in `mergePages` (`core/reports/lib/rows.ts`), keep every page's `total` row. Expected: the last assertion fails, a Total count above 1.
3. All dates: remove `'allDates'` from `RANGE_PRESETS` (`core/reports/lib/params.ts`). Expected: `report-period-preset-allDates` not found.
4. 1024: in `tableLines` (`core/reports/lib/table-lines.ts`), return `autoLines(columns, width)` in place of the nearest-narrower arrangement. Expected: `headings` polls 9, not 8 (Team is drawn).
5. Phone opening row: in `rowLayout`, return `layout` for every row kind (drop the `keeps` pruning). Expected: `report-cell-team-<id>` has count 1, not 0.
6. Phone blank slot: delete the added `addColumn(p, ['cost'])`. Expected: the wrapped poll gives `false` (four columns fit one narrowed line at 390).

- [ ] **Step 6: Check and commit (the driver runs the commit)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write tests/e2e/reports-ledger-v3.spec.ts tests/e2e/reports-wrap.spec.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add tests/scripts/reports-stub/server/register.go.txt tests/e2e/reports-ledger-v3.spec.ts tests/e2e/reports-wrap.spec.ts
git commit -m "test(core): e2e of report engine v3 on a stub team balance report; a wide report narrows instead of scrolling sideways"
```

Expected: the check is green (it does not run e2e). The commit holds red e2e on purpose; Tasks 14, 15, 16a and 18 turn them green, and the PR's CI runs only after Task 22.

---

### Task 14: F10/F5 view — the table without sideways scroll; the opening row

View facts used: `ReportTable` wraps the heading row and the list in `Sideways` (a horizontal `ScrollView` whose offset drives `FrozenLabel`) when a desktop line is one line (`ReportTable.tsx:169-214,327-340`); `OneLineHeading` and `OneLineRow` draw the label in `FrozenLabel` with `oneLine.labelStyle` and every other column at `oneLine.amountWidth` (`ReportTable.tsx:91-111`, `ReportRow.tsx:259-330`); `StickyGroup` draws its label lines in `FrozenLabel` (`ReportRow.tsx:393-424`); `BlocksRow` already draws `rowLayout(...)` (`ReportRow.tsx:332-367`), which Task 13 prunes; the heading row and the list keep a scroll bar's gutter on web (`web:[scrollbar-gutter:stable]`, `ReportTable.tsx:144,264`), so the tracks have the table's width less the gutter; `FrozenLabel` is imported only by `ReportTable.tsx:23`; `reportTableLayout`, `ReportTableLayout` and `scrollsSideways` are used only by `ReportTable.tsx` and `ReportRow.tsx`. Head: "Opening row" (label in `text-muted-foreground`, no rule, test id `report-row-opening:<group key JSON>` from `labelId`), F10 per width, Q2.

**Files:**
- Modify: `core/reports/components/ReportTable.tsx` (no `Sideways`, widths from `layout.widths`, the gutter)
- Modify: `core/reports/components/ReportRow.tsx` (`oneLineLabelStyle`, `OneLineRow`, `StickyGroup`, `TableRowProps`, label tone; `FrozenLabel` deleted)
- Modify: `core/reports/components/report-table-layout.ts` (`ReportTableLayout`, `reportTableLayout` deleted)
- Modify: `core/reports/components/__tests__/report-table-layout.test.ts` (its `describe('reportTableLayout', …)` deleted with the function)
- Test (written in Task 13a, run here): `tests/e2e/reports-wrap.spec.ts`, `tests/e2e/reports-ledger-v3.spec.ts`

**Interfaces:**
- Consumes: `LineLayout` with `widths` (Task 13); pruned `rowLayout` (Task 13); `ROW_RULE`/`ROW_WEIGHT` `opening` (Task 10).
- Produces: `ReportTable` with no `Sideways`; `oneLineLabelStyle(layout: LineLayout, labelId: string | undefined): { flex: number; minWidth?: number }` (exported from `ReportRow.tsx`); `TableRowProps` without `oneLine` and `scrollX`; `OneLineRow`/`OneLineHeading` draw each column at `layout.widths[col]` with `numberOfLines={1}`; `FrozenLabel` deleted; an opening row's label in `text-muted-foreground`; `HeadingRowProps = { columns; layout; format }` (Task 15 adds `sorting`).

- [ ] **Step 1: The tests (written in Task 13a)**

No vitest: this task changes components only (CLAUDE.md: front-end behaviour is covered by e2e). Task 13a wrote the e2e and showed the red one red at this point; this task turns it green and keeps the others green:

| Change | E2E (Task 13a) | At the start of this task |
|---|---|---|
| No sideways scroll on a desktop; every heading ends inside the table; text cells end in "…"; figure columns keep 128 | `reports-wrap.spec.ts` "on a desktop a table wider than its view narrows its text columns and never scrolls sideways" (1100×720) | FAIL: `outside` above 0 (the `Sideways` scroller) |
| A desktop too narrow for the narrowed line wraps to the nearer arrangement, inside the table, at the Q2 width | `reports-ledger-v3.spec.ts` "on a desktop too narrow for its narrowed line …" (1024×720) | PASS (mutation-proved in Task 13a); must stay green with the gutter subtracted |
| One line per row where it fits (no regression) | `reports-wrap.spec.ts:187` | PASS; must stay green |
| The opening row is drawn first under an open group | `reports-ledger-v3.spec.ts` "an open group starts with its opening row …" | PASS; must stay green without `FrozenLabel` |
| A phone opening row is two lines, no blank line | `reports-ledger-v3.spec.ts` phone test, up to its sort press | the opening part PASS; the test fails later at `report-sort-amount` until Task 15 |
| Sticky group rows without `FrozenLabel` | `reports-wrap.spec.ts` (sticky tests), `reports-groups-v2.spec.ts` (Task 21) | PASS; must stay green |

The opening row label's quieter tone (`text-muted-foreground`) is a colour: no e2e reads it; the reviewer checks it against the head's "Opening row".

- [ ] **Step 2: `report-table-layout.ts` and its test**

Delete `ReportTableLayout`, `layoutFor` and `reportTableLayout` (and their comments) from `components/report-table-layout.ts`; it keeps `ROW_HEIGHT`, `AMOUNT_WIDTH`, `LABEL_MIN_WIDTH`, `PHONE_LABEL_WIDTH`, `PHONE_AMOUNT_WIDTH`, `INDENT`, `TEXT_FLOOR` and `columnAlign`. Drop the `Breakpoint` import if nothing else in the file uses it. In `components/__tests__/report-table-layout.test.ts`, delete `describe('reportTableLayout', …)` and remove `reportTableLayout` from its import.

- [ ] **Step 3: `ReportRow.tsx`**

Imports: drop `Animated` from the `react-native` import (`Pressable, Text, View` stay), and change the `report-table-layout` import to `import { columnAlign, INDENT, ROW_HEIGHT } from './report-table-layout'`.

Replace `LabelText`:

```tsx
// An opening row's label is a note before the group's own rows, so it is quieter.
function labelTone(kind: RowKind): string {
    return kind === 'opening' ? 'text-muted-foreground' : 'text-foreground'
}

function LabelText({ row, text }: { row: ResultRow; text: string }) {
    return (
        <Text
            numberOfLines={1}
            className={`min-w-0 flex-1 text-sm ${labelTone(row.kind)} ${ROW_WEIGHT[row.kind]}`}
        >
            {text}
        </Text>
    )
}
```

Replace the comment above `labelStyle` with `// Child rows indent the label only, so the figures stay aligned. It grows to fill its slot, as a block cell does.`

Delete `FrozenLabel` (with its comment) and replace `TableRowProps` and `OneLineRow` with:

```tsx
/**
 * The label column's box on one line (F10): it takes the width the other columns
 * leave, and never less than the width the line gave it.
 */
export function oneLineLabelStyle(layout: LineLayout, labelId: string | undefined) {
    const minWidth = layout.kind === 'one-line' && labelId ? layout.widths[labelId] : undefined
    return { flex: 1, minWidth }
}

export interface TableRowProps {
    item: VisibleRow
    columns: ResultColumn[]
    layout: LineLayout
    format: NumberFormat
    onToggleGroup(row: ResultRow): void
    isCopy?: boolean
}

// Each column at the width the line gave it; a cell too narrow for its text ends
// in "…" (numberOfLines 1), so nothing runs past the table's edge.
function OneLineRow({ item, columns, layout, format, onToggleGroup, isCopy }: TableRowProps) {
    if (layout.kind !== 'one-line') return null
    const [label, ...amounts] = columns
    const cells = amounts.map((column, i) => (
        <RowCell
            key={column.id}
            item={item}
            column={column}
            format={format}
            justify={amountJustify(column, i + 1)}
            onToggleGroup={onToggleGroup}
            width={layout.widths[column.id]}
            isCopy={isCopy}
        />
    ))
    return (
        <View className={`flex-row bg-background ${ROW_RULE[item.row.kind]}`}>
            <View style={oneLineLabelStyle(layout, label?.id)}>
                <RowLabel
                    item={item}
                    format={format}
                    onToggleGroup={onToggleGroup}
                    isCopy={isCopy}
                />
            </View>
            {cells}
        </View>
    )
}
```

In `StickyGroup`, replace

```tsx
    const frozenStyle = props.layout.kind === 'one-line' ? props.oneLine.labelStyle : { flex: 1 }
    const labels = (own ? chain.slice(0, -1) : chain).map(item => (
        <View key={item.row.id} className="flex-row bg-background">
            <FrozenLabel scrollX={props.scrollX} style={frozenStyle}>
                <RowLabel
                    item={item}
                    format={props.format}
                    onToggleGroup={props.onToggleGroup}
                    isCopy
                />
            </FrozenLabel>
        </View>
    ))
```

with

```tsx
    const labelBox = oneLineLabelStyle(props.layout, props.columns[0]?.id)
    const labels = (own ? chain.slice(0, -1) : chain).map(item => (
        <View key={item.row.id} className="flex-row bg-background">
            <View style={labelBox}>
                <RowLabel
                    item={item}
                    format={props.format}
                    onToggleGroup={props.onToggleGroup}
                    isCopy
                />
            </View>
        </View>
    ))
```

Remove the `ReactNode` import only if nothing else uses it (`RenderCell` does: keep it).

- [ ] **Step 4: `ReportTable.tsx`**

Replace the whole file with:

```tsx
import { FlashList, type ListRenderItemInfo } from '@shopify/flash-list'
import { type Breakpoint, useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useMemo, useState } from 'react'
import { ActivityIndicator, Platform, Text, View } from 'react-native'
import { headingText, type NumberFormat } from '../lib/format'
import type { VisibleRow } from '../lib/rows'
import type { DisplayLayout } from '../lib/spec'
import { stickyIndices } from '../lib/sticky'
import { type Justify, type LineLayout, tableLines } from '../lib/table-lines'
import type { ResultColumn, ResultRow } from '../lib/types'
import {
    amountJustify,
    BlockView,
    cellBoxStyle,
    oneLineLabelStyle,
    type RenderCell,
    StickyGroup,
    TableRow,
    type TableRowProps,
} from './ReportRow'

const TEXT_ALIGN: Record<Justify, string> = {
    start: 'text-left',
    center: 'text-center',
    end: 'text-right',
}

// The heading row and the list keep a scroll bar's gutter on web (scrollbar-gutter:
// stable), so a line's tracks have the table's width less the gutter.
const SCROLL_GUTTER = Platform.OS === 'web' ? 16 : 0

// The value kept from an earlier render while it is equal in content, so a new
// array or object with the same content does not redraw every row.
function useStable<T>(value: T): T {
    const [kept, setKept] = useState(value)
    if (kept === value || JSON.stringify(kept) === JSON.stringify(value)) return kept
    setKept(value)
    return value
}

function HeadingCell({
    column,
    justify,
    width,
    format,
}: {
    column: ResultColumn
    justify: Justify
    width?: number
    format: NumberFormat
}) {
    const text = headingText(column, format)
    return (
        <View
            testID={`report-heading-${column.id}`}
            className="justify-center px-3"
            style={cellBoxStyle(width)}
        >
            <Text
                numberOfLines={1}
                className={`text-xs font-semibold uppercase text-muted-foreground ${TEXT_ALIGN[justify]}`}
            >
                {text}
            </Text>
        </View>
    )
}

// The label's heading sits over the label's text, which starts after the row's indent.
function LabelHeading({ column, format }: { column: ResultColumn; format: NumberFormat }) {
    return (
        <View className="grow pl-1">
            <HeadingCell column={column} justify="start" format={format} />
        </View>
    )
}

interface HeadingRowProps {
    columns: ResultColumn[]
    layout: LineLayout
    format: NumberFormat
}

function OneLineHeading({ columns, layout, format }: HeadingRowProps) {
    if (layout.kind !== 'one-line') return null
    const [label, ...amounts] = columns
    if (!label) return null
    const cells = amounts.map((column, i) => (
        <HeadingCell
            key={column.id}
            column={column}
            justify={amountJustify(column, i + 1)}
            width={layout.widths[column.id]}
            format={format}
        />
    ))
    return (
        <>
            <View style={oneLineLabelStyle(layout, label.id)}>
                <LabelHeading column={label} format={format} />
            </View>
            {cells}
        </>
    )
}

// The heading is drawn by the same blocks as the rows, so each heading sits over
// its values and wraps the same way.
function BlocksHeading({ columns, layout, format }: HeadingRowProps) {
    if (layout.kind !== 'blocks') return null
    const byId = new Map(columns.map(column => [column.id, column]))
    const labelId = columns[0]?.id
    const renderCell: RenderCell = (col, justify) => {
        const column = byId.get(col)
        if (!column) return null
        if (col === labelId) return <LabelHeading column={column} format={format} />
        return <HeadingCell column={column} justify={justify} format={format} />
    }
    return (
        <View className="flex-1">
            <BlockView block={layout.root} renderCell={renderCell} />
        </View>
    )
}

function HeadingCells(props: HeadingRowProps) {
    if (props.layout.kind === 'one-line') return <OneLineHeading {...props} />
    return <BlocksHeading {...props} />
}

// Above the list, outside its vertical scroll, so it always stays at the top. Both
// keep a scroll bar's gutter, so the heading's tracks match the rows'.
function HeadingRow(props: HeadingRowProps) {
    return (
        <View
            testID="report-heading-row"
            className="flex-row border-b border-border bg-background web:[scrollbar-gutter:stable] web:overflow-y-hidden"
        >
            <HeadingCells {...props} />
        </View>
    )
}

function LoadingMore({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-loading-more" className="items-center py-3">
            <ActivityIndicator />
        </View>
    )
}

function LayoutNotice({ text }: { text: string | undefined }) {
    if (!text) return null
    return (
        <Text testID="report-layout-notice" className="px-3 py-2 text-xs text-muted-foreground">
            {text}
        </Text>
    )
}

// Everything a row draws from: FlashList's extraData, the one signal that redraws
// the rows, so renderItem itself never changes.
interface TableData extends Omit<TableRowProps, 'item'> {
    rows: VisibleRow[]
}

function renderTableItem({ item, index, target, extraData }: ListRenderItemInfo<VisibleRow>) {
    const { rows, ...row }: TableData = extraData
    if (target !== 'StickyHeader') return <TableRow {...row} item={item} />
    return <StickyGroup {...row} rows={rows} index={index} />
}

const rowKey = (item: VisibleRow) => item.row.id
const rowType = (item: VisibleRow) => item.row.kind
const STICKY_CONFIG = { useNativeDriver: Platform.OS !== 'web' }

interface TableListProps extends Omit<TableRowProps, 'item'> {
    rows: VisibleRow[]
    onNearEnd(): void
    isLoadingMore: boolean
}

function TableList({ rows, onNearEnd, isLoadingMore, ...row }: TableListProps) {
    const { columns, layout, format, onToggleGroup } = row
    const sticky = useMemo(() => stickyIndices(rows), [rows])
    const extraData = useMemo<TableData>(
        () => ({ columns, layout, format, onToggleGroup, rows }),
        [columns, layout, format, onToggleGroup, rows]
    )
    return (
        <>
            <HeadingRow columns={columns} layout={layout} format={format} />
            <FlashList
                data={rows}
                renderItem={renderTableItem}
                keyExtractor={rowKey}
                getItemType={rowType}
                extraData={extraData}
                stickyHeaderIndices={sticky}
                stickyHeaderConfig={STICKY_CONFIG}
                onEndReached={onNearEnd}
                onEndReachedThreshold={0.5}
                className="web:[scrollbar-gutter:stable]"
                ListFooterComponent={<LoadingMore isVisible={isLoadingMore} />}
            />
        </>
    )
}

export interface ReportTableProps {
    columns: ResultColumn[]
    rows: VisibleRow[]
    /** The design and params of the run whose rows are shown; a new value starts the list at the top. */
    runKey: string
    format: NumberFormat
    breakpoint: Breakpoint
    layouts: readonly DisplayLayout[]
    onToggleGroup(row: ResultRow): void
    onNearEnd(): void
    isLoadingMore: boolean
}

// FlashList measures every row (wrapped rows have the heights their content
// gives them); onEndReached asks for the next page, through useReportRun's
// guard (hasNextPage and not already fetching). FlashList checks it after each
// commit and again when the rows change, so a first page shorter than the view
// asks for the next one too. The table never scrolls sideways (F10): its line is
// fitted to its width, narrowed, or wrapped (tableLines).
export function ReportTable(props: ReportTableProps) {
    const { rows, runKey, breakpoint, layouts, onToggleGroup, onNearEnd, isLoadingMore } = props
    const columns = useStable(props.columns)
    const format = useStable(props.format)
    const windowWidth = useWindowWidth()
    // The table's own width (a builder drawer narrows it): local layout state from onLayout.
    const [width, setWidth] = useState(windowWidth)
    const lineWidth = width - SCROLL_GUTTER
    const layout = useMemo(
        () => tableLines({ columns, width: lineWidth, breakpoint, layouts }),
        [columns, lineWidth, breakpoint, layouts]
    )
    if (columns.length === 0) return null
    const notice = layout.kind === 'blocks' ? layout.notice : undefined
    // FlashList keeps the row in view anchored when the rows change, which a live
    // re-run of the same design and params needs; a run with a new design, new
    // params or a new sort is another list, so it starts at the top.
    return (
        <View
            testID="report-table"
            className="min-h-0 flex-1"
            onLayout={e => setWidth(e.nativeEvent.layout.width)}
        >
            <LayoutNotice text={notice} />
            <TableList
                key={runKey}
                rows={rows}
                columns={columns}
                layout={layout}
                format={format}
                onToggleGroup={onToggleGroup}
                onNearEnd={onNearEnd}
                isLoadingMore={isLoadingMore}
            />
        </View>
    )
}
```

- [ ] **Step 5: Check**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/components/ReportTable.tsx core/reports/components/ReportRow.tsx core/reports/components/report-table-layout.ts core/reports/components/__tests__/report-table-layout.test.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: PASS (`grep -rn "FrozenLabel\|scrollsSideways\|reportTableLayout\|Sideways" core/reports` prints nothing).

- [ ] **Step 6: Run the e2e: the red one is green, the others stay green, and Q2's width holds**

Check the ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`; kill a leftover server there only if it was started from `/Users/nas/tmp/tinycld/books-purchases`). Then, one file at a time:

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-wrap.spec.ts`
Expected: PASS, the whole file (the 1100 desktop test is now green).

Run: the same with `tests/e2e/reports-ledger-v3.spec.ts`.
Expected: the opening-row, paged, All dates and 1024 tests PASS; the heading-sort, Clear sort and phone tests still FAIL at `report-sort-amount` / `report-invalid` (Task 15); the builder test still FAILS at the This row's field part (Tasks 16–18). The 1024 test is the check of Q2's computed number: its measured line is at least 864 (the General Ledger's narrowed line fits) and under 1008 (the stub's does not). If it fails on the width, record the measured `line` in the task's report and stop: Q2's premise is wrong and the human must see the number before the build goes on.

- [ ] **Step 7: Commit**

The driver commits:

```bash
git add core/reports/components/ReportTable.tsx core/reports/components/ReportRow.tsx core/reports/components/report-table-layout.ts core/reports/components/__tests__/report-table-layout.test.ts
git commit -m "feat(core): report tables fit their width with no sideways scroll; opening rows"
```

---

### Task 15: F8/F11 view — heading sort control, sort in the run and the model

View facts used: `useReportView` builds the run body from the URL's options (`hooks/useReportView.ts:87-101,140-144`) and writes the URL with `router.setParams(serializeViewOptions(...))` (`:177`), which replaces the history entry; `ReportViewModel` is `ReturnType<typeof useReportView>` (`:302`); the refused states draw `ProblemsBody` with an Edit action (`ReportView.tsx:64-67,98-113`; `ReportStates.tsx:78-109` takes any `action`); `run.problems` holds the refusal's problems for an `invalid`/`missing` run and is empty otherwise (`lib/run-state.ts:69-74`); the rows key now carries the sort (Task 12), so a new sort is a new `runKey` and the list starts at the top (F11); `ReportTable` is drawn only by `ReportView.tsx` `TableBody` (`:133-149`); the group fold toggle is a React Native `Pressable` with `accessibilityRole="button"` (`ReportRow.tsx:146-174`); icons take `useThemeColor('muted-foreground')` (`ReportRow.tsx:106-110`). Rulings R7 (sortable columns), R8 (Clear sort), R15 (control parts), R11.

The view model holds the run's problems, not its refusal (`RunView`, `lib/run-state.ts:24-32`), so the helper takes the problems: `refusalSortProblem(problems: readonly Problem[])`. The Clear sort button is a `ProblemsBody` action, which `ReportStates.tsx` already draws, so that file does not change.

**Files:**
- Modify: `core/reports/lib/view-url.ts` (`nextSort`)
- Modify: `core/reports/lib/run-client.ts` (`refusalSortProblem`)
- Modify: `core/reports/hooks/useReportView.ts` (sort in the body and the model)
- Modify: `core/reports/components/ReportView.tsx` (sort props to the table; Clear sort action)
- Modify: `core/reports/components/ReportTable.tsx` (heading sort control)
- Test: `core/reports/lib/__tests__/view-url.test.ts`, `core/reports/lib/__tests__/run-client.test.ts`

**Interfaces:**
- Consumes: `ViewSort` and `RunOptions.sort` (Task 10); `ViewOptions.sort`, `SORT_KEY` (Task 11); sort-aware `rowsKeyOf` (Task 12); `ResultColumn.sortable` set by the run (Task 7); `HeadingRowProps`, `ReportTable` without `Sideways` (Task 14).
- Produces: `nextSort(current: ViewSort | null, col: string): ViewSort | null`; `refusalSortProblem(problems: readonly Problem[]): string | null`; `ReportViewModel.sort: ViewSort | null`, `ReportViewModel.onSortColumn(col: string): void`, `ReportViewModel.onClearSort(): void`, `ReportViewModel.sortProblem: string | null`; `ReportTableProps.sort: ViewSort | null`, `ReportTableProps.onSortColumn(col: string): void`; run body `options.sort` = `[sort]` when the URL has one; heading control test id `report-sort-<col>`, label "Sort by <heading>"; Clear sort test id `report-clear-sort`.

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/__tests__/view-url.test.ts` (add `nextSort` to the import from `'../view-url'`):

```ts
describe('nextSort', () => {
    // A heading press cycles ascending → descending → the report's own order.
    it('cycles a column ascending, descending, then off', () => {
        const asc = nextSort(null, 'amount')
        expect(asc).toEqual({ col: 'amount', dir: 'asc' })
        const desc = nextSort(asc, 'amount')
        expect(desc).toEqual({ col: 'amount', dir: 'desc' })
        expect(nextSort(desc, 'amount')).toBeNull()
    })

    it('starts another column ascending', () => {
        expect(nextSort({ col: 'amount', dir: 'desc' }, 'customer')).toEqual({
            col: 'customer',
            dir: 'asc',
        })
        expect(nextSort({ col: 'amount', dir: 'asc' }, 'customer')).toEqual({
            col: 'customer',
            dir: 'asc',
        })
    })
})
```

Append to `core/reports/lib/__tests__/run-client.test.ts` (add `refusalSortProblem` to the import from `'../run-client'`):

```ts
describe('refusalSortProblem', () => {
    // R8: a refusal of the reader's sort is offered "Clear sort".
    it('is the message of the first problem under options.sort', () => {
        expect(
            refusalSortProblem([
                { path: 'columns.0.id', code: 'invalid', message: 'not this one' },
                {
                    path: 'options.sort.0.col',
                    code: 'unknown_column',
                    message: 'the report has no column "nosuch"',
                },
            ])
        ).toBe('the report has no column "nosuch"')
        expect(
            refusalSortProblem([
                { path: 'options.sort', code: 'invalid', message: 'a pivot report has no opened rows to sort' },
            ])
        ).toBe('a pivot report has no opened rows to sort')
    })

    it('is null for any other refusal', () => {
        expect(refusalSortProblem([])).toBeNull()
        expect(
            refusalSortProblem([
                { path: 'options.sorted', code: 'invalid', message: 'x' },
                { path: 'source', code: 'unknown_source', message: 'y' },
            ])
        ).toBeNull()
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/view-url.test.ts core/reports/lib/__tests__/run-client.test.ts`
Expected: FAIL — `nextSort is not a function`, `refusalSortProblem is not a function`.

- [ ] **Step 3: The two helpers**

Append to `lib/view-url.ts`:

```ts
/** The sort after a press on a column's heading: ascending, then descending, then the report's own order. */
export function nextSort(current: ViewSort | null, col: string): ViewSort | null {
    if (current?.col !== col) return { col, dir: 'asc' }
    return current.dir === 'asc' ? { col, dir: 'desc' } : null
}
```

Append to `lib/run-client.ts`:

```ts
const SORT_OPTION_PATH = 'options.sort'

/**
 * Why the run refused the reader's sort (a column the report no longer has, R8):
 * the message of the first problem under options.sort, else null.
 */
export function refusalSortProblem(problems: readonly Problem[]): string | null {
    const problem = problems.find(
        p => p.path === SORT_OPTION_PATH || p.path.startsWith(`${SORT_OPTION_PATH}.`)
    )
    return problem?.message ?? null
}
```

- [ ] **Step 4: Run them to see them pass**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/view-url.test.ts core/reports/lib/__tests__/run-client.test.ts`
Expected: PASS.

- [ ] **Step 5: `useReportView.ts`**

Change the imports: `import { refusalSortProblem, type RunBody, type ViewSort } from '../lib/run-client'` (replacing `import type { RunBody } from '../lib/run-client'`), and add `nextSort` to the import from `'../lib/view-url'`.

Replace `runBody`:

```ts
// The reader's sort goes before the report's own (F8); the client sends one key.
function sortOptions(sort: ViewSort | null): { sort?: ViewSort[] } {
    return sort ? { sort: [sort] } : {}
}

function runBody(
    source: ViewSource | null,
    defs: readonly ParamDef[],
    options: ViewOptions,
    expansion: Expansion
): RunBody | null {
    if (!source) return null
    const common = {
        params: postedParams(defs, options.params),
        options: { today: todayIso(), ...expansionOptions(expansion), ...sortOptions(options.sort) },
    }
    return source.kind === 'saved'
        ? { reportId: source.reportId, ...common }
        : { spec: source.spec, ...common }
}
```

In the returned model, after `onResetDisplay: …,` add:

```ts
        sort: options.sort,
        // A new sort is a new rows key (rowsKeyOf), so the list starts at the top (F11).
        onSortColumn: (col: string) => write({ ...options, sort: nextSort(options.sort, col) }),
        onClearSort: () => write({ ...options, sort: null }),
        // A run refused for the reader's sort offers Clear sort (R8).
        sortProblem: refusalSortProblem(run.problems),
```

- [ ] **Step 6: `ReportView.tsx`**

Replace `editAction` with:

```tsx
function editAction(model: ReportViewModel) {
    if (!model.onEdit || !model.access.canEdit) return null
    return { label: 'Edit report', testID: 'report-missing-edit', onPress: model.onEdit }
}

// R8: a run refused for the reader's sort (a copied link naming a column the
// report no longer has) offers to clear it, which runs the report in its own
// order; any other refusal offers the editor.
function refusalAction(model: ReportViewModel) {
    if (!model.sortProblem) return editAction(model)
    return { label: 'Clear sort', testID: 'report-clear-sort', onPress: model.onClearSort }
}
```

In `ReportBody`, replace `const edit = editAction(model)` with `const fix = refusalAction(model)` and pass `action={fix}` to both `ProblemsBody` elements (`report-missing` and `report-invalid`).

In `TableBody`, add to `<ReportTable …>`:

```tsx
            sort={model.sort}
            onSortColumn={model.onSortColumn}
```

- [ ] **Step 7: `ReportTable.tsx` heading sort control**

Imports: add `import { useThemeColor } from '@tinycld/core/lib/use-app-theme'`, `import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react-native'`, `Pressable` to the `react-native` import, and `import type { ViewSort } from '../lib/run-client'`.

Replace `HeadingCell`, `LabelHeading`, `HeadingRowProps`, `OneLineHeading` and `BlocksHeading` with:

```tsx
/** The reader's sort the heading shows, and the press that changes it. */
interface HeadingSort {
    sort: ViewSort | null
    onSortColumn(col: string): void
}

const SORT_ICON_SIZE = 12
const SORT_ICONS = { asc: ArrowUp, desc: ArrowDown, none: ArrowUpDown } as const
type SortIconKind = keyof typeof SORT_ICONS
const HEADING_JUSTIFY = { start: 'flex-start', center: 'center', end: 'flex-end' } as const

const sortIconOf = (sort: ViewSort | null, col: string): SortIconKind =>
    sort?.col === col ? sort.dir : 'none'

function SortIcon({ kind }: { kind: SortIconKind }) {
    const color = useThemeColor('muted-foreground')
    const Icon = SORT_ICONS[kind]
    return <Icon size={SORT_ICON_SIZE} color={color} />
}

function HeadingText({ text, justify }: { text: string; justify: Justify }) {
    return (
        <Text
            numberOfLines={1}
            className={`shrink text-xs font-semibold uppercase text-muted-foreground ${TEXT_ALIGN[justify]}`}
        >
            {text}
        </Text>
    )
}

interface HeadingContentProps {
    column: ResultColumn
    text: string
    justify: Justify
    sorting: HeadingSort
}

// R15: a column the run can sort by (ResultColumn.sortable) has a heading that is a
// button; each press cycles ascending → descending → the report's order. The icon
// sits outside the text, so the ellipsis never hides it. A running value's
// heading is plain text.
function HeadingContent({ column, text, justify, sorting }: HeadingContentProps) {
    if (!column.sortable) return <HeadingText text={text} justify={justify} />
    return (
        <Pressable
            testID={`report-sort-${column.id}`}
            accessibilityRole="button"
            accessibilityLabel={`Sort by ${text}`}
            onPress={() => sorting.onSortColumn(column.id)}
            className="min-h-11 flex-1 flex-row items-center gap-1"
            style={{ justifyContent: HEADING_JUSTIFY[justify] }}
        >
            <HeadingText text={text} justify={justify} />
            <SortIcon kind={sortIconOf(sorting.sort, column.id)} />
        </Pressable>
    )
}

function HeadingCell({
    column,
    justify,
    width,
    format,
    sorting,
}: {
    column: ResultColumn
    justify: Justify
    width?: number
    format: NumberFormat
    sorting: HeadingSort
}) {
    const text = headingText(column, format)
    return (
        <View
            testID={`report-heading-${column.id}`}
            className="justify-center px-3"
            style={cellBoxStyle(width)}
        >
            <HeadingContent column={column} text={text} justify={justify} sorting={sorting} />
        </View>
    )
}

// The label's heading sits over the label's text, which starts after the row's indent.
function LabelHeading({
    column,
    format,
    sorting,
}: {
    column: ResultColumn
    format: NumberFormat
    sorting: HeadingSort
}) {
    return (
        <View className="grow pl-1">
            <HeadingCell column={column} justify="start" format={format} sorting={sorting} />
        </View>
    )
}

interface HeadingRowProps {
    columns: ResultColumn[]
    layout: LineLayout
    format: NumberFormat
    sorting: HeadingSort
}

function OneLineHeading({ columns, layout, format, sorting }: HeadingRowProps) {
    if (layout.kind !== 'one-line') return null
    const [label, ...amounts] = columns
    if (!label) return null
    const cells = amounts.map((column, i) => (
        <HeadingCell
            key={column.id}
            column={column}
            justify={amountJustify(column, i + 1)}
            width={layout.widths[column.id]}
            format={format}
            sorting={sorting}
        />
    ))
    return (
        <>
            <View style={oneLineLabelStyle(layout, label.id)}>
                <LabelHeading column={label} format={format} sorting={sorting} />
            </View>
            {cells}
        </>
    )
}

// The heading is drawn by the same blocks as the rows, so each heading sits over
// its values and wraps the same way.
function BlocksHeading({ columns, layout, format, sorting }: HeadingRowProps) {
    if (layout.kind !== 'blocks') return null
    const byId = new Map(columns.map(column => [column.id, column]))
    const labelId = columns[0]?.id
    const renderCell: RenderCell = (col, justify) => {
        const column = byId.get(col)
        if (!column) return null
        if (col === labelId) return <LabelHeading column={column} format={format} sorting={sorting} />
        return <HeadingCell column={column} justify={justify} format={format} sorting={sorting} />
    }
    return (
        <View className="flex-1">
            <BlockView block={layout.root} renderCell={renderCell} />
        </View>
    )
}
```

`HeadingCells` and `HeadingRow` pass their props through unchanged. Thread `sorting` through the list:

```tsx
interface TableListProps extends Omit<TableRowProps, 'item'> {
    rows: VisibleRow[]
    onNearEnd(): void
    isLoadingMore: boolean
    sorting: HeadingSort
}

function TableList({ rows, onNearEnd, isLoadingMore, sorting, ...row }: TableListProps) {
```

and render `<HeadingRow columns={columns} layout={layout} format={format} sorting={sorting} />` (the rows' `extraData` does not change: a sort press does not redraw rows; a new sort's rows come with a new `runKey`).

In `ReportTableProps`, after `isLoadingMore: boolean` add:

```ts
    /** The reader's sort (the view URL's `s=`); null: the report's own order. */
    sort: ViewSort | null
    onSortColumn(col: string): void
```

In `ReportTable`, after `const format = useStable(props.format)` add `const sorting = { sort: props.sort, onSortColumn: props.onSortColumn }`, and pass `sorting={sorting}` to `<TableList …>`.

- [ ] **Step 8: Check**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/view-url.ts core/reports/lib/run-client.ts core/reports/hooks/useReportView.ts core/reports/components/ReportView.tsx core/reports/components/ReportTable.tsx core/reports/lib/__tests__/view-url.test.ts core/reports/lib/__tests__/run-client.test.ts && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: PASS.

- [ ] **Step 8a: Run the e2e Task 13a wrote red for this task**

The UI behaviour — the press cycle asc → desc → report order, `s=` in the URL and the copied link, `role="button"` with "Sort by Amount", no control on the running `balance` column, the list starting at the top, and Clear sort on a refused `s=nosuch:asc` — is pinned by Task 13a's `reports-ledger-v3.spec.ts`, at desktop and phone. Before this task its heading-sort and phone tests failed at `report-sort-amount` (element not found) and its Clear sort test at `report-invalid` (the URL's sort was not sent).

Check the ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`). Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-ledger-v3.spec.ts`
Expected: every test PASSES except the builder test, which still fails at the This row's field part (Tasks 16–18). The 1024 test stays green with the sort icons in the headings.

- [ ] **Step 9: Commit**

The driver commits:

```bash
git add core/reports/lib/view-url.ts core/reports/lib/run-client.ts core/reports/hooks/useReportView.ts core/reports/components/ReportView.tsx core/reports/components/ReportTable.tsx core/reports/lib/__tests__/view-url.test.ts core/reports/lib/__tests__/run-client.test.ts
git commit -m "feat(core): a report heading sorts its column; a refused sort offers Clear sort"
```

### Task 16: Builder formula parts (lib) — Add up / Count, This row's field, Period start / end

F1, F2, F3 (builder UI). Facts used: the part list and `partOf` (`lib/builder/formula-parts.ts:18-302`); `editable`, `childContext`, `partsAt`, `leafPart` (`lib/builder/formula-ops.ts:67-354`); `formulaType` (`lib/builder/formula-type.ts:66-170`); `describeNode` (`lib/builder/formula-describe.ts:99-175`, lookup text at `:113-120`); the two context builders (`lib/builder/formula-context.ts:410-434`, `:453-483`) and the column type reader (`:188-197`); the pick menu (`components/builder/report-formula.tsx:252-292`); `PathLeaf` (`components/builder/report-formula-paths.tsx:136-164`); the shared fixtures (`lib/builder/__tests__/formula-fixture.ts`, `catalog-fixture.ts`: sales fields customer, amount, cost, quantity, currency, day, status; link team).

**Files:**
- Modify: `core/reports/lib/builder/formula-parts.ts` (parts `outer`, `bound`; `LOOKUP_PICKS`; `boundChoices`; `ReportFormulaContext.rowSource`; `partOf`)
- Modify: `core/reports/lib/builder/formula-ops.ts` (`editable`, `partsAt`, `leafPart`)
- Modify: `core/reports/lib/builder/formula-type.ts` (`FormulaTypes.rowSource`, `typesOf`, outer / bound / count / sum types)
- Modify: `core/reports/lib/builder/formula-describe.ts` (outer, bound, add up, count)
- Modify: `core/reports/lib/builder/formula-context.ts` (`rowSource` in both contexts and in the column type reader)
- Modify: `core/reports/lib/builder/__tests__/formula-fixture.ts` (`rowSource` on `CTX` and `LAYOUT`)
- Test: `core/reports/lib/builder/__tests__/formula-ops.test.ts`, `formula-describe.test.ts`, `formula-type.test.ts`

The two components that draw these parts (`report-formula.tsx`, `report-formula-paths.tsx`) are Task 16a. This task is lib only: after it the part menu offers `outer` and `bound` (it lists `partsAt`), but no leaf draws them yet.

**Interfaces:**
- Consumes (Task 10): `SpecNode.outer?: SpecHop[]`, `SpecNode.bound?: 'from' | 'to'`, `SpecNode.pick?: 'min' | 'max' | 'sum' | 'count'` in `core/reports/lib/spec.ts`.
- Produces: `type LookupPick = 'min' | 'max' | 'sum' | 'count'`; `LOOKUP_PICKS: readonly { value: LookupPick; label: string }[]` (labels Smallest, Largest, Add up, Count); parts `'outer'` (label "This row's field") and `'bound'` (label "Range start or end") in `REPORT_PARTS`; `boundChoices(params: readonly FormulaParam[]): { param: string; bound: 'from' | 'to'; label: string }[]` (labels "<param label> start" / "<param label> end"); `ReportFormulaContext.rowSource: string | null` (the report's source; null in a layout computed row); `FormulaTypes.rowSource?: string | null`. Task 16a uses all of them.

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/builder/__tests__/formula-fixture.ts`, give the shared contexts the report row's source (the type now requires it):

```ts
export const CTX: ReportFormulaContext = {
    catalog: CATALOG,
    leaf: 'col',
    values: [
        { key: 'amount', label: 'Amount', type: 'money' },
        { key: 'cost', label: 'Cost', type: 'money' },
        { key: 'qty', label: 'Quantity', type: 'number' },
    ],
    source: SALES_KEY,
    rowSource: SALES_KEY,
    records: [{ group: 'byTeam', label: 'Team', source: TEAMS_KEY }],
    params: [{ id: 'asOf', label: 'As of', type: 'asOf' }],
    totals: true,
    windows: true,
    lookups: true,
}
// A computed row looks values up (F16); only its lookups read params. It has no
// report row, so "This row's field" is not offered there.
export const LAYOUT: ReportFormulaContext = {
    ...CTX,
    leaf: 'subtotal',
    source: null,
    rowSource: null,
    records: [],
    params: [],
    lookupParams: CTX.params,
    totals: false,
    windows: false,
    lookups: true,
}
```

Append to `core/reports/lib/builder/__tests__/formula-ops.test.ts` (add `boundChoices, LOOKUP_PICKS` to the `../formula-parts` import and `SALES_KEY` is already imported):

```ts
describe('the v3 lookup and leaf parts', () => {
    // Sales on the same day as the report's row (F1 count, F2 outer).
    const SAME_DAY: SpecNode = {
        op: 'lookup',
        source: SALES_KEY,
        where: { op: 'eq', args: [{ path: [{ field: 'day' }] }, { outer: [{ field: 'day' }] }] },
        value: { path: [{ field: 'id' }] },
        pick: 'count',
    }

    it('picks a lookup match as Smallest, Largest, Add up or Count', () => {
        expect(LOOKUP_PICKS).toEqual([
            { value: 'min', label: 'Smallest' },
            { value: 'max', label: 'Largest' },
            { value: 'sum', label: 'Add up' },
            { value: 'count', label: 'Count' },
        ])
        expect(isEditableNode({ ...LOOKUP, pick: 'sum' }, PARAMS)).toBe(true)
        expect(isEditableNode(SAME_DAY, PARAMS)).toBe(true)
    })

    it("offers This row's field only inside a lookup that a report row reads", () => {
        expect(partsAt(CTX, { value: 0 }, [])).not.toContain('outer')
        expect(partsAt(CTX, LOOKUP, [0, 1])).toContain('outer')
        expect(partsAt(CTX, LOOKUP, [1])).toContain('outer')
        // A computed layout row has no report row to read.
        expect(partsAt(LAYOUT, LOOKUP, [0, 1])).not.toContain('outer')
    })

    it("edits This row's field inside a lookup and keeps one outside a lookup read-only", () => {
        expect(partOf({ outer: [{ field: 'day' }] })).toBe('outer')
        expect(isEditableNode({ outer: [{ field: 'day' }] }, PARAMS)).toBe(false)
        expect(
            isEditableNode({ op: 'add', args: [{ outer: [{ field: 'amount' }] }, { value: 1 }] }, PARAMS)
        ).toBe(false)
        expect(withPart(CTX, LOOKUP, [0, 1], 'outer')).toEqual({ outer: [{ field: 'customer' }] })
    })

    it('offers a start and an end for each date range param', () => {
        expect(boundChoices(WITH_PERIOD.params)).toEqual([
            { param: 'period', bound: 'from', label: 'Period start' },
            { param: 'period', bound: 'to', label: 'Period end' },
        ])
        expect(boundChoices(CTX.params)).toEqual([])
        expect(partsAt(WITH_PERIOD, { value: 0 }, [])).toContain('bound')
        expect(partsAt(CTX, { value: 0 }, [])).not.toContain('bound')
        // A period slot still takes the range itself, not one of its ends.
        expect(partsAt(WITH_PERIOD, IN_PERIOD, [1])).toEqual(['param'])
        expect(withPart(WITH_PERIOD, { value: 0 }, [], 'bound')).toEqual({
            param: 'period',
            bound: 'from',
        })
        expect(partOf({ param: 'period', bound: 'to' })).toBe('bound')
        expect(isEditableNode({ param: 'period', bound: 'to' }, PARAMS)).toBe(true)
        expect(isEditableNode({ param: 'asOf', bound: 'to' }, PARAMS)).toBe(false)
    })
})
```

Append to `core/reports/lib/builder/__tests__/formula-type.test.ts` (import `TEAMS_KEY` from `./catalog-fixture` next to `CATALOG, SALES_KEY`):

```ts
describe('formulaType of the v3 parts', () => {
    const types = {
        catalog: CATALOG,
        source: SALES_KEY,
        rowSource: SALES_KEY,
        col: (k: string) => CTX.values.find(v => v.key === k)?.type,
        records: CTX.records,
        params: WITH_PERIOD.params,
    }

    it('types a count as a number, a sum as its value, a bound as a date', () => {
        expect(formulaType({ ...LOOKUP, pick: 'count' }, types)).toBe('number')
        const added = {
            op: 'lookup',
            source: SALES_KEY,
            value: { path: [{ field: 'amount' }] },
            pick: 'sum' as const,
        }
        expect(formulaType(added, types)).toBe('money')
        expect(formulaType({ param: 'period', bound: 'from' }, types)).toBe('date')
    })

    it("types This row's field from the report's source inside a lookup on another source", () => {
        const day = {
            op: 'lookup',
            source: TEAMS_KEY,
            value: { outer: [{ field: 'day' }] },
        }
        expect(formulaType(day, types)).toBe('date')
        expect(typesOf(CTX).rowSource).toBe(SALES_KEY)
    })
})
```

Append to `core/reports/lib/builder/__tests__/formula-describe.test.ts` (import `SALES_KEY` from `./catalog-fixture`):

```ts
describe('describeNode for the v3 parts', () => {
    const sameDay = (pick: 'sum' | 'count', field: string) => ({
        op: 'lookup',
        source: SALES_KEY,
        where: { op: 'eq', args: [{ path: [{ field: 'day' }] }, { outer: [{ field: 'day' }] }] },
        value: { path: [{ field }] },
        pick,
    })

    it("says add up, count and this row's field", () => {
        expect(describeNode(CTX, sameDay('sum', 'amount'))).toBe(
            "add up Amount in Stub sales where Day = this row's Day"
        )
        expect(describeNode(CTX, sameDay('count', 'id'))).toBe(
            "count ID in Stub sales where Day = this row's Day"
        )
    })

    it('says a range start or end by its param', () => {
        const before = { op: 'lt', args: [{ path: [{ field: 'day' }] }, { param: 'period', bound: 'from' }] }
        expect(describeNode(WITH_PERIOD, before)).toBe('Day < {Period start}')
        expect(describeNode(WITH_PERIOD, { param: 'period', bound: 'to' })).toBe('{Period end}')
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/builder/__tests__/formula-ops.test.ts core/reports/lib/builder/__tests__/formula-type.test.ts core/reports/lib/builder/__tests__/formula-describe.test.ts`
Expected: FAIL. `LOOKUP_PICKS` and `boundChoices` are undefined ("expected undefined to deeply equal …" / "boundChoices is not a function"); `partOf({ outer })` is `'unknown'`; `partsAt` has no `'outer'` / `'bound'`; `formulaType` of a count lookup is `'relation'`; describe says "look up Amount … (smallest)" and "a part the editor cannot show". The tests that existed before stay green.

- [ ] **Step 3: Implement the parts (`formula-parts.ts`)**

Add `'outer'` after `'path'` and `'bound'` after `'param'` in `VALUE_PARTS`:

```ts
const VALUE_PARTS = [
    'number',
    'text',
    'yesno',
    'empty',
    'list',
    'field',
    'path',
    'outer',
    'record',
    'param',
    'bound',
] as const
```

Add to `PART_LABELS` (after `path` and after `param`):

```ts
    outer: "This row's field",
    bound: 'Range start or end',
```

Add below `PART_LABELS`:

```ts
export type LookupPick = 'min' | 'max' | 'sum' | 'count'
/** Which match a lookup takes (lookup.go lookupIn): one value, or all of them added up or counted. */
export const LOOKUP_PICKS: readonly { value: LookupPick; label: string }[] = [
    { value: 'min', label: 'Smallest' },
    { value: 'max', label: 'Largest' },
    { value: 'sum', label: 'Add up' },
    { value: 'count', label: 'Count' },
]
```

In `ReportFormulaContext`, after `source`:

```ts
    /**
     * The report's source: what an {outer} path reads (lookup.go lookupCtx.outerPath).
     * null where there is no report row, a layout computed row.
     */
    rowSource: string | null
```

Below `isRangeParam`:

```ts
/** The first and last day of each date range param (params.go paramExpr bound). */
export function boundChoices(params: readonly FormulaParam[]) {
    return params.filter(isRangeParam).flatMap(p => {
        const name = p.label ?? p.id
        return [
            { param: p.id, bound: 'from' as const, label: `${name} start` },
            { param: p.id, bound: 'to' as const, label: `${name} end` },
        ]
    })
}
```

Replace `partOf`:

```ts
export function partOf(node: SpecNode): ReportPart | 'unknown' {
    if (node.record !== undefined) return 'record'
    if (node.col !== undefined || node.subtotal !== undefined) return 'field'
    if (node.path !== undefined) return 'path'
    if (node.outer !== undefined) return 'outer'
    if (node.param !== undefined) return node.bound !== undefined ? 'bound' : 'param'
    if (node.op === undefined) return valuePartOf(node.value)
    return operatorPartOf(node)
}
```

- [ ] **Step 4: Implement editing (`formula-ops.ts`)**

Import `boundChoices` from `./formula-parts`. In `editable`, after `case 'param':`:

```ts
        // The report row's value is read only inside a lookup (formula.go ctxBase.outerPath).
        case 'outer':
            return inLookup && onlyKeys(node, ['outer']) && isFieldPath(node.outer ?? [])
        case 'bound': {
            const param = params.find(p => p.id === node.param)
            return onlyKeys(node, ['param', 'bound']) && param !== undefined && isRangeParam(param)
        }
```

In `partsAt`, inside the filter, before `if (part === 'param') return hasParams`:

```ts
        if (part === 'outer') return here.inLookup === true && here.rowSource !== null
        if (part === 'bound') return boundChoices(here.params).length > 0
```

In `leafPart`, after `case 'path':`:

```ts
        case 'outer': {
            const field = firstFieldOf(here, here.rowSource)
            return field ? { outer: [{ field }] } : current
        }
        case 'bound': {
            const [first] = boundChoices(here.params)
            return first ? { param: first.param, bound: first.bound } : current
        }
```

(`childContext` spreads `here`, so `rowSource` stays the report's source in every nested lookup, R4.)

- [ ] **Step 5: Implement types and the read-back line**

In `formula-type.ts`, add to `FormulaTypes` after `source`:

```ts
    /** The report's source, which an {outer} path reads; absent reads as none. */
    rowSource?: string | null
```

In `typesOf`, add `rowSource: ctx.rowSource,` after `source`. In `formulaType`, replace the `lookup` case and add two cases before it:

```ts
        case 'outer': {
            const info = types.rowSource
                ? describePath(types.catalog, types.rowSource, node.outer ?? [])
                : null
            return info?.field.type ?? ''
        }
        case 'bound':
            return 'date'
        // lookup.go lookupIn: a count is a number; a sum keeps its value's type.
        case 'lookup':
            if (node.pick === 'count') return 'number'
            return formulaType(lookupValueNode(node), { ...types, source: node.source ?? null })
```

In `formula-describe.ts` `describeLeaf`, add after `case 'param':`:

```ts
        case 'outer': {
            const info = here.rowSource
                ? describePath(here.catalog, here.rowSource, n.outer ?? [])
                : null
            return info ? `this row's ${pathLabel(info)}` : "this row's field (missing)"
        }
        case 'bound': {
            const name = here.params.find(p => p.id === n.param)?.label ?? n.param
            return `{${name} ${n.bound === 'to' ? 'end' : 'start'}}`
        }
```

Replace the `lookup` case of `describeNode`:

```ts
            case 'lookup': {
                const inside = contextAt(here, n, [1])
                const [condition] = REPORT_TREE.childrenOf(n)
                const source = here.catalog.get(n.source ?? '')?.label ?? `${n.source} (missing)`
                const where = whereText(condition, inside)
                const value = say(lookupValueNode(n), inside, false)
                if (n.pick === 'sum') return `add up ${value} in ${source}${where}`
                if (n.pick === 'count') return `count ${value} in ${source}${where}`
                const pick = n.pick === 'max' ? 'largest' : 'smallest'
                return `look up ${value} in ${source}${where} (${pick})`
            }
```

In `formula-context.ts`: in `layoutContext`'s return add `rowSource: null,` after `source: null,`; in `formulaContext`'s return add `rowSource: draft.source,` after `source: draft.source,`; in the column type reader's `formulaTypes` object add `rowSource: draft.source,` after `source: draft.source,`.

- [ ] **Step 6: Run the tests to see them pass**

Run: the Step 2 command.
Expected: PASS (all tests of the three files).

- [ ] **Step 7: Check the member**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/builder/formula-parts.ts core/reports/lib/builder/formula-ops.ts core/reports/lib/builder/formula-type.ts core/reports/lib/builder/formula-describe.ts core/reports/lib/builder/formula-context.ts core/reports/lib/builder/__tests__/formula-fixture.ts core/reports/lib/builder/__tests__/formula-ops.test.ts core/reports/lib/builder/__tests__/formula-type.test.ts core/reports/lib/builder/__tests__/formula-describe.test.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```
Expected: biome, tsc and every vitest file green. A tsc error "Property 'rowSource' is missing" names a context literal this step did not list: add `rowSource` there (the report's source, or null where no report row exists).

- [ ] **Step 8: Commit (the driver runs it)**

```bash
git add core/reports/lib/builder/formula-parts.ts core/reports/lib/builder/formula-ops.ts core/reports/lib/builder/formula-type.ts core/reports/lib/builder/formula-describe.ts core/reports/lib/builder/formula-context.ts core/reports/lib/builder/__tests__/formula-fixture.ts core/reports/lib/builder/__tests__/formula-ops.test.ts core/reports/lib/builder/__tests__/formula-type.test.ts core/reports/lib/builder/__tests__/formula-describe.test.ts
git commit -m "feat(core): report formula parts add up and count lookups, read this row's field and a range's start or end"
```

---

### Task 16a: Builder formula leaves (UI) — Add up / Count menu, This row's field, Range start or end

F1, F2, F3 (builder UI). Facts used: the pick menu of `LookupLeaf` (`components/builder/report-formula.tsx:252-292`, items `min`/`max` written in the component); `ParamLeaf` and `ReportFormulaLeaf` in the same file; `PathLeaf` and `ColumnPicker` (`components/builder/report-formula-paths.tsx:136-164`); `contextAt`, `partOf`, `describePath`, `pathLabel` (lib, as `PathLeaf` uses them); a `PickMenu` row's test id is `${testID}-${item.id}` (`core/ui/pick-menu/index.tsx:23,32`).

**Files:**
- Modify: `core/reports/components/builder/report-formula.tsx` (pick menu from `LOOKUP_PICKS`, `BoundLeaf`)
- Modify: `core/reports/components/builder/report-formula-paths.tsx` (`OuterLeaf`)
- Test (written in Task 13a, run here): `tests/e2e/reports-ledger-v3.spec.ts` "the builder counts a lookup on this row, …"

**Interfaces:**
- Consumes (Task 16): `LOOKUP_PICKS`, `LookupPick`, `boundChoices`, parts `'outer'` and `'bound'`, `ReportFormulaContext.rowSource`.
- Produces: leaf test ids `<id>-outer` (a `ColumnPicker` over the report's source, to-one paths), `<id>-bound` (a `PickMenu`, items `<param>-from` / `<param>-to`, label "<param label> start" / "<param label> end"); lookup pick menu items `min|max|sum|count` under `<id>-pick` (labels Smallest, Largest, Add up, Count).

No vitest: this task changes components only (CLAUDE.md). The e2e Task 13a wrote drives every control it adds.

- [ ] **Step 1: Run the builder e2e to see it red for this task's reason**

Check the ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`). Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-ledger-v3.spec.ts -g "the builder counts a lookup"`
Expected: FAIL at `report-builder-column-3-formula-0-1-outer`: element not found. (Task 16 made the part menu offer `outer`, so the step before it passes; no leaf draws the part yet.)

- [ ] **Step 2: Implement the leaves (UI)**

In `components/builder/report-formula.tsx`, import `boundChoices, LOOKUP_PICKS` from `../../lib/builder/formula-parts` and `OuterLeaf` from `./report-formula-paths`. In `LookupLeaf`, replace the `picks` and `pickLabel` lines:

```tsx
    const picks: PickItem[] = LOOKUP_PICKS.map(p => ({
        id: p.value,
        label: p.label,
        isSelected: pick === p.value,
        onSelect: () => set({ ...expr, pick: p.value }),
    }))
    const sourceLabel = ctx.catalog.get(expr.source ?? '')?.label ?? `${expr.source} (missing)`
    const pickLabel = LOOKUP_PICKS.find(p => p.value === pick)?.label ?? pick
```

Add after `ParamLeaf`:

```tsx
// The first or last day of a date range param (params.go paramExpr bound).
function BoundLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'bound') return null
    const here = contextAt(ctx, root, path)
    const choices = boundChoices(here.params)
    const isCurrent = (c: (typeof choices)[number]) => c.param === expr.param && c.bound === expr.bound
    const items: PickItem[] = choices.map(c => ({
        id: `${c.param}-${c.bound}`,
        label: c.label,
        isSelected: isCurrent(c),
        onSelect: () => set({ param: c.param, bound: c.bound }),
    }))
    const shown = choices.find(isCurrent)?.label ?? `${expr.param} (missing)`
    return (
        <PickMenu
            label={shown}
            purpose={`Range start or end${where}`}
            items={items}
            testID={`${id}-bound`}
        />
    )
}
```

In `ReportFormulaLeaf`, add `<OuterLeaf {...props} />` after `<PathLeaf {...props} />` and `<BoundLeaf {...props} />` after `<ParamLeaf {...props} />`.

In `components/builder/report-formula-paths.tsx`, add after `PathLeaf`:

```tsx
/** A field of the report's row, read inside a lookup (lookup.go lookupCtx.outerPath): to-one paths only. */
export function OuterLeaf({ ctx, root, path, expr, set, id }: ReportLeafProps) {
    if (partOf(expr) !== 'outer') return null
    const here = contextAt(ctx, root, path)
    if (!here.rowSource) return null
    const info = describePath(here.catalog, here.rowSource, expr.outer ?? [])
    const label = info ? `This row's ${pathLabel(info)}` : "Choose this row's field"
    return (
        <ColumnPicker
            catalog={here.catalog}
            baseKey={here.rowSource}
            onPick={next => set({ outer: next })}
            label={label}
            testID={`${id}-outer`}
            toOneOnly
        />
    )
}
```

- [ ] **Step 3: Check the member**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/components/builder/report-formula.tsx core/reports/components/builder/report-formula-paths.tsx
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```
Expected: biome, tsc and every vitest file green.

- [ ] **Step 4: Run the builder e2e: its failure moves past this task's controls**

Run: the Step 1 command.
Expected: the lookup steps pass (the read-back line is "= count Customer in Stub sales where Day = this row's Day", and column 4's `-root-bound` reads "Day start"); the test now FAILS at `report-builder-column-2-rollup`: element not found, Task 18's control. A failure at or before `-root-bound` is a defect in this task's leaves, fixed here. Then run the existing formula e2e, one file at a time, which drive the same menus: `tests/e2e/reports-formulas-v2.spec.ts`, then `tests/e2e/reports-formula-leaves.spec.ts`. Expected: each PASS (the lookup's Smallest/Largest items keep their ids `min`/`max`).

- [ ] **Step 5: Commit (the driver runs it)**

```bash
git add core/reports/components/builder/report-formula.tsx core/reports/components/builder/report-formula-paths.tsx
git commit -m "feat(core): report formula editor offers Add up, Count, This row's field and a range's start or end"
```

---

### Task 17: Builder draft — v3 round-trip, opening and rollup drafts

F9 (builder), F5, F6 (draft). Facts used: `formulaDraftSchema`, `columnFields`, `groupLevelSchema` (`lib/builder/draft.ts:79-166`); `draftIssues` (`:370-384`); `columnDraft` (`:525-557`); `groupLevelDraft` (`:642-657`); `specFormula`, `specColumns` (`:676-696`); `keptParams` (`:797-820`); `specGroups` (`:846-867`); `specFromDraft` writes `CURRENT_SPEC_VERSION` (`:888`, 3 after Task 10); `paramsInNode` reads `node.param`, so a `{ param, bound }` node is already read (`lib/builder/filter.ts:809-816`); the draft test file's `META` and `CATALOG` (`lib/builder/__tests__/draft.test.ts:25,581`).

Optional fields: `ColumnDraft.rollup` and `GroupLevelDraft.opening` are declared `.nullable().optional()`: a column or level read from a spec without them carries no key, so the many hand-built drafts in the existing tests (and the "Add group level" / "Add column" literals) stay valid and their `toEqual` checks unchanged. `undefined` and `null` both mean "none".

**Files:**
- Modify: `core/reports/lib/builder/draft.ts`
- Test: `core/reports/lib/builder/__tests__/draft.test.ts`

**Interfaces:**
- Consumes: Task 10 `SpecColumn.rollup?: SpecNode`, `SpecGroup.opening?: { label: string; cells?: Record<string, SpecNode> }`, `CURRENT_SPEC_VERSION = 3`; Task 16 `isEditableNode` (edits `outer`, `bound`, `pick: 'sum' | 'count'`).
- Produces: `openingDraftSchema`; `type OpeningDraft = { label: string; cells: Record<string, FormulaDraft> }`; `ColumnDraft.rollup?: FormulaDraft | null`; `GroupLevelDraft.opening?: OpeningDraft | null`; `openingDraft(label: string): OpeningDraft`; `setOpeningCell(opening: OpeningDraft, col: string, formula: FormulaDraft | null): OpeningDraft`; `draftIssues` problems at `columns.N.rollup` and `groups.N.opening[.cells.<col>]`.

- [ ] **Step 1: Write the failing tests**

Append to `core/reports/lib/builder/__tests__/draft.test.ts` (add `openingDraft, setOpeningCell` to the `../draft` import):

```ts
// A team's sales before the period: the opening of a running balance (F1 sum, F3 bound).
const OPEN_BALANCE: SpecNode = {
    op: 'coalesce',
    args: [
        {
            op: 'lookup',
            source: SALES_KEY,
            where: {
                op: 'and',
                args: [
                    {
                        op: 'eq',
                        args: [
                            { path: [{ link: 'team' }] },
                            { record: 'byTeam', path: [{ field: 'id' }] },
                        ],
                    },
                    {
                        op: 'lt',
                        args: [{ path: [{ field: 'day' }] }, { param: 'period', bound: 'from' }],
                    },
                ],
            },
            value: { path: [{ field: 'amount' }] },
            pick: 'sum',
        },
        { value: 0 },
    ],
}

const V3: ReportSpec = {
    version: CURRENT_SPEC_VERSION,
    source: SALES_KEY,
    params: [{ id: 'period', type: 'dateRange', label: 'Period', default: { preset: 'allDates' } }],
    columns: [
        { id: 'team', label: 'Team', path: [{ link: 'team' }] },
        { id: 'day', label: 'Day', path: [{ field: 'day' }] },
        { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
        {
            id: 'sameDay',
            label: 'Same day',
            formula: {
                op: 'lookup',
                source: SALES_KEY,
                where: {
                    op: 'eq',
                    args: [{ path: [{ field: 'day' }] }, { outer: [{ field: 'day' }] }],
                },
                value: { path: [{ field: 'id' }] },
                pick: 'count',
            },
        },
        {
            id: 'balance',
            label: 'Balance',
            formula: {
                op: 'add',
                args: [OPEN_BALANCE, { op: 'runningSum', args: [{ path: [{ field: 'amount' }] }] }],
            },
            rollup: {
                op: 'add',
                args: [OPEN_BALANCE, { op: 'sum', args: [{ path: [{ field: 'amount' }] }] }],
            },
        },
    ],
    filter: { op: 'between', args: [{ path: [{ field: 'day' }] }, { param: 'period' }] },
    sort: [{ col: 'day' }],
    groups: [
        {
            id: 'byTeam',
            col: 'team',
            subtotals: true,
            open: true,
            opening: { label: 'Before the period', cells: { balance: OPEN_BALANCE } },
        },
    ],
}

const BALANCE = 4

describe('spec version 3 in the builder', () => {
    it('keeps every v3 part through the draft and writes version 3', () => {
        const draft = draftFromSpec(V3, META, CATALOG)
        expect(builderSchema(CATALOG).safeParse(draft).success).toBe(true)
        expect(draft.columns[3]).toMatchObject({ kind: 'formula', formula: { kind: 'editable' } })
        expect(draft.columns[BALANCE]?.rollup).toEqual({ kind: 'editable', node: V3.columns[BALANCE]?.rollup })
        expect(draft.groups[0]?.opening).toEqual({
            label: 'Before the period',
            cells: { balance: { kind: 'editable', node: OPEN_BALANCE } },
        })
        const saved = specFromDraft(draft, CATALOG)
        expect(saved.version).toBe(3)
        expect(saved).toEqual(V3)
        expect(JSON.stringify(saved)).toBe(JSON.stringify(V3))
    })

    it('keeps a rollup or an opening cell the editor cannot show read-only and saves it unchanged', () => {
        const odd: SpecNode = { op: 'shout', args: [] }
        const spec: ReportSpec = {
            ...V3,
            columns: V3.columns.map((c, i) => (i === BALANCE ? { ...c, rollup: odd } : c)),
            groups: [{ ...(V3.groups?.[0] ?? { id: 'byTeam', col: 'team' }), opening: { label: 'Before', cells: { amount: odd } } }],
        }
        const draft = draftFromSpec(spec, META, CATALOG)
        expect(draft.columns[BALANCE]?.rollup).toEqual({ kind: 'fixed', node: odd })
        expect(draft.groups[0]?.opening?.cells.amount).toEqual({ kind: 'fixed', node: odd })
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    it('"Start each group with a row" writes the opening with its label and only the cells set', () => {
        const draft = draftFromSpec(V3, META, CATALOG)
        const off = { ...draft, groups: draft.groups.map(g => ({ ...g, opening: null })) }
        expect(specFromDraft(off, CATALOG).groups?.[0]).not.toHaveProperty('opening')
        const bare = openingDraft('Opening')
        expect(bare).toEqual({ label: 'Opening', cells: {} })
        const on = (opening: ReturnType<typeof openingDraft>) => ({
            ...draft,
            groups: draft.groups.map(g => ({ ...g, opening })),
        })
        expect(specFromDraft(on(bare), CATALOG).groups?.[0]?.opening).toEqual({ label: 'Opening' })
        const withCell = setOpeningCell(bare, 'balance', { kind: 'editable', node: OPEN_BALANCE })
        expect(specFromDraft(on(withCell), CATALOG).groups?.[0]?.opening).toEqual({
            label: 'Opening',
            cells: { balance: OPEN_BALANCE },
        })
        expect(setOpeningCell(withCell, 'balance', null)).toEqual(bare)
    })

    it('"A formula" on group and total rows writes the column rollup, and the total takes it out', () => {
        const draft = draftFromSpec(V3, META, CATALOG)
        const sum: SpecNode = { op: 'sum', args: [{ path: [{ field: 'amount' }] }] }
        const withRollup = (rollup: FormulaDraft | null) => ({
            ...draft,
            columns: draft.columns.map((c, i) => (i === 2 ? { ...c, rollup } : c)),
        })
        expect(specFromDraft(withRollup({ kind: 'editable', node: sum }), CATALOG).columns[2]).toEqual({
            id: 'amount',
            label: 'Amount',
            path: [{ field: 'amount' }],
            aggregate: 'sum',
            rollup: sum,
        })
        expect(specFromDraft(withRollup(null), CATALOG).columns[2]).not.toHaveProperty('rollup')
    })

    it('flags a rollup with totals of the rows shown, an empty opening label and an opening cell on the first column', () => {
        const draft = draftFromSpec(V3, META, CATALOG)
        const rowsTotal = {
            ...draft,
            columns: draft.columns.map((c, i) => (i === BALANCE ? { ...c, total: 'rows' as const } : c)),
        }
        expect(draftIssues(rowsTotal, CATALOG)).toContainEqual({
            path: ['columns', BALANCE, 'rollup'],
            message:
                "Totals that add the rows shown cannot also show a formula: choose Recompute from all rows, or the column's total",
        })
        const unlabelled = {
            ...draft,
            groups: draft.groups.map(g => ({ ...g, opening: { label: '', cells: {} } })),
        }
        const parsed = builderSchema(CATALOG).safeParse(unlabelled)
        expect(parsed.success).toBe(false)
        expect(parsed.error?.issues).toContainEqual(
            expect.objectContaining({ path: ['groups', 0, 'opening', 'label'], message: 'Enter a label' })
        )
        const onLabel = {
            ...draft,
            groups: draft.groups.map(g => ({
                ...g,
                opening: setOpeningCell(openingDraft('Opening'), 'team', { kind: 'editable', node: { value: null } }),
            })),
        }
        expect(draftIssues(onLabel, CATALOG)).toContainEqual({
            path: ['groups', 0, 'opening', 'cells', 'team'],
            message: "The opening row's label fills the first column",
        })
    })

    it('keeps a param that only a rollup, an opening cell or a range bound reads', () => {
        const draft = draftFromSpec(V3, META, CATALOG)
        const plain = (columns: typeof draft.columns) => columns.map(c => ({ ...c, rollup: null }))
        const unread = {
            ...draft,
            filter: emptyGroup(),
            columns: plain(draft.columns).filter(c => c.id !== 'balance'),
            groups: draft.groups.map(g => ({ ...g, opening: null })),
        }
        expect(keptParams(unread)).toEqual([])
        const balance = draft.columns[BALANCE]
        if (balance?.kind !== 'formula') throw new Error('V3 has a balance formula column')
        const byRollup = {
            ...unread,
            columns: [
                ...unread.columns,
                { ...balance, formula: { kind: 'editable' as const, node: { value: 0 } } },
            ],
        }
        expect(keptParams(byRollup).map(p => p.id)).toEqual(['period'])
        const byOpening = { ...unread, groups: draft.groups }
        expect(keptParams(byOpening).map(p => p.id)).toEqual(['period'])
        const bound: SpecNode = { param: 'period', bound: 'to' }
        const byBound = {
            ...unread,
            detailLabel: { kind: 'editable' as const, node: bound },
        }
        expect(keptParams(byBound).map(p => p.id)).toEqual(['period'])
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/builder/__tests__/draft.test.ts`
Expected: FAIL. "openingDraft is not a function"; the round trip loses `rollup` and `opening` (`expected { …(5) } to deeply equal { …(6) }`); `keptParams(byRollup)` is `[]`. The earlier tests of the file stay green.

- [ ] **Step 3: Implement the draft fields**

In `draft.ts`, after `formulaDraft`:

```ts
/** "Start each group with a row" (F5): its label and a formula per column it fills. */
export const openingDraftSchema = z.object({
    label: labelSchema,
    cells: z.record(z.string(), formulaDraftSchema),
})
export type OpeningDraft = z.infer<typeof openingDraftSchema>

export function openingDraft(label: string): OpeningDraft {
    return { label, cells: {} }
}

/** The opening row with column `col` showing `formula`, or no value there when null. */
export function setOpeningCell(
    opening: OpeningDraft,
    col: string,
    formula: FormulaDraft | null
): OpeningDraft {
    const kept = Object.entries(opening.cells).filter(([key]) => key !== col)
    const cells = formula ? [...kept, [col, formula] as const] : kept
    return { ...opening, cells: Object.fromEntries(cells) }
}
```

Add to `columnFields`, after `detail`:

```ts
    /** On group, subtotal and total rows: a formula instead of the column's total (F6). */
    rollup: formulaDraftSchema.nullable().optional(),
```

Add to `groupLevelSchema`, after `fill`:

```ts
    /** The row that starts each opened group of this level (F5); none when null or absent. */
    opening: openingDraftSchema.nullable().optional(),
```

- [ ] **Step 4: Implement the reads and writes**

In `columnDraft`, add to `common` after `detail: column.detail ?? 'show',`:

```ts
        ...(column.rollup ? { rollup: formulaDraft(column.rollup, params) } : {}),
```

Add before `groupLevelDraft`:

```ts
function openingFromSpec(
    opening: NonNullable<SpecGroup['opening']>,
    params: readonly ParamDef[]
): OpeningDraft {
    const cells = Object.entries(opening.cells ?? {}).map(
        ([col, node]) => [col, formulaDraft(node, params)] as const
    )
    return { label: opening.label, cells: Object.fromEntries(cells) }
}
```

In `groupLevelDraft`'s returned object, after `fill: {…},`:

```ts
        ...(g.opening ? { opening: openingFromSpec(g.opening, params) } : {}),
```

In `specColumns`, add below its first line (`const ctx = formulaContext(draft, catalog, { kind: 'column', selfKey: '' })`):

```ts
    const levelCtx = formulaContext(draft, catalog, { kind: 'level' })
```

and, inside the map, after `if (column.detail === 'blank') out.detail = 'blank'`:

```ts
        // A rollup is worked out on the level rows (levels.go levelCtx.col).
        if (column.rollup) out.rollup = specFormula(column.rollup, levelCtx)
```

Add before `specGroups`:

```ts
function specOpening(opening: OpeningDraft, ctx: ReportFormulaContext) {
    const cells = Object.entries(opening.cells)
    if (cells.length === 0) return { label: opening.label }
    const written = cells.map(([col, formula]) => [col, specFormula(formula, ctx)] as const)
    return { label: opening.label, cells: Object.fromEntries(written) }
}
```

In `specGroups`' returned object, after `...(g.open ? { open: true } : {}),`:

```ts
            ...(g.opening ? { opening: specOpening(g.opening, ctx) } : {}),
```

In `keptParams`, change the column loop and the group loop:

```ts
    for (const column of draft.columns) {
        if (column.kind === 'formula') node(column.formula.node)
        node(column.rollup?.node)
    }
    for (const group of draft.groups) {
        paramsInGroup(group.having, read)
        if (group.fill.on) paramsInGroup(group.fill.where, read)
        node(group.label?.node)
        for (const cell of Object.values(group.opening?.cells ?? {})) node(cell.node)
    }
```

- [ ] **Step 5: Implement the problems the engine refuses**

Add before `draftIssues` (messages follow the engine refusals of Tasks 5–6, R5, R6):

```ts
const ROLLUP_ROWS =
    "Totals that add the rows shown cannot also show a formula: choose Recompute from all rows, or the column's total"
const ROLLUP_NESTED = 'A child row column has no totals to replace'
const ROLLUP_PIVOT = "A pivot's cell column is totalled by its pivot columns"
const OPENING_PIVOT = 'This level never opens, so it has no opening row'
const OPENING_LABEL_COLUMN = "The opening row's label fills the first column"

// bind.go columns: a rollup replaces a recomputed total of the report's own rows.
function rollupIssues(draft: BuilderDraft): Issue[] {
    return draft.columns.flatMap((column, i): Issue[] => {
        if (!column.rollup) return []
        const path = ['columns', i, 'rollup']
        if (column.total === 'rows') return [{ path, message: ROLLUP_ROWS }]
        if (nestDepth(column) > 0) return [{ path, message: ROLLUP_NESTED }]
        if (draft.pivot.on && draft.pivot.cell === column.id) return [{ path, message: ROLLUP_PIVOT }]
        return []
    })
}

// bind.go groupsAndPivot: an opening row shows under a level that opens, in the
// report's columns, with its label in the first.
function openingIssues(draft: BuilderDraft): Issue[] {
    const first = draft.columns[0]?.id
    const ids = new Set(draft.columns.map(column => column.id))
    return draft.groups.flatMap((group, i): Issue[] => {
        if (!group.opening) return []
        const at = ['groups', i, 'opening']
        if (draft.pivot.on && i === draft.groups.length - 1) return [{ path: at, message: OPENING_PIVOT }]
        return Object.keys(group.opening.cells).flatMap((col): Issue[] => {
            const path = [...at, 'cells', col]
            if (col === first) return [{ path, message: OPENING_LABEL_COLUMN }]
            if (!ids.has(col)) return [{ path, message: `The report has no column "${col}"` }]
            return []
        })
    })
}
```

In `draftIssues`, add `...rollupIssues(draft),` and `...openingIssues(draft),` after `...detailBlankIssues(draft),`.

- [ ] **Step 6: Run the tests to see them pass**

Run: the Step 2 command.
Expected: PASS (every test of `draft.test.ts`).

- [ ] **Step 7: Check the member**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/lib/builder/draft.ts core/reports/lib/builder/__tests__/draft.test.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```
Expected: green.

- [ ] **Step 8: Commit (the driver runs it)**

```bash
git add core/reports/lib/builder/draft.ts core/reports/lib/builder/__tests__/draft.test.ts
git commit -m "feat(core): report builder drafts keep every v3 part and edit opening rows and rollups"
```

---

### Task 18: Builder UI — Group and sort opening row, Columns rollup, Arrange desktop

F5, F6, F10 (builder UI). Facts used: `setLevel`, `LevelFormula`, `GroupHaving` (`components/builder/GroupLevelParts.tsx:40-78,220-248`); `GroupRow` renders the level parts (`components/builder/GroupSortSection.tsx:143-151`); `ColumnRow` and its parts (`components/builder/ColumnsSection.tsx:87-149`, `TotalField` `:209-229`); the width tabs and switch (`components/builder/ArrangeSection.tsx:28-36,355-387`); `startingLayout`, `placeCells` (`lib/table-lines.ts:307-316`); `displayIssues` (`lib/builder/draft.ts:456-469`); `NEW_FORMULA` (`components/builder/parts.tsx:50`). The builder's Desktop tab already exists (R18f); the vitest pins that the desktop width is edited like tablet and phone, so it is green at once (no code is missing for it). The UI parts are driven by Task 13a's builder e2e (`reports-ledger-v3.spec.ts` "the builder counts a lookup on this row, …"): it sets a column's "On group and total rows show" to a formula, turns on "Start each group with a row", fills "Row label", adds an opening cell formula, saves, and reads the opening row and the rollup cell in the view. Their draft writes are pinned by Task 17's vitest. Not driven by e2e: the opening cell's Remove button and the Arrange caption (a static text); the reviewer reads them.

**Files:**
- Modify: `core/reports/components/builder/GroupLevelParts.tsx` (`GroupOpening`, `OpeningCell`, `TextAction`)
- Modify: `core/reports/components/builder/GroupSortSection.tsx` (render `GroupOpening`)
- Modify: `core/reports/components/builder/ColumnsSection.tsx` (`ColumnRollup`)
- Modify: `core/reports/components/builder/ArrangeSection.tsx` (caption of the automatic fit)
- Test: `core/reports/lib/builder/__tests__/draft.test.ts`

**Interfaces:**
- Consumes: Task 17 `OpeningDraft`, `openingDraft`, `setOpeningCell`, `ColumnDraft.rollup`, `GroupLevelDraft.opening`.
- Produces: `GroupOpening({ builder, groups, index })` (switch test id `report-builder-group-N-opening`, label `report-builder-group-N-opening-label`, cell rows `report-builder-group-N-opening-cell-<col>` with `-add`, `-remove`, `-formula`); `ColumnRollup` (pick menu test id `report-builder-column-N-rollup`, items `-total` and `-custom`, formula `report-builder-column-N-rollup-formula`; the formula item is `custom`, not `formula`, because a `PickMenu` row's id is `${testID}-${item.id}` and `-formula` is the editor's id); Arrange caption test id `report-builder-arrange-auto`.

- [ ] **Step 1: Write the test that pins the desktop arrangement**

Append to `core/reports/lib/builder/__tests__/draft.test.ts` (add `displayIssues` to the `../draft` import; add `import { placeCells, startingLayout } from '../../table-lines'`):

```ts
describe('the desktop arrangement in the builder', () => {
    it('starts a desktop layout that places every column', () => {
        const layout = startingLayout('desktop', V3.columns)
        expect(layout.width).toBe('desktop')
        expect(layout.cells.map(c => c.col)).toEqual(V3.columns.map(c => c.id))
        expect(placeCells(layout.tracks, layout.cells)).not.toHaveProperty('problem')
    })

    it('keeps a desktop layout through the draft as it keeps a tablet one', () => {
        for (const width of ['desktop', 'tablet'] as const) {
            const spec: ReportSpec = { ...V3, display: { layouts: [startingLayout(width, V3.columns)] } }
            expect(specFromDraft(draftFromSpec(spec, META, CATALOG), CATALOG)).toEqual(spec)
        }
    })

    it('names a desktop problem at its layout as it does for a tablet', () => {
        for (const width of ['desktop', 'tablet'] as const) {
            const layout = { width, tracks: 3, cells: [{ col: 'nosuch', line: 1 }] }
            expect(displayIssues({ layouts: [layout] }, ['team'])).toEqual([
                {
                    path: ['display', 'layouts', 0, 'cells', 0, 'col'],
                    message: 'the report has no column "nosuch"',
                },
            ])
        }
    })
})
```

- [ ] **Step 2: Run the test**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/builder/__tests__/draft.test.ts`
Expected: PASS at once (the desktop width already edits like the others, R18f). The test-writer shows it red by a mutation: change `WIDTHS` / `displayIssues` so desktop is skipped (for example `display.layouts.filter(l => l.width !== 'desktop')` in `displayIssues`), see "expected [] to deeply equal [ { … } ]", and revert.

- [ ] **Step 3: Add the opening row part (`GroupLevelParts.tsx`)**

Add imports: `TextInput` (with `useWatch`) from `@tinycld/core/ui/form`; `Button, ButtonText` from `@tinycld/core/ui/button`; `type ColumnDraft, type OpeningDraft, openingDraft, setOpeningCell` from `../../lib/builder/draft`; `NEW_FORMULA` from `./parts`. Add at the end of the file:

```tsx
interface TextActionProps {
    isVisible: boolean
    text: string
    label: string
    onPress: () => void
    testID: string
}

function TextAction({ isVisible, text, label, onPress, testID }: TextActionProps) {
    if (!isVisible) return null
    return (
        <Button variant="outline" size="sm" onPress={onPress} accessibilityLabel={label} testID={testID}>
            <ButtonText>{text}</ButtonText>
        </Button>
    )
}

interface OpeningCellProps {
    builder: ReportBuilder
    index: number
    column: ColumnDraft
    opening: OpeningDraft
    onChange: (opening: OpeningDraft) => void
    testID: string
}

// One column of the opening row: no value, or a formula worked out on the
// level's rows (levels.go levelCtx: totals, group fields, lookups, asked values).
function OpeningCell({ builder, index, column, opening, onChange, testID }: OpeningCellProps) {
    const formula = opening.cells[column.id] ?? null
    const set = (next: FormulaDraft | null) => onChange(setOpeningCell(opening, column.id, next))
    const caption = formula ? `${column.label}:` : `${column.label}: No value`
    return (
        <View testID={testID} className="gap-1">
            <View className="flex-row items-center gap-2">
                <Text numberOfLines={1} className="min-w-0 flex-1 text-sm text-foreground">
                    {caption}
                </Text>
                <TextAction
                    isVisible={formula === null}
                    text="Add a value"
                    label={`Add a value for ${column.label}`}
                    onPress={() => set(NEW_FORMULA)}
                    testID={`${testID}-add`}
                />
                <TextAction
                    isVisible={formula !== null}
                    text="Remove"
                    label={`Remove the value for ${column.label}`}
                    onPress={() => set(null)}
                    testID={`${testID}-remove`}
                />
            </View>
            <LevelFormula
                isVisible={formula !== null}
                builder={builder}
                place={{ kind: 'level', index }}
                formula={formula}
                onChange={set}
                testID={`${testID}-formula`}
            />
        </View>
    )
}

interface OpeningFieldsProps {
    isVisible: boolean
    builder: ReportBuilder
    index: number
    testID: string
    cells: ReactNode
}

function OpeningFields({ isVisible, builder, index, testID, cells }: OpeningFieldsProps) {
    if (!isVisible) return null
    return (
        <View className="gap-1.5">
            <TextInput
                name={`groups.${index}.opening.label`}
                control={builder.form.control}
                label="Row label"
                testID={`${testID}-label`}
                maxLength={200}
            />
            {cells}
        </View>
    )
}

/**
 * "Start each group with a row" (F5): a row first under each opened group of the
 * level, with its own label and a value in any column after the first (the label
 * fills the first). It shows also under a group with no rows of its own.
 */
export function GroupOpening({ builder, groups, index }: Omit<LevelPartProps, 'target'>) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const group = groups[index]
    if (!group) return null
    const testID = `report-builder-group-${index}-opening`
    const opening = group.opening ?? null
    const setOpening = (next: OpeningDraft | null) => setLevel(builder, groups, index, { opening: next })
    const cells = opening
        ? columns.slice(1).map(column => (
              <OpeningCell
                  key={column.id}
                  builder={builder}
                  index={index}
                  column={column}
                  opening={opening}
                  onChange={setOpening}
                  testID={`${testID}-cell-${column.id}`}
              />
          ))
        : null
    return (
        <View className="gap-1.5">
            <View className="flex-row items-center gap-2">
                <Switch
                    value={opening !== null}
                    onValueChange={on => setOpening(on ? openingDraft('Opening') : null)}
                    accessibilityLabel="Start each group with a row"
                    testID={testID}
                />
                <Text className="flex-1 text-sm text-foreground">Start each group with a row</Text>
            </View>
            <OpeningFields
                isVisible={opening !== null}
                builder={builder}
                index={index}
                testID={testID}
                cells={cells}
            />
        </View>
    )
}
```

(Add `import type { ReactNode } from 'react'` to the file's imports.)

In `GroupSortSection.tsx`, import `GroupOpening` with the other level parts and render it in `GroupRow` after `<GroupHaving builder={builder} groups={groups} index={index} />`:

```tsx
            <GroupOpening builder={builder} groups={groups} index={index} />
```

- [ ] **Step 4: Add the rollup part (`ColumnsSection.tsx`)**

Import `type PickItem, PickMenu` from `@tinycld/core/ui/pick-menu` and `LevelFormula` from `./GroupLevelParts`. Add after `TotalField`:

```tsx
const COLUMN_TOTAL = "The column's total"
const ROLLUP_FORMULA = 'A formula'

/**
 * "On group and total rows show" (F6): the column's total, or a formula worked out
 * on those rows instead. Not with "Add the rows shown" (they cannot go together); a
 * formula set before that choice stays in view, with its refusal, so it can be taken
 * out.
 */
function ColumnRollup({ builder, columns, index }: Omit<ColumnPartProps, 'source'>) {
    const column = columns[index]
    if (!column) return null
    const rollup = column.rollup ?? null
    if (column.total === 'rows' && rollup === null) return null
    const testID = `report-builder-column-${index}-rollup`
    const setRollup = (next: FormulaDraft | null) =>
        builder.setColumns(columns.map((c, i) => (i === index ? { ...c, rollup: next } : c)))
    const items: PickItem[] = [
        {
            id: 'total',
            label: COLUMN_TOTAL,
            isSelected: rollup === null,
            onSelect: () => setRollup(null),
        },
        {
            // Not 'formula': the row's test id would be the editor's (`${testID}-formula`).
            id: 'custom',
            label: ROLLUP_FORMULA,
            isSelected: rollup !== null,
            onSelect: () => {
                if (rollup === null) setRollup(NEW_FORMULA)
            },
        },
    ]
    const shown = rollup === null ? COLUMN_TOTAL : ROLLUP_FORMULA
    return (
        <View className="gap-1">
            <View className="flex-row items-center gap-2">
                <Text className="text-xs text-muted-foreground">On group and total rows show</Text>
                <PickMenu
                    label={shown}
                    purpose={`On group and total rows of ${column.label} show`}
                    items={items}
                    testID={testID}
                />
            </View>
            <LevelFormula
                isVisible={rollup !== null}
                builder={builder}
                place={{ kind: 'level' }}
                formula={rollup}
                onChange={setRollup}
                testID={`${testID}-formula`}
            />
        </View>
    )
}
```

In `ColumnRow`, render it after `<TotalField … />`:

```tsx
            <ColumnRollup builder={builder} columns={columns} index={index} />
```

- [ ] **Step 5: Caption the automatic fit (`ArrangeSection.tsx`)**

Add before `ArrangeSection`:

```tsx
// F10: with no arrangement, a width fits its columns on one line, cutting long
// text short, and wraps only when they still do not fit.
function AutoCaption({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <Text testID="report-builder-arrange-auto" className="text-xs text-muted-foreground">
            Off: the columns fit on one line, with long text cut short, or wrap when they do not fit.
        </Text>
    )
}
```

In `ArrangeSection`, add after the switch row:

```tsx
            <AutoCaption isVisible={arrange.layout === undefined} />
```

- [ ] **Step 6: Check the member**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome format --write core/reports/components/builder/GroupLevelParts.tsx core/reports/components/builder/GroupSortSection.tsx core/reports/components/builder/ColumnsSection.tsx core/reports/components/builder/ArrangeSection.tsx core/reports/lib/builder/__tests__/draft.test.ts
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```
Expected: green. Then run the builder e2e that drive these sections, one file at a time (ports first: `lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`):
`cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-groups-v2.spec.ts`, then the same with `tests/e2e/reports-formulas-v2.spec.ts`.
Expected: both pass (no existing test id moved; the new parts add controls only).

Then run the builder e2e Task 13a wrote red for this task: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-ledger-v3.spec.ts -g "the builder counts a lookup"`.
Before this task it failed at `report-builder-column-2-rollup` (element not found, Task 16a's Step 4). Expected now: PASS — the opening row "Brought forward" is first under the team with Amount 17.00 (`sum(amount)` of the team), the team's Total row shows 10.00 (the largest sale, not the 17.00 sum), and the count column reads 2, 2, 1.

- [ ] **Step 7: Commit (the driver runs it)**

```bash
git add core/reports/components/builder/GroupLevelParts.tsx core/reports/components/builder/GroupSortSection.tsx core/reports/components/builder/ColumnsSection.tsx core/reports/components/builder/ArrangeSection.tsx core/reports/lib/builder/__tests__/draft.test.ts
git commit -m "feat(core): report builder starts groups with a row, shows a formula on total rows, and explains the automatic fit"
```

---

### Task 19: Help and package docs

F1–F11 (help, docs). Facts used: `core/help/reports.md` sections (Totals `:44`, Sort `:67`, Look up a value `:71`, Formulas `:99`, Narrow screens `:103`, Run a report `:107`); `docs/packages.md` limits list (`:1244-1248`), "What a report returns" (`:1267-1288`), "Spec version 2" (`:1290-1310`).

**Files:**
- Modify: `core/help/reports.md`
- Modify: `docs/packages.md`

**Interfaces:**
- Consumes: the names and words of Tasks 1–18 (R16 builder wording; R7 viewer sort; R9 grouped pages).
- Produces: help sections "Opening rows", "Sort a report", "When columns do not fit"; docs "Spec version 3".

- [ ] **Step 1: Edit the help topic**

In `core/help/reports.md` frontmatter, replace the `tags` line:

```yaml
tags: [reports, report builder, share report, publish, filter, group, pivot, formula, totals, groups, lookup, formulas, phone, sort, opening row, all dates]
```

Append to `## Totals` (after the Debit example paragraph):

```markdown
To show something other than the total on group and total rows, open the column in **Columns** and set **On group and total rows show** to **A formula**. Build the formula from totals, group fields, lookups and asked values. Opened rows still show the column's own value. This choice does not show while **Total** is **Add the rows shown**. A formula that reads a group field has no value on the Total row, because the Total row is not inside a group.
```

Replace the paragraph under `## Look up a value`:

```markdown
To read a value from another kind of record, add a formula column and pick **Look up**. Choose the **Source**, a condition under **Where**, and the **Value**. The report reads only records you may see. Under **Which match**, choose what the lookup gives:

- **Smallest** or **Largest**: one value of the records that match.
- **Add up**: the sum of the values of all records that match. With no match, the cell is empty. If the records that match have amounts in two currencies, the cell is empty and the report shows a note.
- **Count**: the number of records that match and have a value. With no match, it is 0.

Inside **Where** or **Value**, pick **This row's field** to compare with a field of the report's own row. For example, "Day = This row's Day" finds the records of the same day as each row. On group and total rows, **This row's field** has no value.
```

Add a new section after `## Opened rows`:

```markdown
## Opening rows

To start each group with a row of its own, open **Group and sort**, find the level, and turn on **Start each group with a row**. Enter a **Row label**, for example "Beginning balance". For each column after the first, select **Add a value** and build a formula, or leave it at **No value**. The row shows first under each group that you open. It also shows under a group that has no rows of its own, if the level shows every record with **Show every …, even with no rows**. A closed group and a group that **Show only groups where…** hides have no opening row.
```

Replace `## Sort` with:

```markdown
## Sort

To sort, open **Group and sort**, select **Add sort**, and pick the field. To sort groups by a field of their record, pick it by name (for example **Team › Name**). To sort by anything else, pick **Formula…**. A sort formula that reads a row's fields sorts the detail rows only. Group levels skip it.

## Sort a report while you read it

To sort the rows by a column, select the column's heading. The first press sorts from low to high, the next press from high to low, and the third press puts the report's own order back. An arrow beside the heading shows the order. The rows sort inside their groups; the groups keep their order. A running value, such as a running balance, keeps the report's own order, so its figures do not change. A column that shows a running value or a percent has no sort control. **Copy link** keeps the sort. If a link sorts by a column that the report no longer has, the report shows why and a **Clear sort** button; select it to show the report in its own order.
```

Replace `## Formulas`' paragraph:

```markdown
A formula is built from parts you pick from menus: **Number**, **Text**, **Column**, **Field**, **This row's field** (inside a lookup), **Group field** and **Asked value**; **Range start or end** (the first or last day of a period you ask for, such as "Period start"); **Add**, **Subtract**, **Multiply**, **Divide** and **Round**; totals such as **Sum of** and running values such as **Running sum**; comparisons, **All of**, **Any of**, **Not** and **Is empty**; **If** and **First match**; text and date parts; and **Look up**. When the period is **All dates**, "Period start" and "Period end" have no value, so a comparison with them matches no record.
```

Replace `## Narrow screens` and its paragraph with:

```markdown
## When columns do not fit

A report never scrolls sideways. When its columns do not fit on one line, the text columns get narrower first: the first column, then the widest text columns. Long text ends in "…". Numbers, amounts and dates keep their width. When the columns still do not fit, each row wraps: the label on the first line and the figures below it, each under its heading.

To choose where each column goes at a width, open **Arrange**, pick **Phone**, **Tablet** or **Desktop**, and turn on **Arrange columns for this width**. An arrangement always applies at its own width. A width with no arrangement uses the next narrower one when the columns do not fit. The heading row and the open group rows stay at the top while you scroll. A long report loads more rows as you scroll, also when it has groups.
```

In `## Run a report`, replace the first bullet:

```markdown
- The chips above the report set its period and filters; **Add filter** adds a chip
  for a filter you can set while you read. In the period chip, **All dates** shows
  every row, also a row with no date. A link copied with **Copy link** opens the
  report with the same choices.
```

- [ ] **Step 2: Edit the package docs**

In `docs/packages.md`, replace the cursor bullet (`:1244-1248`):

```markdown
- A page cursor is tied to the report's sort keys, its viewer sort
  (`options.sort`), its group levels and the value types. A cursor from a
  report that sorts or groups differently is refused. A detail or grouped
  report pages: a page holds 200 detail rows by default, and at most 1,000
  (`options.limit`). A grouped page also holds the group, opening and subtotal
  rows between its detail rows; every page ends with the Total. The group
  rows themselves are read whole, at most 5,000.
```

Append to the "What a report returns" list (after the unset-parameter bullet):

```markdown
- A row's `kind` is `detail`, `group`, `opening`, `subtotal`, `computed` or
  `total`. An `opening` row (id `opening:<group key JSON>`) is the first row
  under an opened group whose level has `opening`; it carries only the cells
  its `cells` name, and its `section` is the group row.
- `options.sort` (`[{ col, dir }]`, at most 10) orders detail rows inside their
  groups before the spec's own sort keys; group order and running values keep
  the spec's order. A `ResultColumn` with `sortable: true` accepts it. A sort
  on a running value or percent, a child row column, an unknown column or a
  pivot report is refused under `options.sort`.
- A grouped result pages (`page.next`); running values continue on the next
  page. A client merges pages by row id and keeps only the last page's Total.
- A `lookup` with `pick: "sum"` over amounts in more than one currency gives no
  value and the notice `mixedCurrency` naming its columns.
```

Replace the lookup row of the v2 table (`:1308`):

```markdown
| operator `lookup` | `{ op: "lookup", source, where?, value, pick? }`: from the rows of another source that the reader may see, one value (`min` by default, or `max`), their sum (`sum`: numbers or amounts; no match is empty), or the count of rows with a value (`count`: no match is 0). |
```

Change the v2 heading paragraph's first sentence to "**Spec version 2.** Version 2 added these fields; a version 2 spec loads unchanged as version 3." and add after the v2 table:

```markdown
**Spec version 3.** `reports.CurrentVersion` is 3. Every new field is
optional, and a stored version 1 or 2 spec loads unchanged with no upgrade
notice (the step from 2 to 3 changes nothing). A version 3 spec sent to an
older server is refused by its version.

| Field | What it does |
|---|---|
| formula leaf `{ outer: [path] }` | Inside a lookup's `where` or `value`: the report row's value of a to-one path of the base source (a nested lookup still reads the report's row). Blank on group, subtotal and total rows. Refused outside a lookup and on a to-many path. |
| formula leaf `{ param, bound: "from" \| "to" }` | The first or last day of a date range param, as a `Date`. Blank (not unset) on an unset range or on All dates, so a comparison with it matches no row. |
| range preset `allDates` | A date range with no bounds: `between` on it matches every row, a row with a blank day included. The resolved param has no `from` or `to`. |
| `groups[].opening` | `{ label, cells? }`: the opening row of each opened group of the level. Each cell is a formula on the level (totals, records of this and outer groups, lookups, params) whose type and currency are its column's; not on the first column; not on a pivot's last level. A level with `opening` lets a group with no base rows open (a fill group included). |
| `columns[].rollup` | A formula shown on group, subtotal and total rows (and layout sections) in place of the column's total; detail rows keep the column's value. Blank where it reads a value the row does not have (a group's record on the Total). Refused with `total: "rows"`, on a child row column and on a pivot's cell column. |
| run `options.sort` | See "What a report returns". |
```

- [ ] **Step 3: Regenerate and check**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH pnpm run packages:generate
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
```
Expected: the generator lists the core `reports` help topic with no error; the check is green (its help frontmatter test parses the topic). Read the new text once against the shipped behaviour of Tasks 1–18: each control name in bold matches its on-screen label (R16).

- [ ] **Step 4: Commit (the driver runs it)**

```bash
git add core/help/reports.md docs/packages.md
git commit -m "docs(core): report help and package docs for spec version 3"
```

---

### Task 20: Run the v3 e2e spec with every task in

F1–F8, F10, F11 (e2e). The stub report `salesstub.team_balance` and `tests/e2e/reports-ledger-v3.spec.ts` were written in Task 13a, shown red there, and turned green by Tasks 14, 15, 16a and 18. This task runs the whole file once with Tasks 1–19 in, before the re-runs of Task 21. A failure here is fixed in the file of the task that owns the behaviour, never in the test.

**Files:** none (a fix goes into the owning task's file, which the driver commits).

**Interfaces:**
- Consumes: Task 13a's spec and stub report; everything Tasks 1–19 produce.
- Produces: nothing new.

- [ ] **Step 1: Run the spec**

Check the ports first: `lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'` (kill a leftover server only if it was started from `/Users/nas/tmp/tinycld/books-purchases`).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-ledger-v3.spec.ts`
Expected: PASS, 8 tests. If the hub row "Stub team balance" is missing, search `tmp/e2e-serve-7210.log` for "shipped reports not reconciled": the message names the spec part the engine refused; fix the engine task that refuses a valid part. Any other failure is diagnosed at its root and fixed in its owning task's file (Tasks 2–8 engine, 13–15 view, 16–18 builder); no re-run, no bumped timeout, no edited assertion.

- [ ] **Step 2: Commit (the driver runs it)**

Nothing to commit when Step 1 passes. A fix made in Step 1 is committed with the files it touched and a message naming the defect.

---

### Task 21: Re-run the existing report e2e under F10

F10 (e2e). `reports-wrap.spec.ts` was changed to F10 in Task 13a (the 1100 desktop test narrows instead of scrolling sideways; the phone blank-slot test has a fifth column so it still wraps under R21) and turned green in Task 14. F10 and R21 change how every report table fits at every width, phones with few columns included (a phone report whose narrowed line fits in 374 pt now draws one line where v2 wrapped). So every other report spec that draws a table is re-run here, one file at a time, with Tasks 1–20 in. Task 13a computed the phone line of every table in these files: none asserts a wrap that F10 now fits on one line, so no test here changes. A failure is diagnosed at its root and fixed in the file of the task that owns the behaviour (Tasks 13–15), never in the test.

**Files:** none.

**Interfaces:**
- Consumes: Tasks 13–15 (one narrowed line, no sideways scroller, `numberOfLines={1}` cells, figure widths 128, heading sort controls); Task 13a's `reports-wrap.spec.ts`.
- Produces: nothing new.

- [ ] **Step 1: Run the changed file**

Check the ports first (`lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'`).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-wrap.spec.ts`
Expected: PASS, the whole file (the `:187` desktop one-line test and the phone wrap tests `:147`, `:504`, `:553` included; review 2 computed that each phone report there still does not fit one narrowed line at 374: `wideReport` 80 + 3 × 108 = 404, the blank-slot report 456).

- [ ] **Step 2: Re-run the other report specs, one file at a time**

Run each, in turn, with the same prefix (`cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- <file>`):
1. `tests/e2e/reports-view.spec.ts` (the title-ellipsis helper at `:111` measures the title, not the table, R18e)
2. `tests/e2e/reports-source-params.spec.ts` (`:277` phone, no sideways scroll; it also opens the shipped v2 "Stub team totals", Review Focus 1)
3. `tests/e2e/reports-scroll-anchor.spec.ts` (`:448` phone, 512 wide narrowed: still wraps)
4. `tests/e2e/reports-groups-v2.spec.ts`
5. `tests/e2e/reports-filter.spec.ts` (`:605` phone, 404 narrowed: still wraps)
6. `tests/e2e/reports-create.spec.ts` (`:144` phone, `customer` only, 80 narrowed: one line, as in v2)
7. `tests/e2e/reports-formula-layout.spec.ts` (`:192` phone, 268 narrowed: one line, as in v2)
8. `tests/e2e/reports-formulas-v2.spec.ts` (`:192` phone, 160 narrowed: one line, as in v2)
9. `tests/e2e/reports-group-pivot.spec.ts` (`:201` phone, 484 narrowed: still wraps)
10. `tests/e2e/reports-share.spec.ts` (`:172` no table; `:280` phone, 188 narrowed: one line, as in v2)

Expected: each PASS unchanged (widths computed in Task 13a). A failure is handled as the task's opening paragraph says.

- [ ] **Step 3: Commit (the driver runs it)**

Nothing to commit when every file passes. A fix made in Step 1 or 2 goes into its owning task's file and is committed with a message naming the defect.

---

### Task 22: LAST — a fictional ledger on `zoo_*` with the General Ledger's spec shape (Go)

F1–F8 together (engine acceptance; process rule from the p5-trial-balance retro). Facts used: `newZoo`, `z.rec`, `z.save`, `rule` (`core/server/reports/zoo_test.go:38-265`); `z.run`, `rowsOf` (`run_test.go:16-60`); `Load`, `Validate(app, spec)`; row kinds `KindGroup`, `KindDetail`, `KindSubtotal`, `KindTotal` (`run.go:107-114`) and Task 6's `KindOpening`; a detail row's `Section` is its group row's id, a subtotal's too (`assemble.go:311,327,341`); money cells are `int64` cents; a money field names a per-row currency field with `Currency: "<field key>"` (as `zoo.feedings.cost`, `zoo_test.go:186`), so debit and credit are read in each line's own currency, as Books `books.journal_lines.debit/credit` are (`books/server/report_sources.go:77-78`), and `opening`, `balance`, `rollup` and the opening cell combine through Task 2b's rule (R24); `add` carries a blank operand (`formula.go:558-577`); a source with no `FiscalStartMonth` starts its fiscal year in January. The spec follows Books spec §5 (`books/docs/specs/general-ledger-design.md:95-118`) with fictional names: `opening` with the retained-earnings fold, `byNormal`, `runningSum`, `rollup`, the `opening` row, `having`, `fill`, the Split column (`count` and `min` lookups with `outer`), `detail.label`, the chart sort.

The rollup here is `byNormal(add(opening, coalesce(sum(net), 0)))`: with spec §5's `add(opening, sum(net))`, an account with no line in the period (its `sum(net)` blank) shows a blank ending balance on its Total row, since `add` carries a blank. The Books plan must make the same change (reported back).

The time bound (R24 cost 2). On a detail row the opening lookup's code is a correlated subquery that the guard repeats about 11 times, so `TestABalanceShapedReportOverTwoThousandLinesRunsInTime` adds 1,000 two-line entries (2,000 lines: 1,000 before July, 1,000 in it, over the 7 accounts that take lines) and requires one run of July to take under 4 s by the Go test clock (`time.Since`). `zoo_ledger_lines` gets the two indexes a per-line source of this shape declares (Books declares `(account, date)` and `(entry, position)`, `books/pb-migrations/2100000002_create_books_ledger.js:263-264`), so the bound measures the engine, not a missing index. How the budget was estimated: a `sqlite3` model of the detail statement on the same data (the guarded balance with the opening lookup's value, count and currency subqueries as Task 2b writes them, its `mixed` and currency cells, and the Split count lookup, over the 1,000 lines in the period) ran in 0.36 s on the planning machine (1.8 s without the indexes, at 20 accounts). The engine's run adds the group pass (8 groups, their opening and having lookups), the `w` CTE and the label and sort columns: about 2×, so about 0.7 s. A 5× margin gives 3.5 s, rounded up to 4 s; the "about 2×" is an estimate, not a measure, and the margin also allows for a CI runner about 2× slower than the planning machine with other packages' tests running (`go test -tags no_ui ./...`, `.github/workflows/ci.yml:152`). Step 2 records the measured time; the runner compares it with the 0.7 s estimate before the PR, and the budget is reopened only with that evidence. A run over the budget is an engine defect (the guard repeats more than R24 counts), fixed at its source in Task 2b's code, never by a higher budget; if no fix is in scope, stop and report.

The ledger (amounts in cents, every line posted unless marked):

| entry | date | lines (account debit/credit) |
|---|---|---|
| J0 | 2025-12-15 | 1100 Bank debit 2000; 4000 Sales credit 2000 |
| J1 | 2026-07-01 | 1000 Cash credit 10000; 5000 Rent debit 10000 |
| J2 | 2026-07-02 | 1000 Cash debit 3000; 2000 Loan credit 4500; 4000 Sales debit 1500 |
| J3 (draft) | 2026-07-03 | 1000 Cash debit 99999; 5000 Rent credit 99999 (`posted` false) |

Accounts also: 1200 Petty cash (asset, active, no lines), 1300 Idle (asset, inactive, no lines), 3900 Retained earnings (equity, `detail_type` retained_earnings).

**Files:**
- Create: `core/server/reports/balance_shape_test.go`

**Interfaces:**
- Consumes: every engine part of Tasks 1–8, Task 2b's currency-field rule included (R24).
- Produces: nothing new.

- [ ] **Step 1: Write the test**

Create `core/server/reports/balance_shape_test.go`:

```go
package reports

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// A fictional balance report with the spec shape the checkpoint brief names,
// run against the whole engine: a chart of accounts, entries and their lines,
// each line a debit or a credit on one account in the line's own currency (a
// per-row currency field): every line is USD.
//
//	zoo_ledger_accounts  number, full_name, type, detail_type, inactive
//	zoo_ledger_entries   number, memo
//	zoo_ledger_lines     entry, account, date (a text day), debit, credit,
//	                     currency, posted, position
func withBalanceZoo(t *testing.T, z *zoo) {
	t.Helper()
	signedIn := rule(`@request.auth.id != ""`)
	accounts := core.NewBaseCollection("zoo_ledger_accounts")
	accounts.ListRule, accounts.ViewRule = signedIn, signedIn
	accounts.Fields.Add(
		&core.TextField{Name: "number"},
		&core.TextField{Name: "full_name"},
		&core.SelectField{Name: "type", Values: []string{"asset", "liability", "equity", "income", "expense"}, MaxSelect: 1},
		&core.TextField{Name: "detail_type"},
		&core.BoolField{Name: "inactive"},
	)
	z.save(t, accounts)
	entries := core.NewBaseCollection("zoo_ledger_entries")
	entries.ListRule, entries.ViewRule = signedIn, signedIn
	entries.Fields.Add(&core.TextField{Name: "number"}, &core.TextField{Name: "memo"})
	z.save(t, entries)
	lines := core.NewBaseCollection("zoo_ledger_lines")
	lines.ListRule, lines.ViewRule = signedIn, signedIn
	lines.Fields.Add(
		&core.RelationField{Name: "entry", CollectionId: entries.Id, MaxSelect: 1},
		&core.RelationField{Name: "account", CollectionId: accounts.Id, MaxSelect: 1},
		&core.TextField{Name: "date"},
		&core.NumberField{Name: "debit"},
		&core.NumberField{Name: "credit"},
		&core.TextField{Name: "currency"},
		&core.BoolField{Name: "posted"},
		&core.NumberField{Name: "position"},
	)
	// The indexes a per-line source of this shape declares, so the time test
	// measures the engine, not a full scan per looked-up row.
	lines.AddIndex("idx_zoo_ledger_lines_account_date", false, "account, date", "")
	lines.AddIndex("idx_zoo_ledger_lines_entry", false, "entry, position", "")
	z.save(t, lines)

	for _, a := range []struct{ key, number, name, typ, detail string; inactive bool }{
		{"cash", "1000", "Cash", "asset", "", false},
		{"bank", "1100", "Bank", "asset", "", false},
		{"petty", "1200", "Petty cash", "asset", "", false},
		{"idle", "1300", "Idle", "asset", "", true},
		{"loan", "2000", "Loan", "liability", "", false},
		{"re", "3900", "Retained earnings", "equity", "retained_earnings", false},
		{"sales", "4000", "Sales", "income", "", false},
		{"rent", "5000", "Rent", "expense", "", false},
	} {
		z.rec(t, a.key, "zoo_ledger_accounts", map[string]any{
			"number": a.number, "full_name": a.name, "type": a.typ, "detail_type": a.detail, "inactive": a.inactive,
		})
	}
	for _, e := range []string{"J0", "J1", "J2", "J3"} {
		z.rec(t, e, "zoo_ledger_entries", map[string]any{"number": e})
	}
	for _, l := range []struct {
		key, entry, account, date string
		debit, credit, position  int
		posted                   bool
	}{
		{"j0a", "J0", "bank", "2025-12-15", 2000, 0, 1, true},
		{"j0b", "J0", "sales", "2025-12-15", 0, 2000, 2, true},
		{"j1a", "J1", "cash", "2026-07-01", 0, 10000, 1, true},
		{"j1b", "J1", "rent", "2026-07-01", 10000, 0, 2, true},
		{"j2a", "J2", "cash", "2026-07-02", 3000, 0, 1, true},
		{"j2b", "J2", "loan", "2026-07-02", 0, 4500, 2, true},
		{"j2c", "J2", "sales", "2026-07-02", 1500, 0, 3, true},
		{"j3a", "J3", "cash", "2026-07-03", 99999, 0, 1, false},
		{"j3b", "J3", "rent", "2026-07-03", 0, 99999, 2, false},
	} {
		z.rec(t, l.key, "zoo_ledger_lines", map[string]any{
			"entry": z.id(l.entry), "account": z.id(l.account), "date": l.date,
			"debit": l.debit, "credit": l.credit, "currency": "USD", "position": l.position, "posted": l.posted,
		})
	}

	RegisterSource(Source{
		Key: "zoo.ledger_accounts", Collection: "zoo_ledger_accounts", Label: "Ledger accounts",
		Fields: []Field{
			{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax},
			{Key: "full_name", Label: "Full name", Type: Text, Aggregates: MinMax},
			{Key: "type", Label: "Type", Type: Select, Options: []Option{
				{Value: "asset", Label: "Asset"}, {Value: "liability", Label: "Liability"}, {Value: "equity", Label: "Equity"},
				{Value: "income", Label: "Income"}, {Value: "expense", Label: "Expense"},
			}},
			{Key: "detail_type", Label: "Detail type", Type: Text},
			{Key: "inactive", Label: "Inactive", Type: Bool},
		},
		Links: []Link{{Key: "lines", Label: "Lines", Target: "zoo.ledger_lines", Cardinality: ToMany, BackField: "account"}},
	})
	RegisterSource(Source{
		Key: "zoo.ledger_entries", Collection: "zoo_ledger_entries", Label: "Ledger entries",
		Fields: []Field{
			{Key: "number", Label: "Number", Type: Text, Aggregates: MinMax},
			{Key: "memo", Label: "Memo", Type: Text},
		},
	})
	RegisterSource(Source{
		Key: "zoo.ledger_lines", Collection: "zoo_ledger_lines", Label: "Ledger lines",
		Fields: []Field{
			{Key: "date", Label: "Date", Type: Date, TextDay: true, Aggregates: MinMax},
			{Key: "debit", Label: "Debit", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "credit", Label: "Credit", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "posted", Label: "Posted", Type: Bool},
			{Key: "position", Label: "Position", Type: Number, Aggregates: SumAll},
		},
		Links: []Link{
			{Key: "entry", Label: "Entry", Target: "zoo.ledger_entries", Cardinality: ToOne, Field: "entry"},
			{Key: "account", Label: "Account", Target: "zoo.ledger_accounts", Cardinality: ToOne, Field: "account"},
		},
	})
}

// The spec's named pieces, inlined as §5 inlines them. FROM is the period's
// first day; NET a line's debit less credit; PL the profit and loss types.
const (
	glFrom = `{"param":"period","bound":"from"}`
	glNet  = `{"op":"sub","args":[{"path":[{"field":"debit"}]},{"path":[{"field":"credit"}]}]}`
	glPL   = `{"value":["income","expense"]}`
	// opening: the account's posted net before the period (a P&L account only
	// within the period's fiscal year), plus, on retained earnings, every P&L
	// net of the fiscal years before it.
	glOpening = `{"op":"add","args":[
		{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_lines","where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"account"}]},{"record":"byChartRow","path":[{"field":"id"}]}]},
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"lt","args":[{"path":[{"field":"date"}]},FROM]},
			{"op":"or","args":[
				{"op":"not","args":[{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},PL]}]},
				{"op":"gte","args":[{"op":"fiscalYear","args":[{"path":[{"field":"date"}]}]},{"op":"fiscalYear","args":[FROM]}]}]}]},
			"value":NET,"pick":"sum"},{"value":0}]},
		{"op":"if","args":[
			{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"detail_type"}]},{"value":"retained_earnings"}]},
			{"op":"coalesce","args":[{"op":"lookup","source":"zoo.ledger_lines","where":{"op":"and","args":[
				{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
				{"op":"in","args":[{"path":[{"link":"account"},{"field":"type"}]},PL]},
				{"op":"lt","args":[{"op":"fiscalYear","args":[{"path":[{"field":"date"}]}]},{"op":"fiscalYear","args":[FROM]}]}]},
				"value":NET,"pick":"sum"},{"value":0}]},
			{"value":0}]}]}`
	// byNormal(X): a credit-normal account shows the negated net.
	glByNormal = `{"op":"if","args":[
		{"op":"in","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":["liability","equity","income"]}]},
		{"op":"sub","args":[{"value":0},X]},X]}`
	// The other line's account of a 2-line entry, "-Split-" for more.
	glCount = `{"op":"lookup","source":"zoo.ledger_lines",
		"where":{"op":"eq","args":[{"path":[{"link":"entry"}]},{"outer":[{"link":"entry"}]}]},
		"value":{"path":[{"field":"id"}]},"pick":"count"}`
	glSplit = `{"op":"case","args":[
		{"op":"eq","args":[COUNT,{"value":2}]},
		{"op":"lookup","source":"zoo.ledger_lines","where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"link":"entry"}]},{"outer":[{"link":"entry"}]}]},
			{"op":"ne","args":[{"path":[{"field":"id"}]},{"outer":[{"field":"id"}]}]}]},
			"value":{"path":[{"link":"account"},{"field":"full_name"}]},"pick":"min"},
		{"op":"gt","args":[COUNT,{"value":2}]},{"value":"-Split-"},
		{"value":null}]}`
	glSpec = `{"version":3,"source":"zoo.ledger_lines",
		"params":[
			{"id":"period","type":"dateRange","label":"Period","default":{"preset":"thisMonthToDate"}},
			{"id":"rows","type":"choice","label":"Rows","default":"active","options":[{"value":"active","label":"With activity"},{"value":"all","label":"All"}]},
			{"id":"accounts","type":"choice","label":"Accounts","multiple":true,
			 "optionsFrom":{"source":"zoo.ledger_accounts","value":[{"field":"id"}],"label":[{"field":"full_name"}]}}],
		"columns":[
			{"id":"account","label":"Account / Date","path":[{"link":"account"}],"labelField":"full_name"},
			{"id":"number","label":"No.","path":[{"link":"entry"},{"field":"number"}]},
			{"id":"split","label":"Split","formula":SPLIT},
			{"id":"debit","label":"Debit","path":[{"field":"debit"}],"aggregate":"sum"},
			{"id":"credit","label":"Credit","path":[{"field":"credit"}],"aggregate":"sum"},
			{"id":"balance","label":"Balance",
			 "formula":BALANCE,
			 "rollup":ROLLUP}],
		"filter":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"posted"}]},{"value":true}]},
			{"op":"between","args":[{"path":[{"field":"date"}]},{"param":"period"}]},
			{"op":"in","args":[{"col":"account"},{"param":"accounts"}]}]},
		"groups":[{"id":"byChartRow","col":"account","open":true,"subtotals":true,
			"label":{"op":"if","args":[
				{"op":"isEmpty","args":[{"record":"byChartRow","path":[{"field":"number"}]}]},
				{"record":"byChartRow","path":[{"field":"full_name"}]},
				{"op":"concat","args":[{"record":"byChartRow","path":[{"field":"number"}]},{"value":" "},{"record":"byChartRow","path":[{"field":"full_name"}]}]}]},
			"fill":{"where":{"op":"in","args":[{"path":[{"field":"id"}]},{"param":"accounts"}]}},
			"having":{"op":"or","args":[
				{"op":"gt","args":[{"op":"count","args":[{"path":[{"field":"id"}]}]},{"value":0}]},
				{"op":"ne","args":[OPENING,{"value":0}]},
				{"op":"and","args":[
					{"op":"eq","args":[{"param":"rows"},{"value":"all"}]},
					{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"inactive"}]},{"value":false}]}]}]},
			"opening":{"label":"Beginning balance","cells":{"balance":OPENING_BALANCE}}}],
		"sort":[
			{"formula":{"op":"case","args":[
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"asset"}]},{"value":1},
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"liability"}]},{"value":2},
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"equity"}]},{"value":3},
				{"op":"eq","args":[{"record":"byChartRow","path":[{"field":"type"}]},{"value":"income"}]},{"value":4},
				{"value":5}]}},
			{"formula":{"record":"byChartRow","path":[{"field":"number"}]}},
			{"formula":{"record":"byChartRow","path":[{"field":"full_name"}]}},
			{"formula":{"path":[{"field":"date"}]}},
			{"formula":{"path":[{"field":"position"}]}}],
		"detail":{"label":{"path":[{"field":"date"}]}}}`
)

// balanceSpec fills the named pieces into the spec.
func balanceSpec() string {
	opening := strings.NewReplacer("FROM", glFrom, "NET", glNet, "PL", glPL).Replace(glOpening)
	byNormal := func(x string) string { return strings.ReplaceAll(glByNormal, "X", x) }
	running := `{"op":"add","args":[` + opening + `,{"op":"runningSum","args":[` + glNet + `]}]}`
	total := `{"op":"add","args":[` + opening + `,{"op":"coalesce","args":[{"op":"sum","args":[` + glNet + `]},{"value":0}]}]}`
	return strings.NewReplacer(
		"SPLIT", strings.ReplaceAll(glSplit, "COUNT", glCount),
		"OPENING_BALANCE", byNormal(opening),
		"BALANCE", byNormal(running),
		"ROLLUP", byNormal(total),
		"OPENING", opening,
	).Replace(glSpec)
}

// glLine is one row under an account: its kind, label, balance and split.
type glLine struct {
	Kind, Label    string
	Balance, Split any
}

// balanceGroups lists the group rows' labels in order, and each
// group's rows (opening, lines, subtotal) under its label.
func balanceGroups(res *Result) ([]string, map[string][]glLine) {
	var order []string
	byID := map[string]string{}
	rows := map[string][]glLine{}
	for _, r := range res.Rows {
		if r.Kind == KindGroup {
			order = append(order, r.Label)
			byID[r.ID] = r.Label
			continue
		}
		if label, ok := byID[r.Section]; ok {
			rows[label] = append(rows[label], glLine{r.Kind, r.Label, r.Cells["balance"], r.Cells["split"]})
		}
	}
	return order, rows
}

func requireGroupRows(t *testing.T, rows map[string][]glLine, account string, want ...glLine) {
	t.Helper()
	if got := rows[account]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s rows = %#v\nwant %#v", account, got, want)
	}
}

func glOpeningRow(balance int64) glLine {
	return glLine{KindOpening, "Beginning balance", balance, nil}
}

func glSubtotalRow(account string, balance int64) glLine {
	return glLine{KindSubtotal, "Total " + account, balance, nil}
}

func glDetailRow(day string, balance int64, split any) glLine {
	return glLine{KindDetail, day, balance, split}
}

func runBalances(t *testing.T, z *zoo, params map[string]string) *Result {
	t.Helper()
	return z.run(t, "ann", balanceSpec(), Options{Today: "2026-07-15"}, params)
}

const glJuly = `{"from":"2026-07-01","to":"2026-07-31"}`

func TestABalanceShapedSpecLoadsAndValidates(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	spec, info, err := Load([]byte(balanceSpec()))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != CurrentVersion || info.UpgradedFrom != 0 {
		t.Fatalf("version %d, upgraded from %d", spec.Version, info.UpgradedFrom)
	}
	if err := Validate(z.app, spec); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// A debit-normal account credited 100.00 then debited 30.00 shows −100.00,
// −70.00 and ends at −70.00; a credit-normal account credited 45.00 shows
// +45.00; Split names the other account of a 2-line entry and "-Split-" for 3.
func TestABalanceShapedReportRunsBalancesByNormalSide(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": glJuly})
	order, rows := balanceGroups(res)
	want := []string{"1000 Cash", "1100 Bank", "2000 Loan", "3900 Retained earnings", "4000 Sales", "5000 Rent"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("accounts = %v\nwant %v", order, want)
	}
	requireGroupRows(t, rows, "1000 Cash",
		glOpeningRow(0),
		glDetailRow("2026-07-01", -10000, "Rent"),
		glDetailRow("2026-07-02", -7000, "-Split-"),
		glSubtotalRow("1000 Cash", -7000))
	requireGroupRows(t, rows, "2000 Loan",
		glOpeningRow(0),
		glDetailRow("2026-07-02", 4500, "-Split-"),
		glSubtotalRow("2000 Loan", 4500))
	// The draft entry J3 is left out.
	requireGroupRows(t, rows, "5000 Rent",
		glOpeningRow(0),
		glDetailRow("2026-07-01", 10000, "Cash"),
		glSubtotalRow("5000 Rent", 10000))
	// Last fiscal year's sale folds into retained earnings, not into Sales.
	requireGroupRows(t, rows, "4000 Sales",
		glOpeningRow(0),
		glDetailRow("2026-07-02", -1500, "-Split-"),
		glSubtotalRow("4000 Sales", -1500))
	requireGroupRows(t, rows, "3900 Retained earnings", glOpeningRow(2000), glSubtotalRow("3900 Retained earnings", 2000))
	requireGroupRows(t, rows, "1100 Bank", glOpeningRow(2000), glSubtotalRow("1100 Bank", 2000))
	// The Total is in no account: its balance reads a record it does not have.
	last := res.Rows[len(res.Rows)-1]
	if last.Kind != KindTotal || last.Cells["balance"] != nil {
		t.Fatalf("last row = %s %#v, want the Total with a blank balance", last.Kind, last.Cells["balance"])
	}
	// Every balance that is not 0 read at least one line, so it carries that
	// line's currency (R24: the first known code of its reads).
	for _, r := range res.Rows {
		if b := r.Cells["balance"]; b != nil && b != int64(0) && r.Cells["balance@currency"] != "USD" {
			t.Fatalf("%s %q balance %#v has currency %#v, want USD", r.Kind, r.Label, b, r.Cells["balance@currency"])
		}
	}
}

func TestABalanceShapedReportCarriesTheBalanceIntoALaterPeriod(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": `{"from":"2026-07-02","to":"2026-07-31"}`})
	_, rows := balanceGroups(res)
	requireGroupRows(t, rows, "1000 Cash",
		glOpeningRow(-10000),
		glDetailRow("2026-07-02", -7000, "-Split-"),
		glSubtotalRow("1000 Cash", -7000))
}

func TestABalanceShapedReportUnderAllDatesBeginsAtZero(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": `{"preset":"allDates"}`})
	order, rows := balanceGroups(res)
	for _, account := range order {
		if got := rows[account]; len(got) == 0 || !reflect.DeepEqual(got[0], glOpeningRow(0)) {
			t.Fatalf("%s starts with %#v, want a 0.00 beginning balance", account, got)
		}
	}
	requireGroupRows(t, rows, "1000 Cash",
		glOpeningRow(0),
		glDetailRow("2026-07-01", -10000, "Rent"),
		glDetailRow("2026-07-02", -7000, "-Split-"),
		glSubtotalRow("1000 Cash", -7000))
	requireGroupRows(t, rows, "4000 Sales",
		glOpeningRow(0),
		glDetailRow("2025-12-15", 2000, "Bank"),
		glDetailRow("2026-07-02", 500, "-Split-"),
		glSubtotalRow("4000 Sales", 500))
	if _, shown := rows["3900 Retained earnings"]; shown {
		t.Fatalf("retained earnings has nothing before All dates, so the having hides it: %v", order)
	}
}

func TestABalanceShapedReportShowsQuietActiveGroupsOnAll(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	res := runBalances(t, z, map[string]string{"period": glJuly, "rows": `"all"`})
	order, rows := balanceGroups(res)
	requireGroupRows(t, rows, "1200 Petty cash", glOpeningRow(0), glSubtotalRow("1200 Petty cash", 0))
	for _, account := range order {
		if account == "1300 Idle" {
			t.Fatalf("an inactive account with nothing to show is hidden: %v", order)
		}
	}
}

// On a detail row the opening's looked-up code is a subquery the currency
// guard repeats (R24), so a run over 2,000 lines must stay inside a budget:
// 5 times the estimate in the plan (Task 22).
func TestABalanceShapedReportOverTwoThousandLinesRunsInTime(t *testing.T) {
	z := newZoo(t)
	withBalanceZoo(t, z)
	accounts := []string{"cash", "bank", "petty", "loan", "re", "sales", "rent"}
	for i := range 1000 {
		entry := fmt.Sprintf("B%d", i)
		z.rec(t, entry, "zoo_ledger_entries", map[string]any{"number": entry})
		// Even entries fall before July, odd ones in it.
		day := fmt.Sprintf("2026-%02d-%02d", 1+i%6, 1+i%28)
		if i%2 == 1 {
			day = fmt.Sprintf("2026-07-%02d", 1+i%28)
		}
		for j, side := range []struct {
			account       string
			debit, credit int
		}{{accounts[i%7], 100, 0}, {accounts[(i+3)%7], 0, 100}} {
			z.rec(t, fmt.Sprintf("%s-%d", entry, j), "zoo_ledger_lines", map[string]any{
				"entry": z.id(entry), "account": z.id(side.account), "date": day,
				"debit": side.debit, "credit": side.credit, "currency": "USD", "position": j + 1, "posted": true,
			})
		}
	}
	start := time.Now()
	res := runBalances(t, z, map[string]string{"period": glJuly})
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("the run took %v, over the 4s budget", took)
	}
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	details := 0
	for _, r := range res.Rows {
		if r.Kind == KindDetail {
			details++
			if r.Cells["balance"] == nil {
				t.Fatalf("detail row %q has no balance", r.Label)
			}
		}
	}
	if details == 0 {
		t.Fatal("the first page has no detail rows")
	}
}
```

- [ ] **Step 2: Run it**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'BalanceShaped' ./reports/`
Expected: PASS (6 tests); write the time test's measured run time in the task's report. No red is expected once Tasks 1–8 hold. A failure is an engine defect: find which part gives the wrong row (the opening lookup, `bound`, `allDates`, the rollup, the opening row, `fill` / `having`, `outer`, `count`) and fix it in the file of the task that owns that part (File Structure table), with a focused test there; never change this test's numbers.

- [ ] **Step 3: Run the package's whole Go suite and the isolation check**

Run:
```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/
cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts
```
Expected: the suite PASS; the isolation check reports 0 references (the test names only fictional `zoo_ledger_*` collections and `zoo.ledger_*` sources).

- [ ] **Step 4: Commit (the driver runs it)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
gofmt -w core/server/reports/balance_shape_test.go
git add core/server/reports/balance_shape_test.go
git commit -m "test(core): a balance-shaped report runs on the v3 engine end to end"
```
