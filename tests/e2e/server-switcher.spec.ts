import { expect, type Page, test } from '@playwright/test'
import { appShell, login, TEST_USER_EMAIL, TEST_USER_PASSWORD } from './helpers'

// The saved-server switcher, exercised through the web build.
//
// TWO deployments serve this file — the config's webServer array starts an
// independent PocketBase on each of PORT and PORT_2, with its own data dir and
// its own seeded copy of the fixture user. That is what makes the interesting
// states reachable: serverKeyFor normalizes an address to scheme+host+port, so
// two localhost PORTS are two distinct servers exactly as two self-hosted boxes
// would be. No router and no second hostname are involved.
//
// Web is deliberately the platform under test rather than a stand-in for
// native: on web a row navigates to the target origin (localStorage is
// origin-partitioned, so a browser cannot hold another origin's session),
// whereas native repoints the running app in place. The copy differs per
// platform for exactly this reason, and the web copy is what this asserts.

const PORT = Number(process.env.E2E_PORT ?? 7200)
const SECOND_PORT = Number(process.env.E2E_PORT_2 ?? 7201)
const PRIMARY = `http://localhost:${PORT}`
const SECOND = `http://localhost:${SECOND_PORT}`

const MOBILE_VIEWPORT = { width: 390, height: 844 }

function currentHost(page: Page): string {
    return new URL(page.url()).host
}

// Both deployments are seeded from the same fixture, so the same credentials
// work on each — while the SESSIONS stay separate, because localStorage is
// partitioned per origin. That is the property under test.
async function loginAt(page: Page, origin: string) {
    await page.goto(`${origin}/`)
    const identifier = page.getByTestId('identifier')
    const shell = page.getByTestId('nav-home').or(page.getByTestId('nav-more')).first()

    const landed = await Promise.race([
        identifier
            .waitFor({ state: 'visible', timeout: 20_000 })
            .then(() => 'form' as const)
            .catch(() => 'neither' as const),
        shell
            .waitFor({ state: 'visible', timeout: 20_000 })
            .then(() => 'shell' as const)
            .catch(() => 'neither' as const),
    ])
    if (landed === 'shell') return

    await identifier.fill(TEST_USER_EMAIL)
    await page.getByPlaceholder('Password').fill(TEST_USER_PASSWORD)
    await page.getByText('Sign in', { exact: true }).last().click()
    await expect(shell).toBeVisible({ timeout: 20_000 })
}

// Puts `origin` into this browser's saved-server list. The list is per-origin
// storage, so this seeds it from within the page it belongs to — the same shape
// the app itself writes (setActiveServer writes both halves).
async function saveServer(page: Page, origin: string) {
    await page.evaluate(o => {
        const raw = window.localStorage.getItem('tinycld:servers')
        const list: { origin: string; label: string; addedAt: number }[] = raw
            ? JSON.parse(raw)
            : []
        if (!list.some(s => s.origin === o)) {
            list.push({ origin: o, label: new URL(o).host, addedAt: Date.now() })
        }
        window.localStorage.setItem('tinycld:servers', JSON.stringify(list))
    }, origin)
}

