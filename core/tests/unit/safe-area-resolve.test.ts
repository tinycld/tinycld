import {
    type AppOrientation,
    type Insets,
    islandSide,
    resolveInsets,
} from '@tinycld/core/lib/safe-area-resolve'
import { describe, expect, it } from 'vitest'

const landscape: Insets = { top: 0, left: 59, right: 59, bottom: 21 }

describe('islandSide', () => {
    // LOCKED MAPPING — verified on an iPhone 17 simulator (iOS 26.5) by
    // locking each landscape orientation and observing which interface side
    // the Dynamic Island overlapped. The names really are crossed. Changing
    // these expectations requires re-verifying on a simulator, not reasoning
    // from Apple's interface-vs-device orientation docs.
    it('maps each landscape rotation to the housing side', () => {
        expect(islandSide('landscape-left')).toBe('right')
        expect(islandSide('landscape-right')).toBe('left')
    })

    it('reports no side outside landscape', () => {
        expect(islandSide('portrait')).toBeNull()
        expect(islandSide('unknown')).toBeNull()
    })
})

describe('resolveInsets', () => {
    it('passes portrait through untouched', () => {
        const portrait: Insets = { top: 59, left: 0, right: 0, bottom: 34 }
        expect(resolveInsets(portrait, 'portrait', 'ios')).toEqual(portrait)
    })

    it('keeps only the left inset when the housing is on the left (landscape-right)', () => {
        expect(resolveInsets(landscape, 'landscape-right', 'ios')).toEqual({
            top: 0,
            left: 59,
            right: 0,
            bottom: 21,
        })
    })

    it('keeps only the right inset when the housing is on the right (landscape-left)', () => {
        expect(resolveInsets(landscape, 'landscape-left', 'ios')).toEqual({
            top: 0,
            left: 0,
            right: 59,
            bottom: 21,
        })
    })

    it('passes symmetric insets through while the orientation is unknown', () => {
        // Conservative pre-first-event behavior: symmetric padding can waste a
        // gutter for a frame but can never put content under the island.
        expect(resolveInsets(landscape, 'unknown', 'ios')).toEqual(landscape)
    })

    it('never touches non-iOS platforms', () => {
        // Android display-cutout insets are genuinely per-side already.
        const cutout: Insets = { top: 0, left: 30, right: 0, bottom: 0 }
        expect(resolveInsets(cutout, 'landscape-left', 'android')).toEqual(cutout)
        expect(resolveInsets(landscape, 'landscape-left', 'web')).toEqual(landscape)
    })

    it('passes zero horizontal insets through (iPad, web)', () => {
        const flat: Insets = { top: 24, left: 0, right: 0, bottom: 20 }
        for (const orientation of ['landscape-left', 'landscape-right'] as AppOrientation[]) {
            expect(resolveInsets(flat, orientation, 'ios')).toEqual(flat)
        }
    })

    it('leaves an already one-sided pair alone', () => {
        // Defensive: if iOS ever starts reporting per-side insets itself, the
        // correction must not zero the one real value.
        const oneSided: Insets = { top: 0, left: 59, right: 0, bottom: 21 }
        expect(resolveInsets(oneSided, 'landscape-right', 'ios')).toEqual(oneSided)
    })

    it('does not mutate its input', () => {
        const input = { ...landscape }
        resolveInsets(input, 'landscape-left', 'ios')
        expect(input).toEqual(landscape)
    })
})
