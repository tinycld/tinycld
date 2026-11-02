import { describe, expect, it } from 'vitest'
import { canSolvePow } from '../pow-capability'

describe('canSolvePow', () => {
    it('needs a secure context with Web Crypto', () => {
        expect(canSolvePow({ isSecureContext: true, subtle: {} })).toBe(true)
        expect(canSolvePow({ isSecureContext: false, subtle: undefined })).toBe(false)
        expect(canSolvePow({ isSecureContext: true, subtle: undefined })).toBe(false)
    })
})
