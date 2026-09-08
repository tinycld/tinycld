import { expect, test } from '@playwright/test'
import { appShell, login } from './helpers'

// Opening a deep link while signed out used to lose the destination: the gate
// is an overlay, so the URL survived the sign-in form, but LoginModal then
// pushed '/' unconditionally and the landing redirect sent the user to the
// first nav package instead of the thing they clicked.
//
// The target is a settings route because settings is committed in the app shell
// itself, so this spec holds in any assembly — a feature-package route would
// skip whenever that package isn't linked. /settings/personal specifically: it
// is a leaf that renders itself, where bare /a/settings redirects to a section.
test('signing in from a deep link lands on the attempted route', async ({ page }) => {
    await login(page, { startAt: '/a/settings/personal' })

    // Screen first, URL second: a URL changes when the router ACCEPTS a
    // navigation, before the target screen commits, so asserting the URL alone
    // would race the still-loading route (see the appShell note in helpers.ts).
    // The document title is stamped by the screen's own DocumentTitle, so it
    // only matches once that screen is actually mounted.
    await expect(appShell(page)).toBeVisible()
    await expect(page).toHaveTitle(/Settings — Personal$/, { timeout: 20_000 })
    await expect(page).toHaveURL(/\/a\/settings\/personal(?:[/?]|$)/)
})

// The no-pending-route fallback. '/' is filtered out when the gate records it,
// so signing in at the root must still take the landing redirect to the first
// nav package — behavior unchanged by this feature.
test('signing in at the root still lands on the default package', async ({ page }) => {
    await login(page)

    await expect(appShell(page)).toBeVisible()
    // Landed somewhere real, not stranded on the bare root or app prefix.
    await expect(page).not.toHaveURL(/\/a\/?$/)
})
