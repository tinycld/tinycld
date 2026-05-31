import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import Constants from 'expo-constants'
import { Bug, ChevronRight } from 'lucide-react-native'
import { Platform, Pressable, Text, View } from 'react-native'
import { getCoreConfigOptional } from '../../lib/core-config'
import { openPackageIssue } from '../../lib/help/report-issue'
import { usePackage } from '../../lib/packages/use-packages'

interface Props {
    pkgSlug: string
    isLast?: boolean
}

export function ReportIssueRow({ pkgSlug, isLast = true }: Props) {
    const pkg = usePackage(pkgSlug)
    const mutedColor = useThemeColor('muted-foreground')
    const repoUrl = pkg?.repository?.url
    if (!pkg || !repoUrl) return null

    const borderClass = isLast ? '' : 'border-b border-border'

    return (
        <Pressable
            accessibilityRole="button"
            accessibilityLabel={`Report an issue with ${pkg.name}`}
            onPress={() =>
                openPackageIssue({
                    repoUrl,
                    issueTemplate: pkg.repository?.issueTemplate,
                    pkgName: pkg.name,
                    pkgSlug: pkg.slug,
                    pkgVersion: pkg.version,
                    appVersion: Constants.expoConfig?.version ?? 'unknown',
                    commit: (getCoreConfigOptional()?.release ?? 'dev').slice(0, 7),
                    platform: Platform.OS,
                })
            }
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
