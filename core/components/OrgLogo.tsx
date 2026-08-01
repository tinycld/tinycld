import { NameAvatar } from '@tinycld/core/components/NameAvatar'
import type { ReactNode } from 'react'

interface OrgLogoProps {
    org: { id: string; name: string } | null | undefined
    size?: number
    /** Rendered when org is null/loading. Defaults to nothing. */
    fallback?: ReactNode
}

/**
 * Round avatar for the organization: consistent colored initials keyed off the
 * org name. (Uploaded logo images went away with the `orgs` collection —
 * single-org branding is name-only; see use-org-info.ts.)
 */
export function OrgLogo({ org, size = 36, fallback = null }: OrgLogoProps) {
    if (!org) return <>{fallback}</>
    return <NameAvatar firstName={org.name} colorKey={org.id} size={size} />
}
