import {
    type Browser,
    type ElementHandle,
    expect,
    type Locator,
    type Page,
    test,
} from '@playwright/test'
import { createInvitedUser, loginAs, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    holdRuns,
    listScroller,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    scrollRowTo,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Web: when rows above the view grow, the rows in view stay where they are. A
 * report grouped by the booked day, one group per day, all closed.
 *
 * - Desktop, the list standing still with a group in the middle first in view:
 *   Expand all opens every group above it, and a sale added in another browser
 *   adds a day above it; on every frame that row keeps its place (the run's
 *   "Updating…" note once took a line above the table while the run loaded).
 * - Phone, the list scrolling at wheel speed while Expand all lands: the rows
 *   wrap there, so the opened rows are taller than first estimated. From each
 *   frame to the next the row first in view moves by the scrolled amount only.
 *   The list holds the row first in view at the scroll offset; it once held the
 *   row first in view when the scroll started, and the rows opened between the
 *   two moved the content down.
 */

const GROUPS = 40
const WHEEL_STEP = 8

function dayOf(i: number): string {
    return new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10)
}

async function groupedByDay(p: Page) {
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    for (let i = 0; i < GROUPS; i++) {
        await addSale(p, {
            team,
            customer: `Anchor ${String(i).padStart(2, '0')}`,
            amount: 1000 + i,
            cost: 400,
            quantity: 3,
            day: dayOf(i),
        })
    }
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await addColumn(p, ['cost'])
    await addColumn(p, ['quantity'])
    await addColumn(p, ['booked'])
    await p.getByTestId('report-builder-section-group').click()
    await p.getByTestId('report-builder-group-add').click()
    await chooseOption(p, 'report-builder-group-0-col', 'booked')
    await saveNewReport(p, uniqueName('Anchor'))
    return team
}

// Scrolls the row to the list's top edge and checks it got there (the list can
// scroll only so far, and then the row above would still be in view).
async function toTop(row: Locator) {
    await scrollRowTo(row)
    const list = await listScroller(row)
    const gap = await row.evaluate(
        (element, scroller) =>
            element.getBoundingClientRect().top - scroller.getBoundingClientRect().top,
        list
    )
    expect(Math.abs(gap), 'the row is at the list top').toBeLessThanOrEqual(1)
}

/** Records the row's top edge on every frame until the returned stop is called. */
async function watchTop(p: Page, testId: string): Promise<() => Promise<number[]>> {
    await p.evaluate(id => {
        const tops: number[] = []
        let watching = true
        const frame = () => {
            const element = document.querySelector(`[data-testid="${CSS.escape(id)}"]`)
            if (element) tops.push(element.getBoundingClientRect().top)
            if (watching) requestAnimationFrame(frame)
        }
        requestAnimationFrame(frame)
        Object.assign(window, {
            stopWatchingTop: () => {
                watching = false
                return tops
            },
        })
    }, testId)
    return () =>
        p.evaluate(() => (window as unknown as { stopWatchingTop(): number[] }).stopWatchingTop())
}

// Runs `change`, then checks the row stayed where it was on every frame meanwhile.
async function expectHeldWhile(p: Page, row: Locator, change: () => Promise<void>) {
    const before = (await row.boundingBox())?.y ?? -1
    const stop = await watchTop(p, (await row.getAttribute('data-testid')) ?? '')
    await change()
    await expect
        .poll(async () => Math.abs(((await row.boundingBox())?.y ?? -999) - before))
        .toBeLessThanOrEqual(2)
    const tops = await stop()
    expect(tops.length).toBeGreaterThan(0)
    const moved = Math.max(...tops.map(top => Math.abs(top - before)))
    expect(moved, 'largest move of the first row in view').toBeLessThanOrEqual(2)
}

/**
 * Moves the list 4 px down and then 1 px back and forth on every frame, so the
 * scroll never ends and each move is too small to render new rows; records the
 * row's top edge plus how far the list has moved, which stays where the row
 * started while the rows in view hold. Stops when the returned stop is called.
 */
async function jiggleAndWatch(
    p: Page,
    row: Locator,
    list: ElementHandle<HTMLElement>
): Promise<() => Promise<number[]>> {
    const testId = (await row.getAttribute('data-testid')) ?? ''
    await p.evaluate(
        ({ id, scroller }) => {
            const tops: number[] = []
            let moved = 0
            let frames = 0
            let watching = true
            const frame = () => {
                const step = frames === 0 ? 4 : frames % 2 === 0 ? 1 : -1
                frames += 1
                scroller.scrollTop += step
                moved += step
                requestAnimationFrame(() => {
                    const element = document.querySelector(`[data-testid="${CSS.escape(id)}"]`)
                    if (element) tops.push(element.getBoundingClientRect().top + moved)
                    if (watching) frame()
                })
            }
            frame()
            Object.assign(window, {
                stopJiggle: () => {
                    watching = false
                    return tops
                },
            })
        },
        { id: testId, scroller: list }
    )
    return () => p.evaluate(() => (window as unknown as { stopJiggle(): number[] }).stopJiggle())
}

async function addSaleElsewhere(
    browser: Browser,
    user: { username: string; password: string },
    team: string
) {
    const other = await browser.newContext()
    try {
        const q = await other.newPage()
        await loginAs(q, user.username, user.password)
        await openReportStub(q)
        await addSale(q, { team, customer: 'Earlier', amount: 100, cost: 0, day: '2025-12-31' })
    } finally {
        await other.close()
    }
}

interface Frame {
    /** The list rows in view: test id → top edge. */
    tops: Record<string, number>
    /** The row whose box holds the list's top edge. */
    first: string
    /** How many group rows are open. */
    open: number
}

