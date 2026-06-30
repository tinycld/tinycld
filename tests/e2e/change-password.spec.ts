import { expect, test } from '@playwright/test'
import { login, ORG_SLUG, TEST_USER_PASSWORD } from './helpers'

// The only authenticated user in the e2e fixture is the shared TEST_USER, and
// signup is disabled so we can't provision a throwaway account. A real password
// change therefore has to use that shared user — so each test that mutates the
// password restores it before finishing, and the suite runs serially to avoid a
// half-changed password leaking across tests in this file. (Cross-file workers
// could still race, but the change→restore window is a single round-trip and no
// other spec depends on this user's password mid-flight.)
test.describe
    .serial('change password', () => {
        const NEW_PASSWORD = 'ChangedPw5678!'

        // Open Settings → Personal via the user menu (SPA nav, not page.goto).
        async function openPersonalSettings(page: import('@playwright/test').Page) {
            await page.getByLabel('User menu').click()
            await page.getByText('Settings', { exact: true }).click()
            await page.waitForURL(new RegExp(`/a/${ORG_SLUG}/settings/personal`))
        }

        async function fillPasswordForm(
            page: import('@playwright/test').Page,
            current: string,
            next: string,
            confirm: string
        ) {
            await page.getByText('Change password').click()
            await page.getByTestId('oldPassword').fill(current)
            await page.getByTestId('password').fill(next)
            await page.getByTestId('passwordConfirm').fill(confirm)
        }

        test('validates the form before submitting', async ({ page }) => {
            await login(page)
            await openPersonalSettings(page)

            // Mismatched confirmation is caught client-side (deterministic copy).
            await fillPasswordForm(page, TEST_USER_PASSWORD, NEW_PASSWORD, 'Mismatch9999!')
            await page.getByText('Save', { exact: true }).click()
            await expect(page.getByText('Passwords do not match', { exact: true })).toBeVisible()

            // The form stays open and nothing changed: a later login with the
            // original password (in the next test) still works.
            await page.getByText('Cancel', { exact: true }).click()
        })

        test('changes the password, then restores it', async ({ page }) => {
            await login(page)
            await openPersonalSettings(page)

            await fillPasswordForm(page, TEST_USER_PASSWORD, NEW_PASSWORD, NEW_PASSWORD)
            await page.getByText('Save', { exact: true }).click()

            // Success toast confirms the server accepted the change, and the user
            // stays signed in (no redirect to the connect/login screen).
            await expect(page.getByText('Password changed')).toBeVisible()
            await expect(page).toHaveURL(new RegExp(`/a/${ORG_SLUG}/settings/personal`))

            // Let the toast auto-dismiss so the restore assertion can't match this
            // stale one (default toast duration is 4s).
            await expect(page.getByText('Password changed')).toBeHidden({ timeout: 10_000 })

            // Restore the shared fixture password so other specs keep working. The
            // helper re-authed the session with NEW_PASSWORD, so that's the current
            // password now.
            await openPersonalSettings(page)
            await fillPasswordForm(page, NEW_PASSWORD, TEST_USER_PASSWORD, TEST_USER_PASSWORD)
            await page.getByText('Save', { exact: true }).click()
            await expect(page.getByText('Password changed')).toBeVisible()
        })
    })
