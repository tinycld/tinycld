import * as fs from 'node:fs'
import * as path from 'node:path'
import type { Page } from '@playwright/test'

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

export async function login(page: Page) {
    await page.goto('/')
    await page.getByTestId('identifier').fill(TEST_USER_EMAIL)
    await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)
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
// Returns after the URL change is observed. Callers that need to wait
// for a specific page element (e.g. a sidebar entry, an inbox row, a
// button) MUST assert on it explicitly — this helper deliberately
// doesn't wait on package-internal UI so it works for any package,
// including ones with no sidebar.
//
// `pkg` is the lowercase slug (mail, calendar, drive, ...).
export async function navigateToPackage(page: Page, pkg: string) {
    // Match by URL prefix rather than exact href: some packages (calc,
    // text, …) rewrite their rail link to deep-link the user's last
    // visited file (e.g. /a/<org>/calc/<id>), so the rail anchor no
    // longer matches the bare /a/<org>/<pkg> URL. Prefix match keeps
    // this working regardless of whether the rail item is bare or
    // deep-linked.
    const railLink = page.locator(`a[href^="/a/${ORG_SLUG}/${pkg}"]`).first()
    await railLink.waitFor({ state: 'visible' })
    await railLink.click()
    await page.waitForURL(new RegExp(`/a/${ORG_SLUG}/${pkg}(/|$|\\?)`))
}

export async function clickSidebarItem(page: Page, label: string) {
    await page.getByText(label, { exact: true }).click()
}
