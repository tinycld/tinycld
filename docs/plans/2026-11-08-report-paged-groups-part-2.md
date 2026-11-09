# Report paged group rows and fold list (J1, J2) — Part 2: e2e, client, docs, one-line width

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Core's report client sends the fold list, counts a page as the engine counts it, shows a failed next page as a row with Retry, its e2e, docs and help follow J1, and the one-line layout shares spare width by `flexGrow` (J2).

**Architecture:** See the outline `docs/plans/2026-11-08-report-paged-groups.md` (Architecture, Rulings R1–R10). Part 1 (Tasks 1–6) made the engine page group rows and take `options.collapse`. This part: Task 7 writes the J1 e2e red; Task 8 keeps folds under open levels in `Expansion.collapsed` and counts group, opening, detail and subtotal rows as a page's place (`pageRowCount`); Task 9 throws a later page's refusal so TanStack keeps the loaded pages, and the table ends in an error row with Retry; Task 10 moves the existing paging e2e and the docs to the J1 counts; Task 11 adds `shareLineWidth` (spare room by grow) for one line.

**Tech Stack:** TypeScript (React Native + web, TanStack Query 5.104), vitest, Playwright.

**Spec:** `/Users/nas/tmp/tinycld/books-purchases/books/docs/specs/journal-report-design.md` §4, with the brief `/Users/nas/tmp/tinycld/auto-helix-books/staging/p5-journal-report/cp/cp0.md` (Checkpoint 0, including J2 added by the human 2026-11-08). Verdict MAJOR; lane purchases does NOT take `exclusive`; the core merge needs the human's approval.

**Verdict: MAJOR.** Lane purchases does NOT take `exclusive`; the core merge needs the human's approval (brief D4).

Every path is relative to `/Users/nas/tmp/tinycld/books-purchases/tinycld`. Do not commit (the driver commits); the "Commit" step of each task is the driver's.

Commands (from `/Users/nas/tmp/tinycld/books-purchases/tinycld`):
- vitest: `pnpm exec vitest run <test file>`
- biome on the files you changed: `pnpm exec biome check <files>`; typecheck: `pnpm exec tinycld-pkg typecheck`
- e2e, one file at a time: first `lsof -iTCP -sTCP:LISTEN -nP | grep -E ':(7210|7211)'` (kill a leftover server only if it was started from `/Users/nas/tmp/tinycld/books-purchases`), then `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- <file>.spec.ts` (add `-g "<title part>"` to run some tests of the file). A run longer than about 4 minutes is the driver's: hand back `NEEDS-RUN: <command>`.

## Global Constraints

- "A page holds up to `options.limit` rows (default 200, at most 1,000) counting group, opening, detail and subtotal rows, in document order; a closed group counts as its one row." (J1a)
- "`options.collapse: [groupKey]` (at most 5,000): groups closed by the reader under a level that is open by default or under `expandAll`. … `expand` keeps its meaning for levels that are closed by default." (J1b)
- "`lib/view-url.ts` no change (the fold state is not in the link)." (J1b)
- J2 (brief): "core's one-line layout shares spare width by each column's `flexGrow`, not by shortfall from natural width"; existing tests and width-pinning e2e change "only where the new rule changes them, each with the reason".
- A failed next page "shows a row 'Could not load more rows' with Retry (test ids `report-loading-more-error`, `report-loading-more-retry`) instead of stopping silently". (brief)
- Core names no package: e2e use the `salesstub` package. No raw PocketBase writes in e2e; `page.route` refuses with `route.fulfill({ status: 400 })`, never `route.abort()`. Phone e2e: navigate at the default viewport, then `page.setViewportSize`. Read layout with `expect.poll`.
- Biome, no `any`, no biome-ignore, no `console.*`, no `useEffect` + `useState` sync; comments say why. Write help in Simplified Technical English.
- Rulings R2 (fold semantics), R3 (what a page counts), R4 (`page.size`), R8 (next-page failure) and R10 (J2 scope) of the outline bind every task.

## Review Focus

1. A failed next page must not loop: the list calls `onEndReached` again on redraws, and the same request fails again (Task 9 test "asks again only on Retry"); and a live rerun after it must not turn the report into the whole-report error (Task 9 test "stays ready through a live rerun").
2. Reopening the only folded group under the open levels must give back the first run's body, so the cached first run serves it (Task 8 test "unfolding … sends no expand").
3. A fold under Expand all keeps Expand all and sends only the fold (Task 8 test "under Expand all").
4. A fold re-run over closed groups must load as many rows as the reader had, group rows counted, or the rows below the view vanish (Task 8 `useReportRun` test).
5. A column with Extra room **None** next to a growing column: it gets its content width when there is room and never more (Task 11 test "no Extra room").

## Interfaces from Part 1 (consumed here)

- `reports.Options.Collapse [][]any` — JSON `collapse`, at most 5,000 keys; refused at `options.collapse`. Base state per R2: `expandAll` → every group; `expand` absent **or `collapse` present** → the open levels; else only `expand`'s groups. `collapse` closes listed groups; `expand` opens groups of closed levels.
- `page.next` on every paged grouped result that has more rows. A later page starts after the cursor's group row and opening row: it does not repeat earlier group rows. The Totals after the page's slice repeat on every page.
- No `rowCap` notice for a grouped report without pivot, layout or nest.
- `Page.Size` = detail rows on the page (R4). The client never reads it.

---

### Task 7: E2E spec for paged group rows (red first)

**Files:**
- Modify: `tests/e2e/reports-helpers.ts` (add the moved helpers after `scrollRowTo`, line 182)
- Modify: `tests/e2e/reports-ledger-v3.spec.ts` (delete lines 38, 43–48, 70–72, 85–98, 103–113, 115–182; update imports and the callers of `openPagedTeams` at lines 191 and 305)
- Create: `tests/e2e/reports-paged-groups.spec.ts`

**Interfaces:**
- Consumes: `loginAsShippedOwner`, `openReportStub`, `addTeam`, `addSales`, `uniqueName`, `listScroller`, `scrollRowTo` (reports-helpers.ts); `chooseParamOption`, `hubRow`, `openReportsHub`, `rowOf` (report-view-helpers.ts); `skipWithoutReportsStub` (helpers.ts).
- Produces (exported from `tests/e2e/reports-helpers.ts`): `TEAM_BALANCE_TITLE`, `openTeamBalance(page: Page): Promise<void>`, `showTeams(page: Page, search: string, teams: string[]): Promise<void>`, `rowIds(page: Page): Promise<string[]>`, `detailLabels(page: Page): Locator`, `type SentRun = { cursor?: string; limit?: number }`, `USUAL_PAGE = 200`, `MAX_PAGE = 1000`, `interface PagedTeamsOptions { smallTeams?: number; nextPages?: Promise<void>; baseAmount?: number; pageRows?: number; refuseFirstNextPage?: boolean; height?: number }`, `openPagedTeams(page: Page, prefix: string, opts?: PagedTeamsOptions): Promise<{ stem: string; teams: string[]; cursors: string[]; sent: SentRun[]; firstSale: Locator }>` (`cursors`: the cursors of the run filtered to the teams only; `sent`: every run request, as before). Test ids used: `report-loading-more`, `report-loading-more-error`, `report-loading-more-retry`, `report-end`.

- [ ] **Step 1: Move the team-balance helpers to `reports-helpers.ts`**

In `tests/e2e/reports-helpers.ts`, add to the imports: `chooseParamOption`, `hubRow`, `openReportsHub`, `rowOf` from `./report-view-helpers` (a separate `import { … } from './report-view-helpers'` line; the existing re-export block stays). Then append after `scrollRowTo`:

