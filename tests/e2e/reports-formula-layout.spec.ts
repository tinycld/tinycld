import type { Page } from '@playwright/test'
import { skipWithoutReportsStub } from './helpers'
import { expect, shareMember, test } from './member-fixtures'
import {
    addColumn,
    cellOf,
    chooseOption,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    seedStub,
    startReport,
    TEAMS_SOURCE,
    uniqueName,
} from './reports-helpers'

/**
 * Formula columns (the forms engine's formula editor), to-many totals and child
 * rows, and a statement layout, against the salesstub package. The file's shared
 * member owns the two sales the layout tests read; no test adds a sale to them or
 * saves a report. The other tests have a member each, with their own sales; shared
 * teams are scoped by a name stem.
 */

shareMember('rptlayout', async p => {
    const team = uniqueName('Team')
    await seedStub(p, {
        teams: [{ name: team }],
        sales: [
            { team, customer: 'Owed', amount: 3000, cost: 0, day: '2026-01-05' },
            { team, customer: 'Settled', amount: 5000, cost: 0, day: '2026-01-06', status: 'paid' },
        ],
    })
})

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
    test.skip(() => skipWithoutReportsStub(), 'needs the salesstub package')

    test('a formula column subtracts cost from amount', async ({ freshMember }) => {
        const p = freshMember.page
        const team = uniqueName('Team')
        await seedStub(p, {
            teams: [{ name: team }],
            sales: [{ team, customer: 'Acme', amount: 1000, cost: 400, day: '2026-01-05' }],
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
    })

    test('a to-many link adds a total per row or shows child rows', async ({ freshMember }) => {
        const p = freshMember.page
        const stem = uniqueName('M')
        const team = `${stem} team`
        await seedStub(p, {
            teams: [{ name: team }],
            sales: [
                { team, customer: 'First buyer', amount: 100, cost: 0, day: '2026-01-05' },
                { team, customer: 'Second buyer', amount: 200, cost: 0, day: '2026-01-06' },
            ],
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

        // A child row is labelled by its first value, here the customer, and that
        // value is not drawn a second time in its own column.
        await addColumn(p, ['sales', 'customer'], 'nest')
        for (const customer of ['First buyer', 'Second buyer']) {
            await expect(rowOf(p, customer)).toHaveText(customer)
            await expect(await cellOf(p, 'sales_customer', customer)).toBeEmpty()
        }

        // Child rows keep their own order (bind.go sort): the sort menu offers the
        // total per row, never the child rows column.
        await p.getByTestId('report-builder-section-group').click()
        await p.getByTestId('report-builder-sort-add-menu').click()
        await expect(p.getByTestId('report-builder-sort-add-menu-sales_id')).toBeVisible()
        await expect(p.getByTestId('report-builder-sort-add-menu-sales_customer')).toHaveCount(0)
        await p.keyboard.press('Escape')
    })

    test('a layout shows sections by status and a computed row', async ({ p }) => {
        await openReportsHub(p)
        await buildStatusLayout(p)
        await expectStatusLayout(p)

        // A row with a column formula can drop its formula for the other columns;
        // the column formula still fills its column.
        const row = 'report-builder-layout-2'
        await expect(p.getByTestId(`${row}-formula-remove`)).toHaveCount(0)
        await p.getByTestId(`${row}-cell-amount-add`).click()
        await chooseOption(p, `${row}-cell-amount-root-part`, 'field')
        await chooseOption(p, `${row}-cell-amount-root-field`, 'section')
        await expect(await cellOf(p, 'amount', 'Open less paid')).toHaveText('$30.00')
        await p.getByTestId(`${row}-formula-remove`).click()
        await expect(p.getByTestId(`${row}-formula-add`)).toBeVisible()
        await expect(p.getByTestId(`${row}-formula-summary`)).toHaveCount(0)
        await expect(await cellOf(p, 'amount', 'Open less paid')).toHaveText('$30.00')
        await p.getByTestId(`${row}-cell-amount-remove`).click()
        // With neither a formula nor a column formula, Save refuses and says why
        // at the row, and the builder stays open.
        await p.getByTestId('report-builder-title').fill(uniqueName('Layout'))
        await p.getByTestId('report-builder-save').click()
        await expect(p.getByTestId(`${row}-formula-error`)).toHaveText(
            'A computed row needs a formula or a column formula'
        )
        await expect(p.getByTestId('report-builder')).toBeVisible()
        await p.getByTestId(`${row}-formula-add`).click()
        await chooseOption(p, `${row}-formula-root-part`, 'sub')
        await chooseOption(p, `${row}-formula-0-part`, 'field')
        await chooseOption(p, `${row}-formula-0-field`, 'section')
        await chooseOption(p, `${row}-formula-1-part`, 'field')
        await chooseOption(p, `${row}-formula-1-field`, 'section2')
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
    })

    test('on a phone a layout is built in the builder sheet and shows in Preview', async ({
        p,
    }) => {
        await openReportsHub(p)
        await p.setViewportSize({ width: 390, height: 844 })
        await buildStatusLayout(p)
        await p.getByTestId('report-builder-mode-preview').click()
        await expectStatusLayout(p)
    })
})
