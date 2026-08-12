import { DemoBanner } from '@tinycld/core/components/DemoBanner'
import { NotificationDrawer } from '@tinycld/core/components/NotificationDrawer'
import { FilePickerSheetHost } from '@tinycld/core/file-viewer/FilePickerSheetHost'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDeviceInsets } from '@tinycld/core/lib/use-safe-area'
import { memo } from 'react'
import { Platform, View } from 'react-native'
import { MobileDrawer } from './MobileDrawer'
import { MobileTabBar } from './MobileTabBar'
import { MoreDrawer } from './MoreDrawer'
import { PackageProviderWrapper } from './PackageProviderWrapper'
import { PackageTabs } from './PackageTabs'

// Memoized: the only prop is the stable `isReady` bool. Without this, the
// parent WorkspaceLayout re-rendering (it subscribes to activePkgSlug, which
// ActivePkgSync rewrites after every nav) would re-render this whole subtree —
// PackageProviderWrapper, PackageTabs, and every frozen screen — a second time
// per tab switch. memo() lets that parent re-render stop here.
export const MobileLayout = memo(function MobileLayout({ isReady = true }: { isReady?: boolean }) {
    const isDrawerOpen = useWorkspaceStore(s => s.isDrawerOpen)
    const insets = useDeviceInsets()
    const bgColor = useThemeColor('background')
    // Subscribed rather than read once: the testID must re-render when the
    // active package changes, or it would keep naming whichever package mounted
    // first. This does re-render the subtree per tab switch — the cost the
    // memo() above avoids for its other props — but activePkgSlug changes only
    // on an actual package switch, which is already a full screen transition.
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)

    return (
        <PackageProviderWrapper>
            <View
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
                {/* Horizontal insets go HERE, not on the root: the tab bar and
                    the slide-in drawer below are edge-anchored chrome that must
                    keep bleeding to the screen edge (they inset their own
                    contents). Padding the root would pull them inward and leave
                    a band of app background beside them. useDeviceInsets zeroes
                    the housing-free side, so in landscape exactly one of these
                    is nonzero — the notch side — and without it every package
                    screen inside PackageTabs runs under the notch.

                    The sheets stay INSIDE this box: BottomDrawer rests at the
                    bottom edge of its PARENT, which is what puts it exactly on
                    the tab bar without a manual offset — moving it out would
                    slide it under the bar, the bug that component was written to
                    avoid. So the padding insets the sheet itself by a few points
                    rather than only its content; that is the accepted trade for
                    keeping the vertical contract. */}
                <View
                    className="flex-1 overflow-hidden"
                    style={{ paddingLeft: insets.left, paddingRight: insets.right }}
                >
                    <PackageTabs />
                    {isReady && <MoreDrawer />}
                    {isReady && <NotificationDrawer mobile />}
                    {isReady && <FilePickerSheetHost />}
                </View>
                {isReady && <MobileTabBar />}
                {isReady && <MobileDrawer isVisible={isDrawerOpen} />}
            </View>
        </PackageProviderWrapper>
    )
})
