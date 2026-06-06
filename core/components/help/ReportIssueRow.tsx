import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Bug, ChevronRight } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { useReportIssue } from '../../lib/help/use-report-issue'
import { usePackage } from '../../lib/packages/use-packages'

interface Props {
    pkgSlug: string
    isLast?: boolean
}

export function ReportIssueRow({ pkgSlug, isLast = true }: Props) {
    const pkg = usePackage(pkgSlug)
    const reportIssue = useReportIssue(pkgSlug)
    const mutedColor = useThemeColor('muted-foreground')
    if (!pkg || !reportIssue) return null

    const borderClass = isLast ? '' : 'border-b border-border'

    return (
        <Pressable
            accessibilityRole="button"
            accessibilityLabel={`Report an issue with ${pkg.name}`}
            onPress={reportIssue}
            className={`flex-row items-center px-4 py-3.5 ${borderClass} hover:bg-surface-secondary`}
            style={({ pressed }) => [pressed && { opacity: 0.7 }]}
        >
            <Bug size={18} color={mutedColor} />
            <View className="flex-1 pl-3 pr-2">
                <Text className="text-base font-medium text-foreground">Report an issue</Text>
                <Text className="text-sm text-muted-foreground mt-0.5">
                    Opens GitHub in your browser
                </Text>
            </View>
            <ChevronRight size={18} color={mutedColor} />
        </Pressable>
    )
}
