import { OrgLogo } from '@tinycld/core/components/OrgLogo'
import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Text, View } from 'react-native'
import type { PreviewModel } from './use-workspace-preview'

const RAIL_APP_CLASS = {
    new: 'size-6 items-center justify-center rounded-md border border-primary bg-primary/20',
    settled: 'size-6 items-center justify-center rounded-md',
} as const

/**
 * `isNew` tints the icon so the person sees the apps they are choosing appear;
 * a settled icon looks like an idle icon on the real rail.
 */
export function RailApp({ icon, isNew }: { icon: string; isNew: boolean }) {
    const primary = useThemeColor('primary')
    const railText = useThemeColor('rail-text')
    const Icon = getIcon(icon)
    return (
        <View className={isNew ? RAIL_APP_CLASS.new : RAIL_APP_CLASS.settled}>
            <Icon size={13} color={isNew ? primary : railText} />
        </View>
    )
}

export function railAppsOf(model: PreviewModel, isNew: boolean) {
    return model.apps.map(a => <RailApp key={a.slug} icon={a.icon} isNew={isNew} />)
}

export function OrgMark({ model }: { model: PreviewModel }) {
    if (model.isGhost) {
        return (
            <View className="size-7 items-center justify-center rounded-lg border border-dashed border-rail-text">
                <Text className="text-xs font-extrabold text-rail-text">?</Text>
            </View>
        )
    }
    if (model.logoUrl) {
        const org = {
            id: 'org',
            name: model.name,
            logoUrl: model.logoUrl,
            logoCrop: model.logoCrop,
        }
        return <OrgLogo org={org} size={28} />
    }
    return (
        <View className="size-7 items-center justify-center rounded-lg bg-primary">
            <Text className="text-xs font-extrabold text-primary-foreground">
                {model.initial || '?'}
            </Text>
        </View>
    )
}

const AVATAR_CLASS = {
    owner: {
        circle: '-ml-1.5 size-5 items-center justify-center rounded-full border-2 border-background bg-primary',
        text: 'text-[8px] font-bold text-primary-foreground',
    },
    member: {
        circle: '-ml-1.5 size-5 items-center justify-center rounded-full border-2 border-background bg-muted/30',
        text: 'text-[8px] font-bold text-foreground',
    },
} as const

function Avatar({ initials, isOwner }: { initials: string; isOwner: boolean }) {
    const style = isOwner ? AVATAR_CLASS.owner : AVATAR_CLASS.member
    return (
        <View className={style.circle}>
            <Text className={style.text}>{initials}</Text>
        </View>
    )
}

/** A miniature of the workspace that fills in as each step is completed. */
export function WorkspacePreview({
    model,
    isNewApps,
}: {
    model: PreviewModel
    isNewApps: boolean
}) {
    const rail = railAppsOf(model, isNewApps)
    // The only person on a ghost workspace is the owner being created.
    const avatars = model.memberInitials.map((i, n) => (
        <Avatar key={`${i}-${n}`} initials={i} isOwner={model.isGhost} />
    ))
    return (
        <View
            className="w-full max-w-[340px] h-[230px] flex-row overflow-hidden rounded-xl bg-background shadow-lg"
            style={{ opacity: model.isGhost ? 0.75 : 1 }}
            accessibilityLabel="Preview of your workspace"
        >
            <View className="w-12 items-center gap-2 bg-rail-background py-2">
                <OrgMark model={model} />
                {rail}
            </View>
            <View className="w-24 border-r border-border bg-surface-secondary p-2.5">
                <Text className="mb-2 text-[11px] font-bold text-foreground" numberOfLines={1}>
                    {model.name}
                </Text>
                <View className="mb-2 h-1.5 rounded bg-border" />
                <View className="h-1.5 w-2/3 rounded bg-border" />
            </View>
            <View className="flex-1 gap-2 p-3">
                <View className="flex-row justify-end pl-1.5">{avatars}</View>
                <View className="h-1.5 w-3/5 rounded bg-border" />
                <View className="h-1.5 rounded bg-border" />
            </View>
        </View>
    )
}
