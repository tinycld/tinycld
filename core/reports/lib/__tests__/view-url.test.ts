import { describe, expect, it } from 'vitest'
import { DEFAULT_DISPLAY } from '../format'
import type { ParamValue } from '../params'
import type { ParamDef } from '../spec'
import { parseViewOptions, reportLink, serializeViewOptions, type ViewOptions } from '../view-url'

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
        }
        const search = serializeViewOptions(DEFS, options)
        expect(search).toMatchObject({
            'p-period': '2026-01-01..2026-03-31',
            div1000: '1',
            negative: 'parens',
            red: '1',
            hide: 'cost,cost.p0',
        })
        expect(parseViewOptions(DEFS, search)).toEqual(options)
    })

    it('writes nothing for defaults, so a clean link stays clean', () => {
        const search = serializeViewOptions(DEFS, {
            params: { period: { kind: 'preset', preset: 'thisMonth' } },
            display: DEFAULT_DISPLAY,
        })
        expect(Object.values(search).every(value => value === undefined)).toBe(true)
        expect(Object.keys(search)).toEqual(
            expect.arrayContaining(['p-period', 'p-kinds', 'div1000', 'hide'])
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
        })
        expect(parsed.params).toEqual({})
        expect(parsed.display).toEqual({ ...DEFAULT_DISPLAY, hidden: ['cost'] })
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
        })
        expect(search['p-teams']).toBe('["a1",{"group":"north"}]')
        expect(parseViewOptions([TEAMS], search).params.teams).toEqual(picked)
    })

    it('builds a full link the reader can copy', () => {
        const link = reportLink('https://acme.example', 'r1', DEFS, {
            params: { name: { kind: 'text', text: 'Acme' } },
            display: DEFAULT_DISPLAY,
        })
        expect(link).toBe('https://acme.example/a/reports/r1?p-name=Acme')
    })
})
