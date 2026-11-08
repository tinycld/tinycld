# Report Layout Fit — Part 2 (Tasks 9–13) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove the part-1 layout fit at 390, 768, 1024 and 1280 with one acceptance e2e, and finish the reader-facing chrome the human decided: the Total only at the real end with a "Loading more rows…" row and an "End of report" row (D8, D12), the layout notice in the one notice band with a 2-line cap on a phone (H5), a preset-only period chip and a fade on the chip row (D11), and on a phone a top area that scrolls away with the rows while the column headings stay (D9).

**Architecture:** All client-side, in core's report view. H1 is a pure step after the page merge (`holdWholeReportRows` in `rows.ts`, applied by `run-state.ts`), so the list ends where the loaded rows end, while the column measure still reads the held rows. H5 and D11 are small pure helpers (`notices.ts`, `chipText`) plus component changes. D9 moves the top area into the list's header on a phone and floats the column heading row over the list, positioned by the list's scroll offset; FlashList's `stickyHeaderConfig.offset` keeps the sticky group copy under it. Web and native draw the same tree.

**Tech Stack:** TypeScript, React Native + react-native-web (Expo Router), Uniwind classes, FlashList v2 (2.3.3: `stickyHeaderConfig.offset`), React Native `Animated`, vitest, Playwright, Biome.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/layout-review.md` (§8 H1, H2, H5, H6; §11) with the binding decisions `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/layout-decisions.md` (D8, D9, D11, D12). Outline: `docs/plans/2026-11-07-report-layout-fit-outline.md`. Part 1 (Tasks 1–8, approved): `docs/plans/2026-11-07-report-layout-fit-part-1.md`.

All paths are relative to the core repo `/Users/nas/tmp/tinycld/books-purchases/tinycld` unless absolute. Line numbers are at main `0cdbe24d` and are given only for files part 1 does not change. A file part 1 changes is named by symbol, with the part-1 task that changes it; read its state at the end of part 1 before you edit it.

---

## For review

### Verdict: client-only core change, no spec, engine or Go change

- No Go change, no migration, no spec field, no new dependency.
- Changed signatures: `chipText(def, value, labels?)` and `valueText(def, value, labels?)` lose their `resolved` parameter (Task 12); `ReportTableProps.isLoadingMore` becomes `hasMore` (Task 10); `ReportNotices` gains `layoutNotice?` and `isCompact?` (Task 11); `ReportBody` and `ReportTable` gain `top?: ReactNode` (Task 13); `RunView` gains `heldRows`, `ReportRun` gains an optional `heldRows?` (Task 10). Grep of `/Users/nas/tmp/tinycld/books-purchases/books` finds none of these used there except `run.isLoadingMore` in `books/tinycld/books/screens/helix-mockup.tsx`, which stays valid (the field stays on `ReportRun`).
- Removed: `LayoutNotice` and the testID `report-layout-notice` from `ReportTable.tsx` (Task 11; the notice is now `report-notice-layout` in the band); `resolvedDates` in `params.ts` (Task 12, unused after the change).
- Books e2e to re-check after the core merge (fixed in Books, not here): `books/tests/e2e/general-ledger.spec.ts` phone tests that read `report-heading-row` or a chip after a scroll (Task 13: the top area scrolls away), and any Books spec that expects a paged report's Total before the last page (Task 10). `general-ledger.spec.ts`'s `report-loading-more` poll (about line 988) stays valid: the row is shown while a later page is left.

### Where part 2 differs from the outline, and why

| Outline | This plan | Why |
|---|---|---|
| T10: "`mergePages` drops the last page's whole-report rows while it has `page.next`" | `mergePages` is unchanged. A new pure `holdWholeReportRows(result)` in `rows.ts`, applied in `run-state.ts` after the merge, returns the shown result and the held rows. | `runFirstPage` (`core/reports/lib/run-client.ts:185-208`) joins several server pages into one first page with `mergePages`. If `mergePages` dropped the Total there, that page would never hold it again, and the measure (outline T10 constraint) could not read it. |
| T11 files: `ReportNotices.tsx` | Also a new pure `core/reports/lib/notices.ts` (`bandNotices`, `layoutNoticeOf`, `isBandCut`) with a unit test. | The 2-line cap needs a line estimate without `onLayout` text measuring (part 1 Global Constraints); a pure function can be tested. |
| T12: "chips keep the side scroll with a fade edge" | The fade is four `bg-background` strips with rising opacity, not a gradient. | No gradient exists in the codebase and adding `expo-linear-gradient` is a new dependency; strips use only semantic tokens and work on web and native. |
| T13: "part-2 planner may split web/native parity into its own task" | One task. The same tree runs on web and native; on the phone path the sticky group copy and the floating heading share one JS-driven scroll value, so they move together on both. | A split would leave a task with no test of its own (native has no e2e). |

### Decisions this part implements

| Decision | Task |
|---|---|
| Acceptance at the four widths (outline T9) | 9 |
| D8 / H1 Grand Total only with the last page; a "Loading more rows…" row stands in its place | 10 |
| D12 on screen a short end-of-report row (page footer only in exports, later) | 10 |
| H5 layout notice in the notice band; band capped at 2 lines with "Show all" on a phone | 11 |
| D11 / H2 chips keep the side scroll with a fade, and a preset-only period chip | 12 |
| D9 on a phone the top area scrolls away with the rows; only the column headings stay | 13 |

---

## Global Constraints

- No spec change; no Go change; no new dependency. Animation uses `Animated` from `react-native` only.
- Web and native: the same tree on both. Widths come from part 1's pure measure; never `onLayout` *text* measuring. `onLayout` is allowed for container heights and widths (the row-width probe, the top area's height, the heading row's height, the chip row's width).
- Exact reader texts: "Loading more rows…" (with the ellipsis character), "End of report", "Show all", "Show less".
- The Total and a layout's whole-report computed rows show only when no later page is left (D8). The measure is still taken with them (part 1 D1; outline T10 constraint).
- The period chip names its preset only ("Day: This fiscal year"); the document header carries the days (D11). A set day or range keeps its days on the chip ("Day: Feb 1 – Feb 28, 2026").
- On a phone (`model.breakpoint === 'mobile'`) with rows shown (`model.state === 'ready'` and `model.isBodyShown`), the header, chips, notices and document header scroll with the rows; the column heading row stays at the top of the table; a stuck group copy sits right under it (D9). Every other state, and every wider screen, keeps today's fixed top area.
- No raw hex; semantic Tailwind tokens; `useThemeColor` for icons. No `any`, no `biome-ignore`, no `console.*`.
- JSX minimal: no `.map()` or ternaries inside a returned tree; compute above the return.
- Tests: vitest for pure lib code (`core/reports/lib/__tests__`); Playwright for UI behaviour on the `salesstub` reports, phone viewport `{ width: 390, height: 844 }` where the phone path differs. No vitest for components or hooks.
- Commands, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`:
  - unit: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run <file>`
  - types: `pnpm exec tinycld-pkg typecheck`
  - lint: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check <files>`
  - e2e (one file): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts` (check ports 7210/7211 first, lane rules)
- Commits: the driver commits; each task's last step lists the files to stage.
- Docs: update `core/help/reports.md` and `docs/packages.md` in the task that changes the behaviour they describe.

## Review Focus

1. **A live re-run (or a fold) while later pages are left.** Expect: the re-run's first page still holds its Total back; the list ends in "Loading more rows…", never in a Total; the columns do not move. Test: Task 10 run-state test "holds the Total back again when a re-run's first page has a later page".
2. **A last page that holds only the whole-report rows** (the server's final page has no detail rows). Expect: the Total shows and "End of report" follows it. Test: Task 10 rows test "shows a last page of whole-report rows alone".
3. **One very long notice on a phone, or several short ones.** Expect: at most 2 lines and "Show all"; "Show all" shows every notice in full, the layout notice included; a desktop never caps. Test: Task 11 `isBandCut` cases and the e2e in `reports-wrap.spec.ts`.
4. **A filter chip with long text at 390.** Expect: the chip row scrolls sideways, a fade marks its right edge until the row is scrolled to its end; no fade on a desktop where the chips fit. Test: Task 12 e2e.
5. **A phone reader scrolls back to the top.** Expect: the heading row returns to just under the document header, nothing overlaps; at 1280 the title never moves while the rows scroll. Test: Task 13 e2e.

---

### Task 9: Acceptance at 390, 768, 1024 and 1280

**Files:**
- Create: `tests/e2e/reports-fit.spec.ts`

**Interfaces:**
- Consumes: Tasks 1–8 behaviour (part 1). e2e helpers `createInvitedUser`, `skipWithoutReportsStub` (`tests/e2e/helpers.ts`); `addColumn`, `addSale`, `addSales`, `addTeam`, `chooseOption`, `openReportStub`, `openReportsHub`, `SALES_SOURCE`, `saveNewReport`, `startReport`, `uniqueName` (`tests/e2e/reports-helpers.ts`). TestIDs: `report-table`, `report-heading-<col>`, `report-cell-<col>-<rowId>`, `report-row-<rowId>` (detail ids start `d:`).
- Produces: `tests/e2e/reports-fit.spec.ts` (`test.describe('report fit at four widths', …)`).

A ledger-like salesstub report (the customer label, three text columns, a date, three figures, grouped with subtotals and a Total) must, at each of the four widths: not scroll sideways; never cut a figure or date cell or its heading (a heading may use two lines, so its text box is checked for height too); show every group, subtotal and Total label whole; and put every figure cell's text inside its heading's box horizontally.

The builder's column order is: customer 0 (the label a new report starts with), status 1, currency 2, team name 3, day 4, amount 5, cost 6, quantity 7; so the aggregate menus are `report-builder-column-5-aggregate` (amount) and `-6-` (cost). Confirm with the builder (`reports-wrap.spec.ts`'s `wideReport` sets `report-builder-column-1-aggregate` for its first added column) before you run.

- [ ] **Step 1: Write the spec**

```ts
// tests/e2e/reports-fit.spec.ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addSales,
    addTeam,
    chooseOption,
    openReportStub,
    openReportsHub,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The layout fit goal: at 390, 768, 1024 and 1280 a ledger-like report (a label,
 * text columns, a date and amounts, grouped with subtotals and a Total) fits its
 * view with core defaults only: nothing scrolls sideways, no figure, date or
 * heading word is cut, every group, subtotal and Total label is whole, and every
 * figure stands under its heading.
 */
