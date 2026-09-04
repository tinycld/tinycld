import type { SerializableTriggerConfig, TriggerItem } from '@tinycld/core/lib/editor/rich/triggers'
import { createTriggerBridgeRender } from '@tinycld/core/lib/editor/rich/webview/source/trigger-render-bridge'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// The wire format between the WebView page and the native host. Nothing on this
// path can be e2e'd — boards' Playwright suite runs on web, where none of this
// code executes — so these assertions are the only automated proof the two
// sides agree.

const CONFIG: SerializableTriggerConfig = {
    id: 'boards-mention',
    char: '@',
    allItems: [],
    insertTemplate: '[[@{id}]] ',
}

const ITEMS: TriggerItem[] = [
    { id: 'u1', label: 'Ada Lovelace', secondary: 'ada@example.com' },
    { id: 'u2', label: 'Grace Hopper', secondary: 'grace@navy.mil' },
]

function makeRect(): DOMRect {
    return { top: 100, left: 40, width: 2, height: 18 } as DOMRect
}

// The bridge listens on both window and document (platforms differ in which
// one delivers), so the fake records BOTH and dispatches to whatever is
// registered. Stubbing rather than booting jsdom keeps the suite in node and
// makes the listener add/remove itself observable.
type Listener = (evt: { data: unknown }) => void

function fakeTarget() {
    const listeners = new Set<Listener>()
    return {
        listeners,
        addEventListener: (_type: string, fn: Listener) => listeners.add(fn),
        removeEventListener: (_type: string, fn: Listener) => listeners.delete(fn),
    }
}

let win: ReturnType<typeof fakeTarget>
let doc: ReturnType<typeof fakeTarget>

function setup(overrides: { editorInstanceId?: string } = {}) {
    const posted: Record<string, unknown>[] = []
    const command = vi.fn()
    const exit = vi.fn()
    const render = createTriggerBridgeRender(CONFIG, {
        postToHost: message => posted.push(message as Record<string, unknown>),
        newRequestId: id => `${id}-req1`,
        exitSuggestion: exit as never,
        editorInstanceId: overrides.editorInstanceId ?? 'rich-1',
    })
    const handlers = render()
    const props = {
        items: ITEMS,
        query: '',
        command,
        clientRect: () => makeRect(),
        editor: { view: {} },
    }
    return { posted, command, exit, handlers, props }
}

/** Deliver a host→page message the way the WebView runtime would. */
function deliver(message: unknown) {
    deliverRaw(JSON.stringify(message))
}

function deliverRaw(data: unknown) {
    for (const fn of [...win.listeners, ...doc.listeners]) fn({ data })
}

beforeEach(() => {
    win = fakeTarget()
    doc = fakeTarget()
    vi.stubGlobal('window', { ...win, scrollX: 0, scrollY: 0 })
    vi.stubGlobal('document', doc)
})

afterEach(() => {
    vi.unstubAllGlobals()
})

describe('show-popover', () => {
    it('posts the kind, rect, items and instance id', () => {
        const { posted, handlers, props } = setup()
        handlers.onStart?.(props as never)

        expect(posted).toHaveLength(1)
        expect(posted[0]).toMatchObject({
            namespace: 'ui',
            type: 'show-popover',
            requestId: 'boards-mention-req1',
            payload: {
                kind: 'trigger:boards-mention',
                rect: { top: 100, left: 40, width: 2, height: 18 },
                payload: { items: ITEMS, query: '', selectedIndex: 0 },
                editorInstanceId: 'rich-1',
            },
        })
    })

    it('posts NOTHING when there is no anchor', () => {
        // Better to stay silent than ask the host to draw at (0,0), which reads
        // to a user as a popover that appeared in the corner for no reason.
        const { posted, handlers, props } = setup()
        handlers.onStart?.({ ...props, clientRect: () => null } as never)
        expect(posted).toEqual([])
    })
})

