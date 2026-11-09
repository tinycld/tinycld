// @vitest-environment happy-dom
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import type { UseFormReturn } from 'react-hook-form'
import { AccessibilityInfo, Platform, Pressable, Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
    gizmos,
    orderDefinition,
    orderFormHost,
    partsDefinition,
    rowA,
    rowB,
} from '../../../lib/forms/__tests__/table-fixtures'
import type { BuiltinFieldProps } from '../../../lib/forms/host-ui'
import type { FormValues } from '../../../lib/forms/logic'
import { byTestId, menuItem, pickRowAction, rowIdsOf, TableHarness } from './table-harness'

afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
})

function setUp(rows = [rowA, rowB]) {
    const formRef: { current: UseFormReturn<FormValues> | null } = { current: null }
    const view = render(
        <TableHarness
            host={gizmos}
            definition={partsDefinition}
            initialValues={{ name: 'N', parts: rows }}
            formRef={formRef}
        />
    )
    const rowsNow = () => {
        const rows = formRef.current?.getValues('parts')
        return Array.isArray(rows) ? rows : []
    }
    return { ...view, formRef, rowsNow }
}

describe('TableField', () => {
    it('shows a header with a star on required columns, and one row per value', () => {
        const { container } = setUp()
        expect(byTestId(container, 'form-table-parts-head-label').textContent).toBe('Label *')
        expect(byTestId(container, 'form-table-parts-head-done').textContent).toBe('Done')
        expect(rowIdsOf(container, 'parts')).toEqual(['a', 'b'])
    })

    it('adds a row with every editable cell empty', () => {
        const { container, rowsNow } = setUp()
        fireEvent.click(byTestId(container, 'form-table-parts-add'))
        expect(rowIdsOf(container, 'parts')).toHaveLength(3)
        expect(rowsNow()[2]).toEqual({
            id: expect.any(String),
            label: null,
            qty: null,
            done: null,
            due: null,
            size: null,
        })
    })

    it('edits a cell through its own input', () => {
        const { container, rowsNow } = setUp()
        fireEvent.input(byTestId(container, 'form-table-parts-a-label'), {
            target: { value: 'Axle' },
        })
        expect(rowsNow()[0]).toMatchObject({ id: 'a', label: 'Axle' })
    })

    it('shows computed cells and totals live', () => {
        const { container } = setUp()
        expect(byTestId(container, 'form-table-parts-a-double').textContent).toBe('2')
        expect(byTestId(container, 'form-input-total_qty').textContent).toBe('3')
        fireEvent.input(byTestId(container, 'form-table-parts-a-qty'), { target: { value: '5' } })
        expect(byTestId(container, 'form-table-parts-a-double').textContent).toBe('10')
        expect(byTestId(container, 'form-input-total_qty').textContent).toBe('7')
    })

    it('keeps a cell edit when its row moves', async () => {
        const { container, rowsNow } = setUp()
        fireEvent.input(byTestId(container, 'form-table-parts-a-qty'), { target: { value: '5' } })
        pickRowAction(container, 'parts', 'a', 'down')
        await waitFor(() => expect(rowIdsOf(container, 'parts')).toEqual(['b', 'a']))
        expect(rowsNow()).toEqual([rowB, { ...rowA, qty: 5 }])
    })

    it('disables Move up on the first row and Move down on the last', () => {
        const { container, rowsNow } = setUp()
        expect(
            byTestId(container, 'form-table-parts-menu-a').getAttribute('accessibilityLabel')
        ).toBe('Line 1 actions')
        pickRowAction(container, 'parts', 'a', 'up')
        pickRowAction(container, 'parts', 'b', 'down')
        expect(rowsNow()).toEqual([rowA, rowB])
        fireEvent.click(byTestId(container, 'form-table-parts-menu-a'))
        expect(menuItem('parts', 'up').getAttribute('aria-disabled')).toBe('true')
        expect(menuItem('parts', 'down').getAttribute('aria-disabled')).not.toBe('true')
    })

    it('keeps focus on the moved row menu button so a keyboard user can press again', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const third = { ...rowB, id: 'c', label: 'C' }
        const { container } = setUp([rowA, rowB, third])
        pickRowAction(container, 'parts', 'a', 'down')
        await waitFor(() => expect(rowIdsOf(container, 'parts')).toEqual(['b', 'a', 'c']))
        await waitFor(() =>
            expect(focus.mock.contexts.at(-1)).toBe(byTestId(container, 'form-table-parts-menu-a'))
        )
        pickRowAction(container, 'parts', 'a', 'down')
        await waitFor(() => expect(rowIdsOf(container, 'parts')).toEqual(['b', 'c', 'a']))
        await waitFor(() =>
            expect(focus.mock.contexts.at(-1)).toBe(byTestId(container, 'form-table-parts-menu-a'))
        )
    })

    it('removes the only row from its menu and puts focus on Add row', async () => {
        const focus = vi.spyOn(HTMLElement.prototype, 'focus')
        const { container, rowsNow } = setUp([rowA])
        pickRowAction(container, 'parts', 'a', 'remove')
        expect(rowsNow()).toEqual([])
        await waitFor(() =>
            expect(focus.mock.contexts.at(-1)).toBe(byTestId(container, 'form-table-parts-add'))
        )
    })

    it('says so when there are no rows', () => {
        const { container } = setUp([])
        expect(byTestId(container, 'form-table-parts-empty').textContent).toBe('No rows yet.')
    })
})

