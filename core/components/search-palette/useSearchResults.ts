import { useQuery } from '@tanstack/react-query'
import { pb } from '@tinycld/core/lib/pocketbase'
import { buildSections, type SearchSection } from '@tinycld/core/lib/search/build-sections'
import { searchPackages } from '@tinycld/core/lib/search/registry'
import type { ParsedQuery, SearchRow } from '@tinycld/core/lib/search/types'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'

const MIN_QUERY_LENGTH = 2
const DEBOUNCE_MS = 300

/** The aggregator's response shape — mirrors core/server/search's Response. */
interface SearchResponse {
    rows: SearchRow[]
    counts: Record<string, number>
    partial?: string[]
    truncated?: string[]
}

/**
 * Search every in-scope package through core's federated endpoint.
 *
 * One request, not one per package. The server fans out to each package's
 * registered source in-process, normalizes the rows, and ranks them — so the
 * mapping from a package's own shape to a display row lives in one place that
 * both this palette and the CLI read, rather than in a per-package TypeScript
 * adapter the CLI cannot import.
 *
 * What we give up by batching: rows no longer arrive per package, so a slow
 * package delays the whole answer rather than just its own section. The server
 * bounds each source with its own timeout and reports the ones it dropped, which
 * is what keeps that from becoming an invisible hang.
 */
export function useSearchResults(parsed: ParsedQuery): {
    sections: SearchSection[]
    isSearching: boolean
    /** Packages that failed or timed out server-side; rows are missing for these. */
    partial: string[]
} {
    const query = useDebouncedValue(parsed.include.join(' '), DEBOUNCE_MS)
    const not = useDebouncedValue(parsed.exclude.join(' '), DEBOUNCE_MS)
    // Chips are already validated against installed slugs by parseQuery, so
    // they can be forwarded as-is; the server drops any it does not know.
    const chips = parsed.chips
    const enabled = query.length >= MIN_QUERY_LENGTH

    const { data, isFetching } = useQuery({
        queryKey: ['federated-search', query, not, chips],
        queryFn: ({ signal }) =>
            pb.send('/api/search', {
                method: 'GET',
                query: {
                    q: query,
                    ...(not ? { not } : {}),
                    // Repeated `pkg` params: the server reads them as a list.
                    ...(chips.length > 0 ? { pkg: chips } : {}),
                },
                signal,
            }) as Promise<SearchResponse>,
        enabled,
        // A search is point-in-time: don't retry a failure (the user is still
        // typing) and don't refetch on window focus.
        retry: false,
        refetchOnWindowFocus: false,
    })

    // Group by package for rendering. The server already ordered the rows, so
    // buildSections only arranges them — it must not re-sort, or it would
    // discard the cross-package ranking it cannot reproduce (the scorer lives
    // server-side now).
    const rowsBySlug: Record<string, SearchRow[]> = {}
    for (const row of data?.rows ?? []) {
        rowsBySlug[row.slug] = rowsBySlug[row.slug] ?? []
        rowsBySlug[row.slug].push(row)
    }

    return {
        sections: buildSections(rowsBySlug, searchPackages, parsed.chips, data?.rows ?? []),
        isSearching: isFetching,
        partial: data?.partial ?? [],
    }
}
