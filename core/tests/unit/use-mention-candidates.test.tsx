// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// A mention autocomplete used to read the whole `users` collection the moment
// someone typed `@`. The predicate is now in the query, which makes it the thing
// worth asserting: a guest or a deactivated account must never be offered as a
// mention target, the window must stay capped, and nothing may be fetched before
// the user has typed anything.
//
// The predicate is also written positively (inArray over roles, disabled = false)
// because pbtsdb compiles a `where` to a PocketBase filter and `not(...)` becomes
// `!(...)`, which the server refuses. A local-only collection evaluates the same
// expression in memory, so this test proves the positive form still selects the
// rows the refused `not()` form was meant to.

const h = vi.hoisted(() => ({
    roster: [
        { id: 'u-owner', name: 'Ada', email: 'ada@example.com', role: 'owner', disabled: false },
        { id: 'u-admin', name: 'Alan', email: 'alan@example.com', role: 'admin', disabled: false },
        {
            id: 'u-member',
            name: 'Alice',
            email: 'alice@example.com',
            role: 'member',
            disabled: false,
        },
        {
            id: 'u-guest',
            name: 'Alvin',
            email: 'alvin@example.com',
            role: 'guest',
            disabled: false,
        },
        { id: 'u-off', name: 'Amos', email: 'amos@example.com', role: 'member', disabled: true },
        { id: 'u-other', name: 'Zoe', email: 'zoe@example.com', role: 'member', disabled: false },
        // Enough same-prefix members to overflow the 20-row cap.
        ...Array.from({ length: 25 }, (_, i) => ({
            id: `u-bulk-${i}`,
            name: `Bulk ${String(i).padStart(2, '0')}`,
            email: `bulk${i}@example.com`,
            role: 'member',
            disabled: false,
        })),
    ],
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const users = createCollection(
        localOnlyCollectionOptions({
            id: 'mention-users',
            getKey: (r: { id: string }) => r.id,
            initialData: h.roster,
            defaultIndexType: BasicIndex,
        })
    )
    // The real pbtsdb store pushes `orderBy` + `limit` to PocketBase, which has
    // its own index. A local-only collection evaluates them in memory and warns
    // unless the sort column is indexed here.
    users.createIndex(row => row.name)
    return { useStore: (...names: string[]) => names.map(() => users) }
})

import { MENTION_CANDIDATE_LIMIT, useMentionCandidates } from '../../lib/use-mention-candidates'

afterEach(() => cleanup())

async function candidates(search: string, options?: Parameters<typeof useMentionCandidates>[1]) {
    const { result } = renderHook(() => useMentionCandidates(search, options))
    await waitFor(() => expect(result.current).toBeDefined())
    return result.current
}

describe('useMentionCandidates', () => {
    it('offers owners, admins and members matching the typed prefix', async () => {
        const names = (await candidates('Al')).map(s => s.displayName)
        expect(names).toContain('Alan')
        expect(names).toContain('Alice')
    })

    it('never offers a guest — they exist only to hold a share link', async () => {
        const names = (await candidates('Al')).map(s => s.displayName)
        expect(names).not.toContain('Alvin')
    })

    it('never offers a deactivated account', async () => {
        const names = (await candidates('Am')).map(s => s.displayName)
        expect(names).not.toContain('Amos')
    })

    it('honours the candidate cap even when far more rows match', async () => {
        const matches = await candidates('Bulk')
        expect(matches).toHaveLength(MENTION_CANDIDATE_LIMIT)
    })

    it('excludes the current user — self-mentions are noise the notifier drops', async () => {
        const matches = await candidates('Al', { currentUserId: 'u-member' })
        expect(matches.map(s => s.userId)).not.toContain('u-member')
        expect(matches.map(s => s.userId)).toContain('u-admin')
    })

    it('runs no query before anything is typed', async () => {
        expect(await candidates('')).toEqual([])
    })

    it('runs no query when the caller disables mentions', async () => {
        expect(await candidates('Al', { disabled: true })).toEqual([])
    })

    it('surfaces the email as the secondary line', async () => {
        const [first] = await candidates('Ada')
        expect(first).toEqual({
            userId: 'u-owner',
            displayName: 'Ada',
            secondary: 'ada@example.com',
        })
    })
})
