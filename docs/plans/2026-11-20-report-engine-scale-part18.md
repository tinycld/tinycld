# Report Engine Scale, Part 18: The Trial Balance's E2E, Help, 100k Run And Ledger Record Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The Books e2e, the Books help and the performance record say what the Trial Balance does after task 9c-1a: an opened account shows "Balance before <month>" and only the lines of the as-of month.

**Architecture:** Task 9c-1a (plan part 17, Books Go) changed what an opened account row shows. This part changes no product code. It teaches the e2e snapshot helper the opening row, rewrites the one test that expected a prior-year line under Retained earnings, corrects one help paragraph, and records the 100,000-line run in the ledger.

**Tech Stack:** Playwright (Books e2e, `books/tests/e2e`), Markdown (Books help, Books ledger), Go test `TestStatementsScale` (run by the driver only).

**Spec:** `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/plan-outline.md` (section "State after plan part 17") and `/Users/nas/tmp/tinycld/books-purchases/tinycld/docs/plans/2026-11-20-report-engine-scale-part17.md` (its header, the Interfaces block of its task, and its last section). Human decision (2026-11-20): an opened account shows "Balance before <month>" plus the as-of month's lines; the drill-down still lists every line.

**This part holds task 9c-1b only.** It closes task 9c-1.

## Global Constraints

- Trees: core `15a017bf` plus this plan; books at the commit of task 9c-1a (`feat(reports): the Trial Balance reads the stored months`). Branch `books/p5-report-engine-scale` in both. Start this task only when 9c-1a is committed in books: `git -C /Users/nas/tmp/tinycld/books-purchases/books log --oneline -1` names it.
- This task changes Books only: two e2e files, one help file, one ledger file. It changes no Go file, no core file and no product TypeScript.
- The implementer starts no e2e run and no scale run: each is a `NEEDS-RUN` for the driver (steps 7 and 8). The implementer runs no `pnpm install` and no `packages:generate`.
- A red run of the e2e is not possible in this task: the e2e needs a build and takes more than 4 minutes, so it is the driver's. With 9c-1a in the tree, the unchanged spec fails in the test "earlier fiscal years fold onto Retained earnings…" at the wait "the prior-year line under Retained earnings". The plan states the expected GREEN only.
- E2E rules of the repo: no raw PocketBase write, no `page.goto` for in-app navigation, no bumped timeout, one snapshot for each group of assertions, no hard-coded fiscal month, close date or seeded balance (`books-settings.spec.ts` changes them on the shared DB for a few seconds).
- Bars at 500,000 lines (human, 2026-11-20): Trial Balance with an Accounts filter 1 s or less; every account 2 s or less. No bar is held at 100,000 lines.
- A timing run starts only on a quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4.
- Rules of the lane: `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/agent-rules.md`. Change files with the Edit tool. Read a file with the Read tool right before its first Edit.

## Review Focus

1. An opened account with no line in the as-of month (Retained earnings): its opening row alone shows, and the wait of `openAccountRow` must end. Test: step 3, the Retained earnings block.
2. An as-of day in an earlier month (Custom day): the opening row names that month, not the month of today, and the line of that day shows under its own account. Test: step 3, the block after `chooseAsOf(page, { day: priorDay })`.
3. A line of an earlier month under an opened account as of today: it must not show. Test: step 3, `income.filter(row => !row.label.startsWith(thisMonth))`.
4. Collapse all and Expand all with an opening row on screen: the lines close and open again, and `detailsUnder` does not count the opening row. Test: the existing test "opening an account shows its posted lines…" (its waits use `detailsUnder(...).length`), with the new label assertion of step 3.
5. The opening row on a phone (core draws each row on two lines there): no Books e2e opens an account at the phone width. The row is drawn by core's report view, the same code as the General Ledger's "Beginning balance" row; this task adds no phone run, because the phone path runs no Books code for it.

---

### Task 1 (9c-1b): Books: the Trial Balance's e2e, help, 100k run and ledger record

**Files:**
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/books-report-helpers.ts` (the import at line 14, `TbSnapshot` at line 36, `tbSnapshot` at lines 317-360, `detailsUnder` and `openAccountRow` at lines 362-377)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/trial-balance.spec.ts` (the import at lines 11-27, the test at line 402, the test at line 525)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/help/reports.md` (line 35)
- Modify: `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (append at the end)

