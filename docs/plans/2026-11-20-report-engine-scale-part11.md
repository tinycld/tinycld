# Report Engine Scale, Part 11: Books E2E of the Note and Refresh — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Books Playwright specs press Refresh where a report is not live, and one new spec pins the note, Refresh by press and by the key R, and the builder's Preview on a phone.

**Architecture:** Three shared helpers in `books/tests/e2e/books-report-helpers.ts` (`manualBand`, `refreshReport`, `nextReportRun`) and the note's text (`MANUAL_NOTE`). Six tests that waited for a live re-run of a report that is now manual press Refresh through `refreshReport`. The Profit and Loss test that stays live also asserts that it has no band. A new spec, `report-manual-refresh.spec.ts`, holds two tests: the report's band on a desktop window, and the band of the builder's Preview at a phone width (other code: core `SheetSurface`).

**Tech Stack:** Playwright (ES modules), TypeScript, core's e2e helpers (`@tinycld/core/e2e-helpers`), the Books e2e helpers.

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/handoff-backend-planner-part7.md` (section "Design proposed", items 4 and 6), the outline `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (task 8d; the sections "State after plan part 9" and "State after plan part 10"), parts 9 and 10 of this plan (their Interfaces blocks), and the human's decision of 2026-11-20 in `/Users/nas/code/tinycld/docs/plans/auto-helix/p5-report-engine-scale.md`.

Workspace root: `/Users/nas/tmp/tinycld/books-purchases`. Books is `books/`, on the branch `books/p5-report-engine-scale`. This part changes Books e2e files only and holds TWO tasks (8d-1, 8d-2). Tasks 8a (core server), 8b (core client) and 8c (Books Go) are built. This is the last part of task 8.

The code of both tasks was applied to the tree once by the planner (`tinycld-pkg typecheck` in `books`: exit 0; `biome check` of the ten files: exit 0) and taken out. The diff is `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part11-scratch/8d-edits.patch`, and the new spec is `part11-scratch/report-manual-refresh.spec.ts.txt`. This plan's code is the authority: two fragments were changed after that build, in the plan review, and were NOT compiled again (the poll of task 8d-1 step 8, and the key R block of the new spec's first test). Both use only names the built code already used; steps 9 and 2 typecheck them. No e2e was run: the driver runs it (see "The driver's e2e runs").

## Global Constraints