```ts
// The stub's shipped "Stub team balance" (salesstub.team_balance): sales grouped by
// team, each team open by default, with an opening row ("Before the period"), a
// running balance and a subtotal row ("Total <team>"). Shipped reports belong to
// the seed's admin, so these helpers run after loginAsShippedOwner.
export const TEAM_BALANCE_TITLE = 'Stub team balance'
const YEAR = new Date().getFullYear()

export async function openTeamBalance(page: Page) {
    await openReportsHub(page)
    await page.getByTestId('reports-hub-tab-mine').click()
    await hubRow(page, TEAM_BALANCE_TITLE).click()
    await expect(page.getByTestId('report-title')).toHaveText(TEAM_BALANCE_TITLE)
}

export async function showTeams(page: Page, search: string, teams: string[]) {
    await chooseParamOption(page, 'teams', { from: 'add-filter', search, labels: teams })
}

/**
 * The rows' ids in screen order, read in one snapshot. A row drawn twice is listed
 * twice, so a repeat can be counted (a sticky copy has its own test id and is not
 * listed).
 */
export function rowIds(page: Page) {
    return page
        .getByTestId('report-table')
        .evaluate(table =>
            [...table.querySelectorAll('[data-testid^="report-row-"]')].map(
                el => el.getAttribute('data-testid')?.replace('report-row-', '') ?? ''
            )
        )
}

export function detailLabels(page: Page) {
    return page.getByTestId('report-table').locator('[data-testid^="report-row-d:"]')
}

/** A run request as the client sent it, before openPagedTeams' route shrinks its page. */
export type SentRun = { cursor?: string; limit?: number }

// The client's page sizes: its usual page (RUN_PAGE_SIZE) and the server's most
// (RUN_MAX_PAGE), which a fold run asks for while it looks for the group in view.
export const USUAL_PAGE = 200
export const MAX_PAGE = 1000

export interface PagedTeamsOptions {
    /** Teams of one sale each, after the two teams of six. */
    smallTeams?: number
    /** A next page (a request with a cursor) waits for it. */
    nextPages?: Promise<void>
    /** Team n's sales start at baseAmount × n cents and go up a cent each. */
    baseAmount?: number
    /** The rows a page holds once the route shrinks the client's sizes (default 5). */
    pageRows?: number
    /** Refuses (400) the first next page of the run filtered to the teams, once. */
    refuseFirstNextPage?: boolean
    /**
     * The window height the report opens at (default 1600, which draws every row of
     * two teams). A short window loads no next page until the reader scrolls.
     */
    height?: number
}

// What the server answers for a cursor it cannot read (run.go `invalid`).
const CURSOR_REFUSAL = { status: 400, message: 'The page cursor is not valid.', data: {} }

/**
 * Signs in, seeds two teams of six sales each (team A first), and `smallTeams` more
 * teams of one sale each after them, and opens the report on them alone, in pages
 * of `pageRows` rows. A page counts group, opening, detail and subtotal rows (J1a),
 * so team A alone is 9 rows (group, opening, six sales, subtotal) and a small team 4.
 */
export async function openPagedTeams(page: Page, prefix: string, opts: PagedTeamsOptions = {}) {
    const { smallTeams = 0, nextPages, baseAmount = 100, pageRows = 5, height = 1600 } = opts
    let isRefusing = opts.refuseFirstNextPage === true
    await loginAsShippedOwner(page)
    const stem = uniqueName(prefix)
    const letters = 'ABCDEFGH'.slice(0, 2 + smallTeams)
    const teams = [...letters].map(letter => `${stem} ${letter}`)
    await openReportStub(page)
    for (const [i, team] of teams.entries()) {
        await addTeam(page, team)
        await addSales(page, {
            team,
            customerPrefix: `${stem}${letters[i]}`,
            count: i < 2 ? 6 : 1,
            firstDay: `${YEAR}-02-01`,
            firstAmount: baseAmount * (i + 1),
            cost: 0,
            quantity: 1,
        })
    }
    const cursors: string[] = []
    const sent: SentRun[] = []
    await page.route('**/api/reports/run', async route => {
        const body = route.request().postDataJSON()
        const cursor: string | undefined = body.options?.cursor
        // Only the run filtered to these teams: the first, unfiltered run pages too.
        const isTeamsRun = body.params?.teams !== undefined
        if (cursor && isRefusing && isTeamsRun) {
            isRefusing = false
            await route.fulfill({
                status: 400,
                contentType: 'application/json',
                body: JSON.stringify(CURSOR_REFUSAL),
            })
            return
        }
        if (cursor && nextPages) await nextPages
        if (cursor && isTeamsRun) cursors.push(cursor)
        sent.push({ cursor, limit: body.options?.limit })
        // The client's page sizes become pageRows. A fold run asks for max(200, the
        // rows in view), which is 200 here, and then the server's most while it looks
        // for the group in view, so it also gets pages of pageRows and joins them
        // until it holds the rows that were in view.
        const asked = body.options?.limit
        const limit = asked === USUAL_PAGE || asked === MAX_PAGE ? pageRows : asked
        await route.continue({
            postData: JSON.stringify({ ...body, options: { ...body.options, limit } }),
        })
    })
    // By default tall enough that the list draws every row of both teams at once, so
    // one snapshot holds the whole merged list (the list draws only rows near the view).
    await page.setViewportSize({ width: 1280, height })
    await openTeamBalance(page)
    // The first run has no Teams filter, so it shows these sales too: no row
    // tells the two runs apart. Once the run with the Teams filter has answered
    // and the view no longer says "Updating…", every row is the new run's.
    await expect(detailLabels(page).first()).toBeVisible()
    const teamsRun = page.waitForResponse(
        response =>
            response.url().includes('/api/reports/run') &&
            response.request().postDataJSON()?.params?.teams !== undefined
    )
    await showTeams(page, stem, teams)
    await teamsRun
    await expect(page.getByTestId('report-updating')).toHaveCount(0)
    const firstSale = rowOf(page, `${stem}A 00`)
    await expect(firstSale).toBeVisible()
    return { stem, teams, cursors, sent, firstSale }
}
```

In `tests/e2e/reports-ledger-v3.spec.ts`: delete `TEAM_BALANCE_TITLE`, `openTeamBalance`, `showTeams`, `rowIds`, `detailLabels`, `SentRun`, `USUAL_PAGE`, `MAX_PAGE` and `openPagedTeams` (with their comments); import `detailLabels`, `MAX_PAGE`, `openPagedTeams`, `openTeamBalance`, `rowIds`, `showTeams` (`detailLabels` is still called at lines 445, 455, 460, 475, 482, 503, 593) (and `SentRun` if `foldRequests` still names it) from `./reports-helpers`; drop the now-unused imports (`addSales`, `chooseParamOption`, `hubRow`, `openReportsHub` when nothing else in the file uses them — let biome say). Change the two callers:

```ts
// pagedTeams (line 191)
const { firstSale, ...paged } = await openPagedTeams(page, prefix, { smallTeams })
// 'the Total waits for the last page…' (line 305)
const { teams, firstSale } = await openPagedTeams(page, 'BalEnd', {
    nextPages,
    baseAmount: 600_000,
})
```

- [ ] **Step 2: Write the failing e2e spec**

`tests/e2e/reports-paged-groups.spec.ts`:

```ts
import { expect, type Locator, type Page, test } from '@playwright/test'
import { skipWithoutReportsStub } from './helpers'
import { listScroller, openPagedTeams, rowIds, rowOf, scrollRowTo } from './reports-helpers'

/**
 * J1 (core paged group rows and fold list) on the stub's "Stub team balance", a
 * report grouped by team whose teams open by default. openPagedTeams seeds two
 * teams of six sales and SMALL_TEAMS teams of one sale (teams A–H) and shrinks each
 * page to PAGE_ROWS rows. A page counts group, opening, detail and subtotal rows
 * (J1a): page 1 holds team A (group, opening, six sales, subtotal: 9 rows) and team
 * B's group, opening and first sale; team C first comes on page 2; team H on page 4.
 */

const PAGE_ROWS = 12
const SMALL_TEAMS = 6
// Page 1 (12 rows) is longer than the list at these heights, so no next page
// loads before the reader scrolls toward its end.
const DESKTOP = { width: 1280, height: 500 }
const PHONE = { width: 390, height: 844 }

const totals = (ids: readonly string[]) => ids.filter(id => id.startsWith('total'))

type DrawnRow = { id: string; text: string; isOpen: boolean }

/** The drawn rows (not their sticky copies) in screen order, in one snapshot. */
function drawnRows(page: Page): Promise<DrawnRow[]> {
    return page.getByTestId('report-table').evaluate(table =>
        [...table.querySelectorAll('[data-testid^="report-row-"]')].map(el => ({
            id: el.getAttribute('data-testid')?.replace('report-row-', '') ?? '',
            text: el.textContent ?? '',
            isOpen: el.getAttribute('aria-expanded') === 'true',
        }))
    )
}

/**
 * Scrolls the list from its top to its end, a screen at a time, until the Total row
 * is drawn, and returns every row it saw once, in the order first seen. The list
 * draws only the rows near its view, so no one snapshot holds a long list; the list
 * loads its next pages as the scroll reaches them.
 */
async function scrollThrough(page: Page, row: Locator): Promise<DrawnRow[]> {
    const scroller = await listScroller(row)
    await scroller.evaluate(el => {
        el.scrollTop = 0
    })
    const seen = new Map<string, DrawnRow>()
    await expect
        .poll(
            async () => {
                for (const drawn of await drawnRows(page)) seen.set(drawn.id, drawn)
                await scroller.evaluate(el => {
                    el.scrollTop += el.clientHeight * 0.8
                })
                return totals([...seen.keys()]).length
            },
            { intervals: [100] }
        )
        .toBe(1)
    return [...seen.values()]
}

/**
 * J1a on scroll: page 1 shows teams A and B; scrolling to its end shows "Loading
 * more rows…" while the next page is held, then team C (only page 2 has it), and the
 * report ends in its Total with every team once.
 */
async function scrollToNextGroups(page: Page, prefix: string, size: typeof DESKTOP) {
    let release = () => {}
    const nextPages = new Promise<void>(resolve => {
        release = resolve
    })
    const { teams, firstSale } = await openPagedTeams(page, prefix, {
        smallTeams: SMALL_TEAMS,
        pageRows: PAGE_ROWS,
        nextPages,
        height: size.height,
    })
    await page.setViewportSize(size)
    await expect(rowOf(page, teams[2] ?? '')).toHaveCount(0)
    const scroller = await listScroller(firstSale)
    await scroller.evaluate(el => {
        el.scrollTop = el.scrollHeight
    })
    await expect(page.getByTestId('report-loading-more')).toContainText('Loading more rows…')
    expect(totals(await rowIds(page))).toEqual([])

    release()
    const rows = await scrollThrough(page, firstSale)
    const groups = rows.filter(row => row.id.startsWith('g:'))
    expect(groups).toHaveLength(teams.length)
    for (const team of teams) {
        expect(groups.filter(g => g.text.includes(team)), `the group row of ${team}`).toHaveLength(1)
    }
    expect(rows[rows.length - 1]?.id).toMatch(/^total/)
    await expect(page.getByTestId('report-end')).toHaveText('End of report')
}

/** The row's top edge, in px under its list's top edge. */
async function offsetInList(row: Locator) {
    const scroller = await listScroller(row)
    return row.evaluate(
        (el, list) => el.getBoundingClientRect().top - list.getBoundingClientRect().top,
        scroller
    )
}

/**
 * The first drawn row (not a sticky copy, which has its own test id) at the top of
 * the view: the first whose bottom edge is under the list's top edge. Returns its id
 * and its top edge in px under the list's top edge (negative when the view cuts it).
 */
async function topRowInView(page: Page, row: Locator) {
    const scroller = await listScroller(row)
    const top = await page.getByTestId('report-table').evaluate((table, list) => {
        const listTop = list.getBoundingClientRect().top
        for (const el of table.querySelectorAll('[data-testid^="report-row-"]')) {
            const box = el.getBoundingClientRect()
            if (box.bottom > listTop) {
                return {
                    id: el.getAttribute('data-testid')?.replace('report-row-', '') ?? '',
                    offset: box.top - listTop,
                }
            }
        }
        return null
    }, scroller)
    if (!top) throw new Error('no row is drawn at the top of the view')
    return top
}

/** The top edge of the row with this id, in px under its list's top edge. */
function offsetOfRow(page: Page, id: string) {
    return offsetInList(page.getByTestId(`report-row-${id}`))
}

/**
 * Makes the list `rows` rows of `row`'s height tall, keeping the page's width, so
 * the list loads its next page only near the end of what it holds. Returns the row
 * height.
 */
async function shortenList(page: Page, row: Locator, rows: number) {
    const scroller = await listScroller(row)
    const listHeight = await scroller.evaluate(el => el.clientHeight)
    const rowHeight = await row.evaluate(el => el.getBoundingClientRect().height)
    const viewport = page.viewportSize()
    if (!viewport) throw new Error('the page has no viewport size')
    await page.setViewportSize({
        width: viewport.width,
        height: Math.round(viewport.height - listHeight + rows * rowHeight),
    })
    await expect.poll(() => scroller.evaluate(el => el.clientHeight)).toBeLessThan(listHeight)
    return rowHeight
}

/**
 * J1b: folds team B, near the end of page 1, before the later teams have loaded.
 * The fold keeps the row at the top of the view (the first drawn row that is not a
 * sticky copy) and B's group row where they were, and every other team, also the
 * ones that load after the fold, stays open.
 */
async function foldNearFirstPageEnd(page: Page, prefix: string, size: typeof DESKTOP) {
    const { stem, teams, cursors } = await openPagedTeams(page, prefix, {
        smallTeams: SMALL_TEAMS,
        pageRows: PAGE_ROWS,
        height: size.height,
    })
    await page.setViewportSize(size)
    const groupB = rowOf(page, teams[1] ?? '')
    await expect(groupB).toHaveAttribute('aria-expanded', 'true')
    const rowHeight = await shortenList(page, groupB, 5)
    // Two rows under the list's top: the top row's place is taken by the sticky
    // copy of an open group row (and, on a phone, by the floating heading), which
    // would take the press.
    await scrollRowTo(groupB, 2 * rowHeight)
    await expect(groupB).toBeInViewport()
    const before = await offsetInList(groupB)
    const topBefore = await topRowInView(page, groupB)
    // The cursor requests of the teams run so far are the list's own next pages:
    // at most pages 2 and 3. Team H is on page 4, so at least one team loads only
    // after the fold, which is what the test is about.
    expect(cursors.length, 'next pages loaded before the fold').toBeLessThan(3)

    await groupB.click()
    await expect(groupB).toHaveAttribute('aria-expanded', 'false')
    await expect(rowOf(page, `${stem}B 00`)).toHaveCount(0)
    await expect
        .poll(async () => Math.abs((await offsetInList(groupB)) - before))
        .toBeLessThanOrEqual(rowHeight)
    // The row at the top of the view stays the same row, at the same place (the
    // brief's check). The rows above B do not change in a fold of B, so it moves
    // at most as B may.
    const topAfter = await topRowInView(page, groupB)
    expect(topAfter.id, 'the row at the top of the view').toBe(topBefore.id)
    expect(Math.abs((await offsetOfRow(page, topBefore.id)) - topBefore.offset)).toBeLessThanOrEqual(rowHeight)

    const rows = await scrollThrough(page, groupB)
    const groups = rows.filter(row => row.id.startsWith('g:'))
    for (const team of teams) {
        const group = groups.find(g => g.text.includes(team))
        expect(group, `the group row of ${team}`).toBeDefined()
        expect(group?.isOpen, `${team} is open`).toBe(team !== teams[1])
    }
    expect(rows[rows.length - 1]?.id).toMatch(/^total/)
}

test.describe('reports: paged groups', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a grouped report with more groups than one page loads the next groups on scroll and ends in the Total', async ({
        page,
    }) => {
        await scrollToNextGroups(page, 'PgScroll', DESKTOP)
    })

    // The phone draws core's wrap lines (other code than the desktop line).
    test('on a phone a grouped report loads the next groups on scroll and ends in the Total', async ({
        page,
    }) => {
        await scrollToNextGroups(page, 'PgPhScroll', PHONE)
    })

    test('a fold near the end of the first page keeps every other group open after the next page loads', async ({
        page,
    }) => {
        await foldNearFirstPageEnd(page, 'PgFold', DESKTOP)
    })

    test('on a phone a fold near the end of the first page keeps every other group open', async ({
        page,
    }) => {
        await foldNearFirstPageEnd(page, 'PgPhFold', PHONE)
    })

    test('a next page that fails shows a row with Retry, and Retry loads the next groups and then the Total', async ({
        page,
    }) => {
        // The default 1600 px window: page 1 is shorter than the list, so the list
        // asks for page 2 at once, and that request is refused.
        const { teams, firstSale } = await openPagedTeams(page, 'PgRetry', {
            smallTeams: SMALL_TEAMS,
            pageRows: PAGE_ROWS,
            refuseFirstNextPage: true,
        })
        const failed = page.getByTestId('report-loading-more-error')
        await expect(failed).toContainText('Could not load more rows')
        await expect(page.getByTestId('report-loading-more')).toHaveCount(0)
        // The rows already shown stay; the Total waits for the last page.
        await expect(firstSale).toBeVisible()
        await expect(rowOf(page, teams[2] ?? '')).toHaveCount(0)
        expect(totals(await rowIds(page))).toEqual([])

        await page.getByTestId('report-loading-more-retry').click()
        const rows = await scrollThrough(page, firstSale)
        expect(rows.some(row => row.text.includes(teams[2] ?? '?'))).toBe(true)
        expect(rows[rows.length - 1]?.id).toMatch(/^total/)
        await expect(failed).toHaveCount(0)
        await expect(page.getByTestId('report-end')).toHaveText('End of report')
    })
})
```

