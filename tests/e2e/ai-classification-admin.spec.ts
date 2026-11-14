import { expect, test } from '@playwright/test'
import { login } from './helpers'

test('admin enters a key, verifies it, and sees month spend', async ({ page }) => {
    await login(page)
    // Mail's account and org settings carry the same label, so pick by test id.
    await page.getByTestId('nav-settings').click()
    await page.getByTestId('settings-index').waitFor({ state: 'visible' })
    await page.getByTestId('settings-system-ai-classification').click()
    await page.getByTestId('classifier-api-key').fill('wrong-key')
    await page.getByTestId('classifier-save').click()
    await expect(page.getByTestId('classifier-save')).toBeDisabled()
    await page.getByTestId('classifier-verify').click()
    await expect(page.getByTestId('classifier-status')).toContainText('rejected the key')

    await page.getByTestId('classifier-api-key').fill('test-key')
    await page.getByTestId('classifier-save').click()
    await expect(page.getByTestId('classifier-save')).toBeDisabled()
    await page.getByTestId('classifier-verify').click()
    await expect(page.getByTestId('classifier-verify-result')).toHaveText('Connected')
    await expect(page.getByTestId('classifier-status')).toHaveCount(0)
    await expect(page.getByTestId('classifier-month-spend')).toContainText('calls')

    // Typing a new key disables Verify until it is saved.
    await page.getByTestId('classifier-api-key').fill('unsaved')
    await expect(page.getByTestId('classifier-verify')).toBeDisabled()
    await expect(page.getByTestId('classifier-unsaved-key-hint')).toBeVisible()
    await page.getByTestId('classifier-api-key').fill('')
    await expect(page.getByTestId('classifier-verify')).toBeEnabled()

    // Removing the key turns the feature off.
    await page.getByTestId('classifier-remove-key').click()
    await expect(page.getByTestId('classifier-remove-key')).toHaveCount(0)
    await expect(page.getByTestId('classifier-verify')).toBeDisabled()
    await expect(page.getByTestId('classifier-api-key')).toHaveAttribute('placeholder', 'sk-or-…')

    // The automatic action threshold persists across leaving and reopening the panel.
    const threshold = page.getByTestId('classifier-auto-action-threshold')
    await expect(threshold).toHaveValue('0.9')
    await threshold.fill('0.95')
    await page.getByTestId('classifier-save').click()
    await expect(page.getByTestId('classifier-save')).toBeDisabled()
    await page.getByTestId('nav-settings').click()
    await page.getByTestId('settings-index').waitFor({ state: 'visible' })
    await page.getByTestId('settings-system-ai-classification').click()
    await expect(threshold).toHaveValue('0.95')
    await threshold.fill('0.9')
    await page.getByTestId('classifier-save').click()
    await expect(page.getByTestId('classifier-save')).toBeDisabled()
    await expect(threshold).toHaveValue('0.9')

    // Later specs rely on a stored key.
    await page.getByTestId('classifier-api-key').fill('test-key')
    await page.getByTestId('classifier-save').click()
    await expect(page.getByTestId('classifier-save')).toBeDisabled()
    await expect(page.getByTestId('classifier-remove-key')).toBeVisible()
})
