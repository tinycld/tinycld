import {
    FormulaEditor,
    FormulaNumberInput,
    type FormulaOperators,
    FormulaPlacesPick,
    FormulaValuePick,
} from '@tinycld/core/components/forms-builder/FormulaEditor'
import { RULE_INPUT_CLASS } from '@tinycld/core/components/forms-builder/rule-parts'
import { treeAt } from '@tinycld/core/lib/forms/builder/formula-tree'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useWatch } from '@tinycld/core/ui/form'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { type PickItem, PickMenu } from '@tinycld/core/ui/pick-menu'
import { useState } from 'react'
import { Text } from 'react-native'
import type { ReportBuilder } from '../../hooks/useReportBuilder'
import type { FormulaDraft } from '../../lib/builder/draft'
import { columnChoices, type FormulaPlace, formulaContext } from '../../lib/builder/formula-context'
import { describeNode, linkLabel } from '../../lib/builder/formula-describe'
import {
    addOperandNode,
    addOtherwise,
    canRemoveNode,
    contextAt,
    forcedScaleAt,
    hasOtherwise,
    isNumberList,
    isVariadicNode,
    linksOf,
    paramsAt,
    partLabel,
    partsAt,
    removeOperandNode,
    removeOtherwise,
    settleScales,
    slotLabel,
    withPart,
} from '../../lib/builder/formula-ops'
import {
    boundChoices,
    isReportPart,
    LOOKUP_PICKS,
    partOf,
    REPORT_TREE,
    type ReportFormulaContext,
    SCALED_PARTS,
} from '../../lib/builder/formula-parts'
import { lookupPicks, typesOf } from '../../lib/builder/formula-type'
import { type SpecNode, specNodeSchema } from '../../lib/spec'
import { FixedFormula, NEW_FORMULA } from './parts'
import { OuterLeaf, PathLeaf, RecordLeaf, type ReportLeafProps } from './report-formula-paths'

// The report builder's formula parts (OQ2): the operator set the FormulaEditor
// draws for a column, a layout computed row, a group level or a row label, with
// the controls beside each part. The parts and their editing rules are in
// lib/builder/formula-*.ts; the number, value and places views are the forms
// editor's own (F15). The leaves that read a field by a path are in
// report-formula-paths.tsx.

type LeafProps = ReportLeafProps

function NumberLeaf({ expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'number') return null
    return (
        <FormulaNumberInput
            value={Number(expr.value)}
            onChange={n => set({ value: n })}
            label={`Number${where}`}
            testID={`${id}-number`}
        />
    )
}

function TextLeaf({ expr, set, id, where }: LeafProps) {
    const placeholderColor = useThemeColor('field-placeholder')
    if (partOf(expr) !== 'text') return null
    return (
        <PlainInput
            value={String(expr.value)}
            onChangeText={text => set({ value: text })}
            placeholder="Text"
            placeholderTextColor={placeholderColor}
            accessibilityLabel={`Text${where}`}
            testID={`${id}-text`}
            className={RULE_INPUT_CLASS}
        />
    )
}

type ListItem = string | number

const listText = (items: readonly unknown[]) => items.map(String).join(', ')

function readList(text: string, numbers: boolean): ListItem[] {
    const parts = text
        .split(',')
        .map(s => s.trim())
        .filter(s => s !== '')
    return numbers ? parts.map(Number).filter(Number.isFinite) : parts
}

const sameList = (a: readonly unknown[], b: readonly unknown[]) =>
    a.length === b.length && a.every((item, i) => item === b[i])

interface ListTextProps {
    items: readonly unknown[]
    numbers: boolean
    onChange: (items: ListItem[]) => void
    label: string
    testID: string
}

