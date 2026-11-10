import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub, waitForTableFit } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    hubRow,
    openReportStub,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
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

// The total heading reads after March (pivot.order "spec"): further right on one
// line, or on a later line, or further right on the same line, when the rows wrap.
async function expectTotalAfterMarch(p: Page) {
    const march = p.getByTestId('report-heading-amount.p2')
    const total = p.getByTestId('report-heading-amount')
    await expect(total).toBeVisible()
    await expect
        .poll(async () => {
            const [m, t] = await Promise.all([march.boundingBox(), total.boundingBox()])
            if (!m || !t) return false
            const sameLine = Math.abs(t.y - m.y) < m.height / 2
            return sameLine ? t.x > m.x : t.y > m.y
        })
        .toBe(true)
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
            await expectTotalAfterMarch(p)
            expect(await waitForTableFit(p)).toBe('one-line')
        } finally {
            await close()
        }
    })

    // The builder's own controls for P2 and P3: Columns (every month of a period) and
    // Total after the pivot columns, on a report built from nothing.
    test('the builder shows every month of a period and puts the total after', async ({ page }) => {
        const { p, close } = await seed(page, 'rptpdbuild', QUARTER_SALES)
        try {
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-day')
                .click()
            await chooseOption(p, 'report-builder-filter-0-op', 'between')
            await p.getByTestId('report-builder-filter-0-ask').click()
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'customer')
            await p.getByTestId('report-builder-section-pivot').click()
            await p.getByTestId('report-builder-pivot-on').click()
            await p.getByTestId('report-builder-pivot-field').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-day')
                .click()
            // Columns is offered only once the pivot is bucketed.
            await expect(p.getByTestId('report-builder-pivot-range')).toHaveCount(0)
            await chooseOption(p, 'report-builder-pivot-bucket', 'month')
            await chooseOption(p, 'report-builder-pivot-cell', 'amount')
            await chooseOption(p, 'report-builder-pivot-range', 'day')
            await expect(p.getByTestId('report-builder-pivot-range')).toContainText(
                'Show every month of Day'
            )
            await p.getByTestId('report-builder-pivot-total-after').click()
            await saveNewReport(p, uniqueName('Every month'))

            await p.getByTestId('report-add-filter').click()
            await p.getByTestId('report-add-filter-day').click()
            await p.getByTestId('report-period-preset-custom').click()
            await p.getByTestId('report-period-from').fill('2026-01-01')
            await p.getByTestId('report-period-to').fill('2026-03-31')
            await p.keyboard.press('Escape')
            await expect(p.getByTestId('report-period-popover')).toHaveCount(0)
            await expectMonthHeadings(p)
            await expect(await cellOf(p, 'amount.p1', 'Ann')).toHaveText('0.00')
            await expect(await cellOf(p, 'amount.p2', 'Bo')).toHaveText('25.00')
            await expectTotalAfterMarch(p)
        } finally {
            await close()
        }
    })

    // The builder's section picker on a Percent of a section's total column.
    test('the builder picks the section a percent divides by', async ({ page }) => {
        const { p, close } = await seed(page, 'rptpdshare', [
            { customer: 'Owed', amount: 3000, day: '2026-01-05' },
            { customer: 'Settled', amount: 5000, day: '2026-01-06', status: 'paid' },
        ])
        try {
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['status'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-section-layout').click()
            await p.getByTestId('report-builder-layout-add-section').click()
            await p.getByTestId('report-builder-layout-0-title').fill('Open sales')
            await chooseOption(p, 'report-builder-layout-0-key', 'open')
            await p.getByTestId('report-builder-layout-add-section').click()
            await p.getByTestId('report-builder-layout-1-title').fill('Paid sales')
            await chooseOption(p, 'report-builder-layout-1-key', 'paid')

            await p.getByTestId('report-builder-add-formula').click()
            await p.getByTestId('report-builder-column-3-label').fill('Share')
            const formula = 'report-builder-column-3-formula'
            await chooseOption(p, `${formula}-root-part`, 'percentOfSection')
            // A percent shows each row's own share: it offers no total.
            await p.getByTestId('report-builder-column-3-aggregate').click()
            await expect(p.getByRole('menuitem')).toHaveCount(1)
            await p.keyboard.press('Escape')

            await expect(p.getByTestId(`${formula}-root-section`)).toContainText('Open sales')
            await expect(await cellOf(p, 'formula', 'Total Open sales')).toHaveText('100.0%')
            await chooseOption(p, `${formula}-root-section`, 'section2')
            await expect(p.getByTestId(`${formula}-root-section`)).toContainText('Paid sales')
            await expect(await cellOf(p, 'formula', 'Total Open sales')).toHaveText('60.0%')
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
            await expectTotalAfterMarch(p)
            await expect(await cellOf(p, 'amount.p1', 'Ann')).toHaveText('0.00')
        } finally {
            await close()
        }
    })
})
