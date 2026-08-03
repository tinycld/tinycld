import { expect, type Page, test } from '@playwright/test'
import { appShell, login } from './helpers'

// The saved-server switcher, exercised through the web build.
//
// SCOPE, stated up front so the gaps are not mistaken for coverage: this file
// runs against the ordinary single-origin e2e stack, so it can only reach the
// states one origin can produce — the section renders, the current server is
// listed and marked active, and the drawer's other rows survive alongside it.
// It CANNOT exercise an actual cross-origin switch, because that needs two
// servers on two hostnames. That case belongs to the multi-org harness.
//
// Web is deliberately the platform under test here rather than a stand-in for
// native: on web a row navigates to the target origin (localStorage is
// origin-partitioned, so a browser cannot hold another origin's session),
// whereas native repoints the running app in place. The copy differs per
// platform for exactly this reason, and the web copy is what this asserts.

const MOBILE_VIEWPORT = { width: 390, height: 844 }

// The one origin this stack serves — the switcher's sole entry here.
function currentHost(page: Page): string {
    return new URL(page.url()).host
}

async function openMoreDrawer(page: Page) {
    await page.getByTestId('nav-more').click()
    // The drawer animates in; gate on a row rather than the container so we
    // don't race the slide.
    await expect(page.getByText('Sign out', { exact: true })).toBeVisible()
}

test.describe('server switcher — mobile More drawer', () => {
    test.use({ viewport: MOBILE_VIEWPORT })

    test('lists the current server, marked as current', async ({ page }) => {
        await login(page)
        await expect(appShell(page)).toBeVisible()
        await openMoreDrawer(page)

        await expect(page.getByTestId('drawer-servers-label')).toBeVisible()

        // The row is labelled by hostname, and is the active one — so it is
        // non-interactive and announces itself as current.
        const row = page.getByTestId(`drawer-server-${currentHost(page)}`)
        await expect(row).toBeVisible()
        await expect(row).toHaveAttribute('aria-label', /current server/i)
    })

    // The web build cannot keep a session on another origin, so it must not
    // repeat the native promise that the others stay signed in.
    test('tells the truth about what switching does on web', async ({ page }) => {
        await login(page)
        await openMoreDrawer(page)

        await expect(page.getByText(/opens that server in this tab/i)).toBeVisible()
        await expect(page.getByText(/sign you out of the others/i)).toHaveCount(0)
    })

    test('offers a way to add another server', async ({ page }) => {
        await login(page)
        await openMoreDrawer(page)

        await expect(page.getByTestId('drawer-add-server')).toBeVisible()
    })

    // Regression guard for the ScrollView added to MoreDrawer. BottomDrawer caps
    // its height at 85% of the screen with no internal scroller, and clips from
    // the BOTTOM — where Sign out lives. Adding the Servers section pushed the
    // content past that cap on a phone viewport, so without the ScrollView this
    // row becomes unreachable.
    test('Sign out is still reachable below the servers section', async ({ page }) => {
        await login(page)
        await openMoreDrawer(page)

        const signOut = page.getByText('Sign out', { exact: true })
        await signOut.scrollIntoViewIfNeeded()
        await expect(signOut).toBeInViewport()
    })
})

test.describe('server switcher — desktop user menu', () => {
    // A single saved server is nothing to switch between, so the menu section
    // stays hidden — the same `> 1` reasoning the org switcher uses. Pinning
    // this stops the section reappearing as a one-row list that only names
    // where you already are.
    test('stays hidden when there is only one server to choose from', async ({ page }) => {
        await login(page)
        await expect(page.getByTestId('nav-home')).toBeVisible()

        await page.getByLabel('User menu').click()
        await expect(page.getByText('Settings', { exact: true })).toBeVisible()

        await expect(page.getByText('Servers', { exact: true })).toHaveCount(0)
    })
})
