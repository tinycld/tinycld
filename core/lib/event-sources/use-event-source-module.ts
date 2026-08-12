import { useQuery } from '@tanstack/react-query'
import { loadEventSourceModule } from './registry'
import type { EventSourceModule } from './types'

/**
 * Resolve a registered event source's module for rendering. Null while the
 * dynamic import is pending and forever when the source is unregistered or
 * malformed. Hosts must NOT call `module.useEventSource` conditionally on
 * this settling — mount a child component once the module is non-null, so
 * the hook call is unconditional for that component's entire lifetime
 * (see SearchPalette's PackageActions for the pattern and the crash it avoids).
 */
export function useEventSourceModule(target: string, id: string): EventSourceModule | null {
    const { data } = useQuery({
        queryKey: ['event-source-module', target, id],
        queryFn: () => loadEventSourceModule(target, id),
        staleTime: Number.POSITIVE_INFINITY,
    })
    return data ?? null
}