- [ ] **Step 3: Check the moved helpers compile and lint**

Run: `pnpm exec biome check tests/e2e/reports-helpers.ts tests/e2e/reports-ledger-v3.spec.ts tests/e2e/reports-paged-groups.spec.ts && pnpm exec tinycld-pkg typecheck`
Expected: no errors.

- [ ] **Step 4: Run the spec to see it fail for the right reasons**

Run (ports checked first): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-paged-groups.spec.ts`
Expected:
- The two "loads the next groups on scroll" tests (desktop, phone) PASS (Part 1's engine already pages group rows and the client merges pages by row id). If one fails, find the root cause: an engine defect of Part 1 is reported with the failure, a test defect is fixed in the test.
- The two fold tests FAIL at `<team> is open` (expected `true`, received `false`) for the first team that loaded after the fold (C if page 2 had not loaded, else E or later): the client turns a fold under the open levels into `expand` with the groups open at the fold (`toggleGroup`, `rows.ts:109`), so teams that load later come closed.
- The Retry test FAILS waiting for `report-loading-more-error`: a refused later page is a `RunPage` of kind `invalid`, and `runView` shows `report-invalid` in place of the table (R8).

Record the run's result in the task report. A test that fails for another reason is a test defect: fix the test, not the app.

- [ ] **Step 5: Commit** (driver) — `reports: e2e for paged group rows and the fold list (red)`

---

### Task 8: Client fold list and page count

**Files:**
- Modify: `core/reports/lib/rows.ts` (`Expansion` and its constants lines 75–84; `openGroupKeys` 88–90 deleted; `toggleGroup` 102–121; `expansionOptions` 133–141)
- Modify: `core/reports/lib/run-client.ts` (`RunOptions` 21–30; `foldKeyOf` 63–72; `detailRowCount` 74–77 → `pageRowCount`; `RUN_PAGE_SIZE`/`RUN_MAX_PAGE` docs 134–137; `runFirstPage` 177–208)
- Modify: `core/reports/hooks/useReportRun.ts` (`ReaderPlace` 55–67; `firstPagePlace` 94–116; `queryFn` 130–134)
- Test: `core/reports/lib/__tests__/rows.test.ts`, `core/reports/lib/__tests__/run-client.test.ts`, `core/reports/hooks/__tests__/useReportRun.test.tsx`

**Interfaces:**
- Consumes: Part 1 `options.collapse` and R2; `mergePages`, `holdWholeReportRows` (rows.ts).
- Produces: `type Expansion = { all: boolean; keys: readonly unknown[][]; collapsed: readonly unknown[][]; isDefault?: boolean }`; `DEFAULT_EXPANSION`, `NO_EXPANSION`, `EXPAND_ALL` with `collapsed: []`; `toggleGroup(expansion: Expansion, row: ResultRow, rows: readonly ResultRow[]): Expansion`; `expansionOptions(expansion: Expansion): { expand?: unknown[][]; expandAll?: boolean; collapse?: unknown[][] }`; `RunOptions.collapse?: unknown[][]`; `pageRowCount(result: RunResult): number` (replaces `detailRowCount`); `runFirstPage(body, target, signal?, untilRow?)` with `target` in counted rows; `ReaderPlace = { rows: number; groupRow?: string }` (useReportRun.ts, not exported).

The place count must count exactly what `options.limit` counts (R3): group, opening, detail and subtotal rows; Total and computed rows never. `page.size` (R4: detail rows) is never read by the client. For a report the engine does not page by groups (pivot, layout, nest), the server counts detail rows only; the client's count is then larger than the server's, which is harmless: `runFirstPage` compares the target and the loaded rows with the same count, so it reloads the same rows, and asks the server for no fewer rows than it needs.

- [ ] **Step 1: Write the failing tests**

In `core/reports/lib/__tests__/rows.test.ts`, replace the `expansion state` describe (lines 170–191) with:

```ts
describe('expansion state', () => {
    const a = group('g:a', ['a'], { expanded: true })
    const b = group('g:b', ['b'], { expanded: true })
    const rows = [a, b]

    it('sends no expand on the first run, so groups marked open open', () => {
        expect(expansionOptions(DEFAULT_EXPANSION)).toEqual({})
    })

    it('collapse sends an empty expand, so groups marked open stay closed', () => {
        expect(foldAllTarget([a])).toEqual(NO_EXPANSION)
        expect(expansionOptions(NO_EXPANSION)).toEqual({ expand: [] })
    })

    // J1b: the server gets the fold, not the list of every other open group, so a
    // fold works in a report with more open groups than expand may list (5,000).
    it('folding a group under an open level adds its key to collapsed and sends only the fold', () => {
        const folded = toggleGroup(DEFAULT_EXPANSION, a, rows)
        expect(folded).toEqual({ all: false, keys: [], collapsed: [['a']], isDefault: true })
        expect(expansionOptions(folded)).toEqual({ collapse: [['a']] })
    })

    it('folding a group under Expand all keeps Expand all and sends the fold', () => {
        const folded = toggleGroup(EXPAND_ALL, a, rows)
        expect(folded).toEqual({ all: true, keys: [], collapsed: [['a']] })
        expect(expansionOptions(folded)).toEqual({ expandAll: true, collapse: [['a']] })
    })

    // Back to the first run's body, so the cached first run serves it.
    it('unfolding a folded group under an open level removes its key and sends no expand for it', () => {
        const folded = toggleGroup(DEFAULT_EXPANSION, a, rows)
        const closedA = { ...a, expanded: false }
        const unfolded = toggleGroup(folded, closedA, [closedA, b])
        expect(unfolded).toEqual(DEFAULT_EXPANSION)
        expect(expansionOptions(unfolded)).toEqual({})
        const underAll = toggleGroup(toggleGroup(EXPAND_ALL, a, rows), closedA, [closedA, b])
        expect(expansionOptions(underAll)).toEqual({ expandAll: true })
    })

    // R2: with collapse sent, the open levels stay open and expand only adds groups
    // of closed levels.
    it('opening a group of a closed level under the open levels sends it in expand, beside the folds', () => {
        const folded = toggleGroup(DEFAULT_EXPANSION, a, rows)
        const closedLevel = group('g:a.x', ['a', 'x'], { section: 'g:a' })
        const opened = toggleGroup(folded, closedLevel, [a, closedLevel, b])
        expect(expansionOptions(opened)).toEqual({ expand: [['a', 'x']], collapse: [['a']] })
        // Nothing folded: collapse is still sent (empty), or expand alone would open
        // only its own groups.
        const openedOnly = toggleGroup(DEFAULT_EXPANSION, closedLevel, [a, closedLevel, b])
        expect(expansionOptions(openedOnly)).toEqual({ expand: [['a', 'x']], collapse: [] })
    })

    it('closing a group drops the folds and opened groups under it', () => {
        const opened = group('g:a.x', ['a', 'x'], { section: 'g:a', expanded: true })
        const folded = group('g:a.y', ['a', 'y'], { section: 'g:a' })
        const start = { ...DEFAULT_EXPANSION, keys: [['a', 'x']], collapsed: [['a', 'y']] }
        const closed = toggleGroup(start, a, [a, opened, folded, b])
        expect(closed).toEqual({ all: false, keys: [], collapsed: [['a']], isDefault: true })
    })

    it('Expand all and Collapse all clear collapsed', () => {
        expect(foldAllTarget([group('g:a', ['a'])]).collapsed).toEqual([])
        expect(foldAllTarget([a]).collapsed).toEqual([])
        expect(expansionOptions(EXPAND_ALL)).toEqual({ expandAll: true })
    })
})
```

The old case `a press under the default state keeps the groups the result shows open` goes: J1b (R2) replaces it with the fold tests above, which send only the fold.

In the `toggleGroup` describe (lines 210–237), the inputs and outputs gain `collapsed` (the type now requires it), and the Expand-all case follows J1b:

```ts
    it('opens a closed group by adding its key', () => {
        const closed = group('g:c', ['c'])
        expect(toggleGroup(NO_EXPANSION, closed, [closed])).toEqual({
            all: false,
            keys: [['c']],
            collapsed: [],
        })
    })

    it('closes a group and every group under it', () => {
        const next = toggleGroup(
            { all: false, keys: [['a'], ['a', 'x'], ['b']], collapsed: [] },
            outer,
            rows
        )
        expect(next).toEqual({ all: false, keys: [['b']], collapsed: [] })
    })

    // J1b changed this case: under Expand all a fold is sent as collapse, and Expand
    // all stays, so the groups of later pages open too (was: the open keys in view).
    it('keeps Expand all when closing one group under it', () => {
        expect(toggleGroup(EXPAND_ALL, inner, rows)).toEqual({
            all: true,
            keys: [],
            collapsed: [['a', 'x']],
        })
    })

    it('adds a key once', () => {
        const closed = group('g:c', ['c'])
        expect(toggleGroup({ all: false, keys: [['c']], collapsed: [] }, closed, [closed])).toEqual(
            { all: false, keys: [['c']], collapsed: [] }
        )
    })
