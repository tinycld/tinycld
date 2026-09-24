// @vitest-environment happy-dom
import { createCollection, localOnlyCollectionOptions } from '@tanstack/db'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@tinycld/core/lib/pocketbase', () => ({ useStore: () => [] }))

import { useGroupGrants } from '../../lib/groups/use-group-grants'

interface KeeperRow {
    id: string
    zoo: string
    user: string
    group: string
    role: 'editor' | 'viewer'
}

const ROLES = [
    { value: 'editor' as const, label: 'Editor' },
    { value: 'viewer' as const, label: 'Viewer' },
]

function keepers(initialData: KeeperRow[]) {
    return createCollection(
        localOnlyCollectionOptions({
            id: `keepers-${Math.random()}`,
            getKey: (r: KeeperRow) => r.id,
            initialData,
        })
    )
}

function wrapper() {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    return ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
}

afterEach(cleanup)

describe('useGroupGrants', () => {
    it('lists only grant rows (user empty) for the resource', async () => {
        const collection = keepers([
            { id: 'direct', zoo: 'bronx', user: 'u1', group: '', role: 'editor' },
            { id: 'grant', zoo: 'bronx', user: '', group: 'g1', role: 'viewer' },
            { id: 'derived', zoo: 'bronx', user: 'u2', group: 'g1', role: 'viewer' },
            { id: 'other', zoo: 'sd', user: '', group: 'g2', role: 'viewer' },
        ])
        const { result } = renderHook(
            () =>
                useGroupGrants({
                    collection,
                    roles: ROLES,
                    isForResource: row => row.zoo === 'bronx',
                    buildRow: grant => ({ ...grant, zoo: 'bronx' }),
                }),
            { wrapper: wrapper() }
        )
        await waitFor(() => expect(result.current.isReady).toBe(true))
        expect(result.current.grants).toEqual([{ grantId: 'grant', groupId: 'g1', role: 'viewer' }])
    })

    it('adds, changes role and removes through the collection', async () => {
        const collection = keepers([])
        const { result } = renderHook(
            () =>
                useGroupGrants({
                    collection,
                    roles: ROLES,
                    isForResource: row => row.zoo === 'bronx',
                    buildRow: grant => ({ ...grant, zoo: 'bronx' }),
                }),
            { wrapper: wrapper() }
        )
        await waitFor(() => expect(result.current.isReady).toBe(true))

        act(() => result.current.onAdd('g9', 'viewer'))
        await waitFor(() => expect(result.current.grants).toHaveLength(1))
        const inserted = collection.toArray[0]
        expect(inserted).toMatchObject({ zoo: 'bronx', user: '', group: 'g9', role: 'viewer' })

        act(() => result.current.onRoleChange(inserted.id, 'editor'))
        await waitFor(() => expect(result.current.grants[0]?.role).toBe('editor'))

        act(() => result.current.onRemove(inserted.id))
        await waitFor(() => expect(result.current.grants).toHaveLength(0))
    })
})
