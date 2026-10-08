import { REALTIME_PROTOCOL, realtimeProtocols } from '@tinycld/core/lib/realtime/protocols'
import { describe, expect, it } from 'vitest'

describe('realtimeProtocols', () => {
    it("carries the signed-in user's auth token", () => {
        expect(realtimeProtocols({ authToken: 'a.b.c' })).toEqual([
            REALTIME_PROTOCOL,
            'tinycld.auth.a.b.c',
        ])
    })

    it('prefers a share-link session over the auth token', () => {
        expect(realtimeProtocols({ authToken: 'a.b.c', shareSession: 's.t.u' })).toEqual([
            REALTIME_PROTOCOL,
            'tinycld.share.s.t.u',
        ])
    })

    it('offers only the base protocol without a credential', () => {
        expect(realtimeProtocols({ authToken: '' })).toEqual([REALTIME_PROTOCOL])
    })
})
