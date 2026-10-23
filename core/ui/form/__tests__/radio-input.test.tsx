// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it } from 'vitest'
import { RadioInput } from '../RadioInput'

afterEach(cleanup)

function Harness() {
    const { control } = useForm({ defaultValues: { s: 'x' } })
    return (
        <RadioInput
            control={control}
            name="s"
            label="Pick"
            options={[
                { value: 'x', label: 'Ex' },
                { value: 'y', label: 'Why' },
            ]}
        />
    )
}

describe('RadioInput', () => {
    it('names its group with the label, so a reader and a locator can find it', () => {
        const { container } = render(<Harness />)
        const group = container.querySelector('[accessibilityrole="radiogroup"]')
        expect(group?.getAttribute('accessibilitylabel')).toBe('Pick')
    })
})
