import type { HostTableColumn } from '@tinycld/core/lib/forms/definition'
import type { HostRowKind } from '@tinycld/core/lib/forms/host'
import type {
    BuiltinFieldProps,
    FieldMode,
    FormHostUi,
    TableCellContext,
} from '@tinycld/core/lib/forms/host-ui'
import type { FormValues } from '@tinycld/core/lib/forms/logic'
import type { TableRow } from '@tinycld/core/lib/forms/values'
import { formatHint } from '@tinycld/core/lib/shortcuts/hint'
import { useScopeOwner } from '@tinycld/core/lib/shortcuts/scopes'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Button, ButtonIcon, ButtonText } from '@tinycld/core/ui/button'
import {
    ButtonGroup,
    type Control,
    Controller,
    NumberInput,
    TextInput,
    Toggle,
    useController,
} from '@tinycld/core/ui/form'
import { Menu } from '@tinycld/core/ui/menu'
import { ShortcutButton } from '@tinycld/core/ui/shortcut-button'
import { EllipsisVertical, Plus } from 'lucide-react-native'
import { type ComponentType, type ReactNode, useState } from 'react'
import type { UseFormGetValues } from 'react-hook-form'
import { type HostInstance, Platform, Pressable, Text, View, type ViewStyle } from 'react-native'
import { NARROW_BELOW } from '../widths'
import { DateField } from './DateField'
import { formatComputed } from './format-computed'
import { slotStyle, TABLE_GRID_GAP, TABLE_ROW_MENU_WIDTH } from './table-layout'
import {
    ADD_BUTTON,
    cellKey,
    MENU_COLUMN,
    TABLE_ROW_KEYS,
    type TableFieldApi,
    useTableField,
} from './use-table-field'

export interface TableFieldProps {
    ui: FormHostUi | null
    name: string
    label: string
    columns: readonly HostTableColumn[]
    control: Control<FormValues>
    getValues: UseFormGetValues<FormValues>
    mode: FieldMode
    recordId?: string
    containerWidth: number
    computedRows: unknown
    /** A host table: on a record, an absent value means its rows have not loaded. */
    isHostTable: boolean
    /** A host table's kinds of row; absent means every row draws every column. */
    rowKinds?: HostRowKind[]
    /** Show and register core's row keys (remove, move, duplicate, open menu). */
    rowShortcuts?: boolean
    testID: string
}

/** What every part of one table shares. */
interface TableBase {
    ui: FormHostUi | null
    name: string
    control: Control<FormValues>
    mode: FieldMode
    recordId?: string
    isEditable: boolean
    /** Prefix of every testID in the table. */
    ids: string
    /** The visible columns. */
    columns: readonly HostTableColumn[]
    rowKinds?: HostRowKind[]
    rowShortcuts: boolean
    /** The wide grid's header slots: the first kind's columns, or every column. */
    slots: readonly HostTableColumn[]
}

interface CellProps {
    base: TableBase
    table: TableFieldApi
    column: HostTableColumn
    row: TableRow
    index: number
}

interface RowProps {
    base: TableBase
    table: TableFieldApi
    row: TableRow
    index: number
    count: number
}

const headerText = (c: HostTableColumn) => (c.required ? `${c.label} *` : c.label)

function kindOfRow(base: TableBase, row: TableRow): HostRowKind | undefined {
    if (!base.rowKinds) return undefined
    return base.rowKinds.find(k => k.key === row.kind) ?? base.rowKinds[0]
}

/** The columns a row draws in its own order: its kind's, or all of them. */
function rowColumns(base: TableBase, row: TableRow): HostTableColumn[] {
    const kind = kindOfRow(base, row)
    if (!kind) return [...base.columns]
    return kind.columns.flatMap(key => base.columns.filter(c => c.key === key))
}

