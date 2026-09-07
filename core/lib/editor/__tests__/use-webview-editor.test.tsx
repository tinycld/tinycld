// @vitest-environment happy-dom
import { act, render, renderHook } from '@testing-library/react'
import type { EditorWebViewProps } from 'editor-webview'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { EditorMessage } from '../message-bus/types'

/**
 * The hosting hook against a mocked native module: the init handshake, the
 * lifecycle of the pooled instance, and the few things the hook does for the
 * page on its own (state, focus, editable, keyboard).
 */
const native = {
    postMessage: vi.fn<(key: string, data: string) => boolean>(() => true),
    requestFocus: vi.fn<(key: string) => void>(),
    destroy: vi.fn<(key: string) => void>(),
}
let hostProps: EditorWebViewProps | null = null
const listeners = new Map<string, (data: string) => void>()

vi.mock('editor-webview', () => ({
    EditorWebView: (props: EditorWebViewProps) => {
        hostProps = props
        return null
    },
    postMessage: (key: string, data: string) => native.postMessage(key, data),
    requestFocus: (key: string) => native.requestFocus(key),
    destroy: (key: string) => native.destroy(key),
    getState: () => ({ exists: true, loaded: true, attached: true }),
    subscribe: (key: string, l: { onMessage: (data: string) => void }) => {
        listeners.set(key, l.onMessage)
        return () => listeners.delete(key)
    },
}))

const { useWebViewEditor } = await import('../use-webview-editor')

beforeEach(() => {
    hostProps = null
    listeners.clear()
    native.postMessage.mockReset().mockReturnValue(true)
    native.requestFocus.mockReset()
    native.destroy.mockReset()
})

afterEach(() => {
    vi.useRealTimers()
})

/** Every message the hook posted, parsed, with the key it posted to. */
function posted(): Array<{ key: string; message: EditorMessage }> {
    return native.postMessage.mock.calls.map(([key, data]) => ({
        key,
        message: JSON.parse(data) as EditorMessage,
    }))
}

function postedTypes(): string[] {
    return posted().map(({ message }) => `${message.namespace}/${message.type}`)
}

function mount(
    options: { generation?: number; editable?: boolean; onFocusChange?: (f: boolean) => void } = {}
) {
    const hook = renderHook(
        (props: { generation: number; editable: boolean }) =>
            useWebViewEditor({
                editorHtml: '<html></html>',
                initPayload: { generation: props.generation },
                editable: props.editable,
                onFocusChange: options.onFocusChange,
            }),
        {
            initialProps: {
                generation: options.generation ?? 0,
                editable: options.editable ?? true,
            },
        }
    )
    const Host = hook.result.current.EditorComponent
    const view = render(<Host />)
    return { hook, view }
}

function pageSays(message: unknown) {
    act(() => {
        for (const listener of listeners.values()) listener(JSON.stringify(message))
    })
}

