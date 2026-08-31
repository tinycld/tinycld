import { expect, type Page, test } from '@playwright/test'

// Smoke-tests for the /admin flow. Split into three tests so most of the
// coverage runs without the one-time PW_SETUP_TOKEN:
//   1. bootstrap (needs PW_SETUP_TOKEN) — fills the first-run wizard and
//      creates the superuser. Skipped if the token isn't exported.
//   2. dashboard packages tab — logs in as the superuser, asserts every
//      bundled feature package shows up.
//   3. system settings — saves a value and asserts it reaches the client.
//
// The tests run serially: the later tests depend on the superuser created by
// test 1 (or by a previous bootstrap if PW_SETUP_TOKEN was consumed earlier).
//
// PW_SETUP_TOKEN is scraped from `docker logs <container>` by the workflow
// before invoking playwright.

const SETUP_TOKEN = process.env.PW_SETUP_TOKEN

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

async function loginAsSuperuser(page: Page) {
    // /setup, not /admin: the pre-auth bootstrap + superuser-login console moved
    // there in the single-org migration (app/admin.tsx → app/setup.tsx). /admin
    // is now the authenticated console behind AuthGate, which renders a
    // LoginModal rather than the superuser form.
    await page.goto('/a/setup')
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
    test('bootstrap superuser via /setup wizard', async ({ page }) => {
        test.skip(
            !SETUP_TOKEN,
            'PW_SETUP_TOKEN not set — workflow must scrape it from `docker logs` and export before running'
        )

        await page.goto(`/a/setup?token=${SETUP_TOKEN}`)

        await expect(page.getByText('Welcome to TinyCld')).toBeVisible()

        // The wizard form has five required fields. Application Name and
        // App URL were added after this spec was first written; without
        // them the submit handler short-circuits on validation and the
        // 'Create Account & Continue' click resolves into nothing.
        await page
            .getByRole('textbox', { name: 'Application Name', exact: true })
            .fill('Smoke TinyCld')
        await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
        await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
        await page
            .getByRole('textbox', { name: 'Confirm Password', exact: true })
            .fill(SUPERUSER_PASSWORD)
        await page
            .getByRole('textbox', { name: 'App URL', exact: true })
            .fill('http://localhost:7090')

        await page.getByRole('button', { name: 'Create Account & Continue' }).click()

        // Setup wizard transitions in-place to the dashboard. Single-org: the
        // default tab is Packages (SetupDashboard defaultTab), and the
        // Organizations tab is now a static "managed by the router" explainer
        // rather than a create form with an empty list.
        await expect(page.getByText('Packages', { exact: true }).first()).toBeVisible()
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
    test('superuser can configure system settings (Sentry DSN + VAPID)', async ({ page }) => {
        const TEST_DSN = 'https://e2ekey@o1.ingest.sentry.io/42'

        await loginAsSuperuser(page)
        await page.getByText('Settings', { exact: true }).first().click()

        // Sentry DSN: fill, save. On a fresh deployment the field starts empty, so
        // filling it makes the form dirty and enables Save. After a successful save
        // the form is no longer dirty and the button re-disables — wait for that so
        // the value is persisted before we reload. (The smoketest runner always
        // boots a clean DB, so the field is reliably empty here.)
        await page.getByRole('textbox', { name: 'Sentry DSN', exact: true }).fill(TEST_DSN)
        await page.getByTestId('sentry-dsn-save').click()
        await expect(page.getByTestId('sentry-dsn-save')).toBeDisabled()

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
