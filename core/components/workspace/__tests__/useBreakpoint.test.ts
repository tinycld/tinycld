import { describe, expect, it } from 'vitest'
import { breakpointFor } from '../useBreakpoint'

describe('breakpointFor', () => {
    it('gives the bucket of a width, so a view can use its own width, not the window', () => {
        expect(breakpointFor(480)).toBe('mobile')
        expect(breakpointFor(767)).toBe('mobile')
        expect(breakpointFor(768)).toBe('tablet')
        expect(breakpointFor(1023)).toBe('tablet')
        expect(breakpointFor(1024)).toBe('desktop')
    })
})
