import * as fs from 'node:fs'
import * as path from 'node:path'
import { expect, type Locator, type Page } from '@playwright/test'

export const TEST_USER_EMAIL = process.env.TEST_USER_LOGIN || 'user@tinycld.org'

// Single-org: the `app/a/[orgSlug]/` route segment was collapsed to `app/(app)/`
// (an invisible route group), so authenticated URLs no longer carry an `/a/<org>`
// prefix — they are bare `/contacts`, `/settings/...`, `/admin/...`. LANDED_URL
// matches "we're inside the authenticated app shell" (any of those sections, or
// the bare root while the index redirect to the first package is mid-flight).
// `shortcut-stub` is in the list because the app-shell CI job assembles app +
// core ONLY and scaffolds the stub, making it the sole nav package — so the
// post-login index redirect lands on /shortcut-stub. Omitting it made every
// waitForURL(LANDED_URL) time out on CI while passing locally (where the real
// feature packages are present and the redirect lands on /contacts).
const APP_SECTIONS = 'contacts|settings|admin|help|mail|drive|calendar|calc|text|shortcut-stub'
export const LANDED_URL = new RegExp(`/(?:${APP_SECTIONS})(?:/|$|\\?)`)
export const TEST_USER_PASSWORD = process.env.TEST_USER_PW || 'TestUser1234!'
export const TEST_USER_USERNAME = process.env.TEST_USER_USERNAME ?? 'tester'

// isPackageLinked checks whether a given feature package is present in this
// workspace. Tests that depend on package-contributed routes or collections
// should guard with `test.skip(!isPackageLinked('mail'), ...)` so they run when
// the package is present (dev/package CI) and skip when the app shell runs
// standalone (its own CI).
//
// A feature is "present" when its sibling member dir exists at the workspace
// root and carries a manifest.ts (the marker that makes a dir a member — see
// tinycld.packages.ts). The old `<checkout>/packages/@tinycld/<slug>` layout is
// gone: features are now flat sibling dirs under the workspace root, resolved
// the same way as shortcutStubInstalled() below.
export function isPackageLinked(slug: string): boolean {
    // tinycld/tests/e2e/helpers.ts → tinycld/tests → tinycld → workspace root → <slug>/manifest.ts
    const manifest = path.resolve(import.meta.dirname, '..', '..', '..', slug, 'manifest.ts')
    return fs.existsSync(manifest)
}

// The keyboard-shortcut and offline-overlay specs drive a minimal stub package
// (shortcut-stub) scaffolded by tests/scripts/scaffold-shortcut-stub.ts. The scaffold
// writes the package at <workspaceRoot>/shortcut-stub (sibling of tinycld/).
export function shortcutStubInstalled(): boolean {
    // tinycld/tests/e2e/helpers.ts → tinycld/tests → tinycld → workspace root → shortcut-stub/
    const stubDir = path.resolve(import.meta.dirname, '..', '..', '..', 'shortcut-stub')
    return fs.existsSync(stubDir)
}

// Guard for the stub-dependent specs. A plain dev workspace usually hasn't
// scaffolded shortcut-stub, so those specs skip (their nav entry + landing route
// are absent and they'd otherwise hang). On CI the scaffold step is mandatory
// (ci.yml "Scaffold shortcut-stub package" runs before e2e) — so we DON'T allow a
// silent skip there: if the stub is missing on CI the scaffold step regressed, and
// the specs should fail loudly rather than vanish into a green run. Returns the
// `test.skip` condition (true = skip); throws on CI when the stub is absent.
export function skipWithoutShortcutStub(): boolean {
    if (shortcutStubInstalled()) return false
    if (process.env.CI) {
        throw new Error(
            'shortcut-stub is not scaffolded but CI is set — the "Scaffold shortcut-stub package" step must run before e2e. Refusing to silently skip stub-dependent specs on CI.'
        )
    }
    return true
}

