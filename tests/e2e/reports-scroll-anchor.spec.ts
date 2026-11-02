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
 * - Phone, the list scrolled at wheel speed until the next row is first in view,
 *   and Expand all landing right after: the rows wrap there, so the opened rows
 *   are taller than first estimated. From each frame to the next the row first in
 *   view moves by the scrolled amount only.
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

// Scrolls the row's top edge to `below` px under the list's top edge and checks it
// got there (the list can scroll only so far, and then the row above would still
// be in view).
async function toTop(row: Locator, below = 0) {
    await scrollRowTo(row, below)
    const list = await listScroller(row)
    const gap = await row.evaluate(
        (element, scroller) =>
            element.getBoundingClientRect().top - scroller.getBoundingClientRect().top,
        list
    )
    expect(Math.abs(gap - below), `the row is ${below} px under the list top`).toBeLessThanOrEqual(
        1
    )
}

/** How tall the list's content is: it grows when rows arrive. */
async function contentHeight(row: Locator): Promise<number> {
    return (await listScroller(row)).evaluate(scroller => scroller.scrollHeight)
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

// Runs `change` and waits until `landed` sees the new rows in the list, then checks
// the row stayed where it was on every frame meanwhile.
async function expectHeldWhile(
    p: Page,
    row: Locator,
    change: () => Promise<void>,
    landed: () => Promise<void>
) {
    const before = (await row.boundingBox())?.y ?? -1
    const stop = await watchTop(p, (await row.getAttribute('data-testid')) ?? '')
    await change()
    await landed()
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
    /** The rows drawn above the list's bottom edge: test id → top edge of its label. */
    tops: Record<string, number>
    /**
     * The row first in view: the last one whose top is at or above the list's top
     * edge (a wrapped row's label holds only its first line, so the edge may fall
     * on a later line of that row).
     */
    first: string
    /** How many group rows are open. */
    open: number
    /** How far the list scrolls after this frame. */
    step: number
    /** How far the list could still scroll down before this frame's step. */
    room: number
}

/**
 * Scrolls the list down `step` px on every frame, the way a wheel does, until the
 * row first in view has scrolled away, and stops there, as a flick that just
 * ended; records each frame, until groups have been open for 30 frames (or 600
 * frames have passed). At the row change it calls `onCrossed` once (the page's
 * `reportRowCrossed`), so a held change lands in the moments after it, while the
 * list still counts the scroll as going on (until 100 ms without a scroll event).
 */
async function scrollAndRecord(
    p: Page,
    list: ElementHandle<HTMLElement>,
    step: number,
    onCrossed: () => void
): Promise<() => Promise<Frame[]>> {
    await p.exposeFunction('reportRowCrossed', onCrossed)
    await p.evaluate(
        ({ scroller, by }) => {
            const frames: Frame[] = []
            let openFor = 0
            let startFirst = ''
            const state = { crossed: false, done: Promise.resolve(frames) }
            const crossed = (window as unknown as { reportRowCrossed(): void }).reportRowCrossed
            state.done = new Promise<Frame[]>(resolve => {
                const frame = () => {
                    const box = scroller.getBoundingClientRect()
                    const tops: Record<string, number> = {}
                    let first = ''
                    let firstTop = Number.NEGATIVE_INFINITY
                    for (const row of scroller.querySelectorAll('[data-testid^="report-row-"]')) {
                        const r = row.getBoundingClientRect()
                        if (r.top >= box.bottom) continue
                        const id = row.getAttribute('data-testid') ?? ''
                        tops[id] = r.top
                        if (r.top <= box.top + 0.5 && r.top > firstTop) {
                            first = id
                            firstTop = r.top
                        }
                    }
                    if (frames.length === 0) startFirst = first
                    if (!state.crossed && first !== startFirst) {
                        state.crossed = true
                        crossed()
                    }
                    const open = scroller.querySelectorAll('[aria-expanded="true"]').length
                    const room = scroller.scrollHeight - scroller.clientHeight - scroller.scrollTop
                    const next = state.crossed ? 0 : by
                    frames.push({ tops, first, open, step: next, room })
                    if (open > 0) openFor += 1
                    if (openFor >= 30 || frames.length >= 600) {
                        resolve(frames)
                        return
                    }
                    scroller.scrollTop += next
                    requestAnimationFrame(frame)
                }
                requestAnimationFrame(frame)
            })
            Object.assign(window, { scrollRecording: state })
        },
        { scroller: list, by: step }
    )
    type Shared = { scrollRecording: { done: Promise<Frame[]> } }
    return () => p.evaluate(() => (window as unknown as Shared).scrollRecording.done)
}

interface Jumps {
    /** The largest amount the row first in view moved beyond the list's own step. */
    largest: number
    /** The frames compared with the frame before them. */
    compared: number[]
    /** Compared frames whose step the list could not take in full (its end). */
    clamped: number[]
}

function jumps(frames: Frame[]): Jumps {
    const result: Jumps = { largest: 0, compared: [], clamped: [] }
    for (let i = 1; i < frames.length; i++) {
        const { first, tops, step, room } = frames[i - 1]
        const now = frames[i].tops[first]
        if (!first || now === undefined) continue
        result.compared.push(i)
        if (room < step) result.clamped.push(i)
        result.largest = Math.max(result.largest, Math.abs(now - (tops[first] - step)))
    }
    return result
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
            await expectHeldWhile(
                p,
                target,
                () => pressToolbar(p, 'report-fold-all', 'Expand all'),
                async () => {
                    await expect(target).toHaveAttribute('aria-expanded', 'true')
                    await expect(rowOf(p, 'Anchor 25')).toBeInViewport()
                }
            )

            // A sale added in another browser makes a new day at the top; the report
            // re-runs live, and the new day's rows make the list taller.
            await toTop(target)
            const height = await contentHeight(target)
            await expectHeldWhile(
                p,
                target,
                async () => {
                    const rerun = p.waitForResponse('**/api/reports/run')
                    await addSaleElsewhere(browser, user, team)
                    await rerun
                },
                () => expect.poll(() => contentHeight(target)).toBeGreaterThan(height)
            )
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
            await toTop(target, 2)
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

    test('on a phone, rows opening above right after a wheel scroll move nothing in view', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptanchorph')
        try {
            await groupedByDay(p)
            // At 844 px high the far end of the rows the list draws ahead crosses a row
            // edge on the same scroll step as the top of the view does; the list then
            // renders and takes the new first row anyway. At 800 px the two happen
            // apart, so the step that changes the first row renders nothing.
            await p.setViewportSize({ width: 390, height: 800 })
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
            // Start so that the wheel steps take the next row exactly 2 px past the list
            // top: the row change is then clear-cut (rows level with the edge are
            // ambiguous), and it happens on a step of its own.
            const next = rowOf(p, dayOf(11))
            const height =
                ((await next.boundingBox())?.y ?? 0) - ((await start.boundingBox())?.y ?? 0)
            expect(height, 'a phone row is taller than a wheel step').toBeGreaterThan(WHEEL_STEP)
            await toTop(start, -((height % WHEEL_STEP) + 2))
            // Expand all is held while the list scrolls; it is let go as the row first
            // in view scrolls away, so it lands just after that.
            const release = await holdRuns(p)
            await pressToolbar(p, 'report-fold-all', 'Expand all')
            await expect(p.getByTestId('report-updating')).toBeVisible()
            const recorded = await scrollAndRecord(
                p,
                await listScroller(start),
                WHEEL_STEP,
                release
            )
            const frames = await recorded()
            await p.unrouteAll({ behavior: 'wait' })
            const opened = frames.findIndex(frame => frame.open > 0)
            // The groups opened while the list was moving, not before or after.
            expect(opened).toBeGreaterThan(0)
            expect(frames.length - opened).toBeGreaterThanOrEqual(30)
            const found = jumps(frames)
            // The frame the groups opened on was compared with the one before it, and
            // no compared step met the end of the list.
            expect(found.compared).toContain(opened)
            expect(found.clamped).toEqual([])
            expect(found.largest, 'largest jump beyond the scroll').toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })
})
