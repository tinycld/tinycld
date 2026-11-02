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
 * Web: when rows above the view grow, the rows in view stay where they are. A
 * report grouped by the booked day, one group per day, all closed; the list
 * stands still with a group in the middle as the first row in view. Expand all
 * opens every group above it, and that row keeps its place on every frame (the
 * run's "Updating…" note once took a line above the table while the run loaded
 * and moved the whole table down for that time).
 *
 * This does not cover a list that is scrolling while the groups land.
 */

const GROUPS = 30
const TARGET = 20

function dayOf(i: number): string {
    return new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10)
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
                    amount: 1000 + i,
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
            await expect(target).toHaveAttribute('aria-expanded', 'false')
            // The target is the first row in view, at the list's top edge.
            await scrollRowTo(target)
            await expect(rowOf(p, dayOf(0))).not.toBeInViewport()
            const before = (await target.boundingBox())?.y ?? -1
            const stop = await watchTop(p, (await target.getAttribute('data-testid')) ?? '')
            await pressToolbar(p, 'report-fold-all', 'Expand all')
            await expect(target).toHaveAttribute('aria-expanded', 'true')
            // The target's own sale shows under it once the opened rows are laid out.
            await expect(rowOf(p, `Anchor ${TARGET}`)).toBeInViewport()
            await expect
                .poll(async () => Math.abs(((await target.boundingBox())?.y ?? -999) - before))
                .toBeLessThanOrEqual(2)
            // Not only at the end: no frame drew it anywhere else on the way.
            const tops = await stop()
            expect(tops.length).toBeGreaterThan(0)
            const moved = Math.max(...tops.map(top => Math.abs(top - before)))
            expect(moved, 'largest move of the first row in view').toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })
})
