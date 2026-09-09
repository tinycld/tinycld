import { Popover } from '@tinycld/core/ui/popover'
import { type ReactElement, useCallback, useRef, useState } from 'react'
import { Platform, Pressable, Text, View } from 'react-native'

/** How long a press must be held on touch before the names appear. */
const LONG_PRESS_MS = 400

export interface ReactorTooltipProps {
    /** The finished sentence, e.g. "You and Nathan reacted 👍". */
    text: string
    /** The chip this describes. */
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
 * WEB HOVER IS A RAW DOM LISTENER, not Pressable's onHoverIn. RNW's synthetic
 * hover never fires for this wrapper — the chip inside is itself a Pressable
 * and owns the pointer — while a `mouseenter` listener attached to the same
 * node fires reliably. This was verified rather than assumed: the DOM event
 * logs, the synthetic one does not.
 *
 * Anchored to a POINT rather than a ref: Popover's ref anchor is only ever
 * used together with a trigger it clones and measures, and a pointer event
 * already carries coordinates — the same shape ContextMenu uses.
 */
export function ReactorTooltip({ text, children, testID }: ReactorTooltipProps) {
    const [point, setPoint] = useState<{ x: number; y: number } | null>(null)
    const pressTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
    const cleanupRef = useRef<(() => void) | null>(null)

    const close = useCallback(() => setPoint(null), [])

    const cancelPress = useCallback(() => {
        if (pressTimer.current) {
            clearTimeout(pressTimer.current)
            pressTimer.current = null
        }
    }, [])

    // Web only. The ref callback both attaches and tears down, so a chip
    // unmounting (a reaction taken back) leaves no listener behind.
    const attachHover = useCallback((node: unknown) => {
        cleanupRef.current?.()
        cleanupRef.current = null
        if (Platform.OS !== 'web') return

        const el = node as HTMLElement | null
        if (!el?.addEventListener) return

        const onEnter = (event: MouseEvent) => setPoint({ x: event.clientX, y: event.clientY })
        const onLeave = () => setPoint(null)
        el.addEventListener('mouseenter', onEnter)
        el.addEventListener('mouseleave', onLeave)
        cleanupRef.current = () => {
            el.removeEventListener('mouseenter', onEnter)
            el.removeEventListener('mouseleave', onLeave)
        }
    }, [])

    const touchProps =
        Platform.OS === 'web'
            ? {}
            : {
                  onPressIn: (event: { nativeEvent: { pageX: number; pageY: number } }) => {
                      const { pageX, pageY } = event.nativeEvent
                      cancelPress()
                      pressTimer.current = setTimeout(
                          () => setPoint({ x: pageX, y: pageY }),
                          LONG_PRESS_MS
                      )
                  },
                  onPressOut: cancelPress,
              }

    return (
        <>
            <Pressable
                ref={attachHover as never}
                {...touchProps}
                testID={testID}
                accessible={false}
            >
                {children}
            </Pressable>
            <TooltipSurface point={point} onClose={close} text={text} />
        </>
    )
}

function TooltipSurface({
    point,
    onClose,
    text,
}: {
    point: { x: number; y: number } | null
    onClose: () => void
    text: string
}) {
    if (!point) return null
    return (
        <Popover
            anchor={point}
            isOpen
            onOpenChange={open => {
                if (!open) onClose()
            }}
            placement="top-start"
            focus="none"
            // The surface is placed against the very chip the pointer is on,
            // so an interactive one lands between a press and its release:
            // the chip gets pointerdown, this appears, and mouseup goes here
            // instead. The toggle silently never fired.
            pointerTransparent
            title="Reactions"
        >
            <View className="px-2.5 py-1.5 max-w-[220px]">
                <Text className="text-[12.5px] text-foreground">{text}</Text>
            </View>
        </Popover>
    )
}
