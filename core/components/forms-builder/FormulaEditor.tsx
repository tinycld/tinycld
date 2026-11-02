import {
    addOperand,
    asExpr,
    canRemoveOperand,
    childrenOf,
    describeFormula,
    exprAt,
    type FormulaOperands,
    type FormulaPath,
    isFormulaPart,
    isVariadic,
    PART_LABELS,
    partOf,
    partsFor,
    removeOperand,
    setAggregateColumn,
    setAggregateTable,
    setPlaces,
    withChildren,
    withPart,
} from '@tinycld/core/lib/forms/builder'
import {
    type FormulaTree,
    replaceTreeAt,
    treeAt,
} from '@tinycld/core/lib/forms/builder/formula-tree'
import type { ComputeExpr } from '@tinycld/core/lib/forms/expression'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Fragment, type ReactNode } from 'react'
import { Text, View } from 'react-native'
import { NumberText, type PickItem, PickMenu, RemoveButton } from './rule-parts'

const PLACES = [0, 1, 2, 3, 4, 5, 6]

/** What a leaf's controls get: the part, how to replace it, and its test id stem. */
export interface FormulaLeafProps<E> {
    root: E
    path: FormulaPath
    expr: E
    set: (next: E) => void
    id: string
    /** ", part 1.2" for a screen reader; "" for the whole formula. */
    where: string
}

/**
 * The parts a formula editor offers and how each behaves. Forms leave it out and
 * get Number, Field, the arithmetic and the table totals (computeOperators); the
 * report builder passes the report operators.
 */
export interface FormulaOperators<E> extends FormulaTree<E> {
    parse(value: unknown): E
    partOf(expr: E): string
    partsAt(root: E, path: FormulaPath): readonly string[]
    label(part: string): string
    /**
     * The part at `path` changed to another kind: returns the replacement for that
     * part (operands carried over where they can be), not a new root. The editor
     * puts it in place with `replaceTreeAt`.
     */
    withPart(root: E, path: FormulaPath, part: string): E
    /** A caption above a part ("When", "Then"), or null. */
    slotLabel(expr: E, index: number): string | null
    isVariadic(expr: E): boolean
    canRemove(expr: E): boolean
    addOperand(expr: E): E
    removeOperand(expr: E, index: number): E
    describe(expr: E): string
    /**
     * The controls beside a part's menu. Called as a function, not drawn as a
     * component: an operator set is rebuilt each render, and a new component
     * type would remount its inputs and drop the caret while typing.
     */
    renderLeaf(props: FormulaLeafProps<E>): ReactNode
}

interface Common {
    /** The stored formula; a draft holds it untyped. */
    value: unknown
    testID: string
}

/** Without `operators`, today's forms editor; with them, that operator set's own tree (R16). */
export type FormulaEditorProps<E = ComputeExpr> =
    | (Common & {
          operators?: undefined
          onChange: (expr: ComputeExpr) => void
          operands: FormulaOperands
          /** What a Field part is called: "Field", or "Column" in a column's formula. */
          valueNoun: string
      })
    | (Common & {
          operators: FormulaOperators<E>
          onChange: (expr: E) => void
          // The operator set says what a part reads; the forms props would be ignored.
          operands?: never
          valueNoun?: never
      })

// The leaf views both operator sets draw (F15): a typed number, a value
// picked from a list, and decimal places.

/** A number typed into a formula part. A cleared number is 0, the value the engines read for nothing. */
export function FormulaNumberInput({
    value,
    onChange,
    label,
    testID,
}: {
    value: number
    onChange: (n: number) => void
    label: string
    testID: string
}) {
    return (
        <NumberText
            value={value}
            onChange={n => onChange(n ?? 0)}
            label={label}
            placeholder="0"
            emptyAs={0}
            testID={testID}
        />
    )
}

