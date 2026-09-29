/// <reference lib="dom" />
// The reference is for the page.evaluate callbacks below, which run in the
// browser: a Node-only spec project (hosting's) compiles this file too.
//
// Playwright helpers for the first-run setup wizard. They find a step by its
// registry id and its Continue by test id, never by copy, so a spec outside
// core keeps passing when core rewords a step.
//
// This is its own entry point (also re-exported from e2e-helpers) because it
// has no runtime import of @playwright/test: a caller that installs its own
// Playwright can load it without Playwright's "required a second time" error,
// which the full e2e-helpers would trigger by resolving core's copy.
import { expect, type Locator, type Page } from '@playwright/test'
import {
    CORE_STEP_IDS,
    SETUP_CONTINUE_TEST_ID,
    SETUP_DONE_OPEN_TEST_ID,
    SETUP_DONE_TEST_ID,
    SETUP_INVITE_EMAIL_TEST_ID,
    SETUP_INVITE_SEND_TEST_ID,
    SETUP_INVITE_USERNAME_TEST_ID,
    SETUP_SKIP_TEST_ID,
    SETUP_WORKSPACE_NAME_TEST_ID,
    setupStepTestId,
} from '@tinycld/core/lib/setup/step-ids'

export { CORE_STEP_IDS, SETUP_SKIP_TEST_ID }

export function setupStep(page: Page, stepId: string): Locator {
    return page.getByTestId(setupStepTestId(stepId))
}

export async function expectSetupStep(page: Page, stepId: string) {
    await setupStep(page, stepId).waitFor({ state: 'visible' })
}

export async function continueSetupStep(page: Page, stepId: string) {
    await expectSetupStep(page, stepId)
    await setupStep(page, stepId).getByTestId(SETUP_CONTINUE_TEST_ID).click()
}

/** Skips a step with the shell's Skip, which sits outside the step itself. */
export async function skipSetupStep(page: Page, stepId: string) {
    await expectSetupStep(page, stepId)
    await page.getByTestId(SETUP_SKIP_TEST_ID).click()
}

/** Continues each step in order, waiting for each one to show first. */
export async function continueSetupSteps(page: Page, stepIds: readonly string[]) {
    for (const stepId of stepIds) {
        await continueSetupStep(page, stepId)
    }
}

/**
 * Skips every step still to do, then opens the workspace from the Done
 * screen. The wizard has no other exit: an owner or admin with an unfinished
 * wizard is sent back into it from every app route.
 */
export async function completeSetupBySkipping(page: Page) {
    const done = setupDone(page)
    const skip = page.getByTestId(SETUP_SKIP_TEST_ID)
    // Bounded: a wizard has a handful of steps, and each Skip moves past one.
    for (let i = 0; i < 20; i++) {
        await expect(done.or(skip)).toBeVisible()
        if (await done.isVisible()) break
        const step = page.getByTestId(/^setup-step-/).first()
        const stepId = await step.getAttribute('data-testid')
        await skip.click()
        // Wait for the skipped step to leave before pressing Skip again, so
        // one press never skips two steps.
        await expect(page.getByTestId(stepId ?? '')).toHaveCount(0)
    }
    await done.getByTestId(SETUP_DONE_OPEN_TEST_ID).click()
    await expect(page.getByTestId('nav-settings')).toBeVisible()
}

/** The organization step's name field. */
export function setupWorkspaceName(page: Page): Locator {
    return setupStep(page, CORE_STEP_IDS.workspace).getByTestId(SETUP_WORKSPACE_NAME_TEST_ID)
}

/** A real 1x1 PNG: the logo cropper decodes the bytes, so they must decode. */
export const ONE_PIXEL_PNG_BASE64 =
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='

/**
 * Picks a logo on the organization step through the real file picker and
 * saves the cropper, the same way avatar.spec.ts attaches a photo.
 *
 * Playwright's chooser interception suppresses the native dialog, and with it
 * the window blur → focus round trip a real dialog causes. The picker runs
 * inside that round trip for every real user, so the helper restores it: blur
 * as the chooser opens, refocus as it closes, and only then deliver the file.
 */
export async function attachSetupLogo(
    page: Page,
    file: { name: string; mimeType: string; buffer: Buffer }
) {
    const step = setupStep(page, CORE_STEP_IDS.workspace)
    const chooserPromise = page.waitForEvent('filechooser')
    await step.getByTestId('avatar-upload').click()
    const chooser = await chooserPromise
    await page.evaluate(() => window.dispatchEvent(new Event('blur')))
    await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    await page.evaluate(
        () => new Promise<void>(resolve => requestAnimationFrame(() => setTimeout(resolve, 0)))
    )
    await chooser.setFiles(file)
    await page.getByTestId('avatar-cropper-save').click()
}

/** Sends one invite from the team step's form; email may be left out. */
export async function inviteFromSetup(page: Page, invite: { username: string; email?: string }) {
    const team = setupStep(page, CORE_STEP_IDS.team)
    await team.getByTestId(SETUP_INVITE_USERNAME_TEST_ID).fill(invite.username)
    if (invite.email) await team.getByTestId(SETUP_INVITE_EMAIL_TEST_ID).fill(invite.email)
    await team.getByTestId(SETUP_INVITE_SEND_TEST_ID).click()
}

/** The Done screen, the wizard's last; it has no registry step. */
export function setupDone(page: Page): Locator {
    return page.getByTestId(SETUP_DONE_TEST_ID)
}
