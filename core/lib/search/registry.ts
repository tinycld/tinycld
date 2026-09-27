import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import { log } from '@tinycld/core/lib/logger'
import type { SearchAdapterModule, SearchPackage } from './types'

type SearchEntryLike = {
    manifest: { slug: string; nav?: { label?: string; icon?: string; order?: number } }
    search?: { label?: string; load: () => Promise<unknown> }
}

/** Packages that declare `search`, ordered by nav.order. */
export function deriveSearchPackages(entries: readonly SearchEntryLike[]): SearchPackage[] {
    const out: SearchPackage[] = []
    for (const e of entries) {
        if (!e.search) continue
        out.push({
            slug: e.manifest.slug,
            label: e.search.label ?? e.manifest.nav?.label ?? e.manifest.slug,
            icon: e.manifest.nav?.icon ?? 'search',
            order: e.manifest.nav?.order ?? 0,
        })
    }
    return out.sort((a, b) => a.order - b.order)
}

export const searchPackages = deriveSearchPackages(tinycldConfig as readonly SearchEntryLike[])

function isSearchAdapterModule(mod: unknown): mod is SearchAdapterModule {
    return (
        typeof mod === 'object' &&
        mod !== null &&
        typeof (mod as Record<string, unknown>).useSearchActions === 'function'
    )
}

/**
 * Build an adapter loader over the declared entries. Split from the
 * module-level binding so tests can use fake loaders.
 */
export function createSearchAdapterLoader(entries: readonly SearchEntryLike[]) {
    const loaders = new Map<string, () => Promise<unknown>>()
    for (const e of entries) {
        if (e.search) loaders.set(e.manifest.slug, e.search.load)
    }

    // Adapter modules are cached after first load so opening the palette in an
    // eight-package workspace does not re-import on every keystroke.
    const cache = new Map<string, SearchAdapterModule>()

    /**
     * Returns null when the package declares no search, or its module does not
     * export a `useSearchActions` function: the palette then has no actions
     * for that package instead of crashing on a bad module.
     */
    return async function loadSearchAdapter(slug: string): Promise<SearchAdapterModule | null> {
        const cached = cache.get(slug)
        if (cached) return cached
        const load = loaders.get(slug)
        if (!load) return null
        const mod = await load()
        if (!isSearchAdapterModule(mod)) {
            log.warn('search.adapter', 'skipping a module without a useSearchActions hook', {
                slug,
            })
            return null
        }
        cache.set(slug, mod)
        return mod
    }
}

export const loadSearchAdapter = createSearchAdapterLoader(
    tinycldConfig as readonly SearchEntryLike[]
)
