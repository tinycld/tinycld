import { describe, expect, it } from 'vitest'
import {
    DEFAULT_EXPANSION,
    EXPAND_ALL,
    expansionOptions,
    foldAllLabel,
    foldAllTarget,
    hasRows,
    holdWholeReportRows,
    mergePages,
    NO_EXPANSION,
    toggleGroup,
    visibleRows,
} from '../rows'
import type { Notice, ResultRow, RunResult } from '../types'

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

    // A paged run: page 1 holds an open group and its first details; its subtotal
    // row comes on page 2. The spec says the level has subtotals, so the group row
    // shows no figures on page 1 either, and nothing changes when page 2 lands.
    it('blanks an open group of a level with subtotals before its subtotal page arrives', () => {
        const spec = { groups: [{ subtotals: true }] }
        const pageOne = [group('g:a', ['a'], { expanded: true }), detail('d:1', 'g:a')]
        expect(visibleRows(pageOne, spec)[0].cells).toEqual({})
        const merged = [...pageOne, subtotal('s:a', 'g:a'), total]
        expect(visibleRows(merged, spec)[0].cells).toEqual({})
        // Without the spec, page 1 alone cannot know the subtotal comes.
        expect(visibleRows(pageOne)[0].cells).toEqual({ cost: 100 })
    })

    it('keeps the figures of a group that no subtotal row ends', () => {
        const closed = group('g:a', ['a'])
        const open = group('g:b', ['b'], { expanded: true })
        // A closed group of a level with subtotals, and an open one of a level without.
        expect(visibleRows([closed], { groups: [{ subtotals: true }] })[0].cells).toEqual({
            cost: 100,
        })
        expect(visibleRows([open], { groups: [{}] })[0].cells).toEqual({ cost: 100 })
        // An inner level without subtotals under an outer level with them.
        const inner = group('g:b|x', ['b', 'x'], { expanded: true })
        const spec = { groups: [{ subtotals: true }, {}] }
        expect(visibleRows([open, inner], spec).map(r => r.cells)).toEqual([{}, { cost: 100 }])
    })

    it('blanks a layout section and a closed group of a subtotalled level in a layout report', () => {
        const spec = { groups: [{}, { subtotals: true }], layout: [{ section: 'Income' }] }
        const section = group('g:income', ['income'])
        const inner = group('g:income|x', ['income', 'x'])
        const shown = visibleRows([section, inner], spec)
        expect(shown.map(r => r.cells)).toEqual([{}, {}])
    })

    it('does not offer a toggle on a group that cannot open', () => {
        const shown = visibleRows([group('g:a', ['a'], { canExpand: false })])
        expect(shown[0].isToggle).toBe(false)
    })

    it('leaves out the cell a detail row’s label repeats, a child row’s too', () => {
        const parent: ResultRow = {
            ...detail('d:1'),
            labelColumn: 'name',
            cells: { name: 'Team A', customer: null, cost: 50 },
        }
        const child: ResultRow = {
            id: 'd:1/s1',
            kind: 'detail',
            depth: 2,
            label: 'First buyer',
            parent: 'd:1',
            labelColumn: 'customer',
            cells: { customer: 'First buyer', amount: 100, 'amount@currency': 'USD' },
        }
        const shown = visibleRows([parent, child])
        expect(shown[0].cells).toEqual({ customer: null, cost: 50 })
        expect(shown[1].cells).toEqual({ amount: 100, 'amount@currency': 'USD' })
    })

    // E2: the engine sends no cell for a column blank on detail rows; the row draws
    // what it was sent, so that column's slot stays empty and its group keeps the value.
    it('keeps a detail row without the cell of a column blank on detail rows', () => {
        const opened = group('g:a', ['a'], { expanded: true, cells: { no: 1000, cost: 100 } })
        const line: ResultRow = {
            ...detail('d:1', 'g:a'),
            labelColumn: 'name',
            cells: { name: 'Team A', cost: 50 },
        }
        const shown = visibleRows([opened, line])
        expect(shown[0].cells).toEqual({ no: 1000, cost: 100 })
        expect(shown[1].cells).toEqual({ cost: 50 })
        expect(shown[1].cells).not.toHaveProperty('no')
    })

    it('leaves out the column a detail label of exactly {col} repeats', () => {
        // The engine names it (detail.label {col: 'cost'}); any other label names none.
        const byCost: ResultRow = {
            ...detail('d:1'),
            label: '50',
            labelColumn: 'cost',
            cells: { name: 'Team A', cost: 50 },
        }
        const byFormula: ResultRow = { ...byCost, labelColumn: undefined }
        expect(visibleRows([byCost])[0].cells).toEqual({ name: 'Team A' })
        expect(visibleRows([byFormula])[0].cells).toEqual({ name: 'Team A', cost: 50 })
    })

    it('leaves out a blank labelling cell too (the label reads "(none)")', () => {
        const child: ResultRow = {
            id: 'd:1/s1',
            kind: 'detail',
            depth: 2,
            label: '(none)',
            parent: 'd:1',
            labelColumn: 'customer',
            cells: { customer: null, amount: 100 },
        }
        expect(visibleRows([child])[0].cells).toEqual({ amount: 100 })
    })
})

