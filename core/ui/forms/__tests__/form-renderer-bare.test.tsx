// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { Text } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'
import { widgetHost } from '../../../lib/forms/__tests__/fixtures'
import { defaultDefinitionFor, hostDefSchema } from '../../../lib/forms/host'
import type { BuiltinFieldProps, FormHostUi } from '../../../lib/forms/host-ui'
import type { FormLayoutKind } from '../FormRenderer'
import { FormRenderer } from '../FormRenderer'
import { FIELD_CELL_BORDER_WIDTH } from '../fields/table-layout'

afterEach(cleanup)

// The widget's notes and comments draw their own chrome: `chrome: 'bare'`.
const bareHost = {
    ...hostDefSchema.parse({
        ...widgetHost,
        builtin: widgetHost.builtin.map(b =>
            b.key === 'notes' || b.key === 'comments' ? { ...b, chrome: 'bare' } : b
        ),
    }),
    pkgSlug: 'gizmos',
    pkgName: 'Gizmos',
    ui: async () => ui,
}

function NotesField({ fieldKey }: BuiltinFieldProps) {
    return <Text testID={`own-${fieldKey}`}>Notes, drawn by the host</Text>
}

const ui: FormHostUi = {
    components: { NotesField, CommentsSection: () => null },
}

const definition = defaultDefinitionFor(bareHost)
const ctx = { today: '2026-01-01', userId: 'u' }

function Harness({
    layout,
    readOnly,
    hostUi = ui,
}: {
    layout: FormLayoutKind
    readOnly?: boolean
    hostUi?: FormHostUi
}) {
    const form = useForm<Record<string, unknown>>({ defaultValues: {} })
    return (
        <FormRenderer
            host={bareHost}
            ui={hostUi}
            definition={definition}
            layout={layout}
            form={form}
            context={ctx}
            recordId="r1"
            readOnly={readOnly}
        />
    )
}

const byId = (container: HTMLElement, id: string) => container.querySelector(`[testID="${id}"]`)
const classOf = (el: Element | null) => el?.getAttribute('class') ?? ''

describe('a bare built-in', () => {
    it.each([
        ['edit', false],
        ['read', true],
    ])('renders only the width wrapper in %s mode', (_mode, readOnly) => {
        const { container, queryByText } = render(<Harness layout="sections" readOnly={readOnly} />)
        const cell = byId(container, 'form-field-notes')
        expect(byId(container, 'own-notes')).toBeTruthy()
        expect(queryByText('Notes')).toBeNull()
        expect(byId(container, 'form-field-notes-box')).toBeNull()
        expect(classOf(cell)).toContain('px-1.5')
        expect(classOf(cell)).not.toContain('mb-3')
        expect(classOf(cell)).not.toContain('border')
        // The neighbouring built-in keeps its labelled cell.
        expect(queryByText('Name')).toBeTruthy()
    })

    it('leaves an empty wrapper when the host component draws nothing', () => {
        const { container, queryByText } = render(<Harness layout="sections" />)
        expect(byId(container, 'form-field-comments')?.textContent).toBe('')
        expect(queryByText('Comments')).toBeNull()
    })

    it('keeps the labelled cell when its component is missing', () => {
        const noNotes: FormHostUi = { components: { CommentsSection: () => null } }
        const { container, queryByText } = render(<Harness layout="sections" hostUi={noNotes} />)
        expect(byId(container, 'form-field-notes-box')).toBeTruthy()
        expect(queryByText('Notes')).toBeTruthy()
    })

    it('keeps the labelled cell on the design canvas', () => {
        const { container, getByText } = render(<Harness layout="design" />)
        expect(getByText('Notes')).toBeTruthy()
        expect(classOf(byId(container, 'form-field-notes'))).toContain('mb-3')
        const box = byId(container, 'form-field-notes-box') as HTMLElement | null
        expect(box?.style.borderWidth).toBe(`${FIELD_CELL_BORDER_WIDTH}px`)
    })
})
