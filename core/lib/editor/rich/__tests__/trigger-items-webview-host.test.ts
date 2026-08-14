import { describe, expect, it } from 'vitest'
import { TriggerItemsWebViewHost } from '../trigger-items-webview-host'

// The host memoizes what it has sent so a live query re-emitting an unchanged
// roster does not flood the bridge. The subtlety is WHEN it is allowed to
// record that memo — see the delivery test below, which is the bug that made
// `@` offer "No matches" for a whole session.

const ITEMS = [{ id: 'u1', label: 'Ada' }]

function hostWith(deliver: () => boolean) {
    const sent: unknown[] = []
    const host = new TriggerItemsWebViewHost({
        postMessage: message => {
            const ok = deliver()
            if (ok) sent.push(message)
            return ok
        },
    })
    return { host, sent }
}

describe('TriggerItemsWebViewHost', () => {
    it('pushes a roster the first time', () => {
        const { host, sent } = hostWith(() => true)
        expect(host.push('m', ITEMS)).toBe(true)
        expect(sent).toHaveLength(1)
    })

    it('skips an unchanged roster', () => {
        const { host, sent } = hostWith(() => true)
        host.push('m', ITEMS)
        expect(host.push('m', ITEMS)).toBe(false)
        expect(sent).toHaveLength(1)
    })

    it('pushes again when the roster changes', () => {
        const { host, sent } = hostWith(() => true)
        host.push('m', ITEMS)
        host.push('m', [...ITEMS, { id: 'u2', label: 'Grace' }])
        expect(sent).toHaveLength(2)
    })

    // The regression. The editor mounts before the members query resolves, so
    // the first push is []. When the real roster arrives the WebView may still
    // be mounting and postMessage returns false — if that attempt were memoized,
    // every later emission would carry the same members, match the memo, and be
    // skipped forever. The page would keep the empty list and the picker would
    // say "No matches" for the rest of the session.
    it('does not memoize a push the bridge refused', () => {
        let online = false
        const { host, sent } = hostWith(() => online)

        expect(host.push('m', ITEMS)).toBe(false)
        expect(sent).toHaveLength(0)

        online = true
        expect(host.push('m', ITEMS)).toBe(true)
        expect(sent).toHaveLength(1)
    })

    it('re-sends everything after a reset, for a reloaded page', () => {
        const { host, sent } = hostWith(() => true)
        host.push('m', ITEMS)
        host.reset()
        expect(host.push('m', ITEMS)).toBe(true)
        expect(sent).toHaveLength(2)
    })

    it('tracks each trigger id separately', () => {
        const { host, sent } = hostWith(() => true)
        host.push('mention', ITEMS)
        host.push('slash', ITEMS)
        expect(sent).toHaveLength(2)
    })
})