/** A value a part reads: a field, a column or a section, by key. */
export function FormulaValuePick({
    choices,
    selected,
    onChange,
    purpose,
    testID,
}: {
    choices: readonly { key: string; label: string }[]
    selected: string
    onChange: (key: string) => void
    purpose: string
    testID: string
}) {
    const items: PickItem[] = choices.map(c => ({
        id: c.key,
        label: c.label,
        isSelected: c.key === selected,
        onSelect: () => onChange(c.key),
    }))
    const label = choices.find(c => c.key === selected)?.label ?? `${selected} (missing)`
    return <PickMenu label={label} purpose={purpose} items={items} testID={testID} />
}

/**
 * The decimal places of a part that keeps a scale: a forms Round, or a report
 * formula's Round, Divide, Average or percent. A forms Divide has no places.
 */
export function FormulaPlacesPick({
    places,
    onChange,
    purpose,
    testID,
}: {
    places: number
    onChange: (n: number) => void
    purpose: string
    testID: string
}) {
    const items: PickItem[] = PLACES.map(n => ({
        id: String(n),
        label: String(n),
        isSelected: n === places,
        onSelect: () => onChange(n),
    }))
    const label = places === 1 ? '1 place' : `${places} places`
    return <PickMenu label={label} purpose={purpose} items={items} testID={testID} />
}

type ComputeLeafProps = FormulaLeafProps<ComputeExpr> & {
    operands: FormulaOperands
    valueNoun: string
}

function NumberPart({ expr, set, id, where }: ComputeLeafProps) {
    if (typeof expr !== 'number') return null
    return (
        <FormulaNumberInput
            value={expr}
            onChange={set}
            label={`Number${where}`}
            testID={`${id}-number`}
        />
    )
}

function ValuePick({ expr, set, id, where, operands, valueNoun }: ComputeLeafProps) {
    if (typeof expr !== 'object' || !('var' in expr)) return null
    return (
        <FormulaValuePick
            choices={operands.values}
            selected={expr.var}
            onChange={key => set({ var: key })}
            purpose={`${valueNoun}${where}`}
            testID={`${id}-field`}
        />
    )
}

function PlacesPick({ expr, set, id, where }: ComputeLeafProps) {
    if (typeof expr !== 'object' || !('round' in expr)) return null
    return (
        <FormulaPlacesPick
            places={expr.round[1]}
            onChange={n => set(setPlaces(expr, n))}
            purpose={`Decimal places${where}`}
            testID={`${id}-places`}
        />
    )
}

function ColumnPick({
    isVisible,
    label,
    purpose,
    items,
    testID,
}: {
    isVisible: boolean
    label: string
    purpose: string
    items: PickItem[]
    testID: string
}) {
    if (!isVisible) return null
    return <PickMenu label={label} purpose={purpose} items={items} testID={testID} />
}

/** Sum of a column: its table and column. Number of rows: its table. */
function TotalPick({ expr, set, id, where, operands }: ComputeLeafProps) {
    if (typeof expr !== 'object' || !('sumOf' in expr || 'count' in expr)) return null
    const isSum = 'sumOf' in expr
    const tableKey = isSum ? expr.sumOf[0].var : expr.count[0].var
    const columnKey = isSum ? expr.sumOf[1] : ''
    const { tables } = operands
    const table = tables.find(t => t.key === tableKey)
    const offered = isSum ? tables.filter(t => t.columns.length > 0) : tables
    const tableItems: PickItem[] = offered.map(t => ({
        id: t.key,
        label: t.label,
        isSelected: t.key === tableKey,
        onSelect: () => set(setAggregateTable(expr, t.key, operands)),
    }))
    const columnItems: PickItem[] = (table?.columns ?? []).map(c => ({
        id: c.key,
        label: c.label,
        isSelected: c.key === columnKey,
        onSelect: () => set(setAggregateColumn(expr, c.key)),
    }))
    const columnLabel =
        table?.columns.find(c => c.key === columnKey)?.label ?? `${columnKey} (missing)`
    return (
        <>
            <PickMenu
                label={table?.label ?? `${tableKey} (missing)`}
                purpose={`Table${where}`}
                items={tableItems}
                testID={`${id}-table`}
            />
            <ColumnPick
                isVisible={isSum}
                label={columnLabel}
                purpose={`Column${where}`}
                items={columnItems}
                testID={`${id}-column`}
            />
        </>
    )
}

