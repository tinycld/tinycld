import { useQuery } from '@tanstack/react-query'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useDebouncedValue } from '@tinycld/core/lib/use-debounced-value'

interface UseApiSearchOptions<TResult> {
    endpoint: string
    buildQueryParams?: (query: string) => Record<string, string>
    extractResults: (response: unknown) => TResult[]
    extractTotal?: (response: unknown) => number
    minQueryLength?: number
    debounceMs?: number
}

interface UseApiSearchReturn<TResult> {
    results: TResult[]
    total: number
    isSearching: boolean
    error: string | null
}

function toErrorMessage(err: unknown): string {
    return err instanceof Error ? err.message : 'Search failed'
}

export function useApiSearch<TResult>(
    query: string,
    options: UseApiSearchOptions<TResult>
): UseApiSearchReturn<TResult> {
    const {
        endpoint,
        buildQueryParams,
        extractResults,
        extractTotal,
        minQueryLength = 2,
        debounceMs = 300,
    } = options

    const debouncedQuery = useDebouncedValue(query, debounceMs)
    const enabled = debouncedQuery.length >= minQueryLength

    // React Query owns the fetch lifecycle: it aborts a superseded request via
    // the queryFn `signal`, tracks in-flight/error state, and caches by the
    // debounced query — replacing the hand-rolled AbortController + useState
    // machinery this hook used to carry.
    const search = useQuery({
        queryKey: ['api-search', endpoint, debouncedQuery, buildQueryParams?.(debouncedQuery)],
        queryFn: ({ signal }) =>
            pb.send(endpoint, {
                method: 'GET',
                query: buildQueryParams ? buildQueryParams(debouncedQuery) : { q: debouncedQuery },
                signal,
            }),
        enabled,
        // A search is inherently point-in-time; don't retry a failed query
        // (the user will just keep typing) and don't refetch on focus.
        retry: false,
        refetchOnWindowFocus: false,
    })

    if (!enabled) {
        return { results: [], total: 0, isSearching: false, error: null }
    }

    return {
        results: search.data ? extractResults(search.data) : [],
        total: search.data && extractTotal ? extractTotal(search.data) : 0,
        isSearching: search.isFetching,
        error: search.error ? toErrorMessage(search.error) : null,
    }
}
