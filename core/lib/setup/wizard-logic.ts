import { z } from 'zod'
import { CORE_STEP_IDS } from './step-ids'
import type { StepStatus, WizardState } from './types'

const wizardStateSchema = z.object({
    startedAt: z.string(),
    acknowledged: z.array(z.string()),
    skipped: z.array(z.string()),
    dismissedAt: z.string().optional(),
    completedAt: z.string().optional(),
    orgNameSeeded: z.boolean().optional(),
})

export const WORKSPACE_STEP_ID = CORE_STEP_IDS.workspace

export function parseWizardState(raw: string | undefined): WizardState | null {
    if (!raw) return null
    try {
        const parsed = wizardStateSchema.safeParse(JSON.parse(raw))
        return parsed.success ? parsed.data : null
    } catch {
        return null
    }
}

/** undefined: still loading. */
export function stepIsDone(status: StepStatus, state: WizardState): boolean | undefined {
    if (status.isDone === null) return state.acknowledged.includes(status.id)
    return status.isDone
}

const addTo = (list: string[], id: string) => (list.includes(id) ? list : [...list, id])

/**
 * The state after Continue on a step. A step whose derived state says "not
 * done" (or has not loaded yet) is recorded as skipped: otherwise resume would
 * open the same step again and Continue would appear to do nothing. A step
 * with no derived state is acknowledged, which is what makes it done. A skip
 * is cleared only when the step is done, so Continue never hides a step that
 * still needs work.
 */
export function continueStep(state: WizardState, status: StepStatus): WizardState {
    if (status.isDone === null || status.isDone === true) {
        return {
            ...state,
            acknowledged: addTo(state.acknowledged, status.id),
            skipped: state.skipped.filter(x => x !== status.id),
        }
    }
    return { ...state, skipped: addTo(state.skipped, status.id) }
}

export function skipStep(state: WizardState, id: string): WizardState {
    return { ...state, skipped: addTo(state.skipped, id) }
}

export interface WizardSummary {
    steps: { id: string; label: string; phase: 'done' | 'skipped' | 'todo' }[]
    nextStepId: string | null
    doneCount: number
    total: number
    isSettled: boolean
}

function phaseOf(status: StepStatus, state: WizardState): 'done' | 'skipped' | 'todo' {
    if (stepIsDone(status, state)) return 'done'
    if (state.skipped.includes(status.id)) return 'skipped'
    return 'todo'
}

export function summarizeWizard(statuses: StepStatus[], state: WizardState): WizardSummary {
    const visible = statuses.filter(s => s.isVisible === true)
    const steps = visible.map(s => ({ id: s.id, label: s.label, phase: phaseOf(s, state) }))
    return {
        steps,
        nextStepId: steps.find(s => s.phase === 'todo')?.id ?? null,
        doneCount: steps.filter(s => s.phase === 'done').length,
        total: steps.length,
        isSettled:
            statuses.every(s => s.isVisible !== undefined) &&
            visible.every(s => stepIsDone(s, state) !== undefined),
    }
}

export function shouldOpenWizard(input: {
    role: string | null
    state: WizardState | null
    isSettled: boolean
}): boolean {
    if (!input.isSettled || !input.state) return false
    if (input.role !== 'owner' && input.role !== 'admin') return false
    return !input.state.dismissedAt && !input.state.completedAt
}

/**
 * The saved workspace name, or '' while nobody has chosen one. PocketBase
 * names every new server "Acme", so the saved name counts only once the
 * workspace step was acknowledged or the provisioner set it
 * (`create-owner --org-name`). Unknown state reads as unchosen.
 */
export function chosenOrgName(orgName: string, state: WizardState | null): string {
    if (!state) return ''
    const isChosen = state.acknowledged.includes(WORKSPACE_STEP_ID) || state.orgNameSeeded === true
    return isChosen ? orgName : ''
}
