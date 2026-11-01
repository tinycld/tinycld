import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, loginAs, skipWithoutReportsStub } from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    cellOf,
    chooseOption,
    hubRow,
    openReportStub,
    openReportsHub,
    pressToolbar,
    rowOf,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * The report view against the salesstub package: the period chip and filter
 * chips edit the report's params, Customize changes the number format and the
 * columns, a copied link reopens the report as it was, a sale added in another
 * browser shows in the open report, and an expanded group keeps the figures in
 * view while it loads. A fresh invited member owns every sale.
 * Days are in the current calendar year, which is the stub source's fiscal year.
 *
 * The copied-link test opens the link with page.goto after signing in: opening a
 * pasted link is a full page load, which is the behaviour under test, not an
 * in-app move.
 */
const YEAR = new Date().getFullYear()

async function makeReport(page: Page, label: string, askDay: boolean) {
    const invited = await createInvitedUser(page, label)
    const p = invited.inviteePage
    const team = uniqueName('Team')
    await openReportStub(p)
    await addTeam(p, team)
    await addSale(p, { team, customer: 'Alpha', amount: 150000, cost: 10000, day: `${YEAR}-01-10` })
    await addSale(p, { team, customer: 'Beta', amount: 20000, cost: 5000, day: `${YEAR}-02-10` })
    await addSale(p, { team, customer: 'Gamma', amount: 30000, cost: 5000, day: `${YEAR}-03-10` })
    await openReportsHub(p)
    await startReport(p, SALES_SOURCE)
    await addColumn(p, ['amount'])
    await chooseOption(p, 'report-builder-column-1-aggregate', 'sum')
    await addColumn(p, ['cost'])
    if (askDay) {
        await p.getByTestId('report-builder-section-filter').click()
        await p.getByTestId('report-builder-filter-add-condition').click()
        await p
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-day')
            .click()
        await chooseOption(p, 'report-builder-filter-0-op', 'between')
        await p.getByTestId('report-builder-filter-0-ask').click()
        await chooseOption(p, 'report-builder-filter-0-default', 'thisFiscalYear')
        await p.getByTestId('report-builder-filter-add-condition').click()
        await p
            .getByTestId('report-builder-picker')
            .getByTestId('report-builder-pick-field-customer')
            .click()
        await chooseOption(p, 'report-builder-filter-1-op', 'contains')
        await p.getByTestId('report-builder-filter-1-ask').click()
    }
    await saveNewReport(p, uniqueName('View'))
    return invited
}

async function addCustomerFilter(p: Page, text: string) {
    await p.getByTestId('report-add-filter').click()
    await p.getByTestId('report-add-filter-customer').click()
    await p.getByTestId('report-filter-value').fill(text)
    await p.getByTestId('report-filter-apply').click()
    await expect(p.getByTestId('report-param-customer')).toHaveText(`Customer: “${text}”`)
}

