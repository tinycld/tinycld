import type { LoadedSetupStep, StepStatus } from '@tinycld/core/lib/setup/types'
import { useSetupWizardState } from '@tinycld/core/lib/setup/use-setup-wizard-state'
import { shouldOpenWizard } from '@tinycld/core/lib/setup/wizard-logic'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import type { ReactNode } from 'react'

type StatusRender = (statuses: StepStatus[]) => ReactNode

/**
 * Each step's hooks live in its own module, so they cannot be called in a
 * loop in one component. The chain renders one tiny component per step; each
 * calls its step's hooks at top level and passes the growing status list on.
 * No effects, no state: statuses are recomputed on every render.
 */
export function StepStatusChain({
    steps,
    index = 0,
    acc = [],
    children,
}: {
    steps: LoadedSetupStep[]
    index?: number
    acc?: StepStatus[]
    children: StatusRender
}) {
    if (index === steps.length) return <>{children(acc)}</>
    return (
        <StepStatusLink step={steps[index]} steps={steps} index={index} acc={acc}>
            {children}
        </StepStatusLink>
    )
}

function StepStatusLink({
    step,
    steps,
    index,
    acc,
    children,
}: {
    step: LoadedSetupStep
    steps: LoadedSetupStep[]
    index: number
    acc: StepStatus[]
    children: StatusRender
}) {
    const isDone = step.useIsStepDone()
    const isVisible = step.useIsStepVisible()
    const next = [...acc, { id: step.id, label: step.label, isDone, isVisible }]
    return (
        <StepStatusChain steps={steps} index={index + 1} acc={next}>
            {children}
        </StepStatusChain>
    )
}

const addTo = (list: string[], id: string) => (list.includes(id) ? list : [...list, id])
const now = () => new Date().toISOString()

/**
 * `update` patches the state as of this render, so two updates fired without
 * awaiting the first would each start from the same state and one would be
 * lost. Every caller awaits one action before it navigates or runs another.
 */
export function useWizardActions() {
    const { state, isReady, update } = useSetupWizardState()
    return {
        state,
        isReady,
        skip: (id: string) => update(s => ({ ...s, skipped: addTo(s.skipped, id) })),
        acknowledge: (id: string) =>
            update(s => ({
                ...s,
                acknowledged: addTo(s.acknowledged, id),
                skipped: s.skipped.filter(x => x !== id),
            })),
        finishLater: () => update(s => ({ ...s, dismissedAt: now() })),
        complete: () => update(s => ({ ...s, completedAt: now() })),
    }
}

export type WizardActions = ReturnType<typeof useWizardActions>

export function useShouldOpenSetupWizard(): boolean {
    const { role, isReady: roleReady } = useCurrentRole()
    const { state, isReady } = useSetupWizardState()
    return shouldOpenWizard({ role, state, isSettled: roleReady && isReady })
}
