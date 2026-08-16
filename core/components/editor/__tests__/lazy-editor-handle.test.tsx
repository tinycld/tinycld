// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { useRef } from 'react'
import { Text, View } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { EditorResult } from '../../../lib/editor/types'
import type { WarmEditorLease } from '../../../lib/editor/warm/types'

/**
 * The imperative handle: how a caller drives a surface from OUTSIDE its own
 * chrome. A comment composer's Reply button lives in the activity list, and a
 * keyboard shortcut lives on the screen — neither can reach `slots`, which
 * exists only while the surface is already editing.
 *
 * The load-bearing rule is that `edit()` is IDEMPOTENT. A session that is
 * already open (including one displaced by a steal, which is the case this
 * exists for) must reclaim the editor without re-snapshotting the revert
 * baseline — otherwise the user's half-typed text becomes the thing Escape
 * reverts to.
 */

const editorHandle = {
    getHTML: vi.fn(async () => '<p>held</p>'),
    getText: vi.fn(async () => 'held'),
    getMarkdown: vi.fn(async () => 'typed by the user'),
    setContent: vi.fn(),
    setMarkdown: vi.fn(),
    focus: vi.fn(),
    clear: vi.fn(),
    getSelection: vi.fn(async () => null),
}

const heldResult = {
    editor: editorHandle,
    EditorComponent: () => <Text>EDITOR</Text>,
    commands: {},
    toolbarState: {},
} as unknown as EditorResult

let leaseState: { result: EditorResult | null } = { result: heldResult }
const acquire = vi.fn()
const release = vi.fn()

function currentLease(): WarmEditorLease {
    return {
        isWarm: true,
        acquire,
        release,
        generation: 1,
        ready: true,
        holder: leaseState.result ? 'comment:a' : null,
        result: leaseState.result,
    }
}

// Built per call rather than closed over a module-level const: vi.mock factories
// are hoisted above these declarations, so capturing one directly would read it
// in its temporal dead zone.
vi.mock('../../../lib/editor/warm', () => ({
    useWarmEditor: () => currentLease(),
}))

const { useLazyEditor, LazyEditor } = await import('../LazyEditor')
type Handle = import('../LazyEditor').LazyEditorHandle

/** The handle from the most recent render, for the test to drive. */
let handle: Handle | null = null

function Probe({
    startOpen = false,
    value = 'persisted',
    onCommit = vi.fn(),
    onCancel,
}: {
    startOpen?: boolean
    value?: string
    onCommit?: (content: string) => void
    onCancel?: () => void
}) {
    const slots = useLazyEditor({
        readView: <Text>READ VIEW</Text>,
        value,
        contentFormat: 'markdown',
        editorOptions: {},
        surfaceId: 'comment:a',
        canEdit: true,
        startOpen,
        onCommit,
        onCancel,
        renderEditor: () => <Text>EDITOR</Text>,
    })
    handle = slots.handle
    return <View>{slots.body}</View>
}

beforeEach(() => {
    leaseState = { result: heldResult }
    handle = null
    vi.clearAllMocks()
    // Restored explicitly: one test overrides it, and clearAllMocks resets call
    // history without restoring an implementation.
    editorHandle.getMarkdown.mockResolvedValue('typed by the user')
})

afterEach(cleanup)

