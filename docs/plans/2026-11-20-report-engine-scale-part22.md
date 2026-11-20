# Report engine scale, part 22: the Balance Sheet's e2e, help, 100k run and ledger record (task 9c-2b-2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Books e2e, the Books help and the performance record say what the Balance Sheet does after task 9c-2b-1: an opened account shows "Balance before <month>" and only the lines of the as-of month, on Accrual and on Cash.

**Architecture:** Task 9c-2b-1 (plan part 21, Books Go) changed what an opened account row of the Balance Sheet shows. This part changes no product code. It teaches the layout-report snapshot helper the opening row, extends the one Balance Sheet test that opens an account (a line of last month, the opening row, both methods), corrects one help paragraph, and records the 100,000-line run in the ledger.

**Tech Stack:** Playwright (Books e2e, `books/tests/e2e`), Markdown (Books help, Books ledger), Go test `TestStatementsScale` (run by the driver only).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (section "State after plan part 21") and `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-20-report-engine-scale-part21.md` (its header and the Interfaces block of its task). Human decision (2026-11-20): an opened account shows "Balance before <month>" plus the as-of month's lines; the drill-down still lists every line. The pattern is part 18 (the Trial Balance): `tinycld/docs/plans/2026-11-20-report-engine-scale-part18.md`.

**This part holds task 9c-2b-2 only.** It closes task 9c-2b.

## Global Constraints

