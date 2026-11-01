import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ArrowLeft } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Pressable, Text, View } from 'react-native'

export const REPORT_TAP_TARGET = 44

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

export function ReportHeader({
    title,
    onBack,
    toolbar,
}: {
    title: string
    onBack: () => void
    toolbar: ReactNode
}) {
    return (
        <View className="gap-1 px-4 pt-2">
            <ReportBackLink onBack={onBack} />
            <View className="flex-row items-center justify-between gap-2">
                <Text
                    testID="report-title"
                    numberOfLines={1}
                    className="min-w-0 shrink text-2xl font-semibold text-foreground"
                >
                    {title}
                </Text>
                <View className="min-w-0 flex-1 items-end">{toolbar}</View>
            </View>
        </View>
    )
}