describe('popover-update', () => {
    it('re-posts the filtered items as the query grows', () => {
        const { posted, handlers, props } = setup()
        handlers.onStart?.(props as never)
        handlers.onUpdate?.({ ...props, items: [ITEMS[0]], query: 'ada' } as never)

        expect(posted[1]).toMatchObject({
            type: 'popover-update',
            requestId: 'boards-mention-req1',
            payload: { payload: { items: [ITEMS[0]], query: 'ada', selectedIndex: 0 } },
        })
    })

    it('clamps the selection instead of resetting it when the list shrinks', () => {
        const { posted, handlers, props } = setup()
        handlers.onStart?.(props as never)
        handlers.onKeyDown?.({ event: { key: 'ArrowDown', preventDefault: () => {} } } as never)
        handlers.onUpdate?.({ ...props, items: [ITEMS[0]], query: 'ada' } as never)

        const last = posted.at(-1) as { payload: { payload: { selectedIndex: number } } }
        expect(last.payload.payload.selectedIndex).toBe(0)
    })

    it('carries the real query on arrow keys', () => {
        // text's slash-menu bridge posts an empty query here; a body that
        // renders the query would blank it on every arrow press.
        const { posted, handlers, props } = setup()
        handlers.onStart?.({ ...props, query: 'ad' } as never)
        handlers.onKeyDown?.({ event: { key: 'ArrowDown', preventDefault: () => {} } } as never)

        const last = posted.at(-1) as { payload: { payload: { query: string } } }
        expect(last.payload.payload.query).toBe('ad')
    })
})

describe('popover-result', () => {
    it('runs the plugin command for the chosen item', () => {
        const { command, handlers, props } = setup()
        handlers.onStart?.(props as never)
        deliver({
            namespace: 'ui',
            type: 'popover-result',
            requestId: 'boards-mention-req1',
            payload: { action: 'select', payload: { itemId: 'u2' } },
        })
        expect(command).toHaveBeenCalledWith(ITEMS[1])
    })

    it('ignores a result for a DIFFERENT request', () => {
        const { command, handlers, props } = setup()
        handlers.onStart?.(props as never)
        deliver({
            namespace: 'ui',
            type: 'popover-result',
            requestId: 'someone-elses-request',
            payload: { action: 'select', payload: { itemId: 'u2' } },
        })
        expect(command).not.toHaveBeenCalled()
    })

    it('ignores an unknown item id rather than inserting something wrong', () => {
        const { command, handlers, props } = setup()
        handlers.onStart?.(props as never)
        deliver({
            namespace: 'ui',
            type: 'popover-result',
            requestId: 'boards-mention-req1',
            payload: { action: 'select', payload: { itemId: 'ghost' } },
        })
        expect(command).not.toHaveBeenCalled()
    })

    it('exits the in-page plugin on dismiss', () => {
        const { exit, handlers, props } = setup()
        handlers.onStart?.(props as never)
        deliver({
            namespace: 'ui',
            type: 'popover-result',
            requestId: 'boards-mention-req1',
            payload: { action: 'dismiss' },
        })
        expect(exit).toHaveBeenCalled()
    })

    it('ignores a message from another namespace', () => {
        const { command, handlers, props } = setup()
        handlers.onStart?.(props as never)
        deliver({
            namespace: 'markdown',
            type: 'popover-result',
            requestId: 'boards-mention-req1',
            payload: { action: 'select', payload: { itemId: 'u2' } },
        })
        expect(command).not.toHaveBeenCalled()
    })

    it('survives a non-JSON message', () => {
        const { handlers, props } = setup()
        handlers.onStart?.(props as never)
        expect(() => deliverRaw('not json')).not.toThrow()
    })
})

describe('onExit', () => {
    it('tells the host it wound down, then stops listening', () => {
        const { posted, command, handlers, props } = setup()
        handlers.onStart?.(props as never)
        handlers.onExit?.(props as never)

        expect(posted.at(-1)).toMatchObject({
            type: 'popover-exited',
            requestId: 'boards-mention-req1',
            payload: { editorInstanceId: 'rich-1' },
        })

        // A late result must not reach a torn-down popover.
        deliver({
            namespace: 'ui',
            type: 'popover-result',
            requestId: 'boards-mention-req1',
            payload: { action: 'select', payload: { itemId: 'u2' } },
        })
        expect(command).not.toHaveBeenCalled()
    })

    it('can reopen after exiting', () => {
        const { posted, handlers, props } = setup()
        handlers.onStart?.(props as never)
        handlers.onExit?.(props as never)
        handlers.onStart?.(props as never)
        expect(posted.at(-1)).toMatchObject({ type: 'show-popover' })
    })
})
