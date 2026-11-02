import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
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
 * Web: when rows above the view change height, the rows in view stay where they
 * are. A report grouped by the booked day, one group per day, all closed; the
 * list is scrolled so a group in the middle is the first row in view. Expand all
 * opens every group above it, and that row keeps its place on screen on every
 * frame: once with the list standing still, once while it is scrolling slowly.
 * Every frame counts: the run's "Updating…" note once took a line above the
 * table while the run loaded, and moved the whole table down for that time.
 */

const GROUPS = 40
const TARGET = 25

function dayOf(i: number): string {
    return new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10)
}

interface Watch {
    /** How far the watch has scrolled the list so far. */
    scrolled(): Promise<number>
    /** Stops it; the row's top edge on each frame, plus what the watch had scrolled by then. */
    stop(): Promise<number[]>
}

interface PageWatch {
    scrolled: number
    stop(): number[]
}

/**
 * Records the row's top edge on every frame. With `scrollBy`, it also scrolls the
 * list down 1 px every third frame, up to that many px, the way a slow wheel or
 * drag does: a browser reports such a scroll late (react-native-web sends the
 * scroll's first event, then none until it ends), so the list's own idea of its
 * offset lags the scroller's.
 */
async function watchRow(p: Page, testId: string, scrollBy = 0): Promise<Watch> {
    await p.evaluate(
        ({ id, limit }) => {
            const row = () => document.querySelector(`[data-testid="${CSS.escape(id)}"]`)
            let scroller = row()?.parentElement ?? null
            while (scroller && getComputedStyle(scroller).overflowY !== 'auto') {
                scroller = scroller.parentElement
            }
            const tops: number[] = []
            let watching = true
            let frames = 0
            const watch: PageWatch = {
                scrolled: 0,
                stop: () => {
                    watching = false
                    return tops
                },
            }
            const frame = () => {
                frames += 1
                if (scroller && watch.scrolled < limit && frames % 3 === 0) {
                    scroller.scrollTop += 1
                    watch.scrolled += 1
                }
                const element = row()
                if (element) tops.push(element.getBoundingClientRect().top + watch.scrolled)
                if (watching) requestAnimationFrame(frame)
            }
            requestAnimationFrame(frame)
            Object.assign(window, { reportRowWatch: watch })
        },
        { id: testId, limit: scrollBy }
    )
    return {
        scrolled: () =>
            p.evaluate(
                () => (window as unknown as { reportRowWatch: PageWatch }).reportRowWatch.scrolled
            ),
        stop: () =>
            p.evaluate(() =>
                (window as unknown as { reportRowWatch: PageWatch }).reportRowWatch.stop()
            ),
    }
}

test.describe('scroll position on web', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('groups opening above the view keep the visible rows in place', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptanchor')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            for (let i = 0; i < GROUPS; i++) {
                await addSale(p, {
                    team,
                    customer: `Anchor ${String(i).padStart(2, '0')}`,
                    amount: 100,
                    cost: 0,
                    day: dayOf(i),
                })
            }
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await addColumn(p, ['booked'])
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'booked')
            await saveNewReport(p, uniqueName('Anchor'))

            const target = rowOf(p, dayOf(TARGET))
            const testId = (await target.getAttribute('data-testid')) ?? ''
            const expandAbove = async (scrollBy: number) => {
                await expect(target).toHaveAttribute('aria-expanded', 'false')
                // The target is the first row in view, at the list's top edge.
                await scrollRowTo(target)
                await expect(rowOf(p, dayOf(0))).not.toBeInViewport()
                const before = (await target.boundingBox())?.y ?? -1
                const watch = await watchRow(p, testId, scrollBy)
                // A scroll in progress has moved a few px before the groups open.
                await expect.poll(watch.scrolled).toBeGreaterThanOrEqual(Math.min(scrollBy, 8))
                await pressToolbar(p, 'report-fold-all', 'Expand all')
                await expect(target).toHaveAttribute('aria-expanded', 'true')
                // The target's own sale shows under it once the opened rows are laid out.
                await expect(rowOf(p, `Anchor ${TARGET}`)).toBeInViewport()
                await expect.poll(watch.scrolled).toBe(scrollBy)
                await expect
                    .poll(async () => {
                        const top = (await target.boundingBox())?.y ?? -999
                        return Math.abs(top + scrollBy - before)
                    })
                    .toBeLessThanOrEqual(2)
                // Not only at the end: no frame drew it anywhere else on the way.
                const tops = await watch.stop()
                expect(tops.length).toBeGreaterThan(0)
                const moved = Math.max(...tops.map(top => Math.abs(top - before)))
                expect(moved, `largest move while scrolling ${scrollBy} px`).toBeLessThanOrEqual(2)
            }

            // The list stands still while the groups above open.
            await expandAbove(0)
            await pressToolbar(p, 'report-fold-all', 'Collapse all')
            // The list is scrolling while they open: its own idea of its offset lags.
            await expandAbove(30)
        } finally {
            await close()
        }
    })
})
