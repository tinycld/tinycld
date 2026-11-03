import { describe, expect, it } from 'vitest'
import {
    AMOUNT_MESSAGE,
    AMOUNT_TOO_LARGE_MESSAGE,
    amountParamValue,
    amountValueSchema,
    CURRENCY_MESSAGE,
    DATE_FORMAT_MESSAGE,
    FROM_AFTER_TO_MESSAGE,
    numberValueSchema,
    periodDefaults,
    periodSchema,
    periodValueOf,
    sourceChoiceSchema,
    TEXT_TOO_LONG_MESSAGE,
    TOO_MANY_RECORDS_MESSAGE,
    textValueSchema,
} from '../param-forms'
import type { ParamDef } from '../spec'

const RANGE: ParamDef = { id: 'day', type: 'dateRange', default: { preset: 'thisMonth' } }
const AS_OF: ParamDef = { id: 'asOf', type: 'asOf' }

const issues = (def: ParamDef, values: unknown) => {
    const parsed = periodSchema(def).safeParse(values)
    return parsed.success
        ? []
        : parsed.error.issues.map(issue => `${issue.path.join('.')}: ${issue.message}`)
}

describe('the period form', () => {
    it('checks typed days only for Custom', () => {
        expect(issues(RANGE, { preset: 'thisMonth', from: 'bad', to: null })).toEqual([])
        expect(issues(RANGE, { preset: 'custom', from: '2026-02-30', to: '2026-03-01' })).toEqual([
            `from: ${DATE_FORMAT_MESSAGE}`,
        ])
        expect(issues(RANGE, { preset: 'custom', from: '2026-03-02', to: '2026-03-01' })).toEqual([
            `from: ${FROM_AFTER_TO_MESSAGE}`,
        ])
        expect(issues(AS_OF, { preset: 'custom', from: null, to: '' })).toEqual([
            `to: ${DATE_FORMAT_MESSAGE}`,
        ])
    })

    it('turns the form into a param value', () => {
        expect(periodValueOf(RANGE, { preset: 'lastMonth', from: null, to: null })).toEqual({
            kind: 'preset',
            preset: 'lastMonth',
        })
        expect(
            periodValueOf(RANGE, { preset: 'custom', from: '2026-02-01', to: '2026-02-28' })
        ).toEqual({ kind: 'range', from: '2026-02-01', to: '2026-02-28' })
        expect(periodValueOf(AS_OF, { preset: 'custom', from: null, to: '2026-06-30' })).toEqual({
            kind: 'day',
            day: '2026-06-30',
        })
        expect(
            periodValueOf(RANGE, { preset: 'custom', from: '2026-03-01', to: '2026-02-01' })
        ).toBeNull()
    })

    it('starts from the value shown, with a preset’s resolved days', () => {
        expect(
            periodDefaults(
                { kind: 'preset', preset: 'thisMonth' },
                { id: 'day', type: 'dateRange', from: '2026-10-01', to: '2026-10-31' }
            )
        ).toEqual({
            preset: 'thisMonth',
            from: '2026-10-01',
            to: '2026-10-31',
        })
        expect(periodDefaults({ kind: 'day', day: '2026-06-30' }, undefined)).toEqual({
            preset: 'custom',
            from: null,
            to: '2026-06-30',
        })
        expect(periodDefaults(null, undefined)).toEqual({ preset: 'custom', from: null, to: null })
    })
})

describe('the text filter form', () => {
    it('counts the text in UTF-8 bytes, as the engine does', () => {
        expect(textValueSchema.safeParse({ text: 'x'.repeat(500) }).success).toBe(true)
        // 250 two-byte letters are 500 bytes; one more is too long.
        expect(textValueSchema.safeParse({ text: 'é'.repeat(250) }).success).toBe(true)
        const long = textValueSchema.safeParse({ text: 'é'.repeat(251) })
        expect(long.success).toBe(false)
        expect(long.error?.issues[0]?.message).toBe(TEXT_TOO_LONG_MESSAGE)
    })
})

describe('the number filter form', () => {
    it('needs a typed number: an empty field does not apply', () => {
        expect(numberValueSchema.safeParse({ number: undefined }).success).toBe(false)
        expect(numberValueSchema.safeParse({ number: '12a' }).success).toBe(false)
        expect(numberValueSchema.safeParse({ number: 0 }).success).toBe(true)
        expect(numberValueSchema.safeParse({ number: 12.5 }).success).toBe(true)
    })
})

describe('the amount filter form', () => {
    const issueOf = (values: { amount: string; currency: string }) =>
        amountValueSchema.safeParse(values).error?.issues.map(issue => issue.message) ?? []

    it('reads an amount with grouping and a currency in any case', () => {
        const parsed = amountValueSchema.parse({ amount: '1,500.00', currency: 'usd' })
        expect(amountParamValue(parsed)).toEqual({ kind: 'amount', cents: 150000, currency: 'USD' })
        const any = amountValueSchema.parse({ amount: '-0.07', currency: '' })
        expect(amountParamValue(any)).toEqual({ kind: 'amount', cents: -7, currency: '' })
    })

    it('refuses what the engine refuses, with a message', () => {
        expect(issueOf({ amount: '12.3.4', currency: 'USD' })).toEqual([AMOUNT_MESSAGE])
        expect(issueOf({ amount: '1,50.00', currency: 'USD' })).toEqual([AMOUNT_MESSAGE])
        expect(issueOf({ amount: '', currency: 'USD' })).toEqual([AMOUNT_MESSAGE])
        expect(issueOf({ amount: '10', currency: 'US' })).toEqual([CURRENCY_MESSAGE])
        // The engine holds at most 2^53-1 cents.
        expect(issueOf({ amount: '90071992547409.91', currency: '' })).toEqual([])
        expect(issueOf({ amount: '90071992547409.92', currency: '' })).toEqual([
            AMOUNT_TOO_LARGE_MESSAGE,
        ])
    })
})

describe('the choice-from-a-source form', () => {
    it('limits the search to 200 characters, as the engine counts them', () => {
        const base = { values: [], groups: [] }
        expect(sourceChoiceSchema.safeParse({ ...base, search: 'é'.repeat(200) }).success).toBe(
            true
        )
        expect(sourceChoiceSchema.safeParse({ ...base, search: 'x'.repeat(201) }).success).toBe(
            false
        )
    })

    it('picks at most 1000 records; a whole group has no such cap', () => {
        const ids = (n: number) => Array.from({ length: n }, (_, i) => `r${i}`)
        expect(
            sourceChoiceSchema.safeParse({ search: '', values: ids(1000), groups: ['g'] }).success
        ).toBe(true)
        const over = sourceChoiceSchema.safeParse({ search: '', values: ids(1001), groups: [] })
        expect(over.error?.issues[0]?.message).toBe(TOO_MANY_RECORDS_MESSAGE)
    })
})
