import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { fetchCatalog, isTransientRunError } from '../lib/run-client'
import type { CatalogSource } from '../lib/types'

export const CATALOG_QUERY_KEY = ['reports', 'sources'] as const
const NO_SOURCES: CatalogSource[] = []

// The registry changes only when the server restarts with other packages, so the
// catalog is kept for minutes, not fetched per screen.
export function useReportCatalog() {
    const query = useQuery({
        queryKey: CATALOG_QUERY_KEY,
        queryFn: ({ signal }) => fetchCatalog(signal),
        staleTime: 5 * 60_000,
        retry: (count, error) => count < 2 && isTransientRunError(error),
    })
    const sources = query.data ?? NO_SOURCES
    // One map per catalog answer, so a memo keyed on it (the builder's schema) holds.
    const byKey: ReadonlyMap<string, CatalogSource> = useMemo(
        () => new Map(sources.map(source => [source.key, source])),
        [sources]
    )
    return {
        sources,
        byKey,
        isReady: query.isSuccess,
        isError: query.isError,
        retry: () => void query.refetch(),
    }
}
