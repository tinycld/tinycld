import { FormulaPlacesPick } from '@tinycld/core/components/forms-builder/FormulaEditor'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useWatch } from '@tinycld/core/ui/form'
import { DateField } from '@tinycld/core/ui/forms/fields/DateField'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Switch } from '@tinycld/core/ui/switch'
import { Trash2 } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import {
    AGGREGATE_LABELS,
    aggregatesFor,
    avgScaleFor,
    type Catalog,
    columnHint,
    describePath,
    MAX_PARAMS,
    nextColumnId,
    pathLabel,
    sourceAtLinks,
} from '../../lib/builder/columns'
import {
    type AboveGroup,
    addFilterItem,
    askHighParamFor,
    askParamFor,
    type ConditionDraft,
    emptyCondition,
    emptyGroup,
    type FilterItem,
    type FilterPath,
    filterItemKeys,
    type GroupDraft,
    type LevelScope,
    needsValue,
    opLabel,
    opsFor,
    opsShownFor,
    type RelatedDraft,
    removeFilterItem,
    setMatchAt,
    updateFilterItem,
} from '../../lib/builder/filter'
import type { FormulaRecord } from '../../lib/builder/formula-parts'
import { isDateParam, PRESET_LABELS, presetsFor } from '../../lib/params'
import type { ParamDef, SpecHop } from '../../lib/spec'
import type { Aggregate, CatalogField, CatalogSource, FieldType } from '../../lib/types'
import { REPORT_TAP_TARGET } from '../ReportHeader'
import { ColumnPicker } from './ColumnPicker'
import { IconButton } from './parts'

// A tree of conditions joined by All of / Any of / None of: the report filter, a
// group level's "Show only groups where…" (having, R18) and a fill's conditions on
// the linked records (R6). On a level a condition compares a total of a field, a
// column or a field of a group's record; in a fill under other groups its value can
// be the key of a group above (F21).

const MATCHES = [
    { value: 'all', label: 'All of' },
    { value: 'any', label: 'Any of' },
    { value: 'none', label: 'None of' },
] as const

/** A group above a fill's level, with the type of its column (a value of that type is its key). */
export type AboveChoice = AboveGroup & { type: FieldType }

export interface ConditionTreeProps {
    builder: ReportBuilder
    root: GroupDraft
    /** The whole tree changed; `added` are the params a condition newly asks for. */
    onChange(next: GroupDraft, ...added: ParamDef[]): void
    /** The source the conditions' fields are on. */
    baseKey: string
    /** The test id stem: report-builder-filter, report-builder-group-0-having, … */
    testID: string
    /** On a group level the conditions read totals, columns and group records (R18). */
    level?: LevelScope
    /** The group level's own id: its record's adder is …-add-record. */
    group?: string
    /** In a fill under other groups: the groups above whose key a value can be (F21). */
    above?: readonly AboveChoice[]
    /** What the form refuses before a run, so a blocked Save says why. */
    error?: string
}

// What every part of the tree editor needs: the tree, the asked params, and how to
// change them. Every change replaces the whole tree (and the params, R20) in one
// edit: a param nothing reads any more leaves with it.
interface FilterContext {
    builder: ReportBuilder
    catalog: Catalog
    root: GroupDraft
    params: ParamDef[]
    testID: string
    level: LevelScope | undefined
    group: string
    above: readonly AboveChoice[]
    change(next: GroupDraft, ...added: ParamDef[]): void
}

function itemTestId(ctx: FilterContext, path: FilterPath): string {
    return path.length === 0 ? ctx.testID : `${ctx.testID}-${path.join('-')}`
}

function setCondition(
    ctx: FilterContext,
    path: FilterPath,
    patch: Partial<ConditionDraft>,
    ...added: ParamDef[]
) {
    ctx.change(
        updateFilterItem(ctx.root, path, item =>
            item.kind === 'condition' ? { ...item, ...patch } : item
        ),
        ...added
    )
}

