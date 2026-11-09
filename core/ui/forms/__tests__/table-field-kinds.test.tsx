// @vitest-environment happy-dom
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { type UseFormReturn, useForm } from 'react-hook-form'
import { Platform, Pressable, Text } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { asFormHost, fieldOf } from '../../../lib/forms/__tests__/table-fixtures'
import { defaultDefinitionFor, hostDefSchema } from '../../../lib/forms/host'
import type { BuiltinFieldProps, FormHostUi, TableCellContext } from '../../../lib/forms/host-ui'
import type { FormValues } from '../../../lib/forms/logic'
import type { TableRow } from '../../../lib/forms/values'
import { createMatcher } from '../../../lib/shortcuts/matcher'
import { useShortcutRegistry } from '../../../lib/shortcuts/registry'
import { pushScope, resetScopes, ScopeProvider } from '../../../lib/shortcuts/scopes'
import { OverlayProvider } from '../../overlay'
import { FieldInput } from '../FieldInput'
import { tableKindKeyLabel } from '../fields/use-table-field'
import { byTestId, pickRowAction, rowIdsOf } from './table-harness'

// The key glyphs are drawn for the platform the module sees at load: an Apple one here.
vi.hoisted(() => {
    Object.defineProperty(navigator, 'platform', { value: 'MacIntel', configurable: true })
})

const columns = [
    { key: 'name', label: 'Name', kind: 'text', required: true },
    { key: 'qty', label: 'Qty', kind: 'number', addOnSubmit: true },
    { key: 'done', label: 'Done', kind: 'toggle' },
    { key: 'text', label: 'Text', kind: 'text', required: true },
    { key: 'rate', label: 'Rate', kind: 'number' },
]

/** Items, span notes, and percent lines whose rate sits in the item's Qty slot. */
function billHost(nameComponent?: string) {
    const withComponent = columns.map(c =>
        c.key === 'name' && nameComponent ? { ...c, component: nameComponent } : c
    )
    return asFormHost(
        hostDefSchema.parse({
            key: 'gizmos:bill',
            label: 'Bill',
            collection: 'gizmos_bills',
            formColumn: 'form',
            fieldsColumn: 'fields',
            columns: ['form', 'fields'],
            builtin: [
                {
                    key: 'lines',
                    label: 'Lines',
                    kind: 'table',
                    storage: 'rows',
                    columns: withComponent,
                    rowShortcuts: true,
                    rowKinds: [
                        {
                            key: 'item',
                            label: 'Item',
                            addLabel: 'Add line',
                            columns: ['name', 'qty', 'done'],
                            shortcut: 'Alt+Enter',
                        },
                        {
                            key: 'note',
                            label: 'Note',
                            addLabel: 'Note line',
                            columns: ['text'],
                            span: true,
                            shortcut: 'Alt+t',
                        },
                        {
                            key: 'pct',
                            label: 'Percent',
                            addLabel: 'Percent line',
                            columns: ['rate', 'name', 'done'],
                            columnAt: { rate: 'qty' },
                            shortcut: 'Alt+5',
                        },
                    ],
                },
            ],
        })
    )
}

const blank = { name: null, qty: null, done: null, text: null, rate: null }
const item = (id: string, name: string, qty: number) => ({ ...blank, id, kind: 'item', name, qty })
const r1 = item('r1', 'A', 1)
const r2 = { ...blank, id: 'r2', kind: 'note', text: 'Hello' }
const r3 = item('r3', 'B', 2)
const r4 = { ...blank, id: 'r4', kind: 'pct', name: 'Off', rate: -10 }
const r5 = item('r5', 'C', 3)

let scopeId = 0
const owner = () => scopeId

interface HarnessProps {
    rows: TableRow[]
    width?: number
    ui?: FormHostUi | null
    host?: ReturnType<typeof billHost>
    formRef: { current: UseFormReturn<FormValues> | null }
}

