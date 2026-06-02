import { expect, test } from '@playwright/test'
import { login, navigateToPackage, ORG_SLUG } from './helpers'

// These tests verify app-shell keyboard-shortcut behavior using a
// stub package scaffolded by app/tests/scripts/scaffold-shortcut-stub.ts.
// The stub registers a minimal nav entry with shortcut 'k' and a
// landing route, which is everything the chord/help tests need to
// exercise the app's OWN contract — no coupling to mail, contacts, or
// any real feature package.
//
// To run locally, scaffold the stub first:
//     pnpm exec tsx tests/scripts/scaffold-shortcut-stub.ts
//     cd app && pnpm exec tinycld-pkg test:e2e -- --grep "Keyboard shortcuts"
//
// CI runs the scaffold step as part of the e2e job.

const STUB_NAV_LABEL = 'Shortcut Stub'
const STUB_SLUG = 'shortcut-stub'
const STUB_SHORTCUT = 'k'

test.describe('Keyboard shortcuts', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        // The stub has no sidebar, so pass an explicit waitFor — the
        // helper's default sidebar gate would hang. Waiting on the
        // landing text also confirms the stub's lazy chunk has
        // mounted before we start typing shortcuts.
        await navigateToPackage(page, STUB_SLUG, {
            waitFor: page.getByText('Shortcut stub landing', { exact: true }),
        })
        await expect(page.getByRole('link', { name: STUB_NAV_LABEL, exact: true })).toBeVisible({
            timeout: 10_000,
        })

        // Ensure focus is on body, not an input that would suppress
        // shortcuts.
        await page.evaluate(() => {
            ;(document.activeElement as HTMLElement | null)?.blur?.()
        })
    })

    test('? opens the help dialog and Escape closes it', async ({ page }) => {
        // The help shortcut binds to `Shift+?` because on real keyboards
        // producing a `?` always holds Shift. Playwright's press('?')
        // fires the event with shiftKey=false, so use the explicit combo
        // to match what a human types.
        await page.keyboard.press('Shift+?')
        await expect(page.getByText('Keyboard shortcuts').first()).toBeVisible({
            timeout: 5_000,
        })

        await page.keyboard.press('Escape')
        await expect(page.getByText('Keyboard shortcuts').first()).not.toBeVisible({
            timeout: 5_000,
        })
    })

    test(`t ${STUB_SHORTCUT} jumps to the stub package`, async ({ page }) => {
        // Small delay between keys helps the sequence matcher — the two
        // presses must arrive within its 1s window but fast enough that
        // neither is lost.
        await page.keyboard.press('t', { delay: 50 })
        await page.keyboard.press(STUB_SHORTCUT, { delay: 50 })
        await page.waitForURL(new RegExp(`/a/${ORG_SLUG}/${STUB_SLUG}`), { timeout: 10_000 })
    })

    test('rail renders the manifest-declared icon (cloud-rain), not the fallback', async ({
        page,
    }) => {
        // shortcut-stub declares nav.icon: 'cloud-rain'. That name is NOT
        // in the legacy hand-curated icon map, so this assertion only passes
        // when manifest-driven icon bundling is wired up end-to-end. If a
        // future change reintroduces hand-curation, the rail will render
        // the CircleHelp ("?" / circle-question-mark) fallback and this
        // breaks loudly.
        //
        // lucide-react-native renders icons as inline SVG with one <path>
        // per glyph segment. We assert on a distinctive path fragment from
        // cloud-rain.mjs ("M4 14.899A7 7 0 1 1 15.71 8h1.79a4.5 4.5 0 0 1 2.5 8.242")
        // and confirm the same path is absent from any fallback render.
        const navItem = page.getByTestId(`nav-${STUB_SLUG}`)
        await expect(navItem).toBeVisible()
        const svg = navItem.locator('svg').first()
        await expect(svg).toBeVisible()
        const paths = await svg
            .locator('path')
            .evaluateAll(els => els.map(el => el.getAttribute('d') ?? ''))
        const cloudRainSignature = 'M4 14.899A7 7 0 1 1 15.71 8'
        expect(paths.some(d => d.startsWith(cloudRainSignature))).toBe(true)
    })
})
