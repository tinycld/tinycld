import { describe, expect, it } from 'vitest'
import { numberKeyboardType } from '../number-input-text'

describe('numberKeyboardType', () => {
    const wholeUnsigned = { min: 0 }
    const decimalUnsigned = { allowDecimal: true, min: 0 }
    const decimalSigned = { allowDecimal: true }
    const wholeSigned = { min: -10 }

    it('keeps the digit pad for a whole number that cannot be negative', () => {
        expect(numberKeyboardType(wholeUnsigned, 'ios')).toBe('numeric')
        expect(numberKeyboardType(wholeUnsigned, 'android')).toBe('numeric')
        expect(numberKeyboardType(wholeUnsigned, 'web')).toBe('numeric')
        expect(numberKeyboardType({ min: 1 }, 'ios')).toBe('numeric')
    })

    it('gives a pad with a decimal point to a decimal that cannot be negative', () => {
        expect(numberKeyboardType(decimalUnsigned, 'ios')).toBe('decimal-pad')
        expect(numberKeyboardType(decimalUnsigned, 'android')).toBe('decimal-pad')
        expect(numberKeyboardType(decimalUnsigned, 'web')).toBe('decimal-pad')
    })

    it('gives iOS the numbers and punctuation keyboard when a negative is allowed', () => {
        expect(numberKeyboardType(decimalSigned, 'ios')).toBe('numbers-and-punctuation')
        expect(numberKeyboardType(wholeSigned, 'ios')).toBe('numbers-and-punctuation')
        expect(numberKeyboardType({}, 'ios')).toBe('numbers-and-punctuation')
    })

    it('gives web the text keyboard when a negative is allowed', () => {
        expect(numberKeyboardType(decimalSigned, 'web')).toBe('numbers-and-punctuation')
        expect(numberKeyboardType(wholeSigned, 'web')).toBe('numbers-and-punctuation')
    })

    it('keeps the Android numeric pad, which has a minus and a point, when a negative is allowed', () => {
        expect(numberKeyboardType(decimalSigned, 'android')).toBe('numeric')
        expect(numberKeyboardType(wholeSigned, 'android')).toBe('numeric')
    })
})
