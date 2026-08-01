import { expect, test } from '@playwright/test'
import { login, navigateToAdmin, TEST_USER_EMAIL } from './helpers'

// The admin Super Admins roster, driven as the seeded super-admin. The roster +
// grant/revoke run through the requireAdmin-guarded /api/admin/super-admins
// endpoints (the collection's own rules expose only the caller's row).

test.describe('Admin · Super Admins', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToAdmin(page, 'Super Admins')
    })

    test('lists the seeded super admin', async ({ page }) => {
        // The seed grants the test user super_admins, so their row is present.
        await expect(page.getByTestId(`super-admin-row-${TEST_USER_EMAIL}`)).toBeVisible({
            timeout: 15_000,
        })
    })

    test('grant by email rejects an unknown user', async ({ page }) => {
        await page.getByTestId('super-admin-grant-toggle').click()
        await page.getByTestId('email').fill('nobody-here@example.com')
        await page.getByTestId('super-admin-grant-submit').click()

        // The endpoint 404s an unknown user; the form surfaces the error rather
        // than adding a row.
        await expect(page.getByTestId('super-admin-row-nobody-here@example.com')).toHaveCount(0)
    })
})
