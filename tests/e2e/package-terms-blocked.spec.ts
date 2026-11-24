import { expect, test } from '@playwright/test'
import { appHref } from '@tinycld/core/lib/org-routes'
import { login, loginAs, TEST_COLLABORATOR_PASSWORD, TEST_COLLABORATOR_USERNAME } from './helpers'
import {
    PHONE_VIEWPORT,
    packageNavEntries,
    skipWithoutTermsStub,
    TERMS_STUB,
    termsSuperuser,
} from './package-terms-helpers'

// A package whose terms the owner has not accepted: what everybody sees.
//
// This file writes nothing. The termsstub package declares terms, and its seed
// records that the owner declined them, so on the shared e2e database the stub
// is closed for the whole run. The owner's decisions (accept, decline, the
// first launch, the setup wizard) are in package-terms-owner.spec.ts, on the
// second deployment.
//
// Not covered here: the gate is in the client only. The server answers a
// request for the stub's records and routes as for any other package, and no
// test here asserts a refusal.

const STUB_ROUTE = appHref(TERMS_STUB.slug)

test.describe('package terms: a package nobody may use yet', () => {
    test.skip(skipWithoutTermsStub)

    test('the server holds the terms version and text of the package', async () => {
        const pb = await termsSuperuser('primary')
        const row = await pb
            .collection('pkg_registry')
            .getFirstListItem(pb.filter('slug = {:slug}', { slug: TERMS_STUB.slug }))
        expect(row.terms_version).toBe(TERMS_STUB.version)
        expect(row.terms_accepted_version).toBe('')
        const terms = await pb
            .collection('pkg_terms')
            .getFirstListItem(pb.filter('pkg = {:slug}', { slug: TERMS_STUB.slug }))
        expect(terms.version).toBe(TERMS_STUB.version)
        expect(terms.body).toContain(TERMS_STUB.sentence)
    })

    test('a member has no nav entry and sees the waiting screen on the route', async ({ page }) => {
        await loginAs(page, TEST_COLLABORATOR_USERNAME, TEST_COLLABORATOR_PASSWORD, {
            startAt: STUB_ROUTE,
        })
        await expect(page.getByTestId('pkg-terms-waiting')).toBeVisible()
        await expect(page.getByTestId('pkg-terms-waiting')).toContainText(
            'Waiting for the owner to accept the terms'
        )
        // The member cannot decide.
        await expect(page.getByTestId('pkg-terms-agree')).toHaveCount(0)
        // The member's rail has loaded (an entry of another package shows),
        // and the stub is not in it.
        await expect(packageNavEntries(page).first()).toBeVisible()
        await expect(page.getByTestId(`nav-${TERMS_STUB.slug}`)).toHaveCount(0)
    })

    test('a member on a phone sees the waiting screen and no tab', async ({ page }) => {
        await page.setViewportSize(PHONE_VIEWPORT)
        await loginAs(page, TEST_COLLABORATOR_USERNAME, TEST_COLLABORATOR_PASSWORD, {
            startAt: STUB_ROUTE,
        })
        await expect(page.getByTestId('pkg-terms-waiting')).toBeVisible()
        // The tab bar has loaded, and neither it nor the More list has the stub.
        await expect(packageNavEntries(page).first()).toBeVisible()
        await page.getByTestId('nav-more').click()
        // The More list is open: a count of 0 alone also passes while it is shut.
        await expect(page.getByTestId('more-drawer')).toBeVisible()
        await expect(page.getByTestId(`nav-${TERMS_STUB.slug}`)).toHaveCount(0)
    })

    test('the owner keeps the nav entry and sees the terms with the declined note', async ({
        page,
    }) => {
        await login(page)
        await page.getByTestId(`nav-${TERMS_STUB.slug}`).click()
        const review = page.getByTestId('pkg-terms-review')
        await expect(review).toBeVisible()
        await expect(review.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await expect(review.getByTestId('pkg-terms-declined')).toBeVisible()
        await expect(review.getByTestId('pkg-terms-agree')).toBeVisible()
        // The package sidebar is hidden while the terms are open.
        await expect(page.getByTestId(TERMS_STUB.sidebar)).toHaveCount(0)
    })

    test('settings, help and search do not show the package', async ({ page }) => {
        await login(page)

        await page.getByTestId('nav-settings').click()
        await expect(page.getByTestId('settings-index')).toBeVisible()
        await expect(page.getByTestId(TERMS_STUB.settingsLink)).toHaveCount(0)

        await page.getByTestId('nav-help').click()
        const searchBox = page.getByPlaceholder('Search help topics')
        await searchBox.fill('Terms stub')
        await expect(searchBox).toHaveValue('Terms stub')
        await expect(page.getByText(TERMS_STUB.helpTopic)).toHaveCount(0)

        // The palette: a package name and a colon make a scope chip, but not
        // for a closed package. The text stays plain text.
        await searchBox.fill('')
        await page.getByTestId('nav-settings').click()
        await page.keyboard.press('/')
        const palette = page.getByPlaceholder('Search everywhere…')
        await expect(palette).toBeVisible()
        await page.keyboard.type(`${TERMS_STUB.slug}:`)
        await expect(palette).toHaveValue(`${TERMS_STUB.slug}:`)
        await expect(page.getByTestId(`search-chip-${TERMS_STUB.slug}`)).toHaveCount(0)
    })

    test('the settings panel of the package reads not found', async ({ page }) => {
        await login(page, { startAt: appHref(`settings/${TERMS_STUB.slug}/general`) })
        await expect(page.getByText('Settings not found')).toBeVisible()
        await expect(page.getByTestId(TERMS_STUB.settingsPanel)).toHaveCount(0)
    })
})
