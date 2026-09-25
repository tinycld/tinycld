import type { WizardSummary } from '@tinycld/core/lib/setup/wizard-logic'

/** Progress for the two pre-auth screens, which are not registry steps. */
export function claimSummary(isCodeDone: boolean): WizardSummary {
    return {
        steps: [
            { id: 'claim:code', label: 'Setup code', phase: isCodeDone ? 'done' : 'todo' },
            { id: 'claim:account', label: 'Owner account', phase: 'todo' },
        ],
        nextStepId: null,
        doneCount: isCodeDone ? 1 : 0,
        total: 2,
        isSettled: true,
    }
}
