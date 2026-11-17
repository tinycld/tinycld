import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, loginAs } from './helpers'

// Removing a member and renaming a login, from Settings › Members. Each test
// invites its own member, because both change that account for good.

// SPA navigation, as in admin-sign-in-help.spec.ts: createInvitedUser leaves
// the owner on Settings › Members, so go through the Settings index.
async function openMembers(page: Page) {
    await page.getByTestId('nav-settings').click()
    await expect(page.getByText('Account', { exact: true })).toBeVisible()
    await page.getByText('Members', { exact: true }).click()
}

test.describe('member lifecycle', () => {
    test('removing a member hides them and frees the username', async ({ page }) => {
        const { user, close } = await createInvitedUser(page, 'gone', { name: 'Gone Person' })
        await close()
        await openMembers(page)

        await page.getByTestId(`member-row-${user.username}`).click()
        await page.getByText('Remove', { exact: true }).click()
        // Keep their work is the default choice.
        await expect(page.getByTestId('offboard-plan-keep')).toBeVisible()
        await page.getByTestId('remove-member-confirm').fill('Gone Person')
        await page.getByTestId('remove-member-submit').click()
        await expect(page.getByTestId(`member-row-${user.username}`)).toHaveCount(0)

        // The username is free again.
        await page.getByTestId('members-invite').click()
        await page.getByTestId('username').fill(user.username)
        await page.getByText('Add user and invite', { exact: true }).click()
        await expect(page.getByTestId('invite-link-step')).toBeVisible()
        await page.getByTestId('invite-link-done').click()
        // A new member, not the removed account's invite sent again.
        await expect(page.getByTestId(`member-row-${user.username}`)).toBeVisible()
    })

    test('an admin renames a login and the member signs in with it', async ({ page }) => {
        const { user, close } = await createInvitedUser(page, 'ren', { name: 'Ren Person' })
        await close()
        await openMembers(page)

        await page.getByTestId(`member-row-${user.username}`).click()
        const renamed = `${user.username.slice(0, 26)}-new`
        await page.getByTestId('rename-username').fill(renamed)
        await page.getByTestId('rename-username-submit').click()
        await expect(page.getByTestId(`member-row-${renamed}`)).toBeVisible()

        await loginAs(page, renamed, user.password)
    })
})
