# Report zone times and the record panel — Part 2 (Tasks 7–11)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A detail row whose `open` names a package's registered record panel opens that record in a 480 px panel beside the report on a window of 1360 px or more (`dr=<app path>` in the URL, the pressed row marked), the help and the package docs name the date-time, zone and panel rules, the `salesstub` e2e package ships a date-time report and a record panel, and the core e2e proves T1 and D5 end to end.

**Architecture:** See the outline, `docs/plans/2026-11-15-report-zone-times-and-record-panel.md` (Architecture, Names changed N1–N5, Rulings R1–R19). Part 1 (Tasks 1–6, with 4b) built T3, T1, T2 (with a day typed in a condition zoned too, Q3) and the D5 generator: `lib/generated/report-panels.ts`, the ambient module `@tinycld/app-generated/report-panels`, core's unit-setup mock `{ reportPanels: [] }`, `ReportPanelEntry` in `core/reports/lib/record-panel.ts`, `ReportPanelProps` in `core/reports/hooks/usePanelTitle.ts`, `dateTimeText`, `zoneLine` and `ReportViewModel.zoneWords`. Here: the pure rules (`recordPanelOf`, `rowOpensInPanel`, `recordPanelParams`, `isRowShown`) and the `dr` key (Task 7); the one reader of the generated list (`registered-panels.ts`), the UI-store fields, the row press, the row mark and the record mode of `ReportDrillPanel` (Task 8); help and docs (Task 9); the stub (Task 10); the e2e (Task 11).

**Tech Stack:** TypeScript (React Native + web, expo-router, NativeWind), Zustand, vitest, Playwright; Go only in the stub template.

