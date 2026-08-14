import { Overlay as GluestackOverlay } from '@gluestack-ui/core/overlay/creator'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ChevronRight } from 'lucide-react-native'
import React, {
    createContext,
    forwardRef,
    useCallback,
    useContext,
    useEffect,
    useId,
    useRef,
    useState,
} from 'react'
import { Dimensions, Platform, Pressable, StatusBar, StyleSheet, Text, View } from 'react-native'

// The menu overlay renders inside a statusBarTranslucent RN Modal whose
// content origin is the true top of the screen (behind the status bar).
// But the app's own window is NOT edge-to-edge, so triggerRef.measureInWindow
// reports Y relative to the below-status-bar origin. Those two origins differ
// by the status-bar height on Android, which made menus render that much too
// high (covering their trigger). Add the inset back to every measured Y so
// trigger coordinates live in the Modal's coordinate space. iOS/web: 0.
const ANDROID_STATUS_BAR_OFFSET = Platform.OS === 'android' ? (StatusBar.currentHeight ?? 0) : 0

// MenuContextValue carries root menu state. `contentLayout` is the
// measured rect of <Menu.Content> in window coordinates — submenus
// anchor off it. Single nesting level only: <Menu.Sub> inside a
// <Menu.SubContent> would still anchor off this root rect, which
// produces wrong positioning. Don't nest twice.
interface MenuContextValue {
    isOpen: boolean
    onOpenChange: (open: boolean) => void
    triggerRef: React.RefObject<View | null>
    triggerLayout: { x: number; y: number; width: number; height: number } | null
    setTriggerLayout: (
        layout: {
            x: number
            y: number
            width: number
            height: number
        } | null
    ) => void
    contentLayout: { x: number; y: number; width: number; height: number } | null
    setContentLayout: (
        layout: {
            x: number
            y: number
            width: number
            height: number
        } | null
    ) => void
    // Which submenu (by stable id) is currently open. Only one sibling
    // submenu may be open at a time, so opening one closes the others —
    // each <Menu.Sub> derives its `isOpen` from `activeSubId === myId`.
    activeSubId: string | null
    setActiveSubId: React.Dispatch<React.SetStateAction<string | null>>
}

const MenuContext = createContext<MenuContextValue | null>(null)

function useMenuContext() {
    const ctx = useContext(MenuContext)
    if (!ctx) throw new Error('Menu compound components must be used within <Menu>')
    return ctx
}

// Submenu state lives in its own context so Menu.Item rendered inside
// <Menu.SubContent> still resolves useMenuContext() to the *root* —
// pressing an item closes the entire chain via the root's
// onOpenChange(false).
interface MenuSubContextValue {
    isOpen: boolean
    setOpen: (open: boolean) => void
    triggerLayout: { x: number; y: number; width: number; height: number } | null
    setTriggerLayout: (
        layout: { x: number; y: number; width: number; height: number } | null
    ) => void
    hoverIntentRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>
}

const MenuSubContext = createContext<MenuSubContextValue | null>(null)

function useMenuSubContext() {
    const ctx = useContext(MenuSubContext)
    if (!ctx) throw new Error('Menu.SubTrigger / Menu.SubContent must be used within <Menu.Sub>')
    return ctx
}

// Shared row + content styling. Extracting these keeps Menu.Content,
// Menu.SubContent, Menu.Item, and Menu.SubTrigger visually identical
// without each component duplicating className strings.
const MENU_CONTENT_CLASS =
    'absolute min-w-[200px] border border-border bg-background rounded-lg py-1'
const MENU_CONTENT_SHADOW =
    Platform.OS === 'web'
        ? ({ boxShadow: '0 4px 16px rgba(0,0,0,0.15)' } as object)
        : {
              elevation: 8,
              shadowColor: '#000',
              shadowOffset: { width: 0, height: 4 },
              shadowOpacity: 0.15,
              shadowRadius: 12,
          }
