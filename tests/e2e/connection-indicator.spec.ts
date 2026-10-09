import { expect, test } from '@playwright/test'
import { login, navigateToPackage, skipWithoutShortcutStub } from './helpers'

test.describe('Connection notice', () => {
    // Drives the shortcut-stub package as a stable navigation target; needs it
    // scaffolded. Skip on a plain dev workspace where it isn't present (CI's scaffold
    // step makes it mandatory there — see skipWithoutShortcutStub).
    test.skip(skipWithoutShortcutStub(), 'shortcut-stub not scaffolded in this workspace')

    test('says offline when the browser goes offline and stops on recovery', async ({
        page,
        context,
    }) => {
        await login(page)

        // Navigate explicitly to the shortcut-stub package. Two purposes:
        // (1) we get a stable, package-independent target that exists in
        // both CI (where only the stub is installed) and local dev (where
        // it lands alongside real packages). The stub is provisioned by
        // app/tests/scripts/scaffold-shortcut-stub.ts.
        // (2) the explicit waitFor on the landing text guarantees the
        // stub's lazy chunk has settled before we toggle offline below
        // — otherwise React.lazy's mid-flight fetch fails when the
        // network drops, surfacing a "Failed to fetch" dev overlay
        // that covers the connection notice.
        await navigateToPackage(page, 'shortcut-stub', {
            waitFor: page.getByText('Shortcut stub landing', { exact: true }),
        })

        const offlineNotice = page.getByText('Offline — waiting for a connection', { exact: true })
        await expect(page.getByTestId('connection-indicator')).toBeHidden()

        await context.setOffline(true)
        await expect(offlineNotice).toBeVisible({ timeout: 3_000 })

        // Live updates may still be reconnecting once the network is back, so
        // only the offline wording must go at once.
        await context.setOffline(false)
        await expect(offlineNotice).toBeHidden({ timeout: 2_000 })
    })
})
