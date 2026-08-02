import { expect, test } from '@playwright/test'
import { createInvitedUser, login, loginAs, navigateToPackage } from './helpers'

// The owner/admin split inside the Admin console.
//
// Role is the only privilege axis (the super_admins junction is gone), and it
// grants two different things:
//   - owner OR admin  → the console itself (rail icon, /admin, Organizations,
//     Build History, Settings)
//   - owner ALONE     → Packages, because installing or removing a package
//     rebuilds the artifact the whole deployment runs.
//
// The seeded TEST_USER is the owner. The admin is minted through the UI —
// invite flow, then the Members role picker — rather than a raw PB write, so
// the test drives the same mutations the app does.

test.describe('Admin console · role access', () => {
    test('owner sees Packages; admin reaches the console without it', async ({ page }) => {
        // --- Owner: the console, including Packages ---
        await login(page)
        const rail = page.getByTestId('nav-admin')
        await expect(rail).toBeVisible({ timeout: 15_000 })
        await rail.click()
        await page.getByTestId('pkg-active-admin').waitFor({ state: 'visible' })
        await expect(page.getByTestId('admin-section-packages')).toBeVisible({ timeout: 20_000 })

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

            // --- Admin: console yes, Packages no ---
            const adminContext = await page.context().browser()!.newContext()
            const adminPage = await adminContext.newPage()
            try {
                await loginAs(adminPage, user.username, user.password)

                const adminRail = adminPage.getByTestId('nav-admin')
                await expect(adminRail).toBeVisible({ timeout: 15_000 })
                await adminRail.click()
                await adminPage.getByTestId('pkg-active-admin').waitFor({ state: 'visible' })

                // Organizations is the admin's landing section — reaching it
                // proves the console mounted for a non-owner.
                await expect(adminPage.getByTestId('admin-section-organizations')).toBeVisible({
                    timeout: 20_000,
                })

                // Packages is owner-only: absent from the sidebar, and a
                // hand-typed URL redirects back rather than rendering it.
                await expect(adminPage.getByTestId('admin-section-packages')).toHaveCount(0)
            } finally {
                await adminContext.close()
            }
        } finally {
            await close()
        }
    })

    test('a member gets no Admin rail entry', async ({ page }) => {
        const { inviteePage, close } = await createInvitedUser(page, 'memberrole')
        try {
            // createInvitedUser leaves the invitee signed in as a plain member.
            // The rail button self-gates on isAdmin, so it must not render.
            await expect(inviteePage.getByTestId('nav-admin')).toHaveCount(0)
        } finally {
            await close()
        }
    })
})