const MENU_ROW_BASE_CLASS = 'flex-row items-center gap-2 px-3 py-2'

// SUBMENU_HOVER_CLOSE_DELAY_MS gives the cursor time to travel
// diagonally across the 4px gap between trigger and submenu without
// the close timer firing. 120ms is the sweet spot; below ~80ms feels
// snippy, above ~180ms feels sluggish on close-by-mouse-leave.
const SUBMENU_HOVER_CLOSE_DELAY_MS = 120

// ── Root ──

interface MenuProps {
    children: React.ReactNode
    isOpen?: boolean
    onOpenChange?: (open: boolean) => void
    /** Override trigger position (useful for context menus positioned at cursor) */
    triggerPosition?: { x: number; y: number; width: number; height: number } | null
    /** className forwarded to MenuRoot's wrapper View — needed when Menu wraps a
     * flex layout (e.g. context menu around a full-height editor). */
    className?: string
}

function MenuRoot({
    children,
    isOpen: controlledOpen,
    onOpenChange: controlledOnChange,
    triggerPosition,
    className,
}: MenuProps) {
    const [internalOpen, setInternalOpen] = useState(false)
    const isControlled = controlledOpen !== undefined
    const isOpen = controlledOpen ?? internalOpen
    // When controlled (an `isOpen` prop is supplied), the parent owns the open
    // state and `onOpenChange` just notifies it. When uncontrolled, the menu
    // owns the state: drive `internalOpen` AND still fire the consumer's
    // `onOpenChange` as a side-effect listener. Treating a bare `onOpenChange`
    // (no `isOpen`) as fully controlled — the old behaviour — left the internal
    // state stuck closed, so tap-triggered menus (DriveItemMenuButton) never
    // opened.
    const onOpenChange = useCallback(
        (next: boolean) => {
            if (!isControlled) setInternalOpen(next)
            controlledOnChange?.(next)
        },
        [isControlled, controlledOnChange]
    )
    const triggerRef = useRef<View | null>(null)
    const [internalLayout, setInternalLayout] = useState<{
        x: number
        y: number
        width: number
        height: number
    } | null>(null)
    const [contentLayout, setContentLayout] = useState<{
        x: number
        y: number
        width: number
        height: number
    } | null>(null)
    const [activeSubId, setActiveSubId] = useState<string | null>(null)

    // Shift the trigger Y by the Android status-bar inset so the menu —
    // which renders in a statusBarTranslucent Modal whose origin is the true
    // screen top — lines up with a trigger measured in the (non-edge-to-edge)
    // app window. Applied once here so it covers both measured triggers and
    // externally-supplied triggerPosition (e.g. native context-menu press
    // coords). Submenus stay correct: they position relative to the parent
    // Content via measureInWindow coords that share the raw-window space, so
    // the parent's corrected absolute position carries through.
    const rawTriggerLayout = triggerPosition ?? internalLayout
    // Memoize the offset-adjusted layout so its object identity is stable
    // across renders when the coordinates haven't changed. Returning a fresh
    // object every render would make Content.positionStyle (memoized on
    // triggerLayout) recompute every render, re-firing its publishLayout
    // effect → setContentLayout → re-render → infinite loop.
    const rawX = rawTriggerLayout?.x
    const rawY = rawTriggerLayout?.y
    const rawW = rawTriggerLayout?.width
    const rawH = rawTriggerLayout?.height
    const triggerLayout = React.useMemo(() => {
        if (rawX == null || rawY == null || rawW == null || rawH == null) return null
        if (!ANDROID_STATUS_BAR_OFFSET) return { x: rawX, y: rawY, width: rawW, height: rawH }
        return { x: rawX, y: rawY + ANDROID_STATUS_BAR_OFFSET, width: rawW, height: rawH }
    }, [rawX, rawY, rawW, rawH])

    // Reset which submenu is open whenever the root menu closes, so a
    // reopened menu doesn't flash a stale submenu from the prior session.
    useEffect(() => {
        if (!isOpen) setActiveSubId(null)
    }, [isOpen])

    return (
        <MenuContext.Provider
            value={{
                isOpen,
                onOpenChange,
                triggerRef,
                triggerLayout,
                setTriggerLayout: setInternalLayout,
                contentLayout,
                setContentLayout,
                activeSubId,
                setActiveSubId,
            }}
        >
            <View className={className}>{children}</View>
        </MenuContext.Provider>
    )
}

