// @vitest-environment happy-dom

import { cleanup, fireEvent, render } from '@testing-library/react'
import { MentionInput, type MentionSuggestion } from '@tinycld/core/ui/comments'
import { useForm } from '@tinycld/core/ui/form'
import { afterEach, describe, expect, it, vi } from 'vitest'

// MentionInput detects the `@…` trigger and has to hand the query it is
// typing to its owner, because the candidate pool is a server-side
// search that only the owning package can run. These cover that
// reporting contract: the query as typed, null when no trigger is
// active, and null again once a pick has closed the trigger.
//
// The RN stub renders a TextInput as a custom element that answers a
// DOM `input` event with onChangeText — so typing here is what typing
// is in the component. It does NOT fire onSelectionChange, which is
// exactly the platform-ordering case the component covers by deriving
// the trigger from the new text in onChangeText as well.

const SUGGESTIONS: MentionSuggestion[] = [
    { userId: 'user_ali', displayName: 'Alice', secondary: 'alice@example.com' },
]

function Harness({ onQueryChange }: { onQueryChange: (query: string | null) => void }) {
    const { control } = useForm<{ body: string }>({ defaultValues: { body: '' } })
    return (
        <MentionInput
            control={control}
            name="body"
            suggestions={SUGGESTIONS}
            onQueryChange={onQueryChange}
        />
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

    it('does not filter the supplied candidates — the search already did', () => {
        const onQueryChange = vi.fn()
        // A row the server matched on its email, which the query text
        // does not prefix. A client-side re-filter would drop it.
        const { container, getByText } = render(<Harness onQueryChange={onQueryChange} />)

        type(container, 'hi @zz')
        expect(getByText('Alice')).not.toBeNull()
    })
})
