import { describe, expect, it } from 'vitest'
import { shouldPostInit } from '../use-webview-editor'

/**
 * Init used to be latched one-shot per mount. A warm editor is handed between
 * surfaces by re-initializing it, so the latch becomes generation tracking —
 * post each generation exactly once per page boot, and never before the page
 * is ready.
 */
describe('init dispatch', () => {
    it('posts the first init once the page is ready', () => {
        expect(shouldPostInit(null, 0, 1)).toBe(true)
    })

    it('waits for the page, since nothing would receive it', () => {
        expect(shouldPostInit(null, 0, 0)).toBe(false)
    })

    it('does not repost the generation it already sent', () => {
        expect(shouldPostInit({ generation: 0, epoch: 1 }, 0, 1)).toBe(false)
    })

    it('posts a bumped generation, which is how a handover happens', () => {
        expect(shouldPostInit({ generation: 0, epoch: 1 }, 1, 1)).toBe(true)
    })

    /**
     * Re-parenting the warm editor's WebView on a handover recreates the native
     * view and the page boots again, having lost the init it was sent moments
     * before. Not reposting left the editor at its empty stage one.
     */
    it('reposts the same generation to a page that booted again', () => {
        expect(shouldPostInit({ generation: 3, epoch: 1 }, 3, 2)).toBe(true)
    })
})
