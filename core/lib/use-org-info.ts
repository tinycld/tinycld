import { pb } from '@tinycld/core/lib/pocketbase'

// Single-org deployment: the process IS one org, identified by the deployment
// (subdomain), not by a row in an `orgs` collection or a URL slug. There is no
// `orgs` collection to query anymore.
//
// This shim preserves the `{ orgSlug, orgId, org }` shape so the many call sites
// that still destructure it keep compiling while queries are de-scoped. Org
// branding (name/logo) now comes from the deployment/cookie — a follow-up wires
// that in; for now `org` is null and callers fall back to their defaults.
export function useOrgInfo() {
    return { orgSlug: '', orgId: '', org: null as OrgBranding | null }
}

interface OrgBranding {
    id: string
    name: string
    slug: string
    logo?: string
}

/** Returns a fully-qualified URL for an org's logo, or null when unset. */
export function getOrgLogoUrl(
    org: { id: string; logo?: string } | null | undefined
): string | null {
    if (!org?.id || !org.logo) return null
    return pb.files.getURL({ collectionId: 'orgs', id: org.id }, org.logo)
}