const WIDTHS = [
    { width: 390, height: 844 },
    { width: 768, height: 1024 },
    { width: 1024, height: 800 },
    { width: 1280, height: 800 },
]
const FIGURES = ['day', 'amount', 'cost', 'quantity']

async function fitProblems(p: Page) {
    return p.getByTestId('report-table').evaluate((table, figures) => {
        const cut = (el: Element | null) =>
            el instanceof HTMLElement &&
            (el.scrollWidth > el.clientWidth + 0.5 || el.scrollHeight > el.clientHeight + 0.5)
        const textOf = (el: Element) => el.querySelector('div[dir="auto"]')
        const problems: string[] = []
        if (table.scrollWidth > table.clientWidth) problems.push('table scrolls sideways')
        for (const col of figures) {
            const heading = table.querySelector(`[data-testid="report-heading-${col}"]`)
            if (!heading) {
                problems.push(`heading ${col} missing`)
                continue
            }
            if (cut(textOf(heading))) problems.push(`heading ${col} cut`)
            const head = heading.getBoundingClientRect()
            for (const cell of table.querySelectorAll(`[data-testid^="report-cell-${col}-"]`)) {
                const text = textOf(cell)
                if (!text?.textContent) continue
                const id = cell.getAttribute('data-testid')
                if (cut(text)) problems.push(`${id} cut`)
                const box = text.getBoundingClientRect()
                if (box.left < head.left - 1 || box.right > head.right + 1)
                    problems.push(`${id} outside its heading`)
            }
        }
        for (const row of table.querySelectorAll('[data-testid^="report-row-"]')) {
            const id = row.getAttribute('data-testid') ?? ''
            if (id.startsWith('report-row-d:')) continue
            const text = textOf(row)
            if (text && cut(text)) problems.push(`${id} label cut`)
        }
        return problems
    }, FIGURES)
}

test.describe('report fit at four widths', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a ledger-like report fits 390, 768, 1024 and 1280 with nothing cut that must not be', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptfit')
        try {
            const team = uniqueName('Northwest Regional Wholesale')
            await openReportStub(p)
            await addTeam(p, team)
            // Six-figure amounts: wider than the measure's worst case, so the
            // columns must take their width from the rows.
            await addSales(p, {
                team,
                customerPrefix: 'Fitco Wholesale Partners of the Northwest',
                count: 5,
                firstDay: '2026-01-05',
                firstAmount: 12345678,
                cost: 4000000,
                quantity: 12,
            })
            await addSale(p, {
                team,
                customer: 'Fitco Wholesale Partners of the Northwest 05',
                amount: 12345683,
                cost: 4000000,
                quantity: 12,
                day: '2026-01-10',
                status: 'paid',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            for (const keys of [
                ['status'],
                ['currency'],
                ['team', 'name'],
                ['day'],
                ['amount'],
                ['cost'],
                ['quantity'],
            ]) {
                await addColumn(p, keys)
            }
            await chooseOption(p, 'report-builder-column-5-aggregate', 'sum')
            await chooseOption(p, 'report-builder-column-6-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-group-0-open').click()
            await saveNewReport(p, uniqueName('Fit'))

            for (const size of WIDTHS) {
                await p.setViewportSize(size)
                await expect(p.getByTestId('report-table')).toHaveCSS('opacity', '1')
                await expect
                    .poll(() => fitProblems(p), { message: `fit at ${size.width}` })
                    .toEqual([])
                expect(
                    await p.evaluate(() => document.documentElement.scrollWidth)
                ).toBeLessThanOrEqual(size.width)
            }
        } finally {
            await close()
        }
    })
})
```

- [ ] **Step 2: Run it**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-fit.spec.ts`
Expected: PASS. A failure names the width and the element (for example `["report-cell-amount-total:[\"USD\"] cut"]`). Fix the cause in the layout code of the part-1 task that owns it (`column-measure.ts` for a width, `table-lines.ts` for a line or a span, `ReportRow.tsx` / `ReportTable.tsx` for drawing), never the assertion. If the cause is in part 1's code, write one line to `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/retro-log.md` naming the part-1 task.

- [ ] **Step 3: Lint and commit**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check tests/e2e/reports-fit.spec.ts`

```bash
git add tests/e2e/reports-fit.spec.ts
git commit -m "reports: e2e for the layout fit at four widths"
```

E2E this task breaks: none.

---

### Task 10: H1 and D12 — the Total only with the last page; "Loading more rows…" and "End of report" rows

**Files:**
- Modify: `core/reports/lib/rows.ts` (add `holdWholeReportRows` after `mergePages`, `:203-221`)
- Modify: `core/reports/lib/run-state.ts:24-77` (`RunView.heldRows`; `mergedResult` holds the rows back)
- Modify: `core/reports/hooks/useReportRun.ts:29-48` (`ReportRun`: optional `heldRows?`)
- Modify: `core/reports/hooks/useTableLayout.ts` (created by part-1 Task 4: `useRunMeasures` reads the held rows)
- Modify: `core/reports/components/ReportTable.tsx` (part-1 Tasks 4, 5, 7, 8 change it: `LoadingMore` → `TableEnd`; prop `isLoadingMore` → `hasMore`)
- Modify: `core/reports/components/ReportView.tsx` (part-1 Tasks 4, 7: `TableBody` passes `hasMore`)
- Modify: `core/help/reports.md` ("When columns do not fit", last sentence), `docs/packages.md:1306-1309`
- Test: `core/reports/lib/__tests__/rows.test.ts`, `core/reports/lib/__tests__/run-state.test.ts`
- Test (e2e): `tests/e2e/reports-ledger-v3.spec.ts` (`pagedTeams`; new test)

**Interfaces:**
- Consumes: `withoutAppendedTotals`, `isWholeReportRow` (`rows.ts:185-194`, module-private), `mergePages` (`rows.ts:203`); `TableLayoutModel`, `useRunMeasures` (part-1 Task 4, `useTableLayout.ts`); `measureColumns({ columns, rows, format, levels })` (part-1 Task 2); `visibleRows` (`rows.ts:57`).
- Produces:
  - `type ShownResult = { result: RunResult; held: readonly ResultRow[] }`; `holdWholeReportRows(result: RunResult): ShownResult` (`rows.ts`).
  - `RunView.heldRows: readonly ResultRow[]` (`run-state.ts`); `ReportRun` = `Omit<RunView, 'topics' | 'heldRows'>` plus `heldRows?: readonly ResultRow[]` (optional: a hand-built `ReportRun`, such as Books' mockup, has none).
  - `ReportTableProps.hasMore: boolean` (replaces `isLoadingMore`); `TableEnd({ hasMore })` in `ReportTable.tsx`; testIDs `report-loading-more` (now with the text "Loading more rows…") and `report-end` ("End of report").

Rule (D8, D12, review §11.3, §11.5-1): every server page ends with the whole-report rows (`assemble.go` `pageWindow`). While the newest page has `page.next`, the view drops that page's trailing run of total and computed rows from the list and keeps them as `heldRows`; the list then ends in "Loading more rows…" (a spinner and the text). When no later page is left, the Total shows and "End of report" ends the list. A Total inside a page's slice (one per currency block) keeps its place. The measure reads `model.rows` and the held rows, so the Total's width is in the measure from the first page (outline T10 constraint).

- [ ] **Step 1: Write the failing unit tests**

In `core/reports/lib/__tests__/rows.test.ts`, add `holdWholeReportRows` to the import from `'../rows'`, and after the `describe('mergePages', …)` block add:

```ts
describe('holdWholeReportRows', () => {
    const pageOf = (next: string, rows: ResultRow[]): RunResult => ({
        columns: [{ id: 'cost', label: 'Cost', type: 'money' }],
        rows,
        notices: [],
        topics: [],
        page: { size: rows.length, next },
        params: [],
    })
    const net: ResultRow = { id: 'c:net', kind: 'computed', depth: 0, label: 'Net', cells: { cost: 5 } }

    it('holds the trailing Total and computed rows back while a later page is left', () => {
        const shown = holdWholeReportRows(pageOf('next', [detail('d:1'), net, total]))
        expect(shown.result.rows.map(r => r.id)).toEqual(['d:1'])
        expect(shown.held.map(r => r.id)).toEqual(['c:net', 'total'])
    })

    it('shows them with the last page, and leaves an unpaged result as it is', () => {
        const last = pageOf('', [detail('d:1'), total])
        expect(holdWholeReportRows(last)).toEqual({ result: last, held: [] })
        const unpaged: RunResult = { ...last, page: null }
        expect(holdWholeReportRows(unpaged).result).toBe(unpaged)
    })

    it('keeps a Total inside the page’s slice in its place', () => {
        const block: ResultRow = { ...total, id: 'total:EUR' }
        const shown = holdWholeReportRows(pageOf('next', [detail('d:1'), block, detail('d:2'), total]))
        expect(shown.result.rows.map(r => r.id)).toEqual(['d:1', 'total:EUR', 'd:2'])
        expect(shown.held.map(r => r.id)).toEqual(['total'])
    })

    it('shows a last page of whole-report rows alone', () => {
        const merged = mergePages([pageOf('next', [detail('d:1'), total]), pageOf('', [total])])
        const shown = holdWholeReportRows(merged)
        expect(shown.result.rows.map(r => r.id)).toEqual(['d:1', 'total'])
        expect(shown.held).toEqual([])
    })
})
```

In `core/reports/lib/__tests__/run-state.test.ts`:
- In `it('is idle with no body', …)` add `heldRows: [],` to the expected object (after `topics: [],`).
- Add, inside `describe('runView', …)`:

```ts
    const withTotal = (next: string): RunOutcome => {
        const base = result(['collection/sales'])
        return {
            kind: 'ok',
            result: {
                ...base,
                rows: [...base.rows, { id: 'total', kind: 'total', depth: 0, label: 'Total', cells: {} }],
                page: { size: 1, next },
            },
        }
    }

    it('holds the Total back while a later page is left, and shows it with the last page', () => {
        const first = runView(query({ pages: [withTotal('c1')] }))
        expect(first.result?.rows.map(row => row.id)).toEqual(['d:1'])
        expect(first.heldRows.map(row => row.id)).toEqual(['total'])
        const all = runView(query({ pages: [withTotal('c1'), withTotal('')] }))
        expect(all.result?.rows.map(row => row.id)).toEqual(['d:1', 'total'])
        expect(all.heldRows).toEqual([])
    })

    it('holds the Total back again when a re-run’s first page has a later page', () => {
        const pages = [withTotal('c1')]
        const shown = runView(query({ pages }))
        const updating = runView(query({ pages, isPlaceholderData: true }))
        expect(updating.result).toBe(shown.result)
        expect(updating.heldRows).toBe(shown.heldRows)
        expect(runView(query({ pages: [withTotal('c2')] })).heldRows.map(row => row.id)).toEqual(['total'])
    })
