import { expect, test } from '@playwright/test'
import { login } from './helpers'

// Covers the DocumentTitle component's compositional behavior:
//   - pre-auth screens compose brand + leaf, suppressing the org segment
//   - the settings layout fallback fires when no settings child is mounted
//   - a settings child wins over the layout fallback (react-helmet-async
//     last-mount-wins ordering)
//   - pkg-only mounts compose brand + org + pkg with no leaf
//
// The org segment comes from useOrgInfo() → GET /api/org-info →
// Settings().Meta.AppName, which the seed sets to "Test Organization" (in a
// router-managed tenant the same value is materialized from the org's
// display_name). Routes stay slug-free — the org appears in the TITLE, not
// the URL.
const ORG_NAME = 'Test Organization'

test.describe('Document title', () => {
    test('pre-auth /connect shows brand + leaf only, no org segment', async ({ page }) => {
        await page.goto('/connect')
        await expect(page).toHaveTitle('TinyCld: Connect')
    })

    test('settings layout fallback wins on bare /settings', async ({ page }) => {
        await login(page)
        await page.goto('/settings')
        // The settings index.tsx doesn't mount its own DocumentTitle, so
        // only the layout's <DocumentTitle pkg="Settings" /> is active.
        await expect(page).toHaveTitle(`TinyCld: ${ORG_NAME} — Settings`)
    })

    test('settings child overrides the layout fallback', async ({ page }) => {
        await login(page)
        await page.goto('/settings/personal')
        // Both the layout (pkg="Settings") and the child
        // (pkg="Settings" title="Personal") mount; child wins per
        // react-helmet-async ordering, producing the more-specific title.
        await expect(page).toHaveTitle(`TinyCld: ${ORG_NAME} — Settings — Personal`)
    })

    test('help hub uses pkg without a leaf', async ({ page }) => {
        await login(page)
        await page.goto('/help')
        await expect(page).toHaveTitle(`TinyCld: ${ORG_NAME} — Help`)
    })
})