```

In `fold all` → `turns an expansion into run options` (line 257): the input becomes `{ all: false, keys: [['a']], collapsed: [] }`.

In `core/reports/lib/__tests__/run-client.test.ts`: import `pageRowCount`; in `foldKeyOf` → `is the same for bodies that differ only in their fold or page` add `{ collapse: [['b']] }` and `{ expandAll: true, collapse: [['b']] }` to the `changed` list. In `rowsKeyOf` → `adds the reader's sort to the key, and only the sort`, add `collapse: [['b']]` to the options of the last saved-report `expect` (the one with `expand: [['open']], cursor: 'c'`). Add to the `runFirstPage` describe:

```ts
    // J1a: a page's limit counts group, opening, detail and subtotal rows, so the
    // reader's place counts them too; Total and computed rows never count.
    it('counts the rows a page limit counts', () => {
        const page = result(
            [
                row('g:a', 'group'),
                row('opening:["a"]', 'opening'),
                row('d:1'),
                row('s:a', 'subtotal'),
                row('total:USD', 'total'),
                row('c:net', 'computed'),
            ],
            ''
        )
        expect(pageRowCount(page)).toBe(4)
    })

    // A pin (it passes before this task too): a grouped page that ends between
    // groups (after a subtotal row) joins the next page with no group or subtotal
    // row twice, and the Total last. Page 2 repeats team a's subtotal row, as a page
    // read across a live change could, so the merge's dedupe is exercised.
    it('joins a grouped page that ends between groups', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(
            result([row('g:a', 'group'), row('d:1'), row('s:a', 'subtotal'), row('total', 'total')], 'c1')
        ).mockResolvedValueOnce(
            result(
                [
                    row('s:a', 'subtotal'),
                    row('g:b', 'group'),
                    row('d:2'),
                    row('s:b', 'subtotal'),
                    row('total', 'total'),
                ],
                ''
            )
        )
        const outcome = await runFirstPage(body, 5)
        expect(outcome.kind === 'ok' && outcome.result.rows.map(r => r.id)).toEqual([
            'g:a',
            'd:1',
            's:a',
            'g:b',
            'd:2',
            's:b',
            'total',
        ])
    })

    // With every group closed a page is all group rows: the target counts them.
    it('joins pages of closed groups until it holds the target rows', async () => {
        send.mockReset()
        send.mockResolvedValueOnce(result([row('g:a', 'group'), row('g:b', 'group')], 'c1'))
            .mockResolvedValueOnce(result([row('g:c', 'group'), row('g:d', 'group')], 'c2'))
            .mockResolvedValueOnce(result([row('g:e', 'group')], ''))
        const outcome = await runFirstPage(body, 4)
        expect(outcome.kind === 'ok' && outcome.result.rows.map(r => r.id)).toEqual([
            'g:a',
            'g:b',
            'g:c',
            'g:d',
        ])
        expect(send).toHaveBeenCalledTimes(2)
    })
```

Existing `runFirstPage` tests whose target moves with the new count (each with this reason in a comment, "J1a: the target counts group, opening and subtotal rows too"):
- `keeps joining past the row it must keep until the detail target is in` (line 327): rename to `…until the target is in` and call `runFirstPage(body, 3, undefined, 'g:a')` (page 1 now counts 2 rows, `g:a` and `d:1`; the target 3 still needs page 2). Expectations unchanged.
- `joins pages until it holds the target detail rows, then follows the last cursor` (line 262): rename to `…the target rows…`; expectations unchanged (3 + 2 counted rows, the Total not counted, reach 4 after the same 2 pages).

In `core/reports/hooks/__tests__/useReportRun.test.tsx`, add inside `describe('useReportRun after a fold', …)`:

```ts
    // F11 with J1a: the reader's place counts group rows. With every group closed the
    // list held six group rows over two pages; a fold re-run loads them all again,
    // not only up to the group in view, so the rows below the view stay.
    it('a fold re-run over closed groups loads as many rows as the reader had', async () => {
        const closedFirst = result(
            [row('g:a', 'group'), row('g:b', 'group'), row('g:c', 'group')],
            'k1'
        )
        const closedLast = result([
            row('g:d', 'group'),
            row('g:e', 'group'),
            row('g:f', 'group'),
            row('total', 'total'),
        ])
        const openedFirst = result(
            [row('g:a', 'group'), row('d:a1'), row('g:b', 'group')],
            'k1'
        )
        sent = []
        answer = request => {
            sent.push(request)
            const isFolded = (request.options.expand ?? []).length > 0
            if (request.options.cursor) return Promise.resolve(closedLast)
            return Promise.resolve(isFolded ? openedFirst : closedFirst)
        }
        const fold = renderFold()
        await waitFor(() => expect(fold.result.current.hasMore).toBe(true))
        act(() => fold.result.current.loadMore())
        await waitFor(() => expect(rowIds(fold.result.current)).toContain('g:f'))
        act(() => fold.result.current.onTopRowIndex?.(1))

        sent = []
        fold.rerender({
            runBody: { ...collapsed, options: { ...collapsed.options, expand: [['a']] } },
        })
        await waitFor(() => expect(rowIds(fold.result.current)).toContain('d:a1'))
        await waitFor(() => expect(fold.result.current.isUpdating).toBe(false))
        expect(sent.map(request => request.options.cursor)).toEqual([undefined, 'k1'])
        expect(rowIds(fold.result.current)).toEqual([
            'g:a',
            'd:a1',
            'g:b',
            'g:d',
            'g:e',
            'g:f',
            'total',
        ])
    })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm exec vitest run core/reports/lib/__tests__/rows.test.ts core/reports/lib/__tests__/run-client.test.ts core/reports/hooks/__tests__/useReportRun.test.tsx`
Expected: FAIL — the new fold tests (toggleGroup returns `{ all: false, keys: [['b']] }`), `foldKeyOf` with `collapse` (differs), `pageRowCount` (not exported), the closed-groups `runFirstPage` test (three requests: a page of group rows adds no detail rows, so it reads to the run's end), and the new `useReportRun` test (`sent` is `[undefined]`: the place counted no rows). (Vitest does not type-check: the `collapsed` type errors show in Step 6's typecheck.)

- [ ] **Step 3: Implement the fold list in `rows.ts`**

Replace lines 75–141 (from the `Expansion` doc comment to the end of `expansionOptions`, keeping `foldAllLabel` and `foldAllTarget` as they are) so the file has:

