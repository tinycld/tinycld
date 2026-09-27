import { useQuery } from '@tanstack/react-query'
import { setupStepEntries } from './registry'
import type { LoadedSetupStep, SetupStepModule } from './types'

const useNoDerivedState = () => null
const useAlwaysVisible = () => true

function normalize(
    entry: (typeof setupStepEntries)[number],
    mod: SetupStepModule
): LoadedSetupStep {
    return {
        id: entry.id,
        label: entry.label,
        Component: mod.default,
        useIsStepDone: mod.useIsStepDone ?? useNoDerivedState,
        useIsStepVisible: mod.useIsStepVisible ?? useAlwaysVisible,
    }
}

/**
 * Loads every step module once. All of them are needed before the shell can
 * render: progress and resume depend on each step's hooks.
 */
export function useSetupSteps() {
    const { data } = useQuery({
        queryKey: ['setup-step-modules'],
        queryFn: () => Promise.all(setupStepEntries.map(async e => normalize(e, await e.load()))),
        staleTime: Number.POSITIVE_INFINITY,
        gcTime: Number.POSITIVE_INFINITY,
    })
    return { steps: data }
}
