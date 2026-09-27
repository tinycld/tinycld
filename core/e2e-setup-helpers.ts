// Playwright helpers for the first-run setup wizard. They find a step by its
// registry id and its Continue by test id, never by copy, so a spec outside
// core keeps passing when core rewords a step.
//
// This is its own entry point (also re-exported from e2e-helpers) because it
// has no runtime import of @playwright/test: a caller that installs its own
// Playwright can load it without Playwright's "required a second time" error,
// which the full e2e-helpers would trigger by resolving core's copy.
import type { Locator, Page } from '@playwright/test'
import {
    SETUP_CONTINUE_TEST_ID,
    SETUP_FINISH_LATER_TEST_ID,
    SETUP_RESUME_TEST_ID,
    SETUP_SKIP_TEST_ID,
    setupStepTestId,
} from '@tinycld/core/lib/setup/step-ids'

export { CORE_STEP_IDS } from '@tinycld/core/lib/setup/step-ids'

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

/** Leaves the wizard with the shell's Finish later; the app opens. */
export async function finishSetupLater(page: Page) {
    await page.getByTestId(SETUP_FINISH_LATER_TEST_ID).click()
}

/**
 * Opens Settings from the package rail and presses Continue on its Finish
 * setup card. The wizard opens at its first step still to do.
 */
export async function resumeSetupFromSettings(page: Page) {
    await page.getByTestId('nav-settings').click()
    await page.getByTestId(SETUP_RESUME_TEST_ID).click()
}
