import { describe, expect, it } from 'vitest'
import { FONT_PX, longestWordWidth, textWidth } from '../text-width'

const CELL = { size: 'sm', weight: 'normal' } as const
const BOLD = { size: 'sm', weight: 'semibold' } as const
const HEADING = { size: 'xs', weight: 'semibold', uppercase: true } as const

describe('textWidth', () => {
    it('is zero for no text', () => {
        expect(textWidth('', CELL)).toBe(0)
    })

    it('sizes digits as tabular figures, wider when semibold', () => {
        expect(FONT_PX).toEqual({ sm: 14, xs: 12 })
        // 0.62 em × 14 = 8.68
        expect(textWidth('0', CELL)).toBe(9)
        // 6 digits × 0.62 + 2 narrow × 0.34 = 4.40 em × 14 = 61.6
        expect(textWidth('1,234.56', CELL)).toBe(62)
        // 6 × 0.65 + 2 × 0.40 = 4.70 em × 14 = 65.8
        expect(textWidth('1,234.56', BOLD)).toBe(66)
    })

    it('tells narrow, wide, upper and lower letters apart', () => {
        // M wide 0.95 + i narrow 0.34 = 1.29 em × 14 = 18.06
        expect(textWidth('Mi', CELL)).toBe(19)
        // A upper 0.78 + b lower 0.61 = 1.39 em × 14 = 19.46
        expect(textWidth('Ab', CELL)).toBe(20)
    })

    it('draws a heading in capitals at the small size', () => {
        // "ABC": 3 × 0.80 = 2.4 em × 12 = 28.8
        expect(textWidth('abc', HEADING)).toBe(29)
    })

    it('counts accented Latin as letters and other scripts and symbols at a full em', () => {
        expect(textWidth('É', CELL)).toBe(textWidth('E', CELL))
        expect(textWidth('é', CELL)).toBe(textWidth('e', CELL))
        expect(textWidth('日本', CELL)).toBe(28)
        expect(textWidth('@', CELL)).toBe(14)
    })
})

describe('longestWordWidth', () => {
    it('is the width of the widest word', () => {
        // "reversal": 3 narrow × 0.34 + 5 lower × 0.61 = 4.07 em × 14 = 56.98
        expect(longestWordWidth('Void reversal', CELL)).toBe(57)
        expect(longestWordWidth('', CELL)).toBe(0)
    })
})
