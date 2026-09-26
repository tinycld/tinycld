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

export function setupStepTestId(stepId: string): string {
    return `setup-step-${stepId}`
}
