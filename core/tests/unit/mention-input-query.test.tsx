// @vitest-environment happy-dom

import { cleanup, fireEvent, render } from '@testing-library/react'
import { MentionInput, type MentionSuggestion } from '@tinycld/core/ui/comments'
import { useForm } from '@tinycld/core/ui/form'
import { useWatch } from 'react-hook-form'
import { Pressable, Text } from 'react-native'
import { afterEach, describe, expect, it, vi } from 'vitest'

// MentionInput detects the `@…` trigger and has to hand the query it is
// typing to its owner, because the candidate pool is a server-side
// search that only the owning package can run. These cover that
// reporting contract — the query as typed, null when no trigger is
// active, null again once a pick has closed it — and the caret
// arithmetic the trigger depends on.
//
// The RN stub renders a TextInput as a custom element that answers a DOM
// `input` event with onChangeText and an `rn-selectionchange` event with
// onSelectionChange, so `type` and `moveCaret` below are what typing and
// moving the caret are in the component. Neither is synthesized for the
// other: a real TextInput's ordering between the two differs per
// platform, and the component must not depend on it.

const SUGGESTIONS: MentionSuggestion[] = [
    { userId: 'user_ali', displayName: 'Alice', secondary: 'alice@example.com' },
]

// Mirrors the form value into the DOM so a test can assert on the body
// the component actually produced, which is where a bad splice shows up.
function BodyProbe({ control }: { control: ReturnType<typeof useForm<Body>>['control'] }) {
    const body = useWatch({ control, name: 'body' })
    return <Text testID="body-probe">{`[${body ?? ''}]`}</Text>
}

interface Body {
    body: string
}

function Harness({
    onQueryChange,
    defaultBody = '',
}: {
    onQueryChange?: (query: string | null) => void
    defaultBody?: string
}) {
    const { control } = useForm<Body>({ defaultValues: { body: defaultBody } })
    return (
        <>
            <MentionInput
                control={control}
                name="body"
                suggestions={SUGGESTIONS}
                onQueryChange={onQueryChange}
            />
            <BodyProbe control={control} />
        </>
    )
}

// A harness with a reset button, so a test can move the value from
// outside the input the way a submit handler does.
function ResettableHarness() {
    const { control, reset } = useForm<Body>({ defaultValues: { body: '' } })
    return (
        <>
            <MentionInput control={control} name="body" suggestions={SUGGESTIONS} />
            <Pressable accessibilityRole="button" onPress={() => reset({ body: '' })}>
                <Text>reset</Text>
            </Pressable>
            <BodyProbe control={control} />
        </>
    )
}

function field(container: HTMLElement): Element {
    const el = container.querySelector('rn-textinput')
    if (!el) throw new Error('no text input rendered')
    return el
}

function type(container: HTMLElement, value: string) {
    fireEvent.input(field(container), { target: { value } })
}

function moveCaret(container: HTMLElement, start: number) {
    fireEvent(
        field(container),
        new CustomEvent('rn-selectionchange', { detail: { start, end: start } })
    )
}

function body(container: HTMLElement): string {
    const probe = container.querySelector('[testid="body-probe"]')
    const text = probe?.textContent ?? ''
    return text.slice(1, -1)
}