describe('the LazyEditor handle', () => {
    it('opens a session and takes the instance', () => {
        const { getByText, queryByText } = render(<Probe />)
        expect(getByText('READ VIEW')).toBeTruthy()

        act(() => handle?.edit())

        expect(acquire).toHaveBeenCalled()
        expect(getByText('EDITOR')).toBeTruthy()
        expect(queryByText('READ VIEW')).toBeNull()
    })

    /**
     * The regression this split exists to prevent.
     *
     * `edit()` on an open session must NOT re-run the guard reset. Doing so
     * re-snapshots `baselineRef` against the CURRENT `value` prop, and the
     * baseline is what the no-op rule compares against — so an edit that
     * genuinely reverted the user's text back to the persisted value would stop
     * reading as a no-op and would be written instead of cancelled.
     *
     * Detecting that needs the editor's content to EQUAL the persisted value:
     * with a correct baseline ('persisted') the submit is a no-op and cancels;
     * with a reset one it would still be 'persisted' — so the value prop has to
     * differ from the baseline the reset would capture. The prop is changed
     * between the two edit() calls to make the two cases distinguishable.
     */
    it('reclaims without resetting the baseline when already editing', async () => {
        const onCommit = vi.fn()
        const onCancel = vi.fn()
        // The editor reads back exactly the value the session STARTED with.
        editorHandle.getMarkdown.mockResolvedValue('original')

        const { rerender } = render(
            <Probe value="original" onCommit={onCommit} onCancel={onCancel} />
        )

        act(() => handle?.edit())
        acquire.mockClear()

        // A realtime update lands mid-session: the persisted value moves on,
        // but the revert target must stay what the session opened with.
        rerender(<Probe value="changed by someone else" onCommit={onCommit} onCancel={onCancel} />)

        // The parent says "the caret belongs here" again, mid-session.
        act(() => handle?.edit())

        // It took the instance back...
        expect(acquire).toHaveBeenCalled()

        // ...without moving the baseline. The content still equals what the
        // session opened with, so this is a no-op edit: cancel, never commit.
        // Had edit() re-opened, the baseline would now be 'changed by someone
        // else' and this same content would be written over the record.
        await act(async () => handle?.submit())

        expect(onCancel).toHaveBeenCalled()
        expect(onCommit).not.toHaveBeenCalled()
    })

    /**
     * A displaced surface is still a session. `startOpen` keeps it open through
     * a steal (#193), so `edit()` there must reclaim rather than re-open — the
     * same baseline hazard, reached by the path the composer actually takes.
     */
    it('treats a displaced session as open, not as idle', () => {
        const { rerender, getByText } = render(<Probe startOpen />)
        expect(getByText('EDITOR')).toBeTruthy()

        // Another surface takes the editor.
        leaseState = { result: null }
        rerender(<Probe startOpen />)
        expect(getByText('READ VIEW')).toBeTruthy()
        expect(handle?.isEditing()).toBe(false)

        acquire.mockClear()
        act(() => handle?.edit())

        // Reclaimed. The session was never closed, so this is not a re-open.
        expect(acquire).toHaveBeenCalled()
    })

    it('reports isEditing only while holding a usable editor', () => {
        const { rerender } = render(<Probe />)
        expect(handle?.isEditing()).toBe(false)

        act(() => handle?.edit())
        expect(handle?.isEditing()).toBe(true)

        // Displaced: the session is open but there is nothing to type into.
        leaseState = { result: null }
        rerender(<Probe />)
        expect(handle?.isEditing()).toBe(false)
    })

    it('submits and cancels through the handle', async () => {
        const onCommit = vi.fn()
        render(<Probe onCommit={onCommit} />)

        act(() => handle?.edit())
        await act(async () => handle?.submit())

        expect(onCommit).toHaveBeenCalledWith('typed by the user')
    })

    it('cancel ends the session', () => {
        const onCancel = vi.fn()
        const { getByText } = render(<Probe onCancel={onCancel} />)

        act(() => handle?.edit())
        act(() => handle?.cancel())

        expect(onCancel).toHaveBeenCalled()
        expect(getByText('READ VIEW')).toBeTruthy()
    })

    /**
     * A handle is HELD — a shortcut registered once, a parent effect that fires
     * much later — so its identity must not churn, and the object captured on
     * the first render must still drive the current one.
     */
    it('keeps one stable object across renders', () => {
        const { rerender } = render(<Probe />)
        const first = handle

        rerender(<Probe />)
        expect(handle).toBe(first)

        // And the captured object still works after the re-render.
        act(() => first?.edit())
        expect(acquire).toHaveBeenCalled()
    })

    it('exposes the same handle through the component ref', () => {
        let fromRef: Handle | null = null

        function ComponentProbe() {
            const ref = useRef<Handle>(null)
            // Published during render for the assertion below; the ref is
            // populated by useImperativeHandle before this runs on re-render.
            fromRef = ref.current
            return (
                <LazyEditor
                    ref={ref}
                    readView={<Text>READ VIEW</Text>}
                    value="persisted"
                    contentFormat="markdown"
                    editorOptions={{}}
                    surfaceId="comment:a"
                    canEdit
                    onCommit={vi.fn()}
                    renderEditor={() => <Text>EDITOR</Text>}
                />
            )
        }

        const { rerender } = render(<ComponentProbe />)
        rerender(<ComponentProbe />)

        expect(fromRef).not.toBeNull()
        act(() => fromRef?.edit())
        expect(acquire).toHaveBeenCalled()
    })
})
