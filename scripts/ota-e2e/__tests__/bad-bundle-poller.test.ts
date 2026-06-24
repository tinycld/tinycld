import { describe, expect, it, vi } from 'vitest'
import {
    type BadBundleRow,
    collectionExists,
    extractBadBundles,
    pollForBadBundle,
} from '../bad-bundle-poller'

describe('extractBadBundles', () => {
    it('returns rows with bundle_id, reports, and last_error', () => {
        const rows = extractBadBundles({
            items: [
                {
                    bundle_id: 'build-1-ios',
                    reports: 2,
                    last_error: 'native rollback: crash-launch counter tripped (launches=2)',
                },
                { bundle_id: 'build-2-ios', reports: 1, last_error: '' },
            ],
        })
        expect(rows).toEqual([
            {
                bundleId: 'build-1-ios',
                reports: 2,
                lastError: 'native rollback: crash-launch counter tripped (launches=2)',
            },
            { bundleId: 'build-2-ios', reports: 1, lastError: '' },
        ])
    })

    it('tolerates a missing items array', () => {
        expect(extractBadBundles({})).toEqual([])
    })
})

describe('pollForBadBundle', () => {
    const noSleep = () => Promise.resolve()

    it('resolves with the row once the target bundle reports a non-empty last_error', async () => {
        const responses: BadBundleRow[][] = [
            [],
            [{ bundleId: 'build-9-ios', reports: 1, lastError: '' }],
            [
                {
                    bundleId: 'build-9-ios',
                    reports: 1,
                    lastError: 'native rollback: crash-launch counter tripped (launches=2)',
                },
            ],
        ]
        const fetchRows = vi.fn(() => Promise.resolve(responses.shift() ?? []))
        const row = await pollForBadBundle({
            fetchRows,
            target: 'build-9-ios',
            timeoutMs: 10_000,
            intervalMs: 1_000,
            sleep: noSleep,
        })
        expect(row.lastError).toContain('crash-launch counter tripped')
    })

    it('rejects on timeout, surfacing the last-seen rows', async () => {
        const fetchRows = vi.fn(() =>
            Promise.resolve([{ bundleId: 'build-9-ios', reports: 1, lastError: '' }])
        )
        await expect(
            pollForBadBundle({
                fetchRows,
                target: 'build-9-ios',
                timeoutMs: 2_000,
                intervalMs: 1_000,
                sleep: noSleep,
            })
        ).rejects.toThrow(/timed out.*build-9-ios/)
    })
})

describe('collectionExists', () => {
    it('returns true on HTTP 200 (known collection)', async () => {
        const fetchFn = vi.fn(() => Promise.resolve({ status: 200, ok: true } as Response))
        expect(await collectionExists('http://x', 'tok', 'booking_pages', fetchFn)).toBe(true)
    })
    it('returns false on HTTP 404 (unknown collection)', async () => {
        const fetchFn = vi.fn(() => Promise.resolve({ status: 404, ok: false } as Response))
        expect(await collectionExists('http://x', 'tok', 'booking_pages', fetchFn)).toBe(false)
    })
    it('returns null on any other status (transient)', async () => {
        const fetchFn = vi.fn(() => Promise.resolve({ status: 503, ok: false } as Response))
        expect(await collectionExists('http://x', 'tok', 'booking_pages', fetchFn)).toBeNull()
    })
})