interface GridSlot {
    /** The slot's key: its header column's key, or its own column's key past the header slots. */
    key: string
    /**
     * `slot` when the cell sits under the header column `key`; `span` when it
     * sits under no header column (a span kind's cell, or a column drawn after
     * the header slots), so its test id does not name a header slot.
     */
    place: 'slot' | 'span'
    column: HostTableColumn | null
    /** The slot's size on the wide grid: its header column's `flex` and `minWidth`. */
    size: HostTableColumn
}

/**
 * A grid row's cells in the header slots: a column of the first kind in its
 * own slot, another kind's column in the slot its `columnAt` names, an unused
 * slot empty. Each takes the weight and floor of the header column above it,
 * so the cells line up with the header. A column with no slot is drawn after
 * the slots with its own size, so no cell is lost.
 */
function gridSlots(base: TableBase, row: TableRow): GridSlot[] {
    const own = rowColumns(base, row)
    const kind = kindOfRow(base, row)
    const placed = new Set<string>()
    const slots = base.slots.map(slot => {
        const atSlot = Object.entries(kind?.columnAt ?? {}).find(([, at]) => at === slot.key)
        const key = own.some(c => c.key === slot.key) ? slot.key : atSlot?.[0]
        const column = own.find(c => c.key === key) ?? null
        if (column) placed.add(column.key)
        return { key: slot.key, place: 'slot' as const, column, size: slot }
    })
    const rest = own.filter(c => !placed.has(c.key)).map(spanSlotOf)
    return [...slots, ...rest]
}

/** A cell under no header column: its own column's key and size. */
const spanSlotOf = (column: HostTableColumn): GridSlot => ({
    key: column.key,
    place: 'span',
    column,
    size: column,
})

/** A span kind's cells: each runs across the slots with its own size. */
const spanSlots = (base: TableBase, row: TableRow): GridSlot[] =>
    rowColumns(base, row).map(spanSlotOf)

function CellError({ message }: { message?: string }) {
    if (!message) return null
    return <Text className="text-xs text-danger mt-1">{message}</Text>
}

function DateCell({
    control,
    name,
    disabled,
    testID,
    label,
    onFocus,
    onBlur,
    inputRef,
}: {
    control: Control<FormValues>
    name: string
    disabled: boolean
    testID: string
    label: string
    onFocus: () => void
    onBlur: () => void
    inputRef?: (el: HostInstance | null) => void
}) {
    const { field, fieldState } = useController({ control, name })
    const value = typeof field.value === 'string' ? field.value : null
    return (
        <View>
            <DateField
                value={value}
                onChange={field.onChange}
                withTime={false}
                disabled={disabled}
                onFocus={onFocus}
                onBlur={onBlur}
                inputRef={inputRef}
                testID={testID}
                accessibilityLabel={label}
            />
            <CellError message={fieldState.error?.message} />
        </View>
    )
}

function ComputedCell({
    value,
    decimals,
    testID,
    label,
}: {
    value: unknown
    decimals: number
    testID: string
    label: string
}) {
    const text = formatComputed(value, decimals)
    const tone = text ? 'text-foreground' : 'text-muted-foreground'
    return (
        <Text testID={testID} accessibilityLabel={label} className={`py-2 text-right ${tone}`}>
            {text || '—'}
        </Text>
    )
}

/** The host's component for a column; never on the design canvas, which has no rows to draw. */
function hostCellOf(base: TableBase, column: HostTableColumn) {
    if (base.mode === 'design' || !column.component) return undefined
    return base.ui?.components[column.component]
}

