// @vitest-environment happy-dom
import { cleanup, fireEvent, render } from '@testing-library/react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NumberInput, type NumberInputFieldModeProps } from '../NumberInput'

type Values = { n: number | string | undefined }
let values: () => Values = () => ({ n: undefined })

function Harness(props: Partial<NumberInputFieldModeProps<Values>> & { initial?: number }) {
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

describe('NumberInput bound to a field', () => {
    it('stores a value typed with U+2212 as a negative number', () => {
        const { container } = render(<Harness allowDecimal />)
        type(container, '−2.5')
        expect(values().n).toBe(-2.5)
    })
})

describe('NumberInput text mode', () => {
    function TextHarness(props: { initial?: string; allowDecimal?: boolean }) {
        const [text, setText] = useState(props.initial ?? '')
        textNow = text
        return (
            <NumberInput
                value={text}
                onChangeText={setText}
                label="Q"
                allowDecimal={props.allowDecimal}
                hideSteppers
            />
        )
    }
    let textNow = ''

    it('hands the typed text to onChangeText, with no form', () => {
        const { container } = render(<TextHarness allowDecimal />)
        type(container, '7.555')
        expect(textNow).toBe('7.555')
        expect(inputOf(container).value).toBe('7.555')
    })

    it('normalizes U+2212 to a minus in the text it hands on', () => {
        const { container } = render(<TextHarness allowDecimal />)
        type(container, '−3')
        expect(textNow).toBe('-3')
    })

    it('ignores text that is not a number', () => {
        const { container } = render(<TextHarness allowDecimal initial="2" />)
        type(container, 'abc')
        expect(textNow).toBe('2')
    })
})

describe('NumberInput focus and submit', () => {
    it('passes the focus event to onFocus', () => {
        const onFocus = vi.fn()
        const { container } = render(<Harness onFocus={onFocus} />)
        fireEvent.focus(inputOf(container))
        expect(onFocus).toHaveBeenCalledWith(expect.objectContaining({ type: 'focus' }))
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