const typeAt = (catalog: Catalog, baseKey: string, path: SpecHop[]) =>
    describePath(catalog, baseKey, path)?.field.type ?? 'text'

/** A new condition starts with its type's first operator. */
function startCondition(type: FieldType, parts: Partial<ConditionDraft> & { path: SpecHop[] }) {
    return { ...emptyCondition(parts.path), ...parts, op: opsFor(type)[0] ?? 'eq' }
}

const isCount = (aggregate: Aggregate | '') =>
    aggregate === '' || aggregate === 'count' || aggregate === 'countDistinct'

/**
 * What a condition compares, as a field: its label, its type and options, and the
 * source a relation points at. A total and a column read as fields of their own.
 */
interface ConditionInfo {
    label: string
    field: CatalogField
    target?: CatalogSource
    /** The field a total adds up, or a record's field: what its picker shows. */
    pathText: string
}

function recordOf(ctx: FilterContext, item: ConditionDraft): FormulaRecord | undefined {
    return item.subject === 'record'
        ? ctx.level?.records.find(r => r.group === item.record)
        : undefined
}

function conditionInfo(
    ctx: FilterContext,
    item: ConditionDraft,
    baseKey: string
): ConditionInfo | null {
    if (item.subject === 'column') {
        const col = ctx.level?.columns.find(c => c.key === item.col)
        return col
            ? { label: col.label, field: col.field, target: col.target, pathText: col.label }
            : null
    }
    const record = recordOf(ctx, item)
    if (item.subject === 'record' && !record) return null
    const info = describePath(ctx.catalog, record?.source ?? baseKey, item.path)
    if (!info) return null
    const pathText = pathLabel(info)
    if (item.subject === 'total') {
        const label = `${AGGREGATE_LABELS[item.aggregate || 'count']} of ${pathText}`
        const type = isCount(item.aggregate) ? 'number' : info.field.type
        const options = isCount(item.aggregate) ? undefined : info.field.options
        return {
            label,
            field: { key: info.field.key, label, type, aggregates: [], options },
            pathText,
        }
    }
    const label = record ? `${record.label} › ${pathText}` : pathText
    return { label, field: info.field, target: info.target, pathText }
}

interface ValueProps {
    ctx: FilterContext
    item: ConditionDraft
    path: FilterPath
    info: ConditionInfo
}

function ChoiceChip({
    label,
    isOn,
    testID,
    onPress,
}: {
    label: string
    isOn: boolean
    testID: string
    onPress: () => void
}) {
    const chipClass = isOn ? 'border-primary bg-primary' : 'border-border'
    const textClass = isOn ? 'text-primary-foreground' : 'text-foreground'
    return (
        <Pressable
            testID={testID}
            accessibilityRole="checkbox"
            aria-checked={isOn}
            onPress={onPress}
            className={`justify-center rounded-full border px-3 ${chipClass}`}
            style={{ minHeight: REPORT_TAP_TARGET }}
        >
            <Text className={`text-sm ${textClass}`}>{label}</Text>
        </Pressable>
    )
}

// "Is any of": each option is a chip that is on or off.
function ChoiceValues({ ctx, item, path, info }: ValueProps) {
    const id = itemTestId(ctx, path)
    const toggle = (value: string) => {
        const values = item.values.includes(value)
            ? item.values.filter(v => v !== value)
            : [...item.values, value]
        setCondition(ctx, path, { values })
    }
    const chips = (info.field.options ?? []).map(option => (
        <ChoiceChip
            key={option.value}
            label={option.label}
            isOn={item.values.includes(option.value)}
            testID={`${id}-value-${option.value}`}
            onPress={() => toggle(option.value)}
        />
    ))
    return <View className="flex-row flex-wrap gap-1.5">{chips}</View>
}

