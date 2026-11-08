import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, openAccountSecurity } from './helpers'

// ShortcutButton, driven through the two app-password dialogs in Settings →
// Security: the name prompt (PromptDialog) and the revoke confirm
// (ConfirmDialog). Both bind ⌘/Ctrl+Enter to their action button. An invited
// member runs it so the shared TEST_USER is left with no app passwords.
//
// The "⌘C does not show the hint" rule is in the unit test
// (core/tests/unit/modifier-held.test.ts): proving a hint does NOT appear in
// e2e would need a fixed sleep. A disabled button leaving the key alone is in
// core/tests/unit/button-shortcut.test.tsx: here the prompt's own submit guard
// and its field's submit-on-Enter would hide a broken shortcut guard.
const IS_MAC = process.platform === 'darwin'
const MODIFIER = IS_MAC ? 'Meta' : 'Control'
const HINT = IS_MAC ? '⌘↵' : 'Ctrl+↵'
const ARIA_KEYS = IS_MAC ? 'Meta+Enter' : 'Control+Enter'
const NAME_PLACEHOLDER = 'Thunderbird on my laptop'

test.describe('shortcut buttons', () => {
    let page: Page
    let closeInvitee: () => Promise<void>

    test.beforeEach(async ({ page: adminPage }) => {
        const created = await createInvitedUser(adminPage, 'shortcut')
        page = created.inviteePage
        closeInvitee = created.close
        await openAccountSecurity(page)
    })

    test.afterEach(async () => {
        await closeInvitee()
    })

    async function openNamePrompt() {
        await page.getByTestId('app-password-create').click()
        await page.getByPlaceholder(NAME_PLACEHOLDER).waitFor({ state: 'visible' })
    }

    // The prompt's field also submits on Enter, so the revoke confirm (no
    // field) is what proves the button's shortcut.
    test('⌘/Ctrl+Enter submits the prompt and confirms the revoke', async () => {
        await openNamePrompt()
        await page.getByPlaceholder(NAME_PLACEHOLDER).fill('Thunderbird')
        await page.keyboard.press('ControlOrMeta+Enter')
        await expect(page.getByTestId('app-password-value')).toBeVisible()
        await page.getByTestId('app-password-done').click()

        const row = page.getByTestId('app-password-row')
        await expect(row).toHaveCount(1)
        await row.getByLabel('Revoke Thunderbird').click()
        await expect(page.getByText('Revoke app password?')).toBeVisible()
        await page.keyboard.press('ControlOrMeta+Enter')
        await expect(row).toHaveCount(0)
    })

    test('holding the modifier shows the shortcut without resizing the button', async () => {
        await openNamePrompt()
        const submit = page.getByTestId('app-password-name-submit')
        const hint = submit.getByTestId('shortcut-hint')
        await expect(hint).toBeHidden()
        const before = await submit.boundingBox()

        await page.keyboard.down(MODIFIER)
        await expect(hint).toBeVisible()
        await expect(hint).toHaveText(HINT)
        const during = await submit.boundingBox()
        expect(during?.width).toBe(before?.width)
        expect(during?.height).toBe(before?.height)

        await page.keyboard.up(MODIFIER)
        await expect(hint).toBeHidden()
    })

    test('the tooltip and aria-keyshortcuts name the shortcut', async () => {
        await openNamePrompt()
        const submit = page.getByTestId('app-password-name-submit')
        await expect(submit).toHaveAttribute('aria-keyshortcuts', ARIA_KEYS)
        const tooltip = page.locator('.tinycld-tooltip', { has: submit })
        await expect(tooltip).toHaveAttribute('data-tooltip', `Create ${HINT}`)
    })

    // Below the mobile breakpoint a Dialog renders as a Sheet, a separate
    // layer path (Sheet's own LayerScope). The revoke confirm has no text
    // field, so only the shortcut can press its button: in the name prompt the
    // field's own submit-on-Enter also answers ⌘↵.
    test('the shortcut fires when the dialog is a sheet (phone)', async () => {
        await openNamePrompt()
        await page.getByPlaceholder(NAME_PLACEHOLDER).fill('Phone')
        await page.getByTestId('app-password-name-submit').click()
        await page.getByTestId('app-password-done').click()
        const row = page.getByTestId('app-password-row')
        await expect(row).toHaveCount(1)

        await page.setViewportSize({ width: 390, height: 844 })
        await row.getByLabel('Revoke Phone').click()
        await expect(page.getByText('Revoke app password?')).toBeVisible()
        await page.keyboard.press('ControlOrMeta+Enter')
        await expect(row).toHaveCount(0)
    })
})
