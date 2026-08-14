import { describe, expect, it, vi } from 'vitest'
import { createWarmEditorStore } from '../warm-editor-store'

/**
 * One WebView is shared by every editing surface in a package. The store is who
 * holds it and which configuration generation is live — deliberately plain TS
 * with no React and no WebView, because the handover rules are the part worth
 * testing and neither of those is needed to test them.
 */
describe('warm editor store', () => {
    it('starts unheld', () => {
        const store = createWarmEditorStore()
        expect(store.holder()).toBeNull()
    })

    it('hands the instance to an acquiring surface', () => {
        const store = createWarmEditorStore()
        store.acquire('comment:a')
        expect(store.holder()).toBe('comment:a')
    })

    it('bumps the generation on every acquire, so the page rebuilds', () => {
        const store = createWarmEditorStore()
        const first = store.acquire('comment:a')
        const second = store.acquire('comment:b')
        expect(second).toBeGreaterThan(first)
    })

    /**
     * The handover case. Taking the instance must transfer it outright — an
     * acquire while another surface holds it is how tapping a second comment
     * behaves, and leaving the old holder recorded would let its release later
     * evict the new one.
     */
    it('transfers the instance when another surface acquires it', () => {
        const store = createWarmEditorStore()
        store.acquire('composer:card1')
        store.acquire('comment:a')
        expect(store.holder()).toBe('comment:a')
    })

    it('reports a release by the current holder', () => {
        const store = createWarmEditorStore()
        store.acquire('comment:a')
        expect(store.release('comment:a')).toBe(true)
        expect(store.holder()).toBeNull()
    })

    /**
     * A surface that was displaced still unmounts and still calls release. That
     * late call must not evict whoever took over, or tapping from one comment to
     * another would blank the editor that just opened.
     */
    it('ignores a release from a surface that no longer holds it', () => {
        const store = createWarmEditorStore()
        store.acquire('composer:card1')
        store.acquire('comment:a')

        expect(store.release('composer:card1')).toBe(false)
        expect(store.holder()).toBe('comment:a')
    })

    it('notifies subscribers on acquire and release', () => {
        const store = createWarmEditorStore()
        const listener = vi.fn()
        store.subscribe(listener)

        store.acquire('comment:a')
        store.release('comment:a')

        expect(listener).toHaveBeenCalledTimes(2)
    })

    it('stops notifying after unsubscribe', () => {
        const store = createWarmEditorStore()
        const listener = vi.fn()
        store.subscribe(listener)()

        store.acquire('comment:a')

        expect(listener).not.toHaveBeenCalled()
    })

    /** useSyncExternalStore requires a stable snapshot or it loops forever. */
    it('returns a stable snapshot while nothing changes', () => {
        const store = createWarmEditorStore()
        store.acquire('comment:a')
        expect(store.getSnapshot()).toBe(store.getSnapshot())
    })
})
