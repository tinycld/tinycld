import { FIELD_WIDTHS } from '@tinycld/core/lib/forms/definition'
import { describe, expect, it } from 'vitest'
import { effectiveWidth, NARROW_BELOW, widthClass } from '../widths'

const tokensOf = (classes: string) => classes.split(/\s+/).filter(Boolean)

describe('effectiveWidth', () => {
    it('keeps widths at or above the breakpoint', () => {
        expect(effectiveWidth('quarter', NARROW_BELOW)).toBe('quarter')
        expect(effectiveWidth('third', 1000)).toBe('third')
    })
    it('collapses below the breakpoint: quarter to half, half and third to full', () => {
        expect(effectiveWidth('quarter', NARROW_BELOW - 1)).toBe('half')
        expect(effectiveWidth('half', 500)).toBe('full')
        expect(effectiveWidth('third', 500)).toBe('full')
        expect(effectiveWidth('full', 500)).toBe('full')
    })
    it('treats an unmeasured container as wide', () => {
        expect(effectiveWidth('quarter', 0)).toBe('quarter')
    })
    it('keeps a fit-content width fit-content below the breakpoint', () => {
        expect(effectiveWidth('auto', NARROW_BELOW - 1)).toBe('auto')
        expect(effectiveWidth('auto', NARROW_BELOW)).toBe('auto')
    })
})

describe('effectiveWidth with a narrow width', () => {
    it('lists the five widths, fit content last', () => {
        expect(FIELD_WIDTHS).toEqual(['full', 'half', 'third', 'quarter', 'auto'])
    })
    it.each([
        'full',
        'half',
        'third',
        'quarter',
        'auto',
    ] as const)('uses narrowWidth %s exactly below the breakpoint', narrow => {
        expect(effectiveWidth('quarter', NARROW_BELOW - 1, narrow)).toBe(narrow)
        expect(effectiveWidth('auto', NARROW_BELOW - 1, narrow)).toBe(narrow)
        expect(effectiveWidth('full', NARROW_BELOW - 1, narrow)).toBe(narrow)
    })
    it('ignores narrowWidth at the breakpoint and when the container is unmeasured', () => {
        expect(effectiveWidth('quarter', NARROW_BELOW, 'full')).toBe('quarter')
        expect(effectiveWidth('half', 1280, 'auto')).toBe('half')
        expect(effectiveWidth('quarter', 0, 'full')).toBe('quarter')
        expect(effectiveWidth('auto', 0, 'half')).toBe('auto')
    })
})

describe('widthClass', () => {
    it('maps to basis classes', () => {
        expect(widthClass('full')).toBe('basis-full')
        expect(widthClass('half')).toBe('basis-1/2')
        expect(widthClass('third')).toBe('basis-1/3')
        expect(widthClass('quarter')).toBe('basis-1/4')
    })
    it('sizes a fit-content cell by its content, capped at the row', () => {
        const tokens = tokensOf(widthClass('auto'))
        expect(tokens).toEqual(expect.arrayContaining(['basis-auto', 'max-w-full']))
        expect(tokens).not.toContain('min-w-0')
        expect(tokens).not.toContain('grow')
        expect(tokens.filter(t => /^basis-(full|1\/)/.test(t))).toEqual([])
    })
})
