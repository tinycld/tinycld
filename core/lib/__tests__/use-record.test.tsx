// @vitest-environment happy-dom
import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { useRecord } from '@tinycld/core/lib/use-record'
import { createCollection, disconnectRealtime } from 'pbtsdb/core'
import PocketBase, { BaseAuthStore } from 'pocketbase'
import { afterEach, describe, expect, it } from 'vitest'

interface Note {
    id: string
    owner: string
    title: string
}

const rows: Note[] = [
    { id: 'n1', owner: 'u1', title: 'First' },
    { id: 'n2', owner: 'u1', title: 'Second' },
    { id: 'n3', owner: 'u2', title: 'Elsewhere' },
]

function matching(filter: string | undefined): Note[] {
    if (!filter) return rows
    return rows.filter(row =>
        Object.entries(row).some(([field, value]) => filter.includes(`${field} = "${value}"`))
    )
}

/**
 * A real pbtsdb collection over a PocketBase client whose record service
 * answers from `rows` and counts every request, so a test can tell a lookup
 * served from the store from one that went to the server. Realtime stays
 * closed; only the fetch path is under test.
 */
function makeNotes() {
    const requests: string[] = []
    const service = {
        getFullList: async (opts?: { filter?: string }) => {
            requests.push(opts?.filter ?? '')
            return matching(opts?.filter)
        },
        getList: async (_page: number, _perPage: number, opts?: { filter?: string }) => {
            requests.push(opts?.filter ?? '')
            const items = matching(opts?.filter)
            return { items, totalItems: items.length, totalPages: 1, page: 1, perPage: 500 }
        },
    }
    const pb = new PocketBase('http://127.0.0.1:9', new BaseAuthStore())
    Object.defineProperty(pb, 'collection', { value: () => service })
    disconnectRealtime(pb)
    const notes = createCollection<{ notes: { type: Note } }>(pb)('notes', {
        syncMode: 'on-demand',
        realtime: 'query',
    })
    return { notes, requests }
}

afterEach(() => cleanup())

describe('useRecord', () => {
    it('reads a row another live query holds from the store, with no request', async () => {
        const { notes, requests } = makeNotes()
        const list = renderHook(() =>
            useLiveQuery(q => q.from({ n: notes }).where(({ n }) => eq(n.owner, 'u1')))
        )
        await waitFor(() => expect(list.result.current.data).toHaveLength(2))
        const before = requests.length

        const { result } = renderHook(() => useRecord(notes, 'n2'))

        await waitFor(() => expect(result.current.record?.title).toBe('Second'))
        expect(result.current.isLoading).toBe(false)
        expect(requests).toHaveLength(before)
    })

    it('fetches a row the store does not hold', async () => {
        const { notes, requests } = makeNotes()

        const { result } = renderHook(() => useRecord(notes, 'n3'))

        await waitFor(() => expect(result.current.record?.title).toBe('Elsewhere'))
        expect(requests).toHaveLength(1)
    })

    it('is disabled while the id is empty', async () => {
        const { notes, requests } = makeNotes()

        const { result } = renderHook(() => useRecord(notes, ''))
        const unset = renderHook(() => useRecord(notes, undefined))

        expect(result.current).toEqual({ record: undefined, isLoading: false })
        expect(unset.result.current).toEqual({ record: undefined, isLoading: false })
        await new Promise(resolve => setTimeout(resolve, 20))
        expect(requests).toEqual([])
    })
})