describe('MentionInput onQueryChange', () => {
    afterEach(cleanup)

    it('reports the text typed after @ as the user types it', () => {
        const onQueryChange = vi.fn()
        const { container } = render(<Harness onQueryChange={onQueryChange} />)

        type(container, 'hi @')
        expect(onQueryChange).toHaveBeenLastCalledWith('')

        type(container, 'hi @al')
        expect(onQueryChange).toHaveBeenLastCalledWith('al')

        type(container, 'hi @ali')
        expect(onQueryChange).toHaveBeenLastCalledWith('ali')
    })

    it('reports null when no trigger is active', () => {
        const onQueryChange = vi.fn()
        const { container } = render(<Harness onQueryChange={onQueryChange} />)

        type(container, 'plain text')
        expect(onQueryChange).toHaveBeenLastCalledWith(null)

        // A space after the query ends the mention — the user typed past it.
        type(container, 'hi @ali ')
        expect(onQueryChange).toHaveBeenLastCalledWith(null)

        // An email's host must not open the picker.
        type(container, 'mail me at ali@example')
        expect(onQueryChange).toHaveBeenLastCalledWith(null)
    })

    it('renders the supplied candidates while a trigger is active and closes on pick', () => {
        const onQueryChange = vi.fn()
        const { container, queryByText, getByText } = render(
            <Harness onQueryChange={onQueryChange} />
        )

        expect(queryByText('Alice')).toBeNull()

        type(container, 'hi @al')
        expect(getByText('Alice')).not.toBeNull()

        fireEvent.click(getByText('Alice'))
        expect(onQueryChange).toHaveBeenLastCalledWith(null)
        expect(queryByText('Alice')).toBeNull()
    })

    it('inserts the wire token in place of the typed query', () => {
        const { container, getByText } = render(<Harness />)

        type(container, 'hi @al')
        fireEvent.click(getByText('Alice'))
        expect(body(container)).toBe('hi [[@user_ali]] ')
    })

    it('does not filter the supplied candidates — the search already did', () => {
        // A row the server matched on its email, which the query text
        // does not prefix. A client-side re-filter would drop it.
        const { container, getByText } = render(<Harness />)

        type(container, 'hi @zz')
        expect(getByText('Alice')).not.toBeNull()
    })

    // An edit away from the end of the body must not be read as one at
    // the end. Assuming an end-of-text caret finds the `@bo` further
    // along and opens the picker on it, and a pick then splices between
    // stale offsets and eats the text in between.
    describe('an edit in the middle of the body', () => {
        it('opens no trigger for an @ the caret is nowhere near', () => {
            const onQueryChange = vi.fn()
            const { container, queryByText } = render(<Harness onQueryChange={onQueryChange} />)

            type(container, 'hello @bo')
            expect(onQueryChange).toHaveBeenLastCalledWith('bo')

            // Put the caret inside "hello" and insert there. The body
            // still ends in "@bo", but the caret does not.
            moveCaret(container, 2)
            type(container, 'heXllo @bo')

            expect(onQueryChange).toHaveBeenLastCalledWith(null)
            expect(queryByText('Alice')).toBeNull()
        })

        it('closes the popover rather than splicing on a mid-body edit', () => {
            const { container, getByText, queryByText } = render(<Harness />)

            // Open a trigger at the end, then edit mid-body. The caret
            // arithmetic closes the popover, so no row survives for a
            // pick to splice with — which is the real protection. The
            // body is exactly what was typed, with nothing eaten.
            type(container, 'hello @bo')
            expect(getByText('Alice')).not.toBeNull()

            moveCaret(container, 2)
            type(container, 'heXllo @bo')

            expect(queryByText('Alice')).toBeNull()
            expect(body(container)).toBe('heXllo @bo')
        })

        // The onPick guard is defence in depth: with the caret carried
        // correctly, the popover is already closed in every case above,
        // so nothing normally reaches a stale splice. What can still
        // outrun it is the value changing from OUTSIDE the input — a
        // form reset on submit, which leaves the trigger describing a
        // body that no longer exists.
        it('rejects a pick whose trigger no longer matches the body', () => {
            const { container, getByText } = render(<ResettableHarness />)

            type(container, 'hello @al')
            expect(getByText('Alice')).not.toBeNull()

            // Clear the field the way a submit does, without an input or
            // selection event. The trigger still says atIndex 6 / caret 9.
            fireEvent.click(getByText('reset'))

            // Splicing on those offsets against "" would produce a body
            // of "[[@user_ali]] " out of nowhere. The guard must bail.
            fireEvent.click(getByText('Alice'))
            expect(body(container)).toBe('')
        })

        it('keeps the trigger usable when the edit IS at the mention', () => {
            const onQueryChange = vi.fn()
            const { container, getByText } = render(<Harness onQueryChange={onQueryChange} />)

            type(container, 'hello @al')
            moveCaret(container, 9)
            type(container, 'hello @ali')

            expect(onQueryChange).toHaveBeenLastCalledWith('ali')
            fireEvent.click(getByText('Alice'))
            expect(body(container)).toBe('hello [[@user_ali]] ')
        })
    })

    it('reports the trigger for the caret it is given, not the end of the text', () => {
        const onQueryChange = vi.fn()
        const { container } = render(<Harness onQueryChange={onQueryChange} />)

        type(container, 'hi @al and more text after')
        // The trailing text means the end-of-text caret sees no trigger.
        expect(onQueryChange).toHaveBeenLastCalledWith(null)

        // Move the caret back to just after "@al" — now there is one.
        moveCaret(container, 6)
        expect(onQueryChange).toHaveBeenLastCalledWith('al')
    })
})
