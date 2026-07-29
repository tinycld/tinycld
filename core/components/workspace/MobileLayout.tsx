import { DemoBanner } from '@tinycld/core/components/DemoBanner'
import { NotificationDrawer } from '@tinycld/core/components/NotificationDrawer'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { memo } from 'react'
import { Platform, View } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
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
    const insets = useSafeAreaInsets()
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
                <View className="flex-1 overflow-hidden">
                    <PackageTabs />
                    {isReady && <MoreDrawer />}
                    {isReady && <NotificationDrawer mobile />}
                </View>
                {isReady && <MobileTabBar />}
                {isReady && <MobileDrawer isVisible={isDrawerOpen} />}
            </View>
        </PackageProviderWrapper>
    )
})
