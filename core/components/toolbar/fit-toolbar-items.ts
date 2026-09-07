import type { Placement } from '@tinycld/core/ui/popover'
import type { LucideIcon } from 'lucide-react-native'
import type { ReactElement, ReactNode } from 'react'

/**
 * How a `custom` toolbar item behaves once the row runs out of room.
 *
 * - `{ onPress }` folds into the More menu as one row.
 * - `{ children }` folds in as a submenu holding those `Menu.*` rows.
 * - `'hide'` simply drops: for chrome that informs rather than acts (a presence
 *   stack, a word count) and has nothing to offer as a menu row.
 *
 * A custom item with no `overflow` at all is PINNED — it never leaves the row.
 */
export type ToolbarOverflow =
    | { label: string; icon?: LucideIcon; onPress: () => void; isDisabled?: boolean }
    | { label: string; icon?: LucideIcon; children: ReactNode; isDisabled?: boolean }
    | 'hide'

export type ToolbarItem =
    | {
          type: 'button'
          key: string
          icon: LucideIcon
          label: string
          onPress: () => void
          disabled?: boolean
          isActive?: boolean
      }
    | {
          type: 'menu'
          key: string
          icon: LucideIcon
          label: string
          children: ReactNode
          /** A trigger of the caller's own instead of the default icon button. */
          trigger?: ReactElement
          placement?: Placement
      }
    | { type: 'separator' }
    /** A flexible gap: what follows it sits at the row's right edge. */
    | { type: 'spacer' }
    | {
          type: 'custom'
          key: string
          element: ReactNode
          overflow?: ToolbarOverflow
          /**
           * The item may shrink to this width before anything folds — for a
           * title or breadcrumb that truncates. The fit charges this width,
           * and the slot shrinks no further.
           */
          minWidth?: number
      }

/** An item paired with the key it is measured and rendered under. */
export interface KeyedToolbarItem {
    key: string
    item: ToolbarItem
}

export interface ToolbarFit {
    /** What stays in the row, in the caller's order. */
    visible: KeyedToolbarItem[]
    /** What folded into the More menu, in the caller's order. */
    overflow: KeyedToolbarItem[]
    showMore: boolean
}

export interface FitToolbarInput {
    items: KeyedToolbarItem[]
    /** Intrinsic width of each measurable item; a missing key counts as 0. */
    widths: ReadonlyMap<string, number>
    /** Width of the row the items share with the More button. */
    availableWidth: number
    moreWidth: number
    /** The More button is rendered regardless, so it is always charged. */
    hasPermanentMore: boolean
    gap: number
}

/**
 * Sub-pixel slack. A row that shrink-wraps its content (a centred pill) is
 * exactly as wide as the items it holds, and layout rounding can report it a
 * fraction narrower than their sum — which must not fold an item that fits.
 */
const FIT_TOLERANCE = 1

export function itemKey(item: ToolbarItem, index: number): string {
    if (item.type === 'separator') return `sep-${index}`
    if (item.type === 'spacer') return `spacer-${index}`
    return item.key
}

export function keyToolbarItems(items: ToolbarItem[]): KeyedToolbarItem[] {
    return items.map((item, index) => ({ key: itemKey(item, index), item }))
}

/** Whether an item takes part in measuring (a spacer is a flexible gap, not a box). */
export function isMeasurable(item: ToolbarItem): boolean {
    return item.type !== 'spacer'
}

/** Whether the fit may move this item into the More menu to make room. */
export function isCollapsible(item: ToolbarItem): boolean {
    switch (item.type) {
        case 'button':
        case 'menu':
            return true
        case 'custom':
            return item.overflow !== undefined
        case 'separator':
        case 'spacer':
            return false
    }
}

/**
 * Decide which items stay in the row and which fold into the More menu.
 *
 * Drops collapsible items from the END of the list, one at a time, until the
 * rest fits; pinned items, separators and spacers are never candidates. The
 * first drop reserves room for the More button. Visible and overflow both keep
 * the caller's order, so a right-aligned group after a spacer folds before the
 * group at the left edge does.
 */