function ChoiceValue({ ctx, item, path, info }: ValueProps) {
    const options = info.field.options ?? []
    const items: PickItem[] = options.map(option => ({
        id: option.value,
        label: option.label,
        isSelected: option.value === item.value,
        onSelect: () => setCondition(ctx, path, { value: option.value }),
    }))
    const label = options.find(o => o.value === item.value)?.label ?? 'Choose'
    return (
        <PickMenu
            label={label}
            purpose={`${info.label} value`}
            items={items}
            testID={`${itemTestId(ctx, path)}-value`}
            size="touch"
        />
    )
}

const NUMERIC_TYPES = new Set(['number', 'money', 'percent'])

function OneValue({ ctx, item, path, info, part }: ValueProps & { part: 'value' | 'high' }) {
    const testID = `${itemTestId(ctx, path)}-${part}`
    const label = part === 'high' ? `${info.label}, to` : `${info.label} value`
    const onChange = (text: string | null) => setCondition(ctx, path, { [part]: text ?? '' })
    const type = info.field.type
    if (type === 'date' || type === 'datetime') {
        return (
            <DateField
                value={item[part] || null}
                onChange={onChange}
                withTime={false}
                accessibilityLabel={label}
                testID={testID}
            />
        )
    }
    const keyboardType = NUMERIC_TYPES.has(type) ? 'decimal-pad' : 'default'
    return (
        <PlainInput
            value={item[part]}
            onChangeText={onChange}
            accessibilityLabel={label}
            keyboardType={keyboardType}
            testID={testID}
            className="min-w-24 flex-1 rounded-md border border-border px-2 py-2 text-sm text-foreground"
        />
    )
}

function HighValue({ isVisible, ...props }: ValueProps & { isVisible: boolean }) {
    if (!isVisible) return null
    return <OneValue {...props} part="high" />
}

// An asked value is set by the reader, and a value that is the key of the group
// above is that key: neither shows a value of its own.
function ConditionValue(props: ValueProps) {
    const { item, info } = props
    if (!needsValue(item.op) || item.param || item.above) return null
    if (info.field.type === 'select' && item.op === 'in') return <ChoiceValues {...props} />
    if (info.field.type === 'select') return <ChoiceValue {...props} />
    return (
        <View className="flex-row flex-wrap items-center gap-1.5">
            <OneValue {...props} part="value" />
            <HighValue {...props} isVisible={item.op === 'between'} />
        </View>
    )
}

function presetOf(param: ParamDef): string {
    const value = param.default
    if (typeof value !== 'object' || value === null || !('preset' in value)) return ''
    return String(value.preset)
}

// A date asked when run may start on a preset (This month, End of last month, …).
function DefaultPreset({ ctx, item, path }: ValueProps) {
    const param = ctx.params.find(p => p.id === item.param)
    if (!param || !isDateParam(param)) return null
    const current = presetOf(param)
    const setDefault = (preset: string) => {
        const next = { ...param, default: preset ? { preset } : undefined }
        ctx.builder.setParams(ctx.params.map(p => (p.id === param.id ? next : p)))
    }
    const items: PickItem[] = [
        {
            id: 'none',
            label: 'No default',
            isSelected: current === '',
            onSelect: () => setDefault(''),
        },
        ...presetsFor(param).map(preset => ({
            id: preset,
            label: PRESET_LABELS[preset],
            isSelected: preset === current,
            onSelect: () => setDefault(preset),
        })),
    ]
    const chosen = items.find(i => i.id === current && current !== '')
    const label = chosen ? `Default: ${chosen.label}` : 'No default'
    return (
        <PickMenu
            label={label}
            purpose="Default period"
            items={items}
            testID={`${itemTestId(ctx, path)}-default`}
        />
    )
}