// ── Trigger ──

interface TriggerProps {
    children: React.ReactElement
    /** When true, the trigger won't open the menu on click (useful for context menus that open via onContextMenu) */
    disableClick?: boolean
}

function Trigger({ children, disableClick }: TriggerProps) {
    const { onOpenChange, isOpen, triggerRef, setTriggerLayout } = useMenuContext()
    const isOpenRef = useRef(isOpen)
    isOpenRef.current = isOpen

    const webDivRef = useRef<HTMLElement | null>(null)

    const measureWeb = useCallback(() => {
        if (!webDivRef.current) return
        const rect = webDivRef.current.getBoundingClientRect()
        setTriggerLayout({
            x: rect.left,
            y: rect.top,
            width: rect.width,
            height: rect.height,
        })
    }, [setTriggerLayout])

    const handleClick = useCallback(() => {
        if (disableClick) return
        if (Platform.OS === 'web' && webDivRef.current) {
            measureWeb()
            onOpenChange(!isOpenRef.current)
        } else if (triggerRef.current) {
            triggerRef.current.measureInWindow((x, y, width, height) => {
                setTriggerLayout({ x, y, width, height })
                onOpenChange(!isOpenRef.current)
            })
        } else {
            onOpenChange(!isOpenRef.current)
        }
    }, [disableClick, measureWeb, onOpenChange, setTriggerLayout, triggerRef])

    // Without this, a controlled <Menu> opened via an external hover
    // handler (e.g. the calc menubar's hover-swap between File/Edit/View)
    // has no `triggerLayout` recorded — the Content positions to (0,0)
    // because click was the only path that called `setTriggerLayout`.
    // Re-measuring on hover keeps the recorded rect fresh whenever the
    // pointer crosses the trigger.
    const handleMouseEnter = useCallback(() => {
        if (disableClick) return
        measureWeb()
    }, [disableClick, measureWeb])

    type PressableChildProps = {
        onPress?: (e: unknown) => void
        ref?: React.Ref<View>
    }
    const child = children as React.ReactElement<PressableChildProps>
    const childOnPress = child.props.onPress

    if (Platform.OS === 'web') {
        // The wrapper div stays — it is what measures the trigger rect and
        // catches hover — but the child is ALSO cloned with an onPress.
        //
        // Not cosmetic: a trigger child may branch on `onPress` to decide
        // whether it is interactive at all (cards' assignee/label/due values
        // render bare, unpressable text when it is absent, which is how a
        // read-only card is drawn). Passing children straight through left
        // that prop undefined on web only, so an OWNER'S card properties
        // rendered as inert "None" text with no way to open the picker, while
        // native was fine. The clone makes the contract the same on both
        // platforms: a Menu.Trigger child always receives an onPress.
        //
        // The div keeps `onClickCapture`, which runs BEFORE the child's own
        // handler and already toggles the menu, so the injected onPress only
        // forwards the child's original — toggling here too would open and
        // immediately re-close.
        const webChild = React.cloneElement(child, {
            onPress: childOnPress ?? (() => {}),
        })
        return (
            // biome-ignore lint/a11y/noStaticElementInteractions: wrapper div augments the interactive child (Pressable) with click/hover measurement; the child carries the button role and keyboard semantics.
            <div
                ref={node => {
                    webDivRef.current = node
                    ;(triggerRef as React.MutableRefObject<View | null>).current =
                        node as unknown as View
                }}
                onClickCapture={handleClick}
                onMouseEnter={handleMouseEnter}
            >
                {webChild}
            </div>
        )
    }

    // Wrapping a Pressable child in another Pressable swallows touches on native — the inner responder wins. Clone the child to inject onPress + ref.
    const composedOnPress = (e: unknown) => {
        childOnPress?.(e)
        handleClick()
    }
    return React.cloneElement(child, {
        ref: triggerRef as React.Ref<View>,
        onPress: composedOnPress,
    })
}

