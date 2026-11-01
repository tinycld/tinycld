import { describe, expect, it } from 'vitest'
import {
    cellCurrency,
    cellDisplay,
    DEFAULT_DISPLAY,
    formatReportAmount,
    formatReportPercent,
    mergeHidden,
    type NumberFormat,
    REPORT_AMOUNT_ERROR,
    shownColumns,
} from '../format'
import { MAX_STORED_AMOUNT } from '../money'
import type { ResultColumn, ResultRow } from '../types'

const PLAIN: NumberFormat = {
    divideBy1000: false,
    withoutCents: false,
    negative: 'minus',
    negativeRed: false,
}
const PARENS: NumberFormat = { ...PLAIN, negative: 'parens' }
const WITHOUT_CENTS: NumberFormat = { ...PLAIN, withoutCents: true }
const THOUSANDS: NumberFormat = { ...PLAIN, divideBy1000: true }
const THOUSANDS_WITHOUT_CENTS: NumberFormat = { ...PLAIN, divideBy1000: true, withoutCents: true }
const RED: NumberFormat = { ...PLAIN, negativeRed: true }

const plain = (cents: number, format: NumberFormat = PLAIN) =>
    formatReportAmount(cents, 'USD', format, { withSymbol: false })

const row = (kind: ResultRow['kind'], cells: ResultRow['cells'] = {}): ResultRow => ({
    id: `${kind}:1`,
    kind,
    depth: 0,
    label: 'x',
    cells,
})

const MONEY: ResultColumn = {
    id: 'cost',
    label: 'Cost',
    type: 'money',
    currencyCell: 'cost@currency',
}
const FIXED: ResultColumn = { id: 'price', label: 'Price', type: 'money', currency: 'EUR' }

describe('formatReportAmount', () => {
    it('formats bare numbers, a symbol after the minus, parentheses', () => {
        expect(plain(1234567)).toBe('12,345.67')
        expect(plain(-1234567)).toBe('-12,345.67')
        expect(plain(-0.4)).toBe('0.00')
        expect(formatReportAmount(-1234567, 'USD', PLAIN, { withSymbol: true })).toBe('-$12,345.67')
        expect(plain(-1234567, PARENS)).toBe('(12,345.67)')
    })

    it('rounds once, half away from zero', () => {
        expect(plain(1234550, WITHOUT_CENTS)).toBe('12,346')
        expect(plain(-1234550, WITHOUT_CENTS)).toBe('-12,346')
        expect(plain(1234500, THOUSANDS)).toBe('12.35')
        expect(plain(1234550, THOUSANDS_WITHOUT_CENTS)).toBe('12')
    })

    it('is exact at the stored limit and refuses anything past it', () => {
        expect(plain(MAX_STORED_AMOUNT)).toBe('90,071,992,547,409.91')
        expect(plain(9007199254740949, WITHOUT_CENTS)).toBe('90,071,992,547,409')
        expect(plain(9007199254740499, THOUSANDS)).toBe('90,071,992,547.40')
        expect(plain(MAX_STORED_AMOUNT + 1)).toBe(REPORT_AMOUNT_ERROR)
        expect(plain(Number.POSITIVE_INFINITY)).toBe(REPORT_AMOUNT_ERROR)
    })

    // A float quotient misrounds next to a half near 2^53; integer-exact rounding does not.
    it('matches BigInt rounding at every half near the limit', () => {
        const reference = (cents: number, divisor: bigint) => {
            const big = BigInt(cents)
            const magnitude = big < 0n ? -big : big
            const rounded = magnitude / divisor + (2n * (magnitude % divisor) >= divisor ? 1n : 0n)
            return big < 0n ? -rounded : rounded
        }
        const group = (n: bigint) => n.toString().replace(/\B(?=(\d{3})+$)/g, ',')
        const mismatches: string[] = []
        for (let step = -4; step <= 4; step += 1) {
            for (const base of [
                MAX_STORED_AMOUNT - 50,
                MAX_STORED_AMOUNT - 450,
                MAX_STORED_AMOUNT - 1500,
            ]) {
                const cents = base + step
                const units = reference(cents, 100n)
                const expected = units < 0n ? `-${group(-units)}` : group(units)
                const actual = plain(cents, WITHOUT_CENTS)
                if (actual !== expected) mismatches.push(`${cents}: ${actual} != ${expected}`)
            }
        }
        expect(mismatches).toEqual([])
    })
})

describe('formatReportAmount edge cases', () => {
    it('never shows a sign or parentheses for an amount that rounds to zero', () => {
        expect(plain(-49, WITHOUT_CENTS)).toBe('0')
        expect(plain(-0.4, PARENS)).toBe('0.00')
        expect(plain(-499, THOUSANDS)).toBe('0.00')
    })

    it('combines the symbol with no cents and parentheses', () => {
        const symbol = { withSymbol: true }
        expect(formatReportAmount(1234550, 'USD', WITHOUT_CENTS, symbol)).toBe('$12,346')
        expect(
            formatReportAmount(-1234550, 'USD', { ...WITHOUT_CENTS, negative: 'parens' }, symbol)
        ).toBe('($12,346)')
    })

    it('is exact at the negative stored limit', () => {
        expect(plain(-MAX_STORED_AMOUNT)).toBe('-90,071,992,547,409.91')
        expect(plain(-MAX_STORED_AMOUNT - 1)).toBe(REPORT_AMOUNT_ERROR)
    })
})

