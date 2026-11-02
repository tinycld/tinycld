import type { VisibleRow } from './rows'
import type { ResultRow } from './types'

// FlashList keeps one sticky item at a time: the last sticky index above the
// top, pushed up by the next (R26). Every row where a section starts or ends is
// a sticky index (a group row, open or closed, a subtotal, a computed row, the
// total), so the stuck item always says which open groups enclose the rows in
// view: an open group stays stuck only through its own section. The stuck copy
// draws those groups stacked, outermost first; it draws nothing when there are
// none.

const BOUNDARIES: ReadonlySet<ResultRow['kind']> = new Set([
    'group',
    'subtotal',
    'computed',
    'total',
])

export function stickyIndices(rows: readonly VisibleRow[]): number[] {
    return rows.flatMap((item, index) => (BOUNDARIES.has(item.row.kind) ? [index] : []))
}

/**
 * The open groups the rows from this one on are inside, outermost first: the
 * groups that enclose it, and the row itself when it is an open group. A
 * subtotal ends its group, so its rows are inside that group's parents only.
 */
export function groupChain(rows: readonly VisibleRow[], index: number): ResultRow[] {
    const byId = new Map(rows.map(item => [item.row.id, item]))
    const item = rows[index]
    if (!item || item.row.kind === 'total' || item.row.kind === 'computed') return []
    const parentOf = (row: ResultRow) => (row.section ? byId.get(row.section) : undefined)
    const isOwn = item.row.kind === 'group' && item.isOpen
    let current = isOwn ? item : parentOf(item.row)
    if (item.row.kind === 'subtotal') current = current && parentOf(current.row)
    const chain: ResultRow[] = []
    while (current) {
        if (current.row.kind === 'group' && current.isOpen) chain.unshift(current.row)
        current = parentOf(current.row)
    }
    return chain
}

/**
 * The extra move of the stuck copy as the next sticky row comes up under it.
 * FlashList pushes the copy by the height it measured for the stuck row; the
 * copy is as tall as its stack, so this adds the difference: the copy starts to
 * move when the next row reaches the copy's real bottom and is gone when that
 * row reaches the top. All in the list's content offsets.
 */
export function stickyPush(input: {
    offset: number
    nextY: number | undefined
    stackHeight: number
    itemHeight: number
}): number {
    const { offset, nextY, stackHeight, itemHeight } = input
    if (nextY === undefined) return 0
    const pushed = (height: number) => Math.min(Math.max(offset - (nextY - height), 0), height)
    return pushed(itemHeight) - pushed(stackHeight)
}
