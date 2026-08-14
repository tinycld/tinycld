import { describe, expect, it } from 'vitest'
import { reduceInit } from '../Editor'
import type { RichEditorInitPayload } from '../protocol'

function payload(generation: number, content = ''): RichEditorInitPayload {
    return {
        generation,
        contentFormat: 'markdown',
        initialContent: content,
        placeholder: '',
        editable: true,
        autofocus: false,
        colors: { bg: '#000', fg: '#fff', placeholder: '#888', primary: '#0f0' },
    }
}

describe('init reducer', () => {
    it('accepts the first init', () => {
        expect(reduceInit(null, payload(0))?.generation).toBe(0)
    })

    it('accepts a bumped generation, so a handover reconfigures the page', () => {
        expect(reduceInit(payload(0), payload(1, 'next surface'))?.initialContent).toBe(
            'next surface'
        )
    })

    /**
     * The host posts init from an effect, and a re-delivery of the SAME
     * configuration must not rebuild the editor — that would discard whatever
     * the user has typed since it was applied.
     */
    it('ignores a repeated generation, so typing is not discarded', () => {
        const current = payload(2, 'typed by the user')
        expect(reduceInit(current, payload(2, 'the original seed'))).toBe(current)
    })

    /** Out-of-order delivery must not roll the page back to an older surface. */
    it('ignores an older generation', () => {
        const current = payload(5)
        expect(reduceInit(current, payload(4))).toBe(current)
    })

    it('parks on a null, dropping to stage one', () => {
        expect(reduceInit(payload(3), null)).toBeNull()
    })
})
