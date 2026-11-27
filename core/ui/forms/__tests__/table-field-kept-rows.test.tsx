// @vitest-environment happy-dom
import { act, cleanup, fireEvent, render, renderHook } from '@testing-library/react'
import { type UseFormReturn, useForm } from 'react-hook-form'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { asFormHost } from '../../../lib/forms/__tests__/table-fixtures'
import { defaultDefinitionFor, hostDefSchema } from '../../../lib/forms/host'
import type { FormHostUi } from '../../../lib/forms/host-ui'
import type { FormValues } from '../../../lib/forms/logic'
import type { TableRow } from '../../../lib/forms/values'
import { createMatcher } from '../../../lib/shortcuts/matcher'
import { useShortcutRegistry } from '../../../lib/shortcuts/registry'
import { pushScope, resetScopes, ScopeProvider } from '../../../lib/shortcuts/scopes'
import { TABLE_ROW_KEYS, useTableField } from '../fields/use-table-field'
import { byTestId, rowIdsOf, TableHarness } from './table-harness'

/**
 * A host may keep some rows of a table (rows other records name): such a row
 * offers no Duplicate and no Remove, from its menu or its keys. The fictional
 * rule here keeps a row named "Kept".
 */

const hostDef = hostDefSchema.parse({
    key: 'gizmos:crate',
    label: 'Crate',
    collection: 'gizmos_crates',
    formColumn: 'form',
    fieldsColumn: 'fields',
    columns: ['form', 'fields'],
    builtin: [
        {
            key: 'lines',
            label: 'Lines',
            kind: 'table',
            storage: 'rows',
            rowShortcuts: true,
            columns: [
                { key: 'name', label: 'Name', kind: 'text' },
                { key: 'qty', label: 'Qty', kind: 'number' },
            ],
        },
    ],
})
const host = asFormHost(hostDef)
const definition = defaultDefinitionFor(hostDef)
const linesColumns = hostDef.builtin.find(b => b.key === 'lines')?.columns ?? []

const kept = { id: 'k', name: 'Kept', qty: 1 }
const free = { id: 'f', name: 'Free', qty: 2 }

const ACTIONS = ['insert-above', 'insert-below', 'up', 'down', 'duplicate', 'remove'] as const

const keepNamedKept = (_table: string, row: TableRow) => row.name !== 'Kept'
const uiWith = (canRemoveRow?: FormHostUi['canRemoveRow']): FormHostUi => ({
    components: {},
    canRemoveRow,
})

let scopeId = 0
const owner = () => scopeId

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

function setUp(ui: FormHostUi | null, rows: TableRow[] = [kept, free]) {
    const formRef: { current: UseFormReturn<FormValues> | null } = { current: null }
    const view = render(
        <ScopeProvider owner={owner}>
            <TableHarness
                host={host}
                definition={definition}
                initialValues={{ lines: rows }}
                ui={ui}
                formRef={formRef}
            />
        </ScopeProvider>
    )
    const rowsNow = () => {
        const value = formRef.current?.getValues('lines')
        return (Array.isArray(value) ? value : []) as TableRow[]
    }
    return { ...view, rowsNow }
}

/** The actions the open row menu draws; the menu renders in the overlay host. */
function menuActions(): string[] {
    return ACTIONS.filter(
        action =>
            document.body.querySelector(`[data-testid="form-table-lines-menu-${action}"]`) !== null
    )
}

function openMenu(container: HTMLElement, rowId: string) {
    fireEvent.click(byTestId(container, `form-table-lines-menu-${rowId}`))
}

function pressKeys(keys: string) {
    let fired = false
    act(() => {
        fired = createMatcher().feedAtom(keys, { inInput: true })
    })
    return fired
}

function focusCell(container: HTMLElement, rowId: string, column: string) {
    fireEvent.focusIn(byTestId(container, `form-table-lines-${rowId}-${column}`))
}

