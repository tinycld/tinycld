import { expect, test } from '@playwright/test'
import { readFirstFrame, watchFirstFrame } from './first-frame-helpers'
import { login, navigateToPackage, skipWithoutShortcutStub } from './helpers'

// Core's suite installs no real package, so the shortcut-stub (scaffolded by
// tests/scripts/scaffold-shortcut-stub.ts) declares a notify event through its
// manifest. This proves a package's declared event reaches the toast with the
// policy the package gave it, without core naming any package.
test.describe('Notify registry', () => {
    test.skip(skipWithoutShortcutStub(), 'shortcut-stub not scaffolded in this workspace')

    test("a package's declared event shows as a toast with its variant", async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'shortcut-stub', {
            waitFor: page.getByText('Shortcut stub landing', { exact: true }),
        })

        await page.getByTestId('shortcut-stub-ping').click()

        const toast = page.getByTestId('toast-card').filter({ hasText: 'Pinged' })
        await expect(toast).toBeVisible()
        await expect(toast.getByTestId('toast-variant-info')).toBeVisible()
    })

    // The canonical config emulates reduced motion (playwright.config.ts), as a user
    // who turned motion off: the toast is then drawn at rest, with no fade or slide
    // in (Toast.tsx ToastCard, useReducedMotion).
    test('with reduced motion a toast is drawn at rest on its first frame', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'shortcut-stub', {
            waitFor: page.getByText('Shortcut stub landing', { exact: true }),
        })
        await expect(page.getByTestId('toast-card')).toHaveCount(0)
        await watchFirstFrame(page, '[data-testid="toast-card"]')

        await page.getByTestId('shortcut-stub-ping').click()

        await expect(page.getByTestId('toast-card').filter({ hasText: 'Pinged' })).toBeVisible()
        const frame = await readFirstFrame(page)
        expect(frame.reducedMotion).toBe(true)
        expect(frame.opacity).toBe('1')
        expect(frame.isAtRest, `transform ${frame.transform}`).toBe(true)
    })
})
