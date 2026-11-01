import { describe, expect, it } from 'vitest'
import {
    EXPAND_ALL,
    expansionOptions,
    foldAllLabel,
    foldAllTarget,
    hasRows,
    mergePages,
    NO_EXPANSION,
    toggleGroup,
    visibleRows,
} from '../rows'
import type { ResultRow, RunResult } from '../types'

const group = (id: string, key: unknown[], extra: Partial<ResultRow> = {}): ResultRow => ({
    id,
    kind: 'group',
    depth: key.length - 1,
    label: id,
    groupKey: key,
    canExpand: true,
    cells: { cost: 100 },
    ...extra,
})
const detail = (id: string, section?: string): ResultRow => ({
    id,
    kind: 'detail',
    depth: 1,
    label: id,
    section,
    cells: { cost: 50 },
})
const subtotal = (id: string, section: string): ResultRow => ({
    id,
    kind: 'subtotal',
    depth: 0,
    label: `Total ${id}`,
    section,
    cells: { cost: 100 },
})
const total: ResultRow = {
    id: 'total',
    kind: 'total',
    depth: 0,
    label: 'Total',
    cells: { cost: 300 },
}

describe('visibleRows', () => {
    it('blanks a group row that its own subtotal row follows', () => {
        const rows = [
            group('g:a', ['a'], { expanded: true }),
            detail('d:1', 'g:a'),
            subtotal('s:a', 'g:a'),
            group('g:b', ['b']),
            total,
        ]
        const shown = visibleRows(rows)
        expect(shown[0].cells).toEqual({})
        expect(shown[0]).toMatchObject({ isToggle: true, isOpen: true })
        expect(shown[3].cells).toEqual({ cost: 100 })
        expect(shown[3]).toMatchObject({ isToggle: true, isOpen: false })
        expect(shown[4].isToggle).toBe(false)
    })

    it('does not offer a toggle on a group that cannot open', () => {
        expect(visibleRows([group('g:a', ['a'], { canExpand: false })])[0].isToggle).toBe(false)
    })
})

describe('toggleGroup', () => {
    const outer = group('g:a', ['a'], { expanded: true })
    const inner = group('g:a.x', ['a', 'x'], { section: 'g:a', expanded: true })
    const other = group('g:b', ['b'], { expanded: true })
    const rows = [outer, inner, detail('d:1', 'g:a.x'), other, total]

    it('opens a closed group by adding its key', () => {
        const closed = group('g:c', ['c'])
        expect(toggleGroup(NO_EXPANSION, closed, [closed])).toEqual({ all: false, keys: [['c']] })
    })

    it('closes a group and every group under it', () => {
        const next = toggleGroup({ all: false, keys: [['a'], ['a', 'x'], ['b']] }, outer, rows)
        expect(next).toEqual({ all: false, keys: [['b']] })
    })

    it('keeps every other open group when closing one under Expand all', () => {
        expect(toggleGroup(EXPAND_ALL, inner, rows)).toEqual({ all: false, keys: [['a'], ['b']] })
    })

    it('adds a key once', () => {
        const closed = group('g:c', ['c'])
        expect(toggleGroup({ all: false, keys: [['c']] }, closed, [closed])).toEqual({
            all: false,
            keys: [['c']],
        })
    })
})

describe('fold all', () => {
    it('collapses when any group is open, expands when none is', () => {
        expect(foldAllLabel([group('g:a', ['a'], { expanded: true }), group('g:b', ['b'])])).toBe(
            'Collapse all'
        )
        expect(foldAllTarget([group('g:a', ['a'], { expanded: true })])).toEqual(NO_EXPANSION)
        expect(foldAllLabel([group('g:a', ['a'])])).toBe('Expand all')
        expect(foldAllTarget([group('g:a', ['a'])])).toEqual(EXPAND_ALL)
    })

    it('has no button without a group that opens', () => {
        expect(foldAllLabel([detail('d:1'), total])).toBeNull()
        expect(foldAllLabel([group('g:a', ['a'], { canExpand: false })])).toBeNull()
    })

    it('turns an expansion into run options', () => {
        expect(expansionOptions(NO_EXPANSION)).toEqual({})
        expect(expansionOptions(EXPAND_ALL)).toEqual({ expandAll: true })
        expect(expansionOptions({ all: false, keys: [['a']] })).toEqual({ expand: [['a']] })
    })
})

describe('hasRows', () => {
    it('counts detail and group rows only', () => {
        expect(hasRows([total])).toBe(false)
        expect(hasRows([{ ...total, id: 'c:net', kind: 'computed' }, total])).toBe(false)
        expect(hasRows([detail('d:1'), total])).toBe(true)
        expect(hasRows([group('g:a', ['a']), total])).toBe(true)
    })
})

describe('mergePages', () => {
    const page = (
        ids: string[],
        totalCost: number,
        notices: RunResult['notices'] = []
    ): RunResult => ({
        columns: [{ id: 'cost', label: 'Cost', type: 'money' }],
        rows: [...ids.map(id => detail(id)), { ...total, cells: { cost: totalCost } }],
        notices,
        topics: ['collection/zoo_feedings'],
        page: { size: ids.length, next: 'next' },
        params: [],
    })

    it('keeps every page’s detail rows and the newest total', () => {
        const merged = mergePages([page(['d:1', 'd:2'], 300), page(['d:3'], 350)])
        expect(merged.rows.map(r => r.id)).toEqual(['d:1', 'd:2', 'd:3', 'total'])
        expect(merged.rows[3].cells.cost).toBe(350)
    })

    it('keeps one notice of each code', () => {
        const cap = { code: 'rowCap', message: 'The report shows its first 5000 rows.' }
        expect(mergePages([page(['d:1'], 1, [cap]), page(['d:2'], 2, [cap])]).notices).toEqual([
            cap,
        ])
    })
})
