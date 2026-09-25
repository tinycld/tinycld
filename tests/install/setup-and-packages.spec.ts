import { expect, type Page, test } from '@playwright/test'

// Smoke-tests for the first-run wizard + /admin flow. Split into three tests
// so most of the coverage runs without the one-time PW_SETUP_CODE:
//   1. bootstrap (needs PW_SETUP_CODE) — claims the server and creates the
//      owner through the first-run wizard. Skipped if the code isn't exported.
//   2. dashboard packages tab — logs in as the superuser, asserts every
//      bundled feature package shows up.
//   3. system settings — saves a value and asserts it reaches the client.
//      Drives /settings as the OWNER app user, not the /setup/recovery
//      console: the system-settings panels live in the in-app settings area
//      (/setup/recovery is superuser-recovery only, and redirects any admin
//      away).
//
// The tests run serially: the later tests depend on the superuser created by
// test 1 (or by a previous bootstrap if PW_SETUP_CODE was consumed earlier).
// Test 1 finishes the wizard with "Finish later" (rather than completing every
// step) so it lands in the app and does not leave the owner mid-wizard, which
// would otherwise redirect tests 2 and 3's navigations back into /a/setup.
//
// PW_SETUP_CODE is scraped from `docker logs <container>` by the workflow
// before invoking playwright.

const SETUP_CODE = process.env.PW_SETUP_CODE

// Adjust this list when the public-CI default LINKED_PACKAGES set changes.
// The names match `app/server/bundled-packages.json::name` (capitalized
// labels), which is what PackageManager renders on the dashboard. Calc +
// Text were added to the default bundle alongside drive's share-dialog
// work; keep this in sync with that JSON.
const EXPECTED_BUNDLED = [
    'Calc',
    'Calendar',
    'Contacts',
    'Drive',
    'Google Takeout Import',
    'Mail',
    'Text',
]

// The superuser this spec logs in as. Read from the workspace .env
// (ADMIN_USER_LOGIN / ADMIN_USER_PW — loaded by playwright.config.ts and
// forwarded into the runner's sandbox), falling back to fixed smoke values when
// unset (CI / a bare run). Hardcoding these meant the spec could only ever run
// against a container bootstrapped with those exact values — pointed at any
// other server the sign-in silently failed and every later assertion timed out
// on a missing post-login landmark, which reads as a UI bug rather than bad
// credentials. `||` (not `??`) so an empty-string env var falls back too: the
// runner forwards ADMIN_USER_LOGIN="" when the .env key is absent, and that
// empty value must not override the fallback. Mirrors todo-install.spec.ts.
const SUPERUSER_EMAIL = process.env.ADMIN_USER_LOGIN || 'smoke@example.com'
const SUPERUSER_PASSWORD = process.env.ADMIN_USER_PW || 'SmokeTest1234!'

