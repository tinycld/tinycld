import { expect, test } from '@playwright/test'
import { login } from './helpers'

// Covers the DocumentTitle component's compositional behavior:
//   - pre-auth screens compose brand + leaf
//   - the settings layout fallback fires when no settings child is mounted
//   - a settings child wins over the layout fallback (react-helmet-async
//     last-mount-wins ordering)
//   - pkg-only mounts compose brand + pkg with no leaf
//
// Single-org: there is no org segment to assert. DocumentTitle still supports
// one (`includeOrg`, DocumentTitle.tsx:71), but useOrgInfo() returns
// `org: null` in a single-org deployment, so it never renders — the router
// owns tenancy and materializes no branding into the tenant. The assertions
// that expected "TinyCld: Test Organization — …" were removed rather than
// reworded, because there is currently no source for an org name to come
// from. See HANDOFF "org branding has no source".
test.describe('Document title', () => {
    test('pre-auth /connect shows brand + leaf only', async ({ page }) => {
        await page.goto('/connect')
        await expect(page).toHaveTitle('TinyCld: Connect')
    })

    test('settings layout fallback wins on bare /settings', async ({ page }) => {
        await login(page)
        await page.goto('/settings')
        // The settings index.tsx doesn't mount its own DocumentTitle, so
        // only the layout's <DocumentTitle pkg="Settings" /> is active.
        await expect(page).toHaveTitle('TinyCld: Settings')
    })

    test('settings child overrides the layout fallback', async ({ page }) => {
        await login(page)
        await page.goto('/settings/personal')
        // Both the layout (pkg="Settings") and the child
        // (pkg="Settings" title="Personal") mount; child wins per
        // react-helmet-async ordering, producing the more-specific title.
        await expect(page).toHaveTitle('TinyCld: Settings — Personal')
    })

    test('help hub uses pkg without a leaf', async ({ page }) => {
        await login(page)
        await page.goto('/help')
        await expect(page).toHaveTitle('TinyCld: Help')
    })
})
