import { useOrientationStore } from '@tinycld/core/lib/stores/orientation-store'
import { describe, expect, it } from 'vitest'

describe('orientation-store', () => {
    it('starts unknown so resolveInsets stays conservative pre-first-event', () => {
        expect(useOrientationStore.getState().orientation).toBe('unknown')
    })

    it('exposes setOrientation that updates the orientation', () => {
        useOrientationStore.getState().setOrientation('landscape-left')
        expect(useOrientationStore.getState().orientation).toBe('landscape-left')
        useOrientationStore.getState().setOrientation('unknown')
    })
})