// Signs in as the owner APP user (same credentials as the superuser — the
// first-run wizard mints both from one form, see createOwnerOperator). Needed
// because the system-settings screens are owner-gated app routes; a raw
// _superusers session is not an app user and never reaches them.
async function loginAsOwner(page: Page) {
    await page.goto('/a/settings')
    const identifier = page.getByTestId('identifier')
    const landed = await Promise.race([
        identifier
            .waitFor({ state: 'visible', timeout: 15_000 })
            .then(() => 'login-form' as const)
            .catch(() => 'neither' as const),
        page
            .getByText('Settings', { exact: true })
            .first()
            .waitFor({ state: 'visible', timeout: 15_000 })
            .then(() => 'settings' as const)
            .catch(() => 'neither' as const),
    ])
    if (landed === 'settings') return

    await identifier.fill(SUPERUSER_EMAIL)
    await page.getByPlaceholder('Password').fill(SUPERUSER_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()
    await expect(page.getByText('System', { exact: true })).toBeVisible()
}

async function loginAsSuperuser(page: Page) {
    // /setup/recovery, not /admin: the raw-superuser recovery console moved
    // there once /setup itself became the first-run wizard door. /admin is the
    // authenticated console behind AuthGate, which renders a LoginModal rather
    // than the superuser form.
    await page.goto('/a/setup/recovery')
    await expect(page.getByText('Superuser Login')).toBeVisible()
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()
    // The dashboard renders the nav rail; wait for a rail entry before
    // assertions. Role + hasText, NOT getByRole('tab', { name }) — RN Web
    // renders the label as a child <Text>, so the role exposes no accessible
    // name. This used to wait for an 'Organizations' tab that no longer exists.
    await expect(page.getByRole('tab').filter({ hasText: 'Packages' })).toBeVisible()
}

test.describe.configure({ mode: 'serial' })

test.describe('first-run install', () => {
    test('bootstrap owner via /setup wizard', async ({ page }) => {
        test.skip(
            !SETUP_CODE,
            'PW_SETUP_CODE not set — workflow must scrape it from `docker logs` and export before running'
        )

        await page.goto(`/a/setup?code=${SETUP_CODE}`)

        await expect(page.getByText('Create your owner account')).toBeVisible()

        await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Smoke Owner')
        await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
        await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
        await page
            .getByRole('textbox', { name: 'Confirm password', exact: true })
            .fill(SUPERUSER_PASSWORD)

        await page.getByRole('button', { name: 'Create account' }).click()

        // The owner is created and signed in; the signed-in wizard opens on its
        // first step.
        await expect(page.getByText('Your workspace')).toBeVisible()

        // Tests 2 and 3 sign back in and expect to land on Settings/System —
        // leaving the owner mid-wizard would redirect those navigations back
        // into the wizard instead. "Finish later" dismisses it for now; Settings
        // still offers a "Finish setup" card to resume it.
        await page.getByRole('button', { name: 'Finish later' }).click()
    })

    test('superuser dashboard lists every bundled package', async ({ page }) => {
        await loginAsSuperuser(page)

        // Login lands on the Packages tab by default.
        for (const pkg of EXPECTED_BUNDLED) {
            await expect(
                page.getByText(pkg, { exact: true }),
                `bundled package ${pkg} should appear in the dashboard`
            ).toBeVisible()
        }

        // And confirm the count of "bundled" tags matches — guards against a
        // regression that drops a package without changing its name.
        const bundledTags = page.getByText('bundled', { exact: true })
        await expect(bundledTags).toHaveCount(EXPECTED_BUNDLED.length)
    })

    // The "superuser can create an organization" test was removed with the
    // single-org migration: tenant provisioning belongs to the hosting
    // router, so there is no create form left to drive. The router owns that
    // flow and tests it in its own suite (internal/controlplane).

    // Exercises the full system-settings chain end-to-end: save a value in the
    // /setup Settings UI → server stores it → the app server injects the
    // non-secret public config into app.html → the next page load exposes it on
    // window.__TINYCLD_PUBLIC_CONFIG__ (which lib/app-config.ts reads for the
    // Sentry DSN). Also drives the VAPID generate button.
    test('owner can configure system settings (Sentry DSN + VAPID)', async ({ page }) => {
        const TEST_DSN = 'https://e2ekey@o1.ingest.sentry.io/42'

        await loginAsOwner(page)

        // Each system panel is its own owner-gated route under the System group,
        // reached from the settings index — the path a real operator takes.
        await page.getByText('Error Reporting', { exact: true }).click()

        // Sentry DSN: fill, save. On a fresh deployment the field starts empty, so
        // filling it makes the form dirty and enables Save. After a successful save
        // the form is no longer dirty and the button re-disables — wait for that so
        // the value is persisted before we reload. (The smoketest runner always
        // boots a clean DB, so the field is reliably empty here.)
        await page.getByRole('textbox', { name: 'Sentry DSN', exact: true }).fill(TEST_DSN)
        await page.getByTestId('sentry-dsn-save').click()
        await expect(page.getByTestId('sentry-dsn-save')).toBeDisabled()

        // VAPID lives on its own screen; go back to the index and into it.
        await page.goBack()
        await page.getByText('Web Push', { exact: true }).click()

        // VAPID: generate a keypair server-side; the panel flips to "Configured".
        await page.getByTestId('vapid-generate').click()
        await expect(page.getByText('Configured ✓')).toBeVisible()

        // Reload so the server re-serves app.html with the stored DSN injected as
        // window.__TINYCLD_PUBLIC_CONFIG__ — the value the web client reads at
        // startup. This is the public-config injection chain, end to end. The
        // global is set by an inline <script> regardless of auth, so we can read
        // it on the (logged-out) shell without signing back in.
        await page.goto('/a/settings')
        const injected = await page.evaluate(
            () =>
                (window as unknown as { __TINYCLD_PUBLIC_CONFIG__?: { sentryDsn?: string } })
                    .__TINYCLD_PUBLIC_CONFIG__?.sentryDsn
        )
        expect(injected).toBe(TEST_DSN)
    })
})