// The sign-in POST occasionally returns a transient 400 under parallel CI
// load (PocketBase auto-cancels an overlapping auth request, or the expand
// query races), surfacing as an inline "Failed to authenticate." with the
// form still mounted — the redirect never fires and a single click wedges
// the whole test. Rather than a single fire-and-hope click, submit and then
// race the post-login redirect against that error banner; on the error,
// resubmit. Credentials are correct (the same ones log in across the suite),
// so a bounded retry deterministically clears the race without touching the
// timeout.
export async function login(page: Page) {
    await page.goto('/')
    // Idempotent: a helper earlier in the same test (createInvitedUser) may
    // have left the fixture session authenticated, in which case '/' redirects
    // straight into the shell and NO login form exists — filling it would hang
    // until the test budget dies. Race the two possible landings and
    // short-circuit when the shell is already up. (Cross-user sign-in goes
    // through loginAs, which clears storage first — this helper only ever
    // establishes the fixture user.)
    const identifierField = page.getByTestId('identifier')
    const landed = await Promise.race([
        identifierField
            .waitFor({ state: 'visible', timeout: 15_000 })
            .then(() => 'login-form' as const)
            .catch(() => 'neither' as const),
        page
            .getByTestId('nav-home')
            .waitFor({ state: 'visible', timeout: 15_000 })
            .then(() => 'shell' as const)
            .catch(() => 'neither' as const),
    ])
    if (landed === 'shell') return

    await identifierField.fill(TEST_USER_EMAIL)
    await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)

    // "We're in" = the authenticated app shell mounted. Single-org: post-login
    // lands on the bare root `/`, which then client-side <Redirect>s to the first
    // nav package — a SPA transition that doesn't fire a `load` event and whose
    // interim URL is the section-less `/`, so a waitForURL(LANDED_URL) hangs.
    // Gate on the package rail (always present in the shell) instead — it's
    // timing- and route-independent. nav-home is the workspace rail's home entry.
    const shellReady = page.getByTestId('nav-home')
    const authError = page.getByText('Failed to authenticate', { exact: false })
    const maxAttempts = 3
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        await page.getByText('Sign in', { exact: true }).last().click()
        // Whichever resolves first wins: the shell mounting means we're in.
        const outcome = await Promise.race([
            shellReady
                .waitFor({ state: 'visible', timeout: 15_000 })
                .then(() => 'ok' as const)
                .catch(() => 'timeout' as const),
            authError
                .waitFor({ state: 'visible', timeout: 15_000 })
                .then(() => 'error' as const)
                .catch(() => 'timeout' as const),
        ])
        if (outcome === 'ok') return
        if (outcome === 'error' && attempt < maxAttempts) continue
        // Timed out with no shell and no error banner, or exhausted retries: one
        // last direct wait so the failure surfaces with a clear message + screenshot.
        await shellReady.waitFor({ state: 'visible', timeout: 15_000 })
        return
    }
}

export interface InvitedUser {
    username: string
    email: string
    password: string
}

