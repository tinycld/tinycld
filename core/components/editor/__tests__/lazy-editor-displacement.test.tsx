// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { Text, View } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WarmEditorLease } from '../../../lib/editor/warm/types'

/**
 * Being displaced — another surface taking the one shared editor.
 *
 * This is how an edit finishes when the reader clicks somewhere else, and it
 * used to happen only as a SIDE EFFECT of blur: handing the instance over moved
 * its DOM node, the move blurred it, and the blur handler committed. That made
 * every stray focus loss indistinguishable from a real handover, which is why
 * pressing a toolbar menu silently finished an edit.
 *
 * The lease is mocked because the two guards below are about STATE the store
 * passes through, and both windows they protect are a frame wide — a real
 * editor cannot be held in either on demand.
 */

let holder: string | null = 'comment:a'
let result: unknown = null

const warmEditor = {
    getHTML: vi.fn(async () => '<p>warm</p>'),
    getText: vi.fn(async () => 'warm'),
    getMarkdown: vi.fn(async () => 'edited text'),
    setContent: vi.fn(),
    setMarkdown: vi.fn(),
    focus: vi.fn(),
    clear: vi.fn(),
    getSelection: vi.fn(async () => null),
}

const warmResult = {
    editor: warmEditor,
    EditorComponent: () => null,
    commands: {},
    toolbarState: {},
}

const lease: WarmEditorLease = {
    isWarm: true,
    ready: true,
    get holder() {
        return holder as never
    },
    acquire: vi.fn(),
    release: vi.fn(),
    get result() {
        return result as never
    },
    generation: 1,
}

vi.mock('../../../lib/editor/warm', () => ({
    useWarmEditor: () => lease,
}))

const { useLazyEditor } = await import('../LazyEditor')

function Probe({
    onCommit,
    onRelease,
    commitOnDisplace,
}: {
    onCommit: (content: string) => void
    onRelease?: (content: string) => void
    commitOnDisplace?: boolean
}) {
    const slots = useLazyEditor({
        readView: <Text>read view</Text>,
        value: 'persisted',
        contentFormat: 'markdown',
        editorOptions: {},
        surfaceId: 'comment:a',
        canEdit: true,
        startOpen: true,
        commitOnDisplace,
        onCommit,
        onRelease,
        renderEditor: () => <Text>editing surface</Text>,
        testID: 'probe',
    })
    return <View>{slots.body}</View>
}

beforeEach(() => {
    holder = 'comment:a'
    result = warmResult
    vi.clearAllMocks()
})

afterEach(cleanup)

describe('displacement', () => {
    /**
     * Unmounting with an unfinished edit commits it.
     *
     * This is the route a parent-owned surface actually takes. Cards mounts one
     * inline comment editor, for whichever id is being edited, so clicking a
     * second comment sets that id and UNMOUNTS the first in the same commit —
     * it never renders again to notice it was displaced. Blur used to cover this
     * by firing synchronously during the DOM move.
     */
    it('commits an unfinished edit when the surface goes away', async () => {
        const onCommit = vi.fn()
        const { unmount } = render(<Probe onCommit={onCommit} commitOnDisplace />)

        await act(async () => {
            unmount()
        })

        expect(onCommit).toHaveBeenCalledWith('edited text')
    })

    /**
     * The unmount path must stash too, not only commit.
     *
     * A parent-owned surface is UNMOUNTED by the same commit that hands the
     * editor on, so it never renders again to notice the displacement — and for
     * a surface that stashes rather than commits, doing nothing on unmount
     * loses the revision outright. The surface is gone, and with it the only
     * copy of what was typed. Boards' inline comment edit is the case.
     */
    it('stashes an unfinished edit when a non-committing surface goes away', async () => {
        const onCommit = vi.fn()
        const onRelease = vi.fn()
        const { unmount } = render(<Probe onCommit={onCommit} onRelease={onRelease} />)

        await act(async () => {
            unmount()
        })

        expect(onRelease).toHaveBeenCalledWith('edited text')
        expect(onCommit).not.toHaveBeenCalled()
    })

    it('stashes instead, for a surface that does not commit', async () => {
        const onCommit = vi.fn()
        const onRelease = vi.fn()
        const { rerender } = render(<Probe onCommit={onCommit} onRelease={onRelease} />)

        holder = 'comment:b'
        result = null
        await act(async () => {
            rerender(<Probe onCommit={onCommit} onRelease={onRelease} />)
        })

        expect(onRelease).toHaveBeenCalledWith('edited text')
        expect(onCommit).not.toHaveBeenCalled()
    })

    /**
     * A null holder is the editor PARKED, not stolen. It reads null between a
     * release and the next acquire, and before this surface's own startOpen
     * acquire lands — committing there would write on every transient release,
     * including during the boot.
     */
    it('does nothing when the editor is merely unheld', async () => {
        const onCommit = vi.fn()
        const { rerender } = render(<Probe onCommit={onCommit} commitOnDisplace />)

        holder = null
        result = null
        await act(async () => {
            rerender(<Probe onCommit={onCommit} commitOnDisplace />)
        })

        expect(onCommit).not.toHaveBeenCalled()
    })

    /**
     * A session displaced before its editor ever arrived writes nothing.
     *
     * `holder` is set the instant a surface acquires, but `result` stays null
     * until the singleton has booted — so a surface can be the holder with
     * nothing to type into, which is why `hasHeld` requires a real editor and
     * not just the store's name for it.
     *
     * CAVEAT: this asserts the invariant, but does not reproduce the race that
     * motivated it. That surfaced only under parallel e2e load, where the boot
     * is slow enough to lose; both orderings pass here with the guard reverted.
     * Treat the guard as covered by reasoning, not by this test.
     */
    it('does not commit a session that never received an editor', async () => {
        const onCommit = vi.fn()
        // Named as the holder the instant it acquires, but the singleton has
        // not finished booting — so there is nothing to type into yet.
        holder = 'comment:a'
        result = null
        const { rerender } = render(<Probe onCommit={onCommit} commitOnDisplace />)
        await act(async () => {
            rerender(<Probe onCommit={onCommit} commitOnDisplace />)
        })

        // Displaced before the editor ever arrived. There is nothing to write:
        // committing here would submit against an instance this surface never
        // had, and `submit` with no editor drops the write and ends the session
        // — killing an edit the user had only just opened.
        holder = 'comment:b'
        await act(async () => {
            rerender(<Probe onCommit={onCommit} commitOnDisplace />)
        })

        expect(onCommit).not.toHaveBeenCalled()
    })
})
