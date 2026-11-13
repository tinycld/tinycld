import { expect, type Page, test } from '@playwright/test'
import {
    appShell,
    login,
    TEST_COLLABORATOR_NAME,
    TEST_COLLABORATOR_PASSWORD,
    TEST_COLLABORATOR_USERNAME,
    TEST_USER_NAME,
} from './helpers'

// Sign-out clears the data stores while the signed-in screens are still
// mounted, and react-db keeps their live queries pooled for a few seconds
// after they unmount. Cleaning the collections up under those queries logged
// "[Live Query Error] Source collection '…' was manually cleaned up" once for
// every query and collection. The next sign-in happens in the same tab (the
// SPA survives sign-out), so it must see only the new user's rows.

const LIVE_QUERY_ERROR = /Live Query Error|was manually cleaned up/

async function signOut(page: Page) {
    await page.getByLabel('User menu', { exact: true }).click()
    await page.getByRole('menuitem', { name: 'Sign out', exact: true }).click()
    await expect(page.getByTestId('identifier')).toBeVisible()
}

test('signing out breaks no live query, and the next user sees only their own data', async ({
    page,
}) => {
    const errors: string[] = []
    page.on('console', message => {
        if (LIVE_QUERY_ERROR.test(message.text())) errors.push(message.text())
    })

    await login(page)
    await signOut(page)

    // Same tab, no reload: the stores the first user filled are the ones the
    // second user reads.
    await page.getByTestId('identifier').fill(TEST_COLLABORATOR_USERNAME)
    await page.getByPlaceholder('Password').fill(TEST_COLLABORATOR_PASSWORD)
    await page.getByText('Sign in', { exact: true }).last().click()
    await expect(appShell(page)).toBeVisible()

    await page.getByLabel('User menu', { exact: true }).click()
    const menu = page.getByRole('menu')
    await expect(menu.getByText(TEST_COLLABORATOR_NAME, { exact: true })).toBeVisible()
    await expect(menu.getByText(TEST_USER_NAME, { exact: true })).toHaveCount(0)

    expect(errors).toEqual([])
})
