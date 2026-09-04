import type { EditorMessage } from '@tinycld/core/lib/editor/message-bus/types'
import { TriggerItemsWebViewHost } from '@tinycld/core/lib/editor/rich/trigger-items-webview-host'
import { describe, expect, it } from 'vitest'

function setup() {
    const sent: EditorMessage[] = []
    const host = new TriggerItemsWebViewHost({
        postMessage: message => {
            sent.push(message)
            return true
        },
    })
    return { host, sent }
}

const ROSTER = [{ id: 'u1', label: 'Ada' }]

describe('TriggerItemsWebViewHost', () => {
    it('pushes a roster in the shape the page parses', () => {
        const { host, sent } = setup()
        host.push('boards-mention', ROSTER)

        expect(sent).toHaveLength(1)
        expect(sent[0]).toMatchObject({
            namespace: 'app',
            type: 'trigger-items',
            payload: { triggerId: 'boards-mention', items: ROSTER },
        })
    })

    it('SKIPS an equal roster that arrived as a fresh array', () => {
        // The whole point. A live query re-emits a new array on unrelated
        // writes, so without the identity skip every board edit would push the
        // same roster across the bridge again.
        const { host, sent } = setup()
        host.push('boards-mention', ROSTER)
        host.push('boards-mention', [{ id: 'u1', label: 'Ada' }])
        expect(sent).toHaveLength(1)
    })

    it('pushes when the roster actually changes', () => {
        const { host, sent } = setup()
        host.push('boards-mention', ROSTER)
        host.push('boards-mention', [...ROSTER, { id: 'u2', label: 'Grace' }])
        expect(sent).toHaveLength(2)
    })

    it('notices a changed label, not just a changed length', () => {
        const { host, sent } = setup()
        host.push('boards-mention', ROSTER)
        host.push('boards-mention', [{ id: 'u1', label: 'Ada Lovelace' }])
        expect(sent).toHaveLength(2)
    })

    it('tracks each trigger separately', () => {
        const { host, sent } = setup()
        host.push('boards-mention', ROSTER)
        host.push('emoji', ROSTER)
        expect(sent).toHaveLength(2)
    })

    it('re-sends after a reset, because the page lost its copy', () => {
        // A reloaded page starts with an empty store while the host still
        // remembers sending; without the reset the popover would offer nothing.
        const { host, sent } = setup()
        host.push('boards-mention', ROSTER)
        host.reset()
        host.push('boards-mention', ROSTER)
        expect(sent).toHaveLength(2)
    })
})