**Spec:** `/Users/nas/tmp/tinycld/books-reports/books/docs/specs/audit-log-design.md` §4 (D5 row, T1 row), §3 Help (core part). Brief: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-audit-log/cp/cp0.md` ("Tests it must add", Do 3).

Paths are relative to `/Users/nas/tmp/tinycld/books-reports/tinycld` (core repo, branch `books/p5-audit-log`; do not change branches). Vitest, from that directory: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run <file>`. Typecheck, from the same directory: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck`. Biome on your own files only: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check --write <files>` (the code below is not line-wrapped to biome's width; `--write` formats it), then `biome check <files>` with no errors. E2E, from the same directory, one file, after the port check:

```bash
cd /Users/nas/tmp/tinycld/books-reports/tinycld && { lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7220|7221) ' && { echo PORTS-BUSY; exit 1; }; } ; E2E_PORT=7220 E2E_PORT_2=7221 E2E_IMAP_PORT=1220 E2E_IMAP_PORT_2=1221 pnpm exec tinycld-pkg test:e2e -- <name>.spec.ts
```

A run that takes more than about 4 minutes is the driver's: hand back `NEEDS-RUN: <command>` instead of waiting. A failing e2e that points at the server: search `/Users/nas/tmp/tinycld/books-reports/tinycld/tmp/e2e-serve-7220.log` with `command grep -E`; never read it whole.

## Corrections to the outline (all applied to the outline)

Part 2 keeps every name the outline and the brief's Produces list give. These are additions or file changes found while this part was written; the Books plan does not need any of them. Each one is now applied to the outline (Names changed N3, Global constraints, the Task 8 and 11 summaries, File structure), so the outline and this part name the same things. The items stay here as the record of why.

- **C1 — the row label's focus key.** R12 sends the focus back to the opened row's label through `figure-focus`, but `OpenRowLabel` gets no key today and `RowLinks.figureKey` takes a column. Added: `rowFocusKey(scope: ReportUIScope, rowId: string): string` in `core/reports/lib/figure-focus.ts` (`figureKey(scope, '@open', rowId)`; `@` never starts a column id) and `RowLinks.rowKey(row: ResultRow): string`, which `ReportRow.tsx` passes to `OpenRowLabel` as `focusKey`. *Applied to the outline.*
- **C2 — `ReportBackLink` takes a test id.** Its test id is fixed (`report-back`), and the page form's back link must be `report-record-panel-back`. `core/reports/components/ReportHeader.tsx` gains an optional `testID` prop, default `report-back`. *Applied to the outline.*
- **C3 — Open as full screen keeps the record return.** A row push watches the package tab so Back returns to the report (`watchRecordReturn`, spec D2). Open as full screen pushes the same path, so it must watch too. Added: `useOpenRecordScreen(): (path: string) => void` exported from `core/reports/hooks/useRowLinks.ts`; `onRow` and `useRecordPanel`'s `onOpenFull` both call it. *Applied to the outline.*
- **C4 — the stub needs `packages:generate`.** `refreshReportsStub` (run by every e2e serve) re-writes the stub's files but does not generate: a new route (`screens/panel/[id]`) and the manifest's `reportPanels` reach the app only through `packages:generate`. Task 10 runs the scaffolder once (`pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts` from the workspace root), which generates. *Applied to the outline.*
- **C5 — the e2e pins the locale and the space before AM/PM.** `test.use` sets `locale: 'en-US'` beside `timezoneId`, so the device's hour cycle is 12-hour on every machine. Node's ICU writes a narrow no-break space (U+202F) before "PM"; core builds its text with a plain space (R4), so the spec's expected text replaces every run of white space with one space. *Applied to the outline.*
- **C6 — the panel title belongs to its record path (R13).** A first draft cleared the title on every `openRecord`. A second row that opens the same record (many audit rows open one record) leaves `dr` and the `RecordBoundary` key unchanged, so the package's component does not remount and its `usePanelTitle` effect does not run again: the header went blank and the focus landed on an empty heading. The store now keeps `recordTitle: { open, title } | null` (`setRecordTitle(open, title)`); `PanelTitleProvider` passes the panel's `open` to `usePanelTitle`; the header reads `recordTitleOf(recordTitle, dr)`, so a replaced record never shows the previous title and the same record keeps its own. `openRecord` no longer touches the title. *Applied to the outline.*
- **C7 — a failed panel chunk loads again (R14).** `lazyChunk` keeps its first promise for the session, so a chunk that failed once (offline) would fail every later record of that panel until a reload. `registered-panels.ts` wraps each entry in `reloadableChunk(load)` (`{ Component, reload }`); `RecordBoundary` calls `reload` after it catches, when the failed panel has left the screen, so the next record opened in the panel loads the chunk again. *Applied to the outline.*

Clarifications (no name changes): the row mark uses the `aria-selected` prop only, as `DrillFigure` does (React Native maps it to `accessibilityState.selected` on native; react-native-web writes the attribute); when a hand-edited URL holds both `d` and `dr`, the drill panel shows (`d` wins) and the record panel does not; when two registered paths of one package nest (`history`, `history/deep`), `recordPanelOf` takes the longer; an empty segment (`a//b`, a trailing `/`) is no match.

## Global Constraints

- Core names no package: the e2e and the stub use `salesstub`; nothing in `core/` names it (`pnpm run check:core-isolation`).
- `DETAIL_PANEL_MIN_WINDOW` = 1360, `DETAIL_PANEL_WIDTH` = 480 (`@tinycld/core/lib/detail-surface`); one side surface at a time (a drill, a record or Customize); `dr` never in `reportLink`.
- The panel switches between beside and page by style values only (no remount); Escape closes it (`LayerEscape`, standing approval).
- Native and web: every change runs on both. No new web-only or native-only item.
- Test first in every task: the task's tests are written, run and seen to fail for the named reason before any implementation.
- Biome (4 spaces, single quotes), no `any`, no biome-ignore, no `console.*`; comments say why; JSX minimal (logic in hooks); no `useState` + `useEffect` sync. Light and dark: tokens only (`bg-accent`, `bg-background`, `text-foreground`, `text-muted-foreground`).
- E2E: `test.describe('reports: zone times and record panel', …)`, `test.use({ timezoneId: 'America/Chicago', locale: 'en-US' })`; data through the stub UI (`addTeam`, `addSale`); a raw REST read only for a read-only assertion; no `page.goto` after `login` (`page.reload()` is the behavior under test in one step).
- Make no commits: each task's last step hands the diff to the driver.

## Review Focus

1. Many rows that open one record: only the pressed row is marked, a press on another of them moves the mark, and the panel keeps its title — Task 7 `isRowShown` test, Task 8 store test "keeps the title and moves the mark when another row opens the same record", Task 11 test 2.
2. A `dr` that names no registered panel (a stale link, a removed package): no panel, no error, the report alone — Task 7 `recordPanelOf` null cases, Task 8 `useRecordPanel` returns null and `ReportDrillPanel` draws nothing.
3. A package panel whose chunk fails or whose component throws: "Could not load this record" (`report-record-panel-error`), the header (Open as full screen, ✕) and Escape stay, one `log.error('reports.record-panel', …)`, the report beside it usable, and the next record opened in the panel loads the chunk again — Task 8 `RecordBoundary` (reviewed; the stub cannot fail) and the `reloadableChunk` unit test.
4. A reload or a pasted link with `dr`: the panel shows, no row is marked (B2), and the focus stays where the page put it — Task 8 store (`recordOpens`/`recordFocused`), Task 11 test 2 reload step.
5. Below 1360 px a press pushes the path and leaves no `dr`; a `dr` already in the route there shows the page form — Task 7 `rowOpensInPanel` (`page` → false), Task 8 `useIsDrillPage`, Task 11 test 3.

---

### Task 7: D5 — the URL key and the decision rules

**Files:**
- Modify: `core/reports/lib/view-url.ts` (`RECORD_KEY`; `clearedDrillKeys` clears it; comments)
- Modify: `core/reports/lib/record-panel.ts` (Task 6 created it with `ReportPanelEntry`; add the rules)
- Create: `core/reports/lib/__tests__/record-panel.test.ts`
- Modify: `core/reports/lib/__tests__/view-url.test.ts`, `core/reports/lib/__tests__/drill.test.ts`

**Interfaces:**
- Consumes: `ReportPanelEntry` (`{ slug: string; path: string; load: … }`, Task 6, `core/reports/lib/record-panel.ts`); `DetailSurface` (`@tinycld/core/lib/detail-surface`); `firstOf` (`../lib/drill`); `SearchParams`, `clearedDrillKeys`, `DRILL_KEY`, `DRILL_PREFIX`, `reportLink`, `parseViewOptions` (`view-url.ts`); `drillPanelParams` (`drill.ts`, which spreads `clearedDrillKeys`).
- Produces: `RECORD_KEY = 'dr'` (view-url.ts); `clearedDrillKeys(search)` also clears `dr` (N4); in record-panel.ts: `type RecordPanelMatch = { slug: string; path: string; segments: string[] }`, `recordPanelOf(open: string, panels: readonly Pick<ReportPanelEntry, 'slug' | 'path'>[]): RecordPanelMatch | null`, `rowOpensInPanel(input: { open: string | undefined; surface: DetailSurface; hasPanel: boolean; panels: readonly Pick<ReportPanelEntry, 'slug' | 'path'>[] }): boolean`, `recordPanelParams(open: string, current: SearchParams): Record<string, string | undefined>`, `isRowShown(row: ResultRow, openedRow: string | null, search: SearchParams): boolean`.

- [ ] **Step 1: Write the failing rule tests**

Create `core/reports/lib/__tests__/record-panel.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { isRowShown, recordPanelOf, recordPanelParams, rowOpensInPanel } from '../record-panel'
import type { ResultRow } from '../types'

// A fictional package: core names no real one.
const PANELS = [
    { slug: 'gizmos', path: 'history' },
    { slug: 'gizmos', path: 'history/deep' },
    { slug: 'acme', path: 'card' },
]

function row(id: string, open?: string): ResultRow {
    return { id, kind: 'detail', depth: 0, label: id, cells: {}, ...(open ? { open } : {}) }
}

describe('recordPanelOf', () => {
    it('names the panel and the segments after <slug>/<path>/', () => {
        expect(recordPanelOf('gizmos/history/c/1', PANELS)).toEqual({
            slug: 'gizmos',
            path: 'history',
            segments: ['c', '1'],
        })
    })

    it('reads a path with one leading slash as the same path', () => {
        expect(recordPanelOf('/gizmos/history/c/1', PANELS)).toEqual({
            slug: 'gizmos',
            path: 'history',
            segments: ['c', '1'],
        })
    })

    it('takes the longer of two nested paths', () => {
        expect(recordPanelOf('gizmos/history/deep/1', PANELS)).toEqual({
            slug: 'gizmos',
            path: 'history/deep',
            segments: ['1'],
        })
    })

    it('opens no panel for a path no registered panel names', () => {
        for (const open of [
            'gizmos/history',
            'gizmos/history/',
            'gizmos/historyx/1',
            'other/history/1',
            'gizmos/history/c//1',
            'gizmos/history/1?tab=x',
            'gizmos/history/1#top',
            '//gizmos/history/1',
            '',
        ]) {
            expect(recordPanelOf(open, PANELS), open).toBeNull()
        }
        expect(recordPanelOf('gizmos/history/1', [])).toBeNull()
    })
})

describe('rowOpensInPanel', () => {
    const wide = {
        open: 'gizmos/history/c/1',
        surface: 'panel' as const,
        hasPanel: true,
        panels: PANELS,
    }

    it('opens a registered path beside the report on a wide window of a host', () => {
        expect(rowOpensInPanel(wide)).toBe(true)
    })

    it('pushes on a page surface, an unregistered path, a view with no host or a row with no open', () => {
        expect(rowOpensInPanel({ ...wide, surface: 'page' })).toBe(false)
        expect(rowOpensInPanel({ ...wide, open: 'gizmos/sale/1' })).toBe(false)
        expect(rowOpensInPanel({ ...wide, hasPanel: false })).toBe(false)
        expect(rowOpensInPanel({ ...wide, open: undefined })).toBe(false)
    })
})

describe('recordPanelParams', () => {
    it('writes dr and clears every drill key (one side surface)', () => {
        // toStrictEqual: toEqual ignores undefined keys, so it would pass a clear that clears nothing.
        expect(
            recordPanelParams('gizmos/history/c/1', {
                d: 'r1',
                'd.p-period': 'thisMonth',
                'd.s': 'amount:asc',
            })
        ).toStrictEqual({
            d: undefined,
            'd.p-period': undefined,
            'd.s': undefined,
            dr: 'gizmos/history/c/1',
        })
    })

    it('replaces an open record and leaves the report keys alone', () => {
        expect(
            recordPanelParams('gizmos/history/c/2', {
                dr: 'gizmos/history/c/1',
                'p-period': 'lastMonth',
            })
        ).toStrictEqual({ dr: 'gizmos/history/c/2' })
    })
})

describe('isRowShown', () => {
    const open = 'gizmos/history/c/1'

    it('marks only the pressed row of many that open one record', () => {
        const search = { dr: open }
        expect(isRowShown(row('d:a', open), 'd:a', search)).toBe(true)
        expect(isRowShown(row('d:b', open), 'd:a', search)).toBe(false)
    })

    it('marks no row when the panel shows another record or no record', () => {
        expect(isRowShown(row('d:a', open), 'd:a', { dr: 'gizmos/history/c/2' })).toBe(false)
        expect(isRowShown(row('d:a', open), 'd:a', {})).toBe(false)
        expect(isRowShown(row('d:a', open), 'd:a', { dr: undefined })).toBe(false)
    })

    it('marks no row after a reload (no pressed row) and never a row with no open', () => {
        expect(isRowShown(row('d:a', open), null, { dr: open })).toBe(false)
        expect(isRowShown(row('d:a'), 'd:a', { dr: open })).toBe(false)
    })

    it('reads the first of a repeated dr', () => {
        expect(isRowShown(row('d:a', open), 'd:a', { dr: [open, 'x'] })).toBe(true)
    })
})
```

In `core/reports/lib/__tests__/view-url.test.ts`, add `RECORD_KEY` to the `../view-url` import (alphabetical: after `parseViewOptions`, before `reportLink`), and add inside `describe('two report views on one route (D4)', …)`, after `it('copies a link with no from, d or d. key', …)`:

```ts
    it('names the record key', () => {
        expect(RECORD_KEY).toBe('dr')
    })

    it('clears the record panel with the drill keys (one side surface, R7)', () => {
        expect(clearedDrillKeys({ dr: 'x', d: 'r', other: '1' })).toStrictEqual({
            dr: undefined,
            d: undefined,
        })
    })

    it('copies a link with no dr, whatever the route holds', () => {
        const search = { ...SEARCH, dr: 'gizmos/history/c/1' }
        const link = new URL(reportLink('https://x.test', 'r1', DEFS, parseViewOptions(DEFS, search)))
        expect(link.searchParams.has('dr')).toBe(false)
        expect(link.searchParams.get('p-period')).toBe('lastMonth')
    })
```

In `core/reports/lib/__tests__/drill.test.ts`, inside `describe('drillPanelParams', …)`, after `it('gives nothing for a figure with no drill', …)`:

```ts
    it('closes an open record panel (one side surface, R7)', () => {
        expect(drillPanelParams(COLUMN, rowWith(drill), { dr: 'gizmos/history/c/1' })).toStrictEqual({
            dr: undefined,
            d: 'r2',
            'd.p-period': '2026-03-01..2026-03-31',
            'd.p-status': 'paid',
        })
    })
```

- [ ] **Step 2: Run them and see them fail**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/record-panel.test.ts core/reports/lib/__tests__/view-url.test.ts core/reports/lib/__tests__/drill.test.ts`
Expected: FAIL. record-panel.test.ts: `TypeError: … recordPanelOf is not a function` (and the same for the other three rules). view-url.test.ts: "names the record key" (`expected undefined to be 'dr'`) and "clears the record panel…" (the result lacks `dr: undefined`) fail; "copies a link with no dr" passes (it pins today's `reportLink`, R7). drill.test.ts: "closes an open record panel" fails (no `dr: undefined` key). Every other test passes.

- [ ] **Step 3: The `dr` key**

`core/reports/lib/view-url.ts`, after `export const DRILL_PREFIX = 'd.'`:

```ts
/** The record a package panel shows beside the report (D5): an app path. Never part of a copied link. */
export const RECORD_KEY = 'dr'
```

Replace `clearedDrillKeys`:

```ts
/**
 * Every key of the side panel, cleared (a drill's and a record's: one side surface
 * at a time, R7). router.setParams removes an undefined key.
 */
export function clearedDrillKeys(search: SearchParams): Record<string, undefined> {
    const out: Record<string, undefined> = {}
    for (const key of Object.keys(search)) {
        if (key === DRILL_KEY || key === RECORD_KEY || key.startsWith(DRILL_PREFIX)) {
            out[key] = undefined
        }
    }
    return out
}
```

In `reportLink`'s doc comment, change "so no `from`, `d` or `d.` key can reach a copied link (R10)" to "so no `from`, `d`, `d.` or `dr` key can reach a copied link".

- [ ] **Step 4: The rules**

Add to `core/reports/lib/record-panel.ts`, below Task 6's `ReportPanelEntry` (keep it as it is; merge the imports with the file's own):

```ts
import type { DetailSurface } from '@tinycld/core/lib/detail-surface'
import { firstOf } from './drill'
import type { ResultRow } from './types'
import { clearedDrillKeys, RECORD_KEY, type SearchParams } from './view-url'

/** The registered panel an app path names, and the path's segments after `<slug>/<path>/`. */
export type RecordPanelMatch = { slug: string; path: string; segments: string[] }

type PanelKey = Pick<ReportPanelEntry, 'slug' | 'path'>

/**
 * The panel a row's `open` names (R10): one leading `/` is ignored, then
 * `<slug>/<path>/` and at least one segment, none empty. A path with a query or a
 * hash names none. When two paths of one package nest, the longer one wins.
 */
export function recordPanelOf(open: string, panels: readonly PanelKey[]): RecordPanelMatch | null {
    const path = open.startsWith('/') ? open.slice(1) : open
    if (/[?#]/.test(path)) return null
    let best: RecordPanelMatch | null = null
    for (const panel of panels) {
        const prefix = `${panel.slug}/${panel.path}/`
        if (!path.startsWith(prefix)) continue
        const segments = path.slice(prefix.length).split('/')
        if (segments.some(segment => segment === '')) continue
        if (best && best.path.length >= panel.path.length) continue
        best = { slug: panel.slug, path: panel.path, segments }
    }
    return best
}

/**
 * A row press opens its record beside the report only on a wide window
 * (`panel` surface) of a view that hosts the side panel, for a registered path;
 * otherwise it pushes the path as before (R9).
 */
export function rowOpensInPanel(input: {
    open: string | undefined
    surface: DetailSurface
    hasPanel: boolean
    panels: readonly PanelKey[]
}): boolean {
    if (!input.open || !input.hasPanel || input.surface !== 'panel') return false
    return recordPanelOf(input.open, input.panels) !== null
}

/** The route params that show a record beside the report; a drill shown there closes. */
export function recordPanelParams(
    open: string,
    current: SearchParams
): Record<string, string | undefined> {
    return { ...clearedDrillKeys(current), [RECORD_KEY]: open }
}

/**
 * The pressed row is marked while the panel shows its record (R11). Many rows can
 * open one record, so the row id decides; a reload has no pressed row (Q1).
 */
export function isRowShown(row: ResultRow, openedRow: string | null, search: SearchParams): boolean {
    if (openedRow === null || row.id !== openedRow || !row.open) return false
    return firstOf(search[RECORD_KEY]) === row.open
}
```

- [ ] **Step 5: Run the tests**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/record-panel.test.ts core/reports/lib/__tests__/view-url.test.ts core/reports/lib/__tests__/drill.test.ts`
Expected: PASS (every test of the three files).

"copies a link with no dr, whatever the route holds" passed before the change (it pins today's `reportLink`), so prove once that it bites. In `core/reports/lib/view-url.ts` `reportLink`, after the `for (const [key, value] of Object.entries(serializeViewOptions(defs, options))) { … }` loop, add the line `query.set(RECORD_KEY, 'x')`. Run `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/view-url.test.ts`. Expected: "copies a link with no dr, whatever the route holds" FAILS at `expect(link.searchParams.has('dr')).toBe(false)` (`expected true to be false`). Remove the line with the Edit tool, run the file again (PASS), and write the mutation, the failing line and the undo into the hand-back.

- [ ] **Step 6: Typecheck and biome**

Run: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck` then `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check core/reports/lib/view-url.ts core/reports/lib/record-panel.ts core/reports/lib/__tests__/record-panel.test.ts core/reports/lib/__tests__/view-url.test.ts core/reports/lib/__tests__/drill.test.ts`
Expected: no errors.

- [ ] **Step 7: Hand the diff to the driver** (no commit).

---

### Task 8: D5 view — the record panel and the marked row

**Files:**
- Create: `core/reports/lib/registered-panels.ts`
- Modify: `core/reports/hooks/usePanelTitle.ts` (Task 6 created it with `ReportPanelProps`; add the hook and the provider)
- Modify: `core/reports/stores/report-ui-store.ts`
- Modify: `core/reports/lib/figure-focus.ts` (`rowFocusKey`, C1)
- Modify: `core/reports/hooks/useRowLinks.ts` (`useOpenRecordScreen` C3, the panel press, `rowKey`, `isRowShown`)
- Modify: `core/reports/components/ReportLinks.tsx` (`OpenRowLabel` mark and focus ref; `RowFrame` mark)
- Modify: `core/reports/components/ReportRow.tsx:333-341, 526, 585`
- Modify: `core/reports/components/ReportHeader.tsx:14` (`ReportBackLink` `testID`, C2)
- Modify: `core/reports/hooks/useDrillPanel.ts` (`useRecordPanel`, `RecordPanelModel`; `useIsDrillPage`)
- Modify: `core/reports/components/ReportDrillPanel.tsx` (record mode)
- Test: `core/reports/stores/__tests__/report-ui-store.test.ts`
- Create: `core/reports/lib/__tests__/registered-panels.test.ts` (C7)

`core/reports/screens/ReportScreen.tsx` does not change: `ReportDrillPanel` already sits in `side`, `drill.hasPanel` is already `true`, and `isCovered` reads `useIsDrillPage()`, which now covers `dr`.

**Interfaces:**
- Consumes: Task 6 — `reportPanels: ReportPanelEntry[]` (`@tinycld/app-generated/report-panels`; core unit-setup mocks it to `[]`), `ReportPanelProps`; Task 7 — `RECORD_KEY`, `recordPanelOf`, `rowOpensInPanel`, `recordPanelParams`, `isRowShown`, `clearedDrillKeys`; core — `lazyChunk`, `PreloadableComponent` (`@tinycld/core/lib/lazy-chunk`), `useDetailSurface`, `LayerEscape`, `LoadingState`, `log` (`@tinycld/core/lib/logger`), `figureKey`, `focusFigure`, `focusNode`, `useFigureRef`.
- Produces: `registeredPanels: readonly RegisteredPanel[]` (`{ slug: string; path: string; Component: PreloadableComponent<ReportPanelProps>; reload: () => void }`), `reloadableChunk(load: ReportPanelEntry['load']): Pick<RegisteredPanel, 'Component' | 'reload'>` (C7); `usePanelTitle(title: string): void`, `PanelTitleProvider({ open, children }: { open: string; children: ReactNode })`; store `openedRow: string | null`, `openRecord(rowId: string): void`, `recordOpens: number`, `recordFocused: number`, `noteRecordFocused(): void`, `type RecordTitle = { open: string; title: string }`, `recordTitle: RecordTitle | null`, `setRecordTitle(open: string, title: string): void`, `recordTitleOf(named: RecordTitle | null, open: string): string` (C6); `rowFocusKey(scope: ReportUIScope, rowId: string): string`; `useOpenRecordScreen(): (path: string) => void`; `RowLinks.rowKey(row: ResultRow): string`, `RowLinks.isRowShown(row: ResultRow): boolean`; `OpenRowLabel` props `isShown: boolean`, `focusKey: string`; `ReportBackLink` prop `testID?: string`; `RecordPanelInput`, `useRecordPanel(input: RecordPanelInput): RecordPanelModel | null`, `type RecordPanelModel`; `useIsDrillPage()` true for a `d` or a registered `dr` on a `page` surface; test ids `report-record-panel`, `report-record-panel-header`, `report-record-panel-title`, `report-record-panel-full` ("Open as full screen"), `report-record-panel-close`, `report-record-panel-back`, `report-record-panel-loading`, `report-record-panel-error`.

- [ ] **Step 1: Write the failing store tests**

`core/reports/stores/__tests__/report-ui-store.test.ts`: change the import on line 2 to `import { recordTitleOf, useReportUIStore } from '../report-ui-store'`, and add inside `describe('the report UI store per view', …)`, before `it('starts a visit with both views closed', …)`:

```ts
    // A fictional record path: core names no real package.
    const OPEN = 'gizmos/history/c/1'

    it('opens a record: marks its row and counts the press', () => {
        const before = state().recordOpens
        state().openRecord('d:row1')
        expect(state().openedRow).toBe('d:row1')
        expect(state().recordOpens).toBe(before + 1)
    })

    it("closes the report's Customize when a record opens, and leaves the drill view's", () => {
        state().openCustomize()
        state().openCustomize('drill')
        state().openRecord('d:row1')
        expect(state().scopes.main.isCustomizeOpen).toBe(false)
        expect(state().scopes.drill.isCustomizeOpen).toBe(true)
    })

    it("keeps a record's newest title", () => {
        state().openRecord('d:row1')
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        state().setRecordTitle(OPEN, 'Gizmo 1001 (paid)')
        expect(recordTitleOf(state().recordTitle, OPEN)).toBe('Gizmo 1001 (paid)')
    })

    it('shows no title for another record until its panel names it', () => {
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        expect(recordTitleOf(state().recordTitle, 'gizmos/history/c/2')).toBe('')
        expect(recordTitleOf(null, OPEN)).toBe('')
    })

    // Many rows open one record. A press on another of them leaves dr and the
    // panel's component as they are (no remount, so usePanelTitle does not run
    // again): the title stays and the mark moves (C6).
    it('keeps the title and moves the mark when another row opens the same record', () => {
        state().openRecord('d:a')
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        state().openRecord('d:b')
        expect(state().openedRow).toBe('d:b')
        expect(recordTitleOf(state().recordTitle, OPEN)).toBe('Gizmo 1001')
        // A second press on the opened row keeps it too.
        state().openRecord('d:b')
        expect(recordTitleOf(state().recordTitle, OPEN)).toBe('Gizmo 1001')
    })

    it('marks a record open as focused only once, until the next press', () => {
        state().openRecord('d:row1')
        expect(state().recordOpens).toBeGreaterThan(state().recordFocused)
        state().noteRecordFocused()
        expect(state().recordFocused).toBe(state().recordOpens)
        state().openRecord('d:row2')
        expect(state().recordOpens).toBe(state().recordFocused + 1)
    })

    it('starts a visit with no record opened', () => {
        state().openRecord('d:row1')
        state().setRecordTitle(OPEN, 'Gizmo 1001')
        state().noteRecordFocused()
        state().startVisit()
        expect(state().openedRow).toBeNull()
        expect(state().recordTitle).toBeNull()
        expect(state().recordOpens).toBe(0)
        expect(state().recordFocused).toBe(0)
    })
```

Create `core/reports/lib/__tests__/registered-panels.test.ts` (C7):

```ts
import type { ComponentType } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ReportPanelProps } from '../../hooks/usePanelTitle'
import type { ReportPanelEntry } from '../record-panel'
import { reloadableChunk } from '../registered-panels'

const Panel: ComponentType<ReportPanelProps> = () => null

describe('reloadableChunk (R14)', () => {
    it('keeps a failed chunk until reload, then loads it again', async () => {
        const load = vi
            .fn<ReportPanelEntry['load']>()
            .mockRejectedValueOnce(new Error('offline'))
            .mockResolvedValue({ default: Panel })
        const chunk = reloadableChunk(load)
        await expect(chunk.Component.preload()).rejects.toThrow('offline')
        // lazyChunk keeps its first promise: with no reload the panel stays failed.
        await expect(chunk.Component.preload()).rejects.toThrow('offline')
        expect(load).toHaveBeenCalledTimes(1)
        chunk.reload()
        await expect(chunk.Component.preload()).resolves.toEqual({ default: Panel })
        expect(load).toHaveBeenCalledTimes(2)
    })
})
```

- [ ] **Step 2: Run them and see them fail**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/stores/__tests__/report-ui-store.test.ts core/reports/lib/__tests__/registered-panels.test.ts`
Expected: FAIL. report-ui-store.test.ts: the seven new tests with `TypeError: state(...).openRecord is not a function`, `state(...).setRecordTitle is not a function` or `recordTitleOf is not a function`; the seven older tests pass. registered-panels.test.ts: `Failed to resolve import "../registered-panels"`.

- [ ] **Step 3: The store**

`core/reports/stores/report-ui-store.ts`, in `interface ReportUIState`, after `setDrillOrigin`:

```ts
    // The detail row whose press opened the record panel (D5, R11). Many rows can
    // open one record, so the row id marks the pressed one. Not in the URL: a reload
    // shows the panel with no row marked (Q1).
    openedRow: string | null
    // A row press that opens a record beside the report: marks the row and counts
    // the press (the panel title takes the focus). The report's Customize closes:
    // one side surface at a time. The title is not touched: it belongs to its
    // record path (recordTitle), and a press that opens the same record keeps it.
    openRecord: (rowId: string) => void
    recordOpens: number
    // The recordOpens count whose press has moved the focus to the panel title.
    recordFocused: number
    noteRecordFocused: () => void
    /**
     * The record panel's title, as the package's component names it
     * (usePanelTitle), with the record path it names. The header shows it only
     * while that path is the route's dr (recordTitleOf), so a replaced record never
     * shows the previous title (R13).
     */
    recordTitle: RecordTitle | null
    setRecordTitle: (open: string, title: string) => void
```

Above `interface ReportUIState`, add:

```ts
/** A record panel's title and the record path it names (R13). */
export type RecordTitle = { open: string; title: string }

/** The title the record panel shows for `open`: empty until its component names it. */
export function recordTitleOf(named: RecordTitle | null, open: string): string {
    return named?.open === open ? named.title : ''
}
```

In the store body, after `setDrillOrigin: key => set({ drillOrigin: key }),`:

```ts
    openedRow: null,
    openRecord: rowId =>
        set(state => ({
            ...withScope(state, 'main', { isCustomizeOpen: false }),
            openedRow: rowId,
            recordOpens: state.recordOpens + 1,
        })),
    recordOpens: 0,
    recordFocused: 0,
    noteRecordFocused: () => set(state => ({ recordFocused: state.recordOpens })),
    recordTitle: null,
    setRecordTitle: (open, title) => set({ recordTitle: { open, title } }),
```

Replace `startVisit`:

```ts
    startVisit: () =>
        set({
            scopes: { main: CLOSED, drill: CLOSED },
            builderPicker: null,
            drillOrigin: null,
            openedRow: null,
            recordTitle: null,
            recordOpens: 0,
            recordFocused: 0,
        }),
```

- [ ] **Step 4: Run the store tests**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/stores/__tests__/report-ui-store.test.ts`
Expected: PASS (14 tests). (`registered-panels.test.ts` stays red until Step 5.)

- [ ] **Step 5: The registered panels and the title hook**

Create `core/reports/lib/registered-panels.ts`:

```ts
import { reportPanels } from '@tinycld/app-generated/report-panels'
import { lazyChunk, type PreloadableComponent } from '@tinycld/core/lib/lazy-chunk'
import { createElement } from 'react'
import type { ReportPanelProps } from '../hooks/usePanelTitle'
import type { ReportPanelEntry } from './record-panel'

export interface RegisteredPanel {
    slug: string
    path: string
    Component: PreloadableComponent<ReportPanelProps>
    /** Drops a failed chunk, so the next record opened in this panel loads it again. */
    reload: () => void
}

/**
 * A panel's lazy component that can load again (R14). lazyChunk keeps its first
 * promise for the session, so a chunk that failed once (offline) would fail every
 * later record of the panel. The panel's error boundary calls reload after it
 * catches, when the failed panel has left the screen, so no render starts a load
 * that the same render then drops.
 */
export function reloadableChunk(
    load: ReportPanelEntry['load']
): Pick<RegisteredPanel, 'Component' | 'reload'> {
    let chunk = lazyChunk(load)
    const Panel = (props: ReportPanelProps) => createElement(chunk, props)
    return {
        Component: Object.assign(Panel, { preload: () => chunk.preload() }),
        reload: () => {
            chunk = lazyChunk(load)
        },
    }
}

// The one reader of the generated list (R8): the pure rules take the panels as an
// argument, so tests need no generated file. Each package's chunk loads when a
// record first opens in its panel.
export const registeredPanels: readonly RegisteredPanel[] = reportPanels.map(entry => ({
    slug: entry.slug,
    path: entry.path,
    ...reloadableChunk(entry.load),
}))
```

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/registered-panels.test.ts`
Expected: PASS (1 test).

`core/reports/hooks/usePanelTitle.ts`: keep Task 6's `ReportPanelProps`; add (merge the imports):

```ts
import { createContext, createElement, type ReactNode, use, useEffect } from 'react'
import { useReportUIStore } from '../stores/report-ui-store'

// The record path the report's record panel shows. On the package's own screen
// the same component runs outside the provider (null), and the hook does nothing
// (R13).
const PanelRecord = createContext<string | null>(null)

/** Wraps a package's panel component inside the report's record panel. */
export function PanelTitleProvider({ open, children }: { open: string; children: ReactNode }) {
    return createElement(PanelRecord.Provider, { value: open }, children)
}

/** Names the record a report's side panel shows; the panel's header shows the name. */
export function usePanelTitle(title: string): void {
    const open = use(PanelRecord)
    const setRecordTitle = useReportUIStore(s => s.setRecordTitle)
    // A child telling its host what to show, not state derived in render. The
    // title is filed under its record path, so it never shows for another record.
    useEffect(() => {
        if (open !== null) setRecordTitle(open, title)
    }, [open, title, setRecordTitle])
}
```

- [ ] **Step 6: The row label's focus key**

`core/reports/lib/figure-focus.ts`, after `figureKey`:

```ts
// A row label is filed with the figures under a column id no column can have
// (column ids start with a letter), so the record panel's close finds it (R12).
const OPEN_ROW = '@open'

/** A row label's focus key: the record link of a detail row. */
export function rowFocusKey(scope: ReportUIScope, rowId: string): string {
    return figureKey(scope, OPEN_ROW, rowId)
}
```

- [ ] **Step 7: The row press**

`core/reports/hooks/useRowLinks.ts`. Replace the imports block with:

```ts
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { type Href, useLocalSearchParams, useNavigation, useRouter } from 'expo-router'
import { CommonActions, type NavigationState } from 'expo-router/react-navigation'
import { useCallback, useMemo, useRef } from 'react'
import { drillHref, firstOf, isDrillShown, shownDrillParams, useOpenDrill } from '../lib/drill'
import { figureKey, rowFocusKey } from '../lib/figure-focus'
import { isRowShown, recordPanelParams, rowOpensInPanel } from '../lib/record-panel'
import {
    type OpenedRecord,
    openedRecordOf,
    returnStepOf,
    routeKeysOf,
    tabRouteOf,
} from '../lib/record-return'
import { registeredPanels } from '../lib/registered-panels'
import type { ResultColumn, ResultRow } from '../lib/types'
import { RECORD_KEY } from '../lib/view-url'
import { type ReportUIScope, useReportUIStore } from '../stores/report-ui-store'
import { useStable } from './useStable'
```

Replace `interface RowLinks` and `useRowLinks` (lines 26–78) with:

```ts
/** What a table's figures and rows open. */
export interface RowLinks {
    figureHref(column: ResultColumn, row: ResultRow): Href | null
    onFigure(column: ResultColumn, row: ResultRow): void
    figureKey(column: ResultColumn, row: ResultRow): string
    isFigureShown(column: ResultColumn, row: ResultRow): boolean
    rowHref(row: ResultRow): Href | null
    onRow(row: ResultRow): void
    /** The row label's key (rowFocusKey): the record panel's close sends the focus back there. */
    rowKey(row: ResultRow): string
    /** The record panel shows this row's record, opened by a press on this row (R11). */
    isRowShown(row: ResultRow): boolean
}

/**
 * A record's own screen, pushed in its package's tab; Back from it returns to the
 * report (watchRecordReturn). A row press and the record panel's Open as full
 * screen both open it.
 */
export function useOpenRecordScreen(): (path: string) => void {
    const router = useRouter()
    const navigation = useRouteNavigation()
    const orgHref = useOrgHref()
    return useCallback(
        (path: string) => {
            watchRecordReturn(navigation, tabNameOf(path))
            router.push(orgHref(path))
        },
        [navigation, router, orgHref]
    )
}

/**
 * A row press that opens its record beside the report (D5): a registered panel's
 * path, a wide window and a view that hosts the side panel. False: the row opens as
 * a screen.
 */
function useOpenRecordPanel(hasPanel: boolean) {
    const router = useRouter()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const openRecord = useReportUIStore(s => s.openRecord)
    return (row: ResultRow): boolean => {
        const open = row.open
        if (!open || !rowOpensInPanel({ open, surface, hasPanel, panels: registeredPanels })) {
            return false
        }
        router.setParams(recordPanelParams(open, search))
        openRecord(row.id)
        return true
    }
}

/**
 * The table's links, or undefined where figures and rows are plain (the builder's
 * preview passes no drill). They keep their identity while the drill keys, the
 * shown record and the pressed row stay, so FlashList redraws the rows (extraData)
 * only when a figure's or a row's mark may change.
 */
export function useRowLinks(drill: RowDrill | undefined): RowLinks | undefined {
    const orgHref = useOrgHref()
    const search = useLocalSearchParams()
    const shown = useStable(shownDrillParams(search))
    const shownRecord = firstOf(search[RECORD_KEY])
    const openedRow = useReportUIStore(s => s.openedRow)
    const openScreen = useOpenRecordScreen()
    const openDrill = useOpenDrill({
        sourceId: drill?.sourceId ?? '',
        hasPanel: drill?.hasPanel ?? false,
    })
    const openPanel = useOpenRecordPanel(drill?.hasPanel ?? false)
    // The presses read the newest handlers, so the links need not change with them.
    const latest = useRef({ openDrill, openPanel })
    latest.current = { openDrill, openPanel }
    const sourceId = drill?.sourceId
    const scope = drill?.scope ?? 'main'
    const setDrillOrigin = useReportUIStore(s => s.setDrillOrigin)
    return useMemo<RowLinks | undefined>(() => {
        if (sourceId === undefined) return undefined
        const record = { [RECORD_KEY]: shownRecord }
        return {
            figureHref: (column, row) => {
                const href = drillHref(column, row, sourceId)
                return href === null ? null : orgHref(href)
            },
            onFigure: (column, row) => {
                setDrillOrigin(figureKey(scope, column.id, row.id))
                latest.current.openDrill(column, row)
            },
            figureKey: (column, row) => figureKey(scope, column.id, row.id),
            isFigureShown: (column, row) => isDrillShown(column, row, shown),
            rowHref: row => (row.open ? orgHref(row.open) : null),
            onRow: row => {
                if (!row.open || latest.current.openPanel(row)) return
                openScreen(row.open)
            },
            rowKey: row => rowFocusKey(scope, row.id),
            isRowShown: row => isRowShown(row, openedRow, record),
        }
    }, [sourceId, scope, shown, shownRecord, openedRow, orgHref, openScreen, setDrillOrigin])
}
```

Delete the old `// Only the drill keys mark a figure, so the links change only when they do.` comment line (it described the old memo). `useRouteNavigation`, `tabsOf`, `holdsTab`, `tabNameOf` and `watchRecordReturn` stay as they are below.

- [ ] **Step 8: The row mark and the label's focus**

`core/reports/components/ReportLinks.tsx`. Replace `OpenRowLabelProps` and `OpenRowLabel`:

```tsx
interface OpenRowLabelProps {
    href: Href
    testID: string
    text: string
    textClass: string
    lines: number
    style: ViewStyle
    /** The record panel shows this row's record (R11); the one row marked. */
    isShown: boolean
    /** The label's key (rowFocusKey), under which it takes the focus back when the panel closes. */
    focusKey: string
    onPress(): void
}

/** A row's label that opens its record (D2), named "Open: <label>". */
export function OpenRowLabel(props: OpenRowLabelProps) {
    // The label is the row's one accessible control (RowFrame is not), so it carries
    // the selected state; React Native maps aria-selected to accessibilityState. Only
    // the opened row carries it: ARIA has no selected state for a link, so the other
    // rows leave it out rather than say "false".
    const ref = useFigureRef(props.focusKey, false)
    return (
        <Link href={props.href} asChild>
            <Pressable
                ref={ref}
                testID={props.testID}
                accessibilityRole="link"
                accessibilityLabel={`Open: ${props.text}`}
                aria-selected={props.isShown || undefined}
                onPress={linkPress(props.onPress)}
                className="flex-row items-center pr-3 web:cursor-pointer"
                style={props.style}
            >
                {state => (
                    <Text
                        numberOfLines={props.lines}
                        className={`${props.textClass} ${linkTone(state)}`}
                    >
                        {props.text}
                    </Text>
                )}
            </Pressable>
        </Link>
    )
}
```

Replace `RowFrame` (its comment stays):

```tsx
export function RowFrame({ row, links, isCopy, className, children }: RowFrameProps) {
    const href = links && !isCopy ? links.rowHref(row) : null
    // The row whose record the panel shows takes the selected-row token, as a shown figure does.
    const tone = links && href !== null && links.isRowShown(row) ? 'bg-accent' : 'bg-background'
    if (!links || href === null) return <View className={`${className} ${tone}`}>{children}</View>
    return (
        <Pressable
            accessible={false}
            focusable={false}
            tabIndex={-1}
            onPress={linkPress(() => links.onRow(row))}
            className={`${className} ${tone} web:cursor-pointer`}
        >
            {children}
        </Pressable>
    )
}
```

`core/reports/components/ReportRow.tsx`:
- In `RowLabel`'s `<OpenRowLabel …>` (line 333), after `style={labelStyle(item.row, justify, minHeight)}` add:

```tsx
                isShown={links.isRowShown(item.row)}
                focusKey={links.rowKey(item.row)}
```

- Line 526: `className={`flex-row bg-background ${ROW_RULE[item.row.kind]}`}` → `className={`flex-row ${ROW_RULE[item.row.kind]}`}`.
- Line 585: `className={`bg-background ${ROW_RULE[item.row.kind]}`}` → `className={ROW_RULE[item.row.kind]}`.

(RowFrame now adds `bg-background`, or `bg-accent` on the opened row.)

- [ ] **Step 9: The back link's test id**

`core/reports/components/ReportHeader.tsx:14`, replace the signature line and the `testID`:

```tsx
export function ReportBackLink({
    label,
    onBack,
    testID = 'report-back',
}: {
    label: string
    onBack: () => void
    testID?: string
}) {
```

and `testID="report-back"` → `testID={testID}` in its `Pressable`.

- [ ] **Step 10: The record panel's model**

`core/reports/hooks/useDrillPanel.ts`. Imports become:

```ts
import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useDetailSurface } from '@tinycld/core/lib/use-detail-surface'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { useCallback } from 'react'
import { firstOf, fullReportHref } from '../lib/drill'
import { type FocusTarget, focusFigure, focusNode, rowFocusKey } from '../lib/figure-focus'
import { recordPanelOf } from '../lib/record-panel'
import { registeredPanels } from '../lib/registered-panels'
import { UNREADABLE_SPEC } from '../lib/spec'
import { clearedDrillKeys, DRILL_KEY, DRILL_PREFIX, RECORD_KEY } from '../lib/view-url'
import { recordTitleOf, useReportUIStore } from '../stores/report-ui-store'
import { useReportRecord } from './useReportRecord'
import { useReportView } from './useReportView'
import { useOpenRecordScreen } from './useRowLinks'
```

Add after `export type DrillPanelModel = ReturnType<typeof useDrillPanel>`:

```ts
export interface RecordPanelInput {
    /** The app path the panel shows (the route's `dr`). */
    open: string
    /** The report beside it, for the page form's back link. */
    sourceTitle: string
}

/**
 * The record panel (D5): a package's component for the route's `dr`, beside the
 * report on a wide window and a page over it below 1360 px (style values only, so
 * a crossing never remounts it). Null when no registered panel names the path (a
 * stale link, a removed package): the report shows alone (R10).
 */
export function useRecordPanel({ open, sourceTitle }: RecordPanelInput) {
    const router = useRouter()
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const openScreen = useOpenRecordScreen()
    // The title of this record only: a replaced record shows none until its
    // component names it, and the same record keeps its own (R13).
    const title = useReportUIStore(s => recordTitleOf(s.recordTitle, open))
    // Keyed by the row presses that opened a record, as the drill panel's title is:
    // only a press moves the focus to the title, never a reload or a pasted link.
    const titleKey = useReportUIStore(s => s.recordOpens)
    const titleRef = useCallback((node: FocusTarget | null) => {
        const ui = useReportUIStore.getState()
        if (!node || ui.recordOpens <= ui.recordFocused) return
        ui.noteRecordFocused()
        focusNode(node)
    }, [])
    const match = recordPanelOf(open, registeredPanels)
    const entry = match
        ? registeredPanels.find(panel => panel.slug === match.slug && panel.path === match.path)
        : undefined
    if (!match || !entry) return null
    return {
        open,
        title,
        segments: match.segments,
        Component: entry.Component,
        isPage: surface === 'page',
        backLabel: sourceTitle,
        titleRef,
        titleKey,
        // Closing is a view change on the source's route; the focus goes back to the
        // pressed row's label when it is still drawn (R12).
        onClose: () => {
            router.setParams(clearedDrillKeys(search))
            const row = useReportUIStore.getState().openedRow
            focusFigure(row === null ? null : rowFocusKey('main', row))
        },
        onOpenFull: () => openScreen(open),
        // The boundary calls it after a failure: the next record loads the chunk again (R14).
        reloadChunk: entry.reload,
    }
}

export type RecordPanelModel = NonNullable<ReturnType<typeof useRecordPanel>>
```

Replace `useIsDrillPage`:

```ts
/** Whether the side panel (a drill or a registered record) shows as a page that covers the report. */
export function useIsDrillPage(): boolean {
    const search = useLocalSearchParams()
    const surface = useDetailSurface()
    const targetId = firstOf(search[DRILL_KEY])
    const open = firstOf(search[RECORD_KEY])
    const hasRecord = open !== undefined && recordPanelOf(open, registeredPanels) !== null
    return surface === 'page' && (Boolean(targetId) || hasRecord)
}
```

- [ ] **Step 11: The record mode of the side panel**

`core/reports/components/ReportDrillPanel.tsx`. Imports become (keep any import Part 1 Task 3 added):

```tsx
import { LoadingState } from '@tinycld/core/components/LoadingState'
import { DETAIL_PANEL_WIDTH } from '@tinycld/core/lib/detail-surface'
import { log } from '@tinycld/core/lib/logger'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { LayerEscape } from '@tinycld/core/ui/overlay'
import { useLocalSearchParams } from 'expo-router'
import { Maximize2, X } from 'lucide-react-native'
import { Component, type ReactNode, Suspense } from 'react'
import { Pressable, Text, View } from 'react-native'
import {
    type DrillPanelInput,
    type DrillPanelModel,
    type RecordPanelInput,
    type RecordPanelModel,
    useDrillPanel,
    useRecordPanel,
} from '../hooks/useDrillPanel'
import { PanelTitleProvider } from '../hooks/usePanelTitle'
import type { ReportViewModel } from '../hooks/useReportView'
import { useTableLayout } from '../hooks/useTableLayout'
import { firstOf } from '../lib/drill'
import { layoutNoticeOf } from '../lib/notices'
import { DRILL_KEY, RECORD_KEY } from '../lib/view-url'
import { ReportControls } from './ReportControls'
import { ReportDocumentHeader } from './ReportDocumentHeader'
import { REPORT_TAP_TARGET, ReportBackLink } from './ReportHeader'
import { ReportNotices } from './ReportNotices'
import { MessageBody } from './ReportStates'
import { ReportBody } from './ReportView'
```

Replace `PageBack`, `CloseButton` and `OpenFullButton` (lines 34–71) with header parts both modes share:

```tsx
interface PanelPageBackProps {
    isVisible: boolean
    label: string
    onBack: () => void
    testID?: string
}

function PanelPageBack({ isVisible, label, onBack, testID }: PanelPageBackProps) {
    if (!isVisible) return null
    return <ReportBackLink label={label} onBack={onBack} testID={testID} />
}

interface PanelCloseButtonProps {
    isVisible: boolean
    testID: string
    onClose: () => void
}

function PanelCloseButton({ isVisible, testID, onClose }: PanelCloseButtonProps) {
    const muted = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            accessibilityLabel="Close"
            onPress={onClose}
            className="items-center justify-center"
            style={TAP_STYLE}
        >
            <X size={18} color={muted} />
        </Pressable>
    )
}

interface OpenFullButtonProps {
    isVisible: boolean
    testID: string
    label: string
    onPress: () => void
}

function OpenFullButton({ isVisible, testID, label, onPress }: OpenFullButtonProps) {
    const color = useThemeColor('foreground')
    if (!isVisible) return null
    return (
        <Pressable
            testID={testID}
            accessibilityRole="button"
            onPress={onPress}
            className="flex-row items-center gap-1.5 px-2"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Maximize2 size={16} color={color} />
            <Text className="text-sm text-foreground">{label}</Text>
        </Pressable>
    )
}
```

In `DrillPanelHeader`, replace `<PageBack panel={panel} />`, `<OpenFullButton panel={panel} />` and `<CloseButton panel={panel} />` with:

```tsx
            <PanelPageBack isVisible={panel.isPage} label={panel.backLabel} onBack={panel.onClose} />
```

```tsx
                <OpenFullButton
                    isVisible={panel.recordState === 'ready'}
                    testID="report-drill-open-full"
                    label="Open as full report"
                    onPress={panel.onOpenFull}
                />
                <PanelCloseButton
                    isVisible={!panel.isPage}
                    testID="report-drill-close"
                    onClose={panel.onClose}
                />
```

`RecordLoading`, `DrillRecordStates`, `DrillReport` (with Task 3's `zoneWords` on its `ReportDocumentHeader`), `DrillPanelBody` and `DrillPanel` stay as they are. Replace `ReportDrillPanel` (lines 180–194) with the record mode and the new entry:

```tsx
function RecordPanelHeader({ panel }: { panel: RecordPanelModel }) {
    return (
        <View testID="report-record-panel-header" className="gap-1 border-b border-border px-4 py-2">
            <PanelPageBack
                isVisible={panel.isPage}
                label={panel.backLabel}
                onBack={panel.onClose}
                testID="report-record-panel-back"
            />
            <View className="flex-row items-center gap-2">
                <View
                    key={panel.titleKey}
                    ref={panel.titleRef}
                    testID="report-record-panel-title"
                    tabIndex={-1}
                    accessibilityRole="header"
                    className="min-w-0 flex-1"
                >
                    <Text numberOfLines={1} className="text-lg font-semibold text-foreground">
                        {panel.title}
                    </Text>
                </View>
                <OpenFullButton
                    isVisible
                    testID="report-record-panel-full"
                    label="Open as full screen"
                    onPress={panel.onOpenFull}
                />
                <PanelCloseButton
                    isVisible={!panel.isPage}
                    testID="report-record-panel-close"
                    onClose={panel.onClose}
                />
            </View>
        </View>
    )
}

interface RecordBoundaryState {
    hasError: boolean
}

interface RecordBoundaryProps {
    children: ReactNode
    /** Drops the panel's failed chunk, so the next record loads it again (R14). */
    onError: () => void
}

// A package's panel that fails to load or throws: the panel says so and keeps its
// header (Open as full screen, ✕) and Escape; the report beside it stays usable.
// It is keyed by the record's path, so another record starts fresh (R14).
class RecordBoundary extends Component<RecordBoundaryProps, RecordBoundaryState> {
    state: RecordBoundaryState = { hasError: false }

    static getDerivedStateFromError(): RecordBoundaryState {
        return { hasError: true }
    }

    // Runs after the error text replaced the panel, so the reload starts no load
    // for this record; the next record opened in the panel loads the chunk again.
    componentDidCatch(error: Error) {
        log.error('reports.record-panel', error)
        this.props.onError()
    }

    render() {
        if (this.state.hasError) {
            return (
                <MessageBody
                    isVisible
                    testID="report-record-panel-error"
                    message="Could not load this record"
                />
            )
        }
        return this.props.children
    }
}

function RecordPanelLoading() {
    return (
        <View testID="report-record-panel-loading" className="flex-1">
            <LoadingState />
        </View>
    )
}

function RecordBody({ panel }: { panel: RecordPanelModel }) {
    const { Component: Panel, segments, open, reloadChunk } = panel
    return (
        <View className="min-h-0 flex-1">
            <PanelTitleProvider open={open}>
                <RecordBoundary key={open} onError={reloadChunk}>
                    <Suspense fallback={<RecordPanelLoading />}>
                        <Panel segments={segments} />
                    </Suspense>
                </RecordBoundary>
            </PanelTitleProvider>
        </View>
    )
}

function RecordPanel({ panel }: { panel: RecordPanelModel }) {
    return (
        <View
            testID="report-record-panel"
            role="complementary"
            aria-label={panel.title || 'Record'}
            className="border-border bg-background"
            style={panel.isPage ? PAGE_STYLE : PANEL_STYLE}
        >
            <LayerEscape onEscape={panel.onClose} />
            <RecordPanelHeader panel={panel} />
            <RecordBody panel={panel} />
        </View>
    )
}

function RecordPanelHost(props: RecordPanelInput) {
    const panel = useRecordPanel(props)
    if (!panel) return null
    return <RecordPanel panel={panel} />
}

/**
 * The report's side panel, in ReportView's side slot: the report a figure drilled
 * to (the route's `d`), or the record a row opened (`dr`, D5). One at a time; a
 * hand-edited URL that holds both shows the drill. It draws nothing without either,
 * so Escape is registered only while it is open.
 */
export function ReportDrillPanel({
    sourceId,
    sourceTitle,
}: {
    sourceId: string
    sourceTitle: string
}) {
    const search = useLocalSearchParams()
    const targetId = firstOf(search[DRILL_KEY])
    const open = firstOf(search[RECORD_KEY])
    if (targetId) {
        return <DrillPanel targetId={targetId} sourceId={sourceId} sourceTitle={sourceTitle} />
    }
    if (open) return <RecordPanelHost open={open} sourceTitle={sourceTitle} />
    return null
}
```

- [ ] **Step 12: Typecheck, biome and the reports unit tests**

Run, from the repo root:
- `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck` — Expected: no errors.
- `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check core/reports/lib/registered-panels.ts core/reports/hooks/usePanelTitle.ts core/reports/stores/report-ui-store.ts core/reports/stores/__tests__/report-ui-store.test.ts core/reports/lib/figure-focus.ts core/reports/hooks/useRowLinks.ts core/reports/components/ReportLinks.tsx core/reports/components/ReportRow.tsx core/reports/components/ReportHeader.tsx core/reports/hooks/useDrillPanel.ts core/reports/components/ReportDrillPanel.tsx core/reports/lib/__tests__/registered-panels.test.ts` — Expected: no errors.
- `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports` — Expected: PASS (every reports unit test; from the repo root the app's unit-setup does not mock `report-panels`, so the generated `lib/generated/report-panels.ts` from Task 6 Step 8 is read and no entry's `load` is called; core's `[]` mock applies under `tinycld-pkg test` from `core/`).
- `pnpm run check:core-isolation` — Expected: passes (no package named).

The view (press, mark, 480 px width, title, focus, Escape, reload, Customize, Open as full screen, page form) is proven by Task 11's e2e, and Task 11 Step 3 breaks each of this task's parts once (mutations 2 and 5–10) to see the matching assertion fail for its named reason; this task's reviewer runs no e2e.

- [ ] **Step 13: Hand the diff to the driver** (no commit).

---

### Task 9: Docs and help

**Files:**
- Modify: `core/help/reports.md` (front matter tags; new section "Dates with a time" after "Run a report"; "Open a row's record")
- Modify: `docs/packages.md` (field reference row; "Report sources" bullets; "Shipped and saved reports" bullet; section D list)
- Test: `core/reports/lib/__tests__/statements-docs.test.ts`

**Interfaces:**
- Consumes: the names of Tasks 1–8 (`Select` over text, view sources, `DateTime` zone rule, `dateTimeText`, `reportPanels`, `ReportPanelProps`, `usePanelTitle`, `dr`, `report-panels.ts`).
- Produces: help topic `core:reports` section "Dates with a time".

- [ ] **Step 1: Write the failing doc tests**

`core/reports/lib/__tests__/statements-docs.test.ts`, at the end of `describe('statement docs', …)`:

```ts
    it('names date-time columns, the time zone and the zone line in the help', () => {
        const help = read('core/help/reports.md')
        const at = help.indexOf('## Dates with a time')
        expect(at).toBeGreaterThan(-1)
        const section = help.slice(at, help.indexOf('\n## ', at + 1))
        expect(section).toContain('date and a time')
        expect(section).toContain('your time zone')
        expect(section).toContain('**Times in Central Time**')
        expect(section).toContain('The period chip and a day you type in a filter')
        const opens = help.slice(help.indexOf("## Open a row's record"))
        expect(opens).toContain('in a panel at the right')
    })

    it('names record panels, Select over text, view sources and the zone rule in the docs', () => {
        const docs = read('docs/packages.md')
        const fields = docs.slice(docs.indexOf('### Field reference'), docs.indexOf('#### Help screenshots'))
        expect(fields).toContain('| `reportPanels[]` |')
        const sources = docs.slice(
            docs.indexOf('### Report sources'),
            docs.indexOf('### Shipped and saved reports')
        )
        expect(sources).toContain('A `Select` field may read a text column')
        expect(sources).toContain('view collection')
        expect(sources).toContain('CAST(')
        expect(sources).toContain("in the reader's time zone")
        expect(sources).toContain('keep the UTC day')
        expect(sources).toContain('a day typed in a condition')
        expect(sources).toContain('with a `Date` column still compares in UTC')
        expect(sources).toContain('`timeZone` with every param-options request')
        expect(sources).toContain('dateTimeText')
        const shippedAt = docs.indexOf('### Shipped and saved reports')
        const shipped = docs.slice(shippedAt, docs.indexOf('\n### ', shippedAt + 1))
        for (const name of ['reportPanels', 'ReportPanelProps', 'usePanelTitle', '`dr=<app path>`']) {
            expect(shipped).toContain(name)
        }
        const generated = docs.slice(docs.indexOf('### D. `lib/generated/*`'), docs.indexOf('### E. '))
        expect(generated).toContain('`report-panels.ts`')
    })
```

- [ ] **Step 2: Run them and see them fail**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/statements-docs.test.ts`
Expected: FAIL: "names date-time columns…" (`expected -1 to be greater than -1`) and "names record panels…" (`| \`reportPanels[]\` |` missing); the seven older tests pass. The new assertions on a typed day (`The period chip and a day you type in a filter`, `a day typed in a condition`), on the one limit left (`with a \`Date\` column still compares in UTC`) and on the param-options body's top-level `timeZone` pin the outline's R1 (Q3 answered: a typed day is zoned too, Task 4b) and Task 5's request shape: the help and the docs must say that a typed day is a day of the reader's zone, that a `DateTime` compared with a `Date` column is not, and which key each request carries.

- [ ] **Step 3: The help**

`core/help/reports.md`:
- Front matter `tags`: add `time zone, date and time` after `all dates`.
- After the "Run a report" list (before `## Open the rows behind a figure`), add:

```markdown
## Dates with a time

Some columns hold a date and a time, such as when a record was changed. The report shows them in your time zone: "Nov 14, 2026, 4:07 PM", or with a 24-hour clock when your device uses one. When the report shows such a column, the line under the title names your time zone once, for example **Times in Central Time**.

The period chip and a day you type in a filter, such as "on or after Nov 14, 2026", also use your time zone for these columns: a day starts and ends at your midnight, not at midnight UTC. A report that groups or pivots such a column by day, month or year uses the UTC date. A column that holds only a date, such as a due date, is the same in every time zone.
```

- Replace the paragraph under `## Open a row's record` with:

```markdown
In some reports each line opens the record it comes from, on that record's own screen in its app. Select the line, or its name, to open the record. Back returns to the report as you left it.

Some apps show the record beside the report. On a window 1360 pixels wide or wider, the record then opens in a panel at the right and the line stays marked. Select another line to show its record in the same panel. Select **Open as full screen** to open the record on its own screen, or **Close** (or press Esc) to close the panel. Opening **Customize** closes the panel. A link copied with **Copy link** does not open it. On a narrower window, and on a phone, the record opens on its own screen.
```

- [ ] **Step 4: The package docs**

`docs/packages.md`:

Field reference table, after the `setupSteps[]` row:

```markdown
| `reportPanels[]` | Panels for this package's own app paths that a report's detail row opens beside the report: `{ path, component }`. `path` is one or more segments of letters, digits, `-` and `_`, under the package slug, distinct within the package; `component` is a module path under `tinycld/<slug>/`, resolved through the package's `exports` map as `settings[].component` is, whose default export takes `ReportPanelProps`. Emitted to `lib/generated/report-panels.ts`. See [Shipped and saved reports](#shipped-and-saved-reports). |
```

"Report sources", after the `Field.TextDay` bullet:

```markdown
- A `Select` field may read a text column as well as a select column. Its
  `Options` give the labels; a stored value that no option names shows as
  itself. `CheckSources` checks the options against the values only of a
  select column (a text column has none).
- A source may read a view collection. PocketBase types a bare column of the
  view's query as its source field (an autodate `created` reads as `DateTime`),
  a `CAST(… AS TEXT)` column as text, and any other computed column as json,
  which no field reads: cast every computed column a field reads, for example
  `CAST(CASE WHEN f.grams >= 500 THEN 'big' ELSE 'small' END AS TEXT) AS size`.
  A run reads the view under its list rule. Register the view's sources with
  `changesignal.RegisterView`, so an open report stays current.
- A `DateTime` field holds an instant. A date range (`between` with a
  `dateRange` param), a `date` or `asOf` param, a range's `bound`, and
  a day typed in a condition (a literal `{"value": "2026-11-14"}`, such as a
  builder filter's value) compare with it in the reader's time zone: the
  client sends `options.timeZone` (an IANA name, from `Intl`) with every run
  and `timeZone` with every param-options request, and the engine compares
  with the UTC instants of the reader's midnights
  (`x >= <first day's midnight> AND x < <midnight after the last day>`). A
  missing or unknown zone, a name of UTC itself (`UTC`, `Etc/UTC`,
  `Etc/GMT`, `GMT`) and `Local` are UTC. A condition that compares a
  `DateTime` with a `Date` column still compares in UTC. Buckets and date parts of a
  `DateTime` (`year`, `month`, a group or pivot by day) keep the UTC day. A
  `Date` field is a day and never moves. The view shows a `DateTime` cell as
  day and time in the reader's zone and names the zone once under the title
  ("Times in Central Time"); a package screen writes the same text with
  `dateTimeText(value)` from `@tinycld/core/reports/lib/format`.
```

"Shipped and saved reports", after the drilled-figure bullet:

```markdown
- A detail row's record (`detail.open`) can open beside the report in the
  same 480 px panel. Declare the panel in the manifest:
  `reportPanels: [{ path: 'history', component: 'components/history/RecordHistoryPanel' }]`.
  A row whose `open` is `<slug>/<path>/` and at least one more segment opens
  in the panel when the window is 1360 px or wider and the screen hosts a
  panel; the component gets `ReportPanelProps` (`{ segments }`: the segments
  after `<slug>/<path>/`) and names the record with `usePanelTitle(title)`
  (both from `@tinycld/core/reports/hooks/usePanelTitle`; outside the panel
  the hook does nothing). Below 1360 px, and on a phone, the row pushes the
  path as before, so the package also serves the path as a screen, usually
  with the same component. The panel's header has **Open as full screen**
  (pushes the path), ✕ and Escape. Its state is in the source's URL
  (`dr=<app path>`), so a reload keeps it; a copied link never holds it. A
  drill and a record share the side: opening one closes the other, and
  Customize closes both. The pressed row is marked (`aria-selected`).
```

Section D (`lib/generated/*`), after the `package-icons.ts` bullet:

```markdown
- **`report-panels.ts`** — every installed package's `reportPanels`, as
  `{ slug, path, load }` (`load` imports the component). Only
  `core/reports/lib/registered-panels.ts` reads it.
```

- [ ] **Step 5: Run the doc tests**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run core/reports/lib/__tests__/statements-docs.test.ts`
Expected: PASS (9 tests).

- [ ] **Step 6: Regenerate the help**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld && pnpm run packages:generate` (not while an e2e run is in progress).
Expected: exits 0; `command grep -c 'Dates with a time' lib/generated/package-help.ts` prints at least `1`.

- [ ] **Step 7: Hand the diff to the driver** (no commit).

---

### Task 10: E2E stub

**Files:**
- Modify: `tests/scripts/reports-stub/server/register.go.txt` (sales field `created`; shipped `salesstub.by_time`; `panelOpen`; header comment)
- Create: `tests/scripts/reports-stub/components/record-panel.tsx.txt`
- Create: `tests/scripts/reports-stub/screens/panel/[id].tsx.txt`
- Modify: `tests/scripts/scaffold-reports-stub.ts` (TEMPLATES; exports `./components/*`; manifest `reportPanels`; header comment)
- Modify: `tests/e2e/reports-helpers.ts` (`ZONE_TITLE`, `recordPanel`, header comment)
- Test: `tests/unit/scaffold-reports-stub.test.ts`

The migration `tests/scripts/reports-stub/pb-migrations/2090000300_salesstub.js.txt` does **not** change: `salesstub_sales` already has the autodate `created` (line 10, `stamps`).

**Interfaces:**
- Consumes: `ReportPanelProps`, `usePanelTitle` (`@tinycld/core/reports/hooks/usePanelTitle`, Tasks 6 and 8); manifest `reportPanels` and the generator (Task 6); `reports.DateTime` over an autodate column (core `check.go` `compatible[DateTime]`).
- Produces: sales source field `created` (`reports.DateTime`, `MinMax`); shipped report `salesstub.by_time` "Stub sales by time" (published, Revision 1); panel path `panel` (`salesstub/panel/<id>`); component test ids `stub-record-panel`, `stub-record-panel-body`; screen `salesstub/panel/[id]` (test id `stub-panel-screen`); reports-helpers `ZONE_TITLE = 'Stub sales by time'`, `recordPanel(page: Page): Locator`.

- [ ] **Step 1: Write the failing unit test**

`tests/unit/scaffold-reports-stub.test.ts`, in `it('re-writes a member stub from the templates', …)`, after `expect(readStub('tinycld/salesstub/screens/sale/[id].tsx')).toBe(saleScreen)`:

```ts
        const recordPanel = fs.readFileSync(
            path.join(TEMPLATES, 'components', 'record-panel.tsx.txt'),
            'utf8'
        )
        expect(readStub('tinycld/salesstub/components/record-panel.tsx')).toBe(recordPanel)
        const panelScreen = fs.readFileSync(
            path.join(TEMPLATES, 'screens', 'panel', '[id].tsx.txt'),
            'utf8'
        )
        expect(readStub('tinycld/salesstub/screens/panel/[id].tsx')).toBe(panelScreen)
        expect(readStub('manifest.ts')).toContain(
            "reportPanels: [{ path: 'panel', component: 'components/record-panel' }]"
        )
        const pkg = JSON.parse(readStub('package.json'))
        expect(pkg.exports['./components/*']).toBe('./tinycld/salesstub/components/*.tsx')
        expect(template).toContain('Key: "salesstub.by_time"')
        expect(template).toContain(
            '{Key: "created", Label: "Created", Type: reports.DateTime, Aggregates: reports.MinMax}'
        )
```

- [ ] **Step 2: Run it and see it fail**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run tests/unit/scaffold-reports-stub.test.ts`
Expected: FAIL: `ENOENT: no such file or directory, open '…/reports-stub/components/record-panel.tsx.txt'`.

- [ ] **Step 3: The panel component and the panel screen**

Create `tests/scripts/reports-stub/components/record-panel.tsx.txt`:

```tsx
import { type ReportPanelProps, usePanelTitle } from '@tinycld/core/reports/hooks/usePanelTitle'
import { Text, View } from 'react-native'

// E2E fixture: the record panel a "Stub sales by time" row opens beside the report
// (manifest reportPanels, path "panel"); the stub's panel screen shows the same
// component on its own, where usePanelTitle does nothing.
export default function RecordPanel({ segments }: ReportPanelProps) {
    usePanelTitle(`Stub record ${segments[0] ?? ''}`)
    return (
        <View testID="stub-record-panel" className="flex-1 bg-background p-4">
            <Text testID="stub-record-panel-body" className="text-base text-foreground">
                {segments.join('/')}
            </Text>
        </View>
    )
}
```

Create `tests/scripts/reports-stub/screens/panel/[id].tsx.txt`:

The screen imports the panel by a relative path, not `~/tinycld/salesstub/components/record-panel`. The stub's tsconfig self-alias serves only tsc: Metro resolves `~/*` through the app's own tsconfig (`~/*` → `/Users/nas/tmp/tinycld/books-reports/tinycld/*`), where no `tinycld/salesstub/` exists. No other stub file imports a sibling file, and the Books package's screens import their own files by relative paths for the same reason.

```tsx
import { useLocalSearchParams } from 'expo-router'
import { View } from 'react-native'
import RecordPanel from '../../components/record-panel'

// E2E fixture: the record's own screen. A row press pushes it below 1360 px, and the
// record panel's Open as full screen pushes it on a wide window.
export default function StubPanelScreen() {
    const { id } = useLocalSearchParams<{ id: string }>()
    return (
        <View testID="stub-panel-screen" className="flex-1 bg-background">
            <RecordPanel segments={[id]} />
        </View>
    )
}
```

- [ ] **Step 4: The scaffolder**

`tests/scripts/scaffold-reports-stub.ts`:
- `TEMPLATES`, after the `screens/sale/[id].tsx.txt` entry:

```ts
    ['screens/panel/[id].tsx.txt', `tinycld/${STUB_SLUG}/screens/panel/[id].tsx`],
    ['components/record-panel.tsx.txt', `tinycld/${STUB_SLUG}/components/record-panel.tsx`],
```

- `writePackage`, `exports`, after `'./screens/*': …`:

```ts
            './components/*': `${src}/components/*.tsx`,
```

- `writeManifest`, after the `server: …` line of the template string:

```ts
    reportPanels: [{ path: 'panel', component: 'components/record-panel' }],
```

- Header comment: after "Its sale screen is the record a report's detail row opens (`detail.open`)." add: "Its record panel (`reportPanels`, path `panel`) is the record a "Stub sales by time" row opens beside the report, and its panel screen the same record pushed on a narrow window."

- [ ] **Step 5: The Go source**

`tests/scripts/reports-stub/server/register.go.txt`:
- In the `salesstub.sales` source's `Fields`, after the `booked` field:

```go
			// The record's own autodate, an instant (T1, T2).
			{Key: "created", Label: "Created", Type: reports.DateTime, Aggregates: reports.MinMax},
```

- Before `return nil` in `Register`:

```go
	// A published report over the sales' date-time `created`
	// (tests/e2e/reports-zone-record-panel.spec.ts): its period compares instants
	// in the reader's time zone, its cells show day and time there, and a row opens
	// the stub's record panel (manifest reportPanels, path "panel") beside the report.
	reports.RegisterShipped(reports.Shipped{
		Key: "salesstub.by_time", Title: "Stub sales by time", Revision: 1,
		Description: "Each sale in a period with the time it was entered.",
		Spec: json.RawMessage(strings.ReplaceAll(`{
			"version": 4,
			"source": "salesstub.sales",
			"params": [{"id": "period", "type": "dateRange", "label": "Period", "default": {"preset": "thisMonth"}}],
			"columns": [
				{"id": "created", "label": "Created", "path": [{"field": "created"}]},
				{"id": "customer", "label": "Customer", "path": [{"field": "customer"}]},
				{"id": "amount", "label": "Amount", "path": [{"field": "amount"}]}
			],
			"filter": {"op": "between", "args": [{"path": [{"field": "created"}]}, {"param": "period"}]},
			"sort": [{"col": "created", "dir": "desc"}, {"formula": {"path": [{"field": "id"}]}}],
			"detail": {"label": {"path": [{"field": "created"}]}, "open": PANEL_OPEN}
		}`, "PANEL_OPEN", panelOpen)),
	})
```

- After the `saleOpen` const at the end of the file:

```go

// panelOpen is the stub record a "Stub sales by time" row opens: beside the report
// on a wide window (manifest reportPanels, path "panel"), else as a screen.
const panelOpen = `{"op": "concat", "args": [{"value": "salesstub/panel/"}, {"path": [{"field": "id"}]}]}`
```

- File header comment: after "Its drill reports' figures open another shipped report." add " Its sales carry their date-time `created`, and one report's rows open the stub's record panel."

- [ ] **Step 6: The e2e helpers**

`tests/e2e/reports-helpers.ts`:
- Header comment: in the sales source's field list, after "booked (a day kept as text)," add " created (a date-time),"; at the end, add: "The published "Stub sales by time" shows `created` and opens each row in the stub's record panel (path `panel`)."
- After `export const DRILL_TARGET_TITLE = 'Stub sales by team'`:

```ts

/** The zone report (register.go.txt `salesstub.by_time`): a date-time column; its rows open the stub's record panel. */
export const ZONE_TITLE = 'Stub sales by time'
```

- After `drillPanel`:

```ts

/** The record panel (D5) a detail row opens beside the report. */
export function recordPanel(page: Page): Locator {
    return page.getByTestId('report-record-panel')
}
```

- [ ] **Step 7: Run the unit test**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/vitest run tests/unit/scaffold-reports-stub.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 8: Apply the stub to the workspace and generate (C4)**

Run (not while an e2e run is in progress): `cd /Users/nas/tmp/tinycld/books-reports && pnpm exec tsx tinycld/tests/scripts/scaffold-reports-stub.ts`
Expected: ends with `[scaffold-reports-stub] done`; no `pnpm install` (the stub is already a member). Then:
- `command grep -n "salesstub" /Users/nas/tmp/tinycld/books-reports/tinycld/lib/generated/report-panels.ts` — prints one entry with `path: 'panel'` and `import('@tinycld/salesstub/components/record-panel')`.
- `ls "/Users/nas/tmp/tinycld/books-reports/tinycld/app/a/(app)/salesstub/panel/"` — lists `[id].tsx`.
- `cd /Users/nas/tmp/tinycld/books-reports/tinycld/server && go vet tinycld.org/packages/salesstub/...` — no output (the stub's Go compiles).

- [ ] **Step 9: Check the other stub specs for the new field and the new report**

The sales source now lists nine fields (customer, amount, cost, quantity, currency, day, booked, created, status) and its link `team`. The stub now ships one more published report, "Stub sales by time".

Run: `command grep -rn "Created\|'created'\|fields).toHaveLength\|toHaveCount(10)\|Stub sales\|reports-hub-row" /Users/nas/tmp/tinycld/books-reports/tinycld/tests/e2e/reports-*.spec.ts | command head -40`.

For each hit that counts or lists the sales source's fields, picks a source's first date field, or lists, counts or picks by position the stub's shipped or published reports in the hub, update it for `created` or for "Stub sales by time"; write each change into this task's diff and the report.

The hits at main 9cc926c0, each checked when this plan was written (none changes; re-check them against the tree as it is):
- the `Created` / `'created'` / field-count / `toHaveCount(10)` part: no hit.
- `reports-create.spec.ts:78` and `:174` — `toContainText('Stub sales')` on a new report's hub row: the source's label, not a report list. No change.
- `reports-drill.spec.ts:25`, `reports-failure-states.spec.ts:34` — comments naming "Stub sales by team". No change.
- `reports-formula-leaves.spec.ts:144`, `reports-ledger-v3.spec.ts:539` — formula words that name the source "Stub sales". No change.
- `reports-share.spec.ts:63` — `hubRow(page, title).first()` for a unique title; `:184` — the hub row actions of one row. No change.
- `reports-shipped.spec.ts:35` — the menu of `hubRow(page, SHIPPED_TITLE)` ("Stub larger side"). No change.

`hubRow(page, title)` (`tests/e2e/report-view-helpers.ts:13`) filters the hub rows by `hasText: title`. No shipped title ("Stub larger side", "Stub team totals", "Stub team balance", "Stub statement", "Stub period statement", "Stub period pivot", "Stub sales by team", "Stub drilled sales", "Stub drilled by month") is part of "Stub sales by time", so the new row matches none of them. `reports-share.spec.ts:271` counts the stub's sources (`reports-new-source-*`), not its reports, and the new report adds no source. No spec counts the hub's published rows.

- [ ] **Step 10: Typecheck and biome**

Run: `PATH=/Users/nas/tmp/tinycld/books-reports/node_modules/.bin:$PATH tinycld-pkg typecheck` and `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check tests/scripts/scaffold-reports-stub.ts tests/unit/scaffold-reports-stub.test.ts tests/e2e/reports-helpers.ts /Users/nas/tmp/tinycld/books-reports/salesstub/tinycld/salesstub/components/record-panel.tsx "/Users/nas/tmp/tinycld/books-reports/salesstub/tinycld/salesstub/screens/panel/[id].tsx"`
Expected: no errors (the two stub files are the templates as copied, so a biome error there is fixed in the `.txt` template and Step 8 run again).

- [ ] **Step 11: Hand the diff to the driver** (no commit).

---

### Task 11: E2E `reports: zone times and record panel`

**Files:**
- Create: `tests/e2e/reports-zone-record-panel.spec.ts`

**Interfaces:**
- Consumes: Task 10 (`ZONE_TITLE`, `recordPanel`, the stub report, panel and screen); reports-helpers `addSale`, `addTeam`, `openDrilled`, `openReportStub`, `pressToolbar`, `superuserClient`, `uniqueName`; helpers `inStatement`, `login`, `skipWithoutReportsStub`; Task 3's `report-document-zone`; Task 8's test ids.
- Produces: the spec, nothing else.

The build order puts this task after Tasks 2–10, so the spec runs green against the built code at once. Step 3 then proves that each of its assertions bites: it breaks one part at a time (one mutation for each assertion of the zone line, the 480 px width, the title, the mark, ✕, Escape, the focus back to the row, the reload, Copy link, Customize and Open as full screen) and sees the named assertion fail for the named reason. Those runs are this checkpoint's "test written first and seen red" for Task 3's view wiring and Task 8's view.

- [ ] **Step 1: Write the spec**

Create `tests/e2e/reports-zone-record-panel.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { inStatement, login, skipWithoutReportsStub } from './helpers'
import {
    addSale,
    addTeam,
    openDrilled,
    openReportStub,
    pressToolbar,
    recordPanel,
    superuserClient,
    uniqueName,
    ZONE_TITLE,
} from './reports-helpers'

/**
 * Date-time cells in the reader's time zone (T1) and a detail row's record in the
 * side panel (D5), on the stub's "Stub sales by time"
 * (tests/scripts/reports-stub/server/register.go.txt), whose rows open the stub's
 * record panel (path `panel`). The browser reads America/Chicago; the server is UTC.
 * A sale's `created` is set by the server, so the expected text is read back with a
 * read-only GET and written here in Node; the edges of local days are pinned in Go
 * (core/server/reports/zonerange_test.go), not here.
 */

const WIDE = { width: 1440, height: 900 }
const NARROW = { width: 1280, height: 900 }
const ZONE = 'America/Chicago'
// The report filters on `created`, not `day`: a day no other spec uses keeps these
// sales out of every other report.
const SALE_DAY = '2047-07-07'

interface Sale {
    id: string
    created: string
}

/** Adds `count` sales through the stub's screen and reads back their ids and `created`. */
async function seedSales(page: Page, count: number): Promise<Sale[]> {
    await openReportStub(page)
    const team = uniqueName('Zone Team')
    await addTeam(page, team, 'east')
    const customers = Array.from({ length: count }, (_, i) => uniqueName(`Zone ${i}`))
    for (const customer of customers) {
        await addSale(page, { team, customer, amount: 100, cost: 0, day: SALE_DAY })
    }
    // Read-only: the server sets `created`, which the cells must show in the zone.
    const pb = await superuserClient()
    return Promise.all(
        customers.map(async customer => {
            const sale = await pb
                .collection('salesstub_sales')
                .getFirstListItem(pb.filter('customer = {:customer}', { customer }))
            return { id: sale.id, created: String(sale.created) }
        })
    )
}

/**
 * The text a reader in Chicago sees for an instant. Node's ICU writes a narrow
 * no-break space before AM/PM; the app writes a plain space.
 */
function chicagoText(created: string): string {
    return new Date(created.replace(' ', 'T'))
        .toLocaleString('en-US', {
            timeZone: ZONE,
            month: 'short',
            day: 'numeric',
            year: 'numeric',
            hour: 'numeric',
            minute: '2-digit',
        })
        .replace(/\s+/g, ' ')
}

/** A detail row's label: its record link, named by its id. */
function rowLabel(page: Page, sale: Sale) {
    return page.getByTestId(`report-row-d:${sale.id}`)
}

function markedRows(page: Page) {
    return page.locator('[data-testid^="report-row-d:"][aria-selected="true"]')
}

function param(page: Page, key: string): string | null {
    return new URL(page.url()).searchParams.get(key)
}

test.describe('reports: zone times and record panel', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')
    test.use({ timezoneId: ZONE, locale: 'en-US' })

    test("date-time cells and the zone line are in the reader's time zone", async ({ page }) => {
        await login(page)
        const sales = await seedSales(page, 2)
        await page.setViewportSize(WIDE)
        await openDrilled(page, ZONE_TITLE)
        for (const sale of sales) {
            await expect(rowLabel(page, sale)).toHaveText(chicagoText(sale.created))
        }
        await expect(page.getByTestId('report-document-zone')).toHaveCount(1)
        await expect(inStatement(page, 'report-document-zone')).toHaveText('Times in Central Time')
    })

    test('on a wide window a row opens its record beside the report', async ({ page }) => {
        await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
        await login(page)
        const [a, b] = await seedSales(page, 2)
        await page.setViewportSize(WIDE)
        await openDrilled(page, ZONE_TITLE)
        const panel = recordPanel(page)
        const title = panel.getByTestId('report-record-panel-title')

        // The press opens the panel beside the report, named by the package's panel.
        await rowLabel(page, a).click()
        await expect(panel).toBeVisible()
        await expect.poll(() => panel.evaluate(el => el.getBoundingClientRect().width)).toBe(480)
        await expect(title).toHaveText(`Stub record ${a.id}`)
        await expect(panel.getByTestId('stub-record-panel-body')).toHaveText(a.id)
        expect(param(page, 'dr')).toBe(`salesstub/panel/${a.id}`)
        await expect(rowLabel(page, a)).toHaveAttribute('aria-selected', 'true')
        await expect(markedRows(page)).toHaveCount(1)

        // Another row replaces the record; the mark moves with it.
        await rowLabel(page, b).click()
        await expect(title).toHaveText(`Stub record ${b.id}`)
        await expect(rowLabel(page, b)).toHaveAttribute('aria-selected', 'true')
        await expect(markedRows(page)).toHaveCount(1)

        // ✕ closes it, and the focus goes back to the row's label.
        await panel.getByTestId('report-record-panel-close').click()
        await expect(panel).toHaveCount(0)
        expect(param(page, 'dr')).toBeNull()
        await expect(rowLabel(page, b)).toBeFocused()
        await expect(markedRows(page)).toHaveCount(0)

        // Escape closes it too.
        await rowLabel(page, b).click()
        await expect(panel).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(panel).toHaveCount(0)
        await expect(rowLabel(page, b)).toBeFocused()

        // A reload keeps the panel (it is in the URL); no row is marked after it (Q1).
        await rowLabel(page, b).click()
        await expect(title).toHaveText(`Stub record ${b.id}`)
        await page.reload()
        await expect(panel).toBeVisible()
        await expect(title).toHaveText(`Stub record ${b.id}`)
        await expect(rowLabel(page, b)).toBeVisible()
        await expect(markedRows(page)).toHaveCount(0)

        // A copied link never holds the panel.
        await pressToolbar(page, 'report-copy-link', 'Copy link')
        const link = new URL(await page.evaluate(() => navigator.clipboard.readText()))
        expect(link.searchParams.has('dr')).toBe(false)

        // Customize closes the panel: one side surface at a time (B7).
        await pressToolbar(page, 'report-customize-open', 'Customize')
        await expect(panel).toHaveCount(0)
        await expect.poll(() => param(page, 'dr')).toBeNull()

        // A row press opens the record again (and closes Customize).
        await rowLabel(page, b).click()
        await expect(title).toHaveText(`Stub record ${b.id}`)

        // Open as full screen pushes the record's own screen.
        await panel.getByTestId('report-record-panel-full').click()
        await expect(page.getByTestId('stub-panel-screen')).toBeVisible()
        await expect
            .poll(() => new URL(page.url()).pathname)
            .toMatch(new RegExp(`/salesstub/panel/${b.id}$`))
        const screen = page.getByTestId('stub-panel-screen')
        await expect(screen.getByTestId('stub-record-panel-body')).toHaveText(b.id)
    })

    test('below 1360 px a row opens its record as a screen', async ({ page }) => {
        await login(page)
        const [sale] = await seedSales(page, 1)
        await page.setViewportSize(NARROW)
        await openDrilled(page, ZONE_TITLE)
        await rowLabel(page, sale).click()
        await expect(page.getByTestId('stub-panel-screen')).toBeVisible()
        await expect
            .poll(() => new URL(page.url()).pathname)
            .toMatch(new RegExp(`/salesstub/panel/${sale.id}$`))
        await expect(recordPanel(page)).toHaveCount(0)
    })
})
```

- [ ] **Step 2: Run it**

Run: `cd /Users/nas/tmp/tinycld/books-reports/tinycld && { lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7220|7221) ' && { echo PORTS-BUSY; exit 1; }; } ; E2E_PORT=7220 E2E_PORT_2=7221 E2E_IMAP_PORT=1220 E2E_IMAP_PORT_2=1221 pnpm exec tinycld-pkg test:e2e -- reports-zone-record-panel.spec.ts`
Expected: `3 passed`. A failure is fixed at its source (the core part or the stub, never the spec's waits). If the run takes more than about 4 minutes, hand back `NEEDS-RUN:` with this command.

- [ ] **Step 3: Prove the assertions bite**

Break one part at a time with the Edit tool, run the Step 2 command, see the named failure, and undo the edit with the Edit tool before the next one:

1. `core/reports/lib/record-panel.ts`, `rowOpensInPanel`: change `if (!input.open || !input.hasPanel || input.surface !== 'panel') return false` to `if (!input.open) return false` → test 3 fails: `report-record-panel` count 1 (or `stub-panel-screen` not visible).
2. `core/reports/components/ReportLinks.tsx`, `OpenRowLabel`: change `aria-selected={props.isShown || undefined}` to `aria-selected={undefined}` → test 2 fails at the first `await expect(rowLabel(page, a)).toHaveAttribute('aria-selected', 'true')`. (The id half of `isRowShown`, many rows that open one record, is pinned by Task 7's unit test, and the title of such a press by Task 8's store test: this spec's rows open two records.)
3. `core/reports/lib/view-url.ts`, `clearedDrillKeys`: remove `key === RECORD_KEY || ` → test 2 fails at the `await expect(panel).toHaveCount(0)` after `report-record-panel-close` (✕ leaves `dr`).
4. `core/reports/hooks/useReportView.ts`, the model: change `zoneWords: zoneLine(columns),` to `zoneWords: null,` → test 1 fails at `await expect(page.getByTestId('report-document-zone')).toHaveCount(1)` (count 0). This is Task 3's view wiring seen red.
5. `core/reports/components/ReportDrillPanel.tsx`: change `const PANEL_STYLE = { width: DETAIL_PANEL_WIDTH, borderLeftWidth: 1 } as const` to `const PANEL_STYLE = { width: DETAIL_PANEL_WIDTH + 40, borderLeftWidth: 1 } as const` → test 2 fails at `await expect.poll(() => panel.evaluate(el => el.getBoundingClientRect().width)).toBe(480)` (520).
6. `core/reports/hooks/usePanelTitle.ts`, `usePanelTitle`: change `if (open !== null) setRecordTitle(open, title)` to `if (open === null) return` (the hook names nothing) → test 2 fails at the first `await expect(title).toHaveText(\`Stub record ${a.id}\`)` (empty title).
7. `core/reports/components/ReportDrillPanel.tsx`, `RecordPanel`: delete the line `<LayerEscape onEscape={panel.onClose} />` → test 2 fails at the `await expect(panel).toHaveCount(0)` after `page.keyboard.press('Escape')`.
8. `core/reports/components/ReportDrillPanel.tsx`, `ReportDrillPanel`: add `import { useReportUIStore } from '../stores/report-ui-store'` and change `if (open) return <RecordPanelHost open={open} sourceTitle={sourceTitle} />` to `if (open && useReportUIStore.getState().openedRow !== null) return <RecordPanelHost open={open} sourceTitle={sourceTitle} />` (the panel follows the press held in memory, not the route's `dr`) → test 2 fails at the `await expect(panel).toBeVisible()` after `page.reload()`.
9. `core/reports/hooks/useDrillPanel.ts`, `useRecordPanel`: change `onOpenFull: () => openScreen(open),` to `onOpenFull: () => {},` → test 2 fails at the `await expect(page.getByTestId('stub-panel-screen')).toBeVisible()` after `report-record-panel-full`.
10. `core/reports/hooks/useDrillPanel.ts`, `useRecordPanel`'s `onClose`: delete the two lines `const row = useReportUIStore.getState().openedRow` and `focusFigure(row === null ? null : rowFocusKey('main', row))` → test 2 fails at the first `await expect(rowLabel(page, b)).toBeFocused()` (after ✕).
11. `core/reports/lib/view-url.ts`, `reportLink`: after the `for (const [key, value] of Object.entries(serializeViewOptions(defs, options))) { … }` loop add `query.set(RECORD_KEY, 'x')` → test 2 fails at `expect(link.searchParams.has('dr')).toBe(false)`. (Task 7 Step 5 already saw the view-url vitest fail with the same mutation.)
12. `core/reports/hooks/useReportView.ts`, the Customize open (line 363 at main 9cc926c0): delete `if (scope === 'main') router.setParams(clearedDrillKeys(search))` → test 2 fails at the `await expect(panel).toHaveCount(0)` after `report-customize-open`, or, if Customize takes the side slot and so hides the panel, at `await expect.poll(() => param(page, 'dr')).toBeNull()`.

Each mutation is undone with the Edit tool before the next one; after the last undo, `git -C /Users/nas/tmp/tinycld/books-reports/tinycld diff --stat` shows only the task's own changes, and the Step 2 command passes again (`3 passed`). Write each mutation, its failing line (the spec line and Playwright's first error line) and the undo into the report. A mutation whose run passes is a test that does not bite: strengthen the assertion (never the mutation) and run it again. These twelve runs together take more than 4 minutes: hand back `NEEDS-RUN:` with the Step 2 command and the twelve mutations listed, and what is needed from each run (the failing test and its first error line).

- [ ] **Step 4: Biome**

Run: `/Users/nas/tmp/tinycld/books-reports/node_modules/.bin/biome check tests/e2e/reports-zone-record-panel.spec.ts`
Expected: no errors.

- [ ] **Step 5: The checkpoint's checks (brief Do 3)**

From `/Users/nas/tmp/tinycld/books-reports/tinycld`:
- `pnpm run checks` — Expected: passes (lint, typecheck, core isolation, fork drift).
- `cd core/server && go test ./reports/...` — Expected: `ok`.
- From `/Users/nas/tmp/tinycld/books-reports/tinycld/core`: `pnpm exec tinycld-pkg test` — Expected: all pass.
- From `/Users/nas/tmp/tinycld/books-reports/tinycld`: `pnpm exec vitest run scripts/__tests__/gen-report-panels.test.ts tests/unit/scaffold-reports-stub.test.ts` — Expected: all pass.

Then the core report e2e, one file at a time, each with the port check and the lane's ports:

```bash
cd /Users/nas/tmp/tinycld/books-reports/tinycld && for spec in reports-zone-record-panel reports-column-fit reports-create reports-drill reports-failure-states reports-filter reports-fit reports-formula-layout reports-formula-leaves reports-formulas-v2 reports-group-pivot reports-groups-v2 reports-ledger-v3 reports-paged-groups reports-period-pivot reports-scroll-anchor reports-share reports-shipped reports-source-params reports-statements reports-tab-order reports-union-blank-above reports-view reports-wrap reports-text-width; do { lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7220|7221) ' && { echo PORTS-BUSY; exit 1; }; } ; E2E_PORT=7220 E2E_PORT_2=7221 E2E_IMAP_PORT=1220 E2E_IMAP_PORT_2=1221 pnpm exec tinycld-pkg test:e2e -- "$spec.spec.ts" || echo "FAILED: $spec"; done
```

These runs together take well over 4 minutes: hand back `NEEDS-RUN:` with this command and what is needed from it (every file passes; for a `FAILED:` line, the failure's first error and the spec line). Each failure is fixed at its source, in this checkpoint.

- [ ] **Step 6: Hand the diff to the driver** (no commit).

---

## Self-review

- **Coverage (Part 2's rows of the outline's coverage table):** `useRowLinks`' decision — Task 7 `rowOpensInPanel` tests, used by Task 8 `useOpenRecordPanel`; `reportLink` never holds `dr` — Task 7 view-url test; opening a record clears the drill keys and closes Customize, opening a drill clears `dr` — Task 7 (`recordPanelParams`, `drillPanelParams` with `dr`), Task 8 (`openRecord` closes Customize; `onOpenCustomize` clears `dr` through `clearedDrillKeys`, which Task 11 test 2 proves with a Customize press and Step 3 mutation 12); the title of a record that a second row opens — Task 8 store test; a failed chunk loads again — Task 8 `reloadableChunk` test; docs and help — Task 9; stub — Task 10; e2e 1440 cells and zone line, panel (480 px, title, mark, replace, ✕, Escape, reload, copy link, Customize, Open as full screen), 1280 push — Task 11, each assertion seen red by one Step 3 mutation.
- **Names:** `RECORD_KEY`, `RecordPanelMatch`, `recordPanelOf`, `rowOpensInPanel`, `recordPanelParams`, `isRowShown`, `registeredPanels`, `reloadableChunk`, `usePanelTitle`, `PanelTitleProvider`, `openedRow`, `openRecord`, `recordOpens`, `recordFocused`, `noteRecordFocused`, `RecordTitle`, `recordTitle`, `setRecordTitle`, `recordTitleOf`, `useRecordPanel`, `RecordPanelModel`, `useIsDrillPage`, `ZONE_TITLE`, `recordPanel` and every test id match the outline; the additions are C1–C3, C6 and C7.
