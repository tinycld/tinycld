import type { ReactNode } from 'react'
import { createContext, createElement, useContext } from 'react'

// Single-org deployment: org identity comes from the deployment (subdomain), not
// a URL slug. `useOrgSlug` is retained as a no-op shim (returns '') so the many
// call sites that still reference it keep compiling; the route tree no longer
// carries an [orgSlug] segment.
export const OrgSlugContext = createContext<string>('')

export function OrgSlugProvider({ children }: { slug?: string; children: ReactNode }) {
    return createElement(OrgSlugContext.Provider, { value: '' }, children)
}

export function useOrgSlug(): string {
    return useContext(OrgSlugContext)
}
