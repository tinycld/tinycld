import { expect, type Page, test } from '@playwright/test'
import { clearEmailLog, extractFirstLink, waitForEmailTo } from './email-log-helpers'
import { appShell, createInvitedUser, type InvitedUser } from './helpers'

// Drives the full self-service password-reset flow from the login modal:
// request → email (captured via the mailer LogSender) → confirm screen → sign
// in with the reset password.
//
// The reset targets a throwaway invited member (not the shared TEST_USER):
// resetting TEST_USER's password opens a window where concurrent login() calls
// in other parallel workers get a real 400 "Failed to authenticate". Resetting a
// per-run invitee leaves the shared fixture untouched, so the file runs in
// parallel with the rest of the suite.
test.describe('password reset', () => {
    test('request → emailed link → set new password → sign in', async ({ page }) => {
        let invited: InvitedUser
        let inviteePage: Page
        let closeInvitee: () => Promise<void>
        {
            const created = await createInvitedUser(page, 'pwreset')
            invited = created.user
            inviteePage = created.inviteePage
            closeInvitee = created.close
        }

        clearEmailLog()
        const NEW_PASSWORD = 'ResetPass9!'

        // Drive the reset from the invitee's own (signed-out) session.
        await inviteePage.evaluate(() => {
            window.localStorage.clear()
            window.sessionStorage.clear()
        })
        await inviteePage.goto('/')

        // Open the inline forgot-password form and request a reset.
        await inviteePage.getByTestId('forgot-password-link').click()
        await inviteePage.getByTestId('reset-email').fill(invited.email)
        await inviteePage.getByTestId('reset-submit').click()

        // Generic confirmation is always shown (no account enumeration).
        await expect(inviteePage.getByTestId('reset-sent')).toBeVisible({ timeout: 10_000 })

        // The override hook sends via the mailer LogSender with our own link.
        const email = await waitForEmailTo(invited.email, {
            subjectMatch: /reset your password/i,
            timeoutMs: 10_000,
        })
        const link = extractFirstLink(email, /https?:\/\/[^\s"'<>]+\/reset-password\/[^\s"'<>]+/)
        const resetPath = new URL(link, 'http://localhost').pathname
        expect(resetPath).toMatch(/\/reset-password\/.+/)

        // Visit the emailed link and set a new password.
        await inviteePage.goto(resetPath)
        await expect(inviteePage.getByText('Choose a new password', { exact: true })).toBeVisible({
            timeout: 10_000,
        })
        await inviteePage.getByPlaceholder('At least 8 characters').fill(NEW_PASSWORD)
        await inviteePage.getByPlaceholder('Re-enter password').fill(NEW_PASSWORD)
        await inviteePage.getByTestId('reset-confirm-submit').click()

        // On success the screen does router.replace('/') — the signed-out root,
        // which renders the login gate. Gate on the form itself: it's what the
        // user sees, and it doesn't depend on how the router spells the route.
        await expect(inviteePage.getByTestId('identifier')).toBeVisible({ timeout: 15_000 })

        // Confirm the reset password actually authenticates.
        await inviteePage.goto('/')
        await inviteePage.getByTestId('identifier').fill(invited.email)
        await inviteePage.getByPlaceholder('Password').fill(NEW_PASSWORD)
        await inviteePage.getByText('Sign in', { exact: true }).last().click()
        await appShell(inviteePage).waitFor({ state: 'visible', timeout: 15_000 })

        await closeInvitee()
    })

    test('garbage token shows an invalid/expired error', async ({ page }) => {
        await page.goto('/a/reset-password/not-a-real-token')
        await page.getByPlaceholder('At least 8 characters').fill('brandnewpassword')
        await page.getByPlaceholder('Re-enter password').fill('brandnewpassword')
        await page.getByTestId('reset-confirm-submit').click()
        await expect(page.getByTestId('reset-error')).toBeVisible({ timeout: 10_000 })
    })
})
