import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useDeviceInsets } from '@tinycld/core/lib/use-safe-area'
import {
    LayerEscape,
    OverlayPortal,
    useHasSheetHost,
    useOverlayLayer,
} from '@tinycld/core/ui/overlay'
import { X } from 'lucide-react-native'
import type { ReactNode } from 'react'
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import {
    KeyboardAvoidingView,
    Platform,
    Pressable,
    ScrollView,
    StyleSheet,
    Text,
    View,
} from 'react-native'
import { Gesture, GestureDetector } from 'react-native-gesture-handler'
import Animated, {
    runOnJS,
    useAnimatedStyle,
    useSharedValue,
    withSpring,
    withTiming,
} from 'react-native-reanimated'

const SPRING_CONFIG = { damping: 28, stiffness: 220, mass: 0.8 }
// Off-screen fallback before the sheet has measured itself (first open frame).
const INITIAL_OFFSCREEN = 1000

/** The edge a sheet rests on and slides in from. */
export type SheetSide = 'top' | 'bottom'

// The rounded corners and the border face away from the edge the sheet rests on.
const SIDE_CLASS: Record<SheetSide, string> = {
    bottom: 'absolute left-0 right-0 bottom-0 rounded-t-2xl border-t border-border',
    top: 'absolute left-0 right-0 top-0 rounded-b-2xl border-b border-border',
}

// Footer padding depends on which edge the sheet rests on; the footer is a
// descendant, so the side reaches it by context rather than a prop.
const SheetSideContext = createContext<SheetSide>('bottom')

/**
 * The edge surface: what a Dialog, a Menu and a Popover become on a phone,
 * and what the mobile More menu, the notification drawer and the file picker
 * are on every breakpoint. It rests on the bottom edge by default; `side="top"`
 * hangs it from the top edge instead, which a Menu prefers because most menu
 * triggers sit near the top of the screen.
 *
 * Rendered into the `sheet` overlay host — the mobile chrome's content
 * region, which ends at the top of the tab bar and starts below the status
 * bar — so a bottom sheet rests exactly on the bar and a top sheet clears the
 * status bar, with no offset either way. Where no such host is mounted it
 * falls back to the root host and insets its own top edge. Backdrop first,
 * sheet second: the sheet paints above the backdrop by sibling order, no
 * z-index needed.
 *
 * Drag away from the edge past a threshold (or a fast flick) dismisses; a
 * short drag snaps back; the backdrop and Escape dismiss. Height is intrinsic
 * to the content, capped at `maxHeightPercent` of the host; a `Sheet.Body`
 * inside scrolls once that cap bites while the title row and `Sheet.Footer`
 * stay put.
 */
interface SheetProps {
    isOpen: boolean
    onClose: () => void
    children: ReactNode
    /** Renders a title row with a close button. */
    title?: string
    description?: string
    /** The edge the sheet rests on. Default 'bottom'. */
    side?: SheetSide
    /** Max height as a fraction of the host (0–1). Default 0.85. */
    maxHeightPercent?: number
    /** Background theme token for the sheet surface. Default 'background'. */
    surface?: Parameters<typeof useThemeColor>[0]
    /** Hide the drag-handle pill (shown by default). */
    hideHandle?: boolean
    testID?: string
}

