import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Menu } from '@tinycld/core/ui/menu'
import type { Placement } from '@tinycld/core/ui/popover'
import { EllipsisVertical } from 'lucide-react-native'
import type { ReactElement, ReactNode } from 'react'
import { forwardRef, useState } from 'react'
import type { LayoutChangeEvent } from 'react-native'
import { Platform, Pressable, View } from 'react-native'
import { ToolbarIconButton } from './ToolbarIconButton'
import { ToolbarSeparator } from './ToolbarSeparator'
import {
    fitToolbarItems,
    isMeasurable,
    type KeyedToolbarItem,
    keyToolbarItems,
    type ToolbarItem,
    type ToolbarOverflow,
} from './toolbar/fit-toolbar-items'

export type { ToolbarItem, ToolbarOverflow } from './toolbar/fit-toolbar-items'

interface ResponsiveToolbarProps {
    items: ToolbarItem[]
    /** Pinned at the right edge; never folds into the More menu. */
    rightItems?: ToolbarItem[]
    /**
     * `Menu.*` rows the More menu always holds. Setting this keeps the More
     * button on screen at every width; whatever overflows lands above these
     * rows, behind a separator.
     */
    moreMenu?: ReactNode
    /**
     * A More button of the caller's own, so it can match the row's other
     * buttons. Cloned by the menu with its `onPress` and ref, so it must be a
     * forwardRef component around a Pressable, and it carries the caller's
     * accessibility label. The default button also keeps DOM focus where it
     * is on web (see FOCUS_GUARD); a toolbar over a text editor that supplies
     * its own trigger must do the same.
     */
    moreTrigger?: ReactElement
    /** Accessible name of the default More button, and the title of the sheet the menu opens on a phone. */
    moreLabel?: string
    /** Where the More menu opens; a toolbar at the bottom of a screen passes a `top-*` placement. */
    morePlacement?: Placement
    height?: number
    /** Space between neighbouring slots. */
    gap?: number
    /** Container overrides — padding, background. The row layout itself is fixed. */
    className?: string
}

const DEFAULT_GAP = 2

/**
 * Keep DOM focus where it is when this control is pressed.
 *
 * A toolbar acts on something else — usually a text editor — and on web the
 * browser blurs whatever holds focus on mousedown, collapsing that editor's
 * selection before the press is even handled. Preventing the default keeps the
 * selection, so an action still has something to apply to.
 *
 * Web-only: native has no DOM focus model, and `Platform.select` here would
 * hand React Native an unknown prop.
 */
const FOCUS_GUARD =
    Platform.OS === 'web'
        ? { onMouseDown: (e: { preventDefault: () => void }) => e.preventDefault() }
        : {}

interface Measurements {
    /** Width of the row the items share with the More button; null until laid out. */
    row: number | null
    /** Width of the More button's slot; null until it has rendered once. */
    more: number | null
    /** Intrinsic width of every item seen so far, by key. */
    items: ReadonlyMap<string, number>
}

const UNMEASURED: Measurements = { row: null, more: null, items: new Map() }

/**
 * A row of actions that folds what does not fit into a More menu, on web and
 * native alike.
 *
 * Every slot reports its width through `onLayout` — Yoga on native, a
 * ResizeObserver under react-native-web — so an item is measured once when it
 * first appears and again only if its own size changes. Until the row and every
 * item have a width the toolbar renders everything, invisible and inert, so the
 * first visible paint is already the fitted one. The fit itself is arithmetic
 * over those cached widths (`fitToolbarItems`), derived during render: there is
 * no measuring state to get stuck in, only keys that do or do not have a width.
 *
 * An item that is currently folded away is not on screen, so a size change
 * there is picked up the next time it renders.
 *
 * Hosts: the container is an RN `View`, which does not shrink inside a flex-row
 * parent. Put the toolbar in a column, or give it `flex-1 min-w-0` in a row.
 */
