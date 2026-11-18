import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage, skipWithoutShortcutStub } from './helpers'

// The shortcut-stub (tests/scripts/scaffold-shortcut-stub.ts) has a child Stack
// at /a/shortcut-stub/guard whose editor is guarded by core useLeaveGuard. The
// editor's Leave button calls router.replace to the stub's index, a route of
// the PARENT navigator: expo-router targets that navigator and gives the action
// no source. Discard must replay that move as it was held, so the parent
// navigator finds it and the user lands on the index.
//
// Desktop only: the guard and its replay run the same code at a phone width
// (the stub draws no phone-only screen, sheet or tab bar path for this flow).

const LANDING_URL = /\/a\/shortcut-stub$/
const EDITOR_URL = /\/a\/shortcut-stub\/guard\/edit$/

async function openStub(page: Page) {
    await login(page)
    await navigateToPackage(page, 'shortcut-stub', {
        waitFor: page.getByTestId('shortcut-stub-landing'),
    })
}

async function openEditorFromIndex(page: Page) {
    await page.getByTestId('stub-open-guard').click()
    await page.getByTestId('stub-guard-open-editor').click()
    await expect(page.getByTestId('stub-guard-input')).toBeVisible()
    await expect(page).toHaveURL(EDITOR_URL)
}

function discardButton(page: Page) {
    return page.getByTestId('stub-leave-discard')
}

test.describe('leave-guard-replay: a dirty nested screen left by a parent stack replace', () => {
    test.skip(skipWithoutShortcutStub(), 'shortcut-stub not scaffolded in this workspace')

    test('Keep editing stays; Discard lands on the parent route', async ({ page }) => {
        await openStub(page)
        await openEditorFromIndex(page)
        const input = page.getByTestId('stub-guard-input')
        await input.fill('unsaved draft')

        await page.getByTestId('stub-guard-leave').click()
        await expect(discardButton(page)).toBeVisible()
        await page.getByRole('button', { name: 'Keep editing' }).click()
        await expect(discardButton(page)).toHaveCount(0)
        await expect(page).toHaveURL(EDITOR_URL)
        await expect(input).toHaveValue('unsaved draft')

        await page.getByTestId('stub-guard-leave').click()
        await expect(discardButton(page)).toBeVisible()
        await discardButton(page).click()
        await expect(discardButton(page)).toHaveCount(0)
        await expect(page).toHaveURL(LANDING_URL)
        await expect(page.getByTestId('shortcut-stub-landing')).toBeVisible()
        await expect(page.getByTestId('stub-guard-editor')).toHaveCount(0)
    })

    // A reload opens the editor as the first and only route of its stack, so
    // nothing is below it there.
    test('Discard lands on the parent route when the editor is the first route of its stack', async ({
        page,
    }) => {
        await openStub(page)
        await openEditorFromIndex(page)
        await page.reload()
        const input = page.getByTestId('stub-guard-input')
        await expect(input).toBeVisible()
        await expect(page).toHaveURL(EDITOR_URL)
        await input.fill('draft after reload')

        await page.getByTestId('stub-guard-leave').click()
        await expect(discardButton(page)).toBeVisible()
        await discardButton(page).click()
        await expect(discardButton(page)).toHaveCount(0)
        await expect(page).toHaveURL(LANDING_URL)
        await expect(page.getByTestId('shortcut-stub-landing')).toBeVisible()
    })
})