function HostCell({
    Component,
    label,
    base,
    table,
    column,
    row,
    index,
}: CellProps & { Component: ComponentType<BuiltinFieldProps>; label: string }) {
    const rowKind = base.rowKinds && typeof row.kind === 'string' ? row.kind : null
    const context: TableCellContext = {
        columnKey: column.key,
        rowIndex: index,
        row: table.computedRowOf(row),
        onRowChange: cells => table.onRowChange(row.id, cells),
        rowKind,
        rowsAbove: table.rows.slice(0, index),
        replaceRow: (rows, focus) => table.replace(row.id, rows, focus),
        focusRef: table.ownRefFor(cellKey(row.id, column.key)),
        noteFocus: () => table.onCellFocus(row.id, column.key),
        noteBlur: () => table.onCellBlur(row.id, column.key),
        focusNext: () => table.focusNext(row.id, column.key),
        submitEditing: () => table.submitEditing(row.id, column.key),
    }
    return (
        <Controller
            control={base.control}
            name={`${base.name}.${index}.${column.key}`}
            render={({ field, fieldState }) => (
                <View>
                    <Component
                        fieldKey={base.name}
                        label={label}
                        recordId={base.recordId}
                        value={field.value}
                        onChange={field.onChange}
                        mode={base.mode}
                        disabled={!base.isEditable}
                        table={context}
                    />
                    <CellError message={fieldState.error?.message} />
                </View>
            )}
        />
    )
}

function CellInput(props: CellProps) {
    const { base, table, column, row, index } = props
    const name = `${base.name}.${index}.${column.key}`
    const testID = `${base.ids}-${row.id}-${column.key}`
    const label = `${column.label}, row ${index + 1}`
    const disabled = !base.isEditable
    const onSubmitEditing = column.addOnSubmit
        ? () => table.submitEditing(row.id, column.key)
        : undefined
    // The input's own focus and blur events: on iOS a wrapping View never hears them.
    const onFocus = () => table.onCellFocus(row.id, column.key)
    const onBlur = () => table.onCellBlur(row.id, column.key)
    const Host = hostCellOf(base, column)
    if (Host) return <HostCell {...props} Component={Host} label={label} />
    const takesText = column.kind === 'text' || column.kind === 'number' || column.kind === 'date'
    const inputRef = takesText ? table.textRefFor(cellKey(row.id, column.key)) : undefined
    switch (column.kind) {
        case 'text':
            return (
                <TextInput
                    control={base.control}
                    name={name}
                    editable={!disabled}
                    inputRef={inputRef}
                    onFocus={onFocus}
                    onBlur={onBlur}
                    onSubmitEditing={onSubmitEditing}
                    testID={testID}
                    accessibilityLabel={label}
                />
            )
        case 'number':
            return (
                <NumberInput
                    control={base.control}
                    name={name}
                    min={column.min}
                    max={column.max}
                    allowDecimal
                    disabled={disabled}
                    inputRef={inputRef}
                    onFocus={onFocus}
                    onBlur={onBlur}
                    onSubmitEditing={onSubmitEditing}
                    testID={testID}
                    accessibilityLabel={label}
                />
            )
        case 'toggle':
            return (
                <Toggle
                    control={base.control}
                    name={name}
                    label={label}
                    hideLabel
                    disabled={disabled}
                    onFocus={onFocus}
                    onBlur={onBlur}
                    wrapperProps={{ testID }}
                />
            )
        case 'date':
            return (
                <DateCell
                    control={base.control}
                    name={name}
                    disabled={disabled}
                    onFocus={onFocus}
                    onBlur={onBlur}
                    inputRef={inputRef}
                    testID={testID}
                    label={label}
                />
            )
        case 'select':
            return (
                <ButtonGroup
                    control={base.control}
                    name={name}
                    label={label}
                    hideLabel
                    options={column.options}
                    horizontal
                    disabled={disabled}
                    onFocus={onFocus}
                    onBlur={onBlur}
                    wrapperProps={{ testID }}
                />
            )
        case 'computed':
            return (
                <ComputedCell
                    value={table.computedRowOf(row)[column.key]}
                    decimals={column.decimals}
                    testID={testID}
                    label={label}
                />
            )
    }
}

/**
 * The cell's box: the table's focus target for the cell. On web it also notes
 * the cell's focus, which bubbles from any element inside it, so the row keys
 * know which row and column the keyboard is in. On native a View never hears a
 * child's focus or blur: each cell input notes its own (`onFocus` / `onBlur`,
 * a host cell's `noteFocus` / `noteBlur`).
 */
