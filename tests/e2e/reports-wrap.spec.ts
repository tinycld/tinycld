import { expect, type Locator, type Page, test } from '@playwright/test'
import { createInvitedUser, login, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    openReportStub,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * E14: on a phone a row too wide for one line wraps (the label on line 1, the
 * figures aligned on line 2 under headings that wrap the same way) and nothing
 * scrolls sideways; an open group's row sticks while its rows scroll under it.
 * Desktop keeps one line per row and the heading row stays at the top. An author
 * arrangement for phones places columns on chosen lines.
 */

async function seed(p: Page, sales: number) {
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    for (let i = 0; i < sales; i++) {
        await addSale(p, {
            team,
            customer: `Wrapco ${String(i).padStart(2, '0')}`,
            amount: 1000 + i,
            cost: 400,
            quantity: 3,
            day: '2026-01-05',
            status: i % 2 === 0 ? 'open' : 'paid',
        })
    }
}

async function wideReport(p: Page, title: string, grouped: boolean) {
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await addColumn(p, ['cost'])
    await addColumn(p, ['quantity'])
    await p.getByTestId('report-builder-section-group').click()
    if (grouped) {
        await addColumn(p, ['status'])
        await p.getByTestId('report-builder-group-add').click()
        await chooseOption(p, 'report-builder-group-0-col', 'status')
        await p.getByTestId('report-builder-group-0-open').click()
    } else {
        // Sorted, so "Wrapco 00" is the first row, the one a scroll takes away.
        await chooseOption(p, 'report-builder-sort-add-menu', 'customer')
    }
    await saveNewReport(p, title)
}

async function boxes(p: Page, row: string) {
    const label = rowOf(p, row)
    const id = (await label.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
    const box = async (testId: string) => p.getByTestId(testId).boundingBox()
    return {
        label: await label.boundingBox(),
        amount: await box(`report-cell-amount-${id}`),
        cost: await box(`report-cell-cost-${id}`),
        headLabel: await box('report-heading-customer'),
        headAmount: await box('report-heading-amount'),
        headCost: await box('report-heading-cost'),
    }
}

async function bottomOf(locator: Locator): Promise<number> {
    const box = await locator.boundingBox()
    return (box?.y ?? 0) + (box?.height ?? 0)
}

// A row the list scrolled up under the heading row, or one it no longer holds.
async function isScrolledAway(row: Locator, headingBottom: number): Promise<boolean> {
    if ((await row.count()) === 0) return true
    return (await bottomOf(row)) <= headingBottom + 1
}

test.describe('report rows per width', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('on a phone a wide row wraps under aligned wrapped headings, with no sideways scroll, and an open group sticks', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptwrap')
        try {
            await seed(p, 24)
            await wideReport(p, uniqueName('Wrap'), true)
            await p.setViewportSize({ width: 390, height: 844 })
            await expect
                .poll(async () => {
                    const b = await boxes(p, 'Wrapco 00')
                    return b.label && b.amount
                        ? b.amount.y >= b.label.y + b.label.height - 1
                        : false
                })
                .toBe(true)
            const b = await boxes(p, 'Wrapco 00')
            expect(Math.abs((b.headAmount?.x ?? 0) - (b.amount?.x ?? -9))).toBeLessThanOrEqual(1)
            expect(Math.abs((b.headCost?.x ?? 0) - (b.cost?.x ?? -9))).toBeLessThanOrEqual(1)
            expect((b.headAmount?.y ?? 0) > (b.headLabel?.y ?? 0)).toBe(true)
            expect(
                await p.evaluate(() => document.documentElement.scrollWidth)
            ).toBeLessThanOrEqual(390)

            // Scroll the open group's rows: its own row goes up under the heading row,
            // and its stuck copy stays just under the heading row.
            const headingBottom = await bottomOf(p.getByTestId('report-heading-row'))
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 900)
            const sticky = p.getByTestId('report-sticky-group')
            await expect(sticky).toContainText('Open')
            await expect.poll(() => isScrolledAway(rowOf(p, 'Open'), headingBottom)).toBe(true)
            await expect
                .poll(async () => Math.abs(((await sticky.boundingBox())?.y ?? 0) - headingBottom))
                .toBeLessThanOrEqual(2)
        } finally {
            await close()
        }
    })

    test('on a desktop a row stays one line and the heading row stays at the top', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptline')
        try {
            await seed(p, 24)
            await wideReport(p, uniqueName('Line'), false)
            await expect
                .poll(async () => {
                    const b = await boxes(p, 'Wrapco 00')
                    return b.label && b.amount ? Math.abs(b.label.y - b.amount.y) : 99
                })
                .toBeLessThanOrEqual(1)
            const before = await p.getByTestId('report-heading-row').boundingBox()
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 900)
            // FlashList keeps a row it scrolled away mounted for reuse, so the row is
            // scrolled out of view rather than gone.
            await expect(rowOf(p, 'Wrapco 00')).not.toBeInViewport()
            await expect
                .poll(async () => (await p.getByTestId('report-heading-row').boundingBox())?.y)
                .toBe(before?.y)
        } finally {
            await close()
        }
    })

    test('an author arrangement for phones puts a cell spanning two lines in the middle of the row', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptarr')
        try {
            await seed(p, 1)
            await wideReport(p, uniqueName('Arrange'), false)
            await p.getByTestId('report-edit-open').click()
            await p.getByTestId('report-builder-section-arrange').click()
            await p.getByTestId('report-builder-arrange-width-mobile').click()
            await p.getByTestId('report-builder-arrange-on').click()
            // A cell that sets no alignment shows the one the table gives it.
            await expect(p.getByTestId('report-builder-arrange-amount-justify')).toContainText(
                'Align end'
            )
            await expect(p.getByTestId('report-builder-arrange-customer-justify')).toContainText(
                'Align start'
            )
            await chooseOption(p, 'report-builder-arrange-tracks', '3')
            // Line 1: customer · quantity · amount; line 2: (empty) · quantity · cost.
            await chooseOption(p, 'report-builder-arrange-customer-span', '1')
            await chooseOption(p, 'report-builder-arrange-amount-line', '1')
            await chooseOption(p, 'report-builder-arrange-amount-start', '3')
            await chooseOption(p, 'report-builder-arrange-cost-line', '2')
            await chooseOption(p, 'report-builder-arrange-cost-start', '3')
            await chooseOption(p, 'report-builder-arrange-quantity-line', '1')
            await chooseOption(p, 'report-builder-arrange-quantity-start', '2')
            await chooseOption(p, 'report-builder-arrange-quantity-lines', '2')
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder')).toBeHidden()
            await p.setViewportSize({ width: 390, height: 844 })
            const id = async () =>
                (await rowOf(p, 'Wrapco 00').getAttribute('data-testid'))?.replace(
                    'report-row-',
                    ''
                ) ?? ''
            await expect
                .poll(async () => {
                    const b = await boxes(p, 'Wrapco 00')
                    const quantity = await p
                        .getByTestId(`report-cell-quantity-${await id()}`)
                        .boundingBox()
                    if (!b.label || !b.amount || !b.cost || !quantity) return false
                    const between =
                        quantity.x > b.label.x && quantity.x + quantity.width <= b.amount.x + 1
                    const spans =
                        quantity.y <= b.amount.y + 1 &&
                        quantity.y + quantity.height >= b.cost.y + b.cost.height - 1
                    return (
                        Math.abs(b.label.y - b.amount.y) <= 1 &&
                        b.cost.y > b.amount.y &&
                        between &&
                        spans
                    )
                })
                .toBe(true)
        } finally {
            await close()
        }
    })

    test('an open group stays stuck only through its own section', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptsect')
        try {
            await seed(p, 16)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'customer')
            // No subtotal rows, so an open group's row keeps its figures.
            await p.getByTestId('report-builder-group-0-subtotals').click()
            await saveNewReport(p, uniqueName('Sections'))
            const first = rowOf(p, 'Wrapco 00')
            await first.click()
            await expect(first).toHaveAttribute('aria-expanded', 'true')

            // A little scroll: the open group's row goes up under the heading row, and
            // its stuck copy, figures included, stays just under the heading row.
            const headingBottom = await bottomOf(p.getByTestId('report-heading-row'))
            const sticky = p.getByTestId('report-sticky-group')
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 30)
            await expect(sticky).toContainText('Wrapco 00')
            await expect(sticky).toContainText('10.00')
            await expect
                .poll(async () => (await first.boundingBox())?.y ?? 0)
                .toBeLessThan(headingBottom)
            await expect
                .poll(async () => Math.abs(((await sticky.boundingBox())?.y ?? 0) - headingBottom))
                .toBeLessThanOrEqual(2)

            // Into the closed groups after it: no open group is above, so nothing sticks.
            await p.mouse.wheel(0, 200)
            await expect(sticky).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('on a desktop a table wider than its view scrolls sideways with its labels held at the left', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptside')
        try {
            await seed(p, 1)
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            for (const keys of [
                ['amount'],
                ['cost'],
                ['quantity'],
                ['currency'],
                ['day'],
                ['booked'],
                ['status'],
                ['team', 'name'],
            ]) {
                await addColumn(p, keys)
            }
            await saveNewReport(p, uniqueName('Sideways'))
            await p.setViewportSize({ width: 1100, height: 720 })
            const label = rowOf(p, 'Wrapco 00')
            const id = (await label.getAttribute('data-testid'))?.replace('report-row-', '') ?? ''
            const x = async (testId: string) => (await p.getByTestId(testId).boundingBox())?.x ?? 0
            const before = {
                label: (await label.boundingBox())?.x ?? 0,
                headLabel: await x('report-heading-customer'),
                headAmount: await x('report-heading-amount'),
            }
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(300, 0)
            await expect
                .poll(async () => before.headAmount - (await x('report-heading-amount')))
                .toBeGreaterThan(100)
            // The heading and the rows move in step; the label column stays put.
            expect(
                Math.abs((await x('report-heading-amount')) - (await x(`report-cell-amount-${id}`)))
            ).toBeLessThanOrEqual(1)
            expect(
                Math.abs((await x('report-heading-customer')) - before.headLabel)
            ).toBeLessThanOrEqual(1)
            expect(
                Math.abs(((await label.boundingBox())?.x ?? 0) - before.label)
            ).toBeLessThanOrEqual(1)
        } finally {
            await close()
        }
    })

    test('an arrangement names its problems in the builder, and a pivot that cannot follow it says so', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptpivn')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            for (const status of ['open', 'paid'] as const) {
                const customer = `Pivco ${status}`
                await addSale(p, {
                    team,
                    customer,
                    amount: 100,
                    cost: 40,
                    day: '2026-01-05',
                    status,
                })
            }
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await addColumn(p, ['cost'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'customer')
            await p.getByTestId('report-builder-section-pivot').click()
            await p.getByTestId('report-builder-pivot-on').click()
            await p.getByTestId('report-builder-pivot-field').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-status')
                .click()
            await chooseOption(p, 'report-builder-pivot-cell', 'amount')

            await p.getByTestId('report-builder-section-arrange').click()
            await p.getByTestId('report-builder-arrange-width-mobile').click()
            await p.getByTestId('report-builder-arrange-on').click()
            await chooseOption(p, 'report-builder-arrange-tracks', '2')
            await chooseOption(p, 'report-builder-arrange-customer-span', '1')
            await chooseOption(p, 'report-builder-arrange-amount-line', '1')
            // On the customer's track: the cell says why, and Save is refused.
            await chooseOption(p, 'report-builder-arrange-cost-line', '1')
            await chooseOption(p, 'report-builder-arrange-cost-start', '1')
            await expect(p.getByTestId('report-builder-arrange-cost-problem')).toHaveText(
                'the cell overlaps another'
            )
            await p.getByTestId('report-builder-title').fill(uniqueName('Pivot arrange'))
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder-arrange-cost-problem')).toBeVisible()
            await expect(p.getByTestId('report-builder')).toBeVisible()

            // Line 2, track 1: the entry is valid, but the second pivot column wraps
            // onto that track.
            await chooseOption(p, 'report-builder-arrange-cost-line', '2')
            await expect(p.getByTestId('report-builder-arrange-cost-problem')).toHaveCount(0)
            await saveNewReport(p, uniqueName('Pivot arrange'))
            await p.setViewportSize({ width: 390, height: 844 })
            await expect(p.getByTestId('report-layout-notice')).toBeVisible()
        } finally {
            await close()
        }
    })
})

