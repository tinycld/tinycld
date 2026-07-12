import { DemoBanner } from '@tinycld/core/components/DemoBanner'
import { NotificationDrawer } from '@tinycld/core/components/NotificationDrawer'
import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Platform, View } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { MobileLayout } from './MobileLayout'
import { PackageProviderWrapper } from './PackageProviderWrapper'
import { PackageRail } from './PackageRail'
import { PackageSidebar } from './PackageSidebar'
import { PackageTabs } from './PackageTabs'
import { useBreakpoint } from './useBreakpoint'

// Docked sidebar width. The tablet breakpoint (768–1023dp) is narrow enough that
// the full desktop width would crowd the workspace and push toolbars (e.g. the
// drive List/Grid toggle) off-screen, so it gets a slimmer panel there.
const SIDEBAR_WIDTH_DESKTOP = 260
const SIDEBAR_WIDTH_TABLET = 175

export function WorkspaceLayout({ isReady = true }: { isReady?: boolean }) {
    const bgColor = useThemeColor('background')
    const breakpoint = useBreakpoint()
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const activePkg = usePackage(activePkgSlug ?? '')
    const insets = useSafeAreaInsets()

    if (breakpoint === 'mobile') return <MobileLayout isReady={isReady} />

    // Tablet + desktop both dock the package sidebar as a persistent panel next
    // to the rail. (Mobile uses MobileDrawer instead.) An earlier tablet-only
    // variant rendered the sidebar as a full-screen modal overlay, but it
    // defaulted open with no UI to toggle it — so it just covered the workspace
    // with a touch-capturing dim layer and blocked all interaction. The docked
    // panel matches the intended "contextual sidebar to the right of the rail".
    const hasSidebar = activePkg?.sidebar != null
    const sidebarWidth = breakpoint === 'tablet' ? SIDEBAR_WIDTH_TABLET : SIDEBAR_WIDTH_DESKTOP

    return (
        <View
            className="flex-1"
            style={[
                {
                    backgroundColor: bgColor,
                    paddingTop: insets.top,
                    paddingLeft: insets.left,
                    paddingRight: insets.right,
                },
                Platform.OS === 'web' ? ({ height: '100vh' } as object) : undefined,
            ]}
        >
            <DemoBanner />
            <View className="flex-1 flex-row" style={{ minHeight: 0 }}>
                {isReady && <PackageRail />}

                {isReady && <NotificationDrawer />}

                <PackageProviderWrapper>
                    {isReady && hasSidebar && <PackageSidebar width={sidebarWidth} />}

                    <View
                        className="flex-1"
                        style={{
                            backgroundColor: bgColor,
                            minHeight: 0,
                        }}
                    >
                        <PackageTabs />
                    </View>
                </PackageProviderWrapper>
            </View>
        </View>
    )
}
