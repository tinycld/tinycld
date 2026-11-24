import { describe, expect, it } from 'vitest'
import {
    drillCellParams,
    drillHref,
    drillPanelParams,
    encodeDrillValue,
    firstOf,
    fullReportHref,
    isDrillShown,
    shownDrillParams,
} from '../drill'
import { valueFromJson } from '../params'
import type { ParamDef } from '../spec'
import type { DrillParams, ResultColumn, ResultRow } from '../types'
import { DRILL_PREFIX, decodeParamValue, parseViewOptions } from '../view-url'

const TARGET: ParamDef[] = [
    { id: 'period', type: 'dateRange' },
    {
        id: 'customers',
        type: 'choice',
        multiple: true,
        optionsFrom: {
            source: 'zoo.sales',
            value: [{ field: 'customer' }],
            label: [{ field: 'customer' }],
            group: [{ field: 'status' }],
        },
    },
    {
        id: 'status',
        type: 'choice',
        options: [
            { value: 'open', label: 'Open' },
            { value: 'paid', label: 'Paid' },
        ],
    },
]

const COLUMN: ResultColumn = {
    id: 'amount',
    label: 'Amount',
    type: 'money',
    drill: { reportId: 'r2', title: 'Sales by customer' },
}

// A @drill cell is not a CellValue (R12): the test builds the cells as the
// server sends them.
function rowWith(drill: DrillParams | undefined): ResultRow {
    const cells: Record<string, unknown> = { amount: 1500 }
    if (drill) cells['amount@drill'] = drill
    return {
        id: 'g:open',
        kind: 'group',
        depth: 0,
        label: 'Open',
        cells: cells as ResultRow['cells'],
    }
}

function searchOf(href: string): Record<string, string> {
    return Object.fromEntries(new URLSearchParams(href.split('?')[1]))
}

const def = (id: string) => TARGET.find(d => d.id === id) as ParamDef

describe('drillHref', () => {
    it('writes each value as the target reads it back, with the source report', () => {
        const drill = {
            period: { from: '2026-03-01', to: '2026-03-31' },
            customers: ['acme'],
            status: 'paid',
        }
        const href = drillHref(COLUMN, rowWith(drill), 'r1')
        expect(href?.startsWith('reports/r2?')).toBe(true)
        const search = searchOf(href ?? '')
        expect(search.from).toBe('r1')
        expect(parseViewOptions(TARGET, search).params).toEqual({
            period: valueFromJson(def('period'), drill.period),
            customers: valueFromJson(def('customers'), ['acme']),
            status: valueFromJson(def('status'), 'paid'),
        })
    })

    it('writes a preset and a group pick', () => {
        const drill = { period: { preset: 'thisFiscalYearToDate' }, customers: [{ group: 'open' }] }
        const search = searchOf(drillHref(COLUMN, rowWith(drill), 'r1') ?? '')
        expect(parseViewOptions(TARGET, search).params).toEqual({
            period: { kind: 'preset', preset: 'thisFiscalYearToDate' },
            customers: valueFromJson(def('customers'), [{ group: 'open' }]),
        })
    })

    it('leaves out a cleared value, so the target takes its default', () => {
        const search = searchOf(
            drillHref(COLUMN, rowWith({ period: null, status: 'open' }), 'r1') ?? ''
        )
        expect(Object.keys(search).sort()).toEqual(['from', 'p-status'])
    })

    it('builds the path under the base of the host that draws the report', () => {
        const href = drillHref(COLUMN, rowWith({ status: 'paid' }), 'r1', 'zoo/reports')
        expect(href?.startsWith('zoo/reports/r2?')).toBe(true)
        expect(searchOf(href ?? '')).toEqual({ from: 'r1', 'p-status': 'paid' })
    })

    it('gives no href without a @drill cell or a column drill', () => {
        expect(drillHref(COLUMN, rowWith(undefined), 'r1')).toBeNull()
        expect(
            drillHref({ ...COLUMN, drill: undefined }, rowWith({ status: 'open' }), 'r1')
        ).toBeNull()
        expect(drillCellParams(COLUMN, rowWith(undefined))).toBeNull()
    })
})

describe('fullReportHref', () => {
    it("writes the panel's options without the prefix, with the source report", () => {
        const options = parseViewOptions(
            TARGET,
            {
                'd.p-period': '2026-03-01..2026-03-31',
                'd.s': 'amount:asc',
                'p-period': 'lastMonth',
            },
            DRILL_PREFIX
        )
        const href = fullReportHref('r2', TARGET, options, 'r1')
        expect(href.startsWith('reports/r2?')).toBe(true)
        const hosted = fullReportHref('r2', TARGET, options, 'r1', 'zoo/reports')
        expect(hosted).toBe(href.replace('reports/r2?', 'zoo/reports/r2?'))
        const search = searchOf(href)
        expect(search).toEqual({
            'p-period': '2026-03-01..2026-03-31',
            s: 'amount:asc',
            from: 'r1',
        })
    })
})