**Interfaces:**
- Consumes (task 9c-1a, Books Go): an opened account row of the Trial Balance has first a row with id `opening:<key>`, whose label core's client writes as `Balance before <Mon YYYY>` (the month of the As of day, for example `Balance before Nov 2026`), then the posted lines of that month up to the day (ids `d:…`). The opening row shows for every opened account, also when the account has no line in that month. Its Debit and Credit cells have the test ids `report-cell-debit-opening:<key>` and `report-cell-credit-opening:<key>`.
- Consumes (core, exists): `bucketWords(type: BucketLabelType, key: string | number): string` from `@tinycld/core/reports/lib/dates` (`bucketWords('month', '2026-11')` is `Nov 2026`); `pressToolbar(page, testId, label)` and `rowOf(page, label)` from `@tinycld/core/e2e-helpers`.
- Consumes (Books e2e, exist): `snapshotWhen(page, take, isReady, message)`, `browserToday(page)`, `chooseAsOf(page, { preset } | { day })`, `tbSnapshot(page)`; in the spec `groupRow`, `groupLabels`, `figures`, `isWhole`.
- Produces (Books e2e helpers, for later report specs): `TbSnapshot['rows'][number]['kind']` is `'group' | 'opening' | 'detail'`; `detailsUnder(snapshot, label)` returns the rows of kind `detail` under an account, without its opening row; `openingUnder(snapshot: TbSnapshot, label: string): TbSnapshot['rows'][number] | undefined`; `balanceBeforeLabel(day: string): string` (a `YYYY-MM-DD` day gives `Balance before <Mon YYYY>`); `openAccountRow(page, label)` waits for the opening row, not for a line.

What was checked when this plan was written: the two e2e files with the changes of steps 1 to 3 pass `tsc --noEmit` over the Books tsconfig and `biome check` (a scratch copy, removed). The e2e was not run. The diffs of that copy are `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/part18-scratch-helpers.diff` and `part18-scratch-spec.diff`.

Why the helper changes: today `tbSnapshot` drops every row whose id does not start with `g:` or `d:`, so it does not see the opening row, and `openAccountRow` waits for at least one line. An opened Retained earnings row has the opening row and no line, so that wait would never end.

- [ ] **Step 1: The snapshot helper reads the opening row**

In `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/books-report-helpers.ts` make these five edits.

1a. The import (line 14). Replace

```ts
import { rangeWords } from '@tinycld/core/reports/lib/dates'
```

with

```ts
import { bucketWords, rangeWords } from '@tinycld/core/reports/lib/dates'
```

1b. The row kind of `TbSnapshot` (line 36). Replace

```ts
        kind: 'group' | 'detail'
        label: string
        number: string
```

with

```ts
        kind: 'group' | 'opening' | 'detail'
        label: string
        number: string
```

1c. The comment over `tbSnapshot` (line 317). Replace

```ts
// Every mounted row of the report table, top to bottom, with its No., Debit and Credit
// cells, and the Total row, read in ONE pass so no re-render lands between two reads. The
```

with

```ts
// Every mounted row of the report table, top to bottom, with its No., Debit and Credit
// cells, and the Total row, read in ONE pass so no re-render lands between two reads. The
// row kind comes from core's row id: g: an account, opening: its balance before the as-of
// month, d: a line. The
```

1d. Inside `tbSnapshot`. Replace

```ts
        const rows: TbSnapshot['rows'] = []
        let total = { label: '', debit: '', credit: '' }
```

with

```ts
        const kindOf = (id: string): TbSnapshot['rows'][number]['kind'] | null => {
            if (id.startsWith('g:')) return 'group'
            if (id.startsWith('opening:')) return 'opening'
            if (id.startsWith('d:')) return 'detail'
            return null
        }
        const rows: TbSnapshot['rows'] = []
        let total = { label: '', debit: '', credit: '' }
```

and, a few lines under it, replace

```ts
            const kind = id.startsWith('g:') ? 'group' : id.startsWith('d:') ? 'detail' : null
            if (!kind) continue
```

with

```ts
            const kind = kindOf(id)
            if (!kind) continue
```

`kindOf` is declared inside the `page.evaluate` callback: that callback runs in the browser and reads nothing from the module.

1e. `detailsUnder` and `openAccountRow` (lines 362-377). Replace the whole of

