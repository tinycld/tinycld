import { useCallback, useEffect } from 'react'
import type { ViewStyle } from 'react-native'
import { Platform } from 'react-native'
import { Gesture } from 'react-native-gesture-handler'
import { runOnJS, useAnimatedStyle, useSharedValue, withSpring } from 'react-native-reanimated'

/** The edge a panel rests on, and therefore the direction it leaves in. */
export type SwipeAnchor = 'left' | 'right' | 'top' | 'bottom'

// Shared with Sheet (ui/sheet) so every edge surface settles identically.
const SPRING_CONFIG = { damping: 28, stiffness: 220, mass: 0.8 }

// A drag has to travel this far before it counts as a dismissal rather than a
// scroll that wandered sideways. Mirrors shouldDismissSwipe, which the unit
// test pins; the worklets below inline the comparison because they run on the
// UI thread and cannot call a non-worklet JS function.
const DISMISS_DISTANCE = 100
const DISMISS_VELOCITY = 500

/** +1 when the panel leaves rightward or downward, -1 when it leaves the other way. */
const DIRECTION: Record<SwipeAnchor, number> = { right: 1, bottom: 1, left: -1, top: -1 }

const IS_HORIZONTAL: Record<SwipeAnchor, boolean> = {
    left: true,
    right: true,
    top: false,
    bottom: false,
}

interface SwipeToDismissOptions {
    /** The edge the panel rests on. Drags away from it dismiss. */
    anchor: SwipeAnchor
    /** Called once the panel has finished animating off-screen. */
    onClose: () => void
    /**
     * How far the panel must travel to clear the screen. Pass a measured value
     * where the size is not a constant — a Drawer's width is a percentage class,
     * so it has to come from onLayout.
     */
    distance: number
    /** Re-centres the panel when it reopens, so a reused node starts on-screen. */
    isOpen?: boolean
}

/**
 * Swipe an edge-anchored panel away to dismiss it.
 *
 * The panel tracks the finger 1:1, clamped so it can only move AWAY from the
 * edge it rests on; released short of the threshold it springs back, and past
 * it (or on a fast flick) it springs off-screen. Modelled on the two gestures
 * core already had — Sheet's vertical drag and MobileDrawer's closeGesture —
 * and generalised over an axis sign so one hook serves all four anchors.
 *
 * `onClose` fires from the spring's COMPLETION callback, not immediately. Both
 * callers unmount the panel the instant their open flag flips, so closing first
 * would make it vanish mid-drag instead of sliding off; waiting for the spring
 * keeps it mounted for the ~200ms the animation needs.
 *
 * Native only. On web the gesture is disabled and the style is inert: a drawer
 * there already has Escape, a backdrop click and a close button, and the boards
 * peek deliberately leaves the surface behind it operable — a live gesture
 * detector risks the "intercepts pointer events" regressions that surface has
 * already paid for once.
 */
export function useSwipeToDismiss({
    anchor,
    onClose,
    distance,
    isOpen = true,
}: SwipeToDismissOptions): {
    gesture: ReturnType<typeof Gesture.Pan>
    /**
     * Typed as a plain ViewStyle rather than Reanimated's AnimatedStyle: the
     * hook only ever produces a transform, and the wider union does not assign
     * to the style prop of a gluestack-created Content.
     */
    animatedStyle: ViewStyle
} {
    const offset = useSharedValue(0)
    const dir = DIRECTION[anchor]
    const isHorizontal = IS_HORIZONTAL[anchor]
    const isEnabled = Platform.OS !== 'web'

    // A panel kept mounted across opens (Sheet does this) would otherwise
    // reopen still translated from the drag that closed it.
    useEffect(() => {
        if (isOpen) offset.value = 0
    }, [isOpen, offset])

    const close = useCallback(() => onClose(), [onClose])

    const basePan = Gesture.Pan().enabled(isEnabled)
    // Only a deliberate drag along the panel's own axis starts this, so a
    // scroll inside the panel is never stolen into a dismissal.
    const gesture = (
        isHorizontal ? basePan.activeOffsetX(dir * 10) : basePan.activeOffsetY(dir * 10)
    )
        .onUpdate(e => {
            const travel = isHorizontal ? e.translationX : e.translationY
            // Clamped to movement AWAY from the edge: dragging back the other
            // way would lift the panel off the edge it rests on and open a gap.
            offset.value = dir * Math.max(0, dir * travel)
        })
        .onEnd(e => {
            const travel = isHorizontal ? e.translationX : e.translationY
            const velocity = isHorizontal ? e.velocityX : e.velocityY
            // Inlined rather than calling shouldDismissSwipe: this runs on the
            // UI thread, which cannot call a non-worklet JS function.
            if (dir * travel > DISMISS_DISTANCE || dir * velocity > DISMISS_VELOCITY) {
                offset.value = withSpring(dir * distance, SPRING_CONFIG, finished => {
                    if (finished) runOnJS(close)()
                })
            } else {
                offset.value = withSpring(0, SPRING_CONFIG)
            }
        })

    const animatedStyle = useAnimatedStyle(() =>
        isHorizontal
            ? { transform: [{ translateX: offset.value }] }
            : { transform: [{ translateY: offset.value }] }
    )

    return { gesture, animatedStyle: animatedStyle as ViewStyle }
}

export { shouldDismissSwipe } from './should-dismiss'
