// @vitest-environment happy-dom
import { act, cleanup, fireEvent, render as renderBare } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { widgetHost } from '../../../lib/forms/__tests__/fixtures'
import { type DraftDefinition, newCustomField } from '../../../lib/forms/builder'
import { defaultDefinitionFor } from '../../../lib/forms/host'
import { OverlayProvider } from '../../../ui/overlay'
import { RulesTab, selectionAfterRemoval } from '../RulesTab'
import type { DraftRule } from '../rule-parts'
import { useEditorForm } from '../use-editor-form'

// The rule menus render through the overlay host.
const render = (ui: ReactElement) => renderBare(<OverlayProvider>{ui}</OverlayProvider>)

afterEach(() => {
    cleanup()
    screen.breakpoint = 'desktop'
})
const base = defaultDefinitionFor(widgetHost)
const def = {
    ...base,
    fields: [
        ...base.fields,
        newCustomField('select', 'Severity', 'main', new Set()),
        newCustomField('text', 'Steps', 'main', new Set()),
    ],
}
const plain = {
    id: 'f1',
    host: 'gizmos:widget',
    name: 'Widget',
    slug: 'widget',
    is_default: true,
    archived: false,
    order: 0,
    definition: def,
}
const host = {
    ...widgetHost,
    pkgSlug: 'gizmos',
    pkgName: 'Gizmos',
    ui: async () => ({ components: {} }),
}
const admin = {
    host,
    forms: [plain],
    defaultForm: plain,
    isReady: true,
    saveDefinition: { mutateAsync: vi.fn(), isPending: false },
}
vi.mock('../../../lib/forms/use-form-admin', async importOriginal => ({
    ...(await importOriginal<object>()),
    useFormAdmin: () => admin,
}))
vi.mock('../../../lib/forms/use-forms-of-host', () => ({
    useFormRow: () => ({ row: plain, isReady: true }),
    useFormsOfHost: () => ({ forms: [plain], defaultForm: plain, isReady: true }),
}))
vi.mock('../../../lib/forms/use-form-hosts', () => ({ useFormHost: () => host }))
const screen = vi.hoisted(() => ({ breakpoint: 'desktop' }))
vi.mock('@tinycld/core/components/workspace/useBreakpoint', () => ({
    useBreakpoint: () => screen.breakpoint,
}))
// RN stub hosts carry `testID`; Menu rows are DOM elements with `data-testid`
// under the overlay host, outside the render container.
const q = (_c: ParentNode, id: string) =>
    document.body.querySelector(`[testID="${id}"], [data-testid="${id}"]`) as HTMLElement | null

let editorRef: ReturnType<typeof useEditorForm> | null = null
function Harness() {
    const editor = useEditorForm(plain, host)
    editorRef = editor
    return <RulesTab editor={editor} isVisible />
}

