import { expect, type Page, test } from '@playwright/test'

// Smoke-tests for the /admin flow. Split into three tests so most of the
// coverage runs without the one-time PW_SETUP_TOKEN:
//   1. bootstrap (needs PW_SETUP_TOKEN) — fills the first-run wizard and
//      creates the superuser. Skipped if the token isn't exported.
//   2. dashboard packages tab — logs in as the superuser, asserts every
//      bundled feature package shows up.
//   3. super-admin grant — logs in as the superuser, grants an app user
//      access to the console. Regression test for the grant 500.
//   4. system settings — saves a value and asserts it reaches the client.
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

const SUPERUSER_EMAIL = 'smoke@example.com'
const SUPERUSER_PASSWORD = 'SmokeTest1234!'

// The user granted super-admin below. Single-org: the setup console has no
// user-creation UI (that was the org-create form), so the grant test mints
// its target through the superuser API first. Creating the *fixture* via the
// API is fine — the grant itself, which is what this spec tests, still runs
// through the console UI.
const GRANT_TARGET_NAME = 'Smoke Grantee'
const GRANT_TARGET_EMAIL = 'grantee@smoke.example'
const GRANT_TARGET_PASSWORD = 'GranteePass1234!'

// Mints the app user the grant test targets, via the superuser REST API.
// Idempotent: a 400 from a duplicate email means a previous run already
// created it, which is fine — the grant only needs the user to exist.
async function createGrantTarget(page: Page) {
    const auth = await page.request.post('/api/collections/_superusers/auth-with-password', {
        data: { identity: SUPERUSER_EMAIL, password: SUPERUSER_PASSWORD },
    })
    expect(auth.ok()).toBeTruthy()
    const { token } = (await auth.json()) as { token: string }

    const res = await page.request.post('/api/collections/users/records', {
        headers: { Authorization: token },
        data: {
            email: GRANT_TARGET_EMAIL,
            password: GRANT_TARGET_PASSWORD,
            passwordConfirm: GRANT_TARGET_PASSWORD,
            name: GRANT_TARGET_NAME,
            username: 'smokegrantee',
            verified: true,
        },
    })
    // 400 == already exists from an earlier run in the same container.
    if (!res.ok() && res.status() !== 400) {
        throw new Error(`create grant target failed: ${res.status()} ${await res.text()}`)
    }
}

async function loginAsSuperuser(page: Page) {
    await page.goto('/admin')
    await expect(page.getByText('Superuser Login')).toBeVisible()
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(SUPERUSER_EMAIL)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(SUPERUSER_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()
    // The dashboard renders the nav rail; wait for a rail entry before assertions.
    await expect(page.getByText('Organizations', { exact: true }).first()).toBeVisible()
}

test.describe.configure({ mode: 'serial' })

test.describe('first-run install', () => {
    test('bootstrap superuser via /admin wizard', async ({ page }) => {
        test.skip(
            !SETUP_TOKEN,
            'PW_SETUP_TOKEN not set — workflow must scrape it from `docker logs` and export before running'
        )

        await page.goto(`/admin?token=${SETUP_TOKEN}`)

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
    // single-org migration: OrganizationsTab is now a static empty state
    // explaining that tenant provisioning belongs to the multi-org router,
    // so there is no create form left to drive. The router owns that flow
    // and tests it in its own suite (internal/controlplane).

    test('superuser can grant another user super admin', async ({ page }) => {
        await loginAsSuperuser(page)

        // Regression test for the grant 500: a PB-superuser grantor carries a
        // non-nil auth identity whose id lives in _superusers, NOT users. The
        // handler stamped that id into super_admins.created_by (a users relation),
        // which failed relation validation and returned a 500 the dev console
        // never surfaced (it only reached Sentry/the _logs DB). Granting while
        // logged in as the superuser — the common /admin path — is exactly this.
        //
        // The grant handler resolves an EXISTING user by email
        // (resolveGrantTarget in coreserver/super_admins.go), so mint one first.
        await createGrantTarget(page)

        await page.getByText('Super Admins', { exact: true }).first().click()

        // Wait for the section to mount (the grant CTA) before interacting.
        await expect(page.getByRole('button', { name: 'Grant access' })).toBeVisible()

        await page.getByRole('button', { name: 'Grant access' }).click()
        await page.getByPlaceholder('person@example.com').fill(GRANT_TARGET_EMAIL)
        await page.getByRole('button', { name: 'Grant super admin' }).click()

        // On success the form closes, the roster refetches, and the granted user
        // appears as a row. Before the fix this never happened — the POST 500'd
        // and the email surfaced as an inline form error instead. Asserting the
        // grantee's row (not just "not empty") makes the regression signal precise.
        await expect(page.getByText(GRANT_TARGET_EMAIL, { exact: true })).toBeVisible()
    })

    // Exercises the full system-settings chain end-to-end: save a value in the
    // /admin Settings UI → server stores it → the app server injects the
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
        await page.goto('/admin')
        const injected = await page.evaluate(
            () =>
                (window as unknown as { __TINYCLD_PUBLIC_CONFIG__?: { sentryDsn?: string } })
                    .__TINYCLD_PUBLIC_CONFIG__?.sentryDsn
        )
        expect(injected).toBe(TEST_DSN)
    })
})
