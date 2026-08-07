import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage } from './helpers'

// The palette's own dialog role/label — see SearchPalette.web.tsx's
// PaletteCard, which sets role="dialog" aria-label="Search".
function paletteDialog(page: Page) {
    return page.getByRole('dialog', { name: 'Search' })
}

// Open the palette from a closed state. Never call this (or press '/') while
// the palette is already open — the shortcut is deliberately suppressed
// while focus sits in an input (see the "slash inside a text input" test
// below), which means the browser's default keystroke still lands a literal
// '/' character in whatever is focused, corrupting an in-progress query.
async function openPalette(page: Page) {
    await page.keyboard.press('/')
    await expect(paletteDialog(page)).toBeVisible()
}

// useSearchResults debounces 300ms and only fires once >=2 chars of include
// text are typed (MIN_QUERY_LENGTH), so every assertion here waits on the
// actual `/search` network response or the resulting DOM — never a fixed
// timeout, which flakes under CI load and doesn't prove the request landed.
async function typeAndWait(page: Page, text: string) {
    await page.keyboard.type(text)
    await page.waitForResponse(r => r.url().includes('/search') && r.status() === 200)
}

test.describe('Search palette', () => {
    test('opens seeded with the current package as a chip', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await expect(page.getByTestId('search-chip-cards')).toBeVisible()
    })

    test('backspace on empty text pops the chip and widens to every package', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await expect(page.getByTestId('search-chip-cards')).toBeVisible()

        await page.keyboard.press('Backspace')
        await expect(page.getByTestId('search-chip-cards')).toHaveCount(0)
    })

    test('typing a package name plus a colon turns it into a scope chip', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace') // widen first: start from zero chips
        await page.keyboard.type('drive:')
        await expect(page.getByTestId('search-chip-drive')).toBeVisible()
    })

    // Regression guard (the bug this exists to prevent: a literal package
    // name becoming an unwanted scope chip). Real seeded data: the "Sentry
    // alert: high error rate in production" thread's snippet reads "...in the
    // mail service...", so the word "mail" is genuinely indexed and findable
    // with zero chips — proving the guard protects real search results, not
    // just an absent testid.
    test('a package name without a colon stays literal search text', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace') // zero chips: search everywhere
        await typeAndWait(page, 'mail')

        await expect(page.getByTestId('search-chip-mail')).toHaveCount(0)
        await expect(
            paletteDialog(page).getByText('Sentry alert: high error rate in production')
        ).toBeVisible()
    })

    test('two chips search exactly those packages, grouped by nav.order', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace')
        // drive (nav.order 12) before cards (nav.order 25) — real seeded
        // "onboarding" hits in both: drive's "Onboarding Slides.pptx" and
        // cards' "Onboarding checklist for new members".
        await typeAndWait(page, 'drive: cards: onboarding')

        await expect(page.getByTestId('search-chip-drive')).toBeVisible()
        await expect(page.getByTestId('search-chip-cards')).toBeVisible()

        const rows = page.getByRole('option')
        await expect(rows.filter({ hasText: 'Onboarding Slides.pptx' })).toBeVisible()
        await expect(rows.filter({ hasText: 'Onboarding checklist for new members' })).toBeVisible()

        // Grouped by package (2+ chips) uses a section HEADING naming the
        // package, not a per-row badge — that only appears in the flat,
        // unscoped list (see the zero-chips test below).
        const headings = page.getByTestId('search-section-heading')
        await expect(headings.filter({ hasText: 'Drive' })).toBeVisible()
        await expect(headings.filter({ hasText: 'Cards' })).toBeVisible()

        const optionTexts = await rows.allTextContents()
        const driveIndex = optionTexts.findIndex(t => t.includes('Onboarding Slides.pptx'))
        const cardsIndex = optionTexts.findIndex(t =>
            t.includes('Onboarding checklist for new members')
        )
        expect(driveIndex).toBeGreaterThanOrEqual(0)
        expect(cardsIndex).toBeGreaterThanOrEqual(0)
        expect(driveIndex).toBeLessThan(cardsIndex)
    })

    test('zero chips is a flat, score-ordered list with per-row package badges', async ({
        page,
    }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace')
        await typeAndWait(page, 'onboarding')

        // Title-PREFIX matches (drive, cards both start with "Onboarding")
        // must outrank mail's title-SUBSTRING match ("Design review: new
        // onboarding flow" contains but doesn't start with the term) — this
        // is the actual scoring behavior wired end to end, not merely that
        // all three appear.
        const rows = page.getByRole('option')
        const texts = await rows.allTextContents()
        const driveIndex = texts.findIndex(t => t.includes('Onboarding Slides.pptx'))
        const cardsIndex = texts.findIndex(t => t.includes('Onboarding checklist for new members'))
        const mailIndex = texts.findIndex(t => t.includes('Design review: new onboarding flow'))
        expect(driveIndex).toBeGreaterThanOrEqual(0)
        expect(cardsIndex).toBeGreaterThanOrEqual(0)
        expect(mailIndex).toBeGreaterThanOrEqual(0)
        expect(driveIndex).toBeLessThan(mailIndex)
        expect(cardsIndex).toBeLessThan(mailIndex)

        // Flat list (no chips) shows a badge naming each row's package —
        // the thing that disappears once a chip scopes the list to one
        // package (see the single-chip test below). Badges render inside the
        // matching row, so scope to it rather than the whole dialog.
        await expect(
            rows.filter({ hasText: 'Onboarding Slides.pptx' }).getByText('Drive', { exact: true })
        ).toBeVisible()
        await expect(
            rows
                .filter({ hasText: 'Design review: new onboarding flow' })
                .getByText('Mail', { exact: true })
        ).toBeVisible()
        await expect(
            rows
                .filter({ hasText: 'Onboarding checklist for new members' })
                .getByText('Cards', { exact: true })
        ).toBeVisible()
    })

    test('exactly one chip is a flat list with no package badges', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace') // drop the seeded "cards" chip
        await typeAndWait(page, 'drive: onboarding')

        await expect(page.getByTestId('search-chip-drive')).toBeVisible()
        const row = page.getByRole('option').filter({ hasText: 'Onboarding Slides.pptx' })
        await expect(row).toBeVisible()
        // Single-chip results are already scoped to one package, so no badge
        // is shown on the row (the label would be redundant with the chip).
        await expect(row.getByText('Drive', { exact: true })).toHaveCount(0)
    })

    // Excludes a word using real seeded data: "Q2 Product Roadmap Review" and
    // "Design review: new onboarding flow" both contain "review", but only
    // the second contains "onboarding" — so -onboarding is a genuine
    // discriminator, not a cousin case that would pass even if exclusion were
    // broken.
    test('a -term excludes matching results end to end', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace') // drop the seeded "cards" chip
        await typeAndWait(page, 'mail: review -onboarding')

        await expect(page.getByTestId('search-chip-mail')).toBeVisible()
        await expect(
            page.getByRole('option').filter({ hasText: 'Q2 Product Roadmap Review' })
        ).toBeVisible()
        await expect(
            page.getByRole('option').filter({ hasText: 'Design review: new onboarding flow' })
        ).toHaveCount(0)
    })

    test('a hyphen inside a word stays literal and still matches', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace') // drop the seeded "cards" chip
        // "Product Roadmap 2026.docx" (description: "Full product roadmap
        // for 2026") is real seeded drive content containing both words.
        // Typing the mid-token-hyphen form "roadmap-2026" must not be torn
        // into an exclusion of "2026" (parseQuery keeps a mid-token hyphen
        // literal) — the backend's own FTS sanitizer then re-splits it into
        // two AND-ed prefix terms, both of which this file satisfies.
        await typeAndWait(page, 'drive: roadmap-2026')

        await expect(
            page.getByRole('option').filter({ hasText: 'Product Roadmap 2026.docx' })
        ).toBeVisible()
    })

    test('selecting a result with Enter navigates to it', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await page.keyboard.press('Backspace') // drop the seeded "cards" chip
        await typeAndWait(page, 'mail: Q2 Product Roadmap')

        await expect(page.getByRole('option').first()).toBeVisible()
        await page.keyboard.press('Enter')

        await expect(page.getByTestId('mail-thread-detail')).toBeVisible()
        await expect(page).toHaveTitle(/Q2 Product Roadmap Review/)
    })

    test('escape closes the palette without navigating', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        const url = page.url()

        await openPalette(page)
        await page.keyboard.press('Escape')

        await expect(paletteDialog(page)).toHaveCount(0)
        expect(page.url()).toBe(url)
    })

    // Regression guard for the C1 bug: parseQuery used to recognize `pkg:`
    // ANYWHERE in the raw text, but the renderer stripped the chip prefix by
    // COMPUTED LENGTH (assuming chips were always a leading prefix). Typing a
    // chip-forming token AFTER free text made that slice cut into the free
    // text instead of the chip prefix, silently destroying it on the very
    // next keystroke. Seeded-chip case: the palette opens with "cards"
    // already a chip (see navigateToPackage below), then the user types free
    // text, then a SECOND `pkg:`-shaped token ("mail:") that must NOT be
    // promoted to a chip (only the leading run counts) and must NOT eat
    // either neighboring word. Asserts on the actual input DOM value — not
    // just a result matching, which could pass by coincidence.
    test('typing free text then a second pkg:-looking token loses no word', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'cards')
        await openPalette(page)
        await expect(page.getByTestId('search-chip-cards')).toBeVisible()

        const input = page.getByLabel('Search across packages')
        await page.keyboard.type('Onboarding mail: checklist')

        // "mail:" typed after free text must stay literal text, not a
        // second chip — only the leading run (the seeded "cards" chip) can
        // ever become a chip. Checked against the real DOM value, not just
        // the parsed result: this is exactly what the C1 bug corrupted —
        // the box used to strip a computed-length prefix that disagreed with
        // where the chip token actually sat, cutting into "Onboarding".
        await expect(page.getByTestId('search-chip-mail')).toHaveCount(0)
        await expect(page.getByTestId('search-chip-cards')).toBeVisible()
        await expect(input).toHaveValue('Onboarding mail: checklist')
    })

    // '/' inside an RN TextInput never reaches the global shortcut listener
    // at all: react-native-web's TextInput.handleKeyDown unconditionally
    // calls stopPropagation() on every keydown, and the shortcuts provider
    // listens on `window` in the bubble phase — so this holds regardless of
    // which package owns the field. Asserting on a literal '/' landing in
    // the box (not just the dialog's absence) proves the keystroke reached
    // the input rather than being dropped by some unrelated failure.
    test('slash inside a text input does not open the palette', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'contacts')
        const searchBox = page.getByPlaceholder('Search contacts...')
        await searchBox.click()

        await page.keyboard.press('/')

        await expect(searchBox).toHaveValue('/')
        await expect(paletteDialog(page)).toHaveCount(0)
    })
})
