// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { asFormHost, fieldOf } from '../../../lib/forms/__tests__/table-fixtures'
import { defaultDefinitionFor, hostDefSchema } from '../../../lib/forms/host'
import type { FormValues } from '../../../lib/forms/logic'
import type { TableRow } from '../../../lib/forms/values'
import { useShortcutRegistry } from '../../../lib/shortcuts/registry'
import { pushScope, resetScopes, ScopeProvider } from '../../../lib/shortcuts/scopes'
import { OverlayProvider } from '../../overlay'
import { FieldInput } from '../FieldInput'
import { byTestId } from './table-harness'

/**
 * Name is weighted 3 and Qty 2; Done has no weight. A percent row draws its
 * rate in the Qty slot (`columnAt`), and a discount row leaves the Qty slot empty.
 */
const host = asFormHost(
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
                columns: [
                    { key: 'name', label: 'Name', kind: 'text', flex: 3 },
                    { key: 'qty', label: 'Qty', kind: 'number', flex: 2 },
                    { key: 'done', label: 'Done', kind: 'toggle' },
                    { key: 'rate', label: 'Rate', kind: 'number' },
                ],
                rowKinds: [
                    {
                        key: 'item',
                        label: 'Item',
                        addLabel: 'Add line',
                        columns: ['name', 'qty', 'done'],
                    },
                    {
                        key: 'pct',
                        label: 'Percent',
                        addLabel: 'Percent line',
                        columns: ['name', 'rate', 'done'],
                        columnAt: { rate: 'qty' },
                    },
                    {
                        key: 'disc',
                        label: 'Discount',
                        addLabel: 'Discount line',
                        columns: ['name', 'done'],
                    },
                ],
            },
        ],
    })
)

const blank = { name: null, qty: null, done: null, rate: null }
const rows: TableRow[] = [
    { ...blank, id: 'i1', kind: 'item', name: 'Widget', qty: 1 },
    { ...blank, id: 'p1', kind: 'pct', name: 'Off', rate: -10 },
    { ...blank, id: 'd1', kind: 'disc', name: 'Promo' },
]

let scopeId = 0
const owner = () => scopeId

function Harness({ width }: { width: number }) {
    const form = useForm<FormValues>({ defaultValues: { lines: rows } })
    return (
        <OverlayProvider>
            <ScopeProvider owner={owner}>
                <FieldInput
                    host={host}
                    ui={null}
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

/** The flex grow an element's inline style gives it ('' when it sets none). */
const flexOf = (el: HTMLElement) => el.style.flexGrow

/** The row's own child (its cell slot) that holds a column's cell. */
function slotOf(container: HTMLElement, rowId: string, column: string): HTMLElement {
    const row = byTestId(container, `form-table-lines-row-${rowId}`)
    let el: HTMLElement | null = byTestId(container, `form-table-lines-${rowId}-${column}`)
    while (el && el.parentElement !== row) el = el.parentElement
    if (!el) throw new Error(`no slot for ${rowId}/${column}`)
    return el
}

beforeEach(() => {
    resetScopes()
    scopeId = pushScope('compose')
})

afterEach(() => {
    cleanup()
    const registry = useShortcutRegistry.getState()
    for (const id of Array.from(registry.shortcuts.keys())) registry.unregister(id)
    resetScopes()
})

describe('TableField column flex weights (wide grid)', () => {
    it("gives a weighted column's header text and cell slot its weight", () => {
        const { container } = render(<Harness width={1000} />)
        expect(flexOf(byTestId(container, 'form-table-lines-head-name'))).toBe('3')
        expect(flexOf(byTestId(container, 'form-table-lines-head-qty'))).toBe('2')
        expect(flexOf(slotOf(container, 'i1', 'name'))).toBe('3')
        expect(flexOf(slotOf(container, 'i1', 'qty'))).toBe('2')
    })

    it('gives a column without a weight flex 1 in its header and cell slot', () => {
        const { container } = render(<Harness width={1000} />)
        expect(flexOf(byTestId(container, 'form-table-lines-head-done'))).toBe('1')
        expect(flexOf(slotOf(container, 'i1', 'done'))).toBe('1')
    })

    it('gives a columnAt cell and an empty slot the weight of the header column above them', () => {
        const { container } = render(<Harness width={1000} />)
        // The percent row's rate sits under Qty, so it takes Qty's weight.
        expect(flexOf(slotOf(container, 'p1', 'rate'))).toBe('2')
        // The discount row draws nothing in the Qty slot: name, empty, done, menu.
        const discount = byTestId(container, 'form-table-lines-row-d1')
        const empty = discount.children[1] as HTMLElement
        expect(empty.querySelector('[testID^="form-table-lines-d1-"]')).toBeNull()
        expect(flexOf(slotOf(container, 'd1', 'done'))).toBe('1')
        expect(flexOf(empty)).toBe('2')
    })
})

describe('TableField column flex weights (phone cards)', () => {
    it('draws the cards as before: no header, every cell, and no flex weight on a cell', () => {
        const { container } = render(<Harness width={300} />)
        expect(container.querySelector('[testID^="form-table-lines-head-"]')).toBeNull()
        for (const column of ['name', 'qty', 'done']) {
            const slot = slotOf(container, 'i1', column)
            expect(flexOf(slot)).toBe('')
        }
    })
})