describe('TableField canRemoveRow', () => {
    it('without a predicate offers Duplicate and Remove on the first row', () => {
        const { container } = setUp(uiWith())
        openMenu(container, 'k')
        expect(menuActions()).toEqual([...ACTIONS])
    })

    it('without a predicate offers Duplicate and Remove on the second row', () => {
        const { container } = setUp(null)
        openMenu(container, 'f')
        expect(menuActions()).toEqual([...ACTIONS])
    })

    it("passes the table's field key and the row's current cells", () => {
        const canRemoveRow = vi.fn(keepNamedKept)
        const { container } = setUp(uiWith(canRemoveRow))
        fireEvent.input(byTestId(container, 'form-table-lines-f-qty'), { target: { value: '7' } })
        canRemoveRow.mockClear()
        openMenu(container, 'f')
        expect(canRemoveRow).toHaveBeenCalledWith(
            'lines',
            expect.objectContaining({ id: 'f', name: 'Free', qty: 7 })
        )
    })

    it('draws no Duplicate and no Remove on a kept row; Insert and Move stay', () => {
        const { container } = setUp(uiWith(keepNamedKept))
        openMenu(container, 'k')
        expect(menuActions()).toEqual(['insert-above', 'insert-below', 'up', 'down'])
    })

    it('still offers Duplicate and Remove on a row the host does not keep', () => {
        const { container } = setUp(uiWith(keepNamedKept))
        openMenu(container, 'f')
        expect(menuActions()).toEqual([...ACTIONS])
    })

    it('the remove and duplicate keys change no row when focus is in a kept row', () => {
        const { container, rowsNow } = setUp(uiWith(keepNamedKept))
        focusCell(container, 'k', 'qty')
        expect(pressKeys(TABLE_ROW_KEYS.remove)).toBe(true)
        expect(pressKeys(TABLE_ROW_KEYS.duplicate)).toBe(true)
        expect(rowsNow()).toEqual([kept, free])
        expect(rowIdsOf(container, 'lines')).toEqual(['k', 'f'])
    })

    it('the remove key still removes a row the host does not keep', () => {
        const { container, rowsNow } = setUp(uiWith(keepNamedKept))
        focusCell(container, 'f', 'qty')
        expect(pressKeys(TABLE_ROW_KEYS.remove)).toBe(true)
        expect(rowsNow()).toEqual([kept])
    })
})

describe('useTableField canRemoveRow', () => {
    function renderTable(canRemoveRow?: (row: TableRow) => boolean) {
        const formRef: { current: UseFormReturn<FormValues> | null } = { current: null }
        const { result } = renderHook(() => {
            const form = useForm<FormValues>({ defaultValues: { lines: [kept, free] } })
            formRef.current = form
            return useTableField({
                control: form.control,
                getValues: form.getValues,
                name: 'lines',
                columns: linesColumns,
                computedRows: undefined,
                isPendingWhenAbsent: false,
                isEditable: true,
                rowShortcuts: false,
                canRemoveRow,
            })
        })
        const rowsNow = () => formRef.current?.getValues('lines') as TableRow[]
        return { result, rowsNow }
    }

    it('remove and duplicate change no row for a kept row', () => {
        const { result, rowsNow } = renderTable(row => row.name !== 'Kept')
        act(() => result.current.remove('k'))
        act(() => result.current.duplicate('k'))
        expect(rowsNow()).toEqual([kept, free])
    })

    it('remove and duplicate act on a row the predicate allows', () => {
        const { result, rowsNow } = renderTable(row => row.name !== 'Kept')
        act(() => result.current.duplicate('f'))
        expect(rowsNow().map(r => r.name)).toEqual(['Kept', 'Free', 'Free'])
        act(() => result.current.remove('f'))
        expect(rowsNow().map(r => r.name)).toEqual(['Kept', 'Free'])
        expect(rowsNow()[1]?.id).not.toBe('f')
    })
})
