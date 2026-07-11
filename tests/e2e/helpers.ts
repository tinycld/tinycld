import * as fs from 'node:fs'
import * as path from 'node:path'
import type { Locator, Page } from '@playwright/test'

export const ORG_SLUG = 'test-org'
export const TEST_USER_EMAIL = process.env.TEST_USER_LOGIN || 'user@tinycld.org'
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
    await page.getByTestId('identifier').fill(TEST_USER_EMAIL)
    await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)

    const authError = page.getByText('Failed to authenticate', { exact: false })
    const maxAttempts = 3
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        await page.getByText('Sign in', { exact: true }).last().click()
        // Whichever resolves first wins: a URL change means we're in.
        const outcome = await Promise.race([
            page
                .waitForURL(/\/a\//, { timeout: 15_000 })
                .then(() => 'ok' as const)
                .catch(() => 'timeout' as const),
            authError
                .waitFor({ state: 'visible', timeout: 15_000 })
                .then(() => 'error' as const)
                .catch(() => 'timeout' as const),
        ])
        if (outcome === 'ok' || /\/a\//.test(page.url())) return
        if (outcome === 'error' && attempt < maxAttempts) continue
        // Timed out with no redirect and no error banner, or exhausted retries:
        // give the redirect one last direct wait so the failure surfaces with a
        // clear message and a fresh screenshot.
        await page.waitForURL(/\/a\//, { timeout: 15_000 })
        return
    }
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
