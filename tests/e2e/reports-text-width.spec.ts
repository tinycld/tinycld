import { expect, test } from '@playwright/test'
import { type TextStyle, textWidth } from '../../core/reports/lib/text-width'
import { login } from './helpers'

/**
 * The report column measure estimates text widths from a character table
 * (core/reports/lib/text-width.ts) instead of measuring the screen. This holds
 * the table to the font the app really draws with on this machine: every
 * estimate is at least the drawn width (so a figure sized by it is never cut),
 * and the samples together are not more than 35% over.
 */
const SAMPLES = [
    '1,234,567.89',
    '$35,100.00',
    '-00,000.00',
    'Sep 30, 2026',
    'Void reversal of JE-10',
    'Accounts receivable',
    'Harbor Lane Design LLC',
    'WWWW mmmm',
    'Total 1000 Business Checking',
    'Beginning balance',
    'Éclair à la crème',
]
const STYLES: { style: TextStyle; font: string }[] = [
    { style: { size: 'sm', weight: 'normal' }, font: '400 14px' },
    { style: { size: 'sm', weight: 'semibold' }, font: '600 14px' },
    { style: { size: 'xs', weight: 'semibold', uppercase: true }, font: '600 12px' },
]

test.describe('report text width estimate', () => {
    test('is at least the drawn width of every sample in the app font, and not far over in total', async ({
        page,
    }) => {
        await login(page)
        // Any text the app draws: react-native-web gives every Text the same family.
        const family = await page
            .locator('div[dir="auto"]')
            .first()
            .evaluate(el => getComputedStyle(el).fontFamily)
        for (const { style, font } of STYLES) {
            const texts = SAMPLES.map(text => (style.uppercase ? text.toUpperCase() : text))
            const drawn = await page.evaluate(
                ({ texts, font, family }) => {
                    const context = document.createElement('canvas').getContext('2d')
                    if (!context) return []
                    context.font = `${font} ${family}`
                    return texts.map(text => context.measureText(text).width)
                },
                { texts, font, family }
            )
            // No canvas context draws nothing, and every check below would pass.
            expect(drawn).toHaveLength(SAMPLES.length)
            expect(drawn.every(width => width > 0)).toBe(true)
            const estimates = SAMPLES.map(text => textWidth(text, style))
            const under = SAMPLES.filter((_, i) => (estimates[i] ?? 0) < (drawn[i] ?? 0))
            expect(under, `estimated narrower than drawn at ${font}`).toEqual([])
            const total = (list: number[]) => list.reduce((sum, n) => sum + n, 0)
            expect(total(estimates)).toBeLessThanOrEqual(total(drawn) * 1.35)
        }
    })
})
