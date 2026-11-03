import { and, eq, inArray, like, or } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { collectionByName } from '@tinycld/core/lib/pocketbase'
import { materialize } from 'pbtsdb'
import { type RecordLabel, relatedIdsMatching, renderRecordLabel, searchTokens } from './label'

// How many matches a search returns at once — and, because both the filter
// and the cap are in the request, how many rows are ever fetched.
export const VISIBLE_LIMIT = 50
// How many related rows one search word may match. Their ids go into the
// target's filter, and so into its realtime topic, which has a length limit.
const RELATED_LIMIT = 10

// The row key the related record is selected under. `$` keeps it clear of
// every PocketBase field name, which cannot start with one.
const RELATED_KEY = '$labelRelated'

// The collections are named at runtime (by a package's automation catalog, or
// a label's `relations`), so each is one of the registered stores, resolved by
// name, and a label field cannot be a key of its row type.
export type LabelCollection = NonNullable<ReturnType<typeof collectionByName>>

/** The collection to search, and the one its label's relation points at. */
export interface LabelSources {
    collection: LabelCollection | undefined
    related: LabelCollection | undefined
    /** Only records whose fields equal these values. */
    match?: Record<string, string | number | boolean>
}

export interface LabelledRecord {
    id: string
    name: string
}

type Row = Record<string, unknown>

// The ref proxy answers an arbitrary property; only the row type refuses to be
// indexed by a plain string. `string` is the narrowest type both `like` and
// `orderBy` accept for a value expression, so the cast lands there rather than
// on `any`, and it covers exactly this one access.
function fieldRef(record: object, field: string): string {
    return (record as Record<string, string>)[field]
}

type Condition = ReturnType<typeof or>

// What an `or` of no conditions means. `id` is never empty, and the filter it
// compiles to is valid on every collection.
function noMatch(record: object): Condition {
    return eq(fieldRef(record, 'id'), '')
}

function anyOf(record: object, conditions: Condition[]): Condition {
    if (conditions.length === 0) return noMatch(record)
    return conditions.reduce((all, condition) => or(all, condition))
}

function toLabelled(label: RecordLabel, row: Row): LabelledRecord {
    return {
        id: typeof row.id === 'string' ? row.id : '',
        name: renderRecordLabel(label, row, row[RELATED_KEY] as Row | undefined),
    }
}

/**
 * The related rows any search word matches, bounded. Only a label that reads
 * through a relation needs them: the target's own filter cannot test a field
 * of another collection, so it tests the relation against these rows' ids.
 */
function useRelatedMatches(sources: LabelSources, label: RecordLabel, tokens: string[]) {
    const relation = label.related
    const collection = sources.related
    const isNeeded = Boolean(relation && collection && tokens.length > 0)
    const { data, isReady } = useLiveQuery({
        query: q => {
            if (!relation || !collection || tokens.length === 0) return null
            const [sortField] = relation.fields
            return q
                .from({ rel: collection })
                .where(({ rel }) =>
                    anyOf(
                        rel,
                        tokens.flatMap(token =>
                            relation.fields.map(field => like(fieldRef(rel, field), `%${token}%`))
                        )
                    )
                )
                .orderBy(({ rel }) => fieldRef(rel, sortField))
                .limit(RELATED_LIMIT)
        },
    })
    return {
        isReady: !isNeeded || isReady,
        rows: (isNeeded ? (data ?? []) : []) as Row[],
    }
}

// One word's condition: some own field contains it, or the relation points at
// a related row that does. A word no field can contain matches no record,
// rather than being dropped and widening the search.
function tokenCondition(
    record: object,
    label: RecordLabel,
    token: string,
    relatedRows: Row[]
): Condition {
    const conditions = label.ownFields.map(field => like(fieldRef(record, field), `%${token}%`))
    if (label.related) {
        const ids = relatedIdsMatching(label, token, relatedRows)
        if (ids.length > 0) conditions.push(inArray(fieldRef(record, label.related.relation), ids))
    }
    return anyOf(record, conditions)
}

/**
 * The records whose label contains every word of `term`, named by `label`,
 * at most VISIBLE_LIMIT of them. The related rows a label reads are filed by
 * `materialize()`, so they stay live with the result.
 */
export function useLabelledRecords(sources: LabelSources, label: RecordLabel, term: string) {
    const tokens = searchTokens(label, term)
    const related = useRelatedMatches(sources, label, tokens)
    const { collection } = sources
    const relation = label.related
    const relatedCollection = sources.related
    const { data, isReady } = useLiveQuery({
        query: q => {
            // Held back until the related matches are in, so one search does
            // not first fetch the rows its own fields match and then fetch
            // again with the related ids added.
            if (!collection || !related.isReady) return null
            // Both the filter and the cap belong in the request: the target is
            // an arbitrary collection, so it could be the deployment's largest
            // table. `like` compiles to `<field> ~ "%<term>%"`; the explicit `%`
            // on both sides is what makes it a contains match, since PocketBase
            // only auto-wraps an operand that carries no `%` of its own
            // (`wrapLikeParams`). The order is the label's first field rather
            // than id because id order is meaningless to a human.
            let query = q.from({ record: collection })
            for (const [key, value] of Object.entries(sources.match ?? {})) {
                query = query.where(({ record }) => eq(fieldRef(record, key), value))
            }
            if (tokens.length > 0) {
                query = query.where(({ record }) =>
                    tokens
                        .map(token => tokenCondition(record, label, token, related.rows))
                        .reduce((all, condition) => and(all, condition))
                )
            }
            const sorted = query
                .orderBy(({ record }) => fieldRef(record, label.ownFields[0] ?? 'id'))
                .limit(VISIBLE_LIMIT)
            if (!relation || !relatedCollection) return sorted
            return sorted.select(({ record }) => ({
                ...record,
                [RELATED_KEY]: materialize(
                    q
                        .from({ rel: relatedCollection })
                        .where(({ rel }) =>
                            eq(fieldRef(rel, 'id'), fieldRef(record, relation.relation))
                        )
                        .findOne()
                ),
            }))
        },
    })
    return {
        isReady: related.isReady && isReady,
        records: ((data ?? []) as Row[]).map(row => toLabelled(label, row)),
    }
}

/**
 * The name of one record by id, live. A one-row query, not `collection.get`:
 * the target syncs on demand, so a row nobody fetched is absent from the store
 * and `get` answers undefined. Runs no request while `id` is empty.
 */
export function useLabelledRecord(
    sources: LabelSources,
    label: RecordLabel,
    id: string
): LabelledRecord | undefined {
    const { collection } = sources
    const relation = label.related
    const relatedCollection = sources.related
    const { data } = useLiveQuery({
        query: q => {
            if (!collection || !id) return null
            const query = q
                .from({ record: collection })
                .where(({ record }) => eq(fieldRef(record, 'id'), id))
            if (!relation || !relatedCollection) return query
            return query.select(({ record }) => ({
                ...record,
                [RELATED_KEY]: materialize(
                    q
                        .from({ rel: relatedCollection })
                        .where(({ rel }) =>
                            eq(fieldRef(rel, 'id'), fieldRef(record, relation.relation))
                        )
                        .findOne()
                ),
            }))
        },
    })
    const row = (data as Row[] | undefined)?.[0]
    return row ? toLabelled(label, row) : undefined
}
