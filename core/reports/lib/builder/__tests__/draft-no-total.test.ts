import { describe, expect, it } from 'vitest'
import { CURRENT_SPEC_VERSION, type ReportSpec } from '../../spec'
import { draftFromSpec, specFromDraft } from '../draft'
import { CATALOG, SALES_KEY } from './catalog-fixture'

// The builder has no control for noTotal, so a save writes it back as the spec
// gives it: a customized copy of a report without a Total row has none either.
describe('a report’s noTotal in the builder', () => {
    const spec: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns: [{ id: 'status', label: 'Status', path: [{ field: 'status' }] }],
        noTotal: true,
    }

    it('round-trips a spec with noTotal through the draft unchanged', () => {
        const draft = draftFromSpec(spec, { title: 'T', description: '' }, CATALOG)
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    it('writes no noTotal for a spec without one', () => {
        const { noTotal: _, ...plain } = spec
        const draft = draftFromSpec(plain, { title: 'T', description: '' }, CATALOG)
        expect('noTotal' in specFromDraft(draft, CATALOG)).toBe(false)
    })
})
