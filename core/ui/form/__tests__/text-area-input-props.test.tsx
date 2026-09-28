// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TextAreaInput, type TextAreaInputProps } from '../TextAreaInput'

// TextAreaInput dropped every React Native prop except a fixed allowlist, so
// a caller's testID and onSubmitEditing type-checked and then did nothing.
type Forwarded = Pick<
    TextAreaInputProps,
    'testID' | 'onSubmitEditing' | 'accessibilityLabel' | 'maxLength'
>

function Harness(props: Forwarded) {
    const { control } = useForm({ defaultValues: { field: '' } })
    return <TextAreaInput control={control} name="field" label="Field" {...props} />
}

function inputOf(container: HTMLElement) {
    const input = container.querySelector('rn-textinput')
    if (!input) throw new Error('no rn-textinput rendered')
    return input
}

afterEach(cleanup)

describe('TextAreaInput forwards React Native props', () => {
    it('uses the field name as the test id by default', () => {
        const { container } = render(<Harness />)
        expect(inputOf(container).getAttribute('testID')).toBe('field')
    })

    it("uses the caller's testID instead of the field name", () => {
        const { container } = render(<Harness testID="custom-input" />)
        expect(inputOf(container).getAttribute('testID')).toBe('custom-input')
    })

    it("uses the caller's accessibilityLabel instead of the label", () => {
        const { container } = render(<Harness accessibilityLabel="Custom" />)
        expect(inputOf(container).getAttribute('accessibilityLabel')).toBe('Custom')
    })

    it('calls onSubmitEditing when the input submits', () => {
        const onSubmitEditing = vi.fn()
        const { container } = render(<Harness onSubmitEditing={onSubmitEditing} />)
        inputOf(container).dispatchEvent(new Event('SubmitEditing'))
        expect(onSubmitEditing).toHaveBeenCalledTimes(1)
    })

    it('forwards maxLength to the native input', () => {
        const { container } = render(<Harness maxLength={140} />)
        expect(inputOf(container).getAttribute('maxLength')).toBe('140')
    })
})

// The component owns the form binding and the field's look, so className and
// defaultValue must not type-check — a caller passing either is ignored
// silently rather than told at compile time.
function TypeCheck() {
    const { control } = useForm({ defaultValues: { field: '' } })
    return (
        <TextAreaInput
            control={control}
            name="field"
            // @ts-expect-error className is owned by the component
            className="not-allowed"
        />
    )
}
void TypeCheck

function TypeCheckDefaultValue() {
    const { control } = useForm({ defaultValues: { field: '' } })
    return (
        <TextAreaInput
            control={control}
            name="field"
            // @ts-expect-error defaultValue is owned by the component
            defaultValue="not-allowed"
        />
    )
}
void TypeCheckDefaultValue
