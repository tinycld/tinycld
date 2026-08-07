import { tinycldConfig } from '@tinycld/app-generated/tinycld-config'
import type { SearchAdapterModule, SearchPackage } from './types'

type SearchEntryLike = {
    manifest: { slug: string; nav?: { label?: string; icon?: string; order?: number } }
    search?: { endpoint: string; label?: string; load: () => Promise<unknown> }
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
            endpoint: e.search.endpoint,
        })
    }
    return out.sort((a, b) => a.order - b.order)
}

export const searchPackages = deriveSearchPackages(tinycldConfig as readonly SearchEntryLike[])

const loaders = new Map<string, () => Promise<unknown>>()
for (const e of tinycldConfig as readonly SearchEntryLike[]) {
    if (e.search) loaders.set(e.manifest.slug, e.search.load)
}

// Adapter modules are cached after first load so opening the palette in an
// eight-package workspace does not re-import on every keystroke.
const cache = new Map<string, SearchAdapterModule>()

export async function loadSearchAdapter(slug: string): Promise<SearchAdapterModule | null> {
    const cached = cache.get(slug)
    if (cached) return cached
    const load = loaders.get(slug)
    if (!load) return null
    const mod = (await load()) as SearchAdapterModule
    cache.set(slug, mod)
    return mod
}