// "Ask when run" turns the value into a param the reader sets from a chip (R16); a
// range of numbers or amounts asks both ends (F18). Yes/no cannot be asked. A
// read-back condition that already asks stays switchable off even when its
// operator could not be asked today. A column's asked value takes the column's id.
function AskToggle(props: ValueProps) {
    const { ctx, item, path, info } = props
    if (item.above) return null
    const isAsking = item.param !== ''
    // A relation is asked as a record picked from the source it points at (R19).
    const canAsk = askParamFor(info.field, item.op, 'probe', info.target) !== null
    if (!isAsking && !canAsk) return null
    const needed = askHighParamFor(info.field, item.op, 'probe') ? 2 : 1
    const isFull = !isAsking && ctx.params.length + needed > MAX_PARAMS
    const onChange = (on: boolean) => {
        if (!on) {
            setCondition(ctx, path, { param: '', paramHigh: '' })
            return
        }
        const taken = ctx.params.map(p => p.id)
        const id = nextColumnId(taken, columnHint(item.path) || item.col)
        const low = askParamFor(info.field, item.op, id, info.target)
        if (!low) return
        const asked = askHighParamFor(info.field, item.op, low.id)
        if (!asked) {
            setCondition(ctx, path, { param: low.id }, low)
            return
        }
        const high = { ...asked, id: nextColumnId([...taken, low.id], asked.id) }
        setCondition(ctx, path, { param: low.id, paramHigh: high.id }, low, high)
    }
    return (
        <View className="flex-row flex-wrap items-center gap-2">
            <Switch
                value={isAsking}
                onValueChange={onChange}
                isDisabled={isFull}
                accessibilityLabel="Ask when the report runs"
                testID={`${itemTestId(ctx, path)}-ask`}
            />
            <Text className="text-sm text-foreground">Ask when the report runs</Text>
            <DefaultPreset {...props} />
        </View>
    )
}

// In a fill under other groups, a value can be the key of a group above, so each
// parent shows its own records (a team's region is the region above, F21).
function AboveMenu({ ctx, item, path, info }: ValueProps) {
    const offered = ctx.above.filter(g => g.type === info.field.type || g.id === item.above)
    if (!needsValue(item.op) || item.op === 'in' || offered.length === 0) return null
    const items: PickItem[] = [
        {
            id: ':value',
            label: 'A value',
            isSelected: item.above === '',
            onSelect: () => setCondition(ctx, path, { above: '' }),
        },
        ...offered.map(g => ({
            id: g.id,
            label: `Same as the ${g.label} above`,
            isSelected: g.id === item.above,
            onSelect: () =>
                setCondition(ctx, path, {
                    above: g.id,
                    param: '',
                    paramHigh: '',
                    value: '',
                    high: '',
                }),
        })),
    ]
    const shown = items.find(i => i.isSelected)?.label ?? `${item.above} (missing)`
    return (
        <PickMenu
            label={shown}
            purpose={`${info.label} compared with`}
            items={items}
            testID={`${itemTestId(ctx, path)}-above`}
        />
    )
}

// An average's decimal places: whole cents for amounts (formula.go checkAggregate),
// else the author's, starting at the number default.
function TotalPlaces({ ctx, item, path, info }: ValueProps) {
    if (item.subject !== 'total' || item.aggregate !== 'avg') return null
    const id = itemTestId(ctx, path)
    if (info.field.type === 'money') {
        return (
            <Text className="text-xs text-muted-foreground" testID={`${id}-cents`}>
                Whole cents
            </Text>
        )
    }
    return (
        <FormulaPlacesPick
            places={item.scale ?? avgScaleFor(info.field.type)}
            onChange={scale => setCondition(ctx, path, { scale })}
            purpose={`${info.label} decimal places`}
            testID={`${id}-places`}
        />
    )
}

interface RowProps<T> {
    ctx: FilterContext
    item: T
    path: FilterPath
    baseKey: string
}

// A removed item takes the values its conditions asked for with it (ctx.change).
function RemoveButton({
    ctx,
    path,
    label,
}: {
    ctx: FilterContext
    path: FilterPath
    label: string
}) {
    return (
        <IconButton
            label={label}
            testID={`${itemTestId(ctx, path)}-remove`}
            isVisible
            icon={Trash2}
            onPress={() => ctx.change(removeFilterItem(ctx.root, path))}
        />
    )
}

