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

export function setupStepTestId(stepId: string): string {
    return `setup-step-${stepId}`
}
