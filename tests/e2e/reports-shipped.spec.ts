import { expect, test } from '@playwright/test'
import { login, skipWithoutReportsStub, TEST_USER_USERNAME } from './helpers'
import {
    hubRow,
    openReportsHub,
    pressToolbar,
    SHIPPED_FORMULA,
    SHIPPED_TITLE,
    superuserClient,
    uniqueName,
} from './reports-helpers'

/**
 * A shipped report, against the salesstub package: the stub registers one with
 * reports.RegisterShipped, and the server creates it, published and owned by the
 * first admin, when it starts or when that admin is saved. Its formula uses `if`,
 * which the builder edits (OQ2); a rename saves it unchanged. The seeded owner (an
 * admin) reads it: Delete is not offered for it, and Save as keeps a copy the
 * reader owns. The saved formula is read back with a read-only superuser request;
 * nothing is written outside the UI.
 */
test.describe('a shipped report', () => {
    test.skip(skipWithoutReportsStub(), 'needs the salesstub package')

    test('is published, cannot be deleted, and a copy edits its formula and saves it unchanged', async ({
        page,
    }) => {
        await login(page)
        await openReportsHub(page)
        await page.getByTestId('reports-hub-tab-published').click()
        const row = hubRow(page, SHIPPED_TITLE)
        await expect(row).toHaveCount(1)

        // An admin may delete a published report, but never one a package ships.
        await row.locator('[data-testid^="reports-hub-row-menu-"]').click()
        await expect(page.getByTestId('reports-hub-action-open')).toBeVisible()
        await expect(page.getByTestId('reports-hub-action-duplicate')).toBeVisible()
        await expect(page.getByTestId('reports-hub-action-delete')).toHaveCount(0)
        await page.getByTestId('reports-hub-action-open').click()
        await expect(page.getByTestId('report-title')).toHaveText(SHIPPED_TITLE)

        // Save as keeps a copy the reader owns.
        const title = uniqueName('Shipped copy')
        await pressToolbar(page, 'report-save-as', 'Save as…')
        const saveAs = page.getByRole('textbox', { name: 'Save as', exact: true })
        await expect(saveAs).toHaveValue(`Copy of ${SHIPPED_TITLE}`)
        await saveAs.fill(title)
        await saveAs.press('Enter')
        await expect(page.getByTestId('report-title')).toHaveText(title)

        // A version 1 shipped report runs with no "upgraded" notice (R1, F25): once the
        // run has drawn its table or its empty state, no notice says so.
        await expect(
            page.getByTestId('report-table').or(page.getByTestId('report-empty'))
        ).toBeVisible()
        await expect(page.getByTestId('report-notice-upgraded')).toHaveCount(0)
        // The copy's builder edits the if formula (OQ2): its parts are menus.
        await pressToolbar(page, 'report-edit-open', 'Edit report')
        await expect(page.getByTestId('report-builder')).toBeVisible()
        const formula = 'report-builder-column-3-formula'
        await expect(page.getByTestId(`${formula}-root-part`)).toHaveText('If')
        await expect(page.getByTestId(`${formula}-summary`)).toHaveText(
            '= if Amount > Cost then Amount else Cost'
        )

        // A rename saves the formula exactly as it was.
        const renamed = `${title} renamed`
        await page.getByTestId('report-builder-title').fill(renamed)
        await page.getByTestId('report-builder-save').click()
        await expect(page.getByTestId('report-builder')).toBeHidden()
        await expect(page.getByTestId('report-title')).toHaveText(renamed)

        const pb = await superuserClient()
        const reader = await pb
            .collection('users')
            .getFirstListItem(pb.filter('username = {:name}', { name: TEST_USER_USERNAME }))
        const saved = await pb
            .collection('reports')
            .getFirstListItem(pb.filter('title = {:title}', { title: renamed }))
        expect(saved.owner).toBe(reader.id)
        expect(saved.shipped_key).toBe('')
        expect(saved.spec.columns[3]).toEqual({
            id: 'larger',
            label: 'Larger',
            formula: SHIPPED_FORMULA,
        })
    })
})
