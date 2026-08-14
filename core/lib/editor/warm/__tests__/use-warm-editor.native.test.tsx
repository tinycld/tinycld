// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { EditorResult } from '../../types'
import type { WarmEditorLease } from '../types'
import { createWarmEditorStore } from '../warm-editor-store'

/**
 * Covers the NATIVE lease — the path every other warm test skips.
 *
 * The rest of the suite imports the `.web` variants, where `isWarm` is false
 * and `result` is always null, so the branch LazyEditor actually takes on
 * native is never exercised. That gap hid two real defects: the commit policy
 * never reaching the warm editor, and a handover mid-submit writing one
 * surface's text onto another's record.
 *
 * The WebView is mocked, not booted. What matters here is the lease's
 * bookkeeping — who holds the instance, what generation is live, and whether
 * the surface's options reach the host — none of which needs a real editor.
 */

const editorResult = () =>
    ({
        editor: {
            getHTML: vi.fn(async () => '<p>x</p>'),
            getText: vi.fn(async () => 'x'),
            getMarkdown: vi.fn(async () => 'x'),
            setContent: vi.fn(),
            focus: vi.fn(),
            clear: vi.fn(),
            getSelection: vi.fn(async () => null),
        },
        EditorComponent: () => null,
        commands: {},
        toolbarState: {},
    }) as unknown as EditorResult

/**
 * A stand-in for WarmEditorHost that shares its contract — one store, one
 * result, an options setter — without mounting a WebView. useWarmEditor reads
 * the context through useWarmContext, so the module is mocked to hand back
 * this fake.
 */
function createFakeHost() {
    const store = createWarmEditorStore()
    const result = editorResult()
    const setOptions = vi.fn()
    return { store, result, setOptions, drafts: null as never }
}

let host = createFakeHost()

vi.mock('../WarmEditorHost.native', () => ({
    useWarmContext: () => host,
}))

const { useWarmEditor } = await import('../use-warm-editor.native')

function Harness({
    surfaceId,
    onLease,
    onFocus,
}: {
    surfaceId: string
    onLease: (lease: WarmEditorLease) => void
    onFocus?: () => void
}): ReactNode {
    const lease = useWarmEditor(surfaceId, {
        contentFormat: 'markdown',
        initialContent: '',
        onFocus,
    })
    onLease(lease)
    return null
}

afterEach(() => {
    cleanup()
    host = createFakeHost()
})

describe('useWarmEditor (native)', () => {
    it('reports warm when a host is mounted', () => {
        let lease!: WarmEditorLease
        render(<Harness surfaceId="comment:a" onLease={l => (lease = l)} />)
        expect(lease.isWarm).toBe(true)
    })

    it('hands the instance over only to the surface holding it', () => {
        let a!: WarmEditorLease
        let b!: WarmEditorLease
        render(
            <>
                <Harness surfaceId="comment:a" onLease={l => (a = l)} />
                <Harness surfaceId="comment:b" onLease={l => (b = l)} />
            </>
        )

        // Nobody has acquired: neither surface gets an editor.
        expect(a.result).toBeNull()
        expect(b.result).toBeNull()

        act(() => a.acquire())
        expect(a.result).not.toBeNull()
        expect(b.result).toBeNull()

        // The handover — tapping a second comment.
        act(() => b.acquire())
        expect(a.result).toBeNull()
        expect(b.result).not.toBeNull()
    })

    /**
     * The generation is what lets an in-flight read know it has gone stale. A
     * handover must move it, or LazyEditor's mid-submit guard cannot fire.
     */
    it('bumps the generation on every handover', () => {
        let a!: WarmEditorLease
        let b!: WarmEditorLease
        render(
            <>
                <Harness surfaceId="comment:a" onLease={l => (a = l)} />
                <Harness surfaceId="comment:b" onLease={l => (b = l)} />
            </>
        )

        act(() => a.acquire())
        const atAcquire = a.generation

        act(() => b.acquire())
        expect(b.generation).toBeGreaterThan(atAcquire)
    })

    /**
     * The surface's options — including the wrapped focus/blur/submit handlers
     * that carry the commit policy — must reach the host on acquire. Passing
     * only the raw consumer options here is what left blur-commit and ⌘↵ inert
     * on native.
     */
    it("posts the acquiring surface's options to the host", () => {
        const onFocus = vi.fn()
        let lease!: WarmEditorLease
        render(<Harness surfaceId="comment:a" onLease={l => (lease = l)} onFocus={onFocus} />)

        act(() => lease.acquire())

        expect(host.setOptions).toHaveBeenCalledOnce()
        const posted = host.setOptions.mock.calls[0][0]
        expect(posted.onFocus).toBe(onFocus)
    })

    /**
     * A surface can unmount while still holding the instance (the card closes
     * mid-edit). Without the release-on-unmount effect the store keeps a holder
     * that no longer exists and the editor never parks.
     */
    it('releases when a holding surface unmounts', () => {
        let lease!: WarmEditorLease
        const view = render(<Harness surfaceId="comment:a" onLease={l => (lease = l)} />)

        act(() => lease.acquire())
        expect(host.store.holder()).toBe('comment:a')

        view.unmount()
        expect(host.store.holder()).toBeNull()
    })

    /**
     * A displaced surface still unmounts and still releases. Honouring that
     * release would blank the editor that just took over.
     */
    it('a displaced surface unmounting does not evict the new holder', () => {
        let a!: WarmEditorLease
        let b!: WarmEditorLease
        const view = render(
            <>
                <Harness surfaceId="comment:a" onLease={l => (a = l)} />
                <Harness surfaceId="comment:b" onLease={l => (b = l)} />
            </>
        )

        act(() => a.acquire())
        act(() => b.acquire())
        expect(host.store.holder()).toBe('comment:b')

        view.unmount()
        // b released on its own unmount; the point is that a's release did not
        // clear b while b still held it.
        expect(host.store.holder()).toBeNull()
    })
})