/**
 * Scrolls the list down `step` px on every frame, the way a wheel does, and
 * records each frame. It runs until groups have been open for 30 frames (or 600
 * frames have passed), so the change lands while the list is moving.
 */
async function scrollAndRecord(p: Page, list: ElementHandle<HTMLElement>, step: number) {
    await p.evaluate(
        ({ scroller, by }) => {
            const frames: Frame[] = []
            let openFor = 0
            const done = new Promise<Frame[]>(resolve => {
                const frame = () => {
                    const box = scroller.getBoundingClientRect()
                    const tops: Record<string, number> = {}
                    let first = ''
                    let firstTop = Number.POSITIVE_INFINITY
                    for (const row of scroller.querySelectorAll('[data-testid^="report-row-"]')) {
                        const r = row.getBoundingClientRect()
                        if (r.bottom <= box.top || r.top >= box.bottom) continue
                        const id = row.getAttribute('data-testid') ?? ''
                        tops[id] = r.top
                        if (r.top <= box.top && r.bottom > box.top && r.top < firstTop) {
                            first = id
                            firstTop = r.top
                        }
                    }
                    const open = scroller.querySelectorAll('[aria-expanded="true"]').length
                    frames.push({ tops, first, open })
                    if (open > 0) openFor += 1
                    if (openFor >= 30 || frames.length >= 600) {
                        resolve(frames)
                        return
                    }
                    scroller.scrollTop += by
                    requestAnimationFrame(frame)
                }
                requestAnimationFrame(frame)
            })
            Object.assign(window, { scrollRecording: done })
        },
        { scroller: list, by: step }
    )
    return {
        frames: () =>
            p.evaluate(
                () => (window as unknown as { scrollRecording: Promise<Frame[]> }).scrollRecording
            ),
    }
}

// The largest amount the row first in view moved beyond the scrolled step from
// one frame to the next. A row's label holds only its first line on a phone, so
// a frame whose top edge falls on a later line has no first row and is skipped.
function largestJump(frames: Frame[], step: number): number {
    let largest = 0
    for (let i = 1; i < frames.length; i++) {
        const { first, tops } = frames[i - 1]
        const now = frames[i].tops[first]
        if (!first || now === undefined) continue
        largest = Math.max(largest, Math.abs(now - (tops[first] - step)))
    }
    return largest
}

test.describe('scroll position on web', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('rows opening or arriving above a still list keep the row in view in place', async ({
        page,
        browser,
    }) => {
        const { inviteePage: p, user, close } = await createInvitedUser(page, 'rptanchor')
        try {
            const team = await groupedByDay(p)
            const target = rowOf(p, dayOf(25))
            await expect(target).toHaveAttribute('aria-expanded', 'false')
            await toTop(target)
            await expectHeldWhile(p, target, async () => {
                await pressToolbar(p, 'report-fold-all', 'Expand all')
                await expect(target).toHaveAttribute('aria-expanded', 'true')
                await expect(rowOf(p, 'Anchor 25')).toBeInViewport()
            })

            // A sale added in another browser makes a new day at the top; the report
            // re-runs live.
            await toTop(target)
            await expectHeldWhile(p, target, async () => {
                const rerun = p.waitForResponse('**/api/reports/run')
                await addSaleElsewhere(browser, user, team)
                await rerun
            })
        } finally {
            await close()
        }
    })

    test('rows opening above a list that is moving keep the row in view in place', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptanchormv')
        try {
            await groupedByDay(p)
            const target = rowOf(p, dayOf(25))
            // The row above the target is still 2 px in view, so it is the first row
            // in view while Expand all runs. A 4 px move then makes the target first,
            // and the list keeps moving a pixel each frame until the groups open.
            await scrollRowTo(target, 2)
            const before = (await target.boundingBox())?.y ?? -1
            const release = await holdRuns(p)
            await pressToolbar(p, 'report-fold-all', 'Expand all')
            await expect(p.getByTestId('report-updating')).toBeVisible()
            const stop = await jiggleAndWatch(p, target, await listScroller(target))
            release()
            await p.unrouteAll({ behavior: 'wait' })
            await expect(target).toHaveAttribute('aria-expanded', 'true')
            await expect(rowOf(p, 'Anchor 25')).toBeInViewport()
            const tops = await stop()
            expect(tops.length).toBeGreaterThan(0)
            const moved = Math.max(...tops.map(top => Math.abs(top - before)))
            expect(moved, 'largest move of the row beyond the list moves').toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })

    test('on a phone, rows opening above while the list scrolls move nothing in view', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptanchorph')
        try {
            await groupedByDay(p)
            await p.setViewportSize({ width: 390, height: 844 })
            // The rows are drawn wrapped (by other code) before the list is used.
            const start = rowOf(p, dayOf(10))
            const startId = ((await start.getAttribute('data-testid')) ?? '').replace(
                'report-row-',
                ''
            )
            const startAmount = p.getByTestId(`report-cell-amount-${startId}`)
            await expect
                .poll(async () => {
                    const label = await start.boundingBox()
                    const amount = await startAmount.boundingBox()
                    return label && amount ? amount.y >= label.y + label.height - 1 : false
                })
                .toBe(true)
            await toTop(start)
            const recording = await scrollAndRecord(p, await listScroller(start), WHEEL_STEP)
            await pressToolbar(p, 'report-fold-all', 'Expand all')
            const frames = await recording.frames()
            const opened = frames.findIndex(frame => frame.open > 0)
            // The groups opened while the list was moving, not before or after.
            expect(opened).toBeGreaterThan(0)
            expect(frames.length - opened).toBeGreaterThanOrEqual(30)
            expect(
                largestJump(frames, WHEEL_STEP),
                'largest jump beyond the scroll'
            ).toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })
})
