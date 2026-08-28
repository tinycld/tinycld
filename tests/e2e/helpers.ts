import * as fs from 'node:fs'
import * as path from 'node:path'
import { expect, type Locator, type Page } from '@playwright/test'

export const TEST_USER_EMAIL = process.env.TEST_USER_LOGIN || 'user@tinycld.org'

// DEPRECATED — use appShell(page) instead.
//
// LANDED_URL matches "we're inside the authenticated app shell" by URL. It is
// kept only because it is re-exported from @tinycld/core/e2e-helpers and a
// package outside this repo may still import it; nothing here uses it.
//
// Two things make it a poor gate, both learned the hard way. It hardcodes a
// package list, so the day the app-shell CI job assembled without those
// packages every wait timed out (hence `shortcut-stub` in the list). And a URL
// changes when the router ACCEPTS a navigation, not when the screen commits —
// so waits on it race a still-loading chunk, and when the route shape shifts
// underneath them they hang for the full timeout while the app works fine.
// appShell()/packageScreen() assert what the user can actually see.
const APP_SECTIONS = 'contacts|settings|help|mail|drive|calendar|calc|text|shortcut-stub'
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
// The authenticated app shell, whichever chrome this viewport renders:
// PackageRail (nav-home) on tablet/desktop, MobileTabBar (nav-more) on mobile.
//
// Prefer this over waitForURL(LANDED_URL) for "did we get into the app". A URL
// changes when the router accepts a push — before the target screen's chunk has
// loaded, let alone committed — so URL-gated waits both race and, when the
// route shape shifts underneath them, hang for the full timeout while the app
// sits there working perfectly. LANDED_URL additionally hardcoded a package
// list, which broke CI the day the shell assembled without those packages.
export function appShell(page: Page): Locator {
    return page.getByTestId('nav-home').or(page.getByTestId('nav-more')).first()
}

// The screen for `pkg` is mounted (not merely routed to). Stamped by the
// workspace layout from activePkgSlug, which app/(app)/_layout sets after the
// route commits.
export function packageScreen(page: Page, pkg: string): Locator {
    return page.getByTestId(`pkg-active-${pkg}`)
}

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
    // Gate on the shell chrome instead — timing- and route-independent.
    //
    // WHICH chrome depends on the viewport: WorkspaceLayout renders MobileLayout
    // (MobileTabBar, testID nav-more) below the mobile breakpoint and PackageRail
    // (testID nav-home) at or above it. Gating on nav-home alone made login()
    // unusable in any mobile-viewport spec — it waited out the full timeout for
    // an element that layout never renders. Accept either.
    const shellReady = appShell(page)
    // Match EITHER message the sign-in form can render for a failed attempt.
    // 'Failed to authenticate' is the clean credential rejection; 'Something
    // went wrong' is the generic fallback in core/lib/errors.ts, which is what
    // a TRANSIENT failure under parallel load actually produces. Watching only
    // the first meant the common case never retried: the race below resolved
    // 'timeout' after 15s, fell through to a final 15s wait, and blew the 30s
    // test budget in beforeEach — taking every worker that hit the same moment
    // with it (three at once, in the run that surfaced this).
    const authError = page
        .getByText('Failed to authenticate', { exact: false })
        .or(page.getByText('Something went wrong', { exact: false }))
        .first()
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
        if (outcome === 'error' && attempt < maxAttempts) {
            // Wait out the visible error before re-clicking. It stays on screen
            // after a failed attempt, so an immediate retry would race the
            // banner it just saw and resolve 'error' again instantly, burning
            // every attempt in a few milliseconds without a real retry.
            await authError.waitFor({ state: 'hidden', timeout: 5_000 }).catch(() => {})
            continue
        }
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

// The second member the seed provisions ready-to-use (scripts/seed-db.ts,
// COLLABORATOR_DEFAULTS). These MUST match that file: playwright.config.ts
// never loads .env, so on a clean checkout the seed and this helper can only
// agree via a shared literal. The env vars are the CI override for both sides.
export const TEST_COLLABORATOR_EMAIL =
    process.env.TEST_COLLABORATOR_LOGIN || 'collaborator@tinycld.org'
export const TEST_COLLABORATOR_USERNAME = process.env.TEST_COLLABORATOR_USERNAME || 'collaborator'
// The DISPLAY name, which is what collaborative UI (presence avatars, caret
// labels) actually renders — so specs assert against this rather than a literal.
export const TEST_COLLABORATOR_NAME = 'Collaborator Tester'
export const TEST_COLLABORATOR_PASSWORD =
    process.env.TEST_COLLABORATOR_PW || process.env.TEST_USER_PW || TEST_USER_PASSWORD

/**
 * A SECOND signed-in person, in their own browser context.
 *
 * Use this whenever a spec needs "somebody else" — a viewer to gate, a
 * collaborator to see a live edit, a non-member to refuse. It signs in the
 * pre-seeded collaborator account, which costs ONE login (~1.4s) against
 * createInvitedUser's ~5.8s: that helper drives the entire invite arc through
 * the UI (two full app boots) and leaves the owner's page stranded on Settings,
 * forcing every caller into a second login() just to get back to their board.
 * That setup cost — not the assertions — is what made seven cards specs carry
 * test.slow() and then time out on CI.
 *
 * Reach for createInvitedUser instead ONLY when the invite flow itself is under
 * test, or when the spec MUTATES the account's credentials (change-password,
 * password-reset): this account is shared across the run, so changing its
 * password would break every later spec that signs in with it.
 *
 * The caller's own `page` is untouched — no re-login needed afterwards.
 *
 *     const { page: bobPage, user: bob, close } = await signInAsCollaborator(page)
 *     try { ... } finally { await close() }
 */
