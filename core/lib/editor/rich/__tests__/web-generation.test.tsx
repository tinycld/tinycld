// core/lib/editor/rich/__tests__/web-generation.test.tsx
// @vitest-environment happy-dom
import { render, waitFor } from '@testing-library/react'
import { useEffect } from 'react'
import { expect, test } from 'vitest'
import type { EditorResult } from '../../types'
import { useRichEditor } from '../use-rich-editor.web'

/**
 * Exercises the one thing a handover depends on: bumping `generation` must
 * REBUILD the web editor, not mutate it. Native has always done this by keying
 * its WebView mount on the same value; before this the web hook ignored the
 * option entirely, so a surface that took the shared instance inherited the
 * previous surface's document, undo stack and selection.
 */

/** A holder object, so the captured result stays a live reference TS can type. */
interface Captured {
    result: EditorResult | null
}

function Harness({
    content,
    generation,
    captured,
}: {
    content: string
    generation: number
    captured: Captured
}) {
    const result = useRichEditor({ initialContent: content, generation })
    // Published through an effect rather than during render: the test reads it
    // to drive assertions, and writing to it mid-render is a side effect.
    useEffect(() => {
        captured.result = result
    }, [result, captured])
    return <result.EditorComponent />
}

test('bumping generation rebuilds the editor with the new content', async () => {
    const captured: Captured = { result: null }

    const { rerender } = render(
        <Harness content="<p>alpha</p>" generation={0} captured={captured} />
    )
    await waitFor(() => expect(captured.result?.isReady).toBe(true))
    await waitFor(async () => expect(await captured.result?.editor.getText()).toContain('alpha'))

    rerender(<Harness content="<p>beta</p>" generation={1} captured={captured} />)

    await waitFor(async () => {
        const text = await captured.result?.editor.getText()
        expect(text).toContain('beta')
        expect(text).not.toContain('alpha')
    })
})

test('undo after a generation bump cannot reach the previous surface content', async () => {
    const captured: Captured = { result: null }

    const { rerender } = render(
        <Harness content="<p>alpha</p>" generation={0} captured={captured} />
    )
    await waitFor(() => expect(captured.result?.isReady).toBe(true))

    rerender(<Harness content="<p>beta</p>" generation={1} captured={captured} />)
    await waitFor(async () => expect(await captured.result?.editor.getText()).toContain('beta'))

    // A rebuilt editor has a fresh history plugin, so there is nothing to undo
    // back into. A MUTATED editor would still hold alpha in its undo stack —
    // one surface's text reachable from another's, which is the leak this
    // guards against.
    captured.result?.commands.undo()

    expect(await captured.result?.editor.getText()).not.toContain('alpha')
})
