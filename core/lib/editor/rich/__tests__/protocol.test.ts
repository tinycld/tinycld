import { describe, expect, it } from 'vitest'
import { decodeUpdate, encodeUpdate } from '../webview/source/protocol'

/**
 * The base64 helpers carry Yjs updates once native collaboration lands: the
 * channel is a JSON string pipe and updates are binary. They are tested now
 * because a silent corruption there would surface as document divergence,
 * which is nearly impossible to debug after the fact.
 */
describe('yjs update encoding', () => {
    it('round-trips an empty update', () => {
        expect(Array.from(decodeUpdate(encodeUpdate(new Uint8Array([]))))).toEqual([])
    })

    it('round-trips every byte value', () => {
        const bytes = new Uint8Array(256)
        for (let i = 0; i < 256; i++) bytes[i] = i
        expect(Array.from(decodeUpdate(encodeUpdate(bytes)))).toEqual(Array.from(bytes))
    })

    it('round-trips each remainder length (padding boundaries)', () => {
        // Lengths 1 and 2 mod 3 are where base64 padding is emitted; an
        // off-by-one there truncates the final byte.
        for (const length of [1, 2, 3, 4, 5, 6, 7]) {
            const bytes = new Uint8Array(length)
            for (let i = 0; i < length; i++) bytes[i] = (i * 37 + 11) & 0xff
            expect(Array.from(decodeUpdate(encodeUpdate(bytes)))).toEqual(Array.from(bytes))
        }
    })

    it('produces standard base64', () => {
        // Cross-checks against a known encoding so the alphabet and bit order
        // match anything else that might read this wire format.
        const bytes = new Uint8Array([0x4d, 0x61, 0x6e])
        expect(encodeUpdate(bytes)).toBe('TWFu')
        expect(encodeUpdate(new Uint8Array([0x4d, 0x61]))).toBe('TWE=')
        expect(encodeUpdate(new Uint8Array([0x4d]))).toBe('TQ==')
    })

    it('round-trips a realistic binary payload', () => {
        const bytes = new Uint8Array(1024)
        for (let i = 0; i < bytes.length; i++) bytes[i] = (i * 131 + 17) & 0xff
        const decoded = decodeUpdate(encodeUpdate(bytes))
        expect(decoded.length).toBe(bytes.length)
        expect(Array.from(decoded)).toEqual(Array.from(bytes))
    })
})
