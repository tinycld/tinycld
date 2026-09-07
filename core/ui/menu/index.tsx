import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import {
    type Placement,
    Popover,
    type PopoverAnchor,
    type PopoverPresentation,
    placeSubmenu,
    type Rect,
    type SheetSide,
    type Size,
    usePopoverContext,
} from '@tinycld/core/ui/popover'
import { Check, ChevronRight, type LucideIcon } from 'lucide-react-native'
import React, {
    createContext,
    type Dispatch,
    type ReactElement,
    type ReactNode,
    type SetStateAction,
    useCallback,
    useContext,
    useEffect,
    useId,
    useMemo,
    useRef,
    useState,
} from 'react'
import { createPortal } from 'react-dom'
import { Platform, Pressable, Text, View } from 'react-native'

/**
 * A list of commands on a Popover: `role="menu"`, arrow keys and typeahead,
 * one submenu level, and rows that close the whole surface when chosen. On
 * the mobile breakpoint the same rows render in a Sheet.
 *
 * Rows are props, not children: a row is a label with an icon, a shortcut
 * and a state, and a sheet must be able to draw the same row as a touch
 * target. `Menu.Custom` holds the rare widget (a color grid) a menu mixes in.
 */
export interface MenuProps {
    /** The element that opens the menu. Cloned with `onPress` and a ref. */
    trigger?: ReactElement
    /** For a controlled menu with no trigger of its own: a ref or a point. */
    anchor?: PopoverAnchor
    isOpen?: boolean
    onOpenChange?: (open: boolean) => void
    placement?: Placement
    /** `auto` is a sheet on the mobile breakpoint. Pin `popover` for two or three rows beside their trigger. */
    presentation?: PopoverPresentation
    width?: number
    /** Title of the sheet the menu becomes on a phone. */
    title?: string
    /**
     * The edge that sheet rests on. Default `top`, since most menu triggers sit
     * near the top of the screen; pass `bottom` for a menu opened from the
     * bottom of a screen.
     */
    sheetSide?: SheetSide
    className?: string
    testID?: string
    children: ReactNode
}

interface MenuContextValue {
    /** Which submenu is open. One at a time: opening a sibling closes the other. */
    activeSubId: string | null
    setActiveSubId: Dispatch<SetStateAction<string | null>>
}

const MenuContext = createContext<MenuContextValue | null>(null)

function useMenuContext(): MenuContextValue {
    const ctx = useContext(MenuContext)
    if (!ctx) throw new Error('Menu rows must be rendered inside <Menu>')
    return ctx
}

function MenuRoot({
    trigger,
    anchor,
    isOpen,
    onOpenChange,
    placement = 'bottom-start',
    presentation = 'auto',
    width,
    title,
    sheetSide = 'top',
    className,
    testID,
    children,
}: MenuProps) {
    const [activeSubId, setActiveSubId] = useState<string | null>(null)
    const handleOpenChange = useCallback(
        (next: boolean) => {
            // A reopened menu must not flash the submenu from its last session.
            if (!next) setActiveSubId(null)
            onOpenChange?.(next)
        },
        [onOpenChange]
    )
    const context = useMemo(() => ({ activeSubId, setActiveSubId }), [activeSubId])

    return (
        <Popover
            trigger={trigger}
            anchor={anchor}
            isOpen={isOpen}
            onOpenChange={handleOpenChange}
            placement={placement}
            presentation={presentation}
            width={width}
            title={title}
            sheetSide={sheetSide}
            sheetContentClassName="pb-2"
            className={className}
            testID={testID}
            role="menu"
            focus="none"
            onKeyDown={handleMenuKeyDown}
        >
            <MenuContext.Provider value={context}>{children}</MenuContext.Provider>
        </Popover>
    )
}

// ── Keyboard ──

const ROW_SELECTOR = '[role="menuitem"], [role="menuitemcheckbox"]'

