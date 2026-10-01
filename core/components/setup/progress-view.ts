import type { OperationStatus, ProgressStep } from './use-install-progress'

export interface StepRow {
    // Position in the stream of the row's first event. The stream only appends,
    // so this identifies the row across renders.
    id: string
    step: string
    isCurrent: boolean
    isFailed: boolean
    stepProgress: number | null
}

const isFailure = (step: ProgressStep) => step.message.startsWith('FAILED')

// A step streams many events (every Metro percentage line, every pnpm tick), but
// a user needs one row per step. Only a run of events with the same step merges,
// so a step that comes back later (a failure reported under it) gets its own row.
export function collapseSteps(steps: ProgressStep[], status: OperationStatus): StepRow[] {
    const rows: StepRow[] = []
    for (const [position, event] of steps.entries()) {
        const last = rows[rows.length - 1]
        if (last?.step === event.step) {
            last.isFailed ||= isFailure(event)
            last.stepProgress = event.stepProgress ?? last.stepProgress
            continue
        }
        rows.push({
            id: String(position),
            step: event.step,
            isCurrent: false,
            isFailed: isFailure(event),
            stepProgress: event.stepProgress ?? null,
        })
    }
    const current = rows[rows.length - 1]
    if (current && status === 'running') current.isCurrent = true
    return rows
}

export interface DebugLine extends ProgressStep {
    // Position in the append-only stream; see StepRow.id.
    id: string
}

export function debugLines(steps: ProgressStep[]): DebugLine[] {
    return steps.map((step, position) => ({ ...step, id: String(position) }))
}
