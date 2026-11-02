import { describe, expect, it } from 'vitest'
import type { VisibleRow } from '../rows'
import { groupChain, stickyIndices, stickyPush } from '../sticky'
import type { ResultRow } from '../types'

const row = (id: string, kind: ResultRow['kind'], extra: Partial<ResultRow> = {}): VisibleRow => ({
    row: { id, kind, depth: 0, label: id, cells: {}, ...extra },
    cells: {},
    isToggle: kind === 'group',
    isOpen: extra.expanded === true,
})

const ROWS = [
    row('g:a', 'group', { expanded: true }),
    row('g:a1', 'group', { expanded: true, section: 'g:a', depth: 1 }),
    row('d:1', 'detail', { section: 'g:a1', depth: 2 }),
    row('s:a1', 'subtotal', { section: 'g:a1', depth: 1 }),
    row('g:a2', 'group', { section: 'g:a', depth: 1 }),
    row('s:a', 'subtotal', { section: 'g:a' }),
    row('g:b', 'group'),
    row('total', 'total'),
]
const ids = (index: number) => groupChain(ROWS, index).map(r => r.id)

describe('sticky groups', () => {
    it('sticks every row where a section starts or ends', () => {
        expect(stickyIndices(ROWS)).toEqual([0, 1, 3, 4, 5, 6, 7])
    })
    it('stacks the open groups that enclose the stuck one, outermost first', () => {
        expect(ids(1)).toEqual(['g:a', 'g:a1'])
        expect(ids(0)).toEqual(['g:a'])
        expect(ids(2)).toEqual(['g:a', 'g:a1'])
    })
    it('ends an open group at its subtotal or at the next section; nothing outside every group', () => {
        expect(ids(3)).toEqual(['g:a'])
        expect(ids(4)).toEqual(['g:a'])
        expect(ids(5)).toEqual([])
        expect(ids(6)).toEqual([])
        expect(ids(7)).toEqual([])
    })
})

describe('pushing the stuck stack by its own height', () => {
    // The next sticky row is at 400; FlashList pushes by the measured 44.
    const push = (offset: number, stackHeight: number) =>
        stickyPush({ offset, nextY: 400, stackHeight, itemHeight: 44 })
    it('adds nothing while the stack is the measured height', () => {
        for (const offset of [300, 356, 380, 400]) expect(push(offset, 44)).toBe(0)
    })
    it('starts a taller stack moving when the next row reaches its bottom', () => {
        expect(push(311, 88)).toBe(0)
        expect(push(312, 88)).toBe(0)
        // FlashList has not moved it yet: this moves it 20 up.
        expect(push(332, 88)).toBe(-20)
        // FlashList moves it 22 and this 44: 66 up, 22 still in view.
        expect(push(378, 88)).toBe(-44)
        // Gone when the next row reaches the top.
        expect(push(400, 88)).toBe(-44)
        expect(push(500, 88)).toBe(-44)
    })
    it('holds a shorter stack until the next row reaches it', () => {
        expect(push(370, 22)).toBe(14)
        expect(push(400, 22)).toBe(22)
    })
    it('adds nothing when no sticky row follows', () => {
        expect(stickyPush({ offset: 900, nextY: undefined, stackHeight: 88, itemHeight: 44 })).toBe(
            0
        )
    })
})