```ts
/**
 * Which groups a run opens: every one (`all`, Expand all), the levels the report
 * marks open (`isDefault`), or only `keys` (after Collapse all). On the first two,
 * `keys` adds groups of closed levels the reader opened, and `collapsed` holds the
 * groups the reader closed (J1b, R2): the server gets the few folds, never the
 * list of every other open group, which has no limit in a paged report.
 */
export type Expansion = {
    all: boolean
    keys: readonly unknown[][]
    collapsed: readonly unknown[][]
    isDefault?: boolean
}

export const DEFAULT_EXPANSION: Expansion = { all: false, keys: [], collapsed: [], isDefault: true }
export const NO_EXPANSION: Expansion = { all: false, keys: [], collapsed: [] }
export const EXPAND_ALL: Expansion = { all: true, keys: [], collapsed: [] }

const keyText = (key: readonly unknown[] | undefined) => JSON.stringify(key ?? [])

const hasKey = (keys: readonly unknown[][], key: readonly unknown[]) => {
    const text = keyText(key)
    return keys.some(k => keyText(k) === text)
}

const withoutKeys = (keys: readonly unknown[][], drop: ReadonlySet<string>) =>
    keys.filter(k => !drop.has(keyText(k)))

const withKey = (keys: readonly unknown[][], key: unknown[]) =>
    hasKey(keys, key) ? keys : [...keys, key]

// The row and every group row whose chain of sections reaches it. Rows come parent
// first, so one pass finds them all.
function groupAndDescendants(row: ResultRow, rows: readonly ResultRow[]): Set<string> {
    const ids = new Set([row.id])
    for (const r of rows) {
        if (r.kind === 'group' && r.section !== undefined && ids.has(r.section)) ids.add(r.id)
    }
    return ids
}

// The keys of the row's group and of every group under it.
function subtreeKeys(row: ResultRow, rows: readonly ResultRow[]): Set<string> {
    const ids = groupAndDescendants(row, rows)
    return new Set(rows.filter(r => ids.has(r.id)).map(r => keyText(r.groupKey)))
}

// Under Expand all or the open levels, a press changes only the group pressed. A
// group the reader closed opens by leaving `collapsed`; any other closed group is
// of a closed level and opens by its key. An open group the reader opened by its
// key closes by losing it; one open by its level closes by a fold. Closing a group
// also forgets what was opened or folded under it.
function toggleOnOpenBase(expansion: Expansion, row: ResultRow, rows: readonly ResultRow[]) {
    const key = row.groupKey ?? []
    if (!row.expanded) {
        if (hasKey(expansion.collapsed, key)) {
            return { ...expansion, collapsed: withoutKeys(expansion.collapsed, new Set([keyText(key)])) }
        }
        return { ...expansion, keys: withKey(expansion.keys, key) }
    }
    const subtree = subtreeKeys(row, rows)
    const below = new Set([...subtree].filter(text => text !== keyText(key)))
    const keys = withoutKeys(expansion.keys, subtree)
    const collapsed = withoutKeys(expansion.collapsed, below)
    if (hasKey(expansion.keys, key)) return { ...expansion, keys, collapsed }
    return { ...expansion, keys, collapsed: [...collapsed, key] }
}

export function toggleGroup(
    expansion: Expansion,
    row: ResultRow,
    rows: readonly ResultRow[]
): Expansion {
    if (expansion.all || expansion.isDefault) return toggleOnOpenBase(expansion, row, rows)
    const key = row.groupKey ?? []
    if (!row.expanded) return { ...expansion, keys: withKey(expansion.keys, key) }
    return { ...expansion, keys: withoutKeys(expansion.keys, subtreeKeys(row, rows)) }
}
```

Keep `foldAllLabel` and `foldAllTarget` unchanged (they return the constants, whose `collapsed` is `[]`). Replace `expansionOptions`:

```ts
// R2: with `collapse` sent (even empty), the open levels stay open and `expand`
// only adds groups of closed levels; without it, a sent `expand` opens only its
// own groups (after Collapse all). A default state with no change sends nothing,
// so it is the first run's body.
export function expansionOptions(expansion: Expansion): {
    expand?: unknown[][]
    expandAll?: boolean
    collapse?: unknown[][]
} {
    const collapse = [...expansion.collapsed]
    if (expansion.all) return collapse.length > 0 ? { expandAll: true, collapse } : { expandAll: true }
    if (!expansion.isDefault) return { expand: [...expansion.keys] }
    if (expansion.keys.length === 0 && collapse.length === 0) return {}
    const expand = expansion.keys.length > 0 ? { expand: [...expansion.keys] } : {}
    return { ...expand, collapse }
}
```

Delete `openGroupKeys` (no caller is left). `toggleOnOpenBase`'s returns satisfy `Expansion` by inference from the spread; if tsc needs it, annotate the function's return type as `Expansion`.

- [ ] **Step 4: Count a page as the engine counts it (`run-client.ts`)**

In `RunOptions` add, after `expandAll`:

```ts
    /** Groups the reader closed under an open level or Expand all (J1b); not in the rows or fold key. */
    collapse?: unknown[][]
```

In `foldKeyOf`, leave it out with the other fold options:

```ts
    const {
        expand: _expand,
        expandAll: _expandAll,
        collapse: _collapse,
        cursor: _cursor,
        limit: _limit,
        ...options
    } = body.options
```

(`rowsKeyOf` already keys on the report or spec and the sort only.) Replace `detailRowCount`:

```ts
// What a page's limit counts (J1a, R3): group, opening, detail and subtotal rows.
// Total and computed rows repeat on every page and never count.
const COUNTED_KINDS: ReadonlySet<ResultRow['kind']> = new Set([
    'group',
    'opening',
    'detail',
    'subtotal',
])

/** How many rows of a run's result a page's limit counts. */
export function pageRowCount(result: RunResult): number {
    return result.rows.filter(row => COUNTED_KINDS.has(row.kind)).length
}
```

(import `ResultRow` with `RunResult` from `./types`). Change the docs of `RUN_PAGE_SIZE` to `/** The rows a run page asks for (group, opening, detail and subtotal rows count, J1a). */` and of `RUN_MAX_PAGE` to `/** The most rows the server gives a page (run.go runLimits.maxPage). */`. In `runFirstPage`, the doc's first sentence becomes "A run's first page, holding at least `target` rows (what a page's limit counts, `pageRowCount`) and the row `untilRow` when the run has them:", and `loaded += detailRowCount(outcome.result)` becomes `loaded += pageRowCount(outcome.result)`.

- [ ] **Step 5: The reader's place counts rows (`useReportRun.ts`)**

Import `pageRowCount` in place of `detailRowCount`. Replace lines 55–67:

```ts
/**
 * How far a run's first page loads: its rows as a page's limit counts them (group,
 * opening, detail and subtotal rows, J1a), and the group row it must hold (the
 * group at or above the row at the top of the view).
 */
type ReaderPlace = { rows: number; groupRow?: string }

const pageRowsOf = (results: readonly RunResult[]) =>
    results.reduce((n, result) => n + pageRowCount(result), 0)

/** The pages in view: the body they answer (its fold key) and their results. */
type ShownPages = { foldKey: string; results: RunResult[] }

const NO_PLACE: ReaderPlace = { rows: 0 }
```

In `firstPagePlace`, `detailRows: detailRowsOf(results)` becomes `rows: pageRowsOf(results)` (both places), and its leading comment's "loads as many detail rows as the reader had" becomes "loads as many rows as the reader had (counted as a page's limit counts them)"; drop the sentence "with none, the detail rows alone decide" and write "with none, the row count alone decides". In `queryFn`, `runFirstPage(body, place.detailRows, signal, place.groupRow)` becomes `runFirstPage(body, place.rows, signal, place.groupRow)`.

- [ ] **Step 6: Run the unit tests to verify they pass**

Run: `pnpm exec vitest run core/reports/lib/__tests__/rows.test.ts core/reports/lib/__tests__/run-client.test.ts core/reports/hooks/__tests__/useReportRun.test.tsx core/reports/lib/__tests__/view-url.test.ts`
Expected: PASS. The other `useReportRun after a fold` tests keep their expectations: their closed page holds 3 group rows, so the target is 3, and page 1 of the open pages (3 counted rows) meets it, as the detail target 0 did.

Run: `pnpm exec biome check core/reports/lib/rows.ts core/reports/lib/run-client.ts core/reports/hooks/useReportRun.ts core/reports/lib/__tests__/rows.test.ts core/reports/lib/__tests__/run-client.test.ts core/reports/hooks/__tests__/useReportRun.test.tsx && pnpm exec tinycld-pkg typecheck`
Expected: no errors.

- [ ] **Step 7: Run the fold e2e**

Run (ports checked first): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-paged-groups.spec.ts -g "fold"`
Expected: both fold tests (desktop and phone) PASS. The Retry test is Task 9's.

- [ ] **Step 8: Commit** (driver) — `reports: send folds under open levels as collapse and count a page as the engine does (J1b)`

---

### Task 9: A failed next page shows a row with Retry

Core has no next-page failure state today: `TableEnd` (`ReportTable.tsx:301`) shows only `report-loading-more` or `report-end`, a later page's refusal replaces the table with `report-invalid`, and a later page's thrown error turns the view into `report-error` (R8). This task adds the row with the brief's ids.

**Files:**
- Modify: `core/lib/query-errors.ts` (new `QueryRefusal`; `reportQueryError` lines 19–40)
- Modify: `core/reports/lib/run-client.ts` (new `LoadMoreRefused`)
- Modify: `core/reports/lib/run-state.ts` (`RunQueryState`, `RunView`, `baseState`, `runView`)
- Modify: `core/reports/hooks/useReportRun.ts` (`ReportRun`; `queryFn`; `runView` call; `loadMore`; new `retryLoadMore`)
- Modify: `core/reports/components/ReportTable.tsx` (`TableEnd` 298–318; `TableListProps` 371–380; `useTableListProps` 404–437; `ReportTableProps` 531–565; `ReportTable` 576–614)
- Modify: `core/reports/components/ReportView.tsx` (`TableBody` 155–180)
- Modify (test helper): `core/tests/fake-pb-server.ts`
- Test: `core/lib/__tests__/query-errors.test.ts`, `core/reports/lib/__tests__/run-state.test.ts`, `core/reports/hooks/__tests__/useReportRun.test.tsx`

**Interfaces:**
- Consumes: Task 8 `useReportRun` (place by rows); `MessageBody` (`core/reports/components/ReportStates.tsx:43`, core's existing error-with-Retry style); `RunRefusal`.
- Produces: `class QueryRefusal extends Error` (`@tinycld/core/lib/query-errors`); `class LoadMoreRefused extends QueryRefusal { readonly refusal: RunRefusal }` (run-client.ts); `RunQueryState.isLoadMoreError: boolean` (a failed next page keeps the state `ready`); `ReportRun.loadMoreFailed?: boolean`, `ReportRun.retryLoadMore?(): void` (optional: a hand-built ReportRun, such as a static mockup, loads no pages); `TableEnd({ hasMore, failed, onRetry }: { hasMore: boolean; failed: boolean; onRetry?: () => void })`; `ReportTableProps.loadMoreFailed?: boolean`, `ReportTableProps.onRetryMore?(): void`; `class FakeAnswer { status: number; body: unknown }` (fake-pb-server.ts); test ids `report-loading-more-error`, `report-loading-more-retry`; text "Could not load more rows".

- [ ] **Step 1: Write the failing tests**

`core/tests/fake-pb-server.ts` — a route may answer with a status (test helper, needed to send a 400 through the real `pb` client). Add after `json`:

```ts
/** A route's answer with an HTTP status other than 200, such as a refusal. */
export class FakeAnswer {
    readonly status: number
    readonly body: unknown

