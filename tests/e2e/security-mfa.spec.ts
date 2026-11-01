import { expect, type Page, test } from '@playwright/test'
import { appShell, clearSession, createInvitedUser, type InvitedUser } from './helpers'
import { totpCode } from './totp-helpers'

// Two-step verification, passkeys and app passwords, driven through the UI.
// Each test turns MFA on for its own invited member: turning it on for the
// shared TEST_USER would stop every other spec's password login.
test.describe('security: two-step verification', () => {
    let invited: InvitedUser
    let inviteePage: Page
    let closeInvitee: () => Promise<void>

    test.beforeEach(async ({ page }) => {
        const created = await createInvitedUser(page, 'mfa')
        invited = created.user
        inviteePage = created.inviteePage
        closeInvitee = created.close
    })

    test.afterEach(async () => {
        await closeInvitee()
    })

    async function openSecurity(page: Page) {
        await page.getByLabel('User menu').click()
        await page.getByRole('menu').getByText('Settings', { exact: true }).click()
        const accountGroup = page.getByText('Account', { exact: true }).locator('..')
        await accountGroup.getByText('Security', { exact: true }).click()
        await page.getByTestId('totp-section').waitFor({ state: 'visible' })
    }

    // Signing out is clearing this browser's session; the next load shows the
    // sign-in overlay. Returns once the password step is submitted.
    async function signOutAndSubmitPassword(page: Page) {
        await clearSession(page)
        await page.goto('/')
        await page.getByTestId('identifier').fill(invited.username)
        await page.getByPlaceholder('Password').fill(invited.password)
        await page.getByTestId('login-submit').click()
    }

    async function enrollAuthenticator(page: Page): Promise<{ secret: string; codes: string[] }> {
        await page.getByTestId('totp-start').click()
        const secret =
            (await page.getByTestId('totp-secret').textContent())?.replace(/\s+/g, '') ?? ''
        expect(secret).toMatch(/^[A-Z2-7]+$/)
        await page.getByTestId('totp-confirm-code').fill(totpCode(secret))
        await page.getByTestId('totp-confirm').click()
        const panel = page.getByTestId('recovery-codes')
        await expect(panel).toBeVisible()
        const codes = (await panel.textContent())?.match(/[a-z2-9]{5}-[a-z2-9]{5}/g) ?? []
        expect(codes).toHaveLength(10)
        await page.getByTestId('recovery-codes-done').click()
        await expect(page.getByTestId('totp-on')).toBeVisible()
        return { secret, codes }
    }

    test('turns on an authenticator app, then signs in with a code and a recovery code', async () => {
        await openSecurity(inviteePage)
        const { secret, codes } = await enrollAuthenticator(inviteePage)

        // The password alone now stops at the code step.
        await signOutAndSubmitPassword(inviteePage)
        await expect(inviteePage.getByTestId('mfa-step')).toBeVisible()
        // The step enrollment used is spent, so type the next one (the server
        // accepts one step of clock skew).
        await inviteePage.getByTestId('mfa-code').fill(totpCode(secret, 1))
        await inviteePage.getByTestId('mfa-submit').click()
        await appShell(inviteePage).waitFor({ state: 'visible', timeout: 15_000 })

        // A recovery code works once.
        await signOutAndSubmitPassword(inviteePage)
        await inviteePage.getByTestId('mfa-switch').click()
        await inviteePage.getByTestId('mfa-recovery-code').fill(codes[0])
        await inviteePage.getByTestId('mfa-recovery-submit').click()
        await appShell(inviteePage).waitFor({ state: 'visible', timeout: 15_000 })

        await signOutAndSubmitPassword(inviteePage)
        await inviteePage.getByTestId('mfa-switch').click()
        await inviteePage.getByTestId('mfa-recovery-code').fill(codes[0])
        await inviteePage.getByTestId('mfa-recovery-submit').click()
        await expect(inviteePage.getByTestId('mfa-error')).toBeVisible()
    })

    test('creates, shows once and revokes an app password', async () => {
        await openSecurity(inviteePage)
        await inviteePage.getByTestId('app-password-create').click()
        await inviteePage.getByPlaceholder('Thunderbird on my laptop').fill('Thunderbird')
        await inviteePage.getByText('Create', { exact: true }).click()

        const value = inviteePage.getByTestId('app-password-value')
        await expect(value).toHaveText(/^[a-z]{4} [a-z]{4} [a-z]{4} [a-z]{4}$/)
        await inviteePage.getByTestId('app-password-done').click()
        await expect(value).toBeHidden()

        const row = inviteePage.getByTestId('app-password-row')
        await expect(row).toHaveCount(1)
        await row.getByLabel('Revoke Thunderbird').click()
        await inviteePage.getByText('Revoke', { exact: true }).click()
        await expect(row).toHaveCount(0)
    })

    test('adds a passkey and signs in with it', async () => {
        // Chromium's virtual authenticator stands in for Touch ID / Windows
        // Hello: it holds discoverable credentials and always verifies the user.
        const cdp = await inviteePage.context().newCDPSession(inviteePage)
        await cdp.send('WebAuthn.enable')
        await cdp.send('WebAuthn.addVirtualAuthenticator', {
            options: {
                protocol: 'ctap2',
                transport: 'internal',
                hasResidentKey: true,
                hasUserVerification: true,
                isUserVerified: true,
                automaticPresenceSimulation: true,
            },
        })

        await openSecurity(inviteePage)
        await inviteePage.getByTestId('passkey-add').click()
        await expect(inviteePage.getByTestId('passkey-row')).toHaveCount(1)

        // A passkey is a second factor: the password alone stops at the step,
        // and the passkey finishes it.
        await signOutAndSubmitPassword(inviteePage)
        await expect(inviteePage.getByTestId('mfa-step')).toBeVisible()
        await inviteePage.getByTestId('mfa-passkey').click()
        await appShell(inviteePage).waitFor({ state: 'visible', timeout: 15_000 })

        // And it signs in on its own, with no username or password.
        await clearSession(inviteePage)
        await inviteePage.goto('/')
        await inviteePage.getByTestId('login-passkey').click()
        await appShell(inviteePage).waitFor({ state: 'visible', timeout: 15_000 })
    })
})
