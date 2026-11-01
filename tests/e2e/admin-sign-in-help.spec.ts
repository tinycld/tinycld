import { expect, type Page, test } from '@playwright/test'
import { extractFirstLink, waitForEmailTo } from './email-log-helpers'
import { appShell, clearSession, createInvitedUser, type InvitedUser } from './helpers'
import { totpCode } from './totp-helpers'

// An owner helping a member who cannot sign in: remove their two-step
// verification, give them a temporary password, or email a reset link to
// another address. Each test works on its own invited member.
test.describe('admin sign-in help', () => {
    let invited: InvitedUser
    let inviteePage: Page
    let closeInvitee: () => Promise<void>

    test.beforeEach(async ({ page }) => {
        const created = await createInvitedUser(page, 'signinhelp')
        invited = created.user
        inviteePage = created.inviteePage
        closeInvitee = created.close
    })

    test.afterEach(async () => {
        await closeInvitee()
    })

    // SPA navigation, as in invite-flow.spec.ts.
    async function openMember(page: Page) {
        await page.getByTestId('nav-settings').click()
        // The invite in beforeEach leaves this page on Settings › Members, whose
        // heading and "Members" role group also match the link text below.
        // Since expo-router 58 a link press commits on a later render, not
        // inside the click, so wait for the Settings index to replace it.
        await expect(page.getByText('Account', { exact: true })).toBeVisible()
        await page.getByText('Members', { exact: true }).click()
        await page.getByTestId(`member-row-${invited.username}`).click()
        await page.getByTestId('member-sign-in-help').waitFor({ state: 'visible' })
    }

    async function enrollAuthenticator(page: Page) {
        await page.getByLabel('User menu').click()
        await page.getByRole('menu').getByText('Settings', { exact: true }).click()
        const accountGroup = page.getByText('Account', { exact: true }).locator('..')
        await accountGroup.getByText('Security', { exact: true }).click()
        await page.getByTestId('totp-start').click()
        const secret =
            (await page.getByTestId('totp-secret').textContent())?.replace(/\s+/g, '') ?? ''
        await page.getByTestId('totp-confirm-code').fill(totpCode(secret))
        await page.getByTestId('totp-confirm').click()
        await page.getByTestId('recovery-codes-done').click()
        await expect(page.getByTestId('totp-on')).toBeVisible()
    }

    async function submitPassword(page: Page, password: string) {
        await clearSession(page)
        await page.goto('/')
        await page.getByTestId('identifier').fill(invited.username)
        await page.getByPlaceholder('Password').fill(password)
        await page.getByTestId('login-submit').click()
    }

    test('resets two-step verification and sets a temporary password the member must replace', async ({
        page,
    }) => {
        await enrollAuthenticator(inviteePage)

        await openMember(page)
        await expect(page.getByTestId('member-mfa-status')).toContainText('authenticator app')
        await page.getByTestId('member-mfa-reset').click()
        await page.getByText('Reset', { exact: true }).click()
        await expect(page.getByTestId('member-mfa-status')).toHaveText(
            'Two-step verification is off.'
        )

        await page.getByTestId('member-set-password').click()
        await page.getByTestId('member-temp-password').fill('Temporary123!')
        await page.getByTestId('member-temp-password-confirm').fill('Temporary123!')
        await page.getByTestId('member-temp-password-save').click()
        await expect(page.getByText(/Temporary password set/)).toBeVisible()

        // The temporary password opens only the "choose a new password" step.
        await submitPassword(inviteePage, 'Temporary123!')
        await expect(inviteePage.getByTestId('password-step')).toBeVisible()
        await inviteePage.getByTestId('new-password').fill('MemberChose123!')
        await inviteePage.getByTestId('new-password-confirm').fill('MemberChose123!')
        await inviteePage.getByTestId('new-password-submit').click()
        await expect(inviteePage.getByTestId('sign-in-notice')).toContainText('new password is set')

        await inviteePage.getByTestId('identifier').fill(invited.username)
        await inviteePage.getByPlaceholder('Password').fill('MemberChose123!')
        await inviteePage.getByTestId('login-submit').click()
        await appShell(inviteePage).waitFor({ state: 'visible', timeout: 15_000 })
    })

    test('emails a reset link to another address', async ({ page }) => {
        const elsewhere = `${invited.username}@elsewhere.example`
        await openMember(page)
        await page.getByTestId('member-send-reset').click()
        await expect(page.getByTestId('member-reset-email')).toHaveValue(invited.email)
        await page.getByTestId('member-reset-email').fill(elsewhere)
        await page.getByTestId('member-reset-email-send').click()
        await expect(page.getByText(`Reset link sent to ${elsewhere}`)).toBeVisible()

        const email = await waitForEmailTo(elsewhere)
        const link = extractFirstLink(email, /https?:\/\/[^\s"'<>]+\/reset-password\/[^\s"'<>]+/)
        expect(link).toContain('/reset-password/')
    })

    // On mobile, a passkey sign-in runs in the browser sheet on this page. A
    // temporary password must be replaced there, with guidance, not end in
    // an unexplained error.
    test('the passkey sheet asks for a new password after a temporary one', async ({ page }) => {
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
        await inviteePage.getByLabel('User menu').click()
        await inviteePage.getByRole('menu').getByText('Settings', { exact: true }).click()
        const accountGroup = inviteePage.getByText('Account', { exact: true }).locator('..')
        await accountGroup.getByText('Security', { exact: true }).click()
        await inviteePage.getByTestId('passkey-add').click()
        await expect(inviteePage.getByTestId('passkey-row')).toHaveCount(1)

        await openMember(page)
        await page.getByTestId('member-set-password').click()
        await page.getByTestId('member-temp-password').fill('Temporary123!')
        await page.getByTestId('member-temp-password-confirm').fill('Temporary123!')
        await page.getByTestId('member-temp-password-save').click()
        await expect(page.getByText(/Temporary password set/)).toBeVisible()

        // What the app opens in the sheet: a fresh browser on this page.
        await clearSession(inviteePage)
        await inviteePage.goto(`/a/native-auth#mode=login&code_challenge=${'A'.repeat(43)}`)
        await inviteePage.getByTestId('native-auth-continue').click()
        await expect(inviteePage.getByText('Choose a new password', { exact: true })).toBeVisible()
        await expect(inviteePage.getByText(/Something went wrong/)).toHaveCount(0)
        await inviteePage.getByTestId('new-password').fill('MemberChose123!')
        await inviteePage.getByTestId('new-password-confirm').fill('MemberChose123!')
        const saved = inviteePage.waitForResponse(r => r.url().includes('/confirm-password-reset'))
        await inviteePage.getByTestId('new-password-submit').click()
        expect((await saved).status()).toBe(204)

        // The new password now signs in; with a passkey on file the next step
        // is the second factor, not another password change.
        await inviteePage.goto('/')
        await inviteePage.getByTestId('identifier').fill(invited.username)
        await inviteePage.getByPlaceholder('Password').fill('MemberChose123!')
        await inviteePage.getByTestId('login-submit').click()
        await expect(inviteePage.getByTestId('mfa-step')).toBeVisible()
    })
})
