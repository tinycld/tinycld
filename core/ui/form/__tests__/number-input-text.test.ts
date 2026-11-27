import { describe, expect, it } from 'vitest'
import { parseNumberText } from '../number-input-text'

describe('parseNumberText', () => {
    const decimal = { allowDecimal: true }

    it('reads U+2212 MINUS SIGN as a minus, decimal or integer', () => {
        expect(parseNumberText('−5', {})).toEqual({ text: '-5', value: -5 })
        expect(parseNumberText('−1.25', decimal)).toEqual({ text: '-1.25', value: -1.25 })
        expect(parseNumberText('−', decimal)).toEqual({ text: '-', value: null })
    })

    it('keeps partial decimal text with no value yet', () => {
        expect(parseNumberText('', decimal)).toEqual({ text: '', value: null })
        expect(parseNumberText('-', decimal)).toEqual({ text: '-', value: null })
        expect(parseNumberText('.', decimal)).toEqual({ text: '.', value: null })
        expect(parseNumberText('1.', decimal)).toEqual({ text: '1.', value: 1 })
    })

    it('keeps every decimal the user typed, for the caller to judge', () => {
        expect(parseNumberText('7.555', decimal)).toEqual({ text: '7.555', value: 7.555 })
    })

    it('refuses text that is not a number', () => {
        expect(parseNumberText('1.2.3', decimal)).toBeNull()
        expect(parseNumberText('abc', decimal)).toBeNull()
        expect(parseNumberText('1.5', {})).toBeNull()
        expect(parseNumberText('1−2', decimal)).toBeNull()
    })

    it('refuses a value outside min and max', () => {
        expect(parseNumberText('11', { min: 0, max: 10 })).toBeNull()
        expect(parseNumberText('-1', { min: 0, max: 10 })).toBeNull()
        expect(parseNumberText('−1', { ...decimal, min: 0 })).toBeNull()
        expect(parseNumberText('7', { min: 0, max: 10 })).toEqual({ text: '7', value: 7 })
    })

    it('keeps an empty or a lone minus integer text with no value', () => {
        expect(parseNumberText('', {})).toEqual({ text: '', value: null })
        expect(parseNumberText('−', {})).toEqual({ text: '-', value: null })
    })
})
