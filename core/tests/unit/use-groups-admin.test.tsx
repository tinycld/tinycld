// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({ registry: {} as Record<string, unknown> }))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const mk = <T extends { id: string }>(id: string, initialData: T[]) =>
        createCollection(
            localOnlyCollectionOptions({
                id: `${id}-${Math.random()}`,
                getKey: (r: T) => r.id,
                initialData,
            })
        )
    h.registry = {
        groups: mk('groups', [{ id: 'g1', name: 'Sales', description: '' }]),
        group_members: mk('group_members', [{ id: 'm1', group: 'g1', user: 'u1' }]),
        users: mk('users', [
            { id: 'u1', name: 'Ann', email: 'ann@x.test', role: 'member', disabled: false },
            { id: 'u2', name: 'Bo', email: 'bo@x.test', role: 'member', disabled: false },
            { id: 'u3', name: 'Guest', email: 'g@x.test', role: 'guest', disabled: false },
            { id: 'u4', name: 'Off', email: 'off@x.test', role: 'member', disabled: true },
        ]),
    }
    return { useStore: (...names: string[]) => names.map(n => h.registry[n]) }
})

import { useGroupMembersAdmin, useGroupsAdmin } from '../../lib/groups/use-groups-admin'

function wrapper() {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    return ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
}

afterEach(cleanup)

describe('useGroupsAdmin', () => {
    it('lists groups with member counts and creates a new group', async () => {
        const { result } = renderHook(() => useGroupsAdmin(), { wrapper: wrapper() })
        await waitFor(() => expect(result.current.isReady).toBe(true))
        expect(result.current.groups).toEqual([
            { id: 'g1', name: 'Sales', description: '', memberCount: 1 },
        ])

        let newId = ''
        await act(async () => {
            newId = await result.current.createGroup({ name: 'Ops', description: 'On call' })
        })
        await waitFor(() => expect(result.current.groups).toHaveLength(2))
        expect(result.current.groups.find(g => g.id === newId)).toMatchObject({
            name: 'Ops',
            memberCount: 0,
        })
    })
})

describe('useGroupMembersAdmin', () => {
    it('offers only enabled non-guest non-members as candidates', async () => {
        const { result } = renderHook(() => useGroupMembersAdmin('g1'), { wrapper: wrapper() })
        await waitFor(() => expect(result.current.members).toHaveLength(1))
        expect(result.current.members[0]).toMatchObject({ userId: 'u1', name: 'Ann' })
        expect(result.current.candidates.map(c => c.userId)).toEqual(['u2'])

        act(() => result.current.addMember('u2'))
        await waitFor(() => expect(result.current.members).toHaveLength(2))
        expect(result.current.candidates).toHaveLength(0)
    })
})
