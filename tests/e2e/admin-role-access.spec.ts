import { expect, test } from '@playwright/test'
import { clickSidebarItem, createInvitedUser, login, loginAs, navigateToPackage } from './helpers'

// The owner/admin split inside Settings.
//
// Role is the only privilege axis, and it grants two different things:
//   - owner OR admin  → the Organization group (Storage, Members, Labels,
//     Audit Log)
//   - owner ALONE     → Packages and Build History, because installing,
//     removing, or disabling a package rebuilds — or changes what runs in —
//     the artifact the whole deployment serves. Enablement is a
//     pkg_registry.status write, the same axis as install, so the entire
//     Packages screen is owner-only rather than split across two lists.
//
// The seeded TEST_USER is the owner. The admin is minted through the UI —
// invite flow, then the Members role picker — rather than a raw PB write, so
// the test drives the same mutations the app does.

test.describe('Settings · role access', () => {
    test('owner gets Packages and Build History; admin does not', async ({ page }) => {
        // --- Owner: Packages opens the install manager ---
        await login(page)
        await navigateToPackage(page, 'settings')
        await expect(page.getByText('Build History', { exact: true })).toBeVisible({
            timeout: 15_000,
        })
        await clickSidebarItem(page, 'Packages')
        await expect(page.getByTestId('settings-install-manager')).toBeVisible({ timeout: 20_000 })

        // --- Promote a fresh member to admin through the Members UI ---
        const { user, close } = await createInvitedUser(page, 'adminrole')
        try {
            await login(page)
            await navigateToPackage(page, 'settings')
            await page.getByText('Members', { exact: true }).first().click()

            // Filter to the new user: the roster holds every account earlier
            // specs invited, so the row may be far down the list.
            await page.getByPlaceholder('Search by name, email, or role').fill(user.username)
            await page.getByTestId(`member-row-${user.username}`).click()

            // The drawer's role picker is the app's own mutation path.
            await page.getByTestId('role-option-admin').click()
            // The drawer's role badge re-renders off the persisted value, so
            // it flipping to Admin is what proves the write committed. Match
            // the badge's own testID rather than its text: the visible caps
            // come from CSS text-transform, so the DOM still reads "Admin",
            // and the word appears a dozen times in the picker copy besides.
            await expect(page.getByTestId(`member-badge-role-${user.username}`)).toHaveText(
                'Admin',
                { timeout: 10_000 }
            )

            // --- Admin: the org settings yes, the owner-only tiers no ---
            const adminContext = await page.context().browser()!.newContext()
            const adminPage = await adminContext.newPage()
            try {
                await loginAs(adminPage, user.username, user.password)
                await navigateToPackage(adminPage, 'settings')

                // Reaching Labels proves the Organization group mounted for a
                // non-owner — the admin tier is intact...
                await clickSidebarItem(adminPage, 'Labels')
                await expect(adminPage).toHaveURL(/settings\/labels/, { timeout: 20_000 })
                await adminPage.goBack()

                // ...but Packages and Build History are owner-only, so neither
                // appears in the settings index.
                await expect(adminPage.getByText('Packages', { exact: true })).toHaveCount(0)
                await expect(adminPage.getByText('Build History', { exact: true })).toHaveCount(0)
            } finally {
                await adminContext.close()
            }
        } finally {
            await close()
        }
    })

    test('a member sees no Organization settings', async ({ page }) => {
        const { inviteePage, close } = await createInvitedUser(page, 'memberrole')
        try {
            // createInvitedUser leaves the invitee signed in as a plain member.
            // AdminSettings self-gates on isAdmin, so the whole Organization
            // group — and every owner-only entry inside it — must not render.
            await navigateToPackage(inviteePage, 'settings')
            await expect(inviteePage.getByText('Personal', { exact: true })).toBeVisible({
                timeout: 15_000,
            })
            await expect(inviteePage.getByText('Members', { exact: true })).toHaveCount(0)
            await expect(inviteePage.getByText('Build History', { exact: true })).toHaveCount(0)
        } finally {
            await close()
        }
    })
})
