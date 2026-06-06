import { isChunkLoadError } from '@tinycld/core/lib/use-chunk-load-recovery'
import { describe, expect, it } from 'vitest'

describe('isChunkLoadError', () => {
    it('matches errors with name=ChunkLoadError', () => {
        const err = Object.assign(new Error('whatever'), { name: 'ChunkLoadError' })
        expect(isChunkLoadError(err)).toBe(true)
    })

    it.each([
        ['webpack/metro chunk-load text', 'Loading chunk 23 failed'],
        ['raw ChunkLoadError text', 'ChunkLoadError: failed'],
        [
            'chrome/firefox dynamic-import failure',
            'Failed to fetch dynamically imported module: /a.js',
        ],
        ['safari dynamic-import failure', 'Importing a module script failed'],
        ['lower-case dynamic import failure', 'error loading dynamically imported module'],
    ])('matches via message: %s', (_label, msg) => {
        expect(isChunkLoadError(new Error(msg))).toBe(true)
    })

    it('matches a bare event message argument too (no error object)', () => {
        expect(isChunkLoadError(undefined, 'Loading chunk 99 failed')).toBe(true)
    })

    it('does not match unrelated errors', () => {
        expect(isChunkLoadError(new TypeError('x is not a function'))).toBe(false)
        expect(isChunkLoadError(new Error('Network request failed'))).toBe(false)
        expect(isChunkLoadError(null)).toBe(false)
        expect(isChunkLoadError(undefined)).toBe(false)
        expect(isChunkLoadError({})).toBe(false)
    })
})
