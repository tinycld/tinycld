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
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The report formula leaves that read the report's asked values and the scales
 * the engine fixes: an asked value in a column (typed like any other number, so it
 * is shown as a number and is a pivot cell), whole cents, yes or no, and a layout
 * computed row whose lookup reads an asked value the row itself cannot. A fresh
 * invited member owns the sale; teams are shared, so names are unique.
 */

async function seedSale(page: Page, label: string) {
    const invited = await createInvitedUser(page, label)
    const p = invited.inviteePage
    const team = uniqueName('Leaf team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSale(p, {
        team,
        customer: 'Leafy',
        amount: 1500,
        cost: 0,
        quantity: 3,
        day: '2026-01-05',
    })
    await openReportsHub(p)
    return { p, close: invited.close }
}

// Ask when run on a quantity condition makes the number param `quantity` (Quantity).
async function askQuantity(p: Page) {
    await p.getByTestId('report-builder-section-filter').click()
    await p.getByTestId('report-builder-filter-add-condition').click()
    await p
        .getByTestId('report-builder-picker')
        .getByTestId('report-builder-pick-field-quantity')
        .click()
    await chooseOption(p, 'report-builder-filter-0-op', 'gte')
    await p.getByTestId('report-builder-filter-0-ask').click()
}

test.describe('report formula leaves', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an asked value, whole cents and yes or no in columns', async ({ page }) => {
        const { p, close } = await seedSale(page, 'rptleaves')
        try {
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await askQuantity(p)

            // An asked number is a number column: Show as, and a pivot cell once totalled.
            await p.getByTestId('report-builder-add-formula').click()
            const asked = 'report-builder-column-2-formula'
            await chooseOption(p, `${asked}-root-part`, 'param')
            await expect(p.getByTestId(`${asked}-root-param`)).toHaveText('Quantity')
            await expect(p.getByTestId(`${asked}-summary`)).toHaveText('= {Quantity}')
            await expect(p.getByTestId('report-builder-column-2-format')).toBeVisible()
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-pivot').click()
            await p.getByTestId('report-builder-pivot-on').click()
            await chooseOption(p, 'report-builder-pivot-cell', 'formula')
            await expect(p.getByTestId('report-builder-pivot-cell')).toContainText('Formula')
            await p.getByTestId('report-builder-pivot-on').click()

            // An amount divided by a number keeps whole cents: no places to pick.
            await p.getByTestId('report-builder-add-formula').click()
            const f = 'report-builder-column-3-formula'
            await chooseOption(p, `${f}-root-part`, 'if')
            await chooseOption(p, `${f}-0-yesno`, 'false')
            await chooseOption(p, `${f}-1-part`, 'div')
            await chooseOption(p, `${f}-1-0-part`, 'field')
            await chooseOption(p, `${f}-1-0-field`, 'amount')
            await p.getByTestId(`${f}-1-1-number`).fill('2')
            await expect(p.getByTestId(`${f}-1-cents`)).toHaveText('Whole cents')
            await expect(p.getByTestId(`${f}-1-places`)).toHaveCount(0)
            await expect(p.getByTestId(`${f}-summary`)).toHaveText(
                '= if no then Amount ÷ 2 else empty'
            )
            await expect(await cellOf(p, 'formula2', 'Leafy')).toBeEmpty()
            await chooseOption(p, `${f}-0-yesno`, 'true')
            await expect(p.getByTestId(`${f}-0-yesno`)).toHaveText('Yes')
            await expect(await cellOf(p, 'formula2', 'Leafy')).toHaveText('7.50')
        } finally {
            await close()
        }
    })

    test('a layout computed row looks a value up by an asked value', async ({ page }) => {
        const { p, close } = await seedSale(page, 'rptlayoutask')
        try {
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['status'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await askQuantity(p)
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')

            await p.getByTestId('report-builder-section-layout').click()
            await p.getByTestId('report-builder-layout-add-section').click()
            await p.getByTestId('report-builder-layout-0-title').fill('Open sales')
            await chooseOption(p, 'report-builder-layout-0-key', 'open')
            await p.getByTestId('report-builder-layout-add-row').click()
            await p.getByTestId('report-builder-layout-1-label').fill('Looked up')
            const f = 'report-builder-layout-1-formula'

            // The row reads no asked value itself; a lookup inside it does.
            await p.getByTestId(`${f}-root-part`).click()
            await expect(p.getByTestId(`${f}-root-part-lookup`)).toBeVisible()
            await expect(p.getByTestId(`${f}-root-part-param`)).toHaveCount(0)
            await p.getByTestId(`${f}-root-part-lookup`).click()
            await chooseOption(p, `${f}-root-source`, SALES_SOURCE)
            await chooseOption(p, `${f}-0-part`, 'eq')
            await chooseOption(p, `${f}-0-0-part`, 'path')
            await p.getByTestId(`${f}-0-0-path`).click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-quantity')
                .click()
            await chooseOption(p, `${f}-0-1-part`, 'param')
            await expect(p.getByTestId(`${f}-0-1-param`)).toHaveText('Quantity')
            await chooseOption(p, `${f}-1-part`, 'path')
            await p.getByTestId(`${f}-1-path`).click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-amount')
                .click()
            await expect(p.getByTestId(`${f}-summary`)).toHaveText(
                '= look up Amount in Stub sales where Quantity = {Quantity} (smallest)'
            )
            // The engine runs it: the computed row is drawn below its section.
            await expect(await cellOf(p, 'amount', 'Total Open sales')).toHaveText('$15.00')
            await expect(rowOf(p, 'Looked up')).toBeVisible()
        } finally {
            await close()
        }
    })
})
