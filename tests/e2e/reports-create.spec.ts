import { expect, test } from '@playwright/test'
import { createInvitedUser, skipWithoutReportsStub } from './helpers'
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
            await p.getByTestId('report-builder-save').click()
            await expect(p.getByTestId('report-builder')).toBeHidden()
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
            await openReportsHub(p)
            await p.setViewportSize({ width: 390, height: 844 })
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
        } finally {
            await close()
        }
    })
})
