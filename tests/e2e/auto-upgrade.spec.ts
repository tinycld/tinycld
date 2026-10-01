import { expect, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from './helpers'

test.describe('automatic updates', () => {
    test('owner turns automatic updates off and on', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'settings')
        await clickSidebarItem(page, 'Packages')
        const toggle = page.getByTestId('autoupgrade-switch')
        await expect(toggle).toBeVisible({ timeout: 20_000 })

        // On by default: the migration seeds the row.
        await expect(toggle).toHaveAttribute('aria-checked', 'true')
        // The e2e server runs with TINYCLD_AUTOUPGRADE_DISABLED=1, so the status
        // names that instead of a next check that would never run.
        const status = page.getByTestId('autoupgrade-status')
        await expect(status).toHaveText('Checks disabled: TINYCLD_AUTOUPGRADE_DISABLED is set')

        await toggle.click()
        await expect(toggle).toHaveAttribute('aria-checked', 'false')

        // The value is stored, not local state: it survives a fresh load of the screen.
        await navigateToPackage(page, 'settings')
        await clickSidebarItem(page, 'Packages')
        await expect(page.getByTestId('autoupgrade-switch')).toHaveAttribute(
            'aria-checked',
            'false'
        )

        await page.getByTestId('autoupgrade-switch').click()
        await expect(page.getByTestId('autoupgrade-switch')).toHaveAttribute('aria-checked', 'true')
    })
})
