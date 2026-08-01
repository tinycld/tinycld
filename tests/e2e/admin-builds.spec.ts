import { expect, test } from '@playwright/test'
import { login, navigateToAdmin } from './helpers'

// The admin Build History tab, driven as the seeded super-admin. pkg_build now
// grants super-admins read access (the super_admin_rules migration), so the tab
// renders through the pbtsdb store rather than 403'ing as it did when pkg_build
// was superuser-only on every rule.

test.describe('Admin · Build History', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToAdmin(page, 'Build History')
    })

    test('renders the build history surface for a super admin', async ({ page }) => {
        // The key assertion is that the tab MOUNTS (no 403/redirect), which proves
        // the super-admin pkg_build read rule works. A fresh seed has no builds, so
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