export async function signInAsCollaborator(page: Page): Promise<{
    page: Page
    user: InvitedUser
    close: () => Promise<void>
}> {
    const context = await page.context().browser()!.newContext()
    const collaboratorPage = await context.newPage()
    await loginAs(collaboratorPage, TEST_COLLABORATOR_EMAIL, TEST_COLLABORATOR_PASSWORD)
    return {
        page: collaboratorPage,
        user: {
            username: TEST_COLLABORATOR_USERNAME,
            email: TEST_COLLABORATOR_EMAIL,
            password: TEST_COLLABORATOR_PASSWORD,
        },
        close: () => context.close(),
    }
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
    // No URL wait: the very next getByText auto-waits for 'Invite' to be
    // actionable, which proves the Members screen rendered. The URL only
    // proves the router accepted the push.
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
    // Deliberately the LEGACY unprefixed path: invite links minted before app
    // routes moved under /a are still arriving from inboxes, so this doubles as
    // the end-to-end proof that legacyAppRedirect (core/server/coreserver/
    // static.go) forwards them to /a/accept-invite/<token>. invite-flow.spec.ts
    // covers the current shape.
    await inviteePage.goto(`/accept-invite/${tokenMatch[1]}`)
    await expect(inviteePage.getByText("You're invited", { exact: true })).toBeVisible({
        timeout: 10_000,
    })
    await inviteePage.getByTestId('name').fill('Invited Tester')
    await inviteePage.getByTestId('password').fill(user.password)
    await inviteePage.getByTestId('confirmPassword').fill(user.password)
    await inviteePage.getByText(/Set password and sign in/i).click()
    await appShell(inviteePage).waitFor({ state: 'visible', timeout: 15_000 })

    return { user, inviteePage, close: () => inviteeContext.close() }
}

// Sign in as a specific (non-fixture) user by identifier + password. Mirrors
// login() but for accounts minted via createInvitedUser.
export async function loginAs(page: Page, identifier: string, password: string) {
    // goto BEFORE clearing: storage is origin-scoped, and a freshly created
    // context sits on about:blank, where reading localStorage throws
    // SecurityError. Navigating first gives the page a real origin; the
    // reload then re-mounts the app against the cleared store.
    await page.goto('/')
    await page.evaluate(() => {
        window.localStorage.clear()
        window.sessionStorage.clear()
    })
    await page.reload()
    await page.getByTestId('identifier').fill(identifier)
    await page.getByPlaceholder('Password').fill(password)
    await page.getByText('Sign in', { exact: true }).last().click()
    await appShell(page).waitFor({ state: 'visible', timeout: 15_000 })
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
    // Gate on the SCREEN, not the URL. `pkg-active-<slug>` is stamped by the
    // workspace layout from activePkgSlug, which app/(app)/_layout sets only
    // after the route commits — so it means "this package is mounted". A URL
    // changes at the START of a SPA transition, while the target's lazy chunk
    // is still loading, which is why the URL-based version needed
    // `waitUntil: 'commit'` everywhere and still raced.
    //
    // settings/help are app chrome, not packages: app/(app)/_layout maps them
    // to activePkgSlug=null on purpose (they leave the rail highlight where it
    // was), so no `pkg-active-*` marker ever appears for them. Gate those on
    // their own heading instead — waiting for the marker would hang forever.
    const CHROME_HEADINGS: Record<string, string> = { settings: 'Settings', help: 'Help' }
    const chromeHeading = CHROME_HEADINGS[pkg]
    const onTarget = chromeHeading
        ? page.getByText(chromeHeading, { exact: true }).first()
        : packageScreen(page, pkg)

    // Don't navigate if we're already there, or if we're about to be. login()
    // returns as soon as the shell mounts, but the index then <Redirect>s to
    // the first nav package — which IS this package when it's the only one
    // installed. Clicking the rail in that window fires a SECOND navigation to
    // the same route while the first is still streaming its chunks, and that
    // interruption wedges the screen/sidebar chunk in Metro ("loaded but never
    // committed" → the sidebar watchdog remounts for ~45s → the list lands
    // scrolled and double-clicks misfire).
    //
    // The short wait only applies on the bare root, where that redirect is the
    // thing possibly in flight; anywhere else nothing is auto-heading here, so
    // a miss costs nothing and we click the rail.
    const redirectMayBeInFlight = new URL(page.url()).pathname === '/'
    const alreadyThere = await onTarget
        .waitFor({ state: 'visible', timeout: redirectMayBeInFlight ? 2000 : 1 })
        .then(
            () => true,
            () => false
        )

    if (!alreadyThere) {
        // Match by URL prefix rather than exact href: some packages (calc,
        // text, …) rewrite their rail link to deep-link the user's last visited
        // file (e.g. /calc/<id>), so the anchor no longer matches bare /<pkg>.
        const railLink = page.locator(`a[href^="/${pkg}"]`).first()
        await railLink.waitFor({ state: 'visible' })
        await railLink.click()
        await onTarget.waitFor({ state: 'visible' })
    }

    if (options?.waitFor) {
        await options.waitFor.waitFor({ state: 'visible' })
    }
}

export async function clickSidebarItem(page: Page, label: string) {
    await page.getByText(label, { exact: true }).click()
}
