// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { View } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'
import { FieldCell } from '../FieldCell'
import { FormLayout } from '../FormLayout'
import { FIELD_CELL_BORDER_WIDTH } from '../fields/table-layout'

afterEach(cleanup)

function byId(container: HTMLElement, id: string) {
    const el = container.querySelector(`[testID="${id}"]`)
    if (!el) throw new Error(`no ${id} rendered`)
    return el
}

const cellOf = (container: HTMLElement) => byId(container, 'form-field-sev')
const boxOf = (container: HTMLElement) => byId(container, 'form-field-sev-box')
const classOf = (el: Element) => el.getAttribute('class') ?? ''

function Cell(props: Partial<Parameters<typeof FieldCell>[0]>) {
    return (
        <FormLayout>
            <FieldCell
                fieldKey="sev"
                label="Severity"
                required={false}
                width="half"
                containerWidth={1000}
                {...props}
            >
                <View testID="child" />
            </FieldCell>
        </FormLayout>
    )
}

describe('FieldCell', () => {
    it('renders the label, a required marker and the error', () => {
        const { container, getByText } = render(
            <FormLayout>
                <FieldCell
                    fieldKey="sev"
                    label="Severity"
                    required
                    width="third"
                    containerWidth={1000}
                    error="Severity is required"
                >
                    <View testID="child" />
                </FieldCell>
            </FormLayout>
        )
        expect(getByText('Severity')).toBeTruthy()
        expect(getByText('*')).toBeTruthy()
        expect(getByText('Severity is required')).toBeTruthy()
        expect(cellOf(container).getAttribute('class')).toContain('basis-1/3')
    })
    it('gives the error its own test id from the field key', () => {
        const { container } = render(<Cell fieldKey="severity" error="Severity is required" />)
        expect(byId(container, 'form-field-severity-error').textContent).toBe(
            'Severity is required'
        )
    })
    it('draws no error element when the field has no error', () => {
        const { container } = render(<Cell fieldKey="severity" />)
        expect(byId(container, 'form-field-severity')).toBeTruthy()
        expect(container.querySelector('[testID="form-field-severity-error"]')).toBeNull()
    })
    it('derives the error test id from a given cell test id', () => {
        const { container } = render(
            <Cell fieldKey="severity" testID="custom" error="Severity is required" />
        )
        expect(byId(container, 'custom-error').textContent).toBe('Severity is required')
        expect(container.querySelector('[testID="form-field-severity-error"]')).toBeNull()
    })
    it('draws no error element in a bare cell', () => {
        const { container, queryByText } = render(
            <Cell fieldKey="severity" isBare error="Severity is required" />
        )
        expect(byId(container, 'form-field-severity')).toBeTruthy()
        expect(container.querySelector('[testID="form-field-severity-error"]')).toBeNull()
        expect(queryByText('Severity is required')).toBeNull()
    })
    it('collapses the width class in a narrow container', () => {
        const { container } = render(
            <FieldCell
                fieldKey="sev"
                label="Severity"
                required={false}
                width="quarter"
                containerWidth={400}
            >
                <View testID="child" />
            </FieldCell>
        )
        expect(cellOf(container).getAttribute('class')).toContain('basis-1/2')
    })
    it('renders the child inside the cell', () => {
        const { container } = render(
            <FieldCell
                fieldKey="sev"
                label="Severity"
                required={false}
                width="full"
                containerWidth={1000}
            >
                <View testID="child" />
            </FieldCell>
        )
        expect(cellOf(container).querySelector('[testID="child"]')).toBeTruthy()
    })
    it('shows a primary border only when selected', () => {
        const { container, rerender } = render(
            <FieldCell
                fieldKey="sev"
                label="Severity"
                required={false}
                width="full"
                containerWidth={1000}
            >
                <View testID="child" />
            </FieldCell>
        )
        expect(classOf(boxOf(container))).toContain('border-transparent')
        rerender(
            <FieldCell
                fieldKey="sev"
                label="Severity"
                required={false}
                width="full"
                containerWidth={1000}
                isSelected
            >
                <View testID="child" />
            </FieldCell>
        )
        expect(classOf(boxOf(container))).toContain('border-primary')
    })
    it('keeps the basis and padding on the root and the border on the inner box', () => {
        const { container } = render(<Cell isSelected />)
        const root = cellOf(container)
        const box = boxOf(container)
        expect(box.parentElement).toBe(root)
        expect(classOf(root)).toContain('basis-1/2')
        expect(classOf(root)).toContain('px-1.5')
        expect(classOf(root)).not.toContain('border')
        expect((box as HTMLElement).style.borderWidth).toBe(`${FIELD_CELL_BORDER_WIDTH}px`)
        expect(classOf(box)).toContain('rounded-md')
        expect(classOf(box)).not.toContain('basis-')
        expect(box.querySelector('[testID="child"]')).toBeTruthy()
    })
    it('draws the drop bar on the inner box edge and tints the box', () => {
        const { container, rerender } = render(<Cell />)
        expect(classOf(boxOf(container))).not.toContain('bg-primary/10')
        rerender(<Cell dropIndicator="before" />)
        const bar = byId(container, 'form-field-sev-drop-before')
        expect(bar.parentElement).toBe(boxOf(container))
        expect(classOf(bar)).toContain('left-0')
        expect(classOf(bar)).toContain('w-[3px]')
        expect(classOf(boxOf(container))).toContain('bg-primary/10')
        rerender(<Cell dropIndicator="after" />)
        expect(classOf(byId(container, 'form-field-sev-drop-after'))).toContain('right-0')
        expect(container.querySelector('[testID="form-field-sev-drop-before"]')).toBeNull()
    })
    it('lays a full-width cell’s drop bars along its top and bottom edges', () => {
        const { container, rerender } = render(<Cell width="full" dropIndicator="before" />)
        const top = byId(container, 'form-field-sev-drop-before')
        expect(classOf(top)).toContain('top-0')
        expect(classOf(top)).toContain('h-[3px]')
        expect(classOf(top)).not.toContain('w-[3px]')
        rerender(<Cell width="full" dropIndicator="after" />)
        expect(classOf(byId(container, 'form-field-sev-drop-after'))).toContain('bottom-0')
        // A half cell collapsed to full width in a narrow container is full width too.
        rerender(<Cell width="half" containerWidth={400} dropIndicator="after" />)
        expect(classOf(byId(container, 'form-field-sev-drop-after'))).toContain('bottom-0')
    })
    it('dims the cell while it is dragged', () => {
        const { container, rerender } = render(<Cell />)
        expect(classOf(cellOf(container))).not.toContain('opacity-50')
        rerender(<Cell isDragging />)
        expect(classOf(cellOf(container))).toContain('opacity-50')
    })
})