/**
 * Roving focus for the rows of whichever menu surface holds focus — the root
 * or an open submenu, both `role="menu"`. Arrow keys wrap, Home and End jump,
 * a printable character jumps to the next row starting with it. Enter and
 * Space are the row's own business.
 */
function handleMenuKeyDown(event: KeyboardEvent) {
    const surface = event.currentTarget as HTMLElement
    const active = document.activeElement as HTMLElement | null
    // A search field inside the menu owns its own keys: typing must not
    // become typeahead, and the arrows must not leave the caret.
    if (isEditable(event.target)) return
    const scope = (active?.closest('[role="menu"]') as HTMLElement | null) ?? surface
    const rows = Array.from(scope.querySelectorAll<HTMLElement>(ROW_SELECTOR)).filter(
        row =>
            row.closest('[role="menu"]') === scope && row.getAttribute('aria-disabled') !== 'true'
    )
    if (rows.length === 0) return
    const index = active ? rows.indexOf(active) : -1

    // A key the menu answers is the menu's: it must not also reach the
    // screen's shortcuts (a list's j/k) through the window listener.
    const focusAt = (next: number) => {
        event.preventDefault()
        event.stopPropagation()
        rows[(next + rows.length) % rows.length]?.focus()
    }

    switch (event.key) {
        case 'ArrowDown':
            return focusAt(index + 1)
        case 'ArrowUp':
            return focusAt(index === -1 ? rows.length - 1 : index - 1)
        case 'Home':
            return focusAt(0)
        case 'End':
            return focusAt(rows.length - 1)
    }

    if (event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey) {
        const wanted = event.key.toLowerCase()
        for (let step = 1; step <= rows.length; step += 1) {
            const candidate = rows[(index + step) % rows.length]
            if (candidate.textContent?.trim().toLowerCase().startsWith(wanted)) {
                focusAt(index + step)
                return
            }
        }
    }
}

function isEditable(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false
    return target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable
}

// ── Rows ──

export interface MenuItemProps {
    label: string
    icon?: LucideIcon
    /** Any leading node; takes precedence over `icon` and `colorDot`. */
    leading?: ReactNode
    colorDot?: string
    onSelect?: () => void
    /** Renders the row as a link on web; `onSelect` still runs on a plain click. */
    href?: string
    /** Shown right-aligned, as written (`⌘B`). */
    shortcut?: string
    /** A check mark: the current choice among alternatives. */
    isSelected?: boolean
    isDisabled?: boolean
    isDestructive?: boolean
    /** Stable identity for tests when labels are not unique. */
    testID?: string
}

function MenuItem({
    label,
    icon,
    leading,
    colorDot,
    onSelect,
    href,
    shortcut,
    isSelected = false,
    isDisabled = false,
    isDestructive = false,
    testID,
}: MenuItemProps) {
    const { close } = usePopoverContext()
    const select = useCallback(() => {
        onSelect?.()
        close()
    }, [onSelect, close])
    return (
        <MenuRow
            role="menuitem"
            label={label}
            leading={<RowLeading icon={icon} leading={leading} colorDot={colorDot} />}
            trailing={<RowTrailing shortcut={shortcut} isChecked={isSelected} />}
            onActivate={select}
            href={href}
            isDisabled={isDisabled}
            isDestructive={isDestructive}
            testID={testID}
        />
    )
}

export interface MenuCheckboxItemProps {
    label: string
    isChecked: boolean
    onToggle: () => void
    colorDot?: string
    isDisabled?: boolean
    testID?: string
}

/** A row that toggles and stays open, for filters and view options. */
function MenuCheckboxItem({
    label,
    isChecked,
    onToggle,
    colorDot,
    isDisabled = false,
    testID,
}: MenuCheckboxItemProps) {
    return (
        <MenuRow
            role="menuitemcheckbox"
            isChecked={isChecked}
            label={label}
            leading={<CheckboxSwatch colorDot={colorDot} isChecked={isChecked} />}
            trailing={<RowTrailing isChecked={isChecked} />}
            onActivate={onToggle}
            isDisabled={isDisabled}
            testID={testID}
        />
    )
}

