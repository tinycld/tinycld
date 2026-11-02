import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
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

            // Scroll the open group's rows: its row stays stuck under the heading row.
            const heading = await p.getByTestId('report-heading-row').boundingBox()
            await p.getByTestId('report-table').hover()
            await p.mouse.wheel(0, 900)
            const sticky = p.getByTestId('report-sticky-group')
            await expect(sticky).toBeVisible()
            await expect(sticky).toContainText('Open')
            await expect
                .poll(async () =>
                    Math.abs(
                        ((await sticky.boundingBox())?.y ?? 0) -
                            ((heading?.y ?? 0) + (heading?.height ?? 0))
                    )
                )
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
})
