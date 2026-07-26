import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, type InvitedUser, LANDED_URL } from './helpers'

// Password changes need a real, mutable account. Rather than mutate the shared
// TEST_USER (whose password every other spec's login() depends on — a mutation
// there races cross-file workers into 400 "Failed to authenticate"), we
// provision a throwaway invited member per run and change ITS password. The
// account is discarded afterward, so there's nothing to restore and no shared
// state to leak. This lets the file run in parallel with the rest of the suite.
test.describe('change password', () => {
    const NEW_PASSWORD = 'ChangedPw5678!'

    let invited: InvitedUser
    let inviteePage: Page
    let closeInvitee: () => Promise<void>

    test.beforeEach(async ({ page }) => {
        const created = await createInvitedUser(page, 'pwchange')
        invited = created.user
        inviteePage = created.inviteePage
        closeInvitee = created.close
    })

    test.afterEach(async () => {
        await closeInvitee()
    })

    // Open Settings → Personal via the user menu (SPA nav, not page.goto).
    async function openPersonalSettings(page: Page) {
        await page.getByLabel('User menu').click()
        await page.getByText('Settings', { exact: true }).click()
        await page.waitForURL(/\/settings\/personal/)
    }

    async function fillPasswordForm(page: Page, current: string, next: string, confirm: string) {
        await page.getByText('Change password').click()
        await page.getByTestId('oldPassword').fill(current)
        await page.getByTestId('password').fill(next)
        await page.getByTestId('passwordConfirm').fill(confirm)
    }

    test('validates the form before submitting', async () => {
        await openPersonalSettings(inviteePage)

        // Mismatched confirmation is caught client-side (deterministic copy).
        await fillPasswordForm(inviteePage, invited.password, NEW_PASSWORD, 'Mismatch9999!')
        await inviteePage.getByText('Save', { exact: true }).click()
        await expect(inviteePage.getByText('Passwords do not match', { exact: true })).toBeVisible()

        await inviteePage.getByText('Cancel', { exact: true }).click()
    })

    test('changes the password, then signs in with it', async () => {
        await openPersonalSettings(inviteePage)

        await fillPasswordForm(inviteePage, invited.password, NEW_PASSWORD, NEW_PASSWORD)
        await inviteePage.getByText('Save', { exact: true }).click()

        // Success toast confirms the server accepted the change, and the user
        // stays signed in (no redirect to the connect/login screen).
        await expect(inviteePage.getByText('Password changed')).toBeVisible()
        await expect(inviteePage).toHaveURL(/\/settings\/personal/)

        // The new password actually authenticates: sign out and back in with it.
        await inviteePage.evaluate(() => {
            window.localStorage.clear()
            window.sessionStorage.clear()
        })
        await inviteePage.goto('/')
        await inviteePage.getByTestId('identifier').fill(invited.username)
        await inviteePage.getByPlaceholder('Password').fill(NEW_PASSWORD)
        await inviteePage.getByText('Sign in', { exact: true }).last().click()
        await inviteePage.waitForURL(LANDED_URL, { timeout: 15_000 })
    })
})
