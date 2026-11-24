import { useActivePkgTermsBlocked } from '@tinycld/core/components/package-terms/PackageTermsGate'
import { shouldSettleDrawerOpen } from '@tinycld/core/components/workspace/drawer-release'
import { packageSidebars } from '@tinycld/core/lib/packages/derive-components'
import { usePackage } from '@tinycld/core/lib/packages/use-packages'
import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDeviceInsets } from '@tinycld/core/lib/use-safe-area'
import { Suspense, useCallback, useEffect, useState } from 'react'
import { Pressable, View } from 'react-native'
import { Gesture, GestureDetector } from 'react-native-gesture-handler'
import Animated, {
    runOnJS,
    useAnimatedStyle,
    useSharedValue,
    withSpring,
} from 'react-native-reanimated'

const PANEL_WIDTH = 280
const EDGE_WIDTH = 20

const SPRING_CONFIG = {
    damping: 25,
    stiffness: 200,
    mass: 0.8,
}

interface MobileDrawerProps {
    isVisible: boolean
}

export function MobileDrawer({ isVisible }: MobileDrawerProps) {
    const overlayBg = useThemeColor('overlay-backdrop')
    const sidebarBg = useThemeColor('sidebar-background')
    const insets = useDeviceInsets()
    const activePkgSlug = useWorkspaceStore(s => s.activePkgSlug)
    const setDrawerOpen = useWorkspaceStore(s => s.setDrawerOpen)
    const isEdgeSwipeSuspended = useWorkspaceStore(s => s.isEdgeSwipeSuspended)
    const pkg = usePackage(activePkgSlug ?? '')
    const isTermsBlocked = useActivePkgTermsBlocked()
    const translateX = useSharedValue(-PANEL_WIDTH)

    // Defer mounting the active package's sidebar until the drawer has been
    // opened at least once for this package. The sidebar runs live queries
    // (folder counts, calendar lists, etc.) that block the JS thread on mount;
    // pre-mounting it on every tab switch was the dominant cost of bottom-tab
    // navigation. Reset when the package changes so a freshly-tabbed-to package
    // doesn't pre-mount its sidebar before the user actually asks for it.
    const [openedForSlug, setOpenedForSlug] = useState<string | null>(null)
    const hasOpened = openedForSlug === activePkgSlug && activePkgSlug != null

    const openDrawer = useCallback(() => setDrawerOpen(true), [setDrawerOpen])
    const closeDrawer = useCallback(() => setDrawerOpen(false), [setDrawerOpen])

    useEffect(() => {
        translateX.value = withSpring(isVisible ? 0 : -PANEL_WIDTH, SPRING_CONFIG)
        if (isVisible && activePkgSlug && openedForSlug !== activePkgSlug) {
            setOpenedForSlug(activePkgSlug)
        }
    }, [isVisible, translateX, activePkgSlug, openedForSlug])

    const settle = (e: { translationX: number; velocityX: number }) => {
        'worklet'
        const shouldOpen = shouldSettleDrawerOpen(
            translateX.value,
            e.translationX,
            e.velocityX,
            PANEL_WIDTH
        )
        translateX.value = withSpring(shouldOpen ? 0 : -PANEL_WIDTH, SPRING_CONFIG)
        runOnJS(shouldOpen ? openDrawer : closeDrawer)()
    }

    // Disabled while a package drag is live (plus a short grace after it
    // ends): on a physical phone, a fingertip dragged into the left edge of
    // the glass can micro-lift as it reverses, and the replacement touch is
    // born inside this strip — indistinguishable from a deliberate edge
    // swipe. See setEdgeSwipeSuspended in workspace-store.
    const edgeGesture = Gesture.Pan()
        .enabled(!isEdgeSwipeSuspended)
        .activeOffsetX(10)
        .onUpdate(e => {
            translateX.value = Math.max(-PANEL_WIDTH, Math.min(0, -PANEL_WIDTH + e.translationX))
        })
        .onEnd(settle)

    const closeGesture = Gesture.Pan()
        .activeOffsetX(-10)
        .onUpdate(e => {
            translateX.value = Math.max(-PANEL_WIDTH, Math.min(0, e.translationX))
        })
        .onEnd(settle)

    const panelStyle = useAnimatedStyle(() => ({
        transform: [{ translateX: translateX.value }],
    }))

    const backdropStyle = useAnimatedStyle(() => ({
        opacity: (translateX.value + PANEL_WIDTH) / PANEL_WIDTH,
    }))

    const SidebarComponent = pkg ? packageSidebars[pkg.slug] : null

    // A package whose terms the owner has not accepted shows no sidebar here
    // either (PackageTermsGate covers its screen).
    if (!pkg?.sidebar || isTermsBlocked) return null

    return (
        <>
            {!isVisible ? (
                <GestureDetector gesture={edgeGesture}>
                    <Animated.View
                        style={{
                            position: 'absolute',
                            top: 0,
                            left: 0,
                            bottom: 0,
                            width: EDGE_WIDTH,
                            zIndex: 200,
                        }}
                    />
                </GestureDetector>
            ) : null}
            <GestureDetector gesture={closeGesture}>
                <View
                    style={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        bottom: 0,
                        zIndex: isVisible ? 200 : -1,
                    }}
                    pointerEvents={isVisible ? 'auto' : 'none'}
                >
                    <Animated.View
                        style={[
                            { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 },
                            backdropStyle,
                        ]}
                    >
                        <Pressable
                            testID="mobile-drawer-backdrop"
                            className="flex-1"
                            style={{
                                backgroundColor: overlayBg,
                            }}
                            onPress={() => setDrawerOpen(false)}
                        />
                    </Animated.View>
                    <Animated.View
                        testID="mobile-drawer-panel"
                        style={[
                            {
                                position: 'absolute',
                                top: 0,
                                left: 0,
                                bottom: 0,
                                width: PANEL_WIDTH,
                                zIndex: 201,
                                backgroundColor: sidebarBg,
                                paddingTop: insets.top,
                                // No paddingBottom: the sidebar this panel
                                // hosts reserves the home-indicator inset on
                                // its own ScrollView content (SidebarNav).
                                // Padding here too would stack both insets and
                                // re-create the dead strip at the bottom.
                                // Pinned to the left edge, so in the landscape
                                // rotation that puts the sensor housing on the
                                // left it sits over this panel's contents. Pad
                                // the panel, not the parent: its background
                                // still covers the gutter. Side-corrected, so
                                // with the housing on the right this is 0 and
                                // the panel stays full-bleed.
                                paddingLeft: insets.left,
                            },
                            panelStyle,
                        ]}
                    >
                        {hasOpened && SidebarComponent ? (
                            <Suspense fallback={null}>
                                <SidebarComponent isCollapsed={false} />
                            </Suspense>
                        ) : null}
                    </Animated.View>
                </View>
            </GestureDetector>
        </>
    )
}
