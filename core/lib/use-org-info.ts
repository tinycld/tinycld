import { useQuery } from '@tanstack/react-query'
import { getResolvedAddress } from './server-address'

// Deployment branding: the server's unauthenticated /api/org-info returns
// Settings().Meta.AppName — the name set in the setup wizard, or one injected
// by an embedding supervisor through the runtime config.
//
// `org.id` is a stable synthetic key, not a server-side id; it is used only as
// an avatar color key.

export interface OrgBranding {
    id: string
    name: string
}

// Exported for unit testing; prefer useOrgInfo in components.
// Reads the address lazily (not PB_SERVER_ADDR, which throws pre-resolution)
// because DocumentTitle mounts this hook on pre-auth screens under
// MinimalProviders, before the server address exists.
export async function fetchOrgInfo(): Promise<{ name: string }> {
    const addr = getResolvedAddress()
    if (!addr) return { name: '' }
    const res = await fetch(`${addr}/api/org-info`, { cache: 'no-store' })
    if (!res.ok) return { name: '' }
    const body = (await res.json()) as Partial<{ name: string }>
    return { name: body.name ?? '' }
}

export function useOrgInfo() {
    // Branding changes only when an operator renames the deployment, so cache
    // it for the session; a transient fetch blip renders the same fallbacks as
    // "no branding".
    const { data } = useQuery({
        queryKey: ['org-info'],
        queryFn: fetchOrgInfo,
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })

    const name = data?.name?.trim() ?? ''
    const org: OrgBranding | null = name ? { id: 'org', name } : null
    return { org }
}
