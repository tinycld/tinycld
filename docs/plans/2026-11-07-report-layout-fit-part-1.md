# Report Layout Fit — Part 1 (Tasks 1–8) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Report columns, rows and headings fit their content at 390, 768, 1024 and 1280 on web and native with core defaults only (no spec change), so that Books' General Ledger and Trial Balance need no further core spacing changes, while existing reports and authored layouts keep working.

**Architecture:** A pure, deterministic measure (`text-width.ts` → `column-measure.ts`) gives every column a `natural` and a `min` width from its heading, the rows of the run as it first lands, and fixed worst cases; it is kept for the run (D1). `table-lines.ts` shares the measured view width by content (CSS `table-layout: auto`: mins first, spare room by what each column lacks, then by grow), picks the layout by a fit test on the measured width (one line → authored arrangements widest first, each line fitted by content → automatic lines packed by content), and lets every non-detail row's label take the empty cells after it. React Native flex draws the result with px widths; no CSS grid, no canvas, no `onLayout` text measuring, so web and native draw the same thing.

**Tech Stack:** TypeScript, React Native + react-native-web (Expo Router), Uniwind classes, FlashList v2 (fork), lucide-react-native, vitest, Playwright, Biome.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/layout-review.md` (§1–11, ranked list, H1–H7) with the binding human decisions `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-general-ledger/layout-decisions.md` (D1–D12 below). Outline of all parts: `docs/plans/2026-11-07-report-layout-fit-outline.md`. Part 2 (`docs/plans/2026-11-07-report-layout-fit-part-2.md`, Tasks 9–13) covers the four-width acceptance e2e, H1, H5, H2/D11, D9 and D12. Style and conventions: `docs/plans/2026-11-06-report-engine-v3.md`.

All paths are relative to the core repo `/Users/nas/tmp/tinycld/books-purchases/tinycld` unless absolute. Line numbers are at main `0cdbe24d`.

---

## For review

### Verdict: client-only core change, no spec or engine change

- No Go change, no migration, no spec field, `CURRENT_SPEC_VERSION` stays 3 (D4).
- Removed exports: `AMOUNT_WIDTH`, `LABEL_MIN_WIDTH`, `PHONE_LABEL_WIDTH`, `PHONE_AMOUNT_WIDTH`, `TEXT_FLOOR` (`core/reports/components/report-table-layout.ts:5-8,20`), `labelFloor` (`core/reports/lib/table-lines.ts:364`). Changed signatures: `fitOneLine`, `tableLines` (no `breakpoint` after Task 5), `rowLayout` (adds `need`), `ReportBody` (adds `table`), `ReportTable` props, `RenderCell` (adds `minHeight`), `cellBoxStyle`. Grep of `/Users/nas/tmp/tinycld/books-purchases/books` finds none of them used there (`books/tinycld/books/screens/helix-mockup.tsx` builds a `ReportViewModel` by hand; the model keeps every field it has, and the one new field, `onSetSort`, is optional).
- Books e2e that this changes (fixed in Books after the core merge, not here): `books/tests/e2e/general-ledger.spec.ts:821-871` (phone: the opening and section-total rows become one line by H7, Task 6: `lines: 2` → `lines: 1`), `:873-912` (at 900 the tablet arrangement is used only when one line does not fit; re-check after Task 5), and any Books spec that clicks `report-sort-*` at a phone width (Task 7; grep finds none today).

### The decisions this part implements

| Decision | Where |
|---|---|
| D1 measure from the first rows (heading, cells incl. group/subtotal/total rows, fixed worst cases for dates and figures), fixed for the run | Tasks 2–4 (`measureKey`, `useTableLayout`) |
| D2 no in-cell wrap | text stays `numberOfLines={1}`; headings may use 2 lines (H4, Task 8) |
| D3 an all-empty column is as wide as its heading | Task 2 |
| D4 core defaults only | all |
| D5 authored layouts kept; each line fitted by content | Task 5 |
| D6 continuation lines shorter than 44 px when not tappable | Task 8 |
| D7 measure/fit code reusable by exports | Tasks 1–5 are pure functions of (columns, rows, format, width) |
| D10 sort from a menu in the blocks layout only | Task 7 |
| H4 headings wrap to 2 lines, longest word in the measure | Tasks 2, 7 |
| H7 a non-detail row's label in the empty leading tracks of its figure line | Task 6 |
| Review §7.1 items 1–7 (measure, spans, distribution, fit choice, auto lines by content, indent + compact lines, pivot levels / label chrome / tabular-nums) | Tasks 2–7 |

The Go/TS duplicate placement code (`core/server/reports/display.go:76-195`, `core/reports/lib/table-lines.ts:88-152`) stays: the builder checks an arrangement on every edit with `placeCells` (`core/reports/lib/builder/draft.ts:544`) and the server checks it on save; `core/reports/lib/fixtures/display-cells.json` keeps them equal (test "the shared display cell table", `table-lines.test.ts:90`). Removing either needs a server round-trip per edit, so it is not safe here.

---

## Global Constraints

- No spec change; no Go change; no new dependency.
- Web and native: flex layout only; widths come from the pure measure; never `onLayout` text measuring, canvas, or CSS grid. The one `onLayout` stays the row-width probe (`ReportTable.tsx:297-299`).
- Never cut by core: number, money, percent, date, datetime and bool cells (`isFigureColumn`), typed (date/number/money) labels, and the longest word of a heading. Other text may end in "…" (`numberOfLines={1}`), never wraps inside a cell (D2).
- The measure is kept for the run: it changes only when the run key (design + params + sort), the number format, the shown columns, or the row levels change — never on a later page, a fold or a live re-run (D1).
- A line never exceeds the measured row width; the table never scrolls sideways.
- Tappable lines (the first line of a row, every line of a group row, one-line sortable headings) keep `minHeight` 44 (`ROW_HEIGHT`).
- No raw hex; semantic Tailwind tokens; `useThemeColor` for icons. No `any`, no `biome-ignore`, no `console.*`.
- JSX minimal: no `.map()` or ternaries inside a returned tree; compute above the return.
- Tests: vitest for the pure lib (`core/reports/lib/__tests__`, `core/reports/components/__tests__/report-table-layout.test.ts`); Playwright for UI behaviour on the `salesstub` reports, phone viewport `{ width: 390, height: 844 }` where the phone path differs. No vitest for components.
- Commands, from `/Users/nas/tmp/tinycld/books-purchases/tinycld`:
  - unit: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run <file>`
  - types: `pnpm exec tinycld-pkg typecheck`
  - lint: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check <files>`
  - e2e (one file): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts` (check the ports first, lane rules)
- Commits: the driver commits; each task's last step lists the files to stage.
- Docs: update `core/help/reports.md` and `docs/packages.md` in the task that changes the behaviour they describe.

## Review Focus

1. **A later page holds a longer value than the first page** (a 7-digit amount, a long memo). Expect: the columns do not move; a text value ends in "…"; an amount column already holds −99,999.99 in the reader's format (the figure worst case) and the report's Total, which is in the first page. Test: Task 2 "keeps the figure worst case when the first rows are short".
2. **The reader changes Customize (hides a column, ÷1000, no cents) while reading**. Expect: the measure is taken again for the new columns/format, not kept from the old one. Test: Task 3 `measureKey` cases.
3. **A view narrower than one column's min** (builder drawer ~300 px, a long date label deep in groups). Expect: no crash, no sideways scroll; that column is clamped to the line and cut. Test: Task 5 "clamps a column wider than its line".
4. **Non-Latin text** (CJK names, accented Latin, emoji). Expect: estimated at least as wide as drawn. Test: Task 1 cases for `日本`, `É`, `@`.
5. **A report with no rows on its first page, or only a Total** (empty period). Expect: columns sized by their headings; the layout still chosen; no NaN widths. Test: Task 2 "sizes every column by its heading when there are no rows" and Task 3 `shareWidth` with zero naturals.

---

### Task 1: Text width estimate

**Files:**
- Create: `core/reports/lib/text-width.ts`
- Test: `core/reports/lib/__tests__/text-width.test.ts`
- Test (e2e): `tests/e2e/reports-text-width.spec.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `type TextStyle = { size: 'sm' | 'xs'; weight: 'normal' | 'semibold'; uppercase?: boolean }`, `FONT_PX: { sm: 14; xs: 12 }`, `textWidth(text: string, style: TextStyle): number` (px, rounded up), `longestWordWidth(text: string, style: TextStyle): number`. The module imports nothing, so a Playwright spec can import it.

The app has no custom font (`global.css` sets none): web draws in react-native-web's system stack (`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif`), iOS in SF, Android in Roboto, Linux CI Chromium in Liberation Sans (Arial metrics; Playwright's deps install `fonts-liberation`). Measured on this machine with Chromium canvas (scratch script, 2026-11-07), per em: SF regular digits ≤ 0.617, semibold ≤ 0.641; Arial regular/bold digits 0.556; narrow letters ≤ 0.389; upper (not M/W) ≤ 0.778; lower (not m/w) ≤ 0.611 bold. The table below is the widest of those, rounded up, so the estimate is an upper bound on every platform.

- [ ] **Step 1: Write the failing test**

```ts
// core/reports/lib/__tests__/text-width.test.ts
import { describe, expect, it } from 'vitest'
import { FONT_PX, longestWordWidth, textWidth } from '../text-width'

const CELL = { size: 'sm', weight: 'normal' } as const
const BOLD = { size: 'sm', weight: 'semibold' } as const
const HEADING = { size: 'xs', weight: 'semibold', uppercase: true } as const

describe('textWidth', () => {
    it('is zero for no text', () => {
        expect(textWidth('', CELL)).toBe(0)
    })

    it('sizes digits as tabular figures, wider when semibold', () => {
        expect(FONT_PX).toEqual({ sm: 14, xs: 12 })
        // 0.62 em × 14 = 8.68
        expect(textWidth('0', CELL)).toBe(9)
        // 6 digits × 0.62 + 2 narrow × 0.34 = 4.40 em × 14 = 61.6
        expect(textWidth('1,234.56', CELL)).toBe(62)
        // 6 × 0.65 + 2 × 0.40 = 4.70 em × 14 = 65.8
        expect(textWidth('1,234.56', BOLD)).toBe(66)
    })

    it('tells narrow, wide, upper and lower letters apart', () => {
        // M wide 0.95 + i narrow 0.34 = 1.29 em × 14 = 18.06
        expect(textWidth('Mi', CELL)).toBe(19)
        // A upper 0.78 + b lower 0.61 = 1.39 em × 14 = 19.46
        expect(textWidth('Ab', CELL)).toBe(20)
    })

    it('draws a heading in capitals at the small size', () => {
        // "ABC": 3 × 0.80 = 2.4 em × 12 = 28.8
        expect(textWidth('abc', HEADING)).toBe(29)
    })

    it('counts accented Latin as letters and other scripts and symbols at a full em', () => {
        expect(textWidth('É', CELL)).toBe(textWidth('E', CELL))
        expect(textWidth('é', CELL)).toBe(textWidth('e', CELL))
        expect(textWidth('日本', CELL)).toBe(28)
        expect(textWidth('@', CELL)).toBe(14)
    })
})

