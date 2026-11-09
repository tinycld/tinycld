# Report paged group rows and fold list (J1) — outline

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. The tasks are in the parts below; this file is the outline every part shares.

**Scope:** J1 (paged group rows J1a, fold list J1b) and J2 (one-line spare width by `flexGrow`, added to checkpoint 0 by the human, brief D1 reversed).

**Goal:** A grouped report (not a pivot, not a layout report, no nest) reads its group rows a page at a time in document order, with no 5,000-group cut, and a run takes a fold list (`options.collapse`) so a reader can close groups of an open report however many groups it has.

**Verdict: MAJOR** (auto-helix rule). J1a changes the run contract of every grouped report (group rows page; the group-row cap goes for them) and J1b adds a run option. No spec version change (spec stays version 4), no core migration, no generator change, no removed export. **Lane purchases does NOT take `exclusive`; the core merge needs the human's approval** (brief D4). The checkpoint stops after the PR opens.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/journal-report-design.md` §4 (rows J1a and J1b, every column; "Core PR #452"; "Core tests"; the verdict). Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-journal-report/cp/cp0.md` (Checkpoint 0).

**Architecture.** The engine keeps one SQL statement per kind. A *paged grouped* run (grouped, no pivot, no `layout`, no nest; `runner.pagedGroups()`) sends: (1) a **page statement** of level rows — every level's `o<L>` CTE as today, plus each row's *path* (its ancestors' and its own sibling ordinal `__p<i>`) and its own order values (`__s<L>_<j>`), filtered by a group keyset after the cursor's group path and limited to `limit+1+anchors` rows in document order; (2) a **Total statement** (`SELECT * FROM o0` over the same CTEs); (3) the existing **detail statement** over the open innermost groups of the page. Go assembles those rows as today and then cuts the page (`cutPage`) at `limit` rows counting group, opening, detail and subtotal rows; the cut's last row gives the cursor (`groupCursor`: the group path's keys and order values, the mode `in` or `after`, and the detail keyset inside an open innermost group). Pivots, layout reports and nests keep today's statement, `capLevelRows` and `rowCapNotice`. The fold list adds `expansion.closed`; `expansion.where` and `expansion.has` honor it, and the cursor fingerprint gains the fold state. The client keeps folds under open levels in `Expansion.collapsed`, sends `options.collapse`, counts a page as the server counts it, and shows a failed next page as a row with Retry.

**Tech stack:** Go 1.26 (PocketBase, SQLite), TypeScript (React Native + web, TanStack Query 5.104), vitest, Playwright.

Paths are relative to `/Users/nas/tmp/tinycld/books-purchases/tinycld` (core repo, branch `books/p5-journal-report`, from main b021c3af). Go tests run from `core/server`: `go test ./reports/ -run <Name>`.

## Global constraints

