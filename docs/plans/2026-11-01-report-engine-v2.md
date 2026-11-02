# Report Engine v2 (Checkpoint 0 of p5-trial-balance) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core report-engine and report-UI extensions E1–E14 and OQ2 that the approved Trial Balance design needs: text-day dates, totals that add the rows shown, group filters, record groups with labels and fill, sort by formula, `lookup`, detail labels, choice params whose options come from a source (with a new read route and a searching picker), source headings, unpublished shipped reports, exported report e2e helpers, spec version 2 with a full builder round-trip, groups that open by default, every report operator in the builder's formula editor, and report rows that wrap onto more lines on narrow screens.

**Architecture:** The Go engine (`core/server/reports`) gains new spec fields (all optional, spec version 2, the 1→2 step is the identity), new formula leaves and operators (`record`, `lookup`), level features (`having`, `label`, `fill`, `open`, `total: "rows"`), a param-options read route, and two `Source` hooks (`Heading`, `Watch`). The TypeScript side (`core/reports`) reads the v2 spec strictly, round-trips every v2 field through the builder draft, edits formulas with the forms `FormulaEditor` through a new optional `operators` prop, draws a searching param picker, prints the source heading, and lays the report table out per width (one line on desktop, wrapped lines on narrow widths, author placement through `display.layouts`). Core e2e drives the `salesstub` stub package, extended for every new capability.

**Tech Stack:** Go (PocketBase fork v0.40.4, `tinycld.org/core/reports`, SQLite), TypeScript, React Native + react-native-web (Expo Router), Uniwind classes, react-hook-form + zod v4, TanStack Query 5, Zustand, Playwright, vitest, Biome.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/trial-balance-design.md` §0 and §4 (binding; this plan builds all of §4 plus E14, which the human added on 2026-11-01), with the engine spec `/Users/nas/code/tinycld/docs/superpowers/specs/2026-10-30-report-engine-design.md` as context, the helix ledger `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/trial-balance-ledger.md` (checkpoint 0), and the design packet `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-trial-balance/packet.md`. Prior core plans: `docs/plans/2026-10-30-report-engine.md`, `docs/plans/2026-10-30-report-sharing.md` ("Contracts for p5-report-builder"), `docs/plans/2026-10-31-report-builder.md` (this repo). E14's model: `~/code/gridley` (`src/grid.tsx`, `src/layout.tsx`, `src/types.ts`, `README.md`).

---

## For review

This section is for the human before anything runs. Core UI has no design gate, so the layout per width is written out here.

### Answers (the human, 2026-11-01): plan approved

- **Q1** yes (`Source.Watch`). **Q2** refuse having on a layout's first group. **Q4** `pick` optional, default `min`. **Q6** refuse `total: "rows"` on running values and percents. **Q7** allow correlated lookups. **Q8** no automatic wrapping on desktop. **Q9** yes, author arrangement per width with the Arrange section.
- **Q3 changed:** fill on every group level — a fill record appears under each parent group, under every currency (R6, Task 6, builder in Task 22).
- **Q5 changed:** amounts with a per-row currency in `record` leaves and `lookup` values carry their currency through (R3, Task 5a).
- **Q10 changed:** general spanning cells by guillotine cuts; only a pinwheel is refused with "These cells overlap in a way that can't be laid out; move one cell." (R28, Tasks 15, 24, 25).
- **Web scroll offset:** fixed in the FlashList fork (Tasks 25a–25d, with a STOP at Task 25c for the human to review and merge the fork PR); the web caveat leaves the [platform] list.

### Preflight rulings (2026-11-01), applied

Order: Task 9 (formula sort keys) runs before Tasks 5a and 6. Under the no-limits rule five limits became features, and no new refusal was added: lookups in layout computed rows (F16, R21 withdrawn), `total: "rows"` on a number column with no aggregate (F17, R8), an amount param so a money total can be asked (F18, R18 and new R29, Task 12a), "All <group>" as a group reference the server expands (F19, R13 and new R30, Task 13a), and link columns that show their record's label (F22, R9 and new R31, Task 5b). Money currency keys are now gathered from every formula a level reads (F5, R3).

### Why this is one exclusive PR

E12 changes the report spec contract every lane's report units write (version 2, new fields the builder must keep). E4 and E8 change the run contract (relation-typed values, a new route `POST /api/reports/param-options`, the result's `heading`) and the builder draft contract. OQ2 adds a prop to a forms-engine component. E14 changes how every report table draws on narrow screens and adds `display` to the spec. E1, E9, E10 and E11 alone would be small, but the Books checkpoints need the whole set (spec OQ1). One PR on `books/p5-trial-balance` in `tinycld`, `exclusive` held by lane purchases until it merges, merged before Books checkpoint 1. No core migration, no generator change, no removed or renamed export.

### The spec format: version 2

`CurrentVersion` becomes 2. Every new field is optional. The 1→2 upgrade step is the identity, so every stored v1 spec loads unchanged (R1: it also gives no "upgraded" notice, because nothing changed). A v2 spec sent to an older server is refused by its version (`spec.go:212`). The client schema accepts versions 1 and 2 and writes 2.

A v2 spec that uses every new part (fictional `zoo.*` sources; the Trial Balance's own spec is in the design spec §5):

```jsonc
{
  "version": 2,
  "source": "zoo.feedings",
  "params": [
    { "id": "asOf", "type": "asOf", "label": "As of", "default": { "preset": "endOfLastMonth" } },
    { "id": "rows", "type": "choice", "label": "Rows", "default": "nonzero",
      "options": [ { "value": "nonzero", "label": "Non-zero" }, { "value": "all", "label": "All" } ] },
    // E8: options read from a source, under the reader's rules; value, label and group are paths on it.
    { "id": "animals", "type": "choice", "label": "Animals", "multiple": true,
      "optionsFrom": { "source": "zoo.animals", "value": [{ "field": "id" }], "label": [{ "field": "name" }],
                       "group": [{ "field": "species" }], "where": { "op": "eq", "args": [{ "path": [{ "field": "active" }] }, { "value": true }] } } }
  ],
  "columns": [
    // E4: a path that ends at a to-one link is a relation to the linked source (its id; blank when hidden).
    { "id": "animal", "label": "Animal", "path": [{ "link": "animal" }] },
    // E4: a record leaf reads a field of the group's linked record.
    { "id": "weight", "label": "Weight", "formula": { "record": "byAnimal", "path": [{ "field": "weight" }] } },
    // E2: the total adds the rows the result shows (after having), per currency.
    { "id": "cost", "label": "Cost", "path": [{ "field": "cost" }], "aggregate": "sum", "total": "rows" },
    // E6: one value from another source's rows the reader may see; NULL when none match.
    { "id": "site", "label": "Site", "formula": { "op": "lookup", "source": "zoo.sites",
        "where": { "op": "eq", "args": [{ "path": [{ "field": "region" }] }, { "value": "north" }] },
        "value": { "path": [{ "field": "name" }] }, "pick": "min" } }
  ],
  "filter": { "op": "and", "args": [
    { "op": "lte", "args": [{ "path": [{ "field": "due" }] }, { "param": "asOf" }] },   // E1: `due` is a text day column
    { "op": "in", "args": [{ "col": "animal" }, { "param": "animals" }] } ] },
  "groups": [ {
    "id": "byAnimal", "col": "animal", "subtotals": false,
    "label": { "record": "byAnimal", "path": [{ "field": "name" }] },                        // E4 label (default: the target's first text field)
    "fill": { "where": { "op": "eq", "args": [{ "path": [{ "field": "active" }] }, { "value": true }] } }, // E4 fill: records with no rows
    "having": { "op": "or", "args": [ { "op": "eq", "args": [{ "param": "rows" }, { "value": "all" }] },
                                      { "op": "ne", "args": [{ "op": "sum", "args": [{ "path": [{ "field": "cost" }] }] }, { "value": 0 }] } ] }, // E3
    "open": false                                                                             // E13
  } ],
  "sort": [ { "formula": { "record": "byAnimal", "path": [{ "field": "weight" }] }, "dir": "desc" }, { "col": "cost" } ], // E5
  "detail": { "label": { "op": "concat", "args": [{ "path": [{ "field": "fed" }] }, { "value": " · " }, { "path": [{ "field": "note" }] }] } }, // E7
  "display": { "layouts": [ {                                                                 // E14: author placement per width
    "width": "mobile", "tracks": 3,
    "cells": [ { "col": "animal", "line": 1, "span": 3 },
               { "col": "weight", "line": 2, "span": 1 },
               { "col": "cost", "line": 2, "span": 1, "justify": "end" },
               { "col": "site", "line": 2, "span": 1, "justify": "end" } ] } ] }
}
```

New fields, by part: `columns[].total` (`"recompute"` default | `"rows"`); `groups[].having`, `groups[].label`, `groups[].fill: { where? }`, `groups[].open`; `sort[]` is `{ col, dir }` or `{ formula, dir }`; `detail: { label }`; `params[].optionsFrom: { source, value, label, group?, where? }`; formula leaf `{ record, path }`; operator `{ op: "lookup", source, where?, value: <node>, pick? }`; `display: { layouts: [{ width, tracks, cells: [{ col, line, start?, span?, lines?, justify? }] }] }`.

### Engine behaviour, per change

- **E1 `Field.TextDay`** (`registry.go`, `check.go`): a `Date` field with `TextDay: true` reads a `text` column that holds `YYYY-MM-DD`. `CheckSources` accepts only a text column for it and refuses `TextDay` on any other type. It reads like a date column (`fieldSQL` takes the first ten characters, `stmt.go:85-94`), so filters, buckets, presets and pivots work unchanged; the catalog says `date`.
- **E2 `total: "rows"`** (`levels.go`, `run.go`, `pivot.go`): a `rows` column's group, subtotal and total cells are the sum of the cells of the rows directly under them that the result shows (after `having`), per currency. The innermost group computes from its base rows. With no groups, the total adds the detail cells. Pivot cells of a `rows` cell column follow the same rule. Layout sections add their shown rows; computed rows are unchanged. A number or amount column with no aggregate also adds its rows: its detail cells, and per group the sum of its base rows (F17). Refused on a column that is neither a number nor an amount, and on a running value or percent (R8). `recompute` (the default) totals every base row (OQ6).
- **E3 `groups[].having`** (`levels.go`, `run.go`): a condition compiled on that level's rows (aggregates, columns, record fields, params; a condition on an unset param matches every group, as in `filter`). A hidden group hides its children. `recompute` totals still read every base row; `rows` totals add only the rows shown. Refused on the first group of a layout report (R7).
- **E4 record groups** (`bind.go`, `formula.go`, new `record.go`, `levels.go`): a path that ends at a to-one link, or at the `id` field, is a relation value that carries its target source (R2). `if`, `case` and `coalesce` of relations need one target. A group whose column is such a value is a record group: its key is the record id (no two records merge). The `record` leaf reads the record's fields on the group's level, its subtotal and every row below (blank on outer rows). `groups[].label` labels any group's row (a formula on that level; default for a record group: its record's label, R31). A column whose value is a relation shows its record's label beside the id (`<col>@label`, `columns[].labelField` picks the field, R31). `groups[].fill` adds the target's records (filtered by `where`, under the reader's rules) that have no base rows, on any group level, under each parent group and every currency (R6); a fill's `where` may tie a record to its parent (`{col}` of an enclosing group reads the parent's key, `{record}` its record). A fill row opens only onto a deeper level's fill records.
- **E5 sort by formula** (`levels.go`, `run.go`, `paging.go`): `sort[]` takes `{ formula, dir }`. On a level the formula is a cell of that level (skipped where it is blank); on detail rows a row value. A formula sort key can page a detail report like a column key; only a running value or a percent is refused, as for columns.
- **Per-row currencies (Q5):** a `record` amount or a looked-up amount whose source keeps a currency per row carries that currency: beside the value on group and detail rows (the cell's `<col>@currency`), and as a currency key of the rows when it is totalled, so totals group by it (R3).
- **E6 `lookup`** (`formula.go`, new `lookup.go`): one value (`min` by default, or `max`, R5) of `value` over the rows of `source` that match `where`, read from that source's rule CTE, so the reader's list rule applies; NULL when none match. Inside a lookup a path reads the looked-up source; a column, a record field or a param reads the report's row (a correlated subquery, R4). Allowed in columns, filter, group keys, having, labels, sort and layout computed rows (F16: there `{col}` reads the computed row's section values and `{param}` the run's params). The run's topics include the looked-up collection (the change signal reads every table the statement reads, `changesignal/topics.go:18-36`).
- **E7 `detail.label`** (`run.go`, `assemble.go`): a formula that labels each detail row (default: the first column, as today). Each row also says which column its label repeats (`labelColumn`, R11) so the client drops exactly that cell from child rows.
- **E8 `optionsFrom`** (`params.go`, `bind.go`, new `options.go`, `endpoint.go`): a choice param may read its options from a source. A run checks a posted value by type only (R13); a multiple choice may also hold whole groups, `{"group": "<value>"}`, which the server expands under the reader's rules at run time (R30). A new param type `amount` holds cents and a currency (R29). New read route `POST /api/reports/param-options` (approved, OQ5), described under "API".
- **E9 `Source.Heading`** (`registry.go`, `run.go`): `func(app core.App) string`, the name a report on that base source prints above its title; the result gains `heading`. With it, `Source.Watch []string` (R14, Q1): collections the source's Go hooks read (`Heading`, `FiscalStartMonth`, `Currency`), added to every run's topics, so a settings change re-runs an open report.
- **E10 `Shipped.Unpublished`** (`shipped.go`): core creates such a report unpublished and never publishes it on a revision; the package shares it (R15).
- **E11 report e2e helpers**: the generic part of `tests/e2e/reports-helpers.ts` (`openReportsHub`, `hubRow`, `rowOf`, `cellOf`, `pressToolbar`) moves to `tests/e2e/report-view-helpers.ts` with a new `chooseParamOption`, exported through `@tinycld/core/e2e-helpers`.
- **E12 spec version 2**: `spec.go` `CurrentVersion = 2`, identity step, strict decode of every new field, alias resolution in every new part (having, labels, fill, sort, detail label, lookup, record paths, optionsFrom), `Validate` for every new field, and a Go test that a spec with every v2 field loads, validates and is stored unchanged.
- **E13 `groups[].open`** (`run.go`): a run with no expansion state (`options.expand` absent and no `expandAll`) opens the levels marked `open`. `options.expand: []` opens nothing (R10), which is how Collapse all keeps them closed.
- **E14 wrapped rows** (client only; the engine validates `display`): see "Report table layout per width" below.
- **OQ2 formula editor**: the forms `FormulaEditor` gains an optional `operators` prop; forms keep today's set. The report builder passes the report operator set (every operator in `formula.go` except `exists`/`notExists`, which the filter editor owns).

### API / route changes

- `POST /api/reports/param-options` (signed-in users, session-only like `/api/reports/run`; an OAuth token is refused by default-deny). Body: `{ reportId | spec, param, search?, after?, group?, values?, limit?, params?, today? }`. Answer: `{ options: [{ value, label, group? }], groups: [{ value, label }], next? }`. Options are read under the reader's list rules, ordered by label then value, 50 per page (`limit` up to 1000), searched by label (case-insensitive contains). `values` labels chosen values (no paging); `group` narrows to one group (the picker's "All <group>"); `params` are the report's other params, for a `where` that reads them. A saved report is read through its view rule, exactly as a run reads it. Errors as the run route: 400 invalid, 403 no access, 422 unknown reference, 408 timeout.
- `POST /api/reports/run`: result gains `heading` (E9); rows gain `labelColumn` (E7); `options.expand: []` now means "open nothing" (E13); topics gain watched collections (E9).
- No change to `GET /api/reports/sources` (a text day field reports `date`).

### Client lib changes

`lib/spec.ts` (v2 schema), `lib/types.ts` (`heading`, `labelColumn`, param-option types), `lib/rows.ts` (`DEFAULT_EXPANSION`, explicit empty expansion, child cells by `labelColumn`), `lib/run-client.ts` (`fetchParamOptions`), `lib/params.ts` (options-from-source values and chip text), new `lib/table-lines.ts` (E14 line layouts), `lib/builder/formula-ops.ts` (report operator set, replacing the ComputeExpr bridge in `lib/builder/formula.ts`), `lib/builder/draft.ts` (every v2 field), `lib/builder/filter.ts` (aggregate-mode conditions, relation conditions), `lib/builder/columns.ts` (link-ending paths, record fields), `lib/builder/errors.ts` (`detail`, `display` paths). Forms: `core/lib/forms/builder/formula-tree.ts` (generic tree walk) and `core/components/forms-builder/FormulaEditor.tsx` (`operators` prop).

### Builder and view UI, with layout per width

Widths are core's breakpoints (`useBreakpoint`): `mobile` (phone), `tablet`, `desktop`. The builder is an inline drawer (440pt) on desktop, an overlay drawer (440pt) on tablet and a full-height sheet with a Design | Preview switch on phone (unchanged); every new control below stacks vertically inside its section card, so it reads the same at every width, and every menu and picker is a popover on desktop and tablet and a bottom sheet on phone (core `PickMenu`, `Popover`).

- **Columns section** (each column card): for a link column, a "Show as" menu of the linked record's fields (default: its first text field, R31); a "Total" choice for a number or amount column — "Recompute from all rows" (default) / "Add the rows shown" (E2); under the column list a "Row label" card: "First column" (default) or a formula (E7), edited with the report formula editor.
- **Formulas** (columns, row label, group label, sort formula, layout rows): the report formula editor (OQ2): the same part menu, now listing Number, Text, Yes or no, Empty, Column, Field, Group field, Asked value, the arithmetic, totals (Sum of, Count of, …), running values, comparisons, All of / Any of / Not, Is empty, If, First match, First not empty, the text and date operators, and Look up. Parts with slots show a small slot caption ("When", "Then", "Otherwise"; "Where", "Value"). A formula the editor cannot show (an unknown key or operator) stays read-only and is saved unchanged (R17).
- **Group and sort section** (each group level card): "Group by" lists columns, including a column that is a link ("Team (record)"); for a record group three more rows: "Label" (a field picker over the linked source, or "Formula…"), a "Show every Team, even with no rows" switch with its own condition editor (any level; under another group a condition can be "Same as the <group> above"), and, for every group, "Show only groups where…" (the filter editor in aggregate mode: Total of a field, Column, Group field, with Ask when run) and an "Open when the report opens" switch. Sort rows: "Sort by" lists columns, the group record's fields ("Team › Name") and "Formula…".
- **Filter section**: a condition on a link ("Team (record)") offers is / is not / is any of / is empty; its value is picked from the linked records (a searching picker); "Ask when run" turns it into a choice param whose options come from that source (E8). On a money field or total, "Ask when run" asks an amount (R29); on "is between" a number or amount asks two params, from and to.
- **Arrange section** (new, E14): width tabs Phone · Tablet · Desktop; per width a switch "Arrange columns for this width" (off = automatic), a "Columns per line" choice (1–12), and one row per column with Line, Start, Span, Lines and Align choices; a column left out of an arrangement is not shown at that width. The live preview shows the arrangement for the width it is drawn at.
- **Report view, param chips**: a choice param from a source opens a picker (popover 340pt on desktop and tablet, bottom sheet on phone): a search field at the top, the options in groups with an "All <group>" row at each group head, more options loading near the end of the list, Apply at the bottom. "All <group>" picks the whole group, kept as a group (R30). The chip shows up to two labels ("Teams: N1, All North"), else "N selected". An amount param opens an amount box and a currency box (blank for any currency); its chip reads "Budget: USD 1,500.00".
- **Document header**: the source heading (E9), else the organization name, then the title and the period.

### Report table layout per width (E14)

Modelled on Gridley: one cell renderer per column; per-width layouts place each column on a line (`line`, like Gridley's `row`) with `start`, `span` (Gridley's `colSpan`), `lines` (Gridley's `rowSpan`) and `justify`; the heading row uses the same line layout, so each heading sits over its values. React Native has no CSS grid on native, so a row is drawn with flex (the same code on web and native): its arrangement is cut into a tree of blocks — a stack of blocks top to bottom, or blocks side by side as wide as their share of equal tracks (a percent) — so a plain arrangement is a vertical stack of lines, each a flex row of cells with spacers for empty tracks, and `justify` aligns each cell. Row content is flex lines; FlashList positions the rows (it measures each one).

- **Desktop:** one line per row, the label column flexible (min 220pt), amount columns 128pt. When they do not fit, the whole table (heading row and list together) sits in one horizontal scroller, so headings and cells move sideways in step; the label column stays frozen at the left by a `translateX` that follows the sideways scroll (R27). An author arrangement for desktop replaces the one line.
- **Tablet and phone, automatic:** when the label (min 220pt tablet, 168pt phone) and the amount columns (128pt / 108pt) fit the table's width, one line as today. When they do not fit, each row wraps: line 1 is the label across the row; the amount columns follow on lines 2, 3, … in aligned tracks (`tracks = max(1, floor(width / 104))`, 3 at 390pt), in column order; the heading wraps the same way. No sideways scroll. Example, the Trial Balance on a 390pt phone: line 1 "Cash and bank:Business checking"; line 2 No. · Debit · Credit, each under its heading.
- **Author arrangement:** `display.layouts[]` for a width replaces the automatic layout there; columns not listed are not shown at that width (the label column, if left out, gets a line of its own above, so a row always has its label); the pivot cell column's entry stands for all its pivot columns, which flow one per track from that position. The arrangement is cut into blocks by straight lines (guillotine cuts), so a cell with `lines > 1` may sit anywhere, mid-row included, beside the lines it spans and as tall as they are; `justify` aligns it (Gridley's demo of an ID centred down a two-line row works at either edge or in the middle). Only a pinwheel arrangement is refused (R28).
- **The list:** core's custom row window and its spacers (`components/use-row-window.ts`) are replaced by FlashList v2 (`@shopify/flash-list` 2.3.3, the `nathanstitt` fork core already pins and uses on web in the emoji picker). Rows measure themselves, so wrapped rows of any height need no height scheme. Paging stays: `onEndReached` calls `useReportRun.loadMore`, which keeps its guard (`hasNextPage && !isFetchingNextPage`); FlashList checks the end after every commit and again when the rows change, so a first page shorter than the view asks for the next page by itself (R25).
- **Sticky headers:** the column heading row sits above the list, outside its scroll, so it always stays at the top. Open group rows are FlashList `stickyHeaderIndices`: while a group's rows scroll, its row stays stuck under the heading row until the next group pushes it up. FlashList keeps one sticky item at a time; every open group at any depth is a sticky index, so the stuck item is the innermost open group above the top, drawn with the open groups that enclose it stacked above it — nested levels read as a stack (for example "Assets" over "Current assets") in one sticky slot (R26). Folding changes the rows, so the indices are recomputed from the visible rows on every render; a press on a stuck group folds it.
- **Rows keep their styling:** group, subtotal, computed and total rows keep their weight and rules (the rule draws across the whole wrapped row); a group row's label is its fold toggle (its figures take the same press, not a tab stop); a group row followed by its subtotal shows only its label line; child rows indent the label only, so figures stay aligned; pivot columns are ordinary amount columns in every layout.

### Help changes

Core `help/reports.md` gains sections: Totals (recompute or add the rows shown), Hide groups (having), Group by a linked record (label, show every record), Sort (by a group field or a formula), Look up a value, Opened rows (row label), Filters that list records, Fold groups (open when the report opens), Formulas (the full operator list), Narrow screens (wrapping and Arrange). `docs/packages.md` "Report sources" gains `TextDay`, `Heading`, `Watch`, link-ending paths and spec v2 fields; "Shipped and saved reports" gains `Unpublished` and how a package shares its reports; a new "Report e2e helpers" paragraph names the exported helpers.

### Driver rulings

- **R1. Silent identity upgrade.** `Load` gives the "upgraded" notice only when an upgrade step changed the document. The 1→2 step changes nothing, so a v1 report runs with no notice; its next save stores version 2.
- **R2. Relation values.** A value path that ends at a to-one link means the linked row's `id` (the join, so a linked row the reader cannot see reads blank, as every to-one value does, `stmt.go:117-140`); a path ending at the `id` field is a relation to its own source. Both carry the target source; `if`, `case` and `coalesce` refuse results that link to different sources.
- **R3. Amounts in records and lookups carry their currency (Q5, changed).** Read on a group, subtotal or detail row and not totalled, a record amount's currency is its record's own currency field and a looked-up amount's is the picked row's (a subquery beside the value; the cell's `<col>@currency`). Read inside a total, it is read once per base row, so its currency becomes a currency key of the rows like a path amount's and the totals group by it. A lookup of such an amount picks the row with the smallest or largest amount and returns that row's amount and currency (ties by currency code). Amounts combine only when their currencies come from the same place (a currency key, a fixed currency, or the same record field or lookup), as the engine's money rule already says. The currency keys of a level come from every formula it reads — columns, labels, having, sort formulas and the row label — so a having on a total of an amount no column shows still totals per currency (F5; record leaves add none of their own).
- **R4. Correlated lookups.** Inside a lookup, `{col}`, `{record}` and `{param}` read the report's row; a lookup that reads none of them is uncorrelated and SQLite evaluates it once per statement.
- **R5. `pick` defaults to `min`.**
- **R6. Fill on every level (Q3, changed).** A fill level's keys are its base rows' keys united with each parent row (the level above's keys, its own fill rows included; the report's currencies for the first level, or one no-currency row when it has no rows) crossed with the target's records that match the fill's `where`. The `where` reads the target's fields, params, lookups, and the enclosing groups (`{col}` of an enclosing group's column → the parent's key; `{record}` of an enclosing record group → its record), so "accounts of this type" ties to the parent. Fill rows sort with their siblings, add nothing to `recompute` totals, add their (blank or zero) cells to `rows` totals, meet `having`, and open only onto a deeper level's fill records.
- **R7. Having on a layout's first group is refused:** layout sections are fixed rows; hide rows on a deeper group.
- **R8. `total: "rows"`** is allowed on any number or amount column, with or without an aggregate (F17: with none, a group adds its base rows' values), that is not a running value or percent; with no groups it adds the detail cells.
- **R9. Group labels** apply to any group (default: today's key text; for a record group its record's label by R31 — never a raw id; "(none)" only for a blank link).
- **R10. `options.expand`:** absent → the levels marked `open` expand; `[]` → nothing expands; a list → those groups. The client's first run sends no `expand`; Collapse all sends `[]`.
- **R11. `Row.labelColumn`** names the column whose cell a row's label repeats (detail rows without `detail.label`, nested child rows); `lib/rows.ts` drops exactly that cell from child rows.
- **R12. param-options** as described under "API": page 50, `limit` ≤ 1000, search ≤ 200 characters, `values` ≤ 1000, groups capped at 200 and sent on the first page only (the client keeps one head per group), label/group of a select field shown by its option label.
- **R13. An options-from-source choice value** is checked by type only: one string (single) or a list of up to 1000 strings, each 1–500 characters, plus up to 200 group references (R30). Past 1000 values the run and the picker say "pick a whole group instead".
- **R14. `Source.Watch`** (beyond the spec's E9 text; see Q1): `CheckSources` refuses a watched collection that does not exist or is a system collection; each run adds the topics of the watched collections of every source whose collection it read.
- **R15. Sharing a shipped report is the package's own work:** core adds no share API. A package binds `OnRecordAfterCreateSuccess("reports")` (fired by `ReconcileShipped`'s `app.Save`) and reconciles at serve start; `docs/packages.md` says so (Books' `report_shares.go` uses it).
- **R16. The `operators` prop** is typed as a discriminated union on the forms `FormulaEditor`: without it the editor is today's forms editor, part for part and test id for test id; with it the editor works on the operator set's own tree type. The report operator set keeps the forms part ids for the parts both have (`number`, `field`, `add`, `sub`, `mul`, `div`, `round`, `min`, `max`), so existing report e2e ids stay.
- **R17. Report formula drafts edit the spec's own `{ op, args }` tree** (no ComputeExpr bridge). A formula is read-only only when it has a key or operator the editor does not write; `reports-shipped.spec.ts`'s `if` formula becomes editable and that spec changes to say so.
- **R18. The having editor** is the filter editor in aggregate mode: a condition's subject is "Total of a field" (sum, count, smallest, largest, average), "Column" or "Group field"; Ask when run follows the filter's rules; a money total asks an amount param (R29), and a range asks two params.
- **R19. Relation conditions** offer is / is not / is any of / is empty / is not empty; a typed value is picked from the target's records through param-options with a one-param draft spec; Ask when run makes a choice param with `optionsFrom { source: target, value: [id], label: [first text field] }` (multiple for "is any of").
- **R20. Params stay while anything reads them:** the builder keeps a param while the filter, a column formula, a group label, having or fill, a sort formula or the row label reads it (`keptParams`).
- **R21. Record leaves are refused in the filter and group keys** (they read rows before grouping); a fill's `where` may read only the records of the groups above it (R6). Lookups work everywhere, layout computed rows included (F16; the earlier refusal is withdrawn).
- **R22. Stub changes:** `salesstub_sales.booked_on` (text day, written from the sale's day), `salesstub_teams.region` (select north/south, a radio on the team form), both sources print the heading "Stub Sales Co", and a second shipped report "Stub team totals" (unpublished) uses a record group with label, `total: "rows"`, a sort formula, a detail label, a text-day filter and an options-from-source param grouped by region.
- **R23. E14 wrap rule:** automatic wrapping applies at tablet and phone widths when the one-line layout does not fit; desktop keeps one line and sideways scroll unless the author arranged desktop (wide pivots read better scrolled). Row content is drawn with flex lines (the human's review of 2026-11-01); FlashList positions the rows. See Q8.
- **R24. E14 author arrangement is in spec v2 (`display.layouts`) and the builder edits it** (the no-limits rule; see Q9). The engine validates it (known column ids, `width` one of mobile|tablet|desktop, `tracks` 1–12, `line` 1–8, `start` and `span` within the tracks, `lines` 1–8 under R28, `justify` start|center|end, no two cells overlapping) and otherwise ignores it.
- **R25. FlashList v2 replaces the custom row window.** Rows measure themselves; `onEndReached` (threshold 0.5) calls `loadMore`, whose guard stays; the "first page shorter than the view" fill comes from FlashList's own end check after each commit (`useBoundDetection.ts:96-108,165-170`). `use-row-window.ts` and its test are deleted.
- **R26. Sticky groups stack in one slot.** Every open group row is a sticky index; the stuck copy (`target: 'StickyHeader'`) draws the chain of open groups enclosing it, outermost first, so the reader sees every level they are inside even though FlashList sticks one item at a time. Chosen over sticking only the outermost group, which would hide the level the reader is reading.
- **R27. Frozen label column on desktop sideways scroll:** the heading row and the list share one horizontal scroller; label cells follow its offset with `translateX`, so they stay at the left as today's two-stack table kept them.
- **R28. `lines` (rowSpan) by guillotine cuts (Q10, changed).** An arrangement is cut recursively — a straight cut between lines into top and bottom blocks (tried first), or between tracks into left and right blocks — until each block holds one cell or none; a cell may then span lines anywhere, mid-row included (it sits in a side block beside stacked lines, stretched to their height). Only a pinwheel, which no straight cut can split, is refused, by the engine and the builder, with "These cells overlap in a way that can't be laid out; move one cell." on its first cell.
- **R29. Amount params (F18).** A param of type `amount` is a whole number of cents with a currency code (`{"amount": 150000, "currency": "USD"}`), or a bare number for any currency. It is money: against an amount with its own currency (a currency key, a fixed or carried currency), a comparison or a computed value holds only where the currencies match (`CASE WHEN <currency> IS <param's currency> THEN … END`), so a USD budget never meets EUR totals. The URL writes `p-budget=1500.00 USD`.
- **R30. Whole groups (F19).** "All <group>" adds `{"group": "<value>"}` to the choice's value; the server expands it at run time to every option of that group the reader may list (the options select under the reader's rules), so a group has no size cap and follows rule changes. Explicit values keep the 1000 cap.
- **R31. Link labels (F22).** A column whose value is a relation with a target shows that record's label: the column's `labelField`, else the target's first text field, else its first other field, else the target source's name; a select label field shows its option's label; a visible record with a blank label field reads "(no name)"; a blank or hidden link reads "(none)". The engine sends it as `<col>@label` (`ResultColumn.labelCell`); a detail row labelled by such a column and a record group's default label use the same rule.

### [platform] web-only or native-only items

None new is web-only or native-only by design: wrapped rows are flex lines (web and native); the param picker list pages with `ScrollView` `onScroll` near the end (web and native); every popover and menu is a core component that becomes a sheet on phone. FlashList v2 runs on web in this fork (`src/isNewArch.ts:19` treats web as the new architecture; `measureLayout.web.ts` and `PlatformHelper.web.ts` measure DOM elements; core's emoji picker already uses it on web), with one web caveat: sticky headers animate with the JS driver on web (`useNativeDriver: Platform.OS !== 'web'`, as React Native Web has no native driver) — drawback: on a slow web device the stuck group can trail the scroll by a frame. Inherited, unchanged and already signed off: `DocumentTitle` (web tab title only), `web:[scrollbar-gutter:stable]` on the table scrollers (changes nothing on native), `useLeaveGuard`'s browser Back/Forward half (web only).

### Open questions (recommended answer first)

- **Q1. `Source.Watch` (R14).** The spec's "settings change re-runs" needs it: `Heading` and `FiscalStartMonth` read a settings row by Go, which no SQL topic covers. **Recommended: add it.**
- **Q2. Having on a layout's first group (R7).** **Recommended: refuse** (sections are fixed rows; a hidden section would leave a computed row reading nothing).
- **Q3. Fill on deeper levels (R6).** **Answered: every level**, under each parent group.
- **Q4. `lookup` `pick` (R5).** **Recommended: optional, default `min`** (deterministic, as the Trial Balance writes it).
- **Q5. Amounts with a per-row currency in `record`/`lookup` (R3).** **Answered: build them**; the amount carries its currency.
- **Q6. `total: "rows"` on running values and percents (R8).** **Recommended: refuse** (a sum of running sums means nothing).
- **Q7. Correlated lookups (R4).** **Recommended: allow** (a rate on the row's day is a real need).
- **Q8. Automatic wrapping on desktop (R23).** **Recommended: no** — desktop keeps one line and sideways scroll; authors may arrange desktop.
- **Q9. Author placement per width in spec v2 with builder support (R24).** **Recommended: yes** (the Arrange section), per the no-limits rule.
- **Q10. General `lines` (R28).** **Answered: guillotine cuts**; only a pinwheel is refused.

The scroll position on web: the pinned FlashList fork's web correction raced the browser's own scroll anchoring from a stale offset; Tasks 25a–25d fix it in the fork (`overflow-anchor: none` on the list's scroller, a correction from the DOM scroller's current `scrollTop` before paint), stop for the human to merge the fork PR, pin the merged commit, and add a web e2e.

---

## Global Constraints

- Core names no package: no feature slug, scope, collection name or `/api/<slug>/` route in `core/`, `app/`, `cli/`, `scripts/`, `server/`. `PATH=$WS/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts` (in `$WS/tinycld`) stays at 0 references. Go tests use the fictional `zoo_*` collections; e2e uses the `salesstub` stub under `tests/`.
- All screens support web and native at desktop, tablet and phone widths.
- Data rules (CLAUDE.md): pbtsdb for collection data; `useMutation` from `@tinycld/core/lib/mutations`; no raw PocketBase writes in components, hooks or e2e (the one existing superuser seed in `reports-share.spec.ts` stays the only one); row types derive from queries; Zustand only for shared UI state; no `useState` + `useEffect` sync (the table's measured width is local layout state from `onLayout`); every form uses zod through `zodResolver`; RN 0.88 `onPress` wraps `handleSubmit`: `onPress={() => void handleSubmit(fn)()}`; JSX stays minimal with `isVisible` props; semantic colour tokens only; no `any`, no `biome-ignore`, no `console.*`.
- zod v4: a recursive schema is typed `z.ZodType<T, T>`.
- Report column ids the builder makes are lowercase snake_case, at most 40 characters (forms `keySchema`; `nextColumnId`).
- Go: only `errors.Is(err, sql.ErrNoRows)` means "not found"; any other error is a fault, logged and answered 500.
- Spec limits (engine): 100 columns, 4 link hops, 10 sort keys, 6 group levels, 200 layout items, labels ≤ 200, ids `^[A-Za-z][A-Za-z0-9_]{0,63}$`, 20 params, text params ≤ 500, formula depth 32 and 1000 nodes; new: choice values ≤ 1000, param-options limit ≤ 1000, search ≤ 200, display layouts ≤ 3 (one per width), tracks 1–12, lines 1–8.
- Front-end behaviour is tested by Playwright on the `salesstub` stub at desktop and at phone 390×844 where the phone path runs different code; vitest only for non-UI logic (every `lib/` module, the row window). Go code keeps Go tests (`-tags no_ui`).
- E2E: never `page.goto()` for in-app moves; set `E2E_PORT=7210 E2E_PORT_2=7211` before every e2e command; no timeouts bumped, no forced-serial runs; every created record uses `uniqueName`.
- Run tools from `$WS/node_modules/.bin` (`$WS` = `/Users/nas/tmp/tinycld/books-purchases`); `pnpm exec` starts a workspace install. Never run `pnpm install`, except in Task 25d, where the human asked for the install that brings in the merged FlashList fork.
- Commits: repo `$WS/tinycld`, branch `books/p5-trial-balance`; messages never mention Claude. Every task's commit step runs the member's full check (`tinycld-pkg check`: biome check --write, tsc, all vitest) and pushes.
- Before each lint step, run `PATH=$WS/node_modules/.bin:$PATH biome format --write <the task's new and changed files>` from `$WS/tinycld`; the plan's code blocks are not wrapped to Biome's 100 columns.

## Review Focus

1. A v1 saved report (or the v1 shipped "Stub larger side") is opened after this PR → it runs with no "upgraded" notice and its next save stores version 2 (Task 2 `TestAV1SpecLoadsSilentlyAsVersion2`; Task 20 e2e on the shipped copy).
2. A record group whose linked row the reader cannot see → the group's key reads blank, its label "(none)", and no field of the hidden row shows (Task 5 `TestARecordTheReaderCannotSeeIsBlank`).
3. `having` hides a group in a report with two currencies → each currency's `rows` total adds only its shown rows, the `recompute` total still adds every row (Task 8 `TestRowsTotalsAddOnlyShownRowsPerCurrency`).
4. param-options for a reader who cannot list some of the source's rows → those rows never appear, by search, by group or by `values` (Task 13 `TestParamOptionsReadOnlyRowsTheReaderMayList`).
5. On a phone, Collapse all on a report whose groups open by default, then a live re-run → the groups stay closed (Task 16 `rows.test.ts` "collapse sends an empty expand"; Task 22 e2e).

## Deferrals

Deferrals: none. Every item of spec §4 (E1–E13, OQ2), the human's E14 and the human's answers of 2026-11-01 are built in this plan. Task 25c is a STOP for the human's review of the FlashList fork PR, not a deferral.

## What was probed

- `cd $WS/tinycld && PATH=$WS/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/spec.test.ts` runs one vitest file from the member root (9 passed): arguments after `--` go to vitest.
- `cd $WS/tinycld/core/server && go test -tags no_ui -count=1 -run 'TestGroupsLoadLazily$' ./reports/` passes (0.8 s).
- Go `encoding/json` decodes an absent `expand` to a nil slice and `"expand": []` to a non-nil empty slice (a scratch program printed `true false 0`), so R10 can tell them apart.
- SQLite accepts a CTE that references a CTE written later in the same `WITH`, and a correlated `(SELECT COALESCE(sum(v),0) FROM b WHERE b.k IS a.k …)` inside a CTE (`sqlite3` printed `2` and `1|12`, `2|0`), so `o<L>` may read `o<L+1>` without reordering the level CTEs.
- FlashList v2.3.3 (the pinned `nathanstitt/flash-list` fork, `node_modules/@shopify/flash-list/src`): web is supported (`isNewArch.ts:19`, `measureLayout.web.ts`, `PlatformHelper.web.ts` with `supportsOffsetCorrection: false`); `stickyHeaderIndices` sticks one item at a time, pushed by the next (`recyclerview/components/StickyHeaders.tsx:80-150`), and the stuck copy is rendered by `renderItem` with `target: 'StickyHeader'`; `onEndReached` is checked after each commit and re-armed when `data` changes (`recyclerview/hooks/useBoundDetection.ts:96-108,165-170`, `RecyclerView.tsx:640`), so a short first page asks for the next.
- `core/server/reports/formula_cases_test.go` and the forms readers (`core/server/forms/formula_cases_test.go`, `core/lib/forms/__tests__/formula-cases.test.ts`) read only cases with a `forms` formula, so report-only cases with new keys do not disturb them.

---

## File Structure

| File | Responsibility |
|---|---|
| `core/server/reports/registry.go` | `Field.TextDay`, `Source.Heading`, `Source.Watch` |
| `core/server/reports/check.go` | text day columns, watched collections |
| `core/server/reports/spec.go` | version 2, identity step, new fields, alias resolution (`aliaser`) |
| `core/server/reports/bind.go` | link-ending paths, new field validation, `display` validation |
| `core/server/reports/formula.go` | `expr.target`, `record` and `lookup` dispatch, target rules |
| `core/server/reports/record.go` (new) | group targets, `record` leaf SQL, default labels |
| `core/server/reports/lookup.go` (new) | `lookupCtx`, `lookupIn` |
| `core/server/reports/levels.go` | having (`__own`), labels (`__label`), fill keyset, `rows` totals, formula sort keys |
| `core/server/reports/run.go` | legs with visibility, detail label and formula sort, expansion `open`, heading, watched topics |
| `core/server/reports/assemble.go` | group labels, `labelColumn`, fill rows that cannot open |
| `core/server/reports/layout.go` | computed legs carry `__own`, `__label`, `__n` |
| `core/server/reports/pivot.go` | `rows` totals on pivot cells |
| `core/server/reports/params.go` | `optionsFrom` defs and values |
| `core/server/reports/options.go` (new) | param-options query and handler |
| `core/server/reports/endpoint.go` | binds `POST /api/reports/param-options` |
| `core/server/reports/shipped.go` | `Shipped.Unpublished` |
| `core/server/reports/*_test.go` | Go tests per extension (new: `textday_test.go`, `relation_test.go`, `lookup_test.go`, `record_test.go`, `fill_test.go`, `having_test.go`, `rowstotal_test.go`, `sortformula_test.go`, `detaillabel_test.go`, `open_test.go`, `options_test.go`, `heading_test.go`, `v2_test.go`) |
| `core/lib/forms/fixtures/formula-cases.json` | `lookup` and `record` rows |
| `core/reports/lib/*.ts`, `core/reports/lib/builder/*.ts` | client lib (above), with vitest |
| `core/reports/lib/table-lines.ts` (new) | E14 line layouts (flex lines, edge cells) |
| `core/lib/forms/builder/formula-tree.ts` (new), `core/components/forms-builder/FormulaEditor.tsx` | generic formula tree, `operators` prop |
| `core/reports/components/builder/report-formula.tsx` (new) | the report operator set with its leaf controls |
| `core/reports/components/builder/ConditionTree.tsx` (new) | the filter editor, reusable for having and fill |
| `core/reports/components/builder/ArrangeSection.tsx` (new) | E14 Arrange section |
| `core/reports/components/builder/*Section.tsx`, `ColumnPicker.tsx`, `parts.tsx`, `ReportBuilderPanel.tsx` | builder controls |
| `core/reports/components/ParamPickers.tsx`, `hooks/useParamOptions.ts` (new) | the searching source picker, chip labels |
| `core/reports/components/ReportTable.tsx`, `ReportRow.tsx`, `report-table-layout.ts`; `lib/sticky.ts` (new); `use-row-window.ts` (deleted) | E14 flex lines on FlashList, sticky heading and groups, sideways scroll |
| `core/reports/components/ReportDocumentHeader.tsx`, `hooks/useReportView.ts`, `stores/report-ui-store.ts` | heading, expansion default |
| `tests/scripts/reports-stub/**` | stub: text day, region, heading, second shipped report |
| `tests/e2e/report-view-helpers.ts` (new), `tests/e2e/reports-helpers.ts`, `core/e2e-helpers.ts` | E11 |
| `tests/e2e/reports-formulas-v2.spec.ts`, `reports-groups-v2.spec.ts`, `reports-source-params.spec.ts`, `reports-wrap.spec.ts` (new), `reports-shipped.spec.ts` | e2e |
| `core/help/reports.md`, `docs/packages.md` | help and package docs |

---

## Tasks

**Run the tasks in this order** (the numbers name the tasks; the order is binding): 0, 1, 2, 3, 4, 5, 5b, 9, 5a, 6, 7, 8, 10, 11, 12, 12a, 13, 13a, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 25a, 25b, 25c (STOP), 25d, 26, 27, 28. Task 9 (formula sort keys) runs before 5a and 6 because their tests and walks use formula sort keys; Task 5b (link labels) follows Task 5, whose record helpers it uses.

### Task 0: Preflight

**Files:** none.

- [ ] **Step 1: Branch and tree**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && git branch --show-current && git status --short && git log --oneline -2`
Expected: `books/p5-trial-balance`; a clean tree; the plan's own commits (four or more, the newest the plan's latest revision) on top of `7a21579b`, which is an ancestor (`git merge-base --is-ancestor 7a21579b HEAD` exits 0).

- [ ] **Step 2: Baseline checks are green**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: both pass. A failure is fixed at its root before Task 1 (global rule), never skipped.

---

### Task 1: E1 — a Date field over a text day column (`Field.TextDay`)

Engine facts used: `validateField` (`registry.go:217-255`), `checkField` and `compatible` (`check.go:97-146`), `fieldSQL` reads a date as its first ten characters (`stmt.go:85-94`), so a text `YYYY-MM-DD` reads the same.

**Files:**
- Modify: `core/server/reports/registry.go` (Field, `validateField`)
- Modify: `core/server/reports/check.go:116` (`checkField`)
- Modify: `core/server/reports/zoo_test.go` (a text day column `due` on `zoo_feedings`)
- Create: `core/server/reports/textday_test.go`

**Interfaces:**
- Produces: `reports.Field.TextDay bool` (Books registers `date` and `auto_reverse_on` with it; later zoo tests filter on `due`).

- [ ] **Step 1: Give the zoo a text day column**

In `zoo_test.go`, add `&core.TextField{Name: "due"},` to the `feedings.Fields.Add(...)` list after `&core.TextField{Name: "note"},`; in `(z *zoo) feeding`, add `"due": day,` to the record data; in `registerZooSources`, add to `zoo.feedings` Fields after `note`:

```go
			{Key: "due", Label: "Due", Type: Date, TextDay: true, Aggregates: MinMax},
```

- [ ] **Step 2: Write the failing tests**

Create `core/server/reports/textday_test.go`:

```go
package reports

import (
	"strings"
	"testing"
)

// A text column that holds YYYY-MM-DD reads like a date: compared, bucketed
// and grouped by its day.
func TestATextDayReadsLikeADate(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":1,"source":"zoo.feedings","columns":[
		{"id":"due","path":[{"field":"due"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"filter":{"op":"lte","args":[{"path":[{"field":"due"}]},{"value":"2026-02-10"}]},
		"groups":[{"id":"m","col":"due","bucket":"month"}]}`, Options{}, nil)
	requireRows(t, res, "label", "2026-01", "2026-02", "Total")
	requireRows(t, res, "cost", int64(1700), int64(2500), int64(4200))
	if res.Columns[0].Type != Date {
		t.Fatalf("due column = %+v", res.Columns[0])
	}
}

func TestCheckSourcesWantsATextColumnForATextDay(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.bad_day", Collection: "zoo_feedings", Label: "Bad",
		Fields: []Field{{Key: "grams", Label: "Grams", Type: Date, TextDay: true}}})
	err := CheckSources(z.app)
	if err == nil || !strings.Contains(err.Error(), `column "grams" is a number field`) {
		t.Fatalf("err = %v", err)
	}
}

func TestOnlyADateFieldReadsATextDay(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "only a date field reads a text day") {
			t.Fatalf("recover = %v", r)
		}
	}()
	RegisterSource(Source{Key: "zoo.bad_text", Collection: "zoo_feedings", Label: "Bad",
		Fields: []Field{{Key: "note", Label: "Note", Type: Text, TextDay: true}}})
}
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'TextDay' ./reports/`
Expected: FAIL to compile: `unknown field TextDay in struct literal`.

- [ ] **Step 4: Implement**

In `registry.go`, add to `Field` after `Options`:

```go
	// TextDay, on a Date field, reads a text column that holds a day as
	// YYYY-MM-DD, for a package that stores days as text. The engine reads its
	// first ten characters, as it reads a date column's day.
	TextDay bool
```

In `validateField`, after the type switch:

```go
	if f.TextDay && f.Type != Date {
		return fmt.Errorf("only a date field reads a text day")
	}
```

In `check.go` `checkField`, replace the `compatible` check (line 116) with:

```go
	want := compatible[f.Type]
	if f.TextDay {
		want = []string{core.FieldTypeText}
	}
	if !slices.Contains(want, pf.Type()) {
		return fmt.Sprintf("column %q is a %s field, which a %s field cannot read", f.Column, pf.Type(), f.Type)
	}
```

- [ ] **Step 5: Run the reports tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS (the fuzz seeds, catalog and check tests still pass with the new field).

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports/registry.go core/server/reports/check.go core/server/reports/zoo_test.go core/server/reports/textday_test.go
git commit -m "feat(core): a report date field can read a text day column"
git push origin books/p5-trial-balance
```

---

### Task 2: E12 — spec version 2 with a silent identity upgrade

Engine facts used: `Load`/`loadWith`/`upgrade` (`spec.go:165-230`), the "upgraded" notice (`run.go:217-224`), `bind` refuses a version other than `CurrentVersion` (`bind.go:163-165`), saved specs are stored as `Load` returns them (`saved.go:120-131`). Client: `CURRENT_SPEC_VERSION` and the strict schema (`core/reports/lib/spec.ts:9,84`).

**Files:**
- Modify: `core/server/reports/spec.go:14-21,173-189`
- Modify: `core/server/reports/bind_test.go:114`, `saved_test.go:63`, `spec_test.go:107,170-188`
- Create: `core/server/reports/v2_test.go`
- Modify: `core/reports/lib/spec.ts:9`, `core/reports/lib/__tests__/spec.test.ts`

**Interfaces:**
- Produces: `CurrentVersion == 2`; `LoadInfo.UpgradedFrom` is 0 when no step changed the document (R1); client `CURRENT_SPEC_VERSION = 2`.

- [ ] **Step 1: Write the failing Go test**

Create `core/server/reports/v2_test.go`:

```go
package reports

import (
	"encoding/json"
	"testing"
)

// A version 1 spec loads unchanged as version 2: the step is the identity,
// so the run says nothing about an upgrade and a save stores version 2.
func TestAV1SpecLoadsSilentlyAsVersion2(t *testing.T) {
	z := newZoo(t)
	raw := `{"version":1,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}]}`
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != 2 || info.UpgradedFrom != 0 {
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
	if err := json.Unmarshal(stored, &doc); err != nil || doc["version"] != float64(2) {
		t.Fatalf("stored = %s", stored)
	}
}

// A step that changes the document still says so.
func TestAChangingStepIsNoticed(t *testing.T) {
	withZooRegistry(t)
	steps := map[int]upgradeStep{1: func(doc map[string]any) error {
		doc["columns"] = doc["cols"]
		delete(doc, "cols")
		return nil
	}}
	_, info, err := loadWith([]byte(`{"version":1,"source":"zoo.feedings","cols":[]}`), steps, 2)
	if err != nil || info.UpgradedFrom != 1 {
		t.Fatalf("info %+v, err %v", info, err)
	}
}

func TestAVersion3SpecIsRefused(t *testing.T) {
	_, _, err := Load([]byte(`{"version":3,"source":"zoo.feedings","columns":[]}`))
	requireSpecError(t, err, 400, "version", "newer than this server reads (2)")
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'V1Spec|ChangingStep|Version3' ./reports/`
Expected: FAIL: `version 1, upgradedFrom 0` and the version 3 message says `(1)`.

- [ ] **Step 3: Implement**

In `spec.go`, replace lines 14-21:

```go
// CurrentVersion is the spec version this engine writes. Load upgrades an
// older spec one version at a time with upgradeSteps.
const CurrentVersion = 2

// upgradeSteps[v] turns a version v document into version v+1. upgrade sets
// the version, not each step. Add a step whenever the spec format changes,
// and raise CurrentVersion with it.
//
// 1 → 2 adds only optional parts (column totals, group having, labels, fill
// and open, formula sort keys, the detail label, options from a source, the
// record and lookup formulas, display layouts), so a version 1 document is
// already a valid version 2 document.
var upgradeSteps = map[int]upgradeStep{
	1: func(map[string]any) error { return nil },
}
```

In `loadWith`, replace the `UpgradedFrom` assignment:

```go
	info := &LoadInfo{Aliased: resolveAliases(&spec)}
	// The notice says the report changed; a step that changed nothing (1 → 2)
	// gives none.
	if from != target && !sameExceptVersion(raw, upgraded) {
		info.UpgradedFrom = from
	}
```

Add below `upgrade`:

```go
// sameExceptVersion reports whether two spec documents differ only in their
// version. Numbers stay json.Number, so a large integer compares exactly.
func sameExceptVersion(a, b []byte) bool {
	read := func(raw []byte) (map[string]any, bool) {
		var doc map[string]any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if dec.Decode(&doc) != nil {
			return nil, false
		}
		delete(doc, "version")
		return doc, true
	}
	da, okA := read(a)
	db, okB := read(b)
	return okA && okB && reflect.DeepEqual(da, db)
}
```

(Add `"reflect"` to the imports.)

- [ ] **Step 4: Move the tests that pinned version 2 as the future**

- `bind_test.go:114`: the case `"version"` decodes `{"version":1,…}` directly (not loaded) and still expects `load the spec first`.
- `saved_test.go:63`: post `{"version":3}` and expect `"version":{"code":"invalid"`.
- `spec_test.go:107`: the case becomes `{"version":3,"columns":[]}` with `newer than this server reads (2)`.
- `spec_test.go:170-188` (`TestLoadKeepsLargeIntegersAcrossAnUpgrade`): load with `Load` (the real 1 → 2 step) and assert the value is `9007199254740993` and `info.UpgradedFrom == 0`; delete the stand-in `steps` and the comment that says CurrentVersion is 1.

- [ ] **Step 5: Client version**

In `core/reports/lib/spec.ts` set `export const CURRENT_SPEC_VERSION = 2`. In `core/reports/lib/__tests__/spec.test.ts` add:

```ts
    it('reads a version 1 spec and a version 2 spec', () => {
        expect(parseSpec({ ...FEEDINGS, version: 1 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 2 })).not.toBeNull()
        expect(parseSpec({ ...FEEDINGS, version: 3 })).toBeNull()
    })
```

- [ ] **Step 6: Run**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports`
Expected: PASS (the builder now writes `version: 2`; `draft.test.ts` compares against `CURRENT_SPEC_VERSION` where it builds a spec; a literal `version: 1` it expects from `specFromDraft` changes to `CURRENT_SPEC_VERSION`).

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports/spec.go core/server/reports/v2_test.go core/server/reports/bind_test.go core/server/reports/saved_test.go core/server/reports/spec_test.go core/reports/lib/spec.ts core/reports/lib/__tests__/spec.test.ts core/reports/lib/builder/__tests__/draft.test.ts
git commit -m "feat(core): report spec version 2; version 1 upgrades unchanged"
git push origin books/p5-trial-balance
```

---

### Task 3: E4 (part 1) — relation values that carry their target source

Engine facts used: `binder.path` ends a value path only with a field (`bind.go:342-412`); `scope.value` builds a path's SQL (`stmt.go:142-145`); `rowsCTE.colValue` and `rowsCtx.path` carry a path's type into `expr` (`rows.go:169-196`, `detail.go:52-58`); `levelCtx.col`/`path` (`levels.go:220-354`); `if`, `case`, `coalesce` type their results with `commonType` (`formula.go:533-584`).

**Files:**
- Modify: `core/server/reports/bind.go` (`pathPlan`, `binder.path`)
- Modify: `core/server/reports/formula.go` (`expr.target`, `oneTarget`, if/case/coalesce)
- Modify: `core/server/reports/stmt.go:143` (`scope.value`), `rows.go:176`, `detail.go:57`, `levels.go:231-242,353`, `assemble.go:67-75` (`labelOf`)
- Create: `core/server/reports/relation_test.go`

**Interfaces:**
- Produces: `pathPlan.target *Source`, `pathPlan.endsAtLink bool`; `expr.target *Source`; `oneTarget(values []expr, op, at string) (*Source, error)`. A path `[…, {"link": L}]` (L to-one) or `[…, {"field": "id"}]` is a relation value; Tasks 4–6 read `target` to make record groups.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/relation_test.go`:

```go
package reports

import "testing"

// A path that ends at a to-one link reads the linked row's id.
func TestAPathEndingAtALinkReadsTheLinkedRowsID(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}]}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "animal", z.id("leo"), z.id("leo"), z.id("zed"), z.id("zed"), nil)
	if c := res.Columns[1]; c.Type != Relation || c.Label != "Animal" {
		t.Fatalf("animal column = %+v", c)
	}
}

// A linked row the reader cannot see reads blank, as every to-one value does.
func TestALinkToAHiddenRowReadsBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"vault","path":[{"link":"vault"}]}],"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "name", "Kit", "Leo", "Zed", nil)
	requireRows(t, res, "vault", nil, nil, nil, nil)
}

func TestAFilterComparesARelation(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"value":"`+z.id("zed")+`"}]},
		"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "note", "f3", "f4", nil)
}

func TestIfOverRelationsNeedsOneTarget(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"if","args":[
		{"value":true},{"path":[{"link":"animal"}]},{"path":[{"link":"animal"},{"link":"site"}]}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.0.formula", "link to different sources")
}

func TestATotalAcrossAToManyLinkStillEndsWithAField(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.animals","columns":[{"id":"n","path":[{"link":"feedings","mode":"aggregate"}],"aggregate":"count"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.0.path", "or with a link to one row")
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'LinkedRowsID|HiddenRowReadsBlank|ComparesARelation|OneTarget|StillEndsWithAField' ./reports/`
Expected: FAIL: `a path ends with a field` for every link-ending path.

- [ ] **Step 3: Implement the binder part**

In `bind.go`, add to `pathPlan`:

```go
	// target is the source a relation value points at: the linked source of
	// a path that ends at a to-one link, or the source whose id it reads.
	target *Source
	// endsAtLink is true for a path whose last step is a link.
	endsAtLink bool
```

In `binder.path`, in the field branch after `out.field == nil` is checked, add:

```go
			if out.field.Key == idField.Key {
				out.target = src
			}
```

Replace the end of `binder.path` (the `if kind != pathLinks { b.bad(at, "a path ends with a field") … }` block, `bind.go:406-409`) with:

```go
	if kind != pathLinks {
		last := out.hops[len(out.hops)-1]
		if last.link.Cardinality != ToOne {
			b.bad(at, "a path ends with a field, or with a link to one row")
			return nil
		}
		// A path that ends at a to-one link reads the linked row's id through
		// the join: a relation to the target, blank when the reader cannot
		// see that row.
		out.field, out.src, out.target, out.endsAtLink = src.field(idField.Key), src, src, true
		return out
	}
```

- [ ] **Step 4: Carry the target through every context**

In `formula.go`, add to `expr` after `cur currency`:

```go
	// target is the source a relation value points at, nil for any other
	// value. A group by such a value is a record group.
	target *Source
```

Add below `combined`:

```go
// oneTarget is the source the relations among values point at; two that
// point at different sources are refused, since a record group could not
// read both.
func oneTarget(values []expr, op, at string) (*Source, error) {
	var out *Source
	for _, v := range values {
		if v.target == nil {
			continue
		}
		if out != nil && out.Key != v.target.Key {
			return nil, invalid(at, fmt.Sprintf("the results of %s link to different sources", op))
		}
		out = v.target
	}
	return out, nil
}
```

In `operatorExpr`: in `case "if"`, before the return, `target, err := oneTarget(args[1:], "if", at)`, return the error, and set `target: target` on the returned expr (build it in a variable, wrap with `wholeCents`, then set `.target`). Do the same in `case "case"` with `results` and in `case "coalesce"` with `args` (both the one-argument and the many-argument return).

Set the target where a path's value is built:
- `stmt.go:143` `scope.value`: `return expr{sql: fieldSQL(sc.join(p.hops), p.field), typ: p.field.Type, target: p.target}`.
- `rows.go:176` `colValue`: after building `ref`, `if cp.tm < 0 { ref.target = cp.path.target }`.
- `detail.go:57` `rowsCtx.path`: add `target: bound.target` to the expr.
- `levels.go` `levelCtx.col`, the `case cp.path != nil:` branch: `expr{sql: sql, typ: cp.path.field.Type, target: cp.path.target}`; `levelCtx.path`: add `target: bound.target`.

In `assemble.go` `labelOf`, a path that ends at a link is named by the link:

```go
	case cp.path != nil && cp.path.endsAtLink:
		return cp.path.hops[len(cp.path.hops)-1].link.Label
	case cp.path != nil:
		return cp.path.field.Label
```

- [ ] **Step 5: Run the reports tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports/bind.go core/server/reports/formula.go core/server/reports/stmt.go core/server/reports/rows.go core/server/reports/detail.go core/server/reports/levels.go core/server/reports/assemble.go core/server/reports/relation_test.go
git commit -m "feat(core): report paths may end at a to-one link and carry its source"
git push origin books/p5-trial-balance
```

---

### Task 4: E6 — the `lookup` operator

Engine facts used: `compileNode` and `nodeKinds` (`formula.go:106-196`); the `fctx` interface (`formula.go:79-94`) and `ctxBase` (`formula.go:756-781`); a new scope reads a source's rule CTE (`stmt.go:108-115`, `stmt.go:41-51`); `scope.exists` (`stmt.go:207-240`); `compileWhere` (`stmt.go:242-251`); the binder's formula size count (`bind.go:296-315`); the formula-table harness (`formula_cases_test.go:56-160`); the rows-CTE, joins and levels each own a `*rowsCTE` with its statement (`rows.go:58-66`, `join.go:46-54`, `levels.go:17-30`).

**Files:**
- Modify: `core/server/reports/spec.go` (`Node.Source`, `Node.Pick`)
- Modify: `core/server/reports/formula.go` (`fctx.lookup`, dispatch, `ctxBase.lookup`)
- Create: `core/server/reports/lookup.go`
- Modify: `core/server/reports/join.go`, `detail.go`, `levels.go` (`lookup` methods), `layout.go` (`layoutCtx.lookup`), `bind.go:296-315` (`nodeSize` counts a lookup's value)
- Modify: `core/server/reports/formula_cases_test.go` (harness: `lookupTypes`, `lookupRows`, `tableCtx.lookup`)
- Modify: `core/lib/forms/fixtures/formula-cases.json` (four `lookup` cases)
- Create: `core/server/reports/lookup_test.go`

**Interfaces:**
- Consumes: `expr.target`, `oneTarget` (Task 3).
- Produces: `Node.Source string` (`json:"source,omitempty"`), `Node.Pick string` (`json:"pick,omitempty"`); `fctx.lookup(n *Node, at string) (expr, error)`; `lookupValue(n *Node, at string) (*Node, error)`; `lookupIn(outer fctx, r *rowsCTE, n *Node, at string) (expr, error)`; `lookupCtx`.

- [ ] **Step 1: Write the failing Go tests**

Create `core/server/reports/lookup_test.go`:

```go
package reports

import (
	"slices"
	"testing"
)

const siteNorth = `{"op":"lookup","source":"zoo.sites",
	"where":{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]},
	"value":{"path":[{"field":"name"}]}}`

// A lookup reads one value from another source's rows the reader may list.
func TestALookupReadsAnotherSourceUnderTheReadersRules(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"site","formula":` + siteNorth + `}],"sort":[{"col":"note"}]}`
	res := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, res, "site", "North", "North", "North", "North", nil)
	if !slices.Contains(res.Topics, "collection/zoo_sites") {
		t.Fatalf("topics = %v", res.Topics)
	}
	// zoo_sites lists only members: a guest's lookup finds nothing.
	guest := z.person(t, "gus", "guest")
	z.rec(t, "kg", "zoo_keepers", map[string]any{"user": guest.Id, "animal": z.id("leo")})
	res = z.run(t, "gus", spec, Options{}, nil)
	requireRows(t, res, "site", nil, nil, nil)
}

// A lookup that reads a report column is correlated: each row looks up its own.
func TestALookupMayReadTheRowsColumns(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"weight","formula":{"op":"lookup","source":"zoo.animals",
			"where":{"op":"eq","args":[{"path":[{"field":"id"}]},{"col":"animal"}]},
			"value":{"path":[{"field":"weight"}]}}}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "weight", int64(190), int64(190), int64(300), int64(300), nil)
}

func TestALookupInTheFilterPicksMinOrMaxAndIsBlankWithNoMatch(t *testing.T) {
	z := newZoo(t)
	zedID := `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Zed"}]},"value":{"path":[{"field":"id"}]}}`
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":[{"link":"animal"}]},`+zedID+`]},"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "note", "f3", "f4", nil)

	heaviest := `{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"weight"}]},"pick":"max"}`
	none := `{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Nobody"}]},"value":{"path":[{"field":"weight"}]}}`
	res = z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"heaviest","formula":`+heaviest+`},{"id":"none","formula":`+none+`}],"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "heaviest", int64(300), int64(300), nil)
	requireRows(t, res, "none", nil, nil, nil)
}

// A computed layout row may look a value up (F16): here a section's cost
// plus the weight of Leo.
func TestALayoutRowLooksAValueUp(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"}],
		"layout":[{"section":"l","title":"Lions","key":"lion"},
			{"row":"r","label":"Lions and Leo","formula":{"op":"add","args":[{"subtotal":"l"},
				{"op":"lookup","source":"zoo.animals","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"Leo"}]},"value":{"path":[{"field":"weight"}]}}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Lions and Leo")
	requireRows(t, res, "cost", int64(3500), int64(3500), int64(3690))
}

func TestALookupIsRefusedWhereItCannotBeRead(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]struct{ spec, at, want string }{
		"unknown source": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.gone","value":{"path":[{"field":"id"}]}}}]}`,
			"columns.0.formula.source", "does not exist"},
		"a total inside": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","value":{"op":"sum","args":[{"path":[{"field":"weight"}]}]}}}]}`,
			"columns.0.formula.value", "one value per matching row"},
		// Task 5a builds this (Q5) and deletes the case.
		"an amount with a row currency": {`{"version":2,"source":"zoo.animals","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.feedings","value":{"path":[{"field":"cost"}]}}}]}`,
			"columns.0.formula.value.path", "read it through a column"},
		"a bad pick": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"id"}]},"pick":"avg"}}]}`,
			"columns.0.formula.pick", "min or max"},
		"a source on another op": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"add","source":"zoo.animals","args":[{"value":1},{"value":2}]}}]}`,
			"columns.0.formula", "only lookup takes a source"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatal(err)
			}
			err = Validate(app, spec)
			status := 400
			if name == "unknown source" {
				status = 422
			}
			requireSpecError(t, err, status, c.at, c.want)
		})
	}
}
```

(`z.person` is the saved-zoo helper at `saved_fixture_test.go:62`; it is a method on `*zoo` and works on any zoo.)

- [ ] **Step 2: Add the shared formula-table cases**

Append to `cases` in `core/lib/forms/fixtures/formula-cases.json` (report-only: no `forms` key; the forms readers skip them):

```json
{"name": "lookup picks the smallest matching value", "types": {"c": "text"}, "values": {"c": "a"},
 "lookupTypes": {"code": "text", "rate": "number"},
 "lookupRows": [{"code": "a", "rate": 3}, {"code": "a", "rate": 1}, {"code": "b", "rate": 9}],
 "formula": {"op": "lookup", "source": "table", "where": {"op": "eq", "args": [{"path": [{"field": "code"}]}, {"col": "c"}]}, "value": {"path": [{"field": "rate"}]}},
 "expect": 1},
{"name": "lookup pick max takes the largest", "types": {"c": "text"}, "values": {"c": "a"},
 "lookupTypes": {"code": "text", "rate": "number"},
 "lookupRows": [{"code": "a", "rate": 3}, {"code": "a", "rate": 1}],
 "formula": {"op": "lookup", "source": "table", "where": {"op": "eq", "args": [{"path": [{"field": "code"}]}, {"col": "c"}]}, "value": {"path": [{"field": "rate"}]}, "pick": "max"},
 "expect": 3},
{"name": "lookup with no match is empty", "types": {"c": "text"}, "values": {"c": "z"},
 "lookupTypes": {"code": "text", "rate": "number"}, "lookupRows": [{"code": "a", "rate": 3}],
 "formula": {"op": "lookup", "source": "table", "where": {"op": "eq", "args": [{"path": [{"field": "code"}]}, {"col": "c"}]}, "value": {"path": [{"field": "rate"}]}},
 "expect": null},
{"name": "lookup inside arithmetic", "types": {}, "values": {},
 "lookupTypes": {"rate": "number"}, "lookupRows": [{"rate": 4}],
 "formula": {"op": "add", "args": [{"op": "lookup", "source": "table", "value": {"path": [{"field": "rate"}]}}, {"value": 1}]},
 "expect": 5}
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Lookup|FormulaCases' ./reports/`
Expected: FAIL to load: `json: unknown field "source"` (strict decode).

- [ ] **Step 4: Implement the node fields and the dispatch**

In `spec.go`, add to `Node` after `Where`:

```go
	// Source and Pick belong to lookup: the source it reads, and "min" (the
	// default) or "max" of the matching rows' value. A lookup's Value is a
	// formula node over that source, not a literal.
	Source string `json:"source,omitempty"`
	Pick   string `json:"pick,omitempty"`
```

In `formula.go`, add to `fctx`:

```go
	// lookup compiles a lookup node: a value from another source's rows.
	lookup(n *Node, at string) (expr, error)
```

In `compileNode`, before the `nodeKinds` check:

```go
	if n.Op == "lookup" {
		return c.lookup(n, at)
	}
	if n.Source != "" || n.Pick != "" {
		return expr{}, invalid(at, "only lookup takes a source and a pick")
	}
```

Add to `ctxBase`:

```go
func (ctxBase) lookup(_ *Node, at string) (expr, error) {
	return expr{}, invalid(at, "a lookup is read in a column, the filter, a group, a sort or a label")
}
```

In `layout.go`, a computed row may look a value up (F16; R21's refusal is withdrawn): the lookup reads its source's rule CTE in the layout statement, with no report row to read (a column or a record inside it is refused as layout rows refuse them; params resolve directly):

```go
func (lc *layoutCtx) lookup(n *Node, at string) (expr, error) {
	return lookupIn(lc, &rowsCTE{c: lc.c, s: lc.s}, n, at)
}
```

In `bind.go` `nodeSize`, after `add(n.Where, false)`:

```go
	if n.Op == "lookup" {
		if v, err := lookupValue(n, ""); err == nil {
			add(v, false)
		}
	}
```

- [ ] **Step 5: Implement `lookup.go`**

```go
package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// lookupValue decodes a lookup's value: a formula node over the looked-up
// source.
func lookupValue(n *Node, at string) (*Node, error) {
	if n.Value == nil {
		return nil, invalid(at+".value", "a lookup names the value it reads")
	}
	var v Node
	dec := json.NewDecoder(bytes.NewReader(n.Value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return nil, invalid(at+".value", "a lookup's value is a formula: "+err.Error())
	}
	return &v, nil
}

// lookupIn compiles a lookup in a context whose rows CTE is r: the smallest
// (pick min, the default) or largest value of the node's value over the rows
// of its source that match its where, NULL when none match. Every row of the
// source is read through its rule CTE, so the reader's list rule applies.
// Inside, a path reads the looked-up source; a column, a group record or a
// param reads the report's row through outer, which makes the subquery
// correlated. SQLite runs an uncorrelated scalar subquery once per statement.
func lookupIn(outer fctx, r *rowsCTE, n *Node, at string) (expr, error) {
	if len(n.Args) > 0 || n.Link != nil || n.Scale != nil {
		return expr{}, invalid(at, "lookup takes a source, a where, a value and a pick")
	}
	if n.Pick != "" && n.Pick != "min" && n.Pick != "max" {
		return expr{}, invalid(at+".pick", "pick is min or max")
	}
	src := lookupSource(r.c.p.app, n.Source)
	if src == nil {
		return expr{}, unknownRef(at+".source", "unknown_source", fmt.Sprintf("the source %q does not exist", n.Source))
	}
	valueNode, err := lookupValue(n, at)
	if err != nil {
		return expr{}, err
	}
	sc := newScope(r.s, src)
	v, err := compileFormula(&lookupCtx{outer: outer, r: r, sc: sc, src: src}, valueNode, at+".value")
	if err != nil {
		return expr{}, err
	}
	if v.agg || v.win {
		return expr{}, invalid(at+".value", "a lookup reads one value per matching row: totals and running values go outside it")
	}
	where := "1"
	if n.Where != nil {
		w, err := compileWhere(&lookupCtx{outer: outer, r: r, sc: sc, src: src, inWhere: true}, n.Where, at)
		if err != nil {
			return expr{}, err
		}
		if w.agg || w.win {
			return expr{}, invalid(at+".where", "a lookup's where reads one row at a time")
		}
		if !w.unset {
			where = "COALESCE(" + w.sql + ", 0)"
		}
	}
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	sql := fmt.Sprintf("(SELECT %s(%s)\n%s\nWHERE %s)", pick, v.sql, sc.render(), where)
	return expr{sql: sql, typ: v.typ, target: v.target, cur: v.cur}, nil
}

// lookupCtx compiles inside a lookup: paths on the looked-up source (after
// prefix, inside an exists of the where), everything else through outer.
type lookupCtx struct {
	ctxBase
	outer   fctx
	r       *rowsCTE
	sc      *scope
	src     *Source
	prefix  []hopPlan
	inWhere bool
}

func (l *lookupCtx) col(id, at string) (expr, error) { return l.outer.col(id, at) }
// A param is the run's value wherever the lookup sits (a layout row's outer
// context reads no params, so a lookup there resolves them itself).
func (l *lookupCtx) param(id, at string) (expr, error) {
	v, err := paramOf(l.r.c, id, at)
	if err != nil {
		return expr{}, err
	}
	return paramExpr(l, v), nil
}
func (l *lookupCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(l, l.r, n, at) }

func (l *lookupCtx) path(p Path, at string) (expr, error) {
	bound, err := bindValuePath(l.r.c.p.app, l.src, p, at, pathValue)
	if err != nil {
		return expr{}, err
	}
	if bound.field.Type == Money && bound.field.Currency != "" {
		return expr{}, invalid(at, "this amount has its own row's currency, which totals cannot group by: read it through a column")
	}
	hops := append(append([]hopPlan{}, l.prefix...), bound.hops...)
	e := l.sc.value(&pathPlan{hops: hops, field: bound.field, src: bound.src, target: bound.target})
	if bound.field.Type == Money && bound.src.Currency != nil {
		e.cur = currency{fixed: bound.src}
	}
	return e, nil
}

func (l *lookupCtx) aggregate(op Aggregate, _ *Node, _ int, at string) (expr, error) {
	return expr{}, invalid(at, fmt.Sprintf("%s totals rows: a lookup reads one value per matching row", op))
}

func (l *lookupCtx) window(op string, _ *Node, _ int, at string) (expr, error) {
	return expr{}, invalid(at, op+" reads the rows a report shows: use it in a column")
}

func (l *lookupCtx) exists(n *Node, at string) (expr, error) {
	link, err := bindValuePath(l.r.c.p.app, l.src, n.Link, at+".link", pathLinks)
	if err != nil {
		return expr{}, err
	}
	mk := func(sc *scope, hops []hopPlan) fctx {
		src := sc.src
		if len(hops) > 0 {
			src = hops[len(hops)-1].to
		}
		return &lookupCtx{outer: l.outer, r: l.r, sc: sc, src: src, prefix: hops, inWhere: true}
	}
	return l.sc.exists(l.prefix, link, n.Where, mk, at)
}

func (l *lookupCtx) bind(v any) string   { return l.r.s.bind(v) }
func (l *lookupCtx) fiscalStart() string { return l.bind(int64(l.r.c.fiscal)) }
func (l *lookupCtx) filter() bool        { return l.inWhere }
```

Note on `lookupCtx.path` with a prefix: `bindValuePath` binds from `l.src`, the source the prefix ends on, and the hops are joined after the prefix, exactly as `joinCtx.path` does (`join.go:72-79`).

Add the method to the three row contexts:

```go
// join.go
func (j *joinCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(j, j.r, n, at) }
// detail.go
func (rc *rowsCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(rc, rc.r, n, at) }
// levels.go
func (lc *levelCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(lc, lc.lv.r, n, at) }
```

- [ ] **Step 6: Teach the formula-table harness lookups**

In `formula_cases_test.go`, add to `formulaCase`:

```go
	LookupTypes map[string]FieldType `json:"lookupTypes"`
	LookupRows  []map[string]any     `json:"lookupRows"`
```

Add to `tableCtx`: `lookupTypes map[string]FieldType` and `lookupRows []map[string]any`, set in `runFormulaCase` from the case. Add:

```go
// lookup reads the case's lookupRows as the looked-up source: a path is one
// of their columns, a column the table's own row.
func (c *tableCtx) lookup(n *Node, at string) (expr, error) {
	value, err := lookupValue(n, at)
	if err != nil {
		return expr{}, err
	}
	inner := &tableLookupCtx{outer: c}
	v, err := compileFormula(inner, value, at+".value")
	if err != nil {
		return expr{}, err
	}
	where := "1"
	if n.Where != nil {
		w, err := compileFormula(inner, n.Where, at+".where")
		if err != nil {
			return expr{}, err
		}
		where = "COALESCE(" + w.sql + ", 0)"
	}
	pick := "min"
	if n.Pick == "max" {
		pick = "max"
	}
	return expr{sql: fmt.Sprintf("(SELECT %s(%s) FROM (%s) l WHERE %s)", pick, v.sql, c.lookupTable(), where), typ: v.typ}, nil
}

func (c *tableCtx) lookupTable() string {
	cols := make([]string, 0, len(c.lookupTypes))
	for name := range c.lookupTypes {
		cols = append(cols, name)
	}
	sort.Strings(cols)
	if len(c.lookupRows) == 0 {
		parts := make([]string, len(cols))
		for i, name := range cols {
			parts[i] = "NULL AS " + name
		}
		return "SELECT " + strings.Join(parts, ", ") + " WHERE 0"
	}
	selects := make([]string, 0, len(c.lookupRows))
	for _, row := range c.lookupRows {
		parts := make([]string, len(cols))
		for i, name := range cols {
			parts[i] = c.bind(jsonValue(row[name])) + " AS " + name
		}
		selects = append(selects, "SELECT "+strings.Join(parts, ", "))
	}
	return strings.Join(selects, " UNION ALL ")
}

type tableLookupCtx struct {
	ctxBase
	outer *tableCtx
}

func (l *tableLookupCtx) col(id, at string) (expr, error) { return l.outer.col(id, at) }
func (l *tableLookupCtx) path(p Path, at string) (expr, error) {
	if len(p) != 1 || p[0].Field == "" {
		return expr{}, invalid(at, "a lookup row path is one field")
	}
	typ, ok := l.outer.lookupTypes[p[0].Field]
	if !ok {
		return expr{}, invalid(at, "unknown lookup column "+p[0].Field)
	}
	return expr{sql: "l." + p[0].Field, typ: typ}, nil
}
func (l *tableLookupCtx) param(string, string) (expr, error) { return expr{}, fmt.Errorf("no params") }
func (l *tableLookupCtx) aggregate(Aggregate, *Node, int, string) (expr, error) {
	return expr{}, fmt.Errorf("no totals in a lookup")
}
func (l *tableLookupCtx) window(string, *Node, int, string) (expr, error) {
	return expr{}, fmt.Errorf("no windows")
}
func (l *tableLookupCtx) lookup(n *Node, at string) (expr, error) { return l.outer.lookup(n, at) }
func (l *tableLookupCtx) bind(v any) string                    { return l.outer.bind(v) }
func (l *tableLookupCtx) fiscalStart() string                  { return l.outer.fiscalStart() }
func (l *tableLookupCtx) filter() bool                         { return false }
```

- [ ] **Step 7: Run the reports tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/ ./forms/`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/lib/forms`
Expected: PASS (the forms readers skip the new cases, which carry no `forms`).

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports core/lib/forms/fixtures/formula-cases.json
git commit -m "feat(core): the report lookup operator reads another source under the reader's rules"
git push origin books/p5-trial-balance
```

---

### Task 5: E4 (part 2) — record groups, the `record` leaf and group labels

Engine facts used: group keys are computed per base row in `rows()` (`rows.go:83-95`) by `rowValue` (`rows.go:220-232`); a level reads its keys as `a.k<i>` (`levels.go:155-163`, `levels.go:196-205`); a formula reading a row value is blank on a level (`levels.go:302-343`); the level CTE `o<L>` selects `__level, __row, k…, ck…, cells, __ord` (`levels.go:195-209`) and the computed layout legs select the same columns (`layout.go:54-87`); `group` labels a row with `display` (`assemble.go:200-208`).

**Files:**
- Modify: `core/server/reports/spec.go` (`Node.Record`, `Group.Label`)
- Modify: `core/server/reports/formula.go` (`fctx.record`, dispatch, `nodeKinds`, `ctxBase.record`)
- Create: `core/server/reports/record.go`
- Modify: `core/server/reports/rows.go` (`compiler.groupTargets`), `levels.go` (`levelCtx.record`, `readsRowValue`, `labelSQL`, `render` columns), `detail.go` (`rowsCtx.record`), `join.go` (`joinCtx.record`), `layout.go` (`layoutCtx.record`, legs gain three columns), `lookup.go` (`lookupCtx.record`), `run.go:384-423` (`groupStatement` passes the label), `assemble.go` (labels)
- Modify: `core/server/reports/formula_cases_test.go` (`recordTypes`, `record`), `core/lib/forms/fixtures/formula-cases.json` (two `record` cases)
- Create: `core/server/reports/record_test.go`

**Interfaces:**
- Consumes: `expr.target`, `pathPlan.target` (Task 3), `lookupCtx` (Task 4).
- Produces: `Node.Record string` (`json:"record,omitempty"`); `Group.Label *Node` (`json:"label,omitempty"`); `fctx.record(group string, p Path, at string) (expr, error)`; `compiler.groupTargets []*Source`; `(c *compiler) recordOf(group, at string) (int, *Source, error)`; `(c *compiler) groupIndex(id string) int`; `recordSQL(s *stmt, target *Source, bound *pathPlan, key string) string`; `firstTextField(src *Source) *Field`; `labelText(v any) string`. Every level row and computed leg now ends in `__own` (1 until Task 7), `__label`, `__n`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/record_test.go`:

```go
package reports

import "testing"

const byAnimal = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"species","formula":{"record":"g","path":[{"field":"species"}]}},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"cost"}]}`

// A group by a link carries the linked record: it labels the row by the
// record's first text field and a record leaf reads its fields on the group,
// on the rows below it, and nowhere outside it.
func TestARecordGroupLabelsItsRowsAndReadsTheRecord(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", byAnimal, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Leo", "Total")
	requireRows(t, res, "species", "zebra", "lion", nil)
	requireRows(t, res, "cost", int64(1000), int64(3500), int64(4500))
	if res.Rows[0].GroupKey[0] != z.id("zed") {
		t.Fatalf("group key = %v", res.Rows[0].GroupKey)
	}
	open := z.run(t, "ann", byAnimal, Options{Expand: [][]any{res.Rows[1].GroupKey}}, nil)
	requireRows(t, open, "species", "zebra", "lion", "lion", "lion", nil)
}

func TestAGroupLabelIsAFormula(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","label":{"op":"concat","args":[
			{"record":"g","path":[{"field":"name"}]},{"value":" at "},{"record":"g","path":[{"link":"site"},{"field":"name"}]}]}}],
		"sort":[{"col":"cost"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed at South", "Leo at North", "Total")
}

// A record the reader cannot see is a blank key and a blank label.
func TestARecordTheReaderCannotSeeIsBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"vault","path":[{"link":"vault"}]},
		{"id":"vname","formula":{"record":"g","path":[{"field":"name"}]}},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"vault"}]}`, Options{}, nil)
	requireRows(t, res, "label", "(none)", "Total")
	requireRows(t, res, "vname", nil, nil)
}

func TestARecordLeafIsRefusedWhereItCannotBeRead(t *testing.T) {
	app := withZooRegistry(t)
	cases := map[string]struct {
		spec, at, want string
		status         int
	}{
		"unknown group": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"record":"nope","path":[{"field":"name"}]}}]}`,
			"columns.0.formula", "has no group", 422},
		"not a record group": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"x","formula":{"record":"g","path":[{"field":"name"}]}}],"groups":[{"id":"g","col":"n"}]}`,
			"columns.1.formula", "is not grouped by a link", 400},
		"in the filter": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]}],"groups":[{"id":"g","col":"a"}],
			"filter":{"op":"eq","args":[{"record":"g","path":[{"field":"name"}]},{"value":"Leo"}]}}`,
			"filter.args.0", "before they are grouped", 400},
		"no path": {`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},{"id":"x","formula":{"record":"g"}}],"groups":[{"id":"g","col":"a"}]}`,
			"columns.1.formula", "names a field of the record", 400},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(c.spec))
			if err != nil {
				t.Fatal(err)
			}
			requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
		})
	}
}
```

Append two report-only cases to `formula-cases.json`:

```json
{"name": "a record field reads the group's record", "types": {}, "values": {},
 "recordTypes": {"name": "text"}, "record": {"name": "Cash"},
 "formula": {"record": "g", "path": [{"field": "name"}]}, "expect": "Cash"},
{"name": "if over a record field", "types": {}, "values": {},
 "recordTypes": {"kind": "text"}, "record": {"kind": "equity"},
 "formula": {"op": "if", "args": [{"op": "eq", "args": [{"record": "g", "path": [{"field": "kind"}]}, {"value": "equity"}]}, {"value": 1}, {"value": 2}]},
 "expect": 1}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Record|GroupLabel|FormulaCases' ./reports/`
Expected: FAIL to load: `json: unknown field "record"`.

- [ ] **Step 3: Spec fields and dispatch**

In `spec.go`, add to `Node` after `Subtotal`:

```go
	// Record names a group whose key is a link: the leaf reads Path from the
	// linked record, on that group's rows and every row below them.
	Record string `json:"record,omitempty"`
```

Add to `Group`:

```go
	// Label is a formula on the group's level that labels its rows. Without
	// it a record group is labelled by its record's first text field, any
	// other group by its key.
	Label *Node `json:"label,omitempty"`
```

In `formula.go`: add `record(group string, p Path, at string) (expr, error)` to `fctx`; in `compileNode`'s leaf switch, put first `case n.Record != "": return c.record(n.Record, n.Path, at)`; replace `nodeKinds` with:

```go
func nodeKinds(n *Node) int {
	kinds := 0
	// A record leaf's path is part of it; a lookup's value is not a literal.
	path := n.Path != nil && n.Record == ""
	value := n.Value != nil && n.Op != "lookup"
	for _, set := range []bool{n.Op != "", n.Col != "", path, n.Param != "", value, n.Subtotal != "", n.Record != ""} {
		if set {
			kinds++
		}
	}
	return kinds
}
```

and its message: `"a formula node is one of op, col, path, param, value, subtotal or record"`. Add to `ctxBase`:

```go
func (ctxBase) record(_ string, _ Path, at string) (expr, error) {
	return expr{}, invalid(at, "a record field is read on a group's rows, the rows below them, a sort, a label or a having")
}
```

- [ ] **Step 4: `record.go`**

```go
package reports

import (
	"fmt"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
)

// groupIndex is the index of the group with id, or -1.
func (c *compiler) groupIndex(id string) int {
	for i, g := range c.p.groups {
		if g.group.ID == id {
			return i
		}
	}
	return -1
}

// recordOf finds the group a record leaf names and the source its key
// links to.
func (c *compiler) recordOf(group, at string) (int, *Source, error) {
	gi := c.groupIndex(group)
	if gi < 0 {
		return 0, nil, unknownRef(at, "unknown_group", fmt.Sprintf("the report has no group %q", group))
	}
	if gi >= len(c.groupTargets) || c.groupTargets[gi] == nil {
		return 0, nil, invalid(at, fmt.Sprintf("the group %q is not grouped by a link: it has no record", group))
	}
	return gi, c.groupTargets[gi], nil
}

// bindRecordPath binds a record leaf's path on the group's target: to-one
// links, then a field. An amount needs its source's fixed currency here; Task
// 5a lifts that (Q5: the amount carries its own currency).
func bindRecordPath(app core.App, target *Source, p Path, at string) (*pathPlan, error) {
	if len(p) == 0 {
		return nil, invalid(at, "a record leaf names a field of the record")
	}
	bound, err := bindValuePath(app, target, p, at+".path", pathValue)
	if err != nil {
		return nil, err
	}
	if bound.field.Type == Money && bound.field.Currency != "" {
		return nil, invalid(at+".path", "this amount has its own row's currency, which totals cannot group by: read it through a column")
	}
	return bound, nil
}

// recordExpr types a record leaf; sql is filled in by its context.
func recordExpr(bound *pathPlan) expr {
	e := expr{typ: bound.field.Type, target: bound.target}
	if bound.field.Type == Money && bound.src.Currency != nil {
		e.cur = currency{fixed: bound.src}
	}
	return e
}

// recordSQL reads a group record's field: a subquery on the target's rule
// CTE, keyed by the group key, so a record the reader cannot see reads
// blank.
func recordSQL(s *stmt, target *Source, bound *pathPlan, key string) string {
	sc := newScope(s, target)
	v := sc.value(bound)
	return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s IS %s)", v.sql, sc.render(), q(sc.alias, "id"), key)
}

// firstTextField is the field a record group is labelled by when it names
// no label.
func firstTextField(src *Source) *Field {
	for i := range src.Fields {
		if src.Fields[i].Type == Text && src.Fields[i].Key != idField.Key {
			return &src.Fields[i]
		}
	}
	return nil
}

// labelText is a label value as row text: (none) for a blank.
func labelText(v any) string {
	switch x := v.(type) {
	case nil:
		return blankLabel
	case string:
		if x == "" {
			return blankLabel
		}
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprint(v)
}
```

- [ ] **Step 5: Group targets and the contexts**

In `rows.go` `compiler`, add `groupTargets []*Source` with the comment "the source each group's key links to (nil for a group that is not by a link)". In `rows()`, before the groups loop: `c.groupTargets = make([]*Source, len(c.p.groups))`; inside it, after the bucket handling: `if g.group.Bucket == "" { c.groupTargets[i] = e.target }`.

`formulaCurrencies` (`rows.go:143-155`) registers the currency key of every amount a formula reads by path, before any level groups by the keys. A record leaf's path is on its group's target, not on the base source, so it must not be bound there, and the labels' formulas must be walked too (F5):

```go
func (r *rowsCTE) formulaCurrencies(n *Node) {
	if n == nil {
		return
	}
	if n.Path != nil && n.Record == "" {
		if bound, err := bindValuePath(r.c.p.app, r.c.p.base, n.Path, "", pathValue); err == nil {
			r.pathCurrency(bound)
		}
	}
	for _, a := range n.Args {
		r.formulaCurrencies(a)
	}
}
```

and in `rows()`, after the columns' walk: `for _, g := range c.p.groups { r.formulaCurrencies(g.group.Label) }`. (Tasks 7, 9 and 10 add the having, sort-formula and detail-label walks.)

Contexts:

```go
// levels.go
func (lc *levelCtx) record(group string, p Path, at string) (expr, error) {
	c := lc.lv.c
	gi, target, err := c.recordOf(group, at)
	if err != nil {
		return expr{}, err
	}
	bound, err := bindRecordPath(c.p.app, target, p, at)
	if err != nil {
		return expr{}, err
	}
	e := recordExpr(bound)
	// Outside the group (a level above it, the total) there is no one record.
	if lc.lv.n < gi+1 {
		e.sql = "NULL"
		return e, nil
	}
	e.sql = recordSQL(lc.lv.r.s, target, bound, fmt.Sprintf("a.k%d", gi))
	return e, nil
}

// detail.go: a base row is under every group, so its record is its own key.
func (rc *rowsCtx) record(group string, p Path, at string) (expr, error) {
	c := rc.r.c
	gi, target, err := c.recordOf(group, at)
	if err != nil {
		return expr{}, err
	}
	bound, err := bindRecordPath(c.p.app, target, p, at)
	if err != nil {
		return expr{}, err
	}
	e := recordExpr(bound)
	e.sql = recordSQL(rc.r.s, target, bound, fmt.Sprintf("w.k%d", gi))
	return e, nil
}

// join.go (R21)
func (j *joinCtx) record(_ string, _ Path, at string) (expr, error) {
	return expr{}, invalid(at, "the filter, a group key and a fill read rows before they are grouped: read the field through a path")
}

// layout.go
func (lc *layoutCtx) record(_ string, _ Path, at string) (expr, error) { return expr{}, lc.only(at) }

// lookup.go
func (l *lookupCtx) record(g string, p Path, at string) (expr, error) { return l.outer.record(g, p, at) }
```

In `levels.go` `readsRowValue`, before the `n.Path != nil` test:

```go
	if n.Record != "" {
		gi := lv.c.groupIndex(n.Record)
		return !inAggregate && gi >= 0 && lv.n < gi+1
	}
```

In `formula_cases_test.go`: add `RecordTypes map[string]FieldType json:"recordTypes"` and `Record map[string]any json:"record"` to `formulaCase`; add `recordTypes map[string]FieldType` and `recordRow map[string]any` to `tableCtx` (set from the case in `runFormulaCase`; the row is not named `record`, which is the method), and:

```go
func (c *tableCtx) record(_ string, p Path, at string) (expr, error) {
	if len(p) != 1 || p[0].Field == "" {
		return expr{}, invalid(at, "a record path is one field here")
	}
	typ, ok := c.recordTypes[p[0].Field]
	if !ok {
		return expr{}, invalid(at, "unknown record field "+p[0].Field)
	}
	return expr{sql: c.bind(jsonValue(c.recordRow[p[0].Field])), typ: typ}, nil
}
func (l *tableLookupCtx) record(g string, p Path, at string) (expr, error) { return l.outer.record(g, p, at) }
```

- [ ] **Step 6: Labels on level rows**

In `levels.go` add:

```go
// labelSQL is the label of the level's rows: the group's label formula, a
// record group's first text field, or NULL (the key labels the row).
func (lv *level) labelSQL() (string, error) {
	if lv.n == 0 {
		return "NULL", nil
	}
	g := lv.c.p.groups[lv.n-1]
	if g.group.Label != nil {
		e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting}, g.group.Label, g.at+".label")
		if err != nil {
			return "", err
		}
		if e.win {
			return "", invalid(g.at+".label", "a running value or a percent is not a label")
		}
		return e.sql, nil
	}
	target := lv.c.groupTargets[lv.n-1]
	if target == nil {
		return "NULL", nil
	}
	f := firstTextField(target)
	if f == nil {
		return "NULL", nil
	}
	return recordSQL(lv.r.s, target, &pathPlan{field: f, src: target}, fmt.Sprintf("a.k%d", lv.n-1)), nil
}
```

Change `render(cells []string)` to `render(cells []string, own, label string)` and, after the `__ord` column, append `own+" AS __own"`, `label+" AS __label"`, `"a.__n AS __n"`. In `groupStatement` (`run.go:392-406`), after the pivot cells: `label, err := lv.labelSQL()` (return the error) and call `lv.render(cells, "1", label)`. In `layout.go` `layoutLegs`, after `cells = append(cells, fmt.Sprintf("%d AS __ord", i))` append `"1 AS __own", "NULL AS __label", "NULL AS __n"`, so every leg of the union has the same columns.

In `assemble.go` `group`:

```go
	g := p.groups[n-1]
	label := rn.display(g.col, row[fmt.Sprintf("k%d", n-1)], g.group.Bucket != "")
	// A record group and a labelled group are named by their label; a blank
	// one reads (none), never the record's id.
	if g.group.Label != nil || rn.c.groupTargets[n-1] != nil {
		label = labelText(row["__label"])
	}
	rn.groupWithLabel(t, row, n, section, id, label, key, g.group.Subtotals)
```

(A record group whose target has no text field and no label: `labelSQL` is NULL, so the label is `(none)`; give such a group a label formula. `docs/packages.md` says so in Task 26.)

- [ ] **Step 7: Run the reports tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS (layout and pivot tests still pass with the three new leg columns).

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports core/lib/forms/fixtures/formula-cases.json
git commit -m "feat(core): record groups, the record leaf and group labels"
git push origin books/p5-trial-balance
```

---

### Task 5b: F22 — a link column shows its record's label, never a raw id

Engine facts used: a relation value carries its target (Task 3); `recordSQL` reads a target's field through its rule CTE keyed by a value (Task 5); level legs and computed legs share one column list (`levels.go` `render`, `layout.go` `layoutLegs`); `cells` maps `v<idx>` to cells (`assemble.go:242-266`); a detail row's default label is its first column (`assemble.go:233-240`).

Design (R31): every column whose value is a relation with a known target (a path ending at a to-one link or at `id`, a lookup or formula of one) shows the target record's label beside the id: the cell `<col>@label`, and the result column's `labelCell`. The label field is the column's `labelField` (a field of the target the author picks in the builder) or, without one, the target's first text field, else its first other field, else the target source's own name. A blank link (no id, or a record the reader cannot see, which reads blank through the join) has no label, and the client shows "(none)"; a visible record whose label field is blank shows "(no name)". A record group's default label (R9) and a detail row's default label use the same rule, so no row or cell ever shows a raw id.

**Files:**
- Modify: `core/server/reports/spec.go` (`Column.LabelField`), `record.go` (`labelFieldOf`, `labelSQL`), `run.go` (`ResultColumn.LabelCell`; `vl<idx>` in the detail select and level cells), `levels.go` (`labelSQL` of a record group uses `labelFieldOf`), `layout.go` (legs add `NULL AS vl<idx>`), `assemble.go` (`cells`, `columns`, `firstLabel`)
- Modify: `core/server/reports/relation_test.go`, `record_test.go`

**Interfaces:**
- Consumes: Tasks 3, 5.
- Produces: `Column.LabelField string` (`json:"labelField,omitempty"`); `labelFieldOf(src *Source, chosen string) *Field`; `labelValueSQL(s *stmt, target *Source, f *Field, key string) string`; `selectLabelSQL(s *stmt, f *Field, value string) string`; `ResultColumn.LabelCell string` (`json:"labelCell,omitempty"`, `"<id>@label"`); rows carry `vl<idx>` (NULL when the column has no target).

- [ ] **Step 1: Write the failing tests**

In `relation_test.go`, extend `TestAPathEndingAtALinkReadsTheLinkedRowsID` (it still asserts the ids) with:

```go
	requireRows(t, res, "animal@label", "Leo", "Leo", "Zed", "Zed", nil)
	if c := res.Columns[1]; c.LabelCell != "animal@label" {
		t.Fatalf("animal column = %+v", c)
	}
```

and `TestALinkToAHiddenRowReadsBlank` with `requireRows(t, res, "vault@label", nil, nil, nil, nil)`. Add:

```go
// The author picks the field a link column shows; a target with no fields
// shows its source's name, never an id.
func TestALinkColumnShowsTheChosenLabelField(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}],"labelField":"species"}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "animal@label", "lion", "lion", "zebra", "zebra", nil)
	keepers := z.run(t, "ann", `{"version":2,"source":"zoo.keepers","columns":[{"id":"k","path":[{"field":"id"}]}]}`, Options{}, nil)
	requireRows(t, keepers, "k@label", "Keepers", "Keepers", nil)
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}],"labelField":"nope"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 422, "columns.0.labelField", "has no field")
}

// A select label field shows its option's label, not the stored value.
func TestALinkLabelFieldThatIsASelectShowsItsOptionLabel(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"site","path":[{"link":"animal"},{"link":"site"}],"labelField":"region"}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "site@label", "North", "North", "South", "South", nil)
}

// A link column first in the report labels its detail rows by the record.
func TestADetailRowIsLabelledByItsLinkedRecord(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Leo", "Leo", "Zed", "Zed", "Total")
}
```

(ann keeps leo and zed: two feedings each. The keepers report as ann lists her two keeper rows k1 and k2.)

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'LinkedRowsID|HiddenRowReadsBlank|ChosenLabelField|LabelFieldThatIsASelect|LabelledByItsLinkedRecord' ./reports/`
Expected: FAIL to load: `json: unknown field "labelField"`.

- [ ] **Step 3: Implement**

`spec.go` `Column`: `LabelField string \`json:"labelField,omitempty"\`` ("the field of a link column's record it shows; the first text field when empty").

`record.go`:

```go
// labelFieldOf is the field a record is shown by (R31): the chosen field, else
// the first text field, else the first other field; nil when the source has
// no field but its id (then its own name is shown).
func labelFieldOf(src *Source, chosen string) *Field {
	if chosen != "" {
		return src.field(chosen)
	}
	if f := firstTextField(src); f != nil {
		return f
	}
	for i := range src.Fields {
		if src.Fields[i].Key != idField.Key {
			return &src.Fields[i]
		}
	}
	return nil
}

// labelValueSQL reads a record's label keyed by a relation value: NULL for a
// blank link, "(no name)" for a record whose label field is blank, the
// source's name for a source with no field to show.
func labelValueSQL(s *stmt, target *Source, f *Field, key string) string {
	if f == nil {
		return fmt.Sprintf("(CASE WHEN %s IS NULL THEN NULL ELSE %s END)", key, s.bind(target.Label))
	}
	read := recordSQL(s, target, &pathPlan{field: f, src: target}, key)
	if f.Type == Select {
		read = selectLabelSQL(s, f, read)
	}
	return fmt.Sprintf("(CASE WHEN %s IS NULL THEN NULL ELSE COALESCE(NULLIF(CAST(%s AS TEXT), ''), %s) END)", key, read, s.bind("(no name)"))
}

// selectLabelSQL is a select value shown by its option's label; a value no
// option names shows as stored.
func selectLabelSQL(s *stmt, f *Field, value string) string {
	if len(f.Options) == 0 {
		return value
	}
	var b strings.Builder
	b.WriteString("(CASE " + value)
	for _, o := range f.Options {
		b.WriteString(" WHEN " + s.bind(o.Value) + " THEN " + s.bind(o.Label))
	}
	b.WriteString(" ELSE " + value + " END)")
	return b.String()
}
```

`levels.go` `labelSQL`: the record-group default becomes `labelValueSQL(lv.r.s, target, labelFieldOf(target, groupColumn.col.LabelField), fmt.Sprintf("a.k%d", lv.n-1))` (`groupColumn` is the group's column plan).

`run.go`: add `LabelCell string \`json:"labelCell,omitempty"\`` to `ResultColumn`; `compiler` gains `labelled map[string]bool`. In `detailSelect`, after each column's expr `e`, append `labelOf(e) + " AS vl<idx>"`, and in `groupStatement`'s cells loop the same with the level cell (the key on the group's own level, NULL elsewhere), where:

```go
// labelOf is the label beside a relation value with a target, or NULL. An
// unknown labelField is an unknown reference at columns.N.labelField.
func (c *compiler) labelOf(s *stmt, cp *colPlan, e expr) (string, error) {
	if e.target == nil {
		if cp.col.LabelField != "" {
			return "", invalid(cp.at+".labelField", "only a column that links to a record shows a label field")
		}
		return "NULL", nil
	}
	f := labelFieldOf(e.target, cp.col.LabelField)
	if cp.col.LabelField != "" && f == nil {
		return "", unknownRef(cp.at+".labelField", "unknown_field", fmt.Sprintf("the source %q has no field %q", e.target.Key, cp.col.LabelField))
	}
	c.labelled[cp.col.ID] = true
	if e.sql == "NULL" {
		return "NULL", nil
	}
	return labelValueSQL(s, e.target, f, e.sql), nil
}
```

`layout.go`: computed legs append `NULL AS vl<idx>` for every column. `assemble.go`: `columns` sets `rc.LabelCell = cp.col.ID + "@label"` when `c.labelled[id]`; `cells` sets `out[id+"@label"] = row["vl<idx>"]` for a labelled column; `firstLabel` uses `labelText(row["vl0"])` when the first column is labelled (a blank link reads "(none)").

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): link columns show their record's label"
git push origin books/p5-trial-balance
```

---

### Task 5a: Q5 (changed by the human) — amounts with a per-row currency in `record` leaves and `lookup` values

Engine facts used: a money value's currency is a currency key of `w` (`ck<n>`) or a source's fixed currency (`formula.go:32-74`); every level groups by the currency keys, so a total never adds two currencies (`levels.go:32-51`); the keys are known before any level compiles (`rows.go:108-114`); a money cell names its currency cell `<col>@currency` (`assemble.go:51-65,242-266`); Tasks 4 and 5 refused a money field with a per-row currency field in a lookup value and a record leaf.

Design (R3, changed): the amount carries its currency through.
- **On a group row, its subtotal, or a detail row, not totalled**, a record amount's currency is its record's own currency field, and a looked-up amount's currency is the currency of the row the lookup picked; each is read by a subquery next to the value (`currency.sql`), and the cell's `<col>@currency` shows it.
- **Inside a total** (an aggregate's argument, or a formula column with an aggregate), the amount is read once per base row, so its currency becomes a currency key of `w` like a path amount's (registered before the levels compile, by a walk of every formula): the totals group by it, one row per currency, as the engine's money rule says.
- **A lookup of such an amount** picks the matching row with the smallest (`min`) or largest (`max`) amount and returns that row's amount and currency (ties broken by currency code), so the two always come from one row.
- **Combining amounts** keeps today's rule: amounts combine only when their currencies come from the same place. Each currency has an identity — a currency key, a source's fixed currency, or `rec:<group>:<path>` / `lk:<node JSON>` for a carried currency — and `combined` compares identities, so two reads of the same record field combine and a record amount plus a path amount is refused ("amounts in different currencies cannot be combined").

**Files:**
- Modify: `core/server/reports/formula.go` (`currency.id`, `currency.sql`, `none`, `same`), `rows.go` (`compiler.valueCurrencies`, `rowsCTE.valueCurrencies`), `record.go` (`bindRecordPath` keeps the currency field; `recordCurrency`), `lookup.go` (picked-row value and currency), `levels.go`/`detail.go` (`record` and `lookup` read registered keys inside totals), `run.go` (`vc<idx>` currency columns in level and detail selects), `layout.go` (legs add `NULL AS vc<idx>`), `assemble.go` (`cells`, `currencyOfColumn`)
- Modify: `core/server/reports/zoo_test.go` (`zoo_animals.fee`, `fee_currency`), `lookup_test.go` (the refused case becomes a working one)
- Create: `core/server/reports/valuecurrency_test.go`

**Interfaces:**
- Consumes: Tasks 4, 5.
- Produces: `currency{key string; fixed *Source; sql string; id string}`; `compiler.valueCur map[string]string` (identity → `ck<n>`); level and detail rows carry `vc<idx>` (a column's carried currency, NULL when it has none); `ResultColumn.CurrencyCell` is set for a column with a carried currency.

- [ ] **Step 1: Give the zoo an amount with a per-row currency on a record**

In `zoo_test.go`: `animals.Fields.Add(…)` gains `&core.NumberField{Name: "fee"}` and `&core.TextField{Name: "fee_currency"}`; the animals' data gain `"fee": 700, "fee_currency": "EUR"` (leo), `"fee": 300, "fee_currency": "USD"` (zed), `"fee": 20, "fee_currency": "GBP"` (mo), `"fee": 50, "fee_currency": "USD"` (kit); `zoo.animals` Fields gain `{Key: "fee", Label: "Fee", Type: Money, Aggregates: SumAll, Currency: "fee_currency"}` and `{Key: "fee_currency", Label: "Fee currency", Type: Text}`.

- [ ] **Step 2: Write the failing tests**

Create `core/server/reports/valuecurrency_test.go`:

```go
package reports

import "testing"

const feeByAnimal = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"fee","formula":{"record":"g","path":[{"field":"fee"}]}},
	{"id":"fees","formula":{"record":"g","path":[{"field":"fee"}]},"aggregate":"sum"}],
	"groups":[{"id":"g","col":"animal"}],"sort":[{"col":"fees"}]}`

// A record's amount carries its record's currency into the cell; totalled, it
// groups by that currency.
func TestARecordAmountCarriesItsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feeByAnimal, Options{}, nil)
	// One total per fee currency, each after its groups: EUR (leo, 2 × 700) sorts
	// before USD (zed, 2 × 300).
	requireRows(t, res, "label", "Leo", "Total", "Zed", "Total")
	cells := map[string]any{}
	for _, row := range res.Rows {
		if row.Kind == KindGroup {
			cells[row.Label] = row.Cells["fee@currency"]
		}
	}
	if cells["Leo"] != "EUR" || cells["Zed"] != "USD" {
		t.Fatalf("fee currencies = %v", cells)
	}
	totals := map[any]any{}
	for _, row := range res.Rows {
		if row.Kind == KindTotal {
			totals[row.Cells["fees@currency"]] = row.Cells["fees"]
		}
	}
	if totals["EUR"] != int64(1400) || totals["USD"] != int64(600) {
		t.Fatalf("totals = %v", totals)
	}
	if c := res.Columns[1]; c.CurrencyCell != "fee@currency" {
		t.Fatalf("fee column = %+v", c)
	}
}

// A looked-up amount carries the currency of the row it picked.
func TestALookedUpAmountCarriesThePickedRowsCurrency(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.sites","columns":[{"id":"name","path":[{"field":"name"}]},
		{"id":"top","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"fee"}]},"pick":"max"}},
		{"id":"low","formula":{"op":"lookup","source":"zoo.animals","value":{"path":[{"field":"fee"}]}}}],"sort":[{"col":"name"}]}`, Options{}, nil)
	// ann may list leo (700 EUR), zed (300 USD), kit (50 USD).
	first := res.Rows[0]
	if first.Cells["top"] != int64(700) || first.Cells["top@currency"] != "EUR" || first.Cells["low"] != int64(50) || first.Cells["low@currency"] != "USD" {
		t.Fatalf("row = %+v", first.Cells)
	}
}

func TestCarriedCurrenciesCombineOnlyWithTheirOwn(t *testing.T) {
	app := withZooRegistry(t)
	twice, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"x","formula":{"op":"add","args":[{"record":"g","path":[{"field":"fee"}]},{"record":"g","path":[{"field":"fee"}]}]}}],"groups":[{"id":"g","col":"a"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(app, twice); err != nil {
		t.Fatalf("one record field twice combines: %v", err)
	}
	mixed, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"x","formula":{"op":"add","args":[{"record":"g","path":[{"field":"fee"}]},{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],"groups":[{"id":"g","col":"a"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, mixed), 400, "columns.1.formula", "different currencies")
}
```

In `lookup_test.go`, delete the case "an amount with a row currency" (Q5 builds it). In `record_test.go`, nothing refuses a record amount any more.

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'CarriesItsCurrency|PickedRowsCurrency|CombineOnlyWithTheirOwn' ./reports/`
Expected: FAIL: `this amount has its own row's currency, which totals cannot group by`.

- [ ] **Step 4: Currencies with an identity**

`formula.go`:

```go
// currency is where a money value's currency comes from: a currency key of w
// (the ck<n> that holds each row's currency), a source's fixed currency, or a
// subquery that reads the currency where the value is read (sql: a record's or
// a looked-up row's own currency). id names it for combining: two values
// combine only when their currencies come from the same place (R3).
type currency struct {
	key   string
	fixed *Source
	sql   string
	id    string
}

func (a currency) none() bool { return a.key == "" && a.fixed == nil && a.sql == "" }

func (a currency) same(b currency) bool {
	switch {
	case a.key != "" || b.key != "":
		return a.key == b.key
	case a.fixed != nil || b.fixed != nil:
		return a.fixed != nil && b.fixed != nil && a.fixed.Key == b.fixed.Key
	}
	return a.id == b.id
}
```

- [ ] **Step 5: Carried currencies in record leaves and lookups**

`record.go`: `bindRecordPath` no longer refuses an amount with a currency field. Add:

```go
// recordCurrencyID names a record amount's currency for combining.
func recordCurrencyID(group string, p Path) string {
	raw, _ := json.Marshal(p)
	return "rec:" + group + ":" + string(raw)
}

// recordCurrency is a record amount's currency read beside it: its record's
// currency field, keyed like the value, or the source's fixed currency.
func recordCurrency(s *stmt, target *Source, bound *pathPlan, key, id string) currency {
	f := bound.field
	switch {
	case f.Type != Money:
		return currency{}
	case f.Currency != "":
		cur := &pathPlan{hops: bound.hops, field: bound.src.field(f.Currency), src: bound.src}
		return currency{sql: recordSQL(s, target, cur, key), id: id}
	case bound.src.Currency != nil:
		return currency{fixed: bound.src}
	}
	return currency{}
}
```

`recordExpr` drops its `cur` (each context sets it): `levelCtx.record` sets `e.cur = recordCurrency(lc.lv.r.s, target, bound, fmt.Sprintf("a.k%d", gi), recordCurrencyID(group, p))`; `rowsCtx.record` uses the registered key when one exists (inside a total), else the same subquery keyed by `w.k<gi>`:

```go
	id := recordCurrencyID(group, p)
	if ck, ok := c.valueCur[id]; ok && bound.field.Currency != "" {
		e.cur = currency{key: ck}
	} else {
		e.cur = recordCurrency(rc.r.s, target, bound, fmt.Sprintf("w.k%d", gi), id)
	}
```

`lookup.go`: `lookupCtx.path` no longer refuses an amount with a currency field; it returns the value with `cur: currency{sql: <the currency column>, id: "pending"}` (the column SQL of the currency field on the lookup's scope). In `lookupIn`, when `v.typ == Money` and `v.cur.sql != ""`, the value and its currency come from one picked row:

```go
	if v.typ == Money && v.cur.sql != "" {
		dir := "ASC"
		if n.Pick == "max" {
			dir = "DESC"
		}
		pickOne := func(col string) string {
			return fmt.Sprintf("(SELECT %s\n%s\nWHERE %s AND %s IS NOT NULL\nORDER BY %s %s, %s LIMIT 1)", col, sc.render(), where, v.sql, v.sql, dir, v.cur.sql)
		}
		raw, _ := json.Marshal(n)
		id := "lk:" + string(raw)
		cur := currency{sql: pickOne(v.cur.sql), id: id}
		if ck, ok := r.c.valueCur[id]; ok && !inLevel(outer) {
			cur = currency{key: ck}
		}
		return expr{sql: pickOne(v.sql), typ: Money, target: v.target, cur: cur}, nil
	}
```

with:

```go
// inLevel is true when a formula is compiled on a group, subtotal or total
// row (a levelCtx, or a lookup inside one): there a picked or record currency
// is read by subquery; on a base row inside a total, by its registered key.
func inLevel(f fctx) bool {
	switch x := f.(type) {
	case *levelCtx:
		return true
	case *lookupCtx:
		return inLevel(x.outer)
	}
	return false
}
```

- [ ] **Step 6: Register currency keys for carried amounts inside totals**

`rows.go`: add `valueCur map[string]string` to `compiler` (reset in `rows()` with `currencies`), and call `r.valueCurrencies()` after `r.formulaCurrencies(…)`:

```go
// valueCurrencies adds a currency key to w for every record or looked-up
// amount with a per-row currency read inside a total: a total groups by its
// currency as by a path amount's (R3). Read outside a total, such an amount
// carries its currency by subquery and needs no key.
func (r *rowsCTE) valueCurrencies() {
	var walk func(n *Node, inTotal bool)
	walk = func(n *Node, inTotal bool) {
		if n == nil {
			return
		}
		switch {
		case n.Record != "" && inTotal:
			r.recordCurrencyKey(n)
		case n.Op == "lookup" && inTotal:
			r.lookupCurrencyKey(n)
		}
		inTotal = inTotal || aggregateOp(n)
		for _, a := range n.Args {
			walk(a, inTotal)
		}
	}
	for _, cp := range r.c.p.cols {
		walk(cp.col.Formula, cp.col.Aggregate != "")
	}
	for _, g := range r.c.p.groups {
		walk(g.group.Label, false)
	}
	for _, s := range r.c.p.sort {
		walk(s.formula, false)
	}
	// Task 7 adds walk(g.group.Having, false) beside the label walk; Task 10
	// adds the detail label.
}

// recordCurrencyKey reads a record amount's currency field in w, keyed by the
// group key's own expression.
func (r *rowsCTE) recordCurrencyKey(n *Node) {
	gi, target, err := r.c.recordOf(n.Record, "")
	if err != nil {
		return
	}
	bound, err := bindRecordPath(r.c.p.app, target, n.Path, "")
	if err != nil || bound.field.Type != Money || bound.field.Currency == "" {
		return
	}
	cur := recordCurrency(r.s, target, bound, r.inner(fmt.Sprintf("w.k%d", gi)), "")
	r.currencyKey(recordCurrencyID(n.Record, n.Path), cur.sql)
}

// lookupCurrencyKey compiles the lookup inside w and keeps its picked row's
// currency as a key.
func (r *rowsCTE) lookupCurrencyKey(n *Node) {
	e, err := lookupIn(r.join(false), r, n, "")
	if err != nil || e.typ != Money || e.cur.sql == "" {
		return
	}
	raw, _ := json.Marshal(n)
	r.currencyKey("lk:"+string(raw), e.cur.sql)
}

func (r *rowsCTE) currencyKey(id, sql string) {
	if _, seen := r.c.valueCur[id]; seen {
		return
	}
	name := fmt.Sprintf("ck%d", len(r.c.currencies))
	r.add(name, sql)
	r.c.currencies = append(r.c.currencies, name)
	r.c.valueCur[id] = name
}
```

(A record key is only known once the group keys are in `w`, which `rows()` has done by then; a walk that cannot bind leaves the error to the compile that follows.)

- [ ] **Step 7: Carried currencies reach the cells**

`run.go` `groupStatement`: after each column's cell, append `curSQL + " AS vc<idx>"` where `curSQL` is `e.cur.sql` or `NULL`, and record `c.carried[cp.col.ID] = true` when `e.cur.sql != ""`; `detailSelect` does the same with the detail expr. `layout.go` computed legs append `NULL AS vc<idx>` for every column (the union's columns stay equal). `assemble.go`: `cells` sets `out[id+"@currency"]` from `row["vc<idx>"]` when it is not nil, else from the column's currency key as today; `currencyOfColumn` sets `CurrencyCell` when the column has a currency key or `c.carried[id]`.

- [ ] **Step 8: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS (`markCurrencies` treats the new keys like any other: a column whose totals hold two fee currencies is marked mixed).

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): record and looked-up amounts carry their own currency"
git push origin books/p5-trial-balance
```

---

### Task 6: E4 (part 3) — `groups[].fill` on every group level: records with no rows

Engine facts used: `a<L>` groups `w` by the level keys and counts `__n` (`levels.go:170-178`); to-many totals join `d<L>_<idx>` onto `a<L>` (`levels.go:73-97`, `levels.go:179-194`); `joinCtx` on a scope other than the root reads that scope's paths and refuses report columns (`join.go:56-79`); siblings are ordered within their parent (`levels.go:32-51`); a group row is pushed with `CanExpand` (`assemble.go:212-231`); totals are one per currency (`assemble.go:146-189`).

Design (R6, changed by the human — Q3: every level):
- **Keys.** The keyset of a fill level L is the keys of its base rows (`k0…k<L-1>` and the currency keys, from `w`) united with every parent row of level L-1 crossed with the target's records that match the fill's `where`. The parent rows of level 1 are the report's currencies (or one no-currency row when the report has no rows); of level L > 1, level L-1's keyset — its fill rows included, when level L-1 has a fill — so a fill record also appears under a parent that is itself a fill record. A fill record therefore appears under each parent group, under every currency.
- **Where.** The fill's `where` reads the target's fields (paths), params, lookups, and the group above: `{col}` naming the column of an enclosing group reads that parent row's key (`p.k<i>`), and `{record}` naming an enclosing record group reads that parent's record. So "the accounts of this account type" is `eq([type], {col: type})` on a level under a group by type. The target's records are read through its rule CTE.
- **Ordering, totals, having.** Fill rows sort with their siblings under their parent by the spec's sort keys; `recompute` totals read base rows only (a fill row adds nothing); `rows` totals add a fill row's cells (blank or zero); `having` applies to fill rows as to any group.
- **Opening.** A fill row has no base rows, so it opens only when a deeper level has a fill (then it opens on that level's fill records).

**Files:**
- Modify: `core/server/reports/spec.go` (`Fill`, `Group.Fill`), `levels.go` (`level.fill`, `compiler.fillSets`, `compiler.fillKeys`, `fillCtx`, `render`, `toManyTotal`), `assemble.go` (`groupWithLabel`)
- Create: `core/server/reports/fill_test.go`

**Interfaces:**
- Consumes: `compiler.groupTargets`, `recordSQL` (Task 5).
- Produces: `type Fill struct { Where *Node \`json:"where,omitempty"\` }`; `Group.Fill *Fill` (`json:"fill,omitempty"`); `(c *compiler) fillKeys(r *rowsCTE, n int, at string) (string, error)`; `compiler.fillSets map[int]string` (level → its keyset SQL); `fillCtx` (paths on the target, `{col}`/`{record}` of enclosing groups on the parent row `p`); a fill row has `__n = 0`.

- [ ] **Step 1: Records with no rows, made by the fill tests only**

The fill tests add, with `z.rec`, a site and two animals ann owns that have no feeding (made in the tests, not in `newZoo`, so no other test's rows change): see `withEmptyAnimals` below.

- [ ] **Step 2: Write the failing tests**

Create `core/server/reports/fill_test.go`:

```go
package reports

import "testing"

// withEmptyAnimals adds a site and two of ann's animals with no feeding: Lia
// (lion, at North) and Eli (elk, at East).
func withEmptyAnimals(t *testing.T, z *zoo) {
	t.Helper()
	east := z.rec(t, "east", "zoo_sites", map[string]any{"name": "East", "region": "north"})
	z.rec(t, "lia", "zoo_animals", map[string]any{"name": "Lia", "species": "lion", "site": z.id("north"), "owner": z.id("ann"), "active": true})
	z.rec(t, "eli", "zoo_animals", map[string]any{"name": "Eli", "species": "elk", "site": east.Id, "owner": z.id("ann"), "active": true})
}

func animalFill(fill, filter string) string {
	spec := `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"animal","fill":` + fill + `}],"sort":[{"col":"cost"},{"formula":{"record":"g","path":[{"field":"name"}]}}]`
	if filter != "" {
		spec += `,"filter":` + filter
	}
	return spec + "}"
}

// Fill adds the target's records that have no rows, under the reader's rules:
// ann sees eli, kit and lia (active, no feeding she may read); mo is bob's.
func TestFillAddsRecordsWithNoRows(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", animalFill(`{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}`, ""), Options{}, nil)
	requireRows(t, res, "label", "Eli", "Kit", "Lia", "Zed", "Leo", "Total")
	requireRows(t, res, "cost", int64(0), int64(0), int64(0), int64(1000), int64(3500), int64(4500))
	requireRows(t, res, "n", int64(0), int64(0), int64(0), int64(2), int64(2), int64(4))
	if res.Rows[0].CanExpand || !res.Rows[3].CanExpand {
		t.Fatalf("a fill row with no deeper fill has nothing to open: %+v", res.Rows[:4])
	}
}

// With no rows at all the first level's fill records sit under the no-currency total.
func TestFillWithNoRowsSitsUnderOneTotal(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	res := z.run(t, "ann", animalFill(`{}`, `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"none"}]}`), Options{}, nil)
	if last := res.Rows[len(res.Rows)-1]; last.ID != "total:[null]" || len(res.Rows) != 6 {
		t.Fatalf("rows = %+v", res.Rows)
	}
}

const sitesThenAnimals = `{"version":2,"source":"zoo.feedings","columns":[
	{"id":"site","path":[{"link":"animal"},{"link":"site"}]},
	{"id":"animal","path":[{"link":"animal"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[
		{"id":"s","col":"site","fill":{}},
		{"id":"a","col":"animal","fill":{"where":{"op":"and","args":[
			{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]},
			{"op":"eq","args":[{"path":[{"link":"site"}]},{"col":"site"}]}]}}}],
	"sort":[{"formula":{"record":"s","path":[{"field":"name"}]}},{"formula":{"record":"a","path":[{"field":"name"}]}}]}`

// A deeper fill adds, under each parent, the records its where ties to that
// parent; a parent that is itself a fill record opens on them.
func TestFillOnADeeperLevelSitsUnderEachParent(t *testing.T) {
	z := newZoo(t)
	withEmptyAnimals(t, z)
	// Open the sites, not the animals, so no feeding rows show.
	sites := [][]any{{z.id("east"), "USD"}, {z.id("north"), "USD"}, {z.id("south"), "USD"}}
	res := z.run(t, "ann", sitesThenAnimals, Options{Expand: sites}, nil)
	requireRows(t, res, "label", "East", "Eli", "North", "Leo", "Lia", "South", "Zed", "Total")
	requireRows(t, res, "cost", int64(0), int64(0), int64(3500), int64(3500), int64(0), int64(1000), int64(1000), int64(4500))
	if !res.Rows[0].CanExpand || res.Rows[1].CanExpand {
		t.Fatalf("east opens on its fill record, which opens on nothing: %+v", res.Rows[:2])
	}
}

func TestFillIsRefusedWhereItCannotWork(t *testing.T) {
	app := withZooRegistry(t)
	notRecord := `{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"n","fill":{}}]}`
	ownColumn := `{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"a","fill":{"where":{"op":"gt","args":[{"col":"c"},{"value":0}]}}}]}`
	for _, c := range []struct{ spec, at, want string }{
		{notRecord, "groups.0.fill", "only a group by a link"},
		{ownColumn, "groups.0.fill.where.args.0", "the keys of the groups above it"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Fill' ./reports/`
Expected: FAIL to load: `json: unknown field "fill"`.

- [ ] **Step 4: Spec**

`spec.go`: `Group` gains `Fill *Fill \`json:"fill,omitempty"\`` ("adds the linked records that have no rows, under each parent group"), and:

```go
// Fill is a record group's "show every record": the target's records that
// match Where (all it may list when Where is nil) and have no base rows, under
// each parent group and currency.
type Fill struct {
	Where *Node `json:"where,omitempty"`
}
```

- [ ] **Step 5: Fill keysets on every level**

`levels.go`: `level` gains `fill string`; `compiler` gains `fillSets map[int]string` (reset in `groupStatement`). In `compiler.level`, after the keys:

```go
	if n >= 1 && c.p.groups[n-1].group.Fill != nil {
		fill, err := c.fillKeys(r, n, c.p.groups[n-1].at+".fill")
		if err != nil {
			return nil, err
		}
		lv.fill = fill
		c.fillSets[n] = fill
	}
```

```go
// parentKeys selects the parent rows of level n: the currencies for level 1
// (or one no-currency row when there are no rows); for a deeper level the
// keyset of level n-1, its fill rows included.
func (c *compiler) parentKeys(n int) string {
	if n == 1 {
		if len(c.currencies) == 0 {
			return "SELECT 1 AS __one"
		}
		cks := strings.Join(c.currencies, ", ")
		nulls := strings.TrimSuffix(strings.Repeat("NULL, ", len(c.currencies)), ", ")
		return fmt.Sprintf("SELECT DISTINCT %s FROM w UNION ALL SELECT %s WHERE NOT EXISTS (SELECT 1 FROM w)", cks, nulls)
	}
	if set, ok := c.fillSets[n-1]; ok {
		return set
	}
	keys := make([]string, 0, n-1+len(c.currencies))
	for i := range n - 1 {
		keys = append(keys, fmt.Sprintf("k%d", i))
	}
	return "SELECT DISTINCT " + strings.Join(append(keys, c.currencies...), ", ") + " FROM w"
}

// fillKeys selects level n's keys: those of its base rows, and each parent row
// crossed with the target's records that match the fill's where (R6).
func (c *compiler) fillKeys(r *rowsCTE, n int, at string) (string, error) {
	target := c.groupTargets[n-1]
	if target == nil {
		return "", invalid(at, "only a group by a link to one record shows records with no rows")
	}
	sc := newScope(r.s, target)
	where := "1"
	if w := c.p.groups[n-1].group.Fill.Where; w != nil {
		j := &joinCtx{r: r, sc: sc, src: target, isFilter: true, visiting: map[string]bool{}}
		e, err := compileWhere(&fillCtx{joinCtx: j, n: n}, w, at)
		if err != nil {
			return "", err
		}
		if !e.unset {
			where = "COALESCE(" + e.sql + ", 0)"
		}
	}
	keys := make([]string, 0, n+len(c.currencies))
	picked := make([]string, 0, n+len(c.currencies))
	for i := range n - 1 {
		keys = append(keys, fmt.Sprintf("k%d", i))
		picked = append(picked, fmt.Sprintf("p.k%d AS k%d", i, i))
	}
	keys = append(keys, fmt.Sprintf("k%d", n-1))
	picked = append(picked, q(sc.alias, "id")+fmt.Sprintf(" AS k%d", n-1))
	for _, ck := range c.currencies {
		keys = append(keys, ck)
		picked = append(picked, "p."+ck+" AS "+ck)
	}
	from := strings.TrimPrefix(sc.render(), "FROM ")
	return fmt.Sprintf("SELECT %s FROM w UNION SELECT %s\nFROM (%s) p\nCROSS JOIN %s\nWHERE %s",
		strings.Join(keys, ", "), strings.Join(picked, ", "), c.parentKeys(n), from, where), nil
}

// fillCtx compiles a fill's where: the target's paths (joinCtx on its scope),
// and the parent row p: {col} naming an enclosing group's column reads that
// parent's key, {record} naming an enclosing record group reads its record.
type fillCtx struct {
	*joinCtx
	n int
}

func (f *fillCtx) col(id, at string) (expr, error) {
	for i := range f.n - 1 {
		if g := f.r.c.p.groups[i]; g.col.col.ID == id {
			e, err := f.r.rowValue(g.col, at, map[string]bool{})
			if err != nil {
				return expr{}, err
			}
			e.sql = fmt.Sprintf("p.k%d", i)
			return e, nil
		}
	}
	return expr{}, invalid(at, "a fill's where reads its records' fields, params, and the keys of the groups above it")
}

func (f *fillCtx) record(group string, p Path, at string) (expr, error) {
	gi, target, err := f.r.c.recordOf(group, at)
	if err != nil {
		return expr{}, err
	}
	if gi >= f.n-1 {
		return expr{}, invalid(at, "a fill's where reads the records of the groups above it")
	}
	bound, err := bindRecordPath(f.r.c.p.app, target, p, at)
	if err != nil {
		return expr{}, err
	}
	e := recordExpr(bound)
	e.sql = recordSQL(f.r.s, target, bound, fmt.Sprintf("p.k%d", gi))
	return e, nil
}

func (f *fillCtx) lookup(n *Node, at string) (expr, error) { return lookupIn(f, f.r, n, at) }
```

(For level 1, `parentKeys` with no currencies is `SELECT 1 AS __one`, and the picked list has no `p.` columns, so the cross join adds every matching record once.)

`render` builds `a<L>` from the keyset when the level has a fill (a left join keeps a record with no base rows; `count(w.__id)` counts 0 for it, and every aggregate reads `w.` columns that are NULL there):

```go
	a := fmt.Sprintf("a%d", lv.n)
	var ctes []string
	if lv.fill != "" {
		sel := make([]string, len(lv.keys))
		on := make([]string, len(lv.keys))
		for i, k := range lv.keys {
			sel[i] = "kx." + k + " AS " + k
			on[i] = "w." + k + " IS kx." + k
		}
		ctes = []string{fmt.Sprintf("%s AS (SELECT %s FROM (%s) kx LEFT JOIN w ON %s GROUP BY %s)", a,
			strings.Join(append(append(sel, lv.aggs...), "count(w.__id) AS __n"), ", "),
			lv.fill, strings.Join(on, " AND "), strings.Join(prefixed("kx.", lv.keys), ", "))}
	} else {
		keyCols := slices.Clone(lv.keys)
		ctes = []string{fmt.Sprintf("%s AS (SELECT %s FROM w%s)", a,
			strings.Join(append(append(keyCols, lv.aggs...), "count(*) AS __n"), ", "), groupBy(lv.keys))}
	}
```

A fill record has no `d<L>_<idx>` row, so its to-many sum and count would read NULL instead of 0: `lv.tmCols` becomes `[]tmCol` with `type tmCol struct{ d, idx string; zero bool }` (`zero` for `Sum` and `Count`, set in `toManyTotal`), and `render`'s `g<L>` CTE selects `COALESCE(d.x<idx>, 0) AS x<idx>` for those.

- [ ] **Step 6: A fill row opens only onto a deeper fill**

`assemble.go` `groupWithLabel`, after `canOpen`:

```go
	// A fill record has no base rows: it opens only onto a deeper level's fill
	// records. A layout section with no rows has no __n and keeps today's rule.
	if v, ok := row["__n"]; ok && v != nil && toInt(v) == 0 && !rn.c.deeperFill(n) {
		canOpen = false
	}
```

with `func (c *compiler) deeperFill(n int) bool` true when a group after index `n-1` has a fill.

- [ ] **Step 7: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): record groups can show every record, under each parent group"
git push origin books/p5-trial-balance
```

---

### Task 7: E3 — `groups[].having`

Engine facts used: a condition on an unset param matches every row only where the context says it is a filter (`formula.go:288-304`); level cells compile in `levelCtx` (`levels.go:212-384`); `groupStatement` unions one leg per level, filtering level n ≥ 2 by the expanded keys (`run.go:384-423`); `expansion.where` writes `<prefix>k<i>` (`run.go:622-652`); a layout's sections are first-level rows (`layout.go:291-321`).

**Files:**
- Modify: `core/server/reports/spec.go` (`Group.Having`), `bind.go` (`layout`: refuse having on a layout's first group), `levels.go` (`levelCtx.cond`, `filter()`, `level.own`), `run.go` (`groupStatement` legs, `compiler.shown`)
- Create: `core/server/reports/having_test.go`

**Interfaces:**
- Produces: `Group.Having *Node` (`json:"having,omitempty"`); `(lv *level) own() (string, error)`; `(c *compiler) shown(n int) string`; every level row's `__own` is 1 when it passes its having. Task 8 reads `__own`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/having_test.go`:

```go
package reports

import "testing"

const speciesHaving = `{"version":2,"source":"zoo.feedings","params":[{"id":"min","type":"number"}],"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species","having":{"op":"gte","args":[
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"min"}]}}],
	"sort":[{"col":"species"},{"col":"note"}]}`

// Having hides groups; a condition on an unset param hides none. The
// default total still adds every base row (OQ6).
func TestHavingHidesGroupsAndAnUnsetParamHidesNone(t *testing.T) {
	z := newZoo(t)
	all := z.run(t, "ann", speciesHaving, Options{}, nil)
	requireRows(t, all, "label", "lion", "zebra", "Total")
	some := z.run(t, "ann", speciesHaving, Options{ExpandAll: true}, map[string]string{"min": "1500"})
	requireRows(t, some, "label", "lion", "lion", "lion", "Total")
	requireRows(t, some, "cost", int64(3500), int64(1000), int64(2500), int64(4500))
}

// A hidden group hides the groups under it.
func TestAHiddenGroupHidesItsChildren(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","having":{"op":"eq","args":[{"col":"species"},{"value":"lion"}]}},{"id":"h","col":"note"}],
		"sort":[{"col":"note"}]}`, Options{ExpandAll: true}, nil)
	// Each note group opens on its one feeding, labelled by the first column.
	requireRows(t, res, "label", "lion", "f1", "lion", "f2", "lion", "Total")
}

func TestHavingReadsARecordField(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal","having":{"op":"gt","args":[{"record":"g","path":[{"field":"weight"}]},{"value":200}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Total")
}

// A having that totals an amount no column shows, in two currencies, still
// groups by the amount's currency: its key is known before the levels (F5).
func TestHavingOnAnAmountNoColumnShows(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 100, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],
		"groups":[{"id":"g","col":"species","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":150}]}}],
		"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Total", "lion", "zebra", "Total")
	requireRows(t, res, "n", int64(1), int64(2), int64(2), int64(4))
}

func TestHavingIsRefusedWhereItCannotWork(t *testing.T) {
	app := withZooRegistry(t)
	for _, c := range []struct{ spec, at, want string }{
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"groups":[{"id":"g","col":"n","having":{"value":3}}]}`,
			"groups.0.having", "having is a condition"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"n","having":{"op":"gt","args":[{"op":"percentOfTotal","scale":2,"args":[{"col":"c"}]},{"value":10}]}}]}`,
			"groups.0.having", "not a condition"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"}],
			"groups":[{"id":"g","col":"n","having":{"op":"gt","args":[{"col":"c"},{"value":0}]}}],"layout":[{"section":"s","title":"S","key":"f1"}]}`,
			"groups.0.having", "a layout's sections are fixed rows"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Having|HiddenGroup' ./reports/`
Expected: FAIL to load: `json: unknown field "having"`.

- [ ] **Step 3: Implement**

`spec.go` `Group`: `Having *Node \`json:"having,omitempty"\`` ("a condition on the group's level; a group that fails it is hidden with every row under it").

`bind.go` `layout`, after the `len(p.spec.Groups) == 0` check (R7):

```go
	if len(p.spec.Groups) > 0 && p.spec.Groups[0].Having != nil {
		b.bad("groups.0.having", "a layout's sections are fixed rows: hide rows on a deeper group")
	}
```

`levels.go`: add `cond bool` to `levelCtx` ("compiling a having: a condition on an unset param matches every group") and change `func (lc *levelCtx) filter() bool { return lc.cond }`. Add:

```go
// own is 1 for a row of this level that passes its group's having.
func (lv *level) own() (string, error) {
	if lv.n == 0 {
		return "1", nil
	}
	g := lv.c.p.groups[lv.n-1]
	if g.group.Having == nil {
		return "1", nil
	}
	at := g.at + ".having"
	e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting, cond: true}, g.group.Having, at)
	if err != nil {
		return "", err
	}
	if e.win {
		return "", invalid(at, "a running value or a percent is not a condition on a group")
	}
	if e.typ != Bool && e.typ != "" {
		return "", invalid(at, "having is a condition")
	}
	if e.unset {
		return "1", nil
	}
	return "COALESCE(" + e.sql + ", 0)", nil
}
```

`rows.go` `rows()`: walk each group's having for currency keys before any level compiles (F5): `r.formulaCurrencies(g.group.Having)` beside the label walk; and in Task 5a's `valueCurrencies`, add `walk(g.group.Having, false)` beside `walk(g.group.Label, false)`.

`run.go` `groupStatement`: compile `own, err := lv.own()` before `lv.render(cells, own, label)` (aggregates it reads join `a<L>`), and replace the leg:

```go
		leg := fmt.Sprintf("SELECT * FROM o%d", n)
		if n >= 1 {
			leg = fmt.Sprintf("SELECT x.* FROM o%d x WHERE %s", n, c.shown(n))
			if n >= 2 {
				leg += " AND " + expand.where(c, s, n-1, "x.")
			}
		}
```

Add:

```go
// shown is true for a level-n row (alias x) that passes its having and whose
// enclosing groups pass theirs: a hidden group hides every row under it.
func (c *compiler) shown(n int) string {
	conds := []string{"x.__own"}
	for m := 1; m < n; m++ {
		if c.p.groups[m-1].group.Having == nil {
			continue
		}
		match := []string{"p.__own"}
		for i := range m {
			match = append(match, fmt.Sprintf("p.k%d IS x.k%d", i, i))
		}
		for _, ck := range c.currencies {
			match = append(match, fmt.Sprintf("p.%s IS x.%s", ck, ck))
		}
		conds = append(conds, fmt.Sprintf("EXISTS (SELECT 1 FROM o%d p WHERE %s)", m, strings.Join(match, " AND ")))
	}
	return strings.Join(conds, " AND ")
}
```

Details of a hidden innermost group never reach the result: `assemble` attaches detail rows only under a group row it emits (`assemble.go:217-227`).

- [ ] **Step 4: Run the reports tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): a report group can hide groups that fail a condition"
git push origin books/p5-trial-balance
```

---

### Task 8: E2 — `columns[].total: "rows"`

Engine facts used: each level's cell (`levels.go:106-110`, `run.go:392-400`); sibling order compiles the sort cells before the level renders (`levels.go:112-153`); pivot cells (`pivot.go:58-81`); layout sections read `o1` cells (`layout.go:228-244`); `o<L+1>` may be read from `o<L>` (probed: SQLite accepts it in any CTE order).

**Files:**
- Modify: `core/server/reports/spec.go` (`Column.Total`), `bind.go` (`columns`: values), `levels.go` (`level.rowsTotal`, `level.childSum`, `sortValue`), `run.go` (`groupStatement` cells), `pivot.go` (`pivotCells`)
- Create: `core/server/reports/rowstotal_test.go`

**Interfaces:**
- Consumes: `__own` (Task 7).
- Produces: `Column.Total string` (`json:"total,omitempty"`, `""`/`"recompute"`/`"rows"`); `(lv *level) childSum(cell string) string`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/rowstotal_test.go`:

```go
package reports

import "testing"

const speciesRows = `{"version":2,"source":"zoo.feedings","params":[{"id":"min","type":"number"}],"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"},
	{"id":"all","path":[{"field":"cost"}],"aggregate":"sum"}],
	"groups":[{"id":"g","col":"species","having":{"op":"gte","args":[
		{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"min"}]}}],
	"sort":[{"col":"species"}]}`

func TestRowsTotalsAddOnlyShownRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", speciesRows, Options{}, nil)
	requireRows(t, res, "shown", int64(3500), int64(1000), int64(4500))
	res = z.run(t, "ann", speciesRows, Options{}, map[string]string{"min": "1500"})
	requireRows(t, res, "label", "lion", "Total")
	requireRows(t, res, "shown", int64(3500), int64(3500))
	requireRows(t, res, "all", int64(3500), int64(4500))
}

// Each currency's total adds that currency's shown rows.
func TestRowsTotalsAddOnlyShownRowsPerCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 100, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
	res := z.run(t, "ann", speciesRows, Options{}, map[string]string{"min": "1500"})
	requireRows(t, res, "label", "Total", "lion", "Total")
	requireRows(t, res, "shown", int64(0), int64(3500), int64(3500))
	requireRows(t, res, "all", int64(100), int64(3500), int64(4500))
}

// A level above the innermost adds its shown child groups.
func TestRowsTotalsAddShownChildGroups(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"},
		{"id":"all","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"name","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}}],
		"sort":[{"col":"species"}]}`, Options{Expand: [][]any{{"lion", "USD"}, {"zebra", "USD"}}}, nil)
	requireRows(t, res, "label", "lion", "Leo", "zebra", "Total")
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(0), int64(3500))
	requireRows(t, res, "all", int64(3500), int64(3500), int64(1000), int64(4500))
}

// With no groups the total adds the detail cells.
func TestRowsTotalWithoutGroupsAddsTheDetailCells(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"big","formula":{"op":"if","args":[{"op":"gt","args":[{"path":[{"field":"cost"}]},{"value":1000}]},{"path":[{"field":"cost"}]},{"value":0}]},"total":"rows"}],
		"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "big", int64(0), int64(2500), int64(0), int64(0), int64(2500))
}

func TestRowsTotalsOnPivotCells(t *testing.T) {
	z := newZoo(t)
	pivoted := speciesRows[:len(speciesRows)-1] + `,"pivot":{"path":[{"field":"fed"}],"bucket":"month","cell":"shown"}}`
	res := z.run(t, "ann", pivoted, Options{}, map[string]string{"min": "1500"})
	requireRows(t, res, "shown.p0", int64(1000), int64(1000))
	requireRows(t, res, "shown.p1", int64(2500), int64(2500))
	requireRows(t, res, "shown.p2", int64(0), int64(0))
}

func TestALayoutSectionAddsItsShownRows(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"name","path":[{"link":"animal"},{"field":"name"}]},
		{"id":"shown","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"}],
		"groups":[{"id":"g","col":"species"},{"id":"h","col":"name","having":{"op":"gt","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":1500}]}}],
		"layout":[{"section":"l","title":"Lions","key":"lion"},{"section":"z","title":"Zebras","key":"zebra"},
			{"row":"d","label":"Diff","formula":{"op":"sub","args":[{"subtotal":"l"},{"subtotal":"z"}]}}]}`, Options{}, nil)
	requireRows(t, res, "label", "Lions", "Total Lions", "Zebras", "Total Zebras", "Diff")
	requireRows(t, res, "shown", int64(3500), int64(3500), int64(0), int64(0), int64(3500))
}

// A number path column with no aggregate adds its rows too (F17): the
// innermost group sums its base rows, the levels above their shown groups.
func TestRowsTotalOnANumberWithoutAnAggregate(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"grams","path":[{"field":"grams"}],"total":"rows"}],
		"groups":[{"id":"g","col":"species"}],"sort":[{"col":"species"}]}`, Options{}, nil)
	requireRows(t, res, "grams", int64(1300), int64(400), int64(1700))
}

func TestRowsTotalIsRefusedWhereItMeansNothing(t *testing.T) {
	app := withZooRegistry(t)
	for _, c := range []struct{ spec, at, want string }{
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"c","path":[{"field":"cost"}],"aggregate":"sum","total":"some"}]}`,
			"columns.0.total", "total is recompute or rows"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}],"total":"rows"}]}`,
			"columns.0.total", "only a number or an amount column adds the rows shown"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]},{"id":"c","path":[{"field":"cost"}],"aggregate":"sum"},
			{"id":"p","formula":{"op":"percentOfTotal","scale":2,"args":[{"col":"c"}]},"total":"rows"}],"groups":[{"id":"g","col":"n"}]}`,
			"columns.2.total", "is not added up"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'RowsTotal|ShownRows|ShownChild|LayoutSectionAdds' ./reports/`
Expected: FAIL to load: `json: unknown field "total"`.

- [ ] **Step 3: Spec and binder**

`spec.go` `Column`, after `Format`:

```go
	// Total is how the column's group, subtotal and total cells are made:
	// "recompute" (the default) from every base row under them, or "rows",
	// the sum of the cells of the rows the result shows directly under them.
	Total string `json:"total,omitempty"`
```

`bind.go` `columns`, inside the loop after the label check:

```go
		switch {
		case c.Total != "" && c.Total != "recompute" && c.Total != "rows":
			b.bad(at+".total", "total is recompute or rows")
		}
```

- [ ] **Step 4: The cells**

`levels.go`:

```go
// childSum adds the cell of the shown rows of the next level that belong to
// the row of this level (alias a), in its currency.
func (lv *level) childSum(cell string) string {
	match := []string{"x.__own"}
	for i := range lv.n {
		match = append(match, fmt.Sprintf("x.k%d IS a.k%d", i, i))
	}
	for _, ck := range lv.c.currencies {
		match = append(match, fmt.Sprintf("x.%s IS a.%s", ck, ck))
	}
	return fmt.Sprintf("(SELECT COALESCE(sum(x.%s), 0) FROM o%d x WHERE %s)", cell, lv.n+1, strings.Join(match, " AND "))
}

// rowsTotal is a "rows" column's cell on this level (R8): the sum of its
// shown child rows above the innermost group; the innermost group keeps its
// own cell; with no groups the total adds the detail cells.
func (lv *level) rowsTotal(cp *colPlan, idx int, e expr) (string, error) {
	at := cp.at + ".total"
	if e.win {
		return "", invalid(at, "a running value or a percent is not added up")
	}
	groups := len(lv.c.p.groups)
	bare := cp.path != nil && cp.col.Aggregate == ""
	if !bare && (!isNumeric(e.typ) || e.typ == Bool || e.typ == "") {
		return "", invalid(at, "only a number or an amount column adds the rows shown")
	}
	if bare && (!isNumeric(cp.path.field.Type) || cp.path.field.Type == Bool) {
		return "", invalid(at, "only a number or an amount column adds the rows shown")
	}
	switch {
	case lv.n < groups:
		return lv.childSum(fmt.Sprintf("v%d", idx)), nil
	case cp.path != nil && cp.col.Aggregate == "" && cp.tm < 0 && lv.groupKey(cp) == "":
		// A value with no aggregate (F17): the innermost group sums its base
		// rows' values; type-checked against the field's type, not the blank cell.
		t := cp.path.field.Type
		if !isNumeric(t) || t == Bool {
			return "", invalid(at, "only a number or an amount column adds the rows shown")
		}
		return lv.agg(fmt.Sprintf("r%d", idx), fmt.Sprintf("COALESCE(sum(w.c%d), 0)", idx)), nil
	case groups == 0:
		d, err := (&rowsCtx{r: lv.r, visiting: map[string]bool{}}).col(cp.col.ID, cp.at)
		if err != nil {
			return "", err
		}
		if !isNumeric(d.typ) || d.typ == Bool || d.typ == "" {
			return "", invalid(at, "only a number or an amount column adds the rows shown")
		}
		return lv.agg(fmt.Sprintf("r%d", idx), "COALESCE(sum("+d.sql+"), 0)"), nil
	}
	return e.sql, nil
}
```

`run.go` `groupStatement`, in the cells loop after `e, err := lv.cell(cp)`:

```go
			if cp.col.Total == "rows" {
				if e.sql, err = lv.rowsTotal(cp, idx, e); err != nil {
					return nil, "", err
				}
			}
```

`levels.go` `sortValue`: a group sorted by a `rows` column above the innermost level sorts by what it shows:

```go
	if cp.col.Total == "rows" && lv.n < len(lv.c.p.groups) {
		return lv.childSum(fmt.Sprintf("v%d", lv.c.p.index(cp))), true, nil
	}
```

(after the window check). `pivot.go` `pivotCells`, inside the values loop:

```go
		if cell.col.Total == "rows" && lv.n < len(c.p.groups) {
			out = append(out, fmt.Sprintf("%s AS pv%d", lv.childSum(fmt.Sprintf("pv%d", j)), j))
			continue
		}
```

- [ ] **Step 5: Run the reports tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): a report column can total the rows it shows"
git push origin books/p5-trial-balance
```

---

### Task 9: E5 — sort by a formula

Engine facts used: `SortKey` is `{col, dir}` and `col` is serialized always (`spec.go:81-84`); `binder.sorts` (`bind.go:505-524`); `level.order` and `sortValue` (`levels.go:112-153`); `detailSelect` builds `s<i>` sort columns and their cursor fingerprints (`run.go:474-529`, `paging.go:13-52`).

**Files:**
- Modify: `core/server/reports/spec.go` (`SortKey`), `bind.go` (`sortPlan`, `sorts`), `levels.go` (`order`), `run.go` (`detailSelect`)
- Create: `core/server/reports/sortformula_test.go`

**Interfaces:**
- Produces: `SortKey{Col string \`json:"col,omitempty"\`; Formula *Node \`json:"formula,omitempty"\`; Dir string}`; `sortPlan{col *colPlan; formula *Node; at string; desc bool}`.

- [ ] **Step 1: Write the failing tests**

```go
package reports

import "testing"

// Groups sort by a formula over the group record; details by a row formula.
func TestSortByAFormula(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"animal","path":[{"link":"animal"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"animal"}],
		"sort":[{"formula":{"record":"g","path":[{"field":"weight"}]},"dir":"desc"}]}`, Options{}, nil)
	requireRows(t, res, "label", "Zed", "Leo", "Total")

	byGrams := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"sort":[{"formula":{"op":"sub","args":[{"value":0},{"path":[{"field":"grams"}]}]}}]}`
	res = z.run(t, "ann", byGrams, Options{}, nil)
	requireRows(t, res, "note", "f2", "f1", "f3", "f4", nil)
	first := z.run(t, "ann", byGrams, Options{Limit: 2}, nil)
	requireRows(t, first, "note", "f2", "f1", nil)
	next := z.run(t, "ann", byGrams, Options{Limit: 2, Cursor: first.Page.Next}, nil)
	requireRows(t, next, "note", "f3", "f4", nil)
}

func TestASortKeyIsAColumnOrAFormula(t *testing.T) {
	app := withZooRegistry(t)
	for _, c := range []struct{ spec, at, want string }{
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"sort":[{"col":"n","formula":{"value":1}}]}`,
			"sort.0", "a column or a formula"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"sort":[{}]}`,
			"sort.0", "a column or a formula"},
		{`{"version":2,"source":"zoo.feedings","columns":[{"id":"c","path":[{"field":"cost"}]}],"sort":[{"formula":{"op":"runningSum","args":[{"col":"c"}]}}]}`,
			"sort.0.formula", "not a sort key"},
	} {
		spec, _, err := Load([]byte(c.spec))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), 400, c.at, c.want)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'SortByAFormula|SortKeyIsA' ./reports/`
Expected: FAIL to load: `json: unknown field "formula"`.

- [ ] **Step 3: Implement**

`spec.go`:

```go
// SortKey sorts by a column or by a formula ("asc" by default). On a group
// level a formula is a cell of that level; on detail rows a row value.
type SortKey struct {
	Col     string `json:"col,omitempty"`
	Formula *Node  `json:"formula,omitempty"`
	Dir     string `json:"dir,omitempty"`
}
```

`bind.go`: `type sortPlan struct { col *colPlan; formula *Node; at string; desc bool }`; in `sorts`, replace the loop body:

```go
		at := fmt.Sprintf("sort.%d", i)
		if s.Dir != "" && s.Dir != "asc" && s.Dir != "desc" {
			b.bad(at+".dir", "dir is asc or desc")
		}
		desc := s.Dir == "desc"
		if (s.Col == "") == (s.Formula == nil) {
			b.bad(at, "a sort key is a column or a formula")
			continue
		}
		if s.Formula != nil {
			p.sort = append(p.sort, sortPlan{formula: s.Formula, at: at + ".formula", desc: desc})
			continue
		}
		cp := b.column(p, s.Col, at+".col")
		if cp == nil {
			continue
		}
		if cp.nest > 0 {
			b.bad(at+".col", "child rows keep their own order")
			continue
		}
		p.sort = append(p.sort, sortPlan{col: cp, at: at + ".col", desc: desc})
```

`rows.go` `rows()`: `for _, s := range c.p.sort { r.formulaCurrencies(s.formula) }` beside the label walk (F5: a sort formula's amounts register their currency keys before the levels).

`levels.go` `order`: replace `lv.sortValue(s.col, fmt.Sprintf("sort.%d.col", i))` with `lv.sortKey(s)` and add:

```go
// sortKey is a sort key's value on this level, or ok false where it means
// nothing (a path, or a formula that reads a row value).
func (lv *level) sortKey(s sortPlan) (string, bool, error) {
	if s.formula == nil {
		return lv.sortValue(s.col, s.at)
	}
	if lv.readsRowValue(s.formula, false, map[string]bool{}) {
		return "", false, nil
	}
	e, err := compileFormula(&levelCtx{lv: lv, visiting: lv.visiting}, s.formula, s.at)
	if err != nil {
		return "", false, err
	}
	if e.win {
		return "", false, notASortKey(s.at)
	}
	return e.sql, true, nil
}
```

`run.go` `detailSelect`, the sort loop:

```go
	for i, s := range c.p.sort {
		ctx := &rowsCtx{r: r, visiting: visiting, detail: frame}
		var e expr
		var err error
		key := ""
		if s.formula != nil {
			e, err = compileFormula(ctx, s.formula, s.at)
			// The formula fingerprints its cursor, so a cursor from another
			// sort is refused.
			raw, _ := json.Marshal(s.formula)
			key = "=" + string(raw)
		} else {
			e, err = ctx.col(s.col.col.ID, s.at)
			key = s.col.col.ID
		}
		if err != nil {
			return "", nil, err
		}
		if e.win {
			return "", nil, notASortKey(s.at)
		}
		name := fmt.Sprintf("s%d", i)
		sel = append(sel, e.sql+" AS "+name)
		order = append(order, sortKey{name: name, desc: s.desc, col: key, typ: e.typ})
		frame.order = append(frame.order, "("+e.sql+" IS NULL)", e.sql+dirOf(s.desc))
	}
```

Update the two other readers of `sortPlan.col` (`grep -n "\.col\.col\|s\.col" core/server/reports/*.go`) to use `s.at` for their error paths; existing tests that expect `sort.N.col` paths keep them (column keys still carry `.col`).

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): a report can sort by a formula"
git push origin books/p5-trial-balance
```

---

### Task 10: E7 — `detail.label`, and `Row.labelColumn`

Engine facts used: `detail` labels a row with its first column (`assemble.go:233-240`); nested child rows the same with their level's first column (`nest.go:125-142`); `detailSelect` (`run.go:474-529`).

**Files:**
- Modify: `core/server/reports/spec.go` (`Detail`, `Spec.Detail`), `bind.go` (`bind`: detail shape), `run.go` (`Row.LabelColumn`, `detailSelect`), `assemble.go` (`detail`), `nest.go` (`nestedRows`)
- Create: `core/server/reports/detaillabel_test.go`

**Interfaces:**
- Produces: `type Detail struct { Label *Node \`json:"label,omitempty"\` }`; `Spec.Detail *Detail` (`json:"detail,omitempty"`); `Row.LabelColumn string` (`json:"labelColumn,omitempty"`).

- [ ] **Step 1: Write the failing tests**

```go
package reports

import "testing"

func TestADetailLabelIsAFormula(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]},{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"sort":[{"col":"note"}]`
	plain := z.run(t, "ann", spec+`}`, Options{}, nil)
	requireRows(t, plain, "label", "f1", "f2", "f3", "f4", "Total")
	if plain.Rows[0].LabelColumn != "note" {
		t.Fatalf("labelColumn = %q", plain.Rows[0].LabelColumn)
	}
	labelled := z.run(t, "ann", spec+`,"detail":{"label":{"op":"concat","args":[{"path":[{"field":"fed"}]},{"value":" · "},{"path":[{"field":"note"}]}]}}}`, Options{}, nil)
	requireRows(t, labelled, "label", "2026-01-05 · f1", "2026-02-10 · f2", "2026-01-20 · f3", "2026-03-03 · f4", "Total")
	if labelled.Rows[0].LabelColumn != "" {
		t.Fatalf("a label that is no cell names none: %q", labelled.Rows[0].LabelColumn)
	}
}

func TestADetailNeedsItsLabel(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"detail":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "detail", "detail has a label")
}
```

(`fed` is a date field: `fieldSQL` reads its day, so the label shows `2026-01-05`.)

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'DetailLabel|DetailNeeds' ./reports/`
Expected: FAIL to compile: `LabelColumn` undefined.

- [ ] **Step 3: Implement**

`spec.go`: add to `Spec` after `Layout`: `Detail *Detail \`json:"detail,omitempty"\``, and:

```go
// Detail shapes detail rows: Label is a formula on a detail row that labels
// it (the first column's value without it).
type Detail struct {
	Label *Node `json:"label,omitempty"`
}
```

`bind.go` `bind`, after `b.layout(p)`:

```go
	if d := spec.Detail; d != nil && d.Label == nil {
		b.bad("detail", "detail has a label")
	}
```

`run.go` `Row`, after `Label`:

```go
	// LabelColumn names the column whose cell the label repeats, so the
	// client can leave that cell out; "" when the label is no cell's value.
	LabelColumn string `json:"labelColumn,omitempty"`
```

`rows.go` `rows()`: `if d := c.p.spec.Detail; d != nil { r.formulaCurrencies(d.Label) }` (F5).

`detailSelect`, before the nest parent: 

```go
	if d := c.p.spec.Detail; d != nil && d.Label != nil {
		e, err := compileFormula(&rowsCtx{r: r, visiting: visiting, detail: frame}, d.Label, "detail.label")
		if err != nil {
			return "", nil, err
		}
		sel = append(sel, e.sql+" AS __label")
	}
```

`assemble.go` `detail`:

```go
func (rn *runner) detail(t *tree, row map[string]any, section string, depth int) {
	id := fmt.Sprintf("d:%v", row["__id"])
	label, labelColumn := rn.firstLabel(rn.c.p.cols, row), rn.c.p.cols[0].col.ID
	if v, ok := row["__label"]; ok {
		label, labelColumn = labelText(v), ""
	}
	rn.push(Row{ID: id, Kind: KindDetail, Depth: depth, Label: label, LabelColumn: labelColumn, Section: section, Cells: rn.cells(row)})
	if len(t.nested) > 0 {
		rn.nestedRows(t, 0, row["__n0"], id, section, depth+1)
	}
}
```

`nest.go` `nestedRows`: add `LabelColumn: cols[0].col.ID` to the pushed `Row`.

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): a report can label its detail rows with a formula"
git push origin books/p5-trial-balance
```

---

### Task 11: E13 — `groups[].open`, and an explicit empty expansion

Engine facts used: `Options.Expand` and `parseExpansion` (`run.go:17-35`, `run.go:576-615`); `expansion.where` and `has` (`run.go:617-659`); the detail pass runs only with expanded keys (`run.go:252`); `groupWithLabel` asks `rn.expand.has(key)` (`assemble.go:214-215`).

**Files:**
- Modify: `core/server/reports/spec.go` (`Group.Open`), `run.go` (`expansion.open`, `parseExpansion` callers, `has`, `where`, detail-pass condition), `assemble.go:215`
- Create: `core/server/reports/open_test.go`

**Interfaces:**
- Produces: `Group.Open bool` (`json:"open,omitempty"`); `(e *expansion) has(key []any, n int) bool`; R10: `Options.Expand == nil && !ExpandAll` → open levels expand; `Expand` non-nil and empty → nothing.

- [ ] **Step 1: Write the failing test**

```go
package reports

import "testing"

func TestOpenGroupsOpenWhenTheRunHasNoExpansion(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"species","open":true}],"sort":[{"col":"note"}]}`
	first := z.run(t, "ann", spec, Options{}, nil)
	requireRows(t, first, "kind", "group", "detail", "detail", "group", "detail", "detail", "total")
	if !first.Rows[0].Expanded {
		t.Fatalf("lion = %+v", first.Rows[0])
	}
	closed := z.run(t, "ann", spec, Options{Expand: [][]any{}}, nil)
	requireRows(t, closed, "kind", "group", "group", "total")
	one := z.run(t, "ann", spec, Options{Expand: [][]any{first.Rows[3].GroupKey}}, nil)
	requireRows(t, one, "kind", "group", "group", "detail", "detail", "total")
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'OpenGroups' ./reports/`
Expected: FAIL to load: `json: unknown field "open"`.

- [ ] **Step 3: Implement**

`spec.go` `Group`: `Open bool \`json:"open,omitempty"\`` ("the level opens when a run has no expansion state").

`run.go`: add `open map[int]bool` to `expansion` ("levels (1 for the first group) that open because the run said nothing"); in `Run`, after `parseExpansion`:

```go
	// No expansion state at all (expand absent, no expandAll): the levels
	// marked open expand. expand: [] is a choice to open nothing (R10).
	if req.Options.Expand == nil && !req.Options.ExpandAll {
		for i, g := range p.groups {
			if g.group.Open {
				expand.open[i+1] = true
			}
		}
	}
```

(`parseExpansion` initializes `open: map[int]bool{}`.) Change `has`:

```go
// has is true when the level-n group with key opens.
func (e *expansion) has(key []any, n int) bool {
	return e.all || e.open[n] || e.set[keyString(key)]
}
```

In `where`, the first line becomes `if e.all || e.open[depth] { return "1" }`. In `run()`, the detail-pass condition becomes:

```go
	if rn.c.p.pivot == nil && (len(rn.c.p.groups) == 0 || len(rn.expand.keys) > 0 || rn.expand.all || rn.expand.open[len(rn.c.p.groups)]) {
```

`assemble.go:215`: `expanded := canOpen && rn.expand.has(key, n)`.

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): report groups can open when the report opens"
git push origin books/p5-trial-balance
```

---

### Task 12: E8 (part 1) — `params[].optionsFrom` in the spec and in a run

Engine facts used: `ParamDef` (`spec.go:111-118`); `checkParamDefs` (`params.go:62-93`) is not app-aware, so source checks go in the binder (`bind.go:161-183`); `choiceValue` (`params.go:191-218`); `paramExpr` binds a multiple choice as a list (`params.go:344-365`); `compileDry` (`run.go:661-682`).

**Files:**
- Modify: `core/server/reports/spec.go` (`OptionsFrom`, `ParamDef.OptionsFrom`), `params.go` (`maxChoiceValues`, `checkParamDefs`, `choiceValue`), `bind.go` (`plan.options`, `optionsPlan`, `binder.optionSources`)
- Create: `core/server/reports/options.go` (only `optionsSelect` in this task; Task 13 adds the rest)
- Modify: `core/server/reports/run.go` (`compileDry` compiles each options select)
- Create: `core/server/reports/options_test.go`

**Interfaces:**
- Produces: `type OptionsFrom struct { Source string; Value, Label, Group Path; Where *Node }` (json `source,value,label,group,where`); `ParamDef.OptionsFrom *OptionsFrom` (`json:"optionsFrom,omitempty"`); `plan.options map[string]*optionsPlan`; `type optionsPlan struct { def *ParamDef; at string; src *Source; value, label, group *pathPlan }`; `(c *compiler) optionsSelect(s *stmt, op *optionsPlan) (string, error)` → `SELECT v, l, g … GROUP BY 1`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/options_test.go`:

```go
package reports

import (
	"fmt"
	"strings"
	"testing"
)

// animalsParam is a feedings report whose Animals filter lists the animals
// the reader may see, grouped by species.
const animalsParam = `{"version":2,"source":"zoo.feedings",
	"params":[{"id":"animals","type":"choice","label":"Animals","multiple":true,
		"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}]}}],
	"columns":[{"id":"note","path":[{"field":"note"}]}],
	"filter":{"op":"in","args":[{"path":[{"link":"animal"}]},{"param":"animals"}]},
	"sort":[{"col":"note"}]}`

// A run checks an options-from-source value by type only.
func TestARunTakesAnyIDForAnOptionsFromSourceParam(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `["` + z.id("leo") + `"]`})
	requireRows(t, res, "note", "f1", "f2", nil)
	res = z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `["not-an-animal"]`})
	requireRows(t, res, "note", nil)
	all := z.run(t, "ann", animalsParam, Options{}, nil)
	requireRows(t, all, "note", "f1", "f2", "f3", "f4", nil)
	many := make([]string, maxChoiceValues+1)
	for i := range many {
		many[i] = fmt.Sprintf("%q", fmt.Sprint("x", i))
	}
	_, err := z.try(t, "ann", animalsParam, Options{}, map[string]string{"animals": "[" + strings.Join(many, ",") + "]"})
	requireSpecError(t, err, 400, "params.animals", "at most 1000")
}

func TestOptionsFromIsChecked(t *testing.T) {
	app := withZooRegistry(t)
	base := `{"version":2,"source":"zoo.feedings","columns":[{"id":"n","path":[{"field":"note"}]}],"params":[%s]}`
	for _, c := range []struct {
		param, at, want string
		status          int
	}{
		{`{"id":"p","type":"text","optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}]}}`, "params.0", "only a choice param has options", 400},
		{`{"id":"p","type":"choice","options":[{"value":"a","label":"A"}],"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}]}}`, "params.0.options", "or names the source they come from", 400},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.gone","value":[{"field":"id"}],"label":[{"field":"name"}]}}`, "params.0.optionsFrom.source", "does not exist", 422},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"nope"}],"label":[{"field":"name"}]}}`, "params.0.optionsFrom.value.0.field", "has no field", 422},
		{`{"id":"p","type":"choice","optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"where":{"value":2}}}`, "params.0.optionsFrom.where", "where is a condition", 400},
	} {
		spec, _, err := Load([]byte(fmt.Sprintf(base, c.param)))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'OptionsFrom' ./reports/`
Expected: FAIL to load: `json: unknown field "optionsFrom"`.

- [ ] **Step 3: Spec and run values**

`spec.go`: add `OptionsFrom *OptionsFrom \`json:"optionsFrom,omitempty"\`` to `ParamDef`, and:

```go
// OptionsFrom reads a choice param's options from a source, under the
// reader's list rules: Value is what the param holds, Label what the picker
// shows, Group (optional) how it groups them, Where which rows count.
type OptionsFrom struct {
	Source string `json:"source"`
	Value  Path   `json:"value"`
	Label  Path   `json:"label"`
	Group  Path   `json:"group,omitempty"`
	Where  *Node  `json:"where,omitempty"`
}
```

`params.go`: add `maxChoiceValues = 1000` to the const block ("the most values a multiple choice holds"). In `checkParamDefs`, the non-choice case refuses `d.OptionsFrom != nil` with the same message, and the choice case becomes:

```go
		case ParamChoice:
			if (len(d.Options) == 0) == (d.OptionsFrom == nil) {
				return invalid(at+".options", "a choice param lists its options, or names the source they come from")
			}
```

In `choiceValue`, replace `valid` and bound the list (R13):

```go
	valid := func(s string) bool {
		if d.OptionsFrom != nil {
			// Its options live in a source the reader's rules filter: a run
			// checks the value's type only.
			return s != "" && len(s) <= maxParamText
		}
		return slices.ContainsFunc(d.Options, func(o Option) bool { return o.Value == s })
	}
```

and after `items, ok := v.([]any)`: `if len(items) > maxChoiceValues { return out, invalid(at, fmt.Sprintf("the value is a list of at most %d choices", maxChoiceValues)) }`.

- [ ] **Step 4: The binder and the options select**

`bind.go`: add `options map[string]*optionsPlan` to `plan` and `optionsPlan` (Interfaces above). In `bind`, after `b.merge(checkParamDefs(spec.Params))`, call `b.optionSources(p)`:

```go
// optionSources binds each choice param's options source: its value, label
// and group paths, to-one only.
func (b *binder) optionSources(p *plan) {
	p.options = map[string]*optionsPlan{}
	for i := range p.spec.Params {
		d := &p.spec.Params[i]
		if d.Type != ParamChoice || d.OptionsFrom == nil {
			continue
		}
		at := fmt.Sprintf("params.%d.optionsFrom", i)
		of := d.OptionsFrom
		src := lookupSource(b.app, of.Source)
		if src == nil {
			b.missing(at+".source", "unknown_source", fmt.Sprintf("the source %q does not exist", of.Source))
			continue
		}
		op := &optionsPlan{def: d, at: at, src: src}
		op.value = b.path(src, of.Value, at+".value", pathValue, "")
		op.label = b.path(src, of.Label, at+".label", pathValue, "")
		if of.Group != nil {
			op.group = b.path(src, of.Group, at+".group", pathValue, "")
		}
		if op.value != nil && op.label != nil {
			p.options[d.ID] = op
		}
	}
}
```

Create `options.go`:

```go
package reports

import "fmt"

// optionsSelect selects a choice param's options: the rows of its source
// the reader may list that pass its where, one row per value (v) with its
// label (l) and group (g) as text. A where on another param reads that
// param's value for the request.
func (c *compiler) optionsSelect(s *stmt, op *optionsPlan) (string, error) {
	sc := newScope(s, op.src)
	value := "CAST(" + sc.value(op.value).sql + " AS TEXT)"
	label := "CAST(" + sc.value(op.label).sql + " AS TEXT)"
	group := "NULL"
	if op.group != nil {
		group = "CAST(" + sc.value(op.group).sql + " AS TEXT)"
	}
	where := "1"
	if w := op.def.OptionsFrom.Where; w != nil {
		// A rows CTE with no root: a where reads its source's paths, never a
		// report column (joinCtx.col refuses outside the root scope).
		r := &rowsCTE{c: c, s: s}
		e, err := compileWhere(&joinCtx{r: r, sc: sc, src: op.src, isFilter: true, visiting: map[string]bool{}}, w, op.at)
		if err != nil {
			return "", err
		}
		if !e.unset {
			where = "COALESCE(" + e.sql + ", 0)"
		}
	}
	return fmt.Sprintf("SELECT %s AS v, min(COALESCE(%s, '')) AS l, min(%s) AS g\n%s\nWHERE %s AND %s IS NOT NULL\nGROUP BY 1",
		value, label, group, sc.render(), where, value), nil
}
```

`run.go` `compileDry`, after `groupStatement`:

```go
	for _, d := range p.spec.Params {
		if op := p.options[d.ID]; op != nil {
			if _, err := c.optionsSelect(newStmt(nil, nil), op); err != nil {
				return err
			}
		}
	}
```

- [ ] **Step 5: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): a report choice param can take its options from a source"
git push origin books/p5-trial-balance
```

---

### Task 12a: F18 — an amount param (a number of cents and a currency)

Engine facts used: param types and parsing (`params.go:15-218`), `paramExpr` (`params.go:344-365`), the currency rules (`formula.go:32-74`, Task 5a's `currency.id`), `operator` (`formula.go:288-304`).

Design (R29): a param of type `amount` holds a whole number of cents and a currency code: `{"amount": 150000, "currency": "USD"}`, or just a number for "in any currency". It is money: on a filter, a having, or in a formula it compares and combines with amounts under the money rules. Where an amount param with a currency meets an amount with its own currency (a currency key, a fixed currency, a carried currency), the operation holds only where the two currencies are the same: the compiled SQL is guarded `CASE WHEN <that amount's currency> IS <the param's currency> THEN <the operation> END`, so a USD budget never compares with EUR totals (the condition is false, a computed amount is blank). So "Ask when run" can ask a money total (R18).

**Files:**
- Modify: `core/server/reports/params.go` (`ParamAmount`, parse, resolved value, `paramExpr`), `formula.go` (`currency.param`, the guard in `operator`, `fctx.currencySQL`), `levels.go`/`detail.go`/`join.go`/`lookup.go`/`layout.go`/`formula_cases_test.go` (`currencySQL`)
- Modify: `core/reports/lib/types.ts` (`PARAM_TYPES` gains `'amount'`; `ResolvedParam.value` may be `{ amount: number; currency: string }`), `lib/params.ts` (`{ kind: 'amount'; cents; currency }` values, JSON, chip text), `lib/view-url.ts` (`p-<id>=150.00 USD`)
- Create: `core/server/reports/amountparam_test.go`; Test: `core/reports/lib/__tests__/params.test.ts`, `view-url.test.ts`

**Interfaces:**
- Produces: `ParamAmount = "amount"`; `paramValue.currency string`; `currency.param string` (a param's currency code; `""` is "any"); `fctx.currencySQL(cur currency) string`; TS `ParamValue` member `{ kind: 'amount'; cents: number; currency: string }`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/amountparam_test.go`:

```go
package reports

import "testing"

const budgetHaving = `{"version":2,"source":"zoo.feedings","params":[{"id":"budget","type":"amount","label":"Budget"}],"columns":[
	{"id":"species","path":[{"link":"animal"},{"field":"species"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"},
	{"id":"left","formula":{"op":"sub","args":[{"param":"budget"},{"op":"sum","args":[{"path":[{"field":"cost"}]}]}]}}],
	"groups":[{"id":"g","col":"species","having":{"op":"gte","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"param":"budget"}]}}],
	"sort":[{"col":"species"}]}`

// An amount param compares and combines only with amounts of its currency.
func TestAnAmountParamKeepsToItsCurrency(t *testing.T) {
	z := newZoo(t)
	z.rec(t, "f7", "zoo_feedings", map[string]any{"animal": z.id("leo"), "cost": 100, "currency": "EUR",
		"fed": "2026-01-07 00:00:00.000Z", "grams": 10, "note": "f7", "due": "2026-01-07"})
	all := z.run(t, "ann", budgetHaving, Options{}, nil)
	requireRows(t, all, "label", "lion", "Total", "lion", "zebra", "Total")

	usd := z.run(t, "ann", budgetHaving, Options{}, map[string]string{"budget": `{"amount":1500,"currency":"USD"}`})
	requireRows(t, usd, "label", "Total", "lion", "Total")
	requireRows(t, usd, "left", nil, int64(-2000), int64(-3000))

	eur := z.run(t, "ann", budgetHaving, Options{}, map[string]string{"budget": `{"amount":50,"currency":"EUR"}`})
	requireRows(t, eur, "label", "lion", "Total", "Total")
	requireRows(t, eur, "left", int64(-50), int64(-50), nil)

	any := z.run(t, "ann", budgetHaving, Options{}, map[string]string{"budget": `1500`})
	requireRows(t, any, "label", "Total", "lion", "Total")
}

func TestAnAmountParamValueIsChecked(t *testing.T) {
	z := newZoo(t)
	for _, bad := range []string{`"12"`, `{"amount":1.5,"currency":"USD"}`, `{"amount":100,"currency":"usd"}`, `{"amount":100,"rate":2}`} {
		_, err := z.try(t, "ann", budgetHaving, Options{}, map[string]string{"budget": bad})
		requireSpecError(t, err, 400, "params.budget", "an amount is")
	}
}
```

(The rows come one total per currency, EUR first. Unset, the having matches every group; with a USD budget the EUR rows fail the guard; `left` is blank on rows whose currency differs from the budget's.)

In `params.test.ts`: `valueFromJson({ id: 'b', type: 'amount' }, { amount: 150000, currency: 'USD' })` gives `{ kind: 'amount', cents: 150000, currency: 'USD' }`; `valueToJson` gives it back; `chipText({ id: 'b', type: 'amount', label: 'Budget' }, { kind: 'amount', cents: 150000, currency: 'USD' }, undefined)` is `'Budget: USD 1,500.00'`. In `view-url.test.ts`: `p-b=1500.00 USD` parses to that value and serializes back; `p-b=12` (no currency) parses to `{ kind: 'amount', cents: 1200, currency: '' }`.

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'AmountParam' ./reports/`
Expected: FAIL: `unknown param type "amount"`.

- [ ] **Step 3: Engine**

`params.go`: `ParamAmount = "amount"` beside the others; `checkParamDefs` treats it like `ParamNumber` (no options); `paramValue` gains `currency string`; in `parseParam`:

```go
	case ParamAmount:
		cents, cur, ok := amountValue(v)
		if !ok {
			return out, invalid(at, `an amount is a whole number of cents, or {"amount": cents, "currency": "USD"}`)
		}
		out.value, out.currency = cents, cur
```

```go
var currencyCodeRe = regexp.MustCompile(`^[A-Z]{3}$`)

func amountValue(v any) (int64, string, bool) {
	whole := func(n any) (int64, bool) {
		num, ok := n.(json.Number)
		if !ok {
			return 0, false
		}
		i, err := num.Int64()
		return i, err == nil
	}
	if i, ok := whole(v); ok {
		return i, "", true
	}
	m, ok := v.(map[string]any)
	if !ok || len(m) > 2 {
		return 0, "", false
	}
	i, ok := whole(m["amount"])
	cur, _ := m["currency"].(string)
	if !ok || (len(m) == 2 && !currencyCodeRe.MatchString(cur)) {
		return 0, "", false
	}
	return i, cur, true
}
```

`resolvedParams` gives an amount `Value: map[string]any{"amount": v.value, "currency": v.currency}`. `paramExpr`:

```go
	case ParamAmount:
		if v.unset {
			return expr{sql: "NULL", typ: Money, unset: true}
		}
		return expr{sql: c.bind(v.value), typ: Money, cur: currency{param: v.currency, id: "param:" + v.currency}}
```

`formula.go`: `currency` gains `param string`; `none()` is also false for a param with a code (`a.param != ""`); `combined` skips a value whose currency is a param's (the guard takes it); `fctx` gains `currencySQL(cur currency) string` — `rowsCtx`: `"w." + key`; `joinCtx`: `j.r.inner("w." + key)`; `levelCtx`: `"a." + key`; `layoutCtx`: `"cur." + key`; `lookupCtx`: `l.outer.currencySQL(cur)`; `tableCtx`: `"NULL"`; in every context a fixed currency is `c.bind(fixed.Currency(app))` (`""` when dry), a carried one is `cur.sql`, a param's is `c.bind(cur.param)`. In `operator`, after `operatorExpr`:

```go
	// An amount param holds only against amounts of its currency (R29).
	if p, other, ok := paramAgainst(args); ok && (conditionOps[n.Op] || e.typ == Money) {
		e.sql = fmt.Sprintf("(CASE WHEN %s IS %s THEN %s END)", c.currencySQL(other), c.bind(p.param), e.sql)
	}
```

```go
// paramAgainst finds an amount param with a currency among values and another
// amount whose currency is not a param's.
func paramAgainst(values []expr) (currency, currency, bool) {
	var p, other currency
	for _, v := range values {
		switch {
		case v.cur.param != "":
			p = v.cur
		case !v.cur.none():
			other = v.cur
		}
	}
	return p, other, p.param != "" && !other.none()
}
```

(A condition's guard yields NULL where the currencies differ, which `own()` and the filter read as false.)

- [ ] **Step 4: Client values, chips and URL**

`types.ts`: `PARAM_TYPES = [..., 'amount']`; `ResolvedParam.value?: string | number | { amount: number; currency: string }`. `params.ts`: `ParamValue` gains `| { kind: 'amount'; cents: number; currency: string }`; `valueFromJson` for `amount` reads a whole number (currency `''`) or `{ amount, currency }` (currency `/^[A-Z]{3}$/` or absent); `valueToJson` writes `{ amount: cents, currency }` (just the number when the currency is `''`); `valueText` writes `"<currency> <amount>"` with the core money formatter (`formatCents` in `lib/money.ts`), or the amount alone with no currency. `view-url.ts`: an amount is `"<amount> <CUR>"` (`textFromCents` / `centsFromText` from `lib/builder/filter.ts`, moved to `lib/money.ts` so both read them).

- [ ] **Step 5: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/server/reports core/reports
git commit -m "feat(core): amount params, kept to their currency"
git push origin books/p5-trial-balance
```

---

### Task 13: E8 (part 2) — `POST /api/reports/param-options`

Engine facts used: `Register` binds the read routes with `RequireAuth("users")` and a body limit (`endpoint.go:48-66`); `handleRun` decodes strictly, reads a saved report through `viewableSpec` and answers refusals with `runError` (`endpoint.go:104-247`); `runner.query` runs a statement under the time limit (`run.go:292-318`); OAuth classifies no scope for a report route, so a token is refused by default-deny (`endpoint_test.go:159-186`).

**Files:**
- Modify: `core/server/reports/options.go` (`OptionsRequest`, `ParamOptions`, `Options`, `handleParamOptions`, cursor, `queryRows`)
- Modify: `core/server/reports/endpoint.go` (route; `requestSpec` shared by both handlers)
- Modify: `core/server/reports/options_test.go`

**Interfaces:**
- Consumes: `plan.options`, `optionsSelect` (Task 12).
- Produces: `func ReadParamOptions(ctx context.Context, app core.App, req OptionsRequest) (*ParamOptions, error)` (not `Options`, which is the run's options type, `run.go:18`); `ParamOptions{Options []ParamOption; Groups []OptionGroup; Next string}` (json `options,groups,next`); `ParamOption{Value, Label, Group string}`; `OptionGroup{Value, Label string}`; route `POST /api/reports/param-options`; `requestSpec(re *core.RequestEvent, spec json.RawMessage, reportID string) ([]byte, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `options_test.go`:

```go
func (z *zoo) options(t *testing.T, user, spec string, req OptionsRequest) *ParamOptions {
	t.Helper()
	loaded, _, err := Load([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	req.Spec, req.Auth = loaded, z.recs[user]
	res, err := ReadParamOptions(context.Background(), z.app, req)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	return res
}

func optionLabels(res *ParamOptions) []string {
	out := []string{}
	for _, o := range res.Options {
		out = append(out, o.Label)
	}
	return out
}

// Review Focus 4: by list, search, group or values, a reader gets only rows
// their rules let them list.
func TestParamOptionsReadOnlyRowsTheReaderMayList(t *testing.T) {
	z := newZoo(t)
	if got := optionLabels(z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"})); !slices.Equal(got, []string{"Kit", "Leo", "Zed"}) {
		t.Fatalf("ann = %v", got)
	}
	if got := optionLabels(z.options(t, "bob", animalsParam, OptionsRequest{Param: "animals"})); !slices.Equal(got, []string{"Mo", "Zed"}) {
		t.Fatalf("bob = %v", got)
	}
	// "o" is in Leo and in bob's Mo, which ann may not list.
	if got := optionLabels(z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Search: "o"})); !slices.Equal(got, []string{"Leo"}) {
		t.Fatalf("search = %v", got)
	}
	lion := "lion"
	if got := optionLabels(z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Group: &lion})); !slices.Equal(got, []string{"Leo"}) {
		t.Fatalf("group = %v", got)
	}
	values := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Values: []string{z.id("leo"), z.id("mo")}})
	if got := optionLabels(values); !slices.Equal(got, []string{"Leo"}) || values.Options[0].Value != z.id("leo") || values.Options[0].Group != "lion" {
		t.Fatalf("values = %+v", values.Options)
	}
	groups := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals"}).Groups
	if len(groups) != 3 || groups[0] != (OptionGroup{Value: "cat", Label: "cat"}) {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestParamOptionsPageByLabel(t *testing.T) {
	z := newZoo(t)
	first := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Limit: 2})
	if got := optionLabels(first); !slices.Equal(got, []string{"Kit", "Leo"}) || first.Next == "" {
		t.Fatalf("first = %v next %q", got, first.Next)
	}
	next := z.options(t, "ann", animalsParam, OptionsRequest{Param: "animals", Limit: 2, After: first.Next})
	if got := optionLabels(next); !slices.Equal(got, []string{"Zed"}) || next.Next != "" {
		t.Fatalf("next = %v next %q", got, next.Next)
	}
	if len(first.Groups) == 0 || len(next.Groups) != 0 {
		t.Fatalf("groups come with the first page only: %v then %v", first.Groups, next.Groups)
	}
}

func TestParamOptionsWhereAndRefusals(t *testing.T) {
	z := newZoo(t)
	active := strings.Replace(animalsParam, `"group":[{"field":"species"}]`,
		`"group":[{"field":"species"}],"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}`, 1)
	if got := optionLabels(z.options(t, "bob", active, OptionsRequest{Param: "animals"})); !slices.Equal(got, []string{"Zed"}) {
		t.Fatalf("bob active = %v", got)
	}
	loaded, _, _ := Load([]byte(animalsParam))
	for _, c := range []struct {
		req       OptionsRequest
		at, want string
	}{
		{OptionsRequest{Param: "nope"}, "param", "no choice param"},
		{OptionsRequest{Param: "animals", Search: strings.Repeat("x", 201)}, "search", "at most 200"},
		{OptionsRequest{Param: "animals", Limit: 1001}, "limit", "1 to 1000"},
		{OptionsRequest{Param: "animals", After: "garbage"}, "after", "not one this list gave"},
	} {
		c.req.Spec, c.req.Auth = loaded, z.recs["ann"]
		_, err := ReadParamOptions(context.Background(), z.app, c.req)
		requireSpecError(t, err, 400, c.at, c.want)
	}
	_, err := ReadParamOptions(context.Background(), z.app, OptionsRequest{Spec: loaded, Param: "animals"})
	if !errors.Is(err, ErrNoReader) {
		t.Fatalf("no reader: %v", err)
	}
}

func TestParamOptionsEndpoint(t *testing.T) {
	s := newSavedZoo(t)
	o := s.saved(t, "O", "ann", "O", animalsParam, false)
	const url = "/api/reports/param-options"
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"animals"}`, http.StatusOK, `"label":"Kit"`, `"groups":[`)
	s.expect(t, "ann", http.MethodPost, url, `{"reportId":"`+o.Id+`","param":"animals","search":"ze"}`, http.StatusOK, `"label":"Zed"`)
	s.expect(t, "ot", http.MethodPost, url, `{"reportId":"`+o.Id+`","param":"animals"}`, http.StatusForbidden, `"reportId":{"code":"no_access"`)
	s.expect(t, "", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"animals"}`, http.StatusUnauthorized)
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"param":"animals","sql":"x"}`, http.StatusBadRequest, `unknown field`)
	s.expect(t, "ann", http.MethodPost, url, `{"spec":`+animalsParam+`,"reportId":"`+o.Id+`","param":"animals"}`, http.StatusBadRequest, `not both`)
	if got := oauth.ScopeForRoute(http.MethodPost, url); len(got) != 0 {
		t.Fatalf("param-options resolved to scopes %v, want default-deny", got)
	}
}
```

(Add imports: `context`, `errors`, `net/http`, `slices`, `tinycld.org/core/oauth`. `s.expect` is the saved-zoo helper, `saved_fixture_test.go:129-141`, which sends through the real router with the user's session token, or none for `""`.)

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'ParamOptions' ./reports/`
Expected: FAIL to compile: `undefined: ReadParamOptions`.

- [ ] **Step 3: Implement the options read**

Append to `options.go` (imports: `bytes`, `context`, `encoding/base64`, `encoding/json`, `errors`, `io`, `net/http`, `strings`, `github.com/pocketbase/pocketbase/core`):

```go
// Param-options limits (R12).
const (
	optionsPage     = 50
	maxOptionSearch = 200
	maxOptionGroups = 200
)

// OptionsRequest asks for one page of a choice param's options.
type OptionsRequest struct {
	Spec   *Spec
	Param  string
	Search string
	// After is the previous page's next; Group narrows to one group's
	// options; Values labels the given values (no paging).
	After  string
	Group  *string
	Values []string
	Limit  int
	// Params are the report's other params, for a where that reads them.
	Params  map[string]json.RawMessage
	Today   string
	Auth    *core.Record
	Headers map[string]string
}

type ParamOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Group string `json:"group,omitempty"`
}

type OptionGroup struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type ParamOptions struct {
	Options []ParamOption `json:"options"`
	Groups  []OptionGroup `json:"groups"`
	Next    string        `json:"next,omitempty"`
}

// ReadParamOptions reads one page of a choice param's options for its reader, under
// the reader's list rules, ordered by label then value. It writes nothing.
func ReadParamOptions(ctx context.Context, app core.App, req OptionsRequest) (*ParamOptions, error) {
	if req.Auth == nil || req.Auth.IsSuperuser() {
		return nil, ErrNoReader
	}
	p, specErr := bind(app, req.Spec)
	if specErr != nil {
		return nil, specErr
	}
	op := p.options[req.Param]
	if op == nil {
		return nil, invalid("param", fmt.Sprintf("the report has no choice param %q whose options come from a source", req.Param))
	}
	limit := req.Limit
	switch {
	case limit == 0:
		limit = optionsPage
	case limit < 1 || limit > maxChoiceValues:
		return nil, invalid("limit", fmt.Sprintf("limit is 1 to %d", maxChoiceValues))
	}
	if len(req.Search) > maxOptionSearch {
		return nil, invalid("search", fmt.Sprintf("a search has at most %d characters", maxOptionSearch))
	}
	if len(req.Values) > maxChoiceValues {
		return nil, invalid("values", fmt.Sprintf("at most %d values", maxChoiceValues))
	}
	today, err := todayOf(req.Today)
	if err != nil {
		return nil, err
	}
	params, specErr := resolveParams(p.spec.Params, req.Params, periodContext{today: today, fiscalStart: fiscalStartOf(p.base, app)})
	if specErr != nil {
		return nil, specErr
	}
	c := newCompiler(p, app, listRequestInfo(req.Auth, req.Headers), params)
	res := &ParamOptions{Options: []ParamOption{}, Groups: []OptionGroup{}}
	if err := c.optionsPage(ctx, op, req, limit, res); err != nil {
		return nil, err
	}
	// Groups come with the first page only; later pages carry options (F9).
	if op.group != nil && len(req.Values) == 0 && req.After == "" {
		if err := c.optionGroups(ctx, op, req, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (c *compiler) optionsPage(ctx context.Context, op *optionsPlan, req OptionsRequest, limit int, res *ParamOptions) error {
	s := newStmt(c.app, c.info)
	inner, err := c.optionsSelect(s, op)
	if err != nil {
		return err
	}
	conds := c.optionConds(s, req)
	if len(req.Values) > 0 {
		parts := make([]string, len(req.Values))
		for i, v := range req.Values {
			parts[i] = s.bind(v)
		}
		conds = append(conds, "o.v IN ("+strings.Join(parts, ", ")+")")
	} else if req.After != "" {
		label, value, err := readOptionsCursor(req.After)
		if err != nil {
			return err
		}
		conds = append(conds, fmt.Sprintf("(o.l > %s OR (o.l = %s AND o.v > %s))", s.bind(label), s.bind(label), s.bind(value)))
	}
	body := fmt.Sprintf("SELECT o.v AS v, o.l AS l, o.g AS g FROM (%s) o WHERE %s ORDER BY o.l, o.v LIMIT %s",
		inner, strings.Join(conds, " AND "), s.bind(int64(limit+1)))
	rows, err := queryRows(ctx, c.app, s, body)
	if err != nil {
		return err
	}
	more := len(rows) > limit && len(req.Values) == 0
	if len(rows) > limit {
		rows = rows[:limit]
	}
	for _, row := range rows {
		res.Options = append(res.Options, ParamOption{
			Value: fmt.Sprint(row["v"]),
			Label: optionText(op.label.field, row["l"]),
			Group: groupValue(row["g"]),
		})
	}
	if more {
		last := rows[len(rows)-1]
		res.Next = optionsCursor(fmt.Sprint(last["l"]), fmt.Sprint(last["v"]))
	}
	return nil
}

func (c *compiler) optionGroups(ctx context.Context, op *optionsPlan, req OptionsRequest, res *ParamOptions) error {
	s := newStmt(c.app, c.info)
	inner, err := c.optionsSelect(s, op)
	if err != nil {
		return err
	}
	conds := append(c.optionConds(s, OptionsRequest{Search: req.Search}), "o.g IS NOT NULL")
	body := fmt.Sprintf("SELECT DISTINCT o.g AS g FROM (%s) o WHERE %s ORDER BY o.g LIMIT %s",
		inner, strings.Join(conds, " AND "), s.bind(int64(maxOptionGroups)))
	rows, err := queryRows(ctx, c.app, s, body)
	if err != nil {
		return err
	}
	for _, row := range rows {
		v := groupValue(row["g"])
		res.Groups = append(res.Groups, OptionGroup{Value: v, Label: optionText(op.group.field, v)})
	}
	return nil
}

// optionConds are the search and group conditions on the options select (o).
func (c *compiler) optionConds(s *stmt, req OptionsRequest) []string {
	conds := []string{"1"}
	if req.Search != "" {
		conds = append(conds, "instr(lower(o.l), lower("+s.bind(req.Search)+")) > 0")
	}
	if req.Group != nil {
		conds = append(conds, "o.g IS "+s.bind(*req.Group))
	}
	return conds
}

// optionText shows a value as the picker does: a select's option label, or
// the text itself; a blank reads (none).
func optionText(f *Field, v any) string {
	text := groupValue(v)
	if f != nil && f.Type == Select {
		for _, o := range f.Options {
			if o.Value == text {
				return o.Label
			}
		}
	}
	if text == "" {
		return blankLabel
	}
	return text
}

func groupValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func optionsCursor(label, value string) string {
	b, _ := json.Marshal([]string{label, value})
	return base64.RawURLEncoding.EncodeToString(b)
}

func readOptionsCursor(text string) (string, string, error) {
	bad := invalid("after", "the cursor is not one this list gave")
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return "", "", bad
	}
	var parts []string
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 2 {
		return "", "", bad
	}
	return parts[0], parts[1], nil
}

// queryRows runs a read statement under the run time limit: readRows,
// shared with the runner (below), with no topics.
func queryRows(ctx context.Context, app core.App, s *stmt, body string) ([]map[string]any, error) {
	rows, _, err := readRows(ctx, app, s, nil, body, false)
	return rows, err
}

type paramOptionsBody struct {
	Spec     json.RawMessage            `json:"spec"`
	ReportID string                     `json:"reportId"`
	Param    string                     `json:"param"`
	Search   string                     `json:"search"`
	After    string                     `json:"after"`
	Group    *string                    `json:"group"`
	Values   []string                   `json:"values"`
	Limit    int                        `json:"limit"`
	Params   map[string]json.RawMessage `json:"params"`
	Today    string                     `json:"today"`
}

// handleParamOptions answers POST /api/reports/param-options: one page of a
// choice param's options, for a draft spec or a saved report the caller can
// view (the reports read exception, OQ5). It writes nothing.
func handleParamOptions(re *core.RequestEvent) error {
	var body paramOptionsBody
	if err := decodeStrict(re, &body, "a param-options request"); err != nil {
		return err
	}
	raw, err := requestSpec(re, body.Spec, body.ReportID)
	if err != nil {
		return err
	}
	spec, _, err := Load(raw)
	if err != nil {
		return runError(re, err)
	}
	res, err := ReadParamOptions(re.Request.Context(), re.App, OptionsRequest{
		Spec: spec, Param: body.Param, Search: body.Search, After: body.After, Group: body.Group,
		Values: body.Values, Limit: body.Limit, Params: body.Params, Today: body.Today,
		Auth: re.Auth, Headers: ruleHeaders(re.Request.Header),
	})
	if err != nil {
		return runError(re, err)
	}
	return re.JSON(http.StatusOK, res)
}
```

(`bytes` is not needed here; leave it out.)

- [ ] **Step 4: Share the statement runner, the strict decode and the spec-or-report rule; bind the route**

Extract from `run.go` `runner.query` (`run.go:292-318`) the statement runner both reads use (F14), and make `runner.query` call it:

```go
// readRows runs one read statement with the rule CTEs and ctes under the run
// time limit; with topics, it also returns the change-signal topics of every
// collection the statement reads.
func readRows(ctx context.Context, app core.App, s *stmt, ctes []string, body string, topics bool) ([]map[string]any, []string, error) {
	with, err := s.with(ctes...)
	if err != nil {
		return nil, nil, err
	}
	sql := with + body
	ctx, cancel := context.WithTimeout(ctx, runLimits.timeout)
	defer cancel()
	var read []string
	if topics {
		if read, err = changesignal.TopicsRead(ctx, app, sql, s.params); err != nil {
			return nil, nil, mapRunError(err)
		}
	}
	rows, err := app.DB().NewQuery(sql).Bind(s.params).WithContext(ctx).Rows()
	if err != nil {
		return nil, nil, mapRunError(err)
	}
	out, err := scanRows(rows)
	if err != nil {
		return nil, nil, mapRunError(err)
	}
	return out, read, nil
}

func (rn *runner) query(s *stmt, ctes []string, body string) ([]map[string]any, error) {
	rows, topics, err := readRows(rn.ctx, rn.app, s, ctes, body, true)
	for _, t := range topics {
		rn.topics[t] = true
	}
	return rows, err
}
```

and from `handleRun`'s decode (`endpoint.go:105-113`):

```go
// decodeStrict reads one JSON object with no unknown field and nothing after
// it, answering 400 naming what the request should have been.
func decodeStrict(re *core.RequestEvent, into any, what string) error {
	dec := json.NewDecoder(re.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return re.BadRequestError("The request is not "+what+": "+err.Error(), nil)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return re.BadRequestError("The request is not "+what+": it has data after the first object.", nil)
	}
	return nil
}
```

`handleRun` calls `decodeStrict(re, &body, "a report run")` (its messages stay as `TestRunEndpoint` expects: "The request is not a report run: …").


In `endpoint.go`, move the spec/reportId switch of `handleRun` (`endpoint.go:114-126`) into:

```go
// requestSpec is a request's spec: the posted draft, or a saved report's
// spec read through its view rule. Never both.
func requestSpec(re *core.RequestEvent, spec json.RawMessage, reportID string) ([]byte, error) {
	switch {
	case reportID != "" && spec != nil:
		return nil, re.BadRequestError("The request has a spec or a reportId, not both.", nil)
	case reportID != "":
		return viewableSpec(re, reportID)
	case spec == nil:
		return nil, re.BadRequestError("The request has no spec and no reportId.", nil)
	}
	return []byte(spec), nil
}
```

`handleRun` calls it (`raw, err := requestSpec(re, body.Spec, body.ReportID)`; `saved := body.ReportID != ""` stays for the saved-report topics). In `Register`, beside the run route:

```go
		e.Router.POST("/api/reports/param-options", handleParamOptions).
			Bind(apis.RequireAuth("users"), apis.BodyLimit(maxRunBody))
```

and extend `Register`'s doc comment: "and POST /api/reports/param-options, the options of a choice param read from a source (ReadParamOptions)".

- [ ] **Step 5: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/ ./oauth/`
Expected: PASS (the OAuth route table needs no entry: an unknown route is denied).

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): POST /api/reports/param-options reads a choice param's options"
git push origin books/p5-trial-balance
```

---

### Task 13a: F19 — a whole group picked from a source is expanded on the server

Engine facts used: an options-from-source choice value is checked by type only (Task 12, `choiceValue`); `optionsSelect` reads the options under the reader's rules (Task 12); every context resolves a param through `paramOf` and `paramExpr` (`join.go:21-27`, `params.go:344-365`).

Design (R30, changing R13): a multiple choice from a source may hold, besides record values, group references `{"group": "<group value>"}`. The server expands a group reference at run time to every option of that group the reader may list (the options select, filtered by group), so a group has no size cap and follows rule changes. Explicit values keep the cap of 1000; past it the run says "the value is a list of at most 1000 choices; pick a whole group instead", and the picker says the same before Apply. Group references are capped at 200 (the groups a picker lists).

**Files:**
- Modify: `core/server/reports/params.go` (`paramValue.groups`, `choiceValue`, `ResolvedParam.Groups`), `options.go` (`groupListSQL`), `join.go`/`detail.go`/`levels.go`/`lookup.go` (their `param` builds the list through `paramLeaf`)
- Modify: `core/server/reports/options_test.go`

**Interfaces:**
- Consumes: Tasks 12, 13.
- Produces: `paramValue.groups []string`; `ResolvedParam.Groups []string` (`json:"groups,omitempty"`); `paramLeaf(c *compiler, s *stmt, f fctx, id string, v paramValue) expr`; `(c *compiler) groupListSQL(s *stmt, op *optionsPlan, values, groups []string) string`.

- [ ] **Step 1: Write the failing test**

Append to `options_test.go`:

```go
// A group reference picks every option of the group the reader may list, at
// run time: no cap, and it follows the reader's rules.
func TestAGroupReferenceExpandsOnTheServer(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `[{"group":"zebra"}]`})
	requireRows(t, res, "note", "f3", "f4", nil)
	res = z.run(t, "ann", animalsParam, Options{}, map[string]string{"animals": `["` + z.id("leo") + `",{"group":"zebra"}]`})
	requireRows(t, res, "note", "f1", "f2", "f3", "f4", nil)
	if p := res.Params[0]; len(p.Groups) != 1 || p.Groups[0] != "zebra" || len(p.List) != 1 {
		t.Fatalf("resolved = %+v", p)
	}
	_, err := z.try(t, "ann", animalsParam, Options{}, map[string]string{"animals": `[{"group":7}]`})
	requireSpecError(t, err, 400, "params.animals", "a group is")
}
```

and in `TestARunTakesAnyIDForAnOptionsFromSourceParam` the over-cap message becomes `"pick a whole group instead"`.

- [ ] **Step 2: Run it to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'GroupReference|AnyIDForAnOptionsFromSource' ./reports/`
Expected: FAIL: `the value is a list of the param's options`.

- [ ] **Step 3: Implement**

`params.go`: `paramValue` gains `groups []string`; `ResolvedParam` gains `Groups []string \`json:"groups,omitempty"\`` (filled with `v.groups`). In `choiceValue`, for a multiple choice with `OptionsFrom`, each item is a string (a value) or an object `{"group": string}`:

```go
	for _, item := range items {
		if m, ok := item.(map[string]any); ok && d.OptionsFrom != nil {
			g, ok := m["group"].(string)
			if !ok || len(m) != 1 || len(g) > maxParamText {
				return out, invalid(at, `a group is {"group": "<group value>"}`)
			}
			out.groups = append(out.groups, g)
			continue
		}
		s, ok := item.(string)
		if !ok || !valid(s) {
			return out, invalid(at, "the value is a list of the param's options")
		}
		out.list = append(out.list, s)
	}
	if len(out.list) > maxChoiceValues {
		return out, invalid(at, fmt.Sprintf("the value is a list of at most %d choices; pick a whole group instead", maxChoiceValues))
	}
	if len(out.groups) > maxOptionGroups {
		return out, invalid(at, fmt.Sprintf("the value holds at most %d groups", maxOptionGroups))
	}
	out.unset = len(out.list) == 0 && len(out.groups) == 0
```

(the `len(items) > maxChoiceValues` check from Task 12 moves to `out.list` here). `options.go`:

```go
// groupListSQL is a choice list with group references expanded under the
// reader's rules: the values of the options select whose group is one of
// groups, plus the explicit values.
func (c *compiler) groupListSQL(s *stmt, op *optionsPlan, values, groups []string) (string, error) {
	inner, err := c.optionsSelect(s, op)
	if err != nil {
		return "", err
	}
	marks := make([]string, len(groups))
	for i, g := range groups {
		marks[i] = s.bind(g)
	}
	sql := fmt.Sprintf("SELECT o.v FROM (%s) o WHERE o.g IN (%s)", inner, strings.Join(marks, ", "))
	for _, v := range values {
		sql += " UNION SELECT " + s.bind(v)
	}
	return "(" + sql + ")", nil
}

// paramLeaf is a param as a formula leaf in a context with a statement: a
// choice with group references reads the expanded list.
func paramLeaf(c *compiler, s *stmt, f fctx, id string, v paramValue) (expr, error) {
	if len(v.groups) == 0 {
		return paramExpr(f, v), nil
	}
	op := c.p.options[id]
	if op == nil {
		return paramExpr(f, v), nil
	}
	sql, err := c.groupListSQL(s, op, v.list, v.groups)
	if err != nil {
		return expr{}, err
	}
	return expr{sql: sql, typ: Text, list: true}, nil
}
```

Each context's `param` calls `paramLeaf` with its compiler and statement (`rowsCtx`: `rc.r.c, rc.r.s`; `joinCtx`: `j.r.c, j.sc.s`; `levelCtx`: `lv.c, lv.r.s`; `lookupCtx`: `l.r.c, l.r.s`). `x IN (SELECT …)` is the `in` operator's existing `(arg IN list)` form.

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): a whole group picked from a source expands on the server"
git push origin books/p5-trial-balance
```

---

### Task 14: E9 `Source.Heading` and `Source.Watch`; E10 `Shipped.Unpublished`

Engine facts used: `Source` (`registry.go:102-120`), `validateSource` (`registry.go:165-215`), `checkSource` (`check.go:69-95`); `Run` builds the result (`run.go:171-205`) and collects topics in `run()` (`run.go:267-276`); `ReconcileShipped` creates a missing report published (`shipped.go:104-122`).

**Files:**
- Modify: `core/server/reports/registry.go`, `check.go`, `run.go` (`Result.Heading`, watched topics), `shipped.go`
- Create: `core/server/reports/heading_test.go`
- Modify: `core/server/reports/shipped_test.go`

**Interfaces:**
- Produces: `Source.Heading func(app core.App) string`; `Source.Watch []string`; `Result.Heading string` (`json:"heading,omitempty"`); `Shipped.Unpublished bool`.

- [ ] **Step 1: Write the failing tests**

Create `core/server/reports/heading_test.go`:

```go
package reports

import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func withHeading(t *testing.T, heading string, watch ...string) {
	t.Helper()
	src := *lookupRegistered("zoo.feedings")
	src.Heading = func(core.App) string { return heading }
	src.Watch = watch
	RegisterSource(src)
}

func TestARunPrintsItsSourcesHeadingAndWatchesWhatItReads(t *testing.T) {
	z := newZoo(t)
	withHeading(t, "  Zoo Ltd  ", "zoo_sites")
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	if res.Heading != "Zoo Ltd" {
		t.Fatalf("heading = %q", res.Heading)
	}
	if !slices.Contains(res.Topics, "collection/zoo_sites") {
		t.Fatalf("topics = %v", res.Topics)
	}
	other := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[{"id":"n","path":[{"field":"name"}]}]}`, Options{}, nil)
	if other.Heading != "" || slices.Contains(other.Topics, "collection/zoo_sites") {
		t.Fatalf("animals: heading %q, topics %v", other.Heading, other.Topics)
	}
}

func TestCheckSourcesRefusesAWatchedCollectionItCannotSignal(t *testing.T) {
	z := newZoo(t)
	withHeading(t, "", "zoo_nothing")
	if err := CheckSources(z.app); err == nil || !strings.Contains(err.Error(), `watched collection "zoo_nothing" does not exist`) {
		t.Fatalf("err = %v", err)
	}
	withHeading(t, "", "_superusers")
	if err := CheckSources(z.app); err == nil || !strings.Contains(err.Error(), "is a system collection") {
		t.Fatalf("err = %v", err)
	}
}
```

Append to `shipped_test.go`:

```go
// An unpublished shipped report is created unpublished and stays so on a
// new revision; its package shares it, and it still cannot be deleted.
func TestAnUnpublishedShippedReportIsNeverPublished(t *testing.T) {
	s := newSavedZoo(t)
	r1 := feedCosts(1, "Feed costs")
	r1.Unpublished = true
	RegisterShipped(r1)
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row := shippedRow(t, s, "zoo.feed_costs")
	if row.GetBool("published") || row.GetString("published_by") != "" || row.GetString("owner") != s.id("ad") {
		t.Fatalf("created: %v", row.PublicExport())
	}
	r2 := feedCosts(2, "Feed costs 2")
	r2.Unpublished = true
	RegisterShipped(r2)
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row = shippedRow(t, s, "zoo.feed_costs")
	if row.GetBool("published") || row.GetString("title") != "Feed costs 2" {
		t.Fatalf("revised: %v", row.PublicExport())
	}
	s.expect(t, "ad", http.MethodDelete, reportURL(row.Id), "", http.StatusBadRequest, `"shipped_key":{"code":"shipped"`)
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'Heading|WatchedCollection|UnpublishedShipped' ./reports/`
Expected: FAIL to compile: `src.Heading undefined`.

- [ ] **Step 3: Implement**

`registry.go` `Source`, after `FiscalStartMonth`:

```go
	// Heading is the name a report on this base source prints above its
	// title (a business name), read at each run. nil prints the
	// organization's name.
	Heading func(app core.App) string
	// Watch names the collections Heading, FiscalStartMonth and Currency
	// read. A run that reads this source subscribes to them too, so a
	// settings change runs an open report again.
	Watch []string
```

`validateSource`, before the fields loop:

```go
	for _, w := range src.Watch {
		if w == "" {
			return fmt.Errorf("a watched collection needs a name")
		}
	}
```

`checkSource`, before `checkRule`:

```go
	for _, w := range src.Watch {
		wc, err := app.FindCollectionByNameOrId(w)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("watched collection %q does not exist", w))
		case wc.Name != w:
			problems = append(problems, fmt.Sprintf("watched collection %q must be named %q, not by id", w, wc.Name))
		case wc.System:
			problems = append(problems, fmt.Sprintf("watched collection %q is a system collection", w))
		}
	}
```

`run.go`: add to `Result` after `Topics`:

```go
	// Heading is what the report prints above its title: the base source's
	// Heading, or "" for the organization's name.
	Heading string `json:"heading,omitempty"`
```

In `Run`, after `rn.res.Params = …`: `if h := p.base.Heading; h != nil { rn.res.Heading = strings.TrimSpace(h(app)) }`. In `run()`, before the topics are copied out:

```go
	// A source's Go hooks read collections no statement reads (R14).
	for _, src := range Sources(rn.app) {
		if rn.topics[changesignal.Topic(src.Collection)] {
			for _, w := range src.Watch {
				rn.topics[changesignal.Topic(w)] = true
			}
		}
	}
```

`shipped.go` `Shipped`, after `Revision`:

```go
	// Unpublished ships the report unpublished: core never publishes it, and
	// the package shares it with the people who should see it.
	Unpublished bool
```

In `ReconcileShipped`, replace the two `published` lines of a new record:

```go
			rec.Set("published", !s.Unpublished)
			if !s.Unpublished {
				rec.Set("published_by", admin.Id)
			}
```

and update the `Shipped` doc ("a published report every user sees, or with Unpublished a report its package shares").

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): report source headings and watched collections; unpublished shipped reports"
git push origin books/p5-trial-balance
```

---

### Task 15: E14 `display` validation; E12 aliases in every v2 part, round-trip, and rule parity

Engine facts used: `resolveAliases`, `aliasPath`, `aliasNode` (`spec.go:232-304`); `loadStored` stores `json.Marshal(spec)` (`saved.go:120-131`); the fuzz test (`fuzz_test.go:183-202`).

**Files:**
- Modify: `core/server/reports/spec.go` (`Display`, `DisplayLayout`, `DisplayCell`, `Spec.Display`; the `aliaser`)
- Modify: `core/server/reports/bind.go` (`binder.display`, `placeCells`)
- Modify: `core/server/reports/v2_test.go`, `fuzz_test.go`

**Interfaces:**
- Produces: `Spec.Display *Display` (`json:"display,omitempty"`); `Display{Layouts []DisplayLayout}`; `DisplayLayout{Width string; Tracks int; Cells []DisplayCell}`; `DisplayCell{Col string; Line, Start, Span, Lines int; Justify string}` (json `col,line,start,span,lines,justify`, the last four `omitempty`); `placeCells(tracks int, cells []DisplayCell) ([]placed, error)` with `type placed struct{ start, span, line, lines int }` — the same placement rule `lib/table-lines.ts` implements in Task 24.

- [ ] **Step 1: Write the failing tests**

Append to `v2_test.go`:

```go
// every is a v2 spec that uses every new part (the zoo's "For review" spec).
const every = `{"version":2,"source":"zoo.feedings",
	"params":[{"id":"asOf","type":"asOf","label":"As of","default":{"preset":"endOfLastMonth"}},
		{"id":"rows","type":"choice","label":"Rows","default":"nonzero","options":[{"value":"nonzero","label":"Non-zero"},{"value":"all","label":"All"}]},
		{"id":"animals","type":"choice","label":"Animals","multiple":true,"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"name"}],"group":[{"field":"species"}],
			"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}}}],
	"columns":[{"id":"animal","label":"Animal","path":[{"link":"animal"}]},
		{"id":"weight","label":"Weight","formula":{"record":"byAnimal","path":[{"field":"weight"}]}},
		{"id":"cost","label":"Cost","path":[{"field":"cost"}],"aggregate":"sum","total":"rows"},
		{"id":"site","label":"Site","formula":{"op":"lookup","source":"zoo.sites","where":{"op":"eq","args":[{"path":[{"field":"region"}]},{"value":"north"}]},"value":{"path":[{"field":"name"}]},"pick":"min"}}],
	"filter":{"op":"and","args":[{"op":"lte","args":[{"path":[{"field":"due"}]},{"param":"asOf"}]},{"op":"in","args":[{"col":"animal"},{"param":"animals"}]}]},
	"groups":[{"id":"byAnimal","col":"animal","label":{"record":"byAnimal","path":[{"field":"name"}]},
		"fill":{"where":{"op":"eq","args":[{"path":[{"field":"active"}]},{"value":true}]}},
		"having":{"op":"or","args":[{"op":"eq","args":[{"param":"rows"},{"value":"all"}]},{"op":"ne","args":[{"op":"sum","args":[{"path":[{"field":"cost"}]}]},{"value":0}]}]},
		"open":true}],
	"sort":[{"formula":{"record":"byAnimal","path":[{"field":"weight"}]},"dir":"desc"},{"col":"cost"}],
	"detail":{"label":{"op":"concat","args":[{"path":[{"field":"fed"}]},{"value":" · "},{"path":[{"field":"note"}]}]}},
	"display":{"layouts":[{"width":"mobile","tracks":3,"cells":[{"col":"animal","line":1,"span":3},{"col":"weight","line":2},{"col":"cost","line":2,"justify":"end"},{"col":"site","line":2,"justify":"end"}]}]}}`

// A spec with every v2 part loads, validates, runs, and is stored with
// every part unchanged.
func TestASpecWithEveryV2PartIsKeptWhole(t *testing.T) {
	z := newZoo(t)
	spec, _, stored := mustLoad(t, every)
	if err := Validate(z.app, spec); err != nil {
		t.Fatal(err)
	}
	z.run(t, "ann", every, Options{Today: "2026-04-15"}, nil)
	_, _, again := mustLoad(t, string(stored))
	if !bytes.Equal(stored, again) {
		t.Fatalf("stored twice differs:\n%s\n%s", stored, again)
	}
	for _, key := range []string{`"optionsFrom"`, `"record":"byAnimal"`, `"total":"rows"`, `"op":"lookup"`, `"pick":"min"`, `"label":{`, `"fill":{`, `"having":{`, `"open":true`, `"dir":"desc"`, `"detail":{`, `"display":{`} {
		if !strings.Contains(string(stored), key) {
			t.Fatalf("stored spec lost %s: %s", key, stored)
		}
	}
}

func mustLoad(t *testing.T, raw string) (*Spec, *LoadInfo, []byte) {
	t.Helper()
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return spec, info, stored
}

// zoo.animals renames weight from mass: every v2 part that names a field
// is resolved.
func TestAliasesResolveInEveryV2Part(t *testing.T) {
	withZooRegistry(t)
	raw := strings.NewReplacer(`"field":"weight"`, `"field":"mass"`).Replace(every)
	spec, info, _ := mustLoad(t, raw)
	if spec.Columns[1].Formula.Path[0].Field != "weight" || spec.Sort[0].Formula.Path[0].Field != "weight" {
		t.Fatalf("record paths not resolved: %+v %+v", spec.Columns[1].Formula, spec.Sort[0].Formula)
	}
	if len(info.Aliased) != 2 {
		t.Fatalf("aliased = %+v", info.Aliased)
	}
	// A lookup's where resolves against the looked-up source.
	looked := strings.NewReplacer(
		`"source":"zoo.sites"`, `"source":"zoo.animals"`,
		`{"path":[{"field":"region"}]},{"value":"north"}`, `{"path":[{"field":"mass"}]},{"value":190}`,
	).Replace(every)
	spec, _, _ = mustLoad(t, looked)
	if got := spec.Columns[3].Formula.Where.Args[0].Path[0].Field; got != "weight" {
		t.Fatalf("lookup where = %q, want weight", got)
	}
}

func TestDisplayLayoutsAreChecked(t *testing.T) {
	app := withZooRegistry(t)
	base := `{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"note"}]},{"id":"b","path":[{"field":"cost"}]}],"display":{"layouts":[%s]}}`
	for _, c := range []struct {
		layout, at, want string
		status           int
	}{
		{`{"width":"watch","tracks":2,"cells":[]}`, "display.layouts.0.width", "mobile, tablet or desktop", 400},
		{`{"width":"mobile","tracks":13,"cells":[]}`, "display.layouts.0.tracks", "1 to 12", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"zz","line":1}]}`, "display.layouts.0.cells.0.col", "has no column", 422},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":9}]}`, "display.layouts.0.cells.0", "line is 1 to 8", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"span":2},{"col":"b","line":1}]}`, "display.layouts.0.cells.1", "no room on line 1", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"lines":2},{"col":"b","line":2,"start":1}]}`, "display.layouts.0.cells.1", "overlaps", 400},
		{`{"width":"mobile","tracks":2,"cells":[{"col":"a","line":1,"justify":"left"}]}`, "display.layouts.0.cells.0.justify", "start, center or end", 400},
		{`{"width":"mobile","tracks":2,"cells":[]},{"width":"mobile","tracks":1,"cells":[]}`, "display.layouts.1.width", "one layout per width", 400},
	} {
		spec, _, err := Load([]byte(fmt.Sprintf(base, c.layout)))
		if err != nil {
			t.Fatal(err)
		}
		requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
	}
}

// A pinwheel over five real columns is a 400 (not an unknown column).
func TestDisplayRefusesAPinwheel(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[
		{"id":"a","path":[{"field":"note"}]},{"id":"b","path":[{"field":"cost"}]},{"id":"c","path":[{"field":"grams"}]},
		{"id":"d","path":[{"field":"fed"}]},{"id":"e","path":[{"field":"currency"}]}],
		"display":{"layouts":[{"width":"mobile","tracks":3,"cells":[{"col":"a","line":1,"start":1,"span":2},{"col":"b","line":1,"start":3,"lines":2},
			{"col":"c","line":3,"start":2,"span":2},{"col":"d","line":2,"start":1,"lines":2},{"col":"e","line":2,"start":2}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "display.layouts.0.cells.0", "can't be laid out")
}

func TestPlaceCellsFlowsIntoFreeTracks(t *testing.T) {
	got, err := placeCells(3, []DisplayCell{{Col: "a", Line: 1, Span: 3}, {Col: "b", Line: 2}, {Col: "c", Line: 2}, {Col: "d", Line: 3}})
	if err != nil {
		t.Fatal(err)
	}
	want := []placed{{1, 3, 1, 1}, {1, 1, 2, 1}, {2, 1, 2, 1}, {1, 1, 3, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("placed = %+v", got)
	}
}

// A cell may span lines anywhere a straight cut frees it, mid-row included;
// only a pinwheel is refused (R28).
func TestPlaceCellsCutsAnyLayoutButAPinwheel(t *testing.T) {
	mid := []DisplayCell{{Col: "a", Line: 1}, {Col: "m", Line: 1, Start: 2, Lines: 2}, {Col: "b", Line: 1, Start: 3}, {Col: "c", Line: 2, Start: 1}, {Col: "d", Line: 2, Start: 3}}
	if _, err := placeCells(3, mid); err != nil {
		t.Fatal(err)
	}
	pinwheel := []DisplayCell{
		{Col: "a", Line: 1, Start: 1, Span: 2},
		{Col: "b", Line: 1, Start: 3, Lines: 2},
		{Col: "c", Line: 3, Start: 2, Span: 2},
		{Col: "d", Line: 2, Start: 1, Lines: 2},
		{Col: "e", Line: 2, Start: 2},
	}
	_, err := placeCells(3, pinwheel)
	requireSpecError(t, err, 400, "cells.0", "These cells overlap in a way that can't be laid out; move one cell.")
}
```

(Imports: `bytes`, `fmt`, `reflect`, `strings`.) The alias test's second half points the lookup at `zoo.animals` and its where at `mass`, and checks the where's path is resolved.

Extend `fuzz_test.go`: in `f.Fuzz`, after the parity check, call `z.lookupParity(t, mux, user)` and `z.fillParity(t, mux, user)`:

```go
// lookupParity: a lookup reads exactly what the list API lists for the reader.
func (z *zoo) lookupParity(t *testing.T, mux http.Handler, user string) {
	t.Helper()
	res := z.run(t, user, `{"version":2,"source":"zoo.feedings","columns":[{"id":"id","path":[{"field":"id"}]},
		{"id":"lk","formula":{"op":"lookup","source":"zoo.sites","where":{"op":"eq","args":[{"path":[{"field":"name"}]},{"value":"North"}]},"value":{"path":[{"field":"id"}]}}}]}`,
		Options{Limit: 1000}, nil)
	var want any
	if ids := z.listIDs(t, mux, "zoo_sites", user, `name="North"`); len(ids) > 0 {
		want = ids[0]
	}
	for _, row := range res.Rows {
		if row.Kind == KindDetail && row.Cells["lk"] != want {
			t.Fatalf("%s: lookup = %v, list API = %v", user, row.Cells["lk"], want)
		}
	}
}

// fillParity: a fill adds exactly the animals the list API lists for the reader.
func (z *zoo) fillParity(t *testing.T, mux http.Handler, user string) {
	t.Helper()
	res := z.run(t, user, `{"version":2,"source":"zoo.feedings","columns":[{"id":"a","path":[{"link":"animal"}]},
		{"id":"n","path":[{"field":"id"}],"aggregate":"count"}],"groups":[{"id":"g","col":"a","fill":{}}]}`, Options{}, nil)
	got := map[string]bool{}
	for _, row := range res.Rows {
		if row.Kind == KindGroup && row.GroupKey[0] != nil {
			got[row.GroupKey[0].(string)] = true
		}
	}
	want := z.listIDs(t, mux, "zoo_animals", user, "")
	if len(got) != len(want) {
		t.Fatalf("%s: fill keys %v, list API %v", user, got, want)
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("%s: fill left out %s", user, id)
		}
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 -run 'EveryV2|AliasesResolve|DisplayLayouts|PlaceCells|FuzzRuleParity' ./reports/`
Expected: FAIL to load: `json: unknown field "display"`.

- [ ] **Step 3: The display types and their check**

`spec.go`: add `Display *Display \`json:"display,omitempty"\`` to `Spec`, and:

```go
// Display is how the report table lays its columns out per width (E14).
// The engine checks it and otherwise leaves it to the client.
type Display struct {
	Layouts []DisplayLayout `json:"layouts,omitempty"`
}

// DisplayLayout places columns on lines of Tracks equal tracks at one width
// (mobile, tablet or desktop). A column it does not list is not shown there.
type DisplayLayout struct {
	Width  string        `json:"width"`
	Tracks int           `json:"tracks"`
	Cells  []DisplayCell `json:"cells"`
}

// DisplayCell puts a column on Line (from 1), at Start (0 places it in the
// first free tracks), Span tracks wide (1) and Lines lines tall (1).
type DisplayCell struct {
	Col     string `json:"col"`
	Line    int    `json:"line"`
	Start   int    `json:"start,omitempty"`
	Span    int    `json:"span,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Justify string `json:"justify,omitempty"`
}
```

`bind.go`: call `b.display(p)` at the end of `bind` (before `b.err()`), and add:

```go
const (
	maxDisplayTracks = 12
	maxDisplayLines  = 8
)

var displayWidths = []string{"mobile", "tablet", "desktop"}

func (b *binder) display(p *plan) {
	d := p.spec.Display
	if d == nil {
		return
	}
	seen := map[string]bool{}
	for i, l := range d.Layouts {
		at := fmt.Sprintf("display.layouts.%d", i)
		switch {
		case !slices.Contains(displayWidths, l.Width):
			b.bad(at+".width", "width is mobile, tablet or desktop")
		case seen[l.Width]:
			b.bad(at+".width", "one layout per width")
		}
		seen[l.Width] = true
		if l.Tracks < 1 || l.Tracks > maxDisplayTracks {
			b.bad(at+".tracks", fmt.Sprintf("tracks is 1 to %d", maxDisplayTracks))
			continue
		}
		cols := map[string]bool{}
		for j, cell := range l.Cells {
			cat := fmt.Sprintf("%s.cells.%d", at, j)
			if b.column(p, cell.Col, cat+".col") != nil && cols[cell.Col] {
				b.bad(cat+".col", "a column is placed once per layout")
			}
			cols[cell.Col] = true
			if cell.Justify != "" && cell.Justify != "start" && cell.Justify != "center" && cell.Justify != "end" {
				b.bad(cat+".justify", "justify is start, center or end")
			}
		}
		if _, err := placeCells(l.Tracks, l.Cells); err != nil {
			b.merge(prefixProblem(at, err))
		}
	}
}

// placed is where a display cell sits: its first track, tracks, line and
// lines, all from 1.
type placed struct{ start, span, line, lines int }

// placeCells lays cells out in order: a cell with a start sits there; one
// without takes the first tracks of its line that are free on every line it
// spans; then the cells must cut down by straight lines (no pinwheel, R28).
// lib/table-lines.ts places and cuts cells by the same rules.
func placeCells(tracks int, cells []DisplayCell) ([]placed, error) {
	taken := map[[2]int]bool{}
	out := make([]placed, 0, len(cells))
	for j, c := range cells {
		at := fmt.Sprintf("cells.%d", j)
		span, lines := max(c.Span, 1), max(c.Lines, 1)
		switch {
		case c.Line < 1 || c.Line > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("line is 1 to %d", maxDisplayLines))
		case c.Line+lines-1 > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("a cell ends by line %d", maxDisplayLines))
		case span > tracks:
			return nil, invalid(at, fmt.Sprintf("span is 1 to %d", tracks))
		case c.Start < 0 || c.Start > tracks || (c.Start > 0 && c.Start+span-1 > tracks):
			return nil, invalid(at, "the cell does not fit its line")
		}
		free := func(start int) bool {
			for line := c.Line; line < c.Line+lines; line++ {
				for t := start; t < start+span; t++ {
					if taken[[2]int{line, t}] {
						return false
					}
				}
			}
			return true
		}
		start := c.Start
		if start == 0 {
			for s := 1; s+span-1 <= tracks && start == 0; s++ {
				if free(s) {
					start = s
				}
			}
			if start == 0 {
				return nil, invalid(at, fmt.Sprintf("no room on line %d", c.Line))
			}
		} else if !free(start) {
			return nil, invalid(at, "the cell overlaps another")
		}
		for line := c.Line; line < c.Line+lines; line++ {
			for t := start; t < start+span; t++ {
				taken[[2]int{line, t}] = true
			}
		}
		out = append(out, placed{start, span, c.Line, lines})
	}
	// The client draws an arrangement as blocks cut by straight lines (R28): a
	// pinwheel no straight cut can split cannot be drawn.
	if j := uncuttable(out, tracks); j >= 0 {
		return nil, invalid(fmt.Sprintf("cells.%d", j), pinwheelMessage)
	}
	return out, nil
}

const pinwheelMessage = "These cells overlap in a way that can't be laid out; move one cell."

// piece is a placed cell as a box: tracks [x0, x1) and lines [y0, y1), from 0.
type piece struct{ index, x0, x1, y0, y1 int }

// uncuttable is the index of the first cell of a part no straight cut between
// lines or tracks can split into two non-empty sides, or -1 when the cells cut
// down to one each (guillotine cuts; lib/table-lines.ts cuts the same way).
func uncuttable(ps []placed, tracks int) int {
	pieces := make([]piece, len(ps))
	lines := 0
	for i, p := range ps {
		pieces[i] = piece{i, p.start - 1, p.start - 1 + p.span, p.line - 1, p.line - 1 + p.lines}
		lines = max(lines, p.line-1+p.lines)
	}
	return cutDown(pieces, 0, tracks, 0, lines)
}

func cutDown(pieces []piece, x0, x1, y0, y1 int) int {
	if len(pieces) <= 1 {
		return -1
	}
	split := func(at int, across bool) ([]piece, []piece, bool) {
		var lo, hi []piece
		for _, p := range pieces {
			a0, a1 := p.x0, p.x1
			if across {
				a0, a1 = p.y0, p.y1
			}
			switch {
			case a1 <= at:
				lo = append(lo, p)
			case a0 >= at:
				hi = append(hi, p)
			default:
				return nil, nil, false
			}
		}
		return lo, hi, len(lo) > 0 && len(hi) > 0
	}
	for y := y0 + 1; y < y1; y++ {
		if lo, hi, ok := split(y, true); ok {
			if j := cutDown(lo, x0, x1, y0, y); j >= 0 {
				return j
			}
			return cutDown(hi, x0, x1, y, y1)
		}
	}
	for x := x0 + 1; x < x1; x++ {
		if lo, hi, ok := split(x, false); ok {
			if j := cutDown(lo, x0, x, y0, y1); j >= 0 {
				return j
			}
			return cutDown(hi, x, x1, y0, y1)
		}
	}
	first := pieces[0].index
	for _, p := range pieces {
		first = min(first, p.index)
	}
	return first
}

// prefixProblem moves an error's problem keys under at.
func prefixProblem(at string, err error) *SpecError {
	var se *SpecError
	if !errors.As(err, &se) {
		return invalid(at, err.Error())
	}
	out := &SpecError{Status: se.Status, Problems: map[string]validation.Error{}}
	for k, v := range se.Problems {
		out.Problems[at+"."+k] = v
	}
	return out
}
```

(Add `"errors"` to `bind.go`'s imports.)

- [ ] **Step 4: The aliaser**

Replace `resolveAliases`, `aliasPath` and `aliasNode` in `spec.go` with an `aliaser` that knows each group's target, so record paths, fill wheres, lookups and option sources resolve too:

```go
// aliaser resolves aliased keys in place and records each one. targets maps
// a group id to the source its key links to, for record leaves and fill.
type aliaser struct {
	targets map[string]*Source
	out     []Alias
}

func resolveAliases(spec *Spec) []Alias {
	base := lookupRegistered(spec.Source)
	if base == nil {
		return nil
	}
	a := &aliaser{targets: map[string]*Source{}}
	cols := map[string]*Column{}
	for i := range spec.Columns {
		cols[spec.Columns[i].ID] = &spec.Columns[i]
		a.path(base, spec.Columns[i].Path)
	}
	for _, g := range spec.Groups {
		if c := cols[g.Col]; c != nil {
			a.targets[g.ID] = a.targetOf(base, c, cols, map[string]bool{})
		}
	}
	for i := range spec.Columns {
		a.node(base, spec.Columns[i].Formula)
	}
	a.node(base, spec.Filter)
	for i := range spec.Groups {
		g := &spec.Groups[i]
		a.node(base, g.Having)
		a.node(base, g.Label)
		if g.Fill != nil {
			a.node(a.targets[g.ID], g.Fill.Where)
		}
	}
	for i := range spec.Sort {
		a.node(base, spec.Sort[i].Formula)
	}
	if spec.Detail != nil {
		a.node(base, spec.Detail.Label)
	}
	if spec.Pivot != nil {
		a.path(base, spec.Pivot.Path)
	}
	for i := range spec.Params {
		if of := spec.Params[i].OptionsFrom; of != nil {
			src := lookupRegistered(of.Source)
			a.path(src, of.Value)
			a.path(src, of.Label)
			a.path(src, of.Group)
			a.node(src, of.Where)
		}
	}
	return a.out
}

// path resolves a path's keys and returns the source it ends on, or nil when
// a hop cannot be followed.
func (a *aliaser) path(src *Source, path Path) *Source {
	for i := range path {
		if src == nil {
			return nil
		}
		hop := &path[i]
		if hop.Link != "" {
			hop.Link = a.key(src, hop.Link)
			l := src.link(hop.Link)
			if l == nil {
				return nil
			}
			src = lookupRegistered(l.Target)
		}
		if hop.Field != "" && src != nil {
			hop.Field = a.key(src, hop.Field)
		}
	}
	return src
}

func (a *aliaser) key(src *Source, key string) string {
	if src.field(key) != nil || src.link(key) != nil {
		return key
	}
	to, ok := src.Aliases[key]
	if !ok {
		return key
	}
	a.out = append(a.out, Alias{Source: src.Key, From: key, To: to})
	return to
}

func (a *aliaser) node(src *Source, n *Node) {
	if n == nil || src == nil {
		return
	}
	switch {
	case n.Record != "":
		a.path(a.targets[n.Record], n.Path)
	case n.Op == "lookup":
		a.lookup(n)
		return
	default:
		a.path(src, n.Path)
	}
	for _, arg := range n.Args {
		a.node(src, arg)
	}
	if n.Link != nil {
		// A where clause reads the source the link path ends on.
		a.node(a.path(src, n.Link), n.Where)
	}
}

// lookup resolves a lookup's value and where against its own source; the
// value is re-encoded, since it is kept as raw JSON.
func (a *aliaser) lookup(n *Node) {
	src := lookupRegistered(n.Source)
	if v, err := lookupValue(n, ""); err == nil {
		a.node(src, v)
		if raw, err := json.Marshal(v); err == nil {
			n.Value = raw
		}
	}
	a.node(src, n.Where)
}

// targetOf is the source a group column's value links to: a path that ends
// at a to-one link or at id, a lookup of a relation, or the results of if,
// case and coalesce; nil when it links nowhere.
func (a *aliaser) targetOf(base *Source, c *Column, cols map[string]*Column, seen map[string]bool) *Source {
	if seen[c.ID] {
		return nil
	}
	seen[c.ID] = true
	if c.Path != nil {
		return pathTarget(base, c.Path)
	}
	return a.nodeTarget(base, c.Formula, cols, seen)
}

func (a *aliaser) nodeTarget(src *Source, n *Node, cols map[string]*Column, seen map[string]bool) *Source {
	switch {
	case n == nil:
		return nil
	case n.Path != nil && n.Record == "":
		return pathTarget(src, n.Path)
	case n.Col != "":
		if c := cols[n.Col]; c != nil {
			return a.targetOf(src, c, cols, seen)
		}
	case n.Op == "lookup":
		if v, err := lookupValue(n, ""); err == nil {
			return a.nodeTarget(lookupRegistered(n.Source), v, cols, seen)
		}
	case n.Op == "if" || n.Op == "case" || n.Op == "coalesce":
		for _, arg := range n.Args {
			if t := a.nodeTarget(src, arg, cols, seen); t != nil {
				return t
			}
		}
	}
	return nil
}

// pathTarget is the source a relation path points at (Task 3's rule), with
// keys as they stand.
func pathTarget(src *Source, path Path) *Source {
	for _, hop := range path {
		if src == nil {
			return nil
		}
		if hop.Link != "" {
			l := src.link(hop.Link)
			if l == nil {
				return nil
			}
			src = lookupRegistered(l.Target)
			continue
		}
		if hop.Field == idField.Key {
			return src
		}
		return nil
	}
	return src
}
```

Note: `case "if"`'s first argument is a condition, which has no target, so the loop finds the first result that has one. The existing alias tests (`spec_test.go:147-210`) keep passing: the aliaser walks every part the old functions walked.

- [ ] **Step 5: Run the whole reports package, fuzz seeds included**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go test -tags no_ui -count=1 ./reports/`
Expected: PASS (`FuzzRuleParity` runs its 40 seeds as unit tests, now with the lookup and fill checks).

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/server/reports
git commit -m "feat(core): report display layouts; aliases in every v2 part; lookup and fill rule parity"
git push origin books/p5-trial-balance
```

---

### Task 16: Client lib — the v2 spec schema, result types, expansion, param options

Code facts used: the strict spec schema (`core/reports/lib/spec.ts:13-111`), result types (`lib/types.ts:33-98`), the expansion helpers and child cells (`lib/rows.ts:17-111`), the run client (`lib/run-client.ts:74-99`), choice values and chip text (`lib/params.ts:86-93,209-247`), the store's expansion and the view's default (`stores/report-ui-store.ts:14-38`, `hooks/useReportView.ts:115`).

**Files:**
- Modify: `core/reports/lib/spec.ts`, `lib/types.ts`, `lib/rows.ts`, `lib/run-client.ts`, `lib/params.ts`, `lib/view-url.ts` (a choice list with groups), `lib/format.ts` (a link cell shows its label), `hooks/useReportView.ts:115,217`
- Test: `core/reports/lib/__tests__/spec.test.ts`, `rows.test.ts`, `run-client.test.ts`, `params.test.ts`, `view-url.test.ts`, `format.test.ts`

**Interfaces:**
- Produces (TS): `SpecNode.record?`, `SpecNode.source?`, `SpecNode.pick?`; `ParamDef.optionsFrom?: OptionsFrom`; `SpecColumn.total?: 'recompute' | 'rows'`; `SpecSortKey = { col?: string; formula?: SpecNode; dir? }`; `SpecGroup` with `label?`, `having?`, `fill?: { where? }`, `open?`; `ReportSpec.detail?`; `ReportSpec.display?: { layouts?: DisplayLayout[] }` with `DisplayLayout = { width: 'mobile' | 'tablet' | 'desktop'; tracks: number; cells: DisplayCell[] }`, `DisplayCell = { col: string; line: number; start?: number; span?: number; lines?: number; justify?: 'start' | 'center' | 'end' }`; `RunResult.heading?`; `ResultRow.labelColumn?`; `ParamOption`, `ParamOptionGroup`, `ParamOptionsPage`; `DEFAULT_EXPANSION`; `visibleRows(rows)` (no columns argument); `PARAM_OPTIONS_PATH`, `ParamOptionsBody`, `fetchParamOptions(body, signal?)`; `MAX_CHOICE_VALUES = 1000`; `chipText(def, value, resolved, labels?)`.

- [ ] **Step 1: Write the failing tests**

`spec.test.ts`, add:

```ts
    it('reads every v2 part', () => {
        const spec = {
            version: 2,
            source: 'zoo.feedings',
            params: [
                {
                    id: 'animals',
                    type: 'choice',
                    multiple: true,
                    optionsFrom: {
                        source: 'zoo.animals',
                        value: [{ field: 'id' }],
                        label: [{ field: 'name' }],
                        group: [{ field: 'species' }],
                        where: { op: 'eq', args: [{ path: [{ field: 'active' }] }, { value: true }] },
                    },
                },
            ],
            columns: [
                { id: 'animal', path: [{ link: 'animal' }] },
                { id: 'weight', formula: { record: 'g', path: [{ field: 'weight' }] } },
                { id: 'cost', path: [{ field: 'cost' }], aggregate: 'sum', total: 'rows' },
                {
                    id: 'site',
                    formula: { op: 'lookup', source: 'zoo.sites', value: { path: [{ field: 'name' }] }, pick: 'max' },
                },
            ],
            groups: [
                {
                    id: 'g',
                    col: 'animal',
                    label: { record: 'g', path: [{ field: 'name' }] },
                    having: { op: 'gt', args: [{ col: 'cost' }, { value: 0 }] },
                    fill: { where: { op: 'eq', args: [{ path: [{ field: 'active' }] }, { value: true }] } },
                    open: true,
                },
            ],
            sort: [{ formula: { record: 'g', path: [{ field: 'weight' }] }, dir: 'desc' }, { col: 'cost' }],
            detail: { label: { path: [{ field: 'note' }] } },
            display: {
                layouts: [
                    { width: 'mobile', tracks: 3, cells: [{ col: 'animal', line: 1, span: 3 }, { col: 'cost', line: 2, justify: 'end' }] },
                ],
            },
        }
        expect(parseSpec(spec)).toEqual(spec)
        expect(parseSpec({ ...spec, columns: [{ id: 'x', path: [], total: 'some' }] })).toBeNull()
        expect(parseSpec({ ...spec, display: { layouts: [{ width: 'watch', tracks: 1, cells: [] }] } })).toBeNull()
    })
```

`rows.test.ts`, add (and change existing `visibleRows(rows, COLUMNS)` calls to `visibleRows(rows)`; the child-row test gives its child rows `labelColumn: 'customer'`):

```ts
describe('expansion state', () => {
    it('sends no expand on the first run, so groups marked open open', () => {
        expect(expansionOptions(DEFAULT_EXPANSION)).toEqual({})
    })
    it('collapse sends an empty expand, so groups marked open stay closed', () => {
        expect(foldAllTarget([group('g:a', ['a'], { expanded: true })])).toEqual(NO_EXPANSION)
        expect(expansionOptions(NO_EXPANSION)).toEqual({ expand: [] })
    })
    it('a press under the default state keeps the groups the result shows open', () => {
        const rows = [group('g:a', ['a'], { expanded: true }), group('g:b', ['b'])]
        expect(toggleGroup(DEFAULT_EXPANSION, rows[1] as ResultRow, rows)).toEqual({ all: false, keys: [['a'], ['b']] })
    })
})

describe('child rows', () => {
    it('drop exactly the cell their label repeats, and a label that is no cell drops none', () => {
        const child: ResultRow = { id: 'd:1/x', kind: 'detail', depth: 2, label: 'Ann', parent: 'd:1', labelColumn: 'customer', cells: { customer: 'Ann', amount: 5 } }
        const own: ResultRow = { ...child, labelColumn: undefined }
        expect(visibleRows([child])[0]?.cells).toEqual({ amount: 5 })
        expect(visibleRows([own])[0]?.cells).toEqual({ customer: 'Ann', amount: 5 })
    })
})
```

`params.test.ts`, add:

```ts
describe('a choice whose options come from a source', () => {
    const def: ParamDef = {
        id: 'accounts',
        type: 'choice',
        label: 'Accounts',
        multiple: true,
        optionsFrom: { source: 'x.accounts', value: [{ field: 'id' }], label: [{ field: 'name' }] },
    }
    it('takes any ids, checked by type only', () => {
        expect(valueFromJson(def, ['a1', 'b2'])).toEqual({ kind: 'choice', values: ['a1', 'b2'] })
        expect(valueFromJson(def, [''])).toBeNull()
        expect(valueFromJson(def, Array.from({ length: 1001 }, (_, i) => `x${i}`))).toBeNull()
    })
    it('names one chosen record, and counts more', () => {
        const labels = new Map([['a1', 'Cash'], ['b2', 'Bank']])
        expect(chipText(def, { kind: 'choice', values: ['a1'] }, undefined, labels)).toBe('Accounts: Cash')
        expect(chipText(def, { kind: 'choice', values: ['a1', 'b2', 'c3'] }, undefined, labels)).toBe('Accounts: 3 selected')
    })
})
```

`run-client.test.ts`, add:

```ts
// The PocketBase client is mocked the way core's unit tests mock it
// (vi.mock of @tinycld/core/lib/pocketbase), at the top of run-client.test.ts:
//   const send = vi.fn()
//   vi.mock('@tinycld/core/lib/pocketbase', () => ({ pb: { send: (...args: unknown[]) => send(...args) } }))
describe('fetchParamOptions', () => {
    it('posts the request to the param-options route and returns the page', async () => {
        const page = { options: [{ value: 'a', label: 'Bank' }], groups: [] }
        send.mockResolvedValueOnce(page)
        const signal = new AbortController().signal
        const body = { reportId: 'r1', param: 'accounts', search: 'ba', today: '2026-11-01' }
        await expect(fetchParamOptions(body, signal)).resolves.toEqual(page)
        expect(send).toHaveBeenCalledWith(PARAM_OPTIONS_PATH, { method: 'POST', body, signal, requestKey: null })
        expect(PARAM_OPTIONS_PATH).toBe('/api/reports/param-options')
    })
})
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib`
Expected: FAIL (`parseSpec` returns null for the v2 parts; `DEFAULT_EXPANSION` and `PARAM_OPTIONS_PATH` are not exported).

- [ ] **Step 3: `lib/spec.ts`**

Add to `SpecNode`: `record?: string`, `source?: string`, `pick?: 'min' | 'max'` (comments as in Go), and to `specNodeSchema`: `record: z.string().optional()`, `source: z.string().optional()`, `pick: z.enum(['min', 'max']).optional()`. Then:

```ts
export const optionsFromSchema = z.strictObject({
    source: z.string(),
    value: z.array(specHopSchema),
    label: z.array(specHopSchema),
    group: z.array(specHopSchema).optional(),
    where: specNodeSchema.optional(),
})
export type OptionsFrom = z.infer<typeof optionsFromSchema>
```

`paramDefSchema` gains `optionsFrom: optionsFromSchema.optional()`; `specColumnSchema` gains `total: z.enum(['recompute', 'rows']).optional()`. Add and use in `reportSpecSchema`:

```ts
export const specSortKeySchema = z.strictObject({
    col: z.string().optional(),
    formula: specNodeSchema.optional(),
    dir: z.enum(['asc', 'desc']).optional(),
})
export type SpecSortKey = z.infer<typeof specSortKeySchema>

export const specGroupSchema = z.strictObject({
    id: z.string(),
    col: z.string(),
    bucket: z.enum(BUCKETS).optional(),
    subtotals: z.boolean().optional(),
    label: specNodeSchema.optional(),
    having: specNodeSchema.optional(),
    fill: z.strictObject({ where: specNodeSchema.optional() }).optional(),
    open: z.boolean().optional(),
})
export type SpecGroup = z.infer<typeof specGroupSchema>

export const DISPLAY_WIDTHS = ['mobile', 'tablet', 'desktop'] as const
export type DisplayWidth = (typeof DISPLAY_WIDTHS)[number]
export const displayCellSchema = z.strictObject({
    col: z.string(),
    line: z.number().int(),
    start: z.number().int().optional(),
    span: z.number().int().optional(),
    lines: z.number().int().optional(),
    justify: z.enum(['start', 'center', 'end']).optional(),
})
export type DisplayCell = z.infer<typeof displayCellSchema>
export const displayLayoutSchema = z.strictObject({
    width: z.enum(DISPLAY_WIDTHS),
    tracks: z.number().int(),
    cells: z.array(displayCellSchema),
})
export type DisplayLayout = z.infer<typeof displayLayoutSchema>
```

In `reportSpecSchema`: `sort: z.array(specSortKeySchema).nullish()`, `groups: z.array(specGroupSchema).nullish()`, and add `detail: z.strictObject({ label: specNodeSchema.optional() }).nullish()`, `display: z.strictObject({ layouts: z.array(displayLayoutSchema).optional() }).nullish()`.

- [ ] **Step 4: `lib/types.ts`, `lib/rows.ts`, `lib/run-client.ts`, `lib/params.ts`**

`types.ts`: `ResultRow` gains `/** The column whose cell the label repeats (a child row leaves that cell out). */ labelColumn?: string`; `RunResult` gains `/** The source's heading, printed above the title; else the organization's name. */ heading?: string`; add:

```ts
export interface ParamOption {
    value: string
    label: string
    group?: string
}
export interface ParamOptionGroup {
    value: string
    label: string
}
/** One page of a choice param's options from a source (POST /api/reports/param-options). */
export interface ParamOptionsPage {
    options: ParamOption[]
    groups: ParamOptionGroup[]
    next?: string
}
```

`rows.ts`:

```ts
/**
 * Which groups a run opens: every one, those whose keys are listed, or — on the
 * first run, before any press — the levels the report marks open (the engine's
 * default, sent as no expand at all).
 */
export type Expansion = { all: boolean; keys: readonly unknown[][]; isDefault?: boolean }

export const DEFAULT_EXPANSION: Expansion = { all: false, keys: [], isDefault: true }
export const NO_EXPANSION: Expansion = { all: false, keys: [] }
```

`toggleGroup`'s first line: `const current = expansion.all || expansion.isDefault ? openGroupKeys(rows) : [...expansion.keys]`. `expansionOptions`:

```ts
export function expansionOptions(expansion: Expansion): { expand?: unknown[][]; expandAll?: boolean } {
    if (expansion.isDefault) return {}
    if (expansion.all) return { expandAll: true }
    // An explicit empty list opens nothing, groups marked open included (R10).
    return { expand: [...expansion.keys] }
}
```

`childCells(row)` drops `row.labelColumn` when set (else nothing); `rowCells(row, withSubtotal)` and `visibleRows(rows)` lose the `columns` parameter. `hooks/useReportView.ts`: `?? DEFAULT_EXPANSION` (line 115) and `rows: visibleRows(rows)` (line 217); add `heading: run.result?.heading ?? ''` to the returned model (Task 23 prints it).

`run-client.ts`:

```ts
export const PARAM_OPTIONS_PATH = '/api/reports/param-options'

export type ParamOptionsBody = ({ reportId: string } | { spec: ReportSpec }) & {
    param: string
    search?: string
    after?: string
    group?: string
    values?: string[]
    limit?: number
    params?: Record<string, unknown>
    today: string
}

/** One page of a choice param's options, read under the reader's rules. */
export async function fetchParamOptions(body: ParamOptionsBody, signal?: AbortSignal): Promise<ParamOptionsPage> {
    return pb.send<ParamOptionsPage>(PARAM_OPTIONS_PATH, { method: 'POST', body, signal, requestKey: null })
}
```

`params.ts`: export `MAX_CHOICE_VALUES = 1000`; in `choiceValue`, before the `offered` check:

```ts
    if (def.optionsFrom) {
        // Its options live in a source the reader's rules filter: checked by type only (R13).
        const ids = raw.filter((item): item is string => typeof item === 'string' && item.length > 0 && item.length <= MAX_TEXT_PARAM)
        if (ids.length !== raw.length || ids.length > MAX_CHOICE_VALUES) return null
        if (!def.multiple && ids.length !== 1) return null
        return { kind: 'choice', values: [...new Set(ids)] }
    }
```

`choiceLabels(def, values, labels?)` reads `labels?.get(value)` before `def.options`; `valueText` and `chipText` take an optional fourth argument `labels?: ReadonlyMap<string, string>` and pass it on.

Whole groups (F19, R30): the choice member of `ParamValue` becomes `{ kind: 'choice'; values: string[]; groups?: string[] }`. The `optionsFrom` branch above reads each item as an id or a group reference:

```ts
    if (def.optionsFrom) {
        // Its options live in a source the reader's rules filter: checked by type only (R13).
        // A whole group is a reference the server expands at run time (R30).
        const ids: string[] = []
        const groups: string[] = []
        for (const item of raw) {
            if (typeof item === 'string' && item.length > 0 && item.length <= MAX_TEXT_PARAM) ids.push(item)
            else if (isGroupRef(item)) groups.push(item.group)
            else return null
        }
        if (ids.length > MAX_CHOICE_VALUES || groups.length > MAX_OPTION_GROUPS) return null
        if (!def.multiple && (ids.length !== 1 || groups.length > 0)) return null
        return groups.length > 0 ? { kind: 'choice', values: [...new Set(ids)], groups: [...new Set(groups)] } : { kind: 'choice', values: [...new Set(ids)] }
    }
```

with `MAX_OPTION_GROUPS = 200` and `isGroupRef(item): item is { group: string }` (an object whose only key is a 1–500 character string `group`). `valueToJson` writes a choice as `[...values, ...(groups ?? []).map(group => ({ group }))]`. `choiceLabels` lists the values' labels, then `All <label>` for each group, its label read from `labels?.get('group:' + group)` (else the group value). `view-url.ts` `encodeParamValue` writes a multiple choice as `JSON.stringify(valueToJson(def, value))`, so groups survive the address. Tests (`params.test.ts`): with `def = { id: 'teams', type: 'choice', multiple: true, optionsFrom: { source: 's', value: [{ field: 'id' }], label: [{ field: 'name' }] } }`, `valueFromJson(def, ['a1', { group: 'north' }])` is `{ kind: 'choice', values: ['a1'], groups: ['north'] }`, `valueToJson` gives `['a1', { group: 'north' }]` back, `valueFromJson(def, [{ group: 7 }])` is null, and `chipText({ ...def, label: 'Teams' }, that value, undefined, new Map([['a1', 'N1'], ['group:north', 'North']]))` is `'Teams: N1, All North'`; `view-url.test.ts`: that value encodes to `'["a1",{"group":"north"}]'` and decodes back.

Link labels (F22, R31): `spec.ts` `specColumnSchema` gains `labelField: z.string().min(1).max(MAX_ID_LENGTH).optional()`; `types.ts` `ResultColumn` gains `/** The cell holding the label of the record a link cell names (R31). */ labelCell?: string`. `format.ts` `cellDisplay`, first:

```ts
    // A link shows its record's label, never an id; a blank link reads "(none)" (R31).
    if (column.labelCell) return plainDisplay(linkLabel(row.cells[column.labelCell]), 'left')
```

with `linkLabel = (label: CellValue | undefined) => (typeof label === 'string' ? label : '(none)')`. Tests (`format.test.ts`): a column `{ id: 'team', type: 'relation', labelCell: 'team@label' }` shows `'North team'` for a row with cells `{ team: 'r1', 'team@label': 'North team' }` and `'(none)'` for `{ team: null, 'team@label': null }`.

- [ ] **Step 5: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports`
Expected: PASS.

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/reports
git commit -m "feat(core): the reports client reads spec v2, headings, label columns and param options"
git push origin books/p5-trial-balance
```

---

### Task 17: The `salesstub` stub reaches every v2 capability; E11 report e2e helpers

Code facts used: the stub's templates (`tests/scripts/reports-stub/**`, copied by `tests/scripts/scaffold-reports-stub.ts:38-53`), the stub's forms (`screens/index.tsx.txt:22-135`), the report helpers (`tests/e2e/reports-helpers.ts`), and the core barrel (`core/e2e-helpers.ts`, which re-exports with `.ts` extensions because siblings load it through Node's ESM loader).

**Files:**
- Modify: `tests/scripts/reports-stub/pb-migrations/2090000300_salesstub.js.txt` (`salesstub_sales.booked_on`, `salesstub_teams.region`)
- Modify: `tests/scripts/reports-stub/screens/index.tsx.txt` (write `booked_on`; a region choice on the team form)
- Modify: `tests/scripts/reports-stub/server/register.go.txt` (text day, region, heading, the second shipped report)
- Create: `tests/e2e/report-view-helpers.ts`
- Modify: `tests/e2e/reports-helpers.ts`, `core/e2e-helpers.ts`

**Interfaces:**
- Produces (e2e): `openReportsHub`, `hubRow`, `pressToolbar`, `rowOf`, `cellOf`, `chooseParamOption(page, paramId, { from, search?, labels?, group? })` from `@tinycld/core/e2e-helpers`; `addTeam(page, name, region = 'north')`; stub constants `TEAM_TOTALS_TITLE = 'Stub team totals'`, `STUB_HEADING = 'Stub Sales Co'`.

- [ ] **Step 1: The migration**

In the sales `fields`, after `day`: `{ id: 'salesstub_sales_booked_on', name: 'booked_on', type: 'text', max: 10 },`. In the teams `fields`, after `name`: `{ id: 'salesstub_teams_region', name: 'region', type: 'select', values: ['north', 'south'], maxSelect: 1 },`. (A test fixture migration, never released: edited in place.)

- [ ] **Step 2: The stub screen**

`teamSchema` becomes `z.object({ name: z.string().trim().min(1).max(200), region: z.enum(['north', 'south']) })`; the team form's defaults `{ name: '', region: 'north' }`; the insert `teams.insert({ id: newRecordId(), name: values.name, region: values.region })`; reset `{ name: '', region: values.region }`; add under the name field:

```tsx
            <ButtonGroup
                name="region"
                control={form.control}
                label="Region"
                horizontal
                options={REGION_OPTIONS}
            />
```

with `const REGION_OPTIONS = [{ value: 'north', label: 'North' }, { value: 'south', label: 'South' }]`. In the sale insert add `booked_on: values.day,` (a text day, R22).

- [ ] **Step 3: The stub's Go module**

In `register.go.txt`: the sales source gains

```go
			// A day kept as text, read as a date (E1).
			{Key: "booked", Column: "booked_on", Label: "Booked", Type: reports.Date, TextDay: true, Aggregates: reports.MinMax},
```

and `Heading: stubHeading,`; the teams source gains `{Key: "region", Label: "Region", Type: reports.Select, Options: []reports.Option{{Value: "north", Label: "North"}, {Value: "south", Label: "South"}}},` and `Heading: stubHeading,`; add `func stubHeading(core.App) string { return "Stub Sales Co" }` (the parameter becomes the named import `core`, already imported). After the first shipped report, register:

```go
	// An unpublished shipped report (E10) that uses the v2 parts: a record
	// group with a label, a total of the rows shown, a sort formula, a detail
	// label, a text-day filter and a choice from a source grouped by region.
	// The e2e owner shares it to show who sees it (reports-source-params.spec.ts).
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.team_totals", Title: "Stub team totals", Revision: 1, Unpublished: true,
		Description: "Each team's sales booked on or before a day.",
		Spec: json.RawMessage(`{
			"version": 2,
			"source": "salesstub.sales",
			"params": [
				{"id": "asOf", "type": "asOf", "label": "As of", "default": {"preset": "today"}},
				{"id": "teams", "type": "choice", "label": "Teams", "multiple": true,
				 "optionsFrom": {"source": "salesstub.teams", "value": [{"field": "id"}], "label": [{"field": "name"}], "group": [{"field": "region"}]}}
			],
			"columns": [
				{"id": "team", "label": "Team", "path": [{"link": "team"}]},
				{"id": "region", "label": "Region", "formula": {"record": "byTeam", "path": [{"field": "region"}]}},
				{"id": "amount", "label": "Amount", "path": [{"field": "amount"}], "aggregate": "sum", "total": "rows"}
			],
			"filter": {"op": "and", "args": [
				{"op": "lte", "args": [{"path": [{"field": "booked"}]}, {"param": "asOf"}]},
				{"op": "in", "args": [{"path": [{"link": "team"}]}, {"param": "teams"}]}
			]},
			"groups": [{"id": "byTeam", "col": "team", "label": {"record": "byTeam", "path": [{"field": "name"}]}}],
			"sort": [{"formula": {"record": "byTeam", "path": [{"field": "name"}]}}],
			"detail": {"label": {"op": "concat", "args": [{"path": [{"field": "customer"}]}, {"value": " · "}, {"path": [{"field": "booked"}]}]}}
		}`),
	})
```

- [ ] **Step 4: E11 — move the generic helpers**

Create `tests/e2e/report-view-helpers.ts` with `openReportsHub`, `hubRow`, `pressToolbar`, `rowOf` and `cellOf` moved verbatim from `reports-helpers.ts:67-73,114-146` (import `navigateToPackage` from `'./helpers.ts'`, with the extension, as `core/e2e-helpers.ts` needs), plus:

```ts
export interface ParamPick {
    /** How the picker opens: the param's chip, or Add filter for a param not set yet. */
    from: 'chip' | 'add-filter'
    search?: string
    /** Option labels to tick (a multiple choice) or press (a single choice). */
    labels?: string[]
    /** A group value whose "All <group>" row to press. */
    group?: string
}

/** Opens a param's picker, searches, picks options or a whole group, and applies. */
export async function chooseParamOption(page: Page, paramId: string, pick: ParamPick) {
    if (pick.from === 'chip') await page.getByTestId(`report-param-${paramId}`).click()
    else {
        await page.getByTestId('report-add-filter').click()
        await page.getByTestId(`report-add-filter-${paramId}`).click()
    }
    const picker = page.getByTestId('report-filter-picker')
    await expect(picker).toBeVisible()
    if (pick.search !== undefined) await picker.getByTestId('report-filter-search').fill(pick.search)
    if (pick.group) await picker.getByTestId(`report-filter-group-${pick.group}`).click()
    for (const label of pick.labels ?? []) {
        await picker.getByRole('checkbox', { name: label, exact: true }).or(picker.getByRole('radio', { name: label, exact: true })).click()
    }
    const apply = picker.getByTestId('report-filter-apply')
    if (await apply.isVisible()) await apply.click()
    await expect(picker).toBeHidden()
}
```

In `reports-helpers.ts`, delete the moved functions, add `export { cellOf, chooseParamOption, hubRow, openReportsHub, pressToolbar, rowOf } from './report-view-helpers'` (every existing spec keeps its imports), add the constants `TEAM_TOTALS_TITLE` and `STUB_HEADING`, and give `addTeam` a region:

```ts
export async function addTeam(page: Page, name: string, region: 'north' | 'south' = 'north') {
    await page.getByTestId('salesstub-team-name').fill(name)
    await page.getByRole('radio', { name: region === 'north' ? 'North' : 'South', exact: true }).first().click()
    await page.getByTestId('salesstub-team-create').click()
    await expect(page.getByTestId('salesstub-team-saved')).toHaveText(`Saved ${name}`)
}
```

(The team form's radios come first on the screen, so `.first()` is the team's own.) In `core/e2e-helpers.ts`, add `export * from '../tests/e2e/report-view-helpers.ts'` with the comment "the generic report view helpers (E11): a package's report e2e opens core's view with them".

- [ ] **Step 5: Re-scaffold, regenerate and smoke-test the stub**

Run (the executor's own workspace setup, never at plan time):
`cd /Users/nas/tmp/tinycld/books-purchases && PATH=$PWD/node_modules/.bin:$PATH tsx tinycld/tests/scripts/scaffold-reports-stub.ts && cd tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH pnpm run packages:generate`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-shipped.spec.ts tests/e2e/reports-create.spec.ts`
Expected: the server starts with no refused source (the stub's text day, select and second shipped report validate at start: a refused one is logged as `report source refused`), and both specs pass unchanged (the builder still shows the `if` formula read-only until Task 20).

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add tests/scripts/reports-stub tests/e2e/report-view-helpers.ts tests/e2e/reports-helpers.ts core/e2e-helpers.ts
git commit -m "test(core): the reports stub reaches spec v2; report view e2e helpers exported"
git push origin books/p5-trial-balance
```

---

### Task 18: OQ2 (part 1) — the forms `FormulaEditor` takes an optional `operators` prop

Code facts used: `FormulaEditor` and its parts (`core/components/forms-builder/FormulaEditor.tsx:1-362`): every edit replaces one part by its path through `exprAt`/`replaceAt`; the part helpers (`core/lib/forms/builder/formula.ts:79-262`) are pure; `withChildren` is module-private (`formula.ts:130-136`). Callers: `FormulaSettings.tsx:45` (forms), `core/reports/components/builder/ColumnsSection.tsx:162` and `LayoutSection.tsx:79` (reports). Forms' component tests (`core/components/forms-builder/__tests__/formula-settings.test.tsx`, `columns-editor.test.tsx`) and `tests/e2e/forms-builder.spec.ts` pin today's behaviour.

**Files:**
- Create: `core/lib/forms/builder/formula-tree.ts`, `core/lib/forms/builder/__tests__/formula-tree.test.ts`
- Modify: `core/lib/forms/builder/formula.ts` (export `withChildren`, `FORMULA_PARTS`, `isFormulaPart`; `exprAt`/`replaceAt` delegate to the tree helpers)
- Modify: `core/components/forms-builder/FormulaEditor.tsx`

**Interfaces:**
- Produces: `interface FormulaTree<E> { childrenOf(expr: E): E[]; withChildren(expr: E, parts: E[]): E }`; `treeAt<E>(tree, expr, path): E | undefined`; `replaceTreeAt<E>(tree, expr, path, next): E`; `interface FormulaOperators<E> extends FormulaTree<E> { parse; partOf; partsAt(root, path); label(part); withPart(root, path, part); slotLabel(expr, index); isVariadic; canRemove; addOperand; removeOperand(expr, index); describe; renderLeaf(props: FormulaLeafProps<E>): ReactNode }`; `interface FormulaLeafProps<E> { root: E; path: FormulaPath; expr: E; set(next: E): void; id: string; where: string }`; `computeOperators(operands, valueNoun): FormulaOperators<ComputeExpr>`; `FormulaEditorProps<E = ComputeExpr>` (R16 union).

- [ ] **Step 1: Write the failing test**

Create `core/lib/forms/builder/__tests__/formula-tree.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { ComputeExpr } from '../../expression'
import { childrenOf, withChildren } from '../formula'
import { replaceTreeAt, treeAt } from '../formula-tree'

const tree = { childrenOf, withChildren }
const expr: ComputeExpr = { add: [1, { mul: [{ var: 'a' }, 2] }] }

describe('the formula tree', () => {
    it('reads a part by its path', () => {
        expect(treeAt(tree, expr, [])).toBe(expr)
        expect(treeAt(tree, expr, [1, 0])).toEqual({ var: 'a' })
        expect(treeAt(tree, expr, [5])).toBeUndefined()
    })
    it('replaces a part by its path and leaves the rest', () => {
        expect(replaceTreeAt(tree, expr, [1, 1], 3)).toEqual({ add: [1, { mul: [{ var: 'a' }, 3] }] })
        expect(replaceTreeAt(tree, expr, [9], 3)).toEqual(expr)
        expect(replaceTreeAt(tree, expr, [], 7)).toBe(7)
    })
})
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/lib/forms/builder/__tests__/formula-tree.test.ts`
Expected: FAIL: cannot resolve `../formula-tree` (and `withChildren` is not exported).

- [ ] **Step 3: The tree helpers**

Create `core/lib/forms/builder/formula-tree.ts`:

```ts
import type { FormulaPath } from './formula'

/**
 * How a formula tree holds its parts. The formula editor walks any tree
 * through these two functions: the forms engine's ComputeExpr, or another
 * engine's own tree (the report builder's).
 */
export interface FormulaTree<E> {
    childrenOf(expr: E): E[]
    withChildren(expr: E, parts: E[]): E
}

export function treeAt<E>(tree: FormulaTree<E>, expr: E, path: FormulaPath): E | undefined {
    if (path.length === 0) return expr
    const [head, ...rest] = path
    const child = head === undefined ? undefined : tree.childrenOf(expr)[head]
    return child === undefined ? undefined : treeAt(tree, child, rest)
}

/** The tree with the part at `path` replaced; a path that names no part changes nothing. */
export function replaceTreeAt<E>(tree: FormulaTree<E>, expr: E, path: FormulaPath, next: E): E {
    if (path.length === 0) return next
    const [head, ...rest] = path
    const parts = tree.childrenOf(expr)
    const child = head === undefined ? undefined : parts[head]
    if (child === undefined) return expr
    return tree.withChildren(
        expr,
        parts.map((p, i) => (i === head ? replaceTreeAt(tree, child, rest, next) : p))
    )
}
```

In `formula.ts`: `export function withChildren(…)` (unchanged body); export `FORMULA_PARTS: readonly FormulaPart[] = PART_ORDER` and

```ts
export function isFormulaPart(part: string): part is FormulaPart {
    return (PART_ORDER as readonly string[]).includes(part)
}
```

and make `exprAt`/`replaceAt` delegate: `export const exprAt = (expr: ComputeExpr, path: FormulaPath) => treeAt(COMPUTE_TREE, expr, path)` and `export const replaceAt = (expr: ComputeExpr, path: FormulaPath, next: ComputeExpr) => replaceTreeAt(COMPUTE_TREE, expr, path, next)` with `const COMPUTE_TREE: FormulaTree<ComputeExpr> = { childrenOf, withChildren }`.

- [ ] **Step 4: The editor over any operator set**

Rewrite `FormulaEditor.tsx` so the drawing code is generic and the forms behaviour is one operator set. The markup, labels, accessibility names and test ids of every forms part stay exactly as they are.

```tsx
import {
    addOperand,
    asExpr,
    canRemoveOperand,
    childrenOf,
    describeFormula,
    exprAt,
    type FormulaOperands,
    type FormulaPath,
    isFormulaPart,
    isVariadic,
    PART_LABELS,
    partOf,
    partsFor,
    removeOperand,
    setAggregateColumn,
    setAggregateTable,
    setPlaces,
    withChildren,
    withPart,
} from '@tinycld/core/lib/forms/builder'
import { type FormulaTree, replaceTreeAt, treeAt } from '@tinycld/core/lib/forms/builder/formula-tree'
import type { ComputeExpr } from '@tinycld/core/lib/forms/expression'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Fragment, type ReactNode } from 'react'
import { Text, View } from 'react-native'
import { NumberText, type PickItem, PickMenu, RemoveButton } from './rule-parts'

const PLACES = [0, 1, 2, 3, 4, 5, 6]

/** What a leaf's controls get: the part, how to replace it, and its test id stem. */
export interface FormulaLeafProps<E> {
    root: E
    path: FormulaPath
    expr: E
    set: (next: E) => void
    id: string
    /** ", part 1.2" for a screen reader; "" for the whole formula. */
    where: string
}

/**
 * The parts a formula editor offers and how each behaves. Forms leave it out and
 * get Number, Field, the arithmetic and the table totals (computeOperators); the
 * report builder passes the report operators.
 */
export interface FormulaOperators<E> extends FormulaTree<E> {
    parse(value: unknown): E
    partOf(expr: E): string
    partsAt(root: E, path: FormulaPath): readonly string[]
    label(part: string): string
    withPart(root: E, path: FormulaPath, part: string): E
    /** A caption above a part ("When", "Then"), or null. */
    slotLabel(expr: E, index: number): string | null
    isVariadic(expr: E): boolean
    canRemove(expr: E): boolean
    addOperand(expr: E): E
    removeOperand(expr: E, index: number): E
    describe(expr: E): string
    /**
     * The controls beside a part's menu. Called as a function, not drawn as a
     * component: an operator set is rebuilt each render, and a new component
     * type would remount its inputs and drop the caret while typing.
     */
    renderLeaf(props: FormulaLeafProps<E>): ReactNode
}

interface Common {
    /** The stored formula; a draft holds it untyped. */
    value: unknown
    testID: string
}

/** Without `operators`, today's forms editor; with them, that operator set's own tree (R16). */
export type FormulaEditorProps<E = ComputeExpr> =
    | (Common & {
          operators?: undefined
          onChange: (expr: ComputeExpr) => void
          operands: FormulaOperands
          /** What a Field part is called: "Field", or "Column" in a column's formula. */
          valueNoun: string
      })
    | (Common & { operators: FormulaOperators<E>; onChange: (expr: E) => void })
```

Forms' leaves, rewritten to take `set` and the pure helpers (same ids and labels as today's `NumberPart`, `ValuePick`, `PlacesPick`, `TotalPick`):

```tsx
// The leaf views both operator sets draw (F15): a typed number, a value
// picked from a list, and decimal places.

/** A number typed into a formula part. A cleared number is 0, the value the engines read for nothing. */
export function FormulaNumberInput({ value, onChange, label, testID }: { value: number; onChange: (n: number) => void; label: string; testID: string }) {
    return <NumberText value={value} onChange={n => onChange(n ?? 0)} label={label} placeholder="0" emptyAs={0} testID={testID} />
}

/** A value a part reads: a field, a column or a section, by key. */
export function FormulaValuePick({ choices, selected, onChange, purpose, testID }: { choices: readonly { key: string; label: string }[]; selected: string; onChange: (key: string) => void; purpose: string; testID: string }) {
    const items: PickItem[] = choices.map(c => ({ id: c.key, label: c.label, isSelected: c.key === selected, onSelect: () => onChange(c.key) }))
    const label = choices.find(c => c.key === selected)?.label ?? `${selected} (missing)`
    return <PickMenu label={label} purpose={purpose} items={items} testID={testID} />
}

/** The decimal places of a Round or a Divide. */
export function FormulaPlacesPick({ places, onChange, purpose, testID }: { places: number; onChange: (n: number) => void; purpose: string; testID: string }) {
    const items: PickItem[] = PLACES.map(n => ({ id: String(n), label: String(n), isSelected: n === places, onSelect: () => onChange(n) }))
    return <PickMenu label={`${places} places`} purpose={purpose} items={items} testID={testID} />
}

type ComputeLeafProps = FormulaLeafProps<ComputeExpr> & { operands: FormulaOperands; valueNoun: string }

function NumberPart({ expr, set, id, where }: ComputeLeafProps) {
    if (typeof expr !== 'number') return null
    return <FormulaNumberInput value={expr} onChange={set} label={`Number${where}`} testID={`${id}-number`} />
}

function ValuePick({ expr, set, id, where, operands, valueNoun }: ComputeLeafProps) {
    if (typeof expr !== 'object' || !('var' in expr)) return null
    return <FormulaValuePick choices={operands.values} selected={expr.var} onChange={key => set({ var: key })} purpose={`${valueNoun}${where}`} testID={`${id}-field`} />
}

function PlacesPick({ expr, set, id, where }: ComputeLeafProps) {
    if (typeof expr !== 'object' || !('round' in expr)) return null
    return <FormulaPlacesPick places={expr.round[1]} onChange={n => set(setPlaces(expr, n))} purpose={`Decimal places${where}`} testID={`${id}-places`} />
}
```

`TotalPick` and `ColumnPick` move over the same way (`setTable` → `set(setAggregateTable(expr, key, operands))`, `setColumn` → `set(setAggregateColumn(expr, key))`). Then:

```tsx
function ComputeLeaf(props: ComputeLeafProps) {
    return (
        <>
            <NumberPart {...props} />
            <ValuePick {...props} />
            <PlacesPick {...props} />
            <TotalPick {...props} />
        </>
    )
}

/** The forms engine's parts: Number, Field, the arithmetic and the table totals. */
export function computeOperators(operands: FormulaOperands, valueNoun: string): FormulaOperators<ComputeExpr> {
    return {
        parse: asExpr,
        partOf,
        childrenOf,
        withChildren,
        partsAt: () => partsFor(operands),
        label: part => (part === 'field' ? valueNoun : isFormulaPart(part) ? PART_LABELS[part] : part),
        withPart: (root, path, part) => {
            const current = exprAt(root, path) ?? 0
            return isFormulaPart(part) ? withPart(current, part, operands) : current
        },
        slotLabel: () => null,
        isVariadic,
        canRemove: canRemoveOperand,
        addOperand,
        removeOperand,
        describe: expr => describeFormula(expr, operands),
        renderLeaf: props => <ComputeLeaf {...props} operands={operands} valueNoun={valueNoun} />,
    }
}
```

The generic tree (today's `FormulaNode`, `Operands`, `AddPart`, `RemovePart`, with `formula.*` calls replaced by operator calls):

```tsx
interface Tree<E> {
    operators: FormulaOperators<E>
    root: E
    testID: string
    set: (path: FormulaPath, next: E) => void
}

const idOf = (testID: string, path: FormulaPath) => `${testID}-${path.length ? path.join('-') : 'root'}`
const placeOf = (path: FormulaPath) => path.map(i => i + 1).join('.')
const whereOf = (path: FormulaPath) => (path.length ? `, part ${placeOf(path)}` : '')

interface Removal {
    onPress: () => void
    label: string
}

function SlotLabel({ text }: { text: string | null }) {
    if (!text) return null
    return <Text className="text-xs text-muted-foreground">{text}</Text>
}

function Operands<E>({ tree, path, expr, id }: { tree: Tree<E>; path: FormulaPath; expr: E; id: string }) {
    const { operators } = tree
    const parts = operators.childrenOf(expr)
    if (parts.length === 0) return null
    const holder = operators.label(operators.partOf(expr))
    const canRemove = operators.canRemove(expr)
    const nodes = parts.map((part, index) => {
        const childPath = [...path, index]
        const removal: Removal | null = canRemove
            ? { onPress: () => tree.set(path, operators.removeOperand(expr, index)), label: `Remove part ${placeOf(childPath)} of ${holder}` }
            : null
        return {
            childPath,
            removal,
            slot: operators.slotLabel(expr, index),
            key: `${idOf(tree.testID, childPath)}:${operators.partOf(part)}:${parts.length}`,
        }
    })
    const items = nodes.map(node => (
        <Fragment key={node.key}>
            <SlotLabel text={node.slot} />
            <FormulaNode tree={tree} path={node.childPath} removal={node.removal} />
        </Fragment>
    ))
    return (
        <View className="pl-3 border-l border-border gap-1.5">
            {items}
            <AddPart
                isVisible={operators.isVariadic(expr)}
                onPress={() => tree.set(path, operators.addOperand(expr))}
                label={`Add part to ${holder}${whereOf(path)}`}
                testID={`${id}-add`}
            />
        </View>
    )
}

function FormulaNode<E>({ tree, path, removal }: { tree: Tree<E>; path: FormulaPath; removal: Removal | null }) {
    const expr = treeAt(tree.operators, tree.root, path)
    if (expr === undefined) return null
    const { operators } = tree
    const part = operators.partOf(expr)
    const id = idOf(tree.testID, path)
    const partItems: PickItem[] = operators.partsAt(tree.root, path).map(p => ({
        id: p,
        label: operators.label(p),
        isSelected: p === part,
        onSelect: () => tree.set(path, operators.withPart(tree.root, path, p)),
    }))
    const leaf = operators.renderLeaf({ root: tree.root, path, expr, set: next => tree.set(path, next), id, where: whereOf(path) })
    const purpose = path.length ? `Part ${placeOf(path)}` : 'Formula part'
    return (
        <View className="gap-1.5" testID={id}>
            <View className="flex-row flex-wrap items-center gap-1.5">
                <PickMenu
                    label={operators.label(part)}
                    purpose={purpose}
                    items={partItems}
                    testID={`${id}-part`}
                />
                {leaf}
                <RemovePart removal={removal} testID={`${id}-remove`} />
            </View>
            <Operands tree={tree} path={path} expr={expr} id={id} />
        </View>
    )
}

function FormulaTreeEditor<E>({
    operators,
    value,
    onChange,
    testID,
}: { operators: FormulaOperators<E>; value: unknown; onChange: (expr: E) => void; testID: string }) {
    const root = operators.parse(value)
    const tree: Tree<E> = { operators, root, testID, set: (path, next) => onChange(replaceTreeAt(operators, root, path, next)) }
    const summary = operators.describe(root)
    return (
        <View className="mb-3 gap-1.5" testID={testID}>
            <Text className="text-sm font-semibold text-foreground">Formula</Text>
            <Text className="text-sm text-foreground" accessibilityLabel={`Formula: ${summary}`} testID={`${testID}-summary`}>
                {`= ${summary}`}
            </Text>
            <FormulaNode key={operators.partOf(root)} tree={tree} path={[]} removal={null} />
        </View>
    )
}

/**
 * A formula built from parts picked from menus: no formula is typed, only a
 * number (or a text) where such a part is picked. The line on top reads it back.
 */
export function FormulaEditor<E = ComputeExpr>(props: FormulaEditorProps<E>) {
    if (props.operators === undefined) {
        return (
            <FormulaTreeEditor
                operators={computeOperators(props.operands, props.valueNoun)}
                value={props.value}
                onChange={props.onChange}
                testID={props.testID}
            />
        )
    }
    return <FormulaTreeEditor operators={props.operators} value={props.value} onChange={props.onChange} testID={props.testID} />
}
```

(`RemovePart` and `AddPart` keep their bodies. `withChildren` and the new exports reach `@tinycld/core/lib/forms/builder` through `builder/index.ts`, which re-exports `formula.ts`.)

- [ ] **Step 5: Run the forms and reports tests**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/lib/forms core/components/forms-builder core/reports`
Expected: PASS: the forms editor's component tests are unchanged and green; the report builder still passes `operands` (it moves to `operators` in Task 20).

- [ ] **Step 6: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/lib/forms/builder core/components/forms-builder/FormulaEditor.tsx
git commit -m "feat(core): the formula editor takes an optional operator set"
git push origin books/p5-trial-balance
```

---

### Task 19: OQ2 (part 2) — the report operator set; report formula drafts edit the spec's tree

Code facts used: every operator the engine compiles (`core/server/reports/formula.go:139-590`, plus `lookup` and `record` from Tasks 4–5); the builder's ComputeExpr bridge (`core/reports/lib/builder/formula.ts:36-139`) and its users in `draft.ts:42,249-252,339-368,395-401`; `describePath` reads only paths that end at a field (`lib/builder/columns.ts:47-73`).

**Files:**
- Create: `core/reports/lib/builder/formula-ops.ts`, `core/reports/lib/builder/__tests__/formula-ops.test.ts`
- Modify: `core/reports/lib/builder/columns.ts` (`describePath` reads a path that ends at a to-one link; `firstTextField`), `lib/builder/__tests__/columns.test.ts`

**Interfaces:**
- Consumes: `FormulaTree`, `treeAt` (Task 18).
- Produces: `REPORT_PARTS`, `type ReportPart`; `interface ReportFormulaContext { catalog; leaf: 'col' | 'subtotal'; values: FormulaValue[]; source: string | null; records: FormulaRecord[]; params: { id; label?; type }[]; totals: boolean; windows: boolean; lookups: boolean }`; `REPORT_TREE: FormulaTree<SpecNode>`; `partOf(node): ReportPart | 'unknown'`; `isEditableNode(node): boolean`; `contextAt(ctx, root, path)`; `partsAt(ctx, root, path)`; `partLabel(ctx, part)`; `withPart(ctx, root, path, part)`; `slotLabel(node, index)`; `isVariadicNode`; `canRemoveNode`; `addOperandNode`; `removeOperandNode`; `describeNode(ctx, node)`; `formulaType(node, types: FormulaTypes): FieldType`; `SCALED_PARTS`; `PathInfo.target?: CatalogSource` for a link-ending path; `firstTextField(source): CatalogField | undefined`.

- [ ] **Step 1: Write the failing tests**

Create `core/reports/lib/builder/__tests__/formula-ops.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import type { SpecNode } from '../../spec'
import {
    addOperandNode,
    contextAt,
    describeNode,
    formulaType,
    isEditableNode,
    partOf,
    partsAt,
    removeOperandNode,
    type ReportFormulaContext,
    slotLabel,
    withPart,
} from '../formula-ops'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

const CTX: ReportFormulaContext = {
    catalog: CATALOG,
    leaf: 'col',
    values: [
        { key: 'amount', label: 'Amount', type: 'money' },
        { key: 'cost', label: 'Cost', type: 'money' },
        { key: 'qty', label: 'Quantity', type: 'number' },
    ],
    source: SALES_KEY,
    records: [{ group: 'byTeam', label: 'Team', source: TEAMS_KEY }],
    params: [{ id: 'asOf', label: 'As of', type: 'asOf' }],
    totals: true,
    windows: true,
    lookups: true,
}
const LAYOUT: ReportFormulaContext = { ...CTX, leaf: 'subtotal', source: null, records: [], params: [], totals: false, windows: false, lookups: false }

const LARGER: SpecNode = { op: 'if', args: [{ op: 'gt', args: [{ col: 'amount' }, { col: 'cost' }] }, { col: 'amount' }, { col: 'cost' }] }
const DEBIT: SpecNode = {
    op: 'if',
    args: [
        { op: 'eq', args: [{ op: 'count', args: [{ path: [{ field: 'id' }] }] }, { value: 0 }] },
        { value: null },
        { op: 'if', args: [{ op: 'gte', args: [{ op: 'sum', args: [{ path: [{ field: 'amount' }] }] }, { value: 0 }] }, { op: 'sum', args: [{ path: [{ field: 'amount' }] }] }, { value: null }] },
    ],
}
const LOOKUP: SpecNode = { op: 'lookup', source: TEAMS_KEY, where: { op: 'eq', args: [{ path: [{ field: 'name' }] }, { value: 'North' }] }, value: { path: [{ field: 'id' }] }, pick: 'min' }

describe('which formulas the editor shows', () => {
    it('edits every report operator', () => {
        expect(isEditableNode(LARGER)).toBe(true)
        expect(isEditableNode(DEBIT)).toBe(true)
        expect(isEditableNode(LOOKUP)).toBe(true)
        expect(isEditableNode({ record: 'byTeam', path: [{ field: 'name' }] })).toBe(true)
        expect(isEditableNode({ op: 'concat', args: [{ col: 'amount' }, { value: ' · ' }] })).toBe(true)
    })
    it('keeps read-only what it cannot write', () => {
        expect(isEditableNode({ op: 'exists', link: [{ link: 'team' }] })).toBe(false)
        expect(isEditableNode({ op: 'div', args: [{ col: 'amount' }, { value: 2 }] })).toBe(false)
        expect(isEditableNode({ op: 'shout', args: [] })).toBe(false)
        expect(isEditableNode({ op: 'not', args: [{ value: true }, { value: false }] })).toBe(false)
    })
})

describe('changing a part', () => {
    it('keeps the forms part ids for the shared parts', () => {
        expect(partOf({ value: 0 })).toBe('number')
        expect(partOf({ col: 'amount' })).toBe('field')
        expect(partOf({ op: 'min', args: [{ col: 'amount' }] })).toBe('smallest')
        expect(partOf({ op: 'min', args: [{ col: 'amount' }, { col: 'cost' }] })).toBe('min')
    })
    it('carries operands over, wraps a part, and divides amounts by numbers in whole cents', () => {
        const sub = withPart(CTX, { value: 0 }, [], 'sub')
        expect(sub).toEqual({ op: 'sub', args: [{ value: 0 }, { value: 0 }] })
        expect(withPart(CTX, { col: 'amount' }, [], 'if')).toEqual({ op: 'if', args: [{ value: true }, { col: 'amount' }, { value: null }] })
        expect(withPart(CTX, { op: 'add', args: [{ col: 'amount' }, { col: 'qty' }] }, [], 'div')).toEqual({
            op: 'div',
            args: [{ col: 'amount' }, { col: 'qty' }],
            scale: 0,
        })
        expect(withPart(CTX, { op: 'add', args: [{ col: 'qty' }, { col: 'qty' }] }, [], 'div')).toMatchObject({ scale: 4 })
        expect(withPart(CTX, { value: 0 }, [], 'field')).toEqual({ col: 'amount' })
        expect(withPart(LAYOUT, { value: 0 }, [], 'field')).toEqual({ subtotal: 'amount' })
        expect(withPart(CTX, { value: 0 }, [], 'record')).toEqual({ record: 'byTeam', path: [{ field: 'name' }] })
        expect(withPart(CTX, { value: 0 }, [], 'lookup')).toMatchObject({ op: 'lookup', pick: 'min', where: { value: true } })
    })
    it('adds and removes a case pair before its otherwise', () => {
        const c: SpecNode = { op: 'case', args: [{ value: true }, { value: 1 }, { value: 2 }] }
        const added = addOperandNode(c)
        expect(added.args).toEqual([{ value: true }, { value: 1 }, { value: true }, { value: null }, { value: 2 }])
        expect(removeOperandNode(added, 2).args).toEqual(c.args)
        expect([0, 1, 2].map(i => slotLabel(c, i))).toEqual(['When', 'Then', 'Otherwise'])
        expect([0, 1].map(i => slotLabel(LOOKUP, i))).toEqual(['Where', 'Value'])
    })
})

describe('what a part offers where it sits', () => {
    it('offers no path, record, param, total or lookup in a layout row', () => {
        const parts = partsAt(LAYOUT, { value: 0 }, [])
        for (const p of ['path', 'record', 'param', 'sum', 'lookup', 'runningSum']) expect(parts).not.toContain(p)
        expect(parts).toContain('field')
    })
    it('reads the looked-up source inside a lookup, with no totals', () => {
        const inside = contextAt(CTX, LOOKUP, [1])
        expect(inside.source).toBe(TEAMS_KEY)
        expect(inside.totals).toBe(false)
        expect(contextAt(CTX, { op: 'sum', args: [{ col: 'qty' }] }, [0]).totals).toBe(false)
    })
})

describe('reading a formula back', () => {
    it('says it as a person would', () => {
        expect(describeNode(CTX, { op: 'sub', args: [{ col: 'amount' }, { col: 'cost' }] })).toBe('Amount − Cost')
        expect(describeNode(CTX, LARGER)).toBe('if Amount > Cost then Amount else Cost')
        expect(describeNode(CTX, LOOKUP)).toBe('look up ID in Stub teams where Name = "North" (smallest)')
        expect(describeNode(CTX, { record: 'byTeam', path: [{ field: 'name' }] })).toBe('Team › Name')
    })
    it('types it as the engine does', () => {
        const types = { catalog: CATALOG, source: SALES_KEY, col: (k: string) => CTX.values.find(v => v.key === k)?.type, records: CTX.records, params: CTX.params }
        expect(formulaType(LARGER, types)).toBe('money')
        expect(formulaType({ op: 'div', args: [{ col: 'amount' }, { col: 'qty' }], scale: 0 }, types)).toBe('money')
        expect(formulaType({ op: 'gt', args: [{ col: 'qty' }, { value: 1 }] }, types)).toBe('bool')
        expect(formulaType({ op: 'count', args: [{ path: [{ field: 'customer' }] }] }, types)).toBe('number')
        expect(formulaType({ op: 'concat', args: [{ value: 'a' }] }, types)).toBe('text')
        expect(formulaType(LOOKUP, types)).toBe('relation')
    })
})
```

In `columns.test.ts` add: `describePath(CATALOG, SALES_KEY, [{ link: 'team' }])` gives `{ field: { key: 'id', label: 'Team', type: 'relation' }, target: <teams source>, labels: ['Team'] }` and `firstTextField(<teams>)?.key` is `'name'`.

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/builder`
Expected: FAIL: cannot resolve `../formula-ops`.

- [ ] **Step 3: `columns.ts` — link-ending paths and the first text field**

In `PathInfo` add `/** The source a relation value points at (a path ending at a to-one link, or at id). */ target?: CatalogSource`. In `describePath`, after the loop (a path that ended on a link):

```ts
    const last = path[path.length - 1]
    if (last?.link && source && !viaToMany) {
        const linkLabel = labels[labels.length - 1] ?? source.label
        return { field: { key: 'id', label: linkLabel, type: 'relation', aggregates: ['count', 'countDistinct'] }, source, target: source, viaToMany, mode, labels }
    }
    return null
```

and in the field branch set `target: field.key === 'id' ? source : undefined`. Add:

```ts
/** The field a record is named by: its first text field other than id. */
export function firstTextField(source: CatalogSource): CatalogField | undefined {
    return source.fields.find(f => f.type === 'text' && f.key !== 'id')
}
```

- [ ] **Step 4: `formula-ops.ts`**

```ts
import type { FormulaPath } from '@tinycld/core/lib/forms/builder/formula'
import { type FormulaTree, treeAt } from '@tinycld/core/lib/forms/builder/formula-tree'
import { type ParamDef, type SpecHop, type SpecNode, specNodeSchema } from '../spec'
import type { FieldType } from '../types'
import { type Catalog, describePath, firstTextField, pathLabel } from './columns'
import { onlyKeys } from './formula'

// The report builder's formula parts (OQ2): every operator the engine compiles
// (core/server/reports/formula.go) except exists, which the filter editor owns.
// The parts the forms editor also has keep its ids (number, field, add, sub, mul,
// div, round, min, max), so their test ids stay (R16). A formula draft is the
// spec's own { op, args } tree (R17).

export const RATIO_SCALE = 4
const DEFAULT_PLACES = 2

const VALUE_PARTS = ['number', 'text', 'yesno', 'empty', 'list', 'field', 'path', 'record', 'param'] as const
const ARITHMETIC = ['add', 'sub', 'mul', 'div', 'round', 'min', 'max'] as const
const TOTALS = ['sum', 'count', 'countDistinct', 'avg', 'smallest', 'largest'] as const
const WINDOWS = ['runningSum', 'percentOfParent', 'percentOfTotal'] as const
const LOGIC = ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'between', 'in', 'and', 'or', 'not', 'isEmpty', 'if', 'case', 'coalesce'] as const
const TEXT = ['concat', 'upper', 'lower', 'contains', 'startsWith'] as const
const DATES = ['year', 'quarter', 'month', 'fiscalYear', 'fiscalPeriod', 'addDays', 'addMonths', 'diffDays'] as const
export const REPORT_PARTS = [...VALUE_PARTS, ...ARITHMETIC, ...TOTALS, ...WINDOWS, ...LOGIC, ...TEXT, ...DATES, 'lookup'] as const
export type ReportPart = (typeof REPORT_PARTS)[number]

const VALUE_SET: ReadonlySet<string> = new Set(VALUE_PARTS)
const TOTAL_SET: ReadonlySet<string> = new Set(TOTALS)
const WINDOW_SET: ReadonlySet<string> = new Set(WINDOWS)
export const SCALED_PARTS: ReadonlySet<string> = new Set(['div', 'round', 'avg', 'percentOfParent', 'percentOfTotal'])

const INF = Number.POSITIVE_INFINITY
type Arity = readonly [number, number]
const ARITY: Readonly<Record<string, Arity>> = {
    add: [2, INF], sub: [2, 2], mul: [2, INF], div: [2, 2], round: [1, 1], min: [2, INF], max: [2, INF],
    sum: [1, 1], count: [1, 1], countDistinct: [1, 1], avg: [1, 1], smallest: [1, 1], largest: [1, 1],
    runningSum: [1, 1], percentOfParent: [1, 1], percentOfTotal: [1, 1],
    eq: [2, 2], ne: [2, 2], lt: [2, 2], lte: [2, 2], gt: [2, 2], gte: [2, 2], between: [3, 3], in: [2, 2],
    and: [2, INF], or: [2, INF], not: [1, 1], isEmpty: [1, 1], if: [3, 3], case: [2, INF], coalesce: [1, INF],
    concat: [1, INF], upper: [1, 1], lower: [1, 1], contains: [2, 2], startsWith: [2, 2],
    year: [1, 1], quarter: [1, 1], month: [1, 1], fiscalYear: [1, 1], fiscalPeriod: [1, 1],
    addDays: [2, 2], addMonths: [2, 2], diffDays: [2, 2],
}

export const PART_LABELS: Record<ReportPart, string> = {
    number: 'Number', text: 'Text', yesno: 'Yes or no', empty: 'Empty', list: 'List of texts',
    field: 'Column', path: 'Field', record: 'Group field', param: 'Asked value',
    add: 'Add', sub: 'Subtract', mul: 'Multiply', div: 'Divide', round: 'Round', min: 'Smallest of', max: 'Largest of',
    sum: 'Sum of', count: 'Count of', countDistinct: 'Count distinct of', avg: 'Average of',
    smallest: 'Smallest value of', largest: 'Largest value of',
    runningSum: 'Running sum', percentOfParent: 'Percent of group', percentOfTotal: 'Percent of total',
    eq: 'Is equal', ne: 'Is not equal', lt: 'Is less than', lte: 'Is at most', gt: 'Is more than', gte: 'Is at least',
    between: 'Is between', in: 'Is any of', and: 'All of', or: 'Any of', not: 'Not', isEmpty: 'Is empty',
    if: 'If', case: 'First match', coalesce: 'First not empty',
    concat: 'Join text', upper: 'Upper case', lower: 'Lower case', contains: 'Contains', startsWith: 'Starts with',
    year: 'Year of', quarter: 'Quarter of', month: 'Month of', fiscalYear: 'Fiscal year of', fiscalPeriod: 'Fiscal period of',
    addDays: 'Add days', addMonths: 'Add months', diffDays: 'Days between', lookup: 'Look up',
}

export interface FormulaValue {
    key: string
    label: string
    type: FieldType
}
export interface FormulaRecord {
    group: string
    label: string
    source: string
}

/** What a formula may read where it is edited. */
export interface ReportFormulaContext {
    catalog: Catalog
    /** {col} in a column, label, sort or having; {subtotal} in a layout row. */
    leaf: 'col' | 'subtotal'
    values: readonly FormulaValue[]
    /** The source a {path} reads from; null where no path is read. */
    source: string | null
    records: readonly FormulaRecord[]
    params: readonly Pick<ParamDef, 'id' | 'label' | 'type'>[]
    totals: boolean
    windows: boolean
    lookups: boolean
}

export const isReportPart = (part: string): part is ReportPart => (REPORT_PARTS as readonly string[]).includes(part)

function lookupValueNode(node: SpecNode): SpecNode {
    const parsed = specNodeSchema.safeParse(node.value)
    return parsed.success ? parsed.data : { value: 0 }
}

export const REPORT_TREE: FormulaTree<SpecNode> = {
    childrenOf: node => (node.op === 'lookup' ? [node.where ?? { value: true }, lookupValueNode(node)] : node.op ? (node.args ?? []) : []),
    withChildren: (node, parts) => {
        if (node.op === 'lookup') return { ...node, where: parts[0], value: parts[1] }
        return node.op ? { ...node, args: parts } : node
    },
}

export function partOf(node: SpecNode): ReportPart | 'unknown' {
    if (node.record !== undefined) return 'record'
    if (node.col !== undefined || node.subtotal !== undefined) return 'field'
    if (node.path !== undefined) return 'path'
    if (node.param !== undefined) return 'param'
    if (node.op === undefined) {
        const v = node.value
        if (v === null) return 'empty'
        if (typeof v === 'number') return 'number'
        if (typeof v === 'string') return 'text'
        if (typeof v === 'boolean') return 'yesno'
        if (Array.isArray(v)) return 'list'
        return 'unknown'
    }
    if ((node.op === 'min' || node.op === 'max') && node.args?.length === 1) return node.op === 'min' ? 'smallest' : 'largest'
    return isReportPart(node.op) && !VALUE_SET.has(node.op) ? node.op : 'unknown'
}

const isFieldPath = (path: readonly SpecHop[]) =>
    path.length > 0 && path.every((hop, i) => (i === path.length - 1 ? Boolean(hop.field ?? hop.link) : Boolean(hop.link)))

/** The node has only shapes this editor writes; anything else stays read-only (R17). */
export function isEditableNode(node: SpecNode): boolean {
    const part = partOf(node)
    switch (part) {
        case 'unknown':
            return false
        case 'number':
        case 'text':
        case 'yesno':
        case 'empty':
            return onlyKeys(node, ['value'])
        case 'list':
            return onlyKeys(node, ['value']) && Array.isArray(node.value) && node.value.every(v => typeof v === 'string')
        case 'field':
            return node.col !== undefined ? onlyKeys(node, ['col']) : onlyKeys(node, ['subtotal'])
        case 'path':
            return onlyKeys(node, ['path']) && isFieldPath(node.path ?? [])
        case 'record':
            return onlyKeys(node, ['record', 'path']) && isFieldPath(node.path ?? [])
        case 'param':
            return onlyKeys(node, ['param'])
        case 'lookup':
            return (
                onlyKeys(node, ['op', 'source', 'where', 'value', 'pick']) &&
                typeof node.source === 'string' &&
                specNodeSchema.safeParse(node.value).success &&
                REPORT_TREE.childrenOf(node).every(isEditableNode)
            )
    }
    const [min, max] = ARITY[part] ?? [0, 0]
    const n = node.args?.length ?? 0
    const scaleOk = SCALED_PARTS.has(part) ? node.scale !== undefined && node.scale >= 0 && node.scale <= 6 : node.scale === undefined
    return onlyKeys(node, ['op', 'args', 'scale']) && n >= min && n <= max && scaleOk && (node.args ?? []).every(isEditableNode)
}

/** What may be read at `path`: inside a lookup its source and no totals; inside a total no total. */
export function contextAt(ctx: ReportFormulaContext, root: SpecNode, path: FormulaPath): ReportFormulaContext {
    let here = ctx
    let node: SpecNode | undefined = root
    for (const index of path) {
        if (!node) break
        const part = partOf(node)
        if (part === 'lookup') here = { ...here, source: node.source ?? here.source, totals: false, windows: false }
        else if (TOTAL_SET.has(part)) here = { ...here, totals: false, windows: false }
        else if (WINDOW_SET.has(part)) here = { ...here, windows: false }
        node = REPORT_TREE.childrenOf(node)[index]
    }
    return here
}

export function partsAt(ctx: ReportFormulaContext, root: SpecNode, path: FormulaPath): ReportPart[] {
    const here = contextAt(ctx, root, path)
    return REPORT_PARTS.filter(part => {
        if (part === 'field') return here.values.length > 0
        if (part === 'path') return here.source !== null
        if (part === 'record') return here.records.length > 0
        if (part === 'param') return here.params.length > 0
        if (part === 'lookup') return here.lookups && here.catalog.size > 0
        if (TOTAL_SET.has(part)) return here.totals
        if (WINDOW_SET.has(part)) return here.windows
        return true
    })
}

export function partLabel(ctx: ReportFormulaContext, part: string): string {
    if (part === 'field') return ctx.leaf === 'col' ? 'Column' : 'Section'
    return isReportPart(part) ? PART_LABELS[part] : part
}

function firstFieldOf(ctx: ReportFormulaContext, sourceKey: string | null): string | null {
    const source = sourceKey ? ctx.catalog.get(sourceKey) : undefined
    return source?.fields.find(f => f.key !== 'id')?.key ?? source?.fields[0]?.key ?? null
}

const FILLER: Partial<Record<ReportPart, SpecNode>> = { and: { value: true }, or: { value: true }, concat: { value: '' }, coalesce: { value: null } }

function seedOf(current: SpecNode): SpecNode[] {
    const part = partOf(current)
    return !VALUE_SET.has(part) && part !== 'lookup' && part !== 'unknown' ? REPORT_TREE.childrenOf(current) : [current]
}

/** The part at `path` changed to another kind; operands carry over where they can. */
export function withPart(ctx: ReportFormulaContext, root: SpecNode, path: FormulaPath, part: ReportPart): SpecNode {
    const current = treeAt(REPORT_TREE, root, path) ?? { value: 0 }
    if (partOf(current) === part) return current
    const here = contextAt(ctx, root, path)
    switch (part) {
        case 'number':
            return { value: 0 }
        case 'text':
            return { value: '' }
        case 'yesno':
            return { value: true }
        case 'empty':
            return { value: null }
        case 'list':
            return { value: [] }
        case 'field': {
            const first = here.values[0]
            if (!first) return current
            return here.leaf === 'col' ? { col: first.key } : { subtotal: first.key }
        }
        case 'path': {
            const field = firstFieldOf(here, here.source)
            return field ? { path: [{ field }] } : current
        }
        case 'record': {
            const record = here.records[0]
            const source = record ? here.catalog.get(record.source) : undefined
            const field = source ? (firstTextField(source)?.key ?? firstFieldOf(here, record?.source ?? null)) : null
            return record && field ? { record: record.group, path: [{ field }] } : current
        }
        case 'param': {
            const param = here.params[0]
            return param ? { param: param.id } : current
        }
        case 'lookup': {
            const source = here.catalog.keys().next().value
            return source ? { op: 'lookup', source, where: { value: true }, value: { path: [{ field: 'id' }] }, pick: 'min' } : current
        }
        case 'if':
            return { op: 'if', args: [{ value: true }, current, { value: null }] }
        case 'case':
            return { op: 'case', args: [{ value: true }, current] }
        case 'in':
            return { op: 'in', args: [current, { value: [] }] }
    }
    const [min, max] = ARITY[part] ?? [1, 1]
    const op = part === 'smallest' ? 'min' : part === 'largest' ? 'max' : part
    const seed = max === 1 ? [current] : seedOf(current)
    const filler = FILLER[part] ?? { value: 0 }
    const args = [...seed, ...Array.from({ length: Math.max(0, min - seed.length) }, () => filler)].slice(0, max)
    const node: SpecNode = { op, args }
    if (part === 'div') node.scale = divScale(here, args)
    else if (SCALED_PARTS.has(part)) node.scale = DEFAULT_PLACES
    return node
}

function typesOf(ctx: ReportFormulaContext): FormulaTypes {
    return { catalog: ctx.catalog, source: ctx.source, col: key => ctx.values.find(v => v.key === key)?.type, records: ctx.records, params: ctx.params }
}

// An amount divided by a number keeps whole cents (the engine refuses another scale).
function divScale(ctx: ReportFormulaContext, args: SpecNode[]): number {
    const [top, bottom] = args
    const types = typesOf(ctx)
    const isAmount = (n: SpecNode | undefined) => n !== undefined && formulaType(n, types) === 'money'
    return isAmount(top) && !isAmount(bottom) ? 0 : RATIO_SCALE
}

export function slotLabel(node: SpecNode, index: number): string | null {
    const n = node.args?.length ?? 0
    switch (partOf(node)) {
        case 'if':
            return ['When', 'Then', 'Otherwise'][index] ?? null
        case 'case':
            if (n % 2 === 1 && index === n - 1) return 'Otherwise'
            return index % 2 === 0 ? 'When' : 'Then'
        case 'lookup':
            return ['Where', 'Value'][index] ?? null
        case 'between':
            return ['Value', 'From', 'To'][index] ?? null
        case 'in':
            return ['Value', 'Is any of'][index] ?? null
        case 'addDays':
        case 'addMonths':
            return ['Date', 'By'][index] ?? null
        case 'diffDays':
            return ['From', 'To'][index] ?? null
    }
    return null
}

export function isVariadicNode(node: SpecNode): boolean {
    const part = partOf(node)
    return (ARITY[part]?.[1] ?? 0) === INF
}

export function canRemoveNode(node: SpecNode): boolean {
    const part = partOf(node)
    const n = node.args?.length ?? 0
    if (part === 'case') return n >= 4
    return isVariadicNode(node) && n > (ARITY[part]?.[0] ?? 0)
}

/** Add part: a case gains a When/Then pair before its Otherwise; others a 0 (a yes for All/Any of). */
export function addOperandNode(node: SpecNode): SpecNode {
    if (!isVariadicNode(node)) return node
    const args = node.args ?? []
    const part = partOf(node)
    if (part === 'case') {
        const pair: SpecNode[] = [{ value: true }, { value: null }]
        const hasElse = args.length % 2 === 1
        return { ...node, args: hasElse ? [...args.slice(0, -1), ...pair, ...args.slice(-1)] : [...args, ...pair] }
    }
    const filler = part === 'unknown' ? undefined : FILLER[part]
    return { ...node, args: [...args, filler ?? { value: 0 }] }
}

export function removeOperandNode(node: SpecNode, index: number): SpecNode {
    if (!canRemoveNode(node)) return node
    const args = node.args ?? []
    if (partOf(node) === 'case') {
        const start = index - (index % 2)
        return { ...node, args: args.filter((_, i) => i !== start && i !== start + 1) }
    }
    return { ...node, args: args.filter((_, i) => i !== index) }
}

const SIGNS: Partial<Record<string, string>> = { add: ' + ', sub: ' − ', mul: ' × ', div: ' ÷ ', eq: ' = ', ne: ' ≠ ', lt: ' < ', lte: ' ≤ ', gt: ' > ', gte: ' ≥ ' }
const WORDS: Partial<Record<string, string>> = {
    sum: 'sum of', count: 'count of', countDistinct: 'distinct count of', avg: 'average of', smallest: 'smallest', largest: 'largest',
    runningSum: 'running sum of', upper: 'upper case of', lower: 'lower case of', year: 'year of', quarter: 'quarter of',
    month: 'month of', fiscalYear: 'fiscal year of', fiscalPeriod: 'fiscal period of', not: 'not',
}

/** The formula as one line of text, by label: "Amount − Cost", "if Amount > Cost then Amount else Cost". */
export function describeNode(ctx: ReportFormulaContext, node: SpecNode): string {
    const say = (n: SpecNode, here: ReportFormulaContext, nested: boolean): string => {
        const part = partOf(n)
        const args = n.args ?? []
        const sub = (a: SpecNode | undefined) => (a ? say(a, here, true) : '?')
        const list = () => args.map(a => say(a, here, false)).join(', ')
        const wrap = (text: string) => (nested ? `(${text})` : text)
        switch (part) {
            case 'number':
                return String(n.value)
            case 'text':
                return JSON.stringify(n.value)
            case 'yesno':
                return n.value ? 'yes' : 'no'
            case 'empty':
                return 'empty'
            case 'list':
                return `[${(Array.isArray(n.value) ? n.value : []).map(v => JSON.stringify(v)).join(', ')}]`
            case 'field': {
                const key = n.col ?? n.subtotal ?? ''
                return here.values.find(v => v.key === key)?.label ?? `${key} (missing)`
            }
            case 'path': {
                const info = here.source ? describePath(here.catalog, here.source, n.path ?? []) : null
                return info ? pathLabel(info) : 'a field (missing)'
            }
            case 'record': {
                const record = here.records.find(r => r.group === n.record)
                const info = record ? describePath(here.catalog, record.source, n.path ?? []) : null
                return record && info ? `${record.label} › ${pathLabel(info)}` : `${n.record} (missing)`
            }
            case 'param':
                return `{${here.params.find(p => p.id === n.param)?.label ?? n.param}}`
            case 'lookup': {
                const inside = contextAt(here, n, [1])
                const source = here.catalog.get(n.source ?? '')?.label ?? `${n.source} (missing)`
                const where = n.where && !(n.where.value === true && onlyKeys(n.where, ['value'])) ? ` where ${say(n.where, inside, false)}` : ''
                return `look up ${say(lookupValueNode(n), inside, false)} in ${source}${where} (${n.pick === 'max' ? 'largest' : 'smallest'})`
            }
            case 'if':
                return wrap(`if ${say(args[0] ?? { value: true }, here, false)} then ${say(args[1] ?? { value: null }, here, false)} else ${say(args[2] ?? { value: null }, here, false)}`)
            case 'round':
                return `round(${say(args[0] ?? { value: 0 }, here, false)}, ${n.scale})`
            case 'min':
            case 'max':
                return `${part === 'min' ? 'smallest' : 'largest'} of (${list()})`
            case 'and':
            case 'or':
                return wrap(args.map(a => sub(a)).join(` ${part} `))
            case 'isEmpty':
                return wrap(`${sub(args[0])} is empty`)
            case 'between':
                return wrap(`${sub(args[0])} between ${sub(args[1])} and ${sub(args[2])}`)
            case 'in':
                return wrap(`${sub(args[0])} is any of ${sub(args[1])}`)
            case 'case':
                return `first match of (${list()})`
            case 'coalesce':
                return `first not empty of (${list()})`
            case 'concat':
                return args.map(a => sub(a)).join(' & ')
            case 'contains':
            case 'startsWith':
                return wrap(`${sub(args[0])} ${part === 'contains' ? 'contains' : 'starts with'} ${sub(args[1])}`)
            case 'addDays':
            case 'addMonths':
                return wrap(`${sub(args[0])} + ${sub(args[1])} ${part === 'addDays' ? 'days' : 'months'}`)
            case 'diffDays':
                return `days from ${sub(args[0])} to ${sub(args[1])}`
            case 'percentOfParent':
            case 'percentOfTotal':
                return `${sub(args[0])} as % of ${part === 'percentOfParent' ? 'group' : 'total'}`
        }
        const sign = SIGNS[part]
        if (sign) return wrap(args.map(a => sub(a)).join(sign))
        const word = WORDS[part]
        if (word) return `${word} ${sub(args[0])}`
        return 'a part the editor cannot show'
    }
    return say(node, ctx, false)
}

/** What a formula's type is read from. */
export interface FormulaTypes {
    catalog: Catalog
    source: string | null
    col: (key: string) => FieldType | undefined
    records: readonly FormulaRecord[]
    params: readonly Pick<ParamDef, 'id' | 'type'>[]
}

const PARAM_TYPES: Record<string, FieldType> = { date: 'date', asOf: 'date', dateRange: 'date', number: 'number', text: 'text', choice: 'text' }

/** The formula's result type, as the engine types it (formula.go operatorExpr). */
export function formulaType(node: SpecNode, types: FormulaTypes): FieldType {
    const t = (n: SpecNode | undefined) => (n ? formulaType(n, types) : 'number')
    const args = node.args ?? []
    const anyMoney = (ns: SpecNode[]) => ns.some(n => t(n) === 'money')
    const firstOf = (ns: SpecNode[]) => ns.map(n => t(n)).find(x => x !== 'text' || ns.length === 1) ?? 'text'
    switch (partOf(node)) {
        case 'number':
            return 'number'
        case 'text':
        case 'list':
            return 'text'
        case 'yesno':
            return 'bool'
        case 'empty':
            return 'number'
        case 'field':
            return types.col(node.col ?? node.subtotal ?? '') ?? 'number'
        case 'path':
            return (types.source ? describePath(types.catalog, types.source, node.path ?? [])?.field.type : undefined) ?? 'text'
        case 'record': {
            const record = types.records.find(r => r.group === node.record)
            return (record ? describePath(types.catalog, record.source, node.path ?? [])?.field.type : undefined) ?? 'text'
        }
        case 'param':
            return PARAM_TYPES[types.params.find(p => p.id === node.param)?.type ?? ''] ?? 'text'
        case 'lookup':
            return formulaType(lookupValueNode(node), { ...types, source: node.source ?? null })
        case 'add':
        case 'sub':
        case 'min':
        case 'max':
            return anyMoney(args) ? 'money' : 'number'
        case 'mul':
            return args.filter(a => t(a) === 'money').length === 1 ? 'money' : 'number'
        case 'div':
            return t(args[0]) === 'money' && t(args[1]) !== 'money' ? 'money' : 'number'
        case 'round':
        case 'sum':
        case 'avg':
        case 'smallest':
        case 'largest':
        case 'runningSum': {
            const inner = t(args[0])
            return inner === 'bool' ? 'number' : inner
        }
        case 'count':
        case 'countDistinct':
        case 'year':
        case 'quarter':
        case 'month':
        case 'fiscalYear':
        case 'fiscalPeriod':
        case 'diffDays':
            return 'number'
        case 'percentOfParent':
        case 'percentOfTotal':
            return 'percent'
        case 'addDays':
        case 'addMonths':
            return 'date'
        case 'concat':
        case 'upper':
        case 'lower':
            return 'text'
        case 'if':
            return anyMoney(args.slice(1)) ? 'money' : firstOf(args.slice(1).filter(a => partOf(a) !== 'empty'))
        case 'case': {
            const results = args.filter((_, i) => i % 2 === 1 || (args.length % 2 === 1 && i === args.length - 1))
            return anyMoney(results) ? 'money' : firstOf(results.filter(a => partOf(a) !== 'empty'))
        }
        case 'coalesce':
            return anyMoney(args) ? 'money' : firstOf(args.filter(a => partOf(a) !== 'empty'))
    }
    return 'bool'
}
```

- [ ] **Step 5: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/builder`
Expected: PASS (nothing uses the module yet; Task 20 moves the drafts and the editors onto it).

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check
git add core/reports/lib/builder
git commit -m "feat(core): the report formula operator set"
git push origin books/p5-trial-balance
```

---

### Task 20: OQ2 (part 3) — the report formula leaves; columns and layout rows use them; e2e

Code facts used: `ColumnFormula` and the layout row's formula (`ColumnsSection.tsx:145-170`, `LayoutSection.tsx:60-90`); `ColumnPicker` with `toOneOnly` (`ColumnPicker.tsx:79-205`); `PickMenu` (`@tinycld/core/ui/pick-menu`), `NumberText` (`core/components/forms-builder/rule-parts.tsx`), `PlainInput` (`@tinycld/core/ui/PlainInput`); the formula e2e ids (`tests/e2e/reports-formula-layout.spec.ts:96-104,61-67`); the shipped e2e (`tests/e2e/reports-shipped.spec.ts:51-79`).

**Files:**
- Create: `core/reports/components/builder/report-formula.tsx`, `core/reports/lib/builder/formula-context.ts`, `core/reports/lib/builder/__tests__/formula-context.test.ts`
- Modify: `core/reports/lib/builder/formula.ts` (keep `onlyKeys`; delete the ComputeExpr bridge), `lib/builder/draft.ts` (`FormulaDraft`, `formulaDraft`, `columnTypes`, `specColumns`, `specLayout`), `components/builder/parts.tsx` (`NEW_FORMULA`), `lib/builder/__tests__/formula.test.ts` (delete the bridge cases), `lib/builder/__tests__/draft.test.ts`
- Modify: `core/reports/components/builder/ColumnsSection.tsx`, `LayoutSection.tsx`
- Create: `tests/e2e/reports-formulas-v2.spec.ts`
- Modify: `tests/e2e/reports-shipped.spec.ts`

**Interfaces:**
- Consumes: Task 18 (`FormulaOperators`, `FormulaLeafProps`), Task 19 (formula-ops).
- Produces: `FormulaDraft = { kind: 'editable' | 'fixed'; node: SpecNode }`, `formulaDraft(node)`; `reportOperators(ctx): FormulaOperators<SpecNode>`; `formulaContext(draft, catalog, place): ReportFormulaContext` with `place: { kind: 'column'; selfKey: string } | { kind: 'layout'; index: number } | { kind: 'level' } | { kind: 'detail' }` (`level` = group label, having, sort formula; `detail` = row label); `recordGroups(draft, catalog): FormulaRecord[]`.

- [ ] **Step 1: Write the failing test of the formula context**

Create `core/reports/lib/builder/__tests__/formula-context.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { type BuilderDraft, emptyDraft } from '../draft'
import { formulaContext, recordGroups } from '../formula-context'
import { CATALOG, SALES_KEY, TEAMS_KEY } from './catalog-fixture'

function draft(): BuilderDraft {
    const base = emptyDraft(CATALOG.get(SALES_KEY)!)
    return {
        ...base,
        columns: [
            { kind: 'path', id: 'team', label: 'Team', path: [{ link: 'team' }], aggregate: '', scale: null, format: '' },
            { kind: 'path', id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum', scale: null, format: '' },
        ],
        groups: [{ id: 'byTeam', col: 'team', bucket: '', subtotals: false }],
        layout: [
            { kind: 'section', id: 'open', title: 'Open', key: 'open' },
            { kind: 'row', id: 'gap', label: 'Gap', formula: { kind: 'editable', node: { value: 0 } } },
        ],
    }
}

describe('what a formula reads where it is edited', () => {
    it('a group by a link carries its record', () => {
        expect(recordGroups(draft(), CATALOG)).toEqual([{ group: 'byTeam', label: 'Team', source: TEAMS_KEY }])
    })
    it('a column reads the other columns, paths, records, params and totals', () => {
        const ctx = formulaContext(draft(), CATALOG, { kind: 'column', selfKey: 'amount' })
        expect(ctx.values.map(v => v.key)).toEqual(['team'])
        expect(ctx).toMatchObject({ leaf: 'col', source: SALES_KEY, totals: true, windows: true, lookups: true })
        expect(ctx.records).toHaveLength(1)
    })
    it('a layout row reads only the items before it', () => {
        const ctx = formulaContext(draft(), CATALOG, { kind: 'layout', index: 1 })
        expect(ctx).toMatchObject({ leaf: 'subtotal', source: null, totals: false, lookups: false })
        expect(ctx.values).toEqual([{ key: 'open', label: 'Open', type: 'number' }])
    })
})
```

(The draft fixture uses the Task 21 group fields once they exist; Task 21 adds them to this fixture.)

- [ ] **Step 2: Write the failing e2e**

Create `tests/e2e/reports-formulas-v2.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    openReportStub,
    openReportsHub,
    SALES_SOURCE,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The report formula editor (OQ2): every report operator, edited from menus.
 * A fresh invited member owns the sales; teams are shared, so names are unique.
 */
test.describe('report formulas', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an if over a comparison picks the larger of amount and cost', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptif')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Cheap', amount: 300, cost: 900, day: '2026-01-05' })
            await addSale(p, { team, customer: 'Dear', amount: 1500, cost: 400, day: '2026-01-06' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await addColumn(p, ['cost'])
            await p.getByTestId('report-builder-add-formula').click()
            const f = 'report-builder-column-3-formula'
            await chooseOption(p, `${f}-root-part`, 'if')
            await expect(p.getByTestId(f)).toContainText('When')
            await chooseOption(p, `${f}-0-part`, 'gt')
            await chooseOption(p, `${f}-0-0-part`, 'field')
            await chooseOption(p, `${f}-0-0-field`, 'amount')
            await chooseOption(p, `${f}-0-1-part`, 'field')
            await chooseOption(p, `${f}-0-1-field`, 'cost')
            await chooseOption(p, `${f}-1-part`, 'field')
            await chooseOption(p, `${f}-1-field`, 'amount')
            await chooseOption(p, `${f}-2-part`, 'field')
            await chooseOption(p, `${f}-2-field`, 'cost')
            await expect(p.getByTestId(`${f}-summary`)).toHaveText('= if Amount > Cost then Amount else Cost')
            await expect(await cellOf(p, 'formula', 'Cheap')).toHaveText('9.00')
            await expect(await cellOf(p, 'formula', 'Dear')).toHaveText('15.00')
        } finally {
            await close()
        }
    })

    test('a lookup reads a team by name from another source', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptlookup')
        try {
            const team = uniqueName('Lookup team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Only', amount: 100, cost: 0, day: '2026-01-05' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-add-formula').click()
            const f = 'report-builder-column-1-formula'
            await chooseOption(p, `${f}-root-part`, 'lookup')
            await chooseOption(p, `${f}-root-source`, 'salesstub.teams')
            await chooseOption(p, `${f}-0-part`, 'eq')
            await chooseOption(p, `${f}-0-0-part`, 'path')
            await p.getByTestId(`${f}-0-0-path`).click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
            await chooseOption(p, `${f}-0-1-part`, 'text')
            await p.getByTestId(`${f}-0-1-text`).fill(team)
            await chooseOption(p, `${f}-1-part`, 'path')
            await p.getByTestId(`${f}-1-path`).click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
            await expect(p.getByTestId(`${f}-summary`)).toHaveText(`= look up Name in Stub teams where Name = "${team}" (smallest)`)
            await expect(await cellOf(p, 'formula', 'Only')).toHaveText(team)
        } finally {
            await close()
        }
    })
})
```

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-formulas-v2.spec.ts`
Expected: FAIL: the part menu has no `if` (`report-builder-column-3-formula-root-part-if` not found).

- [ ] **Step 3: Formula drafts are the spec tree**

In `formula.ts`, keep `onlyKeys` and delete everything else (`toComputeExpr`, `fromComputeExpr`, `isAmountExpr`, `formulaOperands`, `RATIO_SCALE`, `FormulaLeaf`). In `draft.ts`:

```ts
export const formulaDraftSchema = z.object({ kind: z.enum(['editable', 'fixed']), node: specNodeSchema })
export type FormulaDraft = z.infer<typeof formulaDraftSchema>

/** A stored formula: editable when the report formula editor can show it, else kept as it is (R17). */
export function formulaDraft(node: SpecNode): FormulaDraft {
    return { kind: isEditableNode(node) ? 'editable' : 'fixed', node }
}
```

Replace `formulaDraft(column.formula, 'col')` and `formulaDraft(item.formula ?? { value: 0 }, 'subtotal')` with `formulaDraft(…)`; `formulaNode(formula, …)` becomes `formula.node` in `specColumns` and `specLayout` (the `isAmount` arguments go). In `columnTypes`, a formula column's type is:

```ts
        const types: FormulaTypes = { catalog, source: draft.source, col: key => { const read = byId.get(key); return read ? typeOf(read) : undefined }, records: [], params: [] }
        return aggregateResultType(column.aggregate, formulaType(column.formula.node, types))
```

(record and param types come in Task 21, where the draft knows its record groups.) `parts.tsx`: `export const NEW_FORMULA: FormulaDraft = { kind: 'editable', node: { value: 0 } }`. In `draft.test.ts`, replace each `{ kind: 'editable', expr: … }` with the spec node it stands for and drop the ComputeExpr imports; the "keeps parts the builder cannot edit" case now uses an `exists` node (an `if` is editable).

- [ ] **Step 4: `formula-context.ts`**

```ts
import type { BuilderDraft } from './draft'
import { columnChoices } from './draft'
import { type Catalog, describePath } from './columns'
import type { FormulaRecord, ReportFormulaContext } from './formula-ops'

export type FormulaPlace =
    | { kind: 'column'; selfKey: string }
    | { kind: 'layout'; index: number }
    | { kind: 'level' }
    | { kind: 'detail' }

/** The group levels whose column is a link to one record: they carry that record. */
export function recordGroups(draft: Pick<BuilderDraft, 'source' | 'columns' | 'groups'>, catalog: Catalog): FormulaRecord[] {
    return draft.groups.flatMap(group => {
        const column = draft.columns.find(c => c.id === group.col)
        const info = column?.kind === 'path' ? describePath(catalog, draft.source, column.path) : null
        return info?.target && column ? [{ group: group.id, label: column.label, source: info.target.key }] : []
    })
}

export type FormulaDraftParts = Pick<BuilderDraft, 'source' | 'columns' | 'groups' | 'params' | 'layout'>

export function formulaContext(draft: FormulaDraftParts, catalog: Catalog, place: FormulaPlace): ReportFormulaContext {
    if (place.kind === 'layout') {
        const earlier = draft.layout.slice(0, place.index)
        return {
            catalog,
            leaf: 'subtotal',
            values: earlier.map(item => ({ key: item.id, label: item.kind === 'section' ? item.title : item.label, type: 'number' })),
            source: null,
            records: [],
            params: [],
            totals: false,
            windows: false,
            lookups: false,
        }
    }
    const selfKey = place.kind === 'column' ? place.selfKey : ''
    const values = columnChoices(draft, catalog)
        .filter(choice => choice.key !== selfKey && !choice.isNested)
        .map(choice => ({ key: choice.key, label: choice.label, type: choice.type }))
    return {
        catalog,
        leaf: 'col',
        values,
        source: draft.source,
        records: recordGroups(draft, catalog),
        params: draft.params.map(p => ({ id: p.id, label: p.label, type: p.type })),
        totals: true,
        windows: place.kind === 'column',
        lookups: true,
    }
}
```

- [ ] **Step 5: `report-formula.tsx` — the operator set with its leaves**

```tsx
import type { FormulaLeafProps, FormulaOperators } from '@tinycld/core/components/forms-builder/FormulaEditor'
import { FormulaNumberInput, FormulaPlacesPick, FormulaValuePick } from '@tinycld/core/components/forms-builder/FormulaEditor'
import { treeAt } from '@tinycld/core/lib/forms/builder/formula-tree'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { describePath, pathLabel } from '../../lib/builder/columns'
import {
    addOperandNode,
    canRemoveNode,
    contextAt,
    describeNode,
    isReportPart,
    isVariadicNode,
    partLabel,
    partOf,
    partsAt,
    REPORT_TREE,
    type ReportFormulaContext,
    removeOperandNode,
    SCALED_PARTS,
    slotLabel,
    withPart,
} from '../../lib/builder/formula-ops'
import { type SpecHop, type SpecNode, specNodeSchema } from '../../lib/spec'
import { ColumnPicker } from './ColumnPicker'

const INPUT_CLASS = 'min-w-24 flex-1 rounded-md border border-border px-2 py-2 text-sm text-foreground'

type LeafProps = FormulaLeafProps<SpecNode> & { ctx: ReportFormulaContext }

function NumberLeaf({ expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'number') return null
    return <FormulaNumberInput value={Number(expr.value)} onChange={n => set({ value: n })} label={`Number${where}`} testID={`${id}-number`} />
}

function TextLeaf({ expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'text') return null
    return <PlainInput value={String(expr.value)} onChangeText={text => set({ value: text })} accessibilityLabel={`Text${where}`} testID={`${id}-text`} className={INPUT_CLASS} />
}

function ListLeaf({ expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'list') return null
    const text = (Array.isArray(expr.value) ? expr.value : []).join(', ')
    const onChange = (next: string) => set({ value: next.split(',').map(s => s.trim()).filter(s => s !== '') })
    return <PlainInput value={text} onChangeText={onChange} accessibilityLabel={`Texts, separated by commas${where}`} testID={`${id}-list`} className={INPUT_CLASS} />
}

function YesNoLeaf({ expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'yesno') return null
    const items: PickItem[] = [true, false].map(v => ({ id: String(v), label: v ? 'Yes' : 'No', isSelected: expr.value === v, onSelect: () => set({ value: v }) }))
    return <PickMenu label={expr.value ? 'Yes' : 'No'} purpose={`Yes or no${where}`} items={items} testID={`${id}-yesno`} />
}

function ValueLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'field') return null
    const here = contextAt(ctx, root, path)
    const key = expr.col ?? expr.subtotal ?? ''
    const choose = (picked: string) => set(here.leaf === 'col' ? { col: picked } : { subtotal: picked })
    return <FormulaValuePick choices={here.values} selected={key} onChange={choose} purpose={`${partLabel(here, 'field')}${where}`} testID={`${id}-field`} />
}

function PathLeaf({ ctx, root, path, expr, set, id }: LeafProps) {
    if (partOf(expr) !== 'path') return null
    const here = contextAt(ctx, root, path)
    if (!here.source) return null
    const info = describePath(here.catalog, here.source, expr.path ?? [])
    const label = info ? pathLabel(info) : 'Choose a field'
    return (
        <ColumnPicker
            catalog={here.catalog}
            baseKey={here.source}
            onPick={(next: SpecHop[]) => set({ path: next })}
            label={label}
            testID={`${id}-path`}
            toOneOnly
        />
    )
}

function RecordLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'record') return null
    const here = contextAt(ctx, root, path)
    const record = here.records.find(r => r.group === expr.record)
    const groups: PickItem[] = here.records.map(r => ({
        id: r.group,
        label: r.label,
        isSelected: r.group === expr.record,
        onSelect: () => set({ record: r.group, path: expr.path ?? [] }),
    }))
    const info = record ? describePath(here.catalog, record.source, expr.path ?? []) : null
    const fieldLabel = info ? pathLabel(info) : 'Choose a field'
    const groupLabel = record?.label ?? `${expr.record} (missing)`
    return (
        <>
            <PickMenu label={groupLabel} purpose={`Group${where}`} items={groups} testID={`${id}-record-group`} />
            <RecordField isVisible={record !== undefined} catalogCtx={here} source={record?.source ?? ''} label={fieldLabel} onPick={next => set({ record: expr.record, path: next })} testID={`${id}-record-field`} />
        </>
    )
}

function RecordField({ isVisible, catalogCtx, source, label, onPick, testID }: { isVisible: boolean; catalogCtx: ReportFormulaContext; source: string; label: string; onPick: (path: SpecHop[]) => void; testID: string }) {
    if (!isVisible) return null
    return <ColumnPicker catalog={catalogCtx.catalog} baseKey={source} onPick={onPick} label={label} testID={testID} toOneOnly />
}

function ParamLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'param') return null
    const here = contextAt(ctx, root, path)
    const items: PickItem[] = here.params.map(p => ({ id: p.id, label: p.label ?? p.id, isSelected: p.id === expr.param, onSelect: () => set({ param: p.id }) }))
    const label = here.params.find(p => p.id === expr.param)?.label ?? `${expr.param} (missing)`
    return <PickMenu label={label} purpose={`Asked value${where}`} items={items} testID={`${id}-param`} />
}

function PlacesLeaf({ expr, set, id, where }: LeafProps) {
    if (!SCALED_PARTS.has(partOf(expr))) return null
    return <FormulaPlacesPick places={expr.scale ?? 0} onChange={n => set({ ...expr, scale: n })} purpose={`Decimal places${where}`} testID={`${id}-places`} />
}

function LookupLeaf({ ctx, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'lookup') return null
    const sources: PickItem[] = [...ctx.catalog.values()].map(s => ({
        id: s.key,
        label: s.label,
        isSelected: s.key === expr.source,
        // Another source reads other fields: its where and value start again.
        onSelect: () => set({ op: 'lookup', source: s.key, where: { value: true }, value: { path: [{ field: 'id' }] }, pick: expr.pick ?? 'min' }),
    }))
    const picks: PickItem[] = (['min', 'max'] as const).map(p => ({ id: p, label: p === 'min' ? 'Smallest' : 'Largest', isSelected: (expr.pick ?? 'min') === p, onSelect: () => set({ ...expr, pick: p }) }))
    return (
        <>
            <PickMenu label={ctx.catalog.get(expr.source ?? '')?.label ?? 'Choose a source'} purpose={`Source${where}`} items={sources} testID={`${id}-source`} />
            <PickMenu label={(expr.pick ?? 'min') === 'min' ? 'Smallest' : 'Largest'} purpose={`Pick${where}`} items={picks} testID={`${id}-pick`} />
        </>
    )
}

function ReportFormulaLeaf(props: LeafProps) {
    return (
        <>
            <NumberLeaf {...props} />
            <TextLeaf {...props} />
            <ListLeaf {...props} />
            <YesNoLeaf {...props} />
            <ValueLeaf {...props} />
            <PathLeaf {...props} />
            <RecordLeaf {...props} />
            <ParamLeaf {...props} />
            <PlacesLeaf {...props} />
            <LookupLeaf {...props} />
        </>
    )
}

function parseNode(value: unknown): SpecNode {
    const parsed = specNodeSchema.safeParse(value)
    return parsed.success ? parsed.data : { value: 0 }
}

/**
 * The formula context of a place in the builder, read with useWatch so the
 * menus follow edits made in other sections (F27); never getValues in render.
 */
export function useFormulaContext(builder: ReportBuilder, place: FormulaPlace): ReportFormulaContext {
    const control = builder.form.control
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const groups = useWatch({ control, name: 'groups' })
    const params = useWatch({ control, name: 'params' })
    const layout = useWatch({ control, name: 'layout' })
    return formulaContext({ source, columns, groups, params, layout }, builder.catalog, place)
}

/** The report builder's formula parts for the FormulaEditor's operators prop (OQ2). */
export function reportOperators(ctx: ReportFormulaContext): FormulaOperators<SpecNode> {
    return {
        ...REPORT_TREE,
        parse: parseNode,
        partOf,
        partsAt: (root, path) => partsAt(ctx, root, path),
        label: part => partLabel(ctx, part),
        withPart: (root, path, part) => (isReportPart(part) ? withPart(ctx, root, path, part) : (treeAt(REPORT_TREE, root, path) ?? root)),
        slotLabel,
        isVariadic: isVariadicNode,
        canRemove: canRemoveNode,
        addOperand: addOperandNode,
        removeOperand: removeOperandNode,
        describe: node => describeNode(ctx, node),
        renderLeaf: props => <ReportFormulaLeaf {...props} ctx={ctx} />,
    }
}
```

- [ ] **Step 6: Wire the column and layout formulas**

`ColumnsSection.tsx` `ColumnFormula`: replace the choices/`formulaOperands` lines and the editor with (the context is read with `useWatch`, F27; the hook is called before the early returns):

```tsx
    const ctx = useFormulaContext(builder, { kind: 'column', selfKey: columns[index]?.id ?? '' })
    return (
        <FormulaEditor
            operators={reportOperators(ctx)}
            value={column.formula.node}
            onChange={node => setFormula({ kind: 'editable', node })}
            testID={testID}
        />
    )
```

`LayoutSection.tsx` does the same with `useFormulaContext(builder, { kind: 'layout', index })` and `value={item.formula.node}`. Remove the now unused `formulaOperands`/`columnChoices` imports.

- [ ] **Step 7: The shipped report's `if` is now editable**

In `tests/e2e/reports-shipped.spec.ts`, replace the read-only assertions (lines 51-55) with:

```ts
        // A version 1 shipped report runs with no "upgraded" notice (R1, F25).
        await expect(page.getByTestId('report-notice-upgraded')).toHaveCount(0)
        // The copy's builder edits the if formula (OQ2): its parts are menus.
        await pressToolbar(page, 'report-edit-open', 'Edit report')
        await expect(page.getByTestId('report-builder')).toBeVisible()
        const formula = 'report-builder-column-3-formula'
        await expect(page.getByTestId(`${formula}-root-part`)).toHaveText('If')
        await expect(page.getByTestId(`${formula}-summary`)).toHaveText('= if Amount > Cost then Amount else Cost')
```

and its header comment: "Its formula uses `if`, which the builder edits (OQ2); a rename saves it unchanged."

- [ ] **Step 8: Run**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-formulas-v2.spec.ts tests/e2e/reports-formula-layout.spec.ts tests/e2e/reports-shipped.spec.ts tests/e2e/forms-builder.spec.ts`
Expected: all pass (the existing formula and layout specs keep their ids; forms keep today's editor).

- [ ] **Step 9: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-formulas-v2.spec.ts tests/e2e/reports-shipped.spec.ts
git commit -m "feat(core): the report builder edits every report operator"
git push origin books/p5-trial-balance
```

---

### Task 21: E12 (builder) — the draft keeps every v2 field; aggregate and relation conditions; kept params

Code facts used: the draft and its conversions (`core/reports/lib/builder/draft.ts:59-500`); the filter draft, its node conversion and params (`lib/builder/filter.ts:32-612`); `askParamFor` (`filter.ts:165-182`); the builder hook's setters (`hooks/useReportBuilder.ts:177-191`); `FilterSection`'s change (`components/builder/FilterSection.tsx:580-590`).

**Files:**
- Modify: `core/reports/lib/builder/filter.ts` (condition subjects, `LevelScope`, relation ops and asks, amount asks and two-param between, `above` conditions, `paramsInNode` reads lookups)
- Modify: `core/reports/lib/builder/__tests__/catalog-fixture.ts` (the teams source gains `region`, a select north/south)
- Modify: `core/reports/lib/builder/draft.ts` (group, sort, column, detail and display fields; `keptParams`; `draftFromSpec`/`specFromDraft`)
- Modify: `core/reports/lib/builder/errors.ts` (`detail` → columns, `display` → arrange)
- Modify: `core/reports/hooks/useReportBuilder.ts` (setters keep params; `setDetailLabel`, `setDisplay`)
- Modify: `core/reports/components/builder/FilterSection.tsx:580-590` (`setFilter(next, added)`)
- Test: `lib/builder/__tests__/draft.test.ts`, `filter.test.ts`, `filter-tree.test.ts`, `errors.test.ts`, `formula-context.test.ts`

**Interfaces:**
- Consumes: Task 19/20 (`formulaDraft`, `formulaType`, `recordGroups`).
- Produces: `ConditionDraft.subject: 'field' | 'total' | 'column' | 'record'`, `.aggregate: Aggregate | ''`, `.col: string`, `.record: string`; `type LevelScope = { colType(key: string): FieldType | undefined; records: FormulaRecord[] }`; `nodeToFilter(node, catalog, baseKey, level?)`, `filterToNode(group, catalog, baseKey, level?)`, `conditionProblems(group, catalog, baseKey, level?)`; `askParamFor(field, op, id, target?)`; `askHighParamFor(field, op, id): ParamDef | null`; `ConditionDraft.paramHigh: string`, `ConditionDraft.above: string`; `type AboveGroup = { id: string; col: string; label: string }`; `aboveOf(groups, index, columns): AboveGroup[]`; `nodeToFilter`/`filterToNode`/`conditionProblems` take a fifth argument `above?: readonly AboveGroup[]`; `ColumnDraft.labelField: string`; `GroupLevelDraft = { id; col; bucket; subtotals; open: boolean; label: FormulaDraft | null; having: GroupDraft; fill: { on: boolean; where: GroupDraft } }`; `SortDraft = { kind: 'col'; col; dir } | { kind: 'formula'; formula: FormulaDraft; dir }`; `ColumnDraft.total: 'recompute' | 'rows'`; `BuilderDraft.detailLabel: FormulaDraft | null`; `BuilderDraft.display: { layouts: DisplayLayout[] }`; `keptParams(draft, added?): ParamDef[]`; `levelScope(draft, catalog): LevelScope`; `groupTarget(draft, catalog, group): CatalogSource | undefined`; builder setters `setColumns(items)`, `setGroups(items, added?)`, `setSort(items, added?)`, `setFilter(filter, added?)`, `setDetailLabel(label)`, `setDisplay(display)`.

- [ ] **Step 1: Write the failing round-trip test**

In `draft.test.ts` add a Trial-Balance-shaped spec over the stub catalog, written in the canonical shape `specFromDraft` writes, and assert it survives the draft unchanged:

```ts
const V2: ReportSpec = {
    version: 2,
    source: SALES_KEY,
    params: [
        { id: 'asOf', type: 'asOf', label: 'As of', default: { preset: 'endOfLastMonth' } },
        { id: 'rows', type: 'choice', label: 'Rows', default: 'nonzero', options: [{ value: 'nonzero', label: 'Non-zero' }, { value: 'all', label: 'All' }] },
        { id: 'min', type: 'number', label: 'Quantity' },
        { id: 'teams', type: 'choice', label: 'Teams', multiple: true, optionsFrom: { source: TEAMS_KEY, value: [{ field: 'id' }], label: [{ field: 'name' }] } },
    ],
    columns: [
        { id: 'team', label: 'Team', path: [{ link: 'team' }] },
        { id: 'name', label: 'Name', formula: { record: 'byTeam', path: [{ field: 'name' }] } },
        { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum', total: 'rows' },
        { id: 'quantity', label: 'Quantity', path: [{ field: 'quantity' }], aggregate: 'sum' },
        {
            id: 'north',
            label: 'North',
            formula: { op: 'lookup', source: TEAMS_KEY, where: { op: 'eq', args: [{ path: [{ field: 'name' }] }, { value: 'North' }] }, value: { path: [{ field: 'id' }] }, pick: 'min' },
        },
    ],
    filter: {
        op: 'and',
        args: [
            { op: 'lte', args: [{ op: 'addDays', args: [{ path: [{ field: 'day' }] }, { value: 0 }] }, { param: 'asOf' }] },
            { op: 'in', args: [{ path: [{ link: 'team' }] }, { param: 'teams' }] },
        ],
    },
    sort: [{ formula: { record: 'byTeam', path: [{ field: 'name' }] }, dir: 'desc' }, { col: 'amount' }],
    groups: [
        {
            id: 'byTeam',
            col: 'team',
            label: { record: 'byTeam', path: [{ field: 'name' }] },
            having: {
                op: 'and',
                args: [
                    { op: 'gte', args: [{ col: 'quantity' }, { param: 'min' }] },
                    { op: 'or', args: [{ op: 'eq', args: [{ param: 'rows' }, { value: 'all' }] }, { op: 'ne', args: [{ op: 'sum', args: [{ path: [{ field: 'amount' }] }] }, { value: 0 }] }] },
                ],
            },
            fill: { where: { op: 'startsWith', args: [{ path: [{ field: 'name' }] }, { value: 'A' }] } },
            open: true,
        },
    ],
    detail: { label: { op: 'concat', args: [{ path: [{ field: 'customer' }] }, { value: ' · ' }, { path: [{ field: 'day' }] }] } },
    display: { layouts: [{ width: 'mobile', tracks: 3, cells: [{ col: 'team', line: 1, span: 3 }, { col: 'amount', line: 2, justify: 'end' }] }] },
}

describe('spec version 2 in the builder', () => {
    it('keeps every v2 part through the draft', () => {
        const draft = draftFromSpec(V2, { title: 'T', description: '' }, CATALOG)
        expect(builderSchema(CATALOG).safeParse(draft).success).toBe(true)
        expect(specFromDraft(draft, CATALOG)).toEqual(V2)
    })
    it('reads an aggregate condition and a relation condition as editable, and the rest as fixed', () => {
        const draft = draftFromSpec(V2, { title: 'T', description: '' }, CATALOG)
        const [column, fixed] = draft.groups[0]?.having.items ?? []
        expect(column).toMatchObject({ kind: 'condition', subject: 'column', col: 'quantity', op: 'gte', param: 'min' })
        // or(...) reads back as an Any-of group: the param test is fixed, the total a condition (F8).
        expect(fixed).toMatchObject({ kind: 'group', match: 'any', items: [{ kind: 'fixed' }, { kind: 'condition', subject: 'total', aggregate: 'sum', op: 'ne' }] })
        expect(draft.filter.items[1]).toMatchObject({ kind: 'condition', subject: 'field', path: [{ link: 'team' }], op: 'in', param: 'teams' })
        expect(draft.groups[0]?.fill).toMatchObject({ on: true, where: { items: [{ kind: 'condition', op: 'startsWith' }] } })
        expect(draft.sort[0]).toMatchObject({ kind: 'formula', dir: 'desc' })
        expect(draft.columns[2]).toMatchObject({ total: 'rows' })
    })
    it('drops a param only when nothing reads it any more', () => {
        const draft = draftFromSpec(V2, { title: 'T', description: '' }, CATALOG)
        const noHaving = { ...draft, groups: draft.groups.map(g => ({ ...g, having: emptyGroup() })) }
        expect(keptParams(noHaving).map(p => p.id)).toEqual(['asOf', 'teams'])
    })
})
```

In `filter.test.ts` add: `askParamFor(<team relation field>, 'in', 'team', <teams source>)` gives `{ id: 'team', type: 'choice', label: 'Team', multiple: true, optionsFrom: { source: TEAMS_KEY, value: [{ field: 'id' }], label: [{ field: 'name' }] } }`; `opsFor('relation')` is `['eq', 'ne', 'in', 'isEmpty', 'notEmpty']`; a level condition `{ subject: 'total', aggregate: 'sum', path: [{ field: 'amount' }], op: 'gt', value: '10.00' }` writes `{ op: 'gt', args: [{ op: 'sum', args: [{ path: [{ field: 'amount' }] }] }, { value: 1000 }] }` and reads back to the same draft. In `errors.test.ts`: `sectionOf('detail.label')` is `'columns'` and `sectionOf('display.layouts.0.cells.1')` is `'arrange'`.

Also in `filter.test.ts` (F18, F21):

```ts
describe('asked amounts and ranges', () => {
    const amount = { key: 'amount', label: 'Amount', type: 'money' as const, aggregates: ['sum' as const] }
    const quantity = { key: 'quantity', label: 'Quantity', type: 'number' as const, aggregates: [] }
    it('asks a money value as an amount param', () => {
        expect(askParamFor(amount, 'gte', 'amount')).toEqual({ id: 'amount', type: 'amount', label: 'Amount' })
    })
    it('asks a range of numbers or amounts as two params', () => {
        expect(askParamFor(quantity, 'between', 'quantity')).toEqual({ id: 'quantity', type: 'number', label: 'Quantity from' })
        expect(askHighParamFor(quantity, 'between', 'quantity')).toEqual({ id: 'quantityTo', type: 'number', label: 'Quantity to' })
        expect(askHighParamFor(amount, 'between', 'amount')).toEqual({ id: 'amountTo', type: 'amount', label: 'Amount to' })
        expect(askHighParamFor(quantity, 'gte', 'quantity')).toBeNull()
    })
    it('writes and reads a two-param between', () => {
        const node = { op: 'between', args: [{ path: [{ field: 'quantity' }] }, { param: 'quantity' }, { param: 'quantityTo' }] }
        const group = nodeToFilter(node, CATALOG, SALES_KEY)
        expect(group.items[0]).toMatchObject({ kind: 'condition', op: 'between', param: 'quantity', paramHigh: 'quantityTo' })
        expect(filterToNode(group, CATALOG, SALES_KEY)).toEqual(node)
        expect([...paramsInGroup(group, new Set())]).toEqual(['quantity', 'quantityTo'])
    })
})

describe('a fill condition that matches the group above', () => {
    const above = [{ id: 'g0', col: 'region', label: 'Region' }]
    it('reads {col} of an enclosing group as "same as the group above" and writes it back', () => {
        const node = { op: 'eq', args: [{ path: [{ field: 'region' }] }, { col: 'region' }] }
        const group = nodeToFilter(node, CATALOG, TEAMS_KEY, undefined, above)
        expect(group.items[0]).toMatchObject({ kind: 'condition', op: 'eq', above: 'g0', param: '', value: '' })
        expect(filterToNode(group, CATALOG, TEAMS_KEY, undefined, above)).toEqual(node)
        expect(conditionProblems(group, CATALOG, TEAMS_KEY, undefined, above)).toEqual([])
    })
    it('reads a {col} that is no enclosing group as a fixed part', () => {
        const node = { op: 'eq', args: [{ path: [{ field: 'region' }] }, { col: 'other' }] }
        expect(nodeToFilter(node, CATALOG, TEAMS_KEY, undefined, above).items[0]).toMatchObject({ kind: 'fixed' })
    })
})
```

In `draft.test.ts`, a spec whose second group (`g1`, by a team record column) has `fill: { where: { op: 'eq', args: [{ path: [{ field: 'region' }] }, { col: 'region' }] } }` under a first group `g0` by a `region` column round-trips through `draftFromSpec`/`specFromDraft` unchanged, its fill condition reading `above: 'g0'`; a column `{ id: 'team', path: [{ link: 'team' }], labelField: 'region' }` reads `labelField: 'region'` and writes it back, and one without `labelField` reads `''` and writes none (F22).

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/builder`
Expected: FAIL (`keptParams` not exported; the v2 parts are dropped).

- [ ] **Step 3: `filter.ts` — subjects, level scope, relations**

```ts
/** What a condition compares (R18): a field of the row; on a group level also a total of a field, a column, or a field of the group's record. */
export const CONDITION_SUBJECTS = ['field', 'total', 'column', 'record'] as const
export type ConditionSubject = (typeof CONDITION_SUBJECTS)[number]

export type ConditionDraft = {
    kind: 'condition'
    subject: ConditionSubject
    /** The field (subject field or total), or the record's field (subject record). */
    path: SpecHop[]
    aggregate: Aggregate | ''
    col: string
    record: string
    op: ConditionOp
    value: string
    high: string
    values: string[]
    param: string
}

/** A group level's conditions read its columns and its group records. */
export type LevelScope = { colType: (key: string) => FieldType | undefined; records: readonly FormulaRecord[] }
```

`conditionSchema` gains `subject: z.enum(CONDITION_SUBJECTS)`, `aggregate: z.enum(['', ...AGGREGATES])`, `col: z.string()`, `record: z.string()`, and `path` becomes `z.array(specHopSchema)` (a column subject has no path). `emptyCondition(path)` sets `subject: 'field', aggregate: '', col: '', record: ''`. `OPS_BY_TYPE.relation` becomes `['eq', 'ne', 'in', 'isEmpty', 'notEmpty']`.

The subject's node and type:

```ts
function subjectNode(c: ConditionDraft): SpecNode {
    switch (c.subject) {
        case 'total':
            return { op: c.aggregate || 'count', args: [{ path: c.path }] }
        case 'column':
            return { col: c.col }
        case 'record':
            return { record: c.record, path: c.path }
    }
    return { path: c.path }
}

function subjectType(c: ConditionDraft, catalog: Catalog, baseKey: string, level?: LevelScope): FieldType | null {
    switch (c.subject) {
        case 'column':
            return level?.colType(c.col) ?? null
        case 'record': {
            const record = level?.records.find(r => r.group === c.record)
            return record ? (describePath(catalog, record.source, c.path)?.field.type ?? null) : null
        }
        case 'total': {
            const type = fieldType(catalog, baseKey, c.path)
            return type && (c.aggregate === 'count' || c.aggregate === 'countDistinct' || c.aggregate === '') ? 'number' : type
        }
    }
    return fieldType(catalog, baseKey, c.path)
}
```

`conditionNode(c, type)` starts from `const target = subjectNode(c)` (the day form `addDays(target, 0)` applies to a `field` or `record` subject of a date type); `itemNode` uses `subjectType(item, catalog, baseKey, level)`; `groupNode`, `filterToNode`, `nodeToFilter`, `itemFrom`, `conditionFrom` and `conditionProblems` take the optional `level` and pass it down. Reading back (`subjectOf`):

```ts
function subjectOf(node: SpecNode | undefined, catalog: Catalog, baseKey: string, level?: LevelScope): Subject | null {
    if (level && node?.col !== undefined && onlyKeys(node, ['col'])) {
        const type = level.colType(node.col)
        return type ? { kind: 'column', raw: [], day: null, path: [], col: node.col, type } : null
    }
    if (level && node?.record !== undefined && onlyKeys(node, ['record', 'path'])) {
        const record = level.records.find(r => r.group === node.record)
        const type = record ? describePath(catalog, record.source, node.path ?? [])?.field.type : undefined
        return type ? { kind: 'record', raw: node.path ?? [], day: null, path: node.path ?? [], record: node.record, type } : null
    }
    const total = level && node?.op && AGGREGATE_OPS.has(node.op) && onlyKeys(node, ['op', 'args']) && node.args?.length === 1 ? plainPath(node.args[0]) : null
    if (total && node?.op) {
        const field = fieldType(catalog, baseKey, total)
        const type = node.op === 'count' || node.op === 'countDistinct' ? 'number' : field
        return type ? { kind: 'total', raw: total, day: null, path: total, aggregate: node.op, type } : null
    }
    const raw = plainPath(node)
    const day = dayPath(node)
    const path = raw ?? day
    const type = path ? fieldType(catalog, baseKey, path) : null
    return path && type ? { kind: 'field', raw, day, path, type } : null
}
```

with `Subject` gaining `kind`, `col?`, `record?`, `aggregate?` and `condition(...)` copying them into the draft (`AGGREGATE_OPS = new Set(['sum', 'count', 'countDistinct', 'avg', 'min', 'max'])`; one-argument `min`/`max` are totals). `askParamFor` and its partner for the high end of a range (F18, R29):

```ts
/** The param an asked value becomes, or null when the field cannot be asked (R16). */
export function askParamFor(field: CatalogField, op: ConditionOp, id: string, target?: CatalogSource): ParamDef | null {
    const label = field.label
    if (!needsValue(op)) return null
    if (isDayType(field.type)) {
        if (op === 'between') return { id, type: 'dateRange', label }
        if (op === 'lte') return { id, type: 'asOf', label }
        return op === 'in' ? null : { id, type: 'date', label }
    }
    const ranged = rangedType(field.type)
    if (ranged) return { id, type: ranged, label: op === 'between' ? `${label} from` : label }
    // A record is picked from the target's records, read under the reader's rules (R19).
    if (field.type === 'relation' && target && (op === 'eq' || op === 'ne' || op === 'in')) {
        const shown = firstTextField(target)?.key ?? 'id'
        return { id, type: 'choice', label, multiple: op === 'in', optionsFrom: { source: target.key, value: [{ field: 'id' }], label: [{ field: shown }] } }
    }
    if (field.type === 'text' || field.type === 'relation') return { id, type: 'text', label }
    if (field.type === 'select') {
        const options = field.options ?? []
        if (op === 'in') return { id, type: 'choice', label, multiple: true, options }
        return { id, type: 'choice', label, options }
    }
    return null
}

/** The second param a range of numbers or amounts asks: its high end. */
export function askHighParamFor(field: CatalogField, op: ConditionOp, id: string): ParamDef | null {
    const ranged = rangedType(field.type)
    return op === 'between' && ranged ? { id: `${id}To`, type: ranged, label: `${field.label} to` } : null
}

// A number asks a number; money asks an amount, kept to its currency (R29).
function rangedType(type: FieldType): 'number' | 'amount' | null {
    if (type === 'number') return 'number'
    return type === 'money' ? 'amount' : null
}
```

`ConditionDraft` gains `paramHigh: string` (the high end's param, '' when none; schema `z.string()`) and `above: string` (an enclosing group's id, '' for none; schema `z.string()`); `emptyCondition` sets both to `''`. A fill condition under other groups ties a record to its parent with `above` (F21; the builder's "Same as the <group> above" in Task 22 writes it):

```ts
/** A group that encloses a level: a fill condition there can read its key ({col}). */
export type AboveGroup = { id: string; col: string; label: string }

/** The groups above level index, labelled by their columns. */
export function aboveOf(groups: readonly { id: string; col: string }[], index: number, columns: readonly { id: string; label: string }[]): AboveGroup[] {
    return groups.slice(0, index).map(g => ({ id: g.id, col: g.col, label: columns.find(c => c.id === g.col)?.label ?? g.col }))
}
```

`nodeToFilter`, `filterToNode` and `conditionProblems` take `above: readonly AboveGroup[] = []` after `level` and pass it down. In `conditionNode(c, type, above)` the value leaf becomes:

```ts
    const group = c.above ? above.find(g => g.id === c.above) : undefined
    const leaf = (text: string): SpecNode => {
        if (group) return { col: group.col }
        return c.param ? { param: c.param } : { value: literal(type, text) }
    }
```

and the `between` case, before the date-range form:

```ts
            if (c.param && c.paramHigh) return { op: 'between', args: [target, { param: c.param }, { param: c.paramHigh }] }
```

Reading: `Leaf` gains `| { above: string }`; `leafOf(node, type, above)` returns `{ above: g.id }` for a node `{ col }` (its only key) whose `col` is an enclosing group's column, and `fromLeaf` copies it to `above`; `betweenCondition` reads three-argument `between` whose two ends are param leaves as `{ param, paramHigh }`. `paramsInGroup` adds `item.paramHigh` when set; `valueProblems` returns null when `c.above` is set (the value is the group's key). Every caller in `draft.ts` passes the enclosing groups to a fill (below); filter and having pass none.

`catalog-fixture.ts`: the teams source's fields gain `{ key: 'region', label: 'Region', type: 'select', aggregates: [], options: [{ value: 'north', label: 'North' }, { value: 'south', label: 'South' }] }`.

`paramsInNode` also walks a lookup's `value` (parsed with `specNodeSchema`), so a param a lookup reads stays.

- [ ] **Step 4: `draft.ts` — the v2 fields**

```ts
const SORT_DIR = z.enum(['asc', 'desc'])
export const sortDraftSchema = z.union([
    z.object({ kind: z.literal('col'), col: z.string().min(1, 'Choose a column'), dir: SORT_DIR }),
    z.object({ kind: z.literal('formula'), formula: formulaDraftSchema, dir: SORT_DIR }),
])
export type SortDraft = z.infer<typeof sortDraftSchema>

export const groupLevelSchema = z.object({
    id: idSchema,
    col: z.string().min(1, 'Choose a column'),
    bucket: z.enum(BUCKET_CHOICES),
    subtotals: z.boolean(),
    open: z.boolean(),
    /** null: the key, or a record group's first text field, labels the row (R9). */
    label: formulaDraftSchema.nullable(),
    having: filterGroupSchema,
    fill: z.object({ on: z.boolean(), where: filterGroupSchema }),
})
export type GroupLevelDraft = z.infer<typeof groupLevelSchema>
```

`columnFields` gains `total: z.enum(['recompute', 'rows'])` and `labelField: z.string()` ('' shows the target's first text field, R31; `draftFromSpec` reads `c.labelField ?? ''`, `specFromDraft` writes it only when set); `draftShape`: `groups: z.array(groupLevelSchema).max(MAX_GROUP_LEVELS)`, `sort: z.array(sortDraftSchema).max(MAX_SORT_KEYS)`, `detailLabel: formulaDraftSchema.nullable()`, `display: z.object({ layouts: z.array(displayLayoutSchema).max(DISPLAY_WIDTHS.length) })`. `emptyDraft` sets `detailLabel: null`, `display: { layouts: [] }`, and its column `total: 'recompute', labelField: ''`; `ColumnsSection`'s new columns set `total: 'recompute', labelField: ''`; `GroupSortSection`'s new level sets `open: false, label: null, having: emptyGroup(), fill: { on: false, where: emptyGroup() }` and a new sort key `{ kind: 'col', col, dir: 'asc' }`.

```ts
/** The source a group's column links to (a record group), or undefined. */
export function groupTarget(draft: Pick<BuilderDraft, 'source' | 'columns'>, catalog: Catalog, group: { col: string }): CatalogSource | undefined {
    const column = draft.columns.find(c => c.id === group.col)
    return column?.kind === 'path' ? describePath(catalog, draft.source, column.path)?.target : undefined
}

/** What a group level's conditions read: the columns' types and the record groups. */
export function levelScope(draft: Pick<BuilderDraft, 'source' | 'columns' | 'groups'>, catalog: Catalog): LevelScope {
    const types = columnTypes(draft, catalog)
    return { colType: key => types.get(key), records: recordGroups(draft, catalog) }
}

/** The params something in the draft still reads, plus a newly asked one (R20). */
export function keptParams(draft: BuilderDraft, added?: ParamDef): ParamDef[] {
    const read = paramsInGroup(draft.filter, new Set<string>())
    const node = (n: SpecNode | undefined) => paramsInNode(n, read)
    for (const column of draft.columns) if (column.kind === 'formula') node(column.formula.node)
    for (const group of draft.groups) {
        paramsInGroup(group.having, read)
        if (group.fill.on) paramsInGroup(group.fill.where, read)
        node(group.label?.node)
    }
    for (const key of draft.sort) if (key.kind === 'formula') node(key.formula.node)
    node(draft.detailLabel?.node)
    const kept = draft.params.filter(p => read.has(p.id))
    return added ? [...kept, added] : kept
}
```

`draftFromSpec` — columns gain `total: column.total ?? 'recompute'`; then, with `const base = { source: spec.source, columns, groups: basicGroups }` (groups with only `id`, `col`), `const level = levelScope(base, catalog)`:

```ts
        groups: (spec.groups ?? []).map((g, index) => {
            const target = groupTarget(base, catalog, g)
            return {
                id: g.id,
                col: g.col,
                bucket: g.bucket ?? '',
                subtotals: g.subtotals === true,
                open: g.open === true,
                label: g.label ? formulaDraft(g.label) : null,
                having: nodeToFilter(g.having, catalog, spec.source, level),
                // A fill on a group that is not by a link (refused by the server)
                // reads against the base source and shows as fixed parts.
                fill: { on: g.fill !== undefined, where: nodeToFilter(g.fill?.where, catalog, target?.key ?? spec.source, undefined, aboveOf(spec.groups ?? [], index, spec.columns)) },
            }
        }),
        sort: (spec.sort ?? []).map(s =>
            s.formula
                ? { kind: 'formula' as const, formula: formulaDraft(s.formula), dir: s.dir ?? 'asc' }
                : { kind: 'col' as const, col: s.col ?? '', dir: s.dir ?? 'asc' }
        ),
        detailLabel: spec.detail?.label ? formulaDraft(spec.detail.label) : null,
        display: { layouts: spec.display?.layouts ?? [] },
```

`specFromDraft` — `params` becomes `keptParams(draft)` (drop `askedParams` and `paramsReadByColumns`); columns write `total: 'rows'` only when set; then:

```ts
    if (draft.sort.length > 0) {
        spec.sort = draft.sort.map(s => {
            const dir = s.dir === 'desc' ? { dir: 'desc' as const } : {}
            return s.kind === 'col' ? { col: s.col, ...dir } : { formula: s.formula.node, ...dir }
        })
    }
    if (draft.groups.length > 0) {
        const level = levelScope(draft, catalog)
        spec.groups = draft.groups.map((g, index) => {
            const having = filterToNode(g.having, catalog, draft.source, level)
            const target = groupTarget(draft, catalog, g)
            const fillWhere = g.fill.on ? filterToNode(g.fill.where, catalog, target?.key ?? draft.source, undefined, aboveOf(draft.groups, index, draft.columns)) : undefined
            return {
                id: g.id,
                col: g.col,
                ...(g.bucket !== '' ? { bucket: g.bucket } : {}),
                ...(g.subtotals ? { subtotals: true } : {}),
                ...(g.label ? { label: g.label.node } : {}),
                ...(having ? { having } : {}),
                ...(g.fill.on ? { fill: fillWhere ? { where: fillWhere } : {} } : {}),
                ...(g.open ? { open: true } : {}),
            }
        })
    }
    if (draft.detailLabel) spec.detail = { label: draft.detailLabel.node }
    if (draft.display.layouts.length > 0) spec.display = { layouts: draft.display.layouts }
```

(Key order inside each object follows the schema, so `toEqual` holds.) `groupingIssues` checks `sort` keys of kind `col` only; `draftIssues` also reports `conditionProblems` of each group's having (`['groups', i, 'having']`) and fill (`['groups', i, 'fill']`), and every display layout `placeCells` refuses (`['display', 'layouts', i]`, Task 24's helper; until Task 24 lands, this check is added there). `columnTypes` gains record and param types through `formulaType(node, { …, records: recordGroups(draft, catalog), params: draft.params })` when the draft has them (`draft` is typed `Pick<BuilderDraft, 'source' | 'columns'> & Partial<Pick<BuilderDraft, 'groups' | 'params'>>`).

`errors.ts`: `BuilderSection` gains `'arrange'`; `sectionOf`: `if (head === 'columns' || head === 'detail') return 'columns'`, `if (head === 'display') return 'arrange'`; `PART_NAMES.display = 'Arrangement'`.

- [ ] **Step 5: The builder hook keeps params on every change**

In `useReportBuilder.ts` replace the setters:

```ts
    // Every change that can drop the last reader of a param recomputes the params
    // in the same edit (R20).
    const keep = (patch: Partial<BuilderDraft>, added?: ParamDef) =>
        form.setValue('params', keptParams({ ...form.getValues(), ...patch }, added), { shouldDirty: true })
```

and return `setColumns: items => { keep({ columns: items }); form.setValue('columns', items, LIST_CHANGE) }`, the same for `setGroups(items, added?)`, `setSort(items, added?)`, `setFilter(filter, added?)`, plus `setDetailLabel: (label: FormulaDraft | null) => { keep({ detailLabel: label }); form.setValue('detailLabel', label, LIST_CHANGE) }` and `setDisplay: (display: BuilderDraft['display']) => form.setValue('display', display, LIST_CHANGE)`. In `FilterSection.tsx`, `change: (next, added) => builder.setFilter(next, added)` (the `params`/`columns` watches and `filterParams` import go).

- [ ] **Step 6: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: PASS (biome, tsc, every vitest).

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports
git commit -m "feat(core): the report builder keeps every spec v2 part"
git push origin books/p5-trial-balance
```

---

### Task 22: E2, E3, E4, E5, E7, E13 in the builder — Columns, Group and sort, the condition tree; e2e

Code facts used: the group and sort cards (`components/builder/GroupSortSection.tsx:59-245`), the column card (`ColumnsSection.tsx:70-170`), the filter editor and its context (`FilterSection.tsx:55-599`), `ColumnPicker` (`ColumnPicker.tsx:92-205`), `FormulaEditor` with `operators` (Task 18), `reportOperators`/`formulaContext` (Task 20), builder setters (Task 21).

**Files:**
- Create: `core/reports/components/builder/ConditionTree.tsx` (the filter editor body, moved out of `FilterSection.tsx` and parameterized)
- Modify: `core/reports/components/builder/FilterSection.tsx` (uses `ConditionTree`), `ColumnPicker.tsx` (`allowLinkEnd`), `GroupSortSection.tsx`, `ColumnsSection.tsx`
- Modify: `core/reports/lib/builder/columns.ts` (`linkTarget`), test `lib/builder/__tests__/columns.test.ts` (the `above`, `paramHigh` and `labelField` draft fields are Task 21's, in `filter.ts` and `draft.ts`)
- Create: `tests/e2e/reports-groups-v2.spec.ts`

**Interfaces:**
- Consumes: Tasks 18–21.
- Produces: `ConditionTree({ builder, root, onChange(next, added?), baseKey, testID, level? })`; `ColumnPicker` prop `allowLinkEnd?: boolean` (offers a to-one link itself as "<Link> (record)", test id `report-builder-pick-record-<key>`); test ids `report-builder-column-<i>-total`, `report-builder-detail-label-on`, `report-builder-detail-label`, `report-builder-group-<i>-label`, `report-builder-group-<i>-fill-on`, `report-builder-group-<i>-fill…` (its condition tree), `report-builder-group-<i>-having…`, `report-builder-group-<i>-having-add-total|column|record`, `report-builder-group-<i>-open`, `report-builder-sort-<i>-col` (options: column ids, `record.<group>.<field>`, `formula`).

- [ ] **Step 1: Write the failing e2e**

Create `tests/e2e/reports-groups-v2.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { appShell, createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Group levels in the builder: group by a linked team (its record labels the row),
 * show every team even with no rows, sort by a team field, hide groups with an
 * asked condition, total only the rows shown, open groups when the report opens,
 * and label opened rows. A fresh invited member owns the sales; teams are shared,
 * so every team name starts with a stem unique to the test and the fill condition
 * keeps only those.
 */

async function seedTeams(p: Page, stem: string) {
    await openReportStub(p)
    await addTeam(p, `${stem} A`)
    await addTeam(p, `${stem} B`)
    await addTeam(p, `${stem} C`)
    await addSale(p, { team: `${stem} A`, customer: 'Acme', amount: 1000, cost: 0, quantity: 5, day: '2026-01-05' })
    await addSale(p, { team: `${stem} B`, customer: 'Bolt', amount: 300, cost: 0, quantity: 2, day: '2026-01-06' })
}

async function groupByTeam(p: Page, stem: string) {
    await startReport(p, SALES_SOURCE)
    await p.getByTestId('report-builder-add-column').click()
    await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-record-team').click()
    await addColumn(p, ['amount'])
    await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
    await p.getByTestId('report-builder-section-group').click()
    await p.getByTestId('report-builder-group-add').click()
    await chooseOption(p, 'report-builder-group-0-col', 'team')
    // Only this test's teams: the fill keeps teams whose name starts with the stem.
    await p.getByTestId('report-builder-group-0-fill-on').click()
    await p.getByTestId('report-builder-group-0-fill-add-condition').click()
    await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
    await chooseOption(p, 'report-builder-group-0-fill-0-op', 'startsWith')
    await p.getByTestId('report-builder-group-0-fill-0-value').fill(stem)
    await p.getByTestId('report-builder-section-filter').click()
    await p.getByTestId('report-builder-filter-add-condition').click()
    await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-link-team').click()
    await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
    await chooseOption(p, 'report-builder-filter-0-op', 'startsWith')
    await p.getByTestId('report-builder-filter-0-value').fill(stem)
}

test.describe('group levels', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a group by a linked team shows every team, labelled and sorted by a team field', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptrec')
        try {
            const stem = uniqueName('Rec')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            await chooseOption(p, 'report-builder-sort-add-menu', 'record.level.name')
            await p.getByTestId('report-builder-sort-0').getByRole('radio', { name: 'Z to A' }).click()
            const rows = p.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')
            await expect(rows).toHaveText([`${stem} C`, `${stem} B`, `${stem} A`])
            await expect(await cellOf(p, 'amount', `${stem} C`)).toHaveText('0.00')
            // A team with no sales has nothing to open.
            await expect(rowOf(p, `${stem} C`)).not.toHaveAttribute('aria-expanded')
            await expect(rowOf(p, `${stem} A`)).toHaveAttribute('aria-expanded', 'false')
        } finally {
            await close()
        }
    })

    test('an asked group condition hides teams and the total adds only the rows shown', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rpthaving')
        try {
            const stem = uniqueName('Hav')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            await p.getByTestId('report-builder-section-columns').click()
            await chooseOption(p, 'report-builder-column-2-total', 'rows')
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            await addColumn(p, ['quantity'])
            await chooseOption(p, 'report-builder-column-4-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await chooseOption(p, 'report-builder-group-0-having-add-column', 'quantity')
            await chooseOption(p, 'report-builder-group-0-having-0-op', 'gte')
            await p.getByTestId('report-builder-group-0-having-0-ask').click()
            const title = uniqueName('Having')
            await saveNewReport(p, title)
            await p.getByTestId('report-add-filter').click()
            await p.getByTestId('report-add-filter-quantity').click()
            await p.getByTestId('report-filter-value').fill('3')
            await p.getByTestId('report-filter-apply').click()
            const rows = p.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')
            await expect(rows).toHaveText([`${stem} A`])
            const totalRow = p.getByTestId('report-table').locator('[data-testid^="report-row-total"]')
            const totalId = (await totalRow.getAttribute('data-testid'))?.replace('report-row-', '')
            await expect(p.getByTestId(`report-cell-amount-${totalId}`)).toHaveText('$10.00')
            await expect(p.getByTestId(`report-cell-amount2-${totalId}`)).toHaveText('$13.00')
        } finally {
            await close()
        }
    })

    test('groups open when the report opens, Collapse all keeps them closed through a live re-run, and opened rows carry their label', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptopen')
        try {
            const stem = uniqueName('Open')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await groupByTeam(p, stem)
            await p.getByTestId('report-builder-group-0-open').click()
            await p.getByTestId('report-builder-section-columns').click()
            await p.getByTestId('report-builder-detail-label-on').click()
            const f = 'report-builder-detail-label'
            await chooseOption(p, `${f}-root-part`, 'concat')
            await chooseOption(p, `${f}-0-part`, 'path')
            await p.getByTestId(`${f}-0-path`).click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-customer').click()
            await p.getByTestId(`${f}-root-add`).click()
            await chooseOption(p, `${f}-1-part`, 'text')
            await p.getByTestId(`${f}-1-text`).fill(' at ')
            await p.getByTestId(`${f}-root-add`).click()
            await chooseOption(p, `${f}-2-part`, 'path')
            await p.getByTestId(`${f}-2-path`).click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-day').click()
            const title = uniqueName('Open groups')
            await saveNewReport(p, title)
            await expect(rowOf(p, 'Acme at 2026-01-05')).toBeVisible()
            await expect(rowOf(p, 'Bolt at 2026-01-06')).toBeVisible()
            await pressToolbar(p, 'report-fold-all', 'Collapse all')
            await expect(rowOf(p, 'Acme at 2026-01-05')).toBeHidden()
            const other = await p.context().newPage()
            try {
                // A new page loads the app first (the initial load login also makes).
                await other.goto('/')
                await appShell(other).waitFor({ state: 'visible' })
                await openReportStub(other)
                await addSale(other, { team: `${stem} C`, customer: 'Cole', amount: 50, cost: 0, day: '2026-01-07' })
            } finally {
                await other.close()
            }
            await expect(await cellOf(p, 'amount', `${stem} C`)).toHaveText('0.50')
            await expect(rowOf(p, 'Cole at 2026-01-07')).toBeHidden()
        } finally {
            await close()
        }
    })

    test('on a phone a group by a linked team is set up in the builder sheet', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptrecph')
        try {
            const stem = uniqueName('RecPh')
            await seedTeams(p, stem)
            await openReportsHub(p)
            await p.setViewportSize({ width: 390, height: 844 })
            await groupByTeam(p, stem)
            await p.getByTestId('report-builder-mode-preview').click()
            const rows = p.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')
            await expect(rows).toHaveCount(3)
            await expect(rowOf(p, `${stem} C`)).toBeVisible()
        } finally {
            await close()
        }
    })
})
```

(Money cells show the currency symbol only on subtotal, computed and total rows (`lib/format.ts:95`), so group cells read `0.00` and the total `$10.00`. `chooseOption(p, 'report-builder-sort-add-menu', …)` opens the Add sort menu, which lists the same keys as a sort row.)

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-groups-v2.spec.ts`
Expected: FAIL: `report-builder-pick-record-team` not found.

- [ ] **Step 2: `ColumnPicker` offers a to-one link itself**

Add `allowLinkEnd?: boolean` to `ColumnPickerProps` and `OpenPickerProps` (passed through `PickerBody`), and in `OpenPicker`:

```tsx
    // A link to one record can be a value of its own: the record (E4), which a
    // group level labels by the record's fields.
    const recordLinks = allowLinkEnd && !pending ? offered.filter(link => link.cardinality === 'toOne') : []
    const records = recordLinks.map(link => (
        <PickRow
            key={`record-${link.key}`}
            label={`${link.label} (record)`}
            testID={`report-builder-pick-record-${link.key}`}
            onPress={() => onPick([...hops, { link: link.key }])}
        />
    ))
```

rendered after `{fields}`. `ColumnsSection`'s Add column picker and `ConditionTree`'s Add condition picker pass `allowLinkEnd`.

- [ ] **Step 3: `ConditionTree.tsx`**

Move everything in `FilterSection.tsx` from `filterTestId` through `GroupBody` into `ConditionTree.tsx`, with these changes:

```tsx
export interface ConditionTreeProps {
    builder: ReportBuilder
    root: GroupDraft
    onChange(next: GroupDraft, added?: ParamDef): void
    /** The source the conditions' fields are on. */
    baseKey: string
    /** The test id stem: report-builder-filter, report-builder-group-0-having, … */
    testID: string
    /** On a group level the conditions also read totals, columns and group fields (R18). */
    level?: LevelScope & { columns: { key: string; label: string }[] }
}

interface FilterContext {
    builder: ReportBuilder
    catalog: Catalog
    root: GroupDraft
    params: ParamDef[]
    testID: string
    level?: ConditionTreeProps['level']
    change(next: GroupDraft, added?: ParamDef): void
}

function itemTestId(ctx: FilterContext, path: FilterPath): string {
    return path.length === 0 ? ctx.testID : `${ctx.testID}-${path.join('-')}`
}

export function ConditionTree({ builder, root, onChange, baseKey, testID, level }: ConditionTreeProps) {
    const params = useWatch({ control: builder.form.control, name: 'params' })
    const ctx: FilterContext = { builder, catalog: builder.catalog, root, params, testID, level, change: onChange }
    return (
        <View testID={testID}>
            <GroupBody ctx={ctx} group={root} path={[]} baseKey={baseKey} />
        </View>
    )
}
```

Every `filterTestId(path)` becomes `itemTestId(ctx, path)`. `ConditionRow` reads its field info by subject:

```tsx
function conditionInfo(ctx: FilterContext, item: ConditionDraft, baseKey: string): { label: string; type: FieldType; field: CatalogField | null; target?: CatalogSource } | null {
    if (item.subject === 'column') {
        const col = ctx.level?.columns.find(c => c.key === item.col)
        const type = ctx.level?.colType(item.col)
        return col && type ? { label: col.label, type, field: null } : null
    }
    const record = item.subject === 'record' ? ctx.level?.records.find(r => r.group === item.record) : undefined
    const info = describePath(ctx.catalog, record?.source ?? baseKey, item.path)
    if (!info) return null
    const counts = item.subject === 'total' && (item.aggregate === 'count' || item.aggregate === 'countDistinct')
    const label = item.subject === 'total' ? `${AGGREGATE_LABELS[item.aggregate || 'count']} of ${pathLabel(info)}` : record ? `${record.label} › ${pathLabel(info)}` : pathLabel(info)
    return { label, type: counts ? 'number' : info.field.type, field: counts ? null : info.field, target: info.target }
}
```

`AskToggle` moves into `ConditionTree` and asks both ends of a range of numbers or amounts (F18, R29); `setCondition(ctx, path, patch, ...added: ParamDef[])` keeps every added param:

```tsx
    const onChange = (on: boolean) => {
        if (!on) {
            setCondition(ctx, path, { param: '', paramHigh: '' })
            return
        }
        const taken = ctx.params.map(p => p.id)
        const id = nextColumnId(taken, columnHint(item.path) || item.col)
        const low = askParamFor(info.field, item.op, id, info.target)
        if (!low) return
        const high = askHighParamFor(info.field, item.op, nextColumnId([...taken, id], `${id}To`))
        if (high) setCondition(ctx, path, { param: low.id, paramHigh: high.id }, low, high)
        else setCondition(ctx, path, { param: low.id }, low)
    }
```

(`isFull` counts two free param slots for a range.) A money total in a having (`subject: 'total'`, aggregate `sum` of an amount) asks an amount param, so a having on a money total can be asked (R18 changed). `ConditionValue`, `AskToggle` and the operator menu take this info (a column or a count asks like a number: `askParamFor({ key: item.col, label, type, aggregates: [] }, op, id)`; a relation passes `target`). A column subject's asked param takes the column id as its id hint (`columnHint(item.path) || item.col`), so a having on Quantity asks `quantity`. A relation condition's typed value is chosen with a `RecordValuePicker` (Task 23 builds it; until then the value is the text box, which takes a record id). For a `total` subject an extra menu picks the aggregate (`${id}-aggregate`: Sum, Count, Smallest, Largest, Average as the field offers). In level mode, `GroupBody`'s add row offers three adders instead of "Add condition":

```tsx
function LevelAdders({ ctx, path, baseKey }: { ctx: FilterContext; path: FilterPath; baseKey: string }) {
    const level = ctx.level
    if (!level) return null
    const id = itemTestId(ctx, path)
    const add = (item: ConditionDraft) => ctx.change(addFilterItem(ctx.root, path, item))
    const columnItems: PickItem[] = level.columns.map(c => ({
        id: c.key,
        label: c.label,
        onSelect: () => add({ ...emptyCondition([]), subject: 'column', col: c.key, op: opsFor(level.colType(c.key) ?? 'number')[0] ?? 'eq' }),
    }))
    const onTotal = (next: SpecHop[]) => {
        const info = describePath(ctx.catalog, baseKey, next)
        const aggregate = info && (info.field.type === 'money' || info.field.type === 'number') ? 'sum' : 'count'
        add({ ...emptyCondition(next), subject: 'total', aggregate, op: 'gt' })
    }
    const record = level.records[0]
    return (
        <>
            <ColumnPicker catalog={ctx.catalog} baseKey={baseKey} onPick={onTotal} label="Add total condition" testID={`${id}-add-total`} toOneOnly />
            <PickMenu label="Add column condition" purpose="Column" items={columnItems} testID={`${id}-add-column`} />
            <RecordAdder isVisible={record !== undefined} ctx={ctx} path={path} record={record} />
        </>
    )
}
```

(`RecordAdder` is a `ColumnPicker` over `record.source` adding `{ subject: 'record', record: record.group, path }`, test id `${id}-add-record`, returning null when not visible.) `GroupBody` shows `LevelAdders` instead of the row adders when `ctx.level` is set. `FilterSection` becomes:

```tsx
export function FilterSection({ builder, problems }: { builder: ReportBuilder; problems: Problem[] }) {
    const root = useWatch({ control: builder.form.control, name: 'filter' })
    const error = builder.form.getFieldState('filter', builder.form.formState).error?.message
    return (
        <BuilderSection title="Filter" section="filter" problems={problems}>
            <FilterError message={error} />
            <ConditionTree builder={builder} root={root} onChange={builder.setFilter} baseKey={builder.base.key} testID="report-builder-filter" />
        </BuilderSection>
    )
}
```

The existing filter e2e ids (`report-builder-filter-…`) are unchanged.

- [ ] **Step 4: Group levels and sort keys**

In `GroupSortSection.tsx`, `GroupRow` gains, after the Subtotals toggle:

```tsx
            <GroupLabel builder={builder} groups={groups} index={index} target={target} />
            <GroupFill builder={builder} groups={groups} index={index} target={target} />
            <GroupHaving builder={builder} groups={groups} index={index} />
            <Toggle name={`groups.${index}.open`} control={control} label="Open when the report opens" testID={`${testID}-open`} />
```

with `const target = groupTarget({ source: builder.base.key, columns }, builder.catalog, group)` (columns from `useWatch`). The parts:

```tsx
interface LevelPartProps {
    builder: ReportBuilder
    groups: BuilderDraft['groups']
    index: number
    target?: CatalogSource
}

const setLevel = (builder: ReportBuilder, groups: BuilderDraft['groups'], index: number, patch: Partial<GroupLevelDraft>, added?: ParamDef) =>
    builder.setGroups(groups.map((g, i) => (i === index ? { ...g, ...patch } : g)), added)

/** "Label": the record's first text field (default), another record field, or a formula. */
function GroupLabel({ builder, groups, index, target }: LevelPartProps) {
    const group = groups[index]
    if (!group || !target) return null
    const testID = `report-builder-group-${index}-label`
    const fieldOf = (label: FormulaDraft | null) =>
        label?.node.record === group.id && label.node.path?.length === 1 ? label.node.path[0]?.field : undefined
    const current = fieldOf(group.label)
    const items: PickItem[] = [
        { id: 'default', label: `Default (${firstTextField(target)?.label ?? 'ID'})`, isSelected: group.label === null, onSelect: () => setLevel(builder, groups, index, { label: null }) },
        ...target.fields.map(f => ({
            id: f.key,
            label: f.label,
            isSelected: f.key === current,
            onSelect: () => setLevel(builder, groups, index, { label: { kind: 'editable', node: { record: group.id, path: [{ field: f.key }] } } }),
        })),
        { id: 'formula', label: 'Formula…', isSelected: group.label !== null && current === undefined, onSelect: () => setLevel(builder, groups, index, { label: { kind: 'editable', node: { record: group.id, path: [{ field: firstTextField(target)?.key ?? 'id' }] } } }) },
    ]
    const shown = items.find(i => i.isSelected)?.label ?? 'Default'
    return (
        <View className="gap-1">
            <PickMenu label={`Label: ${shown}`} purpose="Group label" items={items} testID={testID} />
            <LevelFormula isVisible={group.label !== null && current === undefined} builder={builder} formula={group.label} onChange={label => setLevel(builder, groups, index, { label })} testID={`${testID}-formula`} />
        </View>
    )
}

/** "Show every <target>, even with no rows", with its own conditions, on any level (R6). */
function GroupFill({ builder, groups, index, target }: LevelPartProps) {
    const group = groups[index]
    if (!group || !target) return null
    const testID = `report-builder-group-${index}-fill`
    return (
        <View className="gap-1.5">
            <View className="flex-row items-center gap-2">
                <Switch value={group.fill.on} onValueChange={on => setLevel(builder, groups, index, { fill: { ...group.fill, on } })} accessibilityLabel={`Show every ${target.label}`} testID={`${testID}-on`} />
                <Text className="flex-1 text-sm text-foreground">{`Show every ${target.label}, even with no rows`}</Text>
            </View>
            <FillConditions isVisible={group.fill.on} builder={builder} root={group.fill.where} baseKey={target.key} testID={testID} onChange={(where, added) => setLevel(builder, groups, index, { fill: { on: true, where } }, added)} />
        </View>
    )
}

/** "Show only groups where…": the filter editor in aggregate mode (R18). */
function GroupHaving({ builder, groups, index }: LevelPartProps) {
    const group = groups[index]
    const control = builder.form.control
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    // Read with useWatch, so the condition menus follow other sections (F27).
    const level = { ...levelScope({ source, columns, groups }, builder.catalog), columns: columns.map(c => ({ key: c.id, label: c.label })) }
    if (!group) return null
    return (
        <View className="gap-1">
            <Text className="text-xs text-muted-foreground">Show only groups where…</Text>
            <ConditionTree builder={builder} root={group.having} baseKey={builder.base.key} testID={`report-builder-group-${index}-having`} level={level} onChange={(having, added) => setLevel(builder, groups, index, { having }, added)} />
        </View>
    )
}
```

A fill's conditions on a level under other groups can tie a record to its parent: a condition row in the fill tree gains a "Same as the <group> above" choice (`…-fill-<n>-above`, a `PickMenu` of `aboveOf(groups, index, columns)` whose column type matches the field, plus "A value" to clear it), which sets the condition's `above` (Task 21 reads and writes it as `{ col: <that group's column> }`). `ConditionTree` takes `above?: readonly AboveGroup[]` and passes it to `nodeToFilter`/`filterToNode`/`conditionProblems`; `FillConditions` passes `aboveOf(groups, index, columns)`, filter and having pass none. While `above` is set the value box and Ask when run are hidden (the value is the group's key). (`LevelFormula` draws `FormulaEditor` with `reportOperators(useFormulaContext(builder, { kind: 'level' }))` (the hook before its `isVisible` return) and returns null when not visible; `FillConditions` draws `ConditionTree` and returns null when not visible.)

Sort rows: replace the column `SelectInput` with a `PickMenu` whose items are the columns (`id: column.id`), each record group's fields (`id: record.<group>.<field>`, label `"<Group> › <Field>"`), and `formula` ("Formula…"); choosing one sets the sort key to `{ kind: 'col', col }`, `{ kind: 'formula', formula: { kind: 'editable', node: { record, path: [{ field }] } } }` or `{ kind: 'formula', formula: NEW_FORMULA }`. A formula key that is not a bare record field shows `LevelFormula` (test id `report-builder-sort-<i>-formula`). "Add sort" becomes the same menu (`report-builder-sort-add-menu`) appending a key. `sortRowKeys` reads `key.kind === 'col' ? key.col : JSON.stringify(key.formula.node)`.

- [ ] **Step 5: Columns — Total and Row label**

In `ColumnRow`, after `FormatField`:

```tsx
            <TotalField builder={builder} column={column} index={index} />
```

```tsx
const TOTAL_OPTIONS = [
    { value: 'recompute', label: 'Recompute from all rows' },
    { value: 'rows', label: 'Add the rows shown' },
]

/** A number or amount column totals all rows, or only the rows the report shows (E2). */
function TotalField({ builder, column, index }: { builder: ReportBuilder; column: ColumnDraft; index: number }) {
    const columns = useWatch({ control: builder.form.control, name: 'columns' })
    const source = useWatch({ control: builder.form.control, name: 'source' })
    if (!isNumberColumn(builder, column) && !columnIsAmount(column, { source, columns }, builder.catalog)) return null
    return (
        <SelectInput name={`columns.${index}.total`} control={builder.form.control} label="Total" options={TOTAL_OPTIONS} testID={`report-builder-column-${index}-total`} />
    )
}
```

(The existing "Total" select of a column's aggregate keeps its label "Total"; rename it "Summarize" so the two read apart, and update `tests/e2e/reports-*.spec.ts` only if a spec reads that label text — they read test ids, which stay.) Under the column list, a `DetailLabelCard`:

```tsx
/** "Row label": opened rows are labelled by the first column, or by a formula (E7). */
function DetailLabelCard({ builder }: { builder: ReportBuilder }) {
    const label = useWatch({ control: builder.form.control, name: 'detailLabel' })
    const testID = 'report-builder-detail-label'
    return (
        <View className="gap-1.5 rounded-lg border border-border p-2">
            <View className="flex-row items-center gap-2">
                <Switch value={label !== null} onValueChange={on => builder.setDetailLabel(on ? NEW_FORMULA : null)} accessibilityLabel="Label opened rows with a formula" testID={`${testID}-on`} />
                <Text className="flex-1 text-sm text-foreground">Label opened rows with a formula</Text>
            </View>
            <DetailFormula isVisible={label !== null} builder={builder} formula={label} testID={testID} />
        </View>
    )
}
```

(`DetailFormula` is `FormulaEditor` with `useFormulaContext(builder, { kind: 'detail' })`, or `FixedFormula` for a fixed draft, and returns null when not visible.)

A link column shows its record's label (R31, F22); in `ColumnRow`, after `TotalField`, `<LinkLabelField builder={builder} column={column} index={index} source={source} />` (`ColumnRow` reads `source` with `useWatch`, F27):

```tsx
/** "Show as": the field of a link column's record its cells show. */
function LinkLabelField({ builder, column, index, source }: { builder: ReportBuilder; column: ColumnDraft; index: number; source: string }) {
    const target = linkTarget(builder.catalog, source, column)
    if (!target) return null
    const first = firstTextField(target)
    const items: PickItem[] = [
        { id: '', label: first ? `${first.label} (first text field)` : target.label, isSelected: column.labelField === '', onSelect: () => builder.form.setValue(`columns.${index}.labelField`, '') },
        ...target.fields.filter(f => f.key !== 'id').map(f => ({ id: f.key, label: f.label, isSelected: f.key === column.labelField, onSelect: () => builder.form.setValue(`columns.${index}.labelField`, f.key) })),
    ]
    const label = items.find(i => i.isSelected)?.label ?? `${column.labelField} (missing)`
    return <PickMenu label={label} purpose="Show as" items={items} testID={`report-builder-column-${index}-label-field`} />
}
```

with `linkTarget(catalog, source, column)` (in `lib/builder/columns.ts`, with a `columns.test.ts` case: a team-link column gives the teams source, a customer column undefined): the target source of a path column that ends at a to-one link or at `id`, else undefined. Add to `reports-groups-v2.spec.ts`:

```ts
    test('a link column shows its team by name, then by the field chosen under Show as', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptlbl')
        try {
            const stem = uniqueName('Lbl')
            await openReportStub(p)
            await addTeam(p, `${stem} T`, 'south')
            await addSale(p, { team: `${stem} T`, customer: `${stem} Cy`, amount: 100, cost: 0, day: '2026-01-05' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['customer'])
            await p.getByTestId('report-builder-add-column').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-record-team').click()
            const row = p.getByTestId('report-table').locator('[data-testid^="report-row-"]').filter({ hasText: `${stem} Cy` })
            await expect(row).toContainText(`${stem} T`)
            await chooseOption(p, 'report-builder-column-1-label-field', 'region')
            await expect(row).toContainText('South')
            await expect(row).not.toContainText(`${stem} T`)
        } finally {
            await close()
        }
    })
```

- [ ] **Step 6: A fill under a parent group, in e2e**

Add to `reports-groups-v2.spec.ts`:

```ts
    test('a deeper group shows every team of its region, even with no rows, under each region', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptfill2')
        try {
            const stem = uniqueName('Deep')
            await openReportStub(p)
            await addTeam(p, `${stem} N1`, 'north')
            await addTeam(p, `${stem} N2`, 'north')
            await addTeam(p, `${stem} S1`, 'south')
            await addSale(p, { team: `${stem} N1`, customer: 'Nia', amount: 100, cost: 0, day: '2026-01-05' })
            await addSale(p, { team: `${stem} S1`, customer: 'Sol', amount: 100, cost: 0, day: '2026-01-05' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['team', 'region'])
            await p.getByTestId('report-builder-add-column').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-record-team').click()
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'team_region')
            await p.getByTestId('report-builder-group-0-open').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-1-col', 'team')
            await p.getByTestId('report-builder-group-1-fill-on').click()
            await p.getByTestId('report-builder-group-1-fill-add-condition').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
            await chooseOption(p, 'report-builder-group-1-fill-0-op', 'startsWith')
            await p.getByTestId('report-builder-group-1-fill-0-value').fill(stem)
            await p.getByTestId('report-builder-group-1-fill-add-condition').click()
            await p.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-region').click()
            await chooseOption(p, 'report-builder-group-1-fill-1-above', 'level')
            await chooseOption(p, 'report-builder-sort-add-menu', 'record.level2.name')
            const rows = p.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')
            await expect(rows.filter({ hasText: stem })).toHaveText([`${stem} N1`, `${stem} N2`, `${stem} S1`])
            await expect(rowOf(p, `${stem} N2`)).not.toHaveAttribute('aria-expanded')
        } finally {
            await close()
        }
    })
```

(The sales filter is not narrowed: other tests' teams with this member's sales cannot exist — the member is fresh — and the fill keeps only this test's teams; `N2` sits under North with no sales, `S1` under South.)

- [ ] **Step 7: Run**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-groups-v2.spec.ts tests/e2e/reports-filter.spec.ts tests/e2e/reports-group-pivot.spec.ts tests/e2e/reports-create.spec.ts`
Expected: all pass (the filter, group and create specs keep their ids).

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-groups-v2.spec.ts
git commit -m "feat(core): the builder groups by records, hides groups, totals shown rows and labels opened rows"
git push origin books/p5-trial-balance
```

---

### Task 23: E8, E9, E10 in the view — the source picker, chip labels, the heading; relation values in the builder; e2e

Code facts used: `ParamPicker` and its bodies (`components/ParamPickers.tsx:75-211`), the chips and surfaces (`components/ReportControls.tsx:21-144`), `useReportView` (`hooks/useReportView.ts:77-249`), `ReportDocumentHeader` (`components/ReportDocumentHeader.tsx:20-61`), `isNearEnd` (`components/use-row-window.ts:17-23`), the share flow in `tests/e2e/reports-share.spec.ts:72-92`, hub tabs `reports-hub-tab-mine|shared|published` (`hooks/useReportsHub.ts:18-20`), `signInAsCollaborator` (`tests/e2e/helpers.ts:257-275`).

**Files:**
- Create: `core/reports/lib/param-options.ts`, `core/reports/lib/__tests__/param-options.test.ts`, `core/reports/hooks/useParamOptions.ts`
- Modify: `core/reports/lib/param-forms.ts` (`amountValueSchema`), test `lib/__tests__/param-forms.test.ts`
- Modify: `core/reports/components/ParamPickers.tsx` (`SourceChoiceBody`, `SourcePicker`, `AmountBody`), `ReportControls.tsx`, `ReportView.tsx:153-158`, `ReportDocumentHeader.tsx`, `hooks/useReportView.ts`
- Modify: `core/reports/components/builder/ConditionTree.tsx` (`RecordValuePicker`)
- Create: `tests/e2e/reports-source-params.spec.ts`
- Modify: `tests/e2e/helpers.ts` (`createInvitedUser(page, label, options: { name?: string } = {})`: the invitee's name, default `'Invited Tester'`, filled at `getByTestId('name')`, `helpers.ts:335`)

**Interfaces:**
- Consumes: `fetchParamOptions`, `chipText(…, labels)` (Task 16), `askParamFor(…, target)` (Task 21), the stub's "Stub team totals" (Task 17).
- Produces: `type OptionsSource = { reportId: string } | { spec: ReportSpec }`; `optionSections(pages: ParamOptionsPage[]): { group: ParamOptionGroup | null; options: ParamOption[] }[]` (groups deduped, F9); `toggleGroup(groups, group): string[]`; `useParamOptions({ source, param, search, params, isEnabled })`; `useParamOptionLabels(source, defs, values): ReadonlyMap<string, string>`; view model fields `heading`, `optionsSource`, `postedParams`, `optionLabels`; picker test ids `report-filter-search`, `report-filter-group-<value>`, `report-filter-option-<value>`, `report-filter-more`.

- [ ] **Step 1: Write the failing lib test**

Create `core/reports/lib/__tests__/param-options.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { optionSections, toggleGroup } from '../param-options'

describe('options from a source', () => {
    it('sections the loaded pages by group, in the groups order', () => {
        const pages = [
            { options: [{ value: 'a', label: 'Bank', group: 'asset' }, { value: 'b', label: 'Sales', group: 'income' }], groups: [{ value: 'asset', label: 'Asset' }, { value: 'income', label: 'Income' }], next: 'x' },
            { options: [{ value: 'c', label: 'Cash', group: 'asset' }, { value: 'd', label: 'Misc' }], groups: [] },
        ]
        expect(optionSections(pages)).toEqual([
            { group: null, options: [{ value: 'd', label: 'Misc' }] },
            { group: { value: 'asset', label: 'Asset' }, options: [{ value: 'a', label: 'Bank', group: 'asset' }, { value: 'c', label: 'Cash', group: 'asset' }] },
            { group: { value: 'income', label: 'Income' }, options: [{ value: 'b', label: 'Sales', group: 'income' }] },
        ])
    })
    it('keeps one head per group when a later page repeats the groups, and heads a group no page listed', () => {
        const groups = [{ value: 'asset', label: 'Asset' }]
        const pages = [
            { options: [{ value: 'a', label: 'Bank', group: 'asset' }], groups, next: 'x' },
            { options: [{ value: 'c', label: 'Cash', group: 'asset' }, { value: 'e', label: 'Gear', group: 'equipment' }], groups },
        ]
        expect(optionSections(pages).map(s => [s.group?.value, s.options.map(o => o.value)])).toEqual([
            ['asset', ['a', 'c']],
            ['equipment', ['e']],
        ])
    })
    it('picks a whole group once and unpicks it', () => {
        expect(toggleGroup(['asset'], 'income')).toEqual(['asset', 'income'])
        expect(toggleGroup(['asset', 'income'], 'asset')).toEqual(['income'])
    })
})
```

- [ ] **Step 2: Write the failing e2e**

Create `tests/e2e/reports-source-params.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, login, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    chooseParamOption,
    hubRow,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    STUB_HEADING,
    startReport,
    TEAM_TOTALS_TITLE,
    uniqueName,
} from './reports-helpers'

/**
 * Choice params whose options come from a source (E8), the source heading (E9)
 * and an unpublished shipped report reached through a share (E10), against the
 * stub's "Stub team totals" (unpublished, owned by the seeded owner, the first
 * admin) and a draft over sales. The owner's sales are shared by several specs, so
 * rows are found by team names unique to the test.
 */

async function seedRegions(p: Page, stem: string) {
    await openReportStub(p)
    await addTeam(p, `${stem} N1`, 'north')
    await addTeam(p, `${stem} N2`, 'north')
    await addTeam(p, `${stem} S1`, 'south')
    for (const [team, customer] of [[`${stem} N1`, 'Nia'], [`${stem} N2`, 'Ned'], [`${stem} S1`, 'Sol']] as const) {
        await addSale(p, { team, customer, amount: 100, cost: 0, day: '2026-01-05' })
    }
}

async function openTeamTotals(p: Page) {
    await openReportsHub(p)
    await p.getByTestId('reports-hub-tab-mine').click()
    await hubRow(p, TEAM_TOTALS_TITLE).click()
    await expect(p.getByTestId('report-title')).toHaveText(TEAM_TOTALS_TITLE)
}

test.describe('params from a source, headings and unpublished shipped reports', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an unpublished shipped report prints its heading and reaches only who it is shared with', async ({ page }) => {
        await login(page)
        await openReportsHub(page)
        await page.getByTestId('reports-hub-tab-published').click()
        await expect(hubRow(page, TEAM_TOTALS_TITLE)).toHaveCount(0)
        await openTeamTotals(page)
        await expect(page.getByTestId('report-business-name')).toHaveText(STUB_HEADING)

        // A fresh reader each run, with a name no one else has, so a re-run on
        // the same database finds them as a candidate (F24).
        const readerName = uniqueName('Reader')
        const { inviteePage: c, close } = await createInvitedUser(page, 'rptshr', { name: readerName })
        try {
            await openReportsHub(c)
            await c.getByTestId('reports-hub-tab-published').click()
            await expect(hubRow(c, TEAM_TOTALS_TITLE)).toHaveCount(0)

            await pressToolbar(page, 'report-share-open', 'Share')
            await page.getByTestId('report-share-search').fill(readerName)
            await page.getByTestId('report-share-dialog').locator('[data-testid^="report-share-candidate-"]').filter({ hasText: readerName }).click()
            await page.getByTestId('report-share-done').click()

            await c.getByTestId('reports-hub-tab-shared').click()
            await hubRow(c, TEAM_TOTALS_TITLE).click()
            await expect(c.getByTestId('report-title')).toHaveText(TEAM_TOTALS_TITLE)
            await expect(c.getByTestId('report-business-name')).toHaveText(STUB_HEADING)
        } finally {
            await close()
        }
    })

    test('a choice from a source searches, picks one record, then a whole group', async ({ page }) => {
        await login(page)
        const stem = uniqueName('Src')
        await seedRegions(page, stem)
        await openTeamTotals(page)
        await chooseParamOption(page, 'teams', { from: 'add-filter', search: stem, labels: [`${stem} N1`] })
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${stem} N1`)
        const rows = page.getByTestId('report-table').locator('[data-testid^="report-row-g:"]')
        await expect(rows).toHaveText([`${stem} N1`])
        await chooseParamOption(page, 'teams', { from: 'chip', search: stem, group: 'north' })
        await expect(page.getByTestId('report-param-teams')).toHaveText(`Teams: ${stem} N1, All North`)
        await expect(rowOf(page, `${stem} N2`)).toBeVisible()
        await expect(rows.filter({ hasText: `${stem} S1` })).toHaveCount(0)
    })

    test('on a phone the source picker is a sheet and the page does not scroll sideways', async ({ page }) => {
        await login(page)
        const stem = uniqueName('SrcPh')
        await seedRegions(page, stem)
        await openTeamTotals(page)
        await page.setViewportSize({ width: 390, height: 844 })
        await chooseParamOption(page, 'teams', { from: 'add-filter', search: stem, labels: [`${stem} S1`] })
        await expect(rowOf(page, `${stem} S1`)).toBeVisible()
        await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    })

    test('a condition on a linked team picks a record, and Ask when run lists the teams', async ({ page }) => {
        await login(page)
        const stem = uniqueName('Rel')
        await seedRegions(page, stem)
        await openReportsHub(page)
        await startReport(page, SALES_SOURCE)
        await page.getByTestId('report-builder-section-filter').click()
        await page.getByTestId('report-builder-filter-add-condition').click()
        await page.getByTestId('report-builder-picker').getByTestId('report-builder-pick-record-team').click()
        await page.getByTestId('report-builder-filter-0-value').click()
        await page.getByTestId('report-filter-search').fill(`${stem} S1`)
        await page.getByTestId('report-filter-picker').getByRole('radio', { name: `${stem} S1`, exact: true }).click()
        const table = page.getByTestId('report-table')
        await expect(table.getByText('Sol', { exact: true })).toBeVisible()
        await expect(table.getByText('Nia', { exact: true })).toHaveCount(0)

        await chooseOption(page, 'report-builder-filter-0-op', 'in')
        await page.getByTestId('report-builder-filter-0-ask').click()
        await chooseParamOption(page, 'team', { from: 'add-filter', search: stem, labels: [`${stem} N1`, `${stem} N2`] })
        await expect(table.getByText('Nia', { exact: true })).toBeVisible()
        await expect(table.getByText('Ned', { exact: true })).toBeVisible()
        await expect(table.getByText('Sol', { exact: true })).toHaveCount(0)
    })
})
```

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-source-params.spec.ts`
Expected: FAIL (the heading prints the organization name; the Teams picker has no `report-filter-search`).

- [ ] **Step 3: `lib/param-options.ts` and `hooks/useParamOptions.ts`**

```ts
// lib/param-options.ts
import type { ParamOption, ParamOptionGroup, ParamOptionsPage } from './types'

export type OptionSection = { group: ParamOptionGroup | null; options: ParamOption[] }

/** The loaded options in sections: ungrouped first, then each group in the server's order. */
export function optionSections(pages: readonly ParamOptionsPage[]): OptionSection[] {
    const options = pages.flatMap(page => page.options)
    // Groups come with the first page; a group seen twice is one head (F9). An
    // option of a group no page listed (past the 200 the server lists) gets a
    // head of its own, named by its value.
    const groups = new Map<string, ParamOptionGroup>()
    for (const group of pages.flatMap(page => page.groups)) if (!groups.has(group.value)) groups.set(group.value, group)
    for (const option of options) if (option.group && !groups.has(option.group)) groups.set(option.group, { value: option.group, label: option.group })
    const loose = options.filter(o => !o.group)
    const sections: OptionSection[] = loose.length > 0 ? [{ group: null, options: loose }] : []
    for (const group of groups.values()) {
        const inGroup = options.filter(o => o.group === group.value)
        if (inGroup.length > 0) sections.push({ group, options: inGroup })
    }
    return sections
}

/** A group picked whole (or unpicked): the server expands it at run time (R30). */
export function toggleGroup(groups: readonly string[], group: string): string[] {
    return groups.includes(group) ? groups.filter(g => g !== group) : [...groups, group]
}
```

```ts
// hooks/useParamOptions.ts
import { useInfiniteQuery, useQueries } from '@tanstack/react-query'
import { todayIso } from '../lib/dates'
import type { ParamValues } from '../lib/params'
import { fetchParamOptions } from '../lib/run-client'
import type { ParamDef, ReportSpec } from '../lib/spec'
import type { ParamOptionsPage } from '../lib/types'

export type OptionsSource = { reportId: string } | { spec: ReportSpec }

const NO_PAGE: ParamOptionsPage = { options: [], groups: [] }

/** A choice param's options, a page at a time, searched by label. */
export function useParamOptions(input: { source: OptionsSource; param: string; search: string; params: Record<string, unknown>; isEnabled: boolean }) {
    const { source, param, search, params, isEnabled } = input
    return useInfiniteQuery({
        queryKey: ['reports', 'param-options', source, param, search, params],
        enabled: isEnabled,
        initialPageParam: '',
        queryFn: ({ pageParam, signal }) =>
            fetchParamOptions({ ...source, param, search: search || undefined, after: pageParam || undefined, params, today: todayIso() }, signal),
        getNextPageParam: last => last.next,
    })
}

/** The labels of the chosen records of every options-from-source param, for the chips. */
export function useParamOptionLabels(source: OptionsSource | null, defs: readonly ParamDef[], values: ParamValues): ReadonlyMap<string, string> {
    const wanted = defs.flatMap(def => {
        const value = values[def.id]
        // A request with only groups chosen still runs: its first page lists the groups.
        return def.optionsFrom && value?.kind === 'choice' ? [{ id: def.id, ids: value.values }] : []
    })
    const results = useQueries({
        queries: wanted.map(w => ({
            queryKey: ['reports', 'param-labels', source, w.id, w.ids],
            enabled: source !== null,
            queryFn: ({ signal }: { signal: AbortSignal }) =>
                source ? fetchParamOptions({ ...source, param: w.id, values: w.ids, today: todayIso() }, signal) : Promise.resolve(NO_PAGE),
        })),
    })
    // Group labels are filed as group:<value>, for the chip's "All <group>" (R30).
    return new Map(results.flatMap(r => [
        ...(r.data?.options ?? []).map(o => [o.value, o.label] as const),
        ...(r.data?.groups ?? []).map(g => [`group:${g.value}`, g.label] as const),
    ]))
}
```

- [ ] **Step 4: The searching picker**

In `ParamPickers.tsx`:

```tsx
const sourceChoiceSchema = z.object({
    search: z.string().max(200),
    values: z.array(z.string()).max(MAX_CHOICE_VALUES, 'Pick at most 1000 records, or pick a whole group'),
    groups: z.array(z.string()).max(200),
})

interface SourceBodyProps extends BodyProps {
    source: OptionsSource
    params: Record<string, unknown>
}

function GroupHead({ group, isPicked, canPickAll, onPickAll }: { group: ParamOptionGroup | null; isPicked: boolean; canPickAll: boolean; onPickAll: (value: string) => void }) {
    if (!group) return null
    return (
        <View className="flex-row items-center justify-between px-2 pt-2">
            <Text className="text-xs font-semibold uppercase text-muted-foreground">{group.label}</Text>
            <AllOfGroup isVisible={canPickAll} isPicked={isPicked} group={group} onPress={() => onPickAll(group.value)} />
        </View>
    )
}

function AllOfGroup({ isVisible, isPicked, group, onPress }: { isVisible: boolean; isPicked: boolean; group: ParamOptionGroup; onPress: () => void }) {
    if (!isVisible) return null
    return (
        <Pressable testID={`report-filter-group-${group.value}`} accessibilityRole="checkbox" aria-checked={isPicked} onPress={onPress} style={TARGET_STYLE} className="justify-center px-2">
            <Text className="text-sm text-primary">{`All ${group.label}`}</Text>
        </Pressable>
    )
}

function SectionOptions({ options, values, groupPicked, isMultiple, onToggle }: { options: ParamOption[]; values: string[]; groupPicked: boolean; isMultiple: boolean; onToggle: (value: string) => void }) {
    const rows = options.map(option => (
        <OptionRow key={option.value} value={option.value} label={option.label} isChecked={groupPicked || values.includes(option.value)} isRadio={!isMultiple} onPress={() => onToggle(option.value)} />
    ))
    return <>{rows}</>
}

function PickError({ message }: { message: string | undefined }) {
    if (!message) return null
    return <Text role="alert" testID="report-filter-error" className="text-xs text-danger">{message}</Text>
}

// Options read from a source under the reader's rules: searched, loaded a page at a
// time near the end of the list, and picked one by one or a whole group at a time
// (a group reference the server expands at run time, R30).
function SourceChoiceBody({ def, value, onApply, source, params }: SourceBodyProps) {
    const chosen = value?.kind === 'choice' ? value : null
    const form = useForm({
        resolver: zodResolver(sourceChoiceSchema),
        mode: 'onChange',
        defaultValues: { search: '', values: chosen?.values ?? [], groups: chosen?.groups ?? [] },
    })
    const search = useDebouncedValue(useWatch({ control: form.control, name: 'search' }), 250)
    const values = useWatch({ control: form.control, name: 'values' })
    const groups = useWatch({ control: form.control, name: 'groups' })
    const options = useParamOptions({ source, param: def.id, search, params, isEnabled: true })
    const sections = optionSections(options.data?.pages ?? [])
    const isMultiple = def.multiple === true
    const error = form.formState.errors.values?.message
    const toggle = (option: string) => {
        if (!isMultiple) {
            onApply({ kind: 'choice', values: [option] })
            return
        }
        const next = values.includes(option) ? values.filter(v => v !== option) : [...values, option]
        form.setValue('values', next, { shouldValidate: true })
    }
    const pickAll = (group: string) => form.setValue('groups', toggleGroup(groups, group), { shouldValidate: true })
    const onScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) => {
        if (isNearEnd(event.nativeEvent) && options.hasNextPage && !options.isFetchingNextPage) void options.fetchNextPage()
    }
    const body = sections.map(section => {
        const groupPicked = section.group !== null && groups.includes(section.group.value)
        return (
            <View key={section.group?.value ?? 'loose'}>
                <GroupHead group={section.group} isPicked={groupPicked} canPickAll={isMultiple} onPickAll={pickAll} />
                <SectionOptions options={section.options} values={values} groupPicked={groupPicked} isMultiple={isMultiple} onToggle={toggle} />
            </View>
        )
    })
    const isEmpty = values.length === 0 && groups.length === 0
    const apply = () => void form.handleSubmit(submitted => onApply({ kind: 'choice', values: submitted.values, groups: submitted.groups }))()
    return (
        <View className="gap-2">
            <TextInput name="search" control={form.control} label="Search" testID="report-filter-search" />
            <ScrollView style={LIST_STYLE} nestedScrollEnabled onScroll={onScroll} scrollEventThrottle={16}>
                {body}
                <LoadingMoreOptions isVisible={options.isFetchingNextPage} />
            </ScrollView>
            <PickError message={error} />
            <SourceApply isVisible={isMultiple} isDisabled={isEmpty || error !== undefined} onPress={apply} />
        </View>
    )
}
```

An amount param (R29, F18) is picked with `AmountBody`. In `param-forms.ts`:

```ts
export const amountValueSchema = z.object({
    amount: z.string().trim().refine(text => centsFromText(text) !== null, 'Type an amount, like 1500.00'),
    currency: z.string().trim().regex(/^([A-Za-z]{3})?$/, 'Type a three-letter currency code, like USD, or leave it blank for any currency'),
})
export type AmountValue = z.infer<typeof amountValueSchema>

/** The param value of a submitted amount form; the schema has checked the text. */
export function amountParamValue(form: AmountValue): ParamValue {
    return { kind: 'amount', cents: centsFromText(form.amount) ?? 0, currency: form.currency.toUpperCase() }
}
```

(`param-forms.test.ts`: `'1,500.00'` with `'usd'` gives `{ kind: 'amount', cents: 150000, currency: 'USD' }`; `'12.3.4'` and the currency `'US'` fail with those messages.) In `ParamPickers.tsx`:

```tsx
function AmountBody({ def, value, onApply }: BodyProps) {
    const current = value?.kind === 'amount' ? value : null
    const form = useForm<AmountValue>({
        resolver: zodResolver(amountValueSchema),
        mode: 'onChange',
        defaultValues: { amount: current ? textFromCents(current.cents) : '', currency: current?.currency ?? '' },
    })
    const apply = () => void form.handleSubmit(submitted => onApply(amountParamValue(submitted)))()
    return (
        <View className="gap-2">
            <TextInput name="amount" control={form.control} label={def.label ?? 'Amount'} keyboardType="decimal-pad" testID="report-filter-value" />
            <TextInput name="currency" control={form.control} label="Currency" autoCapitalize="characters" placeholder="Any currency" testID="report-filter-currency" />
            <ApplyButton isDisabled={!form.formState.isValid} onPress={apply} />
        </View>
    )
}
```

and `PickerBody` gains `if (props.def.type === 'amount') return <AmountBody {...props} />` before the number case. Add to `reports-source-params.spec.ts`:

```ts
    test('a having on a money total asks an amount, kept to its currency', async ({ page }) => {
        await login(page)
        const stem = uniqueName('Amt')
        await seedRegions(page, stem)
        await addSale(page, { team: `${stem} N1`, customer: 'Nia', amount: 300, cost: 0, day: '2026-01-06' })
        await openReportsHub(page)
        await startReport(page, SALES_SOURCE)
        await page.getByTestId('report-builder-add-column').click()
        await page.getByTestId('report-builder-picker').getByTestId('report-builder-pick-record-team').click()
        await addColumn(page, ['amount'])
        await chooseOption(page, 'report-builder-column-1-aggregate', 'sum')
        await page.getByTestId('report-builder-section-filter').click()
        await page.getByTestId('report-builder-filter-add-condition').click()
        await page.getByTestId('report-builder-picker').getByTestId('report-builder-pick-link-team').click()
        await page.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-name').click()
        await chooseOption(page, 'report-builder-filter-0-op', 'startsWith')
        await page.getByTestId('report-builder-filter-0-value').fill(stem)
        await page.getByTestId('report-builder-section-group').click()
        await page.getByTestId('report-builder-group-add').click()
        await chooseOption(page, 'report-builder-group-0-col', 'team')
        await page.getByTestId('report-builder-group-0-having-add-total').click()
        await page.getByTestId('report-builder-picker').getByTestId('report-builder-pick-field-amount').click()
        await chooseOption(page, 'report-builder-group-0-having-0-aggregate', 'sum')
        await chooseOption(page, 'report-builder-group-0-having-0-op', 'gte')
        await page.getByTestId('report-builder-group-0-having-0-ask').click()

        const table = page.getByTestId('report-table')
        const pickBudget = async (amount: string, currency: string) => {
            await page.getByTestId('report-add-filter').click()
            await page.getByTestId('report-add-filter-amount').click()
            const picker = page.getByTestId('report-filter-picker')
            await picker.getByTestId('report-filter-value').fill(amount)
            await picker.getByTestId('report-filter-currency').fill(currency)
            await picker.getByTestId('report-filter-apply').click()
        }
        await pickBudget('200.00', 'USD')
        await expect(page.getByTestId('report-param-amount')).toHaveText('Amount: USD 200.00')
        await expect(rowOf(page, `${stem} N1`)).toBeVisible()
        await expect(table.getByText(`${stem} N2`, { exact: true })).toHaveCount(0)
        await page.getByTestId('report-param-amount').click()
        await page.getByTestId('report-filter-picker').getByTestId('report-filter-currency').fill('EUR')
        await page.getByTestId('report-filter-picker').getByTestId('report-filter-apply').click()
        // The sales are in USD: a EUR amount compares with none of them (R29).
        await expect(table.getByText(`${stem} N1`, { exact: true })).toHaveCount(0)
    })
```

(When the param is already set, `report-add-filter-amount` is gone, so the second pick opens the chip.)

`LoadingMoreOptions` (`testID="report-filter-more"`, an `ActivityIndicator`, null when not visible) and `SourceApply` (the existing `ApplyButton`, null when not visible) are small parts. `PickerBody` chooses it first: `if (props.def.type === 'choice' && props.def.optionsFrom && props.source) return <SourceChoiceBody {...props} source={props.source} params={props.params} />`; `ParamPicker` takes `source: OptionsSource | null` and `params: Record<string, unknown>` and passes them on. Export a `SourcePicker({ trigger, isOpen, onOpenChange, def, value, onApply, source, params, title })` (the same `Popover` around `SourceChoiceBody`) for the builder.

- [ ] **Step 5: The view: chip labels, picker source, heading**

`useReportView.ts`:

```ts
    const optionsSource: OptionsSource | null = source ? (source.kind === 'saved' ? { reportId: source.reportId } : { spec: source.spec }) : null
    const posted = postedParams(defs, options.params)
    const optionLabels = useParamOptionLabels(optionsSource, defs, options.params)
```

and return `optionsSource`, `postedParams: posted`, `optionLabels`, and `heading: run.result?.heading || org?.name || ''` (replacing `orgName`). `ReportControls.tsx`: `ParamControl`'s chip text is `chipText(def, value, model.resolved[def.id], model.optionLabels)`; `ParamSurface` passes `source={model.optionsSource}` and `params={model.postedParams}` to `ParamPicker`. `ReportDocumentHeader` renames its `orgName` prop to `heading` ("the source's heading, else the organization's name", E9) and `ReportView` passes `heading={model.heading}`.

- [ ] **Step 6: Relation values in the builder**

In `ConditionTree.tsx`, a relation condition (`info.target` set) with `eq`/`ne`/`in` and no asked param shows a `RecordValuePicker` instead of the text box:

```tsx
function pickerSpec(target: CatalogSource, multiple: boolean): ReportSpec {
    const label = firstTextField(target)?.key ?? 'id'
    return {
        version: CURRENT_SPEC_VERSION,
        source: target.key,
        columns: [{ id: 'id', path: [{ field: 'id' }] }],
        params: [{ id: 'pick', type: 'choice', multiple, optionsFrom: { source: target.key, value: [{ field: 'id' }], label: [{ field: label }] } }],
    }
}

// A record is picked from the linked source's records the reader may list (R19).
function RecordValuePicker({ ctx, item, path, target }: { ctx: FilterContext; item: ConditionDraft; path: FilterPath; target: CatalogSource }) {
    const isMultiple = item.op === 'in'
    const source = { spec: pickerSpec(target, isMultiple) }
    const chosen = isMultiple ? item.values : item.value ? [item.value] : []
    const labels = useParamOptionLabels(source, source.spec.params ?? [], chosen.length > 0 ? { pick: { kind: 'choice', values: chosen } } : {})
    const isOpen = useReportUIStore(s => s.builderPicker === itemTestId(ctx, path))
    const setPicker = useReportUIStore(s => s.setBuilderPicker)
    const def = source.spec.params?.[0]
    if (!def) return null
    const text = chosenText(chosen, labels, target.label)
    const current: ParamValue | null = chosen.length > 0 ? { kind: 'choice', values: chosen } : null
    const trigger = (
        <Button testID={`${itemTestId(ctx, path)}-value`} variant="outline" size="sm">
            <ButtonText>{text}</ButtonText>
        </Button>
    )
    const onApply = (next: ParamValue) => {
        if (next.kind !== 'choice') return
        setCondition(ctx, path, isMultiple ? { values: next.values } : { value: next.values[0] ?? '' })
        setPicker(null)
    }
    return (
        <SourcePicker trigger={trigger} isOpen={isOpen} onOpenChange={open => setPicker(open ? itemTestId(ctx, path) : null)} def={def}
            value={current} onApply={onApply} source={source} params={{}} title={target.label} />
    )
}
```

with:

```tsx
function chosenText(chosen: readonly string[], labels: ReadonlyMap<string, string>, noun: string): string {
    if (chosen.length === 0) return `Choose ${noun}`
    if (chosen.length > 2) return `${chosen.length} selected`
    return chosen.map(id => labels.get(id) ?? '(none)').join(', ')
}
```

(`report-ui-store.ts` gains `builderPicker: string | null` and `setBuilderPicker` — which builder record picker is open is shared UI state the store owns, not a component's `useState`.)

- [ ] **Step 7: Run**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-source-params.spec.ts tests/e2e/reports-view.spec.ts tests/e2e/reports-share.spec.ts`
Expected: all pass (no existing spec reads `report-business-name`; the new spec checks the stub's heading).

- [ ] **Step 8: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports tests/e2e/reports-source-params.spec.ts
git commit -m "feat(core): choice params from a source, source headings, record values in the builder"
git push origin books/p5-trial-balance
```

---

### Task 24: E14 (part 1) — line layouts per width as a block tree (guillotine cuts)

Code facts used: today's layout rule (`components/report-table-layout.ts:42-53`); `isRightAligned` (`lib/format.ts`); the engine's placement and cut rules (Task 15 `placeCells`, `uncuttable`, R28).

Design (R28, Q10 as the human answered): an arrangement is cut into blocks recursively — a straight cut between two lines splits a block into a top and a bottom block, a straight cut between two tracks into a left and a right block — until each block holds one cell or none. A cell may then span lines anywhere, mid-row included: a left/right cut puts it in a block beside stacked lines. Only a "pinwheel" (cells arranged so no straight cut separates any of them) cannot be cut; the engine and the builder refuse it with "These cells overlap in a way that can't be laid out; move one cell." The client draws the tree with flex: a top/bottom split is a column of blocks, a left/right split a row of blocks whose widths are their shares of the tracks; rows stretch their blocks to one height, so a cell that spans lines is as tall as the lines beside it, and `justify` aligns it.

**Files:**
- Create: `core/reports/lib/table-lines.ts`, `core/reports/lib/__tests__/table-lines.test.ts`
- Modify: `core/reports/components/report-table-layout.ts` (`fitsOnOneLine`), `components/__tests__/report-table-layout.test.ts`
- Modify: `core/reports/lib/builder/draft.ts` (`draftIssues` reports each display layout `placeCells` refuses)

**Interfaces:**
- Produces: `placeCells(tracks, cells): { placed: Placed[] } | { problem: { index: number; message: string } }` (the Go rules, the cut rule included); `PINWHEEL_MESSAGE`; `type Justify = 'start' | 'center' | 'end'`; `type Block = { kind: 'cell'; col: string; justify: Justify } | { kind: 'gap' } | { kind: 'stack'; parts: Block[] } | { kind: 'side'; of: number; parts: { block: Block; tracks: number }[] }` (a part is `tracks` of the block's `of` tracks wide); `type LineLayout = { kind: 'one-line' } | { kind: 'blocks'; root: Block }`; `tableLines({ columns, width, breakpoint, layouts, oneLineFits }): LineLayout`; `rowLayout(item: VisibleRow, layout: LineLayout, labelId: string): LineLayout` (a row with no figures draws only its label); `startingLayout(width, columns, tracks?)`; `fitsOnOneLine({ breakpoint, amountCount, width }): boolean`.

- [ ] **Step 1: Write the failing tests**

Create `core/reports/lib/__tests__/table-lines.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { PINWHEEL_MESSAGE, placeCells, rowLayout, startingLayout, tableLines } from '../table-lines'
import type { ResultColumn } from '../types'

const COLS: ResultColumn[] = [
    { id: 'account', label: 'Account', type: 'text' },
    { id: 'number', label: 'No.', type: 'text' },
    { id: 'debit', label: 'Debit', type: 'money', currency: 'USD' },
    { id: 'credit', label: 'Credit', type: 'money', currency: 'USD' },
]

describe('placing cells (the engine rules)', () => {
    it('flows cells into the first free tracks of their line', () => {
        expect(placeCells(3, [{ col: 'a', line: 1, span: 3 }, { col: 'b', line: 2 }, { col: 'c', line: 2 }])).toEqual({
            placed: [
                { start: 1, span: 3, line: 1, lines: 1 },
                { start: 1, span: 1, line: 2, lines: 1 },
                { start: 2, span: 1, line: 2, lines: 1 },
            ],
        })
    })
    it('names a cell with no room', () => {
        expect(placeCells(2, [{ col: 'a', line: 1, span: 2 }, { col: 'b', line: 1 }])).toEqual({ problem: { index: 1, message: 'no room on line 1' } })
    })
    it('lets a cell span lines anywhere a straight cut can free it, and refuses a pinwheel', () => {
        const mid = [{ col: 'a', line: 1 }, { col: 'm', line: 1, start: 2, lines: 2 }, { col: 'b', line: 1, start: 3 }, { col: 'c', line: 2, start: 1 }, { col: 'd', line: 2, start: 3 }]
        expect('placed' in placeCells(3, mid)).toBe(true)
        const pinwheel = [
            { col: 'a', line: 1, start: 1, span: 2 },
            { col: 'b', line: 1, start: 3, lines: 2 },
            { col: 'c', line: 3, start: 2, span: 2 },
            { col: 'd', line: 2, start: 1, lines: 2 },
            { col: 'e', line: 2, start: 2 },
        ]
        expect(placeCells(3, pinwheel)).toEqual({ problem: { index: 0, message: PINWHEEL_MESSAGE } })
    })
})

describe('the table layout per width', () => {
    it('keeps one line on desktop and wherever the columns fit', () => {
        expect(tableLines({ columns: COLS, width: 1280, breakpoint: 'desktop', layouts: [], oneLineFits: false })).toEqual({ kind: 'one-line' })
        expect(tableLines({ columns: COLS, width: 900, breakpoint: 'tablet', layouts: [], oneLineFits: true })).toEqual({ kind: 'one-line' })
    })
    it('wraps a phone row: the label across line 1, the amounts in aligned tracks below', () => {
        expect(tableLines({ columns: COLS, width: 390, breakpoint: 'mobile', layouts: [], oneLineFits: false })).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'cell', col: 'account', justify: 'start' },
                    {
                        kind: 'side',
                        of: 3,
                        parts: [
                            { block: { kind: 'cell', col: 'number', justify: 'start' }, tracks: 1 },
                            { block: { kind: 'cell', col: 'debit', justify: 'end' }, tracks: 1 },
                            { block: { kind: 'cell', col: 'credit', justify: 'end' }, tracks: 1 },
                        ],
                    },
                ],
            },
        })
    })
    it('cuts an arrangement with a mid-row cell spanning two lines into side blocks', () => {
        const layout = tableLines({
            columns: COLS,
            width: 400,
            breakpoint: 'mobile',
            oneLineFits: false,
            layouts: [{ width: 'mobile', tracks: 3, cells: [{ col: 'account', line: 1 }, { col: 'number', line: 1, start: 2, lines: 2, justify: 'center' }, { col: 'debit', line: 1, start: 3 }, { col: 'credit', line: 2, start: 3 }] }],
        })
        expect(layout).toEqual({
            kind: 'blocks',
            root: {
                kind: 'side',
                of: 3,
                parts: [
                    { block: { kind: 'stack', parts: [{ kind: 'cell', col: 'account', justify: 'start' }, { kind: 'gap' }] }, tracks: 1 },
                    { block: { kind: 'cell', col: 'number', justify: 'center' }, tracks: 1 },
                    { block: { kind: 'stack', parts: [{ kind: 'cell', col: 'debit', justify: 'end' }, { kind: 'cell', col: 'credit', justify: 'end' }] }, tracks: 1 },
                ],
            },
        })
    })
    it('puts a label the arrangement leaves out on a line of its own', () => {
        const layout = tableLines({ columns: COLS, width: 400, breakpoint: 'mobile', oneLineFits: false, layouts: [{ width: 'mobile', tracks: 2, cells: [{ col: 'debit', line: 1, span: 2 }] }] })
        expect(layout).toEqual({ kind: 'blocks', root: { kind: 'stack', parts: [{ kind: 'cell', col: 'account', justify: 'start' }, { kind: 'cell', col: 'debit', justify: 'end' }] } })
    })
    it('a row with no figures draws only its label', () => {
        const layout = tableLines({ columns: COLS, width: 390, breakpoint: 'mobile', layouts: [], oneLineFits: false })
        const row = { id: 'g', kind: 'group' as const, depth: 0, label: 'Assets', cells: {} }
        expect(rowLayout({ row, cells: {}, isToggle: true, isOpen: false }, layout, 'account')).toEqual({ kind: 'blocks', root: { kind: 'cell', col: 'account', justify: 'start' } })
    })
    it('starts an arrangement from the columns', () => {
        expect(startingLayout('mobile', [{ id: 'a' }, { id: 'b' }, { id: 'c' }])).toEqual({ width: 'mobile', tracks: 3, cells: [{ col: 'a', line: 1, span: 3 }, { col: 'b', line: 2 }, { col: 'c', line: 2 }] })
    })
})
```

In `report-table-layout.test.ts`: `fitsOnOneLine({ breakpoint: 'mobile', amountCount: 2, width: 390 })` is true (168 + 2 × 108 = 384), with 3 amount columns false; on tablet `220 + n × 128 ≤ width`.

- [ ] **Step 2: Run them to see them fail**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports`
Expected: FAIL: cannot resolve `../table-lines`.

- [ ] **Step 3: `lib/table-lines.ts`**

```ts
import type { Breakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { isRightAligned } from './format'
import type { VisibleRow } from './rows'
import type { DisplayCell, DisplayLayout, DisplayWidth } from './spec'
import type { ResultColumn } from './types'

// E14: a report row wraps onto more lines where its columns do not fit, after
// Gridley: one cell renderer per column, and per width a layout that puts each
// column on a line, at a start track, spanning tracks and lines, with an
// alignment. React Native has no CSS grid on native, so a layout is cut into a
// tree of blocks (guillotine cuts between lines or between tracks) that flex
// draws: a stack of blocks top to bottom, or blocks side by side whose widths
// are their shares of the tracks. Rows measure themselves (R28).

export const AUTO_TRACK_MIN = 104
const MAX_LINES = 8
export const PINWHEEL_MESSAGE = "These cells overlap in a way that can't be laid out; move one cell."

export type Justify = 'start' | 'center' | 'end'
export type Block =
    | { kind: 'cell'; col: string; justify: Justify }
    | { kind: 'gap' }
    | { kind: 'stack'; parts: Block[] }
    | { kind: 'side'; of: number; parts: { block: Block; tracks: number }[] }
export type LineLayout = { kind: 'one-line' } | { kind: 'blocks'; root: Block }
export type Placed = { start: number; span: number; line: number; lines: number }

type Box = { x0: number; x1: number; y0: number; y1: number } // tracks [x0, x1), lines [y0, y1), from 0
type Piece = { index: number; x0: number; x1: number; y0: number; y1: number }

const pieceOf = (p: Placed, index: number): Piece => ({ index, x0: p.start - 1, x1: p.start - 1 + p.span, y0: p.line - 1, y1: p.line - 1 + p.lines })

// A straight cut of the box that separates its pieces into two non-empty sides,
// lines first, or null when none exists (a pinwheel).
function cut(pieces: readonly Piece[], box: Box): { across: boolean; at: number } | null {
    for (let y = box.y0 + 1; y < box.y1; y++) {
        if (pieces.every(p => p.y1 <= y || p.y0 >= y) && pieces.some(p => p.y1 <= y) && pieces.some(p => p.y0 >= y)) return { across: true, at: y }
    }
    for (let x = box.x0 + 1; x < box.x1; x++) {
        if (pieces.every(p => p.x1 <= x || p.x0 >= x) && pieces.some(p => p.x1 <= x) && pieces.some(p => p.x0 >= x)) return { across: false, at: x }
    }
    return null
}

/** The first piece of a part no straight cut separates, or null when the pieces can be cut down to one each. */
function uncuttable(pieces: readonly Piece[], box: Box): Piece | null {
    if (pieces.length <= 1) return null
    const c = cut(pieces, box)
    if (!c) return [...pieces].sort((a, b) => a.index - b.index)[0] ?? null
    const [lo, hi] = c.across
        ? [pieces.filter(p => p.y1 <= c.at), pieces.filter(p => p.y0 >= c.at)]
        : [pieces.filter(p => p.x1 <= c.at), pieces.filter(p => p.x0 >= c.at)]
    const [boxLo, boxHi] = c.across ? [{ ...box, y1: c.at }, { ...box, y0: c.at }] : [{ ...box, x1: c.at }, { ...box, x0: c.at }]
    return uncuttable(lo, boxLo) ?? uncuttable(hi, boxHi)
}

/** The engine's placement rules (core/server/reports/bind.go placeCells, uncuttable). */
export function placeCells(tracks: number, cells: readonly DisplayCell[]): { placed: Placed[] } | { problem: { index: number; message: string } } {
    const taken = new Set<string>()
    const placed: Placed[] = []
    for (const [index, c] of cells.entries()) {
        const span = Math.max(c.span ?? 1, 1)
        const lines = Math.max(c.lines ?? 1, 1)
        const start0 = c.start ?? 0
        if (c.line < 1 || c.line > MAX_LINES) return { problem: { index, message: `line is 1 to ${MAX_LINES}` } }
        if (c.line + lines - 1 > MAX_LINES) return { problem: { index, message: `a cell ends by line ${MAX_LINES}` } }
        if (span > tracks) return { problem: { index, message: `span is 1 to ${tracks}` } }
        if (start0 < 0 || start0 > tracks || (start0 > 0 && start0 + span - 1 > tracks)) return { problem: { index, message: 'the cell does not fit its line' } }
        const free = (start: number) => {
            for (let line = c.line; line < c.line + lines; line++) for (let t = start; t < start + span; t++) if (taken.has(`${line}:${t}`)) return false
            return true
        }
        let start = start0
        if (start === 0) {
            for (let s = 1; s + span - 1 <= tracks && start === 0; s++) if (free(s)) start = s
            if (start === 0) return { problem: { index, message: `no room on line ${c.line}` } }
        } else if (!free(start)) return { problem: { index, message: 'the cell overlaps another' } }
        for (let line = c.line; line < c.line + lines; line++) for (let t = start; t < start + span; t++) taken.add(`${line}:${t}`)
        placed.push({ start, span, line: c.line, lines })
    }
    const pieces = placed.map(pieceOf)
    const lines = Math.max(0, ...pieces.map(p => p.y1))
    const stuck = uncuttable(pieces, { x0: 0, x1: tracks, y0: 0, y1: lines })
    if (stuck) return { problem: { index: stuck.index, message: PINWHEEL_MESSAGE } }
    return { placed }
}

/** A stack with nested stacks folded in; a stack of one block is that block. */
function stackOf(parts: Block[]): Block {
    const flat = parts.flatMap(p => (p.kind === 'stack' ? p.parts : [p]))
    return flat.length === 1 && flat[0] ? flat[0] : { kind: 'stack', parts: flat }
}

/** A part of a side block; a nested side block's parts fold into it (track counts are absolute). */
function sideParts(block: Block, tracks: number): { block: Block; tracks: number }[] {
    return block.kind === 'side' ? block.parts : [{ block, tracks }]
}

// The block tree of pieces inside a box: a piece that fills its box is a cell,
// an empty box a gap, anything else is cut (always possible after placeCells).
function build(pieces: readonly Piece[], box: Box, cellOf: (index: number) => Block): Block {
    if (pieces.length === 0) return { kind: 'gap' }
    const [only] = pieces
    if (pieces.length === 1 && only && only.x0 === box.x0 && only.x1 === box.x1 && only.y0 === box.y0 && only.y1 === box.y1) return cellOf(only.index)
    const edges = (axis: 'y' | 'x') => [...new Set(pieces.flatMap(p => (axis === 'y' ? [p.y0, p.y1] : [p.x0, p.x1])))].sort((a, b) => a - b)
    for (const y of edges('y')) {
        if (y <= box.y0 || y >= box.y1 || !pieces.every(p => p.y1 <= y || p.y0 >= y)) continue
        return stackOf([build(pieces.filter(p => p.y1 <= y), { ...box, y1: y }, cellOf), build(pieces.filter(p => p.y0 >= y), { ...box, y0: y }, cellOf)])
    }
    for (const x of edges('x')) {
        if (x <= box.x0 || x >= box.x1 || !pieces.every(p => p.x1 <= x || p.x0 >= x)) continue
        const left = build(pieces.filter(p => p.x1 <= x), { ...box, x1: x }, cellOf)
        const right = build(pieces.filter(p => p.x0 >= x), { ...box, x0: x }, cellOf)
        return { kind: 'side', of: box.x1 - box.x0, parts: [...sideParts(left, x - box.x0), ...sideParts(right, box.x1 - x)] }
    }
    // Unreachable after placeCells: a lone piece smaller than its box is cut at
    // its own edges above, and pieces with no cut are refused as a pinwheel.
    return { kind: 'gap' }
}

const justifyOf = (column: ResultColumn): Justify => (isRightAligned(column) ? 'end' : 'start')

function autoLines(columns: readonly ResultColumn[], width: number): LineLayout {
    const [label, ...amounts] = columns
    const tracks = Math.max(1, Math.floor(width / AUTO_TRACK_MIN))
    const lines: Block[] = label ? [{ kind: 'cell', col: label.id, justify: 'start' }] : []
    for (let i = 0; i < amounts.length; i += tracks) {
        const line = amounts.slice(i, i + tracks)
        const parts: { block: Block; tracks: number }[] = line.map(column => ({ block: { kind: 'cell', col: column.id, justify: justifyOf(column) }, tracks: 1 }))
        // A short last line keeps each amount on its own track.
        if (line.length < tracks) parts.push({ block: { kind: 'gap' }, tracks: tracks - line.length })
        lines.push({ kind: 'side', of: tracks, parts })
    }
    return { kind: 'blocks', root: stackOf(lines) }
}

// A pivot cell column's entry stands for all its pivot columns, one per track
// from that position, wrapping onto the lines below.
function expandPivots(layout: DisplayLayout, columns: readonly ResultColumn[]): DisplayCell[] {
    return layout.cells.flatMap(cell => {
        const pivots = columns.filter(c => c.pivot?.cell === cell.col)
        if (pivots.length === 0) return [cell]
        const first = (cell.start ?? 1) - 1
        return pivots.map((p, k) => ({ col: p.id, line: cell.line + Math.floor((first + k) / layout.tracks), start: ((first + k) % layout.tracks) + 1, justify: cell.justify }))
    })
}

function authoredLines(layout: DisplayLayout, columns: readonly ResultColumn[], width: number): LineLayout {
    const cells = expandPivots(layout, columns).filter(cell => columns.some(c => c.id === cell.col))
    const result = placeCells(layout.tracks, cells)
    if ('problem' in result) return autoLines(columns, width)
    const pieces = result.placed.map(pieceOf)
    const lines = Math.max(1, ...pieces.map(p => p.y1))
    const cellOf = (index: number): Block => {
        const cell = cells[index]
        const column = columns.find(c => c.id === cell?.col)
        return cell && column ? { kind: 'cell', col: cell.col, justify: cell.justify ?? justifyOf(column) } : { kind: 'gap' }
    }
    const tree = build(pieces, { x0: 0, x1: layout.tracks, y0: 0, y1: lines }, cellOf)
    // An arrangement that leaves out the label column puts the label on a line of
    // its own above it: a row is never without its label.
    const labelId = columns[0]?.id
    if (labelId && !cells.some(c => c.col === labelId)) return { kind: 'blocks', root: stackOf([{ kind: 'cell', col: labelId, justify: 'start' }, tree]) }
    return { kind: 'blocks', root: tree }
}

export function tableLines(input: {
    columns: readonly ResultColumn[]
    width: number
    breakpoint: Breakpoint
    layouts: readonly DisplayLayout[]
    oneLineFits: boolean
}): LineLayout {
    const authored = input.layouts.find(l => l.width === input.breakpoint)
    if (authored) return authoredLines(authored, input.columns, input.width)
    if (input.breakpoint === 'desktop' || input.oneLineFits) return { kind: 'one-line' }
    return autoLines(input.columns, input.width)
}

/** A row with no figures (a group its subtotal follows) draws only its label, across the row. */
export function rowLayout(item: VisibleRow, layout: LineLayout, labelId: string): LineLayout {
    if (layout.kind === 'one-line' || Object.keys(item.cells).length > 0) return layout
    return { kind: 'blocks', root: { kind: 'cell', col: labelId, justify: 'start' } }
}

/** A first arrangement: the first column across line 1, the others in order on the lines below. */
export function startingLayout(width: DisplayWidth, columns: readonly { id: string }[], tracks = 3): DisplayLayout {
    const [first, ...rest] = columns
    const cells: DisplayCell[] = first ? [{ col: first.id, line: 1, span: tracks }] : []
    rest.forEach((c, k) => cells.push({ col: c.id, line: 2 + Math.floor(k / tracks) }))
    return { width, tracks, cells }
}
```

`report-table-layout.ts`:

```ts
/** The label column and the amount columns fit one line at this width (E14). */
export function fitsOnOneLine(input: { breakpoint: Breakpoint; amountCount: number; width: number }): boolean {
    const { breakpoint, amountCount, width } = input
    if (breakpoint === 'mobile') return PHONE_LABEL_WIDTH + amountCount * PHONE_AMOUNT_WIDTH <= width
    return LABEL_MIN_WIDTH + amountCount * AMOUNT_WIDTH <= width
}
```

`draft.ts` `draftIssues`: for each `draft.display.layouts[i]`, a `placeCells` problem becomes `{ path: ['display', 'layouts', i, 'cells', problem.index], message }`.

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: PASS. (The auto-layout test expects no trailing gap at 3 tracks with 3 amounts; the mid-row test's left block is `[account, gap]` because line 2 of track 1 is empty.)

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/reports
git commit -m "feat(core): report line layouts per width, cut into blocks"
git push origin books/p5-trial-balance
```

---

### Task 25: E14 (part 2) — the table on FlashList: flex lines, sticky headings and groups, sideways scroll; the Arrange section; e2e

Code facts used: today's table (two stacks and a custom row window, `components/ReportTable.tsx:113-190`, `components/use-row-window.ts`), the row parts (`components/ReportRow.tsx`), `useReportRun.loadMore`'s guard (`hooks/useReportRun.ts:64-65`: `hasNextPage && !isFetchingNextPage`); FlashList v2.3.3 (the `nathanstitt/flash-list` fork pinned at `core/package-versions.json:30`, used on web by `core/ui/emoji-picker/EmojiPickerGrid.tsx`): `stickyHeaderIndices` with one sticky item at a time, pushed by the next (`node_modules/@shopify/flash-list/src/recyclerview/components/StickyHeaders.tsx:80-150`), `renderItem` called with `target: 'StickyHeader'` for the stuck copy (`src/FlashListProps.ts:18-34`), `stickyHeaderConfig.useNativeDriver` (`FlashListProps.ts:426-466`), `onEndReached` checked after every commit and re-armed when `data` changes (`src/recyclerview/hooks/useBoundDetection.ts:96-108,165-170`, `RecyclerView.tsx:640`), web support (`src/isNewArch.ts:19`, `src/recyclerview/utils/measureLayout.web.ts`, `src/native/config/PlatformHelper.web.ts`: `supportsOffsetCorrection: false`).

**Files:**
- Modify: `core/reports/components/ReportTable.tsx` (FlashList), `ReportRow.tsx` (`TableRow`, `LinesRow`, `StickyGroup`), `ReportView.tsx` (passes `layouts`)
- Delete: `core/reports/components/use-row-window.ts`, `core/reports/components/__tests__/use-row-window.test.ts`
- Create: `core/reports/lib/sticky.ts`, `core/reports/lib/__tests__/sticky.test.ts`
- Modify: `core/reports/hooks/useReportView.ts` (`layouts`), `screens/ReportScreen.tsx` (passes `record.spec?.display?.layouts` or `builder.previewSpec?.display?.layouts`)
- Create: `core/reports/components/builder/ArrangeSection.tsx`; modify `ReportBuilderPanel.tsx`
- Create: `tests/e2e/reports-wrap.spec.ts`

**Interfaces:**
- Consumes: Task 24.
- Produces: `stickyIndices(rows: VisibleRow[]): number[]` (every open group row); `groupChain(rows, index): ResultRow[]` (the open groups enclosing a row, outermost first); `ReportTable` prop `layouts`; unchanged test ids `report-row-<id>`, `report-cell-<col>-<row>`, `report-heading-<col>`, `report-heading-row`, `report-table`, `report-loading-more`; new `report-sticky-group` (the stuck group stack).

- [ ] **Step 1: Write the failing tests**

Create `core/reports/lib/__tests__/sticky.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { groupChain, stickyIndices } from '../sticky'
import type { VisibleRow } from '../rows'
import type { ResultRow } from '../types'

const row = (id: string, kind: ResultRow['kind'], extra: Partial<ResultRow> = {}): VisibleRow => ({
    row: { id, kind, depth: 0, label: id, cells: {}, ...extra },
    cells: {},
    isToggle: kind === 'group',
    isOpen: extra.expanded === true,
})

const ROWS = [
    row('g:a', 'group', { expanded: true }),
    row('g:a1', 'group', { expanded: true, section: 'g:a', depth: 1 }),
    row('d:1', 'detail', { section: 'g:a1', depth: 2 }),
    row('g:b', 'group'),
    row('total', 'total'),
]

describe('sticky groups', () => {
    it('sticks every open group, at any depth', () => {
        expect(stickyIndices(ROWS)).toEqual([0, 1])
    })
    it('stacks the open groups that enclose the stuck one, outermost first', () => {
        expect(groupChain(ROWS, 1).map(r => r.id)).toEqual(['g:a', 'g:a1'])
        expect(groupChain(ROWS, 0).map(r => r.id)).toEqual(['g:a'])
    })
})
```

Create `tests/e2e/reports-wrap.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import { addColumn, addSale, addTeam, chooseOption, openReportStub, openReportsHub, rowOf, SALES_SOURCE, saveNewReport, startReport, uniqueName } from './reports-helpers'

/**
 * E14: on a phone a row too wide for one line wraps (the label on line 1, the
 * figures aligned on line 2 under headings that wrap the same way) and nothing
 * scrolls sideways; an open group's row sticks while its rows scroll under it.
 * Desktop keeps one line per row and the heading row stays at the top. An author
 * arrangement for phones places columns on chosen lines.
 */

async function seed(p: Page, sales: number) {
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    for (let i = 0; i < sales; i++) {
        await addSale(p, { team, customer: `Wrapco ${String(i).padStart(2, '0')}`, amount: 1000 + i, cost: 400, quantity: 3, day: '2026-01-05', status: i % 2 === 0 ? 'open' : 'paid' })
    }
}

async function wideReport(p: Page, title: string, grouped: boolean) {
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await addColumn(p, ['cost'])
    await addColumn(p, ['quantity'])
    if (grouped) {
        await addColumn(p, ['status'])
        await p.getByTestId('report-builder-section-group').click()
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-0-col', 'status')
        await p.getByTestId('report-builder-group-0-open').click()
    }
    await saveNewReport(p, title)
}

async function boxes(p: Page, row: string) {
    const label = rowOf(p, row)
    const id = (await label.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
    const box = async (testId: string) => p.getByTestId(testId).boundingBox()
    return {
        label: await label.boundingBox(),
        amount: await box(`report-cell-amount-${id}`),
        cost: await box(`report-cell-cost-${id}`),
        headLabel: await box('report-heading-customer'),
        headAmount: await box('report-heading-amount'),
        headCost: await box('report-heading-cost'),
    }
}

test.describe('report rows per width', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('on a phone a wide row wraps under aligned wrapped headings, with no sideways scroll, and an open group sticks', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptwrap')
        try {
            await seed(p, 24)
            await wideReport(p, uniqueName('Wrap'), true)
            await p.setViewportSize({ width: 390, height: 844 })
            await expect.poll(async () => {
                const b = await boxes(p, 'Wrapco 00')
                return b.label && b.amount ? b.amount.y >= b.label.y + b.label.height - 1 : false
            }).toBe(true)
            const b = await boxes(p, 'Wrapco 00')
            expect(Math.abs((b.headAmount?.x ?? 0) - (b.amount?.x ?? -9))).toBeLessThanOrEqual(1)
            expect(Math.abs((b.headCost?.x ?? 0) - (b.cost?.x ?? -9))).toBeLessThanOrEqual(1)
            expect((b.headAmount?.y ?? 0) > (b.headLabel?.y ?? 0)).toBe(true)
            expect(await p.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)

            // Scroll the open group's rows: its row stays stuck under the heading row.
            const heading = await p.getByTestId('report-heading-row').boundingBox()
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 900)
            const sticky = p.getByTestId('report-sticky-group')
            await expect(sticky).toBeVisible()
            await expect(sticky).toContainText('Open')
            await expect.poll(async () => Math.abs(((await sticky.boundingBox())?.y ?? 0) - ((heading?.y ?? 0) + (heading?.height ?? 0)))).toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })

    test('on a desktop a row stays one line and the heading row stays at the top', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptline')
        try {
            await seed(p, 24)
            await wideReport(p, uniqueName('Line'), false)
            await expect.poll(async () => {
                const b = await boxes(p, 'Wrapco 00')
                return b.label && b.amount ? Math.abs(b.label.y - b.amount.y) : 99
            }).toBeLessThanOrEqual(1)
            const before = await p.getByTestId('report-heading-row').boundingBox()
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 900)
            await expect(rowOf(p, 'Wrapco 00')).toBeHidden()
            await expect.poll(async () => (await p.getByTestId('report-heading-row').boundingBox())?.y).toBe(before?.y)
        } finally {
            await close()
        }
    })

    test('an author arrangement for phones puts a cell spanning two lines in the middle of the row', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptarr')
        try {
            await seed(p, 1)
            await wideReport(p, uniqueName('Arrange'), false)
            await p.getByTestId('report-edit-open').click()
            await p.getByTestId('report-builder-section-arrange').click()
            await p.getByTestId('report-builder-arrange-width-mobile').click()
            await p.getByTestId('report-builder-arrange-on').click()
            await chooseOption(p, 'report-builder-arrange-tracks', '3')
            // Line 1: customer · quantity · amount; line 2: (empty) · quantity · cost.
            await chooseOption(p, 'report-builder-arrange-customer-span', '1')
            await chooseOption(p, 'report-builder-arrange-amount-line', '1')
            await chooseOption(p, 'report-builder-arrange-amount-start', '3')
            await chooseOption(p, 'report-builder-arrange-cost-line', '2')
            await chooseOption(p, 'report-builder-arrange-cost-start', '3')
            await chooseOption(p, 'report-builder-arrange-quantity-line', '1')
            await chooseOption(p, 'report-builder-arrange-quantity-start', '2')
            await chooseOption(p, 'report-builder-arrange-quantity-lines', '2')
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder')).toBeHidden()
            await p.setViewportSize({ width: 390, height: 844 })
            const id = async () => (await rowOf(p, 'Wrapco 00').getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
            await expect.poll(async () => {
                const b = await boxes(p, 'Wrapco 00')
                const quantity = await p.getByTestId(`report-cell-quantity-${await id()}`).boundingBox()
                if (!b.label || !b.amount || !b.cost || !quantity) return false
                const between = quantity.x > b.label.x && quantity.x + quantity.width <= b.amount.x + 1
                const spans = quantity.y <= b.amount.y + 1 && quantity.y + quantity.height >= b.cost.y + b.cost.height - 1
                return Math.abs(b.label.y - b.amount.y) <= 1 && b.cost.y > b.amount.y && between && spans
            }).toBe(true)
        } finally {
            await close()
        }
    })
})
```

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test -- core/reports/lib/__tests__/sticky.test.ts`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-wrap.spec.ts`
Expected: FAIL (no `../sticky`; on the phone the amount cell sits on the label's line).

- [ ] **Step 2: `lib/sticky.ts`**

```ts
import type { VisibleRow } from './rows'
import type { ResultRow } from './types'

// FlashList keeps one sticky item at a time, pushed up by the next (R26). Every
// open group row, at any depth, is a sticky index, so the stuck item is the
// innermost open group above the top; it is drawn with the open groups that
// enclose it stacked above it, so nested levels read as a stack.

export function stickyIndices(rows: readonly VisibleRow[]): number[] {
    return rows.flatMap((item, index) => (item.row.kind === 'group' && item.isOpen ? [index] : []))
}

export function groupChain(rows: readonly VisibleRow[], index: number): ResultRow[] {
    const byId = new Map(rows.map(item => [item.row.id, item.row]))
    const chain: ResultRow[] = []
    let current: ResultRow | undefined = rows[index]?.row
    while (current) {
        chain.unshift(current)
        current = current.section ? byId.get(current.section) : undefined
    }
    return chain
}
```

Folding changes the rows, so the indices are recomputed from the visible rows on every render and FlashList re-reads them.

- [ ] **Step 3: Rows drawn with flex lines**

In `ReportRow.tsx`, keep `ReportRowLabel`'s label (renamed `RowLabel`, same `report-row-<id>` test id, caret, `aria-expanded`, accessibility label, indent `paddingLeft: INDENT + row.depth * INDENT` — child rows indent the label only, so figures stay aligned) and `AmountCell`'s view (`RowCell`, same `report-cell-<col>-<row>` id), now sized by their parent: `minHeight: ROW_HEIGHT`, `flex` from their slot. Add:

```tsx
const JUSTIFY: Record<Justify, 'flex-start' | 'center' | 'flex-end'> = { start: 'flex-start', center: 'center', end: 'flex-end' }

type RenderCell = (col: string) => ReactNode

/**
 * A row (or the heading) as its block tree (lib/table-lines.ts), drawn with flex:
 * a stack is a column of blocks, a side block a row of blocks as wide as their
 * share of the tracks. A side row stretches its blocks to one height, so a cell
 * that spans lines is as tall as the lines beside it; justify aligns each cell.
 * Heights come from the content: FlashList measures the row.
 */
export function BlockView({ block, renderCell }: { block: Block; renderCell: RenderCell }) {
    if (block.kind === 'gap') return null
    if (block.kind === 'cell') return <CellSlot block={block} renderCell={renderCell} />
    if (block.kind === 'stack') return <StackBlock parts={block.parts} renderCell={renderCell} />
    return <SideBlock of={block.of} parts={block.parts} renderCell={renderCell} />
}

function CellSlot({ block, renderCell }: { block: Extract<Block, { kind: 'cell' }>; renderCell: RenderCell }) {
    return <View style={{ flex: 1, alignItems: JUSTIFY[block.justify], justifyContent: 'center' }}>{renderCell(block.col)}</View>
}

function StackBlock({ parts, renderCell }: { parts: Block[]; renderCell: RenderCell }) {
    const items = parts.map((part, i) => <BlockView key={blockKey(part, i)} block={part} renderCell={renderCell} />)
    return <View className="flex-1">{items}</View>
}

function SideBlock({ of, parts, renderCell }: { of: number; parts: { block: Block; tracks: number }[]; renderCell: RenderCell }) {
    const items = parts.map((part, i) => (
        <View key={blockKey(part.block, i)} style={{ width: `${(part.tracks / of) * 100}%` }}>
            <BlockView block={part.block} renderCell={renderCell} />
        </View>
    ))
    return <View className="flex-row">{items}</View>
}

/** A block's key: its first cell's column, or its place. */
function blockKey(block: Block, index: number): string {
    if (block.kind === 'cell') return block.col
    if (block.kind === 'stack') return block.parts[0] ? blockKey(block.parts[0], index) : `stack-${index}`
    if (block.kind === 'side') return block.parts[0] ? blockKey(block.parts[0].block, index) : `side-${index}`
    return `gap-${index}`
}
```

(`BlockView`'s kind checks are early returns, not JSX ternaries.) `TableRow({ item, columns, layout, format, onToggleGroup, labelId })`: in one-line mode it draws the label then the amount cells in a flex row (label `flex: 1, minWidth` from `report-table-layout`, amount cells `width: layout.amountWidth`); in blocks mode it draws `<BlockView block={rowLayout(item, layout, labelId).root} renderCell={…} />` (the label column's cell is the `RowLabel`, every other a `RowCell`; the `renderCell` function is built above the JSX). The row's container carries `ROW_RULE[row.kind]`, so a subtotal or total rule runs across the whole wrapped row; a group row's figures take the same press as its label (a `Pressable` with `accessible={false}`, `tabIndex={-1}`), as `ReportRowAmounts` does. `StickyGroup({ rows, index, … })` draws `groupChain(rows, index)` as a column of label lines (each a `RowLabel`, so a press folds that group), `testID="report-sticky-group"`, `bg-background border-b border-border`.

- [ ] **Step 4: The table on FlashList**

`ReportTable.tsx`:

```tsx
export interface ReportTableProps {
    columns: ResultColumn[]
    rows: VisibleRow[]
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
// asks for the next one too.
export function ReportTable({ columns, rows, format, breakpoint, layouts, onToggleGroup, onNearEnd, isLoadingMore }: ReportTableProps) {
    const windowWidth = useWindowWidth()
    // The table's own width (a builder drawer narrows it): local layout state from onLayout.
    const [width, setWidth] = useState(windowWidth)
    const scrollX = useRef(new Animated.Value(0)).current
    const [labelColumn, ...amountColumns] = columns
    const oneLine = reportTableLayout({ breakpoint, amountCount: amountColumns.length })
    const layout = tableLines({ columns, width, breakpoint, layouts, oneLineFits: fitsOnOneLine({ breakpoint, amountCount: amountColumns.length, width }) })
    const sticky = stickyIndices(rows)
    if (!labelColumn) return null
    const renderItem = ({ item, index, target }: ListRenderItemInfo<VisibleRow>) =>
        target === 'StickyHeader' ? (
            <StickyGroup rows={rows} index={index} onToggleGroup={onToggleGroup} scrollX={scrollX} />
        ) : (
            <TableRow item={item} columns={amountColumns} labelId={labelColumn.id} layout={layout} oneLine={oneLine} format={format} onToggleGroup={onToggleGroup} scrollX={scrollX} />
        )
    const sideways = layout.kind === 'one-line' && oneLine.scrollsSideways
    return (
        <View testID="report-table" className="min-h-0 flex-1" onLayout={e => setWidth(e.nativeEvent.layout.width)}>
            <Sideways isEnabled={sideways} minWidth={oneLine.labelMin + oneLine.amountContentMinWidth} scrollX={scrollX}>
                <HeadingRow labelColumn={labelColumn} amountColumns={amountColumns} layout={layout} oneLine={oneLine} scrollX={scrollX} />
                <FlashList
                    data={rows}
                    renderItem={renderItem}
                    keyExtractor={item => item.row.id}
                    getItemType={item => item.row.kind}
                    extraData={{ layout, format }}
                    stickyHeaderIndices={sticky}
                    stickyHeaderConfig={{ useNativeDriver: Platform.OS !== 'web' }}
                    onEndReached={onNearEnd}
                    onEndReachedThreshold={0.5}
                    ListFooterComponent={<LoadingMore isVisible={isLoadingMore} />}
                />
            </Sideways>
        </View>
    )
}
```

(The `renderItem` ternary becomes a small `TableItem` component; `reportTableLayout` gains `labelMin`.) The heading row is outside the FlashList and above it, so it stays at the top while the rows scroll (sticky by construction); in blocks mode it is `<BlockView block={layout.root} renderCell={headingCell} />`, the same blocks as the rows, so each heading sits over its values.

`Sideways` is the desktop sideways scroll (one line per row, wide pivots): a horizontal `ScrollView` whose content has the table's minimum width and holds the heading row and the FlashList together, so both move sideways in step; the list scrolls vertically inside it. Its `onScroll` drives `scrollX` (`Animated.event([{ nativeEvent: { contentOffset: { x: scrollX } } }], { useNativeDriver: false })`), and the label cells of the heading, the rows and the stuck group take `transform: [{ translateX: scrollX }]`, so the label column stays frozen at the left as today's two-stack table kept it (R27). When `isEnabled` is false it renders its children in a plain `View`.

`ReportView` passes `layouts={model.layouts}`; `useReportView` returns `layouts: input.layouts ?? []`, which `ReportScreen` fills from `record.spec?.display?.layouts` (saved) or `builder.previewSpec?.display?.layouts` (draft). Delete `use-row-window.ts` and its test (FlashList replaces them; `isNearEnd` moves to `ParamPickers.tsx`, its only other user, as a local helper).

- [ ] **Step 5: The Arrange section**

`ArrangeSection.tsx` (title "Arrange", `section="arrange"`): the chosen width tab lives in the reports UI store (`arrangeWidth`, shared by the section and the preview hint) and is picked with three buttons (`report-builder-arrange-width-mobile|tablet|desktop`: Phone, Tablet, Desktop); a `Switch` "Arrange columns for this width" (`report-builder-arrange-on`: on adds `startingLayout(width, columns)`, off removes that width's layout, through `builder.setDisplay`); a `PickMenu` "Columns per line" (`report-builder-arrange-tracks`, 1–12; cells wider than the new count shrink to it); and one row per column with a "Show" switch (`…-<col>-show`: off removes its cell) and `PickMenu`s Line (1–8), Start (Auto, 1…tracks), Span (1…tracks), Lines (1–8) and Align (Start, Center, End) (`…-<col>-line|start|span|lines|justify`). A cell may span lines anywhere a straight cut can free it (R28); a pinwheel arrangement shows "These cells overlap in a way that can't be laid out; move one cell." on the cell `draftIssues` names. Placement problems show under the section (`problemsFor('arrange', …)`), and Save refuses them as the server does. Add `<ArrangeSection builder={builder} problems={problems} />` after `LayoutSection` in `ReportBuilderPanel.tsx`.

- [ ] **Step 6: Run**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-wrap.spec.ts tests/e2e/reports-view.spec.ts tests/e2e/reports-group-pivot.spec.ts tests/e2e/reports-formula-layout.spec.ts tests/e2e/reports-groups-v2.spec.ts`
Expected: all pass (cells keep their test ids; the paging spec in `reports-view.spec.ts` loads its next page through `onEndReached`).

- [ ] **Step 7: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add -A core/reports tests/e2e/reports-wrap.spec.ts
git commit -m "feat(core): the report table on FlashList: wrapped rows, sticky headings and groups"
git push origin books/p5-trial-balance
```

---

### Task 25a: The FlashList fork corrects the scroll offset on web

Code facts used (the pinned fork, `node_modules/@shopify/flash-list/src`, commit `84a89f4253533633c86a2ce582a5ccfa52f50688`): `useRecyclerViewController.applyOffsetCorrection` (`recyclerview/hooks/useRecyclerViewController.tsx:119-240`) measures how far the first visible item moved and, where `PlatformConfig.supportsOffsetCorrection` is false (web, `native/config/PlatformHelper.web.ts`), calls `scrollViewRef.current?.scrollTo({ y: getAbsoluteLastScrollOffset() + diff })` instead of moving the `ScrollAnchor` that native's `maintainVisibleContentPosition` follows; the fork runs `jest` (`package.json` `"test": "jest"`) and has a web fixture (`fixture/web`, `fixture/expo`).

Why the content still jumps on web (the root causes the tests below pin; confirm each by the failing test before the fix): (1) browsers anchor scrolling themselves (CSS `overflow-anchor: auto`, the default in Chrome and Firefox), so when a row above the view grows the browser already moves `scrollTop`, and FlashList's `scrollTo(last + diff)` moves it again or from a stale value; (2) `getAbsoluteLastScrollOffset()` is the last offset a scroll event reported, which on web can lag the DOM's `scrollTop`, so `last + diff` lands off by the lag. The fix: on web, turn the browser's own anchoring off on the list's scroller (`overflowAnchor: 'none'`) so FlashList is the only one correcting, and correct relatively from the DOM's current `scrollTop` (`node.scrollTop += diff`), synchronously in the layout effect, before paint.

**Files (in the fork clone `/Users/nas/tmp/tinycld/books-purchases-flash-list`):**
- Modify: `src/native/config/PlatformHelper.web.ts` (`offsetCorrection: "scrollTop"`; the other platform files gain `"anchor"`/`"scrollTo"` matching today's behaviour), `src/recyclerview/hooks/useRecyclerViewController.tsx` (the web branch), `src/recyclerview/RecyclerView.tsx` (the web scroller's `overflowAnchor: 'none'` style)
- Create: `src/recyclerview/utils/webOffsetCorrection.ts`, `src/__tests__/webOffsetCorrection.test.ts` (and the render test of Step 2)

- [ ] **Step 1: Clone the fork and branch from the pinned commit**

```bash
git clone git@github.com:nathanstitt/flash-list.git /Users/nas/tmp/tinycld/books-purchases-flash-list
cd /Users/nas/tmp/tinycld/books-purchases-flash-list
git checkout -b books/p5-trial-balance 84a89f4253533633c86a2ce582a5ccfa52f50688
yarn
yarn test
```
Expected: the fork's own suite passes before any change (a failure here is fixed at its root first).

- [ ] **Step 2: Read the fork's own offset tests, then write the failing tests**

Read how the fork already tests offset correction and web, and follow that file's tools and mocks (F20); do not invent a harness:

```bash
cd /Users/nas/tmp/tinycld/books-purchases-flash-list
grep -rlE "applyOffsetCorrection|maintainVisibleContentPosition|PlatformHelper|scrollTo" src | grep -E "__tests__|\.test\." | head
```

Two tests, the first pure (it pins both root causes without a renderer), the second following the closest file the grep found:

1. Move the web correction into a pure function, `src/recyclerview/utils/webOffsetCorrection.ts`, and test it in `src/__tests__/webOffsetCorrection.test.ts`:

```ts
import { correctWebOffset, WEB_SCROLLER_STYLE } from "../recyclerview/utils/webOffsetCorrection";

describe("web offset correction", () => {
  it("moves the DOM scroller by the difference from where it is now", () => {
    const node = { scrollTop: 900, scrollLeft: 0 };
    // The last scroll event said 850; the DOM is at 900. The result must not use 850.
    correctWebOffset(node, 100, false);
    expect(node.scrollTop).toBe(1000);
  });
  it("moves sideways for a horizontal list", () => {
    const node = { scrollTop: 0, scrollLeft: 40 };
    correctWebOffset(node, -10, true);
    expect(node.scrollLeft).toBe(30);
  });
  it("turns the browser's own scroll anchoring off on the scroller", () => {
    expect(WEB_SCROLLER_STYLE).toEqual({ overflowAnchor: "none" });
  });
});
```

2. In the file the grep found closest (or a new `src/__tests__/webOffsetCorrection.render.test.tsx` built with the same render helper and mocks it uses), with web's `PlatformConfig` (`jest.mock("../native/config/PlatformHelper", () => jest.requireActual("../native/config/PlatformHelper.web"))`): render a vertical `FlashList` of 50 items, scroll so item 20 is first, grow item 5 by 100px (its measured layout mocked as that file mocks layouts), and assert the scroller's `scrollTop` grew by exactly 100 once (not 0, not 200) and that `scrollTo` was not called. If that file's harness cannot drive a DOM `scrollTop` (react-native-web is not under jest there), the pure test above and the web e2e in Task 25d are the proof, and the PR body says so; write nothing that mocks the code under test.

Run: `cd /Users/nas/tmp/tinycld/books-purchases-flash-list && yarn test src/__tests__/webOffsetCorrection`
Expected: FAIL (`webOffsetCorrection` does not exist; the render test sees `scrollTo` from the stale offset).

- [ ] **Step 3: Implement**

`PlatformHelper.web.ts`: add `offsetCorrection: "scrollTop" as const` (and `"anchor"` in the iOS and Android helpers, `"scrollTo"` in the default one, so other platforms keep their behaviour). In `applyOffsetCorrection`'s non-anchor branch:

```ts
            if (PlatformConfig.supportsOffsetCorrection) {
              scrollAnchorRef.current?.scrollBy(diff);
            } else if (PlatformConfig.offsetCorrection === "scrollTop") {
              // Web: move the DOM scroller by the difference from where it is
              // now, before paint. The browser's own scroll anchoring is off on
              // this scroller (overflow-anchor: none), so this is the only
              // correction, and the last scroll event's offset can be stale.
              const node = getScrollableNode(scrollViewRef.current);
              if (node) correctWebOffset(node, diff, horizontal);
            } else {
              /* today's scrollTo branch, unchanged */
            }
```

where `src/recyclerview/utils/webOffsetCorrection.ts` is:

```ts
/** The browser's own scroll anchoring would correct a second time; FlashList corrects alone. */
export const WEB_SCROLLER_STYLE = { overflowAnchor: "none" } as const;

/** Moves the DOM scroller by diff from where it is now (not from the last scroll event). */
export function correctWebOffset(node: { scrollTop: number; scrollLeft: number }, diff: number, horizontal: boolean): void {
  if (horizontal) node.scrollLeft += diff;
  else node.scrollTop += diff;
}
```

with `getScrollableNode` reading `scrollViewRef.current?.getScrollableNode?.()` (react-native-web's ScrollView exposes it). In `RecyclerView.tsx`, the scroll component's style gains `Platform.OS === "web" ? WEB_SCROLLER_STYLE : undefined` (typed through the web style cast the fork already uses for web-only styles). The `hasDataChanged` handling after the correction stays as it is.

- [ ] **Step 4: Run the fork's checks**

Run: `cd /Users/nas/tmp/tinycld/books-purchases-flash-list && yarn test && yarn type-check && yarn lint && yarn build`
Expected: all pass, the new test included.

- [ ] **Step 5: Commit on the fork branch**

```bash
cd /Users/nas/tmp/tinycld/books-purchases-flash-list
git add src
git commit -m "fix(web): correct the scroll offset from the DOM scroller, without the browser's own anchoring"
```

---

### Task 25b: Push the fork branch and open its PR

**Files:** none in `tinycld`.

- [ ] **Step 1: Push and open the PR on the fork**

```bash
cd /Users/nas/tmp/tinycld/books-purchases-flash-list
git push -u origin books/p5-trial-balance
gh pr create --repo nathanstitt/flash-list --base main --head books/p5-trial-balance \
  --title "fix(web): keep the visible rows in place when rows above change height" \
  --body "On web, rows that change height above the viewport (a group opening above, a live update adding rows) made the content jump: the browser's own scroll anchoring and FlashList's correction both moved scrollTop, from a stale offset. This turns the browser's anchoring off on the list's scroller and corrects from the DOM scroller's current scrollTop before paint. Tested in src/__tests__/webOffsetCorrection.test.ts."
```
Expected: the PR URL (the fork's default branch is the base; if it is not `main`, use the branch `git remote show origin` names).

- [ ] **Step 2: Note the PR**

Write the PR URL into this plan's ledger entry for checkpoint 0 (`books/docs/plans/trial-balance-ledger.md`, "Checkpoint 0" section) as "flash-list fork PR: <url> — awaiting the human's review".

---

### Task 25c: STOP — the human reviews and merges the fork PR

**Files:** none.

- [ ] **Step 1: Stop and ask**

Stop here. Report the fork PR URL from Task 25b and ask the human to review and merge it. Do not pin an unmerged commit. Continue with Task 25d only after the human says the PR is merged, and record the merge commit's SHA they give (or read it with `gh pr view <url> --repo nathanstitt/flash-list --json mergeCommit`).

---

### Task 25d: Pin the merged fork; install; a web e2e that a group opening above keeps the rows in place

Code facts used: the pin lives in `core/package-versions.json:30` (the source of truth the workspace-root install transcribes into `overrides`) and in `core/package.json:74`; after a pull that changes `core/package-versions.json`, `pnpm run packages:generate` in `tinycld` and then `pnpm install` at the workspace root again (memory: the first install reads the old root copy).

**Files:**
- Modify: `core/package-versions.json:30`, `core/package.json:74` (`github:nathanstitt/flash-list#<merged SHA>`)
- Create: `tests/e2e/reports-scroll-anchor.spec.ts`

- [ ] **Step 1: Pin the merged commit**

Replace `84a89f4253533633c86a2ce582a5ccfa52f50688` with the merged SHA in both files.

- [ ] **Step 2: Install and regenerate (this task is where the plan runs the workspace install)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases && pnpm install
cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH pnpm run packages:generate
cd /Users/nas/tmp/tinycld/books-purchases && pnpm install
node -e "console.log(require('./node_modules/@shopify/flash-list/package.json').version)" && grep -n "overflowAnchor" node_modules/@shopify/flash-list/dist/recyclerview/RecyclerView.js
```
Expected: the installed fork has the fix (`overflowAnchor` is in its build).

- [ ] **Step 3: Write the web e2e**

Create `tests/e2e/reports-scroll-anchor.spec.ts`:

```ts
import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import { addColumn, addSale, addTeam, chooseOption, openReportStub, openReportsHub, pressToolbar, rowOf, SALES_SOURCE, saveNewReport, startReport, uniqueName } from './reports-helpers'

/**
 * Web: when rows above the view change height, the rows in view stay where they
 * are (the FlashList fork's offset correction). A report grouped by status, the
 * Paid group open and scrolled into view; Expand all opens the Open group above
 * it, and a row of Paid keeps its place on screen.
 */
test.describe('scroll position on web', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a group opening above the view keeps the visible rows in place', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptanchor')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            for (let i = 0; i < 20; i++) {
                await addSale(p, { team, customer: `Open ${String(i).padStart(2, '0')}`, amount: 100, cost: 0, day: '2026-01-05' })
                await addSale(p, { team, customer: `Paid ${String(i).padStart(2, '0')}`, amount: 100, cost: 0, day: '2026-01-05', status: 'paid' })
            }
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await addColumn(p, ['status'])
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await saveNewReport(p, uniqueName('Anchor'))
            await rowOf(p, 'Paid').click()
            const target = rowOf(p, 'Paid 10')
            await target.scrollIntoViewIfNeeded()
            const before = (await target.boundingBox())?.y ?? -1
            await pressToolbar(p, 'report-fold-all', 'Expand all')
            await expect(rowOf(p, 'Open 00')).toBeAttached()
            await expect.poll(async () => Math.abs(((await target.boundingBox())?.y ?? -999) - before)).toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 4: Run, then commit**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-scroll-anchor.spec.ts tests/e2e/reports-wrap.spec.ts`
Expected: both pass (the anchor spec fails against the old pin; it is the regression test for the fork fix).

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/package-versions.json core/package.json tests/e2e/reports-scroll-anchor.spec.ts
git commit -m "chore(core): pin the flash-list fork with web offset correction"
git push origin books/p5-trial-balance
```

---

### Task 26: Help and package docs

**Files:**
- Modify: `core/help/reports.md`, `docs/packages.md` ("Report sources" at line 1070, "Shipped and saved reports" at line 1199)

- [ ] **Step 1: Core help**

Add to `core/help/reports.md`, after "## Make a report" and before "## Run a report", task-first sections (simple sentences; shortcuts none):

```markdown
## Totals

To total only what a report shows, open the column in **Columns** and set **Total** to **Add the rows shown**. Group rows, subtotals and the Total row then add the rows you see. A group that is hidden adds nothing. **Recompute from all rows** (the default) totals every row the filter keeps.

## Hide groups

To show only some groups, open **Group and sort**, find the level, and add a condition under **Show only groups where…**. A condition can test a total of a field, a column, or a field of the group's record. Turn on **Ask when the report runs** to let the reader set the value from a chip.

## Group by a linked record

To group by a linked record (for example each team of a sale), add the link as a column: in **Add column**, pick **Team (record)**. Group by that column. The group row shows the record's name; pick another field, or a formula, under **Label**. On any level, turn on **Show every team, even with no rows** to list records that have no rows, and add conditions to choose which. Under another group, a condition can be **Same as the group above**, so each region lists only its own teams.

## Sort

To sort groups by a field of their record, pick it in **Sort by** (for example **Team › Name**). To sort by anything else, pick **Formula…**.

## Look up a value

To read one value from another kind of record, add a formula column and pick **Look up**. Choose the source, a condition under **Where**, and the **Value**. The report reads only records you may see. **Smallest** or **Largest** picks one value when several match.

## Opened rows

When you open a group, its rows are labelled by the first column. To label them another way, turn on **Label opened rows with a formula** in **Columns**.

## Filters that list records

A condition on a linked record lets you pick records by name. Turn on **Ask when the report runs**: the reader picks records from a list they can search, and can pick a whole group at once.

## Fold groups

To open a group level when the report opens, turn on **Open when the report opens** for that level. **Collapse all** closes the groups until you open them again.

## Formulas

A formula is built from parts you pick from menus: numbers, texts, columns, fields, group fields and asked values; add, subtract, multiply, divide and round; totals and running values; comparisons, **All of**, **Any of**, **Not** and **Is empty**; **If** and **First match**; text and date parts; and **Look up**.

## Narrow screens

On a phone or tablet, a row too wide for the screen wraps: the label on the first line and the figures below it, each under its heading. To choose where each column goes, open **Arrange**, pick a width, and turn on **Arrange columns for this width**.
```

Update the help topic's frontmatter `tags` with `totals, groups, lookup, formulas, phone`.

- [ ] **Step 2: Package docs**

In `docs/packages.md` "Report sources": document `Field.TextDay` (a `date` field over a text `YYYY-MM-DD` column), `Source.Heading` and `Source.Watch` (with the rule that every collection `Heading`, `FiscalStartMonth` and `Currency` read is listed), link-ending paths and the `id` field as relation values, and a record group's default label (the first text field; a target with none needs a label formula). Add a "Spec version 2" table of the new fields (the "For review" list), and the param-options route. In "Shipped and saved reports": `Shipped.Unpublished`, and how a package shares its reports (bind `OnRecordAfterCreateSuccess("reports")` for rows with its `shipped_key`, which `ReconcileShipped`'s `app.Save` fires, and reconcile at serve start; core adds no share API, R15). Add a paragraph "Report e2e helpers": a package's e2e opens core's report view with `openReportsHub`, `hubRow`, `rowOf`, `cellOf`, `pressToolbar` and `chooseParamOption` from `@tinycld/core/e2e-helpers`.

- [ ] **Step 3: Regenerate the help index and check**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: PASS (core help is read from `core/help/` at build; no generator run is needed for a core topic's body).

- [ ] **Step 4: Commit**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git add core/help/reports.md docs/packages.md
git commit -m "docs(core): reports help and package docs for spec v2"
git push origin books/p5-trial-balance
```

---

### Task 27: Full checks — core, Go, the app server, core isolation, Books

**Files:** none (fix what fails at its root, in the file the failure names).

- [ ] **Step 1: Core TypeScript**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Expected: biome, tsc and vitest pass.

- [ ] **Step 2: Go**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/core/server && go vet -tags no_ui ./... && go test -tags no_ui -count=1 ./... && go test -tags no_ui -count=1 -race ./reports/...`
Expected: `ok` for every package (the race run of `reports` takes a few minutes).

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld/server && go vet . && go build -o /dev/null .`
Expected: clean (the stub's Go module builds against the new `reports` API).

- [ ] **Step 3: Core names no package**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tsx scripts/check-core-isolation.ts`
Expected: exit 0, no references (the stub and every spec live under `tests/`).

- [ ] **Step 4: Books still passes (Books has not changed yet)**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg check`
Run: `cd /Users/nas/tmp/tinycld/books-purchases/books/server && go vet ./... && go test -count=1 ./...`
Expected: pass. Books' shell still imports nothing this plan removed (`visibleRows` lost a parameter: Books has its own shell copy until checkpoint 1; a failure here is a core change Books relies on, fixed in core).

- [ ] **Step 5: Commit any fix**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/tinycld
git status --short
git commit -am "fix(core): full-check fixes for report engine v2"
git push origin books/p5-trial-balance
```
(Only when Steps 1–4 needed a change; name the fix in the message.)

---

### Task 28: Every reports e2e spec, the forms builder spec, Books' report e2e; push

**Files:** none.

- [ ] **Step 1: Every reports spec and the forms builder spec**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/reports-create.spec.ts tests/e2e/reports-filter.spec.ts tests/e2e/reports-group-pivot.spec.ts tests/e2e/reports-formula-layout.spec.ts tests/e2e/reports-view.spec.ts tests/e2e/reports-share.spec.ts tests/e2e/reports-shipped.spec.ts tests/e2e/reports-formulas-v2.spec.ts tests/e2e/reports-groups-v2.spec.ts tests/e2e/reports-source-params.spec.ts tests/e2e/reports-wrap.spec.ts tests/e2e/reports-scroll-anchor.spec.ts tests/e2e/forms-builder.spec.ts`
Expected: all pass. A failure is diagnosed and fixed at its root (no re-runs, no timeouts raised).

- [ ] **Step 2: Books' report shell and chrome e2e**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/books && E2E_PORT=7210 E2E_PORT_2=7211 PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg test:e2e -- tests/e2e/report-shell-frame.spec.ts tests/e2e/report-shell-period.spec.ts tests/e2e/report-shell-filters.spec.ts tests/e2e/report-shell-customize.spec.ts tests/e2e/report-shell-table.spec.ts`
Expected: all pass: Books' own shell is untouched by this checkpoint.

- [ ] **Step 3: Push**

Run: `cd /Users/nas/tmp/tinycld/books-purchases/tinycld && git push origin books/p5-trial-balance`
Expected: pushed; CI runs the full suite (the reports stub scaffold step already exists).

---

## Spec coverage

| Spec item (trial-balance-design §4, §0) | Tasks |
|---|---|
| E1 `Field.TextDay` | 1, 17, 26 |
| E2 `total: "rows"` (OQ6) | 8, 21, 22, 26 |
| E3 `groups[].having` + builder | 7, 21, 22, 26 |
| E4 record groups, `record` leaf, `label`, `fill` | 3, 5, 5b, 6, 15, 19, 21, 22, 26 |
| E5 sort by formula | 9, 21, 22, 26 |
| E6 `lookup` under the reader's rules | 4, 15, 19, 20, 26 |
| E7 `detail.label` | 10, 16, 21, 22, 26 |
| E8 options from a source, route, picker (OQ5) | 12, 13, 13a, 16, 21, 23, 26 |
| E9 `Source.Heading` → `heading` | 14, 16, 17, 23, 26 |
| E10 `Shipped.Unpublished` | 14, 17, 23, 26 |
| E11 report e2e helpers exported | 17, 26 |
| E12 spec v2, v1 unchanged, builder round-trip | 2, 15, 16, 21 |
| E13 `groups[].open` | 11, 16, 22, 26 |
| E14 wrapped rows (flex), FlashList, sticky heading and groups, author arrangement (human addition) | 15, 16, 24, 25, 26 |
| OQ2 every report operator in the formula editor | 18, 19, 20 |
| Q3 (changed) fill on every level, under each parent | 6, 21, 22 |
| Q5 (changed) per-row currencies in record leaves and lookups | 5a |
| Q10 (changed) spanning cells by guillotine cuts, pinwheel refused | 15, 24, 25 |
| Web scroll offset fixed in the FlashList fork (with the STOP for its PR) | 25a, 25b, 25c, 25d |
| F16 lookups in layout computed rows (R21 withdrawn) | 4 |
| F17 `total: "rows"` on a number column with no aggregate (R8) | 8, 22 |
| F18 amount params; a money total asked when run (R18, R29) | 12a, 16, 21, 22, 23, 26 |
| F19 whole groups expanded on the server (R13, R30) | 13a, 16, 23 |
| F22 link columns show their record's label (R9, R31) | 5b, 16, 21, 22 |
| F5 currency keys from every formula a level reads (R3) | 5, 7, 9, 10 |
| Core tests: formula table `lookup`/`record`, rule-parity fuzz with lookup and fill, vitest v2/round-trip/options, e2e desktop + phone | 4, 5, 15, 16, 21, 23, 20, 22, 23, 25 |
| Books unchanged and green | 27, 28 |

## Self-review notes

- Every engine task cites the code it changes; the plan relies on two probed behaviours (Go's nil vs empty slice for `expand`, SQLite's CTE references) and writes them under "What was probed".
- Placement is one rule in two languages (`placeCells` and the guillotine cut check in Go, Task 15; in TS, Task 24), with the same examples (a mid-row spanning cell, a pinwheel) in both test suites.
- The human's answers of 2026-11-01 are folded in: fill on every level (Task 6), per-row currencies (Task 5a), guillotine cuts (Tasks 15, 24, 25), and the FlashList fork fix with its STOP (Tasks 25a–25d).
- E14 was revised after the human's review: row content is flex lines (FlashList positions the rows), FlashList v2 instead of the custom row window, sticky heading and stacked sticky groups.
- The preflight's 29 findings are applied in the tasks themselves (code and tests); the five no-limits rulings (F16, F17, F18, F19, F22) are built, and no refusal was added.
- The forms editor's ids and labels are preserved by construction (the forms operator set wraps the existing helpers); the report set keeps the shared ids, so the existing report e2e keeps its selectors.