describe('RulesTab', () => {
    it('shows the empty hint and adds an empty rule, with no condition or action row yet', () => {
        const { container, getByText } = render(<Harness />)
        expect(getByText(/no rules yet/i)).toBeTruthy()
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        expect(editorRef?.form.getValues('rules')).toEqual([
            { key: 'rule', match: 'all', when: [], actions: [] },
        ])
        expect(q(container, 'form-rule-0')).toBeTruthy()
        expect(q(container, 'form-cond-0-0')).toBeNull()
        expect(q(container, 'form-action-0-0')).toBeNull()
    })
    it('authors "when severity is high then require steps"', () => {
        const { container } = render(<Harness />)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-add-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-0-0-severity') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-op-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-op-0-0-is') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-value-0-0-option_1') as HTMLElement)
        fireEvent.click(q(container, 'form-action-add-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0-steps') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0-require') as HTMLElement)
        expect(editorRef?.form.getValues('rules.0')).toEqual({
            key: 'rule',
            match: 'all',
            when: [{ field: 'severity', op: 'is', value: 'option_1' }],
            actions: [{ action: 'require', field: 'steps' }],
        })
    })
    it('offers set_options only for a select target', () => {
        const { container } = render(<Harness />)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-action-add-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0-steps') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0') as HTMLElement)
        expect(q(container, 'form-action-kind-0-0-set_options')).toBeNull()
        // A real click's pointerdown outside the open menu dismisses it; fireEvent.click alone does not.
        fireEvent.pointerDown(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0-severity') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0') as HTMLElement)
        expect(q(container, 'form-action-kind-0-0-set_options')).toBeTruthy()
    })
    it('removes a rule', () => {
        const { container } = render(<Harness />)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-rule-remove-0') as HTMLElement)
        expect(editorRef?.form.getValues('rules')).toHaveLength(0)
    })
    it('keeps a later rule intact when an earlier one is removed', () => {
        const { container } = render(<Harness />)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-add-1') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-1-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-1-0-severity') as HTMLElement)
        fireEvent.click(q(container, 'form-rule-remove-0') as HTMLElement)
        expect(editorRef?.form.getValues('rules')).toHaveLength(1)
        expect(q(container, 'form-cond-field-0-0')?.textContent).toContain('Severity')
        expect(q(container, 'form-cond-field-1-0')).toBeNull()
    })
    it('resets the operator and value when the condition field changes', () => {
        const { container } = render(<Harness />)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-add-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-0-0-severity') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-value-0-0-option_1') as HTMLElement)
        fireEvent.pointerDown(q(container, 'form-cond-field-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-cond-field-0-0-steps') as HTMLElement)
        expect(editorRef?.form.getValues('rules.0.when.0')).toEqual({
            field: 'steps',
            op: 'eq',
            value: undefined,
        })
    })
    it('marks the fields the selected rule reads and changes', () => {
        const { container } = render(<Harness />)
        act(() =>
            editorRef?.apply(d => ({
                ...d,
                rules: [
                    {
                        key: 'r1',
                        match: 'all',
                        when: [{ field: 'severity', op: 'is', value: 'option_1' }],
                        actions: [{ action: 'require', field: 'steps' }],
                    },
                ],
            }))
        )
        const pane = q(container, 'form-rule-affected')
        expect(pane?.textContent).toContain('when')
        expect(pane?.textContent).toContain('Require')
    })
    it('seeds set_options from the target and lets set_value pick an option', () => {
        const { container } = render(<Harness />)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-action-add-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0-severity') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0-set_options') as HTMLElement)
        expect(editorRef?.form.getValues('rules.0.actions.0')).toEqual({
            action: 'set_options',
            field: 'severity',
            options: [{ value: 'option_1', label: 'Option 1' }],
        })
        fireEvent.click(q(container, 'form-action-option-add-0-0') as HTMLElement)
        expect(editorRef?.form.getValues('rules.0.actions.0')).toMatchObject({
            options: [
                { value: 'option_1', label: 'Option 1' },
                { value: 'option_2', label: 'Option 2' },
            ],
        })
        fireEvent.pointerDown(q(container, 'form-action-kind-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0-set_value') as HTMLElement)
        fireEvent.click(q(container, 'form-action-value-0-0-option_1') as HTMLElement)
        expect(editorRef?.form.getValues('rules.0.actions.0')).toEqual({
            action: 'set_value',
            field: 'severity',
            value: 'option_1',
        })
    })
    it('falls back to show when a page becomes the target of a require', () => {
        const { container } = render(<Harness />)
        fireEvent.click(q(container, 'form-rule-add') as HTMLElement)
        fireEvent.click(q(container, 'form-action-add-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-kind-0-0-require') as HTMLElement)
        fireEvent.pointerDown(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0-page:main') as HTMLElement)
        expect(editorRef?.form.getValues('rules.0.actions.0')).toEqual({
            action: 'show',
            page: 'main',
        })
    })
    it('reseeds set_options rows when the target changes', () => {
        const { container } = render(<Harness />)
        act(() =>
            editorRef?.apply(d => ({
                ...d,
                fields: [...d.fields, newCustomField('select', 'Priority', 'main', new Set())],
                rules: [
                    {
                        key: 'r1',
                        match: 'all',
                        when: [],
                        actions: [
                            {
                                action: 'set_options',
                                field: 'severity',
                                options: [
                                    { value: 'a', label: 'A' },
                                    { value: 'b', label: 'B' },
                                ],
                            },
                        ],
                    },
                ],
            }))
        )
        expect(q(container, 'form-action-option-remove-0-0-1')).toBeTruthy()
        fireEvent.click(q(container, 'form-action-target-0-0') as HTMLElement)
        fireEvent.click(q(container, 'form-action-target-0-0-priority') as HTMLElement)
        expect(editorRef?.form.getValues('rules.0.actions.0')).toEqual({
            action: 'set_options',
            field: 'priority',
            options: [{ value: 'option_1', label: 'Option 1' }],
        })
        expect(q(container, 'form-action-option-remove-0-0-1')).toBeNull()
    })
})

