// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { Text, View } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { EditorResult } from '../../../lib/editor/types'
import type { WarmEditorLease } from '../../../lib/editor/warm/types'

/**
 * The behavior contract, which is one rule:
 *
 *     holds a usable instance  →  the editor
 *     otherwise                →  readView
 *
 * Idle, displaced by another surface's steal, and waiting on a boot that has
 * not finished are all the SAME case — LazyEditor does not hold an editor, so
 * it renders the caller's read view. There is no third rendering and no loading
 * state in core.
 *
 * `readView: null` is therefore the defect it looks like: it renders an
 * invisible box. That is what made a displaced composer appear to vanish.
 */

const editorHandle = {
    getHTML: vi.fn(async () => '<p>held</p>'),
    getText: vi.fn(async () => 'held'),
    getMarkdown: vi.fn(async () => 'held'),
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

/** The lease the mocked module hands back; each test sets its shape. */
let leaseState: { result: EditorResult | null; ready: boolean } = {
    result: heldResult,
    ready: true,
}
const acquire = vi.fn()
const release = vi.fn()

function currentLease(): WarmEditorLease {
    return {
        isWarm: true,
        acquire,
        release,
        generation: 1,
        ready: leaseState.ready,
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

const { useLazyEditor } = await import('../LazyEditor')

function Probe({
    onCommit = vi.fn(),
    startOpen = true,
    readView = <Text>READ VIEW</Text>,
}: {
    onCommit?: (content: string) => void
    startOpen?: boolean
    readView?: React.ReactNode
}) {
    const slots = useLazyEditor({
        readView,
        value: 'persisted',
        contentFormat: 'markdown',
        editorOptions: {},
        surfaceId: 'comment:a',
        canEdit: true,
        startOpen,
        onCommit,
        renderEditor: s => {
            submitFn = s.submit
            return <s.EditorComponent />
        },
        testID: 'probe',
    })
    return <View>{slots.body}</View>
}

let submitFn: (() => void) | null = null

beforeEach(() => {
    leaseState = { result: heldResult, ready: true }
    submitFn = null
    vi.clearAllMocks()
})

afterEach(cleanup)

describe('LazyEditor renders exactly two things', () => {
    it('renders the editor while holding a ready instance', () => {
        const { getByText, queryByText } = render(<Probe />)

        expect(getByText('EDITOR')).toBeTruthy()
        expect(queryByText('READ VIEW')).toBeNull()
    })

    it('renders the read view while idle', () => {
        const { getByText, queryByText } = render(<Probe startOpen={false} />)

        expect(getByText('READ VIEW')).toBeTruthy()
        expect(queryByText('EDITOR')).toBeNull()
    })

    /**
     * The boot window. A surface that opens already-editing (a composer) can
     * acquire before the singleton has finished booting; it holds the lease but
     * has no usable editor, so it shows its read view and swaps in the editor
     * once ready — never a dead box.
     */
    it('renders the read view while a held editor is still booting', () => {
        leaseState = { result: null, ready: false }
        const { getByText, queryByText, rerender } = render(<Probe />)

        expect(getByText('READ VIEW')).toBeTruthy()
        expect(queryByText('EDITOR')).toBeNull()

        // Boot finishes.
        leaseState = { result: heldResult, ready: true }
        rerender(<Probe />)

        expect(getByText('EDITOR')).toBeTruthy()
    })

    /**
     * The steal. Another surface takes the instance, so this one stops holding
     * it — and must fall back to its read view rather than closing or going
     * blank. Per #193 a startOpen surface stays OPEN; since a non-holder renders
     * readView either way, that affects only whether a tap re-acquires.
     */
    it('renders the read view after being displaced by another surface', () => {
        const { getByText, queryByText, rerender } = render(<Probe />)
        expect(getByText('EDITOR')).toBeTruthy()

        leaseState = { result: null, ready: true }
        rerender(<Probe />)

        expect(getByText('READ VIEW')).toBeTruthy()
        expect(queryByText('EDITOR')).toBeNull()
    })

    /**
     * The data-loss guard. Reading a nonexistent editor would resolve to '' and
     * blank the user's content, so a submit with nothing held must write
     * nothing at all.
     */
    it('writes nothing when submitting while holding no editor', async () => {
        const onCommit = vi.fn()
        // Held and ready first, so renderEditor runs and hands out `submit`.
        const { rerender } = render(<Probe onCommit={onCommit} />)
        expect(submitFn).not.toBeNull()

        // A dialog's Save can outlive the surface that opened it, so the press
        // reaches a session whose editor has since been handed away.
        const heldSubmit = submitFn
        leaseState = { result: null, ready: true }
        rerender(<Probe onCommit={onCommit} />)

        await act(async () => heldSubmit?.())

        // Reading a nonexistent editor resolves to '', and committing that
        // would blank the record. The persisted value must survive untouched.
        expect(onCommit).not.toHaveBeenCalled()
    })
})
