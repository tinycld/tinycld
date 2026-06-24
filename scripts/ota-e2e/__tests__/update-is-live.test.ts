import { describe, expect, it, vi } from 'vitest'

// assertUpdateIsLive delegates the wait loop to pollForBundleId, fed by
// fetchBootBeaconIds. Mock pollForBundleId directly so the success/timeout
// branches are deterministic and instant — the poller's own loop is covered by
// logs-poller's tests.
const pollForBundleId = vi.hoisted(() => vi.fn())
vi.mock('../logs-poller', () => ({ pollForBundleId }))
vi.mock('../boot-beacon-poller', () => ({ fetchBootBeaconIds: vi.fn() }))

import { assertUpdateIsLive } from '../update-is-live'

describe('assertUpdateIsLive', () => {
    it('passes when the boot beacon for the new id appears', async () => {
        pollForBundleId.mockImplementation(async () => 'build-9-ios')
        const fail = vi.fn<(msg: string) => never>()
        const logs: string[] = []
        await assertUpdateIsLive('http://s', 'tok', 'build-9-ios', fail, m => logs.push(m))
        expect(fail).not.toHaveBeenCalled()
        expect(logs.some(l => l.includes('boot-beacon proof'))).toBe(true)
    })

    it('fails when the beacon never arrives (poll rejects)', async () => {
        pollForBundleId.mockImplementation(async () => {
            throw new Error('timed out')
        })
        const fail = vi.fn<(msg: string) => never>()
        await assertUpdateIsLive('http://s', 'tok', 'build-9-ios', fail, () => {})
        expect(fail).toHaveBeenCalledWith(expect.stringMatching(/boot-beacon proof missing/))
    })
})
