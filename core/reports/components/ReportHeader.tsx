import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ArrowLeft } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { ReportUpdating } from './ReportUpdating'

export const REPORT_TAP_TARGET = 44

// The narrowest the title gets before an action folds into "⋯".
const TITLE_MIN_WIDTH = 160

function ReportBackLink({ onBack }: { onBack: () => void }) {
    const muted = useThemeColor('muted-foreground')
    return (
        <Pressable
            testID="report-back"
            accessibilityRole="link"
            onPress={onBack}
            className="flex-row items-center gap-1 self-start"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <ArrowLeft size={16} color={muted} />
            <Text className="text-sm text-muted-foreground">Reports</Text>
        </Pressable>
    )
}

// The title is the toolbar's first item, so the toolbar fits the title and the
// actions together: a long title ends in "…" before the actions, and an action
// folds only when the title is at its narrowest.
function headerItems(title: string, actions: ToolbarItem[]): ToolbarItem[] {
    const titleItem: ToolbarItem = {
        type: 'custom',
        key: 'title',
        minWidth: TITLE_MIN_WIDTH,
        element: (
            <Text
                testID="report-title"
                numberOfLines={1}
                className="min-w-0 shrink text-2xl font-semibold text-foreground"
            >
                {title}
            </Text>
        ),
    }
    return [titleItem, { type: 'spacer' }, ...actions]
}

export function ReportHeader({
    title,
    onBack,
    actions,
    isUpdating = false,
    isUpdatingLive = true,
}: {
    title: string
    onBack: () => void
    actions: ToolbarItem[]
    isUpdating?: boolean
    /** False while another surface (the builder over the view) shows "Updating…". */
    isUpdatingLive?: boolean
}) {
    return (
        <View className="gap-1 px-4 pt-2">
            <View className="flex-row items-center justify-between gap-2">
                <ReportBackLink onBack={onBack} />
                <ReportUpdating isUpdating={isUpdating} isLive={isUpdatingLive} />
            </View>
            <ResponsiveToolbar items={headerItems(title, actions)} className="" />
        </View>
    )
}
