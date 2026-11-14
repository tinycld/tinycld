import { expect, type Locator, type Page, test } from '@playwright/test'
import { skipWithoutReportsStub, waitForTableFit } from './helpers'
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

/** Scrolls the row's list to its end, finding the list and scrolling it in one step. */
function scrollListToEnd(row: Locator) {
    return row.evaluate(element => {
        let list = element.parentElement
        while (list && getComputedStyle(list).overflowY !== 'auto') list = list.parentElement
        if (!list) throw new Error('the row has no vertical scroller')
        list.scrollTop = list.scrollHeight
    })
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
    // setViewportSize returns before the app handles the resize, so a check made now
    // reads the old list. At a phone width the view then swaps its list for the
    // phone's (another list) and draws the rows again once they fit the new width;
    // the fit marker settles in that last draw. Wait for it, then read the list.
    await waitForResizedFit(page, size)
    await expect(firstSale).toBeVisible()
    await expect(rowOf(page, teams[2] ?? '')).toHaveCount(0)
    await scrollListToEnd(firstSale)
    await expect(page.getByTestId('report-loading-more')).toContainText('Loading more rows…')
    expect(totals(await rowIds(page))).toEqual([])

    release()
    const rows = await scrollThrough(page, firstSale)
    const groups = rows.filter(row => row.id.startsWith('g:'))
    expect(groups).toHaveLength(teams.length)
    for (const team of teams) {
        expect(
            groups.filter(g => g.text.includes(team)),
            `the group row of ${team}`
        ).toHaveLength(1)
    }
    expect(rows[rows.length - 1]?.id).toMatch(/^total/)
    await expect(page.getByTestId('report-end')).toHaveText('End of report')
}

/**
 * Waits until the table fits the window of `size`. The fit of the window before
 * the resize (openPagedTeams opens it 1280 px wide) is settled too, so
 * waitForTableFit alone can pass on it: first wait until the fit follows the new
 * width.
 */
async function waitForResizedFit(page: Page, size: typeof DESKTOP) {
    await expect
        .poll(() => page.getByTestId('report-table').evaluate(el => Number(el.dataset.fitWidth)))
        .toBeLessThanOrEqual(size.width)
    await waitForTableFit(page)
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
    // As in scrollToNextGroups: read the list only once it fits the new width.
    await waitForResizedFit(page, size)
    const groupB = rowOf(page, teams[1] ?? '')
    await expect(groupB).toHaveAttribute('aria-expanded', 'true')
    const rowHeight = await shortenList(page, groupB, 4)
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
    expect(
        Math.abs((await offsetOfRow(page, topBefore.id)) - topBefore.offset)
    ).toBeLessThanOrEqual(rowHeight)

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
        const { teams, firstSale, releaseReruns } = await openPagedTeams(page, 'PgRetry', {
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
        releaseReruns()
        const rows = await scrollThrough(page, firstSale)
        expect(rows.some(row => row.text.includes(teams[2] ?? '?'))).toBe(true)
        expect(rows[rows.length - 1]?.id).toMatch(/^total/)
        await expect(failed).toHaveCount(0)
        await expect(page.getByTestId('report-end')).toHaveText('End of report')
    })
})
