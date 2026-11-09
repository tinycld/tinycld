import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage, skipWithoutFormsStub } from './helpers'

/**
 * A record form with a table built-in, end to end, against the forms-host
 * stub's "Order" host (tests/scripts/scaffold-forms-host-stub.ts): lines with
 * an Item cell the host draws (picking an item sets the rate), Qty, Rate and a
 * computed Amount, and a computed Subtotal. Everything is driven through the
 * UI. The reload at the end proves the rows reached the server, not only the
 * local store; it is not in-app navigation.
 */

async function rowIds(page: Page): Promise<string[]> {
    const ids = await page
        .locator('[data-testid^="form-table-lines-row-"]')
        .evaluateAll(els => els.map(el => el.getAttribute('data-testid') ?? ''))
    return ids.map(id => id.replace('form-table-lines-row-', ''))
}

async function openOrder(page: Page, name: string) {
    await page.getByTestId('forms-stub-order-list').getByText(name, { exact: true }).click()
    await expect(page.getByTestId('form-input-lines')).toBeVisible()
}

test.describe('forms table', () => {
    test.skip(skipWithoutFormsStub(), 'forms-host-stub not scaffolded in this workspace')

    test('adds, fills, reorders by keyboard, removes and saves rows', async ({ page }) => {
        const name = `Order ${Date.now()}`
        await login(page)
        await navigateToPackage(page, 'forms-stub')

        await page.getByTestId('forms-stub-order-new').click()
        await page.getByTestId('form-input-name').fill(name)
        await page.getByTestId('form-table-lines-add').click()
        await page.getByTestId('form-table-lines-add').click()
        const [first, second] = await rowIds(page)

        await page.getByTestId(`forms-stub-item-${first}-widget`).click()
        await page.getByTestId(`form-table-lines-${first}-qty`).fill('2')
        await page.getByTestId(`forms-stub-item-${second}-gadget`).click()
        await page.getByTestId(`form-table-lines-${second}-qty`).fill('1')
        await expect(page.getByTestId(`form-table-lines-${first}-amount`)).toHaveText('10.00')
        await expect(page.getByTestId(`form-table-lines-${second}-amount`)).toHaveText('7.50')
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('17.50')

        // The row menu button is the row's one Tab stop: open it from the keyboard.
        await page.getByTestId(`form-table-lines-menu-${second}`).focus()
        await page.keyboard.press('Enter')
        await page.getByTestId('form-table-lines-menu-up').click()
        await expect.poll(() => rowIds(page)).toEqual([second, first])
        // Focus stays on the moved row's menu button, so a keyboard user can move it again.
        await expect(page.getByTestId(`form-table-lines-menu-${second}`)).toBeFocused()

        await page.getByTestId('forms-stub-order-create').click()
        await openOrder(page, name)
        await expect.poll(() => rowIds(page)).toEqual([second, first])
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('17.50')

        await page.getByTestId(`form-table-lines-menu-${first}`).click()
        await page.getByTestId('form-table-lines-menu-remove').click()
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('7.50')
        await page.getByTestId('forms-stub-order-save').click()
        await expect(page.getByTestId('forms-stub-order-saved')).toBeVisible()

        await page.reload()
        await navigateToPackage(page, 'forms-stub')
        await openOrder(page, name)
        await expect.poll(() => rowIds(page)).toEqual([second])
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('7.50')
        await expect(page.getByTestId(`form-table-lines-${second}-rate`)).toHaveValue('7.5')
    })

    test('a second save right after adding a row updates that row, not a copy', async ({
        page,
    }) => {
        const name = `Order ${Date.now()}`
        await login(page)
        await navigateToPackage(page, 'forms-stub')
        await page.getByTestId('forms-stub-order-new').click()
        await page.getByTestId('form-input-name').fill(name)
        await page.getByTestId('forms-stub-order-create').click()
        await openOrder(page, name)

        await page.getByTestId('form-table-lines-add').click()
        const [row] = await rowIds(page)
        await page.getByTestId(`forms-stub-item-${row}-widget`).click()
        await page.getByTestId(`form-table-lines-${row}-qty`).fill('1')
        await page.getByTestId('forms-stub-order-save').click()
        await expect(page.getByTestId('forms-stub-order-saved')).toBeVisible()

        // The second write must be an update of the row the first one inserted.
        const updated = page.waitForResponse(
            response =>
                response.url().includes(`/api/collections/forms_stub_order_lines/records/${row}`) &&
                response.request().method() === 'PATCH'
        )
        await page.getByTestId(`form-table-lines-${row}-qty`).fill('3')
        await page.getByTestId('forms-stub-order-save').click()
        expect((await updated).ok()).toBe(true)

        await page.reload()
        await navigateToPackage(page, 'forms-stub')
        await openOrder(page, name)
        await expect.poll(() => rowIds(page)).toEqual([row])
        await expect(page.getByTestId(`form-table-lines-${row}-amount`)).toHaveText('15.00')
        await expect(page.getByTestId('form-input-subtotal')).toHaveText('15.00')
    })

    test('a required cell left empty blocks Create and names the column', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'forms-stub')
        await page.getByTestId('forms-stub-order-new').click()
        await page.getByTestId('form-input-name').fill(`Order ${Date.now()}`)
        await page.getByTestId('form-table-lines-add').click()
        await page.getByTestId('forms-stub-order-create').click()
        // Once in the error summary and once under the host-drawn Item cell.
        await expect(page.getByText('Item is required')).toHaveCount(2)
        await expect(page.getByTestId('forms-stub-order-create')).toBeVisible()
    })
})
