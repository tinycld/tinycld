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
    SALES_SOURCE,
    startReport,
    TEAMS_SOURCE,
    uniqueName,
} from './reports-helpers'

/**
 * Formula columns (the forms engine's formula editor), to-many totals and child
 * rows, and a statement layout, against the salesstub package. A fresh invited
 * member owns the sales; shared teams are scoped by a name stem.
 */

async function seedStatusSales(page: Page, label: string) {
    const invited = await createInvitedUser(page, label)
    const p = invited.inviteePage
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSale(p, { team, customer: 'Owed', amount: 3000, cost: 0, day: '2026-01-05' })
    await addSale(p, {
        team,
        customer: 'Settled',
        amount: 5000,
        cost: 0,
        day: '2026-01-06',
        status: 'paid',
    })
    return { p, close: invited.close }
}

// Columns customer, status and a summed amount, grouped by status; then a layout
// of one section per status and a computed row of their difference.
async function buildStatusLayout(p: Page) {
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
    await p.getByTestId('report-builder-layout-add-row').click()
    await p.getByTestId('report-builder-layout-2-label').fill('Open less paid')
    const formula = 'report-builder-layout-2-formula'
    await chooseOption(p, `${formula}-root-part`, 'sub')
    await chooseOption(p, `${formula}-0-part`, 'field')
    await chooseOption(p, `${formula}-0-field`, 'section')
    await chooseOption(p, `${formula}-1-part`, 'field')
    await chooseOption(p, `${formula}-1-field`, 'section2')
    await expect(p.getByTestId(`${formula}-summary`)).toHaveText('= Open sales − Paid sales')
}

async function expectStatusLayout(p: Page) {
    await expect(await cellOf(p, 'amount', 'Total Open sales')).toHaveText('$30.00')
    await expect(await cellOf(p, 'amount', 'Total Paid sales')).toHaveText('$50.00')
    await expect(await cellOf(p, 'amount', 'Open less paid')).toHaveText('-$20.00')
}

test.describe('formulas, related rows and layouts', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a formula column subtracts cost from amount', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptformula')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Acme',
                amount: 1000,
                cost: 400,
                day: '2026-01-05',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await addColumn(p, ['cost'])
            await p.getByTestId('report-builder-add-formula').click()
            await p.getByTestId('report-builder-column-3-label').fill('Profit')
            const formula = 'report-builder-column-3-formula'
            await chooseOption(p, `${formula}-root-part`, 'sub')
            await chooseOption(p, `${formula}-0-part`, 'field')
            await chooseOption(p, `${formula}-0-field`, 'amount')
            await chooseOption(p, `${formula}-1-part`, 'field')
            await chooseOption(p, `${formula}-1-field`, 'cost')
            await expect(p.getByTestId(`${formula}-summary`)).toHaveText('= Amount − Cost')
            await expect(p.getByTestId('report-heading-formula')).toHaveText('Profit')
            await expect(await cellOf(p, 'formula', 'Acme')).toHaveText('6.00')
        } finally {
            await close()
        }
    })

    test('a to-many link adds a total per row or shows child rows', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptmany')
        try {
            const stem = uniqueName('M')
            const team = `${stem} team`
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'First buyer',
                amount: 100,
                cost: 0,
                day: '2026-01-05',
            })
            await addSale(p, {
                team,
                customer: 'Second buyer',
                amount: 200,
                cost: 0,
                day: '2026-01-06',
            })
            await openReportsHub(p)
            await startReport(p, TEAMS_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-name')
                .click()
            await chooseOption(p, 'report-builder-filter-0-op', 'startsWith')
            await p.getByTestId('report-builder-filter-0-value').fill(stem)

            await addColumn(p, ['sales', 'id'], 'total')
            await expect(p.getByTestId('report-builder-column-1-aggregate')).toContainText('Count')
            await expect(await cellOf(p, 'sales_id', team)).toHaveText('2')

            // A child row is labelled by its first value, here the customer.
            await addColumn(p, ['sales', 'customer'], 'nest')
            await expect(await cellOf(p, 'sales_customer', 'First buyer')).toHaveText('First buyer')
            await expect(await cellOf(p, 'sales_customer', 'Second buyer')).toHaveText(
                'Second buyer'
            )
        } finally {
            await close()
        }
    })

    test('a layout shows sections by status and a computed row', async ({ page }) => {
        const { p, close } = await seedStatusSales(page, 'rptlayout')
        try {
            await openReportsHub(p)
            await buildStatusLayout(p)
            await expectStatusLayout(p)

            // The layout's order is the report's order; a computed row can be removed.
            const rows = p.getByTestId('report-table').locator('[data-testid^="report-row-"]')
            await p.getByTestId('report-builder-layout-1-up').click()
            await expect(p.getByTestId('report-builder-layout-0-title')).toHaveValue('Paid sales')
            await expect(rows.first()).toHaveText('Paid sales')
            await expect(await cellOf(p, 'amount', 'Open less paid')).toHaveText('-$20.00')
            await p.getByTestId('report-builder-layout-2-remove').click()
            await expect(p.getByTestId('report-builder-layout-2')).toHaveCount(0)
            await expect(rows.filter({ hasText: 'Open less paid' })).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('on a phone a layout is built in the builder sheet and shows in Preview', async ({
        page,
    }) => {
        const { p, close } = await seedStatusSales(page, 'rptlayoutph')
        try {
            await openReportsHub(p)
            await p.setViewportSize({ width: 390, height: 844 })
            await buildStatusLayout(p)
            await p.getByTestId('report-builder-mode-preview').click()
            await expectStatusLayout(p)
        } finally {
            await close()
        }
    })
})
