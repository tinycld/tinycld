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
})
