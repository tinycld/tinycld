import { DemoBanner } from '@tinycld/core/components/DemoBanner'
import { NotificationDrawer } from '@tinycld/core/components/NotificationDrawer'
import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDeviceInsets } from '@tinycld/core/lib/use-safe-area'
import { Platform, View } from 'react-native'
import { MobileLayout } from './MobileLayout'
import { useActivePkgDenied } from './PackageAccessDenied'
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
    const activePkgDenied = useActivePkgDenied()
    const insets = useDeviceInsets()

    if (breakpoint === 'mobile') return <MobileLayout isReady={isReady} />

    // Tablet + desktop both dock the package sidebar as a persistent panel next
    // to the rail. (Mobile uses MobileDrawer instead.) An earlier tablet-only
    // variant rendered the sidebar as a full-screen modal overlay, but it
    // defaulted open with no UI to toggle it — so it just covered the workspace
    // with a touch-capturing dim layer and blocked all interaction. The docked
    // panel matches the intended "contextual sidebar to the right of the rail".
    // A denied package's sidebar sits OUTSIDE the PackageAccessDenied overlay,
    // so hide it here for the same state.
    const hasSidebar = activePkg?.sidebar != null && !activePkgDenied
    const sidebarWidth = breakpoint === 'tablet' ? SIDEBAR_WIDTH_TABLET : SIDEBAR_WIDTH_DESKTOP

    return (
        <View
            // Names the package currently mounted in the shell. Set from the
            // route only after `app/(app)/_layout` has committed it, so it is a
            // "this screen is showing" signal — unlike the URL, which changes at
            // the start of a SPA transition while the target chunk is still
            // loading. E2E gates on this instead of waitForURL; see
            // navigateToPackage in tests/e2e/helpers.ts.
            testID={activePkgSlug ? `pkg-active-${activePkgSlug}` : undefined}
            className="flex-1"
            style={[
                {
                    backgroundColor: bgColor,
                    paddingTop: insets.top,
                },
                Platform.OS === 'web' ? ({ height: '100vh' } as object) : undefined,
            ]}
        >
            <DemoBanner />
            <View className="flex-1 flex-row" style={{ minHeight: 0 }}>
                {/* Horizontal insets are consumed by the edge panels themselves,
                    NOT by this container: padding them here painted the gutters
                    in the app background, leaving a wide light band down the
                    side of the screen. The iOS convention is the opposite:
                    backgrounds run edge to edge and only CONTENT is inset.
                    useDeviceInsets has already side-corrected the pair — only
                    the sensor-housing side is nonzero in landscape — so exactly
                    one of these is ever set: the rail absorbs insets.left when
                    the housing is on the left (bleeding its dark colour into the
                    gutter), or the content pane pads insets.right when it is on
                    the right. The opposite side stays full-bleed. */}
                {isReady && <PackageRail insetLeft={insets.left} insetBottom={insets.bottom} />}

                {isReady && <NotificationDrawer />}

                <PackageProviderWrapper>
                    {/* PackageSidebar takes no bottom inset — its SidebarNav
                        ScrollView reserves the home-indicator gutter on its own
                        content so the list can scroll the full height. */}
                    {isReady && hasSidebar && <PackageSidebar width={sidebarWidth} />}

                    <View
                        className="flex-1"
                        style={{
                            backgroundColor: bgColor,
                            minHeight: 0,
                            paddingRight: insets.right,
                            paddingBottom: insets.bottom,
                        }}
                    >
                        <PackageTabs />
                    </View>
                </PackageProviderWrapper>
            </View>
        </View>
    )
}
