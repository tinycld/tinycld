import { createServer, type Server } from 'node:http'
import { expect, type Page, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from './helpers'

// Drives a manual backup from Settings → Backups to a local PUT sink, then
// checks the ledger row through the UI. Restore is not exercised here: it
// restarts the server, which the shared e2e server cannot survive mid-suite.

interface Sink {
    url: string
    received: () => number
    close: () => Promise<void>
}

// The sink the server PUTs the archive to. It listens on 127.0.0.1, which the
// server can reach because e2e runs both on the same machine, and on port 0 so
// two workers never collide.
async function startSink(): Promise<Sink> {
    let bytes = 0
    const server: Server = createServer((req, res) => {
        req.on('data', (chunk: Buffer) => {
            bytes += chunk.length
        })
        req.on('end', () => {
            res.statusCode = 200
            res.end()
        })
    })
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', () => resolve()))
    const address = server.address()
    const port = typeof address === 'object' && address ? address.port : 0
    return {
        url: `http://127.0.0.1:${port}/backup.age`,
        received: () => bytes,
        close: () => new Promise<void>(resolve => server.close(() => resolve())),
    }
}

// Both BackupNowForm and RestoreForm render a field named `passphrase` labelled
// "Passphrase" (TextInput stamps testID={name} and accessibilityLabel={label}),
// and the fixture user is an owner, so the restore form is on screen too. Every
// fill is scoped to the `backup-now-form` card so a selector can never reach
// across into the restore form — which would fill the wrong field and leave the
// assertion passing against a form the test never touched.
async function fillBackupForm(page: Page, target: string, passphrase: string) {
    const card = page.getByTestId('backup-now-form')
    await card.getByTestId('target').fill(target)
    await card.getByTestId('passphrase').fill(passphrase)
    await card.getByTestId('confirm').fill(passphrase)
}

test.describe('Settings · Backups', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'settings')
        await clickSidebarItem(page, 'Backups')
        await page
            .getByTestId('settings-section-backups')
            .waitFor({ state: 'visible', timeout: 20_000 })
    })

    test('backs up to a PUT URL and records it', async ({ page }) => {
        const sink = await startSink()
        try {
            await fillBackupForm(page, sink.url, 'correct horse battery staple')
            await page.getByTestId('backup-start').click()

            const row = page.locator('[data-testid^="backup-row-"]').first()
            await expect(row).toContainText('Manual backup', { timeout: 20_000 })
            await expect(row).toContainText('Succeeded', { timeout: 60_000 })
            expect(sink.received()).toBeGreaterThan(0)
            await expect(page.getByTestId('backups-last')).toContainText('Last backed up')
        } finally {
            await sink.close()
        }
    })

    test('rejects a short passphrase', async ({ page }) => {
        await fillBackupForm(page, 'https://example.com/x', 'short')
        await expect(
            page.getByTestId('backup-now-form').getByText('At least 12 characters')
        ).toBeVisible()
    })
})
