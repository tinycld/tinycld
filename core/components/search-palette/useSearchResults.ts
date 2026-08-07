import { useQueries } from '@tanstack/react-query'
import { pb } from '@tinycld/core/lib/pocketbase'
import { buildSections, type SearchSection } from '@tinycld/core/lib/search/build-sections'
import { loadSearchAdapter, searchPackages } from '@tinycld/core/lib/search/registry'
import type { ParsedQuery, SearchAdapterModule, SearchRow } from '@tinycld/core/lib/search/types'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'

const MIN_QUERY_LENGTH = 2
const DEBOUNCE_MS = 300

/**
 * Fan out one search per in-scope package and merge the results.
 *
 * useQueries rather than a loop of single-query hooks: the in-scope list
 * changes as the user adds and removes chips, and a hook called per iteration
 * of a `for` loop breaks the Rules of Hooks. React Query also gives each
 * package independent caching and abort-on-supersede for free, so a slow
 * package cannot hold up the rest.
 */
export function useSearchResults(parsed: ParsedQuery): {
    sections: SearchSection[]
    isSearching: boolean
} {
    const scoped =
        parsed.chips.length > 0
            ? searchPackages.filter(p => parsed.chips.includes(p.slug))
            : searchPackages

    const query = useDebouncedValue(parsed.include.join(' '), DEBOUNCE_MS)
    const not = useDebouncedValue(parsed.exclude.join(' '), DEBOUNCE_MS)
    const enabled = query.length >= MIN_QUERY_LENGTH

    // Adapter modules are dynamic imports. Running them through Query rather
    // than an effect keeps the resolution cached and out of component state;
    // loadSearchAdapter already memoizes, so this is belt-and-braces.
    const adapterQueries = useQueries({
        queries: scoped.map(pkg => ({
            queryKey: ['search-adapter', pkg.slug],
            queryFn: () => loadSearchAdapter(pkg.slug),
            staleTime: Number.POSITIVE_INFINITY,
        })),
    })

    const searchQueries = useQueries({
        queries: scoped.map(pkg => ({
            queryKey: ['package-search', pkg.slug, query, not],
            queryFn: ({ signal }: { signal: AbortSignal }) =>
                pb.send(pkg.endpoint, {
                    method: 'GET',
                    query: not ? { q: query, not } : { q: query },
                    signal,
                }),
            enabled,
            // A search is point-in-time: don't retry a failure (the user is
            // still typing) and don't refetch on window focus.
            retry: false,
            refetchOnWindowFocus: false,
        })),
    })

    const rowsBySlug: Record<string, SearchRow[]> = {}
    let isSearching = false

    scoped.forEach((pkg, i) => {
        if (searchQueries[i]?.isFetching) isSearching = true

        const adapter = adapterQueries[i]?.data as SearchAdapterModule | null | undefined
        const data = searchQueries[i]?.data as { items?: unknown[] } | undefined
        if (!adapter || !data?.items) return

        // A package whose request failed simply contributes no rows — one
        // backend erroring must not empty the whole palette.
        rowsBySlug[pkg.slug] = data.items
            .map(hit => adapter.toRow(hit))
            .filter((r): r is Omit<SearchRow, 'slug'> => r !== null)
            .map(r => ({ ...r, slug: pkg.slug }))
    })

    return {
        // Ordering lives entirely here and is unaffected by fetch order:
        // compareRows tie-breaks on nav.order precisely so a late-arriving
        // package cannot change the ranking of what already landed.
        sections: buildSections(rowsBySlug, searchPackages, parsed.chips, parsed.include),
        isSearching,
    }
}