- The six reports that are NOT live: General Ledger, Transaction Detail by Account, Trial Balance, Balance Sheet, Journal, Profit and Loss by Month. Every other shipped Books report stays live. A draft over `books.journal_lines` (the builder's Preview) is not live.
- The note's text, exactly: `This report does not update by itself. Press Refresh to see new changes.`
- Test ids, exactly: `report-manual-band`, `report-manual-note`, `report-refresh` (the report); `report-drill-manual-band`, `report-drill-manual-note`, `report-drill-refresh` (the drill panel); `report-builder-manual-band`, `report-builder-manual-note`, `report-builder-refresh` (the Preview of the builder's sheet on a phone).
- While a Refresh runs the button is disabled and reads `Refreshing…`; the header's `report-updating` reads `Updating…`. The shortcut is the key `r` with no modifier, only on the report's own button.
- E2E rules (CLAUDE.md): sign in with `login(page)`, then `navigateToPackage(page, 'books')`; no `page.goto()` for navigation; data is set up through the UI; no raw PocketBase write; no timeout is raised; no test is made serial; a phone test opens the screen at the default viewport and then calls `page.setViewportSize(PHONE)`.
- Do not assert that a new figure is ABSENT before Refresh: a manual run of a saved report keeps the topics `collection/reports` and `collection/report_shares`, so a share or a save by another worker on the shared DB runs it again. Assert the note, the press, and the figure after the press.
- Press Refresh only after the write is committed: `postEntryOn` / `postOpenEntry` / `createPostedEntry` return when the server has numbered the entry (`JE-<n>` in the title), so one press is enough.
- Core's builder helpers (`startReport`, `addColumn`, `saveNewReport` in `tinycld/tests/e2e/reports-helpers.ts`) are NOT exported by `@tinycld/core/e2e-helpers`. The new spec presses the builder's test ids itself. Do not edit core.
- The implementer does NOT run e2e, `pnpm install` or `packages:generate`. Run only `tinycld-pkg typecheck` and `biome check` of the named files. Edit no Go file and no help file.
- Undo an edit with the Edit tool, never with `git checkout` or `git restore`.

## Review Focus

- A second press of Refresh while the first run is in flight: the button is disabled and reads `Refreshing…` while the run is held (new spec, both tests, `toBeDisabled`).
- Refresh from the empty state (the report has no row yet): the band is there and the first posting shows (new spec, test 1; `report-drill-down.spec.ts`, the first `refreshReport`).
- A live statement beside a panel that is not live: the statement shows the post with no press and has no band; the panel needs its own Refresh (`report-drill-panel.spec.ts`, step 8 of task 8d-1).
- Another worker posts an expense between the statement's run and the panel's: the statement moves and the panel does not, so the poll that compares the two totals presses the panel's Refresh while they differ (`report-drill-panel.spec.ts`, step 8).
- The band at a phone width: the note wraps beside the button and neither is cut at 390 px (new spec, test 2, the `boundingBox` poll).
- A report left mounted under another screen and reached again by Back after a write shows the old figures until Refresh. This is the accepted behavior (the help text of 8c says it). No test of the run list expects otherwise: the planner read every `page.goBack()` and `report-back` press in the listed specs (`report-drill-down.spec.ts` 342, 358, 371, 425, 702, 847, 871; `trial-balance.spec.ts` 835; `report-drill-panel.spec.ts` 266; `report-drill-phone.spec.ts` 108, 110; `pay-bills-cash-basis.spec.ts` 127), and no write is made between the report's run and the return.

Not tested, by design: the key `r` typed in a filter's search box runs nothing, and `r` under the builder's sheet runs nothing. Both are checks that nothing happens, which a run from another worker's share can break; the shortcut matcher has its unit tests in core (`core/tests/unit/shortcuts.test.ts`).

---

### Task 8d-1: The specs of the six reports press Refresh

**Files:**
- Modify: `books/tests/e2e/books-report-helpers.ts` (the import of `@tinycld/core/e2e-helpers` at lines 3-12; after `drillPanel` at line 1240)
- Modify: `books/tests/e2e/journal-report.spec.ts` (import at lines 6-21; the test at line 1320)
- Modify: `books/tests/e2e/trial-balance.spec.ts` (import at lines 11-26; the test at line 481)
- Modify: `books/tests/e2e/balance-sheet.spec.ts` (import at lines 20-37; the test at line 845)
- Modify: `books/tests/e2e/general-ledger.spec.ts` (import at lines 11-25; the test at line 843)
- Modify: `books/tests/e2e/report-drill-down.spec.ts` (import at lines 20-51; the test at line 511)
- Modify: `books/tests/e2e/report-drill-panel.spec.ts` (import at lines 13-30; lines 180-226)
- Modify: `books/tests/e2e/profit-and-loss.spec.ts` (the test at line 424)

**Interfaces:**
- Consumes (core, built): the test ids and texts of Global Constraints; `inStatement(page: Page, testId: string): Locator` from `@tinycld/core/e2e-helpers` (a test id in the report, never in its drill panel).
- Consumes (Books, they exist): `drillPanel(page: Page): Locator` (`books-report-helpers.ts`); each spec's own `snapshotWhen` / `journalWhen` helpers, unchanged.
- Produces (task 8d-2 uses these exact names, all exported from `books/tests/e2e/books-report-helpers.ts`):
  - `MANUAL_NOTE: string`
  - `manualBand(page: Page, surface?: 'report' | 'report-drill'): { band: Locator; note: Locator; refresh: Locator }`
  - `nextReportRun(page: Page): Promise<Request>` (the next `POST /api/reports/run` the page sends)
  - `refreshReport(page: Page, surface?: 'report' | 'report-drill'): Promise<void>`

The red of this task: with 8a, 8b and 8c built, each of the six tests waits for a figure that no run brings, and fails at its poll. No red run was made by the planner (it could not run e2e). The implementer cannot run e2e either: the red run is the driver's, before this task, with the RED command of "The driver's e2e runs"; the run after task 8d-2 is the green.

- [ ] **Step 1: Add the helpers**

In `books/tests/e2e/books-report-helpers.ts`, add `inStatement` to the import from `@tinycld/core/e2e-helpers` (`Request` and `Locator` are imported from `@playwright/test` at line 2 already):

```ts
import {
    authStorageKey,
    cellOf,
    chooseParamOption,
    downloadReport,
    hubRow,
    inStatement,
    navigateToPackage,
    type ReportExportFormat,
    rowOf,
} from '@tinycld/core/e2e-helpers'
```

Right after the function `drillPanel` (it ends at line 1240), add:

```ts
// --- A report that is not live (the six that read every journal line). ---

// The note core draws over a run that is not live (core ReportManualBand MANUAL_NOTE).
export const MANUAL_NOTE =
    'This report does not update by itself. Press Refresh to see new changes.'

// The band's parts by surface: the report itself, or the report in the drill panel. Each
// surface has its own test ids, so the two bands are never mixed up.
export function manualBand(page: Page, surface: 'report' | 'report-drill' = 'report') {
    const own = (part: string) =>
        surface === 'report'
            ? inStatement(page, `report-${part}`)
            : drillPanel(page).getByTestId(`report-drill-${part}`)
    return { band: own('manual-band'), note: own('manual-note'), refresh: own('refresh') }
}

function isReportRun(url: string, method: string): boolean {
    return method === 'POST' && new URL(url).pathname === '/api/reports/run'
}

// Resolves with the next report run the page sends. Start it before the action that
// sends the run, and await it after.
export function nextReportRun(page: Page): Promise<Request> {
    return page.waitForRequest(request => isReportRun(request.url(), request.method()))
}

// Presses Refresh on a report that is not live and waits for a run's answer. The note is
// read first: only a run that is not live draws it. The answer's headers come before the
// rows are drawn, so the caller still waits for the figure it needs. Call it only after
// the write it must show is committed (postOpenEntry waits for the server's number).
export async function refreshReport(
    page: Page,
    surface: 'report' | 'report-drill' = 'report'
): Promise<void> {
    const { note, refresh } = manualBand(page, surface)
    await expect(note).toHaveText(MANUAL_NOTE)
    const answered = page.waitForResponse(response =>
        isReportRun(response.url(), response.request().method())
    )
    await refresh.click()
    await answered
    await expect(refresh).toBeEnabled()
}
```

- [ ] **Step 2: The Journal (`journal-report.spec.ts`)**

In the import from `./books-report-helpers`, add `refreshReport,` between `postEntryOn,` and `voidPostedEntry,`.

Change the test's title (line 1320) from `'a post made on another page appears as a new block in the open Journal without a reload'` to:

```ts
    test('a post made on another page appears as a new block in the open Journal after Refresh, without a reload', async ({
```

In that test, replace

```ts
        await otherContext.close()

        const live = await journalWhen(
            page,
            s => hasTotal(s) && linesOf(s, heading).length === 2,
            'the new block in the open report'
        )
```

with

```ts
        await otherContext.close()

        // The Journal is not live: the post shows when the reader presses Refresh.
        await refreshReport(page)
        const live = await journalWhen(
            page,
            s => hasTotal(s) && linesOf(s, heading).length === 2,
            'the new block in the open report after Refresh'
        )
```

- [ ] **Step 3: The Trial Balance (`trial-balance.spec.ts`)**

In the import from `./books-report-helpers`, add `refreshReport,` between `openTrialBalance,` and `snapshotWhen,`.

Change the test's title (line 481) to:

```ts
    test('a post made on another page appears in the open report after Refresh, without a reload', async ({
```

In that test, replace

```ts
        await postEntry(other, '15.00', account, ACCOUNT_LABELS.checking)

        const live = await snapshotWhen(
            page,
            tbSnapshot,
            s => groupRow(s, account)?.debit === '15.00',
            'the post in the open report'
        )
```

with

```ts
        await postEntry(other, '15.00', account, ACCOUNT_LABELS.checking)

        // The Trial Balance is not live: the post shows when the reader presses Refresh.
        await refreshReport(page)
        const live = await snapshotWhen(
            page,
            tbSnapshot,
            s => groupRow(s, account)?.debit === '15.00',
            'the post in the open report after Refresh'
        )
```

- [ ] **Step 4: The Balance Sheet (`balance-sheet.spec.ts`)**

In the import from `./books-report-helpers`, add `refreshReport,` between `type ReportSnapshot,` and `reportLabelsThrough,`.

Change the test's title (line 845) to:

```ts
    test('a post made on another page appears in the open report after Refresh, without a reload', async ({
```

In that test, replace

```ts
            // Only a run after the post has a row for the new account.
            const live = await snapshotWhen(
                page,
                s => has(s, debited, credited),
                `${debited} and ${credited} in the open report`
            )
```

with

```ts
            // The Balance Sheet is not live: the post shows when the reader presses
            // Refresh. Only a run after the post has a row for the new account.
            await refreshReport(page)
            const live = await snapshotWhen(
                page,
                s => has(s, debited, credited),
                `${debited} and ${credited} in the open report after Refresh`
            )
```

(`snapshotWhen` here is the spec's own three-argument helper; do not change it.)

- [ ] **Step 5: The General Ledger (`general-ledger.spec.ts`)**

In the import from `./books-report-helpers`, add `refreshReport,` between `postEntryOn,` and `snapshotWhen,`.

Change the test's title (line 843) to:

```ts
    test('a post made on another page appears in the open report after Refresh, without a reload', async ({
```

In that test, replace

```ts
        const live = await snapshotWhen(
            page,
            glSnapshot,
            s => linesOf(s, account).length === 1,
            'the post in the open report'
        )
```

with

```ts
        // The General Ledger is not live: the post shows when the reader presses Refresh.
        await refreshReport(page)
        const live = await snapshotWhen(
            page,
            glSnapshot,
            s => linesOf(s, account).length === 1,
            'the post in the open report after Refresh'
        )
```

- [ ] **Step 6: Transaction Detail (`report-drill-down.spec.ts`)**

In the import from `./books-report-helpers`, add `refreshReport,` between `pressFigure,` and `sectionTopKey,`.

Change the test's title (line 511) from `'the report runs again on a post from another session, and sorts by Amount'` to:

```ts
    test('Refresh shows a post from another session, and the report sorts by Amount', async ({
```

In that test, replace

```ts
            const first = await snapshotWhen(
                page,
                tdSnapshot,
                s => tdLinesOf(s, expense).length === 1 && tdSubtotalOf(s, expense) !== undefined,
                `${expense}'s first line without a reload`
            )
```

with

```ts
            // Transaction Detail is not live: a post shows when the reader presses
            // Refresh, here from the empty state.
            await refreshReport(page)
            const first = await snapshotWhen(
                page,
                tdSnapshot,
                s => tdLinesOf(s, expense).length === 1 && tdSubtotalOf(s, expense) !== undefined,
                `${expense}'s first line after Refresh`
            )
```

and replace

```ts
            await snapshotWhen(
                page,
                tdSnapshot,
                s => tdLinesOf(s, expense).length === 2,
                `${expense}'s second line without a reload`
            )
```

with

```ts
            await refreshReport(page)
            await snapshotWhen(
                page,
                tdSnapshot,
                s => tdLinesOf(s, expense).length === 2,
                `${expense}'s second line after Refresh`
            )
```

- [ ] **Step 7: The Profit and Loss stays live (`profit-and-loss.spec.ts`)**

In the test `'the open report re-runs on a post from another page'` (line 424), replace

```ts
            expect(amountOf(live, 'group', income)).toBe(6000)
            const notReloaded = await page.evaluate(
```

with

```ts
            expect(amountOf(live, 'group', income)).toBe(6000)
            // The Profit and Loss stays live: it has no note and no Refresh.
            await expect(page.getByTestId('report-manual-band')).toHaveCount(0)
            await expect(page.getByTestId('report-refresh')).toHaveCount(0)
            const notReloaded = await page.evaluate(
```

This test is the "stays live" half of the unit's e2e: the post shows with no press, and the band is absent after a run has landed. No second copy of this flow goes in the new spec.

- [ ] **Step 8: The drill panel (`report-drill-panel.spec.ts`)**

In the import from `./books-report-helpers`, add `manualBand,` between `figureText,` and `openEntriesFromSidebar,`, and `refreshReport,` between `pressFigure,` and `sectionTopKey,`.

In the first test, replace

```ts
        const live = await snapshotWhen(
            page,
            p => tdSnapshot(p, drillPanel(p)),
            s => tdLinesOf(s, expense).length === 3,
            `${expense}'s third line in the panel without a reload`
        )
        expect(figureDigits(tdSubtotalOf(live, expense)?.cells.amount)).toBe('215.00')
        await expect
            .poll(async () => figureDigits(await figureText(page, 'amount', expense)))
            .toBe('215.00')
```

with

```ts
        // The statement (the Profit and Loss) is live: it shows the post with no press
        // and has no band. Transaction Detail in the panel is not live: its own Refresh
        // brings the line in.
        await expect
            .poll(async () => figureDigits(await figureText(page, 'amount', expense)))
            .toBe('215.00')
        await expect(manualBand(page).band).toHaveCount(0)
        await refreshReport(page, 'report-drill')
        const live = await snapshotWhen(
            page,
            p => tdSnapshot(p, drillPanel(p)),
            s => tdLinesOf(s, expense).length === 3,
            `${expense}'s third line in the panel after Refresh`
        )
        expect(figureDigits(tdSubtotalOf(live, expense)?.cells.amount)).toBe('215.00')
```

Further down in the same test, replace

```ts
        // Other workers post expenses and both views run live, so the two totals are
        // compared until they agree.
        await expect
            .poll(
                async () => {
                    await scrollToEnd(drillPanel(page))
                    const section = await tdSnapshot(page, drillPanel(page))
                    const statement = figureDigits(await figureText(page, 'amount', TOTAL_EXPENSES))
                    const shown = figureDigits(tdTotalOf(section)?.cells.amount)
                    return { equal: shown === statement && statement !== '', shown, statement }
                },
```

with

```ts
        // Other workers post expenses. The statement runs live and the panel does not, so
        // the two totals are compared until they agree, and the panel is refreshed when a
        // Total it shows is not the statement's.
        const panelRefresh = manualBand(page, 'report-drill').refresh
        await expect
            .poll(
                async () => {
                    await scrollToEnd(drillPanel(page))
                    const section = await tdSnapshot(page, drillPanel(page))
                    const statement = figureDigits(await figureText(page, 'amount', TOTAL_EXPENSES))
                    const shown = figureDigits(tdTotalOf(section)?.cells.amount)
                    const isStale = shown !== '' && shown !== statement
                    if (isStale && (await panelRefresh.isEnabled())) await panelRefresh.click()
                    return { equal: shown === statement && statement !== '', shown, statement }
                },
```

(The lines after it, `{ message: … }` and `.toMatchObject({ equal: true })`, stay. The button is pressed only while it is enabled: while a Refresh runs it is disabled, so the poll never starts a second run and never waits in a click.)

- [ ] **Step 9: Typecheck and lint**

Run:

```sh
cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck
```

Expected: exit 0, no error.

Run:

```sh
cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check tests/e2e/books-report-helpers.ts tests/e2e/journal-report.spec.ts tests/e2e/trial-balance.spec.ts tests/e2e/balance-sheet.spec.ts tests/e2e/general-ledger.spec.ts tests/e2e/report-drill-down.spec.ts tests/e2e/report-drill-panel.spec.ts tests/e2e/profit-and-loss.spec.ts
```

Expected: exit 0. Until task 8d-2 is done, `nextReportRun` has no user: it is exported, so neither tool reports it.

- [ ] **Step 10: Commit (in the Books repo)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add tests/e2e/books-report-helpers.ts tests/e2e/journal-report.spec.ts tests/e2e/trial-balance.spec.ts tests/e2e/balance-sheet.spec.ts tests/e2e/general-ledger.spec.ts tests/e2e/report-drill-down.spec.ts tests/e2e/report-drill-panel.spec.ts tests/e2e/profit-and-loss.spec.ts
git commit -m "test(e2e): the reports that are not live press Refresh"
```

---

### Task 8d-2: The spec of the note and Refresh

**Files:**
- Create: `books/tests/e2e/report-manual-refresh.spec.ts`

**Interfaces:**
- Consumes (task 8d-1, `books/tests/e2e/books-report-helpers.ts`): `MANUAL_NOTE: string`; `manualBand(page, surface?): { band: Locator; note: Locator; refresh: Locator }`; `nextReportRun(page): Promise<Request>`.
- Consumes (Books, they exist): `openBooksReports(page)`, `openTransactionDetail(page, { accounts })`, `openEntriesFromSidebar(page)`, `postEntryOn(page, { date, memo, lines })`, `browserToday(page)`, `snapshotWhen(page, take, isReady, message)`, `tdSnapshot`, `tdLinesOf`, `tdSubtotalOf`, `figureDigits` (`books-report-helpers.ts`); `createAccount(page, name, kind)`, `openChart(page)`, `PHONE` (`account-register-helpers.ts`); `ACCOUNT_LABELS` (`journal-entries-helpers.ts`); `uniqueName(prefix)` (`record-form-helpers.ts`).
- Consumes (core, built): `login`, `navigateToPackage` (`@tinycld/core/e2e-helpers`); the hub's `reports-hub-new` and `reports-new-source-<source key>`; the builder sheet's `report-builder`, `report-builder-mode-design`, `report-builder-mode-preview`; the test ids and texts of Global Constraints; a draft's run body is `{ spec: { source, … }, params, options }` (`core/reports/lib/run-client.ts` `RunBody`).
- Produces: nothing for a later task.

How the phone path works (checked in `core/reports/components/builder/ReportBuilderPanel.tsx`): below the mobile breakpoint `BuilderSurface` draws `SheetSurface`, a full-height sheet with the test id `report-builder` and two pills, Design and Preview. The sheet opens on Design. Preview draws `ReportManualBand` with `idPrefix="report-builder"`, the notices and the table (`report-table`). The sheet's header has its own `report-updating` slot. Core's own phone precedent is `tinycld/tests/e2e/reports-create.spec.ts:135`.

- [ ] **Step 1: Write the spec**

Create `books/tests/e2e/report-manual-refresh.spec.ts`:

```ts
import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { createAccount, openChart, PHONE } from './account-register-helpers'
import {
    browserToday,
    figureDigits,
    MANUAL_NOTE,
    manualBand,
    nextReportRun,
    openBooksReports,
    openEntriesFromSidebar,
    openTransactionDetail,
    postEntryOn,
    snapshotWhen,
    tdLinesOf,
    tdSnapshot,
    tdSubtotalOf,
} from './books-report-helpers'
import { ACCOUNT_LABELS } from './journal-entries-helpers'
import { uniqueName } from './record-form-helpers'

// A report that reads every journal line is not live (report engine scale, the human's
// decision of 2026-11-20): it shows a note and Refresh, and new postings show only after
// Refresh. The report here is Transaction Detail by Account; each of the other five
// (General Ledger, Trial Balance, Balance Sheet, Journal, Profit and Loss by Month) is
// the same core band, and four of them press Refresh in their own spec. The report that
// stays live and has no band is pinned in profit-and-loss.spec.ts ("the open report
// re-runs on a post from another page") and, beside a panel that is not live, in
// report-drill-panel.spec.ts.
//
// The phone test runs the builder's Preview: on a phone the builder is a sheet that
// draws its own preview (core SheetSurface), so its band is other code than the
// report's. The report's own band is the same code at every width, so it has no phone
// run.

const EXPENSE = { type: 'Expense', detailType: 'Advertising and marketing' }
const JOURNAL_LINES_SOURCE = 'books.journal_lines'
const REFRESHING = 'Refreshing…'
const UPDATING = 'Updating…'

async function startBooks(page: Page) {
    await login(page)
    await navigateToPackage(page, 'books')
}

// Holds every report run of the page until the returned release is called. The caller
// then waits for the handlers with page.unrouteAll({ behavior: 'wait' }).
async function holdRuns(page: Page): Promise<() => void> {
    let release = () => {}
    const held = new Promise<void>(resolve => {
        release = resolve
    })
    await page.route('**/api/reports/run', async route => {
        await held
        await route.continue()
    })
    return release
}

test.describe('report-manual-refresh', () => {
    test('a report that is not live shows the note, and Refresh by press and by the R key brings in new postings', async ({
        page,
        browser,
    }) => {
        await startBooks(page)
        const expense = uniqueName('RMR expense')
        await openChart(page)
        await createAccount(page, expense, EXPENSE)
        const today = await browserToday(page)

        await openTransactionDetail(page, { accounts: [expense] })
        await expect(page.getByTestId('report-param-accounts')).toContainText(expense)
        await expect(page.getByTestId('report-empty-message')).toBeVisible()
        const { band, note, refresh } = manualBand(page)
        await expect(band).toBeVisible()
        await expect(note).toHaveText(MANUAL_NOTE)
        await expect(refresh).toBeEnabled()
        await page.evaluate(() => {
            ;(window as unknown as { rmrNotReloaded?: boolean }).rmrNotReloaded = true
        })

        // A second browser context signed in as the same user posts through its own
        // session. postEntryOn returns when the server has numbered the entry.
        const otherContext = await browser.newContext()
        try {
            const other = await otherContext.newPage()
            await startBooks(other)
            await openEntriesFromSidebar(other)
            await postEntryOn(other, {
                date: today,
                memo: uniqueName('RMR first'),
                lines: [
                    { account: expense, debit: '21.00' },
                    { account: ACCOUNT_LABELS.checking, credit: '21.00' },
                ],
            })

            // The press starts a run: while it is held the button is off and says so,
            // and the header says "Updating…".
            const release = await holdRuns(page)
            await refresh.click()
            await expect(refresh).toBeDisabled()
            await expect(refresh).toContainText(REFRESHING)
            await expect(page.getByTestId('report-updating')).toHaveText(UPDATING)
            release()
            await page.unrouteAll({ behavior: 'wait' })
            const first = await snapshotWhen(
                page,
                tdSnapshot,
                s => tdLinesOf(s, expense).length === 1 && tdSubtotalOf(s, expense) !== undefined,
                `${expense}'s first line after the Refresh press`
            )
            expect(figureDigits(tdLinesOf(first, expense)[0].cells.amount)).toBe('21.00')
            await expect(refresh).toBeEnabled()
            await expect(refresh).not.toContainText(REFRESHING)
            await expect(page.getByTestId('report-updating')).toHaveCount(0)
            await expect(note).toHaveText(MANUAL_NOTE)

            await postEntryOn(other, {
                date: today,
                memo: uniqueName('RMR second'),
                lines: [
                    { account: expense, debit: '9.00' },
                    { account: ACCOUNT_LABELS.checking, credit: '9.00' },
                ],
            })
        } finally {
            await otherContext.close()
        }

        // The key R, with no modifier, runs the report as the press does. Only the
        // reader's own Refresh makes the button read "Refreshing…" (a run that another
        // worker's share starts does not), so the held run proves the key started it.
        const releaseKey = await holdRuns(page)
        await page.keyboard.press('r')
        await expect(refresh).toBeDisabled()
        await expect(refresh).toContainText(REFRESHING)
        releaseKey()
        await page.unrouteAll({ behavior: 'wait' })
        const second = await snapshotWhen(
            page,
            tdSnapshot,
            s => tdLinesOf(s, expense).length === 2,
            `${expense}'s second line after the R key`
        )
        expect(figureDigits(tdSubtotalOf(second, expense)?.cells.amount)).toBe('30.00')
        const notReloaded = await page.evaluate(
            () => (window as unknown as { rmrNotReloaded?: boolean }).rmrNotReloaded
        )
        expect(notReloaded).toBe(true)
    })

    test('on a phone the Preview of a draft over journal lines shows the note, and Refresh runs the draft again', async ({
        page,
    }) => {
        await startBooks(page)
        // navigateToPackage and the Books sidebar are the desktop layout's: the hub is
        // opened at the default viewport, then the window becomes a phone.
        await openBooksReports(page)
        await page.setViewportSize(PHONE)
        await expect(page.getByTestId('reports-hub')).toBeVisible()

        await page.getByTestId('reports-hub-new').click()
        await page.getByTestId(`reports-new-source-${JOURNAL_LINES_SOURCE}`).click()
        const builder = page.getByTestId('report-builder')
        await expect(builder).toBeVisible()
        await expect(builder.getByTestId('report-builder-mode-design')).toBeVisible()
        await builder.getByTestId('report-builder-mode-preview').click()

        const note = builder.getByTestId('report-builder-manual-note')
        const refresh = builder.getByTestId('report-builder-refresh')
        await expect(builder.getByTestId('report-builder-manual-band')).toBeVisible()
        await expect(note).toHaveText(MANUAL_NOTE)
        await expect(builder.getByTestId('report-table')).toBeVisible()
        await expect(refresh).toBeEnabled()

        // The band fits the phone: the note wraps beside the button, and neither is cut.
        await expect
            .poll(async () => {
                const noteBox = await note.boundingBox()
                const buttonBox = await refresh.boundingBox()
                if (!noteBox || !buttonBox) return null
                return {
                    noteInView: noteBox.x >= 0,
                    buttonInView: buttonBox.x + buttonBox.width <= PHONE.width,
                    apart: noteBox.x + noteBox.width <= buttonBox.x,
                }
            })
            .toEqual({ noteInView: true, buttonInView: true, apart: true })

        const release = await holdRuns(page)
        const run = nextReportRun(page)
        await refresh.click()
        await expect(refresh).toBeDisabled()
        await expect(refresh).toContainText(REFRESHING)
        await expect(builder.getByTestId('report-updating')).toHaveText(UPDATING)
        const body = (await run).postDataJSON() as { spec?: { source?: string } }
        expect(body.spec?.source).toBe(JOURNAL_LINES_SOURCE)
        release()
        await page.unrouteAll({ behavior: 'wait' })
        await expect(refresh).toBeEnabled()
        await expect(refresh).not.toContainText(REFRESHING)
        await expect(builder.getByTestId('report-updating')).toHaveCount(0)
        await expect(note).toHaveText(MANUAL_NOTE)
        await expect(builder.getByTestId('report-table')).toBeVisible()
    })
})
```

Notes for the implementer (do not change the code for them):

- Test 1 starts from the empty state on purpose (the account is new), and the band is there: `ReportTop` is drawn in every run state.
- In test 1 the key `r` is pressed while the focus is on the Refresh button (the last element pressed on that page). The shortcut is not for inputs only; a focused button passes the key on.
- Test 2 saves nothing: the draft is left unsaved, so the shared DB gets no row.
- `waitForRequest` sees a request also while `page.route` holds it, so `nextReportRun` is started before the press and awaited while the run is held.

- [ ] **Step 2: Typecheck and lint**

Run:

```sh
cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck
```

Expected: exit 0, no error.

Run:

```sh
cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH biome check tests/e2e/report-manual-refresh.spec.ts
```

Expected: exit 0.

- [ ] **Step 3: Commit (in the Books repo)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books
git add tests/e2e/report-manual-refresh.spec.ts
git commit -m "test(e2e): the note and Refresh of a report that is not live"
```