```

- [ ] **Step 2: Run them to verify they fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/rows.test.ts core/reports/lib/__tests__/run-state.test.ts`
Expected: FAIL — `holdWholeReportRows` is not exported; `heldRows` is undefined.

- [ ] **Step 3: Write the lib code**

`core/reports/lib/rows.ts`, after `mergePages`:

```ts
/** The merged run as the list shows it, and the whole-report rows it holds back. */
export type ShownResult = { result: RunResult; held: readonly ResultRow[] }

const NO_HELD: readonly ResultRow[] = []

// D8 (H1): every page ends with the whole-report rows (the Total, a layout's
// computed rows). While a later page is left, the list must not end in them, or
// the reader sees an end that is not the end; they are held back until the last
// page lands. The column measure still reads them (useTableLayout), so the Total's
// width is known from the first page. Not done in mergePages: runFirstPage joins
// several server pages into one first page with it, and must keep the Total.
export function holdWholeReportRows(result: RunResult): ShownResult {
    if (!result.page?.next) return { result, held: NO_HELD }
    const rows = withoutAppendedTotals(result.rows)
    if (rows.length === result.rows.length) return { result, held: NO_HELD }
    return { result: { ...result, rows: [...rows] }, held: result.rows.slice(rows.length) }
}
```

Note: the test `toEqual({ result: last, held: [] })` compares `NO_HELD` by value; that is intended.

`core/reports/lib/run-state.ts`:
- Import `holdWholeReportRows, type ShownResult` from `'./rows'` (keep `mergePages`) and `type ResultRow` from `'./types'`.
- `RunView` gains, after `result`:

```ts
    /**
     * The whole-report rows (Total, computed) held back from `result` while a later
     * page is left (D8). The column measure reads them; the list does not show them.
     */
    heldRows: readonly ResultRow[]
```

- Add `const NO_ROWS: readonly ResultRow[] = []`.
- `mergedByPages` becomes `new WeakMap<readonly RunOutcome[], ShownResult | undefined>()`, and `mergedResult` returns `ShownResult | undefined`: `const shown = results.length > 0 ? holdWholeReportRows(mergePages(results)) : undefined`. Rename it `shownResult` and update its comment's first words to "One shown result per pages array".
- `runView`: `const shown = shownResult(query.pages)`; `result: refusal ? undefined : shown?.result`; add `heldRows: refusal ? NO_ROWS : (shown?.held ?? NO_ROWS)`; `topics: shown?.result.topics ?? NO_TOPICS`.

`core/reports/hooks/useReportRun.ts`: change the interface head to

```ts
export interface ReportRun extends Omit<RunView, 'topics' | 'heldRows'> {
    /** Optional, because a hand-built ReportRun (a mockup) holds nothing back. */
    heldRows?: readonly ResultRow[]
```

and import `type ResultRow` with `RunResult` from `'../lib/types'`. The returned object already spreads `view`, so `heldRows` reaches the model.

Run Step 2's command: PASS. Then run `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports` and fix every other test that read a paged result's Total before its last page (the `mergePages` tests are unchanged, since `mergePages` is unchanged).

- [ ] **Step 4: The measure reads the held rows**

In `core/reports/hooks/useTableLayout.ts` (part-1 Task 4), add above `useRunMeasures`:

```ts
// D1 measures the first rows, the whole-report rows included. While a later page
// is left those rows are held back from the list (D8), so the measure reads them here.
// They go first: measureColumns reads only its first MEASURE_ROW_LIMIT rows, and a
// fold's first page can be longer than that.
function measuredRows(model: TableLayoutModel): readonly VisibleRow[] {
    const held = model.run.heldRows ?? []
    return held.length === 0 ? model.rows : [...visibleRows(held), ...model.rows]
}
```

and in `useRunMeasures`, in the `measureColumns({ … })` call, replace `rows: model.rows,` with `rows: measuredRows(model),`. Import `type VisibleRow, visibleRows` from `'../lib/rows'`. The measure key is unchanged: the Total arriving with the last page does not take the measure again (D1).

- [ ] **Step 5: The list's end row**

`core/reports/components/ReportTable.tsx` (state after part-1 Task 8):
- Replace the `LoadingMore` component with:

```tsx
// D8, D12: the list ends in what comes next. While a later page is left the Total
// is held back, and this row stands in its place; after the last page, a short
// end row (a page footer is for exports only).
function TableEnd({ hasMore }: { hasMore: boolean }) {
    if (hasMore) {
        return (
            <View
                testID="report-loading-more"
                className="flex-row items-center justify-center gap-2 py-3"
            >
                <ActivityIndicator />
                <Text className="text-sm text-muted-foreground">Loading more rows…</Text>
            </View>
        )
    }
    return (
        <View testID="report-end" className="items-center py-3">
            <Text className="text-sm text-muted-foreground">End of report</Text>
        </View>
    )
}
```

- In `TableListProps` and `ReportTableProps`, replace `isLoadingMore: boolean` with

```ts
    /** A later page is left: the list ends in "Loading more rows…", else in "End of report". */
    hasMore: boolean
```

- In `TableList`, destructure `hasMore` in place of `isLoadingMore` and use `ListFooterComponent={<TableEnd hasMore={hasMore} />}`. In `ReportTable`, pass `hasMore={props.hasMore}` (rename every `isLoadingMore` in the file; grep to be sure).
- Update the comment above `ReportTable` ("FlashList measures every row…") by appending: "The list ends in TableEnd: more rows on their way, or the end of the report."

`core/reports/components/ReportView.tsx`, in `TableBody`: replace `isLoadingMore={model.run.isLoadingMore}` with `hasMore={model.run.hasMore}`.

- [ ] **Step 6: Help and docs**

`core/help/reports.md`, "When columns do not fit": replace the last sentence "A long report loads more rows as you scroll, also when it has groups." with "A long report loads more rows as you scroll, also when it has groups. Until its last rows have loaded, the list ends with **Loading more rows…**; the Total shows with the last rows, and **End of report** marks the end."

`docs/packages.md:1306-1309`: replace the bullet with:

```md
- A grouped result pages (`page.next`); running values continue on the next
  page. A client merges pages by row id and drops each earlier page's trailing
  total and computed rows (a Total inside a page's slice, one per currency
  block, stays). Core's view also holds the newest page's trailing total and
  computed rows back while it has `page.next` (`holdWholeReportRows`), so the
  Total shows only with the last page; its column measure still reads them.
```

- [ ] **Step 7: Write the e2e**

In `tests/e2e/reports-ledger-v3.spec.ts`, split `pagedTeams` (it starts `async function pagedTeams(page: Page, prefix: string, smallTeams = 0)`):

1. Rename its body up to and including `await expect(firstSale).toBeVisible()` to a new function `openPagedTeams(page: Page, prefix: string, smallTeams = 0, nextPages?: Promise<void>)`. In its `page.route('**/api/reports/run', …)` handler, add as the first line after `const body = route.request().postDataJSON()`: `if (body.options?.cursor && nextPages) await nextPages`. It returns `{ stem, teams, cursors, sent, firstSale }`. Update its doc comment: "Signs in, seeds two teams of six sales each (team A first), and `smallTeams` more teams of one sale each after them, and opens the report on them alone, in pages of 5 detail rows, so 12 sales page across team A's end. A next page (a request with a cursor) waits for `nextPages` when it is given."
2. `pagedTeams(page, prefix, smallTeams = 0)` keeps its doc comment and becomes:

```ts
async function pagedTeams(page: Page, prefix: string, smallTeams = 0) {
    const { firstSale, ...paged } = await openPagedTeams(page, prefix, smallTeams)
    const scroller = await listScroller(firstSale)
    await expect
        .poll(async () => {
            await scroller.evaluate(el => {
                el.scrollTop = el.scrollHeight
            })
            return rowOf(page, `Total ${paged.teams[paged.teams.length - 1]}`).count()
        })
        .toBe(1)
    return paged
}
```

Then add after `test('a grouped report scrolled past its first page keeps its running balance', …)`:

