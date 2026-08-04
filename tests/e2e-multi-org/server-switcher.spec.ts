import { expect, type Page, test } from '@playwright/test'
import {
    TEST_USER_EMAIL as SEEDED_EMAIL,
    TEST_USER_PASSWORD as SEEDED_PASSWORD,
} from '../e2e/helpers'

// Two stacks can serve these specs, and they establish their user differently:
//
//  - the local launcher (scripts/e2e-multi-org.ts) seeds both orgs with
//    reset-dev-db.ts, so the ordinary fixture user exists;
//  - multi-org's TestHostedBrowserE2E provisions real orgs and creates a
//    superuser in each tenant's own DB, passing those credentials in.
//
// Env wins when present, so the same specs cover both without branching.
const LOGIN_EMAIL = process.env.E2E_MULTI_ORG_EMAIL || SEEDED_EMAIL
const LOGIN_PASSWORD = process.env.E2E_MULTI_ORG_PASSWORD || SEEDED_PASSWORD

// The saved-server switcher across TWO orgs — the case no single-origin stack
// can produce.
//
// On the hosted router each org is its own subdomain, its own process and its
// own database. Two orgs are therefore the same object the feature calls "two
// servers": serverKeyFor normalizes to scheme+host+port, so two subdomains get
// distinct keys just as two self-hosted boxes would.
//
// What this pins that the single-origin spec cannot: a second server actually
// appearing in the list, the active one being identified correctly among
// several, and a switch really landing on the other origin.

const PORT = Number(process.env.E2E_MULTI_ORG_PORT ?? 7300)
const ACME = `http://acme.localhost:${PORT}`
const GLOBEX = `http://globex.localhost:${PORT}`

const MOBILE_VIEWPORT = { width: 390, height: 844 }

// Both orgs are seeded from the same fixture, so the same credentials work on
// each — while the SESSIONS stay separate, because localStorage is partitioned
// per origin. That is the property under test.
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

    await identifier.fill(LOGIN_EMAIL)
    await page.getByPlaceholder('Password').fill(LOGIN_PASSWORD)
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

async function openMoreDrawer(page: Page) {
    await page.getByTestId('nav-more').click()
    await expect(page.getByText('Sign out', { exact: true })).toBeVisible()
}

test.describe('two orgs on the router', () => {
    test('each subdomain serves its own app', async ({ page }) => {
        await page.goto(`${ACME}/api/health`)
        await expect(page.locator('body')).toContainText('"code":200')

        await page.goto(`${GLOBEX}/api/health`)
        await expect(page.locator('body')).toContainText('"code":200')
    })

    // The isolation the whole feature rests on: a session on one org is not a
    // session on the other, because localStorage is partitioned per origin.
    test('signing into one org does not sign you into the other', async ({ page }) => {
        await loginAt(page, ACME)

        await page.goto(`${GLOBEX}/`)
        await expect(page.getByTestId('identifier')).toBeVisible({ timeout: 20_000 })
    })
})