function Harness({ rows, width = 1000, ui = null, host = billHost(), formRef }: HarnessProps) {
    const form = useForm<FormValues>({ defaultValues: { lines: rows } })
    formRef.current = form
    return (
        <OverlayProvider>
            <ScopeProvider owner={owner}>
                <FieldInput
                    host={host}
                    ui={ui}
                    field={fieldOf(defaultDefinitionFor(host), 'lines')}
                    control={form.control}
                    getValues={form.getValues}
                    mode="create"
                    containerWidth={width}
                />
            </ScopeProvider>
        </OverlayProvider>
    )
}

function setUp(rows: TableRow[], props: Partial<HarnessProps> = {}) {
    const formRef: { current: UseFormReturn<FormValues> | null } = { current: null }
    const view = render(<Harness rows={rows} formRef={formRef} {...props} />)
    const rowsNow = () => {
        const value = formRef.current?.getValues('lines')
        return (Array.isArray(value) ? value : []) as TableRow[]
    }
    return { ...view, formRef, rowsNow }
}

const has = (root: ParentNode, id: string) =>
    root.querySelector(`[testID="${id}"], [data-testid="${id}"]`) !== null
const menuRow = (id: string) => {
    const el = document.body.querySelector<HTMLElement>(`[data-testid="${id}"]`)
    if (!el) throw new Error(`no menu row ${id}`)
    return el
}

const COLUMN_KEYS = new Set(columns.map(c => c.key))
/** The column cells a row draws, in document order. */
function cellsOf(container: HTMLElement, rowId: string): string[] {
    const prefix = `form-table-lines-${rowId}-`
    const row = byTestId(container, `form-table-lines-row-${rowId}`)
    return [...row.querySelectorAll(`[testID^="${prefix}"]`)]
        .map(el => (el.getAttribute('testID') ?? '').slice(prefix.length))
        .filter(key => COLUMN_KEYS.has(key))
}

function openRowMenu(container: HTMLElement, rowId: string) {
    fireEvent.click(byTestId(container, `form-table-lines-menu-${rowId}`))
}

const pickLineAction = (container: HTMLElement, rowId: string, action: string) =>
    pickRowAction(container, 'lines', rowId, action)

/** Puts the keyboard in a cell, as a Tab or a click there does. */
function focusCell(container: HTMLElement, rowId: string, column: string) {
    fireEvent.focusIn(byTestId(container, `form-table-lines-${rowId}-${column}`))
}

function pressKeys(keys: string) {
    let fired = false
    act(() => {
        fired = createMatcher().feedAtom(keys, { inInput: true })
    })
    return fired
}

/** True when the last focus() call went to the element with this testID or inside it. */
function lastFocusIn(focus: { mock: { contexts: unknown[] } }, id: string) {
    const el = focus.mock.contexts.at(-1)
    const target = document.body.querySelector(`[testID="${id}"]`)
    return target !== null && el instanceof Node && (el === target || target.contains(el))
}

beforeEach(() => {
    resetScopes()
    scopeId = pushScope('compose')
})

afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    const registry = useShortcutRegistry.getState()
    for (const id of Array.from(registry.shortcuts.keys())) registry.unregister(id)
    resetScopes()
})