describe('encodeDrillValue', () => {
    it('writes each JSON form as the URL writes a param', () => {
        expect(encodeDrillValue('paid')).toBe('paid')
        expect(encodeDrillValue(12)).toBe('12')
        // An amount with no currency comes as an object (Go amountJSON), never bare cents.
        expect(encodeDrillValue({ amount: 12000, currency: '' })).toBe('120.00')
        expect(encodeDrillValue({ preset: 'lastMonth' })).toBe('lastMonth')
        expect(encodeDrillValue({ from: '2026-01-01', to: '2026-01-31' })).toBe(
            '2026-01-01..2026-01-31'
        )
        expect(encodeDrillValue(['a', 'b'])).toBe('["a","b"]')
        expect(encodeDrillValue({ amount: 150000, currency: 'USD' })).toBe('1500.00 USD')
        expect(encodeDrillValue(null)).toBeUndefined()
    })

    it('writes an amount with no currency without a trailing space, and reads it back', () => {
        const text = encodeDrillValue({ amount: 12000, currency: '' }) ?? ''
        expect(text.endsWith(' ')).toBe(false)
        const money: ParamDef = { id: 'min', type: 'amount' }
        expect(decodeParamValue(money, text)).toEqual({
            kind: 'amount',
            cents: 12000,
            currency: '',
        })
    })

    it('reads a number and an amount with a currency back as the URL writes them', () => {
        const number: ParamDef = { id: 'n', type: 'number' }
        expect(decodeParamValue(number, encodeDrillValue(12) ?? '')).toEqual({
            kind: 'number',
            number: 12,
        })
        const money: ParamDef = { id: 'min', type: 'amount' }
        const text = encodeDrillValue({ amount: 150000, currency: 'USD' }) ?? ''
        expect(decodeParamValue(money, text)).toEqual({
            kind: 'amount',
            cents: 150000,
            currency: 'USD',
        })
    })
})

describe('drillPanelParams', () => {
    const drill = { period: { from: '2026-03-01', to: '2026-03-31' }, status: 'paid' }

    it('writes d and the d.-prefixed params, and clears an earlier drill', () => {
        const current = {
            'p-period': 'lastMonth',
            d: 'r9',
            'd.p-customers': '["x"]',
            'd.s': 'amount:asc',
        }
        expect(drillPanelParams(COLUMN, rowWith(drill), current)).toStrictEqual({
            d: 'r2',
            'd.p-period': '2026-03-01..2026-03-31',
            'd.p-status': 'paid',
            'd.p-customers': undefined,
            'd.s': undefined,
        })
    })

    it('gives nothing for a figure with no drill', () => {
        expect(drillPanelParams(COLUMN, rowWith(undefined))).toBeNull()
    })

    it('closes an open record panel (one side surface, R7)', () => {
        expect(
            drillPanelParams(COLUMN, rowWith(drill), { dr: 'gizmos/history/c/1' })
        ).toStrictEqual({
            dr: undefined,
            d: 'r2',
            'd.p-period': '2026-03-01..2026-03-31',
            'd.p-status': 'paid',
        })
    })

    it('knows the figure whose drill the panel shows', () => {
        const shown = {
            'p-period': 'lastMonth',
            d: 'r2',
            'd.p-period': '2026-03-01..2026-03-31',
            'd.p-status': 'paid',
        }
        expect(isDrillShown(COLUMN, rowWith(drill), shown)).toBe(true)
        expect(isDrillShown(COLUMN, rowWith({ ...drill, status: 'open' }), shown)).toBe(false)
        expect(isDrillShown(COLUMN, rowWith(drill), { ...shown, 'd.p-customers': '["x"]' })).toBe(
            false
        )
    })

    it('ignores a drill key the route cleared (router.setParams keeps it as undefined)', () => {
        const shown = {
            d: 'r2',
            'd.p-period': '2026-03-01..2026-03-31',
            'd.p-status': 'paid',
            'd.s': undefined,
        }
        expect(isDrillShown(COLUMN, rowWith(drill), shown)).toBe(true)
    })

    it("keeps the mark while the panel's sort changes (a sort is not a chip)", () => {
        const shown = {
            d: 'r2',
            'd.p-period': '2026-03-01..2026-03-31',
            'd.p-status': 'paid',
            'd.s': 'amount:asc',
        }
        expect(isDrillShown(COLUMN, rowWith(drill), shown)).toBe(true)
    })
})

describe('shownDrillParams', () => {
    it('keeps only the drill report and param keys that hold a value', () => {
        const search = {
            d: 'r2',
            'd.s': 'amount:desc',
            'd.p-period': undefined,
            'd.p-status': 'paid',
            'p-period': 'x',
            from: 'r1',
        }
        expect(shownDrillParams(search)).toEqual({ d: 'r2', 'd.p-status': 'paid' })
    })
})

describe('firstOf', () => {
    it('reads the first of a repeated param', () => {
        expect(firstOf(['a', 'b'])).toBe('a')
        expect(firstOf('a')).toBe('a')
        expect(firstOf(undefined)).toBeUndefined()
    })
})
