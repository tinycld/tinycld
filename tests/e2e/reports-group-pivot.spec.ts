import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
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
 * Group levels with subtotals, folding on the report view, and a pivot, against
 * the salesstub package. A fresh invited member owns every sale the report reads.
 */
async function seedSales(page: Page, label: string) {
    const invited = await createInvitedUser(page, label)
    const p = invited.inviteePage
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSale(p, { team, customer: 'Alpha', amount: 1000, cost: 0, day: '2026-01-10' })
    await addSale(p, { team, customer: 'Beta', amount: 2000, cost: 0, day: '2026-02-10' })
    await addSale(p, {
        team,
        customer: 'Gamma',
        amount: 5000,
        cost: 0,
        day: '2026-01-20',
        status: 'paid',
    })
    return { p, close: invited.close }
}

// Columns customer (the draft's first), status and a summed amount; one group
// level on status.
async function groupByStatus(p: Page) {
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['status'])
    await addColumn(p, ['amount'])
    await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
    await p.getByTestId('report-builder-section-group').click()
    await p.getByTestId('report-builder-group-add').click()
    await chooseOption(p, 'report-builder-group-0-col', 'status')
}

async function pivotByMonth(p: Page) {
    await p.getByTestId('report-builder-section-pivot').click()
    await p.getByTestId('report-builder-pivot-on').click()
    await p.getByTestId('report-builder-pivot-field').click()
    await p
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-field-day')
        .click()
    await chooseOption(p, 'report-builder-pivot-bucket', 'month')
    await chooseOption(p, 'report-builder-pivot-cell', 'amount')
}

async function expectMonthPivot(p: Page) {
    await expect(p.getByTestId('report-heading-amount.p0')).toHaveText('2026-01')
    await expect(p.getByTestId('report-heading-amount.p1')).toHaveText('2026-02')
    await expect(await cellOf(p, 'amount.p0', 'Open')).toHaveText('10.00')
    await expect(await cellOf(p, 'amount.p1', 'Open')).toHaveText('20.00')
    await expect(await cellOf(p, 'amount.p0', 'Paid')).toHaveText('50.00')
}

test.describe('grouping and pivoting a report', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a group level with subtotals opens and closes on the report view', async ({ page }) => {
        const { p, close } = await seedSales(page, 'rptgroup')
        try {
            await openReportsHub(p)
            await groupByStatus(p)
            await saveNewReport(p, uniqueName('By status'))
            const table = p.getByTestId('report-table')
            // A closed group shows its figures.
            await expect(await cellOf(p, 'amount', 'Open')).toHaveText('30.00')
            await expect(p.getByTestId('report-fold-all')).toHaveText('Expand all')

            await rowOf(p, 'Open').click()
            await expect(table.getByText('Alpha', { exact: true })).toBeVisible()
            await expect(await cellOf(p, 'amount', 'Total Open')).toHaveText('$30.00')
            // An open group with a subtotal row shows no figures of its own.
            await expect(await cellOf(p, 'amount', 'Open')).toHaveText('')
            await expect(p.getByTestId('report-fold-all')).toHaveText('Collapse all')

            await p.getByTestId('report-fold-all').click()
            await expect(table.getByText('Alpha', { exact: true })).toHaveCount(0)
            await p.getByTestId('report-fold-all').click()
            await expect(table.getByText('Gamma', { exact: true })).toBeVisible()
        } finally {
            await close()
        }
    })

    test('a sort orders the rows, and a group without subtotals keeps its figures', async ({
        page,
    }) => {
        const { p, close } = await seedSales(page, 'rptsort')
        try {
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['status'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-sort-add').click()
            await chooseOption(p, 'report-builder-sort-0-col', 'amount')
            await p
                .getByTestId('report-builder-sort-0')
                .getByRole('radio', { name: 'Z to A', exact: true })
                .click()
            const rows = p.getByTestId('report-table').locator('[data-testid^="report-row-"]')
            await expect(rows).toHaveText(['Gamma', 'Beta', 'Alpha', 'Total'])

            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-group-0-subtotals').click()
            await rowOf(p, 'Open').click()
            // The sort orders the groups too; an open group's rows follow it.
            await expect(rows).toHaveText(['Paid', 'Open', 'Beta', 'Alpha', 'Total'])
            await expect(await cellOf(p, 'amount', 'Open')).toHaveText('30.00')

            await p.getByTestId('report-builder-sort-0-remove').click()
            await expect(p.getByTestId('report-builder-sort-0')).toHaveCount(0)
            await p.getByTestId('report-builder-group-0-remove').click()
            await expect(rows).toHaveCount(4)
        } finally {
            await close()
        }
    })

    test('a pivot splits the total by month', async ({ page }) => {
        const { p, close } = await seedSales(page, 'rptpivot')
        try {
            await openReportsHub(p)
            await groupByStatus(p)
            await pivotByMonth(p)
            await expectMonthPivot(p)
        } finally {
            await close()
        }
    })

    test('on a phone a group and a pivot are set in the builder sheet and show in Preview', async ({
        page,
    }) => {
        const { p, close } = await seedSales(page, 'rptphonep')
        try {
            await openReportsHub(p)
            await p.setViewportSize({ width: 390, height: 844 })
            await groupByStatus(p)
            await pivotByMonth(p)
            await p.getByTestId('report-builder-mode-preview').click()
            await expectMonthPivot(p)
        } finally {
            await close()
        }
    })
})