describe('TableField row kinds', () => {
    it("draws each row with its kind's cells in the first kind's column slots", () => {
        const { container } = setUp([r1, r2, r4])
        expect(has(container, 'form-table-lines-head-name')).toBe(true)
        expect(has(container, 'form-table-lines-head-qty')).toBe(true)
        expect(has(container, 'form-table-lines-head-done')).toBe(true)
        expect(has(container, 'form-table-lines-head-text')).toBe(false)
        expect(has(container, 'form-table-lines-head-rate')).toBe(false)
        expect(cellsOf(container, 'r1')).toEqual(['name', 'qty', 'done'])
        // A span note draws its one cell and nothing in the other slots.
        expect(cellsOf(container, 'r2')).toEqual(['text'])
        // The percent rate sits in the Qty slot, so it comes after Name, before Done.
        expect(cellsOf(container, 'r4')).toEqual(['name', 'rate', 'done'])
    })

    it('shows the kind label on a card', () => {
        const { container } = setUp([r1, r2], { width: 400 })
        expect(byTestId(container, 'form-table-lines-kind-r1').textContent).toBe('Item')
        expect(byTestId(container, 'form-table-lines-kind-r2').textContent).toBe('Note')
        expect(has(container, 'form-table-lines-menu-r2')).toBe(true)
    })

    it('gives each row one row menu button instead of the move and remove buttons', () => {
        const { container } = setUp([r1, r2, r3])
        expect(
            byTestId(container, 'form-table-lines-menu-r2').getAttribute('accessibilityLabel')
        ).toBe('Line 2 actions')
        for (const id of ['r1', 'r2', 'r3']) {
            expect(has(container, `form-table-lines-menu-${id}`)).toBe(true)
            for (const old of ['up', 'down', 'remove'])
                expect(has(container, `form-table-lines-${id}-${old}`)).toBe(false)
        }
    })

    it('lists the line actions in the row menu with their keys', () => {
        const { container } = setUp([r1, r3, r5])
        openRowMenu(container, 'r3')
        const rows = [
            ...document.body.querySelectorAll<HTMLElement>(
                '[data-testid^="form-table-lines-menu-"]'
            ),
        ]
        expect(rows.map(el => [el.getAttribute('data-testid'), el.textContent])).toEqual([
            ['form-table-lines-menu-insert-above', 'Insert line above'],
            ['form-table-lines-menu-insert-below', 'Insert line below'],
            ['form-table-lines-menu-up', 'Move up⌘⌥↑'],
            ['form-table-lines-menu-down', 'Move down⌘⌥↓'],
            ['form-table-lines-menu-duplicate', 'Duplicate⌘⌥D'],
            ['form-table-lines-menu-remove', 'Remove⌘⌥⌫'],
        ])
    })
})