- Core names no package: tests use the `zoo_*` collections (Go) and the `salesstub` package (e2e). (Spec §4 "General".)
- "A page holds up to `options.limit` rows (default 200, at most 1,000) counting group, opening, detail and subtotal rows, in document order; a closed group counts as its one row." (J1a)
- "`having` and `fill` apply before the keyset. The Total row is computed over every row the filter keeps, in a statement of its own, and every page still ends with it." (J1a)
- "The 5,000 cap on group rows (`rowCap`, `capLevelRows`) goes for grouped reports; it stays for pivots, nests and layout reports." (J1a)
- "`options.collapse: [groupKey]` (at most 5,000) … `expand` keeps its meaning for levels that are closed by default." (J1b)
- "A grouped report whose groups fit one page returns the same rows after J1; the golden changes only where it pages, and the reason is written in the test." (#452 pins)
- #452 stays: `recordJoins` (the group keyset compares on the joined columns), `lookupLayers` (keyset and limit on the level's outer select; each lookup computed once per statement), fill keysets loop over parent rows first, every statement goes through `rn.query` (topics), the pinned output.
- `view-url.ts` does not change: the fold state is not in the link.
- E2E: one file at a time, `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <file>.spec.ts`, ports checked first. No raw PocketBase writes in e2e. `page.route` answers with `route.fulfill({ status: 400 })`, never `route.abort()` (memory p1-account-register).
- Biome, no `any`, no biome-ignore, no `console.*`; no `useEffect` + `useState` sync.

## Review focus

1. A row added or removed between pages (a new group sorting before the cursor, the cursor's own group deleted): the next page skips nothing after the cursor and repeats nothing before it (keyset on values, not ordinals; Task 5 and Task 6 tests).
2. Blank group keys and blank sort values at a page seam: `(none)` groups and NULL sort values page in their place (NULLs last), never twice (Task 5 test "blank group at a seam").
3. Two currencies: each currency's Total stays after its own block on the page that holds the block's end, and the last page ends in the Totals (Task 5 and Task 6).
4. A fold sent with a stale cursor (another fold state) is refused with `options.cursor`, and the client then reruns from page 1 (Task 4 test; client already rereads on a fold because the fold changes the body).
5. A next page that fails (refusal or network) keeps the rows already shown and offers Retry; the Total never shows before the last page (Task 9 vitest + e2e).

## File structure

| File | Change | Task |
|---|---|---|
| `core/server/reports/run.go` | `Options.Collapse`; `parseExpansion` → `parseKeys`; `expansion.closed`, `closedEncoded`; `where` (via `keysIn`) and `has` honor folds; `openLevels`; `foldCap` | 1 |
| `core/server/reports/fill.go` | `parentKeys` checks every ancestor depth under the fold state | 1 |
| `core/server/reports/collapse_test.go` (new), `open_test.go`, `fill_test.go`, `groups_test.go` | J1b tests | 1 |
| `core/server/reports/paging.go` | `groupCursor`, `groupFingerprint`, `foldPrint`, `groupCursorOf`, `readGroupCursor`, `afterSQL` (keysetCond generalised) | 2 |
| `core/server/reports/paging_test.go` | cursor round trip and refusals | 2 |
| `core/server/reports/run.go` | `runner.pagedGroups()`, `totalPass`, `groupPass` without the `o0` leg and the probes for paged runs (`groupStatement`'s `withTotal`) | 3 |
| `core/server/reports/ledger_guard_test.go`, `pagedgroups_test.go` (new), `fillplan_test.go` (keyset counts, only if the Total statement changes them) | three statements; Total of its own | 3 |
| `core/server/reports/levels.go` | `orderTerm`, `level.terms`, `orderSQL`; `render` adds `__s<L>_<j>` in a paged statement | 4 |
| `core/server/reports/run.go` | `compiledLevel`, `compileLevels`, `unreadCells` (split out of `groupStatement`); `pagedGroupStatement`, `pathCTEs`, `groupKeyset`; `pagedGroupPass`; `capLevelRows` skipped for paged runs; detail pass reads only the page's leaves; the paged branch of `groupPass` removed (Task 5) | 4, 5 |
| `core/server/reports/assemble.go` | `cutPage` (start by cursor mode, cut at the limit, cursor of the last row); `pageWindow` kept for nests | 4, 5 |
| `core/server/reports/pagedgroups_test.go`, `paging_test.go`, `groups_test.go`, `run_test.go`, `balance_shape_test.go`, `mixedcondition_test.go`, `having_test.go`, `statement_test.go` (`pageStatementOf`: the statement-shape pins on the J1a page statement, and on the unpaged group statement of a layout or pivot) | J1a closed/open/two-level/having/fill/currency/6,000 groups; pivot and layout cap; existing tests moved to the J1a page count | 4, 5 |
| `core/server/reports/fillplan_test.go` | page that starts inside fill groups | 6 |
| `core/server/reports/ledger_scale_test.go` + `testdata/ledger_scale.golden.json`, `ledger_guard_test.go`, `balance_shape_test.go`, `fuzz_test.go` | #452 pins on paged statements; golden where it pages; fuzz fold shapes | 6 |
| `tests/e2e/reports-helpers.ts` | `openTeamBalance`, `showTeams`, `rowIds`, `openPagedTeams` moved here from `reports-ledger-v3.spec.ts` | 7 |
| `tests/e2e/reports-paged-groups.spec.ts` (new) | `reports: paged groups` desktop + phone | 7 |
| `core/reports/lib/rows.ts` | `Expansion.collapsed`; `toggleGroup`, `expansionOptions` | 8 |
| `core/reports/lib/run-client.ts` | `RunOptions.collapse`; `foldKeyOf` leaves it out; `pageRowCount` (was `detailRowCount`); `runFirstPage` counts rows | 8 |
| `core/reports/hooks/useReportRun.ts` | `ReaderPlace.rows`; a later-page refusal thrown (`LoadMoreRefused`); `loadMoreFailed`, `retryLoadMore` | 8, 9 |
| `core/reports/lib/run-state.ts` | `RunQueryState.isLoadMoreError`; a later-page failure keeps `ready` | 9 |
| `core/lib/query-errors.ts` (+ `core/lib/__tests__/query-errors.test.ts`) | `QueryRefusal`: a thrown refusal is logged, not reported | 9 |
| `core/tests/fake-pb-server.ts` | `FakeAnswer` (a route answers with a status) | 9 |
| `core/reports/components/ReportTable.tsx`, `ReportView.tsx` | `TableEnd` error row (`report-loading-more-error`, `report-loading-more-retry`) in core's `MessageBody` style | 9 |
| `core/reports/lib/__tests__/rows.test.ts`, `run-client.test.ts`, `run-state.test.ts`, `core/reports/hooks/__tests__/useReportRun.test.tsx` | vitest | 8, 9 |
| `tests/e2e/reports-ledger-v3.spec.ts` | page counts of pages of 5 rows (J1a counts all four row kinds) | 10 |
| `docs/packages.md`, `core/help/reports.md` | J1 sentences (and the Retry row in help) | 10 |
| `core/reports/lib/table-lines.ts` (`shareWidth` line 404 split into `share` + `shareWidth` (side blocks, unchanged) + `shareLineWidth` (one line); `fitOneLine` line 430 calls `shareLineWidth`) + `core/reports/lib/__tests__/table-lines.test.ts` (`column-measure.test.ts` pins no shared width) | J2: one line shares spare width by `flexGrow` (R10) | 11 |
| core report e2e that read one-line widths (`tests/e2e/reports-column-fit.spec.ts`, `reports-fit.spec.ts`, `reports-wrap.spec.ts`; `reports-text-width.spec.ts` reads text estimates only) | J2: changed only where the new rule changes a width, reason in a comment | 11 |
| `docs/packages.md:1368`, `core/help/reports.md:121, 131` (1338 and help 137 are arranged lines, unchanged by R10) | J2: how spare width is shared | 11 |

## Tasks

Part 1 (`2026-11-08-report-paged-groups-part-1.md`): Tasks 1–6 (engine). Part 2 (`…-part-2.md`): Tasks 7–11 (e2e, client, docs, J2). Task 4 is the largest task (statement, cut, cursor); its implementer may land 3a–3c (order terms, `__s` columns, the page statement) before 3d (pass, cut, cursor) inside the task.

1. **Fold list (J1b, Go).** Produces `Options.Collapse [][]any` (JSON `collapse`), `foldCap = 5000`, `parseKeys(at string, keys [][]any) ([][]any, [][]string, map[string]bool, error)`, `expansion{…; closed map[string]bool; closedKeys [][]any; closedEncoded [][]string; defaults bool}`, `(e *expansion) has(key []any, n int) bool` (false for a closed key), `(e *expansion) where(c, s, depth, prefix) string` (excludes closed keys), `(e *expansion) shownUnder(c, s, depth, prefix) string` (AND of `where` over depths 1..depth), `openLevels(p *plan, opts Options, e *expansion)`; rule: the open levels stay open when `collapse` is sent (Ruling R2).
2. **Group cursor (Go).** Produces `type groupCursor struct { Key string "k"; Group []any "g"; Path [][]any "p"; Mode string "m"; Values []any "v" }`, `const cursorIn = "in"`, `const cursorAfter = "after"`, `foldPrint(e *expansion) string`, `groupFingerprint(levels string, shape string, fold string, keys []sortKey) string`, `type groupPos struct { key []any; path [][]any; mode string }`, `levelShape(levels [][]orderTerm) string`, `groupCursorOf(fp string, keys []sortKey, at groupPos, detail map[string]any) string`, `readGroupCursor(fp string, shape [][]orderTerm, keys []sortKey, groups, currencies int, text string) (groupCursor, error)`, `type orderTerm struct { sql string; desc bool }`, `afterSQL(s *stmt, cols []orderTerm, values []any) string`.
3. **The Total in a statement of its own.** Produces `(rn *runner) pagedGroups() bool`, `(rn *runner) totalPass(s *stmt, ctes []string) ([]map[string]any, error)`, `(c *compiler) groupStatement(…, withTotal bool)` (the `o0` leg only when `withTotal`).
4. **Paged group rows, groups closed (J1a).** Produces `level.terms []orderTerm`, `orderSQL(terms []orderTerm) []string`, `type compiledLevel struct { lv *level; cells, totals []string; own string }`, `(c *compiler) compileLevels(s *stmt, r *rowsCTE, expand *expansion, pivot []pivotValue) ([]compiledLevel, error)`, `(c *compiler) unreadCells(levels []compiledLevel, computed []unionLeg) map[string]bool`, `(c *compiler) pagedGroupStatement(s *stmt, levels []compiledLevel, expand *expansion, cur *groupCursor, limit int64) (levelCTEs, pageCTEs []string, body string, err error)` (one order: compile the levels once, read the cursor from their terms, then build the page statement), `(rn *runner) pagedGroupPass() ([]map[string]any, error)`, `(rn *runner) cutPage(limit int) error` (an error since fix round 1: a cursor that cannot be written fails the run), `runner.groupCur *groupCursor`, `runner.rawGroups map[string]map[string]any`, `runner.readDetails bool` (a one-page run that read no details keeps a nil `page`); paged only while no innermost group opens (gate removed in Task 5).
5. **Paged group rows, groups open (J1a).** Produces `(rn *runner) pagedDetailAfter(s *stmt, keys []sortKey, leaves [][]any) string` (no error since fix round 1, R11), the opening rule (a group row and its opening row share a page), the gate of Task 4 removed; every grouped report without pivot, layout or nest pages its group rows; the paged branch of `groupPass` and `groupStatement`'s `withTotal` removed (dead); `having_test.go` `TestHiddenRowsTakeNoPlaceOnAPage` moved to `pageSize = 3`.
6. **#452 pins and parity on paged statements.** `ledger_guard_test.go` on a cursor page (page, Total, detail statements), `fillplan_test.go` on a page that starts inside fill groups, `balance_shape_test.go` seam between groups, `ledger_scale` golden updated only for runs that page (reason in the test), `FuzzRuleParity` fold shapes and reader parity of merged details.
7. **E2E spec (red first).** Produces `tests/e2e/reports-paged-groups.spec.ts` (`test.describe('reports: paged groups', …)`), exported helpers `openTeamBalance(page)`, `showTeams(page, search, teams)`, `rowIds(page)`, `detailLabels(page)`, `SentRun`, `USUAL_PAGE`, `MAX_PAGE`, `PagedTeamsOptions { smallTeams?, nextPages?, baseAmount?, pageRows?, refuseFirstNextPage?, height? }`, `openPagedTeams(page, prefix, opts?)` in `tests/e2e/reports-helpers.ts`.
8. **Client fold state and page count.** Produces `Expansion = { all: boolean; keys: readonly unknown[][]; collapsed: readonly unknown[][]; isDefault?: boolean }`, `expansionOptions(e): { expand?: unknown[][]; expandAll?: boolean; collapse?: unknown[][] }`, `RunOptions.collapse?: unknown[][]`, `pageRowCount(result: RunResult): number` (replaces `detailRowCount`), `ReaderPlace = { rows: number; groupRow?: string }`.
9. **A failed next page shows a row with Retry.** Produces `QueryRefusal` (core/lib/query-errors.ts), `LoadMoreRefused extends QueryRefusal` (run-client.ts), `RunQueryState.isLoadMoreError: boolean` (state stays `ready`), `ReportRun.loadMoreFailed?: boolean` (the hook's own mark of a failed next page, false while a fetch of more runs), `ReportRun.retryLoadMore?(): void` (optional: a hand-built ReportRun loads no pages), `TableEnd({ hasMore, failed, onRetry })`, `ReportTableProps.loadMoreFailed?`, `ReportTableProps.onRetryMore?`, `FakeAnswer` (core/tests/fake-pb-server.ts); test ids `report-loading-more-error`, `report-loading-more-retry`; text "Could not load more rows".
10. **Existing e2e, help and docs.** `reports-ledger-v3.spec.ts` page counts; `docs/packages.md` (paging paragraph, `options.expand`/`options.collapse` cap, "What a report returns"); `core/help/reports.md` ("however many groups it has").
11. **J2 — one line shares spare width by `flexGrow`** (brief cp0.md:50 "D1 reversed", :77, :98; added by the human 2026-11-08). Produces `shareLineWidth(parts: readonly ColumnMeasure[], width: number): number[] | null` for `fitOneLine`: the spare room past each column's min goes by `grow`, not by what a column lacks of its natural width (a grow-0 column gets only its lack, R10); `shareWidth` keeps its signature and result for side blocks; vitest first: "on one line with spare room, a Description column with `flexGrow: 2` gets more of the spare room than a Name column with `flexGrow: 1`, also when Name's natural width is wider" (fails before); existing `table-lines` / column-fit tests and width-pinning e2e changed only where the rule changes them, each with the reason; packages.md and help say spare width goes by Extra room (`flexGrow`).

## Rulings

- **R1 — Page statement shape.** The group keyset compares a level row's *sibling ordinal* `__p<i>` (the `ROW_NUMBER` `o<i>` already computes as `__ord`) against the number of siblings at or before the cursor's group, which one uncorrelated count over `o<i>` finds from the cursor's order **values** (`__s<i>_<j>`). So the cursor holds values (stable when rows change between pages), and the outer select orders by small integers. Cost if wrong: a slow page statement on reports with many levels; the ledger guard and `BenchmarkLedgerRun` show it, and the fix is local to `pagedGroupStatement`.
- **R2 — Fold list semantics.** Base state: `expandAll` → every group; else `expand` absent **or `collapse` present** → the open levels; else (`expand` present, no `collapse`) → only `expand`'s groups (today's rule; Collapse all still sends `expand: []`). On that base, `expand` opens groups of closed levels and `collapse` closes listed groups (a closed group hides its whole subtree). The client sends `collapse` (possibly `[]`) whenever its state is the default with changes. Cost if wrong: a reader's fold under default state opens or closes the wrong level; caught by `collapse_test.go` and the e2e fold test; the fix is `openLevels` and `expansionOptions`.
- **R3 — What a page counts.** Group, opening, detail and subtotal rows count; Total and computed rows do not. A group row and its opening row stay on one page: when the limit falls between them the page ends before the group row, unless the group row is the page's first counted row, then the page holds the opening too (limit+1). Cost if wrong: a page of limit+1 rows at limit 1 only.
  Shown to the human with the merge request.
- **R4 — `Page.Size`.** Keeps its meaning: the detail rows on the page (unused by the client). `options.limit` counts the four kinds. So a run that fits one page returns byte-identical output and the `ledger_scale` golden is unchanged for it (brief). Cost if wrong: a reader of `size` takes it for the page's row count; none does. Checked in Part 2 against the brief's "`detailRowCount` / `runFirstPage` count what a page's limit now counts": that is the client's place count, not `page.size`. The client never reads `page.size` (no `page.size` / `page?.size` in `core/reports`), and `pageRowCount` (Task 8) counts group, opening, detail and subtotal rows, as `options.limit` does (R3). No conflict.
- **R5 — Statements of a paged grouped run.** Page statement, Total statement (`SELECT * FROM o0`, which also carries the condition probes), detail statement (only when the page has an open innermost group or the cursor is inside one). In a page statement each `o<L>` renders `AS MATERIALIZED`, because the path CTE of the level below reads it a second time; each lookup is then still computed once per statement (ledger guard). Cost if wrong: a lookup computed twice per page; the ledger guard fails.
- **R6 — Cursor contents.** `groupCursor{k, g, p, m, v, h}` (`h` since R11): `g` the group key of the path's deepest group (group keys then currency keys), `p` each path level's order values, `m` `in` (the page ended inside that group: after its row and opening, or after a detail row `v` of it) or `after` (the page ended after the group's whole subtree). The fingerprint covers the group levels, each level's order shape (term count and directions), the fold state (`foldPrint`), and the detail sort keys. Cost if wrong: a cursor taken across a fold; the fold-state test catches it.
- **R7 — Currency order.** Currency blocks order as Go orders the Totals today (`keyString` of the currency key, which for currency codes is text order with a blank currency last); SQL orders `(ck IS NULL), ck` per currency column, which equals it for ISO codes. Cost if wrong: a currency code with characters JSON escapes; none exists.
- **R8 — Next-page failure.** Today a later page's refusal becomes a page whose kind is not `ok`, so `runView` replaces the whole table with "This report cannot run" (`report-invalid`), and a thrown later-page error turns the whole view into `report-error`. Neither is a next-page row. A later page's failure now throws inside the query function (`LoadMoreRefused`), so TanStack keeps the loaded pages and sets `isFetchNextPageError`; the table shows "Could not load more rows" with Retry. A first-page failure keeps today's states and ids (`report-error`, `report-retry`, `report-invalid`). `LoadMoreRefused` extends core's new `QueryRefusal`, which the global query error handler logs at info and does not report (a refusal is not a bug). While the next page has failed, the list's `loadMore` does nothing (only Retry asks again), so redraws do not resend the failing request. Cost if wrong: a stale cursor after a design change shows the error row instead of rerunning; a design change changes the rows key, which restarts at page 1 anyway.
- **R9 — No core e2e counts on the 5,000 notice for a grouped report.** `grep -rn -i -e 'shows its first' -e rowCap -e 'report-notice-rowCap' tests/e2e` finds none at b021c3af (the only cap notice in e2e is `report-notice-pivotCap`, `reports-view.spec.ts:500`, a pivot, which keeps its cap). The e2e that change are the ones whose pages counted only detail rows: `reports-ledger-v3.spec.ts` (Task 10).
- **R10 — J2 scope.** J2 changes the one-line layout only (brief: "the one-line layout"): `fitOneLine` uses `shareLineWidth`; arranged and automatic lines keep `shareWidth`, whose side parts' grow is their tracks (automatic lines give every part 1 track, so a grow share there would split a phone line evenly, whatever its content). On one line, a column with grow 0 (Extra room None) still gets what it lacks of its natural width (in proportion when the room is short) and never more; every other column gets room past its min by grow only. Without that, a figure whose heading is wider than its values would wrap its heading on every screen. Cost if wrong: arranged lines still share by lack; to extend J2 to them, `fitBlock` calls `shareLineWidth` (a one-line change, with its tests).
  Shown to the human with the merge request.
- **R11 — A group that moves between pages (review A BLOCK 1, fix round 1).** The page reads an anchor again only while it and each anchor around it still have the cursor's order values (`pc`'s `an`, read back as `__anchored`); a group that sorts elsewhere now counts as removed, so the page does not start at its new place, and the keyset reads it there when it sorts after the cursor. The details of the cursor's own group that a page showed are never read again: the detail keyset applies to that group wherever it is among the leaves, with the sort values its cursor row has now (a group moves because a value its details share changed, so the cursor's old values would put every detail before or after it). Until a page passes such a moved group, the cursor carries it in `h` (`held`: group key and detail values, at most 32, the oldest dropped). The engine sends a group row the reader saw again at its new place, but the client's `mergePages` keeps the first copy of a row id and drops the later one, so the group's remaining details and subtotal show after the group before them with no heading until the live re-run (which refetches from page 1) lands — shown to the human with the merge request; a whole group the reader saw before it moved later shows again there, as a detail row that moves later does in keyset paging. Cost if wrong: a repeated or missing row after a concurrent edit; `movedgroup_test.go` pins both depths.
- **R12 — A growing column stops at its natural width while another is cut (review A NOTE 1, fix round 1).** On one line the spare room past the mins still goes by `flexGrow` (R10), so Description (grow 2) gets more than Name (grow 1) while both lack room. But a growing column takes room past its natural width only once every growing column has its natural width: until then its share goes to the others by grow (`shareLineWidth`, `naturalCap`). Without it, at 1100 px "Wrapco Longname 00" (needs 142 px) was cut at 124 px while the currency column held 166 px for "USD" (`reports-wrap.spec.ts`). This changes every report's one-line widths, the General Ledger and Trial Balance included: `table-lines.test.ts` "gives each column its min…" (500 px: account 198, number 82, was 168 / 112) and "narrows the text…" (424 px: account 140, number 64, was 130 / 74); the J2 Description-vs-Name test moved from 480 px (where Description now stops at its natural 165 px and Name gets the rest) to 380 px, where both lack room.
  Shown to the human with the merge request.

## Open questions

None. R2 and R10 are the semantic choices the spec and brief leave; each is ruled above with its cost.

## Coverage check

| Brief item | Task |
|---|---|
| Produces `reports.Options.Collapse` (`[][]any`, JSON `collapse`, ≤ 5,000) | 1 |
| Produces `page.next` on every grouped result that has more rows (group path + detail keyset) | 2, 4, 5 |
| Produces no `rowCap` notice for a grouped report (pivots, nests, layouts keep it) | 4, 5 |
| Produces `Expansion.collapsed`, `RunBody.collapse` (as `RunOptions.collapse`, see Books plan changes) | 8 |
| Produces test ids `report-loading-more-error`, `report-loading-more-retry` | 9 |
| Go J1a: closed groups page through every group, no repeat, no gap | 4 |
| Go J1a: open groups | 5 |
| Go J1a: two levels | 4 (closed/inner closed), 5 (open) |
| Go J1a: an open group whose details span a page boundary | 5 |
| Go J1a: `having` hiding groups at a boundary | 4 |
| Go J1a: `fill` groups at a boundary | 4 |
| Go J1a: with a viewer sort | 5 |
| Go J1a: the Total on every page equals the whole report's Total | 4 |
| Go J1a: a cursor from another fold state is refused | 4 |
| Go J1a: 6,000 groups, every group, no `rowCap` notice | 4 |
| Go J1a: a pivot and a layout report still stop at 5,000 with the notice | 4 |
| Go J1a: fits one page → same rows (`ledger_scale` golden unchanged for it) | 3, 6 |
| Go J1a: `checkTopics` passes for a paged run (page and Total statement) | 3, 4 |
| Go J1a: every group closed, `Limit: N` → page 1 holds exactly N group rows | 4 |
| Go #452: ledger guard on a cursor page's level statement and the Total statement | 6 |
| Go #452: fill plan on a page that starts inside fill groups | 6 |
| Go #452: balance shape seam between groups | 6 |
| Go J1b: `collapse` under an open level; under `expandAll`; > 5,000 refused; with `expand` on two levels | 1 |
| Go: fuzz rule parity covers paged group rows | 6 |
| Vitest `rows.ts`: fold under open level / Expand all → `collapsed` + `collapse`; unfold removes, no `expand`; Expand all and Collapse all clear `collapsed` | 8 |
| Vitest `run-client.ts`: `foldKeyOf`, `rowsKeyOf` same with/without `collapse`; grouped page ending between groups merges; fold re-run place counts group rows | 8 |
| E2E `reports-paged-groups.spec.ts` desktop + phone; error row desktop | 7 (red), 8, 9 (green) |
| Existing core report e2e on the 5,000 notice | none exist (R9); paging e2e changed in 10 |
| Files `core/reports/components/ReportTable.tsx` + next-page hook (`useReportRun.ts`) | 9 |
| `docs/packages.md`, `core/help/reports.md` | 10 |
| J2 Files (`table-lines.ts` `shareLineWidth` for one line, width-pinning e2e, docs/help) and the J2 vitest bullet | 11 (R10) |
| Do step 1 (this plan) / step 2 (SDD, checks, Go suite, vitest, e2e one file at a time) | this file / driver |

## Books plan changes

- `RunBody.collapse` in the brief's Produces is `RunOptions.collapse` (the run body's `options.collapse`, `unknown[][]`), beside `expand` and `expandAll`; `RunBody` itself gains no field.
- Ruling R2: a run that sends `collapse` (even `[]`) keeps the spec's open levels open; `expand` then only adds groups of closed levels. The Journal opens every block, so its folds travel in `collapse`.
- Ruling R4: `page.size` still counts detail rows only; `options.limit` counts group, opening, detail and subtotal rows. A Books Go or e2e test that counts a page's rows (the GL paging test, Do step 4) counts the four kinds.
- `detailRowCount` (core `run-client.ts`) is renamed `pageRowCount`; Books does not import it.