// ── Portal ──

function Portal({ children }: { children: React.ReactNode }) {
    const ctx = useMenuContext()

    // On native, render the overlay inside a real RN Modal
    // (statusBarTranslucent + transparent). Without it gluestack drops the
    // content into a plain absolutely-positioned portal host View, which
    // caused three native-only bugs:
    //   1. No full-screen layer behind the menu, so the backdrop Pressable
    //      couldn't catch outside taps — menus stayed open forever.
    //   2. Parent menu and submenu shared one stacking context, so on
    //      Android (which ignores zIndex without elevation) the parent's
    //      text painted through the submenu.
    //   3. The portal host's origin excluded the status-bar inset while
    //      measureInWindow includes it, so menus rendered too high (above
    //      the trigger) on Android.
    // A statusBarTranslucent Modal is full-screen and shares measureInWindow's
    // coordinate space, fixing all three. animationPreset='none' keeps the
    // snappy, animation-free feel the JS overlay had. Web ignores these
    // props (gluestack's web Overlay has no Modal path).
    return (
        <GluestackOverlay
            isOpen={ctx.isOpen}
            isKeyboardDismissable
            useRNModalOnAndroid
            useRNModal={Platform.OS === 'ios'}
            animationPreset="none"
            onRequestClose={() => ctx.onOpenChange(false)}
        >
            <MenuContext.Provider value={ctx}>
                {/* Native-only dismiss backdrop. On web, menus close via a
                 * document-level outside-click handler and a backdrop here
                 * would swallow clicks on sibling triggers (breaking the
                 * menubar hover/click swap). On native there's no such
                 * handler, and menubar menus render no <Menu.Overlay> of
                 * their own — so without this, File/Edit/View etc. could
                 * never be tapped closed. Rendered first so it sits behind
                 * the menu content. */}
                {Platform.OS !== 'web' && (
                    <Pressable
                        style={StyleSheet.absoluteFill}
                        onPress={() => ctx.onOpenChange(false)}
                    />
                )}
                {children}
            </MenuContext.Provider>
        </GluestackOverlay>
    )
}

// ── Overlay ──

const Overlay = forwardRef<View, { onPress?: () => void }>(function Overlay(_props, _ref) {
    const { onOpenChange } = useMenuContext()
    return (
        <Pressable
            onPress={() => onOpenChange(false)}
            className="absolute top-0 left-0 right-0 bottom-0"
        />
    )
})

// ── Content ──

interface ContentProps {
    children: React.ReactNode
    presentation?: 'popover' | 'bottom-sheet'
    placement?: 'top' | 'bottom' | 'left' | 'right'
    align?: 'start' | 'center' | 'end'
    className?: string
    style?: object
}

