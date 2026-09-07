import { describe, expect, it, vi } from 'vitest'
import { createEditorStateStore } from '../editor-state-store'

describe('editor state store', () => {
    it('starts empty and replaces the snapshot on each post', () => {
        const store = createEditorStateStore()
        expect(store.get()).toEqual({})
        store.set({ isBoldActive: true, isReady: true })
        expect(store.get()).toEqual({ isBoldActive: true, isReady: true })
        // A full snapshot each time: a field absent from the next post is gone.
        store.set({ isReady: true })
        expect(store.get()).toEqual({ isReady: true })
    })

    it('notifies subscribers and stops after unsubscribe', () => {
        const store = createEditorStateStore()
        const listener = vi.fn()
        const unsubscribe = store.subscribe(listener)
        store.set({ isReady: true })
        expect(listener).toHaveBeenCalledTimes(1)
        unsubscribe()
        store.set({ isReady: false })
        expect(listener).toHaveBeenCalledTimes(1)
    })

    it('keeps the last state when a post is malformed', () => {
        const store = createEditorStateStore()
        store.set({ isReady: true })
        store.set(null)
        store.set('nope')
        store.set([1, 2])
        expect(store.get()).toEqual({ isReady: true })
    })

    it('hands out a copy, so a caller cannot mutate what the page sent', () => {
        const store = createEditorStateStore()
        const payload = { isReady: true }
        store.set(payload)
        expect(store.get()).not.toBe(payload)
    })
})
