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

// Regression coverage for the bug where Reposition fed the cropper the
// already-thumbnailed (center-cropped-to-square) display URL instead of the
// original upload. Re-editing on the thumbnail applies the stored crop rect
// a SECOND time on top of a crop that already happened, so the subject
// marches off-frame a little further on every reposition — the opposite of
// the feature's promise that framing is stored separately and restored
// exactly (core/help/personalizing-your-avatar.md). The 1x1 PNG this spec
// uploads elsewhere can't distinguish "thumbnail" from "original" (both are
// 1x1), so this test uses a real, larger source image and asserts the
// cropper opens on an image whose natural size exceeds the 256x256 thumb —
// which is only true if it is NOT the thumbnail.
test('reposition reopens the cropper on the original photo, not the display thumbnail', async ({
    page,
}) => {
    await login(page)
    await openPersonalSettings(page)

    // A solid-color 512x512 PNG — larger than the 256x256 AVATAR_THUMB, so
    // natural dimensions alone prove which URL the cropper loaded.
    const largePng = await page.evaluate(async () => {
        const canvas = document.createElement('canvas')
        canvas.width = 512
        canvas.height = 512
        const ctx = canvas.getContext('2d')
        if (!ctx) throw new Error('2d context unavailable')
        ctx.fillStyle = '#3b82f6'
        ctx.fillRect(0, 0, 512, 512)
        const dataUrl = canvas.toDataURL('image/png')
        return dataUrl.split(',')[1]
    })

    await attachAvatar(page, {
        name: 'large-avatar.png',
        mimeType: 'image/png',
        buffer: Buffer.from(largePng ?? '', 'base64'),
    })
    await expect(page.getByTestId('avatar-cropper-save')).toBeVisible()

    // Zoom in before saving so the stored crop is non-default — a reposition
    // bug that only shows up on a non-trivial crop must be caught.
    await page.getByLabel('Zoom').fill('3')
    await page.getByTestId('avatar-cropper-save').click()
    await expect(page.getByTestId('avatar-preview-image')).toBeVisible()

    await page.getByTestId('avatar-reposition').click()
    // expo-image's web renderer puts `data-testid` on the transform wrapper,
    // not the `<img>` itself, so the real element is one level in.
    const cropperImage = page.getByTestId('avatar-cropper-image').locator('img')
    await expect(cropperImage).toBeVisible()

    const src = await cropperImage.getAttribute('src')
    // The regression: Reposition fed the cropper the `?thumb=256x256` display
    // URL instead of the original, so re-editing re-cropped an already-cropped
    // image. Asserting the loaded src carries no `thumb=` param is a direct
    // check on the fix, not just an inference from pixel size.
    expect(src).not.toContain('thumb=')

    const naturalWidth = await cropperImage.evaluate((el: HTMLImageElement) => el.naturalWidth)
    // 256 is AVATAR_THUMB's edge (core/lib/use-avatar-url.ts) — anything
    // larger proves the cropper loaded the un-thumbed original, not the
    // display thumbnail.
    expect(naturalWidth).toBeGreaterThan(256)

    await page.getByTestId('avatar-cropper-cancel').click()
})
