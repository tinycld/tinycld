import { inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useSetupPreviewStore } from '@tinycld/core/lib/setup/setup-preview-store'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { isDeliverySwitchedOn } from '../../setup/system-settings-logic'
import { useSystemSettings } from '../../setup/system-settings-store'

export interface PreviewModel {
    name: string
    initial: string
    logoUrl: string
    logoCrop: string
    apps: { slug: string; icon: string }[]
    memberInitials: string[]
    /** Everyone in the workspace; `memberInitials` is capped for the avatars. */
    memberCount: number
    isMailOn: boolean
    isEmpty: boolean
    /** Before the server is claimed: nothing about the org is shown yet. */
    isGhost: boolean
}

export function initialsOf(name: string): string {
    return name
        .split(/\s+/)
        .filter(Boolean)
        .slice(0, 2)
        .map(part => part[0]?.toUpperCase() ?? '')
        .join('')
}

export function buildPreviewModel(input: {
    orgName: string
    draftName: string | null
    logoUrl: string
    logoCrop: string
    apps: { slug: string; icon: string }[]
    memberInitials: string[]
    memberCount: number
    isMailOn: boolean
}): PreviewModel {
    const name = (input.draftName ?? input.orgName).trim()
    return {
        name,
        initial: name.charAt(0).toUpperCase(),
        logoUrl: input.logoUrl,
        logoCrop: input.logoCrop,
        apps: input.apps,
        memberInitials: input.memberInitials,
        memberCount: input.memberCount,
        isMailOn: input.isMailOn,
        isEmpty: !name && input.apps.length === 0 && input.memberInitials.length === 0,
        isGhost: false,
    }
}

/**
 * The preview on pre-auth screens. It must not read live org data: before the
 * server is claimed the name is PocketBase's default, not the person's.
 */
export function ghostPreviewModel(initials: string | undefined): PreviewModel {
    const memberInitials = initials ? [initials] : []
    return {
        name: '',
        initial: '',
        logoUrl: '',
        logoCrop: '',
        apps: [],
        memberInitials,
        memberCount: memberInitials.length,
        isMailOn: false,
        isEmpty: memberInitials.length === 0,
        isGhost: true,
    }
}

const MAX_AVATARS = 4

export function useWorkspacePreview(): PreviewModel {
    const { org } = useOrgInfo()
    const draftName = useSetupPreviewStore(s => s.draftName)
    const [pkgRegistry, users] = useStore('pkg_registry', 'users')
    const packages = usePackages()
    const { byKey } = useSystemSettings()

    const { data: enabled = [] } = useLiveQuery(query =>
        query
            .from({ p: pkgRegistry })
            .where(({ p }) => inArray(p.status, ['bundled', 'installed']))
            .select(({ p }) => ({ slug: p.slug }))
    )
    const { data: people = [] } = useLiveQuery(query =>
        query.from({ u: users }).select(({ u }) => ({ name: u.name }))
    )

    const enabledSlugs = new Set(enabled.map(e => e.slug))
    const apps = packages
        .filter(p => p.nav && enabledSlugs.has(p.slug))
        .map(p => ({ slug: p.slug, icon: p.nav?.icon ?? '' }))

    return buildPreviewModel({
        orgName: org?.name ?? '',
        draftName,
        logoUrl: org?.logoUrl ?? '',
        logoCrop: org?.logoCrop ?? '',
        apps,
        memberInitials: people.slice(0, MAX_AVATARS).map(p => initialsOf(p.name)),
        memberCount: people.length,
        isMailOn: isDeliverySwitchedOn(byKey.get('mail.delivery_enabled')?.value),
    })
}
