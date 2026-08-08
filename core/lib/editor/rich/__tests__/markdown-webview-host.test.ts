import { describe, expect, it, vi } from 'vitest'
import type { EditorMessage } from '../../message-bus/types'
import { MarkdownWebViewHost } from '../markdown-webview-host'
import { MARKDOWN_RESULT } from '../webview/source/protocol'

/**
 * The markdown channel is the save path for card descriptions, so its failure
 * modes matter more than its happy path: a request that never settles hangs a
 * save, and a rejection loses the user's text.
 */

function makeHost(options: { timeoutMs?: number; canPost?: boolean } = {}) {
    const sent: EditorMessage[] = []
    const host = new MarkdownWebViewHost({
        postMessage: message => {
            sent.push(message)
            return options.canPost ?? true
        },
        timeoutMs: options.timeoutMs,
    })
    return { host, sent }
}

function resultMessage(markdown: string, requestId?: string): EditorMessage {
    return { namespace: 'markdown', type: MARKDOWN_RESULT, payload: { markdown }, requestId }
}

describe('MarkdownWebViewHost', () => {
    it('resolves a get with the WebView response', async () => {
        const { host, sent } = makeHost()
        const pending = host.get()
        const requestId = sent[0]?.requestId
        expect(requestId).toBeDefined()
        host.handleMessage(resultMessage('# hi', requestId))
        await expect(pending).resolves.toBe('# hi')
    })

    it('correlates concurrent requests answered out of order', async () => {
        const { host, sent } = makeHost()
        const first = host.get()
        const second = host.get()
        const [idA, idB] = [sent[0]?.requestId, sent[1]?.requestId]
        expect(idA).not.toBe(idB)

        // Answer the second request first — the resolver must key on the id,
        // not on arrival order.
        host.handleMessage(resultMessage('second', idB))
        host.handleMessage(resultMessage('first', idA))

        await expect(first).resolves.toBe('first')
        await expect(second).resolves.toBe('second')
    })

    it('resolves rather than rejects when the WebView never answers', async () => {
        vi.useFakeTimers()
        try {
            const { host } = makeHost({ timeoutMs: 50 })
            host.seed('seeded body')
            const pending = host.get()
            vi.advanceTimersByTime(51)
            // A save path that throws loses the user's text; a stale value is
            // recoverable.
            await expect(pending).resolves.toBe('seeded body')
        } finally {
            vi.useRealTimers()
        }
    })

    it('settles immediately when the WebView is not mounted', async () => {
        // No fake timers: this must not depend on the timeout elapsing.
        const { host } = makeHost({ canPost: false })
        host.seed('fallback')
        await expect(host.get()).resolves.toBe('fallback')
    })

    it('returns the last known value after a successful round-trip', async () => {
        vi.useFakeTimers()
        try {
            const { host, sent } = makeHost({ timeoutMs: 50 })
            const first = host.get()
            host.handleMessage(resultMessage('live value', sent[0]?.requestId))
            await expect(first).resolves.toBe('live value')

            const second = host.get()
            vi.advanceTimersByTime(51)
            await expect(second).resolves.toBe('live value')
        } finally {
            vi.useRealTimers()
        }
    })

    it('drains in-flight requests on destroy', async () => {
        const { host } = makeHost({ timeoutMs: 10_000 })
        host.seed('draining')
        const pending = host.get()
        host.destroy()
        // Without the drain this would hang until the timeout — long after the
        // component unmounted.
        await expect(pending).resolves.toBe('draining')
    })

    it('tracks set() as the last known value', async () => {
        const { host } = makeHost({ canPost: false })
        host.set('written')
        await expect(host.get()).resolves.toBe('written')
    })

    it('ignores messages from other namespaces', () => {
        const { host } = makeHost()
        expect(host.handleMessage({ namespace: 'ui', type: 'x', payload: null })).toBe(false)
        expect(host.handleMessage({ namespace: 'markdown', type: 'other', payload: null })).toBe(
            false
        )
    })

    it('records an unsolicited result without throwing', async () => {
        const { host } = makeHost({ canPost: false })
        expect(host.handleMessage(resultMessage('pushed'))).toBe(true)
        await expect(host.get()).resolves.toBe('pushed')
    })
})