    constructor(status: number, body: unknown) {
        this.status = status
        this.body = body
    }
}
```

and in `fetchStub` replace the last line with:

```ts
        const answer = await abortable(route(body, init?.signal ?? undefined), init?.signal)
        return answer instanceof FakeAnswer ? json(answer.body, answer.status) : json(answer)
```

`core/lib/__tests__/query-errors.test.ts` — import `QueryRefusal` with `reportQueryError`, and add:

```ts
    // A refusal the person caused is the screen's own state (a next page the server
    // refused): logged, never reported as a bug.
    it('does not report a refusal', () => {
        reportQueryError(new QueryRefusal('reports.run: a later page was refused'), queryKey, vi.fn())
        expect(captureExceptionToSentry).not.toHaveBeenCalled()
    })
```

`core/reports/lib/__tests__/run-state.test.ts` — the `query` helper gains `isLoadMoreError: false,`; add:

```ts
    // R8: a failed next page keeps the pages already shown and the run ready; the
    // list ends in a row with Retry (useReportRun's loadMoreFailed).
    it('stays ready with its rows when only the next page failed', () => {
        const view = runView(query({ isError: true, isLoadMoreError: true, pages: [ok()] }))
        expect(view.state).toBe('ready')
        expect(view.result?.rows).toHaveLength(1)
    })
```

`core/reports/hooks/__tests__/useReportRun.test.tsx` — import `FakeAnswer` from `'../../../tests/fake-pb-server'`; widen `type Answer = (body: RunBody) => Promise<RunResult | FakeAnswer>`; add to `describe('useReportRun', …)`:

```ts
    // R8: a refused next page keeps the rows shown and offers Retry. The list asks
    // for more on every redraw, so loadMore must not ask again until Retry.
    it('keeps the rows of a refused next page, asks again only on Retry, and Retry loads it', async () => {
        const first = result([row('g:A', 'group'), row('d:1')], 'c1')
        const second = result([row('d:2'), row('total', 'total')])
        const refusal = new FakeAnswer(400, {
            status: 400,
            message: 'The page cursor is not valid.',
            data: {},
        })
        let nextPages = 0
        answer = sent => {
            if (!sent.options.cursor) return Promise.resolve(first)
            nextPages += 1
            return Promise.resolve(nextPages === 1 ? refusal : second)
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.hasMore).toBe(true))

        act(() => run.current.loadMore())
        await waitFor(() => expect(run.current.loadMoreFailed).toBe(true))
        expect(run.current.state).toBe('ready')
        expect(rowIds(run.current)).toEqual(['g:A', 'd:1'])
        // loadMore starts nothing: no loading state, and no request reaches the
        // server (a request would arrive after a few awaits, so the test waits).
        act(() => run.current.loadMore())
        expect(run.current.isLoadingMore).toBe(false)
        await act(async () => {
            await new Promise(resolve => setTimeout(resolve, 50))
        })
        expect(nextPages).toBe(1)

        act(() => run.current.retryLoadMore?.())
        // While Retry runs the list shows "Loading more rows…", not the error row.
        expect(run.current.isLoadingMore).toBe(true)
        expect(run.current.loadMoreFailed).toBe(false)
        await waitFor(() => expect(rowIds(run.current)).toEqual(['g:A', 'd:1', 'd:2', 'total']))
        expect(run.current.loadMoreFailed).toBe(false)
        expect(nextPages).toBe(2)
    })

    // TanStack keeps the error status, with the pages, through any later fetch, and
    // a live rerun is no next page: the run must stay ready with its error row, not
    // turn into the whole-report error, until the rerun lands.
    it('stays ready through a live rerun after a refused next page', async () => {
        const first = result([row('g:A', 'group'), row('d:1')], 'c1')
        const refusal = new FakeAnswer(400, {
            status: 400,
            message: 'The page cursor is not valid.',
            data: {},
        })
        let firstPages = 0
        let rerun: ReturnType<typeof held<RunResult>> | undefined
        answer = sent => {
            if (sent.options.cursor) return Promise.resolve(refusal)
            firstPages += 1
            if (firstPages === 1) return Promise.resolve(first)
            rerun = held(first)
            return rerun.promise
        }
        const { result: run } = renderRun()
        await waitFor(() => expect(run.current.hasMore).toBe(true))
        await subscribed()
        act(() => run.current.loadMore())
        await waitFor(() => expect(run.current.loadMoreFailed).toBe(true))

        signal()
        await waitFor(() => expect(rerun).toBeDefined())
        expect(run.current.state).toBe('ready')
        expect(run.current.loadMoreFailed).toBe(true)
        expect(rowIds(run.current)).toEqual(['g:A', 'd:1'])

        act(() => rerun?.release())
        await waitFor(() => expect(run.current.loadMoreFailed).toBe(false))
        expect(run.current.state).toBe('ready')
    })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm exec vitest run core/lib/__tests__/query-errors.test.ts core/reports/lib/__tests__/run-state.test.ts core/reports/hooks/__tests__/useReportRun.test.tsx`
Expected: FAIL — `QueryRefusal` is not exported; the run-state case is `error`; in the hook `loadMoreFailed` stays undefined, because the later refusal becomes `state: 'invalid'`.

- [ ] **Step 3: `QueryRefusal` in `core/lib/query-errors.ts`**

Add after `couldNotReachServer`:

```ts
/**
 * A read the server refused for a reason the person caused, which the screen shows
 * as its own state. It is thrown, so the query keeps it as an error state, and it is
 * logged, not reported: it is not a bug.
 */
export class QueryRefusal extends Error {}
```

In `reportQueryError`, before the `couldNotReachServer` check:

```ts
    if (error instanceof QueryRefusal) {
        log.info('query.refused', error.message, { queryKey })
        return
    }
```

- [ ] **Step 4: `LoadMoreRefused` and the run state**

In `core/reports/lib/run-client.ts` (import `QueryRefusal` from `@tinycld/core/lib/query-errors`), after `RunPage`:

```ts
/**
 * A later page's refusal (R8). It is thrown, not returned as a page, so the query
 * keeps the pages already shown and marks only the next page failed; a first page's
 * refusal stays the run's own state.
 */
export class LoadMoreRefused extends QueryRefusal {
    readonly refusal: RunRefusal

    constructor(refusal: RunRefusal) {
        super(`reports.run: a later page was refused (${refusal.kind})`)
        this.refusal = refusal
    }
}
```

In `core/reports/lib/run-state.ts`:

```ts
export interface RunQueryState {
    hasBody: boolean
    pages: readonly RunOutcome[]
    isPending: boolean
    isError: boolean
    /** Only the next page failed; the pages already loaded stay (R8). */
    isLoadMoreError: boolean
    /** The pages are the previous body's answer, shown while this body's run loads. */
    isPlaceholderData: boolean
}
```

`baseState`: `if (query.isError) return 'error'` becomes `if (query.isError && !query.isLoadMoreError) return 'error'`. `RunView` does not change: whether the error row shows also depends on a fetch in flight, which only the hook knows.

- [ ] **Step 5: The hook throws a later refusal and offers Retry (`useReportRun.ts`)**

Import `LoadMoreRefused`. `ReportRun` (still `extends Omit<RunView, 'topics' | 'heldRows'>`) gains:

```ts
    /**
     * The next page failed (R8): the rows shown stay, and the list ends in a row with
     * Retry. Optional, because a hand-built ReportRun (a mockup) loads no pages.
     */
    loadMoreFailed?: boolean
    /** Asks again for the next page that failed. Optional, as `loadMoreFailed`. */
    retryLoadMore?(): void
```

In `queryFn`, after the `outcome` is computed and before the recent-store mark:

```ts
            // A later page's refusal is thrown, so the query keeps the pages shown and
            // marks only the next page failed (R8).
            if (outcome.kind !== 'ok' && pageParam !== '') throw new LoadMoreRefused(outcome)
```

TanStack's own flag is not enough: after a next page fails, the query keeps `status: 'error'` (with its pages) through any later fetch, and `isFetchNextPageError` turns false as soon as another fetch starts (a live rerun has no direction). So the hook keeps its own mark of the run whose last next page failed. After `runKey.current = queryKey`, add:

```ts
    // The run whose last next page failed (R8), by its query key's text. Set when a
    // next page fails, cleared when one succeeds; it counts only while the query is
    // in error, so a rerun that lands clears it too.
    const runKeyText = JSON.stringify(queryKey)
    const failedNextPageOf = useRef<string | null>(null)
    const isLoadMoreError = query.isError && failedNextPageOf.current === runKeyText
```

Move the `runView({ … })` call below these lines (it reads `isLoadMoreError`; `runKey` and `whenIdle` keep their place) and give it `isLoadMoreError,`. Replace `loadMore` with:

```ts
    const startNextPage = () => {
        if (!query.hasNextPage || query.isFetchingNextPage || waitingForMore.current) return
        waitingForMore.current = true
        setIsWaitingForMore(true)
        const key = runKeyText
        void whenIdle(async () => {
            const next = await query.fetchNextPage()
            failedNextPageOf.current = next.isFetchNextPageError ? key : null
        }).finally(() => {
            // Also redraws, so the mark set above is read.
            waitingForMore.current = false
            setIsWaitingForMore(false)
        })
    }
    // A failed next page waits for Retry: the list asks for more on every redraw,
    // and the same request would fail again.
    const loadMore = () => {
        if (isLoadMoreError) return
        startNextPage()
    }
    const isLoadingMore = query.isFetchingNextPage || isWaitingForMore
