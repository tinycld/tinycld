import { useQuery } from '@tanstack/react-query'
import { getResolvedAddress } from './server-address'

// Single-org deployment: the process IS one org, identified by the deployment
// (subdomain), not by a row in an `orgs` collection. Branding comes from the
// server's unauthenticated /api/org-info, which reads Settings().Meta.AppName —
// the setup wizard's app name in a standalone deployment, or the org's
// display_name in a router-managed tenant (materialized into .runtime/app.json
// and adopted at tenant boot).
//
// The `{ orgSlug, orgId, org }` shape is preserved for the call sites that
// still destructure it. There is no server-side org id or slug in single-org,
// so both stay '' and `org.id` is a stable synthetic key (used only as an
// avatar color key).

export interface OrgBranding {
    id: string
    name: string
    slug: string
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
    // Branding changes only when an operator renames the deployment (or the
    // router re-materializes an org), so cache it for the session; a transient
    // fetch blip renders the same fallbacks as "no branding".
    const { data } = useQuery({
        queryKey: ['org-info'],
        queryFn: fetchOrgInfo,
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })

    const name = data?.name?.trim() ?? ''
    const org: OrgBranding | null = name ? { id: 'org', name, slug: '' } : null
    return { orgSlug: '', orgId: '', org }
}
