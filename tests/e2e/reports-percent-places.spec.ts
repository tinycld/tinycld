import type { Locator, Page } from '@playwright/test'
import { login, skipWithoutReportsStub } from './helpers'
import { expect, test } from './member-fixtures'
import {
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    openReportStub,
    openReportsHub,
    startTeamSales,
    uniqueName,
} from './reports-helpers'

/**
 * A percent column prints the decimal places of its formula (`scale` on the result
 * column): a report built from nothing, grouped by team over the test's own two
 * teams (sales 1.00 and 2.00), with a "Percent of total" column of the amount.
 */

const FORMULA = 'report-builder-column-3-formula'

// A cell shows its whole text: neither the cell nor the text in it is cut.
async function isWhole(cell: Locator): Promise<boolean> {
    return cell.evaluate(el =>
        [el, ...el.querySelectorAll<HTMLElement>('*')].every(
            node => node.scrollWidth <= node.clientWidth
        )
    )
}

async function expectShares(p: Page, labels: string[], shares: string[]) {
    for (const [i, label] of labels.entries()) {
        const cell = await cellOf(p, 'formula', label)
        await expect(cell).toHaveText(shares[i] as string)
        expect(await isWhole(cell)).toBe(true)
    }
}

test.describe('sales-reports: percent places', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a percent of the total prints the places its formula chooses', async ({ page }) => {
        await login(page)
        const stem = uniqueName('Pct')
        const first = `${stem} A`
        const second = `${stem} B`
        await openReportStub(page)
        await addTeam(page, first)
        await addTeam(page, second)
        await addSale(page, {
            team: first,
            customer: 'Ada',
            amount: 100,
            cost: 0,
            day: '2026-07-10',
        })
        await addSale(page, {
            team: second,
            customer: 'Bo',
            amount: 200,
            cost: 0,
            day: '2026-07-11',
        })

        await openReportsHub(page)
        await startTeamSales(page)
        await page.getByTestId('report-builder-section-group').click()
        await page.getByTestId('report-builder-group-add').click()
        await chooseOption(page, 'report-builder-group-0-col', 'team')
        // No subtotal rows: each team is one row with its share.
        await page.getByTestId('report-builder-group-0-subtotals').click()
        await page.getByTestId('report-builder-section-filter').click()
        await page.getByTestId('report-builder-filter-add-condition').click()
        const picker = page.getByTestId('report-builder-picker')
        await picker.getByTestId('report-builder-pick-link-team').click()
        await picker.getByTestId('report-builder-pick-field-name').click()
        await chooseOption(page, 'report-builder-filter-0-op', 'startsWith')
        await page.getByTestId('report-builder-filter-0-value').fill(stem)

        await page.getByTestId('report-builder-add-formula').click()
        await chooseOption(page, `${FORMULA}-root-part`, 'percentOfTotal')
        await chooseOption(page, `${FORMULA}-0-part`, 'field')
        await chooseOption(page, `${FORMULA}-0-field`, 'amount')
        const rows = [first, second, 'Total']

        await chooseOption(page, `${FORMULA}-root-places`, '2')
        await expect(page.getByTestId(`${FORMULA}-root-places`)).toHaveText('2 places')
        await expectShares(page, rows, ['33.33%', '66.67%', '100.00%'])

        await chooseOption(page, `${FORMULA}-root-places`, '0')
        await expectShares(page, rows, ['33%', '67%', '100%'])

        await chooseOption(page, `${FORMULA}-root-places`, '1')
        await expectShares(page, rows, ['33.3%', '66.7%', '100.0%'])
    })
})
