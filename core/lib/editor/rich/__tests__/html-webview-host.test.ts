import { describe, expect, it, vi } from 'vitest'
import type { EditorMessage } from '../../message-bus/types'
import { HtmlWebViewHost } from '../html-webview-host'
import { HTML_GET, HTML_RESULT, HTML_SET } from '../webview/source/protocol'

/**
 * The html channel is what mail's compose window reads on close and on send.
 * Before it existed those calls went over TenTap's bridge, which our page never
 * answers, and the compose window's close button did nothing on native — so
 * the settle-no-matter-what behaviour is the point of these tests.
 */

function makeHost(options: { timeoutMs?: number; canPost?: boolean } = {}) {
    const sent: EditorMessage[] = []
    const host = new HtmlWebViewHost({
        postMessage: message => {
            sent.push(message)
            return options.canPost ?? true
        },
        timeoutMs: options.timeoutMs,
    })
    host.markLive()
    return { host, sent }
}

function resultMessage(html: string, text: string, requestId?: string): EditorMessage {
    return { namespace: 'html', type: HTML_RESULT, payload: { html, text }, requestId }
}

describe('HtmlWebViewHost', () => {
    it('posts a get on the html namespace and resolves with both parts', async () => {
        const { host, sent } = makeHost()
        const pending = host.get()
        expect(sent[0]).toMatchObject({ namespace: 'html', type: HTML_GET })
        host.handleMessage(resultMessage('<p>hi</p>', 'hi', sent[0]?.requestId))
        await expect(pending).resolves.toEqual({ html: '<p>hi</p>', text: 'hi' })
    })

    it('resolves rather than rejects when the WebView never answers', async () => {
        vi.useFakeTimers()
        try {
            const { host } = makeHost({ timeoutMs: 50 })
            const pending = host.get()
            vi.advanceTimersByTime(51)
            // A close handler awaiting this must always get to close().
            await expect(pending).resolves.toEqual({ html: '', text: '' })
        } finally {
            vi.useRealTimers()
        }
    })

    it('settles immediately when the WebView is not mounted', async () => {
        const { host } = makeHost({ canPost: false })
        await expect(host.get()).resolves.toEqual({ html: '', text: '' })
    })

    it('posts setHtml as a set carrying only the html', () => {
        const { host, sent } = makeHost()
        host.setHtml('<p>draft</p>')
        expect(sent[0]).toEqual({
            namespace: 'html',
            type: HTML_SET,
            payload: { html: '<p>draft</p>' },
        })
    })

    it('falls back to the html it was told to set, with no guessed text', async () => {
        const { host } = makeHost({ canPost: false })
        host.setHtml('<p>draft</p>')
        await expect(host.get()).resolves.toEqual({ html: '<p>draft</p>', text: '' })
    })

    it('ignores a malformed result', async () => {
        const { host } = makeHost({ canPost: false })
        expect(
            host.handleMessage({ namespace: 'html', type: HTML_RESULT, payload: { html: 1 } })
        ).toBe(true)
        await expect(host.get()).resolves.toEqual({ html: '', text: '' })
    })

    it('ignores the markdown channel', () => {
        const { host } = makeHost()
        expect(
            host.handleMessage({ namespace: 'markdown', type: HTML_RESULT, payload: null })
        ).toBe(false)
    })
})
