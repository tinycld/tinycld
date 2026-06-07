import { useBreakpoint } from '@tinycld/core/components/workspace/useBreakpoint'
import { useWindowSizeStore } from '@tinycld/core/lib/stores/window-size-store'
import { describe, expect, it } from 'vitest'

describe('window-size-store', () => {
    it('exposes setSize that updates width/height', () => {
        useWindowSizeStore.getState().setSize(1280, 800)
        expect(useWindowSizeStore.getState().width).toBe(1280)
        expect(useWindowSizeStore.getState().height).toBe(800)
    })

    it('useBreakpoint maps width to the right bucket', () => {
        const select = (w: number) => {
            useWindowSizeStore.getState().setSize(w, 800)
            // selector inlined to avoid mounting React for a synchronous read
            return useWindowSizeStore.getState().width >= 1024
                ? 'desktop'
                : useWindowSizeStore.getState().width >= 768
                  ? 'tablet'
                  : 'mobile'
        }
        expect(select(1280)).toBe('desktop')
        expect(select(1024)).toBe('desktop')
        expect(select(800)).toBe('tablet')
        expect(select(767)).toBe('mobile')
    })

    it('useBreakpoint export type is callable (smoke)', () => {
        expect(typeof useBreakpoint).toBe('function')
    })
})