describe('TableField row actions', () => {
    it('inserts an item line above a row and focuses its first cell', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container, rowsNow } = setUp([r1, r3])
        pickLineAction(container, 'r3', 'insert-above')
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toHaveLength(3))
        const added = rowsNow()[1]
        expect(rowsNow().map(r => r.id)).toEqual(['r1', added.id, 'r3'])
        expect(added).toMatchObject({ kind: 'item', name: null, qty: null })
        await waitFor(() =>
            expect(lastFocusIn(focus, `form-table-lines-${added.id}-name`)).toBe(true)
        )
    })

    it('inserts an item line below a row and focuses its first cell', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container, rowsNow } = setUp([r1, r3])
        pickLineAction(container, 'r1', 'insert-below')
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toHaveLength(3))
        const added = rowsNow()[1]
        expect(rowsNow().map(r => r.id)).toEqual(['r1', added.id, 'r3'])
        expect(added).toMatchObject({ kind: 'item', name: null, qty: null })
        await waitFor(() =>
            expect(lastFocusIn(focus, `form-table-lines-${added.id}-name`)).toBe(true)
        )
    })

    it('duplicates a line right below it from the row menu', async () => {
        const { container, rowsNow } = setUp([r1, r3])
        pickLineAction(container, 'r1', 'duplicate')
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toHaveLength(3))
        const copy = rowsNow()[1]
        expect(copy.id).not.toBe('r1')
        expect(rowsNow()).toEqual([r1, { ...r1, id: copy.id }, r3])
    })

    it('duplicates the focused line by its key and keeps focus in the same cell', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container, rowsNow } = setUp([r1, r3])
        focusCell(container, 'r1', 'qty')
        expect(pressKeys('$mod+Alt+d')).toBe(true)
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toHaveLength(3))
        const copy = rowsNow()[1]
        expect(rowsNow()).toEqual([r1, { ...r1, id: copy.id }, r3])
        await waitFor(() =>
            expect(lastFocusIn(focus, `form-table-lines-${copy.id}-qty`)).toBe(true)
        )
    })

    it('moves the focused line down by its key and keeps focus in the same cell', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container } = setUp([r1, r2, r3])
        focusCell(container, 'r1', 'qty')
        expect(pressKeys('$mod+Alt+ArrowDown')).toBe(true)
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual(['r2', 'r1', 'r3']))
        await waitFor(() => expect(lastFocusIn(focus, 'form-table-lines-r1-qty')).toBe(true))
    })

    it('removes a middle line by its key and focuses the same cell of the next line', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container } = setUp([r1, r3, r5])
        focusCell(container, 'r3', 'qty')
        expect(pressKeys('$mod+Alt+Backspace')).toBe(true)
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual(['r1', 'r5']))
        await waitFor(() => expect(lastFocusIn(focus, 'form-table-lines-r5-qty')).toBe(true))
    })

    it('removes the last line by its key and focuses the same cell of the previous line', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container } = setUp([r1, r3])
        focusCell(container, 'r3', 'qty')
        expect(pressKeys('$mod+Alt+Backspace')).toBe(true)
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual(['r1']))
        await waitFor(() => expect(lastFocusIn(focus, 'form-table-lines-r1-qty')).toBe(true))
    })

    it('removes both lines when two remove keys land before a re-render', () => {
        const { container, rowsNow } = setUp([r1, r3])
        act(() => {
            focusCell(container, 'r1', 'qty')
            createMatcher().feedAtom('$mod+Alt+Backspace', { inInput: true })
            focusCell(container, 'r3', 'qty')
            createMatcher().feedAtom('$mod+Alt+Backspace', { inInput: true })
        })
        expect(rowsNow()).toEqual([])
    })

    it('removes the only line from the row menu and focuses Add line', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container } = setUp([r1])
        pickLineAction(container, 'r1', 'remove')
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual([]))
        await waitFor(() => expect(lastFocusIn(focus, 'form-table-lines-add')).toBe(true))
    })
})

describe('TableField add actions', () => {
    it("adds an item line from a shortcut button labelled with the first kind's add label", () => {
        const { container, rowsNow } = setUp([r1])
        const add = byTestId(container, 'form-table-lines-add')
        expect(add.textContent).toContain('Add line')
        const keyed = add.matches('[aria-keyshortcuts]')
            ? add
            : add.querySelector('[aria-keyshortcuts]')
        expect(keyed?.getAttribute('aria-keyshortcuts')).toBe('Alt+Enter')
        fireEvent.click(add)
        expect(rowsNow()).toHaveLength(2)
        expect(rowsNow()[1]).toMatchObject({ kind: 'item', name: null })
    })

    it('offers the other kinds in the Add… menu with their keys', () => {
        const { container, rowsNow } = setUp([r1])
        fireEvent.click(byTestId(container, 'form-table-lines-add-more'))
        expect(menuRow('form-table-lines-add-note').textContent).toBe('Note line⌥T')
        expect(menuRow('form-table-lines-add-pct').textContent).toBe('Percent line⌥5')
        expect(has(document.body, 'form-table-lines-add-item')).toBe(false)
        fireEvent.click(menuRow('form-table-lines-add-note'))
        expect(rowsNow()).toHaveLength(2)
        expect(rowsNow()[1]).toMatchObject({ kind: 'note', text: null })
    })

    it('adds an item line on Enter in the addOnSubmit cell of the last row only', () => {
        const { container, rowsNow } = setUp([r1, r3])
        fireEvent(byTestId(container, 'form-table-lines-r1-qty'), new Event('SubmitEditing'))
        expect(rowsNow().map(r => r.id)).toEqual(['r1', 'r3'])
        fireEvent(byTestId(container, 'form-table-lines-r3-qty'), new Event('SubmitEditing'))
        expect(rowsNow()).toHaveLength(3)
        expect(rowsNow()[2]).toMatchObject({ kind: 'item', name: null, qty: null })
    })

    it('names a kind key after its add label', () => {
        expect(tableKindKeyLabel('Add line')).toBe('Add line')
        expect(tableKindKeyLabel('Description line')).toBe('Add description line')
        expect(tableKindKeyLabel('Subtotal')).toBe('Add subtotal')
    })
})