```

The returned object uses `isLoadingMore,` (in place of the inline expression) and gains `loadMoreFailed: isLoadMoreError && !isLoadingMore,` and `retryLoadMore: startNextPage,` beside `loadMore`. A rerun that itself fails after a next-page failure keeps the rows and the error row (Retry asks for the next page; the next live change reruns): that is accepted, as the rows shown are still the last good answer. (`retry: (count, error) => count < 2 && isTransientRunError(error)` stays: a network failure of a next page is retried twice before the row shows; a refusal is not retried.)

- [ ] **Step 6: The error row (`ReportTable.tsx`, `ReportView.tsx`)**

In `ReportTable.tsx`, import `MessageBody` from `./ReportStates`, and replace `TableEnd`:

```tsx
// D8, D12: the list ends in what comes next. While a later page is left the Total
// is held back, and this row stands in its place; after the last page, a short
// end row (a page footer is for exports only). A next page that failed (R8) ends
// the list in core's error-with-Retry state: the rows above stay.
function TableEnd({
    hasMore,
    failed,
    onRetry,
}: {
    hasMore: boolean
    failed: boolean
    onRetry?: () => void
}) {
    if (failed) {
        const retry = onRetry
            ? { label: 'Retry', testID: 'report-loading-more-retry', onPress: onRetry }
            : null
        return (
            <MessageBody
                isVisible
                testID="report-loading-more-error"
                message="Could not load more rows"
                action={retry}
            />
        )
    }
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

`TableListProps` gains, after `hasMore`:

```ts
    /** The next page failed: the list ends in "Could not load more rows" with Retry. */
    loadMoreFailed: boolean
    onRetryMore?(): void
```

`useTableListProps` destructures `loadMoreFailed, onRetryMore` and sets `ListFooterComponent: <TableEnd hasMore={hasMore} failed={loadMoreFailed} onRetry={onRetryMore} />`. `ReportTableProps` gains, after `hasMore`:

```ts
    /** The next page failed (R8); absent: it did not. */
    loadMoreFailed?: boolean
    /** Asks again for the next page that failed. */
    onRetryMore?(): void
```

`ReportTable` passes `loadMoreFailed={props.loadMoreFailed ?? false}` and `onRetryMore={props.onRetryMore}` to `TableList`. In `ReportView.tsx` `TableBody`, pass `loadMoreFailed={model.run.loadMoreFailed}` and `onRetryMore={model.run.retryLoadMore}` to `ReportTable`.

- [ ] **Step 7: Run the unit tests to verify they pass**

Run: `pnpm exec vitest run core/lib/__tests__/query-errors.test.ts core/reports/lib/__tests__/run-state.test.ts core/reports/hooks/__tests__/useReportRun.test.tsx core/reports/lib/__tests__/run-client.test.ts`
Expected: PASS.

Run: `pnpm exec biome check core/lib/query-errors.ts core/tests/fake-pb-server.ts core/reports/lib/run-client.ts core/reports/lib/run-state.ts core/reports/hooks/useReportRun.ts core/reports/components/ReportTable.tsx core/reports/components/ReportView.tsx core/lib/__tests__/query-errors.test.ts core/reports/lib/__tests__/run-state.test.ts core/reports/hooks/__tests__/useReportRun.test.tsx && pnpm exec tinycld-pkg typecheck`
Expected: no errors.

- [ ] **Step 8: Run the whole J1 e2e file**

Run (ports checked first): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-paged-groups.spec.ts`
Expected: all five tests PASS.

- [ ] **Step 9: Commit** (driver) — `reports: a failed next page shows a row with Retry`

---

### Task 10: Existing paging e2e, help and docs for J1

**Files:**
- Modify: `tests/e2e/reports-ledger-v3.spec.ts` (the `pagedTeams` doc comment; the test `Expand all from closed groups keeps the reader at the group in view`; `foldRequests`). Line numbers moved in Task 7: find them by name.
- Modify: `docs/packages.md` (lines 1244–1250, 1260, 1271–1275, 1306–1311)
- Modify: `core/help/reports.md` (line 111, line 137)

**Interfaces:**
- Consumes: Task 7 helpers (`openPagedTeams`, `MAX_PAGE`, `USUAL_PAGE`, `SentRun`); Tasks 8–9 client behavior.
- Produces: nothing new.

R9 holds: no core e2e counts on the 5,000 notice for a grouped report (`grep -rn -i -e 'shows its first' -e rowCap -e 'report-notice-rowCap' tests/e2e` finds none). The paging e2e whose page counts move are in `reports-ledger-v3.spec.ts` (pages of 5 rows, now counting group, opening, detail and subtotal rows).

- [ ] **Step 1: Move `reports-ledger-v3.spec.ts` to the J1 counts**

`pagedTeams` doc comment: "in pages of 5 detail rows, so 12 sales page across team A's end" becomes "in pages of 5 rows (group, opening, detail and subtotal rows count, J1a), so team A's 9 rows page across its end".

The Expand-all test (`Expand all from closed groups keeps the reader at the group in view`): in its leading comment, "After Collapse all the first page holds every group row of the report" becomes "After Collapse all the first pages hold every group row of the report". Replace its last assertion (`expect(foldRequests(sent), 'requests of Expand all at team B').toHaveLength(2)` and the comment above it) and delete `foldRequests` (no other caller):

```ts
        // The fold run's requests, in pages of 5 rows that count group, opening,
        // detail and subtotal rows (J1a). The reader had 6 group rows (Collapse all)
        // and team B's group row at the top. Request 1 (the usual size) gets team
        // A's group, opening and first three sales; 5 of the 6 rows. Request 2 (the
        // usual size: the rows are not all in yet) gets A's last three sales and its
        // subtotal; team B's group and opening rows stay together on the next page
        // (R3). Request 3 (the server's most: only team B's group row is missing)
        // starts with it. Before J1 the target counted detail rows only (0 here), so
        // request 2 already asked for the server's most.
        expect(
            sent.slice(0, 3).map(run => [run.cursor !== undefined, run.limit]),
            'requests of Expand all at team B'
        ).toEqual([
            [false, USUAL_PAGE],
            [true, USUAL_PAGE],
            [true, MAX_PAGE],
        ])
```

Import `USUAL_PAGE` from `./reports-helpers` beside `MAX_PAGE`; drop `SentRun` from the imports if nothing uses it now.

- [ ] **Step 2: Run the ledger e2e**

Run (ports checked first): `E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-ledger-v3.spec.ts`
Expected: PASS. If an assertion on pages or requests still fails, work out the pages with the J1 count (R3) as in Step 1, change only that assertion, and write the reason beside it. A failure that is not about how a page counts is a defect: find its root cause.

- [ ] **Step 3: `docs/packages.md`**

Lines 1244–1250 become:

```markdown
- A page cursor is tied to the report's sort keys, its viewer sort
  (`options.sort`), its group levels, its fold state (`options.expand`,
  `options.expandAll`, `options.collapse`) and the value types. A cursor from a
  report that sorts, groups or folds differently is refused. Every report
  pages: a page holds 200 rows by default, and at most 1,000 (`options.limit`).
  A grouped report (not a pivot, a layout report or a nest) counts its group,
  opening, detail and subtotal rows in document order: a closed group counts
  as its one row, and a group row and its opening row stay on one page. Its
  group rows page too, however many groups it has. Every page ends with the
  Total. Pivot, layout and nest reports read their group rows whole, at most
  5,000, and count their detail rows.
```

Line 1260 becomes:

```markdown
- `options.expand` and `options.collapse` each list at most 5,000 group keys.
  `options.collapse` closes groups under a level that is open by default or
  under `options.expandAll`. When `collapse` is sent (also empty), the levels
  open by default stay open and `expand` opens groups of closed levels; when
  only `expand` is sent, only its groups open.
```

Lines 1271–1275 become:

```markdown
- Pivot, nest and layout results stop at 5,000 rows and add a notice, once per
  run. A `rowCap` or `nestCap` notice gives the rows it kept in `count`. A
  `pivotCap` notice gives the pivot values it left out in `leftOut`. A grouped
  report pages its group rows instead, so it has no `rowCap` notice. A layout
  report never drops its computed rows for the cap, and it ends in those
  computed rows with no Total row.
```

In lines 1306–1311, the first sentence becomes "A grouped result pages (`page.next`), its group rows too; running values continue on the next page. `page.size` is the page's detail rows." (the rest stays).

- [ ] **Step 4: `core/help/reports.md`**

Line 111 becomes:

```markdown
To open a group level when the report opens, turn on **Open when the report opens** for that level. To close or open one group, select its row. The other groups stay as they are, also the groups that load when you scroll further. **Collapse all** closes the groups until you open them again.
```

In line 137, "A long report loads more rows as you scroll, also when it has groups." becomes "A long report loads more rows as you scroll, also when it has groups, however many groups it has." and, after "…and **End of report** marks the end.", add: "If more rows cannot load, the list ends with **Could not load more rows**. Select **Retry** to try again. The rows that show stay."

- [ ] **Step 5: Check the help renders**

Run (no e2e run may be in progress, rules file): `pnpm run packages:generate` (it builds the help topics, `scripts/gen-help.ts`).
Expected: it ends without an error.

- [ ] **Step 6: Commit** (driver) — `reports: paging e2e, docs and help follow J1`

---

### Task 11: J2 — one line shares spare width by `flexGrow`

Ruling R10 (outline): J2 changes the one-line layout only, as the brief says ("the one-line layout"): `fitOneLine` uses a new `shareLineWidth`. Arranged lines and automatic (wrapped) lines keep `shareWidth`, whose side parts' grow is their tracks (automatic lines give every part 1 track, `table-lines.ts:268`, so sharing by grow there would split a phone line evenly, whatever its content). A column with no grow (Extra room **None**) still gets what it lacks of its natural width, in proportion when the room is short, and never more: otherwise every figure whose heading is wider than its values would wrap its heading on any screen.

**Files:**
- Modify: `core/reports/lib/table-lines.ts` (`shareWidth` lines 398–418 into `share` + `shareWidth` + `shareLineWidth`; `fitOneLine` lines 420–436)
- Test: `core/reports/lib/__tests__/table-lines.test.ts` (new describe after line 145; `fitOneLine by content` lines 118–140)
- Check (change only where the rule changes a width): `tests/e2e/reports-column-fit.spec.ts`, `tests/e2e/reports-fit.spec.ts`, `tests/e2e/reports-wrap.spec.ts`. `tests/e2e/reports-text-width.spec.ts` compares text estimates with drawn text and reads no shared width: not run.
- Modify: `docs/packages.md` (line 1368), `core/help/reports.md` (lines 121, 131)

**Interfaces:**
- Consumes: `ColumnMeasure` (`{ natural; min; grow; max?; pct?; lines? }`, column-measure.ts), `growInto`, `sum`.
- Produces: `shareLineWidth(parts: readonly ColumnMeasure[], width: number): number[] | null`; `shareWidth(parts, width)` unchanged in signature and result; `fitOneLine(columns, width, measures)` unchanged in signature.

- [ ] **Step 1: Write the failing tests**

In `table-lines.test.ts`, add `shareLineWidth` to the `../table-lines` import and add after the `fitOneLine by content` describe:

```ts
// J2 (human, 2026-11-08): one line shares its spare room by Extra room (flexGrow),
// not by what each column lacks of its natural width.
describe('one line shares its spare room by Extra room', () => {
    const COLS_J2: ResultColumn[] = [
        { id: 'name', label: 'Name', type: 'text' },
        { id: 'description', label: 'Description', type: 'text' },
        { id: 'amount', label: 'Amount', type: 'money', currency: 'USD' },
    ]

    it('gives a Description with flexGrow 2 more of the spare room than a Name with flexGrow 1, also when Name is wider', () => {
        // Mins 80 + 80 + 100 = 260; 220 spare by grow: 73 and 146, the 1 left over
        // to the first column.
        const wideName = { name: m(80, 300, 1), description: m(80, 165, 2), amount: m(100, 100, 0) }
        const narrowName = { name: m(80, 120, 1), description: m(80, 300, 2), amount: m(100, 100, 0) }
        for (const measures of [wideName, narrowName]) {
            const widths = fitOneLine(COLS_J2, 480, measures)
            expect(widths).toEqual({ name: 154, description: 226, amount: 100 })
            const spare = (id: 'name' | 'description') => (widths?.[id] ?? 0) - 80
            expect(spare('description')).toBeGreaterThan(spare('name'))
        }
    })

    it('gives a column with no Extra room its content width when there is room, and no more', () => {
        const parts = [m(80, 300, 1), m(60, 140, 0), m(100, 100, 0)]
        // Room for all: the None column takes its 80 of lack, the rest goes by grow.
        expect(shareLineWidth(parts, 600)).toEqual([360, 140, 100])
        // 40 spare: the None column's lack comes first (it gets no other room).
        expect(shareLineWidth(parts, 280)).toEqual([80, 100, 100])
        expect(shareLineWidth(parts, 239)).toBeNull()
    })
})
```

Change `fitOneLine by content` (lines 121–140), each with its reason:

```ts
    // J2: the spare room goes by grow (account and number, 1 each), not first to
    // account for what it lacks of its natural width (was 198 / 82 at 500 and
    // 140 / 64 at 424).
    it('gives each column its min and the spare room to the text by grow', () => {
        // Mins 404; 96 spare: 48 each.
        expect(fitOneLine(COLS, 500, measures)).toEqual({
            account: 168,
            number: 112,
            debit: 110,
            credit: 110,
        })
    })

    it('narrows the text toward its min before the line breaks', () => {
        // 20 spare: 10 each.
        expect(fitOneLine(COLS, 424, measures)).toEqual({
            account: 130,
            number: 74,
            debit: 110,
            credit: 110,
        })
        expect(fitOneLine(COLS, 403, measures)).toBeNull()
        expect(fitOneLine([], 10, measures)).toEqual({})
    })
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm exec vitest run core/reports/lib/__tests__/table-lines.test.ts`
Expected: FAIL — `shareLineWidth` is not exported; `fitOneLine` gives `{ name: 239, description: 141, … }` for the wide Name, and 198 / 82 and 140 / 64 in `fitOneLine by content`.

- [ ] **Step 3: Implement `shareLineWidth`**

Replace lines 398–436 of `table-lines.ts` (the `shareWidth` doc to the end of `fitOneLine`):

```ts
const lackOf = (part: ColumnMeasure) => Math.max(0, part.natural - part.min)

// Shares `width` among the parts: each at its min; then what `lack` gives each part,
// in proportion when the room is short; then the rest by grow, a part stopping at
// its max. The parts add up to `width` exactly: the rounding remainder goes to the
// first part (the label). Null when the mins do not fit.
function share(
    parts: readonly ColumnMeasure[],
    width: number,
    lack: (part: ColumnMeasure) => number
): number[] | null {
    if (parts.length === 0) return []
    const widths = parts.map(part => part.min)
    if (sum(widths) > width) return null
    const lacks = parts.map(lack)
    const totalLack = sum(lacks)
    if (totalLack > 0) {
        const fraction = Math.min(1, (width - sum(widths)) / totalLack)
        for (const [i, need] of lacks.entries())
            widths[i] = (widths[i] ?? 0) + Math.floor(need * fraction)
    }
    growInto(widths, parts, width)
    widths[0] = (widths[0] ?? 0) + width - sum(widths)
    return widths
}

/**
 * Widths that share `width` by content, as CSS table-layout:auto does: the spare
 * room first in proportion to what each part lacks of its natural width, then by
 * grow. A side block's parts (arranged and automatic lines), whose grow is their
 * tracks.
 */
export function shareWidth(parts: readonly ColumnMeasure[], width: number): number[] | null {
    return share(parts, width, lackOf)
}

/**
 * The widths of one line (J2, R10): each column at its min, then the spare room by
 * grow (Extra room, `flexGrow`), so a column with twice the grow gets twice the
 * room, whatever its content. A column with no grow gets only what it lacks of its
 * natural width, and no more, so a figure's heading does not wrap when there is
 * room.
 */
export function shareLineWidth(parts: readonly ColumnMeasure[], width: number): number[] | null {
    return share(parts, width, part => (part.grow > 0 ? 0 : lackOf(part)))
}

/**
 * The widths of one line at this width (review §7.1-3), or null when the columns'
 * mins do not fit: figures, dates and short text are never cut (their min is their
 * natural width); long text narrows to its min. The spare room goes by Extra room
 * (shareLineWidth).
 */
export function fitOneLine(
    columns: readonly ResultColumn[],
    width: number,
    measures: ColumnMeasures
): Record<string, number> | null {
    const widths = shareLineWidth(
        columns.map(column => measureOf(measures, column.id)),
        width
    )
    if (!widths) return null
    return Object.fromEntries(columns.map((column, i) => [column.id, widths[i] ?? 0]))
}
```

- [ ] **Step 4: Run the unit tests**

Run: `pnpm exec vitest run core/reports/lib/__tests__/table-lines.test.ts core/reports/lib/__tests__/column-measure.test.ts`
Expected: PASS. The `shareWidth` describe and `stops a grower at its max …` keep their numbers (`shareWidth` is unchanged), and `a % width is a part of the line` keeps `{ account: 380, number: 200, … }` (no column lacks: 260 spare by grow, number capped at 200, the rest to account). A test that fails elsewhere: work out the width under `shareLineWidth`, change only that expectation, and write the reason beside it ("J2: spare room by grow, was …").

Run: `pnpm exec biome check core/reports/lib/table-lines.ts core/reports/lib/__tests__/table-lines.test.ts && pnpm exec tinycld-pkg typecheck`
Expected: no errors.

- [ ] **Step 5: Run the core e2e that read one-line widths, one file at a time**

Run (ports checked first, each file alone):
`E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-column-fit.spec.ts`
`E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-fit.spec.ts`
`E2E_PORT=7210 E2E_PORT_2=7211 pnpm exec tinycld-pkg test:e2e -- reports-wrap.spec.ts`
Expected: PASS. Where an assertion fails because a one-line column now has another width (for example `reports-wrap.spec.ts:822` "The detail label gets the room its text needs" at 1100 px, where the label now shares the spare room with the other text columns by grow), change only that assertion to what the rule gives, with the reason in a comment ("J2: one line shares spare room by Extra room"). A failure that is not about a shared width is a defect: find its root cause. Record each changed assertion in the task report.

- [ ] **Step 6: Docs and help**

`docs/packages.md` line 1368 becomes:

```markdown
| `display.columns.<col>.flexGrow` | 0 to 10: the column's share of the spare room on one line, the room past every column's least width. A column with 2 gets twice the room of a column with 1, whatever their content. A column with 0 gets at most its content's width (default 1 for text and the label column, 0 for other figures). |
```

`core/help/reports.md` line 121 becomes:

```markdown
A report never scrolls sideways. Each column gets at least its least width. Numbers, amounts, percents, dates and yes/no values are never cut short, and short text such as an entry number is not either. Long text gets narrower first and ends in "…". On one line, the rest of the room goes to the columns by their **Extra room**: a column with **Double** gets twice the room of a column with **Normal**, also when its text is shorter. A column with **None** gets the room that its values need, and no more. A column with no value is as wide as its heading.
```

In line 131, "**Extra room** is the column's share of the spare room on one line: **None**, **Normal**, **Double** or **Triple**." becomes "**Extra room** is the column's share of the spare room on one line: **None**, **Normal**, **Double** or **Triple**. A column with **Double** gets twice the spare room of a column with **Normal**, whichever column has the longer text." (the rest of the line stays).

Then, with no e2e run in progress, `pnpm run packages:generate` must end without an error (it builds the help topics).

- [ ] **Step 7: Commit** (driver) — `reports: one line shares spare width by Extra room (J2)`

---

## Books follow-up from this part

- `detailRowCount` is now `pageRowCount`; Books does not import it.
- `ReportRun.loadMoreFailed` and `retryLoadMore` are optional, so a hand-built `ReportRun` (the Books helix mockup screen) still typechecks.
- J2 changes the one-line widths of every Books report: Do step 4 (general-ledger and trial-balance e2e after the merge) covers them, and checkpoint 2 removes the Journal's Name 16ch (brief, "D1 reversed").
