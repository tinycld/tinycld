import { z } from 'zod'
import type { StepStatus, WizardState } from './types'

const wizardStateSchema = z.object({
    startedAt: z.string(),
    acknowledged: z.array(z.string()),
    skipped: z.array(z.string()),
    dismissedAt: z.string().optional(),
    completedAt: z.string().optional(),
})

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
    const visible = statuses.filter(s => s.isVisible)
    const steps = visible.map(s => ({ id: s.id, label: s.label, phase: phaseOf(s, state) }))
    return {
        steps,
        nextStepId: steps.find(s => s.phase === 'todo')?.id ?? null,
        doneCount: steps.filter(s => s.phase === 'done').length,
        total: steps.length,
        isSettled: visible.every(s => stepIsDone(s, state) !== undefined),
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