```ts
    // D8, D12: while a later page is left, the list ends in "Loading more rows…", never
    // in the Total; the Total comes with the last page, and "End of report" ends the list.
    // The amount column keeps its width when the Total lands (the measure held it).
    test('the Total waits for the last page, a loading row stands in its place, and an end row follows it', async ({
        page,
    }) => {
        let release = () => {}
        const nextPages = new Promise<void>(resolve => {
            release = resolve
        })
        const { teams, firstSale } = await openPagedTeams(page, 'BalEnd', 0, nextPages)
        const loading = page.getByTestId('report-loading-more')
        await expect(loading).toContainText('Loading more rows…')
        expect(occurrences(await rowIds(page), 'total')).toBe(0)
        await expect(page.getByTestId('report-end')).toHaveCount(0)
        const amountWidth = (await page.getByTestId('report-heading-amount').boundingBox())?.width

        release()
        const scroller = await listScroller(firstSale)
        await expect
            .poll(async () => {
                await scroller.evaluate(el => {
                    el.scrollTop = el.scrollHeight
                })
                return occurrences(await rowIds(page), 'total')
            })
            .toBe(1)
        await expect(rowOf(page, `Total ${teams[teams.length - 1]}`)).toHaveCount(1)
        await expect(page.getByTestId('report-end')).toHaveText('End of report')
        await expect(loading).toHaveCount(0)
        const ids = await rowIds(page)
        expect(ids[ids.length - 1]).toMatch(/^total/)
        expect((await page.getByTestId('report-heading-amount').boundingBox())?.width).toBe(amountWidth)
    })
```

(The hold also catches the first, unfiltered run's next page, which is harmless: `showTeams` starts a new run under a new key. `report-heading-amount` is the stub team balance's amount column; if the shipped report names it differently, read its id from `tests/scripts/reports-stub/server/register.go.txt`.)

- [ ] **Step 8: Run tests**

Run: `pnpm exec tinycld-pkg typecheck`; `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`; `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-ledger-v3.spec.ts`
Expected: PASS.

- [ ] **Step 9: Lint and commit**

```bash
git add core/reports/lib/rows.ts core/reports/lib/run-state.ts core/reports/hooks/useReportRun.ts core/reports/hooks/useTableLayout.ts core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/help/reports.md docs/packages.md core/reports/lib/__tests__/rows.test.ts core/reports/lib/__tests__/run-state.test.ts tests/e2e/reports-ledger-v3.spec.ts
git commit -m "reports: show the Total only with the last page, and mark the end of a report"
```

