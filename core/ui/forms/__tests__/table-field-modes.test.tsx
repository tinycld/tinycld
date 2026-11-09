// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it } from 'vitest'
import {
    fieldOf,
    gizmos,
    orderDefinition,
    orderFormHost,
    partsDefinition,
    rowA,
} from '../../../lib/forms/__tests__/table-fixtures'
import type { FormField } from '../../../lib/forms/definition'
import type { FieldMode } from '../../../lib/forms/host-ui'
import type { FormValues } from '../../../lib/forms/logic'
import type { FormHost } from '../../../lib/packages/derive-form-hosts'
import { FieldInput } from '../FieldInput'

afterEach(cleanup)

interface HarnessProps {
    host: FormHost
    field: FormField
    mode: FieldMode
    width: number
    initial: FormValues
}

function Harness({ host, field, mode, width, initial }: HarnessProps) {
    const { control, getValues } = useForm<FormValues>({ defaultValues: initial })
    return (
        <FieldInput
            host={host}
            ui={null}
            field={field}
            control={control}
            getValues={getValues}
            mode={mode}
            containerWidth={width}
        />
    )
}

const parts = fieldOf(partsDefinition, 'parts')
const lines = fieldOf(orderDefinition, 'lines')
const has = (container: HTMLElement, id: string) =>
    container.querySelector(`[testID="${id}"]`) !== null

describe('TableField layouts and modes', () => {
    it('shows rows as cards on a narrow form, with the column names in each card', () => {
        const { container } = render(
            <Harness
                host={gizmos}
                field={parts}
                mode="create"
                width={400}
                initial={{ parts: [rowA] }}
            />
        )
        expect(has(container, 'form-table-parts-cards')).toBe(true)
        expect(has(container, 'form-table-parts-head-label')).toBe(false)
        expect(container.textContent).toContain('Label *')
        expect(has(container, 'form-table-parts-menu-a')).toBe(true)
        expect(has(container, 'form-table-parts-a-up')).toBe(false)
    })

    it('shows a grid on a wide form', () => {
        const { container } = render(
            <Harness
                host={gizmos}
                field={parts}
                mode="create"
                width={1000}
                initial={{ parts: [rowA] }}
            />
        )
        expect(has(container, 'form-table-parts-grid')).toBe(true)
        expect(has(container, 'form-table-parts-head-label')).toBe(true)
    })

    it('read mode shows the rows with no row controls', () => {
        const { container } = render(
            <Harness
                host={gizmos}
                field={parts}
                mode="read"
                width={1000}
                initial={{ parts: [rowA] }}
            />
        )
        expect(has(container, 'form-table-parts-row-a')).toBe(true)
        expect(has(container, 'form-table-parts-menu-a')).toBe(false)
        expect(has(container, 'form-table-parts-add')).toBe(false)
    })

    it('the design canvas shows the columns and no controls', () => {
        const { container } = render(
            <Harness host={gizmos} field={parts} mode="design" width={1000} initial={{}} />
        )
        expect(has(container, 'form-table-parts-head-qty')).toBe(true)
        expect(has(container, 'form-table-parts-empty')).toBe(true)
        expect(has(container, 'form-table-parts-add')).toBe(false)
    })

    it('a host table with no rows passed shows Loading rows and no controls', () => {
        const { container } = render(
            <Harness host={orderFormHost} field={lines} mode="edit" width={1000} initial={{}} />
        )
        expect(has(container, 'form-table-lines-pending')).toBe(true)
        expect(has(container, 'form-table-lines-add')).toBe(false)
    })

    it('a host table on a new record starts empty with Add row', () => {
        const { container } = render(
            <Harness
                host={orderFormHost}
                field={lines}
                mode="create"
                width={1000}
                initial={{ lines: [] }}
            />
        )
        expect(has(container, 'form-table-lines-pending')).toBe(false)
        expect(has(container, 'form-table-lines-add')).toBe(true)
    })
})
