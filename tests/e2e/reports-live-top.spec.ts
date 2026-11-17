import type { Browser, ElementHandle, Page } from '@playwright/test'
import { loginAs, skipWithoutReportsStub } from './helpers'
import { expect, test } from './member-fixtures'
import {
    addColumn,
    addSales,
    chooseOption,
    listScroller,
    openReportStub,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    seedStub,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Web: a reader at the top of a live report sorted newest first sees the rows a
 * live re-run adds above the first row. The list keeps the row in view in place
 * only for a reader scrolled away from the top (reports-scroll-anchor.spec.ts);
 * at the top it stays at the top. Wide (the fixed heading row) and phone (the top
 * area scrolls away with the rows) are two list variants.
 */

const OLD_SALES = 30
const NEW_SALES = 20

interface LiveReport {
    team: string
    /** Every customer of this test starts with it; the report shows only those. */
    stem: string
}

/**
 * A saved report over this test's own sales, newest booked day first: OLD_SALES
 * sales from 2026-01-01 on, one day apart.
 */
async function newestFirst(p: Page): Promise<LiveReport> {
    const team = uniqueName('Team')
    const stem = uniqueName('Live')
    await seedStub(p, {
        teams: [{ name: team }],
        sales: Array.from({ length: OLD_SALES }, (_, i) => ({
            team,
            customer: `${stem} Old ${String(i).padStart(2, '0')}`,
            amount: 1000 + i,
            cost: 400,
            quantity: 1,
            day: new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10),
        })),
    })
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await addColumn(p, ['booked'])
    await p.getByTestId('report-builder-section-filter').click()
    await p.getByTestId('report-builder-filter-add-condition').click()
    await p
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-field-customer')
        .click()
    await chooseOption(p, 'report-builder-filter-0-op', 'startsWith')
    await p.getByTestId('report-builder-filter-0-value').fill(stem)
    await p.getByTestId('report-builder-section-group').click()
    await chooseOption(p, 'report-builder-sort-add-menu', 'booked')
    await p
        .getByTestId('report-builder-sort-0')
        .getByRole('radio', { name: 'Z to A', exact: true })
        .click()
    await saveNewReport(p, uniqueName('Newest'))
    return { team, stem }
}

/** Adds NEW_SALES sales, all booked after the old ones, from another browser. */
async function addNewerElsewhere(
    browser: Browser,
    user: { username: string; password: string },
    report: LiveReport
) {
    const other = await browser.newContext()
    try {
        const q = await other.newPage()
        await loginAs(q, user.username, user.password)
        await openReportStub(q)
        await addSales(q, {
            team: report.team,
            customerPrefix: `${report.stem} New`,
            count: NEW_SALES,
            firstDay: '2027-01-01',
            firstAmount: 5000,
            cost: 400,
            quantity: 1,
        })
    } finally {
        await other.close()
    }
}

// Pixels a wheel step moves the list in one frame.
const WHEEL_STEP = 10
// A small scroll down from the top (any offset above 0 is away from the top), far
// less than the height of the new rows.
const SMALL_SCROLL = 4 * WHEEL_STEP

/**
 * Scrolls the list toward `to` px, WHEEL_STEP px a frame, the way a wheel does (one
 * scroll event a frame), until it is there or past it (the list itself may move it
 * past), then watches 30 more frames (about half a second). Resolves with the
 * largest scroll offset seen.
 */
async function wheelTo(scroller: ElementHandle<HTMLElement>, to: number) {
    return scroller.evaluate(
        (list, { target, step }) =>
            new Promise<number>((resolve, reject) => {
                const down = target > list.scrollTop
                const reached = () =>
                    down ? list.scrollTop >= target - 1 : list.scrollTop <= target + 1
                let farthest = list.scrollTop
                let frames = 0
                let settled = 0
                const frame = () => {
                    farthest = Math.max(farthest, list.scrollTop)
                    frames += 1
                    if (frames > 600)
                        return reject(new Error(`the list stopped at ${list.scrollTop}`))
                    if (reached()) {
                        settled += 1
                        if (settled >= 30) return resolve(farthest)
                    } else {
                        const left = Math.abs(target - list.scrollTop)
                        list.scrollTop += (down ? 1 : -1) * Math.min(step, left)
                    }
                    requestAnimationFrame(frame)
                }
                frame()
            }),
        { target: to, step: WHEEL_STEP }
    )
}

async function expectNewRowsShownAtTop(
    p: Page,
    browser: Browser,
    user: { username: string; password: string },
    report: LiveReport
) {
    const oldTop = rowOf(p, `${report.stem} Old ${OLD_SALES - 1}`)
    await expect(oldTop).toBeInViewport()
    const scroller = await listScroller(oldTop)
    await expect.poll(() => scroller.evaluate(list => list.scrollTop)).toBe(0)

    // The reader scrolls down a few rows (the list holds the row in view there), then
    // back to the top.
    const fourthOld = rowOf(p, `${report.stem} Old ${OLD_SALES - 4}`)
    const fewRowsDown = await fourthOld.evaluate(
        (row, list) =>
            list.scrollTop + row.getBoundingClientRect().top - list.getBoundingClientRect().top,
        scroller
    )
    await wheelTo(scroller, fewRowsDown)
    await expect(oldTop).not.toBeInViewport()
    await wheelTo(scroller, 0)
    await expect(oldTop).toBeInViewport()
    await expect.poll(() => scroller.evaluate(list => list.scrollTop), 'back at the top').toBe(0)

    await addNewerElsewhere(browser, user, report)

    // The newest row is the report's first row, and the reader at the top sees it.
    await expect(rowOf(p, `${report.stem} New ${NEW_SALES - 1}`)).toBeInViewport()

    // The reader starts to scroll down to read the new rows: the list moves only as
    // far as the reader scrolls, and the newest rows stay in view.
    expect(
        await wheelTo(scroller, SMALL_SCROLL),
        'the list does not jump down past the new rows'
    ).toBeLessThan(2 * SMALL_SCROLL)
    await expect(rowOf(p, `${report.stem} New ${NEW_SALES - 3}`)).toBeInViewport()
}

test.describe('report-exports: live rows at the top', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a reader at the top of a wide report sees the rows a live run adds', async ({
        freshMember,
        browser,
    }) => {
        const { page: p, user } = freshMember
        const report = await newestFirst(p)
        await expectNewRowsShownAtTop(p, browser, user, report)
    })

    test('a reader at the top of a phone report sees the rows a live run adds', async ({
        freshMember,
        browser,
    }) => {
        const { page: p, user } = freshMember
        const report = await newestFirst(p)
        await p.setViewportSize({ width: 390, height: 844 })
        // The phone list is drawn: its top area (the title) is the list's header.
        await expect
            .poll(async () => {
                const title = await p.getByTestId('report-title').boundingBox()
                const table = await p.getByTestId('report-table').boundingBox()
                return title && table ? title.y >= table.y : false
            })
            .toBe(true)
        await expectNewRowsShownAtTop(p, browser, user, report)
    })
})
