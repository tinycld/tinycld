import { captureException } from '@tinycld/core/lib/errors'
import { isChunkLoadError, reportBoundaryError } from '@tinycld/core/lib/use-chunk-load-recovery'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@tinycld/core/lib/errors', () => ({ captureException: vi.fn() }))

// Expo's web async-require throws this when a lazy route chunk does not load
// (expo/src/async-require/fetchThenEval.web.ts). The class is not exported,
// so the test builds an error of the same shape.
function expoAsyncRequireError() {
    const url = 'https://h.example/_expo/static/js/web/index-f9b6b8f03a.js'
    return Object.assign(new Error(`Loading module ${url} failed.\n(error: ${url})`), {
        name: 'AsyncRequireError',
    })
}

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

    it('matches the error Expo throws when a lazy route chunk does not load', () => {
        expect(isChunkLoadError(expoAsyncRequireError())).toBe(true)
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

describe('reportBoundaryError', () => {
    const reload = vi.fn()
    let stored: string | null = null

    beforeEach(() => {
        stored = null
        vi.stubGlobal('window', {
            location: { reload },
            sessionStorage: {
                getItem: () => stored,
                setItem: (_key: string, value: string) => {
                    stored = value
                },
            },
        })
    })

    afterEach(() => {
        vi.unstubAllGlobals()
        vi.mocked(captureException).mockClear()
        reload.mockClear()
    })

    it('reloads on a stale route chunk instead of reporting a crash', () => {
        reportBoundaryError(expoAsyncRequireError())
        expect(reload).toHaveBeenCalledTimes(1)
        expect(captureException).not.toHaveBeenCalled()
    })

    it('reports, and does not reload, a stale chunk that comes back right after a reload', () => {
        reportBoundaryError(expoAsyncRequireError())
        reportBoundaryError(expoAsyncRequireError())
        expect(reload).toHaveBeenCalledTimes(1)
        expect(captureException).toHaveBeenCalledWith(
            'chunk-load failure (suppressed reload)',
            expect.any(Error),
            expect.objectContaining({ source: 'errorBoundary' })
        )
    })

    it('reports any other error as a crash', () => {
        const err = new TypeError('x is not a function')
        reportBoundaryError(err)
        expect(reload).not.toHaveBeenCalled()
        expect(captureException).toHaveBeenCalledWith('app.errorBoundary', err)
    })
})
