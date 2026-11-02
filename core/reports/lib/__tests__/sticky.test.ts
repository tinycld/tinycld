import { describe, expect, it } from 'vitest'
import type { VisibleRow } from '../rows'
import { groupChain, stickyIndices } from '../sticky'
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
    row('g:b', 'group'),
    row('total', 'total'),
]

describe('sticky groups', () => {
    it('sticks every open group, at any depth', () => {
        expect(stickyIndices(ROWS)).toEqual([0, 1])
    })
    it('stacks the open groups that enclose the stuck one, outermost first', () => {
        expect(groupChain(ROWS, 1).map(r => r.id)).toEqual(['g:a', 'g:a1'])
        expect(groupChain(ROWS, 0).map(r => r.id)).toEqual(['g:a'])
    })
})
