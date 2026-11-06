import { expect, type Page, test } from '@playwright/test'
import { login, openSettingsItem } from './helpers'

// The brand color is set through the real Organization branding screen; no raw
// PocketBase writes. The e2e database is shared, so every test leaves the color
// empty (the default) when it ends.
//
// Persistence check: there is no page reload. After a save the spec opens another Settings
// section (Profile), then opens Branding again, so the Branding section
// unmounts and remounts and reads the stored row afresh.

// Via the user menu, as avatar.spec.ts does: the rail's Settings link can leave the
// previous package's screen mounted underneath. Scope to the "Organization" group.
async function openBranding(page: Page) {
    await page.getByLabel('User menu').click()
    await page.getByRole('menu').getByText('Settings', { exact: true }).click()
    await showBranding(page)
}

async function showBranding(page: Page) {
    const orgGroup = page.getByText('Organization', { exact: true }).locator('..')
    await orgGroup.getByText('Branding', { exact: true }).click()
    await expect(page.getByTestId('brand-color-input')).toBeVisible()
}

async function leaveAndReturn(page: Page) {
    await openSettingsItem(page, 'Profile')
    await expect(page.getByTestId('avatar-preview')).toBeVisible()
    await expect(page.getByTestId('brand-color-input')).toHaveCount(0)
    await openSettingsItem(page, 'Branding')
    await expect(page.getByTestId('brand-color-input')).toBeVisible()
}

async function saveColor(page: Page) {
    await page.getByTestId('brand-color-save').click()
    await expect(page.getByTestId('brand-color-save')).toBeDisabled()
}

test.describe('org-brand-color:', () => {
    test('refuses a color that is not # and six hex digits', async ({ page }) => {
        await login(page)
        await openBranding(page)
        await page.getByTestId('brand-color-input').fill('#12zz45')
        await page.getByTestId('brand-color-save').click()
        await expect(page.getByText('Enter # and six hex digits, like #0d9488')).toBeVisible()
    })

    test('a typed color persists, a swatch fills it, and Use default clears it', async ({
        page,
    }) => {
        await login(page)
        await openBranding(page)
        const input = page.getByTestId('brand-color-input')

        await input.fill('#1e40af')
        await saveColor(page)
        await leaveAndReturn(page)
        await expect(page.getByTestId('brand-color-input')).toHaveValue('#1e40af')

        await page.getByLabel('Red berry', { exact: true }).click()
        await expect(input).toHaveValue('#980000')

        await page.getByLabel('Use default', { exact: true }).click()
        await expect(input).toHaveValue('')
        await saveColor(page)
        await leaveAndReturn(page)
        await expect(page.getByTestId('brand-color-input')).toHaveValue('')
    })
})
