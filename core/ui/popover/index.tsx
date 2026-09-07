import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'
import { OverlayPortal, useLayerFocus, useOverlayLayer } from '@tinycld/core/ui/overlay'
import { Sheet, type SheetSide } from '@tinycld/core/ui/sheet'
import React, {
    createContext,
    type ReactElement,
    type ReactNode,
    type RefObject,
    useCallback,
    useContext,
    useEffect,
    useLayoutEffect,
    useMemo,
    useRef,
    useState,
} from 'react'
import {
    Platform,
    Pressable,
    Modal as RNModal,
    ScrollView,
    StatusBar,
    StyleSheet,
    View,
} from 'react-native'
import { type Placement, placePopover, type Rect, type Size } from './place'

// The native layer is a statusBarTranslucent RN Modal whose origin is the true
// top of the screen, while `measureInWindow` reports Y from below the status
// bar on Android. Add the inset back so anchor coordinates live in the
// Modal's space. iOS and web: 0.
const ANDROID_STATUS_BAR_OFFSET = Platform.OS === 'android' ? (StatusBar.currentHeight ?? 0) : 0

export type PopoverPresentation = 'auto' | 'popover' | 'sheet'

/** A trigger's ref, or a point (a right-click, a long press) in window coordinates. */
export type PopoverAnchor = RefObject<View | null> | { x: number; y: number }

interface PopoverContextValue {
    isOpen: boolean
    close: () => void
    /** True when the surface is a bottom sheet rather than an anchored box. */
    isSheet: boolean
    /** The surface's box in window coordinates, once placed. Submenus anchor off it. */
    surfaceRect: Rect | null
    /** Web: the node a submenu portals into — a sibling of the scroll region, at the surface's origin. */
    subSlot: HTMLElement | null
    /** The surface's DOM node on web, for roving focus. */
    surfaceNode: () => HTMLElement | null
}

const PopoverContext = createContext<PopoverContextValue | null>(null)

export function usePopoverContext(): PopoverContextValue {
    const ctx = useContext(PopoverContext)
    if (!ctx) throw new Error('This component must be rendered inside a Popover or Menu')
    return ctx
}

export interface PopoverProps {
    /**
     * The element that opens the surface. Cloned with `onPress` and a ref, so
     * it must accept both (a Pressable, or a component that forwards them).
     */
    trigger?: ReactElement
    /** For a controlled surface with no trigger of its own: where to anchor. */
    anchor?: PopoverAnchor
    isOpen?: boolean
    onOpenChange?: (open: boolean) => void
    placement?: Placement
    /** `auto` is a sheet on the mobile breakpoint and a popover elsewhere. */
    presentation?: PopoverPresentation
    /** A fixed width for the surface; otherwise it fits its content, at least 200 wide. */
    width?: number
    /** Title of the sheet the surface becomes on a phone. */
    title?: string
    /** The edge that sheet rests on. Default `bottom`. */
    sheetSide?: SheetSide
    /** Padding of the sheet's scrolled content. */
    sheetContentClassName?: string
    className?: string
    testID?: string
    /** ARIA role of the surface. A Menu passes `menu`. */
    role?: 'menu' | 'dialog'
    /** Web keydown on the surface — a Menu's roving focus. */
    onKeyDown?: (event: KeyboardEvent) => void
    /** `trap` keeps Tab inside (a picker with fields); `none` for a Menu, whose focus roves. */
    focus?: 'trap' | 'none'
    children: ReactNode
}

/**
 * An anchored surface: measured against its trigger or a point, placed by
 * `placePopover`, capped to the room on its side and scrolling inside it,
 * dismissed by an outside press, Escape or the Android back button. On the
 * mobile breakpoint it is a Sheet with the same children.
 */
