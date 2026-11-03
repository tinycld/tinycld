import { describe, expect, it } from 'vitest'
import { deliveryStatus } from '../delivery-status'

const run = (rule: string, fired_at: string, status: string, error = '') => ({
    rule,
    fired_at,
    error,
    results: [{ ref: 'core:send-email', status }],
})

describe('deliveryStatus', () => {
    it('reads each destination by its newest run for the submission', () => {
        const runs = [
            run('a', '2026-11-01 10:00:00.000Z', 'error'),
            run('a', '2026-11-01 11:00:00.000Z', 'ok'),
            run('b', '2026-11-01 10:00:00.000Z', 'ok'),
            run('b', '2026-11-01 11:00:00.000Z', 'error'),
        ]
        expect(deliveryStatus(['a', 'b'], runs)).toEqual({ delivered: 1, total: 2 })
    })

    it('counts a run error as not delivered', () => {
        const runs = [{ ...run('a', '2026-11-01 10:00:00.000Z', 'ok'), error: 'invalid actions' }]
        expect(deliveryStatus(['a'], runs)).toEqual({ delivered: 0, total: 1 })
    })

    // A destination added after the submission, or whose old runs were
    // pruned, has no run for it: that is not a failed delivery.
    it('leaves out a destination with no run for the submission', () => {
        const runs = [run('a', '2026-11-01 10:00:00.000Z', 'ok')]
        expect(deliveryStatus(['a', 'later'], runs)).toEqual({ delivered: 1, total: 1 })
    })
})
