# Report Engine Scale, Part 9: Refresh a Report That Is Not Live (core client) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a run is manual (`"manual":true` from the server), the report shows a note that it does not update by itself and a Refresh button with the shortcut R, on web and native.

**Architecture:** The run result's `manual` mark becomes `RunView.isManual`. `useReportRun` adds `refresh()` (a refetch that waits as a live rerun does) and `isRefreshing`. One new component, `ReportManualBand`, draws the note and the button under the chips. Three surfaces show it: the report view, the drill panel, and the Preview of the builder's sheet on a phone (that sheet draws its own preview, so the view's band is under it). The band is the same code at every width. Only the report view's button has the shortcut.

**Tech Stack:** TypeScript, React Native + react-native-web, TanStack Query (`useInfiniteQuery`), vitest, `ShortcutButton` (`@tinycld/core/ui/shortcut-button`), `useShortcutScope` (`@tinycld/core/lib/shortcuts/scopes`).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part7.md` (section "Design proposed", items 4 and 6), the outline `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (task 8b), part 8 of this plan (`tinycld/docs/plans/2026-11-20-report-engine-scale-part8.md`, its Interfaces block), and the human's decision of 2026-11-20 in `/Users/nas/code/tinycld/docs/plans/auto-helix/p5-report-engine-scale.md`.

Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Core is `tinycld/`, on the branch `books/p5-report-engine-scale`. This part changes core only and holds ONE task (8b). Tasks 8c (Books declares the reports that are not live, the Go test that pins them, Books help) and 8d (Books e2e of the note and the button) are the next plan part. Task 8a (part 8) must be built first: without it no run is manual.

## Global Constraints

- Core names no package: no Books slug, source key, collection name or report title in `tinycld/core` (the help text too).
- Native and web both: the band is plain React Native (`View`, `Text`, `Button`). The shortcut works with a hardware keyboard (web, iPad); the shortcut hint and tooltip of `ShortcutButton` are web only (as for every `ShortcutButton`).
- No raw hex; semantic Tailwind tokens only (`text-muted-foreground`). No `useEffect`. No `any`. No `biome-ignore`.
- JSX stays minimal: no ternary or `.map()` in a `return`; a component that must not render returns `null`.
- The note's text, exactly: `This report does not update by itself. Press Refresh to see new changes.`
- Test ids, exactly: `report-manual-band`, `report-manual-note`, `report-refresh` (the report); `report-drill-manual-band`, `report-drill-manual-note`, `report-drill-refresh` (the drill panel); `report-builder-manual-band`, `report-builder-manual-note`, `report-builder-refresh` (the Preview of the builder's sheet on a phone). Task 8d uses them.
- While a Refresh runs, the button is disabled and reads `Refreshing…`; every "Updating…" slot that reads the view model (the header's, the builder's) shows `Updating…`. The drill panel has no such slot: there the button's text is the only sign.
- The shortcut is the key `r`, scope `list`, with no modifier, not in inputs.
- Front-end behavior is tested by e2e, not by component unit tests (CLAUDE.md). Core has no source that is not live, so the e2e of the band is task 8d, in Books. This task's unit tests are of the hook and of the pure state function only.
- Run only the named tests and `tinycld-pkg typecheck`. The driver runs the full checks. Never run `pnpm install` or `packages:generate`.

## Review Focus

- Two presses of Refresh (or a held R key) while the first run is in flight: one request, not two (`Refresh runs the report again, once for two presses…`, step 1).
- Refresh on a report with more than one page loaded: the pages stay, and the reader keeps the place (`Refresh keeps the pages loaded and the manual mark`, step 1).
- A manual run that is then refused (the report lost its source, access was removed): no band, because there are no figures to refresh (`says a run is manual only when its figures carry the mark`, step 1).
- A manual report opened in the drill panel beside a manual report: each has its own Refresh; only the report's own button takes the key R, so one key never runs two reports (the code of step 3; e2e in task 8d).
- A draft over a source that is not live, built on a phone: the builder's sheet draws its own preview, so the band is added there too, with no shortcut (the sheet is a modal scope); task 8d runs it at a phone width, because this is different code (`SheetSurface`).
- A Refresh while the builder lies over the view: the view's "Updating…" slot is silent then, and the builder's slot reads the model's `isUpdating`, so the model's `isUpdating` carries `isRefreshing` (step 3, `useReportView.ts`).
- The reader types the letter r in a filter's search box: nothing runs, because the shortcut does not set `allowInInputs` (`useButtonShortcut`; not tested here, the shortcut matcher has its own tests in `core/tests/unit/shortcuts.test.ts`).

Known and accepted (the same as every list screen): a plain menu pushes no shortcut scope, so R with a chip menu or "⋯" open runs Refresh behind it; a dialog or sheet is a `modal` scope, so R does nothing under it.

Not tested, by design: the look of the band (task 8d reads the note and the button in a browser); a failed Refresh keeps the figures in view, as a failed live rerun does (`viewState` in `useReportView.ts`, unchanged).

---

### Task 8b: A manual run shows a note and Refresh

**Files:**
- Create: `tinycld/core/reports/components/ReportManualBand.tsx`
- Modify: `tinycld/core/reports/lib/types.ts` (the interface `RunResult`, after `topics` at line 189)
- Modify: `tinycld/core/reports/lib/run-state.ts` (the interface `RunView` at line 26; `runView` at line 73)
- Modify: `tinycld/core/reports/hooks/useReportRun.ts` (the interface `ReportRun` at line 30; the end of `useReportRun`, lines 226-237)
- Modify: `tinycld/core/reports/components/ReportView.tsx` (imports; `ReportTopProps` and `ReportTop` at lines 196-236; `ReportView` at line 260)
- Modify: `tinycld/core/reports/components/ReportDrillPanel.tsx` (imports at line 26; `DrillReport` at line 192)
- Modify: `tinycld/core/reports/components/builder/ReportBuilderPanel.tsx` (imports at line 26; `SheetSurface`, the preview `ModeBody` at line 268)
- Modify: `tinycld/core/reports/hooks/useReportView.ts` (the model's `isUpdating` at line 358; NOT lines 233 and 316, which stay `run.isUpdating`)
- Modify: `tinycld/core/help/reports.md` (the last bullet of "Run a report", lines 207-208)
- Test: `tinycld/core/reports/lib/__tests__/run-state.test.ts`
- Test: `tinycld/core/reports/hooks/__tests__/useReportRun.test.tsx`

`tinycld/core/reports/lib/rows.ts` does NOT change: `mergePages` returns `{ ...last, rows, notices }`, so the joined result keeps the last page's `manual`. The test of step 1 pins it.

**Interfaces:**
- Consumes (from task 8a, part 8): the body of `POST /api/reports/run` holds `"manual":true` for a manual run and has no `manual` key for a live one. A manual run's `topics` is `[]`, or the two saved-report topics when the request has a `reportId`.
- Consumes (they exist): `ShortcutButton` props `label`, `shortcut: { keys, scope, group? }`, `onPress`, `variant`, `isDisabled`, `testID`, `scopeOwner` (`core/ui/shortcut-button/index.tsx`); `useShortcutScope(scope: Scope): ScopeOwner` and the type `ScopeOwner` (`core/lib/shortcuts/scopes.ts`); `Button`, `ButtonText` (`core/ui/button`); in `useReportRun`: `whenIdle(start: () => Promise<unknown>): Promise<void>` (line 200) and `query.refetch()`; in the hook test file: `held`, `row`, `result`, `renderRun`, `rowIds`, `answer`.
- Produces (task 8d relies on these exact names):
  - `RunResult.manual?: boolean` (`lib/types.ts`)
  - `RunView.isManual: boolean` (`lib/run-state.ts`), so `ReportRun.isManual: boolean`
  - `ReportRun.refresh?(): void`, `ReportRun.isRefreshing?: boolean` (`hooks/useReportRun.ts`)
  - `ReportManualBand({ run: ReportRun, scopeOwner?: ScopeOwner, idPrefix?: string })` and `MANUAL_NOTE` (`components/ReportManualBand.tsx`)
  - `ReportViewModel.isUpdating` is true also while a Refresh runs (`run.isUpdating || run.isRefreshing === true`)
  - the test ids and the note text of Global Constraints; the header's `report-updating` text ("Updating…") shows while a Refresh of the report runs; the Refresh button is disabled and reads `Refreshing…` while it runs.

- [ ] **Step 1: Write the failing tests**

In `tinycld/core/reports/lib/__tests__/run-state.test.ts`, the first test (`is idle with no body`) compares the whole view. Add `isManual: false` to it, and add one test after it:

```ts
    it('is idle with no body', () => {
        expect(runView(query({ hasBody: false, isPending: true }))).toEqual({
            state: 'idle',
            result: undefined,
            problems: [],
            isUpdating: false,
            heldRows: [],
            topics: [],
            isManual: false,
        })
    })

    it('says a run is manual only when its figures carry the mark', () => {
        const manual: RunOutcome = { kind: 'ok', result: { ...result([]), manual: true } }
        expect(runView(query({ pages: [manual] })).isManual).toBe(true)
        expect(runView(query({ pages: [ok()] })).isManual).toBe(false)
        // The joined pages keep the mark (mergePages spreads the last page).
        expect(runView(query({ pages: [manual, manual] })).isManual).toBe(true)
        // A refusal shows no figures, so there is nothing to refresh.
        expect(runView(query({ pages: [manual, invalid] })).isManual).toBe(false)
    })
```

In `tinycld/core/reports/hooks/__tests__/useReportRun.test.tsx`, add this `describe` right before `describe('useReportRun after a fold', () => {`:

```tsx
describe('useReportRun of a manual run', () => {
    const manual = (rows: ResultRow[], next = ''): RunResult => ({
        ...result(rows, next),
        topics: [],
        manual: true,
    })

    it('Refresh runs the report again, once for two presses, and says so while it runs', async () => {
        let asked = 0
        const second = held(manual([row('d:1'), row('d:2')]))
        answer = () => {
            asked++
            return asked === 1 ? Promise.resolve(manual([row('d:1')])) : second.promise
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.state).toBe('ready'))
        expect(run.current.isManual).toBe(true)
        expect(run.current.isRefreshing).toBe(false)
        act(() => {
            run.current.refresh?.()
            run.current.refresh?.()
        })
        await waitFor(() => expect(asked).toBe(2))
        expect(run.current.isRefreshing).toBe(true)
        expect(rowIds(run.current)).toEqual(['d:1'])
        await act(async () => second.release())
        await waitFor(() => expect(run.current.isRefreshing).toBe(false))
        expect(rowIds(run.current)).toEqual(['d:1', 'd:2'])
        expect(asked).toBe(2)
    })

    it('Refresh keeps the pages loaded and the manual mark', async () => {
        const asks: string[] = []
        answer = sent => {
            const from = sent.options.cursor ?? ''
            asks.push(from)
            return Promise.resolve(from === '' ? manual([row('d:1')], 'n1') : manual([row('d:2')]))
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.hasMore).toBe(true))
        act(() => run.current.loadMore())
        await waitFor(() => expect(rowIds(run.current)).toEqual(['d:1', 'd:2']))
        const before = asks.length
        act(() => run.current.refresh?.())
        await waitFor(() => expect(asks.length).toBeGreaterThan(before))
        await waitFor(() => expect(run.current.isRefreshing).toBe(false))
        expect(rowIds(run.current)).toEqual(['d:1', 'd:2'])
        expect(run.current.isManual).toBe(true)
    })
})
```

- [ ] **Step 2: Run the tests to see them fail**

Run, from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core`:

```sh
PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH vitest run reports/lib/__tests__/run-state.test.ts reports/hooks/__tests__/useReportRun.test.tsx
```

Expected: FAIL. `is idle with no body` (the view has no `isManual`), `says a run is manual…` (`expected undefined to be true`), and both tests of `useReportRun of a manual run` (`expected undefined to be true` at `isManual`). The other tests pass.

- [ ] **Step 3: Write the implementation**

`tinycld/core/reports/lib/types.ts`, in `RunResult`, after `topics: string[]`:

```ts
    topics: string[]
    /**
     * The run read a source that is too large to follow, so it carries no data
     * topics and does not run again by itself. The reader runs it again with Refresh.
     */
    manual?: boolean
```

`tinycld/core/reports/lib/run-state.ts`, in `RunView`, after `topics: string[]`:

```ts
    topics: string[]
    /** The figures shown do not update by themselves; the reader presses Refresh. */
    isManual: boolean
```

and in the object `runView` returns, after the `topics` line:

```ts
        topics: shown?.result.topics ?? NO_TOPICS,
        isManual: !refusal && shown?.result.manual === true,
```

`tinycld/core/reports/hooks/useReportRun.ts`, in `ReportRun`, after `retry(): void`:

```ts
    retry(): void
    /**
     * Runs the report again and keeps the pages loaded (the Refresh of a manual
     * run). Optional, because a hand-built ReportRun (a mockup) has no query.
     */
    refresh?(): void
    /** A Refresh is in progress. Optional, as `refresh`. */
    isRefreshing?: boolean
```

and replace the end of `useReportRun`, from `const isLoadingMore` to the closing brace of the function:

```ts
    const isLoadingMore = query.isFetchingNextPage || isWaitingForMore
    // Set at once by refresh, so a second press while it runs starts nothing.
    const refreshing = useRef(false)
    // A refetch of the same body is not `isUpdating` (that is a changed body), so
    // the reader's own Refresh has its own state, or nothing would show it runs.
    const [isRefreshing, setIsRefreshing] = useState(false)
    // Waits as a live rerun does (whenIdle), so it never cancels a next page.
    const refresh = () => {
        if (refreshing.current) return
        refreshing.current = true
        setIsRefreshing(true)
        void whenIdle(() => query.refetch()).finally(() => {
            refreshing.current = false
            setIsRefreshing(false)
        })
    }
    return {
        ...view,
        shownRowsKey: pages[0]?.rowsKey,
        onTopRowIndex,
        hasMore: query.hasNextPage,
        isLoadingMore,
        loadMore,
        loadMoreFailed: isLoadMoreError && !isLoadingMore,
        retryLoadMore: startNextPage,
        retry: () => void query.refetch(),
        refresh,
        isRefreshing,
    }
}
```

(`isManual` reaches `ReportRun` through `...view`. The `useState` here is local state set by the reader's own press, the same pattern as `isWaitingForMore` in this hook; it is not synced from other data.)

Create `tinycld/core/reports/components/ReportManualBand.tsx`:

```tsx
import type { ScopeOwner } from '@tinycld/core/lib/shortcuts/scopes'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { ShortcutButton } from '@tinycld/core/ui/shortcut-button'
import { Text, View } from 'react-native'
import type { ReportRun } from '../hooks/useReportRun'

export const MANUAL_NOTE =
    'This report does not update by itself. Press Refresh to see new changes.'

const REFRESH_LABEL = 'Refresh'
const REFRESHING_LABEL = 'Refreshing…'
const REFRESH_SHORTCUT = { keys: 'r', scope: 'list', group: 'Report' } as const
const NO_REFRESH = () => {}

interface RefreshButtonProps {
    testID: string
    isRefreshing: boolean
    onPress: () => void
    scopeOwner: ScopeOwner | undefined
}

// With a scope owner the button has the shortcut R. A second report on the same
// screen (the drill panel, the builder's preview) gives none, so one key never
// runs two reports. The label says a Refresh runs: not every surface that shows
// the band has an "Updating…" slot.
function RefreshButton({ testID, isRefreshing, onPress, scopeOwner }: RefreshButtonProps) {
    const label = isRefreshing ? REFRESHING_LABEL : REFRESH_LABEL
    if (!scopeOwner) {
        return (
            <Button testID={testID} variant="outline" isDisabled={isRefreshing} onPress={onPress}>
                <ButtonText>{label}</ButtonText>
            </Button>
        )
    }
    return (
        <ShortcutButton
            testID={testID}
            label={label}
            shortcut={REFRESH_SHORTCUT}
            variant="outline"
            isDisabled={isRefreshing}
            onPress={onPress}
            scopeOwner={scopeOwner}
        />
    )
}

// The band of a manual run (a run over a source too large to follow): the note
// that says the figures do not update by themselves, and Refresh. It is the same
// on every width, so a phone has the button in view and not under "⋯".
export function ReportManualBand({
    run,
    scopeOwner,
    idPrefix = 'report',
}: {
    run: ReportRun
    /** The screen's `useShortcutScope()`; absent: Refresh has no shortcut. */
    scopeOwner?: ScopeOwner
    /** Starts the test ids, so each surface's band is told from the report's. */
    idPrefix?: string
}) {
    if (!run.isManual) return null
    return (
        <View testID={`${idPrefix}-manual-band`} className="flex-row items-center gap-3 px-4 py-1">
            <Text
                testID={`${idPrefix}-manual-note`}
                className="min-w-0 flex-1 text-sm text-muted-foreground"
            >
                {MANUAL_NOTE}
            </Text>
            <RefreshButton
                testID={`${idPrefix}-refresh`}
                isRefreshing={run.isRefreshing === true}
                onPress={run.refresh ?? NO_REFRESH}
                scopeOwner={scopeOwner}
            />
        </View>
    )
}
```

`tinycld/core/reports/components/ReportView.tsx`. Add two imports (keep the import order biome wants: the scopes import goes after the `ResponsiveToolbar` import, the band import after `./ReportHeader`):

```tsx
import { type ScopeOwner, useShortcutScope } from '@tinycld/core/lib/shortcuts/scopes'
```

```tsx
import { ReportManualBand } from './ReportManualBand'
```

Replace `ReportTopProps` and the start of `ReportTop` (down to and with the `ReportControls` line; the `ReportNotices` and `ReportDocumentHeader` elements after it stay as they are):

```tsx
interface ReportTopProps {
    model: ReportViewModel
    isSortMenuShown: boolean
    layoutNotice: string | undefined
    isCompact: boolean
    /** The owner of Refresh's shortcut; absent while the report is covered. */
    refreshScope: ScopeOwner | undefined
}

// Everything above the rows: the header, the chips, the manual band, the notices
// and the document header.
function ReportTop({
    model,
    isSortMenuShown,
    layoutNotice,
    isCompact,
    refreshScope,
}: ReportTopProps) {
    // The status slot says so at every width, also when Export is folded into "⋯" (D3).
    const busyLabel = model.export?.isPreparing ? 'Preparing…' : undefined
    return (
        <>
            <ReportHeader
                title={model.title}
                onBack={model.onBack}
                backLabel={model.backLabel}
                actions={toolbarItems(model)}
                isUpdating={model.isUpdating}
                isUpdatingLive={!model.isUpdatingInBuilder}
                busyLabel={busyLabel}
            />
            <ReportControls model={model} isSortMenuShown={isSortMenuShown} />
            <ReportManualBand run={model.run} scopeOwner={refreshScope} />
```

In `ReportView`, replace the lines from `const isCompact` to the end of the `top` element:

```tsx
    const isCompact = model.breakpoint === 'mobile'
    const scopeOwner = useShortcutScope('list')
    // A covered report is not in view, so its Refresh takes no key.
    const refreshScope = isCovered ? undefined : scopeOwner
    const top = (
        <ReportTop
            model={model}
            isSortMenuShown={isSortMenuShown}
            layoutNotice={layoutNotice}
            isCompact={isCompact}
            refreshScope={refreshScope}
        />
    )
```

`tinycld/core/reports/components/ReportDrillPanel.tsx`. Add the import after the `./ReportHeader` import:

```tsx
import { ReportManualBand } from './ReportManualBand'
```

and in `DrillReport`, one line after the `ReportControls` element:

```tsx
            <ReportControls model={view} isSortMenuShown={table.layout.kind === 'blocks'} />
            <ReportManualBand run={view.run} idPrefix="report-drill" />
```

`tinycld/core/reports/components/builder/ReportBuilderPanel.tsx`. Add the import after the `'../ReportHeader'` import:

```tsx
import { ReportManualBand } from '../ReportManualBand'
```

and in `SheetSurface`, the band is the first child of the preview `ModeBody`:

```tsx
                <ModeBody isVisible={mode === 'preview'}>
                    <ReportManualBand run={preview.run} idPrefix="report-builder" />
                    <ReportNotices
```

`tinycld/core/reports/hooks/useReportView.ts`, in the returned model (line 358), replace `isUpdating: run.isUpdating,`:

```ts
        // The reader's Refresh runs the same body again, which is not a changed run;
        // every "Updating…" slot (the header's, the builder's) reads this.
        isUpdating: run.isUpdating || run.isRefreshing === true,
```

Leave the two other `run.isUpdating` in this file (the export model's input at line 233 and `withPendingOpen` at line 316) as they are: a Refresh keeps the rows and the export of the figures in view.

- [ ] **Step 4: Update the core help**

In `tinycld/core/help/reports.md`, section "Run a report", replace the last bullet (lines 207-208, "A report that is open stays current: …") with:

```markdown
- A report that is open stays current: when someone changes the data it reads, it
  runs again.
- A report that reads a very large list does not run again by itself. It says so
  under the chips: **This report does not update by itself. Press Refresh to see new
  changes.** Select **Refresh** to run it again; the button reads **Refreshing…**
  while it runs, and the rows you loaded stay. With a keyboard, press R while the
  report itself is in view (not in a panel beside it, and not while you type in a
  box). On a phone, scroll back to the top to reach the button. A report you build
  over such a list works the same way, and so does a copy of a report.
```

Do not change line 21 ("The report updates as you change it."): a changed design is a new run, also for a manual report.

- [ ] **Step 5: Run the tests and the typecheck**

Run, from `/Users/nas/tmp/tinycld/books-purchases/tinycld/core`:

```sh
export PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH
vitest run reports/lib/__tests__/run-state.test.ts reports/hooks/__tests__/useReportRun.test.tsx reports/lib/__tests__/rows.test.ts
tinycld-pkg typecheck
biome check reports/components/ReportManualBand.tsx reports/components/ReportView.tsx reports/components/ReportDrillPanel.tsx reports/components/builder/ReportBuilderPanel.tsx reports/hooks/useReportRun.ts reports/hooks/useReportView.ts reports/lib/run-state.ts reports/lib/types.ts reports/lib/__tests__/run-state.test.ts reports/hooks/__tests__/useReportRun.test.tsx
```

Expected: vitest `Test Files 3 passed`, no failed test; `tinycld-pkg typecheck` exits 0 with no `error TS`; biome exits 0. (The planner built this code on core 2cf340d7: typecheck 0, the two test files 27 passed, biome 0.) When the typecheck names a hand-built `ReportRun` or `RunView` with no `isManual`, add `isManual: false` to that object; do not make the field optional.

- [ ] **Step 6: Commit**

From `/Users/nas/tmp/tinycld/books-purchases/tinycld`:

```bash
git add core/reports/components/ReportManualBand.tsx core/reports/components/ReportView.tsx core/reports/components/ReportDrillPanel.tsx core/reports/components/builder/ReportBuilderPanel.tsx core/reports/hooks/useReportRun.ts core/reports/hooks/useReportView.ts core/reports/lib/run-state.ts core/reports/lib/types.ts core/reports/lib/__tests__/run-state.test.ts core/reports/hooks/__tests__/useReportRun.test.tsx core/help/reports.md
git commit -m "feat(reports): a manual run shows a note and Refresh"
```
