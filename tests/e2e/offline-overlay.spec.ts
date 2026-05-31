import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from './helpers'

test.describe('Offline overlay', () => {
    test('appears when the browser goes offline and dismisses on recovery', async ({
        page,
        context,
    }) => {
        await login(page)

        // Navigate explicitly to the shortcut-stub package. Two purposes:
        // (1) we get a stable, package-independent target that exists in
        // both CI (where only the stub is installed) and local dev (where
        // it lands alongside real packages). The stub is provisioned by
        // app/tests/scripts/scaffold-shortcut-stub.ts.
        // (2) the chunk + screen render are guaranteed to settle before
        // we toggle offline below — otherwise React.lazy's mid-flight
        // fetch fails when the network drops, surfacing a "Failed to
        // fetch" dev overlay that covers the actual offline-overlay.
        await navigateToPackage(page, 'shortcut-stub')
        await expect(page.getByText('Shortcut stub landing')).toBeVisible()

        await expect(page.getByTestId('offline-overlay')).toBeHidden()

        await context.setOffline(true)
        await expect(page.getByTestId('offline-overlay')).toBeVisible({ timeout: 3_000 })
        await expect(page.getByText("You're offline")).toBeVisible()

        await context.setOffline(false)
        await expect(page.getByTestId('offline-overlay')).toBeHidden({ timeout: 2_000 })
    })
})