export function fitToolbarItems({
    items,
    widths,
    availableWidth,
    moreWidth,
    hasPermanentMore,
    gap,
}: FitToolbarInput): ToolbarFit {
    const collapsible = items
        .map((entry, index) => (isCollapsible(entry.item) ? index : -1))
        .filter(index => index >= 0)

    let last = candidate(items, new Set(), widths, moreWidth, hasPermanentMore, gap)
    for (let drop = 0; drop <= collapsible.length; drop++) {
        const dropped = new Set(collapsible.slice(collapsible.length - drop))
        last = candidate(items, dropped, widths, moreWidth, hasPermanentMore, gap)
        if (last.width <= availableWidth + FIT_TOLERANCE) break
    }
    return last.fit
}

function candidate(
    items: KeyedToolbarItem[],
    dropped: ReadonlySet<number>,
    widths: ReadonlyMap<string, number>,
    moreWidth: number,
    hasPermanentMore: boolean,
    gap: number
): { fit: ToolbarFit; width: number } {
    const visible = trimSeparators(items.filter((_, index) => !dropped.has(index)))
    const overflow = overflowList(items, dropped)
    const showMore = hasPermanentMore || dropped.size > 0

    const slots = visible.length + (showMore ? 1 : 0)
    const gaps = Math.max(0, slots - 1) * gap
    const itemWidth = visible.reduce((sum, entry) => sum + chargedWidth(entry, widths), 0)
    const width = itemWidth + gaps + (showMore ? moreWidth : 0)

    return { fit: { visible, overflow, showMore }, width }
}

/** A shrinkable item is charged at its floor, the rest at what they measured. */
function chargedWidth(entry: KeyedToolbarItem, widths: ReadonlyMap<string, number>): number {
    const measured = widths.get(entry.key) ?? 0
    if (entry.item.type === 'custom' && entry.item.minWidth !== undefined) {
        return Math.min(measured, entry.item.minWidth)
    }
    return measured
}

/**
 * The dropped items in order, with a separator wherever the row had one
 * between two of them — so folded groups still read as groups. Informational
 * items (`overflow: 'hide'`) have no row to offer and are left out.
 */
function overflowList(items: KeyedToolbarItem[], dropped: ReadonlySet<number>): KeyedToolbarItem[] {
    const out: KeyedToolbarItem[] = []
    let pendingSeparator: KeyedToolbarItem | null = null
    items.forEach((entry, index) => {
        if (entry.item.type === 'separator') {
            if (out.length > 0) pendingSeparator = entry
            return
        }
        if (!dropped.has(index)) return
        if (entry.item.type === 'custom' && entry.item.overflow === 'hide') return
        if (pendingSeparator) out.push(pendingSeparator)
        pendingSeparator = null
        out.push(entry)
    })
    return trimSeparators(out)
}

/**
 * Strip separators that no longer separate anything: leading, trailing, and
 * doubled up after their neighbours folded away. Spacers are transparent —
 * a separator followed only by a spacer is still trailing — but keep their
 * place, so a row that ends in a spacer still pushes the More button right.
 */
function trimSeparators(items: KeyedToolbarItem[]): KeyedToolbarItem[] {
    const first = items.findIndex(isReal)
    if (first < 0) return items.filter(isSpacer)
    let last = items.length - 1
    while (!isReal(items[last])) last--

    const middle: KeyedToolbarItem[] = []
    let separated = false
    for (const entry of items.slice(first, last + 1)) {
        if (entry.item.type === 'separator') {
            if (separated) continue
            separated = true
        } else if (isReal(entry)) {
            separated = false
        }
        middle.push(entry)
    }
    return [
        ...items.slice(0, first).filter(isSpacer),
        ...middle,
        ...items.slice(last + 1).filter(isSpacer),
    ]
}

function isReal(entry: KeyedToolbarItem): boolean {
    return entry.item.type !== 'separator' && entry.item.type !== 'spacer'
}

function isSpacer(entry: KeyedToolbarItem): boolean {
    return entry.item.type === 'spacer'
}