type DraftRules = NonNullable<DraftDefinition['rules']>

const multiSelect = {
    ...newCustomField('select', 'Tags', 'main', new Set()),
    multiple: true,
    options: [
        { value: 'a', label: 'A' },
        { value: 'b', label: 'B' },
    ],
}
const multiUser = { ...newCustomField('user', 'Watchers', 'main', new Set()), multiple: true }

function seed(rules: DraftRules, extraFields: DraftDefinition['fields'] = []) {
    act(() => editorRef?.apply(d => ({ ...d, fields: [...d.fields, ...extraFields], rules })))
}
const press = (id: string) => {
    const el = q(document.body, id) as HTMLElement
    fireEvent.pointerDown(el)
    fireEvent.click(el)
}
const emptyRule = (key: string): DraftRule => ({ key, match: 'all', when: [], actions: [] })

describe('RulesTab offers only what can be authored', () => {
    it('leaves is_any_of out of a user field operator menu', () => {
        render(<Harness />)
        seed([{ ...emptyRule('r1'), when: [{ field: 'owner', op: 'is' }] }])
        press('form-cond-op-0-0')
        expect(q(document.body, 'form-cond-op-0-0-is')).toBeTruthy()
        expect(q(document.body, 'form-cond-op-0-0-is_any_of')).toBeNull()
    })
    it('writes a list for set_value on a multiple select', () => {
        render(<Harness />)
        seed(
            [
                {
                    ...emptyRule('r1'),
                    actions: [{ action: 'set_value', field: 'tags', value: null }],
                },
            ],
            [multiSelect]
        )
        press('form-action-value-0-0-a')
        press('form-action-value-0-0-b')
        expect(editorRef?.form.getValues('rules.0.actions.0')).toEqual({
            action: 'set_value',
            field: 'tags',
            value: ['a', 'b'],
        })
    })
    it('does not offer set_value for a multiple user target', () => {
        render(<Harness />)
        seed(
            [{ ...emptyRule('r1'), actions: [{ action: 'show', field: 'watchers' }] }],
            [multiUser]
        )
        press('form-action-kind-0-0')
        expect(q(document.body, 'form-action-kind-0-0-require')).toBeTruthy()
        expect(q(document.body, 'form-action-kind-0-0-set_value')).toBeNull()
    })
})