test.describe('switcher with two servers', () => {
    test.use({ viewport: MOBILE_VIEWPORT })

    test('lists both servers, marking only the current one', async ({ page }) => {
        await loginAt(page, ACME)
        await saveServer(page, ACME)
        await saveServer(page, GLOBEX)
        await page.reload()

        await openMoreDrawer(page)

        const acmeRow = page.getByTestId(`drawer-server-acme.localhost:${PORT}`)
        const globexRow = page.getByTestId(`drawer-server-globex.localhost:${PORT}`)

        await expect(acmeRow).toBeVisible()
        await expect(globexRow).toBeVisible()

        await expect(acmeRow).toHaveAttribute('aria-label', /current server/i)
        await expect(globexRow).toHaveAttribute('aria-label', /^switch to/i)
    })

    // The actual switch. On web this is a navigation to the other origin —
    // native's in-place restart has no browser equivalent, which is why the row
    // says "opens in this tab" rather than promising the sessions coexist.
    test('tapping the other server lands on its origin', async ({ page }) => {
        await loginAt(page, ACME)
        await saveServer(page, ACME)
        await saveServer(page, GLOBEX)
        await page.reload()

        await openMoreDrawer(page)
        await page.getByTestId(`drawer-server-globex.localhost:${PORT}`).click()

        await page.waitForURL(/globex\.localhost/, { timeout: 20_000 })
        expect(new URL(page.url()).host).toBe(`globex.localhost:${PORT}`)
    })

    // Arriving on the other origin as a stranger is correct, not a bug: the
    // browser cannot carry a session across origins. Pinned so the honest copy
    // ("You may need to sign in there") stays honest.
    test('lands signed out when the other origin has no session', async ({ page }) => {
        await loginAt(page, ACME)
        await saveServer(page, ACME)
        await saveServer(page, GLOBEX)
        await page.reload()

        await openMoreDrawer(page)
        await page.getByTestId(`drawer-server-globex.localhost:${PORT}`).click()
        await page.waitForURL(/globex\.localhost/, { timeout: 20_000 })

        await expect(page.getByTestId('identifier')).toBeVisible({ timeout: 20_000 })
    })

    // Both sessions surviving independently is the native promise; on web the
    // equivalent is that going BACK finds you still signed in to the first org.
    test('the original org is still signed in after visiting the other', async ({ page }) => {
        await loginAt(page, ACME)
        await saveServer(page, ACME)
        await saveServer(page, GLOBEX)

        await page.goto(`${GLOBEX}/`)
        await expect(page.getByTestId('identifier')).toBeVisible({ timeout: 20_000 })

        await page.goto(`${ACME}/`)
        const shell = page.getByTestId('nav-home').or(page.getByTestId('nav-more')).first()
        await expect(shell).toBeVisible({ timeout: 20_000 })
    })
})

// The desktop counterpart, at the config's default (wide) viewport so the rail +
// user menu render instead of the tab bar + drawer.
//
// This is the surface most worth covering here and the last to get it: UserMenu's
// section is gated on BOTH `hasOrgs` (defer to the cookie org switcher where it
// exists) and `servers.length > 1` (a lone entry is just a label for where you
// already are). Neither gate does anything until a SECOND server exists, so no
// single-origin test can reach the state where they matter — the mobile blocks
// above exercise a different component entirely (ServersDrawerSection).
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

    test('lists both servers, marking only the current one', async ({ page }) => {
        await loginAt(page, ACME)
        await saveServer(page, ACME)
        await saveServer(page, GLOBEX)
        await page.reload()

        await openUserMenu(page)

        await expect(menuLabel(page, 'Servers')).toBeVisible()

        // Web rows carry an href (Menu.Item renders a real <a role="menuitem">),
        // which is what makes middle-click / open-in-new-tab work — so target by
        // it rather than by label text.
        const acmeRow = page.locator(`a[role="menuitem"][href="${ACME}"]`)
        const globexRow = page.locator(`a[role="menuitem"][href="${GLOBEX}"]`)
        await expect(acmeRow).toBeVisible()
        await expect(globexRow).toBeVisible()

        // The active row gets a Check; the other does not. One check across the
        // two rows is the assertion — "exactly one marked current".
        await expect(acmeRow.locator('svg')).toHaveCount(2) // Server icon + Check
        await expect(globexRow.locator('svg')).toHaveCount(1) // Server icon only
    })

    test('choosing the other server navigates to its origin', async ({ page }) => {
        await loginAt(page, ACME)
        await saveServer(page, ACME)
        await saveServer(page, GLOBEX)
        await page.reload()

        await openUserMenu(page)
        await page.locator(`a[role="menuitem"][href="${GLOBEX}"]`).click()

        await page.waitForURL(/globex\.localhost/, { timeout: 20_000 })
        expect(new URL(page.url()).host).toBe(`globex.localhost:${PORT}`)
    })
})
