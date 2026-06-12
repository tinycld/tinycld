import { ScreenHeader } from '@tinycld/core/components/ScreenHeader'
import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'

// Shared content frame for an in-shell Admin section screen. Mirrors the
// centered, max-width column the standalone SetupDashboard uses for its tab
// bodies, so the in-shell screens read identically — only the section nav moved
// from SetupDashboard's internal rail to the workspace PackageSidebar (desktop)
// / MobileDrawer (mobile). On mobile it adds a header with a menu button that
// opens the section drawer, since the rail/sidebar isn't persistently visible
// there — matching how package screens (e.g. calendar) expose their sidebar.
export function AdminScreen({ title, children }: { title: string; children: ReactNode }) {
    const breakpoint = useBreakpoint()
    const setDrawerOpen = useWorkspaceStore(s => s.setDrawerOpen)
    const fgColor = useThemeColor('foreground')

    return (
        <View className="flex-1 bg-background">
            <MobileHeader
                isVisible={breakpoint === 'mobile'}
                title={title}
                fgColor={fgColor}
                onMenuPress={() => setDrawerOpen(true)}
            />
            <ScrollView className="flex-1">
                <View className="w-full self-center p-8 gap-6" style={{ maxWidth: 1040 }}>
                    {children}
                </View>
            </ScrollView>
        </View>
    )
}

function MobileHeader({
    isVisible,
    title,
    fgColor,
    onMenuPress,
}: {
    isVisible: boolean
    title: string
    fgColor: string
    onMenuPress: () => void
}) {
    if (!isVisible) return null

    return (
        <ScreenHeader>
            <View className="flex-row items-center gap-3 px-4 py-3">
                <Pressable testID="drawer-toggle" onPress={onMenuPress} hitSlop={8}>
                    <Menu size={22} color={fgColor} />
                </Pressable>
                <Text className="text-lg font-semibold text-foreground">{title}</Text>
            </View>
        </ScreenHeader>
    )
}