export function Popover({
    trigger,
    anchor,
    isOpen: controlledOpen,
    onOpenChange,
    placement = 'bottom-start',
    presentation = 'auto',
    width,
    title,
    sheetSide = 'bottom',
    sheetContentClassName,
    className,
    testID,
    role,
    onKeyDown,
    focus = 'trap',
    children,
}: PopoverProps) {
    const [internalOpen, setInternalOpen] = useState(false)
    const isControlled = controlledOpen !== undefined
    const isOpen = controlledOpen ?? internalOpen
    const setOpen = useCallback(
        (next: boolean) => {
            if (!isControlled) setInternalOpen(next)
            onOpenChange?.(next)
        },
        [isControlled, onOpenChange]
    )
    const close = useCallback(() => setOpen(false), [setOpen])
    const triggerRef = useRef<View | null>(null)
    const isMobile = useBreakpoint() === 'mobile'
    const isSheet = presentation === 'sheet' || (presentation === 'auto' && isMobile)

    const triggerElement = useTriggerElement(trigger, triggerRef, isOpen, setOpen)
    const resolvedAnchor: PopoverAnchor = anchor ?? triggerRef

    if (isSheet) {
        return (
            <>
                {triggerElement}
                <Sheet
                    isOpen={isOpen}
                    onClose={close}
                    title={title}
                    side={sheetSide}
                    testID={testID}
                >
                    <PopoverContext.Provider value={SHEET_CONTEXT(isOpen, close)}>
                        <Sheet.Body contentClassName={sheetContentClassName ?? 'px-4 pb-4'}>
                            {children}
                        </Sheet.Body>
                    </PopoverContext.Provider>
                </Sheet>
            </>
        )
    }

    return (
        <>
            {triggerElement}
            <PopoverLayer
                isVisible={isOpen}
                anchor={resolvedAnchor}
                placement={placement}
                width={width}
                className={className}
                testID={testID}
                role={role}
                onKeyDown={onKeyDown}
                focus={focus}
                close={close}
            >
                {children}
            </PopoverLayer>
        </>
    )
}

function SHEET_CONTEXT(isOpen: boolean, close: () => void): PopoverContextValue {
    return {
        isOpen,
        close,
        isSheet: true,
        surfaceRect: null,
        subSlot: null,
        surfaceNode: () => null,
    }
}

type PressableChildProps = { onPress?: (e: unknown) => void; ref?: React.Ref<View> }

/**
 * The trigger, cloned with the anchor ref and a composed `onPress` that
 * toggles the surface. Cloning rather than wrapping: a wrapper Pressable
 * swallows the child's touches on native, and a wrapper div on web is a
 * static element with a click handler. The child is always handed an
 * `onPress`, because a trigger may branch on that prop to decide whether it
 * is interactive at all (boards' card values render as inert text without
 * one).
 */
function useTriggerElement(
    trigger: ReactElement | undefined,
    triggerRef: React.MutableRefObject<View | null>,
    isOpen: boolean,
    setOpen: (next: boolean) => void
): ReactNode {
    const isOpenRef = useRef(isOpen)
    isOpenRef.current = isOpen
    const toggle = useCallback(() => setOpen(!isOpenRef.current), [setOpen])

    if (!trigger) return null
    const child = trigger as ReactElement<PressableChildProps>
    const childOnPress = child.props.onPress
    return React.cloneElement(child, {
        ref: triggerRef as React.Ref<View>,
        onPress: (e: unknown) => {
            childOnPress?.(e)
            toggle()
        },
    })
}

interface PopoverLayerProps {
    isVisible: boolean
    anchor: PopoverAnchor
    placement: Placement
    width?: number
    className?: string
    testID?: string
    role?: PopoverProps['role']
    onKeyDown?: PopoverProps['onKeyDown']
    focus: 'trap' | 'none'
    close: () => void
    children: ReactNode
}

function PopoverLayer(props: PopoverLayerProps) {
    if (!props.isVisible) return null
    return <OpenPopoverLayer {...props} />
}

function isPoint(anchor: PopoverAnchor): anchor is { x: number; y: number } {
    return 'x' in anchor
}

