import type { FormulaPath } from './formula'

/**
 * How a formula tree holds its parts. The formula editor walks any tree
 * through these two functions: the forms engine's ComputeExpr, or another
 * engine's own tree (the report builder's).
 */
export interface FormulaTree<E> {
    childrenOf(expr: E): E[]
    withChildren(expr: E, parts: E[]): E
}

export function treeAt<E>(tree: FormulaTree<E>, expr: E, path: FormulaPath): E | undefined {
    if (path.length === 0) return expr
    const [head, ...rest] = path
    const child = head === undefined ? undefined : tree.childrenOf(expr)[head]
    return child === undefined ? undefined : treeAt(tree, child, rest)
}

/** The tree with the part at `path` replaced; a path that names no part changes nothing. */
export function replaceTreeAt<E>(tree: FormulaTree<E>, expr: E, path: FormulaPath, next: E): E {
    if (path.length === 0) return next
    const [head, ...rest] = path
    const parts = tree.childrenOf(expr)
    const child = head === undefined ? undefined : parts[head]
    if (child === undefined) return expr
    return tree.withChildren(
        expr,
        parts.map((p, i) => (i === head ? replaceTreeAt(tree, child, rest, next) : p))
    )
}
