# Report layout fit — outline

Plan parts: `docs/plans/2026-11-07-report-layout-fit-part-1.md` (Tasks 1–8), `docs/plans/2026-11-07-report-layout-fit-part-2.md` (Tasks 9–13).
Inputs: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/layout-review.md` (review), `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/layout-decisions.md` (binding decisions D1–D12).
Core repo: `/Users/nas/tmp/tinycld/books-purchases/tinycld` (main 0cdbe24d). All paths below are relative to it.

## Files

| File | Create / change | What it is for |
|---|---|---|
| `core/reports/lib/text-width.ts` | create (T1) | Pure upper estimate of a text's drawn width (char-class em table, widest of SF / Arial-Liberation / Roboto). No imports, so e2e can import it. |
| `core/reports/lib/column-measure.ts` | create (T2), change (T3) | Pure per-column `natural` / `min` / `grow` from the heading, the shown rows and fixed worst cases; the label's chrome; `measureKey`. Takes over `RowLevels`, `FLAT_LEVELS`, `rowLevelsOf`, `isFigureColumn`, `deepestLabelStart` from `table-lines.ts` (re-exported there). |
| `core/reports/lib/table-lines.ts` | change (T2, T3, T5, T6) | `shareWidth` (CSS table-layout:auto distribution), `fitOneLine` by content, blocks fitted by content (side parts get px widths), layout choice by fit (no breakpoint, no `NARROWER`), auto lines by content, continuation indent, spans for every non-detail row, H7. |
| `core/reports/components/report-table-layout.ts` | change (T3, T8) | Drop `AMOUNT_WIDTH`, `LABEL_MIN_WIDTH`, `PHONE_*`, `TEXT_FLOOR`; add `CONTINUATION_LINE_HEIGHT`, `HEADING_LINE_HEIGHT`. |
| `core/reports/hooks/useStable.ts` | create (T4) | `useStable` moved out of `ReportTable.tsx` (shared by the table and `useTableLayout`). |
| `core/reports/hooks/useTableLayout.ts` | create (T4), change (T5) | Holds the measured row width and the run's kept measure; returns `{ layout, isMeasured, onRowWidth }`; `shownRunKey`. Called once per drawn table: `ReportView` and the builder's phone preview. |
| `core/reports/components/ReportRow.tsx` | change (T4, T5, T6, T8) | Label box at its fitted width; side parts at px widths; root continuation indent; spans; H7 label need; compact continuation heights; tabular-nums on figures. |
| `core/reports/components/ReportTable.tsx` | change (T4, T5, T7, T8) | Takes `layout` / `isMeasured` / `onRowWidth`; heading wraps to 2 lines; no `pl-1`; blocks heading compact, no sort buttons. |
| `core/reports/components/ReportView.tsx` | change (T4, T7) | Calls `useTableLayout`; passes the table to `ReportBody`; tells `ReportControls` to show the Sort menu in blocks mode. |
| `core/reports/components/builder/ReportBuilderPanel.tsx` | change (T4) | Phone preview calls `useTableLayout(preview)`. |
| `core/reports/components/builder/ArrangeSection.tsx` | change (T5) | Hint and caption text for layout choice by fit. |
| `core/reports/components/ReportControls.tsx` | change (T7, T12) | Sort menu chip (blocks only); later chip fade + short period chip. |
| `core/reports/lib/view-url.ts` | change (T7) | `sortChoices`, `sortChipText`. |
| `core/reports/stores/report-ui-store.ts` | change (T7) | `ReportSurface` adds `{ kind: 'sort' }`. |
| `core/reports/hooks/useReportView.ts` | change (T7) | `onSetSort?(sort)` on the model. |
| `core/reports/lib/rows.ts` | change (T10) | `mergePages` drops the last page's whole-report rows while it has `page.next`. |
| `core/reports/components/ReportNotices.tsx` | change (T11) | Layout notice joins the band; 2-line cap with "Show all" on a phone. |
| `core/reports/lib/params.ts` | change (T12) | Preset-only period chip text. |
| `core/help/reports.md`, `docs/packages.md` | change (T5–T8, T10–T13) | Behavior docs, per task. |
| tests: `core/reports/lib/__tests__/text-width.test.ts` (new), `column-measure.test.ts` (new), `table-lines.test.ts`, `core/reports/components/__tests__/report-table-layout.test.ts`, `rows.test.ts`, `params.test.ts`; e2e `tests/e2e/reports-text-width.spec.ts` (new), `reports-fit.spec.ts` (new), `reports-wrap.spec.ts`, `reports-ledger-v3.spec.ts` | | |

## Tasks

| # | Task | Produces for later tasks |
|---|---|---|
| 1 | Text width estimate + calibration e2e | `type TextStyle = { size: 'sm' \| 'xs'; weight: 'normal' \| 'semibold'; uppercase?: boolean }`, `FONT_PX`, `textWidth(text: string, style: TextStyle): number`, `longestWordWidth(text: string, style: TextStyle): number` |
| 2 | Column measure (pure) | `type ColumnMeasure = { natural: number; min: number; grow: number }`, `type ColumnMeasures = Record<string, ColumnMeasure>`, `CELL_PADDING = 24`, `SORT_ICON_ROOM = 16`, `TEXT_MIN_WIDTH`, `MEASURE_ROW_LIMIT = 500`, `cellTextStyle(kind: RowKind): TextStyle`, `labelChrome(levels: RowLevels): number`, `labelNeed(item: VisibleRow, format: NumberFormat): number`, `measureColumns(input: { columns; rows: readonly VisibleRow[]; format: NumberFormat; levels: RowLevels }): ColumnMeasures`; moved `RowLevels`, `FLAT_LEVELS`, `rowLevelsOf`, `isFigureColumn`, `deepestLabelStart` |
| 3 | One line by content (pure) | `shareWidth(parts: readonly ColumnMeasure[], width: number): number[] \| null`, `measureOf(m: ColumnMeasures, id: string): ColumnMeasure`, `fitOneLine(columns, width, measures): Record<string, number> \| null`, `tableLines({ …, measures? })`, `measureKey({ runKey, format, columns, levels, hasDetail }): string` |
| 4 | Table wired to the run's measure | `useStable<T>(value: T): T`, `useTableLayout(model: TableLayoutModel): TableLayout` (`{ layout: LineLayout; isMeasured: boolean; onRowWidth(width: number): void }`), `shownRunKey(model)`, `ReportBody({ model, table })`, `ReportTable` props `layout` / `isMeasured` / `onRowWidth` |
| 5 | Layout choice by fit; authored and auto lines fitted by content; continuation indent | `type SidePart = { block: Block; tracks: number; width?: number }`, `LineLayout` blocks `{ kind: 'blocks'; root: Block; indent: number; notice?: string }`, `continuationIndent(levels: RowLevels): number`, `tableLines({ columns, width, layouts, levels?, measures? })` (no `breakpoint`), `RootBlockView({ root, indent, renderCell })` |
| 6 | Every non-detail row spans its empty cells; H7 | `oneLineLabelSpan` for every non-detail kind; `rowLayout(item, layout, labelId, need: number): LineLayout` |
| 7 | H3: Sort menu in blocks mode (blocks headings not pressable) | model `onSetSort?(sort: ViewSort \| null): void` (`ReportViewModel = Omit<…, 'onSetSort'> & { onSetSort?… }`), `sortChoices`, `sortChipText` (view-url.ts), `SortControl` inside `ReportControls.tsx`, `ReportControls({ model, isSortMenuShown })`, `ReportSurface` `{ kind: 'sort' }`; testIDs `report-sort-menu`, `report-sort-choice-<key>` |
| 8 | Line polish: compact continuation and heading lines, 2-line headings, tabular figures, label heading offset | `CONTINUATION_LINE_HEIGHT = 28`, `HEADING_LINE_HEIGHT = 24`, `type RenderCell = (col: string, justify: Justify, minHeight: number) => ReactNode`, `cellBoxStyle(width: number \| undefined, minHeight: number)`, `RootBlockView({ root, indent, renderCell, firstHeight, restHeight })` |
| 9 | Four-width acceptance e2e on salesstub (390/768/1024/1280): no sideways scroll; no figure/date cell or heading cut (a heading may use 2 lines); group, subtotal and Total labels whole; every figure cell's x within its heading's box. Draft: the appendix below | `tests/e2e/reports-fit.spec.ts` |
| 10 | H1 + end row: Total only with the last page; "Loading more rows…" row; "End of report" row | `mergePages` rule; `report-loading-more` (text), `report-end`. Constraint: the measure (Task 4 `useRunMeasures`) must still see the whole-report rows of page 1 (feed `run.result` tail rows into `measureColumns`), or a Total's width is lost. |
| 11 | H5: layout notice in the notice band; band capped at 2 lines with "Show all" on a phone | `ReportNotices({ notices, layoutNotice?, isLive })` |
| 12 | D11 / H2: preset-only period chip; chips keep the side scroll with a fade edge | `chipText` rule; `report-controls-fade` |
| 13 | D9: on a phone the top area (header, chips, notices, document header) scrolls away with the rows; only the column headings stay; sticky group copy sits under the headings (FlashList `stickyHeaderConfig.offset`) | phone layout of `ReportView`; part-2 planner may split web/native parity into its own task |

Decisions folded in: D1 measure from the first rows, fixed per run (`measureKey`); D2 no in-cell wrap; D3 empty column = heading width; D4 no spec change; D5 authored layouts kept, lines fitted by content (span = grow weight); D6 continuation lines 28 px unless tappable; D7 pure functions reusable by exports; D8 → T10; D9 → T13; D10 → T7; D11 → T12; D12 → T10. Go/TS duplicate placement code (`display.go` / `placeCells`): kept — the builder validates on the client with `placeCells` and the server validates on save; the shared fixture `core/reports/lib/fixtures/display-cells.json` keeps them equal. Removing either copy needs a server round-trip per builder edit, so it is not "safe" in this change.

## Appendix: draft of Task 9 (written with part 1, moved when Task 3 was split)

### Draft for Task 9: Acceptance at 390, 768, 1024 and 1280

**Files:**
- Create: `tests/e2e/reports-fit.spec.ts`

**Interfaces:**
- Consumes: Tasks 1–7 behaviour; e2e helpers `createInvitedUser`, `skipWithoutReportsStub` (`tests/e2e/helpers.ts`), `addColumn`, `addSale`, `addTeam`, `chooseOption`, `openReportStub`, `openReportsHub`, `rowOf`, `saveNewReport`, `SALES_SOURCE`, `startReport`, `uniqueName` (`tests/e2e/reports-helpers.ts`).
- Produces: `tests/e2e/reports-fit.spec.ts` (`test.describe('report fit at four widths', …)`).

A ledger-like salesstub report (a label, three text columns, a date, three figures, grouped with subtotals and a Total) must, at each of the four widths: not scroll sideways; never cut a figure or date cell or its heading (a heading may use two lines, so its text box is checked for height too); show every group, subtotal and Total label whole; and put every figure cell under its heading's box (the draft's `fitProblems` does not check this yet; add it).

- [ ] **Step 1: Write the spec**

```ts
// tests/e2e/reports-fit.spec.ts
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
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
 * view with core defaults only — nothing scrolls sideways, no figure, date or
 * heading word is cut, and every group, subtotal and Total label is whole.
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
            el instanceof HTMLElement && (el.scrollWidth > el.clientWidth + 0.5 || el.scrollHeight > el.clientHeight + 0.5)
        const textOf = (el: Element) => el.querySelector('div[dir="auto"]')
        const problems: string[] = []
        if (table.scrollWidth > table.clientWidth) problems.push('table scrolls sideways')
        for (const col of figures) {
            if (cut(textOf(table.querySelector(`[data-testid="report-heading-${col}"]`) ?? table)))
                problems.push(`heading ${col} cut`)
            for (const cell of table.querySelectorAll(`[data-testid^="report-cell-${col}-"]`)) {
                const text = textOf(cell)
                if (text?.textContent && cut(text)) problems.push(`${cell.getAttribute('data-testid')} cut`)
            }
        }
        for (const row of table.querySelectorAll('[data-testid^="report-row-"]')) {
            const id = row.getAttribute('data-testid') ?? ''
            if (/report-row-d:/.test(id)) continue
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
            for (let i = 0; i < 6; i++) {
                await addSale(p, {
                    team,
                    customer: `Fitco ${String(i).padStart(2, '0')} Wholesale Partners of the Northwest`,
                    amount: 123456 + i,
                    cost: 40000,
                    quantity: 12,
                    day: '2026-01-05',
                    status: i % 2 === 0 ? 'open' : 'paid',
                })
            }
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            for (const keys of [['status'], ['currency'], ['team', 'name'], ['day'], ['amount'], ['cost'], ['quantity']]) {
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
                expect(await p.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(size.width)
            }
        } finally {
            await close()
        }
    })
})
```

(The aggregate menus' test ids follow the column order in the builder: label 0, status 1, currency 2, team name 3, day 4, amount 5, cost 6, quantity 7. Verify with the builder before running.)

- [ ] **Step 2: Run it**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-fit.spec.ts`
Expected: PASS. A failure names the width and the cut element; fix the cause in the layout code (the owning task's file), never the assertion.

- [ ] **Step 3: Lint and commit**

```bash
git add tests/e2e/reports-fit.spec.ts
git commit -m "reports: e2e for the layout fit at four widths"
```

E2E this task breaks: none.

---

