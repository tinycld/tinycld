import { useQuery } from '@tanstack/react-query'
import { log } from '@tinycld/core/lib/logger'
import { setupStepEntries } from './registry'
import type { LoadedSetupStep, SetupStepEntry, SetupStepModule } from './types'

const useNoDerivedState = () => null
const useAlwaysVisible = () => true

// memo() and forwardRef() components are objects, not functions.
function isComponent(value: unknown): boolean {
    if (typeof value === 'function') return true
    return typeof value === 'object' && value !== null && '$$typeof' in value
}

function isOptionalHook(value: unknown): boolean {
    return value === undefined || typeof value === 'function'
}

export function isSetupStepModule(mod: unknown): mod is SetupStepModule {
    if (typeof mod !== 'object' || mod === null) return false
    const candidate = mod as Record<string, unknown>
    return (
        isComponent(candidate.default) &&
        isOptionalHook(candidate.useIsStepDone) &&
        isOptionalHook(candidate.useIsStepVisible)
    )
}

function normalize(entry: SetupStepEntry, mod: SetupStepModule): LoadedSetupStep {
    return {
        id: entry.id,
        label: entry.label,
        Component: mod.default,
        useIsStepDone: mod.useIsStepDone ?? useNoDerivedState,
        useIsStepVisible: mod.useIsStepVisible ?? useAlwaysVisible,
    }
}

/**
 * A package's step module is checked when it loads. A module that does not
 * follow the step contract is left out of the wizard, so one broken package
 * cannot stop the owner from finishing setup.
 */
export async function loadSetupSteps(
    entries: readonly SetupStepEntry[]
): Promise<LoadedSetupStep[]> {
    const loaded = await Promise.all(
        entries.map(async entry => {
            const mod = await entry.load()
            if (isSetupStepModule(mod)) return normalize(entry, mod)
            log.warn('setup.steps', 'skipping a step module without a default component', {
                stepId: entry.id,
            })
            return null
        })
    )
    return loaded.filter(step => step !== null)
}

/**
 * Loads every step module once. All of them are needed before the shell can
 * render: progress and resume depend on each step's hooks.
 */
export function useSetupSteps() {
    const { data } = useQuery({
        queryKey: ['setup-step-modules'],
        queryFn: () => loadSetupSteps(setupStepEntries),
        staleTime: Number.POSITIVE_INFINITY,
        gcTime: Number.POSITIVE_INFINITY,
    })
    return { steps: data }
}