describe('RulesTab keeps a rule the server would refuse from being authored', () => {
    it('offers no hide for a required built-in or the page that holds it', () => {
        render(<Harness />)
        seed([{ ...emptyRule('r1'), actions: [{ action: 'show', field: 'name' }] }])
        press('form-action-kind-0-0')
        expect(q(document.body, 'form-action-kind-0-0-show')).toBeTruthy()
        expect(q(document.body, 'form-action-kind-0-0-hide')).toBeNull()
        seed([{ ...emptyRule('r1'), actions: [{ action: 'show', page: 'main' }] }])
        press('form-action-kind-0-0')
        expect(q(document.body, 'form-action-kind-0-0-hide')).toBeNull()
        seed([{ ...emptyRule('r1'), actions: [{ action: 'show', field: 'severity' }] }])
        press('form-action-kind-0-0')
        expect(q(document.body, 'form-action-kind-0-0-hide')).toBeTruthy()
    })
    it('offers no hide for a locked built-in or the page that holds it', () => {
        const builtins = host.builtin
        host.builtin = builtins.map(b => (b.key === 'due' ? { ...b, locked: true } : b))
        try {
            render(<Harness />)
            // `due` alone on its own page, so only the lock can keep that page shown.
            const onExtra = (d: DraftDefinition): DraftDefinition => ({
                ...d,
                pages: [...d.pages, { key: 'extra', title: 'Extra' }],
                fields: d.fields.map(f => (f.key === 'due' ? { ...f, page: 'extra' } : f)),
            })
            act(() => editorRef?.apply(onExtra))
            seed([{ ...emptyRule('r1'), actions: [{ action: 'show', field: 'due' }] }])
            press('form-action-kind-0-0')
            expect(q(document.body, 'form-action-kind-0-0-show')).toBeTruthy()
            expect(q(document.body, 'form-action-kind-0-0-hide')).toBeNull()
            seed([{ ...emptyRule('r1'), actions: [{ action: 'show', page: 'extra' }] }])
            press('form-action-kind-0-0')
            expect(q(document.body, 'form-action-kind-0-0-hide')).toBeNull()
            // An optional built-in that is not locked can still be hidden.
            seed([{ ...emptyRule('r1'), actions: [{ action: 'show', field: 'owner' }] }])
            press('form-action-kind-0-0')
            expect(q(document.body, 'form-action-kind-0-0-hide')).toBeTruthy()
        } finally {
            host.builtin = builtins
        }
    })
    it('drops hide to show when the target becomes a required built-in', () => {
        render(<Harness />)
        seed([{ ...emptyRule('r1'), actions: [{ action: 'hide', field: 'severity' }] }])
        press('form-action-target-0-0')
        press('form-action-target-0-0-name')
        expect(editorRef?.form.getValues('rules.0.actions.0')).toEqual({
            action: 'show',
            field: 'name',
        })
    })
    it("shows a rule's first error inside its card", async () => {
        render(<Harness />)
        seed([emptyRule('r1')])
        expect(q(document.body, 'form-rule-error-0')).toBeNull()
        await act(async () => {
            await editorRef?.save()
        })
        expect(q(document.body, 'form-rule-error-0')?.textContent).toBe(
            'Add at least one condition'
        )
    })
    it('names the condition value input for a screen reader', () => {
        render(<Harness />)
        seed([{ ...emptyRule('r1'), when: [{ field: 'steps', op: 'eq' }] }])
        const input = document.body.querySelector(
            'rn-textinput[accessibilityLabel="Condition value"]'
        )
        expect(input).toBeTruthy()
    })
    it('says how to write a date a before condition compares with', async () => {
        render(<Harness />)
        seed([
            {
                ...emptyRule('r1'),
                when: [{ field: 'due', op: 'before' }],
                actions: [{ action: 'show', field: 'steps' }],
            },
        ])
        const input = document.body.querySelector(
            'rn-textinput[accessibilityLabel="Condition value"]'
        ) as HTMLElement
        await act(async () => {
            fireEvent.input(input, { target: { value: 'soon' } })
        })
        expect(q(document.body, 'form-cond-error-0-0')?.textContent).toBe('Use YYYY-MM-DD')
        await act(async () => {
            fireEvent.input(input, { target: { value: '2026-01-31' } })
        })
        expect(q(document.body, 'form-cond-error-0-0')).toBeNull()
    })
    it('says how to write a date that set_value writes, once the input loses focus', async () => {
        render(<Harness />)
        seed([
            {
                ...emptyRule('r1'),
                when: [{ field: 'steps', op: 'is_empty' }],
                actions: [{ action: 'set_value', field: 'due', value: null }],
            },
        ])
        const input = q(document.body, 'form-action-value-0-0-date') as HTMLElement
        fireEvent.input(input, { target: { value: 'later' } })
        expect(q(document.body, 'form-action-error-0-0')).toBeNull()
        await act(async () => {
            fireEvent.blur(input)
        })
        expect(q(document.body, 'form-action-error-0-0')?.textContent).toBe('Use YYYY-MM-DD')
    })
})

