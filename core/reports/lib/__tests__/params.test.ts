import { describe, expect, it } from 'vitest'
import {
    chipText,
    controlItems,
    defaultValueOf,
    emptyAction,
    emptyMessage,
    isOptional,
    type ParamValues,
    periodWords,
    postedParams,
    valueFromJson,
    valueToJson,
} from '../params'
import type { ParamDef } from '../spec'

const PERIOD: ParamDef = {
    id: 'period',
    type: 'dateRange',
    label: 'Period',
    default: { preset: 'thisMonth' },
}
const AS_OF: ParamDef = { id: 'asOf', type: 'asOf', default: { preset: 'endOfLastMonth' } }
const STATUS: ParamDef = {
    id: 'status',
    type: 'choice',
    label: 'Status',
    options: [
        { value: 'open', label: 'Open' },
        { value: 'paid', label: 'Paid' },
    ],
}
const KINDS: ParamDef = { ...STATUS, id: 'kinds', label: 'Kinds', multiple: true }
const NAME: ParamDef = { id: 'name', type: 'text', label: 'Name' }
const MIN: ParamDef = { id: 'min', type: 'number', label: 'At least', default: 100 }

describe('param values from JSON', () => {
    it('reads presets, ranges, days, options, text and numbers', () => {
        expect(valueFromJson(PERIOD, { preset: 'thisMonth' })).toEqual({
            kind: 'preset',
            preset: 'thisMonth',
        })
        expect(valueFromJson(PERIOD, { from: '2026-01-01', to: '2026-03-31' })).toEqual({
            kind: 'range',
            from: '2026-01-01',
            to: '2026-03-31',
        })
        expect(valueFromJson(AS_OF, '2026-06-30')).toEqual({ kind: 'day', day: '2026-06-30' })
        expect(valueFromJson(STATUS, 'paid')).toEqual({ kind: 'choice', values: ['paid'] })
        expect(valueFromJson(KINDS, ['paid', 'open', 'paid'])).toEqual({
            kind: 'choice',
            values: ['open', 'paid'],
        })
        expect(valueFromJson(NAME, 'Acme')).toEqual({ kind: 'text', text: 'Acme' })
        expect(valueFromJson(MIN, 5)).toEqual({ kind: 'number', number: 5 })
    })

    it('refuses JSON that does not fit the param', () => {
        expect(valueFromJson(PERIOD, { preset: 'endOfLastMonth' })).toBeNull()
        expect(valueFromJson(AS_OF, { preset: 'thisMonth' })).toBeNull()
        expect(valueFromJson(PERIOD, { from: '2026-03-31', to: '2026-01-01' })).toBeNull()
        expect(valueFromJson(STATUS, 'gone')).toBeNull()
        expect(valueFromJson(KINDS, [])).toBeNull()
        expect(valueFromJson(NAME, 'x'.repeat(501))).toBeNull()
        expect(valueFromJson(MIN, Number.NaN)).toBeNull()
    })

    it('writes the JSON the run endpoint reads', () => {
        expect(valueToJson(PERIOD, { kind: 'preset', preset: 'lastMonth' })).toEqual({
            preset: 'lastMonth',
        })
        expect(
            valueToJson(PERIOD, { kind: 'range', from: '2026-01-01', to: '2026-01-31' })
        ).toEqual({ from: '2026-01-01', to: '2026-01-31' })
        expect(valueToJson(STATUS, { kind: 'choice', values: ['open'] })).toBe('open')
        expect(valueToJson(KINDS, { kind: 'choice', values: ['open'] })).toEqual(['open'])
    })
})

describe('defaults and optional params', () => {
    it('reads a default and calls a param without one optional', () => {
        expect(defaultValueOf(PERIOD)).toEqual({ kind: 'preset', preset: 'thisMonth' })
        expect(isOptional(PERIOD)).toBe(false)
        expect(isOptional(STATUS)).toBe(true)
    })

    it('posts only the values the reader set', () => {
        const values: ParamValues = { status: { kind: 'choice', values: ['open'] } }
        expect(postedParams([PERIOD, STATUS, NAME], values)).toEqual({ status: 'open' })
    })
})

describe('controlItems', () => {
    it('puts the first date param first, then params with defaults, then set filters, then Add filter', () => {
        const values: ParamValues = { name: { kind: 'text', text: 'Acme' } }
        const items = controlItems([STATUS, MIN, NAME, PERIOD], values)
        expect(
            items.map(item =>
                item.kind === 'param' ? `${item.def.id}:${item.isRemovable}` : 'add'
            )
        ).toEqual(['period:false', 'min:false', 'name:true', 'add'])
        const add = items[3]
        expect(add.kind === 'add-filter' && add.defs.map(d => d.id)).toEqual(['status'])
    })

    it('offers no Add filter when every optional param is set', () => {
        const values: ParamValues = { status: { kind: 'choice', values: ['paid'] } }
        expect(controlItems([STATUS], values).map(i => i.kind)).toEqual(['param'])
    })
})

describe('chip text', () => {
    it('shows a preset with the days the server resolved', () => {
        const resolved = {
            id: 'period',
            type: 'dateRange' as const,
            from: '2026-10-01',
            to: '2026-10-31',
        }
        expect(chipText(PERIOD, { kind: 'preset', preset: 'thisMonth' }, resolved)).toBe(
            'Period: This month · Oct 1 – Oct 31, 2026'
        )
        expect(chipText(PERIOD, { kind: 'preset', preset: 'thisMonth' }, undefined)).toBe(
            'Period: This month'
        )
    })

    it('names choices, text and numbers', () => {
        expect(chipText(KINDS, { kind: 'choice', values: ['open', 'paid'] }, undefined)).toBe(
            'Kinds: Open, Paid'
        )
        expect(chipText(NAME, { kind: 'text', text: 'Acme' }, undefined)).toBe('Name: “Acme”')
        expect(chipText(AS_OF, { kind: 'day', day: '2026-06-30' }, undefined)).toBe('Jun 30, 2026')
    })
})

describe('empty state', () => {
    const range = { id: 'period', type: 'dateRange' as const, from: '2026-10-01', to: '2026-10-31' }

    it('names the period', () => {
        expect(periodWords(PERIOD, range)).toBe('October 1 – October 31, 2026')
        expect(emptyMessage(PERIOD, range)).toBe('No rows for October 1 – October 31, 2026')
        expect(emptyMessage(AS_OF, { id: 'asOf', type: 'asOf', value: '2026-09-30' })).toBe(
            'No rows as of September 30, 2026'
        )
        expect(emptyMessage(undefined, undefined)).toBe('No rows')
    })

    it('offers Clear filters first, then the wider period', () => {
        expect(
            emptyAction([PERIOD, STATUS], { status: { kind: 'choice', values: ['open'] } })
        ).toEqual({ kind: 'clear-filters', label: 'Clear filters' })
        expect(emptyAction([PERIOD], {})).toEqual({
            kind: 'preset',
            paramId: 'period',
            preset: 'thisFiscalYear',
            label: 'Show this fiscal year',
        })
        expect(
            emptyAction([PERIOD], { period: { kind: 'preset', preset: 'thisFiscalYear' } })
        ).toBeNull()
        expect(emptyAction([AS_OF], {})).toEqual({
            kind: 'preset',
            paramId: 'asOf',
            preset: 'endOfLastFiscalYear',
            label: 'Show end of last fiscal year',
        })
        expect(emptyAction([NAME], {})).toBeNull()
    })
})
