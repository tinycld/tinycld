import { uuidFromRandomValues } from '@tinycld/core/lib/uuid'
import { describe, expect, it, vi } from 'vitest'

const V4_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

describe('uuidFromRandomValues', () => {
    it('produces a well-formed RFC 4122 v4 UUID', () => {
        expect(uuidFromRandomValues()).toMatch(V4_UUID)
    })

    it('does not delegate to crypto.randomUUID (no recursion on web)', () => {
        // The bug this guards: installing expo-crypto's web randomUUID as
        // crypto.randomUUID makes it call itself forever on origins without a
        // native one. Our implementation must rely solely on getRandomValues,
        // so it stays safe even when randomUUID is the (broken) delegating shim.
        const spy = vi.spyOn(crypto, 'randomUUID').mockImplementation(() => {
            throw new Error('randomUUID must not be called')
        })
        try {
            expect(uuidFromRandomValues()).toMatch(V4_UUID)
            expect(spy).not.toHaveBeenCalled()
        } finally {
            spy.mockRestore()
        }
    })

    it('returns distinct values across calls', () => {
        const a = uuidFromRandomValues()
        const b = uuidFromRandomValues()
        expect(a).not.toBe(b)
    })
})