const MISSING_MESSAGES = {
    field: 'A condition names a field that is no longer offered.',
    total: 'A condition names a field that is no longer offered.',
    column: 'A condition names a column that is no longer in the report.',
    record: 'A condition names a group that no longer links to a record.',
    link: 'A related-rows condition names a link that is no longer offered.',
}

function MissingRow({
    ctx,
    path,
    missing,
}: {
    ctx: FilterContext
    path: FilterPath
    missing: keyof typeof MISSING_MESSAGES
}) {
    return (
        <View
            testID={itemTestId(ctx, path)}
            className="flex-row items-center gap-2 rounded-md border border-border p-2"
        >
            <Text className="flex-1 text-sm text-muted-foreground">
                {MISSING_MESSAGES[missing]}
            </Text>
            <RemoveButton ctx={ctx} path={path} label="Remove condition" />
        </View>
    )
}

type SubjectProps = RowProps<ConditionDraft> & { info: ConditionInfo }

// A field of the rows, or of the records a fill shows.
function FieldSubject({ ctx, item, path, baseKey, info }: SubjectProps) {
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
function TotalSubject({ ctx, item, path, baseKey, info }: SubjectProps) {
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

// One of the report's columns, read on the group row.
function ColumnSubject({ ctx, item, path }: SubjectProps) {
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
function RecordSubject({ ctx, item, path, info }: SubjectProps) {
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

function newTotal(ctx: FilterContext, baseKey: string, path: SpecHop[]): ConditionDraft {
    const field = describePath(ctx.catalog, baseKey, path)?.field
    const offered = field ? aggregatesFor(field, false) : []
    const adds = field && (field.type === 'money' || field.type === 'number')
    const aggregate: Aggregate = adds && offered.includes('sum') ? 'sum' : 'count'
    return { ...emptyCondition(path), subject: 'total', aggregate, op: 'gt' }
}

function newColumnCondition(column: LevelScope['columns'][number]): ConditionDraft {
    return startCondition(column.field.type, { path: [], subject: 'column', col: column.key })
}

function newRecordCondition(
    ctx: FilterContext,
    record: FormulaRecord,
    path: SpecHop[]
): ConditionDraft {
    const type = typeAt(ctx.catalog, record.source, path)
    return startCondition(type, { path, subject: 'record', record: record.group })
}

function ConditionRow({ ctx, item, path, baseKey }: RowProps<ConditionDraft>) {
    const id = itemTestId(ctx, path)
    const info = conditionInfo(ctx, item, baseKey)
    if (!info) return <MissingRow ctx={ctx} path={path} missing={item.subject} />
    const type = info.field.type
    // A new operator starts without an asked value: the param's kind follows the
    // operator. Is any of, and an operator with no value, compare with no group above.
    const opItems: PickItem[] = opsShownFor(type, item.op).map(op => ({
        id: op,
        label: opLabel(op, type),
        isSelected: op === item.op,
        onSelect: () => {
            if (op === item.op) return
            const above = op === 'in' || !needsValue(op) ? '' : item.above
            setCondition(ctx, path, { op, param: '', paramHigh: '', above })
        },
    }))
    const subject = { ctx, item, path, baseKey, info }
    return (
        <View testID={id} className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row flex-wrap items-center gap-1.5">
                <FieldSubject {...subject} />
                <TotalSubject {...subject} />
                <ColumnSubject {...subject} />
                <RecordSubject {...subject} />
                <PickMenu
                    label={opLabel(item.op, type)}
                    purpose="Condition"
                    items={opItems}
                    testID={`${id}-op`}
                />
                <RemoveButton ctx={ctx} path={path} label="Remove condition" />
            </View>
            <TotalPlaces ctx={ctx} item={item} path={path} info={info} />
            <AboveMenu ctx={ctx} item={item} path={path} info={info} />
            <ConditionValue ctx={ctx} item={item} path={path} info={info} />
            <AskToggle ctx={ctx} item={item} path={path} info={info} />
        </View>
    )
}

function setHas(ctx: FilterContext, path: FilterPath, has: boolean) {
    ctx.change(
        updateFilterItem(ctx.root, path, item =>
            item.kind === 'related' ? { ...item, has } : item
        )
    )
}

// "Has / has no related <rows>" over a to-many link; its conditions are on the rows
// the link reaches, which have no level and no group above.
function RelatedRow({ ctx, item, path, baseKey }: RowProps<RelatedDraft>) {
    const id = itemTestId(ctx, path)
    const target = sourceAtLinks(ctx.catalog, baseKey, item.link)
    if (!target) return <MissingRow ctx={ctx} path={path} missing="link" />
    const link = ctx.catalog.get(baseKey)?.links.find(l => l.key === item.link[0]?.link)
    const hasItems: PickItem[] = [
        { id: 'yes', label: 'Has', isSelected: item.has, onSelect: () => setHas(ctx, path, true) },
        {
            id: 'no',
            label: 'Has no',
            isSelected: !item.has,
            onSelect: () => setHas(ctx, path, false),
        },
    ]
    const hasLabel = item.has ? 'Has' : 'Has no'
    const inner: FilterContext = { ...ctx, level: undefined, group: '', above: [] }
    return (
        <View testID={id} className="gap-1.5 rounded-md border border-border p-2">
            <View className="flex-row flex-wrap items-center gap-1.5">
                <PickMenu
                    label={hasLabel}
                    purpose="Related rows"
                    items={hasItems}
                    testID={`${id}-has`}
                />
                <Text className="flex-1 text-sm text-foreground">
                    {`related ${link?.label ?? 'rows'}`}
                </Text>
                <RemoveButton ctx={ctx} path={path} label="Remove related rows" />
            </View>
            <GroupBody ctx={inner} group={item.where} path={path} baseKey={target.key} />
        </View>
    )
}

function FixedRow({ ctx, path }: { ctx: FilterContext; path: FilterPath }) {
    return (
        <View
            testID={itemTestId(ctx, path)}
            className="flex-row items-center gap-2 rounded-md border border-border p-2"
        >
            <Text className="flex-1 text-sm text-muted-foreground">
                A condition the builder cannot edit. It is kept as it is.
            </Text>
            <RemoveButton ctx={ctx} path={path} label="Remove condition" />
        </View>
    )
}

function NestedGroup({ ctx, item, path, baseKey }: RowProps<GroupDraft>) {
    return (
        <View
            testID={itemTestId(ctx, path)}
            className="gap-1.5 rounded-md border border-border p-2"
        >
            <View className="flex-row justify-end">
                <RemoveButton ctx={ctx} path={path} label="Remove group" />
            </View>
            <GroupBody ctx={ctx} group={item} path={path} baseKey={baseKey} />
        </View>
    )
}

function ItemEditor({ ctx, item, path, baseKey }: RowProps<FilterItem>) {
    if (item.kind === 'condition')
        return <ConditionRow ctx={ctx} item={item} path={path} baseKey={baseKey} />
    if (item.kind === 'related')
        return <RelatedRow ctx={ctx} item={item} path={path} baseKey={baseKey} />
    if (item.kind === 'fixed') return <FixedRow ctx={ctx} path={path} />
    return <NestedGroup ctx={ctx} item={item} path={path} baseKey={baseKey} />
}

interface AdderProps {
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
function RowAdders({ ctx, path, baseKey }: AdderProps) {
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
    isOwn,
}: {
    ctx: FilterContext
    path: FilterPath
    record: FormulaRecord
    isOwn: boolean
}) {
    const id = itemTestId(ctx, path)
    const onPick = (next: SpecHop[]) =>
        ctx.change(addFilterItem(ctx.root, path, newRecordCondition(ctx, record, next)))
    return (
        <ColumnPicker
            catalog={ctx.catalog}
            baseKey={record.source}
            onPick={onPick}
            label={`Add ${record.label} condition`}
            testID={isOwn ? `${id}-add-record` : `${id}-add-record-${record.group}`}
            toOneOnly
            allowLinkEnd
        />
    )
}

// On a group level a condition compares a total of the level's rows, a column, or
// a field of a group's record (R18).
function LevelAdders({ ctx, path, baseKey }: AdderProps) {
    const level = ctx.level
    if (!level) return null
    const id = itemTestId(ctx, path)
    const add = (item: ConditionDraft) => ctx.change(addFilterItem(ctx.root, path, item))
    const columnItems: PickItem[] = level.columns.map(c => ({
        id: c.key,
        label: c.label,
        onSelect: () => add(newColumnCondition(c)),
    }))
    const records = level.records.map(record => (
        <RecordAdder
            key={record.group}
            ctx={ctx}
            path={path}
            record={record}
            isOwn={record.group === ctx.group}
        />
    ))
    return (
        <>
            <ColumnPicker
                catalog={ctx.catalog}
                baseKey={baseKey}
                onPick={next => add(newTotal(ctx, baseKey, next))}
                label="Add total condition"
                testID={`${id}-add-total`}
                toOneOnly
            />
            <PickMenu
                label="Add column condition"
                purpose="Column"
                items={columnItems}
                testID={`${id}-add-column`}
            />
            {records}
        </>
    )
}

function GroupBody({
    ctx,
    group,
    path,
    baseKey,
}: {
    ctx: FilterContext
    group: GroupDraft
    path: FilterPath
    baseKey: string
}) {
    const id = itemTestId(ctx, path)
    const matchItems: PickItem[] = MATCHES.map(m => ({
        id: m.value,
        label: m.label,
        isSelected: m.value === group.match,
        onSelect: () => ctx.change(setMatchAt(ctx.root, path, m.value)),
    }))
    const matchLabel = MATCHES.find(m => m.value === group.match)?.label ?? 'All of'
    const onGroup = () =>
        ctx.change(addFilterItem(ctx.root, path, { ...emptyGroup(), match: 'any' }))
    const keys = filterItemKeys(group.items)
    const items = group.items.map((item, i) => (
        <ItemEditor key={keys[i]} ctx={ctx} item={item} path={[...path, i]} baseKey={baseKey} />
    ))
    return (
        <View testID={`${id}-group`} className="gap-1.5">
            <PickMenu
                label={matchLabel}
                purpose="Join conditions"
                items={matchItems}
                testID={`${id}-match`}
            />
            {items}
            <View className="flex-row flex-wrap gap-1.5">
                <RowAdders ctx={ctx} path={path} baseKey={baseKey} />
                <LevelAdders ctx={ctx} path={path} baseKey={baseKey} />
                <Button testID={`${id}-add-group`} variant="outline" size="sm" onPress={onGroup}>
                    <ButtonText>Add group</ButtonText>
                </Button>
            </View>
        </View>
    )
}

function TreeError({ message, testID }: { message: string | undefined; testID: string }) {
    if (!message) return null
    return (
        <Text testID={testID} role="alert" className="text-xs text-danger">
            {message}
        </Text>
    )
}

/** A condition tree's editor: the filter, a level's having, or a fill's conditions. */
export function ConditionTree({
    builder,
    root,
    onChange,
    baseKey,
    testID,
    level,
    group = '',
    above = [],
    error,
}: ConditionTreeProps) {
    const params = useWatch({ control: builder.form.control, name: 'params' })
    const ctx: FilterContext = {
        builder,
        catalog: builder.catalog,
        root,
        params,
        testID,
        level,
        group,
        above,
        change: onChange,
    }
    return (
        <View className="gap-1.5">
            <TreeError message={error} testID={`${testID}-error`} />
            <View testID={testID}>
                <GroupBody ctx={ctx} group={root} path={[]} baseKey={baseKey} />
            </View>
        </View>
    )
}
