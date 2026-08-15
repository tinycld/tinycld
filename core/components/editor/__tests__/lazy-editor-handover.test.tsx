// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { isValidElement } from 'react'
import { Text, View } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WarmEditorLease } from '../../../lib/editor/warm/types'

/**
 * The mid-submit handover.
 *
 * Every surface in the app shares one editor, and reading its content is async.
 * If another surface acquires the instance during that read, the bytes that
 * resolve belong to the INCOMING surface — writing them through onCommit puts
 * one card's text onto another card's record.
 *
 * This is the shape the handoff doc warned about, and it was reachable: the
 * settled flag was set after the await rather than before it, so the guard at
 * the top of submit never fired for the second caller.
 *
 * Driven through a mocked lease rather than a real editor — the race is in
 * LazyEditor's bookkeeping, and a booted editor would only make it harder to
 * schedule deterministically.
 */

let generation = 1
let resolveRead: ((markdown: string) => void) | null = null

const warmEditor = {
    getHTML: vi.fn(async () => '<p>warm</p>'),
    getText: vi.fn(async () => 'warm'),
    getMarkdown: vi.fn(
        () =>
            new Promise<string>(resolve => {
                resolveRead = resolve
            })
    ),
    setContent: vi.fn(),
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
    holder: 'comment:a',
    acquire: vi.fn(),
    release: vi.fn(),
    result: warmResult as never,
    get generation() {
        return generation
    },
}

vi.mock('../../../lib/editor/warm', () => ({
    useWarmEditor: () => lease,
}))

const { useLazyEditor } = await import('../LazyEditor')

let startEditing: (() => void) | null = null
let submit: (() => void) | null = null

function Probe({ onCommit }: { onCommit: (content: string) => void }) {
    const slots = useLazyEditor({
        readView: <Text>read view</Text>,
        value: 'persisted',
        contentFormat: 'markdown',
        editorOptions: {},
        surfaceId: 'comment:a',
        canEdit: true,
        onCommit,
        renderEditor: s => {
            submit = s.submit
            return <Text>editing surface</Text>
        },
        testID: 'probe',
    })
    startEditing =
        (isValidElement<{ onPress?: () => void }>(slots.body) ? slots.body.props.onPress : null) ??
        null
    return <View>{slots.body}</View>
}

beforeEach(() => {
    generation = 1
    resolveRead = null
    vi.clearAllMocks()
})

afterEach(cleanup)

describe('LazyEditor handover safety', () => {
    it('commits when no handover happened during the read', async () => {
        const onCommit = vi.fn()
        render(<Probe onCommit={onCommit} />)

        act(() => startEditing?.())
        act(() => submit?.())

        await act(async () => {
            resolveRead?.('the text I typed')
        })

        expect(onCommit).toHaveBeenCalledWith('the text I typed')
    })

    /**
     * The data-loss case. The editor is handed to another surface while this
     * one's read is in flight, so the resolved content is not this surface's
     * text. Committing it would overwrite this record with another's.
     */
    it('drops the read when the editor was handed over mid-submit', async () => {
        const onCommit = vi.fn()
        render(<Probe onCommit={onCommit} />)

        act(() => startEditing?.())
        act(() => submit?.())

        // Another surface takes the instance while the read is pending.
        generation = 2

        await act(async () => {
            resolveRead?.("the OTHER surface's text")
        })

        expect(onCommit).not.toHaveBeenCalled()
    })

    /**
     * Settling before the await also makes a second submit a no-op, so a
     * double-tap on Save cannot produce two writes.
     */
    it('ignores a second submit while the first read is in flight', async () => {
        const onCommit = vi.fn()
        render(<Probe onCommit={onCommit} />)

        act(() => startEditing?.())
        act(() => submit?.())
        act(() => submit?.())

        await act(async () => {
            resolveRead?.('typed once')
        })

        expect(onCommit).toHaveBeenCalledTimes(1)
        expect(warmEditor.getMarkdown).toHaveBeenCalledTimes(1)
    })
})