function ListText({ items, numbers, onChange, label, testID }: ListTextProps) {
    const placeholderColor = useThemeColor('field-placeholder')
    // The typed text is local, so "a," can pass on its way to the next item. A
    // reset or another part's edit replaces the list under it: then show the list.
    const [text, setText] = useState(listText(items))
    const shown = sameList(readList(text, numbers), items) ? text : listText(items)
    const placeholder = numbers ? '1, 2, 3' : 'a, b, c'
    const change = (next: string) => {
        setText(next)
        onChange(readList(next, numbers))
    }
    return (
        <PlainInput
            value={shown}
            onChangeText={change}
            placeholder={placeholder}
            placeholderTextColor={placeholderColor}
            accessibilityLabel={label}
            testID={testID}
            className={RULE_INPUT_CLASS}
        />
    )
}

// The values Is any of compares with: numbers when it tests a number, else texts.
function ListLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'list') return null
    const numbers = isNumberList(ctx, root, path)
    const noun = numbers ? 'Numbers' : 'Texts'
    const items = Array.isArray(expr.value) ? expr.value : []
    return (
        <ListText
            items={items}
            numbers={numbers}
            onChange={items => set({ value: items })}
            label={`${noun}, separated by commas${where}`}
            testID={`${id}-list`}
        />
    )
}

function YesNoLeaf({ expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'yesno') return null
    const items: PickItem[] = [true, false].map(v => ({
        id: String(v),
        label: v ? 'Yes' : 'No',
        isSelected: expr.value === v,
        onSelect: () => set({ value: v }),
    }))
    const label = expr.value ? 'Yes' : 'No'
    return (
        <PickMenu
            label={label}
            purpose={`Yes or no${where}`}
            items={items}
            testID={`${id}-yesno`}
        />
    )
}

function ValueLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'field') return null
    const here = contextAt(ctx, root, path)
    const key = expr.col ?? expr.subtotal ?? ''
    const choose = (picked: string) =>
        set(here.leaf === 'col' ? { col: picked } : { subtotal: picked })
    return (
        <FormulaValuePick
            choices={here.values}
            selected={key}
            onChange={choose}
            purpose={`${partLabel(here, 'field')}${where}`}
            testID={`${id}-field`}
        />
    )
}

// The asked values that fit the slot: a Period takes a date range, Is any of a
// multiple choice, any other slot a single value (params.go paramExpr).
function ParamLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'param') return null
    const here = contextAt(ctx, root, path)
    const items: PickItem[] = paramsAt(ctx, root, path).map(p => ({
        id: p.id,
        label: p.label ?? p.id,
        isSelected: p.id === expr.param,
        onSelect: () => set({ param: p.id }),
    }))
    const param = here.params.find(p => p.id === expr.param)
    const label = param ? (param.label ?? param.id) : `${expr.param} (missing)`
    return (
        <PickMenu
            label={label}
            purpose={`Asked value${where}`}
            items={items}
            testID={`${id}-param`}
        />
    )
}

// The first or last day of a date range param (params.go paramExpr bound).
function BoundLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'bound') return null
    const here = contextAt(ctx, root, path)
    const choices = boundChoices(here.params)
    const isCurrent = (c: (typeof choices)[number]) =>
        c.param === expr.param && c.bound === expr.bound
    const items: PickItem[] = choices.map(c => ({
        id: `${c.param}-${c.bound}`,
        label: c.label,
        isSelected: isCurrent(c),
        onSelect: () => set({ param: c.param, bound: c.bound }),
    }))
    const shown = choices.find(isCurrent)?.label ?? `${expr.param} (missing)`
    return (
        <PickMenu
            label={shown}
            purpose={`Range start or end${where}`}
            items={items}
            testID={`${id}-bound`}
        />
    )
}

function WholeCents({ isVisible, testID }: { isVisible: boolean; testID: string }) {
    if (!isVisible) return null
    return (
        <Text className="text-xs text-muted-foreground" testID={testID}>
            Whole cents
        </Text>
    )
}

interface PlacesChoiceProps {
    isVisible: boolean
    places: number
    onChange: (n: number) => void
    purpose: string
    testID: string
}

function PlacesChoice({ isVisible, ...props }: PlacesChoiceProps) {
    if (!isVisible) return null
    return <FormulaPlacesPick {...props} />
}

