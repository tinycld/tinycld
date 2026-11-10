import { describe, expect, it } from 'vitest'
import { CURRENT_SPEC_VERSION, type ReportSpec } from '../../spec'
import { type BuilderDraft, draftFromSpec } from '../draft'
import { pivotRangeChoices } from '../pivot'
import { CATALOG, SALES_KEY } from './catalog-fixture'

const BY_MONTH: ReportSpec = {
    version: CURRENT_SPEC_VERSION,
    source: SALES_KEY,
    params: [
        { id: 'period', type: 'dateRange', label: 'Period' },
        { id: 'asOf', type: 'asOf', label: 'As of' },
    ],
    columns: [
        { id: 'customer', label: 'Customer', path: [{ field: 'customer' }] },
        { id: 'amount', label: 'Amount', path: [{ field: 'amount' }], aggregate: 'sum' },
    ],
    groups: [{ id: 'byCustomer', col: 'customer' }],
    pivot: { path: [{ field: 'day' }], bucket: 'month', cell: 'amount' },
}
const META = { title: 'By month', description: '' }
const withPivot = (draft: BuilderDraft, pivot: Partial<BuilderDraft['pivot']>): BuilderDraft => ({
    ...draft,
    pivot: { ...draft.pivot, ...pivot },
})

describe('pivotRangeChoices', () => {
    it('offers every bucket of each date range param on a bucketed date pivot', () => {
        const draft = draftFromSpec(BY_MONTH, META, CATALOG)
        expect(pivotRangeChoices(draft, CATALOG)).toEqual([
            { value: '', label: 'Only the months with rows' },
            { value: 'period', label: 'Show every month of Period' },
        ])
        expect(pivotRangeChoices(withPivot(draft, { bucket: 'fiscalYear' }), CATALOG)).toEqual([
            { value: '', label: 'Only the fiscal years with rows' },
            { value: 'period', label: 'Show every fiscal year of Period' },
        ])
    })

    it('offers none without a bucket, off a date, without a date range param or a pivot', () => {
        const draft = draftFromSpec(BY_MONTH, META, CATALOG)
        expect(pivotRangeChoices(withPivot(draft, { bucket: '' }), CATALOG)).toEqual([])
        const byTeam = withPivot(draft, { path: [{ link: 'team' }], bucket: '' })
        expect(pivotRangeChoices(byTeam, CATALOG)).toEqual([])
        const noRange = { ...draft, params: draft.params.filter(p => p.type !== 'dateRange') }
        expect(pivotRangeChoices(noRange, CATALOG)).toEqual([])
        expect(pivotRangeChoices(withPivot(draft, { on: false }), CATALOG)).toEqual([])
    })

    // The draft issue at pivot.range needs a control that can clear it.
    it('keeps a range whose date range param is gone on screen, to clear it', () => {
        const draft = withPivot(draftFromSpec(BY_MONTH, META, CATALOG), { range: 'period' })
        const noRange = { ...draft, params: draft.params.filter(p => p.type !== 'dateRange') }
        expect(pivotRangeChoices(noRange, CATALOG)).toEqual([
            { value: '', label: 'Only the columns with rows' },
            { value: 'period', label: 'period (missing)' },
        ])
    })

    it('keeps a range that names a param that is not a date range, beside the valid choices', () => {
        const draft = withPivot(draftFromSpec(BY_MONTH, META, CATALOG), { range: 'asOf' })
        expect(pivotRangeChoices(draft, CATALOG)).toEqual([
            { value: '', label: 'Only the months with rows' },
            { value: 'period', label: 'Show every month of Period' },
            { value: 'asOf', label: 'asOf (missing)' },
        ])
    })

    it('adds no missing choice for a range that is offered', () => {
        const draft = withPivot(draftFromSpec(BY_MONTH, META, CATALOG), { range: 'period' })
        expect(pivotRangeChoices(draft, CATALOG).map(c => c.value)).toEqual(['', 'period'])
    })
})
