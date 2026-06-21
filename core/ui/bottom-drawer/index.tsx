import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import type { ReactNode } from 'react'
import { useCallback, useEffect, useState } from 'react'
import { Pressable, StyleSheet, View } from 'react-native'
import { Gesture, GestureDetector } from 'react-native-gesture-handler'
import Animated, {
    runOnJS,
    useAnimatedStyle,
    useSharedValue,
    withSpring,
    withTiming,
} from 'react-native-reanimated'

// Shared mobile bottom sheet. One implementation behind every slide-up sheet in
// the app (event quick-create, the More / notifications menus, the file picker,
// the editor image controls). Built on reanimated rather than a UI-kit overlay
// because the kit's bottom sheet renders its backdrop in a wrapper that defeats
// z-index on native (the dim covered the content), and portals itself to the
// app root so it slid UNDER the tab bar — the two bugs this component avoids.
//
// Behaviour:
//   • Backdrop is rendered BEFORE the sheet, so the sheet paints on top of it by
//     sibling order — no z-index fighting.
//   • The sheet rests at the bottom edge of ITS PARENT. Callers mount it inside
//     the mobile chrome's content region (MobileLayout's flex-1 area), which
//     already ends at the top of the tab bar — so bottom:0 sits exactly on the
//     tab bar with no manual offset. (Rendering inline rather than portaling to
//     the app root is what keeps it above the bar.)
//   • Drag down past a threshold (or a fast flick) dismisses; a short drag snaps
//     back. Tapping the backdrop dismisses.
//   • Height is intrinsic to the content (measured via onLayout to drive the
//     slide/drag distance) and capped at maxHeightPercent of the screen.

const SPRING_CONFIG = { damping: 28, stiffness: 220, mass: 0.8 }
// Off-screen fallback before the sheet has measured itself (first open frame).
const INITIAL_OFFSCREEN = 1000

interface BottomDrawerProps {
    isOpen: boolean
    onClose: () => void
    children: ReactNode
    /** Max height as a fraction of the screen (0–1). Default 0.85. */
    maxHeightPercent?: number
    /** Background theme token for the sheet surface. Default 'background'. */
    surface?: Parameters<typeof useThemeColor>[0]
    /** Hide the drag-handle pill (shown by default). */
    hideHandle?: boolean
}

export function BottomDrawer({
    isOpen,
    onClose,
    children,
    maxHeightPercent = 0.85,
    surface = 'background',
    hideHandle = false,
}: BottomDrawerProps) {
    const overlayBg = useThemeColor('overlay-backdrop')
    const surfaceBg = useThemeColor(surface)
    const handleColor = useThemeColor('border')

    const sheetHeight = useSharedValue(INITIAL_OFFSCREEN)
    const translateY = useSharedValue(INITIAL_OFFSCREEN)
    const backdropOpacity = useSharedValue(0)
    const [mounted, setMounted] = useState(false)

    const close = useCallback(() => onClose(), [onClose])

    useEffect(() => {
        if (isOpen) {
            setMounted(true)
            translateY.value = withSpring(0, SPRING_CONFIG)
            backdropOpacity.value = withTiming(1, { duration: 200 })
        } else if (mounted) {
            translateY.value = withSpring(sheetHeight.value, SPRING_CONFIG)
            backdropOpacity.value = withTiming(0, { duration: 150 })
            const timeout = setTimeout(() => setMounted(false), 300)
            return () => clearTimeout(timeout)
        }
    }, [isOpen, translateY, backdropOpacity, mounted, sheetHeight])

    const panGesture = Gesture.Pan()
        .activeOffsetY(10)
        .onUpdate(e => {
            translateY.value = Math.max(0, e.translationY)
        })
        .onEnd(e => {
            // Threshold inlined (kept in sync with shouldDismissDrawer, which the
            // unit test pins): dragged past 100px or flicked down faster than
            // 500px/s dismisses. This runs on the UI thread, so it must not call
            // a non-worklet JS function — hence the literal comparison here.
            if (e.translationY > 100 || e.velocityY > 500) {
                translateY.value = withSpring(sheetHeight.value, SPRING_CONFIG)
                backdropOpacity.value = withTiming(0, { duration: 150 })
                runOnJS(close)()
            } else {
                translateY.value = withSpring(0, SPRING_CONFIG)
            }
        })

    const sheetStyle = useAnimatedStyle(() => ({
        transform: [{ translateY: translateY.value }],
    }))

    const backdropStyle = useAnimatedStyle(() => ({
        opacity: backdropOpacity.value,
    }))

    if (!mounted) return null

    return (
        <View
            className="absolute top-0 left-0 right-0 bottom-0 z-[250]"
            pointerEvents={isOpen ? 'auto' : 'none'}
        >
            <Animated.View style={[StyleSheet.absoluteFill, backdropStyle]}>
                <Pressable
                    style={[StyleSheet.absoluteFill, { backgroundColor: overlayBg }]}
                    onPress={close}
                    accessibilityRole="button"
                    accessibilityLabel="Close"
                />
            </Animated.View>

            <GestureDetector gesture={panGesture}>
                <Animated.View
                    onLayout={e => {
                        sheetHeight.value = e.nativeEvent.layout.height
                    }}
                    className="absolute left-0 right-0 bottom-0 rounded-t-2xl border-t border-border"
                    style={[
                        {
                            maxHeight: `${maxHeightPercent * 100}%`,
                            backgroundColor: surfaceBg,
                        },
                        sheetStyle,
                    ]}
                >
                    {!hideHandle && (
                        <View className="items-center py-2.5">
                            <View
                                className="w-9 h-1 rounded-sm"
                                style={{ backgroundColor: handleColor }}
                            />
                        </View>
                    )}
                    {children}
                </Animated.View>
            </GestureDetector>
        </View>
    )
}
