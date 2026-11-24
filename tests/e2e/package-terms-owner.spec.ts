import { expect, type Page, test } from '@playwright/test'
import { APP_PREFIX } from '@tinycld/core/lib/org-routes'
import type PocketBase from 'pocketbase'
import { CORE_STEP_IDS, setupStep } from '../../core/e2e-setup-helpers'
import {
    loginOnSecond,
    PHONE_VIEWPORT,
    resetStubTerms,
    skipWithoutTermsStub,
    TERMS_STUB,
    termsSuperuser,
} from './package-terms-helpers'

// The owner's decisions on the terms of a package: the first launch, decline,
// accept later from Settings, and the Apps step of the setup wizard.
//
// FIXTURES BY SUPERUSER WRITE.
//
// 1. (ruling 12, approved by the human 2026-11-23) The terms state of the
//    termsstub package is reset before every test and put back to "declined"
//    after it. Why the UI cannot do it: an acceptance is final in the app, and
//    a decline can only become an acceptance. No screen takes either back, so
//    without the reset only the first test of a database could ever see an
//    undecided package.
// 2. (ruling 30: NOT YET APPROVED by the human) The wizard test creates the
//    `setup.wizard` settings row. Why the UI cannot do it: only the two server
//    paths that create the first owner write that row, and the e2e database is
//    seeded with its users.
// 3. (ruling 30: NOT YET APPROVED by the human) The wizard test turns the stub
//    off first (registry status `disabled`) and puts the status back after.
//    Why the UI cannot do it: the Apps step opens the terms of an app with
//    unaccepted terms and does not toggle it. With the app off, "Agree turns
//    the app on" is shown by the test.
//
// What this therefore does NOT cover: the state a real deployment is in right
// after the server first registers a package with terms (the boot-time sync,
// covered by the Go tests of core/server/pkgterms), and the wizard as a new
// deployment opens it (tests/standalone/first-run-wizard.spec.ts). Every
// decision under test is made through the UI.
//
// This file runs on the SECOND deployment. The terms state of a package is
// one value for the whole deployment, so on the primary one these tests would
// open and close the stub under every other spec of the run. The tests of
// this file run in order in one worker.
//
// Not covered: the gate is in the client only; no test asserts that the
// server refuses a request.

const WIZARD_KEY = 'setup.wizard'

async function openStub(page: Page) {
    await page.getByTestId(`nav-${TERMS_STUB.slug}`).click()
}

