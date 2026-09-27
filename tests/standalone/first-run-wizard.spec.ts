import { expect, type Page, test } from '@playwright/test'
import { bootBinary, buildBinary, setupCodeFromLog } from './boot-binary'

// Drives the real first-run path on a fresh binary. The code is read from the
// server log, the same way a person reads it.
//
// No forced serial mode: each test boots its own binary on its own data dir
// and port, so the three tests are independent. tests/standalone/playwright.config.ts
// already runs with workers: 1.

const OWNER = { name: 'Dana Reyes', email: 'dana@example.com', password: 'OwnerPass1234!' }

type Server = Awaited<ReturnType<typeof bootBinary>>

async function readCode(server: Server): Promise<string> {
    await expect.poll(() => setupCodeFromLog(server.logs()), { timeout: 30_000 }).not.toBeNull()
    return setupCodeFromLog(server.logs()) as string
}

async function claimServer(page: Page, server: Server) {
    const code = await readCode(server)
    await page.goto(server.baseURL)
    await expect(page.getByText('Claim this server')).toBeVisible()
    await page.getByTestId('setup-code').fill(`${code.slice(0, 4)}-${code.slice(4)}`)
    await page.getByRole('button', { name: 'Continue' }).click()

    await page.getByRole('textbox', { name: 'Name', exact: true }).fill(OWNER.name)
    await page.getByRole('textbox', { name: 'Email', exact: true }).fill(OWNER.email)
    await page.getByRole('textbox', { name: 'Password', exact: true }).fill(OWNER.password)
    await page.getByRole('textbox', { name: 'Confirm password', exact: true }).fill(OWNER.password)
    await page.getByRole('button', { name: 'Create account' }).click()
    await expect(page.getByText('Your workspace')).toBeVisible()
}

async function signIn(page: Page, baseURL: string) {
    await page.goto(baseURL)
    await page.getByTestId('identifier').fill(OWNER.email)
    await page.getByPlaceholder('Password').fill(OWNER.password)
    await page.getByRole('button', { name: 'Sign in' }).click()
}

test.beforeAll(() => buildBinary())

test('a wrong code is refused with a specific message', async ({ page }) => {
    const server = await bootBinary()
    try {
        await readCode(server)
        await page.goto(server.baseURL)
        await page.getByTestId('setup-code').fill('AAAA-AAAA')
        await page.getByRole('button', { name: 'Continue' }).click()
        await expect(
            page.getByText('That code does not match. Check the server log for the latest code.')
        ).toBeVisible()
    } finally {
        await server.stop()
    }
})

test('a new server is claimed, set up, paused and resumed', async ({ page }) => {
    const server = await bootBinary()
    try {
        await claimServer(page, server)

        await page
            .getByRole('textbox', { name: 'Workspace name', exact: true })
            .fill('Harbor Dental')
        await page.getByRole('button', { name: 'Continue' }).click()

        await expect(page.getByText('Choose your apps')).toBeVisible()
        await page.getByRole('button', { name: 'Finish later' }).click()

        // Finish later lands in the app; Settings offers to resume.
        await page.getByTestId('nav-settings').click()
        await expect(page.getByText('Finish setup')).toBeVisible()
        await page.getByRole('button', { name: 'Continue' }).click()
        await expect(page.getByText('Choose your apps')).toBeVisible()
    } finally {
        await server.stop()
    }
})

test('a hidden app stays hidden after a restart', async ({ page }) => {
    // End to end, not only in a Go test, because the bug was the boot path
    // itself re-enabling the row.
    const first = await bootBinary()
    const dataDir = first.dataDir
    let hiddenSlug = ''
    try {
        await claimServer(page, first)
        await page.getByRole('button', { name: 'Skip' }).click()
        await expect(page.getByText('Choose your apps')).toBeVisible()
        const card = page.getByTestId(/^setup-app-/).first()
        hiddenSlug = ((await card.getAttribute('data-testid')) ?? '').replace('setup-app-', '')
        await card.click()
    } finally {
        await first.stop()
    }

    const second = await bootBinary({ dataDir })
    try {
        await signIn(page, second.baseURL)
        await page.getByRole('button', { name: 'Finish later' }).click()
        await expect(page.getByTestId('nav-settings')).toBeVisible()
        await expect(page.getByTestId(`nav-${hiddenSlug}`)).toHaveCount(0)
    } finally {
        await second.stop()
    }
})
