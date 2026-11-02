import type { SpecHop } from '../spec'
import type { Aggregate, CatalogField, CatalogLink, CatalogSource, FieldType } from '../types'

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
export const MAX_LAYOUT_ITEMS = 200
// A formula reads columns by id; the builder makes ids of the forms key shape
// (lowercase snake_case, at most 40 characters, F2).
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
    /** The source a relation value points at (a path ending at a to-one link, or at id). */
    target?: CatalogSource
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
    let lastLink: CatalogLink | undefined
    for (const hop of path) {
        if (!source) return null
        if (hop.link) {
            const link = source.links.find(l => l.key === hop.link)
            if (!link) return null
            lastLink = link
            labels.push(link.label)
            if (link.cardinality === 'toMany') {
                viaToMany = true
                mode = hop.mode
            }
            source = catalog.get(link.target)
            continue
        }
        const field = source.fields.find(f => f.key === hop.field)
        if (!field) return null
        const target = field.key === 'id' ? source : undefined
        return { field, source, viaToMany, mode, labels: [...labels, field.label], target }
    }
    // A path that ends at a to-one link reads the linked record's id; only that last
    // link must be to-one (bind.go path).
    if (lastLink?.cardinality === 'toOne' && source) {
        const label = labels[labels.length - 1] ?? source.label
        const field: CatalogField = {
            key: 'id',
            label,
            type: 'relation',
            aggregates: ['count', 'countDistinct'],
        }
        return { field, source, target: source, viaToMany, mode, labels }
    }
    return null
}

/**
 * The source a link column's cells point at (R31): a path column that ends at a
 * to-one link or at id and is not totalled; else undefined. Its cells show the
 * record's label field.
 */
export function linkTarget(
    catalog: Catalog,
    baseKey: string,
    column: { kind: 'path' | 'formula'; path?: readonly SpecHop[]; aggregate: Aggregate | '' }
): CatalogSource | undefined {
    if (column.kind !== 'path' || !column.path || column.aggregate !== '') return undefined
    return describePath(catalog, baseKey, column.path)?.target
}

/** The field a record is named by: its first text field other than id. */
export function firstTextField(source: CatalogSource): CatalogField | undefined {
    return source.fields.find(f => f.type === 'text' && f.key !== 'id')
}

export function pathLabel(info: PathInfo): string {
    return info.labels.join(' › ')
}

/**
 * A link hop that keeps the rows without its linked record (the default), or keeps
 * only the rows that have one (`with: required`, an inner join). Other hops and the
 * hop's mode are kept.
 */
export function withHopJoin(hops: readonly SpecHop[], index: number, required: boolean): SpecHop[] {
    return hops.map((hop, i) => {
        if (i !== index || hop.link === undefined) return hop
        const kept: SpecHop = hop.mode ? { link: hop.link, mode: hop.mode } : { link: hop.link }
        return required ? { ...kept, with: 'required' } : kept
    })
}

/**
 * A column's to-many hop read as a total per row, or as child rows. Child rows show
 * the linked rows there are, so a nest hop has no join (bind.go hopProblem).
 */
export function withHopMode(
    hops: readonly SpecHop[],
    index: number,
    mode: 'aggregate' | 'nest'
): SpecHop[] {
    return hops.map((hop, i) => {
        if (i !== index || hop.link === undefined) return hop
        if (mode === 'nest') return { link: hop.link, mode }
        return hop.with ? { link: hop.link, mode, with: hop.with } : { link: hop.link, mode }
    })
}

/**
 * The index of the one to-many hop a column path crosses, whose mode may switch; null
 * when it crosses none, or more than one (then every one nests: a total crosses one
 * to-many link, bind.go hopProblem).
 */
export function switchableModeHop(
    catalog: Catalog,
    baseKey: string,
    hops: readonly SpecHop[]
): number | null {
    let source = catalog.get(baseKey)
    const many: number[] = []
    hops.forEach((hop, index) => {
        if (hop.link === undefined) return
        const link = source?.links.find(l => l.key === hop.link)
        if (link?.cardinality === 'toMany') many.push(index)
        source = link ? catalog.get(link.target) : undefined
    })
    return many.length === 1 ? (many[0] ?? null) : null
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

/**
 * The type of a column's cells: a count is a number, any other total keeps the
 * value's type. A formula that is always empty has no type ('', formula-type.ts):
 * its cells read as text.
 */
export function aggregateResultType(
    aggregate: Aggregate | '',
    fieldType: FieldType | ''
): FieldType {
    if (aggregate === 'count' || aggregate === 'countDistinct') return 'number'
    return fieldType === '' ? 'text' : fieldType
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