E2E this task breaks: none in core expected (`pagedTeams` waits for the last team's subtotal row, which is not a whole-report row; `a grouped report scrolled past its first page…` reads the Total after every page is merged). Books (after merge): re-check any spec that expects a paged report's Total before its last page.

---

### Task 11: H5 — the layout notice in the notice band; 2 lines and "Show all" on a phone

**Files:**
- Create: `core/reports/lib/notices.ts`
- Modify: `core/reports/components/ReportNotices.tsx:1-45`
- Modify: `core/reports/components/ReportTable.tsx` (part-1 Tasks 4–8 change it: delete `LayoutNotice` and its use)
- Modify: `core/reports/components/ReportView.tsx` (part-1 Tasks 4, 7; Task 10: pass `layoutNotice`, `isCompact`)
- Modify: `core/reports/components/builder/ReportBuilderPanel.tsx` (part-1 Task 4: `SheetSurface` passes `layoutNotice`)
- Modify: `core/help/reports.md` ("When columns do not fit")
- Test: `core/reports/lib/__tests__/notices.test.ts`
- Test (e2e): `tests/e2e/reports-wrap.spec.ts` (`an arrangement names its problems in the builder, and a pivot that cannot follow it says so`)

**Interfaces:**
- Consumes: `textWidth`, `TextStyle` (part-1 Task 1, `core/reports/lib/text-width.ts`); `LineLayout` (part-1 Task 5: blocks variant `{ kind: 'blocks'; root; indent; notice?: string }`); `TableLayout` from `useTableLayout` (part-1 Task 4; `table` in `ReportView` and `SheetSurface`); `useWindowWidth` (`@tinycld/core/components/workspace/useBreakpoint`); `REPORT_TAP_TARGET` (`ReportHeader.tsx`); `Notice` (`core/reports/lib/types.ts`).
- Produces (`core/reports/lib/notices.ts`): `LAYOUT_NOTICE_CODE = 'layout'`; `layoutNoticeOf(layout: LineLayout): string | undefined`; `bandNotices(notices: readonly Notice[], layoutNotice: string | undefined): Notice[]`; `BAND_LINES = 2`; `isBandCut(messages: readonly string[], width: number): boolean`. Component: `ReportNotices({ notices, layoutNotice, isLive = true, isCompact = false }: { notices: Notice[]; layoutNotice?: string; isLive?: boolean; isCompact?: boolean })`; testIDs `report-notice-layout`, `report-notices-toggle`.

Rule (H5, review §11.1, §11.5-5): the table's layout notice (`PIVOT_NOTICE`) is one more notice in the band, last, with code `layout`, in the band's style; the table no longer draws its own. On a phone (`isCompact`) a band whose notices need more than 2 lines (estimated with `textWidth`, word by word, at the window width less the band's padding) shows the notices as one paragraph cut at 2 lines and a **Show all** button; **Show all** shows every notice in full, each on its own line, and the button reads **Show less**. A desktop or tablet never caps. The live region is unchanged.

- [ ] **Step 1: Write the failing test**

```ts
// core/reports/lib/__tests__/notices.test.ts
import { describe, expect, it } from 'vitest'
import { BAND_LINES, bandNotices, isBandCut, LAYOUT_NOTICE_CODE, layoutNoticeOf } from '../notices'
import type { LineLayout } from '../table-lines'
import { textWidth } from '../text-width'

const TEXT = { size: 'sm', weight: 'normal' } as const

describe('bandNotices', () => {
    const cap = { code: 'rowCap', message: 'The report shows its first 5000 rows.' }

    it('puts the layout notice last, and adds nothing without one', () => {
        expect(bandNotices([cap], 'Laid out automatically.')).toEqual([
            cap,
            { code: LAYOUT_NOTICE_CODE, message: 'Laid out automatically.' },
        ])
        expect(bandNotices([cap], undefined)).toEqual([cap])
    })
})

describe('layoutNoticeOf', () => {
    it('reads the notice of a wrapped layout only', () => {
        const root = { kind: 'cell', col: 'a', justify: 'start' } as const
        const blocks: LineLayout = { kind: 'blocks', root, indent: 0, notice: 'Moved.' }
        expect(layoutNoticeOf(blocks)).toBe('Moved.')
        expect(layoutNoticeOf({ ...blocks, notice: undefined })).toBeUndefined()
        expect(layoutNoticeOf({ kind: 'one-line', widths: {} } as LineLayout)).toBeUndefined()
    })
})

describe('isBandCut', () => {
    const line = 'Rows capped here.'
    const width = textWidth(line, TEXT) + 1

    it('fits as many one-line notices as the band has lines', () => {
        expect(BAND_LINES).toBe(2)
        expect(isBandCut([line], width)).toBe(false)
        expect(isBandCut([line, line], width)).toBe(false)
        expect(isBandCut([line, line, line], width)).toBe(true)
    })

    it('counts the lines a long notice wraps to, word by word', () => {
        expect(isBandCut([`${line} ${line}`], width)).toBe(false)
        expect(isBandCut([`${line} ${line} ${line}`], width)).toBe(true)
        // A word wider than the line breaks inside itself.
        expect(isBandCut(['x'.repeat(200)], width)).toBe(true)
    })

    it('cuts nothing with no notices or no width yet', () => {
        expect(isBandCut([], width)).toBe(false)
        expect(isBandCut([line, line, line], 0)).toBe(false)
    })
})
```

(The `blocks` root and the `one-line` shape are part 1's `Block` and `LineLayout`; adjust the literal to part 1's exact field names — read `core/reports/lib/table-lines.ts` — keeping what each case asserts.)

- [ ] **Step 2: Run it to verify it fails**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/notices.test.ts`
Expected: FAIL — `Failed to resolve import "../notices"`.

- [ ] **Step 3: Write the lib code**

```ts
// core/reports/lib/notices.ts
import type { LineLayout } from './table-lines'
import { textWidth } from './text-width'
import type { Notice } from './types'

/** The code of the table's own notice: an arrangement it could not follow (PIVOT_NOTICE). */
export const LAYOUT_NOTICE_CODE = 'layout'

export function layoutNoticeOf(layout: LineLayout): string | undefined {
    return layout.kind === 'blocks' ? layout.notice : undefined
}

/** H5: one band holds the run's notices and, last, the table's layout notice. */
export function bandNotices(notices: readonly Notice[], layoutNotice: string | undefined): Notice[] {
    if (!layoutNotice) return [...notices]
    return [...notices, { code: LAYOUT_NOTICE_CODE, message: layoutNotice }]
}

/** The lines a phone's band shows before "Show all". */
export const BAND_LINES = 2

const NOTICE_TEXT = { size: 'sm', weight: 'normal' } as const

// The lines a text wraps to at this width, word by word, with textWidth's upper
// estimate: a band may offer "Show all" when its text would just fit, never the
// reverse. No layout pass, so the band draws once.
function lineCount(text: string, width: number): number {
    const space = textWidth(' ', NOTICE_TEXT)
    let lines = 1
    let used = 0
    for (const word of text.split(/\s+/).filter(Boolean)) {
        const wordWidth = textWidth(word, NOTICE_TEXT)
        if (used > 0 && used + space + wordWidth > width) {
            lines += 1
            used = wordWidth
        } else {
            used += (used > 0 ? space : 0) + wordWidth
        }
        if (used > width) {
            lines += Math.ceil(used / width) - 1
            used %= width
        }
    }
    return lines
}

/** Whether these notices, each starting a line, need more than BAND_LINES lines. */
export function isBandCut(messages: readonly string[], width: number): boolean {
    if (width <= 0) return false
    let lines = 0
    for (const message of messages) lines += lineCount(message, width)
    return lines > BAND_LINES
}
```

Run Step 2's command: PASS.

- [ ] **Step 4: The band**

Replace `core/reports/components/ReportNotices.tsx` with:

```tsx
import { useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { BAND_LINES, bandNotices, isBandCut } from '../lib/notices'
import type { Notice } from '../lib/types'
import { REPORT_TAP_TARGET } from './ReportHeader'

// px-4 on each side of the band.
const BAND_PADDING = 32
const TOGGLE_STYLE = { minHeight: REPORT_TAP_TARGET }

// On a phone a band of more than BAND_LINES lines is cut there until the reader
// asks for all of it (H5). The choice is this band's own UI state.
function useNoticeBand(notices: Notice[], isCompact: boolean) {
    const [isExpanded, setIsExpanded] = useState(false)
    const width = useWindowWidth() - BAND_PADDING
    const isCut = isCompact && isBandCut(notices.map(notice => notice.message), width)
    return {
        isCut,
        isCapped: isCut && !isExpanded,
        isExpanded,
        toggleText: isExpanded ? 'Show less' : 'Show all',
        onToggle: () => setIsExpanded(open => !open),
    }
}

// mergePages keeps one notice per code, and the layout notice has its own code,
// so the code is a stable key.
function noticeLines(notices: Notice[]) {
    return notices.map(notice => (
        <Text key={notice.code} testID={`report-notice-${notice.code}`} className="text-sm text-foreground">
            {notice.message}
        </Text>
    ))
}

// Capped: the notices run on as one paragraph, cut at BAND_LINES lines.
function noticeParagraph(notices: Notice[]) {
    const spans = notices.map(notice => (
        <Text key={notice.code} testID={`report-notice-${notice.code}`}>
            {`${notice.message} `}
        </Text>
    ))
    return (
        <Text numberOfLines={BAND_LINES} className="text-sm text-foreground">
            {spans}
        </Text>
    )
}

function BandToggle(props: { isVisible: boolean; isExpanded: boolean; text: string; onPress(): void }) {
    if (!props.isVisible) return null
    return (
        <Pressable
            testID="report-notices-toggle"
            accessibilityRole="button"
            accessibilityState={{ expanded: props.isExpanded }}
            onPress={props.onPress}
            className="justify-center self-start"
            style={TOGGLE_STYLE}
        >
            <Text className="text-sm font-semibold text-primary">{props.text}</Text>
        </Pressable>
    )
}

function NoticeBand({ notices, isCompact }: { notices: Notice[]; isCompact: boolean }) {
    const band = useNoticeBand(notices, isCompact)
    if (notices.length === 0) return null
    const body = band.isCapped ? noticeParagraph(notices) : noticeLines(notices)
    return (
        <View testID="report-notices" className="gap-1 border-t border-border bg-surface-secondary px-4 py-2">
            {body}
            <BandToggle
                isVisible={band.isCut}
                isExpanded={band.isExpanded}
                text={band.toggleText}
                onPress={band.onToggle}
            />
        </View>
    )
}

// What the run says about itself (caps that applied, mixed currencies, an upgraded
// spec, renamed fields; spec "Errors"), and, last, the table's own layout notice
// (H5): one band above the table. "Updating…" is not one of them: it has its own
// slot in the header (ReportUpdating). The live region is always mounted, empty and
// without height when there are no notices, so a screen reader announces notices
// when they appear in it. Under the phone's builder sheet, whose Preview shows the
// notices again, the view's band is not a live region, so each notice is announced once.
export function ReportNotices({
    notices,
    layoutNotice,
    isLive = true,
    isCompact = false,
}: {
    notices: Notice[]
    layoutNotice?: string
    isLive?: boolean
    isCompact?: boolean
}) {
    const all = bandNotices(notices, layoutNotice)
    if (!isLive) return <NoticeBand notices={all} isCompact={isCompact} />
    return (
        <View testID="report-notices-region" role="status" aria-live="polite" accessibilityLiveRegion="polite">
            <NoticeBand notices={all} isCompact={isCompact} />
        </View>
    )
}
```

(`useNoticeBand` is called before the early return; `const body = …` is a ternary above the return, not inside the tree. Biome formats the long lines.)

- [ ] **Step 5: Wire it; drop the table's own notice**

`core/reports/components/ReportTable.tsx`: delete the `LayoutNotice` component, its use `<LayoutNotice text={notice} />` in `ReportTable`, and the `const notice = …` line above it.

`core/reports/components/ReportView.tsx`, in `ReportView` (above the return, after `const table = useTableLayout(model)` from part-1 Task 4):

```tsx
    // The table's layout notice joins the run's in the band, once the table is drawn.
    const layoutNotice = model.state === 'ready' ? layoutNoticeOf(table.layout) : undefined
    const isCompact = model.breakpoint === 'mobile'
```

and render `<ReportNotices notices={model.notices} layoutNotice={layoutNotice} isLive={model.isBodyShown} isCompact={isCompact} />`. Import `layoutNoticeOf` from `'../lib/notices'`.

`core/reports/components/builder/ReportBuilderPanel.tsx`, in `SheetSurface` (it has `const table = useTableLayout(preview)` from part-1 Task 4): add above the return `const layoutNotice = preview.state === 'ready' ? layoutNoticeOf(table.layout) : undefined` and render `<ReportNotices notices={preview.notices} layoutNotice={layoutNotice} isCompact />`. The drawer surface draws no `ReportNotices` of its own (the view's band shows), so it needs no change.

- [ ] **Step 6: Help**

`core/help/reports.md`, "When columns do not fit": after "When the columns still do not fit, each row wraps: …", add: "Notes about the report, such as a pivot that cannot follow an arrangement, show in one band above the report. On a phone the band shows two lines; select **Show all** to read every note."

- [ ] **Step 7: Update the e2e this task changes**

First check that the pivot notice still shows at 390 after part-1 Task 5 (one line is tried first): run the test as it is. If it fails because one line fits at 390, add the `status` and `quantity` columns to that test's report (with `addColumn`) so that one line cannot fit, and say so in its comment.

Then, in `tests/e2e/reports-wrap.spec.ts` › `an arrangement names its problems in the builder, and a pivot that cannot follow it says so`, replace its last assertion `await expect(p.getByTestId('report-layout-notice')).toBeVisible()` with:

```ts
            // H5: the layout notice is in the one notice band, not in the table; on a
            // phone the band is cut at two lines until the reader selects Show all.
            const band = p.getByTestId('report-notices-region')
            await expect(band.getByTestId('report-notice-layout')).toContainText(
                'The pivot columns do not fit'
            )
            await expect(p.getByTestId('report-table').getByTestId('report-notice-layout')).toHaveCount(0)
            const toggle = band.getByTestId('report-notices-toggle')
            await expect(toggle).toHaveText('Show all')
            const capped = (await band.getByTestId('report-notices').boundingBox())?.height ?? 0
            await toggle.click()
            await expect(toggle).toHaveText('Show less')
            await expect
                .poll(async () => (await band.getByTestId('report-notices').boundingBox())?.height ?? 0)
                .toBeGreaterThan(capped)
            // A desktop never cuts the band.
            await p.setViewportSize({ width: 1280, height: 800 })
            await expect(band.getByTestId('report-notices-toggle')).toHaveCount(0)
```

(At 390 the notice's ~100 characters need about three lines of the ~358 px band, so the toggle shows. At 1280 the report is one line, so the layout notice may be gone too; the toggle check holds either way.)

- [ ] **Step 8: Run tests**

Run: `pnpm exec tinycld-pkg typecheck`; `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`; `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts` and `… -- reports-view.spec.ts` (its `report-notices-region` test checks the region is empty before any notice; the report there is one line, so no layout notice).
Expected: PASS.

- [ ] **Step 9: Lint and commit**

```bash
git add core/reports/lib/notices.ts core/reports/lib/__tests__/notices.test.ts core/reports/components/ReportNotices.tsx core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/reports/components/builder/ReportBuilderPanel.tsx core/help/reports.md tests/e2e/reports-wrap.spec.ts
git commit -m "reports: one notice band with the layout notice, cut at two lines on a phone"
```

E2E this task breaks (fixed in Step 7): `reports-wrap.spec.ts` › `an arrangement names its problems…` (its `report-layout-notice` assertion). Books: none (grep finds no `report-layout-notice` there).

---

### Task 12: D11 / H2 — a preset-only period chip; the chip row keeps its side scroll with a fade

**Files:**
- Modify: `core/reports/lib/params.ts:291-296` (delete `resolvedDates`), `:348-390` (`valueText`, `chipText`)
- Modify: `core/reports/components/ReportControls.tsx` (part-1 Task 7 changes it: `chipText` call; fade)
- Modify: `core/help/reports.md` ("Run a report")
- Test: `core/reports/lib/__tests__/params.test.ts`
- Test (e2e): `tests/e2e/reports-view.spec.ts:138-140`, `:521-523`, new test

**Interfaces:**
- Consumes: `PRESET_LABELS` (`params.ts`); `ReportControls({ model, isSortMenuShown })` and `SortControl` (part-1 Task 7); `ROW_STYLE` (`ReportControls.tsx`).
- Produces: `valueText(def: ParamDef, value: ParamValue | null, labels?: ChoiceLabels): string`; `chipText(def: ParamDef, value: ParamValue | null, labels?: ChoiceLabels): string` (no `resolved`); `useEdgeFade()` and `EdgeFade({ isVisible })` (module-private in `ReportControls.tsx`); testID `report-controls-fade`.

Rule (D11, review §11.1, §11.5-2): a preset on the chip is its label alone ("Day: This fiscal year"); the document header (`report-document-period`) carries the days. A set day or range keeps its days on the chip. The chip row still scrolls sideways and never wraps; while chips lie past its right edge, four `bg-background` strips of rising opacity (24 px in all) fade that edge. The fade takes no touches.

- [ ] **Step 1: Write the failing test**

In `core/reports/lib/__tests__/params.test.ts`:
- Replace the test `it('shows a preset with the days the server resolved', …)` with:

```ts
    it('names a preset alone; the document header carries its days', () => {
        expect(chipText(PERIOD, { kind: 'preset', preset: 'thisMonth' })).toBe('Period: This month')
    })
```

- Remove the third argument (`undefined` or `resolved`) from every other `chipText(...)` call in the file, so `chipText(def, value)` or `chipText(def, value, labels)`. For example `chipText(def, { kind: 'choice', values: ['a1'] }, undefined, labels)` becomes `chipText(def, { kind: 'choice', values: ['a1'] }, labels)`, and `chipText(PERIOD, { kind: 'preset', preset: 'allDates' }, allDates)` becomes `chipText(PERIOD, { kind: 'preset', preset: 'allDates' })`. If a `resolved`/`allDates` constant is then unused in a test, keep it only where another call (`periodWords`, `emptyMessage`) reads it.

- [ ] **Step 2: Run it to verify it fails**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/params.test.ts`
Expected: FAIL — the labels tests read the old third parameter as `resolved` (names show as "1 selected"). The new preset test passes already (no `resolved` is given); it guards the change. (Typecheck also fails on the arity until Step 3.)

- [ ] **Step 3: Write the lib code**

In `core/reports/lib/params.ts`:
- Delete `resolvedDates` (`:291-296`).
- `valueText(def: ParamDef, value: ParamValue | null, labels?: ChoiceLabels): string`; its `case 'preset':` becomes `return PRESET_LABELS[value.preset]`; `case 'choice': return choiceLabels(def, value, labels)` as before.
- `chipText`:

```ts
// D11: a preset is named alone on its chip ("Day: This fiscal year"); the document
// header under the chips carries the days, so the chip stays short on a phone.
export function chipText(def: ParamDef, value: ParamValue | null, labels?: ChoiceLabels): string {
    const text = valueText(def, value, labels)
    return def.label ? `${def.label}: ${text}` : text
}
```

- Remove an import that is then unused (biome reports it; `rangeChipText`, `shortDay` and `isIsoDay` stay if other code in the file uses them).

In `core/reports/components/ReportControls.tsx`, `ParamControl`: `text={chipText(def, value, model.optionLabels.get(def.id) ?? undefined)}`.

Run Step 2's command and `pnpm exec tinycld-pkg typecheck`: PASS.

- [ ] **Step 4: The fade**

In `core/reports/components/ReportControls.tsx` (after part-1 Task 7), add:

```tsx
// Opacities of the strips that fade the chip row's right edge, left to right.
const FADE_STEPS = [0.25, 0.5, 0.75, 0.95] as const
const FADE_STRIP = { width: 6 }
const FADE_STYLE = { position: 'absolute', top: 0, right: 0, bottom: 0, pointerEvents: 'none' } as const

// D11: the chips keep their side scroll; while chips lie past the right edge, the
// edge fades so the reader sees there is more. Sizes are kept in a ref and only the
// answer is state, so a scroll redraws the row only when the fade comes or goes.
function useEdgeFade() {
    const sizes = useRef({ view: 0, content: 0, x: 0 })
    const [isShown, setIsShown] = useState(false)
    const update = (next: Partial<typeof sizes.current>) => {
        Object.assign(sizes.current, next)
        const { view, content, x } = sizes.current
        setIsShown(content - view - x > 1)
    }
    return {
        isShown,
        onLayout: (e: LayoutChangeEvent) => update({ view: e.nativeEvent.layout.width }),
        onContentSizeChange: (width: number) => update({ content: width }),
        onScroll: (e: NativeSyntheticEvent<NativeScrollEvent>) =>
            update({ x: e.nativeEvent.contentOffset.x }),
    }
}

const FADE_STRIPS = FADE_STEPS.map(opacity => (
    <View key={opacity} className="bg-background" style={[FADE_STRIP, { opacity }]} />
))

function EdgeFade({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View testID="report-controls-fade" className="flex-row" style={FADE_STYLE}>
            {FADE_STRIPS}
        </View>
    )
}
```

and change `ReportControls`' returned tree to:

```tsx
    return (
        <View style={ROW_STYLE}>
            <ScrollView
                horizontal
                testID="report-controls"
                showsHorizontalScrollIndicator={false}
                scrollEventThrottle={16}
                onLayout={fade.onLayout}
                onContentSizeChange={fade.onContentSizeChange}
                onScroll={fade.onScroll}
                contentContainerClassName="flex-row items-center gap-2 px-4 py-2"
            >
                {controls}
                <SortControl model={model} isVisible={isSortMenuShown} />
            </ScrollView>
            <EdgeFade isVisible={fade.isShown} />
        </View>
    )
```

with `const fade = useEdgeFade()` as the first line of `ReportControls` (before its early `return null`, so hooks run in the same order). Imports: `useRef`, `useState` from `react`; `View`, `type LayoutChangeEvent`, `type NativeScrollEvent`, `type NativeSyntheticEvent` from `react-native`. Update the comment above `ReportControls` to: "One line of chips under the header: it scrolls sideways on a narrow screen and never wraps; a fade marks its right edge while more chips lie past it (D11)."

- [ ] **Step 5: Help**

`core/help/reports.md`, "Run a report", first bullet: after "The chips above the report set its period and filters;" insert "the period chip names the period, such as **This month**, and the dates show under the chips." After "…adds a chip for a filter you can set while you read." add "On a narrow screen, swipe the chips sideways to see the rest; the right edge fades while there are more."

- [ ] **Step 6: Update the e2e this task changes and add one**

`tests/e2e/reports-view.spec.ts`:
- `the period chip and a filter chip edit the run, and a copied link keeps them` (`:138-140`): replace the expectation with

```ts
            await expect(p.getByTestId('report-param-day')).toHaveText('Day: This fiscal year')
            await expect(p.getByTestId('report-document-period')).toContainText(String(YEAR))
```

- `on a phone the chips open sheets and Customize is a sheet` (`:521-523`): replace the expectation with

```ts
            await expect(p.getByTestId('report-param-day')).toHaveText('Day: Last fiscal year')
            await expect(p.getByTestId('report-document-period')).toContainText(String(YEAR - 1))
```

- Add, after that test:

```ts
    // D11: a long chip row scrolls sideways under a fade at its right edge; the fade
    // goes once the row is scrolled to its end, and a desktop whose chips fit has none.
    test('on a phone the chips scroll sideways under a fade, and the period chip names its preset only', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await makeReport(page, 'rptchipfade', true, 'Sales')
        try {
            await expect(p.getByTestId('report-param-day')).toHaveText('Day: This fiscal year')
            await expect(p.getByTestId('report-controls-fade')).toHaveCount(0)
            await p.setViewportSize({ width: 390, height: 844 })
            await addCustomerFilter(p, 'Gamma and every other customer of the year')
            const fade = p.getByTestId('report-controls-fade')
            await expect(fade).toBeVisible()
            await p.getByTestId('report-controls').evaluate(el => {
                el.scrollLeft = el.scrollWidth
            })
            await expect(fade).toHaveCount(0)
        } finally {
            await close()
        }
    })
```

- [ ] **Step 7: Run tests**

Run: `pnpm exec tinycld-pkg typecheck`; `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`; `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-view.spec.ts`
Expected: PASS. (If `el.scrollLeft` on `report-controls` does not scroll, the testID sits on the ScrollView's outer element in react-native-web; scroll its first child with `overflow-x: auto` instead, found the way `listScroller` finds the list's.)

- [ ] **Step 8: Lint and commit**

```bash
git add core/reports/lib/params.ts core/reports/lib/__tests__/params.test.ts core/reports/components/ReportControls.tsx core/help/reports.md tests/e2e/reports-view.spec.ts
git commit -m "reports: a preset-only period chip, and a fade on the chip row's edge"
```

E2E this task breaks (fixed in Step 6): `reports-view.spec.ts:138-140`, `:521-523`. Books: none (`general-ledger.spec.ts:375` and `trial-balance.spec.ts:680` use `toContainText` with the preset label).

---

### Task 13: D9 — on a phone the top area scrolls away; the column headings stay, the stuck group under them

**Files:**
- Create: `core/reports/hooks/useScrollAwayTop.ts`
- Modify: `core/reports/components/ReportTable.tsx` (part-1 Tasks 4, 5, 7, 8; Tasks 10, 11: `top` prop; `TableList` splits into `FixedHeadingList` and `ScrollAwayList`)
- Modify: `core/reports/components/ReportView.tsx` (part-1 Tasks 4, 7; Tasks 10, 11: `ReportTop`; the top goes into the list on a phone)
- Modify: `core/help/reports.md` ("When columns do not fit"; the Sort bullet from part-1 Task 7)
- Test (e2e): `tests/e2e/reports-wrap.spec.ts` (`on a phone a wide row wraps under aligned wrapped headings, …`; new test)

**Interfaces:**
- Consumes: `TableLayout` / `useTableLayout` (part-1 Task 4); `ReportControls({ model, isSortMenuShown })` (part-1 Task 7); `ReportNotices({ notices, layoutNotice, isLive, isCompact })` (Task 11); `TableEnd({ hasMore })`, `hasMore` (Task 10); FlashList `stickyHeaderConfig: { useNativeDriver?: boolean; offset?: number }` and `onScroll`.
- Produces:
  - `useScrollAwayTop(): { onScroll; headingStyle; stickyConfig: { useNativeDriver: false; offset: number }; onTopLayout(e: LayoutChangeEvent): void; onHeadingLayout(e: LayoutChangeEvent): void }` (`core/reports/hooks/useScrollAwayTop.ts`).
  - `ReportTableProps.top?: ReactNode` — when given, the table draws it as the list's header (it scrolls with the rows) and floats the heading row over the list.
  - `ReportBody({ model, table, top }: { model: ReportViewModel; table: TableLayout; top?: ReactNode })`.

How it works (D9, review §11): FlashList (2.3.3) renders, inside its scroll content, the list header, then an empty view with `marginTop: stickyHeaderConfig.offset`, then the rows; a sticky copy is drawn at `top: offset` over the list (`node_modules/@shopify/flash-list/dist/recyclerview/RecyclerView.js`, `StickyHeaders.js`). So on a phone: the list header is the top area (header, chips, notices, document header) plus the row-width probe; the offset is the heading row's height, so the empty view is the heading row's slot under the top area; the heading row is an absolutely placed `Animated.View` over the list whose `translateY` is `topHeight − scrollY`, clamped to `[0, topHeight]`: it sits in its slot, rises with the top area, and stops at the top of the table once the top area has gone. The sticky group copy sticks at `top: offset`, right under it. Both move by one JS-driven scroll value (`Animated.event` with `useNativeDriver: false`, and the phone's sticky config sets `useNativeDriver: false` too), so they move together on web and native. The heading row comes before the list in the tree, so its sort buttons (one-line layouts) keep their place in the reading order before the rows. Every other state and width keeps today's fixed top area and heading.

- [ ] **Step 1: Write the failing e2e**

In `tests/e2e/reports-wrap.spec.ts`, add after `on a phone a wide row wraps under aligned wrapped headings, …`:

```ts
    // D9: on a phone the title, chips and notes scroll away with the rows and the
    // column headings stay at the top of the table, the open group's copy right under
    // them; back at the top, the headings sit under the document header again. A
    // desktop keeps its top area in place.
    test('on a phone the title and chips scroll away with the rows while the headings stay', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptaway')
        try {
            await seed(p, 24)
            await wideReport(p, uniqueName('Away'), true)
            const title = p.getByTestId('report-title')
            const table = p.getByTestId('report-table')
            const heading = p.getByTestId('report-heading-row')
            const yOf = async (locator: Locator) => (await locator.boundingBox())?.y ?? 0

            const titleY = await yOf(title)
            await table.hover()
            await p.mouse.wheel(0, 400)
            await expect.poll(() => yOf(title)).toBe(titleY)

            await p.setViewportSize({ width: 390, height: 844 })
            await expect(table).toHaveCSS('opacity', '1')
            await table.hover()
            await p.mouse.wheel(0, -5000)
            const documentHeader = p.getByTestId('report-document-header')
            await expect
                .poll(async () => Math.abs((await yOf(heading)) - (await bottomOf(documentHeader))))
                .toBeLessThanOrEqual(2)

            await p.mouse.wheel(0, 900)
            await expect.poll(async () => (await bottomOf(title)) <= (await yOf(table)) + 1).toBe(true)
            await expect
                .poll(async () => Math.abs((await yOf(heading)) - (await yOf(table))))
                .toBeLessThanOrEqual(2)
            const sticky = p.getByTestId('report-sticky-group')
            await expect(sticky).toContainText('Open')
            await expect
                .poll(async () => Math.abs((await yOf(sticky)) - (await bottomOf(heading))))
                .toBeLessThanOrEqual(2)

            await p.mouse.wheel(0, -5000)
            await expect
                .poll(async () => Math.abs((await yOf(heading)) - (await bottomOf(documentHeader))))
                .toBeLessThanOrEqual(2)
            await expect.poll(() => yOf(title)).toBeGreaterThanOrEqual(await yOf(table))
        } finally {
            await close()
        }
    })
```

Import `type Locator` from `@playwright/test` if the file does not yet (it uses `Locator` in `bottomOf`, so it does).

- [ ] **Step 2: Run it to verify it fails**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts -g "scroll away"`
Expected: FAIL at the phone scroll — the title stays where it was (`bottomOf(title) <= yOf(table)` is false).

- [ ] **Step 3: The hook**

```ts
// core/reports/hooks/useScrollAwayTop.ts
import { useCallback, useMemo, useRef, useState } from 'react'
import { Animated, type LayoutChangeEvent } from 'react-native'

/**
 * D9: on a phone the report's top area (header, chips, notices, document header) is
 * the list's header and scrolls away with the rows. The column heading row floats
 * over the list: it sits in its slot under the top area (FlashList keeps an empty
 * slot of stickyHeaderConfig.offset after the list header), rises with it, and stops
 * at the top once the top area has gone. Sticky group copies stick at the offset,
 * right under it. One JS-driven scroll value moves the heading, and the phone's
 * sticky copies use the JS driver too, so both move together on web and native.
 * Heights are container layouts, not text measures.
 */
export function useScrollAwayTop() {
    const scrollY = useRef(new Animated.Value(0)).current
    const [topHeight, setTopHeight] = useState(0)
    const [headingHeight, setHeadingHeight] = useState(0)
    const onScroll = useMemo(
        () =>
            Animated.event([{ nativeEvent: { contentOffset: { y: scrollY } } }], {
                useNativeDriver: false,
            }),
        [scrollY]
    )
    const headingStyle = useMemo(
        () => ({
            position: 'absolute' as const,
            top: 0,
            left: 0,
            right: 0,
            zIndex: 3,
            // Hidden until the top area's height is known, so it never shows over it.
            opacity: topHeight > 0 ? 1 : 0,
            transform: [
                {
                    translateY: scrollY.interpolate({
                        inputRange: [0, Math.max(1, topHeight)],
                        outputRange: [topHeight, 0],
                        extrapolate: 'clamp',
                    }),
                },
            ],
        }),
        [scrollY, topHeight]
    )
    const stickyConfig = useMemo(
        () => ({ useNativeDriver: false as const, offset: headingHeight }),
        [headingHeight]
    )
    // Stable, so the list header built from them is not new on each render.
    const onTopLayout = useCallback(
        (e: LayoutChangeEvent) => setTopHeight(e.nativeEvent.layout.height),
        []
    )
    const onHeadingLayout = useCallback(
        (e: LayoutChangeEvent) => setHeadingHeight(e.nativeEvent.layout.height),
        []
    )
    return { onScroll, headingStyle, stickyConfig, onTopLayout, onHeadingLayout }
}
```

- [ ] **Step 4: The table**

In `core/reports/components/ReportTable.tsx` (state after Task 11):

1. Extract from `TableList` the shared list data into a hook:

```tsx
// What both list variants give FlashList: the sticky rows, the rows' redraw signal
// and the top-row report.
function useTableListData({ rows, columns, layout, format, onToggleGroup, onTopRowIndex }: TableListProps) {
    const sticky = useMemo(() => stickyIndices(rows), [rows])
    const onViewableItemsChanged = useTopRowReport(onTopRowIndex)
    const extraData = useMemo<TableData>(
        () => ({ columns, layout, format, onToggleGroup, rows }),
        [columns, layout, format, onToggleGroup, rows]
    )
    return { sticky, extraData, onViewableItemsChanged }
}
```

2. Rename today's `TableList` to `FixedHeadingList(props: TableListProps)`, reading `sticky`, `extraData`, `onViewableItemsChanged` from `useTableListData(props)`; its tree is unchanged (heading row above the list, `stickyHeaderConfig={STICKY_CONFIG}`, `ListHeaderComponent={header}` with the probe, `ListFooterComponent={<TableEnd hasMore={props.hasMore} />}`).

3. Add:

```tsx
// D9: the phone's list. The top area is the list's header and scrolls with the rows;
// the heading row floats over the list (useScrollAwayTop). It comes first in the tree,
// so its sort buttons come before the rows in the reading order.
function ScrollAwayList(props: TableListProps & { top: ReactNode }) {
    const { rows, columns, layout, format, sorting, onRowWidth, onNearEnd, hasMore, top } = props
    const { sticky, extraData, onViewableItemsChanged } = useTableListData(props)
    const away = useScrollAwayTop()
    const header = useMemo(
        () => (
            <>
                <View onLayout={away.onTopLayout}>{top}</View>
                <RowWidthProbe onWidth={onRowWidth} />
            </>
        ),
        [top, away.onTopLayout, onRowWidth]
    )
    // Wrapped headings take no presses (part-1 Task 7), so a drag that starts on them
    // passes to the list. One-line headings keep their sort presses (see Open questions).
    const headingTouches = layout.kind === 'blocks' ? 'none' : 'box-none'
    return (
        <View className="min-h-0 flex-1">
            <Animated.View
                style={away.headingStyle}
                onLayout={away.onHeadingLayout}
                pointerEvents={headingTouches}
            >
                <HeadingRow columns={columns} layout={layout} format={format} sorting={sorting} />
            </Animated.View>
            <FlashList
                data={rows}
                renderItem={renderTableItem}
                keyExtractor={rowKey}
                getItemType={rowType}
                extraData={extraData}
                stickyHeaderIndices={sticky}
                stickyHeaderConfig={away.stickyConfig}
                onScroll={away.onScroll}
                onEndReached={onNearEnd}
                onEndReachedThreshold={0.5}
                onViewableItemsChanged={onViewableItemsChanged}
                viewabilityConfig={VIEWABILITY_CONFIG}
                className="web:[scrollbar-gutter:stable]"
                ListHeaderComponent={header}
                ListFooterComponent={<TableEnd hasMore={hasMore} />}
            />
        </View>
    )
}

function TableList({ top, ...props }: TableListProps & { top?: ReactNode }) {
    if (top === undefined) return <FixedHeadingList {...props} />
    return <ScrollAwayList {...props} top={top} />
}
```

(`top` is a new element each `ReportView` render, so the header memo mainly saves the list's own re-renders; React keeps its children mounted either way. Keep the heading row's `web:[scrollbar-gutter:stable]` padding as today, so its tracks stay equal to the rows'. Check the real name of `layout`'s blocks kind in part 1's `LineLayout`.)

4. `ReportTableProps` gains

```ts
    /**
     * D9: the view's top area on a phone. Given, it scrolls away with the rows as the
     * list's header and the heading row stays at the top; not given, both stay put.
     */
    top?: ReactNode
```

and `ReportTable` passes `top={props.top}` to `TableList`.

5. Imports: `Animated` from `react-native`; `type ReactNode` from `react`; `useScrollAwayTop` from `'../hooks/useScrollAwayTop'`.

Note: `TableList` stays keyed by `runKey` (a new run starts at the top, F11), so on a phone the top area mounts again when a run of new params or a new sort lands. See Risks.

- [ ] **Step 5: The view**

In `core/reports/components/ReportView.tsx` (state after Task 11):

```tsx
interface ReportTopProps {
    model: ReportViewModel
    isSortMenuShown: boolean
    layoutNotice: string | undefined
    isCompact: boolean
}

// Everything above the rows: the header, the chips, the notices and the document header.
function ReportTop({ model, isSortMenuShown, layoutNotice, isCompact }: ReportTopProps) {
    return (
        <>
            <ReportHeader
                title={model.title}
                onBack={model.onBack}
                actions={toolbarItems(model)}
                isUpdating={model.isUpdating}
                isUpdatingLive={!model.isUpdatingInBuilder}
            />
            <ReportControls model={model} isSortMenuShown={isSortMenuShown} />
            <ReportNotices
                notices={model.notices}
                layoutNotice={layoutNotice}
                isLive={model.isBodyShown}
                isCompact={isCompact}
            />
            <ReportDocumentHeader
                heading={model.heading}
                title={model.title}
                periodWords={model.periodWords}
                isThousands={model.display.divideBy1000}
            />
        </>
    )
}
```

In `ReportView`, above the return (after `table`, `isSortMenuShown`, `layoutNotice`, `isCompact`):

```tsx
    const top = (
        <ReportTop
            model={model}
            isSortMenuShown={isSortMenuShown}
            layoutNotice={layoutNotice}
            isCompact={isCompact}
        />
    )
    // D9: on a phone, with rows to scroll, the top area scrolls away with them; in
    // any other state, or on a wider screen, it stays above the body.
    const isTopInList = isCompact && model.isBodyShown && model.state === 'ready'
    const fixedTop = isTopInList ? null : top
    const listTop = isTopInList ? top : undefined
```

and the left column becomes `<View className="min-w-0 flex-1">{fixedTop}<ViewBody model={model} table={table} top={listTop} /></View>`. `ViewBody({ model, table, top })` passes `top` to `ReportBody`; `ReportBody({ model, table, top })` passes it to `TableBody`; `TableBody` passes `top={top}` to `ReportTable`. The builder's `SheetSurface` calls `ReportBody` without `top`, so its preview keeps the fixed heading.

- [ ] **Step 6: Help**

`core/help/reports.md`, "When columns do not fit": change "The heading row and the open group rows stay at the top while you scroll." to "The heading row and the open group rows stay at the top while you scroll. On a phone, the title, the chips and the notes scroll away with the rows, so the rows get the screen; scroll back to the top to change a chip." In "Run a report", in the Sort bullet that part-1 Task 7 added, after "use the **Sort** chip above the report instead" add "(on a phone, scroll back to the top to reach it)".

- [ ] **Step 7: Update the e2e this task changes**

In `tests/e2e/reports-wrap.spec.ts` › `on a phone a wide row wraps under aligned wrapped headings, with no sideways scroll, and an open group sticks`: the heading row now moves up as the top area scrolls away, so its bottom must be read after the scroll. Replace the block that starts with the comment "Scroll the open group's rows: its own row goes up under the heading row," (from `const headingBottom = await bottomOf(p.getByTestId('report-heading-row'))` to the last `.toBeLessThanOrEqual(2)` of the test) with:

```ts
            // Scroll the open group's rows: the top area scrolls away, the heading row
            // stays at the top of the table, the group's own row goes up under it, and
            // its stuck copy stays just under it (D9).
            const heading = p.getByTestId('report-heading-row')
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 900)
            const sticky = p.getByTestId('report-sticky-group')
            await expect(sticky).toContainText('Open')
            await expect
                .poll(async () => isScrolledAway(rowOf(p, 'Open'), await bottomOf(heading)))
                .toBe(true)
            await expect
                .poll(async () => Math.abs(((await sticky.boundingBox())?.y ?? 0) - (await bottomOf(heading))))
                .toBeLessThanOrEqual(2)
```

`the next section pushes the whole stuck stack of open groups, on a desktop and a phone`: on a phone the list's scroller now starts at the table's top and the heading row floats over it, so `scrollRowTo(next, height / 2)` in `expectWholeStackPushed` pushes the stack by the heading's height too. In `expectWholeStackPushed`, measure from the heading's bottom (0 on a desktop, the heading's height on a phone):

```ts
    const scroller = await listScroller(next)
    const scrollerTop = (await scroller.boundingBox())?.y ?? 0
    // D9: on a phone the heading row floats over the list's top.
    await scrollRowTo(next, headingBottom - scrollerTop + height / 2)
```

in place of `await scrollRowTo(next, height / 2)` (import `listScroller` from `./reports-helpers`). FlashList fades a pushed copy when the offset is above 0; bounding boxes ignore that.

Re-check (no edit expected; fix at the cause if one fails): `report rows on a touch phone` › `the table keeps the browser touch gestures…` (it reads the heading's bottom before its swipe and starts the swipe at `table.y + 500`; read the bottom after the swipe if needed, as in the wrap test above); `reports-scroll-anchor.spec.ts` › `on a phone, rows opening above right after a wheel scroll move nothing in view` (the scroll offset now includes the top area, and `toTop` measures from the list's top, which on a phone is under the floating heading; add the heading's height to `below` there as in `expectWholeStackPushed` if a row hides under it); `reports-view.spec.ts` › `on a phone the chips open sheets and Customize is a sheet` (chips at the top, before any scroll).

- [ ] **Step 8: Run tests**

Run: `pnpm exec tinycld-pkg typecheck`; `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`; `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts`, then `… -- reports-scroll-anchor.spec.ts`, `… -- reports-view.spec.ts`, `… -- reports-ledger-v3.spec.ts`, `… -- reports-fit.spec.ts`
Expected: PASS. A failure in a re-checked test is fixed at its cause in this task's code (for example a sticky offset read before the heading's height is known), not by loosening the assertion.

- [ ] **Step 9: Lint and commit**

```bash
git add core/reports/hooks/useScrollAwayTop.ts core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/help/reports.md tests/e2e/reports-wrap.spec.ts
git commit -m "reports: on a phone the top area scrolls away and the headings stay"
```

E2E this task breaks (fixed in Step 7): `reports-wrap.spec.ts` › `on a phone a wide row wraps under aligned wrapped headings…` (its heading-bottom reads before the scroll) and › `the next section pushes the whole stuck stack…` (phone half: `expectWholeStackPushed`'s push offset). Books (after merge): re-check `general-ledger.spec.ts` phone tests that read `report-heading-row`, a chip or the title after a scroll.

---

## Self-review notes

- Spec coverage (part 2): outline T9 → Task 9; D8/H1 → Task 10; D12 (on-screen end row; page footer later in exports, H6's `reportFooter` not built) → Task 10; H5 → Task 11; D11/H2 (preset chip, fade; review §11.5-2's one-line phone document header is not in D11's answer and D9 makes the header scroll away, so it is not built) → Task 12; D9 → Task 13; help/docs → Tasks 10–13. Outline T10's measure constraint → Task 10 Step 4.
- Type names used across tasks: `holdWholeReportRows`, `ShownResult`, `RunView.heldRows`, `ReportRun.heldRows?`, `TableEnd`, `hasMore`, `LAYOUT_NOTICE_CODE`, `layoutNoticeOf`, `bandNotices`, `isBandCut`, `BAND_LINES`, `ReportNotices({ notices, layoutNotice, isLive, isCompact })`, `chipText(def, value, labels?)`, `useEdgeFade`, `EdgeFade`, `useScrollAwayTop`, `ReportTop`, `top?: ReactNode` — each defined in the task named in its Interfaces block. Part-1 names consumed: `useTableLayout`, `TableLayout`, `TableLayoutModel`, `useRunMeasures`, `measureColumns`, `LineLayout` (blocks `notice`), `textWidth`, `ReportControls({ model, isSortMenuShown })`, `SortControl`, `ReportBody({ model, table })`.

## Open questions for the human

**Answered 2026-11-07 by the human:** (1) accept the JS driver (one-frame lag on native). (2) Do NOT accept the remount: focus must stay on the changed chip. Task 13 must keep the top area mounted across runs and test it. Preferred: keep the top area inside the list but stop re-keying the list per run (a new run returns to the top by an imperative scroll-to-top on the list ref, not a remount), so a drag on the title still scrolls. Only if that cannot work, use the overlay and record the drag limit as a Design note for the human.

1. **Native smoothness on a phone (D9).** The floating heading row and the stuck group copy follow one JS-driven scroll value on the phone path, so on iOS/Android they can trail the rows by a frame in a fast fling (today the sticky copy uses the native driver). The alternative (a native-driven value) needs a custom scroll component inside FlashList. Accept the JS driver?
2. **Phone top area remounts on a new run (D9).** The top area lives inside the list, which is keyed by the run (so a new run starts at the top, F11). When a chip, filter or sort change lands, the header and chips mount again: a screen-reader or keyboard user's focus leaves the chip. The alternative keeps the top area outside the list as an overlay that moves with the scroll, but a drag that starts on the title then does not scroll the rows. Accept the remount? (The floating heading row has the same drag limit for one-line layouts only, where its headings sort on a press; wrapped headings pass touches to the list.)

## Risks

- FlashList's `stickyHeaderConfig.offset` also adds a `marginTop` of that size after the list header (2.3.3); Task 13 depends on that slot. A FlashList upgrade that drops it shows the heading over the first row; the Task 13 e2e (heading under the document header at the top) catches it.
- The heading's height is known one layout after mount; until then the offset is 0 and the rows sit one heading-height higher, then move down once. The table is drawn hidden until its row width is measured (part-1 Task 4), which usually covers it.
- On a phone the top area sits inside `report-table`, which is drawn at `opacity-0` until the row width is measured (part-1 Task 4), so the title and chips are hidden for that first frame. If it shows as a flash, put the `opacity-0` on the list only.
- `isBandCut` uses the upper text estimate: on a Mac a band that would just fit in 2 lines may offer "Show all". It never hides text without the button.
- Holding the Total depends on the server appending the whole-report rows after each page's slice (`assemble.go` `pageWindow`); a layout report whose computed rows sit inside the slice keeps them in place, as `mergePages` already assumes.