test.describe('package terms: the owner decides', () => {
    test.skip(skipWithoutTermsStub)

    let pb: PocketBase

    test.beforeAll(async () => {
        pb = await termsSuperuser('second')
    })

    test.beforeEach(async () => {
        await resetStubTerms(pb, 'undecided')
    })

    test.afterEach(async () => {
        await resetStubTerms(pb, 'declined')
    })

    test('the first launch sends the owner to the package terms', async ({ page }) => {
        await loginOnSecond(page)
        await expect(page.getByTestId('settings-package-terms')).toBeVisible()
        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText(
            'Not accepted yet'
        )

        await page.getByTestId(`pkg-terms-open-${TERMS_STUB.slug}`).click()
        const dialog = page.getByTestId('pkg-terms-dialog')
        await expect(dialog.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await dialog.getByTestId('pkg-terms-dialog-agree').click()

        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText('Accepted')
        await openStub(page)
        await expect(page.getByTestId(TERMS_STUB.landing)).toBeVisible()
        await expect(page.getByTestId('pkg-terms-review')).toHaveCount(0)
    })

    test('a declined package stays closed, and the next launch does not ask again', async ({
        page,
    }) => {
        await loginOnSecond(page, TERMS_STUB.slug)
        const review = page.getByTestId('pkg-terms-review')
        await expect(review.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await review.getByTestId('pkg-terms-decline').click()
        await expect(review.getByTestId('pkg-terms-declined')).toBeVisible()
        await expect(review.getByTestId('pkg-terms-decline')).toHaveCount(0)

        // A new launch: the owner lands on a package, not on the terms. Wait
        // for the launch redirect to settle before reading where it went.
        await loginOnSecond(page)
        await expect
            .poll(() => new URL(page.url()).pathname)
            .toMatch(new RegExp(`^${APP_PREFIX}/(?!settings/package-terms)[^/]+`))
        expect(page.url()).not.toContain('package-terms')
    })

    test('the owner accepts later from Settings, and the package opens with its surfaces', async ({
        page,
    }) => {
        await resetStubTerms(pb, 'declined')
        await loginOnSecond(page)
        // The rail does not wait for the terms state. The review cover shows
        // only when that state is ready and the stub is pending, so the
        // negative check below reads a loaded state.
        await openStub(page)
        await expect(
            page.getByTestId('pkg-terms-review').getByTestId('pkg-terms-declined')
        ).toBeVisible()
        await page.getByTestId('nav-settings').click()
        await expect(page.getByTestId(TERMS_STUB.settingsLink)).toHaveCount(0)
        await page.getByTestId('settings-link-package-terms').click()
        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText('Declined')

        await page.getByTestId(`pkg-terms-open-${TERMS_STUB.slug}`).click()
        await page.getByTestId('pkg-terms-dialog-agree').click()
        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText('Accepted')

        // Accepted terms can still be read; there is nothing left to decide.
        await page.getByTestId(`pkg-terms-open-${TERMS_STUB.slug}`).click()
        await expect(page.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await expect(page.getByTestId('pkg-terms-dialog-agree')).toHaveCount(0)
        await page.getByTestId('pkg-terms-dialog-close').click()

        await page.getByTestId('nav-settings').click()
        await page.getByTestId(TERMS_STUB.settingsLink).click()
        await expect(page.getByTestId(TERMS_STUB.settingsPanel)).toBeVisible()

        await page.getByTestId('nav-help').click()
        await page.getByPlaceholder('Search help topics').fill('Terms stub')
        await expect(page.getByText(TERMS_STUB.helpTopic).first()).toBeVisible()

        await openStub(page)
        await expect(page.getByTestId(TERMS_STUB.landing)).toBeVisible()
        await expect(page.getByTestId(TERMS_STUB.sidebar)).toBeVisible()
    })

    test('on a phone the owner reads and accepts the terms on the package route', async ({
        page,
    }) => {
        await page.setViewportSize(PHONE_VIEWPORT)
        await loginOnSecond(page, TERMS_STUB.slug)
        const review = page.getByTestId('pkg-terms-review')
        await expect(review.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await review.getByTestId('pkg-terms-agree').click()
        await expect(page.getByTestId('pkg-terms-review')).toHaveCount(0)
        await expect(page.getByTestId(TERMS_STUB.landing)).toBeVisible()
    })

    test('on a phone the first launch shows the terms in a sheet and the owner agrees', async ({
        page,
    }) => {
        await page.setViewportSize(PHONE_VIEWPORT)
        await loginOnSecond(page)
        await expect(page.getByTestId('settings-package-terms')).toBeVisible()

        await page.getByTestId(`pkg-terms-open-${TERMS_STUB.slug}`).click()
        const dialog = page.getByTestId('pkg-terms-dialog')
        await expect(dialog.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
        await dialog.getByTestId('pkg-terms-dialog-agree').click()
        await expect(page.getByTestId(`pkg-terms-state-${TERMS_STUB.slug}`)).toHaveText('Accepted')
    })

    test.describe('the setup wizard', () => {
        // Waits on ruling 30 (the human's approval of fixtures 2 and 3 in the header).
        // Set only when this run made the row: a row left by a stopped run is
        // used as it is and stays.
        let madeWizardRowId = ''
        let stubRowId = ''
        let stubStatus = ''

        test.beforeAll(async () => {
            // A wizard that is complete: its steps stay open by their URL, and
            // nobody on this deployment is sent into the wizard meanwhile.
            const settings = pb.collection('system_settings')
            const found = await settings.getList(1, 1, {
                filter: pb.filter('key = {:key}', { key: WIZARD_KEY }),
            })
            if (found.totalItems === 0) {
                const now = new Date().toISOString()
                const value = JSON.stringify({
                    startedAt: now,
                    acknowledged: [],
                    skipped: [],
                    completedAt: now,
                })
                madeWizardRowId = (await settings.create({ key: WIZARD_KEY, value })).id
            }

            // The app is off before the owner agrees, so the test shows that
            // the acceptance turns it on.
            const stub = await pb
                .collection('pkg_registry')
                .getFirstListItem(pb.filter('slug = {:slug}', { slug: TERMS_STUB.slug }))
            stubRowId = stub.id
            stubStatus = String(stub.status)
            await pb.collection('pkg_registry').update(stubRowId, { status: 'disabled' })
        })

        test.afterAll(async () => {
            if (stubRowId !== '') {
                await pb.collection('pkg_registry').update(stubRowId, { status: stubStatus })
            }
            if (madeWizardRowId !== '') {
                await pb.collection('system_settings').delete(madeWizardRowId)
            }
        })

        test('the Apps step shows the terms, and agreeing turns the app on', async ({ page }) => {
            const step = setupStep(page, CORE_STEP_IDS.apps)
            // A setup step renders outside the app shell: the step itself
            // shows that the sign-in is done.
            await loginOnSecond(page, 'setup/core.apps', step)
            await expect(step).toBeVisible()
            const card = page.getByTestId(`setup-app-${TERMS_STUB.slug}`)
            await expect(card).toHaveAttribute('aria-checked', 'false')
            await expect(page.getByTestId(`setup-app-terms-${TERMS_STUB.slug}`)).toBeVisible()

            // Closing the terms leaves the app off.
            await card.click()
            const dialog = page.getByTestId('pkg-terms-dialog')
            await expect(dialog.getByTestId('pkg-terms-body')).toContainText(TERMS_STUB.sentence)
            await dialog.getByTestId('pkg-terms-dialog-close').click()
            await expect(dialog).toHaveCount(0)
            await expect(card).toHaveAttribute('aria-checked', 'false')
            // Read-only check: closing the terms recorded no acceptance.
            const accepted = await pb.collection('pkg_terms_decisions').getList(1, 1, {
                filter: pb.filter('pkg = {:slug} && decision = {:d}', {
                    slug: TERMS_STUB.slug,
                    d: 'accepted',
                }),
            })
            expect(accepted.totalItems).toBe(0)

            await card.click()
            await page.getByTestId('pkg-terms-dialog-agree').click()
            await expect(card).toHaveAttribute('aria-checked', 'true')
            await expect(page.getByTestId(`setup-app-terms-${TERMS_STUB.slug}`)).toHaveCount(0)
            // The dialog closes only after both writes are saved.
            await expect(dialog).toHaveCount(0)
            // Read-only check: the acceptance also turned the app on.
            const stub = await pb.collection('pkg_registry').getOne(stubRowId)
            expect(stub.status).not.toBe('disabled')
        })
    })
})
