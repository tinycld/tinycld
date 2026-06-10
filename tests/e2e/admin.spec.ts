import { expect, test } from '@playwright/test'

// Guards the superuser console route. The page moved from /setup to /admin; this
// is the regular-CI smoke test that the route resolves, mounts the console (not a
// blank screen or 404), and wires its document title. It deliberately does NOT
// log in — the regular e2e seed provisions an app user, not a known superuser
// password, and the full bootstrap/login/dashboard flow is covered by the
// docker smoke suite (tests/install/setup-and-packages.spec.ts). Here we only
// need to prove the route + page shell are intact after the rename.
//
// SetupPage now has a super-admin app-user path: a logged-in user listed in
// super_admins reaches the dashboard with their normal session (no superuser
// login). That path is exercised by the docker smoke suite (which can seed a
// super-admin grant); the anonymous case below still falls through to the
// superuser login form, so this assertion is unchanged.
test.describe('Admin console route', () => {
    test('/admin resolves to the superuser console', async ({ page }) => {
        await page.goto('/admin')

        // DocumentTitle title="Admin" includeOrg={false} → brand + leaf, no org.
        await expect(page).toHaveTitle('TinyCld: Admin')

        // Unauthenticated, the console shows the superuser login form. Its heading
        // and Sign-in button mounting proves SetupPage rendered (not a 404 / blank).
        await expect(page.getByText('Superuser Login')).toBeVisible({ timeout: 15_000 })
        await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible()
    })
})