// An amount divided by a number, or an average of amounts, keeps whole cents
// (formula.go div, checkAggregate): its places are not the author's to pick.
function PlacesLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (!SCALED_PARTS.has(partOf(expr))) return null
    const forced = forcedScaleAt(ctx, root, path)
    return (
        <>
            <WholeCents isVisible={forced !== undefined} testID={`${id}-cents`} />
            <PlacesChoice
                isVisible={forced === undefined}
                places={expr.scale ?? 0}
                onChange={n => set({ ...expr, scale: n })}
                purpose={`Decimal places${where}`}
                testID={`${id}-places`}
            />
        </>
    )
}

function LookupLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'lookup') return null
    const pick = expr.pick ?? 'min'
    const sources: PickItem[] = [...ctx.catalog.values()].map(s => {
        const isSelected = s.key === expr.source
        return {
            id: s.key,
            label: s.label,
            isSelected,
            // Another source reads other fields: its where and value start again.
            onSelect: () => {
                if (isSelected) return
                set({ op: 'lookup', source: s.key, value: { path: [{ field: 'id' }] }, pick })
            },
        }
    })
    const offered = lookupPicks(expr, typesOf(contextAt(ctx, root, path)))
    const picks: PickItem[] = offered.map(p => ({
        id: p.value,
        label: p.label,
        isSelected: pick === p.value,
        onSelect: () => set({ ...expr, pick: p.value }),
    }))
    const sourceLabel = ctx.catalog.get(expr.source ?? '')?.label ?? `${expr.source} (missing)`
    const pickLabel = LOOKUP_PICKS.find(p => p.value === pick)?.label ?? pick
    return (
        <>
            <PickMenu
                label={sourceLabel}
                purpose={`Source${where}`}
                items={sources}
                testID={`${id}-source`}
            />
            <PickMenu
                label={pickLabel}
                purpose={`Which match${where}`}
                items={picks}
                testID={`${id}-pick`}
            />
        </>
    )
}

// Has related / Has no related, inside a lookup: the link to the looked-up rows'
// related rows (lookup.go lookupCtx.exists).
function ExistsLeaf({ ctx, root, path, expr, set, id, where }: LeafProps) {
    const part = partOf(expr)
    if (part !== 'exists' && part !== 'notExists') return null
    const here = contextAt(ctx, root, path)
    const current = expr.link ?? []
    const items: PickItem[] = linksOf(here).map(link => {
        const isSelected = current.length === 1 && current[0]?.link === link.key
        return {
            id: link.key,
            label: link.label,
            isSelected,
            // Another link reaches other rows: its where starts again.
            onSelect: () => {
                if (!isSelected) set({ op: expr.op, link: [{ link: link.key }] })
            },
        }
    })
    return (
        <PickMenu
            label={linkLabel(here.catalog, here.source, current)}
            purpose={`Related rows${where}`}
            items={items}
            testID={`${id}-link`}
        />
    )
}

interface SmallButtonProps {
    isVisible: boolean
    text: string
    label: string
    onPress: () => void
    testID: string
}

function SmallButton({ isVisible, text, label, onPress, testID }: SmallButtonProps) {
    if (!isVisible) return null
    return (
        <Button
            variant="outline"
            size="sm"
            onPress={onPress}
            accessibilityLabel={label}
            testID={testID}
        >
            <ButtonText>{text}</ButtonText>
        </Button>
    )
}

// A First match gives empty when nothing matches, or its Otherwise (formula.go case).
function OtherwiseLeaf({ expr, set, id, where }: LeafProps) {
    if (partOf(expr) !== 'case') return null
    const has = hasOtherwise(expr)
    return (
        <>
            <SmallButton
                isVisible={!has}
                text="Add otherwise"
                label={`Add an Otherwise to First match${where}`}
                onPress={() => set(addOtherwise(expr))}
                testID={`${id}-otherwise-add`}
            />
            <SmallButton
                isVisible={has}
                text="Remove otherwise"
                label={`Remove the Otherwise of First match${where}`}
                onPress={() => set(removeOtherwise(expr))}
                testID={`${id}-otherwise-remove`}
            />
        </>
    )
}