// Provision a fresh org member via the invite flow, in an isolated browser
// context, and return its credentials. Signup is disabled, so the only way to
// mint a throwaway account is an owner invite: the owner (shared TEST_USER)
// sends an invite, the invitee accepts and sets a password. The invitee is a
// full member of the deployment with its OWN username/email/password — so a spec that
// mutates a password (change-password, password-reset) can operate on this
// account without touching TEST_USER_PASSWORD, which every other spec's login()
// depends on. Runs in its own context so the owner's auth doesn't leak in and
// the returned page can be used (or discarded) independently.
//
// The credentials are unique per call (Date.now() + a caller label) because the
// e2e DB is reset across runs but NOT between tests. Pass an `email` so the
// account can drive email-addressed flows (e.g. password reset by email).
export async function createInvitedUser(
    page: Page,
    label: string
): Promise<{ user: InvitedUser; inviteePage: Page; close: () => Promise<void> }> {
    const stamp = Date.now()
    const user: InvitedUser = {
        username: `${label}${stamp}`,
        email: `${label}-${stamp}@example.com`,
        password: 'InvitedPass1!',
    }

    // --- Owner sends the invite (username + email) from Settings → Members ---
    await login(page)
    await navigateToPackage(page, 'settings')
    // SPA-click into Members — a page.goto here is the hard-nav this file's
    // own docs forbid (tears down the SPA and cancels in-flight chunk loads).
    await page.getByText('Members', { exact: true }).first().click()
    await page.waitForURL(/\/settings\/members/)
    await page.getByText('Invite', { exact: true }).click()
    await expect(page.getByText('Invite a teammate', { exact: true })).toBeVisible({
        timeout: 10_000,
    })
    await page.getByTestId('username').fill(user.username)
    await page.getByTestId('email').fill(user.email)
    await page.getByText('Send invite', { exact: true }).click()

    // The invite-link panel surfaces the accept URL directly (no auto-email).
    await expect(page.getByTestId('invite-link-step')).toBeVisible({ timeout: 10_000 })
    const urlText = await page.getByTestId('invite-link-url').textContent()
    const tokenMatch = urlText?.match(/\/accept-invite\/([0-9a-f]{64})/)
    if (!tokenMatch) throw new Error(`invite link had no token: ${urlText}`)
    await page.getByTestId('invite-link-done').click()

    // --- Invitee accepts in a fresh context and sets its password ---
    const inviteeContext = await page.context().browser()!.newContext()
    const inviteePage = await inviteeContext.newPage()
    await inviteePage.goto(`/accept-invite/${tokenMatch[1]}`)
    await expect(inviteePage.getByText("You're invited", { exact: true })).toBeVisible({
        timeout: 10_000,
    })
    await inviteePage.getByTestId('name').fill('Invited Tester')
    await inviteePage.getByTestId('password').fill(user.password)
    await inviteePage.getByTestId('confirmPassword').fill(user.password)
    await inviteePage.getByText(/Set password and sign in/i).click()
    await inviteePage.waitForURL(LANDED_URL, { timeout: 15_000, waitUntil: 'commit' })

    return { user, inviteePage, close: () => inviteeContext.close() }
}

// Sign in as a specific (non-fixture) user by identifier + password. Mirrors
// login() but for accounts minted via createInvitedUser.
export async function loginAs(page: Page, identifier: string, password: string) {
    await page.evaluate(() => {
        window.localStorage.clear()
        window.sessionStorage.clear()
    })
    await page.goto('/')
    await page.getByTestId('identifier').fill(identifier)
    await page.getByPlaceholder('Password').fill(password)
    await page.getByText('Sign in', { exact: true }).last().click()
    await page.waitForURL(LANDED_URL, { timeout: 15_000, waitUntil: 'commit' })
}