- Trees: core `54e278a3` plus this plan; books at the commit of task 9c-2b-1 (`feat(reports): the Balance Sheet reads the stored months on Accrual`). Branch `books/p5-report-engine-scale` in both. Start this task only when 9c-2b-1 is committed in books: `git -C /Users/nas/tmp/tinycld/books-purchases/books log --oneline -3` names it, and `git -C /Users/nas/tmp/tinycld/books-purchases/books status --short` prints nothing.
- This task changes Books only: three e2e files, one help file, one ledger file. It changes no Go file, no core file and no product TypeScript.
- The implementer starts no e2e run and no scale run: each is a `NEEDS-RUN` for the driver (steps 7 and 8). The implementer runs no `pnpm install` and no `packages:generate`.
- A red run of the e2e is not possible in this task: the e2e needs a build and takes more than 4 minutes, so it is the driver's. The plan states the expected GREEN only.
- E2E rules of the repo: no raw PocketBase write, no `page.goto` for in-app navigation, no bumped timeout, one snapshot for each group of assertions, no hard-coded fiscal month, close date or seeded balance (`books-settings.spec.ts` changes them on the shared DB for a few seconds), no entry dated before 2026-07-01.
- Bar at 500,000 lines (human, 2026-11-20): Balance Sheet on Accrual 2 s or less. No bar is held at 100,000 lines.
- A timing run starts only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4.
- Rules of the lane: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md`. Change files with the Edit tool. Read a file with the Read tool right before its first Edit.

## Review Focus

1. A line of an earlier month under an opened account: it must not show as a line, and the account's Current must still count it. Test: step 3, the entry dated `endOfLastMonth(today)`, `opening.current: 700` and `current: 4950`.
2. Cash: the opened account shows the same opening row and lines as on Accrual, and the rows read on Cash must come from a Cash run, not from the Accrual rows still on screen. Test: step 3, the fold before the change of method, `cashRun`, then the second `openReportAccount`.
3. A layout report with no opening row (Profit and Loss, Sales Tax Liability, Customer Balance): `reportLinesUnder` returns the same lines as before. Test: the existing tests of `profit-and-loss.spec.ts` in the run of step 7 ("pressing an account opens its posted lines…" there reads `reportLinesUnder(...).length === 2`).
4. An opened account above another account of the same group: `isInGroup` must not read the opening row as the end of the group. No test opens an account before an `isInGroup` read; the change of step 1 is one predicate (`isOpenedRow`), and the typecheck of step 5 covers its type.
5. The opening row on a phone (core draws each row on two lines there): no Books e2e opens a Balance Sheet account at the phone width. The row is drawn by core's report view, the same code as the General Ledger's "Beginning balance" row; this task adds no phone run, because the phone path runs no Books code for it.

---

## File Structure

| File | Change |
|---|---|
| `books/tests/e2e/books-report-helpers.ts` | `ReportSnapshot` knows the row kind `opening`; `reportLinesUnder` skips it; new `reportOpeningUnder`; `isInGroup` accepts it |
| `books/tests/e2e/sales-reports-helpers.ts` | `SalesRow.kind` takes its type from `ReportSnapshot` (a hand-written copy of the union no longer compiles) |
| `books/tests/e2e/balance-sheet.spec.ts` | The test that opens an account: a line of last month, the opening row, Accrual and Cash |
| `books/help/reports.md` | Line 115: what an opened Balance Sheet account shows |
| `books/docs/plans/general-ledger-ledger.md` | Append the record "Task 9c-2" |

### Task 1 (9c-2b-2): Books: the Balance Sheet's e2e, help, 100k run and ledger record

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/books-report-helpers.ts` (`ReportSnapshot` at line 565, `reportSnapshot` at lines 607-646, `reportLinesUnder` at lines 673-681, `isInGroup` at line 707)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/sales-reports-helpers.ts` (the import at line 7, `SalesRow` at line 27)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/balance-sheet.spec.ts` (the import at lines 20-38, after `figure` at line 240, the test at line 738)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/help/reports.md` (line 115)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (append at the end)

**Interfaces:**
- Consumes (task 9c-2b-1, Books Go): an opened account row of the Balance Sheet has first a row with id `opening:<key>`, whose label core's client writes as `Balance before <Mon YYYY>` (the month of the As of day), then the posted lines of that month up to the day (ids `d:…`). The opening row's Current is the account's balance before the month; its Prior year is the account's whole Prior year figure; its Change and % change are blank. A line's Prior year is 0.00. The rows are the same on Accrual and on Cash.
- Consumes (core, exist): row test ids `report-row-<id>` and `report-cell-<column>-<id>` (`core/server/reports/assemble.go` writes `"opening:" + keyString(key)`); `report-param-method`, `report-filter-option-cash`, `report-header-param-method` ("Accrual basis" / "Cash basis"); a group row carries `aria-expanded`.
- Consumes (Books e2e, exist): `balanceBeforeLabel(day: string): string` and `endOfLastMonth(day: string): string` (`books-report-helpers.ts`); in the spec `snapshotWhen(page, isReady, message)`, `foldRow(page, label)`, `has`, `figure`, `cents`, `type Row`.
- Produces (Books e2e helpers, for later report specs): `ReportSnapshot['rows'][number]['kind']` is `'group' | 'opening' | 'detail' | 'subtotal' | 'computed' | 'total'`; `reportLinesUnder(snapshot, label)` returns the rows of kind `detail` under an opened row, without its opening row; `reportOpeningUnder(snapshot: ReportSnapshot, label: string): ReportSnapshot['rows'][number] | undefined`. Limit: `openReportAccount(page, label)` is not changed and still waits for one line or more, so a later test that opens a Balance Sheet account with no line in the as-of month (its opening row alone) must wait on `reportOpeningUnder` itself.

What was checked when this plan was written (core `54e278a3`, books `726c1d3f` plus the changes of steps 1 to 3 in a scratch copy of `books/tests/e2e`, removed): `tsc` over the Books tsconfig exits 0, and `biome check` of the three files reports no error. Without step 2, `tsc` fails with `tests/e2e/sales-reports-helpers.ts(249,5): error TS2322`. The diffs of that copy are `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part22-scratch-books-report-helpers.ts.diff`, `part22-scratch-sales-reports-helpers.ts.diff` and `part22-scratch-balance-sheet.spec.ts.diff`. The e2e was NOT run.

A fact of the tree that differs from the outline: today `reportSnapshot` drops a row whose id starts with `opening:` (its `kindOf` returns `null`), so `reportLinesUnder` does not return `[]` with the opening row on screen; it does not see the row at all. Step 1 makes the row a kind of its own, so a test can assert it, and then makes `reportLinesUnder` skip it.

Not proven until the driver's run of step 7:
- The opening row of an account with no balance a year earlier prints `0.00` (not a blank) in Prior year. The step 3 test expects `prior: 0`. When the run shows a blank there, the fix round reads `bsBalanceAt` in `books/server/report_balance_sheet.go` and decides which side is wrong; it does not remove the assertion.
- A change of the Method chip keeps the account rows folded as they were, and `foldRow` works on an account row (it carries `aria-expanded`, as every group row does).
- `isInGroup` with an opening row between the group's title and the account: it holds only when core indents the opening row deeper than the group's title (`row.depth > titleDepth`; core gives the row `Depth: depth + 1`, `core/server/reports/assemble.go`). No test of this run reads that.

- [ ] **Step 1: The snapshot helper knows the opening row**

Read `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/books-report-helpers.ts` (lines 555 to 710), then make these five changes with the Edit tool.

(a) In `type ReportSnapshot`, replace

```ts
        kind: 'group' | 'detail' | 'subtotal' | 'computed' | 'total'
