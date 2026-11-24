// @vitest-environment happy-dom

import { setInert } from '@tinycld/core/ui/overlay/set-inert'
import { afterEach, describe, expect, it } from 'vitest'

/**
 * A cover over content that stays mounted (the package terms cover) holds that
 * content out of play. `inert` stops new focus, but an input that took the
 * focus before the cover came up would keep it and take typed keys.
 */

function mount(): { content: HTMLElement; input: HTMLInputElement; outside: HTMLButtonElement } {
    const content = document.createElement('div')
    const input = document.createElement('input')
    const outside = document.createElement('button')
    content.appendChild(input)
    document.body.append(content, outside)
    return { content, input, outside }
}

afterEach(() => {
    document.body.innerHTML = ''
})

describe('setInert', () => {
    it('marks the node inert and takes the focus out of it', () => {
        const { content, input } = mount()
        input.focus()
        expect(document.activeElement).toBe(input)

        setInert(content, true)

        expect(content.hasAttribute('inert')).toBe(true)
        expect(document.activeElement).not.toBe(input)
    })

    it('leaves the focus alone when it is outside the node', () => {
        const { content, outside } = mount()
        outside.focus()

        setInert(content, true)

        expect(document.activeElement).toBe(outside)
    })

    it('clears the mark', () => {
        const { content } = mount()
        setInert(content, true)

        setInert(content, false)

        expect(content.hasAttribute('inert')).toBe(false)
    })
})
