import { generateKeyBetween } from 'fractional-indexing'

/**
 * Setup steps use the same fractional-indexing keys as boards
 * (boards/tinycld/boards/lib/rank.ts), so a package can place a step between
 * two others without renumbering anything. A key is valid when the library
 * accepts it as a lower bound.
 */
export function isValidOrderKey(key: string): boolean {
    try {
        generateKeyBetween(key, null)
        return true
    } catch {
        return false
    }
}

interface Ordered {
    id: string
    order: string | null
}

// Plain < on purpose: fractional-indexing keys are ordered by code unit, and
// localeCompare would put 'Zz' after 'a0'.
function compareText(a: string, b: string): number {
    if (a < b) return -1
    if (a > b) return 1
    return 0
}

export function compareStepOrder(a: Ordered, b: Ordered): number {
    if (a.order !== b.order) {
        if (a.order === null) return 1
        if (b.order === null) return -1
        return compareText(a.order, b.order)
    }
    return compareText(a.id, b.id)
}
