import { expect, type Page, test } from '@playwright/test'
import {
    login,
    signInAsCollaborator,
    skipWithoutReportsStub,
    TEST_COLLABORATOR_NAME,
    TEST_USER_USERNAME,
} from './helpers'
import {
    addColumn,
    addSale,
    addTeam,
    chooseOption,
    hubRow,
    openReportStub,
    openReportsHub,
    pressToolbar,
    SALES_SOURCE,
    saveNewReport,
    startReport,
    superuserClient,
    uniqueName,
} from './reports-helpers'

/**
 * Sharing, publishing and the hub's actions, against the salesstub package. The
 * seeded owner (an admin) shares and publishes; the seeded collaborator (a member)
 * receives. Each adds a sale whose customer is unique to the test, so "sees only
 * their data" is checked by name: salesstub_sales lets each user read only their
 * own rows.
 *
 * One fixture is seeded by a superuser write (the CLAUDE.md exception): a report
 * whose source no longer exists. A report becomes broken when a package is removed
 * or a source is renamed without an alias and the server restarts, which CI cannot
 * do. The test therefore covers the hub's Broken badge and the report's
 * missing-parts state, not the start-up check that writes the mark (Go tests cover
 * that: broken_test.go).
 */
async function ownerReport(page: Page, customer: string) {
    await login(page)
    const team = uniqueName('Team')
    await openReportStub(page)
    await addTeam(page, team)
    await addSale(page, { team, customer, amount: 1000, cost: 0, day: '2026-01-05' })
    await openReportsHub(page)
    await startReport(page, SALES_SOURCE)
    await addColumn(page, ['amount'])
    await chooseOption(page, 'report-builder-column-1-aggregate', 'sum')
    const title = uniqueName('Shared report')
    await saveNewReport(page, title)
    return title
}

async function openShare(page: Page) {
    await pressToolbar(page, 'report-share-open', 'Share')
    await expect(page.getByTestId('report-share-dialog')).toBeVisible()
}

function rowMenu(page: Page, title: string) {
    return hubRow(page, title).first().locator('[data-testid^="reports-hub-row-menu-"]')
}

