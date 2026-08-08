import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage } from './helpers'

/**
 * The palette's e2e runs against two stub packages (search-alpha, search-beta)
 * scaffolded by tests/scripts/scaffold-search-stubs.ts, NOT against real
 * features. App-shell CI assembles app+core only, so asserting on cards/drive/
 * mail seed rows made every test here fail on packages that were never
 * installed — and made app's CI hostage to another repo's seed fixtures.
 *
 * What that leaves for this file: the behaviours that need a real browser, a
 * real server round-trip and two real packages. Query PARSING is covered
 * exhaustively by parse-query's unit tests, and cross-package MERGE + SCORE
 * ordering by the aggregator's Go tests — re-asserting either through a browser
 * buys nothing but wall-clock.
 *
 * Stub fixtures this file depends on (keep in sync with the scaffolder):
 *   search-alpha (nav.order 990) — "Onboarding checklist",
 *       "Quarterly roadmap review", "Roadmap 2026 planning"
 *   search-beta  (nav.order 991) — "Design review: new onboarding flow",
 *       "Budget review"
 *
 * Rows are located by their "<Stub> … subtitle" text rather than their titles.
 * The titles deliberately resemble real seed data so the stubs read like real
 * packages, which means a developer running with features installed has several
 * rows sharing a title substring; `hasText` would then match more than one and
 * fail strict mode. The subtitles exist only in the stubs, so every locator here
 * is unambiguous both in CI's lean assembly and in a full local workspace.
 */

const ALPHA = 'search-alpha'
const BETA = 'search-beta'

// The palette's own dialog role/label — see SearchPalette.web.tsx's PaletteCard,
// which sets role="dialog" aria-label="Search".
function paletteDialog(page: Page) {
    return page.getByRole('dialog', { name: 'Search' })
}

// Open the palette from a closed state. Never call this (or press '/') while the
// palette is already open — the shortcut is deliberately suppressed while focus
// sits in an input (see the "slash inside a text input" test), which means the
// browser's default keystroke still lands a literal '/' in whatever is focused,
// corrupting an in-progress query.
async function openPalette(page: Page) {
    await page.keyboard.press('/')
    await expect(paletteDialog(page)).toBeVisible()
}

// useSearchResults debounces 300ms and only fires once >=2 chars of include text
// are typed (MIN_QUERY_LENGTH), so every assertion waits on the actual /search
// response or the resulting DOM — never a fixed timeout, which flakes under CI
// load and doesn't prove the request landed.
async function typeAndWait(page: Page, text: string) {
    await page.keyboard.type(text)
    await page.waitForResponse(r => r.url().includes('/search') && r.status() === 200)
}

