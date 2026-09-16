import { useQuery } from '@tanstack/react-query'
import { getResolvedAddress } from './server-address'

// Deployment branding: the server's unauthenticated /api/org-info returns
// Settings().Meta.AppName — the name set in the setup wizard, or one injected
// by an embedding supervisor through the runtime config.
//
// `org.id` is a stable synthetic key, not a server-side id; it is used only as
// an avatar color key.

// Shared with OrgBrandingSection: any mutation that changes the logo or its
// crop must invalidate this key so the rail/sign-in screen (which read
// branding through this query, not through the org_branding collection)
// refetch instead of keeping the session-cached stale value.
export const ORG_INFO_QUERY_KEY = ['org-info'] as const

export interface OrgBranding {
    id: string
    name: string
    logoUrl: string
    logoCrop: string
}

// Exported for unit testing; prefer useOrgInfo in components.
// Reads the address lazily (not PB_SERVER_ADDR, which throws pre-resolution)
// because DocumentTitle mounts this hook on pre-auth screens under
// MinimalProviders, before the server address exists.
export async function fetchOrgInfo(): Promise<{
    name: string
    logoUrl: string
    logoCrop: string
    managedSettings: string[]
}> {
    const addr = getResolvedAddress()
    if (!addr) return { name: '', logoUrl: '', logoCrop: '', managedSettings: [] }
    const res = await fetch(`${addr}/api/org-info`, { cache: 'no-store' })
    if (!res.ok) return { name: '', logoUrl: '', logoCrop: '', managedSettings: [] }
    const body = (await res.json()) as Partial<{
        name: string
        logoUrl: string
        logoCrop: string
        managedSettings: string[]
    }>
    return {
        name: body.name ?? '',
        logoUrl: body.logoUrl ?? '',
        logoCrop: body.logoCrop ?? '',
        // A server that predates this field sends nothing, which reads the same
        // as a standalone deployment: nothing is managed, so nothing is hidden.
        // Failing open is right here — hiding a settings screen because a fetch
        // blipped would be a worse outcome than showing one that saves nothing.
        managedSettings: Array.isArray(body.managedSettings) ? body.managedSettings : [],
    }
}

export function useOrgInfo() {
    // Branding changes only when an operator renames the deployment, so cache
    // it for the session; a transient fetch blip renders the same fallbacks as
    // "no branding".
    const { data } = useQuery({
        queryKey: ORG_INFO_QUERY_KEY,
        queryFn: fetchOrgInfo,
        staleTime: Number.POSITIVE_INFINITY,
        retry: false,
    })

    const name = data?.name?.trim() ?? ''
    // The endpoint returns a server-relative path; resolve it against the
    // same address fetchOrgInfo used, guarding for it being unavailable (this
    // hook runs pre-auth under MinimalProviders, before the address resolves).
    const addr = getResolvedAddress()
    const logoPath = data?.logoUrl ?? ''
    const org: OrgBranding | null = name
        ? {
              id: 'org',
              name,
              logoUrl: logoPath && addr ? `${addr}${logoPath}` : '',
              logoCrop: data?.logoCrop ?? '',
          }
        : null
    // Deliberately outside `org`: that is null until the deployment has a name,
    // and whether a setting is administered here has nothing to do with branding.
    const managedSettings = data?.managedSettings ?? []
    return { org, managedSettings }
}