- [ ] **Step 4: Hand back the e2e run to the driver**

Do not run e2e. Hand back `NEEDS-RUN:` with the command of the next section, and what is needed from it: the `Running N tests` line, and each failure in full.

---

## The driver's e2e runs

**RED (before task 8d-1, ONE command, the six spec files as they are).** Expected: exactly the six tests named in task 8d-1 fail, each at its poll for the posted figure; every other test of the six files passes. A seventh failure is a finding for the triage below, not a reason to change a test's bar.

```sh
cd /Users/nas/tmp/tinycld/books-purchases && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7210|7211) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7210 E2E_PORT_2=7211 E2E_IMAP_PORT=1210 E2E_IMAP_PORT_2=1211 pnpm exec tinycld-pkg test:e2e books/tests/e2e/journal-report.spec.ts books/tests/e2e/trial-balance.spec.ts books/tests/e2e/balance-sheet.spec.ts books/tests/e2e/general-ledger.spec.ts books/tests/e2e/report-drill-down.spec.ts books/tests/e2e/report-drill-panel.spec.ts
```

**GREEN (ONE command, after both tasks).** The new spec, the eight changed or dependent specs of the six reports, and every other Books spec that opens one of the six reports (found with `command grep -lE 'openJournal|openGeneralLedger|openTrialBalance|openBalanceSheet|openProfitAndLossByMonth|openTransactionDetail|Transaction Detail|General Ledger|Trial Balance|Balance Sheet' -r books/tests/e2e`): 13 files.

