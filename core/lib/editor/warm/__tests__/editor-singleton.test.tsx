// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { type ReactNode, useEffect } from 'react'
import { Text } from 'react-native'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * The singleton's four guarantees, all of which the previous per-section host
 * could not make:
 *
 *  - nothing is constructed before a package declares need;
 *  - one declaration boots exactly ONE editor;
 *  - a second package declaring does not boot a second;
 *  - the editor survives a subtree unmounting — it is never disposed.
 *
 * `useRichEditor` is mocked and counted rather than booted. What is under test
 * is the lifecycle around it, and a real editor would make "how many were
 * built" unobservable.
 */

const editorConstructions = vi.fn()

vi.mock('../../rich', () => ({
    // A plain function, not a hook: the real `useRichEditor` builds its editor
    // once per mount, so a module-level count of CALLS with a fresh identity
    // would follow the render loop instead. Counting mounts is what the tests
    // below actually assert, and an effect keyed on nothing reports exactly
    // that — one call per mounted instance, no matter how often it re-renders.
    useRichEditor: (options: { generation?: number }) => {
        useCountMount()
        return {
            // A parked editor is cleared, so this must exist. Reached through a
            // function because vi.mock factories hoist above the declaration.
            editor: { setContent: (content: string) => setContent(content) },
            EditorComponent: () => null,
            commands: {},
            toolbarState: {},
            isReady: true,
            generation: options.generation,
        }
    },
}))

const setContent = vi.fn()

/** One call per mounted editor — a re-render is not a construction. */
function useCountMount() {
    useEffect(() => {
        editorConstructions()
    }, [])
}

const { EditorSingletonProvider, useEditorSingleton } = await import('../editor-singleton')
const { useEditorNeeded } = await import('../use-editor-needed')

function Declarer() {
    useEditorNeeded()
    return null
}

/** Reports whether the singleton currently holds a usable editor. */
function Probe() {
    const singleton = useEditorSingleton()
    return <Text>{singleton?.result ? 'has-editor' : 'no-editor'}</Text>
}

function Wrapper({ children }: { children?: ReactNode }) {
    return <EditorSingletonProvider>{children}</EditorSingletonProvider>
}

beforeEach(() => {
    editorConstructions.mockClear()
    setContent.mockClear()
})
afterEach(cleanup)

describe('the editor singleton', () => {
    it('constructs nothing until a package declares need', () => {
        const { getByText } = render(
            <Wrapper>
                <Probe />
            </Wrapper>
        )

        expect(editorConstructions).not.toHaveBeenCalled()
        expect(getByText('no-editor')).toBeTruthy()
    })

    it('boots exactly one editor on the first declaration', () => {
        const { getByText } = render(
            <Wrapper>
                <Declarer />
                <Probe />
            </Wrapper>
        )

        expect(editorConstructions).toHaveBeenCalledOnce()
        expect(getByText('has-editor')).toBeTruthy()
    })

    it('does not boot a second editor when another package also declares', () => {
        render(
            <Wrapper>
                <Declarer />
                <Declarer />
                <Probe />
            </Wrapper>
        )

        expect(editorConstructions).toHaveBeenCalledOnce()
    })

    /**
     * The "never disposed" guarantee. The declaring section unmounts — the user
     * left Cards — and the editor must still be there when they come back. The
     * per-section host this replaces re-booted here, paying the full cold start
     * again on every re-entry.
     */
    it('keeps the editor when the declaring subtree unmounts', () => {
        const { getByText, rerender } = render(
            <Wrapper>
                <Declarer />
                <Probe />
            </Wrapper>
        )
        expect(editorConstructions).toHaveBeenCalledOnce()

        // The user leaves Cards: the section — and its declaration — is gone.
        rerender(
            <Wrapper>
                <Probe />
            </Wrapper>
        )

        expect(getByText('has-editor')).toBeTruthy()
        expect(editorConstructions).toHaveBeenCalledOnce()
    })

    /**
     * Re-entering the section must not rebuild it either. This is the win the
     * per-section host could not deliver: it re-mounted with the layout, so
     * every return to Cards paid the full boot again.
     */
    it('re-declaring after leaving the section reuses the same editor', () => {
        const { rerender } = render(
            <Wrapper>
                <Declarer />
            </Wrapper>
        )
        rerender(<Wrapper />)
        rerender(
            <Wrapper>
                <Declarer />
            </Wrapper>
        )

        expect(editorConstructions).toHaveBeenCalledOnce()
    })

    /**
     * A parked editor holds nobody's text.
     *
     * It stays mounted off-viewport so it never re-pays the boot, which means
     * whatever it last contained is still in the document. A released comment
     * editor left that comment's own words sitting there, where anything reading
     * the page by text found them twice.
     */
    it('clears the editor when it parks', () => {
        render(
            <Wrapper>
                <Declarer />
            </Wrapper>
        )

        expect(setContent).toHaveBeenCalledWith('')
    })

    it('publishes readiness through the store, so a lease can gate on it', () => {
        function ReadyProbe() {
            const singleton = useEditorSingleton()
            const isReady = singleton?.store.isReady() ?? false
            return <Text>{isReady ? 'ready' : 'booting'}</Text>
        }

        const { getByText } = render(
            <Wrapper>
                <Declarer />
                <ReadyProbe />
            </Wrapper>
        )

        expect(getByText('ready')).toBeTruthy()
    })
})
