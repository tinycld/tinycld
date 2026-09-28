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

// Fields and buttons a spec fills on core's own steps, and the Done screen,
// which has no registry step and so no step container id.
export const SETUP_WORKSPACE_NAME_TEST_ID = 'setup-workspace-name'
export const SETUP_INVITE_USERNAME_TEST_ID = 'setup-invite-username'
export const SETUP_INVITE_EMAIL_TEST_ID = 'setup-invite-email'
export const SETUP_INVITE_SEND_TEST_ID = 'setup-invite-send'
export const SETUP_DONE_TEST_ID = 'setup-done'
// The Done screen's button, which marks setup complete and opens the workspace.
export const SETUP_DONE_OPEN_TEST_ID = 'setup-done-open'

export function setupStepTestId(stepId: string): string {
    return `setup-step-${stepId}`
}
