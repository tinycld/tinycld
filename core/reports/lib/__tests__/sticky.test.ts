import { describe, expect, it } from 'vitest'
import type { VisibleRow } from '../rows'
import { groupChain, stickyIndices } from '../sticky'
import type { ResultRow } from '../types'

const row = (id: string, kind: ResultRow['kind'], extra: Partial<ResultRow> = {}): VisibleRow => ({
    row: { id, kind, depth: 0, label: id, cells: {}, ...extra },
    cells: {},
    isToggle: kind === 'group',
    isOpen: extra.expanded === true,
    isPending: false,
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
