// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TextInput, type TextInputProps } from '../TextInput'

// TextInput once forwarded only a fixed allowlist of React Native props, so a
// caller's testID and onSubmitEditing type-checked and then did nothing.
type Forwarded = Pick<TextInputProps, 'testID' | 'onSubmitEditing' | 'accessibilityLabel'>

function Harness(props: Forwarded) {
    const { control } = useForm({ defaultValues: { field: '' } })
    return <TextInput control={control} name="field" label="Field" {...props} />
}

function inputOf(container: HTMLElement) {
    const input = container.querySelector('rn-textinput')
    if (!input) throw new Error('no rn-textinput rendered')
    return input
}

afterEach(cleanup)

describe('TextInput forwards React Native props', () => {
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
})
