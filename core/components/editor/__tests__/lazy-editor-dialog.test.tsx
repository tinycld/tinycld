// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { isValidElement } from 'react'
import { Text, View } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UseRichEditorOptions } from '../../../lib/editor/rich/options'

/**
 * A blur while a dialog the editor opened holds the focus must NOT end the
 * session — the editor has to survive until the picked image or link lands in
 * it.
 *
 * There are two ways for a caller to say a dialog is open. `slots.setDialogOpen`
 * suits chrome that learns about it while rendering; the `isDialogOpen` prop
 * suits a caller that already owns the state, which otherwise has to push it
 * back up through a useEffect — the useState+useEffect pairing the style guide
 * names as the signal to switch primitives.
 */

// Captures the options LazyEditor hands the editor, so the test can fire the
// blur the same way the real editor would.
let captured: UseRichEditorOptions | null = null

vi.mock('../../../lib/editor/warm', () => ({
    useWarmEditor: () => ({
        isWarm: false,
        acquire: vi.fn(),
        release: vi.fn(),
        result: null,
        generation: 0,
    }),
}))

vi.mock('../../../lib/editor/rich', () => ({
    useRichEditor: (options: UseRichEditorOptions) => {
        captured = options
        return {
            editor: {
                getHTML: vi.fn(async () => '<p>x</p>'),
                getText: vi.fn(async () => 'x'),
                getMarkdown: vi.fn(async () => 'typed'),
                setContent: vi.fn(),
                focus: vi.fn(),
                clear: vi.fn(),
                getSelection: vi.fn(async () => null),
            },
            EditorComponent: () => null,
            commands: {},
            toolbarState: {},
        }
    },
}))

const { useLazyEditor } = await import('../LazyEditor')

let startEditing: (() => void) | null = null
let setDialogOpen: ((open: boolean) => void) | null = null
let isEditing = false

function Probe({ isDialogOpen }: { isDialogOpen?: boolean }) {
    const slots = useLazyEditor({
        readView: <Text>read view</Text>,
        value: 'persisted',
        contentFormat: 'markdown',
        editorOptions: {},
        surfaceId: 'description:card1',
        canEdit: true,
        isDialogOpen,
        onCommit: () => {},
        renderEditor: s => {
            setDialogOpen = s.setDialogOpen
            return <Text>editing surface</Text>
        },
        renderHeader: state => {
            isEditing = state.isEditing
            return null
        },
        testID: 'probe',
    })
    startEditing =
        (isValidElement<{ onPress?: () => void }>(slots.body) ? slots.body.props.onPress : null) ??
        null
    return <View>{slots.body}</View>
}

/** Focus then blur, the way the real editor drives the handlers. */
function focusThenBlur() {
    act(() => captured?.onFocus?.())
    act(() => captured?.onBlur?.())
}

beforeEach(() => {
    captured = null
    startEditing = null
    setDialogOpen = null
    isEditing = false
    vi.clearAllMocks()
})

afterEach(cleanup)

describe('LazyEditor dialog handling', () => {
    it('ends the session on blur when no dialog is open', () => {
        render(<Probe />)
        act(() => startEditing?.())
        expect(isEditing).toBe(true)

        focusThenBlur()

        expect(isEditing).toBe(false)
    })

    it('keeps the session alive on blur while the isDialogOpen prop is true', () => {
        render(<Probe isDialogOpen />)
        act(() => startEditing?.())

        focusThenBlur()

        expect(isEditing).toBe(true)
    })

    it('keeps the session alive on blur after slots.setDialogOpen(true)', () => {
        render(<Probe />)
        act(() => startEditing?.())
        act(() => setDialogOpen?.(true))

        focusThenBlur()

        expect(isEditing).toBe(true)
    })

    // The prop is authoritative when supplied, so a caller that owns the state
    // cannot be contradicted by a stale setDialogOpen call.
    it('lets the prop win over setDialogOpen', () => {
        render(<Probe isDialogOpen />)
        act(() => startEditing?.())
        act(() => setDialogOpen?.(false))

        focusThenBlur()

        expect(isEditing).toBe(true)
    })
})