const Content = forwardRef<View, ContentProps>(function Content(
    { children, placement = 'bottom', align = 'start', className, style: styleProp },
    ref
) {
    const { triggerLayout, setContentLayout } = useMenuContext()
    const [contentSize, setContentSize] = useState<{ width: number; height: number } | null>(null)
    const windowDim = Dimensions.get('window')
    const innerRef = useRef<View | null>(null)
    const webDivRef = useRef<HTMLElement | null>(null)

    const positionStyle = React.useMemo(() => {
        if (!triggerLayout) return {}

        const pos: Record<string, number | string> = {}
        const gap = 4

        // Horizontal alignment
        if (placement === 'bottom' || placement === 'top') {
            if (align === 'start') pos.left = triggerLayout.x
            else if (align === 'end')
                pos.left = triggerLayout.x + triggerLayout.width - (contentSize?.width ?? 200)
            else
                pos.left = triggerLayout.x + triggerLayout.width / 2 - (contentSize?.width ?? 0) / 2
        }

        // Vertical placement
        if (placement === 'top') {
            if (contentSize) {
                pos.top = triggerLayout.y - contentSize.height - gap
            } else {
                pos.top = -9999
            }
        } else {
            pos.top = triggerLayout.y + triggerLayout.height + gap
        }

        // Clamp horizontal
        if (contentSize) {
            if (typeof pos.left === 'number' && pos.left + contentSize.width > windowDim.width - 8)
                pos.left = windowDim.width - contentSize.width - 8
            if (typeof pos.left === 'number' && pos.left < 8) pos.left = 8
        }

        // Flip vertical if overflowing
        if (contentSize && typeof pos.top === 'number') {
            if (pos.top + contentSize.height > windowDim.height - 8) {
                pos.top = triggerLayout.y - contentSize.height - gap
            }
            if (pos.top < 8) {
                pos.top = triggerLayout.y + triggerLayout.height + gap
            }
        }

        return pos
    }, [triggerLayout, placement, align, contentSize, windowDim])

    // Publish content layout (window coords) so submenus can anchor.
    // Keep size measurement in onLayout but resolve position via
    // getBoundingClientRect / measureInWindow once size + positionStyle
    // are stable.
    const publishLayout = useCallback(() => {
        if (Platform.OS === 'web' && webDivRef.current) {
            const rect = webDivRef.current.getBoundingClientRect()
            setContentLayout({
                x: rect.left,
                y: rect.top,
                width: rect.width,
                height: rect.height,
            })
        } else if (innerRef.current) {
            innerRef.current.measureInWindow((x, y, width, height) => {
                if (Number.isFinite(x) && Number.isFinite(y) && width > 0 && height > 0) {
                    setContentLayout({ x, y, width, height })
                }
            })
        }
    }, [setContentLayout])

    // biome-ignore lint/correctness/useExhaustiveDependencies: positionStyle is read indirectly — its change triggers a DOM reflow, and we re-measure the post-reflow rect. Dropping it would skip republishing after overflow flips.
    useEffect(() => {
        if (!contentSize) return
        publishLayout()
    }, [contentSize, positionStyle, publishLayout])

    useEffect(() => {
        return () => setContentLayout(null)
    }, [setContentLayout])

    const setRefs = useCallback(
        (node: View | null) => {
            innerRef.current = node
            if (typeof ref === 'function') ref(node)
            else if (ref) (ref as React.MutableRefObject<View | null>).current = node
            if (Platform.OS === 'web') {
                webDivRef.current = node as unknown as HTMLElement
            }
        },
        [ref]
    )

    return (
        <View
            ref={setRefs}
            onLayout={e => {
                const { width, height } = e.nativeEvent.layout
                setContentSize(prev =>
                    prev?.width === width && prev?.height === height ? prev : { width, height }
                )
            }}
            className={`${MENU_CONTENT_CLASS} ${className ?? ''}`}
            style={[MENU_CONTENT_SHADOW, positionStyle, styleProp]}
        >
            {children}
        </View>
    )
})

// ── Item ──

interface ItemProps {
    children: React.ReactNode
    onPress?: (e?: unknown) => void
    href?: string
    isDisabled?: boolean
    className?: string
    style?: object
    /**
     * Stable identity for tests. Menu labels are not unique — two packages can
     * contribute an action with the same label — and items sit as flat
     * siblings under their group heading, so there is nothing to scope a
     * selection to without this.
     */
    testID?: string
}

