// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { isValidElement } from 'react'
import { Text, View } from 'react-native'
import { afterEach, describe, expect, it } from 'vitest'
import { useLazyEditor } from '../LazyEditor'

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
let startEditing: (() => void) | null = null

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
        (isValidElement<{ onPress?: () => void }>(slots.body) ? slots.body.props.onPress : null) ??
        null

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
})
