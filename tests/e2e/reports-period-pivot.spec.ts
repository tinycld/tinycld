import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub, waitForTableFit } from './helpers'
import {
    addSale,
    addTeam,
    cellOf,
    hubRow,
    openReportStub,
    openReportsHub,
    rowOf,
    uniqueName,
} from './reports-helpers'

/**
 * A percent of a section (P1) on the shipped "Stub period statement", and a month
 * pivot over every month of a period (P2) with its total after the months (P3) and
 * the months in words (P4) on the shipped "Stub period pivot"
 * (tests/scripts/reports-stub/server/register.go.txt). A fresh invited member owns
 * every sale it reads, so the figures are exact.
 */

const STATEMENT = 'Stub period statement'
const PIVOT = 'Stub period pivot'
const MONTHS = ['Jan 2026', 'Feb 2026', 'Mar 2026']

interface Sale {
    customer: string
    amount: number
    day: string
    status?: 'open' | 'paid'
}

async function seed(page: Page, label: string, sales: readonly Sale[]) {
    const invited = await createInvitedUser(page, label)
    const p = invited.inviteePage
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team, 'north')
    for (const sale of sales) await addSale(p, { team, cost: 0, ...sale })
    return { p, close: invited.close }
}

async function openShipped(p: Page, title: string) {
    await openReportsHub(p)
    await p.getByTestId('reports-hub-tab-published').click()
    await hubRow(p, title).click()
}

// The first quarter of 2026 as a Custom period; February has no sale. The
// default period is this fiscal year, which holds no 2026 sale from 2027 on,
// so the wait is for the period control, not for a row.
async function openFirstQuarter(p: Page) {
    await openShipped(p, PIVOT)
    await expect(p.getByTestId('report-param-period')).toBeVisible()
    await p.getByTestId('report-param-period').click()
    await p.getByTestId('report-period-preset-custom').click()
    await p.getByTestId('report-period-from').fill('2026-01-01')
    await p.getByTestId('report-period-to').fill('2026-03-31')
    await expect.poll(() => p.url()).toContain('p-period=2026-01-01..2026-03-31')
    await p.keyboard.press('Escape')
    await expect(p.getByTestId('report-period-popover')).toHaveCount(0)
    await expect(p.getByTestId('report-heading-amount.p3')).toHaveCount(0)
}

const QUARTER_SALES: readonly Sale[] = [
    { customer: 'Ann', amount: 1000, day: '2026-01-10' },
    { customer: 'Bo', amount: 2500, day: '2026-03-10' },
]

async function expectMonthHeadings(p: Page) {
    for (const [i, month] of MONTHS.entries()) {
        await expect(p.getByTestId(`report-heading-amount.p${i}`)).toHaveText(month)
    }
}

test.describe('reports: period pivot', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a percent of a section divides every row by the open sales total', async ({ page }) => {
        const { p, close } = await seed(page, 'rptshare', [
            { customer: 'Ann', amount: 1000, day: '2026-01-10' },
            { customer: 'Bo', amount: 2000, day: '2026-01-11' },
            { customer: 'Cy', amount: 1500, day: '2026-01-12', status: 'paid' },
        ])
        try {
            await openShipped(p, STATEMENT)
            await expect(rowOf(p, 'All sales')).toBeVisible()
            const shares: [string, string][] = [
                ['Ann', '33.3%'],
                ['Bo', '66.7%'],
                ['Total Open sales', '100.0%'],
                ['Cy', '50.0%'],
                ['Total Paid sales', '50.0%'],
                // A computed row divides its own amount: 45.00 of 30.00.
                ['All sales', '150.0%'],
            ]
            for (const [label, share] of shares) {
                await expect(await cellOf(p, 'share', label)).toHaveText(share)
            }
            await expect(p.getByTestId('report-heading-side')).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('a month pivot shows every month of the period, in words, with the total after', async ({
        page,
    }) => {
        const { p, close } = await seed(page, 'rptpd', QUARTER_SALES)
        try {
            await openFirstQuarter(p)
            await expectMonthHeadings(p)
            // February has no sale: a sum reads 0 there.
            await expect(await cellOf(p, 'amount.p1', 'Ann')).toHaveText('0.00')
            await expect(await cellOf(p, 'amount.p1', 'Bo')).toHaveText('0.00')
            await expect(await cellOf(p, 'amount.p0', 'Ann')).toHaveText('10.00')
            await expect(await cellOf(p, 'amount.p2', 'Bo')).toHaveText('25.00')
            await expect(await cellOf(p, 'amount', 'Bo')).toHaveText('25.00')
            // The total column follows the months (pivot.order "spec").
            const march = await p.getByTestId('report-heading-amount.p2').boundingBox()
            const total = await p.getByTestId('report-heading-amount').boundingBox()
            expect(march).not.toBeNull()
            expect(total).not.toBeNull()
            expect(total?.x ?? 0).toBeGreaterThan(march?.x ?? 0)
            expect(await waitForTableFit(p)).toBe('one-line')
        } finally {
            await close()
        }
    })

    test('the month pivot fits a phone with the months in words', async ({ page }) => {
        const { p, close } = await seed(page, 'rptpdphone', QUARTER_SALES)
        try {
            await openFirstQuarter(p)
            await p.setViewportSize({ width: 390, height: 844 })
            // The report has no authored layouts, so core fits one line or
            // its automatic lines (table-lines.ts fitTable).
            const layout = await waitForTableFit(p)
            expect(['one-line', 'auto']).toContain(layout)
            // The FlashList scroller inside the table (ReportTable.tsx) clips its
            // rows sideways, so the table's own scrollWidth cannot see a row wider
            // than the list: measure the table and every scroller in it.
            const overflow = await p.getByTestId('report-table').evaluate(table => {
                const scrollers = [table, ...table.querySelectorAll<HTMLElement>('*')].filter(
                    el =>
                        el === table || ['auto', 'scroll'].includes(getComputedStyle(el).overflowY)
                )
                return Math.max(...scrollers.map(el => el.scrollWidth - el.clientWidth))
            })
            expect(overflow).toBeLessThanOrEqual(0)
            expect(
                await p.evaluate(() => document.documentElement.scrollWidth)
            ).toBeLessThanOrEqual(390)
            await expectMonthHeadings(p)
            await expect(await cellOf(p, 'amount.p1', 'Ann')).toHaveText('0.00')
        } finally {
            await close()
        }
    })
})