function SheetRoot({
    isOpen,
    onClose,
    children,
    title,
    description,
    side = 'bottom',
    maxHeightPercent = 0.85,
    surface = 'background',
    hideHandle = false,
    testID,
}: SheetProps) {
    const overlayBg = useThemeColor('overlay-backdrop')
    const surfaceBg = useThemeColor(surface)
    const handleColor = useThemeColor('border')
    const insets = useDeviceInsets()
    const hasSheetHost = useHasSheetHost()

    // Sign of the off-screen direction: a bottom sheet leaves downward
    // (positive translateY), a top sheet upward. Captured by the worklets
    // below as a plain number, like SPRING_CONFIG.
    const dir = side === 'top' ? -1 : 1
    const sheetHeight = useSharedValue(INITIAL_OFFSCREEN)
    const translateY = useSharedValue(dir * INITIAL_OFFSCREEN)
    const backdropOpacity = useSharedValue(0)
    const [mounted, setMounted] = useState(false)
    const surfaceRef = useRef<View | null>(null)

    const close = useCallback(() => onClose(), [onClose])

    useEffect(() => {
        if (isOpen) {
            setMounted(true)
            translateY.value = withSpring(0, SPRING_CONFIG)
            backdropOpacity.value = withTiming(1, { duration: 200 })
        } else if (mounted) {
            translateY.value = withSpring(dir * sheetHeight.value, SPRING_CONFIG)
            backdropOpacity.value = withTiming(0, { duration: 150 })
            const timeout = setTimeout(() => setMounted(false), 300)
            return () => clearTimeout(timeout)
        }
    }, [isOpen, translateY, backdropOpacity, mounted, sheetHeight, dir])

    // The backdrop covers the host, so an outside press is a backdrop press;
    // the layer joins the stack for Escape and the Android back button only.
    useOverlayLayer({
        isOpen,
        nodes: () => [surfaceRef.current as unknown as Node | null],
        onDismiss: close,
        dismissOnOutside: false,
    })

    const panGesture = Gesture.Pan()
        .activeOffsetY(10)
        .onUpdate(e => {
            // Only movement away from the edge moves the sheet.
            translateY.value = dir * Math.max(0, dir * e.translationY)
        })
        .onEnd(e => {
            // Threshold inlined (kept in sync with shouldDismissSwipe in
            // ui/swipe-dismiss, which the unit test pins): dragged past 100px or
            // flicked away from the edge faster than 500px/s dismisses. This
            // runs on the UI thread, so it must not call a non-worklet JS
            // function — hence the literal comparison here.
            if (dir * e.translationY > 100 || dir * e.velocityY > 500) {
                translateY.value = withSpring(dir * sheetHeight.value, SPRING_CONFIG)
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

    // The sheet host already sits below the status bar; only the root-host
    // fallback puts a top sheet's edge under it.
    const paddingTop = side === 'top' && !hasSheetHost ? insets.top : 0

    if (!mounted) return null

    return (
        <OverlayPortal host="sheet">
            {isOpen ? <LayerEscape onEscape={close} /> : null}
            <View style={StyleSheet.absoluteFill} pointerEvents={isOpen ? 'auto' : 'none'}>
                <Animated.View style={[StyleSheet.absoluteFill, backdropStyle]}>
                    <Pressable
                        style={[StyleSheet.absoluteFill, { backgroundColor: overlayBg }]}
                        onPress={close}
                        accessibilityRole="button"
                        accessibilityLabel="Close"
                    />
                </Animated.View>

                <KeyboardAvoidingView
                    behavior={Platform.OS === 'ios' ? 'padding' : undefined}
                    pointerEvents="box-none"
                    style={StyleSheet.absoluteFill}
                >
                    <GestureDetector gesture={panGesture}>
                        <Animated.View
                            ref={surfaceRef}
                            testID={testID}
                            accessibilityViewIsModal
                            onLayout={e => {
                                sheetHeight.value = e.nativeEvent.layout.height
                            }}
                            className={SIDE_CLASS[side]}
                            style={[
                                {
                                    maxHeight: `${maxHeightPercent * 100}%`,
                                    backgroundColor: surfaceBg,
                                    paddingTop,
                                },
                                sheetStyle,
                            ]}
                        >
                            <SheetSideContext.Provider value={side}>
                                <Handle
                                    isVisible={!hideHandle && side === 'bottom'}
                                    color={handleColor}
                                />
                                <SheetHeader
                                    title={title}
                                    description={description}
                                    onClose={close}
                                />
                                {children}
                                <Handle
                                    isVisible={!hideHandle && side === 'top'}
                                    color={handleColor}
                                />
                            </SheetSideContext.Provider>
                        </Animated.View>
                    </GestureDetector>
                </KeyboardAvoidingView>
            </View>
        </OverlayPortal>
    )
}

/** The drag pill, drawn at the edge the user pulls away from. */
function Handle({ isVisible, color }: { isVisible: boolean; color: string }) {
    if (!isVisible) return null
    return (
        <View className="items-center py-2.5" testID="sheet-handle">
            <View className="w-9 h-1 rounded-sm" style={{ backgroundColor: color }} />
        </View>
    )
}

function SheetHeader({
    title,
    description,
    onClose,
}: {
    title?: string
    description?: string
    onClose: () => void
}) {
    const mutedColor = useThemeColor('muted')
    if (!title) return null
    return (
        <View className="flex-row items-start gap-3 px-5 pt-1 pb-3">
            <View className="flex-1 gap-1">
                <Text className="text-[16px] font-semibold text-foreground">{title}</Text>
                <SheetDescription text={description} />
            </View>
            <Pressable
                accessibilityRole="button"
                accessibilityLabel="Close"
                onPress={onClose}
                hitSlop={8}
                className="rounded-md p-0.5 -mr-1 -mt-0.5 hover:bg-foreground/10"
            >
                <X size={16} color={mutedColor} strokeWidth={2.2} />
            </Pressable>
        </View>
    )
}

function SheetDescription({ text }: { text?: string }) {
    if (!text) return null
    return <Text className="text-[13px] text-muted">{text}</Text>
}

/**
 * The part that scrolls once the sheet reaches its cap.
 *
 * Content is STRETCHED to the sheet's full width. A sheet is as wide as the
 * screen, but the surfaces that become one — a Popover, a Menu, a Dialog —
 * are often authored around a popover's narrow measured box, and a body laid
 * out at that intrinsic width leaves a phone-width sheet mostly empty with its
 * content hugging the left edge. Stretching makes filling the default, so a
 * body only has to opt OUT (see `usePopoverContext().isSheet`) rather than
 * every caller having to remember to opt in.
 */
function SheetBody({
    children,
    contentClassName,
    testID,
}: {
    children: ReactNode
    contentClassName?: string
    testID?: string
}) {
    return (
        <ScrollView
            testID={testID}
            bounces={false}
            keyboardShouldPersistTaps="handled"
            className="grow-0 shrink"
            contentContainerClassName={contentClassName ?? 'px-5 pb-5 gap-3'}
            contentContainerStyle={sheetBodyContentStyle}
        >
            {children}
        </ScrollView>
    )
}

/**
 * Cross-axis stretch for the scrolled content, so children fill the sheet's
 * width instead of sizing to themselves. Exported for the unit test, which
 * runs against the react-native stub and so has no computed layout to read.
 */
const sheetBodyContentStyle = { alignItems: 'stretch' } as const

/** The button row, pinned under the body — above the home indicator when the sheet rests on the bottom edge. */
function SheetFooter({ children, className }: { children: ReactNode; className?: string }) {
    const insets = useDeviceInsets()
    const side = useContext(SheetSideContext)
    const paddingBottom = side === 'bottom' ? Math.max(12, insets.bottom) : 12
    return (
        <View
            className={`flex-row justify-end items-center gap-2 px-5 pt-3 border-t border-border ${className ?? ''}`}
            style={{ paddingBottom }}
        >
            {children}
        </View>
    )
}

const Sheet = Object.assign(SheetRoot, { Body: SheetBody, Footer: SheetFooter })

export type { SheetProps }
export { Sheet, sheetBodyContentStyle }