```

with

```ts
        kind: 'group' | 'opening' | 'detail' | 'subtotal' | 'computed' | 'total'
```

(b) In the comment above `reportSnapshot`, replace

```ts
// row kind comes from core's row id: g: a group or an account, d: a line, s: a group's
// total, c: a computed row, total: the Total.
```

with

```ts
// row kind comes from core's row id: g: a group or an account, opening: an opened account's
// balance before the as-of month, d: a line, s: a group's total, c: a computed row, total:
// the Total.
```

(c) In `reportSnapshot`'s `kindOf` (the one that has the line `if (id.startsWith('c:')) return 'computed'`; the file has other `kindOf` functions, do not change them), replace

```ts
            if (id.startsWith('g:')) return 'group'
            if (id.startsWith('d:')) return 'detail'
            if (id.startsWith('s:')) return 'subtotal'
            if (id.startsWith('c:')) return 'computed'
```

with

```ts
            if (id.startsWith('g:')) return 'group'
            if (id.startsWith('opening:')) return 'opening'
            if (id.startsWith('d:')) return 'detail'
            if (id.startsWith('s:')) return 'subtotal'
            if (id.startsWith('c:')) return 'computed'
```

(d) Replace the whole of `reportLinesUnder` with its comment, which is

```ts
// The opened lines under the row labelled `label`: the detail rows that follow it until the
// next row that is not a line. Empty when the row is not mounted or not open.
export function reportLinesUnder(snapshot: ReportSnapshot, label: string): ReportSnapshot['rows'] {
    const start = snapshot.rows.findIndex(row => row.kind !== 'detail' && row.label === label)
    if (start < 0) return []
    const after = snapshot.rows.slice(start + 1)
    const end = after.findIndex(row => row.kind !== 'detail')
    return end < 0 ? after : after.slice(0, end)
}
```

with

```ts
// A row that shows only under an opened row: a line, or the opening row above the lines.
function isOpenedRow(row: ReportSnapshot['rows'][number]): boolean {
    return row.kind === 'detail' || row.kind === 'opening'
}

// The rows an opened row labelled `label` shows beneath it, until the next row that is not
// one of them: its opening row, when the report has one, then its lines.
function reportRowsUnder(snapshot: ReportSnapshot, label: string): ReportSnapshot['rows'] {
    const start = snapshot.rows.findIndex(row => !isOpenedRow(row) && row.label === label)
    if (start < 0) return []
    const after = snapshot.rows.slice(start + 1)
    const end = after.findIndex(row => !isOpenedRow(row))
    return end < 0 ? after : after.slice(0, end)
}

