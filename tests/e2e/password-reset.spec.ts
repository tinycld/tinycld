import { expect, test } from '@playwright/test'
import { clearEmailLog, extractFirstLink, waitForEmailTo } from './email-log-helpers'
import { ORG_SLUG, TEST_USER_EMAIL, TEST_USER_PASSWORD } from './helpers'

// Drives the full self-service password-reset flow from the login modal:
// request → email (captured via the mailer LogSender) → confirm screen → sign
// in with the reset password.
//
// The new password is set back to TEST_USER_PASSWORD so the shared test account
// is left exactly as the other specs (which call login()) expect.
test.describe('password reset', () => {
    test('request → emailed link → set new password → sign in', async ({ page }) => {
        clearEmailLog()

        await page.goto('/')

        // Open the inline forgot-password form and request a reset.
        await page.getByTestId('forgot-password-link').click()
        await page.getByTestId('reset-email').fill(TEST_USER_EMAIL)
        await page.getByTestId('reset-submit').click()

        // Generic confirmation is always shown (no account enumeration).
        await expect(page.getByTestId('reset-sent')).toBeVisible({ timeout: 10_000 })

        // The override hook sends via the mailer LogSender with our own link.
        const email = await waitForEmailTo(TEST_USER_EMAIL, {
            subjectMatch: /reset your password/i,
            timeoutMs: 10_000,
        })
        const link = extractFirstLink(email, /https?:\/\/[^\s"'<>]+\/reset-password\/[^\s"'<>]+/)
        const resetPath = new URL(link, 'http://localhost').pathname
        expect(resetPath).toMatch(/\/reset-password\/.+/)

        // Visit the emailed link and set a new password (same value, to leave the
        // shared account unchanged for other specs).
        await page.goto(resetPath)
        await expect(page.getByText('Choose a new password', { exact: true })).toBeVisible({
            timeout: 10_000,
        })
        await page.getByPlaceholder('At least 8 characters').fill(TEST_USER_PASSWORD)
        await page.getByPlaceholder('Re-enter password').fill(TEST_USER_PASSWORD)
        await page.getByTestId('reset-confirm-submit').click()

        // On success the screen redirects to the login route.
        await page.waitForURL(/\/$|\/admin|\/a\//, { timeout: 15_000 })

        // Confirm the reset password actually authenticates.
        await page.goto('/')
        await page.getByTestId('identifier').fill(TEST_USER_EMAIL)
        await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)
        await page.getByText('Sign in', { exact: true }).last().click()
        await page.waitForURL(new RegExp(`/a/${ORG_SLUG}|/a/|/admin`), { timeout: 15_000 })
    })

    test('garbage token shows an invalid/expired error', async ({ page }) => {
        await page.goto('/reset-password/not-a-real-token')
        await page.getByPlaceholder('At least 8 characters').fill('brandnewpassword')
        await page.getByPlaceholder('Re-enter password').fill('brandnewpassword')
        await page.getByTestId('reset-confirm-submit').click()
        await expect(page.getByTestId('reset-error')).toBeVisible({ timeout: 10_000 })
    })
})