function CellSlot({
    table,
    row,
    column,
    className,
    style,
    testID,
    children,
}: {
    table: TableFieldApi
    row: TableRow
    column: HostTableColumn
    /** A card's spacing; a grid slot leaves it unset (`style` sizes it). */
    className?: string
    /** The wide grid's width weight and floor; a card leaves it unset. */
    style?: ViewStyle
    /** The wide grid's slot or span id; a card leaves it unset. */
    testID?: string
    children: ReactNode
}) {
    const noteFocus = () => table.onCellFocus(row.id, column.key)
    const onFocus = Platform.OS === 'web' ? noteFocus : undefined
    const slotRef = table.refFor(cellKey(row.id, column.key))
    return (
        <View ref={slotRef} onFocus={onFocus} className={className} style={style} testID={testID}>
            {children}
        </View>
    )
}

/** The key text a row menu item shows, only when the table registers the keys. */
const ROW_KEY_HINTS = {
    up: formatHint(TABLE_ROW_KEYS.up).text,
    down: formatHint(TABLE_ROW_KEYS.down).text,
    duplicate: formatHint(TABLE_ROW_KEYS.duplicate).text,
    remove: formatHint(TABLE_ROW_KEYS.remove).text,
}
const NO_KEY_HINTS = { up: undefined, down: undefined, duplicate: undefined, remove: undefined }

function RowMenuItems({ base, table, row, index, count }: RowProps) {
    const hints = base.rowShortcuts ? ROW_KEY_HINTS : NO_KEY_HINTS
    const itemId = (action: string) => `${base.ids}-menu-${action}`
    const insertAbove = () => table.insertNear(row.id, 0)
    const insertBelow = () => table.insertNear(row.id, 1)
    const moveUp = () => table.move(row.id, -1)
    const moveDown = () => table.move(row.id, 1)
    const duplicate = () => table.duplicate(row.id)
    const remove = () => table.remove(row.id)
    return (
        <>
            <Menu.Item
                label="Insert line above"
                onSelect={insertAbove}
                testID={itemId('insert-above')}
            />
            <Menu.Item
                label="Insert line below"
                onSelect={insertBelow}
                testID={itemId('insert-below')}
            />
            <Menu.Item
                label="Move up"
                shortcut={hints.up}
                isDisabled={index === 0}
                onSelect={moveUp}
                testID={itemId('up')}
            />
            <Menu.Item
                label="Move down"
                shortcut={hints.down}
                isDisabled={index === count - 1}
                onSelect={moveDown}
                testID={itemId('down')}
            />
            <Menu.Item
                label="Duplicate"
                shortcut={hints.duplicate}
                onSelect={duplicate}
                testID={itemId('duplicate')}
            />
            <Menu.Item
                label="Remove"
                shortcut={hints.remove}
                isDestructive
                onSelect={remove}
                testID={itemId('remove')}
            />
        </>
    )
}

/**
 * One Tab stop per row: a button opening the row's actions. One row menu is
 * open at a time; the table holds which, so its key can open it too.
 */
function RowMenu(props: RowProps) {
    const { base, table, row, index } = props
    const fg = useThemeColor('foreground')
    if (!base.isEditable) return null
    const n = index + 1
    const isOpen = table.menuRowId === row.id
    const onOpenChange = (open: boolean) => table.openMenu(open ? row.id : null)
    const onFocus = () => table.onCellFocus(row.id, MENU_COLUMN)
    const onBlur = () => table.onCellBlur(row.id, MENU_COLUMN)
    // Rows only while open: a sheet keeps its content in the DOM while it slides out.
    const items = isOpen ? <RowMenuItems {...props} /> : null
    const trigger = (
        <Pressable
            ref={table.refFor(cellKey(row.id, MENU_COLUMN))}
            onFocus={onFocus}
            onBlur={onBlur}
            accessibilityRole="button"
            accessibilityLabel={`Line ${n} actions`}
            className="p-1.5 rounded-md"
            testID={`${base.ids}-menu-${row.id}`}
        >
            <EllipsisVertical size={16} color={fg} />
        </Pressable>
    )
    return (
        <Menu
            trigger={trigger}
            isOpen={isOpen}
            onOpenChange={onOpenChange}
            placement="bottom-end"
            title={`Line ${n}`}
        >
            {items}
        </Menu>
    )
}