test.describe('sharing reports', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('a viewer sees only their own rows, cannot edit, saves a copy, and loses a removed share at once', async ({
        page,
    }) => {
        const ownerCustomer = uniqueName('Owner customer')
        const title = await ownerReport(page, ownerCustomer)
        await openShare(page)
        await expect(page.getByTestId('report-share-note')).toHaveText(
            'Each person sees only the data they can access.'
        )
        await page.getByTestId('report-share-search').fill(TEST_COLLABORATOR_NAME.slice(0, 6))
        await page
            .getByTestId('report-share-dialog')
            .locator('[data-testid^="report-share-candidate-"]')
            .filter({ hasText: TEST_COLLABORATOR_NAME })
            .click()
        const shareRow = page
            .getByTestId('report-share-dialog')
            .locator('[data-testid^="report-share-row-"]')
            .filter({ hasText: TEST_COLLABORATOR_NAME })
        await expect(shareRow).toContainText('Viewer')
        await page.getByTestId('report-share-done').click()

        const { page: c, close } = await signInAsCollaborator(page)
        try {
            const collabCustomer = uniqueName('Collab customer')
            const team = uniqueName('Collab team')
            await openReportStub(c)
            await addTeam(c, team)
            await addSale(c, {
                team,
                customer: collabCustomer,
                amount: 500,
                cost: 0,
                day: '2026-01-06',
            })
            await openReportsHub(c)
            await c.getByTestId('reports-hub-tab-shared').click()
            await hubRow(c, title).click()
            const table = c.getByTestId('report-table')
            await expect(table.getByText(collabCustomer, { exact: true })).toBeVisible()
            await expect(table.getByText(ownerCustomer, { exact: true })).toHaveCount(0)
            await expect(c.getByTestId('report-edit-open')).toHaveCount(0)

            // Save as keeps a copy the viewer owns and can edit.
            await pressToolbar(c, 'report-save-as', 'Save as…')
            const saveAs = c.getByRole('textbox', { name: 'Save as', exact: true })
            await expect(saveAs).toHaveValue(`Copy of ${title}`)
            await saveAs.fill(`${title} copy`)
            await saveAs.press('Enter')
            await expect(c.getByTestId('report-title')).toHaveText(`${title} copy`)
            await expect(c.getByTestId('report-edit-open')).toBeVisible()

            // Back goes to the shared report the copy was made from; the owner removes the share.
            await c.getByTestId('report-back').click()
            await expect(c.getByTestId('report-title')).toHaveText(title)
            await expect(c.getByTestId('report-table')).toBeVisible()
            await openShare(page)
            await shareRow.locator('[data-testid^="report-share-row-remove-"]').click()
            await expect(shareRow).toHaveCount(0)
            await expect(c.getByTestId('report-no-access')).toHaveText(
                'You no longer have access to this report'
            )
        } finally {
            await close()
        }
    })

    test('an admin publishes a report to every member, and unpublishes it from the hub', async ({
        page,
    }) => {
        const title = await ownerReport(page, uniqueName('Published customer'))
        await openShare(page)
        await page.getByTestId('report-share-publish').click()
        await page.getByTestId('report-share-done').click()

        const { page: c, close } = await signInAsCollaborator(page)
        try {
            await openReportsHub(c)
            await c.getByTestId('reports-hub-tab-published').click()
            await expect(hubRow(c, title)).toBeVisible()

            await page.getByTestId('report-back').click()
            await rowMenu(page, title).click()
            await page.getByTestId('reports-hub-action-unpublish').click()
            await expect(hubRow(c, title)).toHaveCount(0)
        } finally {
            await close()
        }
    })

    test('Duplicate opens a copy, and Delete removes it after a confirm', async ({ page }) => {
        const title = await ownerReport(page, uniqueName('Copied customer'))
        await page.getByTestId('report-back').click()
        await rowMenu(page, title).click()
        await page.getByTestId('reports-hub-action-duplicate').click()
        await expect(page.getByTestId('report-title')).toHaveText(`Copy of ${title}`)
        await page.getByTestId('report-back').click()
        const copy = hubRow(page, `Copy of ${title}`)
        await rowMenu(page, `Copy of ${title}`).click()
        await page.getByTestId('reports-hub-action-delete').click()
        await page.getByText('Delete', { exact: true }).last().click()
        await expect(copy).toHaveCount(0)
        await expect(hubRow(page, title)).toHaveCount(1)
    })

    test('on a phone the row menu and the share dialog are sheets; Duplicate and Delete work', async ({
        page,
    }) => {
        const title = await ownerReport(page, uniqueName('Phone customer'))
        await page.getByTestId('report-back').click()
        await expect(hubRow(page, title)).toBeVisible()
        await page.setViewportSize({ width: 390, height: 844 })

        // The row menu is a sheet titled with the report; Share opens the dialog as a sheet.
        await rowMenu(page, title).click()
        const share = page.getByTestId('reports-hub-action-share')
        await expect(share).toBeVisible()
        await expect(page.getByTestId('reports-hub-action-publish')).toBeVisible()
        await share.click()
        const dialog = page.getByTestId('report-share-dialog')
        await expect(dialog).toBeVisible()
        await expect(dialog).toContainText(`Share “${title}”`)
        await expect.poll(async () => (await dialog.boundingBox())?.width).toBe(390)
        await expect(page.getByTestId('report-share-note')).toBeVisible()
        await page.getByTestId('report-share-done').click()
        await expect(dialog).toHaveCount(0)

        await rowMenu(page, title).click()
        await page.getByTestId('reports-hub-action-duplicate').click()
        await expect(page.getByTestId('report-title')).toHaveText(`Copy of ${title}`)
        await page.getByTestId('report-back').click()
        const copy = hubRow(page, `Copy of ${title}`)
        await rowMenu(page, `Copy of ${title}`).click()
        await page.getByTestId('reports-hub-action-delete').click()
        await page.getByText('Delete', { exact: true }).last().click()
        await expect(copy).toHaveCount(0)
        await expect(hubRow(page, title)).toHaveCount(1)
    })

    test('a report whose source is gone is marked broken and names what is missing', async ({
        page,
    }) => {
        const pb = await superuserClient()
        const owner = await pb
            .collection('users')
            .getFirstListItem(pb.filter('username = {:name}', { name: TEST_USER_USERNAME }))
        const title = uniqueName('Broken report')
        const seeded = await pb.collection('reports').create({
            owner: owner.id,
            title,
            spec: {
                version: 1,
                source: 'salesstub.gone',
                columns: [{ id: 'name', path: [{ field: 'name' }] }],
            },
            source: 'salesstub.gone',
            broken: {
                status: 422,
                problems: {
                    source: {
                        code: 'unknown_source',
                        message: 'the source "salesstub.gone" does not exist',
                    },
                },
            },
        })
        try {
            await login(page)
            await openReportsHub(page)
            const row = hubRow(page, title)
            await expect(row.locator('[data-testid^="reports-hub-broken-"]')).toHaveText('Broken')
            await row.click()
            await expect(page.getByTestId('report-missing')).toContainText(
                'the source "salesstub.gone" does not exist'
            )
        } finally {
            await pb.collection('reports').delete(seeded.id)
        }
    })

    test('a package link shows only that package’s reports and sources', async ({ page }) => {
        await login(page, { startAt: '/a/reports?package=salesstub' })
        await expect(page.getByTestId('reports-hub-filter')).toContainText('Sales Stub')
        await page.getByTestId('reports-hub-new').click()
        await expect(
            page.getByTestId('reports-new-dialog').locator('[data-testid^="reports-new-source-"]')
        ).toHaveCount(2)
        await page.keyboard.press('Escape')
        await page.getByTestId('reports-hub-show-all').click()
        await expect(page.getByTestId('reports-hub-filter')).toHaveCount(0)
    })
})
