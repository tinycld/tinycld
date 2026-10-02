import { useOrgHref } from '@tinycld/core/lib/org-routes'
import type { PackageSettingsGroup } from '@tinycld/core/lib/packages/derive-components'
import { resolvePanel } from '@tinycld/core/lib/packages/resolve-panel'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useNavigateBack } from '@tinycld/core/lib/use-navigate-back'
import { useLocalSearchParams, useRouter } from 'expo-router'
import { ArrowLeft } from 'lucide-react-native'
import { Suspense, useMemo } from 'react'
import { Pressable, Text, View } from 'react-native'

// Renders the package panel a /settings/…/<pkgSlug>/<panelSlug> URL names,
// resolved against the registry the calling route owns. Access control stays
// with the route: the org tree gates on role, the account tree does not.
export function PackagePanelScreen({ groups }: { groups: readonly PackageSettingsGroup[] }) {
    const router = useRouter()
    const orgHref = useOrgHref()
    const navigateBack = useNavigateBack(() => orgHref('settings'))
    const params = useLocalSearchParams<{ section: string[] }>()
    const [pkgSlug, panelSlug] = params.section ?? []

    const fgColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const bgColor = useThemeColor('background')
    const primaryColor = useThemeColor('primary')

    const match = useMemo(
        () => resolvePanel(groups, pkgSlug, panelSlug),
        [groups, pkgSlug, panelSlug]
    )

    if (!match) {
        return (
            <View
                className="flex-1 p-5 items-center justify-center"
                style={{ backgroundColor: bgColor }}
            >
                <Text className="mb-3" style={{ fontSize: 18, fontWeight: 'bold', color: fgColor }}>
                    Settings not found
                </Text>
                <Pressable onPress={() => router.push(orgHref('settings'))}>
                    <Text style={{ fontSize: 15, color: primaryColor }}>Back to Settings</Text>
                </Pressable>
            </View>
        )
    }

    const { group, panel } = match
    const PanelComponent = panel.Component

    return (
        <View className="flex-1" style={{ backgroundColor: bgColor }}>
            <View className="flex-row gap-3 items-center p-5 pb-0">
                <Pressable onPress={navigateBack}>
                    <ArrowLeft size={24} color={fgColor} />
                </Pressable>
                <Text style={{ fontSize: 22, fontWeight: 'bold', color: fgColor }}>
                    {panel.label}
                </Text>
                <Text style={{ fontSize: 15, color: mutedColor }}>{group.packageName}</Text>
            </View>
            <View className="flex-1">
                <Suspense fallback={null}>
                    <PanelComponent />
                </Suspense>
            </View>
        </View>
    )
}