/** The header row and every grid row lay their slots out with the same gap. */
const GRID_ROW_STYLE: ViewStyle = { gap: TABLE_GRID_GAP }

/** The row menu's box and the header's spacer above it: one width, never shrunk. */
const MENU_BOX_STYLE: ViewStyle = { width: TABLE_ROW_MENU_WIDTH, flexShrink: 0 }

function ActionsSpacer({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <View style={MENU_BOX_STYLE} />
}

/**
 * The grid row's menu in a box as wide as the header's `ActionsSpacer`, so the
 * row's cell slots share the same width as the header slots above them.
 */
function GridRowMenu(props: RowProps) {
    if (!props.base.isEditable) return null
    return (
        <View style={MENU_BOX_STYLE} className="items-center">
            <RowMenu {...props} />
        </View>
    )
}

/**
 * A header slot: the same box as the row slots under it (no padding of its
 * own), with the label padded inside it. Padding on the slot itself would add
 * to its flex base size, so header slots would not divide the width as the
 * row slots do.
 */
function HeaderSlot({ base, column }: { base: TableBase; column: HostTableColumn }) {
    return (
        <View testID={`${base.ids}-head-${column.key}`} style={slotStyle(column)}>
            <Text className="px-1 text-xs font-semibold text-muted-foreground">
                {headerText(column)}
            </Text>
        </View>
    )
}

function GridHeader({ base, isVisible }: { base: TableBase; isVisible: boolean }) {
    if (!isVisible) return null
    const cells = base.slots.map(c => <HeaderSlot key={c.key} base={base} column={c} />)
    return (
        <View style={GRID_ROW_STYLE} className="flex-row pb-1 border-b border-border">
            {cells}
            <ActionsSpacer isVisible={base.isEditable} />
        </View>
    )
}

function GridSlotCell({ base, table, row, index, slot }: RowProps & { slot: GridSlot }) {
    const style = slotStyle(slot.size)
    const testID = `${base.ids}-${slot.place}-${row.id}-${slot.key}`
    if (!slot.column) return <View style={style} testID={testID} />
    return (
        <CellSlot table={table} row={row} column={slot.column} style={style} testID={testID}>
            <CellInput base={base} table={table} column={slot.column} row={row} index={index} />
        </CellSlot>
    )
}

/**
 * The row's blur, on web only, where blur bubbles from any element in the row
 * (a number cell's − button too). Native delivers blur to the cell inputs and
 * the menu button only, and each clears its own cell (`onCellBlur`).
 */
function rowBlurOf(table: TableFieldApi) {
    return Platform.OS === 'web' ? table.onRowBlur : undefined
}

function GridRow(props: RowProps) {
    const { base, table, row } = props
    const isSpan = kindOfRow(base, row)?.span ?? false
    // A span kind's one cell runs across every slot.
    const slots = isSpan ? spanSlots(base, row) : gridSlots(base, row)
    const cells = slots.map(slot => <GridSlotCell key={slot.key} {...props} slot={slot} />)
    const onBlur = rowBlurOf(table)
    return (
        <View
            testID={`${base.ids}-row-${row.id}`}
            onBlur={onBlur}
            style={GRID_ROW_STYLE}
            className="flex-row items-start py-1 border-b border-border"
        >
            {cells}
            <GridRowMenu {...props} />
        </View>
    )
}

/** A toggle and a choice list draw their own label, so a card does not repeat it. */
const drawsOwnLabel = (c: HostTableColumn) => c.kind === 'toggle' || c.kind === 'select'

function CardLabel({ text }: { text: string | null }) {
    if (!text) return null
    return <Text className="text-xs font-semibold text-muted-foreground mb-1">{text}</Text>
}

function CardCell(props: CellProps) {
    const label = drawsOwnLabel(props.column) ? null : headerText(props.column)
    return (
        <CellSlot table={props.table} row={props.row} column={props.column} className="mb-2">
            <CardLabel text={label} />
            <CellInput {...props} />
        </CellSlot>
    )
}

function CardKindLabel({ kind, testID }: { kind: HostRowKind | undefined; testID: string }) {
    if (!kind) return <View />
    return (
        <Text testID={testID} className="text-xs font-semibold text-muted-foreground">
            {kind.label}
        </Text>
    )
}

function CardRow(props: RowProps) {
    const { base, table, row, index } = props
    const cells = rowColumns(base, row).map(column => (
        <CardCell
            key={column.key}
            base={base}
            table={table}
            column={column}
            row={row}
            index={index}
        />
    ))
    const onBlur = rowBlurOf(table)
    return (
        <View
            testID={`${base.ids}-row-${row.id}`}
            accessibilityLabel={`Row ${index + 1}`}
            onBlur={onBlur}
            className="rounded-md border border-border p-3 mb-2"
        >
            <View className="flex-row items-center justify-between mb-1">
                <CardKindLabel kind={kindOfRow(base, row)} testID={`${base.ids}-kind-${row.id}`} />
                <RowMenu {...props} />
            </View>
            {cells}
        </View>
    )
}

function PendingRows({ testID }: { testID: string }) {
    return (
        <Text testID={testID} className="text-sm text-muted-foreground py-2">
            Loading rows…
        </Text>
    )
}

function Rows({
    base,
    table,
    isNarrow,
}: {
    base: TableBase
    table: TableFieldApi
    isNarrow: boolean
}) {
    const Row = isNarrow ? CardRow : GridRow
    const layoutID = isNarrow ? `${base.ids}-cards` : `${base.ids}-grid`
    const count = table.rows.length
    const items = table.rows.map((row, index) => (
        <Row key={row.id} base={base} table={table} row={row} index={index} count={count} />
    ))
    return <View testID={layoutID}>{items}</View>
}

function EmptyRows({ isVisible, testID }: { isVisible: boolean; testID: string }) {
    if (!isVisible) return null
    return (
        <Text testID={testID} className="text-sm text-muted-foreground py-2">
            No rows yet.
        </Text>
    )
}

/** A table without row kinds: one "Add row" button. */
function AddRowButton({ base, table }: { base: TableBase; table: TableFieldApi }) {
    const fg = useThemeColor('foreground')
    const buttonRef = table.refFor(ADD_BUTTON)
    return (
        <Pressable
            ref={buttonRef}
            onPress={table.add}
            accessibilityRole="button"
            accessibilityLabel="Add row"
            testID={`${base.ids}-add`}
            className="flex-row items-center gap-1 self-start mt-2 px-2 py-1 rounded-md border border-border"
        >
            <Plus size={16} color={fg} />
            <Text className="text-sm text-foreground">Add row</Text>
        </Pressable>
    )
}

/** The first kind's add action: a ShortcutButton when the kind has a key. */
function FirstKindButton({
    base,
    table,
    kind,
}: {
    base: TableBase
    table: TableFieldApi
    kind: HostRowKind
}) {
    // The focus target wraps the button: core's Button types its ref as its
    // props, not the host view it renders, so it cannot take the table's ref.
    const slotRef = table.refFor(ADD_BUTTON)
    return (
        <View ref={slotRef}>
            <FirstKindAction base={base} table={table} kind={kind} />
        </View>
    )
}

function FirstKindAction({
    base,
    table,
    kind,
}: {
    base: TableBase
    table: TableFieldApi
    kind: HostRowKind
}) {
    const owner = useScopeOwner() ?? undefined
    const onPress = () => table.addAfterFocused(kind.key)
    const testID = `${base.ids}-add`
    if (kind.shortcut) {
        const shortcut = {
            keys: kind.shortcut,
            scope: 'compose' as const,
            allowInInputs: true,
            group: 'Lines',
        }
        return (
            <ShortcutButton
                label={kind.addLabel}
                icon={Plus}
                shortcut={shortcut}
                onPress={onPress}
                variant="outline"
                size="sm"
                scopeOwner={owner}
                testID={testID}
            />
        )
    }
    return (
        <Button onPress={onPress} variant="outline" size="sm" testID={testID}>
            <ButtonIcon as={Plus} />
            <ButtonText>{kind.addLabel}</ButtonText>
        </Button>
    )
}

/** "Add…": the other kinds, each row showing the key the table registered for it. */
function MoreKindsMenu({
    base,
    table,
    kinds,
}: {
    base: TableBase
    table: TableFieldApi
    kinds: HostRowKind[]
}) {
    const [isOpen, setOpen] = useState(false)
    if (kinds.length === 0) return null
    const keyText = (kind: HostRowKind) =>
        kind.shortcut ? formatHint(kind.shortcut).text : undefined
    const addKind = (kind: HostRowKind) => () => table.addAfterFocused(kind.key)
    const rows = kinds.map(kind => (
        <Menu.Item
            key={kind.key}
            label={kind.addLabel}
            shortcut={keyText(kind)}
            onSelect={addKind(kind)}
            testID={`${base.ids}-add-${kind.key}`}
        />
    ))
    const items = isOpen ? rows : null
    const trigger = (
        <Button variant="outline" size="sm" testID={`${base.ids}-add-more`}>
            <ButtonText>Add…</ButtonText>
        </Button>
    )
    return (
        <Menu trigger={trigger} isOpen={isOpen} onOpenChange={setOpen} title="Add">
            {items}
        </Menu>
    )
}

function AddActions({
    base,
    table,
    isVisible,
}: {
    base: TableBase
    table: TableFieldApi
    isVisible: boolean
}) {
    if (!isVisible) return null
    const [first, ...others] = base.rowKinds ?? []
    if (!first) return <AddRowButton base={base} table={table} />
    return (
        <View className="flex-row items-center gap-2 self-start mt-2">
            <FirstKindButton base={base} table={table} kind={first} />
            <MoreKindsMenu base={base} table={table} kinds={others} />
        </View>
    )
}

/** The first kind's visible columns, or every visible column. */
function slotsOf(columns: readonly HostTableColumn[], rowKinds: HostRowKind[] | undefined) {
    const first = rowKinds?.[0]
    if (!first) return columns
    return first.columns.flatMap(key => columns.filter(c => c.key === key))
}

export function TableField(props: TableFieldProps) {
    const { mode, name, rowKinds } = props
    const rowShortcuts = props.rowShortcuts ?? false
    const isEditable = mode === 'create' || mode === 'edit'
    const columns = props.columns.filter(c => !c.hidden)
    const table = useTableField({
        control: props.control,
        getValues: props.getValues,
        name,
        columns,
        computedRows: props.computedRows,
        isPendingWhenAbsent: props.isHostTable && (mode === 'edit' || mode === 'read'),
        isEditable,
        rowKinds,
        rowShortcuts,
    })
    const base: TableBase = {
        ui: props.ui,
        name,
        control: props.control,
        mode,
        recordId: props.recordId,
        isEditable,
        ids: `form-table-${name}`,
        columns,
        rowKinds,
        rowShortcuts,
        slots: slotsOf(columns, rowKinds),
    }
    // 0 means not measured yet: lay out wide, as FieldCell does.
    const isNarrow = props.containerWidth > 0 && props.containerWidth < NARROW_BELOW
    if (table.isPending) return <PendingRows testID={`${base.ids}-pending`} />
    return (
        <View testID={props.testID} accessibilityLabel={props.label}>
            <GridHeader base={base} isVisible={!isNarrow} />
            <Rows base={base} table={table} isNarrow={isNarrow} />
            <EmptyRows isVisible={table.rows.length === 0} testID={`${base.ids}-empty`} />
            <AddActions base={base} table={table} isVisible={isEditable} />
        </View>
    )
}
