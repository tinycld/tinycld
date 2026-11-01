import type { SpecHop } from '../spec'
import type { Aggregate, CatalogField, CatalogSource, FieldType } from '../types'

// Columns as the builder offers them: a path of link hops ending in a field, as
// the catalog describes it. The engine's limits are repeated here only to stop a
// person early; the server is the judge.

export const MAX_HOPS = 4
export const MAX_COLUMNS = 100
export const MAX_LABEL = 200
export const DEFAULT_PIVOT_COLUMNS = 50
export const MAX_PIVOT_COLUMNS = 200
export const MAX_PARAMS = 20
export const MAX_GROUP_LEVELS = 6
export const MAX_SORT_KEYS = 10
// A formula reads columns by id, and the forms formula editor reads only its own
// keys (lowercase snake_case, at most 40 characters), so the builder makes ids of
// that shape.
const MAX_ID = 40

export type Catalog = ReadonlyMap<string, CatalogSource>

export function catalogOf(sources: readonly CatalogSource[]): Catalog {
    return new Map(sources.map(source => [source.key, source]))
}

export const AGGREGATE_LABELS: Record<Aggregate, string> = {
    sum: 'Sum',
    count: 'Count',
    countDistinct: 'Count distinct',
    min: 'Smallest',
    max: 'Largest',
    avg: 'Average',
}

export interface PathInfo {
    field: CatalogField
    source: CatalogSource
    /** The path follows a to-many link. */
    viaToMany: boolean
    /** The to-many hop's mode: a total per row, or child rows. */
    mode: 'aggregate' | 'nest' | undefined
    labels: string[]
}

export function describePath(
    catalog: Catalog,
    baseKey: string,
    path: readonly SpecHop[]
): PathInfo | null {
    let source = catalog.get(baseKey)
    const labels: string[] = []
    let viaToMany = false
    let mode: PathInfo['mode']
    for (const hop of path) {
        if (!source) return null
        if (hop.link) {
            const link = source.links.find(l => l.key === hop.link)
            if (!link) return null
            labels.push(link.label)
            if (link.cardinality === 'toMany') {
                viaToMany = true
                mode = hop.mode
            }
            source = catalog.get(link.target)
            continue
        }
        const field = source.fields.find(f => f.key === hop.field)
        return field ? { field, source, viaToMany, mode, labels: [...labels, field.label] } : null
    }
    return null
}

export function pathLabel(info: PathInfo): string {
    return info.labels.join(' › ')
}

/** The source a list of link hops reaches (an exists filter's target), or null. */
export function sourceAtLinks(
    catalog: Catalog,
    baseKey: string,
    links: readonly SpecHop[]
): CatalogSource | null {
    let source = catalog.get(baseKey) ?? null
    for (const hop of links) {
        const link = source?.links.find(l => l.key === hop.link)
        source = link ? (catalog.get(link.target) ?? null) : null
    }
    return source
}

// A total across a to-many link is computed per parent row. A distinct count
// cannot be, and an amount whose currency differs by row cannot be totalled
// apart by currency (the engine's Decision 8 refuses both).
export function aggregatesFor(field: CatalogField, viaToMany: boolean): Aggregate[] {
    if (!viaToMany) return [...field.aggregates]
    const offered = field.aggregates.filter(a => a !== 'countDistinct')
    return field.type === 'money' && field.currency ? offered.filter(a => a === 'count') : offered
}

export function defaultAggregate(field: CatalogField, viaToMany: boolean): Aggregate | '' {
    if (!viaToMany) return ''
    const offered = aggregatesFor(field, true)
    if ((field.type === 'money' || field.type === 'number') && offered.includes('sum')) return 'sum'
    return offered.includes('count') ? 'count' : (offered[0] ?? '')
}

export function aggregateResultType(aggregate: Aggregate | '', fieldType: FieldType): FieldType {
    return aggregate === 'count' || aggregate === 'countDistinct' ? 'number' : fieldType
}

/** An average needs a scale: whole cents for amounts, two places for numbers. */
export function avgScaleFor(fieldType: FieldType): number {
    return fieldType === 'money' ? 0 : 2
}

/** Any text as a lowercase snake_case key: `teamName` and `Team name` are `team_name`. */
function snakeKey(text: string): string {
    return text
        .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '_')
        .replace(/^[^a-z]+/, '')
        .slice(0, MAX_ID)
        .replace(/_+$/, '')
}

export function nextColumnId(taken: readonly string[], hint: string): string {
    const stem = snakeKey(hint) || 'col'
    if (!taken.includes(stem)) return stem
    for (let n = 2; ; n += 1) {
        const suffix = String(n)
        const id = `${stem.slice(0, MAX_ID - suffix.length)}${suffix}`
        if (!taken.includes(id)) return id
    }
}

/** A column id hint from a path: its keys joined by _, `team` + `name` is `team_name`. */
export function columnHint(path: readonly SpecHop[]): string {
    return snakeKey(path.map(hop => hop.link ?? hop.field ?? '').join('_'))
}
