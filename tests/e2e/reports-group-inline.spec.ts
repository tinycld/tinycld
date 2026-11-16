import type { Locator, Page } from '@playwright/test'
import { skipWithoutReportsStub, waitForTableFit } from './helpers'
import { expect, shareMember, test } from './member-fixtures'
import {
    addColumn,
    cellOf,
    chooseOption,
    detailLabels,
    listScroller,
    openReportsHub,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    seedStub,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * A group level shown in its place, and a group drawn muted (R1, R2). Sales are
 * grouped by status, then by customer: the Open group has one customer, so the
 * status level's "Show a group's rows in its place where…" (a count of 1) draws
 * no Open row and no "Total Open" row, and Ada stands where Open would. Paid has
 * two customers and is drawn as before. The customer level's "Show a group in
 * muted text where…" (a quantity over 2) mutes Ben. The file's shared member owns
 * the three sales, so the report reads only these rows; each test saves its own
 * report and adds no sale.
 */

shareMember('rptinl', async p => {
    const team = uniqueName('Inline')
    const sales = (
        [
            ['Ada', 'open', 1, 100],
            ['Bea', 'paid', 2, 300],
            ['Ben', 'paid', 3, 200],
        ] as const
    ).map(([customer, status, quantity, amount]) => ({
        team,
        customer,
        amount,
        cost: 0,
        quantity,
        day: '2026-07-05',
        status,
    }))
    await seedStub(p, { teams: [{ name: team }], sales })
})

async function pickIn(p: Page, trigger: string, key: string) {
    await p.getByTestId(trigger).click()
    await p.getByTestId('report-builder-picker').getByTestId(key).click()
}

async function inlineAndMutedReport(p: Page) {
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
    await addColumn(p, ['status'])
    await p.getByTestId('report-builder-section-group').click()
    await p.getByTestId('report-builder-group-add').click()
    await chooseOption(p, 'report-builder-group-0-col', 'status')
    await p.getByTestId('report-builder-group-add').click()
    await chooseOption(p, 'report-builder-group-1-col', 'customer')

    const inline = 'report-builder-group-0-inline'
    await pickIn(p, `${inline}-add-total`, 'report-builder-pick-field-customer')
    await chooseOption(p, `${inline}-0-aggregate`, 'count')
    await chooseOption(p, `${inline}-0-op`, 'eq')
    await p.getByTestId(`${inline}-0-value`).fill('1')

    const muted = 'report-builder-group-1-muted'
    await pickIn(p, `${muted}-add-total`, 'report-builder-pick-field-quantity')
    await chooseOption(p, `${muted}-0-aggregate`, 'sum')
    await chooseOption(p, `${muted}-0-op`, 'gt')
    await p.getByTestId(`${muted}-0-value`).fill('2')

    await saveNewReport(p, uniqueName('Inline groups'))
}

const labelIn = (row: Locator, label: string) => row.getByText(label, { exact: true })

const labelX = async (row: Locator, label: string) =>
    (await labelIn(row, label).boundingBox())?.x ?? Number.NaN

// How far the first label stands right of the second, read in one poll step.
const indentOf = (a: [Locator, string], b: [Locator, string]) => async () =>
    (await labelX(...a)) - (await labelX(...b))

// The color the text is drawn in: the innermost element of the cell or label.
const textColor = (element: Locator) =>
    element.evaluate(el => {
        let leaf: Element = el
        while (leaf.firstElementChild) leaf = leaf.firstElementChild
        return getComputedStyle(leaf).color
    })

test.describe('reports: inline and muted groups', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('an inlined group shows its child in its place, and a muted child is drawn muted', async ({
        p,
    }) => {
        await inlineAndMutedReport(p)
        const ada = rowOf(p, 'Ada')
        const paid = rowOf(p, 'Paid')
        await expect(ada).toBeVisible()
        await expect(paid).toBeVisible()
        // The one-child group draws neither its row nor its total.
        await expect(rowOf(p, 'Open')).toHaveCount(0)
        await expect(rowOf(p, 'Total Open')).toHaveCount(0)
        // Its child stands at the first level's indent.
        await waitForTableFit(p)
        await expect.poll(indentOf([ada, 'Ada'], [paid, 'Paid'])).toBe(0)

        // A press on the inlined group's child opens its detail rows.
        await expect(ada).toHaveAttribute('aria-expanded', 'false')
        await ada.click()
        await expect(ada).toHaveAttribute('aria-expanded', 'true')
        await expect(detailLabels(p)).toHaveCount(1)

        // The drawn group opens to its two rows and its total, and folds again.
        await paid.click()
        await expect(paid).toHaveAttribute('aria-expanded', 'true')
        await expect(rowOf(p, 'Bea')).toBeVisible()
        await expect(rowOf(p, 'Ben')).toBeVisible()
        await expect(rowOf(p, 'Total Paid')).toBeVisible()
        await expect.poll(indentOf([rowOf(p, 'Bea'), 'Bea'], [paid, 'Paid'])).toBeGreaterThan(0)

        // Ben (quantity 3) is muted: its label and figure share one color, which
        // is not the color of Bea's label.
        const benLabel = await textColor(labelIn(rowOf(p, 'Ben'), 'Ben'))
        const benFigure = await textColor(await cellOf(p, 'amount', 'Ben'))
        const beaLabel = await textColor(labelIn(rowOf(p, 'Bea'), 'Bea'))
        expect(benFigure).toBe(benLabel)
        expect(benLabel).not.toBe(beaLabel)

        await paid.click()
        await expect(paid).toHaveAttribute('aria-expanded', 'false')
        await expect(rowOf(p, 'Bea')).toHaveCount(0)
        await expect(rowOf(p, 'Total Paid')).toHaveCount(0)
        await paid.click()
        await expect(paid).toHaveAttribute('aria-expanded', 'true')
        await expect(rowOf(p, 'Total Paid')).toBeVisible()
    })

    test('on a phone, the inlined child shows beside the drawn group and opens its rows', async ({
        p,
    }) => {
        await inlineAndMutedReport(p)
        await p.setViewportSize({ width: 390, height: 844 })
        // The phone layout: the report's title scrolls with the rows, inside the table.
        await expect(p.getByTestId('report-table').getByTestId('report-title')).toBeVisible()
        await waitForTableFit(p)
        const ada = rowOf(p, 'Ada')
        await expect(ada).toBeVisible()
        await expect(rowOf(p, 'Paid')).toBeVisible()
        await expect(rowOf(p, 'Open')).toHaveCount(0)
        // No sideways scroll: neither the list's own scroller nor the page.
        const scroller = await listScroller(ada)
        await expect.poll(() => scroller.evaluate(el => el.scrollWidth - el.clientWidth)).toBe(0)
        expect(await p.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
            390
        )
        await ada.click()
        await expect(ada).toHaveAttribute('aria-expanded', 'true')
        await expect(detailLabels(p)).toHaveCount(1)
    })
})