describe('longestWordWidth', () => {
    it('is the width of the widest word', () => {
        // "reversal": 3 narrow × 0.34 + 5 lower × 0.61 = 4.07 em × 14 = 56.98
        expect(longestWordWidth('Void reversal', CELL)).toBe(57)
        expect(longestWordWidth('', CELL)).toBe(0)
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/text-width.test.ts`
Expected: FAIL — `Failed to resolve import "../text-width"`.

- [ ] **Step 3: Write the implementation**

```ts
// core/reports/lib/text-width.ts

/**
 * An upper estimate of a text's drawn width in px, with no layout pass. The app
 * draws text in the platform's system font (react-native-web's system stack on
 * web, SF on iOS, Roboto on Android, Liberation Sans on Linux). Each character
 * class below is the widest of those fonts, rounded up, so a column sized by it
 * never cuts its text. Pure and import-free: a column measure, an export at a
 * page width and an e2e calibration all use the same numbers.
 */
export type TextStyle = { size: 'sm' | 'xs'; weight: 'normal' | 'semibold'; uppercase?: boolean }

/** text-sm and text-xs in px. */
export const FONT_PX = { sm: 14, xs: 12 } as const

type CharClass = 'digit' | 'narrow' | 'wide' | 'upper' | 'lower' | 'other' | 'far'

const EM: Record<TextStyle['weight'], Record<CharClass, number>> = {
    normal: { digit: 0.62, narrow: 0.34, wide: 0.95, upper: 0.78, lower: 0.61, other: 0.62, far: 1 },
    semibold: {
        digit: 0.65,
        narrow: 0.4,
        wide: 0.97,
        upper: 0.8,
        lower: 0.64,
        other: 0.66,
        far: 1,
    },
}

const NARROW = new Set([...'iljfrtI.,:;\'!|` '])
const WIDE = new Set([...'mwMW%&'])
const FAR = new Set(['@'])
// Past Latin Extended-B: other scripts, CJK, emoji and wide symbols (—, …).
const LAST_LATIN = 0x24f

function classOf(ch: string): CharClass {
    if (NARROW.has(ch)) return 'narrow'
    if (WIDE.has(ch)) return 'wide'
    if (FAR.has(ch) || (ch.codePointAt(0) ?? 0) > LAST_LATIN) return 'far'
    if (ch >= '0' && ch <= '9') return 'digit'
    if (ch !== ch.toLowerCase()) return 'upper'
    if (ch !== ch.toUpperCase()) return 'lower'
    return 'other'
}

export function textWidth(text: string, style: TextStyle): number {
    const shown = style.uppercase ? text.toUpperCase() : text
    const em = EM[style.weight]
    let sum = 0
    for (const ch of shown) sum += em[classOf(ch)]
    return Math.ceil(sum * FONT_PX[style.size])
}

/** The widest word: the least width a text can wrap into without cutting a word. */
export function longestWordWidth(text: string, style: TextStyle): number {
    return Math.max(0, ...text.split(/\s+/).map(word => textWidth(word, style)))
}
```

- [ ] **Step 4: Run the unit test**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/text-width.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 5: Write the calibration e2e**

It checks the table against the font the browser really draws with (Liberation Sans on CI, SF on a Mac): every sample's estimate is at least its drawn width, and all samples together are at most 35 % over (so the table is not so wide that one line rarely fits).

```ts
// tests/e2e/reports-text-width.spec.ts
import { expect, test } from '@playwright/test'
import { type TextStyle, textWidth } from '../../core/reports/lib/text-width'
import { login } from './helpers'

/**
 * The report column measure estimates text widths from a character table
 * (core/reports/lib/text-width.ts) instead of measuring the screen. This holds
 * the table to the font the app really draws with on this machine: every
 * estimate is at least the drawn width (so a figure sized by it is never cut),
 * and the samples together are not more than 35% over.
 */
const SAMPLES = [
    '1,234,567.89',
    '$35,100.00',
    '-00,000.00',
    'Sep 30, 2026',
    'Void reversal of JE-10',
    'Accounts receivable',
    'Harbor Lane Design LLC',
    'WWWW mmmm',
    'Total 1000 Business Checking',
    'Beginning balance',
    'Éclair à la crème',
]
const STYLES: { style: TextStyle; font: string }[] = [
    { style: { size: 'sm', weight: 'normal' }, font: '400 14px' },
    { style: { size: 'sm', weight: 'semibold' }, font: '600 14px' },
    { style: { size: 'xs', weight: 'semibold', uppercase: true }, font: '600 12px' },
]

test.describe('report text width estimate', () => {
    test('is at least the drawn width of every sample in the app font, and not far over in total', async ({
        page,
    }) => {
        await login(page)
        // Any text the app draws: react-native-web gives every Text the same family.
        const family = await page
            .locator('div[dir="auto"]')
            .first()
            .evaluate(el => getComputedStyle(el).fontFamily)
        for (const { style, font } of STYLES) {
            const texts = SAMPLES.map(text => (style.uppercase ? text.toUpperCase() : text))
            const drawn = await page.evaluate(
                ({ texts, font, family }) => {
                    const context = document.createElement('canvas').getContext('2d')
                    if (!context) return []
                    context.font = `${font} ${family}`
                    return texts.map(text => context.measureText(text).width)
                },
                { texts, font, family }
            )
            const estimates = SAMPLES.map(text => textWidth(text, style))
            const under = SAMPLES.filter((_, i) => (estimates[i] ?? 0) < (drawn[i] ?? 0))
            expect(under, `estimated narrower than drawn at ${font}`).toEqual([])
            const total = (list: number[]) => list.reduce((sum, n) => sum + n, 0)
            expect(total(estimates)).toBeLessThanOrEqual(total(drawn) * 1.35)
        }
    })
})
```

- [ ] **Step 6: Run the e2e**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-text-width.spec.ts`
Expected: PASS. If a sample is under, raise that character class in `EM` (never lower another) and re-run Steps 4 and 6. If the total is over 35 %, report the numbers to the driver instead of lowering a class.

- [ ] **Step 7: Lint and commit**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check core/reports/lib/text-width.ts core/reports/lib/__tests__/text-width.test.ts tests/e2e/reports-text-width.spec.ts`

```bash
git add core/reports/lib/text-width.ts core/reports/lib/__tests__/text-width.test.ts tests/e2e/reports-text-width.spec.ts
git commit -m "reports: estimate text widths from a character table"
```

E2E this task breaks: none.

---

### Task 2: Column measure (pure)

**Files:**
- Create: `core/reports/lib/column-measure.ts`
- Modify: `core/reports/lib/table-lines.ts:298-358` (move `FIGURE_TYPES`, `isFigureColumn`, `RowLevels`, `FLAT_LEVELS`, `rowLevelsOf`, `groupLabelStart`, `deepestLabelStart` into `column-measure.ts`; re-export them)
- Test: `core/reports/lib/__tests__/column-measure.test.ts`

**Interfaces:**
- Consumes: `textWidth`, `longestWordWidth`, `TextStyle` (Task 1); `cellDisplay`, `headingText`, `rowLabelText`, `NumberFormat` (`core/reports/lib/format.ts:172,230,222,15`); `shortDay` (`core/reports/lib/dates.ts:50`); `INDENT`, `CARET_ROOM` (`core/reports/components/report-table-layout.ts:9,13`); `VisibleRow` (`core/reports/lib/rows.ts:7`).
- Produces (all exported from `core/reports/lib/column-measure.ts`):
  - `type ColumnMeasure = { natural: number; min: number; grow: number }`; `type ColumnMeasures = Record<string, ColumnMeasure>`
  - `CELL_PADDING = 24` (a cell's `px-3`), `SORT_ICON_ROOM = 16` (icon 12 + `gap-1`), `TEXT_MIN_CHARS = 8`, `TEXT_MIN_WIDTH` (= `textWidth('nnnnnnnn', CELL)` = 69), `MEASURE_ROW_LIMIT = 500`
  - `cellTextStyle(kind: RowKind): TextStyle` — semibold for group, subtotal, computed and total rows (as `ROW_WEIGHT`, `ReportRow.tsx:30-37`)
  - `labelChrome(levels: RowLevels): number` — the label column's room besides its text: `INDENT + deepestLabelStart(levels) + CELL_PADDING / 2`
  - `labelNeed(item: VisibleRow, format: NumberFormat): number` — the width a row's whole label needs: its indent, its caret when it is a toggle, its text, `pr-3`
  - `measureColumns(input: { columns: readonly ResultColumn[]; rows: readonly VisibleRow[]; format: NumberFormat; levels: RowLevels }): ColumnMeasures`
  - moved: `RowLevels`, `FLAT_LEVELS`, `rowLevelsOf`, `isFigureColumn` (now also true when `totalType` is a figure type), `deepestLabelStart` (now exported). `table-lines.ts` re-exports `RowLevels`, `FLAT_LEVELS`, `rowLevelsOf`, `isFigureColumn`, so `core/reports/hooks/useReportView.ts:43` and the tests keep their imports.

Rules (review §7.1-1, D1, D3, H4):
- Heading: `headingText(column, format)` at `HEADING` style (xs, semibold, uppercase). Natural = text + `SORT_ICON_ROOM` when `column.sortable` + `CELL_PADDING`; heading min = longest word + icon + padding (a heading may wrap to 2 lines, Task 8).
- Cells of a non-label column: `cellDisplay(column, item.row, item.cells[column.id], format).text` of the first `MEASURE_ROW_LIMIT` rows, at `cellTextStyle(row.kind)`. A column with at least one value also counts its type's worst case at the detail style: date/datetime `shortDay('2026-09-30')` ("Sep 30, 2026"), bool `Yes`, percent `-100.0%`; money −99,999.99 and number −99,999 formatted by `cellDisplay` with the reader's format (so a currency sign, parentheses, ÷1000 and no-cents are counted).
- Label column: the needs (`labelNeed`) of the detail rows, and of a non-detail row whose first other column has a value (it spans nothing, Task 6); a detail label typed date also counts the date worst case at that row's indent.
- natural = max(heading natural, widest cell). min: a figure column or a typed (date/number/money) detail label → max(widest cell, heading min); a text column → max(heading min, min(natural, `TEXT_MIN_WIDTH + CELL_PADDING`)); the label → max(heading min, min(natural, `labelChrome(levels) + TEXT_MIN_WIDTH`)). An all-empty column: natural = heading natural, min = heading min (D3).
- grow: label 1, text 1, figure 0.

- [ ] **Step 1: Write the failing test**

```ts
// core/reports/lib/__tests__/column-measure.test.ts
import { describe, expect, it } from 'vitest'
import { CARET_ROOM, INDENT } from '../../components/report-table-layout'
import {
    CELL_PADDING,
    cellTextStyle,
    deepestLabelStart,
    labelChrome,
    labelNeed,
    measureColumns,
    measureKey,
    SORT_ICON_ROOM,
    TEXT_MIN_WIDTH,
} from '../column-measure'
import { cellDisplay, DEFAULT_DISPLAY } from '../format'
import type { VisibleRow } from '../rows'
import { longestWordWidth, textWidth } from '../text-width'
import type { CellValue, ResultColumn, ResultRow } from '../types'

const CELL = { size: 'sm', weight: 'normal' } as const
const BOLD = { size: 'sm', weight: 'semibold' } as const
const HEADING = { size: 'xs', weight: 'semibold', uppercase: true } as const
const FLAT = { groups: 0, nests: 0 }
const GROUPED = { groups: 1, nests: 0 }

const COLUMNS: ResultColumn[] = [
    // Short headings for the label and memo, so their rows, not their headings, size them.
    { id: 'account', label: 'Date', type: 'text' },
    { id: 'number', label: 'No.', type: 'text' },
    { id: 'memo', label: 'Memo', type: 'text' },
    // A heading wider than the text floor: its longest word is its min.
    { id: 'name', label: 'Customer name', type: 'text' },
    { id: 'debit', label: 'Debit', type: 'money', currency: 'USD', sortable: true },
]

function item(
    kind: ResultRow['kind'],
    label: string,
    cells: Record<string, CellValue>,
    extra: Partial<ResultRow> = {},
    isToggle = false
): VisibleRow {
    const row: ResultRow = { id: `${kind}:${label}`, kind, depth: 1, label, cells, ...extra }
    return { row, cells, isToggle, isOpen: isToggle }
}

const DETAIL = item(
    'detail',
    '2026-10-02',
    { number: 'JE-41', memo: 'Reversal of JE-12 for October rent', debit: 123456 },
    { labelType: 'date', labelValue: '2026-10-02' }
)
const GROUP = item('group', '1000 Business Checking', {}, { depth: 0 }, true)
const TOTAL = item('total', 'Total', { debit: 3510000 }, { depth: 0 })

const measure = (rows: VisibleRow[], levels = GROUPED) =>
    measureColumns({ columns: COLUMNS, rows, format: DEFAULT_DISPLAY, levels })

describe('measureColumns', () => {
    it('sizes every column by its heading when there are no rows', () => {
        const m = measure([], FLAT)
        // A heading may wrap at its words: "CUSTOMER" (103) is more than the text floor (93).
        expect(m.name).toEqual({
            natural: textWidth('Customer name', HEADING) + CELL_PADDING,
            min: longestWordWidth('Customer name', HEADING) + CELL_PADDING,
            grow: 1,
        })
        // A sortable heading keeps room for its icon.
        expect(m.debit.natural).toBe(textWidth('Debit', HEADING) + SORT_ICON_ROOM + CELL_PADDING)
        const date = textWidth('Date', HEADING) + CELL_PADDING
        expect(m.account).toEqual({ natural: date, min: date, grow: 1 })
    })

    it('never cuts a figure: its min is its widest value, Total rows in semibold included', () => {
        const m = measure([DETAIL, TOTAL])
        const debit = COLUMNS[4] as ResultColumn
        const total = textWidth(cellDisplay(debit, TOTAL.row, 3510000, DEFAULT_DISPLAY).text, BOLD)
        const worst = textWidth(cellDisplay(debit, DETAIL.row, -9_999_999, DEFAULT_DISPLAY).text, CELL)
        expect(m.debit.natural).toBe(Math.max(total, worst) + CELL_PADDING)
        expect(m.debit.min).toBe(m.debit.natural)
        expect(m.debit.grow).toBe(0)
    })

    it('keeps the figure worst case when the first rows are short', () => {
        const small = item('detail', '2026-10-02', { debit: 500 }, { labelType: 'date', labelValue: '2026-10-02' })
        const worst = (format: typeof DEFAULT_DISPLAY) =>
            textWidth(cellDisplay(COLUMNS[4] as ResultColumn, small.row, -9_999_999, format).text, CELL) + CELL_PADDING
        expect(measure([small]).debit.natural).toBe(worst(DEFAULT_DISPLAY))
        // With parentheses for negatives the worst case is one character wider.
        const parens = { ...DEFAULT_DISPLAY, negative: 'parens' as const }
        const m = measureColumns({ columns: COLUMNS, rows: [small], format: parens, levels: GROUPED })
        expect(m.debit.natural).toBe(worst(parens))
        expect(worst(parens)).toBeGreaterThan(worst(DEFAULT_DISPLAY))
    })

    it('never cuts short text, and holds long text to eight characters at the least', () => {
        const m = measure([DETAIL])
        expect(m.number.natural).toBe(textWidth('JE-41', CELL) + CELL_PADDING)
        expect(m.number.min).toBe(m.number.natural)
        expect(m.memo.natural).toBe(textWidth('Reversal of JE-12 for October rent', CELL) + CELL_PADDING)
        expect(m.memo.min).toBe(TEXT_MIN_WIDTH + CELL_PADDING)
        expect(m.memo.grow).toBe(1)
    })

    it('shrinks an all-empty column to its heading', () => {
        const m = measure([DETAIL, GROUP, TOTAL])
        expect(m.name.natural).toBe(textWidth('Customer name', HEADING) + CELL_PADDING)
    })

    it('sizes the label by its detail rows, and never cuts a typed date label', () => {
        const m = measure([DETAIL, GROUP])
        const dateNeed = INDENT + INDENT + textWidth('Sep 30, 2026', CELL) + CELL_PADDING / 2
        expect(m.account.natural).toBe(dateNeed)
        expect(m.account.min).toBe(dateNeed)
        // The open group row spans the cells after it, so it does not size the column.
        expect(m.account.natural).toBeLessThan(labelNeed(GROUP, DEFAULT_DISPLAY))
    })

    it('counts a non-detail row whose first other cell has a value: it spans nothing', () => {
        const closed = item('group', '1000 Business Checking', { number: '1000' }, { depth: 0 }, true)
        expect(measure([closed]).account.natural).toBe(labelNeed(closed, DEFAULT_DISPLAY))
    })

    it('lets a text label narrow to its chrome and eight characters', () => {
        const text = item('detail', 'Harbor Lane Design LLC, Accounts receivable', {})
        const m = measure([text])
        expect(m.account.min).toBe(labelChrome(GROUPED) + TEXT_MIN_WIDTH)
        expect(m.account.grow).toBe(1)
    })
})

describe('the label chrome', () => {
    it('is the base indent, the deepest label start and pr-3', () => {
        // One group level: a detail row at 16, a group row's caret at 18.
        expect(deepestLabelStart(GROUPED)).toBe(CARET_ROOM)
        expect(labelChrome(GROUPED)).toBe(INDENT + CARET_ROOM + CELL_PADDING / 2)
        expect(labelChrome(FLAT)).toBe(INDENT + CELL_PADDING / 2)
    })

    it('counts a toggle row’s caret in its label need', () => {
        expect(labelNeed(GROUP, DEFAULT_DISPLAY)).toBe(
            INDENT + CARET_ROOM + textWidth('1000 Business Checking', BOLD) + CELL_PADDING / 2
        )
        expect(cellTextStyle('detail')).toEqual(CELL)
        expect(cellTextStyle('subtotal')).toEqual(BOLD)
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/column-measure.test.ts`
Expected: FAIL — `Failed to resolve import "../column-measure"`.

- [ ] **Step 3: Write the implementation**

Create `core/reports/lib/column-measure.ts`. Move `FIGURE_TYPES`, `isFigureColumn`, `RowLevels`, `FLAT_LEVELS`, `rowLevelsOf`, `groupLabelStart` and `deepestLabelStart` from `table-lines.ts:298-358` into it unchanged except: `deepestLabelStart` is exported, and `isFigureColumn` also reads `totalType`.

```ts
// core/reports/lib/column-measure.ts
import { CARET_ROOM, INDENT } from '../components/report-table-layout'
import { shortDay } from './dates'
import { cellDisplay, headingText, type NumberFormat, rowLabelText } from './format'
import type { VisibleRow } from './rows'
import type { SpecColumn } from './spec'
import { longestWordWidth, type TextStyle, textWidth } from './text-width'
import type { FieldType, ResultColumn, RowKind } from './types'

// Review §7.1-1: every column's width comes from its content — its heading, the
// rows of the run as it first lands (group, subtotal and Total rows included) and
// fixed worst cases — never from the screen. Pure, so web, native and an export at
// a page width size columns the same way. The measure is kept for the run (D1).

/** A column's widths: what its content needs, the least it may get, and its share of spare room. */
export type ColumnMeasure = { natural: number; min: number; grow: number }
export type ColumnMeasures = Record<string, ColumnMeasure>

/** A cell's px-3 on both sides. */
export const CELL_PADDING = 24
/** A sortable heading's icon (12) and its gap-1. */
export const SORT_ICON_ROOM = 16
export const TEXT_MIN_CHARS = 8
/** Rows read for the measure: enough for a first page, bounded for a huge one. */
export const MEASURE_ROW_LIMIT = 500

const CELL: TextStyle = { size: 'sm', weight: 'normal' }
const CELL_BOLD: TextStyle = { size: 'sm', weight: 'semibold' }
const HEADING: TextStyle = { size: 'xs', weight: 'semibold', uppercase: true }

/** The least a text column is held to: eight average letters. */
export const TEXT_MIN_WIDTH = textWidth('n'.repeat(TEXT_MIN_CHARS), CELL)

const BOLD_KINDS: ReadonlySet<RowKind> = new Set(['group', 'subtotal', 'computed', 'total'])

/** A row's text style, as ReportRow's ROW_WEIGHT draws it. */
export function cellTextStyle(kind: RowKind): TextStyle {
    return BOLD_KINDS.has(kind) ? CELL_BOLD : CELL
}

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
    return FIGURE_TYPES.has(column.type) || (!!column.totalType && FIGURE_TYPES.has(column.totalType))
}

// The widest value a column of this type may show on a later page, so a column
// sized by a first page of small values still holds the usual ones. Amounts and
// numbers are formatted as the column draws them (currency, ÷1000, cents, a minus
// or parentheses), so the worst case follows the reader's number format.
const WORST_DAY = shortDay('2026-09-30')
const WORST_TEXT: Partial<Record<FieldType, string>> = {
    date: WORST_DAY,
    datetime: WORST_DAY,
    bool: 'Yes',
    percent: '-100.0%',
}
/** −99,999.99 in cents, and −99,999. */
const WORST_VALUE: Partial<Record<FieldType, number>> = { money: -9_999_999, number: -99_999 }

function worstText(column: ResultColumn, item: VisibleRow, format: NumberFormat): string | undefined {
    const value = WORST_VALUE[column.type]
    if (value === undefined) return WORST_TEXT[column.type]
    return cellDisplay(column, item.row, value, format).text
}

/* … RowLevels, FLAT_LEVELS, rowLevelsOf, groupLabelStart and deepestLabelStart, moved
   verbatim from table-lines.ts:312-358; deepestLabelStart gains `export` … */

/** The label column's room besides its text: the base indent, the deepest label's start and pr-3. */
export function labelChrome(levels: RowLevels): number {
    return INDENT + deepestLabelStart(levels) + CELL_PADDING / 2
}

/** The width a row's whole label needs, as RowLabel draws it (ReportRow labelStyle). */
export function labelNeed(item: VisibleRow, format: NumberFormat): number {
    const { row } = item
    const start = INDENT + row.depth * INDENT + (item.isToggle ? CARET_ROOM : 0)
    return start + textWidth(rowLabelText(row, format), cellTextStyle(row.kind)) + CELL_PADDING / 2
}

type Widths = { natural: number; min: number }

function headingWidths(column: ResultColumn, format: NumberFormat): Widths {
    const text = headingText(column, format)
    const icon = column.sortable ? SORT_ICON_ROOM : 0
    return {
        natural: textWidth(text, HEADING) + icon + CELL_PADDING,
        min: longestWordWidth(text, HEADING) + icon + CELL_PADDING,
    }
}

const hasValue = (value: unknown) => value !== undefined && value !== null

// The widest shown value of a column, with its type's worst case when it has any
// value; 0 when every cell is blank (D3: it is then as wide as its heading).
function widestCell(column: ResultColumn, rows: readonly VisibleRow[], format: NumberFormat) {
    let widest = 0
    let valued: VisibleRow | undefined
    for (const item of rows) {
        const text = cellDisplay(column, item.row, item.cells[column.id], format).text
        if (!text) continue
        valued = valued ?? item
        widest = Math.max(widest, textWidth(text, cellTextStyle(item.row.kind)))
    }
    const worst = valued ? worstText(column, valued, format) : undefined
    if (worst) widest = Math.max(widest, textWidth(worst, CELL))
    return widest === 0 ? 0 : widest + CELL_PADDING
}

// A non-detail row spans the empty cells after its label (Task 6), so its label
// sizes the label column only when the first other cell has a value.
function spansNothing(item: VisibleRow, others: readonly ResultColumn[]) {
    const [first] = others
    return first !== undefined && hasValue(item.cells[first.id])
}

const TYPED_LABELS: ReadonlySet<FieldType> = new Set(['date', 'datetime', 'number', 'money'])

function labelWidths(
    others: readonly ResultColumn[],
    rows: readonly VisibleRow[],
    format: NumberFormat
): { widest: number; isTyped: boolean } {
    let widest = 0
    let isTyped = false
    for (const item of rows) {
        const { row } = item
        if (row.kind !== 'detail' && !spansNothing(item, others)) continue
        widest = Math.max(widest, labelNeed(item, format))
        if (row.kind !== 'detail' || !row.labelType || !TYPED_LABELS.has(row.labelType)) continue
        isTyped = true
        if (row.labelType === 'date' || row.labelType === 'datetime') {
            const start = INDENT + row.depth * INDENT
            widest = Math.max(widest, start + textWidth(WORST_DAY, CELL) + CELL_PADDING / 2)
        }
    }
    return { widest, isTyped }
}

export function measureColumns(input: {
    columns: readonly ResultColumn[]
    rows: readonly VisibleRow[]
    format: NumberFormat
    levels: RowLevels
}): ColumnMeasures {
    const { columns, format, levels } = input
    const rows = input.rows.slice(0, MEASURE_ROW_LIMIT)
    const [label, ...others] = columns
    const measures: ColumnMeasures = {}
    if (label) {
        const heading = headingWidths(label, format)
        const { widest, isTyped } = labelWidths(others, rows, format)
        const natural = Math.max(heading.natural, widest)
        const floor = isTyped ? widest : Math.min(natural, labelChrome(levels) + TEXT_MIN_WIDTH)
        measures[label.id] = { natural, min: Math.max(heading.min, floor), grow: 1 }
    }
    for (const column of others) {
        const heading = headingWidths(column, format)
        const widest = widestCell(column, rows, format)
        const natural = Math.max(heading.natural, widest)
        if (isFigureColumn(column)) {
            measures[column.id] = { natural, min: Math.max(heading.min, widest), grow: 0 }
            continue
        }
        const floor = Math.min(natural, TEXT_MIN_WIDTH + CELL_PADDING)
        measures[column.id] = { natural, min: Math.max(heading.min, floor), grow: 1 }
    }
    return measures
}
```

In `core/reports/lib/table-lines.ts`: delete lines 298-358 (the moved code) and add at the top:

```ts
import { deepestLabelStart, FLAT_LEVELS, isFigureColumn, type RowLevels } from './column-measure'

export { FLAT_LEVELS, isFigureColumn, type RowLevels, rowLevelsOf } from './column-measure'
```

`labelFloor` (`table-lines.ts:364-366`) keeps working on the imported `deepestLabelStart` until Task 3 removes it.

- [ ] **Step 4: Run the tests**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/column-measure.test.ts core/reports/lib/__tests__/table-lines.test.ts core/reports/components/__tests__/report-table-layout.test.ts`
Expected: PASS (all; the old table tests do not change in this task).

- [ ] **Step 5: Typecheck, lint, commit**

Run: `pnpm exec tinycld-pkg typecheck` (expect no errors) and biome on the three files. Imports that `table-lines.ts` no longer uses after the move (`FieldType`, `SpecColumn`, `INDENT`, `CARET_ROOM`, where only the moved code used them) are removed; biome names any left.

```bash
git add core/reports/lib/column-measure.ts core/reports/lib/table-lines.ts core/reports/lib/__tests__/column-measure.test.ts
git commit -m "reports: measure each column from its heading and rows"
```

E2E this task breaks: none (nothing draws from the measure yet).

---

### Task 3: One line by content (pure)

**Files:**
- Modify: `core/reports/lib/table-lines.ts:360-436` (`labelFloor` removed; `fitOneLine` replaced; `shareWidth`, `measureOf` added; `tableLines` takes `measures`)
- Modify: `core/reports/lib/column-measure.ts` (add `measureKey`)
- Modify: `core/reports/components/report-table-layout.ts:5-8,14-20` (remove `AMOUNT_WIDTH`, `LABEL_MIN_WIDTH`, `PHONE_LABEL_WIDTH`, `PHONE_AMOUNT_WIDTH`, `TEXT_FLOOR`; the comment at 17 that names `labelFloor` goes with them)
- Test: `core/reports/lib/__tests__/table-lines.test.ts`, `core/reports/lib/__tests__/column-measure.test.ts`, `core/reports/components/__tests__/report-table-layout.test.ts`

**Interfaces:**
- Consumes: `ColumnMeasure`, `ColumnMeasures`, `measureColumns`, `labelChrome`, `deepestLabelStart` (Task 2).
- Produces:
  - `shareWidth(parts: readonly ColumnMeasure[], width: number): number[] | null` (table-lines.ts) — CSS `table-layout: auto`: each part at its `min`; spare room first in proportion to `natural − min` up to `natural`, then by `grow`; the parts add up to `width` exactly (the rounding remainder goes to the first part); null when the mins do not fit.
  - `measureOf(measures: ColumnMeasures, id: string): ColumnMeasure` — `{ natural: 0, min: 0, grow: 0 }` for an unknown id.
  - `fitOneLine(columns: readonly ResultColumn[], width: number, measures: ColumnMeasures): Record<string, number> | null`
  - `tableLines({ columns, width, breakpoint, layouts, levels?, measures? }): LineLayout` (absent `measures`: `measureColumns` with no rows)
  - `measureKey(input: { runKey: string; format: NumberFormat; columns: readonly ResultColumn[]; levels: RowLevels; hasDetail: boolean }): string` (column-measure.ts)

The table still calls `tableLines` without `measures` until Task 4 wires the hook, so the screen draws from the headings and worst cases alone between the two commits; no e2e is run at this commit.

- [ ] **Step 1: Write the failing tests**

In `core/reports/components/__tests__/report-table-layout.test.ts` delete `describe('fitOneLine', …)` (lines 35-152) with the fixtures above it that only it uses (`WIDE`, `TEXT_IDS`, `wideWidths`, from line 13), drop `fitOneLine` from the import at line 2, and replace the removed constants in the import at lines 4-11 with `import { columnAlign } from '../report-table-layout'`. Keep `rowLevelsOf`, `isFigureColumn`, `columnAlign` tests; add to the `isFigureColumn` test:

```ts
        expect(isFigureColumn({ id: 'x', label: 'X', type: 'text', totalType: 'number' })).toBe(true)
```

In `core/reports/lib/__tests__/table-lines.test.ts`, change the import (lines 5-17): drop `labelFloor`, add `measureOf`, `shareWidth`; add `import type { ColumnMeasures } from '../column-measure'` and `import { labelChrome } from '../column-measure'`. Add:

```ts
const m = (min: number, natural: number, grow: number) => ({ min, natural, grow })

describe('shareWidth', () => {
    const parts = [m(100, 200, 1), m(50, 50, 0), m(80, 120, 1)]

    it('fits the mins exactly, and refuses less', () => {
        expect(shareWidth(parts, 230)).toEqual([100, 50, 80])
        expect(shareWidth(parts, 229)).toBeNull()
    })

    it('gives spare room in proportion to what each part lacks, the remainder to the first', () => {
        // 50 spare over lacks 100 and 40: floor(35.7) = 35, floor(14.3) = 14; 1 left.
        expect(shareWidth(parts, 280)).toEqual([136, 50, 94])
        expect(shareWidth(parts, 370)).toEqual([200, 50, 120])
    })

    it('then gives the rest by grow', () => {
        expect(shareWidth(parts, 470)).toEqual([250, 50, 170])
        expect(shareWidth([m(10, 10, 0), m(20, 20, 0)], 50)).toEqual([30, 20])
    })

    it('handles no parts and parts with no width', () => {
        expect(shareWidth([], 100)).toEqual([])
        expect(shareWidth([m(0, 0, 0), m(0, 0, 0)], 10)).toEqual([10, 0])
    })
})

// Module scope: the per-width tests below reuse it.
const COLS_MEASURES: ColumnMeasures = {
    account: m(120, 180, 1),
    number: m(64, 64, 1),
    debit: m(110, 110, 0),
    credit: m(110, 110, 0),
}

describe('fitOneLine by content', () => {
    const measures = COLS_MEASURES

    it('gives each column its natural width and the spare room to the text', () => {
        // 464 natural; 36 spare by grow over account and number.
        expect(fitOneLine(COLS, 500, measures)).toEqual({ account: 198, number: 82, debit: 110, credit: 110 })
    })

    it('narrows the label toward its min before the line breaks', () => {
        expect(fitOneLine(COLS, 424, measures)).toEqual({ account: 140, number: 64, debit: 110, credit: 110 })
        expect(fitOneLine(COLS, 403, measures)).toBeNull()
        expect(fitOneLine([], 10, measures)).toEqual({})
    })

    it('reads an unknown column as needing nothing', () => {
        expect(measureOf(measures, 'nope')).toEqual({ natural: 0, min: 0, grow: 0 })
    })
})
```

Rewrite the tests that read the old fixed widths so they pass explicit measures (the trees they expect stay):
- `the table layout per width` › `draws one line with its natural widths wherever the columns fit` (lines 104-112): pass `measures: COLS_MEASURES` and expect `fitOneLine(COLS, 1280, measures)` / `fitOneLine(COLS, 900, measures)` as `widths`.
- `wraps a phone row…` (113-139) and `wraps the amounts onto more lines…` (140-173): add `measures: { account: m(300,300,1), number: m(64,64,1), debit: m(110,110,0), credit: m(110,110,0) }` so one line does not fit at 360 / 300 (the old `autoLines` tree is unchanged in this task).
- `F10: one line that fits, then the narrower arrangements` (650-754): give `lines()` a `measures` argument built for `WIDE` — label `m(120, 220, 1)`, each text column `m(93, 128, 1)`, each money column `m(110, 110, 0)` (Σmin 915, Σnatural 220 + 5 × 128 + 3 × 110 = 1190) — and change the expected widths: at 1190 every natural; at 1300 `fitOneLine(WIDE, 1300, measures)` (the label and text columns share the 110 spare by grow); at 1100 `fitOneLine(WIDE, 1100, measures)`; at 914 null; keep every arrangement expectation (Task 5 rewrites the choice).
- `wraps a grouped phone report…` (712-747): replace with `the label min grows with the deepest label start`: one detail row labelled `'Harbor Lane Design LLC, Accounts receivable'` (depth 0, no type); `measureColumns({ columns: COLS, rows: [row], format: DEFAULT_DISPLAY, levels })` gives `account.min === labelChrome(levels) + TEXT_MIN_WIDTH` for `{ groups: 0, nests: 0 }` and for `{ groups: 1, nests: 0 }`, and the grouped min is 18 (`CARET_ROOM`) more than the flat one.
- `the label floor of a pivot report` › `counts the pivot level…` (833-842): `labelFloor` is gone; test `deepestLabelStart` (import from `../column-measure`) instead: `{ groups: 1, nests: 0, pivot: true }` → 0, `{ groups: 1, nests: 0 }` → 18, `{ groups: 2, nests: 0, pivot: true }` → 18, `{ groups: 2, nests: 0 }` → 34, `{ groups: 3, nests: 0, pivot: true }` → 34, `{ groups: 0, nests: 0 }` → 0.
- `fits a one-level pivot on a phone line where a grouped report wraps` (844-852): with no rows both label mins are the heading's, so give `measureColumns` one detail row with a long text label (as in `the label min grows with the deepest label start`) for `{ groups: 1, nests: 0, pivot: true }` and `{ groups: 1, nests: 0 }`; assert the pivot's `account.min` is 18 (`CARET_ROOM`) less than the grouped one's. Rename it `gives a one-level pivot a label min 18 px less than a grouped report's`.

Add to `column-measure.test.ts`:

```ts
describe('measureKey', () => {
    const base = { runKey: 'r1', format: DEFAULT_DISPLAY, columns: COLUMNS, levels: GROUPED, hasDetail: true }

    it('stays for the same run, columns and format', () => {
        expect(measureKey(base)).toBe(measureKey({ ...base, columns: [...COLUMNS] }))
    })

    it('changes with the run, a hidden column, the number format and the levels', () => {
        expect(measureKey({ ...base, runKey: 'r2' })).not.toBe(measureKey(base))
        expect(measureKey({ ...base, columns: COLUMNS.slice(0, 4) })).not.toBe(measureKey(base))
        expect(measureKey({ ...base, format: { ...DEFAULT_DISPLAY, divideBy1000: true } })).not.toBe(measureKey(base))
        expect(measureKey({ ...base, levels: FLAT })).not.toBe(measureKey(base))
        // Groups that start closed: measured again when the first detail rows show.
        expect(measureKey({ ...base, hasDetail: false })).not.toBe(measureKey(base))
    })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts core/reports/lib/__tests__/column-measure.test.ts core/reports/components/__tests__/report-table-layout.test.ts`
Expected: FAIL — `shareWidth`, `measureOf`, `measureKey` are not exported; `fitOneLine` has the old signature.

- [ ] **Step 3: Write the lib code**

`core/reports/lib/table-lines.ts`: remove the import of the removed constants (lines 2-10 keep only nothing from `report-table-layout`), delete `labelFloor` (360-366) and the old `fitOneLine` (368-402), and add:

```ts
import { DEFAULT_DISPLAY } from './format'
import { type ColumnMeasure, type ColumnMeasures, measureColumns } from './column-measure'

const NO_MEASURE: ColumnMeasure = { natural: 0, min: 0, grow: 0 }

export function measureOf(measures: ColumnMeasures, id: string): ColumnMeasure {
    return measures[id] ?? NO_MEASURE
}

const sum = (values: readonly number[]) => values.reduce((total, n) => total + n, 0)

/**
 * Widths that share `width` by content, as CSS table-layout:auto does: each part at
 * its min; the spare room first in proportion to what each part lacks of its
 * natural width, then by grow. The parts add up to `width` exactly: the rounding
 * remainder goes to the first part (the label). Null when the mins do not fit.
 */
export function shareWidth(parts: readonly ColumnMeasure[], width: number): number[] | null {
    if (parts.length === 0) return []
    const widths = parts.map(part => part.min)
    if (sum(widths) > width) return null
    const lack = parts.map(part => Math.max(0, part.natural - part.min))
    const totalLack = sum(lack)
    if (totalLack > 0) {
        const share = Math.min(1, (width - sum(widths)) / totalLack)
        for (const [i, need] of lack.entries()) widths[i] = (widths[i] ?? 0) + Math.floor(need * share)
    }
    const spare = width - sum(widths)
    const totalGrow = sum(parts.map(part => part.grow))
    if (spare > 0 && totalGrow > 0) {
        for (const [i, part] of parts.entries())
            widths[i] = (widths[i] ?? 0) + Math.floor((spare * part.grow) / totalGrow)
    }
    widths[0] = (widths[0] ?? 0) + width - sum(widths)
    return widths
}

/**
 * The widths of one line at this width (review §7.1-3), or null when the columns'
 * mins do not fit: figures, dates and short text are never cut (their min is their
 * natural width); long text narrows to its min, in proportion to what it lacks.
 */
export function fitOneLine(
    columns: readonly ResultColumn[],
    width: number,
    measures: ColumnMeasures
): Record<string, number> | null {
    const widths = shareWidth(
        columns.map(column => measureOf(measures, column.id)),
        width
    )
    if (!widths) return null
    return Object.fromEntries(columns.map((column, i) => [column.id, widths[i] ?? 0]))
}
```

`tableLines` input gains `measures?: ColumnMeasures`; its body uses `const measured = input.measures ?? measureColumns({ columns, rows: [], format: DEFAULT_DISPLAY, levels: levels ?? FLAT_LEVELS })` and `fitOneLine(columns, width, measured)`. The rest of `tableLines` (own layout first, `NARROWER`, `autoLines`) is unchanged in this task.

`core/reports/lib/column-measure.ts`, add:

```ts
/**
 * What the run's measure depends on (D1): the run (design, params, sort), the
 * number format, the shown columns and the row levels. A later page, a fold and a
 * live re-run keep the key, so the columns never move under the reader.
 */
export function measureKey(input: {
    runKey: string
    format: NumberFormat
    columns: readonly ResultColumn[]
    levels: RowLevels
    /** Whether any detail row is shown: a report whose groups start closed measures again when one opens. */
    hasDetail: boolean
}): string {
    const { runKey, format, columns, levels, hasDetail } = input
    const shape = columns.map(column => [column.id, column.label, column.sortable === true])
    return JSON.stringify([runKey, format, shape, levels, hasDetail])
}
```

`core/reports/components/report-table-layout.ts`: delete lines 5-8 and 14-20 (the five constants and the `TEXT_FLOOR` comment). Keep `ROW_HEIGHT`, `INDENT`, `CARET_SIZE`, `CARET_ROOM`, `columnAlign`.

- [ ] **Step 4: Run the unit tests**

Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Typecheck, lint, commit**

Run: `pnpm exec tinycld-pkg typecheck` (expect no errors; `ReportTable.tsx` still passes `breakpoint`, which `tableLines` still takes) and biome on the changed files. Imports that become unused: `table-lines.ts` drops its `report-table-layout` import; `report-table-layout.test.ts:2` drops `fitOneLine`; `table-lines.test.ts` drops `labelFloor`.

```bash
git add core/reports/lib/table-lines.ts core/reports/lib/column-measure.ts core/reports/components/report-table-layout.ts core/reports/lib/__tests__/table-lines.test.ts core/reports/lib/__tests__/column-measure.test.ts core/reports/components/__tests__/report-table-layout.test.ts
git commit -m "reports: share a line's width by its columns' content"
```

E2E this task breaks: none (nothing passes a measure yet).

---

### Task 4: The table wired to the run's measure

**Files:**
- Create: `core/reports/hooks/useStable.ts` (moved from `ReportTable.tsx:34-39`)
- Create: `core/reports/hooks/useTableLayout.ts`
- Modify: `core/reports/components/ReportRow.tsx:271-281,400` (label box at its width; sticky label lines full width)
- Modify: `core/reports/components/ReportTable.tsx:1-39,352-426` (props; no layout computing)
- Modify: `core/reports/components/ReportView.tsx:77-166` (hook; `ReportBody({ model, table })`)
- Modify: `core/reports/components/builder/ReportBuilderPanel.tsx:262` (preview calls the hook)
- Test (e2e): `tests/e2e/reports-wrap.spec.ts:393-552,668-714`, `tests/e2e/reports-ledger-v3.spec.ts:40-46,447-488`

**Interfaces:**
- Consumes: `measureColumns`, `measureKey` (Tasks 2–3); `tableLines` with `measures` (Task 3).
- Produces:
  - `useStable<T>(value: T): T` (`core/reports/hooks/useStable.ts`)
  - `type TableLayoutModel = Pick<ReportViewModel, 'columns' | 'rows' | 'display' | 'breakpoint' | 'layouts' | 'rowLevels' | 'run'>`; `interface TableLayout { layout: LineLayout; isMeasured: boolean; onRowWidth(width: number): void }`; `useTableLayout(model: TableLayoutModel): TableLayout`; `shownRunKey(model: Pick<ReportViewModel, 'run'>): string` (all in `core/reports/hooks/useTableLayout.ts`)
  - `ReportBody({ model, table }: { model: ReportViewModel; table: TableLayout })`
  - `ReportTableProps`: removes `breakpoint`, `layouts`, `rowLevels`; adds `layout: LineLayout`, `isMeasured: boolean`, `onRowWidth(width: number): void`.

No unit test: the hook is wiring over the pure `measureColumns` / `measureKey` / `tableLines` (CLAUDE.md: no vitest for components). The e2e of Step 4 covers it.

- [ ] **Step 1: Write the hooks**

`core/reports/hooks/useStable.ts` — move `useStable` and its comment verbatim from `ReportTable.tsx:32-39` and export it; `ReportTable.tsx` imports it from `'../hooks/useStable'`.

```ts
// core/reports/hooks/useTableLayout.ts
import { useWindowWidth } from '@tinycld/core/components/workspace/useBreakpoint'
import { useMemo, useState } from 'react'
import { type ColumnMeasures, measureColumns, measureKey } from '../lib/column-measure'
import { type LineLayout, tableLines } from '../lib/table-lines'
import type { ReportViewModel } from './useReportView'
import { useStable } from './useStable'

export type TableLayoutModel = Pick<
    ReportViewModel,
    'columns' | 'rows' | 'display' | 'breakpoint' | 'layouts' | 'rowLevels' | 'run'
>

export interface TableLayout {
    layout: LineLayout
    /** False until the rows' width is measured: the table is drawn hidden until then. */
    isMeasured: boolean
    onRowWidth(width: number): void
}

// Which run's rows are shown: its design and its resolved params. It changes when
// a run of a new design or new params lands; not while that run loads behind the
// earlier rows, not when a live re-run of the same design and params answers, and
// not on a group fold, a page load or a display-only change.
export function shownRunKey(model: Pick<ReportViewModel, 'run'>): string {
    return JSON.stringify([model.run.shownRowsKey ?? '', model.run.result?.params ?? []])
}

// The measure taken from the rows shown when the key first appears, kept while the
// key stays (D1): a later page, a fold or a live re-run never moves a column. Kept
// in render, as useStable keeps a value; no effect. `hasDetail` makes a report whose
// groups start closed measure once more when its first detail rows are shown, so
// the text columns get their content's width then, not their heading's.
function useRunMeasures(model: TableLayoutModel): ColumnMeasures {
    const levels = model.rowLevels
    const key = measureKey({
        runKey: shownRunKey(model),
        format: model.display,
        columns: model.columns,
        levels,
        hasDetail: model.rows.some(item => item.row.kind === 'detail'),
    })
    const [kept, setKept] = useState<{ key: string; measures: ColumnMeasures } | null>(null)
    if (kept?.key === key) return kept.measures
    const measures = measureColumns({
        columns: model.columns,
        rows: model.rows,
        format: model.display,
        levels,
    })
    setKept({ key, measures })
    return measures
}

/**
 * The lines of a report table at the width its rows really get. The width is
 * measured by the table's probe (a builder drawer or the Customize panel narrows
 * it); until then the window's width stands in and the table is drawn hidden.
 */
export function useTableLayout(model: TableLayoutModel): TableLayout {
    const windowWidth = useWindowWidth()
    const [rowWidth, setRowWidth] = useState<number>()
    const measures = useRunMeasures(model)
    const columns = useStable(model.columns)
    const levels = useStable(model.rowLevels)
    const { breakpoint, layouts } = model
    const width = rowWidth ?? windowWidth
    const layout = useMemo(
        () => tableLines({ columns, width, breakpoint, layouts, levels, measures }),
        [columns, width, breakpoint, layouts, levels, measures]
    )
    return { layout, isMeasured: rowWidth !== undefined, onRowWidth: setRowWidth }
}
```

The full `levels` object now reaches `tableLines`, `pivot` included (review §2, the `ReportTable.tsx:398` bug).

- [ ] **Step 2: Wire the components**

`core/reports/components/ReportTable.tsx`:
- Remove `useWindowWidth`, `Breakpoint`, `DisplayLayout`, `RowLevels`, `tableLines` imports and `useStable`'s definition (now imported).
- `ReportTableProps` (352-370): delete `breakpoint`, `layouts`, `rowLevels`; add

```ts
    /** The lines of every row and the heading (useTableLayout). */
    layout: LineLayout
    /** False until the rows' width is measured: the table is drawn hidden until then. */
    isMeasured: boolean
    /** Told the width the rows really get (the list's content width). */
    onRowWidth(width: number): void
```

- `ReportTable` (378-426): delete `windowWidth`, `rowWidth` state, `lineWidth`, the `layout` memo; read `props.layout`; `visibility = props.isMeasured ? '' : 'opacity-0'`; pass `onRowWidth={props.onRowWidth}` to `TableList`.

`core/reports/components/ReportRow.tsx`:
- `oneLineLabelStyle` (275-281) becomes `return { width: oneLineLabelWidth(layout, labelId, spanned) }` and its comment: "The label column's box on one line: the width the line gave it and the widths of the columns it spans."
- In `StickyGroup` (400) replace `const labelBox = oneLineLabelStyle(props.layout, props.columns[0]?.id)` with a module constant `const STICKY_LABEL_BOX = { flex: 1 } as const` (a stuck label line runs across the row) and use it.

`core/reports/components/ReportView.tsx`:
- Import `shownRunKey`, `type TableLayout`, `useTableLayout` from `'../hooks/useTableLayout'`; delete the local `shownRunKey` and its comment (133-139).
- `ReportBody({ model, table }: { model: ReportViewModel; table: TableLayout })`; `TableBody({ model, table })` passes `layout={table.layout} isMeasured={table.isMeasured} onRowWidth={table.onRowWidth}` instead of `breakpoint`, `layouts`, `rowLevels`.
- `ViewBody({ model, table })` passes `table` on.
- `ReportView`: first line `const table = useTableLayout(model)`; `<ViewBody model={model} table={table} />`.

`core/reports/components/builder/ReportBuilderPanel.tsx`: in the phone sheet component that renders `<ReportBody model={preview} />` (line 262), add `const table = useTableLayout(preview)` at the top of the component with the other hooks, import it, and render `<ReportBody model={preview} table={table} />`.

- [ ] **Step 3: Typecheck and unit tests**

Run: `pnpm exec tinycld-pkg typecheck` — expected no errors (grep `breakpoint=` in `core/reports/components` to find a missed prop).
Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports` — expected PASS.

- [ ] **Step 4: Update the e2e this task changes**

The one line is now sized by content, so a column is as wide as its values, not 128 px, and a short label is no longer cut at 1100.

`tests/e2e/reports-wrap.spec.ts`:
- `on a desktop a table wider than its view narrows its text columns and never scrolls sideways` (393-494): replace lines 481-490 (label cut; amount heading 128 px) with:

```ts
            // Every figure keeps its whole value and heading (R12); the short label is whole.
            const isCut = (testId: string) =>
                p.getByTestId(testId).evaluate(el => {
                    const text = el.querySelector('div[dir="auto"]') ?? el
                    return text.scrollWidth > text.clientWidth
                })
            const id = (await label.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
            for (const col of ['amount', 'cost', 'quantity', 'day', 'booked']) {
                await expect.poll(() => isCut(`report-cell-${col}-${id}`)).toBe(false)
                await expect.poll(() => isCut(`report-heading-${col}`)).toBe(false)
            }
            const text = label.getByText('Wrapco 00', { exact: true })
            expect(await text.evaluate(el => el.scrollWidth > el.clientWidth)).toBe(false)
```

  and rename the test `on a desktop a wide table fits its columns to their content and never scrolls sideways`.
- `on a phone a column left blank on opened rows keeps its empty slot` (668-714): its comment (677-679) sums the old fixed widths. Add `await addColumn(p, ['quantity'])` after line 681 (`addColumn(p, ['cost'])`), so the row cannot fit one line at 390 by any measure, and change the comment to: "Two more columns: the row must not fit one line at 390, and this test is about the wrapped rows." The `report-builder-column-3-detail` id stays (cost is still column 3).
- `on a narrowed desktop line an open group row shows its whole label…` (499-552): lines 534-538 — the detail label "Wrapco Longname 00" now gets its natural width at 1100. Change the poll to `.toBe(false)` with the comment "The detail label gets the room its text needs." Keep the group-label and cell-alignment checks; rename the test `on a desktop line an open group row shows its whole label across the row`.

`tests/e2e/reports-ledger-v3.spec.ts`:
- Delete `Q2_NARROWED_LINE` and `TEAM_BALANCE_NARROWED_LINE` (lines 45-46).
- `on a desktop too narrow for its narrowed line…` (447-488): the numbers 882 / 1026 are the old fixed widths, and from Task 5 the arrangement used is the first that fits, which a unit test pins (Task 5 `the layout chosen by fit`) and `reports-wrap.spec.ts:222` covers on screen. Rename to `on a view too narrow for one line a report wraps inside the table and never scrolls sideways`; set the viewport to `{ width: 820, height: 720 }`; replace the first poll (475-477) with `await expect(page.getByTestId('report-table')).toHaveCSS('opacity', '1')`; delete lines 478-480 and 482 (the `headings: 8` poll, `fit.line` check and the Team heading check); change line 481 to `expect(fit.lines).toBeGreaterThanOrEqual(2)` and `expect(fit.outside).toBe(0)`; keep the two no-sideways-scroll checks (with 820 in place of 1024). Replace the comment above the test with: "A report too wide for one line at 820 wraps (by an arrangement or by itself); every heading stays inside the table and nothing scrolls sideways."

- [ ] **Step 5: Run the e2e this task touches**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts` and `… -- reports-ledger-v3.spec.ts`
Expected: PASS.

- [ ] **Step 6: Lint and commit**

```bash
git add core/reports/hooks/useStable.ts core/reports/hooks/useTableLayout.ts core/reports/components/ReportRow.tsx core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/reports/components/builder/ReportBuilderPanel.tsx tests/e2e/reports-wrap.spec.ts tests/e2e/reports-ledger-v3.spec.ts
git commit -m "reports: draw the table from the run's column measure"
```

E2E this task breaks (fixed in Step 4): `reports-wrap.spec.ts:481-490`, `:534-538`, `:677-681`; `reports-ledger-v3.spec.ts:45-46,447-488`. Books (after merge): re-check `general-ledger.spec.ts:873-912` (900 px tablet case).

---

### Task 5: Layout choice by fit; authored and automatic lines fitted by content; continuation indent

**Files:**
- Modify: `core/reports/lib/table-lines.ts:33-44` (types), `:213-234` (`autoLines`), `:262-296` (`authoredLines`), `:404-436` (`NARROWER`, `tableLines`)
- Modify: `core/reports/components/ReportRow.tsx:62-111,333-368` (`SideBlock` px widths; `RootBlockView`)
- Modify: `core/reports/components/ReportTable.tsx:196-212` (`BlocksHeading` draws through `RootBlockView`)
- Modify: `core/reports/hooks/useTableLayout.ts` (no `breakpoint`)
- Modify: `core/reports/components/builder/ArrangeSection.tsx:94-97,355-363` (hint and caption)
- Modify: `core/help/reports.md:119-124`, `docs/packages.md:1337`
- Test: `core/reports/lib/__tests__/table-lines.test.ts`
- Test (e2e): `tests/e2e/reports-wrap.spec.ts:148-193,668-714`

**Interfaces:**
- Consumes: `shareWidth`, `measureOf`, `fitOneLine`, `ColumnMeasures`, `measureColumns`, `CELL_PADDING`, `INDENT` (Tasks 2–3).
- Produces:
  - `type SidePart = { block: Block; tracks: number; width?: number }`; `Block`'s side variant is `{ kind: 'side'; of: number; parts: SidePart[] }`. After `tableLines`, every side part has a px `width`; a part with none (a hand-built tree) is drawn at its track share.
  - `LineLayout` blocks variant: `{ kind: 'blocks'; root: Block; indent: number; notice?: string }`
  - `continuationIndent(levels: RowLevels): number` = `max(0, INDENT + (groups + nests) × INDENT − CELL_PADDING / 2)` — the lines after a row's first start under a detail row's label text.
  - `tableLines({ columns, width, layouts, levels?, measures? }): LineLayout` — no `breakpoint`.
  - `RootBlockView({ root, indent, renderCell }: { root: Block; indent: number; renderCell: RenderCell })` (ReportRow.tsx): a stack root's lines after the first are drawn with `paddingLeft: indent`.

Choice (review §7.1-4, D5): one line (`fitOneLine`) → each authored arrangement in the order desktop, tablet, mobile, the first whose lines fit → automatic lines. An arrangement fits when every side block's parts' mins fit its width (a stack's min is its widest part's; a side's the sum of its parts'). Each side block shares its width with `shareWidth`, where a part's `grow` is its `tracks` (an authored span is its share of spare room; an empty track is room the author kept). The lines after the first of a stack root are fitted at `width − indent`. An arrangement `placeCells` refuses is skipped; one whose pivot columns collide is skipped and, when the choice ends in automatic lines, the layout carries `PIVOT_NOTICE`. Automatic lines pack the columns in order, first fit, at their mins: the label starts line 1; a column whose min does not fit the line starts the next one; a column wider than a line alone is clamped to it (it is then cut).

- [ ] **Step 1: Write the failing tests**

Add to `core/reports/lib/__tests__/table-lines.test.ts` a shape helper (strips widths and the indent so the tree tests stay readable) and new tests. Put `shapeOf` and `uniform` at module scope after the imports, and the new `describe` after the `MOBILE_LINES` constant (about line 645): its body reads `WIDE` and the layouts when the file is collected. Import `continuationIndent`, `isFigureColumn`, `type Block` and `type LineLayout` from `../table-lines`.

```ts
// The tree of a layout without its px widths and indent.
function shapeOf(layout: LineLayout): unknown {
    const strip = (block: Block): Block => {
        if (block.kind === 'stack') return { ...block, parts: block.parts.map(strip) }
        if (block.kind !== 'side') return block
        return { ...block, parts: block.parts.map(({ block: b, tracks }) => ({ block: strip(b), tracks })) }
    }
    if (layout.kind === 'one-line') return layout
    const { indent: _indent, ...rest } = layout
    return { ...rest, root: strip(layout.root) }
}

const uniform = (columns: readonly ResultColumn[], min: number): ColumnMeasures =>
    Object.fromEntries(columns.map(c => [c.id, m(min, min, isFigureColumn(c) ? 0 : 1)]))

describe('the layout chosen by fit', () => {
    // WIDE at min 110 each: one line needs 990.
    const measures = uniform(WIDE, 110)
    const at = (width: number, layouts: DisplayLayout[] = []) =>
        tableLines({ columns: WIDE, width, layouts, measures })

    it('draws one line wherever it fits, an arrangement or not', () => {
        expect(at(1000, [DESKTOP_LAYOUT, TABLET_LAYOUT, MOBILE_LAYOUT]).kind).toBe('one-line')
    })

    it('uses the widest arrangement whose lines fit, then the narrower ones', () => {
        // The fixtures carry no widths and no indent, so they compare with the shape.
        expect(shapeOf(at(900, [DESKTOP_LAYOUT, TABLET_LAYOUT]))).toEqual(DESKTOP_LINES)
        expect(shapeOf(at(900, [MOBILE_LAYOUT, TABLET_LAYOUT]))).toEqual(TABLET_LINES)
        expect(shapeOf(at(400, [MOBILE_LAYOUT]))).toEqual(MOBILE_LINES)
    })

    it('falls to the automatic lines when no arrangement fits', () => {
        // The label's min (110) is wider than the view: no arrangement fits.
        const auto = at(100, [MOBILE_LAYOUT])
        expect(auto.kind).toBe('blocks')
        expect(shapeOf(auto)).not.toEqual(MOBILE_LINES)
    })

    it('gives a side block its width by content, a span its share of the spare room', () => {
        const layout = tableLines({
            columns: COLS,
            width: 360,
            layouts: [{ width: 'mobile', tracks: 3, cells: [
                { col: 'account', line: 1, span: 2 }, { col: 'number', line: 1 },
                { col: 'debit', line: 2 }, { col: 'credit', line: 2, start: 3 },
            ] }],
            measures: { account: m(100, 100, 1), number: m(60, 60, 1), debit: m(110, 110, 0), credit: m(110, 110, 0) },
        })
        if (layout.kind !== 'blocks' || layout.root.kind !== 'stack') throw new Error('expected lines')
        const [first, second] = layout.root.parts
        if (first?.kind !== 'side' || second?.kind !== 'side') throw new Error('expected side lines')
        // One line needs 380, so the arrangement is used. Line 1: 200 spare by
        // tracks 2 : 1 → 133 and 66, the remainder 1 to the first part.
        expect(first.parts.map(p => p.width)).toEqual([234, 126])
        // The second line is fitted under the indent (a flat report: 16 − 12 = 4):
        // 356 − 220 = 136 spare by tracks 1 : 1 : 1, the empty track included.
        expect(layout.indent).toBe(4)
        expect(second.parts.map(p => p.width)).toEqual([156, 45, 155])
    })

    it('packs the automatic lines by content, the label sharing line 1', () => {
        const layout = tableLines({
            columns: COLS,
            width: 300,
            layouts: [],
            measures: { account: m(120, 120, 1), number: m(60, 60, 1), debit: m(110, 110, 0), credit: m(110, 110, 0) },
        })
        expect(shapeOf(layout)).toEqual({
            kind: 'blocks',
            root: {
                kind: 'stack',
                parts: [
                    { kind: 'side', of: 3, parts: [
                        { block: { kind: 'cell', col: 'account', justify: 'start' }, tracks: 1 },
                        { block: { kind: 'cell', col: 'number', justify: 'start' }, tracks: 1 },
                        { block: { kind: 'cell', col: 'debit', justify: 'end' }, tracks: 1 },
                    ] },
                    { kind: 'side', of: 1, parts: [
                        { block: { kind: 'cell', col: 'credit', justify: 'end' }, tracks: 1 },
                    ] },
                ],
            },
        })
    })

    it('clamps a column wider than its line, and never draws past the width', () => {
        const layout = tableLines({
            columns: COLS,
            width: 100,
            layouts: [],
            measures: { account: m(150, 150, 1), number: m(60, 60, 1), debit: m(110, 110, 0), credit: m(110, 110, 0) },
        })
        if (layout.kind !== 'blocks' || layout.root.kind !== 'stack') throw new Error('expected lines')
        for (const [i, line] of layout.root.parts.entries()) {
            if (line.kind !== 'side') throw new Error('expected side lines')
            const room = i === 0 ? 100 : 100 - layout.indent
            expect(line.parts.reduce((s, p) => s + (p.width ?? 0), 0)).toBe(room)
        }
    })

    it('starts the continuation lines under a detail row’s label text', () => {
        expect(continuationIndent({ groups: 0, nests: 0 })).toBe(4)
        expect(continuationIndent({ groups: 1, nests: 0 })).toBe(20)
        expect(continuationIndent({ groups: 1, nests: 1 })).toBe(36)
    })
})
```

Update the existing tests (all in `table-lines.test.ts`):
- Every `tableLines({ … breakpoint: '…' … })` call (lines 107, 110, 115-120, 142-147, 175-178, 224-227, 248-251, 255-258, 263-266, 278-281, 323-326, 361-364, 410-413, 445-448, 509-512, 527-530, 548-551, 655, 772-775, 858-861): delete `breakpoint`, pass `measures: uniform(<columns>, 110)` (or the test's own), and wrap compared results in `shapeOf(...)` (and the expected literal too when it is a `LineLayout`). Where a test needs an arrangement to be used, give a width at which one line does not fit (`uniform` × columns > width) and each arrangement line fits.
- `wraps the amounts onto more lines…` (140-173): the automatic lines now pack by content; replace the expected tree with the packing at `uniform(COLS, 110)` and width 300: line 1 `account`, `number` (220), line 2 `debit`, `credit`.
- `uses the arrangement for its own width only, and lays out a refused one automatically` (241-270): rename `uses the first arrangement that fits, whatever the screen, and skips a refused one`; assert a refused arrangement falls through to the next and then to the automatic lines.
- `lays out automatically, and says so, when the pivot columns collide with a cell` (508-524): keep; the notice is still set when the choice ends in automatic lines.
- `F10: one line that fits, then the narrower arrangements` (650-754): delete (replaced by `the layout chosen by fit`).
- `figure-only rows in a wrapped layout` (757-815): build `layout` with `measures: uniform(COLUMNS, 110)` and compare `shapeOf(rowLayout(...))` to the trees; `toBe(layout)` stays.

- [ ] **Step 2: Run test to verify it fails**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts`
Expected: FAIL — `continuationIndent` not exported; `tableLines` still picks by breakpoint; side parts have no `width`.

- [ ] **Step 3: Write the lib code**

Imports: add `INDENT` from `'../components/report-table-layout'` and `CELL_PADDING` from `'./column-measure'` (for `continuationIndent`); drop `Breakpoint` and `AUTO_TRACK_MIN`.

Types (`table-lines.ts:33-44`):

```ts
export type SidePart = { block: Block; tracks: number; width?: number }
export type Block =
    | { kind: 'cell'; col: string; justify: Justify }
    | { kind: 'gap'; lines: number }
    | { kind: 'stack'; parts: Block[] }
    | { kind: 'side'; of: number; parts: SidePart[] }
/** One line, each column at its width; or blocks whose side parts carry px widths. */
export type LineLayout =
    | { kind: 'one-line'; widths: Record<string, number> }
    | { kind: 'blocks'; root: Block; indent: number; notice?: string }
```

`sideParts` (162-164) returns `SidePart[]`. `build` is unchanged (it makes trees without widths). Add the fitting:

```ts
type Extent = { min: number; natural: number }

// What a block needs: a cell its column's measure, a gap nothing, a stack its
// widest part, a side block its parts side by side.
function extentOf(block: Block, measures: ColumnMeasures): Extent {
    if (block.kind === 'cell') return measureOf(measures, block.col)
    if (block.kind === 'gap') return { min: 0, natural: 0 }
    const extents = block.kind === 'stack'
        ? block.parts.map(part => extentOf(part, measures))
        : block.parts.map(part => extentOf(part.block, measures))
    const pick = block.kind === 'stack' ? Math.max : (...n: number[]) => sum(n)
    return {
        min: pick(0, ...extents.map(e => e.min)),
        natural: pick(0, ...extents.map(e => e.natural)),
    }
}

/**
 * The block at this width with px widths on every side part (D5): each side block
 * shares its width by content, a part's tracks being its share of the spare room.
 * Null when a part's min does not fit. `clamp` gives a lone cell wider than its
 * width the width it has (automatic lines), so it is cut rather than refused.
 */
function fitBlock(block: Block, width: number, measures: ColumnMeasures, clamp = false): Block | null {
    if (block.kind === 'cell') return clamp || measureOf(measures, block.col).min <= width ? block : null
    if (block.kind === 'gap') return block
    if (block.kind === 'stack') {
        const parts = block.parts.map(part => fitBlock(part, width, measures, clamp))
        return parts.every(part => part !== null) ? { ...block, parts } : null
    }
    const shares = block.parts.map(part => ({ ...extentOf(part.block, measures), grow: part.tracks }))
    const widths = shareWidth(shares, width) ?? (clamp ? clampedWidths(shares, width) : null)
    if (!widths) return null
    const parts: SidePart[] = []
    for (const [i, part] of block.parts.entries()) {
        const partWidth = widths[i] ?? 0
        const fitted = fitBlock(part.block, partWidth, measures, clamp)
        if (!fitted) return null
        parts.push({ ...part, block: fitted, width: partWidth })
    }
    return { ...block, parts }
}

// Widths when the mins do not fit (one automatic line of a single column wider than
// the view): in proportion to the mins, so they still add up to the width.
function clampedWidths(parts: readonly Extent[], width: number): number[] {
    const total = sum(parts.map(part => part.min)) || 1
    const widths = parts.map(part => Math.floor((part.min / total) * width))
    widths[0] = (widths[0] ?? 0) + width - sum(widths)
    return widths
}

/** Where the lines after a row's first start: under a detail row's label text. */
export function continuationIndent(levels: RowLevels): number {
    return Math.max(0, INDENT + (levels.groups + levels.nests) * INDENT - CELL_PADDING / 2)
}

// A stack root's first line has the whole width; the lines after it start at the
// indent, so they are fitted to what is left.
function fitRoot(root: Block, width: number, measures: ColumnMeasures, indent: number, clamp = false) {
    if (root.kind !== 'stack') return fitBlock(root, width, measures, clamp)
    const parts: Block[] = []
    for (const [i, part] of root.parts.entries()) {
        const fitted = fitBlock(part, i === 0 ? width : width - indent, measures, clamp)
        if (!fitted) return null
        parts.push(fitted)
    }
    return { ...root, parts }
}
```

`autoLines` (213-234) becomes:

```ts
// No arrangement fits: the columns in order on lines, first fit at their mins, the
// label first on line 1 (review §7.1-5). Line 1 has the whole width; the later
// lines start at the indent. A column too wide for a line alone gets that line
// and is cut (fitRoot with clamp).
function autoLines(
    columns: readonly ResultColumn[],
    width: number,
    measures: ColumnMeasures,
    indent: number
): Extract<LineLayout, { kind: 'blocks' }> {
    const lines: ResultColumn[][] = []
    let used = 0
    for (const column of columns) {
        const min = measureOf(measures, column.id).min
        const line = lines[lines.length - 1]
        const room = lines.length === 1 ? width : width - indent
        if (line && used + min <= room) {
            line.push(column)
            used += min
            continue
        }
        lines.push([column])
        used = min
    }
    const labelId = columns[0]?.id
    const sideOf = (line: ResultColumn[]): Block => ({
        kind: 'side',
        of: line.length,
        parts: line.map(column => ({
            block: { kind: 'cell', col: column.id, justify: column.id === labelId ? 'start' : justifyOf(column) },
            tracks: 1,
        })),
    })
    const root = stackOf(lines.map(sideOf))
    return { kind: 'blocks', root: fitRoot(root, width, measures, indent, true) ?? root, indent }
}
```

Delete `AUTO_TRACK_MIN` (line 25; no use outside this file) and fix the comments that name removed code: `report-table-layout.ts:17` (labelFloor) goes with the constants in Task 3; `ArrangeSection.tsx:357` (NARROWER) is rewritten in Step 6.

`authoredLines` (262-296) returns the tree or a refusal instead of calling `autoLines`:

```ts
type Authored = { kind: 'tree'; root: Block } | { kind: 'refused'; notice?: string }

function authoredLines(layout: DisplayLayout, columns: readonly ResultColumn[]): Authored {
    const arranged = placeCells(layout.tracks, layout.cells)
    if ('problem' in arranged) return { kind: 'refused' }
    const cells = expandPivots(layout, arranged.placed, columns)
    const result = placeCells(layout.tracks, cells)
    if ('problem' in result) return { kind: 'refused', notice: PIVOT_NOTICE }
    /* … lines 275-294 unchanged, ending in: */
    if (labelId && !cells.some(c => c.col === labelId))
        return { kind: 'tree', root: stackOf([label, tree]) }
    return { kind: 'tree', root: tree }
}
```

Delete `NARROWER` (404-410). `tableLines` (412-436):

```ts
// Arrangements are tried widest first, whatever the screen: the screen's width
// class only names the builder's tabs.
const WIDTH_ORDER: readonly DisplayWidth[] = ['desktop', 'tablet', 'mobile']

/**
 * The lines of every row (and the heading) at this width, by a fit test on the
 * measured width (review §7.1-4): one line when its columns' mins fit; else the
 * first author arrangement, widest first, whose lines fit, each line shared by
 * content; else the automatic lines. Never wider than `width`.
 */
export function tableLines(input: {
    columns: readonly ResultColumn[]
    width: number
    layouts: readonly DisplayLayout[]
    /** How deep the report's rows go (rowLevelsOf); absent: a flat report. */
    levels?: RowLevels
    /** The run's measure (measureColumns); absent: the headings and worst cases alone. */
    measures?: ColumnMeasures
}): LineLayout {
    const { columns, width, layouts } = input
    const levels = input.levels ?? FLAT_LEVELS
    const measures =
        input.measures ?? measureColumns({ columns, rows: [], format: DEFAULT_DISPLAY, levels })
    const widths = fitOneLine(columns, width, measures)
    if (widths) return { kind: 'one-line', widths }
    const indent = continuationIndent(levels)
    let notice: string | undefined
    for (const name of WIDTH_ORDER) {
        const layout = layouts.find(l => l.width === name)
        if (!layout) continue
        const authored = authoredLines(layout, columns)
        if (authored.kind === 'refused') {
            notice = notice ?? authored.notice
            continue
        }
        const root = fitRoot(authored.root, width, measures, indent)
        if (root) return { kind: 'blocks', root, indent }
    }
    const auto = autoLines(columns, width, measures, indent)
    return notice ? { ...auto, notice } : auto
}
```

`rowLayout` (482-490) and `pruned` (458-473) keep their logic; `pruned`'s side branch keeps each part's `width` (`{ ...part, block: … }`), and the `labelOnly` results carry `indent: layout.indent`. `oneLineLabelWidth`'s `{ kind: 'blocks', root: label }` test literal gains `indent: 0`.

- [ ] **Step 4: Run the unit tests**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts`
Expected: PASS.

- [ ] **Step 5: Draw widths and the indent**

`core/reports/components/ReportRow.tsx`:

```tsx
// A part's px width from the fit; a hand-built tree with none takes its share of the tracks.
function partStyle(part: SidePart, of: number) {
    return part.width === undefined ? { width: `${(part.tracks / of) * 100}%` } : { width: part.width }
}
```

`SideBlock` (86-101) uses `style={partStyle(part, of)}` and its parts type is `SidePart[]`.

```tsx
const lineStyle = (index: number, indent: number) => (index === 0 ? undefined : { paddingLeft: indent })

/**
 * A row's (or the heading's) whole tree: the lines after the first of a stack root
 * start at the indent, under a detail row's label text, as the fit sized them.
 */
export function RootBlockView({ root, indent, renderCell }: { root: Block; indent: number; renderCell: RenderCell }) {
    if (root.kind !== 'stack') return <BlockView block={root} renderCell={renderCell} />
    const lines = root.parts.map((part, i) => (
        <View key={blockKey(part, i)} style={lineStyle(i, indent)}>
            <BlockView block={part} renderCell={renderCell} />
        </View>
    ))
    return <View className="grow">{lines}</View>
}
```

`BlocksRow` (363-367) renders `<RootBlockView root={lines.root} indent={lines.indent} renderCell={renderCell} />`. In `ReportTable.tsx` `BlocksHeading` (207-211) does the same with `layout.root` / `layout.indent`, so headings stay over their values.

`core/reports/hooks/useTableLayout.ts`: drop `breakpoint` from `TableLayoutModel`, the destructure and the `tableLines` call and deps.

- [ ] **Step 6: Builder copy, help and docs**

`ArrangeSection.tsx:94-97` hint becomes:

```ts
        // The preview uses an arrangement only where one line does not fit its width.
        hint: `Used when the columns do not fit on one line and this arrangement fits the screen. Each line gives its columns the room their content needs.`,
```

(`breakpoint` and `WIDTH_NAMES` stay only if still used elsewhere in the file; remove them if not.) The comment at 355-357 becomes: "With no arrangement that fits, the rows wrap by themselves: as many columns as fit on each line." Update `AutoCaption`'s text to "Off: the columns fit on one line when they can. When they do not, the rows wrap by themselves, or use an arrangement of another width that fits."

`core/help/reports.md:119-124` ("When columns do not fit") becomes:

```markdown
## When columns do not fit

A report never scrolls sideways. Each column gets the room its heading and its values need. Numbers, amounts, percents, dates and yes/no values are never cut short, and short text such as an entry number is not either. Long text gets narrower first and ends in "…". Spare room goes to the columns whose text is cut, then to the text columns. A column with no value is as wide as its heading.

When the columns do not fit on one line, the report uses the first arrangement that fits the screen, widest first: **Desktop**, then **Tablet**, then **Phone**. With no arrangement that fits, each row wraps by itself: as many columns as fit go on each line, the label first, and the next lines start under the row's label.

To choose where each column goes when one line does not fit, open **Arrange**, pick **Phone**, **Tablet** or **Desktop**, and turn on **Arrange columns for this width**. Inside a line, each column gets the room its content needs; a column that spans more tracks gets more of the spare room. The heading row and the open group rows stay at the top while you scroll. A long report loads more rows as you scroll, also when it has groups.
```

`docs/packages.md:1337` row becomes:

```markdown
| `display.layouts[]` | `{ width, tracks, cells: [{ col, line, start?, span?, lines?, justify? }] }`: an arrangement of the columns on lines. The view uses one line when its columns fit (each column sized by its heading and values), else the first arrangement whose lines fit, widest `width` first, else automatic lines. Inside a line each column gets its content's width; `span` is its share of the spare room. |
```

- [ ] **Step 7: Typecheck, unit tests**

Run: `pnpm exec tinycld-pkg typecheck` and `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`. Expected: PASS.

- [ ] **Step 8: Update the e2e this task changes**

`tests/e2e/reports-wrap.spec.ts`:
- `on a phone a wide row wraps under aligned wrapped headings…` (148-193): the automatic lines now put the figures that fit beside the label. The last column (`status`, added for grouping) cannot share line 1 when one line does not fit. In `boxes()` add `status: await box(`report-cell-status-${id}`)` and `headStatus: await box('report-heading-status')`; in the poll replace `wrapped` with `b.status.y >= b.label.y + b.label.height - 1`, add `statusUnderHeading: Math.abs((b.headStatus?.x ?? 0) - b.status.x) <= 1`, and replace `headingsWrapped` with `(b.headStatus?.y ?? 0) > (b.headLabel?.y ?? 0)`. Update the file comment (22-29): "on a phone a row too wide for one line wraps: as many columns as fit on each line, each under its heading".
- `the next section pushes the whole stuck stack…` (324-391), phone part (377-386): same change, using `quantity` → the last column (`booked`) for the "wrapped" check.
- `on a phone a column left blank on opened rows keeps its empty slot` (668-714): replace the "figures below the label" poll (692-698) with: the blank `formula` slot is under its heading (`x` within 1) and on the same line as some figure of its row (`y` equal to the `amount` cell's or the `cost` cell's within 1). Keep the text and scroll-width checks.

`tests/e2e/reports-ledger-v3.spec.ts:447-488` (the Task 4 version at 820) asserts only wrapping, headings inside the table and no sideways scroll, so it holds whichever arrangement fits. `on a phone an opening row…` (490-520) may now draw the team-balance report with its own phone arrangement or the automatic lines; Task 6 rewrites its gap check, and here it must still pass as is (the opening row is still its label line, then its figure).

- [ ] **Step 9: Run the e2e this task touches**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts` and `… -- reports-ledger-v3.spec.ts`
Expected: PASS. `an author arrangement for phones puts a cell spanning two lines in the middle of the row` (222-281) must pass unchanged: `wideReport` has eight columns, so one line cannot fit at 390 by any measure, and its three-track arrangement fits.

- [ ] **Step 10: Lint and commit**

```bash
git add core/reports/lib/table-lines.ts core/reports/components/ReportRow.tsx core/reports/components/ReportTable.tsx core/reports/hooks/useTableLayout.ts core/reports/components/builder/ArrangeSection.tsx core/help/reports.md docs/packages.md core/reports/lib/__tests__/table-lines.test.ts tests/e2e/reports-wrap.spec.ts tests/e2e/reports-ledger-v3.spec.ts
git commit -m "reports: choose the layout by fit and fit each line by content"
```

E2E this task breaks (fixed in Step 8): `reports-wrap.spec.ts:158-177`, `:377-386`, `:692-698`; re-check `reports-ledger-v3.spec.ts:451-488`, `reports-wrap.spec.ts:222-281`. Books (after merge): `general-ledger.spec.ts:873-912` if one line fits the view at 900.

---

### Task 6: Every non-detail row spans its empty cells; H7

**Files:**
- Modify: `core/reports/lib/table-lines.ts:455-502` (`pruned`, `rowLayout`, `oneLineLabelSpan`)
- Modify: `core/reports/components/ReportRow.tsx:333-336` (`BlocksRow` passes the label need)
- Modify: `core/help/reports.md` (one paragraph)
- Test: `core/reports/lib/__tests__/table-lines.test.ts:757-815,857-904`
- Test (e2e): `tests/e2e/reports-wrap.spec.ts` (new test), `tests/e2e/reports-ledger-v3.spec.ts:490-520`

**Interfaces:**
- Consumes: `SidePart` widths and `indent` (Task 5); `labelNeed(item, format)` (Task 2).
- Produces: `oneLineLabelSpan(item, columnIds)` spans for every non-detail kind; `rowLayout(item: VisibleRow, layout: LineLayout, labelId: string, need: number): LineLayout`.

Rules (review §7.1-2, H7): on one line, a group, opening, subtotal, computed or total row's label spans the empty cells up to its first value (all of them when it has none). In blocks, after pruning, the label cell takes the empty parts right of it on its line (their tracks and widths add to its own); a side block left with one part becomes that part's block. H7: when a non-detail row's pruned root is a stack whose first line is the label alone and whose second line is a side block starting with empty parts, and those parts' widths plus the indent are at least `need`, the label moves into them: the row becomes that one line, the label part's width is the empty parts' widths plus the indent (it is now the row's first line, drawn without the indent), and the label is its first part.

- [ ] **Step 1: Write the failing tests**

In `table-lines.test.ts`:
- `spans no column of a detail row, or of any other row kind` (885-889) becomes:

```ts
    it('spans every non-detail row up to its first value, and no detail row', () => {
        expect(oneLineLabelSpan(item('detail', {}), others)).toBe(0)
        expect(oneLineLabelSpan(item('subtotal', { credit: 1 }), others)).toBe(2)
        expect(oneLineLabelSpan(item('opening', { credit: 1 }), others)).toBe(2)
        expect(oneLineLabelSpan(item('total', { debit: 5, credit: 1 }), others)).toBe(1)
        expect(oneLineLabelSpan(item('computed', {}), others)).toBe(3)
    })
```

- In `figure-only rows in a wrapped layout` (757-815), with `layout` built at width 390 with `measures: uniform(COLUMNS, 110)` and the `PHONE` arrangement, replace the two pruning tests:

```ts
    it('lets the label take the empty tracks of its line', () => {
        const cells = { amount: 900, cost: 300, balance: null }
        const row = rowLayout(item('subtotal', cells), layout, 'customer', 999)
        if (row.kind !== 'blocks' || row.root.kind !== 'stack') throw new Error('expected lines')
        // Line 1 is the label alone, across the row; the blank balance keeps its track.
        expect(row.root.parts[0]).toEqual(label)
        expect(shapeOf({ ...row, root: row.root.parts[1] as Block })).toMatchObject({
            root: { kind: 'side', of: 3, parts: [{ tracks: 1 }, { tracks: 1 }, { block: { kind: 'gap' } }] },
        })
    })

    it('puts an opening row’s label beside its figure when the empty tracks hold it (H7)', () => {
        const row = rowLayout(item('opening', { balance: 1200 }), layout, 'customer', 150)
        if (row.kind !== 'blocks' || row.root.kind !== 'side') throw new Error('expected one line')
        const [labelPart, figure] = row.root.parts
        expect(labelPart?.block).toEqual(label)
        expect(labelPart?.tracks).toBe(2)
        expect(figure?.block).toEqual(money('balance'))
        // The two empty tracks and the indent: the line keeps the row's width.
        const line = (layout.kind === 'blocks' && layout.root.kind === 'stack' ? layout.root.parts.at(-1) : null) as Extract<Block, { kind: 'side' }>
        expect(labelPart?.width).toBe((line.parts[0]?.width ?? 0) + (line.parts[1]?.width ?? 0) + layout.indent)
    })

    it('keeps the label on its own line when the empty tracks are too narrow', () => {
        const row = rowLayout(item('opening', { balance: 1200 }), layout, 'customer', 10_000)
        if (row.kind !== 'blocks') throw new Error('expected blocks')
        expect(row.root.kind).toBe('stack')
    })
```

  Every other `rowLayout(…)` call in the file gains a fourth argument `0` (detail rows ignore it).

- [ ] **Step 2: Run test to verify it fails**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/table-lines.test.ts`
Expected: FAIL — spans are 0 for non-group rows; `rowLayout` keeps the label in its track.

- [ ] **Step 3: Write the code**

```ts
const isLabelCell = (block: Block, labelId: string) => block.kind === 'cell' && block.col === labelId

// The label takes the empty parts right of it on its line: their tracks and widths
// become its own (review §7.1-2). A side block left with one part is that block.
function labelSpans(block: Extract<Block, { kind: 'side' }>, labelId: string): Block {
    const parts: SidePart[] = []
    for (const part of block.parts) {
        const last = parts[parts.length - 1]
        if (last && isLabelCell(last.block, labelId) && part.block.kind === 'gap') {
            parts[parts.length - 1] = {
                ...last,
                tracks: last.tracks + part.tracks,
                width: last.width === undefined || part.width === undefined ? undefined : last.width + part.width,
            }
            continue
        }
        parts.push(part)
    }
    const [only] = parts
    return parts.length === 1 && only ? only.block : { ...block, parts }
}
```

`pruned(block, keeps, labelId)` gains `labelId`; its side branch returns `labelSpans({ ...block, parts }, labelId)` when a cell is kept.

```ts
// H7: a row whose first line is its label alone moves the label into the empty
// parts that start its next line, when they hold the whole label. That line becomes
// the row's first, drawn without the indent, so the label part also takes the indent.
function labelOntoFigures(root: Block, labelId: string, need: number, indent: number): Block {
    if (root.kind !== 'stack') return root
    const [first, next, ...rest] = root.parts
    if (!first || !isLabelCell(first, labelId) || next?.kind !== 'side') return root
    let room = indent
    let tracks = 0
    let count = 0
    for (const part of next.parts) {
        if (part.block.kind !== 'gap' || part.width === undefined) break
        room += part.width
        tracks += part.tracks
        count++
    }
    if (count === 0 || room < need) return root
    const line: Block = {
        ...next,
        parts: [{ block: first, tracks, width: room }, ...next.parts.slice(count)],
    }
    return rest.length === 0 ? line : stackOf([line, ...rest])
}
```

`rowLayout(item, layout, labelId, need)`:

```ts
export function rowLayout(item: VisibleRow, layout: LineLayout, labelId: string, need: number): LineLayout {
    if (layout.kind === 'one-line') return layout
    const labelOnly: Block = { kind: 'cell', col: labelId, justify: 'start' }
    if (Object.keys(item.cells).length === 0) return { ...layout, root: labelOnly }
    if (item.row.kind === 'detail') return layout
    const keeps = (col: string) => col === labelId || hasValue(item.cells[col])
    if (blockColumns(layout.root).every(keeps)) return layout
    const root = pruned(layout.root, keeps, labelId) ?? labelOnly
    return { ...layout, root: labelOntoFigures(root, labelId, need, layout.indent) }
}
```

(When `labelOntoFigures` returns a lone side line followed by `rest`, `RootBlockView` indents `rest` after the new first line, as before.)

`oneLineLabelSpan` (498-502):

```ts
/**
 * How many columns after the label a row's label spans on one line: a non-detail
 * row (group, opening, subtotal, computed, total) spans its empty cells up to its
 * first value, every column when it has none; a detail row spans none, so its label
 * ends in "…" at the label column's width and every cell keeps its column.
 */
export function oneLineLabelSpan(item: VisibleRow, columnIds: readonly string[]): number {
    if (item.row.kind === 'detail') return 0
    const first = columnIds.findIndex(id => hasValue(item.cells[id]))
    return first === -1 ? columnIds.length : first
}
```

`ReportRow.tsx` `BlocksRow` (333-336): `const lines = rowLayout(item, layout, label?.id ?? '', labelNeed(item, format))` (import `labelNeed` from `'../lib/column-measure'`).

`core/help/reports.md`, after the first paragraph of "When columns do not fit", add: "Group, opening, subtotal and total rows show their whole label: it runs across the empty cells after it. When the rows wrap, a total's label moves onto the line of its figures if there is room."

- [ ] **Step 4: Run the unit tests**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`
Expected: PASS.

- [ ] **Step 5: Write the e2e**

Add to `tests/e2e/reports-wrap.spec.ts`, inside `test.describe('report rows per width', …)`:

```ts
    // Review §7.1-2 and H7: a subtotal row's label spans its empty cells on one line,
    // and on a phone it moves beside its figure when the empty tracks hold it.
    test('a subtotal row shows its whole label on a desktop line and beside its figure on a phone', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptsubspan')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'Ada', amount: 1000, cost: 400, quantity: 3, day: '2026-01-05' })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['quantity'])
            await addColumn(p, ['cost'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-3-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'customer')
            await p.getByTestId('report-builder-group-0-open').click()
            // Phone: the label across line 1, then quantity · cost · amount.
            await p.getByTestId('report-builder-section-arrange').click()
            await p.getByTestId('report-builder-arrange-width-mobile').click()
            await p.getByTestId('report-builder-arrange-on').click()
            await chooseOption(p, 'report-builder-arrange-tracks', '3')
            await chooseOption(p, 'report-builder-arrange-customer-span', '3')
            await chooseOption(p, 'report-builder-arrange-quantity-line', '2')
            await chooseOption(p, 'report-builder-arrange-cost-line', '2')
            await chooseOption(p, 'report-builder-arrange-amount-line', '2')
            await saveNewReport(p, uniqueName('Subtotal span'))

            const subtotal = rowOf(p, 'Total Ada')
            const amount = await cellOf(p, 'amount', 'Total Ada')
            const isCut = () =>
                subtotal
                    .getByText('Total Ada', { exact: true })
                    .evaluate(el => el.scrollWidth > el.clientWidth)
            await p.setViewportSize({ width: 1280, height: 800 })
            await expect(p.getByTestId('report-table')).toHaveCSS('opacity', '1')
            await expect.poll(isCut).toBe(false)

            await p.setViewportSize({ width: 390, height: 844 })
            await expect
                .poll(async () => {
                    const label = await subtotal.boundingBox()
                    const figure = await amount.boundingBox()
                    return label && figure ? Math.abs(label.y - figure.y) <= 1 : false
                })
                .toBe(true)
            await expect.poll(isCut).toBe(false)
        } finally {
            await close()
        }
    })
```

The engine labels a subtotal `Total <group label>` (`core/server/reports/assemble.go:542`). The short name keeps the H7 room check safe at 390: line 2 is 370 wide, its empty tracks and indent give about 264 px, and "Total Ada" needs about 16 + 16 + 76 + 12 = 120 px.

`tests/e2e/reports-ledger-v3.spec.ts` `on a phone an opening row is its label and then its figures…` (490-520): the opening row may now share its figure's line (H7). Rename `on a phone an opening row shows its whole label with its figures right after it, and a heading still sorts`; replace `gapFits` with `figuresFollow: figures.y >= label.y - 1 && figures.y <= label.y + label.height + 8` and add `labelWhole` (the label text's `scrollWidth <= clientWidth`) to the snapshot.

- [ ] **Step 6: Run the e2e**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts` and `… -- reports-ledger-v3.spec.ts`
Expected: PASS (`on a desktop line a closed group row spans its label up to its first figure…`, 556-597, unchanged).

- [ ] **Step 7: Lint and commit**

```bash
git add core/reports/lib/table-lines.ts core/reports/components/ReportRow.tsx core/help/reports.md core/reports/lib/__tests__/table-lines.test.ts tests/e2e/reports-wrap.spec.ts tests/e2e/reports-ledger-v3.spec.ts
git commit -m "reports: every non-detail row spans its empty cells"
```

E2E this task breaks (fixed in Step 5): `reports-ledger-v3.spec.ts:506-515`. Books (after merge): `general-ledger.spec.ts:863-864` (`opening` and `sectionTotal` become `lines: 1` when the label fits the debit and credit tracks).

---

### Task 7: H3 — Sort from a menu when the rows wrap

**Files:**
- Modify: `core/reports/stores/report-ui-store.ts:9` (`ReportSurface` adds `{ kind: 'sort' }`)
- Modify: `core/reports/hooks/useReportView.ts:328-336` (`onSetSort`)
- Modify: `core/reports/lib/view-url.ts` (add `sortChoices`, `sortChipText`)
- Modify: `core/reports/components/ReportControls.tsx:153-175` (Sort chip and menu)
- Modify: `core/reports/components/ReportView.tsx:168-193` (passes `isSortMenuShown`)
- Modify: `core/reports/components/ReportTable.tsx:96-161,196-212` (blocks headings are not buttons)
- Modify: `core/help/reports.md`
- Test: `core/reports/lib/__tests__/view-url.test.ts`
- Test (e2e): `tests/e2e/reports-ledger-v3.spec.ts:517-519`

**Interfaces:**
- Consumes: `TableLayout.layout` (Task 4); `nextSort`, `encodeSort`, `ViewSort` (`view-url.ts:119,197`, `run-client.ts:19`); `headingText` (`format.ts:230`); `Menu` (`@tinycld/core/ui/menu`).
- Produces:
  - `type SortChoice = { key: string; label: string; sort: ViewSort | null }`; `sortChoices(columns: readonly ResultColumn[], format: NumberFormat): SortChoice[]` — "Report order" first, then per sortable column "<heading>, ascending" and "<heading>, descending"; `sortChipText(columns: readonly ResultColumn[], format: NumberFormat, sort: ViewSort | null): string` — "Sort" or "Sort: <heading> ↑/↓".
  - model `onSetSort?(sort: ViewSort | null): void` (optional: a hand-built model has none; the chip is then not shown).
  - `ReportControls({ model, isSortMenuShown }: { model: ReportViewModel; isSortMenuShown: boolean })`; testIDs `report-sort-menu` (chip), `report-sort-choice-<key>` (items, key `none`, `<col>-asc`, `<col>-desc`).

Rule (D10): when the table's layout is `blocks`, the headings are plain text (a sorted column's heading shows its arrow, not as a button) and the Sort chip shows in the controls row. One-line layouts keep tap-to-sort and show no chip.

- [ ] **Step 1: Write the failing test**

In `core/reports/lib/__tests__/view-url.test.ts` add:

```ts
describe('sort choices', () => {
    const columns: ResultColumn[] = [
        { id: 'customer', label: 'Customer', type: 'text', sortable: true },
        { id: 'balance', label: 'Balance', type: 'money', currency: 'USD' },
        { id: 'amount', label: 'Amount', type: 'money', currency: 'USD', sortable: true },
    ]

    it('offers the report order, then each sortable column both ways', () => {
        expect(sortChoices(columns, DEFAULT_DISPLAY)).toEqual([
            { key: 'none', label: 'Report order', sort: null },
            { key: 'customer-asc', label: 'Customer, ascending', sort: { col: 'customer', dir: 'asc' } },
            { key: 'customer-desc', label: 'Customer, descending', sort: { col: 'customer', dir: 'desc' } },
            { key: 'amount-asc', label: 'Amount, ascending', sort: { col: 'amount', dir: 'asc' } },
            { key: 'amount-desc', label: 'Amount, descending', sort: { col: 'amount', dir: 'desc' } },
        ])
    })

    it('names the sort in the chip', () => {
        expect(sortChipText(columns, DEFAULT_DISPLAY, null)).toBe('Sort')
        expect(sortChipText(columns, DEFAULT_DISPLAY, { col: 'amount', dir: 'desc' })).toBe('Sort: Amount ↓')
        expect(sortChipText(columns, DEFAULT_DISPLAY, { col: 'gone', dir: 'asc' })).toBe('Sort')
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports/lib/__tests__/view-url.test.ts`
Expected: FAIL — `sortChoices` is not exported.

- [ ] **Step 3: Write the lib code**

In `core/reports/lib/view-url.ts`:

```ts
export type SortChoice = { key: string; label: string; sort: ViewSort | null }

/** The Sort menu's choices (D10): the report's own order, then each sortable column both ways. */
export function sortChoices(columns: readonly ResultColumn[], format: NumberFormat): SortChoice[] {
    const choices: SortChoice[] = [{ key: 'none', label: 'Report order', sort: null }]
    for (const column of columns) {
        if (!column.sortable) continue
        const text = headingText(column, format)
        for (const dir of ['asc', 'desc'] as const) {
            const words = dir === 'asc' ? 'ascending' : 'descending'
            choices.push({ key: `${column.id}-${dir}`, label: `${text}, ${words}`, sort: { col: column.id, dir } })
        }
    }
    return choices
}

const SORT_ARROW = { asc: '↑', desc: '↓' } as const

export function sortChipText(columns: readonly ResultColumn[], format: NumberFormat, sort: ViewSort | null): string {
    const column = sort ? columns.find(c => c.id === sort.col) : undefined
    if (!sort || !column) return 'Sort'
    return `Sort: ${headingText(column, format)} ${SORT_ARROW[sort.dir]}`
}
```

Run Step 2's command: PASS.

- [ ] **Step 4: Model, store and chip**

`report-ui-store.ts:9`: `export type ReportSurface = { kind: 'param'; paramId: string } | { kind: 'add-filter' } | { kind: 'sort' }`.

`useReportView.ts`, beside `onClearSort` (334): `onSetSort: (sort: ViewSort | null) => { write({ ...options, sort }); closeSurface() },`. `useReportView.ts:341` is `export type ReportViewModel = ReturnType<typeof useReportView>`; change it to `export type ReportViewModel = Omit<ReturnType<typeof useReportView>, 'onSetSort'> & { onSetSort?(sort: ViewSort | null): void }`, so Books' hand-built model (`books/tinycld/books/screens/helix-mockup.tsx`), which has no `onSetSort`, still compiles.

`ReportControls.tsx`:

```tsx
// An element, not a component: the Menu clones its trigger.
function sortChip(text: string, iconColor: string) {
    return (
        <Pressable
            testID="report-sort-menu"
            accessibilityRole="button"
            accessibilityLabel={text}
            className="flex-row items-center gap-1.5 rounded-full border border-border bg-secondary px-3"
            style={CHIP_STYLE}
        >
            <ArrowUpDown size={14} color={iconColor} />
            <Text numberOfLines={1} className="text-sm text-secondary-foreground">
                {text}
            </Text>
        </Pressable>
    )
}

// D10: when the rows wrap, the headings are not buttons; the reader sorts here.
function SortControl({ model, isVisible }: { model: ReportViewModel; isVisible: boolean }) {
    const muted = useThemeColor('muted-foreground')
    const onSetSort = model.onSetSort
    if (!isVisible || !onSetSort) return null
    const trigger = sortChip(sortChipText(model.columns, model.display, model.sort), muted)
    const onOpenChange = (open: boolean) => {
        if (open) model.onOpenSurface({ kind: 'sort' })
        else model.onCloseSurfaceOf('sort')
    }
    const items = sortChoices(model.columns, model.display).map(choice => (
        <Menu.Item
            key={choice.key}
            label={choice.label}
            testID={`report-sort-choice-${choice.key}`}
            onSelect={() => onSetSort(choice.sort)}
        />
    ))
    return (
        <Menu trigger={trigger} isOpen={model.surface?.kind === 'sort'} onOpenChange={onOpenChange} title="Sort" sheetSide="bottom" testID="report-sort-menu-list">
            {items}
        </Menu>
    )
}
```

`ReportControls({ model, isSortMenuShown })`: return null only when `model.controls.length === 0 && !isSortMenuShown`; render `<SortControl model={model} isVisible={isSortMenuShown} />` after `{controls}`. Import `ArrowUpDown` from `lucide-react-native` and `sortChipText`, `sortChoices` from `'../lib/view-url'`.

`ReportView.tsx`: `<ReportControls model={model} isSortMenuShown={table.layout.kind === 'blocks'} />` (compute `const isSortMenuShown = table.layout.kind === 'blocks'` above the return).

- [ ] **Step 5: Plain headings in blocks**

`ReportTable.tsx`: `HeadingCell` and `LabelHeading` take `isPressable: boolean`; `HeadingContent` returns plain text when `!column.sortable || !isPressable`, with `<SortIcon kind={kind} isVisible={kind !== 'none'} />` beside it so a sorted column still shows its arrow (icon first when `justify === 'end'`, as now). `OneLineHeading` passes `isPressable`; `BlocksHeading` passes `isPressable={false}`. The heading measure keeps `SORT_ICON_ROOM` for every sortable column, so the arrow always has room.

- [ ] **Step 6: Help**

`core/help/reports.md`, in "Run a report", add a bullet: "- To sort, press a column heading: each press sorts up, then down, then back to the report's order. When the rows wrap (on a phone), use the **Sort** chip above the report instead."

- [ ] **Step 7: Update the e2e this task changes and add one**

`tests/e2e/reports-ledger-v3.spec.ts:517-519` (phone): replace the heading click with

```ts
        await page.getByTestId('report-sort-menu').click()
        await page.getByTestId('report-sort-choice-amount-asc').click()
```

  and rename the test `… and the Sort chip sorts`. Add after line 519: `await expect(page.getByTestId('report-sort-amount')).toHaveCount(0)` (no heading button in blocks) and `await expect(page.getByTestId('report-sort-menu')).toContainText('Sort: Amount ↑')`.

In the same file, in `a heading sorts the rows inside their groups, and a copied link keeps it` (366-409, desktop): add `await expect(page.getByTestId('report-sort-menu')).toHaveCount(0)` after the first sort press (one line keeps tap-to-sort, no chip).

- [ ] **Step 8: Run tests**

Run: `pnpm exec tinycld-pkg typecheck`; `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`; `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-ledger-v3.spec.ts`
Expected: PASS.

- [ ] **Step 9: Lint and commit**

```bash
git add core/reports/stores/report-ui-store.ts core/reports/hooks/useReportView.ts core/reports/lib/view-url.ts core/reports/components/ReportControls.tsx core/reports/components/ReportView.tsx core/reports/components/ReportTable.tsx core/help/reports.md core/reports/lib/__tests__/view-url.test.ts tests/e2e/reports-ledger-v3.spec.ts
git commit -m "reports: sort from a menu when the rows wrap"
```

E2E this task breaks (fixed in Step 7): `reports-ledger-v3.spec.ts:517-519`. Books: none (its sort presses are at desktop width).

---

### Task 8: Line polish — compact continuation and heading lines, 2-line headings, tabular figures, label heading offset

**Files:**
- Modify: `core/reports/components/report-table-layout.ts` (add `CONTINUATION_LINE_HEIGHT = 28`, `HEADING_LINE_HEIGHT = 24`)
- Modify: `core/reports/components/ReportRow.tsx:39-111,204-269,333-368` (`RenderCell` gains `minHeight`; `BlockView`/`RootBlockView` heights; `cellBoxStyle(width, minHeight)`; tabular figures)
- Modify: `core/reports/components/ReportTable.tsx:78-87,121-161,196-212` (2-line headings; no `pl-1`; compact blocks heading)
- Modify: `core/help/reports.md`
- Test (e2e): `tests/e2e/reports-wrap.spec.ts:148-193`

**Interfaces:**
- Consumes: `RootBlockView` (Task 5); `isFigureColumn` (Task 2); `isPressable` headings (Task 7).
- Produces: `CONTINUATION_LINE_HEIGHT = 28`, `HEADING_LINE_HEIGHT = 24`; `type RenderCell = (col: string, justify: Justify, minHeight: number) => ReactNode`; `cellBoxStyle(width: number | undefined, minHeight: number)`; `BlockView({ block, renderCell, lineHeight })`; `RootBlockView({ root, indent, renderCell, firstHeight, restHeight })`.

Rules (D6, H3, H4, review §7.1-6/7, §11.5-8): in blocks, a row's first line is 44 px; the lines after it are 28 px, except on a group row (a toggle: every line is its press target), which keeps 44; a gap is `lines × lineHeight`. Blocks heading lines are 24 px (not buttons after Task 7). One-line rows and headings keep 44. Headings use up to 2 lines (`numberOfLines={2}`); the measure already holds a heading's longest word. Every figure cell (`isFigureColumn`) uses `tabular-nums`. The label heading loses its `pl-1`, so its text starts where the other headings' do.

- [ ] **Step 1: Write the failing e2e**

In `tests/e2e/reports-wrap.spec.ts` › `on a phone a wide row wraps…` (148-193), after the alignment poll add:

```ts
            // D6: the wrapped line is shorter than the first (not a press target), and it
            // starts under the label's text (review §7.1-6).
            const id = (await rowOf(p, 'Wrapco 00').getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
            const status = p.getByTestId(`report-cell-status-${id}`)
            const labelText = rowOf(p, 'Wrapco 00').getByText('Wrapco 00', { exact: true })
            await expect
                .poll(async () => {
                    const box = await status.boundingBox()
                    const text = await status.locator('div[dir="auto"]').boundingBox()
                    const label = await labelText.boundingBox()
                    if (!box || !text || !label) return null
                    return { compact: box.height <= 30, underLabel: Math.abs(text.x - label.x) <= 1 }
                })
                .toEqual({ compact: true, underLabel: true })
            const headingRow = await p.getByTestId('report-heading-row').boundingBox()
            expect(headingRow?.height ?? 0).toBeLessThan(2 * 44)
```

- [ ] **Step 2: Run it to verify it fails**

Run: `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts -g "wraps under aligned"`
Expected: FAIL — the status cell is 44 px tall; the heading row is 88 px.

- [ ] **Step 3: Write the code**

`report-table-layout.ts`:

```ts
/** A wrapped row's later lines: not a press target, so shorter than ROW_HEIGHT (D6). */
export const CONTINUATION_LINE_HEIGHT = 28
/** A heading line when the rows wrap: plain text, sorted from the Sort chip (D10). */
export const HEADING_LINE_HEIGHT = 24
```

`ReportRow.tsx`:
- `export type RenderCell = (col: string, justify: Justify, minHeight: number) => ReactNode`
- `BlockView({ block, renderCell, lineHeight })`: gap `minHeight: block.lines * lineHeight`; `CellSlot` calls `renderCell(block.col, block.justify, lineHeight)`; `StackBlock`/`SideBlock` pass `lineHeight` down.
- `RootBlockView({ root, indent, renderCell, firstHeight, restHeight })`: line 0 at `firstHeight`, later lines at `restHeight`; a root that is not a stack draws at `firstHeight`.
- `cellBoxStyle(width: number | undefined, minHeight: number)` returns `{ width, minHeight, flexGrow: width === undefined ? 1 : 0 }`; `RowCell` and `RowLabel` take `minHeight` (default `ROW_HEIGHT`) and `labelStyle(row, justify, minHeight)` uses it.
- `cellTextClass(display, justify, kind, isFigure)` adds `tabular-nums` when `isFigure`; `RowCell` passes `isFigureColumn(column)`. `TEXT_ALIGN.end` drops its own `tabular-nums`.
- `BlocksRow`: `const restHeight = item.isToggle ? ROW_HEIGHT : CONTINUATION_LINE_HEIGHT` and `<RootBlockView … firstHeight={ROW_HEIGHT} restHeight={restHeight} />`; its `renderCell` passes `minHeight` to `RowLabel`/`RowCell`.
- `OneLineRow` passes `ROW_HEIGHT` to `RowCell`.

`ReportTable.tsx`:
- `HeadingText`: `numberOfLines={2}`.
- `LabelHeading`: `<View className="grow">` (no `pl-1`).
- `HeadingCell` takes `minHeight`; `OneLineHeading` passes `ROW_HEIGHT`; `BlocksHeading` renders `<RootBlockView root={layout.root} indent={layout.indent} renderCell={renderCell} firstHeight={HEADING_LINE_HEIGHT} restHeight={HEADING_LINE_HEIGHT} />` and its `renderCell` passes the `minHeight` it gets.
- `HeadingContent`'s pressable keeps `min-h-11` (one line only, Task 7).

`core/help/reports.md`, in "When columns do not fit", add: "A column heading that is too long for its column uses two lines. When the rows wrap, the lines after a row's first line are shorter, so more rows fit on the screen."

- [ ] **Step 4: Run tests**

Run: `pnpm exec tinycld-pkg typecheck`; `/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/vitest run core/reports`; `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts` and `… -- reports-ledger-v3.spec.ts`
Expected: PASS (the sticky-stack tests compare heights relatively; `on a desktop a wide table fits…` counts heading cell tops, which stay one per cell).

- [ ] **Step 5: Lint and commit**

```bash
git add core/reports/components/report-table-layout.ts core/reports/components/ReportRow.tsx core/reports/components/ReportTable.tsx core/help/reports.md tests/e2e/reports-wrap.spec.ts
git commit -m "reports: compact wrapped lines and two-line headings"
```

E2E this task breaks: none expected besides the new assertions. Books: `general-ledger.spec.ts:858-862` (`eachLineOneTop` stays true; re-check after merge).

---

## Self-review notes

- Spec coverage (part 1): review §7.1 items 1–7 → Tasks 2–6, 8; D1 → Tasks 2–4; D3 → Task 2; D5 → Task 5; D6 → Task 8; D10/H3 → Task 7; H4 → Tasks 2, 8; H7 → Task 6; §11.5-8 (`pl-1`) → Task 8; pivot levels bug → Task 4; help/docs → Tasks 5–8. The four-width acceptance e2e and H1, H2, H5, H6, D9, D11, D12 → part 2 (Tasks 9–13, outline).
- Type names used across tasks: `ColumnMeasure`, `ColumnMeasures`, `measureColumns`, `measureOf`, `shareWidth`, `fitOneLine`, `SidePart`, `continuationIndent`, `labelNeed`, `labelChrome`, `RootBlockView`, `TableLayout`, `useTableLayout`, `shownRunKey`, `sortChoices`, `sortChipText` — each defined in the task named in its Interfaces block.

## Open questions for the human

**Answered 2026-11-07: the human approved the plan and accepted all five as written (1 accept, 2 accept the conservative table, 3 five-digit floor, 4 accept, 5 accept).**

1. **One line beats an authored arrangement.** With the fit test, an author's Phone or Tablet arrangement is used only when one line does not fit the view (review §7.1-4). A report author who wants the arrangement even where one line fits loses it. Accept, or keep "an arrangement for this screen always applies"?
2. **Width table is conservative.** The estimate takes the widest of SF, Arial/Liberation and Roboto, so on a Mac columns are up to ~15–20 % wider than the text. Fewer reports fit one line on a phone (the Trial Balance at 390 is near the edge). Accept, or calibrate per platform (one table per platform font)?
3. **Figure worst case.** "Fixed worst cases for figures" (D1) is read as: an amount column always holds −99,999.99 and a number −99,999 (in the reader's format), even when the first page's values are smaller. Is five digits the right floor?
4. **Continuation indent.** All wrapped lines (and the wrapped heading) start under a *detail* row's label text, the same for every row so the columns stay aligned; a group row's wrapped figures therefore do not follow the group's own indent. Accept?
5. **Sort chip placement.** The Sort chip sits in the chips row, which scrolls away with the top area on a phone (D9, part 2): to change the sort the reader scrolls back up. Accept, or pin it with the column headings?

## Risks

- The character table under-estimates a font we have not measured (a user-set system font, Android OEM fonts): a figure could be cut there. The calibration e2e guards CI and the dev machine only.
- Measure taken in render (`useRunMeasures` sets state while rendering when the key changes) causes one extra render per new run; a key that changes every render (an unstable value in `measureKey`) would loop. The key is a string of stable parts; Task 3's `measureKey` tests pin it.
- A report whose groups start closed is measured from its group rows first, then once more when the reader opens the first group (`hasDetail` in the key): the columns move once, then stay.
- Part 2's H1 removes the Total from the merged rows while pages remain; the measure must still read page 1's whole-report rows or the Total column width is lost (outline, Task 10 constraint).
- Books e2e (`general-ledger.spec.ts:821-912`) changes shape after Tasks 4–5 and must be updated in the Books PR that follows the core merge.
- `table-lines.test.ts` is rewritten heavily in Tasks 3–5 (about 25 call sites); a reviewer must check that each rewritten test still asserts the behaviour its name states.
