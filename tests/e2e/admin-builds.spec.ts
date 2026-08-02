import { expect, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from './helpers'

// The Build History settings screen, driven as the seeded owner. pkg_build
// grants owners/admins read access (1970000000_admin_console_role_rules), so the
// screen renders through the pbtsdb store rather than 403'ing as it did when
// pkg_build was superuser-only on every rule.

test.describe('Settings · Build History', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'settings')
        await clickSidebarItem(page, 'Build History')
        await page
            .getByTestId('settings-section-build-history')
            .waitFor({ state: 'visible', timeout: 20_000 })
    })

    test('renders the build history surface for an owner', async ({ page }) => {
        // The key assertion is that the screen MOUNTS (no 403/redirect), which proves
        // the owner/admin pkg_build read rule works. A fresh seed has no builds, so
        // the empty-state copy renders; an existing build shows a row. Either
        // satisfies it — and reaching either means the pkg_build list query
        // resolved rather than 403'ing.
        const emptyState = page.getByText(
            'No builds yet. Installing a package saves its first build.'
        )
        const anyBuildRow = page.locator('[data-testid^="build-row-"]').first()
        await expect(emptyState.or(anyBuildRow)).toBeVisible({ timeout: 20_000 })
    })
})