describe('expansion state', () => {
    it('sends no expand on the first run, so groups marked open open', () => {
        expect(expansionOptions(DEFAULT_EXPANSION)).toEqual({})
    })

    it('collapse sends an empty expand, so groups marked open stay closed', () => {
        expect(foldAllTarget([group('g:a', ['a'], { expanded: true })])).toEqual(NO_EXPANSION)
        expect(expansionOptions(NO_EXPANSION)).toEqual({ expand: [] })
    })

    it('a press under the default state keeps the groups the result shows open', () => {
        const rows = [group('g:a', ['a'], { expanded: true }), group('g:b', ['b'])]
        expect(toggleGroup(DEFAULT_EXPANSION, rows[1] as ResultRow, rows)).toEqual({
            all: false,
            keys: [['a'], ['b']],
        })
        expect(toggleGroup(DEFAULT_EXPANSION, rows[0] as ResultRow, rows)).toEqual({
            all: false,
            keys: [],
        })
    })
})

describe('child rows', () => {
    it('drop exactly the cell their label repeats, and a label that is no cell drops none', () => {
        const child: ResultRow = {
            id: 'd:1/x',
            kind: 'detail',
            depth: 2,
            label: 'Ann',
            parent: 'd:1',
            labelColumn: 'customer',
            cells: { customer: 'Ann', amount: 5 },
        }
        const own: ResultRow = { ...child, labelColumn: undefined }
        expect(visibleRows([child])[0]?.cells).toEqual({ amount: 5 })
        expect(visibleRows([own])[0]?.cells).toEqual({ customer: 'Ann', amount: 5 })
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
        expect(expansionOptions(NO_EXPANSION)).toEqual({ expand: [] })
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

    // Each name holds ". ", so a message cut into sentences at ". " would lose
    // part of a sentence: only the server's parts are sentences.
    const having = (team: string) =>
        `The having of the "No. ${team}" group reads a looked-up total that mixes currencies, so that total counts as unknown there.`
    const blank = 'A looked-up total mixes currencies, so it shows no value.'
    const mixed = (parts: string[], more: Partial<Notice> = {}): Notice => ({
        code: 'mixedCurrency',
        message: parts.join(' '),
        ...(parts.length > 1 ? { parts } : {}),
        ...more,
    })

    it('joins the columns, conditions and distinct sentences one code names on each page', () => {
        const first = mixed([having('1')], { conditions: ['groups.0.having'] })
        const second = mixed([blank, having('2')], {
            columns: ['balance'],
            conditions: ['groups.1.having'],
        })
        const third: Notice = { ...second, columns: ['opening', 'balance'] }
        const cap = { code: 'rowCap', message: 'The report shows its first 5000 rows.' }
        const merged = mergePages([
            page(['d:1'], 1, [first]),
            page(['d:2'], 2, [second, cap]),
            page(['d:3'], 3, [third]),
        ])
        expect(merged.notices).toEqual([
            mixed([having('1'), blank, having('2')], {
                columns: ['balance', 'opening'],
                conditions: ['groups.0.having', 'groups.1.having'],
            }),
            cap,
        ])
    })

    it('shows a sentence two pages’ messages share once', () => {
        // Each page names the blank column and a having of its own: the blank
        // sentence is in both messages, which differ as a whole.
        const first = mixed([blank, having('1')], {
            columns: ['balance'],
            conditions: ['groups.0.having'],
        })
        const second = mixed([blank, having('2')], {
            columns: ['balance'],
            conditions: ['groups.1.having'],
        })
        const merged = mergePages([page(['d:1'], 1, [first]), page(['d:2'], 2, [second])])
        expect(merged.notices).toEqual([
            mixed([blank, having('1'), having('2')], {
                columns: ['balance'],
                conditions: ['groups.0.having', 'groups.1.having'],
            }),
        ])
    })

    it('leaves a notice no later page repeats as the server sent it', () => {
        const only: Notice = { code: 'mixedCurrency', message: 'One.', columns: ['cost'] }
        expect(mergePages([page(['d:1'], 1, [only]), page(['d:2'], 2)]).notices).toEqual([only])
    })

    it('takes computed and total rows from the newest page only', () => {
        const withFigures = (ids: string[], net: number, cost: number): RunResult => ({
            ...page(ids, cost),
            rows: [
                ...ids.map(id => detail(id)),
                { id: 'c:net', kind: 'computed', depth: 0, label: 'Net', cells: { cost: net } },
                { ...total, cells: { cost } },
            ],
        })
        const merged = mergePages([
            withFigures(['d:1', 'd:2'], 10, 300),
            withFigures(['d:3'], 20, 350),
        ])
        expect(merged.rows.map(r => r.id)).toEqual(['d:1', 'd:2', 'd:3', 'c:net', 'total'])
        expect(merged.rows[3].cells.cost).toBe(20)
        expect(merged.rows[4].cells.cost).toBe(350)
    })

    it('keeps a grouped run’s rows in the order the server sent them', () => {
        const grouped: RunResult = {
            ...page([], 300),
            rows: [
                group('g:paid', ['paid'], { expanded: false }),
                group('g:open', ['open'], { expanded: true }),
                detail('d:1', 'g:open'),
                detail('d:2', 'g:open'),
                subtotal('s:open', 'g:open'),
                total,
            ],
            page: null,
        }
        expect(mergePages([grouped]).rows.map(r => r.id)).toEqual([
            'g:paid',
            'g:open',
            'd:1',
            'd:2',
            's:open',
            'total',
        ])
    })

    it('returns an empty result for no pages', () => {
        expect(mergePages([])).toMatchObject({ rows: [], notices: [], page: null })
    })

    const opening = (key: string, section: string): ResultRow => ({
        id: `opening:["${key}"]`,
        kind: 'opening',
        depth: 1,
        label: 'Before the period',
        section,
        cells: { cost: 10 },
    })

    // R9/R10: a grouped page carries the sections between its detail rows.
    it('appends the next grouped page’s sections after the first page’s rows', () => {
        const first: RunResult = {
            ...page([], 0),
            rows: [
                group('g:a', ['a'], { expanded: true }),
                opening('a', 'g:a'),
                detail('d:1', 'g:a'),
                detail('d:2', 'g:a'),
                { ...total, cells: { cost: 300 } },
            ],
            page: { size: 2, next: 'c1' },
        }
        const second: RunResult = {
            ...page([], 0),
            rows: [
                detail('d:3', 'g:a'),
                subtotal('s:a', 'g:a'),
                group('g:b', ['b'], { expanded: true }),
                opening('b', 'g:b'),
                detail('d:4', 'g:b'),
                subtotal('s:b', 'g:b'),
                { ...total, cells: { cost: 400 } },
            ],
            page: { size: 2 },
        }
        const merged = mergePages([first, second])
        expect(merged.rows.map(r => r.id)).toEqual([
            'g:a',
            'opening:["a"]',
            'd:1',
            'd:2',
            'd:3',
            's:a',
            'g:b',
            'opening:["b"]',
            'd:4',
            's:b',
            'total',
        ])
        expect(merged.rows[merged.rows.length - 1]?.cells.cost).toBe(400)
        expect(merged.page).toEqual({ size: 2 })
    })

    it('keeps one copy of an opening or subtotal row two pages both give', () => {
        const first: RunResult = {
            ...page([], 0),
            rows: [
                group('g:a', ['a'], { expanded: true }),
                opening('a', 'g:a'),
                detail('d:1', 'g:a'),
                total,
            ],
        }
        const second: RunResult = {
            ...page([], 0),
            rows: [
                opening('a', 'g:a'),
                detail('d:1', 'g:a'),
                detail('d:2', 'g:a'),
                subtotal('s:a', 'g:a'),
                total,
            ],
        }
        const third: RunResult = { ...page([], 0), rows: [subtotal('s:a', 'g:a'), total] }
        expect(mergePages([first, second, third]).rows.map(r => r.id)).toEqual([
            'g:a',
            'opening:["a"]',
            'd:1',
            'd:2',
            's:a',
            'total',
        ])
    })

    // The server keeps a total inside the page's slice in its place (one Total per
    // currency block); only the totals it appends after the slice repeat on each page.
    it('keeps an earlier currency’s Total that sits inside an earlier page', () => {
        const usdTotal: ResultRow = { ...total, id: 'total:USD', cells: { cost: 100 } }
        const gbpTotal = (cost: number): ResultRow => ({
            ...total,
            id: 'total:GBP',
            cells: { cost },
        })
        const first: RunResult = {
            ...page([], 0),
            rows: [
                group('g:usd', ['USD'], { expanded: true }),
                detail('d:1', 'g:usd'),
                subtotal('s:usd', 'g:usd'),
                usdTotal,
                group('g:gbp', ['GBP'], { expanded: true }),
                detail('d:2', 'g:gbp'),
                gbpTotal(50),
            ],
            page: { size: 2, next: 'c1' },
        }
        const second: RunResult = {
            ...page([], 0),
            rows: [detail('d:3', 'g:gbp'), subtotal('s:gbp', 'g:gbp'), gbpTotal(80)],
            page: { size: 1 },
        }
        const merged = mergePages([first, second])
        expect(merged.rows.map(r => r.id)).toEqual([
            'g:usd',
            'd:1',
            's:usd',
            'total:USD',
            'g:gbp',
            'd:2',
            'd:3',
            's:gbp',
            'total:GBP',
        ])
        expect(merged.rows[merged.rows.length - 1]?.cells.cost).toBe(80)
    })
})

describe('holdWholeReportRows', () => {
    const pageOf = (next: string, rows: ResultRow[]): RunResult => ({
        columns: [{ id: 'cost', label: 'Cost', type: 'money' }],
        rows,
        notices: [],
        topics: [],
        page: { size: rows.length, next },
        params: [],
    })
    const net: ResultRow = {
        id: 'c:net',
        kind: 'computed',
        depth: 0,
        label: 'Net',
        cells: { cost: 5 },
    }

    it('holds the trailing Total and computed rows back while a later page is left', () => {
        const shown = holdWholeReportRows(pageOf('next', [detail('d:1'), net, total]))
        expect(shown.result.rows.map(r => r.id)).toEqual(['d:1'])
        expect(shown.held.map(r => r.id)).toEqual(['c:net', 'total'])
    })

    it('shows them with the last page, and leaves an unpaged result as it is', () => {
        const last = pageOf('', [detail('d:1'), total])
        expect(holdWholeReportRows(last)).toEqual({ result: last, held: [] })
        const unpaged: RunResult = { ...last, page: null }
        expect(holdWholeReportRows(unpaged).result).toBe(unpaged)
    })

    it('keeps a Total inside the page’s slice in its place', () => {
        const block: ResultRow = { ...total, id: 'total:EUR' }
        const shown = holdWholeReportRows(
            pageOf('next', [detail('d:1'), block, detail('d:2'), total])
        )
        expect(shown.result.rows.map(r => r.id)).toEqual(['d:1', 'total:EUR', 'd:2'])
        expect(shown.held.map(r => r.id)).toEqual(['total'])
    })

    it('shows a last page of whole-report rows alone', () => {
        const merged = mergePages([pageOf('next', [detail('d:1'), total]), pageOf('', [total])])
        const shown = holdWholeReportRows(merged)
        expect(shown.result.rows.map(r => r.id)).toEqual(['d:1', 'total'])
        expect(shown.held).toEqual([])
    })
})

describe('rows at a skipped level (G1)', () => {
    // A layout report: section › class (skipped for this row) › account. The account
    // sits at depth 1 but is a level-3 group, whose level has no subtotals.
    const spec = {
        groups: [{ subtotals: true }, { subtotals: true }, { subtotals: false }],
        layout: [{}],
    }
    const account: ResultRow = {
        id: 'g:["equity",null,"acct"]',
        kind: 'group',
        depth: 1,
        level: 3,
        label: 'Owner capital',
        canExpand: true,
        expanded: false,
        cells: { amount: 500 },
    }

    it('keeps the figures of a group at depth 1 whose level ends in no subtotal', () => {
        expect(visibleRows([account], spec)[0]?.cells).toEqual({ amount: 500 })
    })

    it('reads depth + 1 when a row carries no level', () => {
        const { level: _level, ...noLevel } = account
        expect(visibleRows([noLevel], spec)[0]?.cells).toEqual({})
    })
})

describe('a computed row inside a section (G4)', () => {
    const spec = { groups: [{ subtotals: true }], layout: [{}] }
    const section = (expanded: boolean): ResultRow => ({
        id: 'g:["equity"]',
        kind: 'group',
        depth: 0,
        level: 1,
        label: 'Equity',
        canExpand: true,
        expanded,
        cells: { amount: 900 },
    })
    const inside: ResultRow = {
        id: 'c:netIncome',
        kind: 'computed',
        depth: 1,
        label: 'Net income',
        section: 'g:["equity"]',
        cells: { amount: 400 },
    }
    const total: ResultRow = {
        id: 's:["equity"]',
        kind: 'subtotal',
        depth: 0,
        level: 1,
        label: 'Total Equity',
        section: 'g:["equity"]',
        cells: { amount: 900 },
    }

    it('sits at depth 1 before the total of an open section and does not open', () => {
        const shown = visibleRows([section(true), inside, total], spec)
        expect(shown.map(r => r.row.id)).toEqual(['g:["equity"]', 'c:netIncome', 's:["equity"]'])
        expect(shown[1]).toMatchObject({ isToggle: false, isOpen: false, cells: { amount: 400 } })
        expect(shown[1]?.row.depth).toBe(1)
    })

    it('is hidden inside a folded section', () => {
        const shown = visibleRows([section(false), inside, total], spec)
        expect(shown.map(r => r.row.id)).toEqual(['g:["equity"]', 's:["equity"]'])
    })

    it('is not held back or dropped as a whole-report row at a page end', () => {
        const page = (rows: ResultRow[], next?: string): RunResult => ({
            columns: [],
            rows,
            notices: [],
            topics: [],
            page: next ? { next, size: 1 } : { size: 1 },
            params: [],
        })
        const first = page([section(true), inside], 'p2')
        expect(holdWholeReportRows(first).result.rows.map(r => r.id)).toEqual([
            'g:["equity"]',
            'c:netIncome',
        ])
        const merged = mergePages([first, page([total])])
        expect(merged.rows.map(r => r.id)).toEqual(['g:["equity"]', 'c:netIncome', 's:["equity"]'])
    })
})
