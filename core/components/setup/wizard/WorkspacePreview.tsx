import { getIcon } from '@tinycld/core/components/workspace/package-icon-map'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Image, Text, View } from 'react-native'
import type { PreviewModel } from './use-workspace-preview'

export function RailApp({ icon }: { icon: string }) {
    const color = useThemeColor('primary')
    const Icon = getIcon(icon)
    return (
        <View className="size-6 items-center justify-center rounded-md bg-primary/20">
            <Icon size={13} color={color} />
        </View>
    )
}

export function OrgMark({ model }: { model: PreviewModel }) {
    if (model.logoUrl) {
        return <Image source={{ uri: model.logoUrl }} className="size-7 rounded-lg" />
    }
    return (
        <View className="size-7 items-center justify-center rounded-lg bg-primary">
            <Text className="text-xs font-extrabold text-primary-foreground">
                {model.initial || '?'}
            </Text>
        </View>
    )
}

function Avatar({ initials }: { initials: string }) {
    return (
        <View className="-ml-1.5 size-5 items-center justify-center rounded-full border-2 border-background bg-muted/30">
            <Text className="text-[8px] font-bold text-foreground">{initials}</Text>
        </View>
    )
}

/** A miniature of the workspace that fills in as each step is completed. */
export function WorkspacePreview({ model, isGhost }: { model: PreviewModel; isGhost: boolean }) {
    const rail = model.apps.map(a => <RailApp key={a.slug} icon={a.icon} />)
    const avatars = model.memberInitials.map((i, n) => <Avatar key={`${i}-${n}`} initials={i} />)
    return (
        <View
            className="w-full max-w-[340px] h-[230px] flex-row overflow-hidden rounded-xl bg-background shadow-lg"
            style={{ opacity: isGhost ? 0.7 : 1 }}
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
            <View className="flex-1 p-3">
                <View className="flex-row justify-end pl-1.5">{avatars}</View>
            </View>
        </View>
    )
}
