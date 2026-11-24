import { DemoBanner } from '@tinycld/core/components/DemoBanner'
import { NotificationDrawer } from '@tinycld/core/components/NotificationDrawer'
import { useActivePkgTermsBlocked } from '@tinycld/core/components/package-terms/PackageTermsGate'
import { FilePickerSheetHost } from '@tinycld/core/file-viewer/FilePickerSheetHost'
import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDeviceInsets } from '@tinycld/core/lib/use-safe-area'
import { memo } from 'react'
import { Platform, View } from 'react-native'
import { MobileDrawerSlot, MobileOverlays, MobileTabBarSlot } from './MobileLayout'
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
    const activePkgTermsBlocked = useActivePkgTermsBlocked()
    const insets = useDeviceInsets()
    const isMobile = breakpoint === 'mobile'

    // Tablet + desktop both dock the package sidebar as a persistent panel next
    // to the rail. (Mobile uses MobileDrawer instead.) An earlier tablet-only
    // variant rendered the sidebar as a full-screen modal overlay, but it
    // defaulted open with no UI to toggle it — so it just covered the workspace
    // with a touch-capturing dim layer and blocked all interaction. The docked
    // panel matches the intended "contextual sidebar to the right of the rail".
    // A denied package's sidebar sits OUTSIDE the PackageAccessDenied overlay,
    // so hide it here for the same state. The same holds for a package whose
    // terms the owner has not accepted (PackageTermsGate).
    const hasSidebar =
        !isMobile && activePkg?.sidebar != null && !activePkgDenied && !activePkgTermsBlocked
    const sidebarWidth = breakpoint === 'tablet' ? SIDEBAR_WIDTH_TABLET : SIDEBAR_WIDTH_DESKTOP

    // One tree shape for every breakpoint: only the chrome around
    // PackageProviderWrapper/PackageTabs varies, via sibling slots that keep
    // their position. A different parent or position would make React remount
    // the whole navigator and every screen when the window crosses a breakpoint.
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
            <WorkspaceBody
                isReady={isReady}
                isMobile={isMobile}
                hasSidebar={hasSidebar}
                sidebarWidth={sidebarWidth}
                bgColor={bgColor}
                insetLeft={insets.left}
                insetRight={insets.right}
                insetBottom={insets.bottom}
            />
        </View>
    )
}

interface WorkspaceBodyProps {
    isReady: boolean
    isMobile: boolean
    hasSidebar: boolean
    sidebarWidth: number
    bgColor: string
    insetLeft: number
    insetRight: number
    insetBottom: number
}

// Memoized on primitives: WorkspaceLayout re-renders on every activePkgSlug
// change (for the testID above), and without this each of those would rebuild
// the provider chain's children and re-render PackageTabs and every frozen
// screen a second time per navigation.
const WorkspaceBody = memo(function WorkspaceBody({
    isReady,
    isMobile,
    hasSidebar,
    sidebarWidth,
    bgColor,
    insetLeft,
    insetRight,
    insetBottom,
}: WorkspaceBodyProps) {
    // Horizontal insets: see the comment on the content box below.
    const contentStyle = isMobile
        ? { paddingLeft: insetLeft, paddingRight: insetRight }
        : { backgroundColor: bgColor, paddingRight: insetRight, paddingBottom: insetBottom }

    return (
        <PackageProviderWrapper>
            <DemoBanner />
            <View className={isMobile ? 'flex-1' : 'flex-1 flex-row'} style={{ minHeight: 0 }}>
                {/* Horizontal insets are consumed by the edge panels themselves,
                    NOT by this container: padding them here painted the gutters
                    in the app background, leaving a wide light band down the
                    side of the screen. The iOS convention is the opposite:
                    backgrounds run edge to edge and only CONTENT is inset.
                    useDeviceInsets has already side-corrected the pair — only
                    the sensor-housing side is nonzero in landscape — so exactly
                    one of these is ever set: the rail absorbs insetLeft when
                    the housing is on the left (bleeding its dark colour into the
                    gutter), or the content pane pads insetRight when it is on
                    the right. The opposite side stays full-bleed. */}
                {isReady && !isMobile && (
                    <PackageRail insetLeft={insetLeft} insetBottom={insetBottom} />
                )}

                {isReady && !isMobile && <NotificationDrawer />}

                {/* PackageSidebar takes no bottom inset — its SidebarNav
                    ScrollView reserves the home-indicator gutter on its own
                    content so the list can scroll the full height. */}
                {isReady && hasSidebar && <PackageSidebar width={sidebarWidth} />}

                {/* On mobile the horizontal insets go HERE, not on the root: the
                    tab bar and the slide-in drawer are edge-anchored chrome
                    that must keep bleeding to the screen edge (they inset
                    their own contents), and without it every package screen
                    runs under the notch. The mobile sheets stay INSIDE this
                    box: a Sheet rests at the bottom edge of its PARENT,
                    which is what puts it exactly on the tab bar without a
                    manual offset. */}
                <View
                    className={isMobile ? 'flex-1 overflow-hidden' : 'flex-1'}
                    style={[{ minHeight: 0 }, contentStyle]}
                >
                    <PackageTabs />
                    <MobileOverlays isVisible={isMobile} isReady={isReady} />
                    {/* Native tablets take this path too, so the
                        source-chooser sheet needs a host here. Null on web. */}
                    {isReady && <FilePickerSheetHost />}
                </View>
                <MobileTabBarSlot isVisible={isMobile} isReady={isReady} />
            </View>
            <MobileDrawerSlot isVisible={isMobile} isReady={isReady} />
        </PackageProviderWrapper>
    )
})
