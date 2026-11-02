// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render } from '@testing-library/react'
import { useForm, useWatch } from 'react-hook-form'
import { Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { widgetHost } from '../../../lib/forms/__tests__/fixtures'
import type { FormField } from '../../../lib/forms/definition'
import type { CollabDoc, FormHostUi } from '../../../lib/forms/host-ui'
import type { FormValues } from '../../../lib/forms/logic'
import { FieldInput, rendersOwnLabel } from '../FieldInput'

afterEach(cleanup)

// FileField stamps the uploader; the signed-in user is all it reads from auth.
vi.mock('@tinycld/core/lib/auth', () => ({ useAuth: () => ({ user: { id: 'u1' } }) }))

const host = {
    ...widgetHost,
    pkgSlug: 'gizmos',
    pkgName: 'Gizmo Works',
    ui: async () => ({ components: {} }),
}

function Harness({
    field,
    mode = 'create',
    ui = null,
    recordId,
    collab,
    initial,
    error,
}: {
    field: FormField
    mode?: 'create' | 'edit' | 'read' | 'design'
    ui?: FormHostUi | null
    recordId?: string
    collab?: CollabDoc | null
    initial?: unknown
    /** A validation error already on the field, as after a failed submit. */
    error?: string
}) {
    const { control, getValues } = useForm<FormValues>({
        defaultValues: { [field.key]: initial ?? (field.kind === 'number' ? 0 : '') },
        errors: error ? { [field.key]: { type: 'manual', message: error } } : undefined,
    })
    const value = useWatch({ control, name: field.key })
    return (
        <>
            <Text testID="value">{String(value)}</Text>
            <FieldInput
                host={host}
                ui={ui}
                field={field}
                control={control}
                getValues={getValues}
                mode={mode}
                recordId={recordId}
                collab={collab}
            />
        </>
    )
}

function byTestId(container: HTMLElement, id: string) {
    const el = container.querySelector(`[testID="${id}"]`)
    if (!el) throw new Error(`no element with testID ${id}`)
    return el
}

const select = {
    key: 's',
    kind: 'select' as const,
    display: 'dropdown' as const,
    multiple: false,
    options: [{ value: 'x', label: 'Ex' }],
}

const base = { page: 'main', label: 'L', width: 'full' as const, required: false, hidden: false }

describe('FieldInput', () => {
    it('renders a text input for text', () => {
        const { container } = render(<Harness field={{ ...base, key: 'a', kind: 'text' }} />)
        expect(container.querySelector('rn-textinput')).toBeTruthy()
    })

    it('renders radio options for select with display radio', () => {
        const { getByText } = render(
            <Harness
                field={{
                    ...base,
                    key: 's',
                    kind: 'select',
                    display: 'radio',
                    multiple: false,
                    options: [{ value: 'x', label: 'Ex' }],
                }}
            />
        )
        expect(getByText('Ex')).toBeTruthy()
    })

    it('renders a host section in edit mode and nothing in create mode', () => {
        const ui: FormHostUi = {
            components: { CommentsSection: () => <Text>Comments body</Text> },
        }
        const field: FormField = { ...base, key: 'comments', kind: 'builtin' }
        const edit = render(<Harness field={field} ui={ui} mode="edit" recordId="r1" />)
        expect(edit.queryByText('Comments body')).toBeTruthy()
        cleanup()
        const create = render(<Harness field={field} ui={ui} mode="create" />)
        expect(create.queryByText('Comments body')).toBeNull()
    })

    it('shows the empty read view for a collaborative builtin in read mode', () => {
        const { getByText } = render(
            <Harness field={{ ...base, key: 'notes', kind: 'builtin' }} mode="read" />
        )
        expect(getByText('Empty')).toBeTruthy()
    })

    it('is read-only for a collaborative field in edit mode with no room', () => {
        const field: FormField = { ...base, key: 'notes', kind: 'builtin' }
        const editable = render(<Harness field={field} mode="create" />)
        const editableCount = editable.container.querySelectorAll('[accessibilityLabel="L"]').length
        cleanup()
        const noRoom = render(<Harness field={field} mode="edit" recordId="r1" collab={null} />)
        const noRoomCount = noRoom.container.querySelectorAll('[accessibilityLabel="L"]').length
        expect(editableCount).toBeGreaterThan(0)
        expect(noRoomCount).toBe(0)
    })

    it('asks to add files after creating when a file field has no record', () => {
        const { getByText } = render(
            <Harness
                field={{
                    ...base,
                    key: 'f',
                    kind: 'file',
                    multiple: true,
                    accept: [],
                    maxSizeMb: 10,
                }}
                mode="create"
            />
        )
        expect(getByText('Add files after creating.')).toBeTruthy()
    })

    const xyOptions = [
        { value: 'x', label: 'Ex' },
        { value: 'y', label: 'Why' },
    ]

    it.each(['dropdown', 'radio'] as const)('read mode shows only the chosen %s label', display => {
        const field: FormField = { ...base, ...select, display, options: xyOptions }
        const { getByText, queryByText, container } = render(
            <Harness field={field} mode="read" initial="x" />
        )
        expect(getByText('Ex')).toBeTruthy()
        expect(queryByText('Why')).toBeNull()
        expect(byTestId(container, 'form-input-s').tagName).not.toBe('BUTTON')
    })

    it('read mode shows every chosen label of a multiple select', () => {
        const field: FormField = {
            ...base,
            ...select,
            multiple: true,
            options: [...xyOptions, { value: 'z', label: 'Zed' }],
        }
        const { getByText, queryByText } = render(
            <Harness field={field} mode="read" initial={['x', 'z']} />
        )
        expect(getByText('Ex, Zed')).toBeTruthy()
        expect(queryByText('Why')).toBeNull()
    })

    it('read mode shows Empty for no value', () => {
        const { getByText } = render(<Harness field={{ ...base, ...select }} mode="read" />)
        expect(getByText('Empty')).toBeTruthy()
    })

    it.each(['dropdown', 'radio'] as const)('changes a %s value in create mode', display => {
        const field: FormField = { ...base, ...select, display }
        const create = render(<Harness field={field} mode="create" />)
        fireEvent.click(create.getByText('Ex'))
        expect(byTestId(create.container, 'value').textContent).toBe('x')
    })

    it('disables the number input in read mode and gives it the form test id', () => {
        const field: FormField = { ...base, key: 'n', kind: 'number' }
        const editableOf = (container: HTMLElement) =>
            byTestId(container, 'form-input-n').hasAttribute('editable')
        const read = render(<Harness field={field} mode="read" />)
        // The stub drops a false boolean prop and writes a true one as an attribute.
        expect(editableOf(read.container)).toBe(false)
        cleanup()
        const create = render(<Harness field={field} mode="create" />)
        expect(editableOf(create.container)).toBe(true)
    })

    it('accepts a decimal into a number field with a fractional step', () => {
        const field: FormField = { ...base, key: 'n', kind: 'number', step: 0.5 }
        const { container } = render(<Harness field={field} />)
        fireEvent.input(byTestId(container, 'form-input-n'), { target: { value: '1.5' } })
        expect(byTestId(container, 'value').textContent).toBe('1.5')
    })

    it.each([
        ['text', { kind: 'text' as const }, ''],
        ['textarea', { kind: 'textarea' as const }, ''],
        ['number', { kind: 'number' as const }, 0],
        ['date', { kind: 'date' as const, time: false }, null],
        ['checklist', { kind: 'checklist' as const }, []],
        ['richtext', { kind: 'richtext' as const, collaborative: false }, ''],
        ['user', { kind: 'user' as const, multiple: false }, null],
        ['checkbox select', { ...select, display: 'checkbox' as const }, []],
    ])('names the %s input with the field label', (_, kind, initial) => {
        const field = { ...base, key: 'k', label: 'Due by', ...kind } as FormField
        const { container } = render(<Harness field={field} initial={initial} />)
        expect(container.querySelector('[accessibilityLabel="Due by"]')).toBeTruthy()
    })

    it('names the file field with the field label once the record exists', () => {
        const field: FormField = {
            ...base,
            key: 'f',
            label: 'Shots',
            kind: 'file',
            multiple: true,
            accept: [],
            maxSizeMb: 10,
        }
        const client = new QueryClient()
        const { container } = render(
            <QueryClientProvider client={client}>
                <Harness field={field} mode="edit" recordId="r1" initial={[]} />
            </QueryClientProvider>
        )
        expect(container.querySelector('[accessibilityLabel="Shots"]')).toBeTruthy()
    })

    it.each([
        ['toggle', { kind: 'toggle' as const }],
        ['radio select', { ...select, display: 'radio' as const }],
        ['dropdown select', { ...select, display: 'dropdown' as const }],
    ])('shows the label inside the %s input, which renders its own', (_, kind) => {
        const field = { ...base, key: 'k', label: 'Urgent', ...kind } as FormField
        const { getByText } = render(<Harness field={field} />)
        expect(getByText('Urgent')).toBeTruthy()
        expect(rendersOwnLabel(host, field, null)).toBe(true)
    })

    // The cell renders the error under the whole field, for every kind; an input
    // that also rendered it showed the message twice.
    it('leaves the error text to the cell and only colours the input', () => {
        const message = 'L is required'
        const text = render(<Harness field={{ ...base, key: 'a', kind: 'text' }} error={message} />)
        expect(text.queryByText(message)).toBeNull()
        expect(text.container.querySelector('rn-textinput')?.getAttribute('class')).toContain(
            'border-danger'
        )
        text.unmount()
        const number = render(
            <Harness field={{ ...base, key: 'n', kind: 'number' }} error={message} />
        )
        expect(number.queryByText(message)).toBeNull()
        number.unmount()
        const radio = render(
            <Harness
                field={{ ...base, ...select, display: 'radio' as const } as FormField}
                error={message}
            />
        )
        expect(radio.queryByText(message)).toBeNull()
    })

    it('lets the cell show the label for inputs that do not render one', () => {
        const field: FormField = { ...base, key: 'k', kind: 'text' }
        expect(rendersOwnLabel(host, field, null)).toBe(false)
        const checkbox = { ...base, ...select, display: 'checkbox' as const } as FormField
        expect(rendersOwnLabel(host, checkbox, null)).toBe(false)
    })

    // A collaborative built-in with a host component renders THAT component,
    // with the room and the fragment it needs; the core editor is the fallback.
    it('hands a collaborative built-in to its host component with collab and fragment', () => {
        const seen: Record<string, unknown>[] = []
        const ui = {
            components: {
                NotesField: (props: Record<string, unknown>) => {
                    seen.push(props)
                    return <Text testID="host-notes">host</Text>
                },
            },
        }
        const collab = { doc: {} as never, awareness: null, user: null, canEdit: true }
        const { container } = render(
            <Harness
                field={{ ...base, key: 'notes', kind: 'builtin' }}
                mode="edit"
                recordId="r1"
                ui={ui as unknown as FormHostUi}
                collab={collab}
            />
        )
        expect(byTestId(container, 'host-notes')).toBeTruthy()
        expect(container.querySelector('[testID="form-input-notes"]')).toBeNull()
        expect(seen.at(-1)).toMatchObject({
            fieldKey: 'notes',
            mode: 'edit',
            recordId: 'r1',
            disabled: false,
            collab,
            fragment: 'widget:r1:md:notes',
        })
    })

    it('shows a placeholder for a host component in design mode', () => {
        const ui = { components: { NotesField: () => <Text>never</Text> } }
        const { getByText, queryByText } = render(
            <Harness
                field={{ ...base, key: 'notes', kind: 'builtin' }}
                mode="design"
                ui={ui as unknown as FormHostUi}
            />
        )
        expect(getByText('Shown by Gizmo Works')).toBeTruthy()
        expect(queryByText('never')).toBeNull()
    })
})