```ts
// The rows that follow the account row labelled `label` until the next account row.
export function detailsUnder(snapshot: TbSnapshot, label: string): TbSnapshot['rows'] {
    const start = snapshot.rows.findIndex(row => row.kind === 'group' && row.label === label)
    if (start < 0) return []
    const after = snapshot.rows.slice(start + 1)
    const end = after.findIndex(row => row.kind !== 'detail')
    return end < 0 ? after : after.slice(0, end)
}

// Opens an account row and waits until its lines show beneath it.
export async function openAccountRow(page: Page, label: string): Promise<void> {
    await rowOf(page, label).click()
    await expect
        .poll(async () => detailsUnder(await tbSnapshot(page), label).length)
        .toBeGreaterThan(0)
}
```

with

```ts
// The rows that follow the account row labelled `label` until the next account row: its
// opening row, then its lines.
function rowsUnder(snapshot: TbSnapshot, label: string): TbSnapshot['rows'] {
    const start = snapshot.rows.findIndex(row => row.kind === 'group' && row.label === label)
    if (start < 0) return []
    const after = snapshot.rows.slice(start + 1)
    const end = after.findIndex(row => row.kind === 'group')
    return end < 0 ? after : after.slice(0, end)
}

// The posted lines under the account row labelled `label`, without its opening row.
export function detailsUnder(snapshot: TbSnapshot, label: string): TbSnapshot['rows'] {
    return rowsUnder(snapshot, label).filter(row => row.kind === 'detail')
}

// The opening row ("Balance before <month>") under the account row labelled `label`.
export function openingUnder(
    snapshot: TbSnapshot,
    label: string
): TbSnapshot['rows'][number] | undefined {
    return rowsUnder(snapshot, label).find(row => row.kind === 'opening')
}

// The opening row's label for an as-of day (YYYY-MM-DD): the month in the words core's
// report view writes for a month label.
export function balanceBeforeLabel(day: string): string {
    return `Balance before ${bucketWords('month', day.slice(0, 7))}`
}

// Opens an account row and waits until its opening row shows beneath it. Every opened
// account has one, also an account with no line in the as-of month; a test that reads the
// lines waits for them itself.
export async function openAccountRow(page: Page, label: string): Promise<void> {
    await rowOf(page, label).click()
    await expect
        .poll(async () => openingUnder(await tbSnapshot(page), label) !== undefined, {
            message: `the opening row under ${label}`,
        })
        .toBe(true)
}
```

The spec is the only caller of `detailsUnder`, `openAccountRow` and `tbSnapshot` (`command grep -rln "detailsUnder\|openAccountRow\|tbSnapshot" /Users/nas/tmp/tinycld/books-purchases/books/tests` prints the helper file and `trial-balance.spec.ts`).

- [ ] **Step 2: The spec imports the two new helpers**

In `/Users/nas/tmp/tinycld/books-purchases/books/tests/e2e/trial-balance.spec.ts` replace the import from `./books-report-helpers` (lines 11-27) with

```ts
import {
    balanceBeforeLabel,
    browserToday,
    chooseAsOf,
    dayWords,
    detailsUnder,
    endOfLastMonth,
    expectedHeading,
    filterToAccounts,
    openAccountRow,
    openBooksReports,
    openEntriesFromSidebar,
    openingUnder,
    openTrialBalance,
    refreshReport,
    snapshotWhen,
    type TbSnapshot,
    tbSnapshot,
} from './books-report-helpers'
```

(`openingUnder` goes after `openEntriesFromSidebar`: biome sorts the names so.)

- [ ] **Step 3: The spec expects the opening row and only the as-of month's lines**

3a. In the test "opening an account shows its posted lines, each labelled with its date, number and memo" (line 402), replace

```ts
        // No. names the account on its row and is blank on its opened lines.
        expect(groupRow(opened, account)?.number).toBe(number)
```

with

```ts
        // The first row under the account is its balance before the as-of month.
        expect(openingUnder(opened, account)?.label).toBe(balanceBeforeLabel(today))
        // No. names the account on its row and is blank on its opened lines.
        expect(groupRow(opened, account)?.number).toBe(number)
```

(`opened` and `today` exist in that test: `today` is `await browserToday(page)`, and the report runs as of today.)

3b. In the test "earlier fiscal years fold onto Retained earnings, and a day in that year shows them on their own" (line 525), replace everything from the line `await openAccountRow(page, 'Consulting Income')` to the end of the test (the lines 554 to 584: both `openAccountRow` blocks, the `chooseAsOf(page, { day: priorDay })` block and the closing `})` of the test) with