async function saveBothServers(page: Page) {
    await loginAt(page, PRIMARY)
    await saveServer(page, PRIMARY)
    await saveServer(page, SECOND)
    await page.reload()
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

// The isolation the whole feature rests on: a session on one deployment is not
// a session on the other, because localStorage is partitioned per origin.
test.describe('two deployments', () => {
    test('signing into one does not sign you into the other', async ({ page }) => {
        await loginAt(page, PRIMARY)

        await page.goto(`${SECOND}/`)
        await expect(page.getByTestId('identifier')).toBeVisible({ timeout: 20_000 })
    })
})

test.describe('switcher with two servers — mobile', () => {
    test.use({ viewport: MOBILE_VIEWPORT })

    test('lists both servers, marking only the current one', async ({ page }) => {
        await saveBothServers(page)
        await openMoreDrawer(page)

        const primaryRow = page.getByTestId(`drawer-server-localhost:${PORT}`)
        const secondRow = page.getByTestId(`drawer-server-localhost:${SECOND_PORT}`)

        await expect(primaryRow).toBeVisible()
        await expect(secondRow).toBeVisible()

        await expect(primaryRow).toHaveAttribute('aria-label', /current server/i)
        await expect(secondRow).toHaveAttribute('aria-label', /^switch to/i)
    })

    // The actual switch. On web this is a navigation to the other origin —
    // native's in-place restart has no browser equivalent, which is why the row
    // says "opens in this tab" rather than promising the sessions coexist.
    test('tapping the other server lands on its origin', async ({ page }) => {
        await saveBothServers(page)
        await openMoreDrawer(page)
        await page.getByTestId(`drawer-server-localhost:${SECOND_PORT}`).click()

        await page.waitForURL(new RegExp(`:${SECOND_PORT}`), { timeout: 20_000 })
        expect(new URL(page.url()).host).toBe(`localhost:${SECOND_PORT}`)
    })

    // Arriving on the other origin as a stranger is correct, not a bug: the
    // browser cannot carry a session across origins. Pinned so the honest copy
    // ("You may need to sign in there") stays honest.
    test('lands signed out when the other origin has no session', async ({ page }) => {
        await saveBothServers(page)
        await openMoreDrawer(page)
        await page.getByTestId(`drawer-server-localhost:${SECOND_PORT}`).click()
        await page.waitForURL(new RegExp(`:${SECOND_PORT}`), { timeout: 20_000 })

        await expect(page.getByTestId('identifier')).toBeVisible({ timeout: 20_000 })
    })

    // Both sessions surviving independently is the native promise; on web the
    // equivalent is that going BACK finds you still signed in to the first one.
    test('the original server is still signed in after visiting the other', async ({ page }) => {
        await loginAt(page, PRIMARY)
        await saveServer(page, PRIMARY)
        await saveServer(page, SECOND)

        await page.goto(`${SECOND}/`)
        await expect(page.getByTestId('identifier')).toBeVisible({ timeout: 20_000 })

        await page.goto(`${PRIMARY}/`)
        const shell = page.getByTestId('nav-home').or(page.getByTestId('nav-more')).first()
        await expect(shell).toBeVisible({ timeout: 20_000 })
    })
})

// The desktop counterpart, at the config's default (wide) viewport so the rail +
// user menu render instead of the tab bar + drawer.
//
// UserMenu's section is gated on `servers.length > 1` — a lone entry is just a
// label for where you already are — so only a SECOND server reaches the state
// where it matters. The mobile blocks above exercise a different component
// entirely (ServersDrawerSection).
test.describe('switcher on desktop — user menu', () => {
    // Menu rows and section labels must be matched WITHIN the menu, not
    // page-wide. The workspace behind the open popover has its own "Settings"
    // and "Servers" text — a feature-less assembly lands on the settings screen,
    // where "Servers" is a nav row — so a bare getByText matches two elements
    // and fails on strict mode. Menu items carry role="menuitem"; section labels
    // are the only uppercase muted text in the popover.
    function menuItem(page: Page, label: string) {
        return page.locator('[role="menuitem"]', { hasText: label })
    }

    function menuLabel(page: Page, label: string) {
        return page.locator('.uppercase.text-muted-foreground', { hasText: label })
    }

    async function openUserMenu(page: Page) {
        await expect(page.getByTestId('nav-home')).toBeVisible({ timeout: 20_000 })
        await page.getByLabel('User menu').click()
        // Settings is a stable neighbour in the same menu — gate on it so we do
        // not race the popover's open animation.
        await expect(menuItem(page, 'Settings')).toBeVisible()
    }

    // A single saved server is nothing to switch between, so the menu section
    // stays hidden. Pinning this stops the section reappearing as a one-row
    // list that only names where you already are.
    test('stays hidden when there is only one server to choose from', async ({ page }) => {
        await login(page)
        await expect(page.getByTestId('nav-home')).toBeVisible()

        await page.getByLabel('User menu').click()
        await expect(page.getByText('Settings', { exact: true })).toBeVisible()

        await expect(page.getByText('Servers', { exact: true })).toHaveCount(0)
    })

    test('lists both servers, marking only the current one', async ({ page }) => {
        await saveBothServers(page)
        await openUserMenu(page)

        await expect(menuLabel(page, 'Servers')).toBeVisible()

        // Web rows carry an href (Menu.Item renders a real <a role="menuitem">),
        // which is what makes middle-click / open-in-new-tab work — so target by
        // it rather than by label text.
        const primaryRow = page.locator(`a[role="menuitem"][href="${PRIMARY}"]`)
        const secondRow = page.locator(`a[role="menuitem"][href="${SECOND}"]`)
        await expect(primaryRow).toBeVisible()
        await expect(secondRow).toBeVisible()

        // The active row gets a Check; the other does not. One check across the
        // two rows is the assertion — "exactly one marked current".
        await expect(primaryRow.locator('svg')).toHaveCount(2) // Server icon + Check
        await expect(secondRow.locator('svg')).toHaveCount(1) // Server icon only
    })

    test('choosing the other server navigates to its origin', async ({ page }) => {
        await saveBothServers(page)
        await openUserMenu(page)
        await page.locator(`a[role="menuitem"][href="${SECOND}"]`).click()

        await page.waitForURL(new RegExp(`:${SECOND_PORT}`), { timeout: 20_000 })
        expect(new URL(page.url()).host).toBe(`localhost:${SECOND_PORT}`)
    })
})
