import { expect, test } from '@playwright/test'
import { createInvitedUser, login, skipWithoutReportsStub } from './helpers'
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
    SALES_SOURCE,
    saveNewReport,
    startReport,
    uniqueName,
} from './reports-helpers'

/**
 * Creating a report with the builder and opening it, against the salesstub
 * package (tests/scripts/scaffold-reports-stub.ts). Each test signs in a fresh
 * member made through the invite flow: salesstub_sales lets each user read only
 * their own rows, so the report shows exactly the sales this test added.
 */
test.describe('creating a report', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a new report shows its columns through a to-one link, then opens from the hub', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptcreate')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Acme',
                amount: 150000,
                cost: 90000,
                day: '2026-01-05',
            })
            await addSale(p, {
                team,
                customer: 'Bolt',
                amount: 50000,
                cost: 10000,
                day: '2026-02-10',
                status: 'paid',
            })

            await openReportsHub(p)
            await startReport(p, SALES_SOURCE)
            // A new report starts with the source's first field.
            await expect(p.getByTestId('report-builder-column-0-label')).toHaveValue('Customer')
            await addColumn(p, ['team', 'name'])
            await addColumn(p, ['amount'])
            await chooseOption(p, 'report-builder-column-2-aggregate', 'sum')
            // The preview behind the panel runs the draft.
            await expect(p.getByTestId('report-heading-team_name')).toHaveText('Team › Name')
            await expect(await cellOf(p, 'amount', 'Acme')).toHaveText('1,500.00')
            // Closing a changed draft asks first.
            await p.getByTestId('report-builder-close').click()
            await expect(p.getByText('Discard changes?', { exact: true })).toBeVisible()
            await p.getByText('Keep editing', { exact: true }).click()
            await expect(p.getByTestId('report-builder')).toBeVisible()

            const title = uniqueName('Sales by customer')
            await saveNewReport(p, title)
            await expect(await cellOf(p, 'team_name', 'Bolt')).toHaveText(team)
            // The total row's id carries its currency, so it is found by its label.
            await expect(await cellOf(p, 'amount', 'Total')).toHaveText('$2,000.00')
            await expect(p.getByTestId('report-document-title')).toHaveText(title)

            await p.getByTestId('report-back').click()
            await expect(p.getByTestId('reports-hub')).toBeVisible()
            const row = hubRow(p, title)
            await expect(row).toContainText('Stub sales')
            await expect(row).toContainText('just now')
            await row.click()
            await expect(p.getByTestId('report-title')).toHaveText(title)
        } finally {
            await close()
        }
    })

    test('Edit report changes a saved report, and closing with changes asks first', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptedit')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Cobalt',
                amount: 1000,
                cost: 500,
                day: '2026-03-01',
            })
            await openReportsHub(p)
            // Discard on a changed new report leaves it for the hub, unsaved.
            await startReport(p, SALES_SOURCE)
            await addColumn(p, ['amount'])
            await p.getByTestId('report-builder-close').click()
            await p.getByText('Discard', { exact: true }).click()
            await expect(p.getByTestId('reports-hub')).toBeVisible()
            await expect(p.getByTestId('report-builder')).toBeHidden()

            await startReport(p, SALES_SOURCE)
            const title = uniqueName('Edit me')
            await saveNewReport(p, title)

            await pressToolbar(p, 'report-edit-open', 'Edit report')
            await expect(p.getByTestId('report-builder-title')).toHaveValue(title)
            await p.getByTestId('report-builder-column-0-label').fill('Client')
            await p.getByTestId('report-builder-close').click()
            await expect(p.getByText('Discard changes?', { exact: true })).toBeVisible()
            await p.getByText('Keep editing', { exact: true }).click()
            await expect(p.getByTestId('report-builder')).toBeVisible()
            // Hold every run while saving: the saved view must wait for the new
            // spec's run, never show the old spec's cached run as current.
            let release = () => {}
            const held = new Promise<void>(resolve => {
                release = resolve
            })
            await p.route('**/api/reports/run', async route => {
                await held
                await route.continue()
            })
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder')).toBeHidden()
            await expect(p.getByTestId('report-loading')).toBeVisible()
            await expect(p.getByTestId('report-heading-customer')).toHaveCount(0)
            release()
            await p.unrouteAll({ behavior: 'wait' })
            await expect(p.getByTestId('report-heading-customer')).toHaveText('Client')
        } finally {
            await close()
        }
    })

    test('on a phone the hub shows cards and the builder is a full sheet with a preview', async ({
        page,
    }) => {
        const { inviteePage: p, close } = await createInvitedUser(page, 'rptphone')
        try {
            const team = uniqueName('Team')
            await openReportStub(p)
            await addTeam(p, team)
            await addSale(p, {
                team,
                customer: 'Delta',
                amount: 2500,
                cost: 1000,
                day: '2026-04-01',
            })
            await p.setViewportSize({ width: 390, height: 844 })
            // A phone has no rail: Reports is a row of the More sheet.
            await p.getByTestId('nav-more').click()
            await p.getByTestId('more-reports').click()
            await expect(p.getByTestId('reports-hub')).toBeVisible()
            await startReport(p, SALES_SOURCE)
            await expect(p.getByTestId('report-builder-mode-design')).toBeVisible()
            await p.getByTestId('report-builder-mode-preview').click()
            await expect(
                p.getByTestId('report-table').getByText('Delta', { exact: true })
            ).toBeVisible()
            await p.getByTestId('report-builder-mode-design').click()
            const title = uniqueName('Phone report')
            await saveNewReport(p, title)
            await p.getByTestId('report-back').click()
            await expect(hubRow(p, title)).toContainText('Stub sales')
            // Cards, not rows under a heading.
            await expect(p.getByTestId('reports-hub-heading-row')).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('when the kinds of report cannot load, the hub says so and Retry brings New report back', async ({
        page,
    }) => {
        // A refusal (4xx) is not retried, so the error shows at once.
        await page.route('**/api/reports/sources', route =>
            route.fulfill({
                status: 400,
                contentType: 'application/json',
                body: JSON.stringify({ status: 400, message: 'Refused.', data: {} }),
            })
        )
        await login(page)
        await openReportsHub(page)
        await expect(page.getByTestId('reports-hub-catalog-error-message')).toHaveText(
            'Could not load the kinds of report, so New report is not available'
        )
        await expect(page.getByTestId('reports-hub-new')).toBeDisabled()
        await page.unrouteAll({ behavior: 'wait' })
        await page.getByTestId('reports-hub-catalog-retry').click()
        await expect(page.getByTestId('reports-hub-catalog-error')).toHaveCount(0)
        await expect(page.getByTestId('reports-hub-new')).toBeEnabled()
    })
})