```ts
        // An opened account shows its balance before the as-of month, then only the lines
        // of that month: the seeded Consulting Income lines of earlier months stay out.
        const asOf = await browserToday(page)
        const thisMonth = `${asOf.slice(0, 7)}-`
        await openAccountRow(page, 'Consulting Income')
        const withIncome = await snapshotWhen(
            page,
            tbSnapshot,
            s => detailsUnder(s, 'Consulting Income').length > 0,
            'the lines of this month under Consulting Income'
        )
        expect(openingUnder(withIncome, 'Consulting Income')?.label).toBe(balanceBeforeLabel(asOf))
        const income = detailsUnder(withIncome, 'Consulting Income')
        expect(income.filter(row => !row.label.startsWith(thisMonth))).toEqual([])

        // The opened Retained earnings row lists no line of an earlier fiscal year: its
        // opening row carries them. No spec posts to Retained earnings, so the row has no
        // line of its own this month and its opening row holds its whole balance.
        await openAccountRow(page, 'Retained earnings')
        const opened = await snapshotWhen(
            page,
            tbSnapshot,
            s => openingUnder(s, 'Retained earnings') !== undefined,
            'the opening row under Retained earnings'
        )
        const before = openingUnder(opened, 'Retained earnings')
        expect(before?.label).toBe(balanceBeforeLabel(asOf))
        expect(detailsUnder(opened, 'Retained earnings')).toEqual([])
        expect(figures(before)).toEqual(figures(groupRow(opened, 'Retained earnings')))
        // The side depends on the seed and the fiscal month; one side holds a figure.
        expect(`${before?.debit}${before?.credit}`).toMatch(/\d/)

        // Closed first, so the press on Consulting Income as of the earlier day opens it.
        await pressToolbar(page, 'report-fold-all', 'Collapse all')
        await snapshotWhen(
            page,
            tbSnapshot,
            s =>
                groupRow(s, 'Consulting Income') !== undefined &&
                s.rows.every(row => row.kind === 'group'),
            'every account closed by Collapse all'
        )

        await chooseAsOf(page, { day: priorDay })
        const thatDay = await snapshotWhen(
            page,
            tbSnapshot,
            isWhole,
            `the seeded accounts as of ${priorDay}`
        )
        expect(figures(groupRow(thatDay, 'Consulting Income'))).toEqual({
            debit: '',
            credit: '3,000.00',
        })
        expect(groupLabels(thatDay)).not.toContain('Retained earnings')

        // As of a day in that year, the line is one of the as-of month's lines of its own
        // account, and the opening row names that month.
        await openAccountRow(page, 'Consulting Income')
        const priorLabel = new RegExp(`^${priorDay} · JE-\\d+ · Prior-year consulting$`)
        const openedThatDay = await snapshotWhen(
            page,
            tbSnapshot,
            s => detailsUnder(s, 'Consulting Income').some(row => priorLabel.test(row.label)),
            'the prior-year line under Consulting Income as of its day'
        )
        expect(openingUnder(openedThatDay, 'Consulting Income')?.label).toBe(
            balanceBeforeLabel(priorDay)
        )
        const priorLine = detailsUnder(openedThatDay, 'Consulting Income').find(row =>
            priorLabel.test(row.label)
        )
        expect(figures(priorLine)).toEqual({ debit: '', credit: '3,000.00' })
    })
```

The lines of the test above line 554 stay as they are (they post 1.00 to Consulting Income dated today, set `priorDay`, filter to the three seeded accounts and check their order). Notes for the reader of this block:

- The test's own post is dated today, so Consulting Income has a line in the as-of month for any fiscal month.
- `expect(detailsUnder(opened, 'Retained earnings')).toEqual([])` holds because no spec and no seed row posts to account 3900 (`command grep -rln "Retained earnings\|3900" /Users/nas/tmp/tinycld/books-purchases/books/tests/e2e` prints three report specs, and each only reads the account). When this assertion fails, a line was posted to Retained earnings in the month of today: find the spec that posts it; do not weaken the assertion.
- The opening row and the account row carry the same Debit and Credit formulas (`debitSide`, `creditSide` of task 9c-1a), so with no line in the month their texts are equal.
- The old wait for the prior-year line under Retained earnings is gone on purpose (the human's decision). The line's label and amount are still checked, under Consulting Income as of its own day.

- [ ] **Step 4: The help paragraph**

In `/Users/nas/tmp/tinycld/books-purchases/books/help/reports.md` replace line 35, which is

```markdown
To see the lines of an account, press its row. Its posted lines open below it, in date order, then by entry number. Each line shows its date, its entry number and its memo, and its amount in Debit or Credit by its sign. The No. column stays blank on the lines. Press a line to open the transaction that posted it. To open every account, press Expand all. When an account is open, the same button reads Collapse all.
```

with

```markdown
To see the lines of an account, press its row. The first row below it reads "Balance before" and the month of the As of day, for example "Balance before Nov 2026". It shows the balance of the account at the end of the month before. For the retained earnings account, it also holds the income and expense of earlier fiscal years. Below it are the posted lines of that month, up to the As of day, in date order, then by entry number. Each line shows its date, its entry number and its memo, and its amount in Debit or Credit by its sign. The No. column stays blank on the lines. Press a line to open the transaction that posted it. Lines of earlier months do not show here. To see every line of an account, press its Debit or Credit figure. To open every account, press Expand all. When an account is open, the same button reads Collapse all.
```

Change no other line of the file. The next paragraph (line 37) already tells how a press on the figure opens the transactions.

- [ ] **Step 5: Typecheck and biome (Books)**

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && PATH=/Users/nas/tmp/tinycld/books-purchases/node_modules/.bin:$PATH tinycld-pkg typecheck
```

Expected: exit 0, no `error TS` line.

Run:

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && /Users/nas/tmp/tinycld/books-purchases/node_modules/.bin/biome check tests/e2e/trial-balance.spec.ts tests/e2e/books-report-helpers.ts
```

Expected: `Checked 2 files`, no error. When biome reports a format or import-order error, fix the file with the Edit tool to what biome prints; do not run `biome --write` on other files.

- [ ] **Step 6: Commit (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add tests/e2e/books-report-helpers.ts tests/e2e/trial-balance.spec.ts help/reports.md && git commit -m "test(reports): an opened Trial Balance account shows its balance before the month"
```

- [ ] **Step 7: Hand the e2e run to the driver**

Do not start it. Hand back:

```
NEEDS-RUN: cd /Users/nas/tmp/tinycld/books-purchases && lsof -iTCP -sTCP:LISTEN -nP | grep -qE ':(7210|7211) ' && { echo PORTS-BUSY; exit 1; }; E2E_PORT=7210 E2E_PORT_2=7211 E2E_IMAP_PORT=1210 E2E_IMAP_PORT_2=1211 pnpm exec tinycld-pkg test:e2e books/tests/e2e/trial-balance.spec.ts books/tests/e2e/report-drill-down.spec.ts books/tests/e2e/report-exports.spec.ts books/tests/e2e/report-manual-refresh.spec.ts
Needed from it: the `Running N tests` line (N is the tests of these four files, not the whole suite), the line of dots, and each failure in full.
```

The four files are every Books spec that opens the Trial Balance (`command grep -rlE "openTrialBalance|Trial Balance" /Users/nas/tmp/tinycld/books-purchases/books/tests/e2e`). The run needs the commits of task 9c-1a and of step 6 in the tree.

Expected GREEN: no `F` in the line of dots, and no failed test. In `trial-balance.spec.ts` the two changed tests pass: "opening an account shows its posted lines, each labelled with its date, number and memo" and "earlier fiscal years fold onto Retained earnings, and a day in that year shows them on their own".

When a test of this run fails, the driver gives the failure to a fix round. The fix finds the cause at its source; it bumps no timeout and removes no assertion.

- [ ] **Step 8: Hand the 100k run to the driver**

Do not start it. Hand back, with the result of step 7:

```
NEEDS-RUN: (quiet machine: `pgrep -fl 'chrome-headless-[s]hell'` prints nothing and the first number of `sysctl -n vm.loadavg` is under 4; not while the e2e of step 7 runs) cd /Users/nas/tmp/tinycld/books-purchases/books/server && BOOKS_SCALE=1 BOOKS_SCALE_OUT=/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-report-engine-scale/scale-task9c1 go test -run 'TestStatementsScale$/100k$' -count=1 -timeout 60m -v .
Needed from it: the files scale-task9c1/st-results.md and scale-task9c1/st-plans.md, and the PASS or FAIL line of the test.
```

No bar is held at 100,000 lines, so the test passes when each run with a bar shows a group row; a run core refuses is recorded as refused.

- [ ] **Step 9: Record the result in the ledger (Books)**

With the files of step 8, append to the end of `/Users/nas/tmp/tinycld/books-purchases/books/docs/plans/general-ledger-ledger.md` (Read the end of the file first; append with the Edit tool). Write the heading and the first two paragraphs as they are here:

```markdown
#### Task 9c-1: the Trial Balance reads the stored months (books `server/report_trial_balance.go`, 2026-11-20)

Before: every figure of the Trial Balance was a sum over the posted lines of the account up to the As of day, so each run read the account's whole history (`SCAN books_journal_lines` in each statement, also with an Accounts filter). After: the report's rows are the posted lines of the month of the As of day; Debit and Credit add a lookup over the stored month balances (`books_account_balances`) of the months before it; the Total adds the account rows shown; an account with stored months and no line in the month gets a row through the fill. The figures do not change: `TestTrialBalanceStoredMonthsEqualTheLines` runs the line read beside the new read.

What the reader sees (human decision, 2026-11-20): an opened account shows "Balance before" and the month of the As of day, then the posted lines of that month up to the day. Lines of earlier months do not show there; a press on the Debit or Credit figure still opens every line. The opened Retained earnings row no longer lists the lines of earlier fiscal years: its opening row holds them.
```

Then add, under them:

1. The line `At 100,000 lines (median of 5, core <core hash>, books <books hash>):` with the short hashes of the two trees the run of step 8 used: `git -C /Users/nas/tmp/tinycld/books-purchases/tinycld rev-parse --short HEAD` and the Books commit of step 6 (`git -C /Users/nas/tmp/tinycld/books-purchases/books rev-parse --short HEAD`).
2. A table with the header `| run | after task 9b | after task 9c-1 | group rows | rows |` and one row for each of the eleven runs, in the order of `st-results.md`. The column `after task 9b` is this text, one for each run in order: `738ms`, `432ms`, `516ms`, `1.373s`, `1.746s`, `153ms`, `1.554s`, `29ms`, `18ms`, `125ms`, `31ms` (TB 1, TB 2, TB 3, BS 1, BS 2, TD 1 to TD 6). The three other columns are the columns `median of 5`, `group rows` and `rows` of `scale-task9c1/st-results.md`, copied as they are; a refused run is written `refused after <time>`.
3. The heading line `Reads of the line tables and of the stored months in the longest statement of TB 1 all accounts, as of, and of TB 2 busy account, as of (scale-task9c1/st-plans.md):` and under it, for each of the two runs, the statement's heading line of `st-plans.md` as plain text (for example `TB 1 all accounts, as of (statement 5: 488.1ms, 106 rows, 10129 chars):`) and, in one fenced block, every plan line of that statement that `st-plans.md` prints (the measure prints the lines that name `books_journal_lines`, `books_cash_basis_lines` or `books_account_balances`). The longest statement of a run is the one with the largest time in its heading. Write what the file shows.
4. One sentence that says whether a `SCAN books_journal_lines` line is still in a Trial Balance statement, and in which run.
5. One sentence that names every run that is refused at 100k, or `No run is refused at 100k.`

Change no code in this step, whatever the run shows. A Trial Balance run that is slower than its `after task 9b` figure, a `SCAN books_journal_lines` that is still there, or a row count of TB 1, TB 2 or TB 3 that differs from 107, 2 and 4 goes in the record as measured and in the hand-back of the task, with the path of `st-plans.md`. The next plan part chooses the next change from it.

- [ ] **Step 10: Commit the record (Books)**

```bash
cd /Users/nas/tmp/tinycld/books-purchases/books && git add docs/plans/general-ledger-ledger.md && git commit -m "docs: the Trial Balance at 100k after it reads the stored months"
```

---

## After this part

Task 9c-1 is complete with this part. The later tasks of the unit (the outline, `plan-outline.md`) are planned from the record of step 9: task 9c-2 (the Balance Sheet on the stored months) and the 500k measure against the bars. They are not in this part.

For the driver: the help file changed, so run `pnpm run packages:generate` in `/Users/nas/tmp/tinycld/books-purchases/tinycld` once before the e2e of step 7, when no other run is in progress.