test.describe('report rows on a touch phone', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')
    test.use({ hasTouch: true })

    test('the table keeps the browser touch gestures, and a swipe scrolls its rows', async ({
        page,
    }) => {
        await login(page)
        await seed(page, 16)
        await wideReport(page, uniqueName('Touch'), false)
        await page.setViewportSize({ width: 390, height: 844 })
        const first = rowOf(page, 'Wrapco 00')
        await expect(first).toBeVisible()

        // Nothing from a row up to the table turns touch gestures (zoom, pan) off.
        const blocked = await first.evaluate(element => {
            const found: string[] = []
            for (let node: Element | null = element; node; node = node.parentElement) {
                const testId = node.getAttribute('data-testid')
                if (getComputedStyle(node).touchAction === 'none')
                    found.push(testId ?? node.tagName)
                if (testId === 'report-table') break
            }
            return found
        })
        expect(blocked).toEqual([])

        const headingBottom = await bottomOf(page.getByTestId('report-heading-row'))
        const table = await page.getByTestId('report-table').boundingBox()
        const cdp = await page.context().newCDPSession(page)
        await cdp.send('Input.synthesizeScrollGesture', {
            x: (table?.x ?? 0) + 100,
            y: (table?.y ?? 0) + 500,
            yDistance: -400,
            gestureSourceType: 'touch',
        })
        await cdp.detach()
        await expect.poll(() => isScrolledAway(first, headingBottom)).toBe(true)
    })
})