type FocusProps = { onFocus?: (event: unknown) => void }

/**
 * The cell input's own onFocus prop. On iOS React Native delivers focus to the
 * TextInput only; a wrapping View never hears it, so the input's own handler is
 * the one path that marks the row on native.
 */
function ownOnFocus(el: Element) {
    const key = Object.keys(el).find(k => k.startsWith('__reactProps$'))
    if (!key) return undefined
    return (el as unknown as Record<string, FocusProps>)[key]?.onFocus
}

/**
 * The testID of the element the last focus() call went to. A string, not the
 * element: a failed match would otherwise serialize the whole rendered tree.
 */
function lastFocusedId(focus: { mock: { contexts: unknown[] } }) {
    const el = focus.mock.contexts.at(-1)
    return el instanceof Element ? el.getAttribute('testID') : null
}

describe('TableField focus on native', () => {
    const realOS = Platform.OS
    afterEach(() => {
        Object.assign(Platform, { OS: realOS })
    })

    it("marks the row focused from a text cell's own focus event, so a row key fires", () => {
        const { container, rowsNow } = setUp([r1, r3, r5])
        const onFocus = ownOnFocus(byTestId(container, 'form-table-lines-r3-name'))
        expect(onFocus).toBeTypeOf('function')
        act(() => onFocus?.({}))
        expect(pressKeys('$mod+Alt+d')).toBe(true)
        const copy = rowsNow()[2]
        expect(rowsNow()).toEqual([r1, r3, { ...r3, id: copy.id }, r5])
    })

    it("marks the row focused from a number cell's own focus event, so a kind key adds after it", () => {
        const { container, rowsNow } = setUp([r1, r3, r5])
        const onFocus = ownOnFocus(byTestId(container, 'form-table-lines-r1-qty'))
        expect(onFocus).toBeTypeOf('function')
        act(() => onFocus?.({}))
        expect(pressKeys('Alt+t')).toBe(true)
        expect(rowsNow().map(r => r.id)).toEqual(['r1', rowsNow()[1].id, 'r3', 'r5'])
        expect(rowsNow()[1]).toMatchObject({ kind: 'note', text: null })
    })

    it('gives the text input of the duplicated line keyboard focus on iOS', async () => {
        const { container, rowsNow } = setUp([r1, r3])
        focusCell(container, 'r1', 'name')
        Object.assign(Platform, { OS: 'ios' })
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        expect(pressKeys('$mod+Alt+d')).toBe(true)
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toHaveLength(3))
        const copy = rowsNow()[1]
        await waitFor(() => expect(lastFocusedId(focus)).toBe(`form-table-lines-${copy.id}-name`))
    })

    it('gives the text input of the next line keyboard focus after a remove on iOS', async () => {
        const { container } = setUp([r1, r3, r5])
        focusCell(container, 'r3', 'name')
        Object.assign(Platform, { OS: 'ios' })
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        expect(pressKeys('$mod+Alt+Backspace')).toBe(true)
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual(['r1', 'r5']))
        await waitFor(() => expect(lastFocusedId(focus)).toBe('form-table-lines-r5-name'))
    })
})