function CheckboxSwatch({ colorDot, isChecked }: { colorDot?: string; isChecked: boolean }) {
    if (!colorDot) return null
    return (
        <View
            style={{
                width: 14,
                height: 14,
                borderRadius: 3,
                borderWidth: 2,
                backgroundColor: isChecked ? colorDot : 'transparent',
                borderColor: colorDot,
            }}
        />
    )
}

function RowLeading({
    icon: Icon,
    leading,
    colorDot,
}: {
    icon?: LucideIcon
    leading?: ReactNode
    colorDot?: string
}) {
    const { isSheet } = usePopoverContext()
    const mutedColor = useThemeColor('muted-foreground')
    if (leading) return <>{leading}</>
    if (Icon) return <Icon size={isSheet ? 18 : 16} color={mutedColor} />
    if (colorDot) {
        return (
            <View
                style={{
                    width: 12,
                    height: 12,
                    borderRadius: 6,
                    marginHorizontal: 2,
                    backgroundColor: colorDot,
                }}
            />
        )
    }
    return <View className={isSheet ? 'w-[18px]' : 'w-4'} />
}

function RowTrailing({ shortcut, isChecked }: { shortcut?: string; isChecked?: boolean }) {
    const primaryColor = useThemeColor('primary')
    if (isChecked) return <Check size={14} color={primaryColor} style={TRAILING_STYLE} />
    if (shortcut) {
        return (
            <Text className="text-xs text-muted-foreground pl-6" style={TRAILING_STYLE}>
                {shortcut}
            </Text>
        )
    }
    return null
}

const TRAILING_STYLE = { marginLeft: 'auto' } as const

interface MenuRowProps {
    role: 'menuitem' | 'menuitemcheckbox'
    isChecked?: boolean
    label: string
    leading?: ReactNode
    trailing?: ReactNode
    onActivate: () => void
    href?: string
    isDisabled: boolean
    isDestructive?: boolean
    testID?: string
    /** Extra keys the row answers (a submenu trigger opens on ArrowRight). */
    onKeyDown?: (event: React.KeyboardEvent) => void
    /** Pointer hover, for a submenu trigger's hover intent. */
    onHoverIn?: () => void
    onHoverOut?: () => void
    /** Open state of the submenu this row triggers. */
    isExpanded?: boolean
    rowRef?: React.Ref<View>
}

const ROW_CLASS = 'flex-row items-center gap-2 px-3 py-2 rounded-md mx-1'
const SHEET_ROW_CLASS = 'flex-row items-center gap-3 px-5 py-3'

/**
 * One row, in whichever presentation the surface has. On web a `div` (or an
 * `a` with `href`) with a menu role: RN's Pressable renders a bare div with
 * no key handling, and a menu built from those was mouse-only. Hover moves
 * focus so the pointer and the arrow keys agree on which row is current, and
 * mousedown never steals focus from a text editor the menu acts on.
 */