/** Picks an item and, in the same press, sets the row's rate, as a product picker does. */
function ItemCell({ value, onChange, table }: BuiltinFieldProps) {
    const pick = () => {
        onChange('Widget')
        table?.onRowChange({ rate: 9 })
    }
    return (
        <Pressable onPress={pick} testID={`pick-${table?.rowIndex}`}>
            <Text>{String(value ?? '')}</Text>
        </Pressable>
    )
}

describe('TableField writes', () => {
    it('keeps a cell change and a row change made in one handler', () => {
        const formRef: { current: UseFormReturn<FormValues> | null } = { current: null }
        const line = { id: 'l1', item: null, qty: 1, rate: null }
        const { container } = render(
            <TableHarness
                host={orderFormHost}
                definition={orderDefinition}
                ui={{ components: { ItemCell } }}
                initialValues={{ name: 'N', lines: [line] }}
                formRef={formRef}
            />
        )
        fireEvent.click(byTestId(container, 'pick-0'))
        expect(formRef.current?.getValues('lines')).toEqual([{ ...line, item: 'Widget', rate: 9 }])
    })

    it('keeps both moves when two presses land before a re-render', () => {
        const third = { ...rowB, id: 'c', label: 'C' }
        const { container, rowsNow } = setUp([rowA, rowB, third])
        fireEvent.click(byTestId(container, 'form-table-parts-menu-a'))
        const down = menuItem('parts', 'down')
        act(() => {
            down.click()
            down.click()
        })
        expect(rowsNow()).toEqual([rowB, third, rowA])
    })

    it('keeps both rows when Add row is pressed twice before a re-render', () => {
        const { container, rowsNow } = setUp()
        const add = byTestId(container, 'form-table-parts-add')
        act(() => {
            add.click()
            add.click()
        })
        expect(rowsNow()).toHaveLength(4)
    })
})

describe('TableField cell labels', () => {
    it('names a toggle cell by its column and row, without repeating the header', () => {
        const { container } = setUp()
        const cell = byTestId(container, 'form-table-parts-b-done')
        expect(cell.querySelector('[accessibilityLabel="Done, row 2"]')).not.toBeNull()
        expect(cell.textContent).toBe('')
    })

    it('names a select cell by its column and row, without repeating the header', () => {
        const { container } = setUp()
        const cell = byTestId(container, 'form-table-parts-b-size')
        expect(cell.querySelector('[accessibilityLabel="Size, row 2"]')).not.toBeNull()
        expect(cell.textContent).toBe('SmallLarge')
    })
})

describe('TableField focus on native', () => {
    const realOS = Platform.OS
    afterEach(() => {
        Object.assign(Platform, { OS: realOS })
    })

    it('puts the screen reader on the moved row menu button after a menu move', async () => {
        const send = vi.spyOn(AccessibilityInfo, 'sendAccessibilityEvent')
        const { container } = setUp()
        fireEvent.click(byTestId(container, 'form-table-parts-menu-a'))
        const down = menuItem('parts', 'down')
        // The menu drew as on web; the focus move after the press runs as on iOS.
        Object.assign(Platform, { OS: 'ios' })
        fireEvent.click(down)
        await waitFor(() => expect(rowIdsOf(container, 'parts')).toEqual(['b', 'a']))
        await waitFor(() =>
            expect(send).toHaveBeenLastCalledWith(
                byTestId(container, 'form-table-parts-menu-a'),
                'focus'
            )
        )
    })

    it('puts the screen reader on Add row after the last row is removed', async () => {
        const send = vi.spyOn(AccessibilityInfo, 'sendAccessibilityEvent')
        const { container } = setUp([rowA])
        fireEvent.click(byTestId(container, 'form-table-parts-menu-a'))
        const remove = menuItem('parts', 'remove')
        // The menu drew as on web; the focus move after the press runs as on iOS.
        Object.assign(Platform, { OS: 'ios' })
        fireEvent.click(remove)
        await waitFor(() =>
            expect(send).toHaveBeenLastCalledWith(
                byTestId(container, 'form-table-parts-add'),
                'focus'
            )
        )
    })
})
