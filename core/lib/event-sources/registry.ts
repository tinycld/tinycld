import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import type { EventSourceModule } from './types'

type EventSourceEntryLike = {
    manifest: { slug: string }
    eventSources?: {
        target: string
        id: string
        label: string
        color?: string
        order: number
        load: () => Promise<unknown>
    }[]
}

export interface RegisteredEventSource {
    contributorSlug: string
    id: string
    label: string
    color?: string
    order: number
    load: () => Promise<unknown>
}

/** target slug → sources sorted by order, ties broken by contributor slug. */
export function deriveEventSources(
    entries: readonly EventSourceEntryLike[]
): Record<string, RegisteredEventSource[]> {
    const byTarget: Record<string, RegisteredEventSource[]> = {}
    for (const e of entries) {
        for (const s of e.eventSources ?? []) {
            byTarget[s.target] ??= []
            byTarget[s.target].push({
                contributorSlug: e.manifest.slug,
                id: s.id,
                label: s.label,
                ...(s.color ? { color: s.color } : {}),
                order: s.order,
                load: s.load,
            })
        }
    }
    for (const list of Object.values(byTarget)) {
        list.sort((a, b) => a.order - b.order || a.contributorSlug.localeCompare(b.contributorSlug))
    }
    return byTarget
}

export const packageEventSources = deriveEventSources(
    tinycldConfig as readonly EventSourceEntryLike[]
)

/**
 * Build a module loader over a source table. Split from the module-level
 * binding so tests can exercise caching and the malformed-module path with
 * fake loaders.
 */
export function createEventSourceLoader(sources: Record<string, RegisteredEventSource[]>) {
    // Modules are cached after first load so a host re-mounting its collectors
    // (every screen visit) does not re-import per mount.
    const cache = new Map<string, EventSourceModule>()
    /**
     * Resolve a registered source's module. Returns null when no such source
     * is registered for the target or the module does not export a
     * `useEventSource` function — the host treats null as "source inactive",
     * never an error.
     */
    return async function loadEventSourceModule(
        target: string,
        id: string
    ): Promise<EventSourceModule | null> {
        const key = `${target}:${id}`
        const cached = cache.get(key)
        if (cached) return cached
        const source = sources[target]?.find(s => s.id === id)
        if (!source) return null
        const mod = (await source.load()) as EventSourceModule
        if (typeof mod?.useEventSource !== 'function') return null
        cache.set(key, mod)
        return mod
    }
}

export const loadEventSourceModule = createEventSourceLoader(packageEventSources)