describe('formatReportPercent', () => {
    it('shows one decimal and nothing for no figure', () => {
        expect(formatReportPercent(12.345)).toBe('12.3%')
        expect(formatReportPercent(-0.05)).toBe('-0.1%')
        expect(formatReportPercent(Number.NaN)).toBe('')
    })
})

describe('cellDisplay', () => {
    it('reads each row’s currency from its currency cell, or the fixed one', () => {
        const eur = row('detail', { cost: 500, 'cost@currency': 'EUR' })
        expect(cellCurrency(MONEY, eur)).toBe('EUR')
        expect(cellCurrency(FIXED, row('detail'))).toBe('EUR')
        expect(cellCurrency({ ...MONEY, currencyCell: undefined }, row('detail'))).toBe('')
    })

    it('shows a symbol on subtotal, computed and total rows only', () => {
        const usd = { 'cost@currency': 'USD' }
        expect(cellDisplay(MONEY, row('detail', usd), 1500, PLAIN).text).toBe('15.00')
        expect(cellDisplay(MONEY, row('group', usd), 1500, PLAIN).text).toBe('15.00')
        expect(cellDisplay(MONEY, row('subtotal', usd), 1500, PLAIN).text).toBe('$15.00')
        expect(cellDisplay(MONEY, row('computed', usd), 1500, PLAIN).text).toBe('$15.00')
        expect(cellDisplay(MONEY, row('total', usd), -1500, RED)).toEqual({
            text: '-$15.00',
            align: 'right',
            isRed: true,
            isError: false,
        })
    })

    it('shows each row in its own currency within one column', () => {
        const usd = row('subtotal', { cost: 1500, 'cost@currency': 'USD' })
        const eur = row('subtotal', { cost: 1500, 'cost@currency': 'EUR' })
        expect(cellDisplay(MONEY, usd, 1500, PLAIN).text).toBe('$15.00')
        expect(cellDisplay(MONEY, eur, 1500, PLAIN).text).toBe('€15.00')
    })

    it('marks an amount past the limit as an error cell', () => {
        expect(cellDisplay(MONEY, row('detail'), MAX_STORED_AMOUNT + 1, PLAIN)).toEqual({
            text: REPORT_AMOUNT_ERROR,
            align: 'right',
            isRed: false,
            isError: true,
        })
    })

    it('uses the total type on total rows (a count of yes/no values is a number)', () => {
        const active: ResultColumn = {
            id: 'active',
            label: 'Active',
            type: 'bool',
            totalType: 'number',
            aggregate: 'count',
        }
        expect(cellDisplay(active, row('detail'), true, PLAIN).text).toBe('Yes')
        expect(cellDisplay(active, row('detail'), false, PLAIN).text).toBe('No')
        expect(cellDisplay(active, row('total'), 1234, PLAIN)).toMatchObject({
            text: '1,234',
            align: 'right',
        })
    })

    it('labels select values, writes dates, and formats percents', () => {
        const status: ResultColumn = {
            id: 'status',
            label: 'Status',
            type: 'select',
            options: [{ value: 'open', label: 'Open' }],
        }
        expect(cellDisplay(status, row('detail'), 'open', PLAIN).text).toBe('Open')
        expect(cellDisplay(status, row('detail'), 'gone', PLAIN).text).toBe('gone')
        const day: ResultColumn = { id: 'day', label: 'Day', type: 'date' }
        expect(cellDisplay(day, row('detail'), '2026-03-05 00:00:00.000Z', PLAIN)).toMatchObject({
            text: 'Mar 5, 2026',
            align: 'left',
        })
        const share: ResultColumn = { id: 'share', label: 'Share', type: 'percent' }
        expect(cellDisplay(share, row('detail'), 25, PLAIN)).toMatchObject({
            text: '25.0%',
            align: 'right',
        })
        const ratio: ResultColumn = {
            id: 'ratio',
            label: 'Ratio',
            type: 'number',
            format: 'percent',
        }
        expect(cellDisplay(ratio, row('detail'), 12.5, PLAIN).text).toBe('12.5%')
    })

    it('shows nothing for a blank cell', () => {
        expect(cellDisplay(MONEY, row('detail'), null, PLAIN).text).toBe('')
        expect(cellDisplay(MONEY, row('detail'), undefined, PLAIN).text).toBe('')
    })
})

describe('shownColumns', () => {
    const columns: ResultColumn[] = [
        { id: 'name', label: 'Name', type: 'text' },
        { id: 'cost', label: 'Cost', type: 'money' },
        { id: 'cost.p0', label: '2026-01', type: 'money', pivot: { cell: 'cost', key: '2026-01' } },
    ]

    it('hides the listed columns but never the label column', () => {
        expect(shownColumns(columns, ['cost', 'name']).map(c => c.id)).toEqual(['name', 'cost.p0'])
        expect(shownColumns(columns, DEFAULT_DISPLAY.hidden).map(c => c.id)).toEqual([
            'name',
            'cost',
            'cost.p0',
        ])
    })
})

describe('mergeHidden', () => {
    it('keeps a hidden id the listed columns do not have, and follows the listed ones', () => {
        const listed = [
            { id: 'cost', isShown: false },
            { id: 'amount', isShown: true },
        ]
        expect(mergeHidden(['cost.p9', 'amount'], listed)).toEqual(['cost.p9', 'cost'])
        expect(mergeHidden([], listed)).toEqual(['cost'])
    })
})
