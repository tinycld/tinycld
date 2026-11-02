import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { AGGREGATE_LABELS, aggregatesFor, describePath } from '../../lib/builder/columns'
import {
    addFilterItem,
    type ConditionDraft,
    emptyGroup,
    type FilterPath,
    opsFor,
    updateFilterItem,
} from '../../lib/builder/filter'
import type { FormulaRecord } from '../../lib/builder/formula-parts'
import type { SpecHop } from '../../lib/spec'
import type { Aggregate } from '../../lib/types'
import { ColumnPicker } from './ColumnPicker'
import {
    type ConditionInfo,
    type FilterContext,
    isCount,
    itemTestId,
    newColumnCondition,
    newRecordCondition,
    newTotal,
    recordOf,
    setCondition,
    startCondition,
    typeAt,
} from './condition-context'

// What a condition compares, picked in its row (a field, a total, a column, a
// group's record), and the adders under a tree: a condition on a field or a link's
// record, related rows, or on a level a total, a column or a record (R18).

export interface SubjectProps {
    ctx: FilterContext
    item: ConditionDraft
    path: FilterPath
    baseKey: string
    info: ConditionInfo
}

// A field of the rows, or of the records a fill shows.
export function FieldSubject({ ctx, item, path, baseKey, info }: SubjectProps) {
    if (item.subject !== 'field') return null
    const onPick = (next: SpecHop[]) =>
        ctx.change(
            updateFilterItem(ctx.root, path, () =>
                startCondition(typeAt(ctx.catalog, baseKey, next), { path: next })
            )
        )
    return (
        <ColumnPicker
            catalog={ctx.catalog}
            baseKey={baseKey}
            onPick={onPick}
            label={info.label}
            testID={`${itemTestId(ctx, path)}-field`}
            toOneOnly
            allowLinkEnd
        />
    )
}

// A total of the level's rows: its aggregate and the field it adds up.
export function TotalSubject({ ctx, item, path, baseKey, info }: SubjectProps) {
    if (item.subject !== 'total') return null
    const id = itemTestId(ctx, path)
    const field = describePath(ctx.catalog, baseKey, item.path)?.field
    const aggregates = field ? aggregatesFor(field, false) : []
    const setAggregate = (aggregate: Aggregate) => {
        const type = isCount(aggregate) ? 'number' : (field?.type ?? 'number')
        // The value's kind follows the total: a count of dates is a number.
        const keepsOp = opsFor(type).includes(item.op) && type === info.field.type
        const op = keepsOp ? item.op : (opsFor(type)[0] ?? 'gt')
        const reset = keepsOp ? {} : { value: '', high: '', values: [], param: '', paramHigh: '' }
        setCondition(ctx, path, { aggregate, scale: null, op, ...reset })
    }
    const items: PickItem[] = aggregates.map(aggregate => ({
        id: aggregate,
        label: AGGREGATE_LABELS[aggregate],
        isSelected: aggregate === item.aggregate,
        onSelect: () => setAggregate(aggregate),
    }))
    const onPick = (next: SpecHop[]) =>
        ctx.change(updateFilterItem(ctx.root, path, () => newTotal(ctx, baseKey, next)))
    return (
        <>
            <PickMenu
                label={AGGREGATE_LABELS[item.aggregate || 'count']}
                purpose="Total"
                items={items}
                testID={`${id}-aggregate`}
            />
            <ColumnPicker
                catalog={ctx.catalog}
                baseKey={baseKey}
                onPick={onPick}
                label={info.pathText}
                testID={`${id}-field`}
                toOneOnly
            />
        </>
    )
}

// One of the report's columns with a value on the group row.
export function ColumnSubject({ ctx, item, path }: SubjectProps) {
    if (item.subject !== 'column' || !ctx.level) return null
    const items: PickItem[] = ctx.level.columns.map(c => ({
        id: c.key,
        label: c.label,
        isSelected: c.key === item.col,
        onSelect: () => {
            if (c.key !== item.col)
                ctx.change(updateFilterItem(ctx.root, path, () => newColumnCondition(c)))
        },
    }))
    const label = ctx.level.columns.find(c => c.key === item.col)?.label ?? item.col
    return (
        <PickMenu
            label={label}
            purpose="Column"
            items={items}
            testID={`${itemTestId(ctx, path)}-column`}
        />
    )
}

