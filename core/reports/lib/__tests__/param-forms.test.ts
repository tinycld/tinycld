import { describe, expect, it } from 'vitest'
import {
    DATE_FORMAT_MESSAGE,
    FROM_AFTER_TO_MESSAGE,
    periodDefaults,
    periodSchema,
    periodValueOf,
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
