import { beforeEach, describe, expect, it, vi } from 'vitest'

// Mock the two IO modules assertUpdateIsLive composes, so we can drive the
// branching (boot-log required, a11y required-only-when-idb-present) without a
// device. The sleep between retries is also stubbed (via fake timers) so the
// retry loops resolve instantly.
vi.mock('../boot-log-scraper', () => ({ scrapeBootBundleId: vi.fn() }))
vi.mock('../a11y-sentinel', () => ({ idbAvailable: vi.fn(), queryA11ySentinel: vi.fn() }))

import { idbAvailable, queryA11ySentinel } from '../a11y-sentinel'
import { scrapeBootBundleId } from '../boot-log-scraper'
import { assertUpdateIsLive } from '../update-is-live'

const mockScrape = vi.mocked(scrapeBootBundleId)
const mockIdbAvailable = vi.mocked(idbAvailable)
const mockQuerySentinel = vi.mocked(queryA11ySentinel)

// A fail() that throws (instead of process.exit) so tests can assert on it.
function throwingFail(msg: string): never {
    throw new Error(msg)
}
const noopLog = () => {}

beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
})

// The retry loops await setTimeout; advance timers as the microtasks queue.
async function runWithTimers(p: Promise<unknown>): Promise<void> {
    await vi.runAllTimersAsync()
    await p
}

describe('assertUpdateIsLive', () => {
    it('passes when boot-log and sentinel both match, idb present', async () => {
        mockScrape.mockResolvedValue('build-5-ios')
        mockIdbAvailable.mockResolvedValue(true)
        mockQuerySentinel.mockResolvedValue('build-5-ios')
        const logs: string[] = []
        await runWithTimers(
            assertUpdateIsLive('UDID', 'build-5-ios', throwingFail, m => logs.push(m))
        )
        expect(logs).toContain('boot-log proof: new bundle JS executed + mounted (id=build-5-ios)')
        expect(logs).toContain('on-screen sentinel proof: update visible (id=build-5-ios)')
    })

    it('fails when the boot-log never reports the new id', async () => {
        mockScrape.mockResolvedValue('embedded-1.0.0')
        const p = assertUpdateIsLive('UDID', 'build-5-ios', throwingFail, noopLog)
        const expectation = expect(p).rejects.toThrow(/boot-log proof missing/)
        await vi.runAllTimersAsync()
        await expectation
        // idb is never consulted once the boot-log proof fails.
        expect(mockIdbAvailable).not.toHaveBeenCalled()
    })

    it('skips the sentinel check (passes) when idb is absent', async () => {
        mockScrape.mockResolvedValue('build-5-ios')
        mockIdbAvailable.mockResolvedValue(false)
        const logs: string[] = []
        await runWithTimers(
            assertUpdateIsLive('UDID', 'build-5-ios', throwingFail, m => logs.push(m))
        )
        expect(logs.some(l => l.includes('idb not found'))).toBe(true)
        expect(mockQuerySentinel).not.toHaveBeenCalled()
    })

    it('fails when idb is present but the sentinel never carries the new id', async () => {
        mockScrape.mockResolvedValue('build-5-ios')
        mockIdbAvailable.mockResolvedValue(true)
        mockQuerySentinel.mockResolvedValue(null)
        const p = assertUpdateIsLive('UDID', 'build-5-ios', throwingFail, noopLog)
        const expectation = expect(p).rejects.toThrow(/on-screen sentinel proof missing/)
        await vi.runAllTimersAsync()
        await expectation
    })

    it('tolerates a transient scrape error then succeeds on retry', async () => {
        mockScrape.mockRejectedValueOnce(new Error('simctl blip')).mockResolvedValue('build-5-ios')
        mockIdbAvailable.mockResolvedValue(false)
        const logs: string[] = []
        await runWithTimers(
            assertUpdateIsLive('UDID', 'build-5-ios', throwingFail, m => logs.push(m))
        )
        expect(logs.some(l => l.includes('boot-log proof'))).toBe(true)
    })
})
