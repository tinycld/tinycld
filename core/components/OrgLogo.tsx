import { Avatar } from '@tinycld/core/components/Avatar'
import type { ReactNode } from 'react'

interface OrgLogoProps {
    org: { id: string; name: string } | null | undefined
    size?: number
    /** Rendered when org is null/loading. Defaults to nothing. */
    fallback?: ReactNode
}

/**
 * Round avatar for the organization: the uploaded logo when one is set,
 * otherwise consistent colored initials keyed off the org name.
 */
export function OrgLogo({ org, size = 36, fallback = null }: OrgLogoProps) {
    if (!org) return <>{fallback}</>
    return <Avatar name={org.name} colorKey={org.id} size={size} />
}
