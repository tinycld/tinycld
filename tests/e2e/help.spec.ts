import { expect, test } from '@playwright/test'
import { readFirstFrame, watchFirstFrame } from './first-frame-helpers'
import { login } from './helpers'

test.describe('Help', () => {
    test('nav rail Help button opens the hub, search finds a core topic, permalink renders', async ({
        page,
    }) => {
        await login(page)

        await page.getByTestId('nav-help').click()
        await expect(page).toHaveURL(/\/help$/)
        await expect(page.getByText('Help', { exact: true })).toBeVisible()

        // Core topics are always present; assert one renders in the index.
        await expect(page.getByText('Light and dark themes')).toBeVisible()

        // Search narrows the list.
        await page.getByPlaceholder('Search help topics').fill('themes')
        await expect(page.getByText('Light and dark themes')).toBeVisible()

        // Clicking a topic navigates to the permalink and renders the body.
        await page.getByText('Light and dark themes').click()
        await expect(page).toHaveURL(/\/help\/core\/themes$/)
        await expect(page.getByText('Light and dark mode')).toBeVisible()
    })

    test('a topic image bundled with the package loads', async ({ page }) => {
        await login(page)
        await page.getByTestId('nav-help').click()
        await page.getByText('Light and dark themes').click()
        await expect(page).toHaveURL(/\/help\/core\/themes$/)

        const image = page.locator('img[src*="themes-1"]').first()
        await expect(image).toBeVisible()
        await expect
            .poll(() => image.evaluate((el: HTMLImageElement) => el.naturalWidth))
            .toBeGreaterThan(0)
        await expect(image).not.toHaveAttribute('src', /^data:/)
    })

    // The canonical config emulates reduced motion (playwright.config.ts), as a user
    // who turned motion off: the help palette then opens at rest. Its open animation
    // is set inline (HelpSearchPalette.web.tsx PaletteCard), so it runs only where its
    // @keyframes are defined. ⌘/ opens it (use-help-search-shortcut.ts).
    test('with reduced motion the help search palette opens with no animation', async ({
        page,
    }) => {
        await login(page)
        await watchFirstFrame(page, '[data-tinycld-help-palette="true"]')
        await page.keyboard.press('ControlOrMeta+/')
        await expect(page.getByRole('dialog', { name: 'Search help' })).toBeVisible()
        const frame = await readFirstFrame(page)
        expect(frame.reducedMotion).toBe(true)
        expect(frame.animationName).toBe('tinycld-help-palette-in')
        expect(frame.animations).toEqual([])
    })
})