function OpenPopoverLayer({
    anchor,
    placement,
    width,
    className,
    testID,
    role,
    onKeyDown,
    focus,
    close,
    children,
}: PopoverLayerProps) {
    const viewportWidth = useWindowSizeStore(s => s.width)
    const viewportHeight = useWindowSizeStore(s => s.height)
    const [anchorRect, setAnchorRect] = useState<Rect | null>(null)
    const [size, setSize] = useState<Size | null>(null)
    const [surfaceRect, setSurfaceRect] = useState<Rect | null>(null)
    const [subSlot, setSubSlot] = useState<HTMLElement | null>(null)
    // State from a callback ref: the surface mounts inside the host a commit
    // after this component does, and effects keyed on it must run then.
    const [surfaceNode, setSurfaceNode] = useState<View | null>(null)
    const surfaceRef = useRef<View | null>(null)
    const setSurfaceRef = useCallback((node: View | null) => {
        surfaceRef.current = node
        setSurfaceNode(node)
    }, [])

    // Measure the anchor before first paint, on open and again on resize. A
    // menu opened from the keyboard or by a parent flipping `isOpen` never
    // touched its trigger, so the measurement cannot live in a click handler.
    const measureAnchor = useCallback(() => {
        if (isPoint(anchor)) {
            setAnchorRect({
                x: anchor.x,
                y: anchor.y + ANDROID_STATUS_BAR_OFFSET,
                width: 0,
                height: 0,
            })
            return
        }
        const node = anchor.current
        if (!node) return
        if (Platform.OS === 'web') {
            const rect = (node as unknown as HTMLElement).getBoundingClientRect()
            setAnchorRect({ x: rect.left, y: rect.top, width: rect.width, height: rect.height })
            return
        }
        node.measureInWindow((x, y, w, h) => {
            setAnchorRect({ x, y: y + ANDROID_STATUS_BAR_OFFSET, width: w, height: h })
        })
    }, [anchor])
    useLayoutEffect(measureAnchor, [measureAnchor])
    useLayoutEffect(() => useWindowSizeStore.subscribe(measureAnchor), [measureAnchor])

    const placed = useMemo(
        () =>
            anchorRect
                ? placePopover({
                      anchor: anchorRect,
                      size,
                      viewport: { width: viewportWidth, height: viewportHeight },
                      placement,
                  })
                : null,
        [anchorRect, size, viewportWidth, viewportHeight, placement]
    )

    // Publish where the surface landed, for submenus. After the position is
    // applied, so the rect is the post-reflow one.
    useLayoutEffect(() => {
        if (!placed || !size) return
        const node = surfaceNode
        if (!node) return
        if (Platform.OS === 'web') {
            const rect = (node as unknown as HTMLElement).getBoundingClientRect()
            setSurfaceRect({ x: rect.left, y: rect.top, width: rect.width, height: rect.height })
            return
        }
        node.measureInWindow((x, y, w, h) => {
            if (w > 0 && h > 0) setSurfaceRect({ x, y, width: w, height: h })
        })
    }, [placed, size, surfaceNode])

    const anchorNode = useCallback(
        () => (isPoint(anchor) ? null : (anchor.current as unknown as Node | null)),
        [anchor]
    )
    useOverlayLayer({
        isOpen: true,
        nodes: () => [surfaceRef.current as unknown as Node | null, anchorNode()],
        onDismiss: close,
        // RN Modal's onRequestClose already answers the Android back button.
        dismissOnEscape: Platform.OS === 'web',
    })
    useLayerFocus({
        isActive: true,
        container: surfaceNode as unknown as HTMLElement | null,
        trap: focus === 'trap',
    })

    // A DOM listener rather than a prop: react-native's View has no
    // `onKeyDown` in its types, and the surface only needs the key on web.
    const onKeyDownRef = useRef(onKeyDown)
    onKeyDownRef.current = onKeyDown
    useEffect(() => {
        if (Platform.OS !== 'web') return
        const node = surfaceNode as unknown as HTMLElement | null
        if (!node) return
        const handler = (event: KeyboardEvent) => onKeyDownRef.current?.(event)
        node.addEventListener('keydown', handler)
        return () => node.removeEventListener('keydown', handler)
    }, [surfaceNode])

    const setSlotRef = useCallback((node: View | null) => {
        if (Platform.OS !== 'web') return
        setSubSlot((node as unknown as HTMLElement | null) ?? null)
    }, [])

    const context = useMemo<PopoverContextValue>(
        () => ({
            isOpen: true,
            close,
            isSheet: false,
            surfaceRect,
            subSlot,
            surfaceNode: () =>
                Platform.OS === 'web'
                    ? (surfaceRef.current as unknown as HTMLElement | null)
                    : null,
        }),
        [close, surfaceRect, subSlot]
    )

    const surfaceStyle = {
        position: 'absolute' as const,
        top: placed?.top ?? 0,
        left: placed?.left ?? 0,
        maxHeight: placed?.maxHeight,
        maxWidth: placed?.maxWidth,
        width,
        // Invisible until measured and placed, so the first frame never shows
        // the surface at the origin.
        opacity: placed && size ? 1 : 0,
    }

    const surface = (
        <PopoverContext.Provider value={context}>
            <View
                ref={setSurfaceRef}
                testID={testID}
                role={role}
                tabIndex={-1}
                pointerEvents="auto"
                className={`${SURFACE_CLASS} ${className ?? ''}`}
                style={[SURFACE_SHADOW, surfaceStyle]}
            >
                {/* Measurement sits on the scroll CONTENT: placement derives
                    maxHeight from the natural size, and measuring the already
                    clamped outer box would feed its own cap back in. */}
                <ScrollView
                    bounces={false}
                    showsVerticalScrollIndicator={false}
                    keyboardShouldPersistTaps="handled"
                    contentContainerStyle={SCROLL_CONTENT_STYLE}
                >
                    <View
                        onLayout={e => {
                            const { width: w, height: h } = e.nativeEvent.layout
                            setSize(prev =>
                                prev?.width === w && prev.height === h
                                    ? prev
                                    : { width: w, height: h }
                            )
                        }}
                    >
                        {children}
                    </View>
                </ScrollView>
                {/* Where a submenu renders: a sibling of the ScrollView (which
                    clips and traps anything drawn outside its box), pinned to
                    the surface's origin so the submenu's offsets are measured
                    from the corner its math assumes. */}
                <View ref={setSlotRef} pointerEvents="box-none" style={SUB_SLOT_STYLE} />
            </View>
        </PopoverContext.Provider>
    )

    // No shortcut-scope bridge here, on purpose: a popover or menu does not
    // take the keyboard from the screen the way a dialog does. The board's
    // `j` still moves the focus ring while a card picker is open (and closes
    // it); a menu's own keys are handled on its surface. Escape is the
    // engine's capture-phase listener, which needs no scope.
    if (Platform.OS === 'web') {
        return <OverlayPortal>{surface}</OverlayPortal>
    }

    // A full-screen, transparent, status-bar-translucent Modal: the one native
    // container that stacks above every host, shares measureInWindow's
    // coordinates, and gives the backdrop something to catch outside taps on.
    return (
        <RNModal
            transparent
            statusBarTranslucent
            visible
            animationType="none"
            onRequestClose={close}
        >
            <Pressable
                style={StyleSheet.absoluteFill}
                onPress={close}
                accessible={false}
                importantForAccessibility="no"
            />
            {surface}
        </RNModal>
    )
}

const SURFACE_CLASS = 'min-w-[200px] border border-border bg-background rounded-lg py-1'
const SURFACE_SHADOW =
    Platform.OS === 'web'
        ? ({ boxShadow: '0 4px 16px rgba(0,0,0,0.15)' } as object)
        : {
              elevation: 8,
              shadowColor: '#000',
              shadowOffset: { width: 0, height: 4 },
              shadowOpacity: 0.15,
              shadowRadius: 12,
          }
const SCROLL_CONTENT_STYLE = { flexGrow: 1 } as const
const SUB_SLOT_STYLE = { position: 'absolute', top: 0, left: 0, width: 0, height: 0 } as const

export type { SheetSide } from '@tinycld/core/ui/sheet'
export { type Placement, placePopover, placeSubmenu, type Rect, type Size } from './place'