const Item = forwardRef<View, ItemProps>(function Item(
    { children, onPress: onPressProp, href, isDisabled, className, style: styleProp, testID },
    ref
) {
    const { onOpenChange } = useMenuContext()
    const [hovered, setHovered] = useState(false)

    const handlePress = useCallback(
        (e?: unknown) => {
            if (isDisabled) return
            onPressProp?.(e)
            onOpenChange(false)
        },
        [isDisabled, onPressProp, onOpenChange]
    )

    const hoverBg = hovered && !isDisabled ? 'bg-accent' : ''
    const itemClass = `${MENU_ROW_BASE_CLASS} ${hoverBg} ${isDisabled ? 'opacity-40' : 'opacity-100'} ${className ?? ''}`

    if (Platform.OS === 'web' && href) {
        return (
            <a
                ref={ref as React.Ref<HTMLAnchorElement>}
                href={href}
                role="menuitem"
                data-testid={testID}
                onClick={e => {
                    if (e.metaKey || e.ctrlKey || e.shiftKey) return
                    e.preventDefault()
                    handlePress()
                }}
                onMouseEnter={() => setHovered(true)}
                onMouseLeave={() => setHovered(false)}
                className={itemClass}
                style={{
                    display: 'flex',
                    textDecoration: 'none',
                    color: 'inherit',
                    cursor: isDisabled ? 'default' : 'pointer',
                    ...(styleProp as React.CSSProperties),
                }}
            >
                {children}
            </a>
        )
    }

    return (
        <Pressable
            ref={ref}
            onPress={handlePress}
            disabled={isDisabled}
            testID={testID}
            accessibilityRole="menuitem"
            onHoverIn={() => setHovered(true)}
            onHoverOut={() => setHovered(false)}
            className={itemClass}
            style={styleProp}
        >
            {children}
        </Pressable>
    )
})

// ── ItemTitle ──

const ItemTitle = forwardRef<
    Text,
    { children: React.ReactNode; className?: string; style?: object }
>(function ItemTitle({ children, className, style: styleProp }, ref) {
    return (
        <Text
            ref={ref}
            className={`text-foreground ${className ?? ''}`}
            style={[{ fontSize: 14 }, styleProp]}
        >
            {children}
        </Text>
    )
})

// ── Label ──

function Label({ children, className }: { children: React.ReactNode; className?: string }) {
    return (
        <Text
            className={`uppercase px-3 py-1 text-muted-foreground ${className ?? ''}`}
            style={{
                fontSize: 11,
                fontWeight: '600',
                letterSpacing: 0.5,
            }}
        >
            {children}
        </Text>
    )
}

// ── Separator ──

function Separator({ className }: { className?: string }) {
    return <View className={`my-1 h-px bg-border ${className ?? ''}`} />
}

// ── Sub ──

function Sub({ children }: { children: React.ReactNode }) {
    // Open state is owned by the root menu (activeSubId), so only one
    // sibling submenu is open at a time — clicking/hovering a second
    // SubTrigger closes the first. triggerLayout stays local since each
    // submenu anchors off its own trigger row.
    const id = useId()
    const { activeSubId, setActiveSubId } = useMenuContext()
    const isOpen = activeSubId === id
    const [triggerLayout, setTriggerLayout] = useState<MenuSubContextValue['triggerLayout']>(null)
    const hoverIntentRef = useRef<ReturnType<typeof setTimeout> | null>(null)

    const setOpen = useCallback(
        (next: boolean) => {
            // Only clear the active id if *we* are the active submenu — a
            // delayed hover-close from this submenu must not stomp a sibling
            // that just opened.
            if (next) setActiveSubId(id)
            else setActiveSubId(prev => (prev === id ? null : prev))
        },
        [id, setActiveSubId]
    )

    useEffect(() => {
        return () => {
            if (hoverIntentRef.current != null) {
                clearTimeout(hoverIntentRef.current)
                hoverIntentRef.current = null
            }
        }
    }, [])

    return (
        <MenuSubContext.Provider
            value={{ isOpen, setOpen, triggerLayout, setTriggerLayout, hoverIntentRef }}
        >
            {children}
        </MenuSubContext.Provider>
    )
}

