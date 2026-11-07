import { describe, expect, it } from 'vitest'
import { DEFAULT_DISPLAY } from '../format'
import type { ParamValue } from '../params'
import type { ParamDef } from '../spec'
import {
    decodeSort,
    encodeSort,
    nextSort,
    parseViewOptions,
    reportLink,
    SORT_KEY,
    serializeViewOptions,
    type ViewOptions,
} from '../view-url'

const PERIOD: ParamDef = { id: 'period', type: 'dateRange', default: { preset: 'thisMonth' } }
const KINDS: ParamDef = {
    id: 'kinds',
    type: 'choice',
    multiple: true,
    options: [
        { value: 'a,b', label: 'A or B' },
        { value: 'c', label: 'C' },
    ],
}
const NAME: ParamDef = { id: 'name', type: 'text' }
const MIN: ParamDef = { id: 'min', type: 'number' }
const DEFS = [PERIOD, KINDS, NAME, MIN]

describe('the view URL', () => {
    it('round-trips params and display options', () => {
        const options: ViewOptions = {
            params: {
                period: { kind: 'range', from: '2026-01-01', to: '2026-03-31' },
                kinds: { kind: 'choice', values: ['a,b', 'c'] },
                name: { kind: 'text', text: 'Acme & Co' },
                min: { kind: 'number', number: 2.5 },
            },
            display: {
                divideBy1000: true,
                withoutCents: false,
                negative: 'parens',
                negativeRed: true,
                hidden: ['cost', 'cost.p0'],
            },
            sort: { col: 'cost', dir: 'desc' },
        }
        const search = serializeViewOptions(DEFS, options)
        expect(search).toMatchObject({
            'p-period': '2026-01-01..2026-03-31',
            div1000: '1',
            negative: 'parens',
            red: '1',
            hide: 'cost,cost.p0',
            s: 'cost:desc',
        })
        expect(parseViewOptions(DEFS, search)).toEqual(options)
    })

    it('writes nothing for defaults, so a clean link stays clean', () => {
        const search = serializeViewOptions(DEFS, {
            params: { period: { kind: 'preset', preset: 'thisMonth' } },
            display: DEFAULT_DISPLAY,
            sort: null,
        })
        expect(Object.values(search).every(value => value === undefined)).toBe(true)
        expect(Object.keys(search)).toEqual(
            expect.arrayContaining(['p-period', 'p-kinds', 'div1000', 'hide', 's'])
        )
    })

    it('falls back to the default for a value the report no longer offers', () => {
        const parsed = parseViewOptions(DEFS, {
            'p-period': 'endOfLastMonth',
            'p-kinds': '["gone"]',
            'p-min': 'many',
            negative: 'sideways',
            div1000: 'yes',
            hide: 'cost,<script>',
            s: 'cost:sideways',
        })
        expect(parsed.params).toEqual({})
        expect(parsed.display).toEqual({ ...DEFAULT_DISPLAY, hidden: ['cost'] })
        expect(parsed.sort).toBeNull()
    })

    it('reads a preset by name and refuses a backwards range', () => {
        expect(parseViewOptions(DEFS, { 'p-period': 'lastQuarter' }).params.period).toEqual({
            kind: 'preset',
            preset: 'lastQuarter',
        })
        expect(parseViewOptions(DEFS, { 'p-period': '2026-03-31..2026-01-01' }).params).toEqual({})
    })

    it('writes an amount as the amount and its currency', () => {
        const BUDGET: ParamDef = { id: 'b', type: 'amount' }
        const budget = { kind: 'amount', cents: 150000, currency: 'USD' } as const
        const parsed = parseViewOptions([BUDGET], { 'p-b': '1500.00 USD' })
        expect(parsed.params.b).toEqual(budget)
        expect(serializeViewOptions([BUDGET], parsed)['p-b']).toBe('1500.00 USD')
        expect(parseViewOptions([BUDGET], { 'p-b': '12' }).params.b).toEqual({
            kind: 'amount',
            cents: 1200,
            currency: '',
        })
        expect(
            serializeViewOptions([BUDGET], {
                params: { b: { kind: 'amount', cents: 1200, currency: '' } },
                display: DEFAULT_DISPLAY,
                sort: null,
            })['p-b']
        ).toBe('12.00')
        for (const bad of ['12 usd', '12.345 USD', 'USD', '1 2 USD', ''])
            expect(parseViewOptions([BUDGET], { 'p-b': bad }).params).toEqual({})
    })

    it('keeps a whole group picked from a source in the address', () => {
        const TEAMS: ParamDef = {
            id: 'teams',
            type: 'choice',
            multiple: true,
            optionsFrom: {
                source: 's',
                value: [{ field: 'id' }],
                label: [{ field: 'name' }],
                group: [{ field: 'region' }],
            },
        }
        const picked: ParamValue = { kind: 'choice', values: ['a1'], groups: ['north'] }
        const search = serializeViewOptions([TEAMS], {
            params: { teams: picked },
            display: DEFAULT_DISPLAY,
            sort: null,
        })
        expect(search['p-teams']).toBe('["a1",{"group":"north"}]')
        expect(parseViewOptions([TEAMS], search).params.teams).toEqual(picked)
    })

    it('builds a full link the reader can copy', () => {
        const link = reportLink('https://acme.example', 'r1', DEFS, {
            params: { name: { kind: 'text', text: 'Acme' } },
            display: DEFAULT_DISPLAY,
            sort: null,
        })
        expect(link).toBe('https://acme.example/a/reports/r1?p-name=Acme')
    })

    it('reads a sort as <column>:<asc|desc> and ignores any other shape', () => {
        expect(SORT_KEY).toBe('s')
        expect(parseViewOptions(DEFS, { s: 'amount:desc' }).sort).toEqual({
            col: 'amount',
            dir: 'desc',
        })
        expect(parseViewOptions(DEFS, { s: 'amount_q1.p0:asc' }).sort).toEqual({
            col: 'amount_q1.p0',
            dir: 'asc',
        })
        for (const bad of ['amount', 'amount:up', ':asc', 'amount:asc:desc', '<b>:asc', ''])
            expect(decodeSort(bad)).toBeNull()
        expect(decodeSort(undefined)).toBeNull()
        expect(encodeSort({ col: 'amount', dir: 'asc' })).toBe('amount:asc')
    })

    it('writes the sort, and leaves it out with none', () => {
        const sorted = serializeViewOptions(DEFS, {
            params: {},
            display: DEFAULT_DISPLAY,
            sort: { col: 'amount', dir: 'desc' },
        })
        expect(sorted.s).toBe('amount:desc')
        const plain = serializeViewOptions(DEFS, {
            params: {},
            display: DEFAULT_DISPLAY,
            sort: null,
        })
        expect('s' in plain).toBe(true)
        expect(plain.s).toBeUndefined()
    })

    it('keeps the sort in a copied link', () => {
        const link = reportLink('https://acme.example', 'r1', DEFS, {
            params: {},
            display: DEFAULT_DISPLAY,
            sort: { col: 'amount', dir: 'desc' },
        })
        expect(link).toBe('https://acme.example/a/reports/r1?s=amount%3Adesc')
    })

    it('reads All dates from the address', () => {
        expect(parseViewOptions(DEFS, { 'p-period': 'allDates' }).params.period).toEqual({
            kind: 'preset',
            preset: 'allDates',
        })
    })
})

describe('nextSort', () => {
    // A heading press cycles ascending → descending → the report's own order.
    it('cycles a column ascending, descending, then off', () => {
        const asc = nextSort(null, 'amount')
        expect(asc).toEqual({ col: 'amount', dir: 'asc' })
        const desc = nextSort(asc, 'amount')
        expect(desc).toEqual({ col: 'amount', dir: 'desc' })
        expect(nextSort(desc, 'amount')).toBeNull()
    })

    it('starts another column ascending', () => {
        expect(nextSort({ col: 'amount', dir: 'desc' }, 'customer')).toEqual({
            col: 'customer',
            dir: 'asc',
        })
        expect(nextSort({ col: 'amount', dir: 'asc' }, 'customer')).toEqual({
            col: 'customer',
            dir: 'asc',
        })
    })
})
