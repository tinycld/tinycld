import type { ComponentType } from 'react'

export interface SetupStepProps {
    /** Call after the step's own save succeeds; the shell moves on. */
    next: () => void
}

/** What a step module exports. Hooks are optional; the registry fills defaults. */
export interface SetupStepModule {
    default: ComponentType<SetupStepProps>
    /** Derived from real data. undefined while loading. Absent: done once acknowledged. */
    useIsStepDone?: () => boolean | undefined
    /** undefined while the answer loads: the step is not shown, and the wizard waits before resuming. Absent: always shown. */
    useIsStepVisible?: () => boolean | undefined
}

export interface SetupStepEntry {
    id: string // `<slug>:<id>`
    label: string
    order: string | null
    load: () => Promise<SetupStepModule>
}

export interface LoadedSetupStep {
    id: string
    label: string
    Component: ComponentType<SetupStepProps>
    /** null: the step has no derived state; `acknowledged` decides. */
    useIsStepDone: () => boolean | undefined | null
    useIsStepVisible: () => boolean | undefined
}

export interface StepStatus {
    id: string
    label: string
    /** undefined while the step does not yet know whether it shows. */
    isVisible: boolean | undefined
    /** true / false; undefined while its data loads; null when the step has no derived state. */
    isDone: boolean | undefined | null
}

export interface WizardState {
    startedAt: string
    acknowledged: string[]
    skipped: string[]
    dismissedAt?: string
    completedAt?: string
    /** Set by `create-owner --org-name`: the saved workspace name was chosen, not PocketBase's default. */
    orgNameSeeded?: boolean
}