// The opened lines under the row labelled `label`, without its opening row. Empty when the
// row is not mounted or not open.
export function reportLinesUnder(snapshot: ReportSnapshot, label: string): ReportSnapshot['rows'] {
    return reportRowsUnder(snapshot, label).filter(row => row.kind === 'detail')
}

// The opening row ("Balance before <month>", see balanceBeforeLabel) under the row labelled
// `label`. Undefined when the row is not mounted, not open, or its report has no opening row.
export function reportOpeningUnder(
    snapshot: ReportSnapshot,
    label: string
): ReportSnapshot['rows'][number] | undefined {
    return reportRowsUnder(snapshot, label).find(row => row.kind === 'opening')
}
```

(e) In `isInGroup`, replace

```ts
        .every(row => (row.kind === 'group' || row.kind === 'detail') && row.depth > titleDepth)
```

with

```ts
        .every(row => (row.kind === 'group' || isOpenedRow(row)) && row.depth > titleDepth)
```

Change nothing else in the file: `TbSnapshot`, `tbSnapshot`, `detailsUnder`, `openingUnder`, `balanceBeforeLabel` and `openReportAccount` stay as they are.

- [ ] **Step 2: `SalesRow` takes its kind from the snapshot**

`salesRows` (`/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/sales-reports-helpers.ts`, line 247) copies `reportSnapshot` rows into `SalesRow[]`, and `SalesRow` holds a hand-written copy of the kind union, so step 1 breaks its typecheck. Read the file (lines 1 to 32), then:

Replace

```ts
import { openBooksReports, reportSnapshot } from './books-report-helpers'
```

with

```ts
import { openBooksReports, type ReportSnapshot, reportSnapshot } from './books-report-helpers'
```

and in `interface SalesRow` replace

```ts
    kind: 'group' | 'detail' | 'subtotal' | 'computed' | 'total'
```

with

```ts
    kind: ReportSnapshot['rows'][number]['kind']
