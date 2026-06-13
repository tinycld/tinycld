import { describe, expect, it } from 'vitest'
import { extractCurrentIds, pollForBundleId } from '../logs-poller'

// Mirrors the real GET /api/logs response: a PB list with items whose slog
// attrs live under `data`, keyed literally `q.currentId`.
const logsResponse = {
    page: 1,
    perPage: 20,
    totalItems: 2,
    totalPages: 1,
    items: [
        {
            id: 'b',
            created: '2026-06-12 10:00:01.000Z',
            level: 0,
            message: 'app-update: request',
            data: { 'q.platform': 'ios', 'q.currentId': 'build-1718200000000-ios' },
        },
        {
            id: 'a',
            created: '2026-06-12 10:00:00.000Z',
            level: 0,
            message: 'app-update: request',
            data: { 'q.platform': 'ios', 'q.currentId': 'embedded-1.13.7' },
        },
    ],
}

describe('extractCurrentIds', () => {
    it('pulls data["q.currentId"] from every item that has one', () => {
        expect(extractCurrentIds(logsResponse)).toEqual([
            'build-1718200000000-ios',
            'embedded-1.13.7',
        ])
    })
    it('skips items lacking a q.currentId and tolerates an empty list', () => {
        expect(
            extractCurrentIds({ items: [{ data: { 'q.platform': 'ios' } }, { data: {} }] })
        ).toEqual([])
        expect(extractCurrentIds({ items: [] })).toEqual([])
    })
})

describe('pollForBundleId', () => {
    const noSleep = () => Promise.resolve()

    it('resolves once the target id appears', async () => {
        let calls = 0
        const fetchCurrentIds = () => {
            calls++
            // first poll: only embedded; second poll: target shows up
            return Promise.resolve(
                calls < 2 ? ['embedded-1.13.7'] : ['build-x-ios', 'embedded-1.13.7']
            )
        }
        await expect(
            pollForBundleId({
                fetchCurrentIds,
                target: 'build-x-ios',
                timeoutMs: 1000,
                intervalMs: 1,
                sleep: noSleep,
            })
        ).resolves.toBe('build-x-ios')
    })

    it('rejects on timeout, naming the last-seen ids', async () => {
        const fetchCurrentIds = () => Promise.resolve(['embedded-1.13.7'])
        await expect(
            pollForBundleId({
                fetchCurrentIds,
                target: 'never',
                timeoutMs: 5,
                intervalMs: 1,
                sleep: noSleep,
            })
        ).rejects.toThrow(/timed out/i)
        await expect(
            pollForBundleId({
                fetchCurrentIds,
                target: 'never',
                timeoutMs: 5,
                intervalMs: 1,
                sleep: noSleep,
            })
        ).rejects.toThrow(/embedded-1.13.7/)
    })
})
