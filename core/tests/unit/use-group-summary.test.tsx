// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const mk = <T extends { id: string }>(id: string, initialData: T[]) =>
        createCollection(localOnlyCollectionOptions({ id, getKey: (r: T) => r.id, initialData }))
    const registry: Record<string, unknown> = {
        groups: mk('groups', [{ id: 'g1', name: 'Sales', description: 'Quota carriers' }]),
        group_members: mk('group_members', [
            { id: 'm1', group: 'g1', user: 'u1' },
            { id: 'm2', group: 'g1', user: 'u2' },
            { id: 'm3', group: 'g2', user: 'u3' },
        ]),
    }
    return { useStore: (...names: string[]) => names.map(n => registry[n]) }
})

import { useGroupSummary } from '../../lib/groups/use-group-summary'

afterEach(cleanup)

describe('useGroupSummary', () => {
    it('returns the group name and its member count', async () => {
        const { result } = renderHook(() => useGroupSummary('g1'))
        await waitFor(() => expect(result.current.isReady).toBe(true))
        expect(result.current).toMatchObject({
            name: 'Sales',
            description: 'Quota carriers',
            memberCount: 2,
        })
    })

    it('reports a missing group as empty, not as an error', async () => {
        const { result } = renderHook(() => useGroupSummary('nope'))
        await waitFor(() => expect(result.current.isReady).toBe(true))
        expect(result.current).toMatchObject({ name: '', memberCount: 0 })
    })
})
