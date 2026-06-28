import { expect, type Page, test } from '@playwright/test'

// Smoke-tests for the /admin flow. Split into three tests so most of the
// coverage runs without the one-time PW_SETUP_TOKEN:
//   1. bootstrap (needs PW_SETUP_TOKEN) — fills the first-run wizard and
//      creates the superuser. Skipped if the token isn't exported.
//   2. dashboard packages tab — logs in as the superuser, asserts every
//      bundled feature package shows up.
//   3. organization creation — logs in as the superuser, creates a test org,
//      asserts it appears in the list. Regression test for the missing
//      'username' bug on user create.
//
// The tests run serially: tests 2 and 3 depend on the superuser created by
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

const TEST_ORG_NAME = 'Smoke Org'
const TEST_ORG_SLUG = 'smoke-org'
const TEST_ORG_OWNER_NAME = 'Smoke Owner'
const TEST_ORG_OWNER_EMAIL = 'owner@smoke.example'
const TEST_ORG_OWNER_PASSWORD = 'OwnerPass1234!'
const TEST_ORG_MAIL_DOMAIN = 'smoke.example'

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

        // Setup wizard transitions in-place to the dashboard with the
        // Organizations tab active.
        await expect(page.getByText('No organizations yet.')).toBeVisible()
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

    test('superuser can create an organization', async ({ page }) => {
        await loginAsSuperuser(page)

        // Switch to the Organizations section via the nav rail.
        await page.getByText('Organizations', { exact: true }).first().click()

        // Regression test for two org-create failures:
        //   1. "The username field is required." — the form once omitted
        //      username (now derived from the email).
        //   2. A 400 on `verified` ("Values don't match.") — the console wrote
        //      through an UNAUTHENTICATED app pb client, so setting the managed
        //      `verified` field on the new owner failed the users manageRule.
        //      The bootstrap now makes the operator a super_admin app user and
        //      the console's shared pb client carries that token, authorizing
        //      the managed-field write. The owner-login step below proves the
        //      account is actually usable (not just that a row appeared).
        await page.getByRole('button', { name: 'New organization' }).click()

        // The create form groups fields under Organization / Owner account
        // fieldsets, so the org name field is just "Name" and the owner's is
        // "Full name". Both are unique within the open form.
        await page.getByRole('textbox', { name: 'Name', exact: true }).fill(TEST_ORG_NAME)
        // The slug auto-derives from the name; overwrite to make the assertion
        // explicit and decoupled from the derivation rules.
        await page.getByRole('textbox', { name: 'Slug', exact: true }).fill(TEST_ORG_SLUG)
        await page
            .getByRole('textbox', { name: 'Full name', exact: true })
            .fill(TEST_ORG_OWNER_NAME)
        await page.getByRole('textbox', { name: 'Email', exact: true }).fill(TEST_ORG_OWNER_EMAIL)
        await page
            .getByRole('textbox', { name: 'Password', exact: true })
            .fill(TEST_ORG_OWNER_PASSWORD)
        // Mail is in EXPECTED_BUNDLED so the form requires a mail domain.
        await page
            .getByRole('textbox', { name: 'Mail domain', exact: true })
            .fill(TEST_ORG_MAIL_DOMAIN)

        await page.getByRole('button', { name: 'Create organization' }).click()

        // After creation the form closes and the org row renders with name + slug.
        await expect(page.getByText('No organizations yet.')).not.toBeVisible()
        await expect(page.getByText(TEST_ORG_NAME, { exact: true })).toBeVisible()
        await expect(page.getByText(TEST_ORG_SLUG, { exact: true })).toBeVisible()
        await expect(page.getByText(TEST_ORG_OWNER_EMAIL, { exact: true })).toBeVisible()

        // The owner must be able to sign in with the password just entered —
        // proves the account is usable, not merely that a row appeared. (The
        // `verified` 400 regression created no owner at all; a password=email
        // mix-up would create one that can't log in.) Use a fresh page so the
        // admin session stays intact for the next serial test.
        const ownerPage = await page.context().newPage()
        try {
            await ownerPage.goto('/')
            await ownerPage.getByTestId('identifier').fill(TEST_ORG_OWNER_EMAIL)
            await ownerPage.getByTestId('login-password').fill(TEST_ORG_OWNER_PASSWORD)
            await ownerPage.getByTestId('login-submit').click()
            await ownerPage.waitForURL(/\/a\//, { timeout: 30_000 })
        } finally {
            await ownerPage.close()
        }
    })

    test('superuser can grant another user super admin', async ({ page }) => {
        await loginAsSuperuser(page)

        // Regression test for the grant 500: a PB-superuser grantor carries a
        // non-nil auth identity whose id lives in _superusers, NOT users. The
        // handler stamped that id into super_admins.created_by (a users relation),
        // which failed relation validation and returned a 500 the dev console
        // never surfaced (it only reached Sentry/the _logs DB). Granting while
        // logged in as the superuser — the common /admin path — is exactly this.
        //
        // Grant the org owner created by the previous (serial) test; that user
        // already exists, so this exercises the grant-an-existing-user path.
        await page.getByText('Super Admins', { exact: true }).first().click()

        // Wait for the section to mount (the grant CTA) before interacting.
        await expect(page.getByRole('button', { name: 'Grant access' })).toBeVisible()

        await page.getByRole('button', { name: 'Grant access' }).click()
        await page.getByPlaceholder('person@example.com').fill(TEST_ORG_OWNER_EMAIL)
        await page.getByRole('button', { name: 'Grant super admin' }).click()

        // On success the form closes, the roster refetches, and the granted user
        // appears as a row. Before the fix this never happened — the POST 500'd
        // and the email surfaced as an inline form error instead. Asserting the
        // owner's row (not just "not empty") makes the regression signal precise.
        await expect(page.getByText(TEST_ORG_OWNER_EMAIL, { exact: true })).toBeVisible()
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

        // Sentry DSN: fill, save.
        await page.getByRole('textbox', { name: 'Sentry DSN', exact: true }).fill(TEST_DSN)
        await page.getByTestId('sentry-dsn-save').click()

        // Reload so the server re-serves app.html with the freshly-stored value
        // injected. (login goto('/admin') is the one allowed full load.)
        await page.goto('/admin')
        const injected = await page.evaluate(
            () =>
                (window as unknown as { __TINYCLD_PUBLIC_CONFIG__?: { sentryDsn?: string } })
                    .__TINYCLD_PUBLIC_CONFIG__?.sentryDsn
        )
        expect(injected).toBe(TEST_DSN)

        // VAPID: generate a keypair server-side; the panel flips to "Configured".
        await page.getByText('Settings', { exact: true }).first().click()
        await page.getByTestId('vapid-generate').click()
        await expect(page.getByText('Configured ✓')).toBeVisible()
    })
})
