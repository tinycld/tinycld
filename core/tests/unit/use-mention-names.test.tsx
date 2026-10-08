// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

// Read-mode mention rendering used to borrow the picker's candidate list for its
// `userId → displayName` map. That worked only because the picker fetched the
// whole roster. Now the picker is a bounded search keyed on what the user is
// typing, so a stored body naming someone who is not a current search hit had
// nothing to resolve against and rendered the raw `[[@id]]` token.
//
// So the lookup is its own query, scoped to the ids the loaded bodies mention.
// Two things are worth asserting: it resolves every mentioned id, and it issues
// NO query when nothing is mentioned — pbtsdb compiles `inArray(field, [])` by
// joining per-value clauses, so an empty array yields a malformed filter the
// server rejects rather than a harmless empty result.

const h = vi.hoisted(() => ({
    queryCount: 0,
    users: [
        { id: 'u_alice', name: 'Alice', email: 'alice@example.com' },
        { id: 'u_bob', name: 'Bob', email: 'bob@example.com' },
        // No display name — the map must fall back to the email rather than
        // render an empty mention.
        { id: 'u_nameless', name: '', email: 'ghost@example.com' },
        // Never mentioned by any body below, so it must never be fetched.
        { id: 'u_absent', name: 'Zoe', email: 'zoe@example.com' },
    ],
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const users = createCollection({
        ...localOnlyCollectionOptions({
            id: 'mention-name-users',
            // Typed as string | number: TanStack DB 0.12's BasicIndex constructor is
            // invariant in the key type and is declared for string | number keys.
            getKey: (r: { id: string }): string | number => r.id,
            initialData: h.users,
        }),
        autoIndex: 'eager',
        defaultIndexType: BasicIndex,
    })
    return { useStore: (...names: string[]) => names.map(() => users) }
})

// Count how many times a non-null query is built, so "issues no query" is an
// assertion about the request rather than about the result being empty.
vi.mock('@tanstack/react-db', async () => {
    const actual = await vi.importActual<typeof import('@tanstack/react-db')>('@tanstack/react-db')
    type QueryFn = (q: never) => unknown
    return {
        ...actual,
        useLiveQuery: (arg: QueryFn | { query: QueryFn }, deps?: unknown[]) => {
            const config = typeof arg === 'function' ? { query: arg } : arg
            const counting = (q: never) => {
                const built = config.query(q)
                if (built !== null && built !== undefined) h.queryCount += 1
                return built
            }
            return actual.useLiveQuery({ ...config, query: counting } as never, deps as never)
        },
    }
})

import { useMentionNames } from '../../lib/comments/use-mention-names'

afterEach(() => {
    cleanup()
    h.queryCount = 0
})

async function names(bodies: string[]) {
    const { result } = renderHook(() => useMentionNames(bodies))
    await waitFor(() => expect(result.current).toBeInstanceOf(Map))
    return result.current
}

describe('useMentionNames', () => {
    it('resolves every id a thread mentions, across root and replies', async () => {
        const map = await waitFor(async () => {
            const m = await names(['hey [[@u_alice]] look', 'agreed, [[@u_bob]]'])
            expect(m.size).toBe(2)
            return m
        })
        expect(map.get('u_alice')).toBe('Alice')
        expect(map.get('u_bob')).toBe('Bob')
    })

    it('resolves two ids mentioned in one body', async () => {
        const map = await waitFor(async () => {
            const m = await names(['[[@u_alice]] and [[@u_bob]] please review'])
            expect(m.size).toBe(2)
            return m
        })
        expect([...map.values()].sort()).toEqual(['Alice', 'Bob'])
    })

    it('falls back to the email when a mentioned user has no display name', async () => {
        const map = await waitFor(async () => {
            const m = await names(['ping [[@u_nameless]]'])
            expect(m.size).toBe(1)
            return m
        })
        expect(map.get('u_nameless')).toBe('ghost@example.com')
    })

    it('never fetches a user no body mentions', async () => {
        const map = await waitFor(async () => {
            const m = await names(['just [[@u_alice]]'])
            expect(m.size).toBe(1)
            return m
        })
        expect(map.has('u_absent')).toBe(false)
    })

    it('issues no query for a thread that mentions nobody', async () => {
        const map = await names(['plain text', 'no mentions here'])
        expect(map.size).toBe(0)
        expect(h.queryCount).toBe(0)
    })

    it('issues no query for an empty thread', async () => {
        const map = await names([])
        expect(map.size).toBe(0)
        expect(h.queryCount).toBe(0)
    })
})
