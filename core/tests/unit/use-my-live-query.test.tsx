// @vitest-environment happy-dom
import { and, createCollection, eq, localOnlyCollectionOptions } from '@tanstack/db'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, type MockInstance, vi } from 'vitest'

// useMyLiveQuery is the path every package takes to "my own rows". It moved
// off react-db's deprecated deps-array form (removed in react-db 1.0) onto the
// object form, where the query's identity comes from the built query itself.
// These tests pin the three things that move must not break: no query runs
// with a blank user id, a changed captured value re-runs the query, and the
// deprecation warning is gone.

const h = vi.hoisted(() => ({ user: null as { id: string } | null }))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: h.user }),
}))

import { useMyLiveQuery } from '../../lib/use-my-live-query'

type Note = { id: string; owner: string; tag: string }

const notes = createCollection(
    localOnlyCollectionOptions({
        id: 'use-my-live-query-notes',
        getKey: (r: Note) => r.id,
        initialData: [
            { id: 'n1', owner: 'u1', tag: 'work' },
            { id: 'n2', owner: 'u1', tag: 'home' },
            { id: 'n3', owner: 'u2', tag: 'work' },
            { id: 'n4', owner: '', tag: 'work' },
        ],
    })
)

function useNotes(tag: string) {
    return useMyLiveQuery((q, { userId }) =>
        q.from({ note: notes }).where(({ note }) => and(eq(note.owner, userId), eq(note.tag, tag)))
    )
}

function ids(rows: Note[] | undefined) {
    return (rows ?? []).map(r => r.id).sort()
}

let warn: MockInstance<typeof console.warn>

beforeEach(() => {
    h.user = null
    warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
})

// react-db warns once per call site per process, so only the first render
// through the hook would ever show the warning. Checking after every test,
// rather than in one dedicated test, is what makes that first render count.
afterEach(() => {
    cleanup()
    const deprecations = warn.mock.calls.filter(args => String(args[0]).includes('deprecated'))
    warn.mockRestore()
    expect(deprecations).toEqual([])
})

describe('useMyLiveQuery', () => {
    it('stays disabled until the user id is known, then runs', async () => {
        const { result, rerender } = renderHook(() => useNotes('work'))

        expect(result.current.isEnabled).toBe(false)
        expect(result.current.data).toBeUndefined()

        h.user = { id: 'u1' }
        rerender()

        await waitFor(() => expect(result.current.isReady).toBe(true))
        expect(result.current.isEnabled).toBe(true)
        // A blank id would have matched n4 (owner ''); only u1's row may come back.
        expect(ids(result.current.data)).toEqual(['n1'])
    })

    it('never calls the query function without a user', () => {
        const queryFn = vi.fn()
        renderHook(() => useMyLiveQuery(queryFn))
        expect(queryFn).not.toHaveBeenCalled()
    })

    it('re-runs when a captured value changes', async () => {
        h.user = { id: 'u1' }
        const { result, rerender } = renderHook(({ tag }) => useNotes(tag), {
            initialProps: { tag: 'work' },
        })
        await waitFor(() => expect(ids(result.current.data)).toEqual(['n1']))

        rerender({ tag: 'home' })
        await waitFor(() => expect(ids(result.current.data)).toEqual(['n2']))
    })

    it('re-runs when the user changes', async () => {
        h.user = { id: 'u1' }
        const { result, rerender } = renderHook(() => useNotes('work'))
        await waitFor(() => expect(ids(result.current.data)).toEqual(['n1']))

        act(() => {
            h.user = { id: 'u2' }
        })
        rerender()
        await waitFor(() => expect(ids(result.current.data)).toEqual(['n3']))
    })

    it('still accepts the deprecated deps argument', async () => {
        h.user = { id: 'u1' }
        const { result } = renderHook(() => {
            const plain = useNotes('work')
            const legacy = useMyLiveQuery(
                (q, { userId }) =>
                    q.from({ note: notes }).where(({ note }) => eq(note.owner, userId)),
                ['ignored']
            )
            return { plain, legacy }
        })
        await waitFor(() => expect(result.current.legacy.isReady).toBe(true))
        expect(ids(result.current.legacy.data)).toEqual(['n1', 'n2'])
    })
})
