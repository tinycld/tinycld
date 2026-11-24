import { holdFocusOut } from '@tinycld/core/ui/overlay/hold-native-focus'
import { describe, expect, it } from 'vitest'

/**
 * Native: content under a cover keeps no focused text input. An input that
 * focused before the cover came up loses the focus, and so does one that takes
 * it later (it mounts lazily and autofocuses), which shows the keyboard.
 */

type Input = { name: string }

function port(focused: Input | null, inside: readonly Input[]) {
    let current = focused
    let onShow: (() => void) | null = null
    const blurred: string[] = []
    return {
        blurred,
        focus: (input: Input) => {
            current = input
        },
        showKeyboard: () => onShow?.(),
        hasListener: () => onShow !== null,
        port: {
            focused: () => current,
            contains: (input: Input) => inside.includes(input),
            blur: (input: Input) => {
                blurred.push(input.name)
                current = null
            },
            onKeyboardShow: (listener: () => void) => {
                onShow = listener
                return () => {
                    onShow = null
                }
            },
        },
    }
}

const inner = { name: 'inner' }
const later = { name: 'later' }
const outer = { name: 'outer' }

describe('holdFocusOut', () => {
    it('blurs an input inside the content that holds the focus', () => {
        const p = port(inner, [inner, later])
        holdFocusOut(p.port)
        expect(p.blurred).toEqual(['inner'])
    })

    it('leaves an input outside the content alone', () => {
        const p = port(outer, [inner])
        holdFocusOut(p.port)
        p.showKeyboard()
        expect(p.blurred).toEqual([])
    })

    it('blurs an input that takes the focus later, when the keyboard shows', () => {
        const p = port(null, [inner, later])
        holdFocusOut(p.port)
        p.focus(later)
        p.showKeyboard()
        expect(p.blurred).toEqual(['later'])
    })

    it('stops listening when released', () => {
        const p = port(null, [inner])
        const release = holdFocusOut(p.port)
        expect(p.hasListener()).toBe(true)
        release()
        expect(p.hasListener()).toBe(false)
        p.focus(inner)
        p.showKeyboard()
        expect(p.blurred).toEqual([])
    })
})