test.describe('the report view', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('the period chip and a filter chip edit the run, and a copied link keeps them', async ({
        page,
        browser,
    }) => {
        const { inviteePage: p, user, close } = await makeReport(page, 'rptchips', true)
        try {
            const table = p.getByTestId('report-table')
            await expect(p.getByTestId('report-param-day')).toHaveText(
                new RegExp(`^Day: This fiscal year · Jan 1 – Dec 31, ${YEAR}$`)
            )
            await expect(table.getByText('Gamma', { exact: true })).toBeVisible()

            await p.getByTestId('report-param-day').click()
            await p.getByTestId('report-period-preset-custom').click()
            await p.getByTestId('report-period-from').fill(`${YEAR}-13-01`)
            await expect(p.getByTestId('report-period-error')).toHaveText(
                'Enter a date as YYYY-MM-DD.'
            )
            await p.getByTestId('report-period-from').fill(`${YEAR}-02-01`)
            await expect(p.getByTestId('report-period-error')).toHaveCount(0)
            await p.getByTestId('report-period-to').fill(`${YEAR}-02-28`)
            await expect.poll(() => p.url()).toContain(`p-day=${YEAR}-02-01..${YEAR}-02-28`)
            await p.keyboard.press('Escape')
            await expect(p.getByTestId('report-period-popover')).toHaveCount(0)
            await expect(table.getByText('Beta', { exact: true })).toBeVisible()
            await expect(table.getByText('Alpha', { exact: true })).toHaveCount(0)
            await expect(p.getByTestId('report-document-period')).toHaveText(
                `February 1 – February 28, ${YEAR}`
            )

            await addCustomerFilter(p, 'Gam')
            await expect(p.getByTestId('report-empty-message')).toHaveText(
                `No rows for February 1 – February 28, ${YEAR}`
            )
            await p.getByTestId('report-empty-action').click()
            await expect(p.getByTestId('report-param-customer')).toHaveCount(0)
            await expect(table.getByText('Beta', { exact: true })).toBeVisible()

            // The copied link is the report's full address with this run's options.
            await p.context().grantPermissions(['clipboard-read', 'clipboard-write'])
            await pressToolbar(p, 'report-copy-link', 'Copy link')
            await expect(p.getByText('Link copied', { exact: true })).toBeVisible()
            const link = await p.evaluate(() => navigator.clipboard.readText())
            // On web the server address is the page's own origin.
            const origin = new URL(p.url()).origin
            expect(link.slice(0, origin.length + 1)).toBe(`${origin}/`)
            expect(link).toContain(`/reports/`)
            expect(link).toContain(`p-day=${YEAR}-02-01..${YEAR}-02-28`)
            const other = await browser.newContext()
            try {
                const q = await other.newPage()
                await loginAs(q, user.username, user.password)
                // Opening a pasted link is a full load by nature.
                await q.goto(link)
                await expect(q.getByTestId('report-param-day')).toHaveText(
                    new RegExp(`^Day: Feb 1 – Feb 28, ${YEAR}$`)
                )
                await expect(
                    q.getByTestId('report-table').getByText('Beta', { exact: true })
                ).toBeVisible()
            } finally {
                await other.close()
            }
        } finally {
            await close()
        }
    })

    test('Customize changes the number format and hides a column', async ({ page }) => {
        const { inviteePage: p, close } = await makeReport(page, 'rptcustomize', false)
        try {
            await expect(await cellOf(p, 'amount', 'Alpha')).toHaveText('1,500.00')
            await pressToolbar(p, 'report-customize-open', 'Customize')
            await p.getByTestId('report-option-divide-by-1000').click()
            await expect(await cellOf(p, 'amount', 'Alpha')).toHaveText('1.50')
            await expect(p.getByTestId('report-thousands-note')).toBeVisible()
            await expect.poll(() => p.url()).toContain('div1000=1')
            await p.getByTestId('report-customize-section-columns').click()
            await p.getByTestId('report-option-column-cost').click()
            await expect(p.getByTestId('report-heading-cost')).toHaveCount(0)
            await expect.poll(() => p.url()).toContain('hide=cost')
            await p.getByTestId('report-customize-reset').click()
            await expect(p.getByTestId('report-heading-cost')).toBeVisible()
            await expect(p.getByTestId('report-thousands-note')).toHaveCount(0)
            await expect.poll(() => p.url()).not.toContain('div1000')
            await p.getByTestId('report-customize-close').click()
            await expect(p.getByTestId('report-customize')).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('Reset to defaults keeps the params the reader set', async ({ page }) => {
        const { inviteePage: p, close } = await makeReport(page, 'rptreset', true)
        try {
            await addCustomerFilter(p, 'Gam')
            await pressToolbar(p, 'report-customize-open', 'Customize')
            await p.getByTestId('report-option-without-cents').click()
            await expect.poll(() => p.url()).toContain('nocents=1')
            await p.getByTestId('report-customize-reset').click()
            await expect.poll(() => p.url()).not.toContain('nocents')
            await expect(p.getByTestId('report-param-customer')).toHaveText('Customer: “Gam”')
            await expect(p).toHaveURL(/p-customer=Gam/)
        } finally {
            await close()
        }
    })

    test('several choices apply on Apply, and one choice applies when pressed', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptchoice')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Alpha',
                amount: 100,
                cost: 0,
                day: `${YEAR}-01-10`,
            })
            await addSale(p, {
                team,
                customer: 'Gamma',
                amount: 200,
                cost: 0,
                day: `${YEAR}-01-20`,
                status: 'paid',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            // An asked "is" (one choice, param status) and an asked "is any of"
            // (several choices, param status2).
            await p.getByTestId('report-builder-section-filter').click()
            for (const [i, op] of ['eq', 'in'].entries()) {
                await p.getByTestId('report-builder-filter-add-condition').click()
                await p
                    .getByTestId('report-builder-picker')
                    .getByTestId('report-builder-pick-field-status')
                    .click()
                await chooseOption(p, `report-builder-filter-${i}-op`, op)
                await p.getByTestId(`report-builder-filter-${i}-ask`).click()
            }
            await saveNewReport(p, uniqueName('Choices'))
            const table = p.getByTestId('report-table')
            await expect(table.getByText('Alpha', { exact: true })).toBeVisible()

            await p.getByTestId('report-add-filter').click()
            await p.getByTestId('report-add-filter-status2').click()
            await expect(p.getByTestId('report-filter-apply')).toBeDisabled()
            await p.getByTestId('report-filter-option-open').click()
            await p.getByTestId('report-filter-option-paid').click()
            await p.getByTestId('report-filter-apply').click()
            await expect(p.getByTestId('report-param-status2')).toHaveText('Status: Open, Paid')
            await expect(table.getByText('Alpha', { exact: true })).toBeVisible()
            await expect(table.getByText('Gamma', { exact: true })).toBeVisible()

            await p.getByTestId('report-add-filter').click()
            await p.getByTestId('report-add-filter-status').click()
            await p.getByTestId('report-filter-option-paid').click()
            await expect(p.getByTestId('report-param-status')).toHaveText('Status: Paid')
            await expect(table.getByText('Alpha', { exact: true })).toHaveCount(0)
            await expect(table.getByText('Gamma', { exact: true })).toBeVisible()
        } finally {
            await close()
        }
    })

    test('an asked number starts empty and applies the typed value', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptnumber')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            const sale = { team, amount: 100, cost: 0, day: `${YEAR}-01-10` }
            await addSale(p, { ...sale, customer: 'Few', quantity: 1 })
            await addSale(p, { ...sale, customer: 'Many', quantity: 5 })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await p.getByTestId('report-builder-section-filter').click()
            await p.getByTestId('report-builder-filter-add-condition').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-quantity')
                .click()
            await chooseOption(p, 'report-builder-filter-0-op', 'gte')
            await p.getByTestId('report-builder-filter-0-ask').click()
            await saveNewReport(p, uniqueName('Quantity'))
            const table = p.getByTestId('report-table')
            await expect(table.getByText('Few', { exact: true })).toBeVisible()

            await p.getByTestId('report-add-filter').click()
            await p.getByTestId('report-add-filter-quantity').click()
            const value = p.getByTestId('report-filter-value')
            await expect(value).toHaveValue('')
            await expect(p.getByTestId('report-filter-apply')).toBeDisabled()
            await value.fill('3')
            await p.getByTestId('report-filter-apply').click()
            await expect(p.getByTestId('report-param-quantity')).toHaveText('Quantity: 3')
            await expect(table.getByText('Many', { exact: true })).toBeVisible()
            await expect(table.getByText('Few', { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('a sale added in another browser shows in the open report', async ({ page, browser }) => {
        const { inviteePage: p, user, close } = await makeReport(page, 'rptlive', false)
        const other = await browser.newContext()
        try {
            const q = await other.newPage()
            await loginAs(q, user.username, user.password)
            await openReportStub(q)
            const team = uniqueName('Live team')
            await addTeam(q, team)
            await addSale(q, {
                team,
                customer: 'Live Co',
                amount: 999,
                cost: 0,
                day: `${YEAR}-04-01`,
            })
            await expect(
                p.getByTestId('report-table').getByText('Live Co', { exact: true })
            ).toBeVisible()
        } finally {
            await other.close()
            await close()
        }
    })

    test('an expanded group keeps the figures while it loads, and stays open for the session', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptfold')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Alpha',
                amount: 1000,
                cost: 0,
                day: `${YEAR}-01-10`,
            })
            await addSale(p, {
                team,
                customer: 'Gamma',
                amount: 5000,
                cost: 0,
                day: `${YEAR}-01-20`,
                status: 'paid',
            })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['status'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            const title = uniqueName('Fold')
            await saveNewReport(p, title)
            const table = p.getByTestId('report-table')
            await expect(await cellOf(p, 'amount', 'Open')).toHaveText('10.00')

            // The expanded run is held, so the figures in view while it loads can be read.
            let release = () => {}
            const held = new Promise<void>(resolve => {
                release = resolve
            })
            await p.route('**/api/reports/run', async route => {
                await held
                await route.continue()
            })
            await rowOf(p, 'Open').click()
            await expect(p.getByTestId('report-updating')).toHaveText('Updating…')
            await expect(await cellOf(p, 'amount', 'Open')).toHaveText('10.00')
            await expect(table.getByText('Alpha', { exact: true })).toHaveCount(0)
            release()
            await p.unrouteAll({ behavior: 'wait' })
            await expect(table.getByText('Alpha', { exact: true })).toBeVisible()
            await expect(p.getByTestId('report-updating')).toHaveCount(0)

            await p.getByTestId('report-back').click()
            await hubRow(p, title).click()
            await expect(p.getByTestId('report-title')).toHaveText(title)
            await expect(table.getByText('Alpha', { exact: true })).toBeVisible()
            await expect(p.getByTestId('report-fold-all')).toHaveText('Collapse all')
        } finally {
            await close()
        }
    })

    test('a pivot over its column cap says how many values were left out', async ({ page }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptcap')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, { team, customer: 'One', amount: 100, cost: 0, day: `${YEAR}-01-10` })
            await addSale(p, { team, customer: 'Two', amount: 100, cost: 0, day: `${YEAR}-02-10` })
            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['status'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            await p.getByTestId('report-builder-section-group').click()
            await p.getByTestId('report-builder-group-add').click()
            await chooseOption(p, 'report-builder-group-0-col', 'status')
            await p.getByTestId('report-builder-section-pivot').click()
            await p.getByTestId('report-builder-pivot-on').click()
            await p.getByTestId('report-builder-pivot-field').click()
            await p
                .getByTestId('report-builder-picker')
                .getByTestId('report-builder-pick-field-day')
                .click()
            await chooseOption(p, 'report-builder-pivot-bucket', 'month')
            await chooseOption(p, 'report-builder-pivot-cell', 'amount')
            await p.getByTestId('report-builder-pivot-max').fill('1')
            await expect(p.getByTestId('report-notice-pivotCap')).toHaveText(
                'The pivot shows its first 1 columns; 1 more are left out.'
            )
        } finally {
            await close()
        }
    })

    test('on a phone the chips open sheets and Customize is a sheet', async ({ page }) => {
        const { inviteePage: p, close } = await makeReport(page, 'rptviewphone', true)
        try {
            await p.setViewportSize({ width: 390, height: 844 })
            const table = p.getByTestId('report-table')
            await addCustomerFilter(p, 'Gam')
            await expect(table.getByText('Gamma', { exact: true })).toBeVisible()
            await expect(table.getByText('Alpha', { exact: true })).toHaveCount(0)
            await p.getByTestId('report-filter-remove-customer').click()
            await expect(p.getByTestId('report-param-customer')).toHaveCount(0)
            await expect(table.getByText('Alpha', { exact: true })).toBeVisible()

            await p.getByTestId('report-param-day').click()
            await expect(p.getByText('Day', { exact: true }).last()).toBeVisible()
            await p.getByTestId('report-period-preset-lastFiscalYear').click()
            await expect(p.getByTestId('report-param-day')).toHaveText(
                new RegExp(`^Day: Last fiscal year · Jan 1 – Dec 31, ${YEAR - 1}$`)
            )
            await expect(p.getByTestId('report-empty')).toBeVisible()
            await pressToolbar(p, 'report-customize-open', 'Customize')
            await expect(p.getByTestId('report-customize')).toBeVisible()
            await p.getByTestId('report-option-without-cents').click()
            await p.getByTestId('report-customize-close').click()
            await expect(p.getByTestId('report-customize')).toBeHidden()
            await expect.poll(() => p.url()).toContain('nocents=1')
        } finally {
            await close()
        }
    })
})
