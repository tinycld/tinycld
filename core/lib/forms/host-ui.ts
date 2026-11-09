import type { ComponentType } from 'react'
import type { HostInstance } from 'react-native'
import type { Awareness } from 'y-protocols/awareness'
import type * as Y from 'yjs'
import type { ZodTypeAny } from 'zod'
import type { HostSchemas, TableRow } from './values'

/** A cell of a table built-in that should take focus. */
export interface TableFocus {
    rowId: string
    columnKey: string
}

/** Set when a host component draws one cell of a table built-in. */
export interface TableCellContext {
    columnKey: string
    rowIndex: number
    /** The row's current cells, computed cells filled. */
    row: TableRow
    /** Sets several cells of this row in one change, such as a rate picked with a product. */
    onRowChange: (cells: Record<string, unknown>) => void
    /** The row's kind (its `kind` cell) on a table with row kinds; null otherwise. */
    rowKind: string | null
    /** The rows before this one, in order (a percent line reads the line above). */
    rowsAbove: TableRow[]
    /** Replaces this row with `rows` at its place (a bundle into its lines), then focuses `focus`. */
    replaceRow: (rows: TableRow[], focus?: TableFocus) => void
    /** Ref for the cell's focusable element, so the table can focus it after a row action. */
    focusRef: (el: HostInstance | null) => void
    /** Call from the cell input's `onFocus`, so the row keys and kind keys act on this row. */
    noteFocus: () => void
    /** Call from the cell input's `onBlur`, so the row keys stop once focus leaves the table. */
    noteBlur: () => void
    /** Moves focus to the next cell in the row, or the next row's first cell. */
    focusNext: () => void
    /** Enter / return key in the cell: adds a row when the column has `addOnSubmit` and the row is last. */
    submitEditing: () => void
}

export interface CollabDoc {
    doc: Y.Doc
    awareness: Awareness | null
    user: { id: string; name: string; color: string } | null
    canEdit: boolean
}

export type FieldMode = 'create' | 'edit' | 'design' | 'read'

export interface BuiltinFieldProps {
    fieldKey: string
    label: string
    recordId?: string
    value: unknown
    onChange: (value: unknown) => void
    mode: FieldMode
    /** Read-only or a design canvas: the component shows the value and offers no edit. */
    disabled: boolean
    /** The room, when the field is collaborative and the screen opened one. */
    collab?: CollabDoc | null
    /** The fragment name resolved from the host's template, for a collaborative field. */
    fragment?: string
    /** Only when the component draws a table cell: `value` and `onChange` are then the cell's. */
    table?: TableCellContext
}

export interface FormHostUi {
    components: Record<string, ComponentType<BuiltinFieldProps>>
    /**
     * A row the host draws ABOVE a built-in's cell, keyed by the built-in's
     * component name like `components`, and rendered with the same props. The
     * sections layout gives it a row of its own; when the renderer owns the
     * scroll view (`scroll`), that row pins to the top while the field scrolls
     * past. Edit and read modes only: the create wizard and the design canvas
     * never render it. A pinned field splits its page into separate rows, so
     * half-width cells before and after it do not share a line.
     */
    pinned?: Record<string, ComponentType<BuiltinFieldProps>>
    useDoc?: (recordId: string) => CollabDoc | null
    /**
     * The value schema of a custom built-in whose component has one, keyed by
     * component name like `components`. Its issue messages are the field's
     * inline text, and a refusal blocks the save.
     */
    schemas?: HostSchemas
    /**
     * A whole-record schema run after the field schemas, for a host's
     * cross-field rules; its issues show at their paths as field errors.
     */
    recordSchema?: ZodTypeAny
}