describe('RulesTab keeps authored work', () => {
    it('keeps set_options rows when the current target is picked again', () => {
        render(<Harness />)
        const options = [
            { value: 'a', label: 'A' },
            { value: 'b', label: 'B' },
        ]
        seed([
            {
                ...emptyRule('r1'),
                actions: [{ action: 'set_options', field: 'severity', options }],
            },
        ])
        press('form-action-target-0-0')
        press('form-action-target-0-0-severity')
        expect(editorRef?.form.getValues('rules.0.actions.0')).toEqual({
            action: 'set_options',
            field: 'severity',
            options,
        })
        expect(q(document.body, 'form-action-option-remove-0-0-1')).toBeTruthy()
    })
    it('keeps the operator and value when the current field is picked again', () => {
        render(<Harness />)
        seed([
            { ...emptyRule('r1'), when: [{ field: 'severity', op: 'is_not', value: 'option_1' }] },
        ])
        press('form-cond-field-0-0')
        press('form-cond-field-0-0-severity')
        expect(editorRef?.form.getValues('rules.0.when.0')).toEqual({
            field: 'severity',
            op: 'is_not',
            value: 'option_1',
        })
    })
})

describe('RulesTab selection', () => {
    const isSelected = (i: number) =>
        (q(document.body, `form-rule-${i}`)?.getAttribute('class') ?? '').includes('border-primary')

    it('is a plain view, not a pressable, where the affected pane does not show', () => {
        screen.breakpoint = 'mobile'
        render(<Harness />)
        seed([emptyRule('r1')])
        expect(q(document.body, 'form-rule-0')?.tagName.toLowerCase()).toBe('rn-view')
        expect(q(document.body, 'form-rule-affected')).toBeNull()
    })
    it('selects a rule when a control inside it is pressed', () => {
        render(<Harness />)
        seed([emptyRule('r1'), emptyRule('r2')])
        expect(q(document.body, 'form-rule-0')?.tagName.toLowerCase()).toBe('rn-pressable')
        expect(isSelected(0)).toBe(true)
        fireEvent.pointerDown(q(document.body, 'form-cond-add-1') as HTMLElement)
        expect(isSelected(1)).toBe(true)
        expect(isSelected(0)).toBe(false)
    })
    it('clears the selection and the pane when the selected rule is removed', () => {
        render(<Harness />)
        seed([emptyRule('r1'), emptyRule('r2'), emptyRule('r3')])
        press('form-cond-add-2')
        expect(isSelected(2)).toBe(true)
        press('form-rule-remove-2')
        expect(editorRef?.form.getValues('rules')).toHaveLength(2)
        expect(isSelected(0) || isSelected(1)).toBe(false)
        expect(q(document.body, 'form-rule-affected')).toBeNull()
    })
    // Pressing a rule's remove button selects that rule first, so the shift is reached through focus or a keyboard; checked directly.
    it('moves the selection back when an earlier rule is removed', () => {
        expect(selectionAfterRemoval(2, 0)).toBe(1)
        expect(selectionAfterRemoval(1, 1)).toBeNull()
        expect(selectionAfterRemoval(0, 2)).toBe(0)
        expect(selectionAfterRemoval(null, 0)).toBeNull()
    })
})
