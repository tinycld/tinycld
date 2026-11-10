// @vitest-environment happy-dom
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import type { UseFormReturn } from 'react-hook-form'
import { Pressable, Text } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'
import { z } from 'zod'
import { asFormHost, orderHostInput } from '../../../lib/forms/__tests__/table-fixtures'
import { defaultDefinitionFor, hostDefSchema } from '../../../lib/forms/host'
import type { BuiltinFieldProps, FormHostUi } from '../../../lib/forms/host-ui'
import type { FormValues } from '../../../lib/forms/logic'
import { byTestId, pickRowAction, rowIdsOf, TableHarness } from './table-harness'

afterEach(cleanup)

/** The order host with its lines table required. */
const requiredLinesHost = hostDefSchema.parse({
    ...orderHostInput,
    builtin: orderHostInput.builtin.map(b => (b.key === 'lines' ? { ...b, required: true } : b)),
})
const host = asFormHost(requiredLinesHost)
const definition = defaultDefinitionFor(requiredLinesHost)

/** A host-drawn Item cell: its Controller registers `lines.<i>.item`, as a host's own table cells do. */
function ItemCell(props: BuiltinFieldProps) {
    const rowId = props.table?.row.id ?? 'none'
    return (
        <Pressable testID={`item-cell-${rowId}`} onPress={() => props.onChange('gear')}>
            <Text>{String(props.value ?? '')}</Text>
        </Pressable>
    )
}

const TOO_MANY = 'An order can have at most 1 line.'

/** A whole-record rule whose issue is at the table's own path. */
const atMostOneLine = z
    .object({ lines: z.array(z.unknown()).nullable().optional() })
    .passthrough()
    .superRefine((values, ctx) => {
        if ((values.lines?.length ?? 0) > 1)
            ctx.addIssue({ code: 'custom', path: ['lines'], message: TOO_MANY })
    })

const plainUi: FormHostUi = { components: { ItemCell } }
const ruledUi: FormHostUi = { components: { ItemCell }, recordSchema: atMostOneLine }

function setUp(initialLines: FormValues[], ui: FormHostUi = plainUi) {
    const formRef: { current: UseFormReturn<FormValues> | null } = { current: null }
    const view = render(
        <TableHarness
            host={host}
            definition={definition}
            initialValues={{ name: 'N', lines: initialLines }}
            ui={ui}
            formRef={formRef}
        />
    )
    const submit = () =>
        act(async () => {
            await formRef.current?.handleSubmit(() => undefined)()
        })
    return { ...view, formRef, submit }
}

/**
 * The texts the table field's own cell draws as its error: FieldCell's alert,
 * a direct child of the cell box, below the table. Row-cell errors live
 * inside the table, deeper in the box, and are not counted here.
 */
function tableErrorsOf(container: HTMLElement, key: string): string[] {
    const box = byTestId(container, `form-field-${key}-box`)
    return [...box.children]
        .filter(el => el.getAttribute('accessibilityRole') === 'alert')
        .map(el => el.textContent ?? '')
}

const row = (id: string) => ({ id, item: 'gear', qty: 1, rate: 2 })

describe('table field: its own error under the table cell', () => {
    it('shows "Lines is required" after a row was added and removed', async () => {
        const { container, submit } = setUp([])
        fireEvent.click(byTestId(container, 'form-table-lines-add'))
        const [added] = rowIdsOf(container, 'lines')
        expect(added).toBeTruthy()
        pickRowAction(container, 'lines', added, 'remove')
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual([]))
        await submit()
        await waitFor(() =>
            expect(tableErrorsOf(container, 'lines')).toEqual(['Lines is required'])
        )
    })

    it('shows a whole-record issue at the table path while rows are on screen', async () => {
        const { container, submit } = setUp([row('r1'), row('r2')], ruledUi)
        expect(rowIdsOf(container, 'lines')).toEqual(['r1', 'r2'])
        await submit()
        await waitFor(() => expect(tableErrorsOf(container, 'lines')).toEqual([TOO_MANY]))
    })

    it('shows "Lines is required" when no row was ever mounted', async () => {
        const { container, submit } = setUp([])
        await submit()
        await waitFor(() =>
            expect(tableErrorsOf(container, 'lines')).toEqual(['Lines is required'])
        )
    })

    it('shows a blank required cell under its row only, not as the table error', async () => {
        const { container, submit } = setUp([{ id: 'r1', item: null, qty: 1, rate: 2 }])
        await submit()
        const rowEl = byTestId(container, 'form-table-lines-row-r1')
        await waitFor(() => expect(rowEl.textContent).toContain('Item is required'))
        expect(tableErrorsOf(container, 'lines')).toEqual([])
    })

    it('names the table error in the error summary after a row has mounted', async () => {
        const { container, submit } = setUp([row('r1')])
        pickRowAction(container, 'lines', 'r1', 'remove')
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual([]))
        await submit()
        await waitFor(() =>
            expect(byTestId(container, 'form-error-summary').textContent).toContain(
                'Lines is required'
            )
        )
    })
})
