import { expect, test } from '@playwright/test'

// Guards the superuser bootstrap/recovery route. The single admin CONSOLE for
// logged-in owners/admins is the in-shell /admin area; /setup is the pre-auth
// bootstrap door (first-run wizard + superuser recovery login). This regular-CI
// smoke test proves the /setup route resolves, mounts the page shell (not a blank
// screen or 404), and wires its document title. It deliberately does NOT log in —
// the regular e2e seed provisions an app user, not a known superuser password,
// and the full bootstrap/login/dashboard flow is covered by the docker smoke
// suite (tests/install/setup-and-packages.spec.ts).
//
// A logged-in owner/admin app user who hits /setup is redirected to /admin; the
// anonymous case below falls through to the superuser login form.
test.describe('Setup / bootstrap route', () => {
    test('/setup resolves to the superuser bootstrap console', async ({ page }) => {
        await page.goto('/a/setup')

        // DocumentTitle title="Setup" includeOrg={false} → brand + leaf, no org.
        await expect(page).toHaveTitle('TinyCld: Setup')

        // Unauthenticated, the console shows the superuser login form. Its heading
        // and Sign-in button mounting proves SetupPage rendered (not a 404 / blank).
        await expect(page.getByText('Superuser Login')).toBeVisible({ timeout: 15_000 })
        await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible()
    })
})