// ── SubTrigger ──

interface SubTriggerProps {
    children: React.ReactNode
    isDisabled?: boolean
    className?: string
}

function SubTrigger({ children, isDisabled, className }: SubTriggerProps) {
    const { isOpen, setOpen, setTriggerLayout, hoverIntentRef } = useMenuSubContext()
    const [hovered, setHovered] = useState(false)
    const innerRef = useRef<View | null>(null)
    const webDivRef = useRef<HTMLElement | null>(null)
    const mutedColor = useThemeColor('muted-foreground')

    const cancelClose = useCallback(() => {
        if (hoverIntentRef.current != null) {
            clearTimeout(hoverIntentRef.current)
            hoverIntentRef.current = null
        }
    }, [hoverIntentRef])

    const measureAndOpen = useCallback(() => {
        if (Platform.OS === 'web' && webDivRef.current) {
            const rect = webDivRef.current.getBoundingClientRect()
            setTriggerLayout({
                x: rect.left,
                y: rect.top,
                width: rect.width,
                height: rect.height,
            })
        } else if (innerRef.current) {
            innerRef.current.measureInWindow((x, y, width, height) => {
                setTriggerLayout({ x, y, width, height })
            })
        }
        setOpen(true)
    }, [setOpen, setTriggerLayout])

    const handleHoverIn = useCallback(() => {
        if (isDisabled) return
        setHovered(true)
        cancelClose()
        measureAndOpen()
    }, [isDisabled, cancelClose, measureAndOpen])

    const handleHoverOut = useCallback(() => {
        setHovered(false)
        cancelClose()
        hoverIntentRef.current = setTimeout(() => {
            setOpen(false)
            hoverIntentRef.current = null
        }, SUBMENU_HOVER_CLOSE_DELAY_MS)
    }, [cancelClose, hoverIntentRef, setOpen])

    const handlePress = useCallback(() => {
        if (isDisabled) return
        if (isOpen) {
            setOpen(false)
        } else {
            measureAndOpen()
        }
    }, [isDisabled, isOpen, measureAndOpen, setOpen])

    const hoverBg = (hovered || isOpen) && !isDisabled ? 'bg-accent' : ''
    const rowClass = `${MENU_ROW_BASE_CLASS} ${hoverBg} ${isDisabled ? 'opacity-40' : 'opacity-100'} ${className ?? ''}`
    const chevron = <ChevronRight size={14} color={mutedColor} style={{ marginLeft: 'auto' }} />

    if (Platform.OS === 'web') {
        const activate = () => {
            if (isDisabled) return
            measureAndOpen()
        }
        return (
            <div
                ref={node => {
                    webDivRef.current = node
                    innerRef.current = node as unknown as View
                }}
                role="menuitem"
                aria-haspopup="menu"
                aria-expanded={isOpen}
                tabIndex={isDisabled ? -1 : 0}
                onMouseEnter={handleHoverIn}
                onMouseLeave={handleHoverOut}
                onClick={activate}
                onKeyDown={e => {
                    if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowRight') {
                        e.preventDefault()
                        activate()
                    }
                }}
                className={rowClass}
                style={{
                    display: 'flex',
                    cursor: isDisabled ? 'default' : 'pointer',
                }}
            >
                {children}
                {chevron}
            </div>
        )
    }

    return (
        <Pressable
            ref={innerRef}
            onPress={handlePress}
            disabled={isDisabled}
            accessibilityRole="menuitem"
            className={rowClass}
        >
            {children}
            {chevron}
        </Pressable>
    )
}

// ── SubContent ──

interface SubContentProps {
    children: React.ReactNode
    className?: string
    style?: object
}

