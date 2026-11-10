import type { Bucket } from '../spec'
import { type Catalog, describePath } from './columns'
import type { BuilderDraft } from './draft'

// "Show every <bucket> of <date range param>" (spec §4 P2): a pivot over a date with
// a bucket may have a column for every bucket a date range param's from..to touches
// (bind.go pivotRange refuses it anywhere else).

export interface PivotRangeChoice {
    value: string
    label: string
}

const BUCKET_WORDS: Record<Bucket, { one: string; many: string }> = {
    day: { one: 'day', many: 'days' },
    month: { one: 'month', many: 'months' },
    quarter: { one: 'quarter', many: 'quarters' },
    year: { one: 'year', many: 'years' },
    fiscalYear: { one: 'fiscal year', many: 'fiscal years' },
}

/**
 * The pivot's range choices: '' (the buckets with rows) and each date range param.
 * Where a range cannot apply, none; but a range left from an earlier choice (its
 * param deleted, or no longer a date range) stays on screen as a "(missing)" choice,
 * so the author sees it and can clear the issue.
 */
export function pivotRangeChoices(
    draft: Pick<BuilderDraft, 'source' | 'params' | 'pivot'>,
    catalog: Catalog
): PivotRangeChoice[] {
    const choices = usableChoices(draft, catalog)
    const { on, range } = draft.pivot
    if (!on || range === '' || choices.some(choice => choice.value === range)) return choices
    const shown =
        choices.length > 0 ? choices : [{ value: '', label: 'Only the columns with rows' }]
    return [...shown, { value: range, label: `${range} (missing)` }]
}

function usableChoices(
    draft: Pick<BuilderDraft, 'source' | 'params' | 'pivot'>,
    catalog: Catalog
): PivotRangeChoice[] {
    const { pivot } = draft
    if (!pivot.on || pivot.bucket === '') return []
    const type = describePath(catalog, draft.source, pivot.path)?.field.type
    if (type !== 'date' && type !== 'datetime') return []
    const ranges = draft.params.filter(param => param.type === 'dateRange')
    if (ranges.length === 0) return []
    const words = BUCKET_WORDS[pivot.bucket]
    return [
        { value: '', label: `Only the ${words.many} with rows` },
        ...ranges.map(param => ({
            value: param.id,
            label: `Show every ${words.one} of ${param.label ?? param.id}`,
        })),
    ]
}
