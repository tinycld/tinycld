import { useQuery } from '@tanstack/react-query'
import { getResolvedAddress } from '../server-address'

export const NEEDS_SETUP_QUERY_KEY = ['setup-check'] as const

/** undefined until the server answers. A failed check reads as "set up". */
export function useNeedsSetup(): boolean | undefined {
    const { data } = useQuery({
        queryKey: NEEDS_SETUP_QUERY_KEY,
        queryFn: async () => {
            const addr = getResolvedAddress()
            if (!addr) return false
            const res = await fetch(`${addr}/api/setup/check`, { cache: 'no-store' })
            if (!res.ok) return false
            const body = (await res.json()) as { needsSetup?: boolean }
            return body.needsSetup === true
        },
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })
    return data
}