test.describe('Search palette', () => {
    test('opens seeded with the current package as a chip, and backspace widens', async ({
        page,
    }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        await openPalette(page)
        await expect(page.getByTestId(`search-chip-${ALPHA}`)).toBeVisible()

        // Backspace on empty text pops the chip rather than deleting a
        // character, which is what lets a user widen to every package without
        // reaching for the mouse.
        await page.keyboard.press('Backspace')
        await expect(page.getByTestId(`search-chip-${ALPHA}`)).toHaveCount(0)
    })

    test('typing a package name plus a colon turns it into a scope chip', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        await openPalette(page)
        await page.keyboard.press('Backspace') // widen first: start from zero chips
        await page.keyboard.type(`${BETA}:`)
        await expect(page.getByTestId(`search-chip-${BETA}`)).toBeVisible()
    })

    test('zero chips searches every package, with a badge naming each row’s package', async ({
        page,
    }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        await openPalette(page)
        await page.keyboard.press('Backspace')
        await typeAndWait(page, 'onboarding')

        // Both stubs match, so this proves the server actually fanned out
        // rather than answering from whichever package was in scope.
        //
        // Rows are located by their stub SUBTITLE, not their title. A developer
        // running with real features installed gets rows whose titles collide
        // with the stubs' (cards seeds "Onboarding checklist for new members",
        // mail seeds a thread with the same subject as beta-1), and `hasText` is
        // a substring match, so a title locator resolves to several rows and
        // fails strict mode. The subtitles are stub-only, so these locators mean
        // the same thing in a lean CI assembly and a full local workspace.
        const rows = page.getByRole('option')
        const alphaRow = rows.filter({ hasText: 'Alpha onboarding subtitle' })
        const betaRow = rows.filter({ hasText: 'Beta onboarding subtitle' })
        await expect(alphaRow).toBeVisible()
        await expect(betaRow).toBeVisible()

        // The flat (unscoped) list labels each row with its package — the thing
        // that disappears once a chip makes the package unambiguous.
        await expect(alphaRow.getByText('Search Alpha', { exact: true })).toBeVisible()
        await expect(betaRow.getByText('Search Beta', { exact: true })).toBeVisible()
    })

    test('two chips group results under a heading per package', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        await openPalette(page)
        await page.keyboard.press('Backspace')
        await typeAndWait(page, `${ALPHA}: ${BETA}: onboarding`)

        await expect(page.getByTestId(`search-chip-${ALPHA}`)).toBeVisible()
        await expect(page.getByTestId(`search-chip-${BETA}`)).toBeVisible()

        // 2+ chips switch from per-row badges to section headings. Ordering
        // across sections is the aggregator's contract and is asserted in Go
        // (TestAggregateMergesAndOrdersAcrossPackages); what matters here is
        // that the grouped presentation renders at all.
        const headings = page.getByTestId('search-section-heading')
        await expect(headings.filter({ hasText: 'Search Alpha' })).toBeVisible()
        await expect(headings.filter({ hasText: 'Search Beta' })).toBeVisible()
    })

    test('exactly one chip is a flat list with no package badges', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        await openPalette(page)
        await typeAndWait(page, 'onboarding') // keep the seeded alpha chip

        const row = page.getByRole('option').filter({ hasText: 'Alpha onboarding subtitle' })
        await expect(row).toBeVisible()
        // Already scoped to one package, so a badge would only repeat the chip.
        await expect(row.getByText('Search Alpha', { exact: true })).toHaveCount(0)
        // Scoped means scoped: beta's match must be absent, not merely unbadged.
        await expect(
            page.getByRole('option').filter({ hasText: 'Beta onboarding subtitle' })
        ).toHaveCount(0)
    })

    // "Budget review" and "Design review: new onboarding flow" both match
    // "review", but only the second matches "onboarding" — so -onboarding is a
    // genuine discriminator rather than a cousin case that would pass even if
    // exclusion were dropped on the way to the server.
    test('a -term excludes matching results end to end', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        await openPalette(page)
        await page.keyboard.press('Backspace')
        await typeAndWait(page, `${BETA}: review -onboarding`)

        await expect(
            page.getByRole('option').filter({ hasText: 'Beta budget subtitle' })
        ).toBeVisible()
        await expect(
            page.getByRole('option').filter({ hasText: 'Beta onboarding subtitle' })
        ).toHaveCount(0)
    })

    test('selecting a result with Enter navigates to it', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        await openPalette(page)
        await typeAndWait(page, 'Onboarding checklist')

        await expect(page.getByRole('option').first()).toBeVisible()
        await page.keyboard.press('Enter')

        // The stub's [id] screen echoes the id it was routed to, so this proves
        // the selected row carried its own id from the server through the
        // palette into the router — not that navigation merely happened.
        await expect(page.getByTestId(`${ALPHA}-detail`)).toHaveText('alpha-1')
    })

    // '/' inside an RN TextInput never reaches the global shortcut listener at
    // all: react-native-web's TextInput.handleKeyDown unconditionally calls
    // stopPropagation() on every keydown, and the shortcuts provider listens on
    // `window` in the bubble phase — so this holds regardless of which input is
    // focused. Uses the help screen's own search box rather than a package's, so
    // the test stays within the app shell it is testing. Asserting the literal
    // '/' lands in the box (not merely that no dialog opened) proves the
    // keystroke reached the input instead of being dropped by something else.
    test('slash inside a text input does not open the palette', async ({ page }) => {
        await login(page)
        await page.getByTestId('nav-help').click()
        const searchBox = page.getByPlaceholder('Search help topics')
        await searchBox.click()

        await page.keyboard.press('/')

        await expect(searchBox).toHaveValue('/')
        await expect(paletteDialog(page)).toHaveCount(0)
    })

    test('escape closes the palette without navigating', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, ALPHA)
        const url = page.url()

        await openPalette(page)
        await page.keyboard.press('Escape')

        await expect(paletteDialog(page)).toHaveCount(0)
        expect(page.url()).toBe(url)
    })
})