function MenuRow({
    role,
    isChecked,
    label,
    leading,
    trailing,
    onActivate,
    href,
    isDisabled,
    isDestructive = false,
    testID,
    onKeyDown,
    onHoverIn,
    onHoverOut,
    isExpanded,
    rowRef,
}: MenuRowProps) {
    const { isSheet } = usePopoverContext()
    const activate = () => {
        if (!isDisabled) onActivate()
    }
    const textClass = `${isSheet ? 'text-[15px]' : 'text-[14px]'} ${
        isDestructive ? 'text-danger' : 'text-foreground'
    }`
    const rowClass = `${isSheet ? SHEET_ROW_CLASS : ROW_CLASS} ${
        isDisabled ? 'opacity-40' : 'hover:bg-accent focus:bg-accent'
    } web:outline-none web:cursor-pointer`
    const content = (
        <>
            {leading}
            <Text className={textClass} numberOfLines={1}>
                {label}
            </Text>
            {trailing}
        </>
    )

    if (Platform.OS === 'web' && !isSheet) {
        const shared = {
            'aria-disabled': isDisabled || undefined,
            'aria-haspopup': isExpanded === undefined ? undefined : ('menu' as const),
            'aria-expanded': isExpanded,
            'data-testid': testID,
            className: rowClass,
            style: { display: 'flex', cursor: isDisabled ? 'default' : 'pointer' } as const,
            onMouseDown: (e: React.MouseEvent) => e.preventDefault(),
            onMouseEnter: (e: React.MouseEvent<HTMLElement>) => {
                e.currentTarget.focus()
                onHoverIn?.()
            },
            onMouseLeave: onHoverOut,
        }
        // Space as well as Enter: both activate per ARIA, and Space would
        // otherwise scroll the surface.
        const handleKeyDown = (e: React.KeyboardEvent) => {
            onKeyDown?.(e)
            if (e.defaultPrevented) return
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                activate()
            }
        }
        if (href) {
            return (
                <a
                    ref={rowRef as unknown as React.Ref<HTMLAnchorElement>}
                    href={href}
                    role="menuitem"
                    tabIndex={-1}
                    onKeyDown={handleKeyDown}
                    {...shared}
                    style={{ ...shared.style, textDecoration: 'none', color: 'inherit' }}
                    onClick={e => {
                        if (e.metaKey || e.ctrlKey || e.shiftKey) return
                        e.preventDefault()
                        activate()
                    }}
                >
                    {content}
                </a>
            )
        }
        // Two literal roles rather than one variable: the a11y lint reads the
        // role off the element to know the div is interactive.
        if (role === 'menuitemcheckbox') {
            return (
                <div
                    ref={rowRef as unknown as React.Ref<HTMLDivElement>}
                    role="menuitemcheckbox"
                    aria-checked={isChecked}
                    tabIndex={-1}
                    onKeyDown={handleKeyDown}
                    onClick={activate}
                    {...shared}
                >
                    {content}
                </div>
            )
        }
        return (
            <div
                ref={rowRef as unknown as React.Ref<HTMLDivElement>}
                role="menuitem"
                tabIndex={-1}
                onKeyDown={handleKeyDown}
                onClick={activate}
                {...shared}
            >
                {content}
            </div>
        )
    }

    return (
        <Pressable
            ref={rowRef}
            onPress={activate}
            disabled={isDisabled}
            testID={testID}
            accessibilityRole="menuitem"
            accessibilityState={{ disabled: isDisabled, checked: isChecked, expanded: isExpanded }}
            onHoverIn={onHoverIn}
            onHoverOut={onHoverOut}
            className={rowClass}
        >
            {content}
        </Pressable>
    )
}

// ── Structure ──

function MenuSection({
    label,
    children,
    testID,
}: {
    label: string
    children: ReactNode
    testID?: string
}) {
    const { isSheet } = usePopoverContext()
    return (
        <View>
            <Text
                testID={testID}
                className={`uppercase text-muted-foreground ${isSheet ? 'px-5 pt-3 pb-1' : 'px-3 py-1'}`}
                style={{ fontSize: 11, fontWeight: '600', letterSpacing: 0.5 }}
            >
                {label}
            </Text>
            {children}
        </View>
    )
}

function MenuSeparator() {
    return <View className="h-px bg-border my-1" />
}

/** A widget among rows: a color grid, a search field. Prefer a Popover when nothing is a row. */
function MenuCustom({ children, className }: { children: ReactNode; className?: string }) {
    return <View className={className ?? 'px-2 py-1'}>{children}</View>
}

// ── Submenu ──

// Time for the cursor to cross the gap between a trigger row and its submenu
// diagonally before the close timer fires. Below ~80ms feels snippy, above
// ~180ms sluggish.
const SUB_HOVER_CLOSE_MS = 120

