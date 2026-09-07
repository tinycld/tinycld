import { describe, expect, it } from 'vitest'
import type { EditorMessage } from '../../message-bus/types'
import { DocumentWebViewHost } from '../document-webview-host'

/**
 * Liveness: a push has to reach an editor that exists. Before the host tracked
 * this, a set made while the WebView was still booting was posted into nothing
 * — the body of a reopened mail draft, the roster-independent part of every
 * "content never appears" report on native.
 */

function makeHost(options: { canPost?: boolean } = {}) {
    const sent: EditorMessage[] = []
    const host = new DocumentWebViewHost<string>(
        {
            namespace: 'markdown',
            types: { set: 'set', get: 'get', result: 'result' },
            initial: '',
            decodeResult: payload => {
                const value = (payload as { value?: unknown } | undefined)?.value
                return typeof value === 'string' ? value : null
            },
            encodeSet: value => ({ value }),
        },
        {
            postMessage: message => {
                sent.push(message)
                return options.canPost ?? true
            },
        }
    )
    return { host, sent }
}

describe('DocumentWebViewHost liveness', () => {
    it('holds a set until the page reports an editor, then delivers it', () => {
        const { host, sent } = makeHost()
        host.set('draft body')
        expect(sent).toEqual([])
        host.markLive()
        expect(sent).toEqual([
            { namespace: 'markdown', type: 'set', payload: { value: 'draft body' } },
        ])
    })

    it('delivers only the latest of several sets made while not live', () => {
        const { host, sent } = makeHost()
        host.set('first')
        host.set('second')
        host.markLive()
        expect(sent.map(m => m.payload)).toEqual([{ value: 'second' }])
    })

    it('posts immediately once live', () => {
        const { host, sent } = makeHost()
        host.markLive()
        host.set('typed')
        expect(sent).toHaveLength(1)
    })

    it('holds again after the editor goes stale, for the next one', () => {
        const { host, sent } = makeHost()
        host.markLive()
        host.markStale()
        host.set('for the rebuilt editor')
        expect(sent).toEqual([])
        host.markLive()
        expect(sent).toHaveLength(1)
    })

    /**
     * A handover rebuilds the editor for a different surface, whose document
     * comes from init. A push the displaced surface left undelivered must not
     * overwrite it.
     */
    it('drops an undelivered set on a handover', () => {
        const { host, sent } = makeHost()
        host.set('previous surface')
        host.seedGeneration(1, 'next surface')
        host.markLive()
        expect(sent).toEqual([])
    })

    it('holds pushes across a handover until the rebuilt editor mounts', () => {
        const { host, sent } = makeHost()
        host.markLive()
        host.seedGeneration(1, 'next surface')
        host.set('for the new editor')
        expect(sent).toEqual([])
        host.markLive()
        expect(sent.map(m => m.payload)).toEqual([{ value: 'for the new editor' }])
    })

    it('answers a get from the last known value while no editor is live', async () => {
        const { host, sent } = makeHost()
        host.set('held')
        await expect(host.get()).resolves.toBe('held')
        expect(sent).toEqual([])
    })
})
