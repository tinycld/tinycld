import { expect, test } from '@playwright/test'
import { login, navigateToAdmin, ORG_SLUG } from './helpers'

// The admin Organizations tab, driven as the seeded super-admin (the e2e seed
// grants the test user a super_admins row). Exercises the store-backed list +
// create + edit flows and the impersonate endpoint — the cross-org admin surface
// that the super_admin API rules + /api/admin/orgs/{id}/impersonate enable.

test.describe('Admin · Organizations', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToAdmin(page, 'organizations', 'Organizations')
    })

    test('lists the seeded org', async ({ page }) => {
        // The seed provisions ORG_SLUG (test-org) with the test user as owner.
        await expect(page.getByTestId(`org-row-${ORG_SLUG}`)).toBeVisible({ timeout: 15_000 })
    })

    test('creates a new organization', async ({ page }) => {
        // orgSlug is capped at 15 chars, so keep the unique suffix short.
        const suffix = Date.now().toString(36).slice(-6)
        const slug = `e2e-${suffix}`

        await page.getByTestId('org-new-toggle').click()
        await page.getByTestId('orgName').fill('E2E Org')
        // orgSlug auto-derives from the name; override it to a unique value.
        await page.getByTestId('orgSlug').fill(slug)
        await page.getByTestId('ownerName').fill('E2E Owner')
        await page.getByTestId('email').fill(`${slug}@example.com`)
        await page.getByTestId('ownerUsername').fill(`e2e${suffix}`)
        await page.getByTestId('password').fill('OwnerPass1234!')
        // When the mail package is linked, the create form requires a mail domain
        // (it emits an org_provisioning intent for mail's hook to act on).
        const mailDomain = page.getByTestId('mailDomain')
        if (await mailDomain.count()) {
            await mailDomain.fill(`${slug}.example.com`)
        }

        await page.getByTestId('org-create-submit').click()

        // The new org appears in the live list (pbtsdb realtime, no refetch).
        await expect(page.getByTestId(`org-row-${slug}`)).toBeVisible({ timeout: 15_000 })
    })

    test('edits the seeded org name', async ({ page }) => {
        const row = page.getByTestId(`org-row-${ORG_SLUG}`)
        await expect(row).toBeVisible()

        await page.getByTestId(`org-row-toggle-${ORG_SLUG}`).click()
        const nameField = page.getByTestId('name')
        await expect(nameField).toBeVisible()

        const newName = `Test Org ${Date.now()}`
        await nameField.fill(newName)
        await page.getByTestId(`org-save-${ORG_SLUG}`).click()

        // The row label updates from the live query.
        await expect(row.getByText(newName)).toBeVisible({ timeout: 15_000 })
    })

    test('impersonates an org owner', async ({ page }) => {
        // The seeded org has a fully-resolved owner (the test user), so its Visit
        // button is present without waiting on optimistic relation-expansion. Visit
        // calls /api/admin/orgs/{id}/impersonate (super-admin reachable) to mint the
        // owner's token, then lands in that org's workspace.
        const visit = page.getByTestId(`org-visit-${ORG_SLUG}`)
        await expect(visit).toBeVisible({ timeout: 15_000 })
        await visit.click()
        await page.waitForURL(new RegExp(`/a/${ORG_SLUG}(/|$)`), { timeout: 20_000 })
        // The workspace mounted under the impersonated session (the org-home rail
        // entry only renders inside a logged-in workspace).
        await expect(page.getByTestId('nav-home')).toBeVisible({ timeout: 15_000 })
    })
})
