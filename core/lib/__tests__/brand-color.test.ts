import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { brandColorSchema, DEFAULT_BRAND_COLOR, swatchColor } from '../brand-color'

describe('brandColorSchema', () => {
    it.each(['', '#0d9488', '#ABCDEF', '  #1e40af  '])('accepts %j', value => {
        expect(brandColorSchema.safeParse({ brand_color: value }).success).toBe(true)
    })

    it.each([
        'red',
        '#12345',
        '#1234567',
        '0d9488',
        '#12345g',
    ])('refuses %j with the format hint', value => {
        const result = brandColorSchema.safeParse({ brand_color: value })
        expect(result.success).toBe(false)
        expect(result.error?.issues[0]?.message).toBe('Enter # and six hex digits, like #0d9488')
    })
})

describe('swatchColor', () => {
    it('shows a valid typed color and the default otherwise', () => {
        expect(swatchColor('#1e40af')).toBe('#1e40af')
        expect(swatchColor('#12')).toBe(DEFAULT_BRAND_COLOR)
        expect(swatchColor('')).toBe(DEFAULT_BRAND_COLOR)
    })
})

// The server prints with mailer.BrandColor when no color is set; the preview must agree.
it('matches the server default', () => {
    const source = readFileSync(path.resolve(__dirname, '../../server/mailer/template.go'), 'utf8')
    expect(source).toContain(`const BrandColor = "${DEFAULT_BRAND_COLOR}"`)
})
