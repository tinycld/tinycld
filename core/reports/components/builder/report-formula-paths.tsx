import type { FormulaLeafProps } from '@tinycld/core/components/forms-builder/FormulaEditor'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { Fragment } from 'react'
import {
    type Catalog,
    describePath,
    pathLabel,
    switchableModeHop,
    withHopJoin,
    withHopMode,
} from '../../lib/builder/columns'
import { contextAt } from '../../lib/builder/formula-ops'
import { partOf, type ReportFormulaContext } from '../../lib/builder/formula-parts'
import type { SpecHop, SpecNode } from '../../lib/spec'
import { ColumnPicker } from './ColumnPicker'

// The report formula leaves that read a field by a path: a field of the source
// (or of a lookup's source), and a field of the record a group level carries. Each
// link of the path says whether rows without it are kept.

/** What a report formula leaf gets: the forms leaf props and what the formula may read. */
export type ReportLeafProps = FormulaLeafProps<SpecNode> & { ctx: ReportFormulaContext }

/** The link hops of a path, with each link's label, walked from `baseKey`. */
function linkHops(catalog: Catalog, baseKey: string, hops: readonly SpecHop[]) {
    let source = catalog.get(baseKey)
    return hops.flatMap((hop, index) => {
        if (hop.link === undefined) return []
        const link = source?.links.find(l => l.key === hop.link)
        source = link ? catalog.get(link.target) : undefined
        return [{ index, label: link?.label ?? hop.link }]
    })
}

interface HopJoinProps {
    hop: SpecHop
    index: number
    label: string
    hops: readonly SpecHop[]
    onChange: (hops: SpecHop[]) => void
    id: string
    where: string
}

// A link keeps the rows that have no linked record, or keeps only the rows that have
// one (`with: required`, bind.go hopPlan.required). Child rows show the linked rows
// there are, so a nest hop has none (bind.go hopProblem).
function HopJoin({ hop, index, label, hops, onChange, id, where }: HopJoinProps) {
    if (hop.mode === 'nest') return null
    const isRequired = hop.with === 'required'
    const choose = (required: boolean) => {
        if (required !== isRequired) onChange(withHopJoin(hops, index, required))
    }
    const items: PickItem[] = [
        {
            id: 'keep',
            label: `Keep rows without ${label}`,
            isSelected: !isRequired,
            onSelect: () => choose(false),
        },
        {
            id: 'required',
            label: `Only rows with ${label}`,
            isSelected: isRequired,
            onSelect: () => choose(true),
        },
    ]
    const shown = isRequired ? items[1] : items[0]
    return (
        <PickMenu
            label={shown?.label ?? ''}
            purpose={`Rows without ${label}${where}`}
            items={items}
            testID={`${id}-join-${index}`}
        />
    )
}

const MODE_LABELS = { aggregate: 'Total per row', nest: 'Child rows' } as const

// A column's one to-many link is a total per row, or child rows (R1).
function HopMode({ hop, index, label, hops, onChange, id, where }: HopJoinProps) {
    const current = hop.mode ?? 'aggregate'
    const items: PickItem[] = (['aggregate', 'nest'] as const).map(mode => ({
        id: mode,
        label: MODE_LABELS[mode],
        isSelected: mode === current,
        onSelect: () => {
            if (mode !== current) onChange(withHopMode(hops, index, mode))
        },
    }))
    return (
        <PickMenu
            label={`${label}: ${MODE_LABELS[current]}`}
            purpose={`${label} shown as${where}`}
            items={items}
            testID={`${id}-mode-${index}`}
        />
    )
}

function HopModeChoice({ isVisible, ...props }: HopJoinProps & { isVisible: boolean }) {
    if (!isVisible) return null
    return <HopMode {...props} />
}

interface HopJoinsProps {
    catalog: Catalog
    baseKey: string
    hops: readonly SpecHop[]
    onChange: (hops: SpecHop[]) => void
    id: string
    where: string
}

/**
 * Each link of a path: whether rows without it are kept and, for a column's one
 * to-many link, whether it is a total or child rows (no-limits ruling: every path
 * the builder writes is editable).
 */
export function HopJoins({ catalog, baseKey, hops, onChange, id, where }: HopJoinsProps) {
    const switchable = switchableModeHop(catalog, baseKey, hops)
    const controls = linkHops(catalog, baseKey, hops).map(({ index, label }) => {
        const hop = hops[index] ?? {}
        const props = { hop, index, label, hops, onChange, id, where }
        return (
            <Fragment key={index}>
                <HopModeChoice {...props} isVisible={index === switchable} />
                <HopJoin {...props} />
            </Fragment>
        )
    })
    return <>{controls}</>
}

export function PathLeaf({ ctx, root, path, expr, set, id, where }: ReportLeafProps) {
    if (partOf(expr) !== 'path') return null
    const here = contextAt(ctx, root, path)
    if (!here.source) return null
    const hops = expr.path ?? []
    const info = describePath(here.catalog, here.source, hops)
    const setPath = (next: SpecHop[]) => set({ path: next })
    const label = info ? pathLabel(info) : 'Choose a field'
    return (
        <>
            <ColumnPicker
                catalog={here.catalog}
                baseKey={here.source}
                onPick={setPath}
                label={label}
                testID={`${id}-path`}
                toOneOnly
            />
            <HopJoins
                catalog={here.catalog}
                baseKey={here.source}
                hops={hops}
                onChange={setPath}
                id={id}
                where={where}
            />
        </>
    )
}

interface RecordFieldProps {
    isVisible: boolean
    catalog: Catalog
    source: string
    hops: readonly SpecHop[]
    label: string
    onPick: (path: SpecHop[]) => void
    id: string
    where: string
}

function RecordField({
    isVisible,
    catalog,
    source,
    hops,
    label,
    onPick,
    id,
    where,
}: RecordFieldProps) {
    if (!isVisible) return null
    return (
        <>
            <ColumnPicker
                catalog={catalog}
                baseKey={source}
                onPick={onPick}
                label={label}
                testID={`${id}-record-field`}
                toOneOnly
            />
            <HopJoins
                catalog={catalog}
                baseKey={source}
                hops={hops}
                onChange={onPick}
                id={id}
                where={where}
            />
        </>
    )
}

// A field of the record a group level carries (a group by a link to one record).
export function RecordLeaf({ ctx, root, path, expr, set, id, where }: ReportLeafProps) {
    if (partOf(expr) !== 'record') return null
    const here = contextAt(ctx, root, path)
    const record = here.records.find(r => r.group === expr.record)
    const hops = expr.path ?? []
    const groups: PickItem[] = here.records.map(r => ({
        id: r.group,
        label: r.label,
        isSelected: r.group === expr.record,
        onSelect: () => set({ record: r.group, path: hops }),
    }))
    const info = record ? describePath(here.catalog, record.source, hops) : null
    const fieldLabel = info ? pathLabel(info) : 'Choose a field'
    return (
        <>
            <PickMenu
                label={record?.label ?? `${expr.record} (missing)`}
                purpose={`Group${where}`}
                items={groups}
                testID={`${id}-record-group`}
            />
            <RecordField
                isVisible={record !== undefined}
                catalog={here.catalog}
                source={record?.source ?? ''}
                hops={hops}
                label={fieldLabel}
                onPick={next => set({ record: expr.record, path: next })}
                id={id}
                where={where}
            />
        </>
    )
}
