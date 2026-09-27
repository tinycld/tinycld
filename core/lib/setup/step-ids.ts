// No imports on purpose: Playwright loads this module in plain Node (see
// e2e-setup-helpers.ts), and the wizard renders from the same ids, so a spec
// cannot drift from the markup it looks for.

export const CORE_STEP_IDS = {
    workspace: 'core:workspace',
    apps: 'core:apps',
    email: 'core:email',
    team: 'core:team',
} as const

// Every step's Continue shares one id, scoped by its step container, so a
// spec advances a step without reading its copy.
export const SETUP_CONTINUE_TEST_ID = 'setup-continue'

// The shell's Skip, one per screen. A step whose done state is derived from
// data stays current until that data exists, so Skip is how a spec passes an
// optional step it does not fill.
export const SETUP_SKIP_TEST_ID = 'setup-skip'

// The shell's Finish later, which leaves the wizard for the app.
export const SETUP_FINISH_LATER_TEST_ID = 'setup-finish-later'

// The Continue on Settings' Finish setup card: the one way back into a
// wizard that was left with Finish later.
export const SETUP_RESUME_TEST_ID = 'setup-resume'

export function setupStepTestId(stepId: string): string {
    return `setup-step-${stepId}`
}
