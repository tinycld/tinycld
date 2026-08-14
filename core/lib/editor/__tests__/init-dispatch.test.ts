import { describe, expect, it } from 'vitest'
import { shouldPostInit } from '../use-webview-editor'

/**
 * Init used to be latched one-shot per mount. A warm editor is handed between
 * surfaces by re-initializing it, so the latch becomes generation tracking —
 * post each generation exactly once, and never before the page is ready.
 */
describe('init dispatch', () => {
    it('posts the first init once the page is ready', () => {
        expect(shouldPostInit(null, 0, true)).toBe(true)
    })

    it('waits for the page, since nothing would receive it', () => {
        expect(shouldPostInit(null, 0, false)).toBe(false)
    })

    it('does not repost the generation it already sent', () => {
        expect(shouldPostInit(0, 0, true)).toBe(false)
    })

    it('posts a bumped generation, which is how a handover happens', () => {
        expect(shouldPostInit(0, 1, true)).toBe(true)
    })
})