// A field of the record a group level carries.
export function RecordSubject({ ctx, item, path, info }: SubjectProps) {
    const record = recordOf(ctx, item)
    if (!record) return null
    const onPick = (next: SpecHop[]) =>
        ctx.change(updateFilterItem(ctx.root, path, () => newRecordCondition(ctx, record, next)))
    return (
        <ColumnPicker
            catalog={ctx.catalog}
            baseKey={record.source}
            onPick={onPick}
            label={info.label}
            testID={`${itemTestId(ctx, path)}-field`}
            toOneOnly
            allowLinkEnd
        />
    )
}

export interface AdderProps {
    ctx: FilterContext
    path: FilterPath
    baseKey: string
}

function AddRelated({ ctx, path, baseKey }: AdderProps) {
    const manyLinks = (ctx.catalog.get(baseKey)?.links ?? []).filter(
        l => l.cardinality === 'toMany'
    )
    if (manyLinks.length === 0) return null
    const items: PickItem[] = manyLinks.map(link => ({
        id: link.key,
        label: link.label,
        onSelect: () =>
            ctx.change(
                addFilterItem(ctx.root, path, {
                    kind: 'related',
                    has: true,
                    link: [{ link: link.key }],
                    where: emptyGroup(),
                })
            ),
    }))
    return (
        <PickMenu
            label="Add related rows"
            purpose="Related rows"
            items={items}
            testID={`${itemTestId(ctx, path)}-add-related`}
        />
    )
}

// A condition on a field of the rows, or of a link's record itself.
export function RowAdders({ ctx, path, baseKey }: AdderProps) {
    if (ctx.level) return null
    const onCondition = (next: SpecHop[]) =>
        ctx.change(
            addFilterItem(
                ctx.root,
                path,
                startCondition(typeAt(ctx.catalog, baseKey, next), { path: next })
            )
        )
    return (
        <>
            <ColumnPicker
                catalog={ctx.catalog}
                baseKey={baseKey}
                onPick={onCondition}
                label="Add condition"
                testID={`${itemTestId(ctx, path)}-add-condition`}
                toOneOnly
                allowLinkEnd
            />
            <AddRelated ctx={ctx} path={path} baseKey={baseKey} />
        </>
    )
}

// The record a group carries: this level's own (test id …-add-record), or a group
// above it (…-add-record-<group>).
function RecordAdder({
    ctx,
    path,
    record,
}: {
    ctx: FilterContext
    path: FilterPath
    record: FormulaRecord
}) {
    const id = itemTestId(ctx, path)
    const isOwn = record.group === ctx.group
    const testID = isOwn ? `${id}-add-record` : `${id}-add-record-${record.group}`
    const onPick = (next: SpecHop[]) =>
        ctx.change(addFilterItem(ctx.root, path, newRecordCondition(ctx, record, next)))
    return (
        <ColumnPicker
            catalog={ctx.catalog}
            baseKey={record.source}
            onPick={onPick}
            label={`Add ${record.label} condition`}
            testID={testID}
            toOneOnly
            allowLinkEnd
        />
    )
}

function ColumnAdder({ ctx, path }: Omit<AdderProps, 'baseKey'>) {
    const columns = ctx.level?.columns ?? []
    if (columns.length === 0) return null
    const items: PickItem[] = columns.map(c => ({
        id: c.key,
        label: c.label,
        onSelect: () => ctx.change(addFilterItem(ctx.root, path, newColumnCondition(c))),
    }))
    return (
        <PickMenu
            label="Add column condition"
            purpose="Column"
            items={items}
            testID={`${itemTestId(ctx, path)}-add-column`}
        />
    )
}

// On a group level a condition compares a total of the level's rows, a column with a
// value on the group row, or a field of a group's record (R18).
export function LevelAdders({ ctx, path, baseKey }: AdderProps) {
    const level = ctx.level
    if (!level) return null
    const add = (item: ConditionDraft) => ctx.change(addFilterItem(ctx.root, path, item))
    const records = level.records.map(record => (
        <RecordAdder key={record.group} ctx={ctx} path={path} record={record} />
    ))
    return (
        <>
            <ColumnPicker
                catalog={ctx.catalog}
                baseKey={baseKey}
                onPick={next => add(newTotal(ctx, baseKey, next))}
                label="Add total condition"
                testID={`${itemTestId(ctx, path)}-add-total`}
                toOneOnly
            />
            <ColumnAdder ctx={ctx} path={path} />
            {records}
        </>
    )
}
