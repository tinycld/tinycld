// @vitest-environment happy-dom
import { act, cleanup, render } from '@testing-library/react'
import { isValidElement } from 'react'
import { Text, View } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UseRichEditorOptions } from '../../../lib/editor/rich/options'

/**
 * Blur does not END a session — whatever took the focus.
 *
 * A dialog the editor opened is the motivating case: the editor has to survive
 * until the picked image or link lands in it. It used to need telling, through
 * a `setDialogOpen` slot and an `isDialogOpen` prop; now nothing does, because
 * losing focus no longer closes anything.
 *
 * Whether a blur WRITES is a separate question, and the surface opts into that
 * with `saveOnBlur` — see lazy-editor-save-on-blur.test.tsx. These cases all
 * run without it, so they assert the lifecycle alone.
 */

// Captures the options LazyEditor hands the editor, so the test can fire the
// blur the same way the real editor would.
let captured: UseRichEditorOptions | null = null

// A HELD lease: there is one editor app-wide and these cases are all about a
// surface that has it. A lease reporting no editor would render the read view,
// so there would be no session for a dialog to keep alive.
vi.mock('../../../lib/editor/warm', () => ({
    useWarmEditor: (_surfaceId: string, options: UseRichEditorOptions) => {
        captured = options
        return {
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
                    focus: vi.fn(),
                    clear: vi.fn(),
                    getSelection: vi.fn(async () => null),
                },
                EditorComponent: () => null,
                commands: {},
                toolbarState: {},
            },
            generation: 1,
        }
    },
}))

const { useLazyEditor } = await import('../LazyEditor')

let startEditing: (() => void) | null = null
let isEditing = false

function Probe() {
    const slots = useLazyEditor({
        readView: <Text>read view</Text>,
        value: 'persisted',
        contentFormat: 'markdown',
        editorOptions: {},
        surfaceId: 'description:card1',
        canEdit: true,
        onCommit: () => {},
        renderEditor: () => <Text>editing surface</Text>,
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
    isEditing = false
    vi.clearAllMocks()
})

afterEach(cleanup)

describe('LazyEditor blur handling', () => {
    /**
     * Blur is not an exit.
     *
     * It used to end the session — and for an edit of existing content, WRITE
     * it — which made every focusable control a hazard: a toolbar's overflow
     * menu, a dialog, anything portalled took focus and finished the edit
     * behind the user's back. A session now ends only when another surface
     * takes the editor, on Escape, or when the caller closes it.
     *
     * The three dialog cases that used to live here are gone with the
     * behaviour: `isDialogOpen` existed to stop a dialog's focus grab ending a
     * session, and nothing needs stopping any more.
     */
    it('keeps the session open when the editor loses focus', () => {
        render(<Probe />)
        act(() => startEditing?.())
        expect(isEditing).toBe(true)

        focusThenBlur()

        expect(isEditing).toBe(true)
    })

    /**
     * Including under a dialog. `isDialogOpen` used to exist for exactly this —
     * an image picker or link prompt takes focus, and that blur would have
     * ended the session under it. Nothing needs telling any more.
     */
    it('keeps it open when a dialog takes the focus', () => {
        render(<Probe />)
        act(() => startEditing?.())

        focusThenBlur()

        expect(isEditing).toBe(true)
    })
})