export interface MenuSubProps {
    label: string
    icon?: LucideIcon
    leading?: ReactNode
    isDisabled?: boolean
    testID?: string
    children: ReactNode
}

/**
 * One level of nesting. Opens on hover, click, Enter, Space and ArrowRight;
 * closes on ArrowLeft, on hovering away, or when a sibling opens. In a sheet
 * the rows render inline under a section heading — a phone has no hover and
 * no room for a second column.
 */
function MenuSub({ label, icon, leading, isDisabled = false, testID, children }: MenuSubProps) {
    const { isSheet } = usePopoverContext()
    if (isSheet) {
        return (
            <MenuSection label={label} testID={testID}>
                {children}
            </MenuSection>
        )
    }
    return (
        <OpenableSub
            label={label}
            icon={icon}
            leading={leading}
            isDisabled={isDisabled}
            testID={testID}
        >
            {children}
        </OpenableSub>
    )
}

function OpenableSub({ label, icon, leading, isDisabled = false, testID, children }: MenuSubProps) {
    const id = useId()
    const { activeSubId, setActiveSubId } = useMenuContext()
    const isOpen = activeSubId === id
    const [rowRect, setRowRect] = useState<Rect | null>(null)
    const rowRef = useRef<View | null>(null)
    const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
    const mutedColor = useThemeColor('muted-foreground')

    const cancelClose = useCallback(() => {
        if (closeTimer.current != null) {
            clearTimeout(closeTimer.current)
            closeTimer.current = null
        }
    }, [])
    useEffect(() => cancelClose, [cancelClose])

    const open = useCallback(() => {
        if (isDisabled) return
        cancelClose()
        const node = rowRef.current
        if (Platform.OS === 'web' && node) {
            const rect = (node as unknown as HTMLElement).getBoundingClientRect()
            setRowRect({ x: rect.left, y: rect.top, width: rect.width, height: rect.height })
        } else if (node) {
            node.measureInWindow((x, y, width, height) => setRowRect({ x, y, width, height }))
        }
        setActiveSubId(id)
    }, [isDisabled, cancelClose, id, setActiveSubId])

    // Only clears the active id if it is OURS: a delayed close from this
    // submenu must not stomp a sibling that has just opened.
    const close = useCallback(() => {
        setActiveSubId(prev => (prev === id ? null : prev))
    }, [id, setActiveSubId])

    const scheduleClose = useCallback(() => {
        cancelClose()
        closeTimer.current = setTimeout(() => {
            close()
            closeTimer.current = null
        }, SUB_HOVER_CLOSE_MS)
    }, [cancelClose, close])

    const panelRef = useRef<View | null>(null)
    const focusFirstRow = () => {
        if (Platform.OS !== 'web') return
        // The panel mounts on the state change this follows; wait a frame.
        requestAnimationFrame(() => {
            const panel = panelRef.current as unknown as HTMLElement | null
            panel?.querySelector<HTMLElement>(ROW_SELECTOR)?.focus()
        })
    }

    return (
        <>
            <MenuRow
                role="menuitem"
                rowRef={rowRef}
                label={label}
                leading={<RowLeading icon={icon} leading={leading} />}
                trailing={<ChevronRight size={14} color={mutedColor} style={TRAILING_STYLE} />}
                onActivate={() => {
                    if (isOpen) close()
                    else {
                        open()
                        focusFirstRow()
                    }
                }}
                isDisabled={isDisabled}
                isExpanded={isOpen}
                testID={testID}
                onHoverIn={open}
                onHoverOut={scheduleClose}
                onKeyDown={e => {
                    if (e.key === 'ArrowRight') {
                        e.preventDefault()
                        open()
                        focusFirstRow()
                    }
                }}
            />
            <SubSurface
                panelRef={panelRef}
                isOpen={isOpen}
                rowRect={rowRect}
                onHoverIn={cancelClose}
                onHoverOut={scheduleClose}
                onCloseKey={() => {
                    close()
                    ;(rowRef.current as unknown as HTMLElement | null)?.focus?.()
                }}
            >
                {children}
            </SubSurface>
        </>
    )
}

