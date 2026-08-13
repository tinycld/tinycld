import type { EditorMessage } from '@tinycld/core/lib/editor/message-bus/types'
import {
    publishUiMessage,
    resetUiMessageBus,
    subscribeUiMessage,
} from '@tinycld/core/lib/editor/overlay/ui-message-bus'
import { beforeEach, describe, expect, it, vi } from 'vitest'

function showPopover(editorInstanceId?: string): EditorMessage {
    return {
        namespace: 'ui',
        type: 'show-popover',
        requestId: 'req1',
        payload: {
            kind: 'trigger:cards-mention',
            ...(editorInstanceId ? { editorInstanceId } : {}),
        },
    }
}

beforeEach(() => {
    resetUiMessageBus()
})

describe('subscribe and publish', () => {
    it('delivers to a subscriber', () => {
        const handler = vi.fn()
        subscribeUiMessage(handler)
        publishUiMessage(showPopover())
        expect(handler).toHaveBeenCalledOnce()
    })

    it('stops delivering after unsubscribe', () => {
        const handler = vi.fn()
        subscribeUiMessage(handler)()
        publishUiMessage(showPopover())
        expect(handler).not.toHaveBeenCalled()
    })

    it('double-unsubscribe is harmless', () => {
        const unsubscribe = subscribeUiMessage(vi.fn())
        unsubscribe()
        expect(() => unsubscribe()).not.toThrow()
    })

    it('a throwing handler does not starve the others, and the error still surfaces', () => {
        const second = vi.fn()
        subscribeUiMessage(() => {
            throw new Error('boom')
        })
        subscribeUiMessage(second)
        expect(() => publishUiMessage(showPopover())).toThrow('boom')
        expect(second).toHaveBeenCalledOnce()
    })
})

describe('instance scoping', () => {
    // The regression guard for the multi-editor bug. A card detail mounts a
    // description editor, a comment composer, and sometimes an inline comment
    // editor at once — each with its own overlay controller. Without filtering,
    // one `@` opens three popovers, two of them measured against the wrong
    // WebView.
    it('a scoped subscriber receives only its own editor traffic', () => {
        const mine = vi.fn()
        const theirs = vi.fn()
        subscribeUiMessage(mine, 'rich-1')
        subscribeUiMessage(theirs, 'rich-2')

        publishUiMessage(showPopover('rich-1'))

        expect(mine).toHaveBeenCalledOnce()
        expect(theirs).not.toHaveBeenCalled()
    })

    it('an unaddressed message broadcasts to every scoped subscriber', () => {
        // dismiss-on-scroll is a screen-wide gesture: every open popover should
        // close, whichever editor it belongs to.
        const first = vi.fn()
        const second = vi.fn()
        subscribeUiMessage(first, 'rich-1')
        subscribeUiMessage(second, 'rich-2')

        publishUiMessage({
            namespace: 'ui',
            type: 'popover-dismiss-on-scroll',
            payload: null,
        })

        expect(first).toHaveBeenCalledOnce()
        expect(second).toHaveBeenCalledOnce()
    })

    it('an UNSCOPED subscriber still sees addressed traffic', () => {
        // Preserves the single-editor case, where passing an id is pointless.
        const handler = vi.fn()
        subscribeUiMessage(handler)
        publishUiMessage(showPopover('rich-1'))
        expect(handler).toHaveBeenCalledOnce()
    })

    it('tolerates a payload that is not an object', () => {
        const handler = vi.fn()
        subscribeUiMessage(handler, 'rich-1')
        expect(() =>
            publishUiMessage({ namespace: 'ui', type: 'popover-exited', payload: null })
        ).not.toThrow()
        expect(handler).toHaveBeenCalledOnce()
    })
})
