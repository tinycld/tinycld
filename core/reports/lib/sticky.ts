import type { VisibleRow } from './rows'
import type { ResultRow } from './types'

// FlashList keeps one sticky item at a time, pushed up by the next (R26). Every
// open group row, at any depth, is a sticky index, so the stuck item is the
// innermost open group above the top; it is drawn with the open groups that
// enclose it stacked above it, so nested levels read as a stack.

export function stickyIndices(rows: readonly VisibleRow[]): number[] {
    return rows.flatMap((item, index) => (item.row.kind === 'group' && item.isOpen ? [index] : []))
}

export function groupChain(rows: readonly VisibleRow[], index: number): ResultRow[] {
    const byId = new Map(rows.map(item => [item.row.id, item.row]))
    const chain: ResultRow[] = []
    let current: ResultRow | undefined = rows[index]?.row
    while (current) {
        chain.unshift(current)
        current = current.section ? byId.get(current.section) : undefined
    }
    return chain
}
