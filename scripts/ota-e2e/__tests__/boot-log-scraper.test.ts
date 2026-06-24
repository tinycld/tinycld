import { describe, expect, it, vi } from 'vitest'
import { extractBootBundleId, scrapeBootBundleId } from '../boot-log-scraper'

describe('extractBootBundleId', () => {
    it('returns the bundle id from a boot line', () => {
        const log =
            'foo\n[tinycld] app-boot: rendered bundle id=build-123-ios hash=abcdef012345\nbar'
        expect(extractBootBundleId(log)).toBe('build-123-ios')
    })

    it('returns the LAST id when multiple boot lines are present', () => {
        const log =
            '[tinycld] app-boot: rendered bundle id=embedded-1.13.7 hash=aaaa\n' +
            '[tinycld] app-boot: rendered bundle id=build-999-ios hash=bbbb\n'
        expect(extractBootBundleId(log)).toBe('build-999-ios')
    })

    it('returns null when no boot line is present', () => {
        expect(extractBootBundleId('nothing to see here')).toBeNull()
    })
})

describe('scrapeBootBundleId', () => {
    it('feeds simctl log output through the parser', async () => {
        const spawnFn = vi.fn(() =>
            Promise.resolve('[tinycld] app-boot: rendered bundle id=build-7-ios hash=cafebabe1234')
        )
        const id = await scrapeBootBundleId('UDID-1', 120, spawnFn)
        expect(id).toBe('build-7-ios')
        expect(spawnFn).toHaveBeenCalledOnce()
    })

    it('returns null when the spawn yields no boot line', async () => {
        const spawnFn = vi.fn(() => Promise.resolve('some unrelated log output'))
        expect(await scrapeBootBundleId('UDID-1', 120, spawnFn)).toBeNull()
    })
})
