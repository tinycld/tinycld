import type { SpecNode } from '../spec'

// Shape checks the report formula modules share: the builder reads only the
// shapes it writes, and keeps any other as it is (R17).

const MAX_PLACES = 6

/** The node sets no key outside `keys`; the builder reads only shapes it writes. */
export function onlyKeys(node: SpecNode, keys: readonly string[]): boolean {
    return Object.entries(node).every(([key, value]) => value === undefined || keys.includes(key))
}

/** The node carries a scale the engine accepts (0 to 6 places). */
export function hasScale(node: SpecNode): node is SpecNode & { scale: number } {
    return node.scale !== undefined && node.scale >= 0 && node.scale <= MAX_PLACES
}