export function ResponsiveToolbar({
    items,
    rightItems,
    moreMenu,
    moreTrigger,
    moreLabel = 'More actions',
    morePlacement = 'bottom-end',
    height = 44,
    gap = DEFAULT_GAP,
    className,
}: ResponsiveToolbarProps) {
    const [measured, setMeasured] = useState(UNMEASURED)
    const keyed = keyToolbarItems(items)
    const hasPermanentMore = moreMenu !== undefined && moreMenu !== null

    const isMeasured =
        measured.row !== null &&
        measured.more !== null &&
        keyed.every(entry => !isMeasurable(entry.item) || measured.items.has(entry.key))

    const fit =
        isMeasured && measured.row !== null && measured.more !== null
            ? fitToolbarItems({
                  items: keyed,
                  widths: measured.items,
                  availableWidth: measured.row,
                  moreWidth: measured.more,
                  hasPermanentMore,
                  gap,
              })
            : null

    const onRowLayout = (event: LayoutChangeEvent) => {
        const { width } = event.nativeEvent.layout
        setMeasured(prev => (prev.row === width ? prev : { ...prev, row: width }))
    }
    const onMoreLayout = (event: LayoutChangeEvent) => {
        const { width } = event.nativeEvent.layout
        setMeasured(prev => (prev.more === width ? prev : { ...prev, more: width }))
    }
    const onItemLayout = (key: string, width: number) => {
        setMeasured(prev => {
            if (prev.items.get(key) === width) return prev
            const next = new Map(prev.items)
            next.set(key, width)
            return { ...prev, items: next }
        })
    }

    const shown = fit ? fit.visible : keyed
    const showMore = fit ? fit.showMore : true

    return (
        <View
            testID="toolbar"
            className={className ?? 'px-2'}
            // `overflow: visible` on every box between a button and the page,
            // so a hover tooltip drawn above the row is not clipped —
            // react-native-web's View hides overflow by default.
            style={{ height, flexDirection: 'row', alignItems: 'center', overflow: 'visible' }}
        >
            <View
                testID="toolbar-row"
                onLayout={onRowLayout}
                style={{
                    flex: 1,
                    minWidth: 0,
                    flexDirection: 'row',
                    alignItems: 'center',
                    gap,
                    overflow: 'visible',
                    opacity: fit ? 1 : 0,
                    pointerEvents: fit ? 'auto' : 'none',
                }}
            >
                {shown.map(entry => (
                    <Slot
                        key={entry.key}
                        entry={entry}
                        isFitted={fit !== null}
                        onWidth={onItemLayout}
                    />
                ))}
                <MoreSlot
                    isVisible={showMore}
                    overflow={fit ? fit.overflow : []}
                    moreMenu={moreMenu}
                    trigger={moreTrigger}
                    label={moreLabel}
                    placement={morePlacement}
                    onLayout={onMoreLayout}
                />
            </View>
            <RightCluster items={rightItems} gap={gap} />
        </View>
    )
}

interface SlotProps {
    entry: KeyedToolbarItem
    isFitted: boolean
    onWidth: (key: string, width: number) => void
}

/**
 * One measured box around an item. A pinned item may shrink once the row is
 * fitted — a title with `numberOfLines={1}` truncates rather than pushing the
 * More button off the edge — but while measuring every slot keeps its
 * intrinsic width, or there would be nothing true to measure.
 */
function Slot({ entry, isFitted, onWidth }: SlotProps) {
    const { key, item } = entry
    if (item.type === 'spacer') {
        return <View testID={`toolbar-item-${key}`} style={{ flex: 1 }} />
    }
    const isPinned = item.type === 'custom' && item.overflow === undefined
    const floor = item.type === 'custom' ? item.minWidth : undefined
    const canShrink = isFitted && (isPinned || floor !== undefined)
    return (
        <View
            testID={`toolbar-item-${key}`}
            onLayout={event => onWidth(key, event.nativeEvent.layout.width)}
            style={{
                flexDirection: 'row',
                alignItems: 'center',
                overflow: 'visible',
                flexShrink: canShrink ? 1 : 0,
                minWidth: canShrink ? (floor ?? 0) : undefined,
            }}
        >
            <RenderItem item={item} />
        </View>
    )
}

function RightCluster({ items, gap }: { items?: ToolbarItem[]; gap: number }) {
    if (!items || items.length === 0) return null
    return (
        <View
            testID="toolbar-right"
            className="pl-2"
            style={{
                flexDirection: 'row',
                alignItems: 'center',
                gap,
                flexShrink: 0,
                overflow: 'visible',
            }}
        >
            {keyToolbarItems(items).map(entry => (
                <View
                    key={entry.key}
                    style={{ flexDirection: 'row', alignItems: 'center', overflow: 'visible' }}
                >
                    <RenderItem item={entry.item} />
                </View>
            ))}
        </View>
    )
}

