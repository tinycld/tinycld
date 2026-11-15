import { describe, expect, it, vi } from 'vitest'
import { linkPress } from '../link-press'

function press(event: Parameters<ReturnType<typeof linkPress>>[0]) {
    const action = vi.fn()
    const preventDefault = vi.fn()
    const stopPropagation = vi.fn()
    linkPress(action)({ preventDefault, stopPropagation, ...event })
    return { action, preventDefault, stopPropagation }
}

describe('linkPress', () => {
    it('runs the action in place of the href on a plain click', () => {
        const { action, preventDefault, stopPropagation } = press({ button: 0 })
        expect(action).toHaveBeenCalledOnce()
        expect(preventDefault).toHaveBeenCalledOnce()
        // A row's own press must not run as well when its label link is pressed.
        expect(stopPropagation).toHaveBeenCalledOnce()
    })

    it('leaves a modified click to the browser', () => {
        for (const key of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey'] as const) {
            const { action, preventDefault } = press({ button: 0, [key]: true })
            expect(action).not.toHaveBeenCalled()
            expect(preventDefault).not.toHaveBeenCalled()
        }
    })

    it('leaves a middle click to the browser', () => {
        const { action, preventDefault } = press({ button: 1 })
        expect(action).not.toHaveBeenCalled()
        expect(preventDefault).not.toHaveBeenCalled()
    })

    it('runs on a native press, which has no keys or button', () => {
        expect(press({}).action).toHaveBeenCalledOnce()
    })
})
