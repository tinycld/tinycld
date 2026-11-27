// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it } from 'vitest'
import { NumberInput, type NumberInputProps } from '../NumberInput'

type Values = { n: number | string | undefined }
let values: () => Values = () => ({ n: undefined })

function Harness(props: Partial<NumberInputProps<Values>> & { initial?: number }) {
    const { initial, ...rest } = props
    const { control, getValues } = useForm<Values>({ defaultValues: { n: initial } })
    values = getValues
    return <NumberInput control={control} name="n" label="N" {...rest} />
}

const inputOf = (c: HTMLElement) => c.querySelector('rn-textinput') as HTMLInputElement
const type = (c: HTMLElement, value: string) => fireEvent.input(inputOf(c), { target: { value } })
const NAMES = { '+': 'Increase N', '−': 'Decrease N' } as const
const press = (c: HTMLElement, glyph: keyof typeof NAMES) => {
    const button = c.querySelector(`[accessibilityLabel="${NAMES[glyph]}"]`)
    fireEvent.click(button as Element)
}

afterEach(cleanup)

describe('NumberInput allowDecimal', () => {
    it('stores 0.5 and -1.25 as numbers', () => {
        const { container } = render(<Harness allowDecimal />)
        type(container, '0.5')
        expect(values().n).toBe(0.5)
        type(container, '-1.25')
        expect(values().n).toBe(-1.25)
    })

    it('does not commit NaN while typing a trailing dot, a sign or a lone dot', () => {
        const { container } = render(<Harness allowDecimal />)
        type(container, '1.')
        expect(values().n).toBe(1)
        expect(inputOf(container).value).toBe('1.')
        type(container, '-')
        expect(values().n).toBe('-')
        type(container, '.')
        expect(values().n).toBe('.')
        expect(inputOf(container).value).toBe('.')
    })

    it('keeps typed zeros so 1.05 can be typed', () => {
        const { container } = render(<Harness allowDecimal />)
        type(container, '1.0')
        expect(inputOf(container).value).toBe('1.0')
        type(container, '1.05')
        expect(values().n).toBe(1.05)
    })

    it('ignores text that is not a number', () => {
        const { container } = render(<Harness allowDecimal initial={2} />)
        type(container, '1.2.3')
        type(container, 'abc')
        expect(values().n).toBe(2)
    })

    it('steps by the increment and keeps decimal precision', () => {
        const { container } = render(<Harness allowDecimal initial={0.1} increment={0.2} />)
        press(container, '+')
        expect(values().n).toBe(0.3)
        press(container, '−')
        expect(values().n).toBe(0.1)
    })
})

const stepperOf = (c: HTMLElement, glyph: keyof typeof NAMES) =>
    c.querySelector(`[accessibilityLabel="${NAMES[glyph]}"]`)

describe('NumberInput hideSteppers', () => {
    for (const isCompact of [false, true]) {
        describe(isCompact ? 'compact' : 'full width', () => {
            it('draws no Decrease and no Increase button', () => {
                const { container } = render(<Harness hideSteppers isCompact={isCompact} />)
                expect(stepperOf(container, '−')).toBeNull()
                expect(stepperOf(container, '+')).toBeNull()
                expect(inputOf(container)).not.toBeNull()
            })

            it('still sets the value by typing and keeps min and max', () => {
                const { container } = render(
                    <Harness hideSteppers isCompact={isCompact} min={0} max={10} initial={3} />
                )
                type(container, '7')
                expect(values().n).toBe(7)
                type(container, '11')
                expect(values().n).toBe(7)
                type(container, '-1')
                expect(values().n).toBe(7)
            })

            it('keeps allowDecimal', () => {
                const { container } = render(
                    <Harness hideSteppers isCompact={isCompact} allowDecimal />
                )
                type(container, '1.25')
                expect(values().n).toBe(1.25)
            })

            it('stays integer without allowDecimal', () => {
                const { container } = render(
                    <Harness hideSteppers isCompact={isCompact} initial={4} />
                )
                type(container, '1.5')
                expect(values().n).toBe(4)
            })
        })
    }

    it('draws both buttons without it, compact or not', () => {
        for (const isCompact of [false, true]) {
            const { container, unmount } = render(<Harness isCompact={isCompact} />)
            expect(stepperOf(container, '−')).not.toBeNull()
            expect(stepperOf(container, '+')).not.toBeNull()
            unmount()
        }
    })
})

describe('NumberInput without allowDecimal', () => {
    it('rejects a decimal point, as before', () => {
        const { container } = render(<Harness initial={3} />)
        type(container, '1.5')
        expect(values().n).toBe(3)
        type(container, '12')
        expect(values().n).toBe(12)
    })

    it('steps by whole increments', () => {
        const { container } = render(<Harness initial={3} />)
        press(container, '+')
        expect(values().n).toBe(4)
    })
})