// ── More menu ──

interface MoreSlotProps {
    isVisible: boolean
    overflow: KeyedToolbarItem[]
    moreMenu: ReactNode
    trigger?: ReactElement
    label: string
    placement: Placement
    onLayout: (event: LayoutChangeEvent) => void
}

function MoreSlot({
    isVisible,
    overflow,
    moreMenu,
    trigger,
    label,
    placement,
    onLayout,
}: MoreSlotProps) {
    if (!isVisible) return null
    const hasBoth = overflow.length > 0 && moreMenu !== undefined && moreMenu !== null
    return (
        <View
            testID="toolbar-more"
            onLayout={onLayout}
            style={{ flexShrink: 0, overflow: 'visible' }}
        >
            <Menu
                trigger={trigger ?? <MoreButton label={label} />}
                placement={placement}
                title={label}
            >
                {overflow.map(entry => (
                    <OverflowItem key={entry.key} item={entry.item} />
                ))}
                {hasBoth ? <Menu.Separator /> : null}
                {moreMenu}
            </Menu>
        </View>
    )
}

const MoreButton = forwardRef<View, { label: string; onPress?: () => void }>(function MoreButton(
    { label, onPress },
    ref
) {
    const mutedColor = useThemeColor('muted-foreground')
    return (
        <Pressable
            ref={ref}
            onPress={onPress}
            className="p-2 rounded-full"
            accessibilityRole="button"
            accessibilityLabel={label}
            testID="toolbar-more-button"
            // The same guard every toolbar button carries, and for
            // the same reason: on web a press moves DOM focus off
            // whatever held it, and when that is a rich-text editor
            // the browser collapses its selection first. It matters
            // MORE here than on the buttons it replaces: at narrow
            // widths every format button folds into this menu, so
            // without it the whole toolbar stops working exactly
            // where there is least room to notice.
            {...FOCUS_GUARD}
        >
            <EllipsisVertical size={18} color={mutedColor} />
        </Pressable>
    )
})

function OverflowItem({ item }: { item: ToolbarItem }) {
    switch (item.type) {
        case 'button':
            return (
                <Menu.Item
                    label={item.label}
                    icon={item.icon}
                    onSelect={item.onPress}
                    isDisabled={item.disabled}
                    isSelected={item.isActive}
                />
            )
        case 'menu':
            return (
                <Menu.Sub label={item.label} icon={item.icon}>
                    {item.children}
                </Menu.Sub>
            )
        case 'separator':
            return <Menu.Separator />
        case 'custom':
            return <CustomOverflowItem overflow={item.overflow} />
        case 'spacer':
            return null
    }
}

function CustomOverflowItem({ overflow }: { overflow: ToolbarOverflow | undefined }) {
    if (overflow === undefined || overflow === 'hide') return null
    if ('children' in overflow) {
        return (
            <Menu.Sub label={overflow.label} icon={overflow.icon} isDisabled={overflow.isDisabled}>
                {overflow.children}
            </Menu.Sub>
        )
    }
    return (
        <Menu.Item
            label={overflow.label}
            icon={overflow.icon}
            onSelect={overflow.onPress}
            isDisabled={overflow.isDisabled}
        />
    )
}

// ── Inline items ──

function RenderItem({ item }: { item: ToolbarItem }) {
    const activeColor = useThemeColor('primary')
    switch (item.type) {
        case 'button':
            return (
                <ToolbarIconButton
                    icon={item.icon}
                    label={item.label}
                    onPress={item.onPress}
                    disabled={item.disabled}
                    color={item.isActive ? activeColor : undefined}
                />
            )
        case 'menu':
            return (
                <Menu
                    trigger={
                        item.trigger ?? <ToolbarIconButton icon={item.icon} label={item.label} />
                    }
                    placement={item.placement}
                    title={item.label}
                >
                    {item.children}
                </Menu>
            )
        case 'separator':
            return <ToolbarSeparator />
        case 'custom':
            return <>{item.element}</>
        case 'spacer':
            return null
    }
}