describe('TableField host cells note their focus', () => {
    const seen = new Map<string, TableCellContext>()

    function FocusCell({ table, value }: BuiltinFieldProps) {
        if (table) seen.set(table.row.id, table)
        const id = table?.row.id ?? 'none'
        // Stands in for the host input's onFocus, which is where a host cell calls noteFocus.
        return (
            <Pressable testID={`note-focus-${id}`} onPress={() => table?.noteFocus()}>
                <Text>{String(value ?? '')}</Text>
            </Pressable>
        )
    }

    it('lets a host cell mark its row focused, so a kind key adds after that row', () => {
        seen.clear()
        const { container, rowsNow } = setUp([r1, r3, r5], {
            host: billHost('FocusCell'),
            ui: { components: { FocusCell } },
        })
        expect(seen.get('r3')?.noteFocus).toBeTypeOf('function')
        fireEvent.click(byTestId(container, 'note-focus-r3'))
        expect(pressKeys('Alt+t')).toBe(true)
        expect(rowsNow().map(r => r.id)).toEqual(['r1', 'r3', rowsNow()[2].id, 'r5'])
        expect(rowsNow()[2]).toMatchObject({ kind: 'note', text: null })
    })
})

type BlurProps = { onBlur?: (event: unknown) => void }

/**
 * The element's own onBlur prop. On iOS React Native delivers blur to the
 * input (or the Pressable) only; the row View never hears it.
 */
function ownOnBlur(el: Element) {
    const key = Object.keys(el).find(k => k.startsWith('__reactProps$'))
    if (!key) return undefined
    return (el as unknown as Record<string, BlurProps>)[key]?.onBlur
}

/** A focus or blur event as the input's own handler gets it. */
const nativeEvent = (el: Element) => ({ nativeEvent: {}, target: el, currentTarget: el })

describe('TableField forgets the focused row on native blur', () => {
    const realOS = Platform.OS
    afterEach(() => {
        Object.assign(Platform, { OS: realOS })
    })

    it("stops the row keys after a text cell's own blur event on iOS", () => {
        const { container, rowsNow } = setUp([r1, r3, r5])
        Object.assign(Platform, { OS: 'ios' })
        const input = byTestId(container, 'form-table-lines-r3-name')
        act(() => ownOnFocus(input)?.(nativeEvent(input)))
        const onBlur = ownOnBlur(input)
        expect(onBlur).toBeTypeOf('function')
        act(() => onBlur?.(nativeEvent(input)))
        expect(pressKeys('$mod+Alt+Backspace')).toBe(false)
        expect(rowsNow()).toEqual([r1, r3, r5])
    })

    it("stops the row keys after the row menu button's own blur event on iOS", () => {
        const { container, rowsNow } = setUp([r1, r3, r5])
        Object.assign(Platform, { OS: 'ios' })
        const button = byTestId(container, 'form-table-lines-menu-r3')
        act(() => ownOnFocus(button)?.(nativeEvent(button)))
        const onBlur = ownOnBlur(button)
        expect(onBlur).toBeTypeOf('function')
        act(() => onBlur?.(nativeEvent(button)))
        expect(pressKeys('$mod+Alt+Backspace')).toBe(false)
        expect(rowsNow()).toEqual([r1, r3, r5])
    })

    it('keeps the row keys on the cell focused next when the old cell blurs late', () => {
        const { container, rowsNow } = setUp([r1, r3, r5])
        Object.assign(Platform, { OS: 'ios' })
        const old = byTestId(container, 'form-table-lines-r3-name')
        const next = byTestId(container, 'form-table-lines-r5-name')
        act(() => ownOnFocus(old)?.(nativeEvent(old)))
        act(() => ownOnFocus(next)?.(nativeEvent(next)))
        act(() => ownOnBlur(old)?.(nativeEvent(old)))
        expect(pressKeys('$mod+Alt+Backspace')).toBe(true)
        expect(rowsNow()).toEqual([r1, r3])
    })
})

