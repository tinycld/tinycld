import { expect, test } from '@playwright/test'

// Guards the superuser recovery route. The single admin CONSOLE for logged-in
// owners/admins is the in-shell /admin area; /a/setup is the pre-auth first-run
// wizard door (redirects into the signed-in wizard once the server is claimed),
// and /a/setup/recovery is the raw-superuser recovery login. This regular-CI
// smoke test proves /a/setup/recovery resolves, mounts the page shell (not a
// blank screen or 404), and wires its document title. It deliberately does NOT
// log in — the regular e2e seed provisions an app user, not a known superuser
// password, and the full bootstrap/login/dashboard flow is covered by the
// docker smoke suite (tests/install/setup-and-packages.spec.ts).
test.describe('Setup recovery route', () => {
    test('/setup/recovery resolves to the superuser recovery console', async ({ page }) => {
        await page.goto('/a/setup/recovery')

        // DocumentTitle title="Recovery" includeOrg={false} → brand + leaf, no org.
        await expect(page).toHaveTitle('TinyCld: Recovery')

        // The console shows the superuser login form regardless of app-user auth
        // state. Its heading and Sign-in button mounting proves SetupRecovery
        // rendered (not a 404 / blank).
        await expect(page.getByText('Superuser Login')).toBeVisible({ timeout: 15_000 })
        await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible()
    })
})
