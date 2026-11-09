// @vitest-environment happy-dom
import { act, cleanup, render, renderHook, waitFor } from '@testing-library/react'
import type { UseFormReturn } from 'react-hook-form'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { orderDefinition, orderFormHost } from '../../../lib/forms/__tests__/table-fixtures'
import type { FormValues } from '../../../lib/forms/logic'
import { OverlayProvider } from '../../overlay'
import { FormRenderer } from '../FormRenderer'
import { useFormEdit } from '../use-form-edit'
import { pickRowAction, rowIdsOf } from './table-harness'

afterEach(cleanup)

const orderForm = {
    id: 'o1',
    host: orderFormHost.key,
    name: 'Order',
    slug: 'order',
    is_default: true,
    archived: false,
    order: 0,
    definition: orderDefinition,
}
vi.mock('../../../lib/forms/use-forms-of-host', () => ({
    useFormsOfHost: () => ({ forms: [orderForm], defaultForm: orderForm, isReady: true }),
}))

const ctx = { today: '2026-01-10', userId: 'me' }
const record = { id: 'w1', name: 'N', subtotal: 12, form: 'o1', fields: {} }
const a = { id: 'a', item: 'gear', qty: 2, rate: 3, amount: 6 }
const b = { id: 'b', item: 'cog', qty: 1, rate: 6, amount: 6 }
const tables = { lines: [a, b] }

/** A cell's path as TableField builds it: a plain string, so react-hook-form types its value as unknown. */
const qtyOf = (index: number): string => `lines.${index}.qty`

