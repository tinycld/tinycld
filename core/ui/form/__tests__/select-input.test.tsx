// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { OverlayProvider } from '../../overlay'
import type { SelectOption } from '../ButtonGroup'
import { SelectInput } from '../SelectInput'

afterEach(cleanup)

let latest: () => string = () => ''

const plainOptions: SelectOption[] = [
    { value: 'x', label: 'Ex' },
    { value: 'y', label: 'Why' },
]

const sectionedOptions: SelectOption[] = [
    { value: '', label: 'None' },
    { value: 'a', label: 'Apple', section: 'Fruit' },
    { value: 'b', label: 'Banana', section: 'Fruit' },
    { value: 'c', label: 'Carrot', section: 'Roots' },
]

function Harness({
    disabled = false,
    initial = 'x',
    hint,
    placeholder,
    onValueChange,
    options = plainOptions,
}: {
    options?: SelectOption[]
    disabled?: boolean
    initial?: string
    hint?: string
    placeholder?: string
    onValueChange?: (value: string) => void
}) {
    const { control, getValues } = useForm({ defaultValues: { s: initial } })
    latest = () => getValues('s')
    return (
        <OverlayProvider>
            <SelectInput
                control={control}
                name="s"
                label="Pick"
                testID="pick"
                hint={hint}
                placeholder={placeholder}
                disabled={disabled}
                onValueChange={onValueChange}
                options={options}
            />
        </OverlayProvider>
    )
}

const q = (id: string) =>
    document.body.querySelector(`[testID="${id}"], [data-testid="${id}"]`) as HTMLElement | null
/** The text of every leaf element outside the trigger, in document order: the open menu's headings and items. */
const menuTexts = () =>
    Array.from(document.body.querySelectorAll('*'))
        .filter(el => el.children.length === 0 && !q('pick')?.contains(el))
        .map(el => el.textContent?.trim() ?? '')
        .filter(text => text !== '' && text !== 'Pick')
const labelOf = (el: HTMLElement | null) =>
    el?.getAttribute('accessibilityLabel') ?? el?.getAttribute('aria-label')
/** The element a screen reader announces as a group with this name: by its own label, or the heading it is labelled by. */
const groupNamed = (name: string) =>
    Array.from(document.body.querySelectorAll<HTMLElement>('*')).find(el => {
        const role = el.getAttribute('role') ?? el.getAttribute('accessibilityRole')
        if (role !== 'group') return false
        const by = el.getAttribute('aria-labelledby')
        const heading = by
            ? document.body.querySelector(`[id="${by}"], [nativeID="${by}"]`)?.textContent?.trim()
            : undefined
        return (labelOf(el) ?? heading) === name
    })

describe('SelectInput', () => {
    it('renders a trigger, not a radio group', () => {
        render(<Harness />)
        expect(q('pick')).not.toBeNull()
        expect(document.querySelector('[accessibilityrole="radiogroup"]')).toBeNull()
        expect(document.querySelector('[accessibilityrole="radio"]')).toBeNull()
    })

    it('shows the field label, the selected option label and an accessible name combining them', () => {
        render(<Harness />)
        expect(document.body.textContent).toContain('Pick')
        expect(q('pick')?.textContent).toContain('Ex')
        expect(labelOf(q('pick'))).toBe('Pick: Ex')
    })

    it('is 44pt tall', () => {
        render(<Harness />)
        expect((q('pick') as HTMLElement).style.minHeight).toBe('44px')
    })

    it('shows the placeholder when no option matches the value', () => {
        render(<Harness initial="" placeholder="Select one" />)
        expect(q('pick')?.textContent).toContain('Select one')
        expect(labelOf(q('pick'))).toBe('Pick: Select one')
    })

    it('opens a menu with one item per option, each with a testID of testID-value', () => {
        render(<Harness />)
        expect(q('pick-y')).toBeNull()
        fireEvent.click(q('pick') as HTMLElement)
        expect(q('pick-x')?.textContent).toContain('Ex')
        expect(q('pick-y')?.textContent).toContain('Why')
    })

    it('sets the value, updates the trigger and calls onValueChange once on a real change', () => {
        const onValueChange = vi.fn()
        render(<Harness onValueChange={onValueChange} />)
        fireEvent.click(q('pick') as HTMLElement)
        fireEvent.click(q('pick-y') as HTMLElement)
        expect(latest()).toBe('y')
        expect(onValueChange).toHaveBeenCalledTimes(1)
        expect(onValueChange).toHaveBeenCalledWith('y')
        expect(labelOf(q('pick'))).toBe('Pick: Why')
    })

    it('does not call onValueChange when the current option is picked again', () => {
        const onValueChange = vi.fn()
        render(<Harness onValueChange={onValueChange} />)
        fireEvent.click(q('pick') as HTMLElement)
        fireEvent.click(q('pick-x') as HTMLElement)
        expect(onValueChange).not.toHaveBeenCalled()
        expect(latest()).toBe('x')
    })

    it('does not open or change anything when disabled', () => {
        const onValueChange = vi.fn()
        render(<Harness disabled onValueChange={onValueChange} />)
        expect(q('pick')?.getAttribute('aria-disabled')).toBe('true')
        fireEvent.click(q('pick') as HTMLElement)
        expect(q('pick-y')).toBeNull()
        expect(onValueChange).not.toHaveBeenCalled()
        expect(latest()).toBe('x')
    })

    it('renders the hint', () => {
        render(<Harness hint="Cannot change" />)
        expect(document.body.textContent).toContain('Cannot change')
    })

    it('draws each section heading before its items, and none over options without a section', () => {
        render(<Harness options={sectionedOptions} initial="" />)
        fireEvent.click(q('pick') as HTMLElement)
        expect(menuTexts()).toEqual(['None', 'Fruit', 'Apple', 'Banana', 'Roots', 'Carrot'])
    })

    it('sets the value of an option under a heading and shows only its label in the trigger', () => {
        render(<Harness options={sectionedOptions} initial="" />)
        fireEvent.click(q('pick') as HTMLElement)
        fireEvent.click(q('pick-b') as HTMLElement)
        expect(latest()).toBe('b')
        expect(q('pick')?.textContent).toContain('Banana')
        expect(q('pick')?.textContent).not.toContain('Fruit')
        expect(labelOf(q('pick'))).toBe('Pick: Banana')
    })

    it('wraps each section in a group named by its heading, and the arrow keys still cross sections', () => {
        render(<Harness options={sectionedOptions} initial="" />)
        fireEvent.click(q('pick') as HTMLElement)
        const fruit = groupNamed('Fruit')
        expect(fruit?.contains(q('pick-a'))).toBe(true)
        expect(fruit?.contains(q('pick-b'))).toBe(true)
        expect(fruit?.contains(q('pick-c'))).toBe(false)
        expect(fruit?.contains(q('pick-'))).toBe(false)
        expect(groupNamed('Roots')?.contains(q('pick-c'))).toBe(true)

        const banana = q('pick-b') as HTMLElement
        banana.focus()
        fireEvent.keyDown(banana, { key: 'ArrowDown' })
        expect(document.activeElement).toBe(q('pick-c'))
    })

    it('draws no heading when no option has a section', () => {
        render(<Harness />)
        fireEvent.click(q('pick') as HTMLElement)
        expect(menuTexts()).toEqual(['Ex', 'Why'])
    })
})