function ReportFormulaLeaf(props: LeafProps) {
    return (
        <>
            <NumberLeaf {...props} />
            <TextLeaf {...props} />
            <ListLeaf {...props} />
            <YesNoLeaf {...props} />
            <ValueLeaf {...props} />
            <PathLeaf {...props} />
            <OuterLeaf {...props} />
            <RecordLeaf {...props} />
            <ParamLeaf {...props} />
            <BoundLeaf {...props} />
            <PlacesLeaf {...props} />
            <LookupLeaf {...props} />
            <ExistsLeaf {...props} />
            <OtherwiseLeaf {...props} />
        </>
    )
}

function parseNode(value: unknown): SpecNode {
    const parsed = specNodeSchema.safeParse(value)
    return parsed.success ? parsed.data : { value: 0 }
}

/**
 * The formula context of a place in the builder, read with useWatch so the menus
 * follow edits made in other sections (F27); never getValues in render.
 */
export function useFormulaContext(
    builder: ReportBuilder,
    place: FormulaPlace
): ReportFormulaContext {
    const control = builder.form.control
    const source = useWatch({ control, name: 'source' })
    const columns = useWatch({ control, name: 'columns' })
    const groups = useWatch({ control, name: 'groups' })
    const params = useWatch({ control, name: 'params' })
    const layout = useWatch({ control, name: 'layout' })
    return formulaContext({ source, columns, groups, params, layout }, builder.catalog, place)
}

/**
 * The columns with their types, read with useWatch (F27). Every section reads them
 * here, so a formula column has the type its formula menus and the saved spec give
 * it (records and params included).
 */
export function useColumnChoices(builder: ReportBuilder) {
    const control = builder.form.control
    const columns = useWatch({ control, name: 'columns' })
    const groups = useWatch({ control, name: 'groups' })
    const params = useWatch({ control, name: 'params' })
    return columnChoices({ source: builder.base.key, columns, groups, params }, builder.catalog)
}

/** The report builder's formula parts for the FormulaEditor's operators prop (OQ2). */
export function reportOperators(ctx: ReportFormulaContext): FormulaOperators<SpecNode> {
    return {
        ...REPORT_TREE,
        parse: parseNode,
        partOf,
        partsAt: (root, path) => partsAt(ctx, root, path),
        label: part => partLabel(ctx, part),
        withPart: (root, path, part) =>
            isReportPart(part)
                ? withPart(ctx, root, path, part)
                : (treeAt(REPORT_TREE, root, path) ?? root),
        slotLabel,
        isVariadic: isVariadicNode,
        canRemove: canRemoveNode,
        addOperand: addOperandNode,
        removeOperand: removeOperandNode,
        describe: node => describeNode(ctx, node),
        renderLeaf: props => <ReportFormulaLeaf {...props} ctx={ctx} />,
    }
}

interface ReportFormulaProps {
    builder: ReportBuilder
    place: FormulaPlace
    formula: FormulaDraft
    onChange: (formula: FormulaDraft) => void
    testID: string
}

/**
 * A formula in the report builder: its parts picked from menus, or, when it has
 * parts the editor cannot show, kept as it is with a Remove (R17).
 */
export function ReportFormula({ builder, place, formula, onChange, testID }: ReportFormulaProps) {
    const ctx = useFormulaContext(builder, place)
    if (formula.kind === 'fixed') {
        return <FixedFormula testID={`${testID}-fixed`} onRemove={() => onChange(NEW_FORMULA)} />
    }
    // Settled against the formula before the edit, so a scale the engine forced
    // and no longer forces is let go (formula-ops.ts settleScales).
    const change = (node: SpecNode) =>
        onChange({ kind: 'editable', node: settleScales(ctx, node, formula.node) })
    return (
        <FormulaEditor
            operators={reportOperators(ctx)}
            value={formula.node}
            onChange={change}
            testID={testID}
        />
    )
}