```sh
cd /Users/nas/tmp/tinycld/books-purchases && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7210|7211) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7210 E2E_PORT_2=7211 E2E_IMAP_PORT=1210 E2E_IMAP_PORT_2=1211 pnpm exec tinycld-pkg test:e2e books/tests/e2e/report-manual-refresh.spec.ts books/tests/e2e/journal-report.spec.ts books/tests/e2e/trial-balance.spec.ts books/tests/e2e/balance-sheet.spec.ts books/tests/e2e/general-ledger.spec.ts books/tests/e2e/report-drill-down.spec.ts books/tests/e2e/report-drill-panel.spec.ts books/tests/e2e/report-drill-phone.spec.ts books/tests/e2e/profit-and-loss.spec.ts books/tests/e2e/report-exports.spec.ts books/tests/e2e/pay-bills-cash-basis.spec.ts books/tests/e2e/pay-bills-sales-tax-cash.spec.ts books/tests/e2e/opening-balances.spec.ts
```

How to read a failure of this run (for the agent that gets the log):

- A test of the list that fails at a poll for a figure of one of the six reports, after a write made while that report stayed open (in another page, or under another screen and reached again by Back): the report is not live. The fix is `await refreshReport(page)` (or `refreshReport(page, 'report-drill')` for the panel) between the committed write and the poll. Never a longer timeout, a retry of the run, or a reload.
- A test that opens the report again from the hub after a write needs no press: the report screen mounts again and the run's stale time is 0, so it runs again.
- `report-builder-manual-band` not found in the phone test: read the answer of `POST /api/reports/run` for the draft. It must hold `"manual":true` (task 8c: `NotLive` on `books.journal_lines`). When it does not, the fault is in 8c or 8a, not in the spec.
- `reports-new-source-books.journal_lines` not found: the signed-in user's catalog (`GET /api/reports/sources`) does not list the source. Stop and report it; do not seed a report by a superuser write.
