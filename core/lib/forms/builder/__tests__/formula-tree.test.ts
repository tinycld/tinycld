import { describe, expect, it } from 'vitest'
import type { ComputeExpr } from '../../expression'
import { childrenOf, withChildren } from '../formula'
import { replaceTreeAt, treeAt } from '../formula-tree'

const tree = { childrenOf, withChildren }
const expr: ComputeExpr = { add: [1, { mul: [{ var: 'a' }, 2] }] }

describe('the formula tree', () => {
    it('reads a part by its path', () => {
        expect(treeAt(tree, expr, [])).toBe(expr)
        expect(treeAt(tree, expr, [1, 0])).toEqual({ var: 'a' })
        expect(treeAt(tree, expr, [5])).toBeUndefined()
    })
    it('replaces a part by its path and leaves the rest', () => {
        expect(replaceTreeAt(tree, expr, [1, 1], 3)).toEqual({
            add: [1, { mul: [{ var: 'a' }, 3] }],
        })
        expect(replaceTreeAt(tree, expr, [9], 3)).toEqual(expr)
        expect(replaceTreeAt(tree, expr, [], 7)).toBe(7)
    })
})