describe('useWebViewEditor', () => {
    it('renders the native host with a stable key and the page source', () => {
        const { hook } = mount()
        expect(hostProps?.instanceKey).toMatch(/^webview-\d+$/)
        expect(hostProps?.source).toBe('<html></html>')
        expect(hook.result.current.pageEpoch).toBe(0)
    })

    it('posts init only once the page reports ready, and again for a later boot', () => {
        const { hook } = mount()
        expect(postedTypes()).not.toContain('app/init')

        pageSays({ type: 'editor-ready' })
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(1)
        expect(hook.result.current.pageEpoch).toBe(1)

        // Same generation, same epoch: nothing new.
        hook.rerender({ generation: 0, editable: true })
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(1)

        // A hand-off bumps the generation.
        hook.rerender({ generation: 1, editable: true })
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(2)

        // The page booted again (content process died): the current
        // generation goes out once more.
        pageSays({ type: 'editor-ready' })
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(3)
        expect(hook.result.current.pageEpoch).toBe(2)
    })

    it('retries an init the pool could not deliver', () => {
        const { hook } = mount()
        native.postMessage.mockReturnValue(false)
        pageSays({ type: 'editor-ready' })
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(1)
        native.postMessage.mockReturnValue(true)
        // Nothing was recorded, so the next render posts it again.
        hook.rerender({ generation: 0, editable: true })
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(2)
    })

    it('reads toolbar state, readiness, and focus edges from stateUpdate', () => {
        const onFocusChange = vi.fn()
        const { hook } = mount({ onFocusChange })
        expect(hook.result.current.isReady).toBe(false)

        pageSays({
            type: 'stateUpdate',
            payload: { isBoldActive: true, isReady: true, isFocused: true },
        })
        expect(hook.result.current.toolbarState.isBoldActive).toBe(true)
        expect(hook.result.current.isReady).toBe(true)
        expect(onFocusChange).toHaveBeenCalledTimes(1)
        expect(onFocusChange).toHaveBeenLastCalledWith(true)

        pageSays({
            type: 'stateUpdate',
            payload: { isBoldActive: false, isReady: true, isFocused: true },
        })
        expect(hook.result.current.toolbarState.isBoldActive).toBe(false)
        expect(onFocusChange).toHaveBeenCalledTimes(1)

        pageSays({ type: 'stateUpdate', payload: { isReady: true, isFocused: false } })
        expect(onFocusChange).toHaveBeenLastCalledWith(false)
    })

    it('focuses through the native view and the page together', () => {
        const { hook } = mount()
        const key = hostProps?.instanceKey
        act(() => hook.result.current.editor.focus({ x: 3, y: 4 }))
        expect(native.requestFocus).toHaveBeenCalledWith(key)
        expect(posted().at(-1)).toEqual({
            key,
            message: { namespace: 'app', type: 'focus', payload: { x: 3, y: 4 } },
        })
        act(() => hook.result.current.editor.focus())
        expect(posted().at(-1)?.message.payload).toBe('end')
    })

    it('sends editable changes as a format command', () => {
        const { hook } = mount()
        hook.rerender({ generation: 0, editable: false })
        expect(posted().at(-1)?.message).toEqual({
            namespace: 'format',
            type: 'set-editable',
            payload: false,
        })
    })

    it('survives a host remount and releases the instance only when the hook unmounts', () => {
        const { hook, view } = mount()
        const key = hostProps?.instanceKey
        pageSays({ type: 'editor-ready' })
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(1)

        // The warm editor is rendered off-screen, then inside whichever
        // surface holds it: the host component comes and goes, the
        // subscription does not.
        view.unmount()
        expect(native.destroy).not.toHaveBeenCalled()
        expect(listeners.size).toBe(1)
        const Host = hook.result.current.EditorComponent
        render(<Host />)
        expect(hostProps?.instanceKey).toBe(key)
        // No reboot, so no second init.
        expect(postedTypes().filter(t => t === 'app/init')).toHaveLength(1)

        hook.unmount()
        expect(native.destroy).toHaveBeenCalledTimes(1)
        expect(native.destroy).toHaveBeenCalledWith(key)
        expect(listeners.size).toBe(0)
    })

    it('routes namespaced messages to the generic subscriber and ui scroll to onScroll', () => {
        const onMessage = vi.fn()
        const onScroll = vi.fn()
        const hook = renderHook(() =>
            useWebViewEditor({
                editorHtml: '',
                initPayload: {},
                editable: true,
                onMessage,
                onScroll,
            })
        )
        const Host = hook.result.current.EditorComponent
        render(<Host />)
        pageSays({ namespace: 'html', type: 'result', payload: { html: '', text: '' } })
        expect(onMessage).toHaveBeenCalledWith({
            namespace: 'html',
            type: 'result',
            payload: { html: '', text: '' },
        })
        pageSays({ namespace: 'ui', type: 'document-scroll', payload: null })
        expect(onScroll).toHaveBeenCalledTimes(1)
        pageSays('not json')
        expect(onMessage).toHaveBeenCalledTimes(1)
    })
})
