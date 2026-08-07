import { useWorkspaceStore } from '@tinycld/core/lib/stores/workspace-store'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('workspace-store edge-swipe suspension', () => {
    beforeEach(() => {
        vi.useFakeTimers()
    })

    afterEach(() => {
        vi.runAllTimers()
        vi.useRealTimers()
    })

    it('suspends immediately when a drag starts', () => {
        useWorkspaceStore.getState().setEdgeSwipeSuspended(true)
        expect(useWorkspaceStore.getState().isEdgeSwipeSuspended).toBe(true)
    })

    it('stays suspended through the grace window after a drag ends', () => {
        const store = useWorkspaceStore.getState()
        store.setEdgeSwipeSuspended(true)
        store.setEdgeSwipeSuspended(false)

        // The micro-lifted re-touch arrives within milliseconds of the drag
        // ending — the suspension must still be in force then.
        vi.advanceTimersByTime(100)
        expect(useWorkspaceStore.getState().isEdgeSwipeSuspended).toBe(true)

        vi.advanceTimersByTime(400)
        expect(useWorkspaceStore.getState().isEdgeSwipeSuspended).toBe(false)
    })

    it('cancels a pending resume when a new drag starts inside the grace window', () => {
        const store = useWorkspaceStore.getState()
        store.setEdgeSwipeSuspended(true)
        store.setEdgeSwipeSuspended(false)
        vi.advanceTimersByTime(100)

        store.setEdgeSwipeSuspended(true)
        vi.advanceTimersByTime(1000)
        expect(useWorkspaceStore.getState().isEdgeSwipeSuspended).toBe(true)

        store.setEdgeSwipeSuspended(false)
        vi.advanceTimersByTime(1000)
        expect(useWorkspaceStore.getState().isEdgeSwipeSuspended).toBe(false)
    })

    it('collapses repeated resumes into the latest grace window', () => {
        const store = useWorkspaceStore.getState()
        store.setEdgeSwipeSuspended(true)
        store.setEdgeSwipeSuspended(false)
        vi.advanceTimersByTime(300)
        store.setEdgeSwipeSuspended(false)

        vi.advanceTimersByTime(300)
        expect(useWorkspaceStore.getState().isEdgeSwipeSuspended).toBe(true)
        vi.advanceTimersByTime(200)
        expect(useWorkspaceStore.getState().isEdgeSwipeSuspended).toBe(false)
    })
})
