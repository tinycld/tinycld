// @vitest-environment happy-dom
import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { create } from 'zustand'

const h = vi.hoisted(() => ({ isFocused: true }))

const setParams = vi.fn()
vi.mock('expo-router', () => ({
    useRouter: () => ({ setParams }),
    useNavigation: () => ({ isFocused: () => h.isFocused }),
}))

import { useUrlStateSync } from '@tinycld/core/lib/use-url-state-sync'

// A stand-in for a package's UI store: one field, mirrored into `?item=`.
const useStore = create<{ item: string | null; open: (id: string | null) => void }>(set => ({
    item: null,
    open: id => set({ item: id }),
}))
const item = () => useStore.getState().item

// The "records" the URL resolves against: an id is only meaningful once its
// row has loaded, which is what `isReady` and the undefined urlValue model.
function useSync({
    params,
    isReady = true,
    onChangeWhileBlurred,
}: {
    params: { item: string }
    isReady?: boolean
    onChangeWhileBlurred?: (value: string | null) => void
}) {
    useUrlStateSync<string | null>({
        isReady,
        urlValue: isReady ? params.item || null : undefined,
        params,
        format: value => ({ item: value ?? undefined }),
        read: () => useStore.getState().item,
        write: value => useStore.getState().open(value),
        subscribe: useStore.subscribe,
        onChangeWhileBlurred,
    })
}

describe('useUrlStateSync', () => {
    beforeEach(() => {
        useStore.setState({ item: null })
    })
    // Unmount every hook, or its store subscription outlives the test and
    // answers the next test's writes with its own params.
    afterEach(() => {
        cleanup()
        h.isFocused = true
        vi.clearAllMocks()
    })

    // URL -> store: a pasted link opens the item, and the URL it arrived on is
    // left exactly as it was. The bug this pins: writing the store->URL
    // direction on that same render stripped the value out of its own link.
    it('writes the URL value into the store on a cold load, and leaves the URL alone', () => {
        renderHook(() => useSync({ params: { item: 'a' } }))
        expect(item()).toBe('a')
        expect(setParams).not.toHaveBeenCalled()
    })

    it('does nothing until ready, then applies the URL', () => {
        const { rerender } = renderHook(
            ({ isReady }) => useSync({ params: { item: 'a' }, isReady }),
            {
                initialProps: { isReady: false },
            }
        )
        expect(item()).toBeNull()
        rerender({ isReady: true })
        expect(item()).toBe('a')
        expect(setParams).not.toHaveBeenCalled()
    })

    // store -> URL: a user action writes the params.
    it('writes the store value to the URL when it changes', () => {
        renderHook(() => useSync({ params: { item: '' } }))
        act(() => useStore.getState().open('b'))
        expect(setParams).toHaveBeenCalledWith({ item: 'b' })
    })

    // Clearing is a transition: the store empties and the URL follows. The
    // URL->store half must not read the still-unchanged URL as an arriving
    // link and restore the value.
    it('clears the URL when the store clears, and does not restore it', () => {
        const { rerender } = renderHook(() => useSync({ params: { item: 'a' } }))
        expect(item()).toBe('a')

        act(() => useStore.getState().open(null))
        expect(setParams).toHaveBeenCalledWith({ item: undefined })
        rerender()
        expect(item()).toBeNull()
        expect(setParams).toHaveBeenCalledTimes(1)
    })

    // The echo of its own write: once the URL carries what the store said,
    // neither half touches the other.
    it('is quiet once the URL has caught up with the store', () => {
        const { rerender } = renderHook(({ params }) => useSync({ params }), {
            initialProps: { params: { item: '' } },
        })
        act(() => useStore.getState().open('b'))
        rerender({ params: { item: 'b' } })
        expect(item()).toBe('b')
        expect(setParams).toHaveBeenCalledTimes(1)
    })

    it('does not write when the URL already spells the store value', () => {
        renderHook(() => useSync({ params: { item: 'a' } }))
        act(() => useStore.getState().open('a'))
        expect(setParams).not.toHaveBeenCalled()
    })

    // A screen stays mounted under a route pushed over it; setParams would
    // then change the covering route's params.
    it('hands a change to onChangeWhileBlurred when the route is not focused', () => {
        const blurred = vi.fn()
        renderHook(() => useSync({ params: { item: 'a' }, onChangeWhileBlurred: blurred }))
        h.isFocused = false
        act(() => useStore.getState().open('b'))
        expect(setParams).not.toHaveBeenCalled()
        expect(blurred).toHaveBeenCalledWith('b')
    })

    // Arriving with a stale store value and a URL naming nothing: the URL wins.
    it('clears a stale store value when the URL names nothing', () => {
        useStore.setState({ item: 'stale' })
        renderHook(() => useSync({ params: { item: '' } }))
        expect(item()).toBeNull()
        expect(setParams).not.toHaveBeenCalled()
    })
})
