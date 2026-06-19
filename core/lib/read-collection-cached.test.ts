import { describe, expect, it, vi } from 'vitest'
import { findCollectionCached, readCollectionCached } from './read-collection-cached'

interface Row {
    id: string
    parent: string
    name: string
}

// A stand-in for a pbtsdb/TanStack-DB collection: just the one method the
// helper relies on. `toArrayWhenReady` resolves from the store, so the fake
// records "the store already holds these rows".
function fakeCollection(rows: Row[]) {
    const toArrayWhenReady = vi.fn(async () => rows)
    return { toArrayWhenReady }
}

const ROWS: Row[] = [
    { id: '1', parent: 'root', name: 'a' },
    { id: '2', parent: 'root', name: 'b' },
    { id: '3', parent: 'folder', name: 'a' },
]

describe('readCollectionCached', () => {
    it('returns every row from the store when no predicate is given', async () => {
        const col = fakeCollection(ROWS)
        expect(await readCollectionCached(col)).toEqual(ROWS)
    })

    it('filters rows by the predicate', async () => {
        const col = fakeCollection(ROWS)
        const siblings = await readCollectionCached(col, r => r.parent === 'root')
        expect(siblings.map(r => r.id)).toEqual(['1', '2'])
    })

    it('reads through the store (toArrayWhenReady), never the network', async () => {
        const col = fakeCollection(ROWS)
        await readCollectionCached(col, () => true)
        // The whole point: one cache-aware store read, no raw getFullList.
        expect(col.toArrayWhenReady).toHaveBeenCalledTimes(1)
    })

    it('returns an empty array when nothing matches', async () => {
        const col = fakeCollection(ROWS)
        expect(await readCollectionCached(col, r => r.parent === 'missing')).toEqual([])
    })

    it('reflects an empty store', async () => {
        const col = fakeCollection([])
        expect(await readCollectionCached(col)).toEqual([])
    })
})

describe('findCollectionCached', () => {
    it('returns the first row matching the predicate', async () => {
        const col = fakeCollection(ROWS)
        const found = await findCollectionCached(col, r => r.parent === 'root' && r.name === 'b')
        expect(found?.id).toBe('2')
    })

    it('returns undefined when no row matches', async () => {
        const col = fakeCollection(ROWS)
        expect(await findCollectionCached(col, r => r.id === 'nope')).toBeUndefined()
    })

    it('resolves a composite key to a single row (the unassign-label use case)', async () => {
        const assignments = [
            {
                id: 'asg1',
                parent: '',
                name: '',
                label: 'L1',
                record_id: 'R1',
                collection: 'contacts',
            },
            {
                id: 'asg2',
                parent: '',
                name: '',
                label: 'L2',
                record_id: 'R1',
                collection: 'contacts',
            },
        ]
        const col = { toArrayWhenReady: async () => assignments }
        const hit = await findCollectionCached(
            col,
            a => a.label === 'L2' && a.record_id === 'R1' && a.collection === 'contacts'
        )
        expect(hit?.id).toBe('asg2')
    })
})
