import { describe, expect, it } from 'vitest'
import { CURRENT_SPEC_VERSION, type ReportSpec } from '../../spec'
import { draftFromSpec, specFromDraft } from '../draft'
import { CATALOG, SALES_KEY } from './catalog-fixture'

// The builder has no control for a report's own empty text, so a save writes it
// back as the spec gives it.
describe('the report’s own empty text in the builder', () => {
    const spec: ReportSpec = {
        version: CURRENT_SPEC_VERSION,
        source: SALES_KEY,
        columns: [{ id: 'status', label: 'Status', path: [{ field: 'status' }] }],
        emptyText: 'No sales to show',
    }

    it('round-trips a spec with an empty text through the draft unchanged', () => {
        const draft = draftFromSpec(spec, { title: 'T', description: '' }, CATALOG)
        expect(specFromDraft(draft, CATALOG)).toEqual(spec)
    })

    it('writes no empty text for a spec without one', () => {
        const { emptyText: _, ...plain } = spec
        const draft = draftFromSpec(plain, { title: 'T', description: '' }, CATALOG)
        expect('emptyText' in specFromDraft(draft, CATALOG)).toBe(false)
    })
})