const SubContent = forwardRef<View, SubContentProps>(function SubContent(
    { children, className, style: styleProp },
    ref
) {
    const { isOpen, setOpen, triggerLayout, hoverIntentRef } = useMenuSubContext()
    const { contentLayout } = useMenuContext()
    const [contentSize, setContentSize] = useState<{ width: number; height: number } | null>(null)
    const windowDim = Dimensions.get('window')

    const positionStyle = React.useMemo(() => {
        if (!isOpen) return { display: 'none' as const }
        if (!contentLayout || !triggerLayout) return { opacity: 0 }

        const pos: Record<string, number | string> = {}
        // Sheets/Excel style: the submenu overlaps the parent horizontally
        // by a few pixels so the two rounded panels read as one continuous
        // surface, and rises vertically by the menu's top padding so the
        // submenu's *first item* aligns with the SubTrigger row (not the
        // submenu's top edge — that would put the first item one row low).
        const overlap = 4
        const verticalLift = 4 // matches MENU_CONTENT_CLASS `py-1` top padding
        const measuredWidth = contentSize?.width ?? 200
        const measuredHeight = contentSize?.height ?? 0

        // Compute target in window coords, then convert to coords relative
        // to the parent Menu.Content (our containing block, since Content
        // is `position: absolute`). Without this conversion the submenu
        // ends up offset by Content's own (left, top) — typically far
        // offscreen — and never visible.
        let leftWindow = contentLayout.x + contentLayout.width - overlap
        if (leftWindow + measuredWidth > windowDim.width - 8) {
            // Flip to open on the parent's left side.
            leftWindow = contentLayout.x - measuredWidth + overlap
        }
        if (leftWindow < 8) leftWindow = 8

        let topWindow = triggerLayout.y - verticalLift
        if (topWindow + measuredHeight > windowDim.height - 8) {
            topWindow = Math.max(8, windowDim.height - measuredHeight - 8)
        }
        if (topWindow < 8) topWindow = 8

        pos.left = leftWindow - contentLayout.x
        pos.top = topWindow - contentLayout.y

        // SubContent is an absolutely-positioned child of the parent
        // Menu.Content, so it shares the parent's stacking context. Parent
        // rows *after* the SubTrigger (e.g. "Bullets & numbering", "Table")
        // come later in DOM order and would paint over the submenu without
        // an explicit lift. zIndex covers web; elevation covers Android
        // (which ignores zIndex on its own).
        pos.zIndex = 50
        pos.elevation = 24

        return pos
    }, [isOpen, contentLayout, triggerLayout, contentSize, windowDim])

    const cancelClose = useCallback(() => {
        if (hoverIntentRef.current != null) {
            clearTimeout(hoverIntentRef.current)
            hoverIntentRef.current = null
        }
    }, [hoverIntentRef])

    const scheduleClose = useCallback(() => {
        cancelClose()
        hoverIntentRef.current = setTimeout(() => {
            setOpen(false)
            hoverIntentRef.current = null
        }, SUBMENU_HOVER_CLOSE_DELAY_MS)
    }, [cancelClose, hoverIntentRef, setOpen])

    if (React.Children.count(children) === 0) return null
    if (!isOpen) return null

    const webHoverProps =
        Platform.OS === 'web'
            ? {
                  onMouseEnter: cancelClose,
                  onMouseLeave: scheduleClose,
              }
            : {}

    return (
        <View
            ref={ref}
            onLayout={e => {
                const { width, height } = e.nativeEvent.layout
                setContentSize(prev =>
                    prev?.width === width && prev?.height === height ? prev : { width, height }
                )
            }}
            className={`${MENU_CONTENT_CLASS} ${className ?? ''}`}
            style={[MENU_CONTENT_SHADOW, positionStyle, styleProp]}
            {...(webHoverProps as object)}
        >
            {children}
        </View>
    )
})

// ── Assemble compound component ──

const Menu = Object.assign(MenuRoot, {
    Trigger,
    Portal,
    Overlay,
    Content,
    Item,
    ItemTitle,
    Label,
    Sub,
    SubTrigger,
    SubContent,
})

export { Menu, Separator }