// Navigate to a package's org-scoped route via the rail link in the app
// shell. We click the rail link rather than calling page.goto() because
// goto is a hard browser navigation that cancels every in-flight fetch,
// including any lazy chunk the previous route had already started
// loading. On CI that cancellation triggers a 5+ second retry/recompile
// cycle inside Metro, and the package's screen chunk (lazy() in
// tinycld.config.ts) doesn't settle until after the test's first
// assertion has already timed out. Clicking does SPA navigation through
// expo-router: previously-loaded chunks stay loaded, the new package's
// chunk downloads cleanly without contention, and the page never tears
// down + remounts.
//
// Returns once the URL has changed. Callers that need to wait for a
// specific element (sidebar mount, an inbox row, a button) can pass
// an opt-in `waitFor` Locator and the helper forwards it to
// `locator.waitFor({ state: 'visible' })`. Examples:
//
//     await navigateToPackage(page, 'mail', {
//         waitFor: page.getByRole('button', { name: 'Compose' }),
//     })
//     await navigateToPackage(page, 'drive', {
//         waitFor: page.getByTestId('package-sidebar-mounted'),
//     })
//
// No default wait — layouts vary (MobileLayout in core doesn't
// render PackageSidebar, packages like text/calc don't contribute
// one), so there's no universal post-navigation signal. The caller
// knows which UI it's about to interact with; let it gate on that.
//
// `pkg` is the lowercase slug (mail, calendar, drive, ...).
export async function navigateToPackage(page: Page, pkg: string, options?: { waitFor?: Locator }) {
    // Single-org: routes are bare `/<pkg>` (the /a/<org> segment is gone).
    const onTarget = new RegExp(`/${pkg}(/|$|\\?)`)
    // Skip the rail-click when we're already on (or already redirecting to) the
    // target package. login() returns the moment the URL hits an app section, but
    // the index then <Redirect>s to the first nav package — which IS this package
    // when it's the only/first one installed (e.g. contacts in the lean shell).
    // Clicking the rail in that window fires a SECOND navigation to the same route
    // while the first is still streaming its lazy chunks, and that interruption
    // wedges the screen/sidebar chunk in Metro ("loaded but never committed" → the
    // sidebar watchdog remounts for ~45s → the list lands scrolled and
    // double-clicks misfire). When we're already headed there, wait for the route
    // + the caller's element instead of re-navigating.
    let alreadyThere = onTarget.test(page.url())
    // Only when sitting on the bare root (the index that redirects to the first
    // package) might an auto-redirect to this package be in flight — wait briefly
    // to catch it. Anywhere else we're not auto-heading here, so skip the wait (no
    // latency penalty on the common cross-package navigation) and click the rail.
    if (!alreadyThere && new URL(page.url()).pathname === '/') {
        // waitUntil: 'commit' — the index→package redirect is a SPA transition
        // that never fires a `load` event, so the default wait would time out.
        await page
            .waitForURL(onTarget, { timeout: 2000, waitUntil: 'commit' })
            .then(() => {
                alreadyThere = true
            })
            .catch(() => {
                /* redirect went elsewhere; fall through to the rail click */
            })
    }
    if (!alreadyThere) {
        // Match by URL prefix rather than exact href: some packages (calc,
        // text, …) rewrite their rail link to deep-link the user's last
        // visited file (e.g. /calc/<id>), so the rail anchor no longer matches
        // the bare /<pkg> URL. Prefix match keeps this working regardless of
        // whether the rail item is bare or deep-linked.
        const railLink = page.locator(`a[href^="/${pkg}"]`).first()
        await railLink.waitFor({ state: 'visible' })
        await railLink.click()
        // waitUntil: 'commit' — SPA rail navigation doesn't fire a `load` event.
        await page.waitForURL(onTarget, { waitUntil: 'commit' })
    }
    if (options?.waitFor) {
        await options.waitFor.waitFor({ state: 'visible' })
    }
}

export async function clickSidebarItem(page: Page, label: string) {
    await page.getByText(label, { exact: true }).click()
}

// Enter the in-shell Admin console (super-admin only) and land on a section.
// Requires a logged-in super-admin session (the e2e seed grants the test user
// super_admins). Clicks the rail entry rather than page.goto so the SPA + its
// lazy chunks stay warm, then clicks the section in the AdminSidebar.
//
// `section` is one of: 'organizations' | 'packages' | 'builds' | 'super-admins'.
export async function navigateToAdmin(page: Page, section: string, sectionLabel: string) {
    const rail = page.getByTestId('nav-admin')
    await rail.waitFor({ state: 'visible', timeout: 15_000 })
    await rail.click()
    await page.waitForURL(/\/admin(\/|$|\?)/)
    // The index redirects to /admin/packages; click the section in the sidebar.
    await page.getByText(sectionLabel, { exact: true }).click()
    await page.waitForURL(new RegExp(`/admin/${section}`))
}
