import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ArrowLeft } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { HUB_BACK_LABEL } from '../lib/report-back'
import { ReportUpdating } from './ReportUpdating'

export const REPORT_TAP_TARGET = 44

// The narrowest the title gets before an action folds into "⋯".
const TITLE_MIN_WIDTH = 160

/** The link back to where the report was opened from: the hub, or the report it was drilled from. */
export function ReportBackLink({
    label,
    onBack,
    testID = 'report-back',
}: {
    label: string
    onBack: () => void
    testID?: string
}) {
    const muted = useThemeColor('muted-foreground')
    return (
        <Pressable
            testID={testID}
            accessibilityRole="link"
            onPress={onBack}
            className="flex-row items-center gap-1 self-start"
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <ArrowLeft size={16} color={muted} />
            <Text numberOfLines={1} className="text-sm text-muted-foreground">
                {label}
            </Text>
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
    backLabel = HUB_BACK_LABEL,
    actions,
    isUpdating = false,
    isUpdatingLive = true,
    busyLabel,
}: {
    title: string
    onBack: () => void
    /** The back link's text: the source report's title after a drill (D3). */
    backLabel?: string
    actions: ToolbarItem[]
    isUpdating?: boolean
    /** False while another surface (the builder over the view) shows "Updating…". */
    isUpdatingLive?: boolean
    /** Work of the reader's own in progress ("Preparing…"), shown in place of "Updating…". */
    busyLabel?: string
}) {
    return (
        <View className="gap-1 px-4 pt-2">
            <View className="flex-row items-center justify-between gap-2">
                <ReportBackLink label={backLabel} onBack={onBack} />
                <ReportUpdating isUpdating={isUpdating} isLive={isUpdatingLive} label={busyLabel} />
            </View>
            <ResponsiveToolbar items={headerItems(title, actions)} className="" />
        </View>
    )
}
