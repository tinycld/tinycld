import { Text, View } from 'react-native'
import type { PreviewModel } from './use-workspace-preview'
import { OrgMark, RailApp } from './WorkspacePreview'

/** The phone form of the workspace preview: the rail laid out as one dark row. */
export function WorkspacePreviewStrip({
    model,
    isVisible,
}: {
    model: PreviewModel
    isVisible: boolean
}) {
    if (!isVisible) return null
    const rail = model.apps.map(a => <RailApp key={a.slug} icon={a.icon} />)
    return (
        <View
            className="flex-row items-center gap-2 bg-rail-background px-3 py-2.5"
            accessibilityLabel="Preview of your workspace"
        >
            <OrgMark model={model} />
            <Text className="text-xs font-bold text-rail-active-text" numberOfLines={1}>
                {model.name}
            </Text>
            <View className="flex-1" />
            {rail}
        </View>
    )
}
