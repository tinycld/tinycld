import { describe, expect, it, vi } from 'vitest'
import { createHeightStore } from '../height-store'

/**
 * The native editor's WebView has no intrinsic height, and `flex-1` resolves
 * to ZERO inside a ScrollView (nothing bounded to fill). So the page measures
 * itself and reports a height, and the host sizes the container to it.
 *
 * Two bugs made that loop misbehave on a device, and both are pinned here
 * because neither shows up in any renderer test:
 *
 *   1. Holding the height in `useState` changed the memoized EditorComponent's
 *      IDENTITY. Consumers render it as <EditorComponent />, so a new identity
 *      remounted the WebView, which reset its viewport to minHeight, which
 *      produced a new measurement — thrashing between 72px and the real
 *      height, forever. Hence a store the box subscribes to.
 *   2. Sub-pixel churn: a re-render that changes the height by a fraction
 *      re-lays-out the WebView, which re-measures and reports again.
 */

describe('height store', () => {
    it('starts empty, so the box falls back to minHeight', () => {
        expect(createHeightStore().get()).toBeNull()
    })

    it('notifies subscribers when the page reports a height', () => {
        const store = createHeightStore()
        const listener = vi.fn()
        store.subscribe(listener)

        store.set(517)

        expect(store.get()).toBe(517)
        expect(listener).toHaveBeenCalledTimes(1)
    })

    it('ignores sub-pixel churn, so the measure/resize loop converges', () => {
        // The real numbers from the device: content measured 516.48, reported
        // as 517. A 517 → 517.9 update must not re-render, or the WebView
        // re-lays-out and reports again indefinitely.
        const store = createHeightStore()
        const listener = vi.fn()
        store.subscribe(listener)

        store.set(517)
        store.set(517.9)
        store.set(516.5)

        expect(listener).toHaveBeenCalledTimes(1)
        expect(store.get()).toBe(517)
    })

    it('accepts a real growth, so typing a new paragraph still resizes', () => {
        const store = createHeightStore()
        const listener = vi.fn()
        store.subscribe(listener)

        store.set(517)
        store.set(556)

        expect(listener).toHaveBeenCalledTimes(2)
        expect(store.get()).toBe(556)
    })

    it('stops notifying once unsubscribed', () => {
        const store = createHeightStore()
        const listener = vi.fn()
        const unsubscribe = store.subscribe(listener)

        unsubscribe()
        store.set(400)

        expect(listener).not.toHaveBeenCalled()
    })
})
