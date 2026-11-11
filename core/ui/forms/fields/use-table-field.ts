import type { HostTableColumn } from '@tinycld/core/lib/forms/definition'
import type { HostRowKind } from '@tinycld/core/lib/forms/host'
import type { TableFocus } from '@tinycld/core/lib/forms/host-ui'
import type { FormValues } from '@tinycld/core/lib/forms/logic'
import {
    duplicateRow,
    insertRow,
    moveRow,
    newTableRow,
    replaceRows,
} from '@tinycld/core/lib/forms/table-rows'
import type { TableRow } from '@tinycld/core/lib/forms/values'
import { moveFocus } from '@tinycld/core/lib/move-focus'
import type { Shortcut } from '@tinycld/core/lib/shortcuts/types'
import { useRegisterShortcuts } from '@tinycld/core/lib/shortcuts/use-register'
import { newRecordId } from 'pbtsdb/core'
import { useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { type Control, type UseFormGetValues, useController, useWatch } from 'react-hook-form'
import { type HostInstance, Platform } from 'react-native'

/** The keys a table with `rowShortcuts` registers; they act on the row that holds focus. */
export const TABLE_ROW_KEYS = {
    remove: '$mod+Alt+Backspace',
    up: '$mod+Alt+ArrowUp',
    down: '$mod+Alt+ArrowDown',
    duplicate: '$mod+Alt+d',
    menu: 'Alt+m',
} as const

/** How the ⇧? sheet names each row key. */
export const TABLE_ROW_KEY_LABELS = {
    remove: 'Remove line',
    up: 'Move line up',
    down: 'Move line down',
    duplicate: 'Duplicate line',
    menu: 'Open line menu',
} as const

type RowKey = keyof typeof TABLE_ROW_KEYS

/** The ⇧? sheet's name for a kind's add key: "Add line" stays, else "Add " + the label. */
export function tableKindKeyLabel(addLabel: string): string {
    if (addLabel === 'Add line') return addLabel
    return `Add ${addLabel.toLowerCase()}`
}

/** The add action among the focus targets; a row's keys always start with the row id. */
export const ADD_BUTTON = ':add'
/** The row menu button's column among a row's focus targets. */
export const MENU_COLUMN = ':menu'
export const cellKey = (rowId: string, columnKey: string) => `${rowId}:${columnKey}`
/** A host cell's own focusable element; it wins over the cell's wrapper. */
const ownKey = (key: string) => `${key}:own`

const SHORTCUT_GROUP = 'Lines'
const NO_SHORTCUTS: Shortcut[] = []

export interface UseTableFieldInput {
    control: Control<FormValues>
    /** Reads the form's values at call time; see `currentRows`. */
    getValues: UseFormGetValues<FormValues>
    name: string
    /** The table's visible columns. */
    columns: readonly HostTableColumn[]
    /** This table's rows in the live computed values, computed cells filled. */
    computedRows: unknown
    /** True where an absent value means the host has not loaded the rows yet. */
    isPendingWhenAbsent: boolean
    isEditable: boolean
    rowKinds?: HostRowKind[]
    rowShortcuts: boolean
}

function asRows(value: unknown): TableRow[] | undefined {
    return Array.isArray(value) ? (value as TableRow[]) : undefined
}

const TEXT_ENTRY = 'input:not([type="hidden"]), textarea, select'
const FOCUSABLE = 'button, a[href], [role="button"], [tabindex]:not([tabindex="-1"])'

/** A React Native TextInput instance; a view or a button has no `isFocused`. */
function isNativeTextInput(target: HostInstance) {
    return typeof (target as { isFocused?: unknown }).isFocused === 'function'
}

/**
 * On native a text input takes keyboard focus with `focus()`, so a hardware
 * keyboard keeps typing in the table; any other target (a menu button, a
 * cell's wrapper) has no keyboard focus to take, so the screen reader's cursor
 * moves there instead. On web a cell's target is its wrapper, so focus goes to
 * the first text entry inside it (a number cell's input, not its − button),
 * else the first focusable element. A button or input target takes focus
 * itself.
 */
function focusTarget(target: HostInstance, isTextEntry: boolean) {
    if (Platform.OS !== 'web') {
        if (isTextEntry) target.focus()
        else moveFocus(target)
        return
    }
    const el = target as unknown as HTMLElement
    if (typeof el.matches !== 'function') {
        moveFocus(target)
        return
    }
    const isSelf = el.matches(`${TEXT_ENTRY}, ${FOCUSABLE}`)
    const inner = isSelf ? el : (el.querySelector(TEXT_ENTRY) ?? el.querySelector(FOCUSABLE))
    moveFocus((inner ?? el) as unknown as HostInstance)
}

/**
 * A focus request. `rowIds` is the row order the change makes (see
 * `rowOrderOf`); it is null once the commit that draws the change has run, or
 * when no commit follows. `commitsLeft` counts the table commits without the
 * change that the request may still wait through.
 */
interface PendingFocus {
    key: string
    rowIds: string | null
    commitsLeft: number
}

/**
 * The table commits without the row change that a request waits through
 * before it is dropped. A row change in a transition can be passed by sync
 * renders of the table (a resize, another field's edit); a request that waited
 * longer could take focus from what the user focused meanwhile.
 */
const MAX_COMMITS_WITHOUT_CHANGE = 3

const rowOrderOf = (list: readonly TableRow[]) => list.map(r => r.id).join('\n')

export function useTableField(input: UseTableFieldInput) {
    const { control, getValues, name, columns, computedRows, isPendingWhenAbsent } = input
    const { isEditable, rowKinds, rowShortcuts } = input
    const { field } = useController({ control, name })
    // Not exact, unlike the controller's own value: a cell edit (`lines.0.qty`)
    // must re-render the rows shown here.
    const watched = useWatch({ control, name })
    const rows = asRows(watched) ?? []
    const computedById = new Map((asRows(computedRows) ?? []).map(r => [r.id, r] as const))
    const targets = useRef(new Map<string, HostInstance>())
    const refs = useRef(new Map<string, (el: HostInstance | null) => void>())
    // The targets registered by a core text, number or date input.
    const textEntries = useRef(new Set<string>())
    // The cell that holds focus, set by each cell's focus event; read by the row keys.
    const focused = useRef<TableFocus | null>(null)
    const [menuRowId, setMenuRowId] = useState<string | null>(null)
    // The cell a row change asked to focus; see `focusAfterChange`.
    const pendingFocus = useRef<PendingFocus | null>(null)

    // Every write starts from the form's rows now, not this render's: a cell's
    // onChange earlier in the same handler, or a second press before the
    // re-render, has already changed them.
    const currentRows = () => asRows(getValues(name)) ?? []

    const kindOf = (row: TableRow | undefined) =>
        rowKinds?.find(k => k.key === row?.kind) ?? rowKinds?.[0]
    /** The columns a row draws: its kind's, or all of them. */
    const columnsOf = (row: TableRow | undefined): HostTableColumn[] => {
        const kind = kindOf(row)
        if (!kind) return [...columns]
        return kind.columns.flatMap(key => columns.filter(c => c.key === key))
    }
    const editableKeysOf = (row: TableRow | undefined) =>
        columnsOf(row)
            .filter(c => c.kind !== 'computed')
            .map(c => c.key)

    // One callback per key, so React does not detach and re-attach every ref on each render.
    const refFor = (key: string) => {
        const known = refs.current.get(key)
        if (known) return known
        const ref = (el: HostInstance | null) => {
            if (el) {
                targets.current.set(key, el)
                return
            }
            targets.current.delete(key)
            refs.current.delete(key)
            textEntries.current.delete(key)
        }
        refs.current.set(key, ref)
        return ref
    }
    /** The ref of a cell's core text, number or date input. */
    const textRefFor = (key: string) => {
        textEntries.current.add(ownKey(key))
        return refFor(ownKey(key))
    }
    const applyPendingFocus = () => {
        const request = pendingFocus.current
        if (!request || request.rowIds !== null) return
        pendingFocus.current = null
        const own = targets.current.get(ownKey(request.key))
        const target = own ?? targets.current.get(request.key)
        if (!target) return
        const isTextEntry =
            own !== undefined &&
            (textEntries.current.has(ownKey(request.key)) || isNativeTextInput(own))
        focusTarget(target, isTextEntry)
    }
    /**
     * Focuses `key` on the frame after the commit that draws the row change
     * `next`: React can re-insert a moved row's node, which drops its focus,
     * and a new row's cell is not registered before that commit. A row change
     * can come from an async callback (a bundle's items loaded after an
     * await), so its commit is scheduled, and a sync render of the table (a
     * resize) can commit before it. The request therefore waits for the
     * commit whose rows are in the order `next` makes, through at most
     * `MAX_COMMITS_WITHOUT_CHANGE` commits without it. It is tried once on the
     * frame after that commit and then dropped, found or not, so it cannot
     * take focus later from what the user focused meanwhile. A newer request
     * replaces an older one.
     */
    const focusAfterChange = (key: string, next: readonly TableRow[]) => {
        pendingFocus.current = {
            key,
            rowIds: rowOrderOf(next),
            commitsLeft: MAX_COMMITS_WITHOUT_CHANGE,
        }
    }
    /** A focus move with no row change, which no commit follows. */
    const focusOnNextFrame = (key: string) => {
        pendingFocus.current = { key, rowIds: null, commitsLeft: 0 }
        requestAnimationFrame(applyPendingFocus)
    }
    /** The row's cell in `columnKey`, else its first editable cell, else its menu button. */
    const focusKeyIn = (row: TableRow, columnKey: string) => {
        const editable = editableKeysOf(row)
        if (columnKey === MENU_COLUMN || editable.includes(columnKey))
            return cellKey(row.id, columnKey)
        return cellKey(row.id, editable[0] ?? MENU_COLUMN)
    }
    const firstCellKey = (row: TableRow) => cellKey(row.id, editableKeysOf(row)[0] ?? MENU_COLUMN)

    const newRow = (kindKey?: string) => {
        const kind = rowKinds?.find(k => k.key === kindKey) ?? rowKinds?.[0]
        return newTableRow(columns, newRecordId(), kind?.key)
    }
    const insert = (index: number, kindKey?: string) => {
        const row = newRow(kindKey)
        const next = insertRow(currentRows(), index, row)
        field.onChange(next)
        focusAfterChange(firstCellKey(row), next)
    }
    const add = () => insert(currentRows().length)
    /** A row of the first kind right above or below `id`. */
    const insertNear = (id: string, offset: 0 | 1) => {
        const index = currentRows().findIndex(r => r.id === id)
        if (index >= 0) insert(index + offset)
    }
    /** After the focused row, or at the end when focus is outside the table. */
    const addAfterFocused = (kindKey?: string) => {
        const before = currentRows()
        const at = before.findIndex(r => r.id === focused.current?.rowId)
        insert(at >= 0 ? at + 1 : before.length, kindKey)
    }
    const duplicate = (id: string, columnKey = MENU_COLUMN) => {
        const before = currentRows()
        const source = before.find(r => r.id === id)
        if (!source) return
        const copyId = newRecordId()
        const next = duplicateRow(before, id, copyId)
        field.onChange(next)
        focusAfterChange(focusKeyIn({ ...source, id: copyId }, columnKey), next)
    }
    const remove = (id: string, columnKey = MENU_COLUMN) => {
        const before = currentRows()
        const index = before.findIndex(r => r.id === id)
        if (index < 0) return
        const next = before.filter(r => r.id !== id)
        field.onChange(next)
        const neighbour = before[index + 1] ?? before[index - 1]
        focusAfterChange(neighbour ? focusKeyIn(neighbour, columnKey) : ADD_BUTTON, next)
    }
    const move = (id: string, delta: -1 | 1, columnKey = MENU_COLUMN) => {
        const before = currentRows()
        const next = moveRow(before, id, delta)
        if (next === before) return
        field.onChange(next)
        const row = next.find(r => r.id === id)
        if (row) focusAfterChange(focusKeyIn(row, columnKey), next)
    }
    const replace = (id: string, replacement: TableRow[], focus?: TableFocus) => {
        const next = replaceRows(currentRows(), id, replacement)
        field.onChange(next)
        if (focus) focusAfterChange(cellKey(focus.rowId, focus.columnKey), next)
    }
    /** The next editable cell of the row, else the next row's first cell, else the add action. */
    const focusNext = (id: string, columnKey: string) => {
        const now = currentRows()
        const index = now.findIndex(r => r.id === id)
        const row = now[index]
        if (!row) return
        const editable = editableKeysOf(row)
        const after = editable[editable.indexOf(columnKey) + 1]
        if (after && editable.includes(columnKey)) {
            focusOnNextFrame(cellKey(id, after))
            return
        }
        const nextRow = now[index + 1]
        focusOnNextFrame(nextRow ? firstCellKey(nextRow) : ADD_BUTTON)
    }
    /** Enter in a cell: an `addOnSubmit` column of the last row adds a row of the first kind. */
    const submitEditing = (id: string, columnKey: string) => {
        const now = currentRows()
        const column = columns.find(c => c.key === columnKey)
        if (!column?.addOnSubmit || now.at(-1)?.id !== id) return
        insert(now.length)
    }
    const onRowChange = (id: string, cells: Record<string, unknown>) =>
        field.onChange(currentRows().map(r => (r.id === id ? { ...r, ...cells } : r)))
    const computedRowOf = (row: TableRow) => computedById.get(row.id) ?? row
    const onCellFocus = (rowId: string, columnKey: string) => {
        focused.current = { rowId, columnKey }
    }
    /**
     * A cell's own blur: the one blur native delivers. It clears the focused
     * cell only while that cell is still the one noted, because the next
     * cell's focus can arrive before this blur.
     */
    const onCellBlur = (rowId: string, columnKey: string) => {
        const at = focused.current
        if (at?.rowId === rowId && at.columnKey === columnKey) focused.current = null
    }
    /** Web only: focus left a row; blur bubbles there from any element in it. */
    const onRowBlur = () => {
        focused.current = null
    }
    const openMenu = (rowId: string | null) => setMenuRowId(rowId)

    // No dependencies: every commit of this table may be the one that draws a requested row.
    const committedRowOrder = rowOrderOf(rows)
    useLayoutEffect(() => {
        const request = pendingFocus.current
        if (!request || request.rowIds === null) return
        if (request.rowIds === committedRowOrder) {
            request.rowIds = null
            requestAnimationFrame(applyPendingFocus)
            return
        }
        request.commitsLeft -= 1
        if (request.commitsLeft <= 0) pendingFocus.current = null
    })

    // The keys read the latest handlers through a ref, so the registered list
    // stays the same array for the table's life and never re-registers.
    const latest = useRef({ remove, move, duplicate, openMenu, addAfterFocused, isEditable })
    latest.current = { remove, move, duplicate, openMenu, addAfterFocused, isEditable }
    const instance = useId()
    const rowKeyRunners: Record<RowKey, (at: TableFocus) => void> = useMemo(
        () => ({
            remove: at => latest.current.remove(at.rowId, at.columnKey),
            up: at => latest.current.move(at.rowId, -1, at.columnKey),
            down: at => latest.current.move(at.rowId, 1, at.columnKey),
            duplicate: at => latest.current.duplicate(at.rowId, at.columnKey),
            menu: at => latest.current.openMenu(at.rowId),
        }),
        []
    )
    const rowKeyShortcuts = useMemo<Shortcut[]>(
        () =>
            (Object.keys(TABLE_ROW_KEYS) as RowKey[]).map(key => ({
                id: `table.${instance}.${name}.${key}`,
                keys: TABLE_ROW_KEYS[key],
                scope: 'compose',
                description: TABLE_ROW_KEY_LABELS[key],
                group: SHORTCUT_GROUP,
                allowInInputs: true,
                when: () => latest.current.isEditable && focused.current !== null,
                run: () => {
                    const at = focused.current
                    if (at) rowKeyRunners[key](at)
                },
            })),
        [instance, name, rowKeyRunners]
    )
    // The first kind's key belongs to its add button (a ShortcutButton); the
    // table registers the keys of the kinds in the "Add…" menu.
    const kindShortcuts = useMemo<Shortcut[]>(
        () =>
            (rowKinds ?? []).slice(1).flatMap(kind => {
                if (!kind.shortcut) return []
                return [
                    {
                        id: `table.${instance}.${name}.add-${kind.key}`,
                        keys: kind.shortcut,
                        scope: 'compose' as const,
                        description: tableKindKeyLabel(kind.addLabel),
                        group: SHORTCUT_GROUP,
                        allowInInputs: true,
                        when: () => latest.current.isEditable,
                        run: () => latest.current.addAfterFocused(kind.key),
                    },
                ]
            }),
        [instance, name, rowKinds]
    )
    // The ⇧? sheet lists a lone registration whatever its `when` returns (so
    // the focus-gated row keys stay listed), so a key is registered only
    // while its action is offered.
    useRegisterShortcuts(isEditable && rowShortcuts ? rowKeyShortcuts : NO_SHORTCUTS)
    useRegisterShortcuts(isEditable ? kindShortcuts : NO_SHORTCUTS)

    return {
        rows,
        isPending: isPendingWhenAbsent && watched === undefined,
        add,
        addAfterFocused,
        insertNear,
        duplicate,
        remove,
        move,
        replace,
        focusNext,
        submitEditing,
        onRowChange,
        computedRowOf,
        refFor,
        ownRefFor: (key: string) => refFor(ownKey(key)),
        textRefFor,
        onCellFocus,
        onCellBlur,
        onRowBlur,
        menuRowId,
        openMenu,
    }
}

export type TableFieldApi = ReturnType<typeof useTableField>