function ComputeLeaf(props: ComputeLeafProps) {
    return (
        <>
            <NumberPart {...props} />
            <ValuePick {...props} />
            <PlacesPick {...props} />
            <TotalPick {...props} />
        </>
    )
}

/** The forms engine's parts: Number, Field, the arithmetic and the table totals. */
export function computeOperators(
    operands: FormulaOperands,
    valueNoun: string
): FormulaOperators<ComputeExpr> {
    const label = (part: string) => {
        if (part === 'field') return valueNoun
        return isFormulaPart(part) ? PART_LABELS[part] : part
    }
    return {
        parse: asExpr,
        partOf,
        childrenOf,
        withChildren,
        partsAt: () => partsFor(operands),
        label,
        withPart: (root, path, part) => {
            const current = exprAt(root, path) ?? 0
            return isFormulaPart(part) ? withPart(current, part, operands) : current
        },
        slotLabel: () => null,
        isVariadic,
        canRemove: canRemoveOperand,
        addOperand,
        removeOperand,
        describe: expr => describeFormula(expr, operands),
        renderLeaf: props => <ComputeLeaf {...props} operands={operands} valueNoun={valueNoun} />,
    }
}

/** Every edit replaces one part of the formula by its path; see lib/forms/builder/formula-tree.ts. */
interface Tree<E> {
    operators: FormulaOperators<E>
    root: E
    testID: string
    set: (path: FormulaPath, next: E) => void
}

const idOf = (testID: string, path: FormulaPath) =>
    `${testID}-${path.length ? path.join('-') : 'root'}`

/** A part's place, for a screen reader: "2", or "1.2" for the second part of the first. */
const placeOf = (path: FormulaPath) => path.map(i => i + 1).join('.')

/** Repeated controls read the same; this suffix tells them apart. The whole formula has none. */
const whereOf = (path: FormulaPath) => (path.length ? `, part ${placeOf(path)}` : '')

/** Removes a part from the one that holds it; `label` names both. */
interface Removal {
    onPress: () => void
    label: string
}

function RemovePart({ removal, testID }: { removal: Removal | null; testID: string }) {
    if (!removal) return null
    return <RemoveButton label={removal.label} onPress={removal.onPress} testID={testID} />
}

function AddPart({
    isVisible,
    onPress,
    label,
    testID,
}: {
    isVisible: boolean
    onPress: () => void
    label: string
    testID: string
}) {
    if (!isVisible) return null
    return (
        <Button
            variant="outline"
            size="sm"
            onPress={onPress}
            className="self-start"
            accessibilityLabel={label}
            testID={testID}
        >
            <ButtonText>Add part</ButtonText>
        </Button>
    )
}

function SlotLabel({ text }: { text: string | null }) {
    if (!text) return null
    return <Text className="text-xs text-muted-foreground">{text}</Text>
}

/**
 * The parts a part holds, as nodes. A part has no id: its path is its
 * identity. The count and the part are in the key, so a removal or a new kind
 * of part remounts the inputs below rather than keeping a neighbour's typed text.
 */
