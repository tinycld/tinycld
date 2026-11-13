// @vitest-environment happy-dom
import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { View } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'
import { definitionSchema } from '../../../lib/forms/definition'
import { hostDefSchema } from '../../../lib/forms/host'
import { FieldCell } from '../FieldCell'
import { FormLayout } from '../FormLayout'
import { FormRenderer } from '../FormRenderer'
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
    // The hint draws under the input, so a field with a hint keeps its input level with
    // the inputs of the other fields in its row (human, 2026-11-12).
    it('draws the description after the input and before the error, both shown', () => {
        const { container, getByText } = render(
            <Cell description="How bad it is" error="Severity is required" />
        )
        const child = byId(container, 'child')
        const hint = getByText('How bad it is')
        const error = byId(container, 'form-field-sev-error')
        expect(child.compareDocumentPosition(hint) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
        expect(hint.compareDocumentPosition(error) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
        expect(error.textContent).toBe('Severity is required')
        expect(classOf(hint)).toContain('mt-1')
        expect(classOf(hint)).not.toContain('mb-1')
    })
    it('draws the description after the input in a designer cell too', () => {
        const { container, getByText } = render(
            <Cell description="How bad it is" onPress={() => undefined} />
        )
        const child = byId(container, 'child')
        const hint = getByText('How bad it is')
        expect(child.compareDocumentPosition(hint) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
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

const tokensOf = (el: Element) => classOf(el).split(/\s+/).filter(Boolean)

/** The classes of a cell sized by its content: basis auto, never grown or shrunk, capped at the row. */
function expectContentSized(el: Element) {
    const tokens = tokensOf(el)
    expect(tokens).toEqual(
        expect.arrayContaining(['basis-auto', 'grow-0', 'shrink-0', 'max-w-full'])
    )
    expect(tokens).not.toContain('min-w-0')
    expect(tokens.filter(t => /^basis-(full|1\/\d)$/.test(t))).toEqual([])
}

/** Three quarters, a fit-content cell, and a quarter that is full width on narrow screens. */
function Row({ containerWidth }: { containerWidth: number }) {
    return (
        <FormLayout>
            <FieldCell
                fieldKey="start"
                label="Start"
                required={false}
                width="quarter"
                containerWidth={containerWidth}
            >
                <View />
            </FieldCell>
            <FieldCell
                fieldKey="end"
                label="End"
                required={false}
                width="quarter"
                containerWidth={containerWidth}
            >
                <View />
            </FieldCell>
            <FieldCell
                fieldKey="team"
                label="Team"
                required={false}
                width="quarter"
                narrowWidth="full"
                containerWidth={containerWidth}
            >
                <View />
            </FieldCell>
            <FieldCell
                fieldKey="level"
                label="Level"
                required={false}
                width="auto"
                containerWidth={containerWidth}
            >
                <View />
            </FieldCell>
        </FormLayout>
    )
}

describe('a row with a fit-content cell and a narrow width', () => {
    const cell = (container: HTMLElement, key: string) => byId(container, `form-field-${key}`)

    it('keeps the quarters a quarter and sizes the fit-content cell by its content on a wide form', () => {
        const { container } = render(<Row containerWidth={1280} />)
        for (const key of ['start', 'end', 'team']) {
            expect(tokensOf(cell(container, key))).toContain('basis-1/4')
        }
        expectContentSized(cell(container, 'level'))
    })

    it('halves the quarters, keeps the fit-content cell, and uses the narrow width on a narrow form', () => {
        const { container } = render(<Row containerWidth={500} />)
        expect(tokensOf(cell(container, 'start'))).toContain('basis-1/2')
        expect(tokensOf(cell(container, 'end'))).toContain('basis-1/2')
        expect(tokensOf(cell(container, 'team'))).toContain('basis-full')
        expect(tokensOf(cell(container, 'team'))).not.toContain('basis-1/2')
        expectContentSized(cell(container, 'level'))
    })

    it('gives a bare fit-content cell the same width classes', () => {
        for (const width of [1280, 500]) {
            const { container, unmount } = render(
                <FormLayout>
                    <FieldCell
                        fieldKey="level"
                        label="Level"
                        required={false}
                        width="auto"
                        containerWidth={width}
                        isBare
                    >
                        <View />
                    </FieldCell>
                </FormLayout>
            )
            expectContentSized(cell(container, 'level'))
            unmount()
        }
    })

    it('applies the narrow width to a bare cell too', () => {
        const { container } = render(
            <FormLayout>
                <FieldCell
                    fieldKey="team"
                    label="Team"
                    required={false}
                    width="quarter"
                    narrowWidth="full"
                    containerWidth={500}
                    isBare
                >
                    <View />
                </FieldCell>
            </FormLayout>
        )
        expect(tokensOf(cell(container, 'team'))).toContain('basis-full')
    })
})

const tinyHost = {
    ...hostDefSchema.parse({
        key: 'gizmos:tiny',
        label: 'Tiny',
        collection: 'gizmos_tinies',
        formColumn: 'form',
        fieldsColumn: 'fields',
        columns: ['name', 'form', 'fields'],
        builtin: [{ key: 'name', label: 'Name', kind: 'text', storage: 'column' }],
    }),
    pkgSlug: 'gizmos',
    pkgName: 'Gizmos',
    ui: async () => ({ components: {} }),
}

const narrowDefinition = definitionSchema.parse({
    version: 1,
    pages: [{ key: 'main', title: 'Main' }],
    fields: [
        { key: 'name', kind: 'builtin', page: 'main', label: 'Name', width: 'full' },
        { key: 'start', kind: 'text', page: 'main', label: 'Start', width: 'quarter' },
        {
            key: 'team',
            kind: 'text',
            page: 'main',
            label: 'Team',
            width: 'quarter',
            narrowWidth: 'full',
        },
    ],
})

function RendererHarness() {
    const form = useForm<Record<string, unknown>>({ defaultValues: {} })
    return (
        <FormRenderer
            host={tinyHost}
            ui={null}
            definition={narrowDefinition}
            layout="wizard"
            form={form}
            context={{ today: '2026-01-01', userId: 'u' }}
        />
    )
}

describe('FormRenderer and a field narrow width', () => {
    const fireLayout = (node: Element, width: number) =>
        fireEvent(node, new CustomEvent('rn-layout', { detail: { width, height: 1, x: 0, y: 0 } }))

    it('passes the field narrowWidth to its cell', () => {
        const { container } = render(<RendererHarness />)
        const root = byId(container, 'form-renderer')
        const teamClasses = () => tokensOf(byId(container, 'form-field-team'))
        act(() => fireLayout(root, 1280))
        expect(teamClasses()).toContain('basis-1/4')
        act(() => fireLayout(root, 500))
        expect(teamClasses()).toContain('basis-full')
        expect(tokensOf(byId(container, 'form-field-start'))).toContain('basis-1/2')
    })
})