function SubSurface({
    panelRef,
    isOpen,
    rowRect,
    onHoverIn,
    onHoverOut,
    onCloseKey,
    children,
}: {
    panelRef: React.MutableRefObject<View | null>
    isOpen: boolean
    rowRect: Rect | null
    onHoverIn: () => void
    onHoverOut: () => void
    onCloseKey: () => void
    children: ReactNode
}) {
    const { surfaceRect, subSlot } = usePopoverContext()
    const viewportWidth = useWindowSizeStore(s => s.width)
    const viewportHeight = useWindowSizeStore(s => s.height)
    const [size, setSize] = useState<Size | null>(null)

    // DOM listeners rather than props: RN's View types have neither mouse
    // nor key handlers, and only web needs them.
    const handlers = useRef({ onHoverIn, onHoverOut, onCloseKey })
    handlers.current = { onHoverIn, onHoverOut, onCloseKey }
    useEffect(() => {
        if (Platform.OS !== 'web' || !isOpen) return
        const node = panelRef.current as unknown as HTMLElement | null
        if (!node) return
        const enter = () => handlers.current.onHoverIn()
        const leave = () => handlers.current.onHoverOut()
        const key = (e: KeyboardEvent) => {
            if (e.key !== 'ArrowLeft') return
            e.preventDefault()
            e.stopPropagation()
            handlers.current.onCloseKey()
        }
        node.addEventListener('mouseenter', enter)
        node.addEventListener('mouseleave', leave)
        node.addEventListener('keydown', key)
        return () => {
            node.removeEventListener('mouseenter', enter)
            node.removeEventListener('mouseleave', leave)
            node.removeEventListener('keydown', key)
        }
    }, [isOpen, panelRef])

    if (!isOpen) return null

    // Window coordinates from the pure function, then converted into the
    // parent surface's box (its border is outside the padding box the slot
    // sits in, hence the 1px).
    const placedStyle =
        surfaceRect && rowRect
            ? (() => {
                  const { top, left } = placeSubmenu({
                      parent: surfaceRect,
                      row: rowRect,
                      size,
                      viewport: { width: viewportWidth, height: viewportHeight },
                  })
                  return {
                      top: top - surfaceRect.y - 1,
                      left: left - surfaceRect.x - 1,
                      opacity: size ? 1 : 0,
                  }
              })()
            : { opacity: 0 }

    const panel = (
        <View
            ref={panelRef}
            role="menu"
            onLayout={e => {
                const { width, height } = e.nativeEvent.layout
                setSize(prev =>
                    prev?.width === width && prev.height === height ? prev : { width, height }
                )
            }}
            className="absolute min-w-[200px] border border-border bg-background rounded-lg py-1"
            style={[SUB_SHADOW, placedStyle, { zIndex: 50, elevation: 24 }]}
        >
            {children}
        </View>
    )

    // On web the panel leaves the scroll region (which clips and traps it)
    // for the slot beside it, staying a DOM descendant of the surface so an
    // outside-press check still reads a submenu click as inside.
    if (Platform.OS !== 'web') return panel
    if (!subSlot) return null
    return createPortal(panel, subSlot)
}

const SUB_SHADOW =
    Platform.OS === 'web'
        ? ({ boxShadow: '0 4px 16px rgba(0,0,0,0.15)' } as object)
        : {
              shadowColor: '#000',
              shadowOffset: { width: 0, height: 4 },
              shadowOpacity: 0.15,
              shadowRadius: 12,
          }

const Menu = Object.assign(MenuRoot, {
    Item: MenuItem,
    CheckboxItem: MenuCheckboxItem,
    Section: MenuSection,
    Separator: MenuSeparator,
    Custom: MenuCustom,
    Sub: MenuSub,
})

export { Menu }
