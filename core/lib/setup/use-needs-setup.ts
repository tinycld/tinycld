import { useQuery } from '@tanstack/react-query'
import { getResolvedAddress, subscribeResolvedAddress } from '@tinycld/core/lib/server-address'
import { useSyncExternalStore } from 'react'

export const NEEDS_SETUP_QUERY_KEY = ['setup-check'] as const

/**
 * undefined until the server answers. A failed check reads as "set up".
 *
 * The check waits for a server address: on native it is resolved after
 * launch, and an answer cached before then would say "set up" for good.
 */
export function useNeedsSetup(): boolean | undefined {
    const addr = useSyncExternalStore(subscribeResolvedAddress, getResolvedAddress)
    const { data } = useQuery({
        queryKey: [...NEEDS_SETUP_QUERY_KEY, addr],
        queryFn: async () => {
            const res = await fetch(`${addr}/api/setup/check`, { cache: 'no-store' })
            if (!res.ok) return false
            const body = (await res.json()) as { needsSetup?: boolean }
            return body.needsSetup === true
        },
        enabled: Boolean(addr),
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })
    return data
}
