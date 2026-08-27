// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { isValidElement } from 'react'
import { Text, View } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

// A held lease. There is one editor app-wide, and a surface that does not hold
// it renders its read view — so a test about the EDITING slots has to be the
// holder, or there is nothing to render.
const focusSpy = vi.fn()

vi.mock('../../../lib/editor/warm', () => ({
    useWarmEditor: () => ({
        isWarm: true,
        ready: true,
        acquire: vi.fn(),
        release: vi.fn(),
        result: {
            editor: {
                getHTML: vi.fn(async () => '<p>x</p>'),
                getText: vi.fn(async () => 'x'),
                getMarkdown: vi.fn(async () => 'typed'),
                setContent: vi.fn(),
                focus: focusSpy,
                clear: vi.fn(),
                getSelection: vi.fn(async () => null),
            },
            EditorComponent: () => null,
            commands: {},
            toolbarState: {},
        },
        generation: 1,
    }),
}))

const { useLazyEditor } = await import('../LazyEditor')

/**
 * Some surfaces cannot take the editor as one tree. A card description draws its
 * toolbar into a row that must stay a DIRECT child of the scroll view for
 * `stickyHeaderIndices` to pin it, so the header and the editing surface are
 * placed separately by the caller.
 *
 * The hook therefore returns slots. Both are still the consumer's to render —
 * core owns the swap and the commit rules, never the chrome.
 */
/**
 * The react-native stub renders Pressable as a bare host element, so `onPress`
 * is never bound to a DOM event and a click cannot reach it. The Probe hands its
 * press handler out here instead, which is what lets a test open a session.
 */
let startEditing: ((event?: unknown) => void) | null = null

function Probe({ canEdit = true }: { canEdit?: boolean }) {
    const slots = useLazyEditor({
        readView: <Text>read view</Text>,
        value: 'persisted',
        contentFormat: 'markdown',
        editorOptions: {},
        surfaceId: 'description:card1',
        canEdit,
        onCommit: () => {},
        renderHeader: ({ isEditing }) => <Text>{isEditing ? 'toolbar' : 'label'}</Text>,
        renderEditor: () => <Text>editing surface</Text>,
        testID: 'probe-press',
    })
    // The read view's press target is the Pressable in the body slot; its
    // onPress is what the swap hangs off.
    startEditing =
        (isValidElement<{ onPress?: (event?: unknown) => void }>(slots.body)
            ? slots.body.props.onPress
            : null) ?? null

    return (
        <View>
            {slots.header}
            {slots.body}
        </View>
    )
}

describe('useLazyEditor slots', () => {
    afterEach(cleanup)

    it('renders the read view and the idle header while nobody is editing', () => {
        const { getByText } = render(<Probe />)
        expect(getByText('read view')).toBeTruthy()
        expect(getByText('label')).toBeTruthy()
    })

    /**
     * The header has to follow the swap. It is a separate slot, so nothing else
     * would tell it an edit had started — and on a card that row is where the
     * formatting toolbar replaces the section label.
     */
    it('swaps both slots together when editing starts', () => {
        const { getByText } = render(<Probe />)
        // The react-native stub renders Pressable as a bare host element, so
        // onPress is never bound to a DOM event and a click cannot reach it.
        // Starting the session directly is what the harness allows.
        act(() => startEditing?.())
        expect(getByText('editing surface')).toBeTruthy()
        expect(getByText('toolbar')).toBeTruthy()
    })

    it('offers no press target when the surface is read-only', () => {
        const { queryByTestId, getByText } = render(<Probe canEdit={false} />)
        expect(queryByTestId('probe-press')).toBeNull()
        expect(getByText('read view')).toBeTruthy()
    })

    /**
     * The read view and the editing surface occupy the same box, so the point
     * someone pressed on the prose is where the caret belongs. Without this the
     * caret landed at the very end of the document and a reader who clicked
     * into the middle of a paragraph had to click again to get there.
     */
    it('puts the caret where the read view was pressed', async () => {
        focusSpy.mockClear()
        render(<Probe />)
        act(() => startEditing?.({ nativeEvent: { pageX: 120, pageY: 240 } }))
        // The focus is deferred to a microtask: the editor is reconfigured
        // during the same commit, and focusing before it has applied the new
        // content moves the caret in the OUTGOING surface's document.
        await act(async () => {})
        expect(focusSpy).toHaveBeenCalledWith({ x: 120, y: 240 })
    })

    /**
     * A press does not always carry coordinates — an accessibility activation
     * and a keyboard-triggered press both arrive without them, and RN calls
     * onPress with no event at all in some paths. Each must still open a
     * session, with the caret at the end.
     */
    it.each([
        ['no event at all', undefined],
        ['an event with no coordinates', { nativeEvent: {} }],
    ])('falls back to the end of the document given %s', async (_label, event) => {
        focusSpy.mockClear()
        render(<Probe />)
        act(() => startEditing?.(event))
        await act(async () => {})
        expect(focusSpy).toHaveBeenCalledWith('end')
    })
})
