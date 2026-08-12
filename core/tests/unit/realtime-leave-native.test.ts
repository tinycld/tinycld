import { describe, expect, it } from 'vitest'

/**
 * React Native defines a global `window` object that is NOT a DOM window: it
 * has no `addEventListener`. A `typeof window === 'undefined'` guard therefore
 * passes on native and the call throws
 *
 *   TypeError: window.addEventListener is not a function
 *
 * from a passive effect, which unmounts the whole screen. That shipped and
 * crashed the cards board on iOS, so the shape is pinned here.
 *
 * This tests the GUARD rather than the hook: reaching the hook needs a React
 * renderer plus expo-router's navigation context, and the bug was never in the
 * hook's logic — it was in which global it probed.
 */

/** The guard as written in useLeaveOnBlur. */
function canListenToPageHide(win: unknown): boolean {
    if (typeof win === 'undefined' || win === null) return false
    return typeof (win as { addEventListener?: unknown }).addEventListener === 'function'
}

describe('pagehide guard', () => {
    it('declines React Native’s window, which has no addEventListener', () => {
        // What RN actually provides: a real object, DOM-less.
        const reactNativeWindow = { navigator: { product: 'ReactNative' } }
        expect(canListenToPageHide(reactNativeWindow)).toBe(false)
    })

    it('accepts a real DOM window', () => {
        const domWindow = { addEventListener() {}, removeEventListener() {} }
        expect(canListenToPageHide(domWindow)).toBe(true)
    })

    it('declines when there is no window at all (SSR, unit tests)', () => {
        expect(canListenToPageHide(undefined)).toBe(false)
        expect(canListenToPageHide(null)).toBe(false)
    })

    it('is not satisfied by a non-callable addEventListener', () => {
        // Guarding on presence rather than callability would pass here and
        // then throw at the call site — the same class of bug.
        expect(canListenToPageHide({ addEventListener: 'nope' })).toBe(false)
    })
})