describe('useFormEdit with a host table', () => {
    it('patch sends the edited row with its amount, the new subtotal and an update plan', async () => {
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx, {}, tables))
        act(() => result.current.form.setValue(qtyOf(0), 3, { shouldDirty: true }))
        let patch: Awaited<ReturnType<typeof result.current.patch>> | undefined
        await act(async () => {
            patch = await result.current.patch()
        })
        const edited = { ...a, qty: 3, amount: 9 }
        expect(patch?.tables.lines).toEqual([edited, b])
        expect(patch?.columns).toEqual({ subtotal: 15 })
        expect(patch?.tableWrites.lines).toEqual({
            insert: [],
            update: [{ row: edited, position: 0 }],
            remove: [],
        })
        expect(patch?.written).toEqual({ lines: [{ ...a, qty: 3 }, b] })
    })

    it('a move and a removal plan new positions and the removed id', async () => {
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx, {}, tables))
        act(() => result.current.form.setValue('lines', [b], { shouldDirty: true }))
        let patch: Awaited<ReturnType<typeof result.current.patch>> | undefined
        await act(async () => {
            patch = await result.current.patch()
        })
        expect(patch?.tableWrites.lines).toEqual({
            insert: [],
            update: [{ row: b, position: 0 }],
            remove: ['a'],
        })
        expect(patch?.columns).toEqual({ subtotal: 6 })
    })

    it('rows not loaded: an edit leaves the stored subtotal alone', async () => {
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx))
        expect(result.current.form.getValues('lines')).toBeUndefined()
        act(() => result.current.form.setValue('name', 'M', { shouldDirty: true }))
        let patch: Awaited<ReturnType<typeof result.current.patch>> | undefined
        await act(async () => {
            patch = await result.current.patch()
        })
        expect(patch?.columns).toEqual({ name: 'M' })
        expect(patch?.tables).toEqual({})
        expect(patch?.tableWrites).toEqual({})
    })

    it('a stored row without its computed amount is not rewritten when another row changes', async () => {
        const stored = {
            lines: [
                { id: 'a', item: 'gear', qty: 2, rate: 3 },
                { id: 'b', item: 'cog', qty: 1, rate: 6 },
            ],
        }
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx, {}, stored))
        act(() => result.current.form.setValue(qtyOf(1), 2, { shouldDirty: true }))
        let patch: Awaited<ReturnType<typeof result.current.patch>> | undefined
        await act(async () => {
            patch = await result.current.patch()
        })
        expect(patch?.tableWrites.lines).toEqual({
            insert: [],
            update: [{ row: { id: 'b', item: 'cog', qty: 2, rate: 6, amount: 12 }, position: 1 }],
            remove: [],
        })
    })

    it('a cleared number cell is null in tables while written keeps the raw empty string', async () => {
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx, {}, tables))
        act(() => result.current.form.setValue(qtyOf(0), '', { shouldDirty: true }))
        let patch: Awaited<ReturnType<typeof result.current.patch>> | undefined
        await act(async () => {
            patch = await result.current.patch()
        })
        expect(patch?.tables.lines?.[0].qty).toBeNull()
        expect(patch?.written.lines).toEqual([{ ...a, qty: '' }, b])
    })

    it('markSaved leaves a saved table clean', async () => {
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx, {}, tables))
        act(() => result.current.form.setValue(qtyOf(1), 4, { shouldDirty: true }))
        let patch: Awaited<ReturnType<typeof result.current.patch>> | undefined
        await act(async () => {
            patch = await result.current.patch()
        })
        act(() => {
            if (patch) result.current.markSaved(patch.written)
        })
        expect(result.current.form.getFieldState('lines').isDirty).toBe(false)
    })

    it('markSaved leaves a table clean when the written row has the same values in another key order', () => {
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx, {}, tables))
        // A row TableField adds puts id first; the live rows the host passes back put id last.
        const added = { id: 'r1', item: 'gear', qty: 2, rate: 3, amount: 6 }
        const live = { item: 'gear', qty: 2, rate: 3, amount: 6, id: 'r1' }
        act(() => result.current.form.setValue('lines', [a, b, added], { shouldDirty: true }))
        expect(result.current.form.getFieldState('lines').isDirty).toBe(true)
        act(() => result.current.markSaved({ lines: [a, b, live] }))
        expect(result.current.form.getFieldState('lines').isDirty).toBe(false)
    })

    it('markSaved keeps a table dirty when a written row value differs from the form', () => {
        const { result } = renderHook(() => useFormEdit(orderFormHost, record, ctx, {}, tables))
        const added = { id: 'r1', item: 'gear', qty: 2, rate: 3, amount: 6 }
        const sent = { item: 'gear', qty: 3, rate: 3, amount: 6, id: 'r1' }
        act(() => result.current.form.setValue('lines', [a, b, added], { shouldDirty: true }))
        act(() => result.current.markSaved({ lines: [a, b, sent] }))
        expect(result.current.form.getFieldState('lines').isDirty).toBe(true)
    })
})

/** The stored row `a` has no qty or rate: its columns were added after it was written. */
const partial = {
    lines: [
        { id: 'a', item: 'gear' },
        { id: 'b', item: 'cog', qty: 5, rate: 1 },
    ],
}

function EditHarness({ formRef }: { formRef: { current: UseFormReturn<FormValues> | null } }) {
    const edit = useFormEdit(orderFormHost, record, ctx, {}, partial)
    formRef.current = edit.form
    if (!edit.definition) return null
    return (
        <OverlayProvider>
            <FormRenderer
                host={orderFormHost}
                ui={null}
                definition={edit.definition}
                layout="sections"
                form={edit.form}
                schemaRef={edit.schemaRef}
                context={ctx}
                recordId={record.id}
            />
        </OverlayProvider>
    )
}

describe('useFormEdit rows in the renderer', () => {
    it('a moved row that lacks a cell does not take its neighbour value', async () => {
        const formRef: { current: UseFormReturn<FormValues> | null } = { current: null }
        const { container } = render(<EditHarness formRef={formRef} />)
        pickRowAction(container, 'lines', 'a', 'down')
        await waitFor(() => expect(rowIdsOf(container, 'lines')).toEqual(['b', 'a']))
        expect(formRef.current?.getValues('lines')).toEqual([
            { id: 'b', item: 'cog', qty: 5, rate: 1 },
            { id: 'a', item: 'gear', qty: null, rate: null },
        ])
    })
})