```

- [ ] **Step 3: The Balance Sheet test of an opened account**

Read `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/balance-sheet.spec.ts` (lines 1 to 60, 213 to 245, 292 to 310 and 738 to 845), then make these four changes with the Edit tool.

(a) The import from `./books-report-helpers` gets three names (`balanceBeforeLabel`, `endOfLastMonth`, `reportOpeningUnder`), in biome's order. Replace

```ts
import {
    browserToday,
    chooseAsOf,
    chooseRows,
    dayWords,
    isInGroup,
    openBalanceSheet,
    openReportAccount,
    postEntryOn,
    type ReportSnapshot,
    refreshReport,
    reportLabelsThrough,
    reportLinesUnder,
    reportSnapshot,
```

with

```ts
import {
    balanceBeforeLabel,
    browserToday,
    chooseAsOf,
    chooseRows,
    dayWords,
    endOfLastMonth,
    isInGroup,
    openBalanceSheet,
    openReportAccount,
    postEntryOn,
    type ReportSnapshot,
    refreshReport,
    reportLabelsThrough,
    reportLinesUnder,
    reportOpeningUnder,
    reportSnapshot,
```

(b) Two helpers after `figure`. Replace

```ts
function figure(snapshot: ReportSnapshot, label: string, column: string): number | null {
    return cents(rowOf(snapshot, label).cells[column])
}
```

with

```ts
function figure(snapshot: ReportSnapshot, label: string, column: string): number | null {
    return cents(rowOf(snapshot, label).cells[column])
}

// What the opened account row labelled `label` shows, from one snapshot: its own Current,
// its opening row (null when none shows), then its lines.
function openedAccount(snapshot: ReportSnapshot, label: string) {
    const shown = (row: Row) => ({
        label: row.label,
        current: cents(row.cells.current),
        prior: cents(row.cells.prior),
        change: row.cells.change ?? '',
        pct: row.cells.pct ?? '',
    })
    const opening = reportOpeningUnder(snapshot, label)
    return {
        current: figure(snapshot, label, 'current'),
        opening: opening ? shown(opening) : null,
        lines: reportLinesUnder(snapshot, label).map(shown),
    }
}

// True when the account row labelled `label` is open onto its opening row and `count` lines.
function isOpenOnto(snapshot: ReportSnapshot, label: string, count: number): boolean {
    return (
        reportOpeningUnder(snapshot, label) !== undefined &&
        reportLinesUnder(snapshot, label).length === count
    )
}
```

(c) The test's title, and a third entry, dated the last day of the month before today's. It is posted first, so the two entries of today keep their order (`first`, `second`). Replace

```ts
    test('pressing an account opens its posted lines, labelled, in posting order', async ({
        page,
    }) => {
        const bank = accountName('BS7 bank')
        const card = accountName('BS7 card')
        const lineMemo = uniqueName('BS7 line memo')
        const entryMemo = uniqueName('BS7 entry memo')
        await startBooks(page)
        await createAccounts(page, [
            { name: bank, kind: BANK },
            { name: card, kind: CREDIT_CARD },
        ])
        await openEntriesFromSidebar(page)
        const today = await browserToday(page)
        await postEntryOn(page, {
```

with

```ts
    test('pressing an account opens its balance before the month, then the lines of that month, on Accrual and on Cash', async ({
        page,
    }) => {
        const bank = accountName('BS7 bank')
        const card = accountName('BS7 card')
        const lineMemo = uniqueName('BS7 line memo')
        const entryMemo = uniqueName('BS7 entry memo')
        await startBooks(page)
        await createAccounts(page, [
            { name: bank, kind: BANK },
            { name: card, kind: CREDIT_CARD },
        ])
        await openEntriesFromSidebar(page)
        const today = await browserToday(page)
        // A line of the month before today's month: the opened account holds it in its
        // opening row and does not list it. The day is after 2026-07-01 (see the header).
        await postEntryOn(page, {
            date: endOfLastMonth(today),
            memo: uniqueName('BS7 entry of last month'),
            lines: [
                { account: bank, debit: '7.00' },
                { account: card, credit: '7.00' },
            ],
        })
        await postEntryOn(page, {
```

(d) The assertions on the opened account. Replace

```ts
        await openReportAccount(page, bank)
        const opened = await snapshotWhen(
            page,
            s => reportLinesUnder(s, bank).length === 2,
            `${bank} opened onto its two lines`
        )
        const lines = reportLinesUnder(opened, bank).map(row => ({
            label: row.label,
            current: cents(row.cells.current),
            prior: cents(row.cells.prior),
            change: row.cells.change ?? '',
            pct: row.cells.pct ?? '',
        }))
        expect(lines).toEqual([
            {
                label: `${today} · ${first} · ${lineMemo}`,
                current: 3000,
                prior: 0,
                change: '',
                pct: '',
            },
            {
                label: `${today} · ${second} · ${entryMemo}`,
                current: 1250,
                prior: 0,
                change: '',
                pct: '',
            },
        ])
```

with

```ts
        // The account's Current is its opening row plus its two lines of this month. The
        // account is new, so a year earlier it has no balance: Prior year reads 0.00.
        const expected = {
            current: 4950,
            opening: {
                label: balanceBeforeLabel(today),
                current: 700,
                prior: 0,
                change: '',
                pct: '',
            },
            lines: [
                {
                    label: `${today} · ${first} · ${lineMemo}`,
                    current: 3000,
                    prior: 0,
                    change: '',
                    pct: '',
                },
                {
                    label: `${today} · ${second} · ${entryMemo}`,
                    current: 1250,
                    prior: 0,
                    change: '',
                    pct: '',
                },
            ],
        }

        await openReportAccount(page, bank)
        const opened = await snapshotWhen(
            page,
            s => isOpenOnto(s, bank, 2),
            `${bank} opened onto its opening row and its two lines on Accrual`
        )
        expect(openedAccount(opened, bank), `${bank} on Accrual`).toEqual(expected)

        // Cash shows the same rows: a journal entry counts on its own date on both methods.
        // The account is folded before the change of method, and core caches a run by its
        // body, so the rows that show after the press on Cash come from a Cash run.
        await foldRow(page, bank)
        await snapshotWhen(
            page,
            s =>
                has(s, bank) &&
                reportOpeningUnder(s, bank) === undefined &&
                reportLinesUnder(s, bank).length === 0,
            `${bank} folded`
        )
        const basis = page.getByTestId('report-header-param-method')
        await expect(basis).toHaveText('Accrual basis')
        const cashRun = page.waitForResponse(response => {
            if (!response.url().includes('/api/reports/run') || !response.ok()) return false
            const body = response.request().postDataJSON() as {
                params?: { method?: string }
            } | null
            return body?.params?.method === 'cash'
        })
        await page.getByTestId('report-param-method').click()
        await page.getByTestId('report-filter-option-cash').click()
        await expect(basis).toHaveText('Cash basis')
        await cashRun
        await openReportAccount(page, bank)
        const onCash = await snapshotWhen(
            page,
            s => isOpenOnto(s, bank, 2),
            `${bank} opened onto its opening row and its two lines on Cash`
        )
        expect(openedAccount(onCash, bank), `${bank} on Cash`).toEqual(expected)
```

Change nothing after it: the rest of the test (the Retained earnings row that does not open) runs on Cash and reads no figure. The lines before it (`const closed = …`, `expect(reportLinesUnder(closed, bank)).toEqual([])`) stay.

Why each figure: bank is a Bank account (debit normal), so a debit adds. Last month 7.00 is the opening row's Current (700). Today 30.00 and 12.50 are the two lines. The account row's Current is 7.00 + 30.00 + 12.50 = 49.50 (4950): the balance before the month plus the month's lines. The account is the test's own (`accountName`), so no other worker changes these figures.

- [ ] **Step 4: The help paragraph**

In `/Users/nas/tmp/tinycld/books-purchases/books/help/reports.md` (Read lines 108 to 118 first) replace line 115, which is

```markdown
To see the lines of an account, press its account row. Its posted lines open below it, in date order, then by entry number. Each line shows its date, its entry number and its memo. Current shows the amount of the line. Prior year shows the amount when the line is on or before the Prior year day, and 0.00 when it is not. Change and % change stay blank on the lines. Retained earnings and Net income do not open.
```

with

```markdown
To see the lines of an account, press its account row. The first row below it reads "Balance before" and the month of the As of day, for example "Balance before Nov 2026". Its Current shows the balance of the account at the end of the month before. Its Prior year shows the balance of the account on the Prior year day. Below it are the posted lines of that month, up to the As of day, in date order, then by entry number. Each line shows its date, its entry number and its memo. Current shows the amount of the line. Prior year shows 0.00 on the lines. Change and % change stay blank on the "Balance before" row and on the lines. Lines of earlier months do not show here. To see every line of an account, press its Current figure. An account opens the same way on Accrual and on Cash. Retained earnings and Net income do not open.
```

Change no other line of the file. Line 117 already tells how a press on the figure opens the transactions.

- [ ] **Step 5: Typecheck and biome (Books)**

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck
```

Expected: exit 0, no `error TS` line. An error `tests/e2e/sales-reports-helpers.ts(249,5): error TS2322` means step 2 is not done.

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check tests/e2e/balance-sheet.spec.ts tests/e2e/books-report-helpers.ts tests/e2e/sales-reports-helpers.ts
```

Expected: `Checked 3 files`, no error. When biome reports a format or import-order error, fix the file with the Edit tool to what biome prints; do not run `biome --write` on other files.

- [ ] **Step 6: Commit (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add tests/e2e/books-report-helpers.ts tests/e2e/sales-reports-helpers.ts tests/e2e/balance-sheet.spec.ts help/reports.md && git commit -m "test(reports): an opened Balance Sheet account shows its balance before the month"
```

- [ ] **Step 7: Hand the e2e run to the driver**

Do not start it. Hand back:

```
NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7210|7211) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7210 E2E_PORT_2=7211 E2E_IMAP_PORT=1210 E2E_IMAP_PORT_2=1211 pnpm exec tinycld-pkg test:e2e books/tests/e2e/balance-sheet.spec.ts books/tests/e2e/trial-balance.spec.ts books/tests/e2e/report-drill-down.spec.ts books/tests/e2e/report-exports.spec.ts books/tests/e2e/report-manual-refresh.spec.ts books/tests/e2e/profit-and-loss.spec.ts
Before it: the help file changed, so run `pnpm run packages:generate` in /Users/nas/tmp/tinycld/books-purchases/tinycld once, when no other run is in progress.
Needed from it: the `Running N tests` line (N is the tests of these six files, not the whole suite), the line of dots, and each failure in full.
```

The first five files are the Books specs that open the Balance Sheet or the Trial Balance (`command grep -rlE "openBalanceSheet|openTrialBalance" /Users/nas/tmp/tinycld/books-purchases/books/tests/e2e` names four of them) and `report-manual-refresh.spec.ts`, which runs both reports. `profit-and-loss.spec.ts` is in the run because it reads `reportLinesUnder` on a report with no opening row (Review Focus 3). The run needs the commits of task 9c-2b-1 and of step 6 in the tree.

Expected GREEN: no `F` in the line of dots, and no failed test. In `balance-sheet.spec.ts` the changed test passes: "pressing an account opens its balance before the month, then the lines of that month, on Accrual and on Cash".

When a test of this run fails, the driver gives the failure to a fix round. The fix finds the cause at its source; it bumps no timeout and removes no assertion.

- [ ] **Step 8: Hand the 100k run to the driver**

Do not start it. Hand back, with the result of step 7:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4; not while the e2e of step 7 runs) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9c2 go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
Needed from it: the files scale-task9c2/st-results.md and scale-task9c2/st-plans.md, the PASS or FAIL line of the test, and the `median of 5` of the two rows `BS 1 accrual, as of` and `BS 2 cash, as of`.
```

No bar is held at 100,000 lines, so the test passes when each run with a bar shows a group row; a run core refuses is recorded as refused. BS 2 (Cash) is reported with BS 1 because task 9c-2b-1 changed the shape of the Cash run too (a lookup of the lines for each account in place of full sums).

- [ ] **Step 9: Record the result in the ledger (Books)**

With the files of step 8, append to the end of `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (Read the end of the file first; append with the Edit tool). Write the heading and the first two paragraphs as they are here:

```markdown
#### Task 9c-2: the Balance Sheet reads the stored months (books `server/report_balance_sheet.go`, 2026-11-20)

Before: Current and Prior year of every account, Retained earnings and Net income were sums over the posted lines up to the day of the column, so each run read the whole history. After: the report's rows are the posted lines of the month of the As of day, on both methods. Current and Prior year of an account add the balance before the month of their day and the lines of that month. On Accrual the balance before the month is a lookup over the stored month balances (`books_account_balances`); on Cash it is a lookup of the account's lines, because no stored month holds the cash rows. An account with stored months and no line in the month gets a row through the fill, under its own section, class and type. Retained earnings and Net income read the stored months plus the lines of their day's month on Accrual. The figures do not change on either method: `TestBalanceSheetStoredMonthsEqualTheLines` runs the line read beside the new read.

What the reader sees (human decision, 2026-11-20): an opened account shows "Balance before" and the month of the As of day, then the posted lines of that month up to the day, on Accrual and on Cash. Lines of earlier months do not show there; a press on the Current or Prior year figure still opens every line.
```

The test named there is in `/Users/nas/tmp/tinycld/books-purchases/books/server/report_balance_sheet_stored_test.go` (`command grep -n "^func TestBalanceSheetStoredMonthsEqualTheLines(" <that file>` prints one line). When it prints none, stop and report it: task 9c-2b-1 is then not the tree this plan was written for.

Then add, under them:

1. The line `At 100,000 lines (median of 5, core <core hash>, books <books hash>):` with the short hashes of the two trees the run of step 8 used: `git -C /Users/nas/tmp/tinycld/books-purchases/tinycld rev-parse --short HEAD` and the Books HEAD at the time of the run of step 8 (`git -C /Users/nas/tmp/tinycld/books-purchases/books rev-parse --short HEAD`, read before the commit of step 10).
2. A table with the header `| run | after task 9c-1 | after task 9c-2 | group rows | rows |` and one row for each of the eleven runs, in the order of `st-results.md`. The column `after task 9c-1` is this text, one for each run in order: `330ms`, `149ms`, `178ms`, `1.424s`, `1.852s`, `163ms`, `1.646s`, `30ms`, `18ms`, `130ms`, `32ms` (TB 1, TB 2, TB 3, BS 1, BS 2, TD 1 to TD 6). The three other columns are the columns `median of 5`, `group rows` and `rows` of `scale-task9c2/st-results.md`, copied as they are; a refused run is written `refused after <time>`.
3. The heading line `Reads of the line tables and of the stored months in the longest statement of BS 1 accrual, as of, and of BS 2 cash, as of (scale-task9c2/st-plans.md):` and under it, for each of the two runs, the statement's heading line of `st-plans.md` as plain text (for example `BS 1 accrual, as of (statement 5: 1.3523s, 14 rows, 76114 chars):`) and, in one fenced block, every plan line of that statement that `st-plans.md` prints (the measure prints the lines that name `books_journal_lines`, `books_cash_basis_lines` or `books_account_balances`). The longest statement of a run is the one with the largest time in its heading. Write what the file shows.
4. One sentence that says whether a `SCAN books_journal_lines` line or a `SCAN books_cash_basis_lines` line is still in a Balance Sheet statement, and in which run and statement.
5. One sentence that names every run that is refused at 100k, or `No run is refused at 100k.`

Change no code in this step, whatever the run shows. A Balance Sheet run that is slower than its `after task 9c-1` figure, a `SCAN` of a line table that is still there, or a `group rows` or `rows` count of BS 1 or BS 2 that differs from the 10 and 20 of the run before goes in the record as measured and in the hand-back of the task, with the path of `st-plans.md`. (The counts may differ with reason: the rows are now the lines of one month, and the fill adds account rows. The record states the numbers; it does not judge them.) The next plan part chooses the next change from it.

- [ ] **Step 10: Commit the record (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add docs/plans/general-ledger-ledger.md && git commit -m "docs: the Balance Sheet at 100k after it reads the stored months"
```

---

## After this part

Task 9c-2b is complete with this part. The later tasks of the unit (the outline, `plan-outline.md`) are planned from the record of step 9: the Cash month table (task 9d), the scan of the as-of month's base rows with no Accounts filter (candidate 9b-3) and the 500k measure against the bars. They are not in this part.

For the driver: the help file changed, so run `pnpm run packages:generate` in `/Users/nas/tmp/tinycld/books-purchases/tinycld` once before the e2e of step 7, when no other run is in progress.

## Self-review

- Coverage of 9c-2b-2 (outline, "State after plan part 21"): (a) the e2e change is steps 1 to 3 and the run of step 7; (b) the help line is step 4; (c) the 100k run with BS 1 and BS 2 is step 8; (d) the ledger record is steps 9 and 10. Typecheck and biome are step 5.
- No placeholder: every code step holds the text to write; the ledger step names each column and its source.
- Names: `reportOpeningUnder`, `isOpenedRow`, `reportRowsUnder` (step 1) are the names step 3 and the Interfaces block use; `openedAccount` and `isOpenOnto` are defined and used in step 3 only.
- Review Focus: lines 1 and 2 are pinned in step 3; line 3 by a spec in the run of step 7; lines 4 and 5 have no e2e, with the reason written on each.
