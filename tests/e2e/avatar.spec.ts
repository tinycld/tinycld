import type { Page } from '@playwright/test'
import { expect, test } from '@playwright/test'
import { login } from './helpers'

// User avatars, end to end: an emoji chosen through the picker persists across
// a reload, and an uploaded photo replaces the initials circle. Both drive the
// real Personal Settings UI — no raw PocketBase writes — so the proof covers
// the whole feature: the mutation, the stored fields, and useAvatarUrl
// resolving them back into a rendered circle.

/** A real 1x1 PNG — the cropper decodes the bytes, so they must actually decode. */
const PNG_BASE64 =
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='

// Via the user menu, matching change-password.spec.ts's proven route: the
// rail's Settings link lands on the hub with the PREVIOUS package's screen
// (e.g. Mail's inbox, whose rows carry a "Personal" folder label) still
// mounted underneath. A plain getByText('Personal') is therefore ambiguous —
// it matches the settings link AND every mail row tagged "Personal" — so this
// scopes to the "Account" settings group, which only the real link is in.
async function openPersonalSettings(page: Page): Promise<void> {
    await page.getByLabel('User menu').click()
    await page.getByText('Settings', { exact: true }).click()
    const accountGroup = page.getByText('Account', { exact: true }).locator('..')
    await accountGroup.getByText('Personal', { exact: true }).click()
    await expect(page.getByTestId('avatar-preview')).toBeVisible()
}

/**
 * Attaches a file by driving the real picker — the same pattern
 * boards/tests/e2e/card-attachments.spec.ts uses (`attachFile`).
 *
 * Playwright's chooser interception suppresses the native dialog — and with it
 * the window blur → focus round trip a real dialog causes. The picker code
 * runs inside that round trip for every real user, so the spec restores it:
 * blur as the chooser opens, refocus as it closes, and only then deliver the
 * selection.
 */
async function attachAvatar(
    page: Page,
    file: { name: string; mimeType: string; buffer: Buffer }
): Promise<void> {
    const chooserPromise = page.waitForEvent('filechooser')
    await page.getByTestId('avatar-upload').click()
    const chooser = await chooserPromise
    await page.evaluate(() => window.dispatchEvent(new Event('blur')))
    await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    // Wait for the renderer to go idle rather than sleeping — a wall-clock
    // sleep is both slower than needed and flaky under load.
    await page.evaluate(
        () => new Promise<void>(resolve => requestAnimationFrame(() => setTimeout(resolve, 0)))
    )
    await chooser.setFiles(file)
}

test('an emoji avatar set through the UI persists across a reload', async ({ page }) => {
    await login(page)
    await openPersonalSettings(page)

    await page.getByTestId('avatar-choose-emoji').click()
    await page.getByTestId('emoji-search').fill('dinosaur')
    await page.getByTestId('emoji-pick-1f996').click()

    await expect(page.getByTestId('avatar-preview')).toContainText('🦖')

    // The point of the feature: the choice follows the user out of settings.
    await page.reload()
    await expect(page.getByTestId('avatar-preview')).toContainText('🦖')
})

test('an uploaded photo replaces the initials circle', async ({ page }) => {
    await login(page)
    await openPersonalSettings(page)

    await attachAvatar(page, {
        name: 'avatar.png',
        mimeType: 'image/png',
        buffer: Buffer.from(PNG_BASE64, 'base64'),
    })
    await expect(page.getByTestId('avatar-cropper-save')).toBeVisible()
    await page.getByTestId('avatar-cropper-save').click()

    await expect(page.getByTestId('avatar-preview-image')).toBeVisible()

    // Persists across a reload the same way the emoji case does — the image
    // is fetched fresh, proving the stored file (not just local UI state) is
    // what renders.
    await page.reload()
    await expect(page.getByTestId('avatar-preview-image')).toBeVisible()
})
