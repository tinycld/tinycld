import { Popover } from '@tinycld/core/ui/popover'
import { type ReactElement, useCallback, useRef, useState } from 'react'
import { Platform, Text, View } from 'react-native'

/** How long a press must be held on touch before the names appear. */
const LONG_PRESS_MS = 400

export interface ReactorTooltipProps {
    /** The finished sentence, e.g. "You and Nathan reacted 👍". */
    text: string
    /** The chip. Must forward `onPress`-adjacent handlers and a ref. */
    children: ReactElement
    testID?: string
}

/**
 * Who reacted, on hover (web) or long-press (native).
 *
 * A Popover rather than core's Tooltip: that one is web-only CSS and returns
 * its children unchanged on native, so it cannot carry this at all. Popover
 * works on both platforms and becomes a sheet on the mobile breakpoint, which
 * is the right shape for a touch device anyway.
 *
 * Mounted only while open — a board can show hundreds of chips, and an
 * always-mounted overlay per chip would be a real cost for something almost
 * none of them will ever show.
 */
export function ReactorTooltip({ text, children, testID }: ReactorTooltipProps) {
    const [isOpen, setIsOpen] = useState(false)
    const anchorRef = useRef<View | null>(null)
    const pressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

    const open = useCallback(() => setIsOpen(true), [])
    const close = useCallback(() => setIsOpen(false), [])

    const cancelPress = useCallback(() => {
        if (pressTimer.current) {
            clearTimeout(pressTimer.current)
            pressTimer.current = null
        }
    }, [])

    const startPress = useCallback(() => {
        cancelPress()
        pressTimer.current = setTimeout(open, LONG_PRESS_MS)
    }, [cancelPress, open])

    // Hover on web, long-press on touch. Both land on the same surface.
    const triggerProps =
        Platform.OS === 'web'
            ? { onHoverIn: open, onHoverOut: close }
            : { onPressIn: startPress, onPressOut: cancelPress }

    return (
        <View ref={anchorRef} {...triggerProps} testID={testID}>
            {children}
            <ReactorSurface
                anchor={anchorRef}
                isOpen={isOpen}
                onOpenChange={setIsOpen}
                text={text}
            />
        </View>
    )
}

/**
 * Split out and given an isVisible-style early return so the Popover is not
 * constructed for a chip nobody is pointing at.
 */
function ReactorSurface({
    anchor,
    isOpen,
    onOpenChange,
    text,
}: {
    anchor: React.RefObject<View | null>
    isOpen: boolean
    onOpenChange: (open: boolean) => void
    text: string
}) {
    if (!isOpen) return null
    return (
        <Popover
            anchor={anchor}
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            placement="top-start"
            title="Reactions"
        >
            <View className="px-2.5 py-1.5 max-w-[220px]">
                <Text className="text-[12.5px] text-foreground">{text}</Text>
            </View>
        </Popover>
    )
}
