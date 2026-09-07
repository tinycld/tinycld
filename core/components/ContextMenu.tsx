import { Menu } from '@tinycld/core/ui/menu'
import type { ReactNode } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { type GestureResponderEvent, Platform, View, type ViewProps } from 'react-native'
import { markContextMenuOpenedByLongPress } from './context-menu-press-guard'

interface ContextMenuProps {
    children: ReactNode
    /**
     * Menu rows shown when the user opens the context menu. Accepts a
     * function so callers can avoid building the JSX tree until the menu
     * is actually opened — important when many ContextMenu wrappers are
     * mounted at once (e.g. one per drive list row).
     */
    content: ReactNode | (() => ReactNode)
    /**
     * Fires when the menu is about to open. Use to mirror the right-click
     * into selection state (e.g. `selectItem(id)`) so the highlighted
     * row matches the menu target. May return a cleanup function; if
     * provided, the cleanup runs when the menu is dismissed *without* the
     * user picking an item (outside-click, escape) — useful to roll back
     * a transient selection. The cleanup is NOT called when a menu item is
     * pressed, because the item's action already implies the user
     * intended the selection.
     */
    onOpen?: () => undefined | (() => void)
    /**
     * Forwarded to the wrapper View. Use `flex-1` when wrapping a child
     * (e.g. ScrollView) that needs to stretch to fill the available
     * space — otherwise the extra wrapper collapses to its intrinsic
     * size and the child stops scrolling.
     */
    className?: string
}

/**
 * A Menu opened by the platform's context gesture: right-click on web, a
 * ~400ms long press on native. The menu is anchored to the press point and
 * mounted only while open — a populated list renders dozens of these
 * wrappers per screen, and most are never opened.
 */
export function ContextMenu({ children, content, onOpen, className }: ContextMenuProps) {
    if (Platform.OS !== 'web') {
        return (
            <ContextMenuNative content={content} onOpen={onOpen} className={className}>
                {children}
            </ContextMenuNative>
        )
    }
    return (
        <ContextMenuWeb content={content} onOpen={onOpen} className={className}>
            {children}
        </ContextMenuWeb>
    )
}

/**
 * Open state plus the onOpen cleanup contract shared by both gestures: a
 * dismissal runs the cleanup, a chosen row does not.
 */
function useContextMenuSession(onOpen: ContextMenuProps['onOpen']) {
    const [point, setPoint] = useState<{ x: number; y: number } | null>(null)
    const onOpenRef = useRef(onOpen)
    onOpenRef.current = onOpen
    const cleanupRef = useRef<(() => void) | undefined>(undefined)
    const chosenRef = useRef(false)

    const openAt = useCallback((x: number, y: number) => {
        chosenRef.current = false
        setPoint({ x, y })
        const cleanup = onOpenRef.current?.()
        cleanupRef.current = typeof cleanup === 'function' ? cleanup : undefined
    }, [])

    // Menu rows run their onSelect before the menu reports closed, so a
    // selection marks the session chosen first and the close skips cleanup.
    const markChosen = useCallback(() => {
        chosenRef.current = true
    }, [])

    const handleOpenChange = useCallback((open: boolean) => {
        if (open) return
        if (!chosenRef.current) cleanupRef.current?.()
        cleanupRef.current = undefined
        setPoint(null)
    }, [])

    return { point, openAt, markChosen, handleOpenChange }
}

function ContextMenuWeb({ children, content, onOpen, className }: ContextMenuProps) {
    const { point, openAt, markChosen, handleOpenChange } = useContextMenuSession(onOpen)

    const handleContextMenu = useCallback(
        (e: { preventDefault: () => void; clientX: number; clientY: number }) => {
            e.preventDefault()
            openAt(e.clientX, e.clientY)
        },
        [openAt]
    )

    return (
        <View
            className={className}
            {...({ onContextMenu: handleContextMenu } as unknown as ViewProps)}
        >
            {children}
            <OpenContextMenu
                point={point}
                content={content}
                onOpenChange={handleOpenChange}
                onChoose={markChosen}
            />
        </View>
    )
}

function OpenContextMenu({
    point,
    content,
    onOpenChange,
    onChoose,
}: {
    point: { x: number; y: number } | null
    content: ReactNode | (() => ReactNode)
    onOpenChange: (open: boolean) => void
    onChoose: () => void
}) {
    if (!point) return null
    const rendered = typeof content === 'function' ? content() : content
    return (
        <Menu isOpen anchor={point} onOpenChange={onOpenChange} presentation="popover">
            {/* A capture-phase press on any row marks the session chosen
                before the row's own onSelect closes the menu. */}
            <View onTouchStart={onChoose} {...({ onMouseDownCapture: onChoose } as object)}>
                {rendered}
            </View>
        </Menu>
    )
}

// Native path: open the context menu via a long-press gesture (~400ms
// hold), mirroring the iOS / Android system context-menu gesture.
//
// Why we don't wrap in a Pressable: drive/mail/etc. rows already have their
// own <Pressable onPress={…}> for select/preview. Adding an outer Pressable
// wins the responder via RN's bubble-up rules (outer becomes responder,
// inner's onPress never fires). Instead we observe touches without claiming
// the responder — RN bubbles onTouchStart/Move/End/Cancel to ancestor Views
// even when an inner Pressable owns the gesture — and run a long-press
// timer in parallel with the row's own onPress handling. A short tap fires
// onTouchEnd quickly, which cancels the timer; a held touch fires it and
// opens the menu.
function ContextMenuNative({ children, content, onOpen, className }: ContextMenuProps) {
    const { point, openAt, markChosen, handleOpenChange } = useContextMenuSession(onOpen)
    const longPressTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
    const startCoordsRef = useRef<{ x: number; y: number } | null>(null)
    // iOS Files tolerates some drift during a long-press; so do we.
    const DRIFT_TOLERANCE_PX = 10

    const cancelLongPress = useCallback(() => {
        if (longPressTimerRef.current) {
            clearTimeout(longPressTimerRef.current)
            longPressTimerRef.current = null
        }
    }, [])

    // Stop the timer if the consumer unmounts mid-hold.
    useEffect(() => cancelLongPress, [cancelLongPress])

    const handleTouchStart = useCallback(
        (e: GestureResponderEvent) => {
            const { pageX, pageY } = e.nativeEvent
            startCoordsRef.current = { x: pageX, y: pageY }
            cancelLongPress()
            longPressTimerRef.current = setTimeout(() => {
                // Tell the underlying row to ignore the press it will receive
                // when the finger lifts — otherwise a long-press would open the
                // menu AND trigger the row's tap (e.g. open the file).
                markContextMenuOpenedByLongPress(Date.now())
                openAt(pageX, pageY)
                longPressTimerRef.current = null
            }, 400)
        },
        [cancelLongPress, openAt]
    )

    const handleTouchMove = useCallback(
        (e: GestureResponderEvent) => {
            const start = startCoordsRef.current
            if (!start) return
            const dx = e.nativeEvent.pageX - start.x
            const dy = e.nativeEvent.pageY - start.y
            if (Math.sqrt(dx * dx + dy * dy) > DRIFT_TOLERANCE_PX) cancelLongPress()
        },
        [cancelLongPress]
    )

    return (
        <>
            <View
                className={className}
                onTouchStart={handleTouchStart}
                onTouchMove={handleTouchMove}
                onTouchEnd={cancelLongPress}
                onTouchCancel={cancelLongPress}
            >
                {children}
            </View>
            <OpenContextMenu
                point={point}
                content={content}
                onOpenChange={handleOpenChange}
                onChoose={markChosen}
            />
        </>
    )
}