describe('TableField host cells note their blur', () => {
    const seen = new Map<string, TableCellContext>()

    // Stand in for the host input's onFocus and onBlur, where a host cell calls noteFocus / noteBlur.
    function BlurCell({ table, value }: BuiltinFieldProps) {
        if (table) seen.set(table.row.id, table)
        const id = table?.row.id ?? 'none'
        return (
            <>
                <Pressable testID={`note-focus-${id}`} onPress={() => table?.noteFocus()}>
                    <Text>{String(value ?? '')}</Text>
                </Pressable>
                <Pressable testID={`note-blur-${id}`} onPress={() => table?.noteBlur()}>
                    <Text>blur</Text>
                </Pressable>
            </>
        )
    }

    it("stops the row keys after a host cell's noteBlur", () => {
        seen.clear()
        const { container, rowsNow } = setUp([r1, r3, r5], {
            host: billHost('BlurCell'),
            ui: { components: { BlurCell } },
        })
        expect(seen.get('r3')?.noteBlur).toBeTypeOf('function')
        fireEvent.click(byTestId(container, 'note-focus-r3'))
        fireEvent.click(byTestId(container, 'note-blur-r3'))
        expect(pressKeys('$mod+Alt+Backspace')).toBe(false)
        expect(rowsNow()).toEqual([r1, r3, r5])
    })
})

/** A table with a date column and the row keys. */
function datedHost() {
    return asFormHost(
        hostDefSchema.parse({
            key: 'gizmos:log',
            label: 'Log',
            collection: 'gizmos_logs',
            formColumn: 'form',
            fieldsColumn: 'fields',
            columns: ['form', 'fields'],
            builtin: [
                {
                    key: 'lines',
                    label: 'Lines',
                    kind: 'table',
                    storage: 'rows',
                    columns: [
                        { key: 'name', label: 'Name', kind: 'text' },
                        { key: 'when', label: 'When', kind: 'date' },
                    ],
                    rowShortcuts: true,
                },
            ],
        })
    )
}

describe('TableField date cells on native', () => {
    const realOS = Platform.OS
    afterEach(() => {
        Object.assign(Platform, { OS: realOS })
    })

    it("gives the duplicated line's date input keyboard focus on iOS", async () => {
        const d1 = { id: 'd1', name: 'A', when: '2026-11-01' }
        const d2 = { id: 'd2', name: 'B', when: '2026-11-02' }
        const { container, rowsNow } = setUp([d1, d2], { host: datedHost() })
        focusCell(container, 'd1', 'when')
        Object.assign(Platform, { OS: 'ios' })
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        expect(pressKeys('$mod+Alt+d')).toBe(true)
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toHaveLength(3))
        const copy = rowsNow()[1]
        expect(rowsNow()).toEqual([d1, { ...d1, id: copy.id }, d2])
        await waitFor(() => expect(lastFocusedId(focus)).toBe(`form-table-lines-${copy.id}-when`))
    })
})

describe('TableField host cells and kinds', () => {
    const seen = new Map<string, TableCellContext>()
    const x1 = item('x1', 'X1', 1)
    const x2 = { ...blank, id: 'x2', kind: 'note', text: 'Bundle' }

    function NameCell({ table, value }: BuiltinFieldProps) {
        if (table) seen.set(table.row.id, table)
        const id = table?.row.id ?? 'none'
        return (
            <Pressable testID={`replace-${id}`} onPress={() => table?.replaceRow([x1, x2])}>
                <Text>{String(value ?? '')}</Text>
            </Pressable>
        )
    }

    it('gives a host cell the rows above it and its kind, and lets it replace its row', async () => {
        seen.clear()
        const { container, rowsNow } = setUp([r1, r2, r3, r4], {
            host: billHost('NameCell'),
            ui: { components: { NameCell } },
        })
        expect(seen.get('r3')?.rowKind).toBe('item')
        expect(seen.get('r3')?.rowsAbove).toEqual([r1, r2])
        expect(seen.get('r4')?.rowKind).toBe('pct')
        expect(seen.get('r1')?.rowsAbove).toEqual([])
        fireEvent.click(byTestId(container, 'replace-r3'))
        await waitFor(() =>
            expect(rowIdsOf(container, 'lines')).toEqual(['r1', 'r2', 'x1', 'x2', 'r4'])
        )
        expect(rowsNow()).toEqual([r1, r2, x1, x2, r4])
    })
})