function Operands<E>({
    tree,
    path,
    expr,
    id,
}: {
    tree: Tree<E>
    path: FormulaPath
    expr: E
    id: string
}) {
    const { operators } = tree
    const parts = operators.childrenOf(expr)
    if (parts.length === 0) return null
    const holder = operators.label(operators.partOf(expr))
    const canRemove = operators.canRemove(expr)
    const removalOf = (index: number, childPath: FormulaPath): Removal | null => {
        if (!canRemove) return null
        return {
            onPress: () => tree.set(path, operators.removeOperand(expr, index)),
            label: `Remove part ${placeOf(childPath)} of ${holder}`,
        }
    }
    const nodes = parts.map((part, index) => {
        const childPath = [...path, index]
        return {
            childPath,
            removal: removalOf(index, childPath),
            slot: operators.slotLabel(expr, index),
            key: `${idOf(tree.testID, childPath)}:${operators.partOf(part)}:${parts.length}`,
        }
    })
    const items = nodes.map(node => (
        <Fragment key={node.key}>
            <SlotLabel text={node.slot} />
            <FormulaNode tree={tree} path={node.childPath} removal={node.removal} />
        </Fragment>
    ))
    return (
        <View className="pl-3 border-l border-border gap-1.5">
            {items}
            <AddPart
                isVisible={operators.isVariadic(expr)}
                onPress={() => tree.set(path, operators.addOperand(expr))}
                label={`Add part to ${holder}${whereOf(path)}`}
                testID={`${id}-add`}
            />
        </View>
    )
}

function FormulaNode<E>({
    tree,
    path,
    removal,
}: {
    tree: Tree<E>
    path: FormulaPath
    /** Null where this part cannot go. */
    removal: Removal | null
}) {
    const expr = treeAt(tree.operators, tree.root, path)
    if (expr === undefined) return null
    const { operators } = tree
    const part = operators.partOf(expr)
    const id = idOf(tree.testID, path)
    const partItems: PickItem[] = operators.partsAt(tree.root, path).map(p => ({
        id: p,
        label: operators.label(p),
        isSelected: p === part,
        onSelect: () => tree.set(path, operators.withPart(tree.root, path, p)),
    }))
    const leaf = operators.renderLeaf({
        root: tree.root,
        path,
        expr,
        set: next => tree.set(path, next),
        id,
        where: whereOf(path),
    })
    const purpose = path.length ? `Part ${placeOf(path)}` : 'Formula part'
    return (
        <View className="gap-1.5" testID={id}>
            <View className="flex-row flex-wrap items-center gap-1.5">
                <PickMenu
                    label={operators.label(part)}
                    purpose={purpose}
                    items={partItems}
                    testID={`${id}-part`}
                />
                {leaf}
                <RemovePart removal={removal} testID={`${id}-remove`} />
            </View>
            <Operands tree={tree} path={path} expr={expr} id={id} />
        </View>
    )
}

function FormulaTreeEditor<E>({
    operators,
    value,
    onChange,
    testID,
}: {
    operators: FormulaOperators<E>
    value: unknown
    onChange: (expr: E) => void
    testID: string
}) {
    const root = operators.parse(value)
    const tree: Tree<E> = {
        operators,
        root,
        testID,
        set: (path, next) => onChange(replaceTreeAt(operators, root, path, next)),
    }
    const summary = operators.describe(root)
    return (
        <View className="mb-3 gap-1.5" testID={testID}>
            <Text className="text-sm font-semibold text-foreground">Formula</Text>
            <Text
                className="text-sm text-foreground"
                accessibilityLabel={`Formula: ${summary}`}
                testID={`${testID}-summary`}
            >
                {`= ${summary}`}
            </Text>
            <FormulaNode key={operators.partOf(root)} tree={tree} path={[]} removal={null} />
        </View>
    )
}

/**
 * A formula built from parts picked from menus: no formula is typed, only a
 * number (or a text) where such a part is picked. The line on top reads it back.
 */
export function FormulaEditor<E = ComputeExpr>(props: FormulaEditorProps<E>) {
    if (props.operators === undefined) {
        return (
            <FormulaTreeEditor
                operators={computeOperators(props.operands, props.valueNoun)}
                value={props.value}
                onChange={props.onChange}
                testID={props.testID}
            />
        )
    }
    return (
        <FormulaTreeEditor
            operators={props.operators}
            value={props.value}
            onChange={props.onChange}
            testID={props.testID}
        />
    )
}
