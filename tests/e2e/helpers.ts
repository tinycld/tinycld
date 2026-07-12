import * as fs from 'node:fs'
import * as path from 'node:path'
import { expect, type Locator, type Page } from '@playwright/test'

export const ORG_SLUG = 'test-org'
export const TEST_USER_EMAIL = process.env.TEST_USER_LOGIN || 'user@tinycld.org'
export const TEST_USER_PASSWORD = process.env.TEST_USER_PW || 'TestUser1234!'
export const TEST_USER_USERNAME = process.env.TEST_USER_USERNAME ?? 'tester'

// isPackageLinked checks whether a given @tinycld/* package is wired into
// this core checkout. Tests that depend on package-contributed routes or
// collections should guard with `test.skip(!isPackageLinked('mail'), ...)`
// so they run when the package is linked (dev/package CI) and are skipped
// when core runs standalone (core's own CI).
export function isPackageLinked(slug: string): boolean {
    const corePackagesDir = path.resolve(import.meta.dirname, '..', '..', 'packages')
    return (
        fs.existsSync(path.join(corePackagesDir, '@tinycld', slug)) ||
        fs.existsSync(path.join(corePackagesDir, slug))
    )
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

export async function login(page: Page) {
    await page.goto('/')
    await page.getByTestId('identifier').fill(TEST_USER_EMAIL)
    await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)
    await page.getByText('Sign in', { exact: true }).last().click()
    await page.waitForURL(/\/a\//, { timeout: 15_000 })
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
// full member of ORG_SLUG with its OWN username/email/password — so a spec that
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
    await page.goto(`/a/${ORG_SLUG}/settings/members`)
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
    await expect(inviteePage.getByText(/Welcome to/i)).toBeVisible({ timeout: 10_000 })
    await inviteePage.getByTestId('name').fill('Invited Tester')
    await inviteePage.getByTestId('password').fill(user.password)
    await inviteePage.getByTestId('confirmPassword').fill(user.password)
    await inviteePage.getByText(/Set password and sign in/i).click()
    await inviteePage.waitForURL(new RegExp(`/a/${ORG_SLUG}`), { timeout: 15_000 })

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
    await page.waitForURL(/\/a\//, { timeout: 15_000 })
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
    const onTarget = new RegExp(`/a/${ORG_SLUG}/${pkg}(/|$|\\?)`)
    // Skip the rail-click when we're already on (or already redirecting to) the
    // target package. login() returns the moment the URL hits /a/<org>, but the
    // org index then <Redirect>s to the first nav package — which IS this package
    // when it's the only/first one installed (e.g. drive in its own CI, where no
    // other nav package exists). Clicking the rail in that window fires a SECOND
    // navigation to the same route while the first is still streaming its lazy
    // chunks, and that interruption wedges the screen/sidebar chunk in Metro
    // ("loaded but never committed" → the sidebar watchdog remounts for ~45s →
    // the list lands scrolled and double-clicks misfire). When we're already
    // headed there, wait for the route + the caller's element instead of
    // re-navigating.
    let alreadyThere = onTarget.test(page.url())
    // Only when sitting on the bare org index (/a/<org> with no package segment)
    // might an auto-redirect to this package be in flight — wait briefly to catch
    // it. Anywhere else we're not auto-heading here, so skip the wait (no latency
    // penalty on the common cross-package navigation) and click the rail.
    if (!alreadyThere && new RegExp(`/a/${ORG_SLUG}(/|$|\\?)?$`).test(page.url())) {
        await page
            .waitForURL(onTarget, { timeout: 2000 })
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
        // visited file (e.g. /a/<org>/calc/<id>), so the rail anchor no
        // longer matches the bare /a/<org>/<pkg> URL. Prefix match keeps
        // this working regardless of whether the rail item is bare or
        // deep-linked.
        const railLink = page.locator(`a[href^="/a/${ORG_SLUG}/${pkg}"]`).first()
        await railLink.waitFor({ state: 'visible' })
        await railLink.click()
        await page.waitForURL(onTarget)
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
    await page.waitForURL(new RegExp(`/a/${ORG_SLUG}/admin`))
    // The index redirects to /admin/packages; click the section in the sidebar.
    await page.getByText(sectionLabel, { exact: true }).click()
    await page.waitForURL(new RegExp(`/a/${ORG_SLUG}/admin/${section}`))
}
