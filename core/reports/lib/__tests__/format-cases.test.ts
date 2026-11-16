import { describe, expect, it } from 'vitest'
import { formatDateTime } from '../date-time'
import fixture from '../fixtures/format-cases.json'
import {
    cellDisplay,
    type DisplayOptions,
    headingText,
    type NumberFormat,
    rowLabelText,
    zoneLine,
} from '../format'
import { emptyMessage, type ParamValue, periodWords, valueText } from '../params'
import type { ParamDef } from '../spec'
import type { CellValue, ResolvedParam, ResultColumn, ResultRow } from '../types'

// One case list for the client and the server (core/server/reports
// export_format_test.go runs the same file against export_format.go), so the
// text of a cell, a label, a heading, the period and the zone line cannot drift.

type CellCase = {
    name: string
    column: ResultColumn
    row: ResultRow
    value: CellValue
    nonFinite?: string
    display: DisplayOptions
    timeZone: string
    hour12: boolean
    text: string
    isRed: boolean
    isError: boolean
}
type LabelCase = { name: string; row: ResultRow; display: DisplayOptions; text: string }
type HeadingCase = { name: string; column: ResultColumn; display: DisplayOptions; text: string }
type PeriodCase = {
    name: string
    def: ParamDef | null
    resolved: ResolvedParam | null
    text: string | null
}
type ZoneCase = { name: string; columns: ResultColumn[]; timeZone: string; text: string | null }
type ParamCase = {
    name: string
    def: ParamDef
    resolved: ResolvedParam
    labels: Record<string, string>
    text: string
}
type FileNameCase = {
    name: string
    title: string
    defs: ParamDef[]
    params: ResolvedParam[]
    today: string
    ext: string
    text: string
}
type FormatCases = {
    cells: CellCase[]
    labels: LabelCase[]
    headings: HeadingCase[]
    periods: PeriodCase[]
    zones: ZoneCase[]
    params: ParamCase[]
    empty: PeriodCase[]
    fileNames: FileNameCase[]
}

const cases = fixture as unknown as FormatCases

function numberFormat(display: DisplayOptions): NumberFormat {
    const { hidden: _hidden, ...format } = display
    return format
}

// JSON holds no NaN or Infinity: a case marks the value it stands for.
function cellValue(c: CellCase): CellValue {
    return c.nonFinite ? Number(c.nonFinite) : c.value
}

// The run answers a param in its resolved shape; the chip text reads the
// value form. The server's ParamsLine reads the resolved shape directly.
function paramValueOf(r: ResolvedParam): ParamValue | null {
    if (r.unset) return null
    const v = r.value
    switch (r.type) {
        case 'choice':
            return {
                kind: 'choice',
                values: r.list ?? (typeof v === 'string' ? [v] : []),
                ...(r.groups ? { groups: r.groups } : {}),
            }
        case 'text':
            return { kind: 'text', text: String(v) }
        case 'number':
            return { kind: 'number', number: Number(v) }
        case 'amount':
            return typeof v === 'object' && v !== null
                ? { kind: 'amount', cents: v.amount, currency: v.currency }
                : null
        default:
            throw new Error(`a params case of type ${r.type} has no value form`)
    }
}

describe('format-cases.json against lib/format.ts', () => {
    it.each(cases.cells.map(c => [c.name, c] as const))('cell: %s', (_name, c) => {
        const shown = cellDisplay(c.column, c.row, cellValue(c), numberFormat(c.display))
        // A date-time cell reads the device clock in the app; the case names the
        // reader's zone and hour cycle, which the export passes explicitly.
        const text =
            c.column.type === 'datetime' && typeof c.value === 'string'
                ? formatDateTime(c.value, c.timeZone, c.hour12)
                : shown.text
        expect({ text, isRed: shown.isRed, isError: shown.isError }).toEqual({
            text: c.text,
            isRed: c.isRed,
            isError: c.isError,
        })
    })

    it.each(cases.labels.map(c => [c.name, c] as const))('label: %s', (_name, c) => {
        expect(rowLabelText(c.row, numberFormat(c.display))).toBe(c.text)
    })

    it.each(cases.headings.map(c => [c.name, c] as const))('heading: %s', (_name, c) => {
        expect(headingText(c.column, numberFormat(c.display))).toBe(c.text)
    })

    it.each(cases.periods.map(c => [c.name, c] as const))('period: %s', (_name, c) => {
        expect(periodWords(c.def ?? undefined, c.resolved ?? undefined)).toBe(c.text)
    })

    it.each(cases.zones.map(c => [c.name, c] as const))('zone: %s', (_name, c) => {
        expect(zoneLine(c.columns, c.timeZone)).toBe(c.text)
    })

    it.each(cases.params.map(c => [c.name, c] as const))('param: %s', (_name, c) => {
        const labels = new Map(Object.entries(c.labels))
        expect(valueText(c.def, paramValueOf(c.resolved), labels)).toBe(c.text)
    })

    it.each(cases.empty.map(c => [c.name, c] as const))('empty: %s', (_name, c) => {
        expect(emptyMessage(c.def ?? undefined, c.resolved ?? undefined)).toBe(c.text)
    })

    // export_tables.json (core/scripts/gen-export-tables.ts) is generated from
    // Intl once; these pins make a table that differs from Intl fail here and in
    // the Go suite.
    it('pins the twelve currencies and six zones the server table must match', () => {
        const currencies = cases.cells
            .filter(
                c => c.column.type === 'money' && c.row.kind === 'subtotal' && c.column.currency
            )
            .map(c => c.column.currency)
        expect(currencies).toEqual(
            expect.arrayContaining([
                'USD',
                'EUR',
                'GBP',
                'CAD',
                'AUD',
                'JPY',
                'CHF',
                'MXN',
                'INR',
                'CNY',
                'BRL',
                'SEK',
            ])
        )
        const zones = cases.zones.filter(c => c.text !== null).map(c => c.timeZone)
        expect(zones).toEqual(
            expect.arrayContaining([
                'America/Chicago',
                'America/New_York',
                'Europe/London',
                'Asia/Tokyo',
                'Australia/Sydney',
                'Asia/Kolkata',
            ])
        )
    })
})
